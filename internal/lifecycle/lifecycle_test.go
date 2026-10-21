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
