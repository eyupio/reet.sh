//go:build linux || darwin

package scan

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

func importLocalFixture(t *testing.T, server string) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	host, _ := os.Hostname()
	cfg := filepath.Join(t.TempDir(), "agent.json")
	if err := writePrivate(cfg, agentConfig{Home: home, Hostname: host, Host: "fixture", ID: "enrollment", Secret: "secret", Server: server, Token: "personal"}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".agents", "skills", "demo")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\r\nname: demo\r\ndescription: Useful instructions\r\n---\r\nPrivate machine instructions.\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return cfg, root
}
func TestSkillImportPreviewCancellationAndOldController(t *testing.T) {
	gets, posts := 0, 0
	unsupported := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			t.Error("unexpected file upload")
			return
		}
		gets++
		if unsupported {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"schemaVersion":1}`)
	}))
	defer srv.Close()
	cfg, root := importLocalFixture(t, srv.URL)
	args := []string{"skill", "import", "--config", cfg, "--name", "demo"}
	var out bytes.Buffer
	if err := runAgentInput(args, strings.NewReader(""), &out, "test"); err != nil {
		t.Fatal(err)
	}
	if gets != 0 || posts != 0 || !strings.Contains(out.String(), "Preview only") || strings.Contains(out.String(), "Private machine instructions") {
		t.Fatal(out.String(), gets, posts)
	}
	if err := runAgentInput(append(args, "--upload"), strings.NewReader("\n"), &out, "test"); err != nil {
		t.Fatal(err)
	}
	if gets != 1 || posts != 0 || !strings.Contains(out.String(), "Import cancelled") {
		t.Fatal(out.String(), gets, posts)
	}
	unsupported = true
	if err := runAgentInput(append(args, "--upload", "--yes"), strings.NewReader(""), &out, "test"); err == nil || posts != 0 {
		t.Fatal("old controller payload", err, posts)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DO_NOT_UPLOAD=private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runAgentInput(args, strings.NewReader(""), &out, "test"); err == nil {
		t.Fatal("credential file accepted")
	}
	if err := runAgentInput(append(args, "--exclude", ".env"), strings.NewReader(""), &out, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Excluded: .env") || strings.Contains(out.String(), "DO_NOT_UPLOAD") {
		t.Fatal(out.String())
	}
}

type changingImportReader struct {
	path    string
	changed bool
}

func (r *changingImportReader) Read(p []byte) (int, error) {
	if r.changed {
		return 0, io.EOF
	}
	r.changed = true
	if err := os.WriteFile(r.path, []byte("Changed after review"), 0600); err != nil {
		return 0, err
	}
	return copy(p, "IMPORT\n"), nil
}
func TestSkillImportChangedDuringConfirmationAndScopeBoundaries(t *testing.T) {
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
		}
		io.WriteString(w, `{"schemaVersion":1}`)
	}))
	defer srv.Close()
	cfg, root := importLocalFixture(t, srv.URL)
	err := runAgentInput([]string{"skill", "import", "--config", cfg, "--name", "demo", "--upload"}, &changingImportReader{path: filepath.Join(root, "SKILL.md")}, io.Discard, "test")
	if err == nil || posts != 0 {
		t.Fatal("changed preview dispatched", err, posts)
	}
	c, err := loadAgentConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ agent, project, name string }{{"agents", "", "../secret"}, {"agents", t.TempDir(), "demo"}, {"other", "", "demo"}, {"cursor", t.TempDir(), "demo"}} {
		if _, _, err := skillImportRoot(c, tc.agent, tc.project, tc.name); err == nil {
			t.Fatal("untrusted root", tc)
		}
	}
}
func TestSkillImportTwoEnrolledMachinesVerifiedRetryAndIsolation(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	if _, err = st.CreateAPIToken(uid, "import", auth.HashToken("personal")); err != nil {
		t.Fatal(err)
	}
	server := api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthAccounts))
	defer server.Close()
	srv := httptest.NewServer(server)
	defer srv.Close()
	var revisions []store.PrivateSkillResult
	for _, machine := range []string{"first", "second"} {
		cfg, root := importLocalFixture(t, srv.URL)
		c, err := loadAgentConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}
		a, key, err := st.EnrollSyncAgent(uid, machine, "test")
		if err != nil {
			t.Fatal(err)
		}
		c.Host = machine
		c.ID = a.ID
		c.Secret = key
		if err = writePrivate(cfg, c); err != nil {
			t.Fatal(err)
		}
		if err = st.TouchEnvironmentFor(uid, machine, "agents", "", ""); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "data.bin"), []byte(machine+"\x00\xff"), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, ".env"), []byte("PRIVATE_KEY=excluded"), 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"skill", "import", "--config", cfg, "--name", "demo", "--exclude", ".env", "--upload", "--yes", "--request-id", "import_request_" + machine}
		var out bytes.Buffer
		if err = runAgentInput(args, strings.NewReader(""), &out, "test"); err != nil {
			t.Fatal(err, out.String())
		}
		// A lost response is recovered by the exact same request, without another revision.
		if err = runAgentInput(args, strings.NewReader(""), io.Discard, "test"); err != nil {
			t.Fatal(err)
		}
		bundles, err := st.PrivateSkillBundlesFor(uid)
		if err != nil {
			t.Fatal(err)
		}
		var newest store.PrivateSkillResult
		for _, b := range bundles {
			r, err := st.PrivateSkillRevisionFor(uid, b.ID, b.Head)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(r.Origin.Reference, a.ID) {
				newest = store.PrivateSkillResult{Bundle: b, Revision: r}
			}
		}
		if newest.Bundle.ID == "" || len(newest.Revision.Files) != 2 {
			t.Fatal("missing verified import", newest)
		}
		for _, f := range newest.Revision.Files {
			if f.Path == ".env" {
				t.Fatal("exclusion lost")
			}
			if f.Path == "data.bin" && (!f.Executable || string(f.Data) != machine+"\x00\xff") {
				t.Fatal("byte/mode loss")
			}
		}
		revisions = append(revisions, newest)
	}
	bundles, err := st.PrivateSkillBundlesFor(uid)
	if err != nil || len(bundles) != 2 || revisions[0].Revision.ManifestDigest == revisions[1].Revision.ManifestDigest {
		t.Fatal(bundles, err)
	}
	request, err := http.NewRequest("GET", srv.URL+"/api/skills", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Reet-Token", "personal")
	response, err := srv.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var shelf struct {
		Skills []json.RawMessage `json:"skills"`
	}
	if err = json.NewDecoder(response.Body).Decode(&shelf); err != nil || response.StatusCode != 200 || len(shelf.Skills) != 0 {
		t.Fatal("import changed shelf", response.StatusCode, shelf, err)
	}
	other, err := st.CreateUser("other-import@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.PrivateSkillRevisionFor(other.ID, revisions[0].Bundle.ID, revisions[0].Revision.ID); err == nil {
		t.Fatal("private import crossed accounts")
	}
}
