package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/scan"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestHealthcheckOKAndFailure(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer ok.Close()
	if err := healthcheck(ok.URL + "/healthz"); err != nil {
		t.Fatalf("healthy server: %v", err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 500) }))
	defer bad.Close()
	if err := healthcheck(bad.URL + "/healthz"); err == nil {
		t.Fatal("500 should fail")
	}
	if err := healthcheck("http://127.0.0.1:1/healthz"); err == nil {
		t.Fatal("refused connection should fail")
	}
}

func TestHealthURLFromAddr(t *testing.T) {
	for in, want := range map[string]string{
		":8080":          "http://127.0.0.1:8080/healthz",
		"0.0.0.0:9000":   "http://127.0.0.1:9000/healthz",
		"127.0.0.1:7000": "http://127.0.0.1:7000/healthz",
	} {
		if got := healthURL(in); got != want {
			t.Errorf("healthURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestServerRegistersOwnerEnvironment(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	owner, err := st.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("REET_CONTROLLER_URL", "")
	t.Setenv("REET_ENV", "test-server")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reportServer(ctx, st)
	envs, err := st.SyncEnvironmentsFor(owner)
	if err != nil || len(envs) != 1 || envs[0].Agent != "reet-server" || envs[0].Host != "test-server" || envs[0].OS == "" {
		t.Fatalf("missing server identity: %+v %v", envs, err)
	}
}

func TestBackupOpenWithAPasswordOrTheKey(t *testing.T) {
	sealer, err := backup.NewSealer(strings.Repeat("k", 32), "test")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("the shelf, in plain sight")
	m := backup.Manifest{ID: "user-1-20260101T000000Z-aaaaaaaa", Kind: "user", OwnerID: 1, Target: "user-1"}
	for _, tc := range []struct {
		name     string
		opts     backup.SealOptions
		args     []string
		envKey   string
		wantFail bool
	}{
		{name: "password", opts: backup.SealOptions{Compress: true, Password: "a copy password long enough"}, args: []string{"--password", "a copy password long enough"}},
		{name: "password wrong", opts: backup.SealOptions{Password: "a copy password long enough"}, args: []string{"--password", "the wrong password here"}, wantFail: true},
		{name: "key", opts: backup.SealOptions{Compress: true}, envKey: strings.Repeat("k", 32)},
		{name: "key missing", opts: backup.SealOptions{Compress: true}, wantFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sealed, err := sealer.SealArchive(m, payload, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "copy.reetbackup")
			if err := os.WriteFile(file, sealed, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(backup.EnvKey, tc.envKey)
			var out bytes.Buffer
			err = runBackup(append([]string{"open"}, append(tc.args, file)...), &out)
			if tc.wantFail {
				if err == nil {
					t.Fatal("opened when it should not")
				}
				return
			}
			if err != nil || !bytes.Equal(out.Bytes(), payload) {
				t.Fatalf("open: %v, %q", err, out.String())
			}
		})
	}
}

func TestBackupUnescrowPrintsAKeyAndOneDestination(t *testing.T) {
	escrow := backup.Escrow{
		Key: strings.Repeat("k", 32),
		Destinations: []backup.EscrowRemote{
			{Name: "offsite", Endpoint: "https://s3.example", Bucket: "b", AccessKey: "ak", SecretKey: "sk", Keep: 5},
			{Name: "other", Endpoint: "https://other.example", Bucket: "b2", AccessKey: "ak2", SecretKey: "sk2", Keep: 2},
		},
	}
	sealed, err := backup.SealEscrow(escrow, "an escrow passphrase long enough")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "escrow.sealed")
	if err := os.WriteFile(file, sealed, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runBackup([]string{"unescrow", "--in", file, "--passphrase", "an escrow passphrase long enough"}, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"REET_BACKUP_KEY=" + strings.Repeat("k", 32),
		"REET_BACKUP_S3_NAME=offsite",
		"REET_BACKUP_S3_SECRET_ACCESS_KEY=sk",
		"# other destinations: other",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("unescrow output is missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "sk2") {
		t.Fatal("unescrow printed the other destination's secret")
	}
}

func TestHealthcheckCLIUsesExplicitURLAndRejectsIgnoredArguments(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { paths = append(paths, r.URL.Path); w.WriteHeader(200) }))
	defer server.Close()
	for _, suffix := range []string{"", "/", "/reet", "/healthz", "/reet/healthz", "/healthz/"} {
		if err := runHealthcheck([]string{"--url", server.URL + suffix}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(paths, ",") != "/healthz,/healthz,/reet/healthz,/healthz,/reet/healthz,/healthz" {
		t.Fatalf("wrong targets: %v", paths)
	}
	before := len(paths)
	for _, args := range [][]string{{"--unknown"}, {"unexpected"}, {"--url", "file:///tmp/healthz"}, {"--url", "reet.sh"}, {"--url", "https://user:secret@example.test"}, {"--url", "https://example.test/?secret=x"}} {
		if err := runHealthcheck(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if err := runHealthcheck([]string{"--help"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != before {
		t.Fatal("invalid arguments or help made requests")
	}
	t.Setenv("REET_ADDR", strings.TrimPrefix(server.URL, "http://"))
	if err := runHealthcheck(nil, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != before+1 || paths[len(paths)-1] != "/healthz" {
		t.Fatal("default local healthcheck changed")
	}
}

func TestHealthcheckTargetEnvironmentAndLocalOverride(t *testing.T) {
	localRequests, remoteRequests := 0, 0
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { localRequests++; w.WriteHeader(200) }))
	defer local.Close()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { remoteRequests++; w.WriteHeader(200) }))
	defer remote.Close()
	t.Setenv("REET_ADDR", strings.TrimPrefix(local.URL, "http://"))
	t.Setenv("REET_URL", remote.URL)
	for _, args := range [][]string{nil, {"--local"}, {"--url", local.URL}} {
		if err := runHealthcheck(args, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	if localRequests != 2 || remoteRequests != 1 {
		t.Fatalf("wrong targets local=%d remote=%d", localRequests, remoteRequests)
	}
	if err := runHealthcheck([]string{"--local", "--url", remote.URL}, &bytes.Buffer{}); err == nil {
		t.Fatal("accepted ambiguous target")
	}
	t.Setenv("REET_ADDR", "")
	t.Setenv("REET_URL", "")
	if err := runHealthcheck(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("missing noninteractive target should explain configuration")
	}
}

func TestVersionAnswersTheAgentUpdateProtocolProbe(t *testing.T) {
	t.Setenv("REET_AGENT_UPDATE_PROTOCOL", "1")
	var out bytes.Buffer
	printVersion(&out, "v1.2.3", "abc")
	if strings.TrimSpace(out.String()) != scan.AgentUpdateProtocol {
		t.Fatalf("probe answer: %q", out.String())
	}
	t.Setenv("REET_AGENT_UPDATE_PROTOCOL", "")
	out.Reset()
	printVersion(&out, "v1.2.3", "abc")
	if strings.TrimSpace(out.String()) != "reet v1.2.3 (abc)" {
		t.Fatalf("version line: %q", out.String())
	}
}
