package scan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillbundle"
)

func TestObservationNegotiatesBeforeReading(t *testing.T) {
	for _, status := range []int{404, 405, 200, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Reet-Agent") != "agent" || r.Header.Get("X-Reet-Agent-Key") != "secret" {
					t.Error("missing enrollment")
				}
				if status != 200 {
					w.WriteHeader(status)
					return
				}
				json.NewEncoder(w).Encode(contentobs.Grant{SchemaVersion: 99, ScanID: strings.Repeat("a", 48), ExpiresAt: time.Now().Add(time.Minute).UnixMilli()})
			}))
			defer srv.Close()
			calls := 0
			err := reportContentObservationsWith(context.Background(), agentConfig{Server: srv.URL, ID: "agent", Secret: "secret"}, "test", func(context.Context, agentConfig, contentobs.Grant) contentobs.Report {
				calls++
				return contentobs.Report{}
			})
			if calls != 0 {
				t.Fatal("read before capability negotiated")
			}
			if status == 403 && err == nil {
				t.Fatal("authorization failure hidden")
			}
			if status != 403 && err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestObservationExactBytesMetadataOnlyAndUnsafeFiles(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".agents", "skills", "demo")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("---\r\nname: demo\r\n---\r\nPRIVATE_INSTRUCTIONS\r\n")
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data.bin"), []byte{0, 255, 1}, 0700); err != nil {
		t.Fatal(err)
	}
	c := agentConfig{Home: home, Host: "laptop"}
	g := contentobs.Grant{SchemaVersion: 1, ScanID: strings.Repeat("a", 48), ExpiresAt: time.Now().Add(time.Minute).UnixMilli()}
	report := collectContentObservations(context.Background(), c, g)
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(report.Scopes) != 1 || len(report.Scopes[0].Skills) != 1 {
		t.Fatal(report)
	}
	m, err := skillbundle.Scan(root, skillbundle.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Scopes[0].Skills[0].Digest != m.Digest {
		t.Fatal("digest lost byte/mode parity")
	}
	raw, _ := json.Marshal(report)
	for _, secret := range []string{"PRIVATE_INSTRUCTIONS", "SKILL.md", "data.bin", root} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("report includes private data %s", secret)
		}
	}
	if err = os.Symlink(filepath.Join(root, "data.bin"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	report = collectContentObservations(context.Background(), c, g)
	if err = report.Validate(); err != nil {
		t.Fatal(err)
	}
	v := report.Scopes[0].Skills[0]
	if v.Complete || v.Digest != "" || report.Scopes[0].Complete || len(v.Issues) == 0 {
		t.Fatal("unsafe scan became comparable", report)
	}
	after, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil || string(after) != string(original) {
		t.Fatal("scan modified contents", err)
	}
}
func TestObservationNegotiatedReportAndRedirectDoesNotLeak(t *testing.T) {
	calls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; t.Error("followed redirect with credentials") }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	if err := reportContentObservations(context.Background(), agentConfig{Server: source.URL, Secret: "secret"}, "test"); err == nil || calls != 0 {
		t.Fatal(err, calls)
	}
	var received contentobs.Report
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/prepare") {
			json.NewEncoder(w).Encode(contentobs.Grant{SchemaVersion: 1, ScanID: strings.Repeat("a", 48), ExpiresAt: time.Now().Add(time.Minute).UnixMilli()})
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	if err := reportContentObservations(context.Background(), agentConfig{Server: srv.URL, Home: t.TempDir()}, "test"); err != nil {
		t.Fatal(err)
	}
	if received.SchemaVersion != 1 || received.Scopes == nil {
		t.Fatal(received)
	}
}
