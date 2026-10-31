package lifecycle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func archive(t *testing.T, names []string, kind byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		data := []byte("new binary")
		h := &tar.Header{Name: name, Typeflag: kind, Mode: 0755, Size: int64(len(data))}
		if kind != tar.TypeReg {
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if kind == tar.TypeReg {
			tw.Write(data)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}
func releaseClient(t *testing.T, tag string, blob []byte, mutate func(string) string) *http.Client {
	t.Helper()
	sum := sha256.Sum256(blob)
	name := fmt.Sprintf("reet_%s_%s_%s.tar.gz", tag, runtime.GOOS, runtime.GOARCH)
	manifest := fmt.Sprintf("%x  %s\n", sum, name)
	if mutate != nil {
		manifest = mutate(manifest)
	}
	return &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/latest"), strings.HasSuffix(r.URL.Path, "/tags/"+tag):
			return response(200, `{"tag_name":"`+tag+`"}`), nil
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			return response(200, manifest), nil
		case strings.HasSuffix(r.URL.Path, "/"+name):
			return response(200, string(blob)), nil
		case strings.HasSuffix(r.URL.Path, "/healthz"):
			return response(200, `{"ok":true,"version":"`+tag+`"}`), nil
		}
		return response(404, ""), nil
	})}
}
func file(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
}
func contents(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func testApp(t *testing.T) (*App, string, string) {
	t.Helper()
	t.Setenv("REET_VERSION", "")
	root := t.TempDir()
	binary := filepath.Join(root, "bin", "reet")
	file(t, binary, "old binary")
	a := New("v1.0.0", "old", strings.NewReader(""), &bytes.Buffer{})
	a.executable = func() (string, error) { return binary, nil }
	a.client = releaseClient(t, "v2.0.0", archive(t, []string{"reet"}, tar.TypeReg), nil)
	a.run = func(_ context.Context, name string, args ...string) (string, error) {
		if strings.Contains(name, ".reet-download-") {
			return "reet v2.0.0 (new)\n", nil
		}
		return "", fmt.Errorf("unexpected host command %s", name)
	}
	return a, filepath.Join(root, "state"), binary
}
func TestReleaseVerification(t *testing.T) {
	for _, tc := range []struct {
		name   string
		names  []string
		kind   byte
		mutate func(string) string
		want   bool
	}{
		{"valid", []string{"reet"}, tar.TypeReg, nil, true},
		{"checksum", []string{"reet"}, tar.TypeReg, func(s string) string { return strings.Repeat("0", 64) + s[64:] }, false},
		{"duplicate manifest", []string{"reet"}, tar.TypeReg, func(s string) string { return s + s }, false},
		{"duplicate binary", []string{"reet", "reet"}, tar.TypeReg, nil, false},
		{"traversal", []string{"../reet"}, tar.TypeReg, nil, false},
		{"symlink", []string{"reet"}, tar.TypeSymlink, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := releaseClient(t, "dev", archive(t, tc.names, tc.kind), tc.mutate)
			path, err := fetchBinary(context.Background(), client, "https://example.invalid", "dev", t.TempDir())
			if (err == nil) != tc.want {
				t.Fatalf("path=%s err=%v", path, err)
			}
			if tc.want && contents(t, path) != "new binary" {
				t.Fatal("wrong executable")
			}
		})
	}
}
func TestUpgradeStandaloneCheckDryRunConfirmationAndApply(t *testing.T) {
	for _, mode := range []string{"--check", "--dry-run", "cancel", "--yes"} {
		t.Run(mode, func(t *testing.T) {
			a, state, binary := testApp(t)
			args := []string{"--state-dir", state}
			if mode != "cancel" {
				args = append(args, mode)
			}
			err := a.Run("upgrade", args)
			if mode == "cancel" {
				if err == nil {
					t.Fatal("confirmation not required")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if mode == "--yes" {
				if contents(t, binary) != "new binary" || contents(t, binary+".previous") != "old binary" {
					t.Fatal("binary not atomically backed up/replaced")
				}
				d, e := load(state)
				if e != nil || d.Version != "v2.0.0" {
					t.Fatalf("manifest %+v %v", d, e)
				}
			} else {
				if contents(t, binary) != "old binary" {
					t.Fatal("preview modified installed binary")
				}
				if _, e := os.Stat(state); !os.IsNotExist(e) {
					t.Fatal("preview created deployment state")
				}
			}
		})
	}
}
func TestFailedDownloadNeverTouchesInstall(t *testing.T) {
	a, state, binary := testApp(t)
	a.client = releaseClient(t, "v2.0.0", archive(t, []string{"reet"}, tar.TypeReg), func(s string) string { return s + s })
	if err := a.Run("upgrade", []string{"--state-dir", state, "--yes"}); err == nil {
		t.Fatal("accepted invalid release")
	}
	if contents(t, binary) != "old binary" {
		t.Fatal("changed installed binary")
	}
}
func TestLiveWALBackupAndRestore(t *testing.T) {
	data := t.TempDir()
	state := t.TempDir()
	path := filepath.Join(data, "reet.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("PRAGMA journal_mode=WAL; CREATE TABLE example(value TEXT); INSERT INTO example VALUES('before')"); err != nil {
		t.Fatal(err)
	}
	backup, err := backupDB(context.Background(), data, state)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE example SET value='after'")
	db.Close()
	if err = restoreDB(backup, data); err != nil {
		t.Fatal(err)
	}
	restored, err := openReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var got string
	if err = restored.QueryRow("SELECT value FROM example").Scan(&got); err != nil || got != "before" {
		t.Fatalf("snapshot %s %v", got, err)
	}
	if info, _ := os.Stat(backup); info.Mode().Perm() != 0600 {
		t.Fatal("backup exposes database")
	}
}
func TestNativeUpgradeRollbackAndStoppedState(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd runner test")
	}
	for _, running := range []bool{true, false} {
		t.Run(fmt.Sprint(running), func(t *testing.T) {
			a, state, binary := testApp(t)
			data := filepath.Join(t.TempDir(), "data")
			os.MkdirAll(data, 0700)
			db, _ := sql.Open("sqlite", filepath.Join(data, "reet.db"))
			db.Exec("CREATE TABLE sample(value TEXT); INSERT INTO sample VALUES('old')")
			db.Close()
			os.MkdirAll(state, 0700)
			d := &deployment{Mode: "native", Binary: binary, Data: data, Addr: "127.0.0.1:8080", Version: "v1.0.0"}
			writeJSON(filepath.Join(state, "deployment.json"), d)
			starts := 0
			stops := 0
			a.run = func(_ context.Context, name string, args ...string) (string, error) {
				if name != "systemctl" {
					return "reet v2.0.0 (new)", nil
				}
				switch args[1] {
				case "show":
					if running {
						return "active", nil
					}
					return "inactive", nil
				case "stop":
					stops++
					return "", nil
				case "start":
					starts++
					if starts == 1 {
						db, _ := sql.Open("sqlite", filepath.Join(data, "reet.db"))
						db.Exec("UPDATE sample SET value='migrated'")
						db.Close()
						return "", errors.New("new service failed")
					}
					return "", nil
				}
				return "", errors.New("unexpected command")
			}
			a.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "/healthz") {
					return response(200, `{"ok":true,"version":"v1.0.0"}`), nil
				}
				return releaseClient(t, "v2.0.0", archive(t, []string{"reet"}, tar.TypeReg), nil).Transport.RoundTrip(r)
			})
			err := a.Run("upgrade", []string{"--state-dir", state, "--yes"})
			if running {
				if err == nil || !strings.Contains(err.Error(), "previous deployment restored") {
					t.Fatalf("expected rollback: %v", err)
				}
				if starts != 2 || stops != 2 || contents(t, binary) != "old binary" {
					t.Fatalf("recovery starts=%d stops=%d", starts, stops)
				}
				got, _ := load(state)
				if got.Version != "v1.0.0" {
					t.Fatal("manifest not rolled back")
				}
				db, _ := openReadOnly(filepath.Join(data, "reet.db"))
				defer db.Close()
				var value string
				db.QueryRow("SELECT value FROM sample").Scan(&value)
				if value != "old" {
					t.Fatal("migrated database not restored")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if starts != 0 {
					t.Fatal("started previously stopped service")
				}
				if contents(t, binary) != "new binary" {
					t.Fatal("stopped service not upgraded")
				}
			}
		})
	}
}
func TestComposePinsDigestAndChecksBuildBeforeStopping(t *testing.T) {
	a, state, binary := testApp(t)
	os.MkdirAll(state, 0700)
	d := &deployment{Mode: "compose", Binary: binary, Data: t.TempDir(), Addr: "127.0.0.1:8080", Version: "v1.0.0", Image: imageRepo + "@sha256:" + strings.Repeat("a", 64)}
	writeJSON(filepath.Join(state, "deployment.json"), d)
	a.writeCompose(d, state)
	calls := []string{}
	a.run = func(_ context.Context, name string, args ...string) (string, error) {
		if name != "docker" {
			return "reet v2.0.0 (new)", nil
		}
		call := strings.Join(args, " ")
		calls = append(calls, call)
		switch args[0] {
		case "pull":
			if args[1] != imageRepo+":v2.0.0" {
				t.Fatal("wrong stable image tag")
			}
			return "", nil
		case "image":
			return `["` + imageRepo + `@sha256:` + strings.Repeat("b", 64) + `"]`, nil
		case "run":
			return "reet v2.0.0 (different-commit)", nil
		}
		t.Fatalf("server touched before build verification: %s", call)
		return "", nil
	}
	if err := a.Run("upgrade", []string{"--state-dir", state, "--yes"}); err == nil || !strings.Contains(err.Error(), "builds differ") {
		t.Fatal(err)
	}
	if len(calls) != 3 || contents(t, binary) != "old binary" {
		t.Fatal("build mismatch mutated deployment")
	}
	config, err := composeConfig(&deployment{Data: "/tmp/data$HOME", Addr: "127.0.0.1:9999", Image: "digest", Environment: map[string]string{"REET_TOKEN": "secret$TOKEN"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "secret$$TOKEN") || !strings.Contains(string(config), "127.0.0.1") {
		t.Fatal("Compose interpolates secrets or exposes port")
	}
}
func TestInstallDryRunAndConfirmation(t *testing.T) {
	a, state, binary := testApp(t)
	target := filepath.Join(t.TempDir(), "newbin")
	args := []string{"--state-dir", state, "--dir", target}
	if err := a.Run("install", append(args, "--dry-run")); err != nil {
		t.Fatal(err)
	}
	if _, e := os.Stat(state); !os.IsNotExist(e) {
		t.Fatal("dry run created state")
	}
	if err := a.Run("install", args); err == nil {
		t.Fatal("missing confirmation")
	}
	if err := a.Run("install", append(args, "--yes")); err != nil {
		t.Fatal(err)
	}
	if contents(t, filepath.Join(target, "reet")) != contents(t, binary) {
		t.Fatal("binary install failed")
	}
	if err := a.Run("install", append(args, "--yes")); err == nil {
		t.Fatal("overwrote deployment")
	}
}
func TestDoctorIsReadOnlyAndRedactsCredentials(t *testing.T) {
	a, state, _ := testApp(t)
	output := &bytes.Buffer{}
	a.Out = output
	t.Setenv("REET_TOKEN", "VERY_SECRET")
	t.Setenv("REET_URL", "https://controller.invalid")
	a.client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer VERY_SECRET" {
			t.Fatal("token not sent")
		}
		return response(401, "VERY_SECRET"), nil
	})}
	if err := a.Run("doctor", []string{"--state-dir", state, "--json"}); err == nil {
		t.Fatal("doctor did not fail for rejected token")
	}
	var r report
	if err := json.Unmarshal(output.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "VERY_SECRET") {
		t.Fatal("leaked credentials")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("doctor created deployment state")
	}
	output.Reset()
	if err := a.Run("upgrade", []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "VERY_SECRET") {
		t.Fatal("help leaked credentials")
	}
}
func TestLifecycleLock(t *testing.T) {
	state := t.TempDir()
	unlock, err := acquire(state)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err = acquire(state); err == nil {
		t.Fatal("concurrent operation permitted")
	}
}

func TestComposeUpgradeSuccessAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			a, state, binary := testApp(t)
			os.MkdirAll(state, 0700)
			oldImage := imageRepo + "@sha256:" + strings.Repeat("a", 64)
			newImage := imageRepo + "@sha256:" + strings.Repeat("b", 64)
			d := &deployment{Mode: "compose", Binary: binary, Data: t.TempDir(), Addr: "127.0.0.1:8080", Version: "v1.0.0", Image: oldImage}
			writeJSON(filepath.Join(state, "deployment.json"), d)
			a.writeCompose(d, state)
			starts := 0
			stops := 0
			a.run = func(_ context.Context, name string, args ...string) (string, error) {
				if name != "docker" {
					return "reet v2.0.0 (new)", nil
				}
				switch args[0] {
				case "pull":
					return "", nil
				case "image":
					return `["` + newImage + `"]`, nil
				case "run":
					return "reet v2.0.0 (new)", nil
				case "compose":
					call := strings.Join(args, " ")
					switch {
					case strings.Contains(call, " ps "):
						return "container", nil
					case strings.Contains(call, " stop "):
						stops++
						return "", nil
					case strings.Contains(call, " up "):
						starts++
						config := contents(t, filepath.Join(state, "compose.json"))
						if starts == 1 {
							if !strings.Contains(config, newImage) {
								t.Fatal("new image was not pinned")
							}
							if fail {
								return "", errors.New("failed new container")
							}
						} else if !strings.Contains(config, oldImage) {
							t.Fatal("rollback did not pin old image")
						}
						return "", nil
					}
				}
				return "", errors.New("unexpected command")
			}
			base := a.client.Transport
			a.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "/healthz") && fail {
					return response(200, `{"ok":true,"version":"v1.0.0"}`), nil
				}
				return base.RoundTrip(r)
			})
			err := a.Run("upgrade", []string{"--state-dir", state, "--yes"})
			got, _ := load(state)
			if fail {
				if err == nil || starts != 2 || stops != 2 || got.Image != oldImage || contents(t, binary) != "old binary" {
					t.Fatalf("rollback err=%v starts=%d stops=%d deployment=%+v", err, starts, stops, got)
				}
			} else {
				if err != nil || starts != 1 || got.Image != newImage {
					t.Fatalf("upgrade err=%v deployment=%+v", err, got)
				}
			}
		})
	}
}

func TestUpgradePreservesChannelAndSeparatesBuildFromRecord(t *testing.T) {
	for _, tc := range []struct {
		name, mode, running, recorded, explicit, want string
		separate                                      bool
	}{
		{"fresh dev", "binary", "dev", "", "", "dev", false},
		{"reinstalled dev stale stable record", "binary", "dev", "v1.0.0", "", "dev", false},
		{"native reinstall dev", "native", "dev", "v1.0.0", "", "dev", false},
		{"stable reinstall stale dev record", "binary", "v2.0.0", "dev", "", "v2.0.0", false},
		{"explicit stable from dev", "binary", "dev", "dev", "latest", "v2.0.0", false},
		{"explicit dev from stable", "binary", "v1.0.0", "v1.0.0", "dev", "dev", false},
		{"pinned release", "binary", "dev", "dev", "v1.0.0", "v1.0.0", false},
		{"stable CLI manages dev compose", "compose", "v1.0.0", "dev", "", "dev", false},
		{"dev CLI manages stable compose", "compose", "dev", "v1.0.0", "", "v2.0.0", false},
		{"separate dev CLI stable target", "native", "dev", "v1.0.0", "", "v2.0.0", true},
		{"separate stable CLI dev target", "native", "v1.0.0", "dev", "", "dev", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, state, binary := testApp(t)
			a.Version = tc.running
			if tc.separate {
				separate := filepath.Join(t.TempDir(), "reet")
				file(t, separate, "other binary")
				a.executable = func() (string, error) { return separate, nil }
			}
			if tc.recorded != "" {
				d := deployment{Mode: tc.mode, Binary: binary, Version: tc.recorded, Data: t.TempDir(), Addr: "127.0.0.1:8080"}
				b, _ := json.Marshal(d)
				file(t, filepath.Join(state, "deployment.json"), string(b))
			}
			requests := 0
			a.client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				requests++
				endpoint := "/latest"
				if tc.explicit == "v1.0.0" {
					endpoint = "/tags/v1.0.0"
				}
				if !strings.HasSuffix(r.URL.Path, endpoint) {
					t.Fatalf("wrong channel request: %s", r.URL.Path)
				}
				return response(200, `{"tag_name":"`+tc.want+`"}`), nil
			})}
			args := []string{"--state-dir", state, "--check"}
			if tc.explicit != "" {
				args = append(args, "--version", tc.explicit)
			}
			if err := a.Run("upgrade", args); err != nil {
				t.Fatal(err)
			}
			out := a.Out.(*bytes.Buffer).String()
			wantRequests := 1
			if tc.want == "dev" {
				wantRequests = 0 // dev resolves without the release API
			}
			if requests != wantRequests || !strings.Contains(out, "Running CLI: "+tc.running+" (old)") || !strings.Contains(out, "Requested: "+tc.want) {
				t.Fatalf("wrong preview: %s", out)
			}
			if tc.recorded != "" && !strings.Contains(out, "Recorded deployment version: "+tc.recorded) {
				t.Fatalf("record mixed with binary build: %s", out)
			}
			if contents(t, binary) != "old binary" {
				t.Fatal("check modified binary")
			}
		})
	}
}

func TestDevUpgradeAppliesDespiteStaleStableRecord(t *testing.T) {
	a, state, binary := testApp(t)
	a.Version = "dev"
	d := deployment{Mode: "binary", Binary: binary, Version: "v1.0.0"}
	b, _ := json.Marshal(d)
	file(t, filepath.Join(state, "deployment.json"), string(b))
	a.client = releaseClient(t, "dev", archive(t, []string{"reet"}, tar.TypeReg), nil)
	a.run = func(_ context.Context, name string, args ...string) (string, error) {
		if strings.Contains(name, ".reet-download-") {
			return "reet dev (new)\n", nil
		}
		return "", fmt.Errorf("unexpected command %s", name)
	}
	if err := a.Run("upgrade", []string{"--state-dir", state, "--yes"}); err != nil {
		t.Fatal(err)
	}
	got, err := load(state)
	if err != nil || got.Version != "dev" || contents(t, binary) != "new binary" || contents(t, binary+".previous") != "old binary" {
		t.Fatalf("upgrade failed: %+v %v", got, err)
	}
}

func TestUpgradeVersionEnvironmentPrecedence(t *testing.T) {
	for _, tc := range []struct{ name, env, flag, want string }{
		{"environment dev", "dev", "", "dev"},
		{"environment stable", "latest", "", "latest"},
		{"environment pinned", "v2.3.4", "", "v2.3.4"},
		{"flag overrides dev", "dev", "latest", "latest"},
		{"flag overrides stable", "latest", "dev", "dev"},
		{"flag overrides invalid environment", "invalid", "dev", "dev"},
		{"unset preserves dev", "", "", "dev"},
		{"invalid environment", "../../invalid", "", "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, state, binary := testApp(t)
			a.Version = "dev"
			t.Setenv("REET_VERSION", tc.env)
			requests := 0
			a.client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				requests++
				if tc.want == "error" {
					t.Fatal("invalid environment reached network")
				}
				endpoint := "/tags/" + tc.want
				resolved := tc.want
				if tc.want == "latest" {
					endpoint = "/latest"
					resolved = "v9.0.0"
				}
				if !strings.HasSuffix(r.URL.Path, endpoint) {
					t.Fatalf("wrong request: %s", r.URL.Path)
				}
				return response(200, `{"tag_name":"`+resolved+`"}`), nil
			})}
			args := []string{"--state-dir", state, "--check"}
			if tc.flag != "" {
				args = append(args, "--version", tc.flag)
			}
			err := a.Run("upgrade", args)
			if tc.want == "error" {
				if err == nil || requests != 0 {
					t.Fatalf("invalid environment accepted: %v", err)
				}
			} else if wantRequests := map[bool]int{true: 0, false: 1}[tc.want == "dev"]; err != nil || requests != wantRequests {
				t.Fatalf("upgrade: %v requests=%d", err, requests)
			}
			if contents(t, binary) != "old binary" {
				t.Fatal("check changed installation")
			}
		})
	}
}

func TestComposeInstallUsesVersionEnvironment(t *testing.T) {
	for _, explicit := range []string{"", "latest"} {
		t.Run("flag="+explicit, func(t *testing.T) {
			a, state, _ := testApp(t)
			t.Setenv("REET_VERSION", "dev")
			args := []string{"--mode", "compose", "--state-dir", state, "--dir", t.TempDir(), "--data-dir", t.TempDir(), "--dry-run"}
			want := "dev"
			if explicit != "" {
				args = append(args, "--version", explicit)
				want = explicit
			}
			if err := a.Run("install", args); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(a.Out.(*bytes.Buffer).String(), "Image channel: "+want) {
				t.Fatal("environment precedence ignored")
			}
			if _, err := os.Stat(state); !os.IsNotExist(err) {
				t.Fatal("preview changed state")
			}
		})
	}
}

func TestDevDownloadUsesOneImmutableBuild(t *testing.T) {
	sha := strings.Repeat("a", 40)
	blob := archive(t, []string{"reet"}, tar.TypeReg)
	name := fmt.Sprintf("reet_dev_%s_%s_%s.tar.gz", sha, runtime.GOOS, runtime.GOARCH)
	calls := []string{}
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/dev/dev-build.txt":
			return response(200, sha+"\n"), nil
		case "/dev/checksums-" + sha + ".txt":
			return response(200, fmt.Sprintf("%x  %s\n", sha256.Sum256(blob), name)), nil
		case "/dev/" + name:
			return response(200, string(blob)), nil
		}
		return response(404, ""), nil
	})}
	path, err := fetchBinary(context.Background(), client, "https://example.invalid", "dev", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if contents(t, path) != "new binary" {
		t.Fatal("wrong binary")
	}
	if len(calls) != 3 {
		t.Fatalf("unexpected downloads: %v", calls)
	}
	for _, bad := range []string{"../other", strings.Repeat("a", 39), ""} {
		client.Transport = transport(func(r *http.Request) (*http.Response, error) { return response(200, bad), nil })
		if _, err := fetchBinary(context.Background(), client, "https://example.invalid", "dev", t.TempDir()); err == nil {
			t.Fatalf("accepted pointer %q", bad)
		}
	}
	client.Transport = transport(func(r *http.Request) (*http.Response, error) { return response(503, ""), nil })
	if _, err := fetchBinary(context.Background(), client, "https://example.invalid", "dev", t.TempDir()); err == nil {
		t.Fatal("pointer outage silently fell back to mutable archives")
	}
}

// The dev release accumulates one asset set per build, so its API response
// outgrows any fixed cap. The channel name is already the tag, so resolving it
// must not depend on that response.
func TestResolveDevDoesNotQueryReleaseAPI(t *testing.T) {
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("dev must not query the release API: %s", r.URL)
		return nil, nil
	})}
	got, err := resolveRelease(context.Background(), client, "https://api.example.invalid/releases", "dev")
	if err != nil || got != "dev" {
		t.Fatalf("got %q, %v", got, err)
	}
}
