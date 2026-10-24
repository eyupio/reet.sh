//go:build linux || darwin

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

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestRemoteImportBrowserApprovalTwoMachinesAndChangedSource(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	if _, err = st.CreateAPIToken(uid, "remote-import", auth.HashToken("remote-personal")); err != nil {
		t.Fatal(err)
	}
	server := api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthAccounts))
	defer server.Close()
	srv := httptest.NewServer(server)
	defer srv.Close()
	// Only the transfer endpoint receives file bytes. All review requests are
	// the same authenticated JSON browser API used by the UI.
	browser := func(method, path string, body any, out any) int {
		t.Helper()
		var input *strings.Reader
		raw, _ := json.Marshal(body)
		input = strings.NewReader(string(raw))
		request, err := http.NewRequest(method, srv.URL+path, input)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Reet-Token", "remote-personal")
		response, err := srv.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if out != nil {
			if err = json.NewDecoder(response.Body).Decode(out); err != nil {
				t.Fatal(err)
			}
		}
		return response.StatusCode
	}
	for index, machine := range []string{"one", "two", "changed"} {
		home := t.TempDir()
		root := filepath.Join(home, ".agents", "skills", "demo")
		if err = os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		instruction := []byte("---\r\nname: demo\r\ndescription: Remote instructions\r\n---\r\nUse the toolkit.\r\n")
		if err = os.WriteFile(filepath.Join(root, "SKILL.md"), instruction, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "run.bin"), []byte(machine+"\x00\xff"), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, ".env"), []byte("EXCLUDED_SECRET=private"), 0600); err != nil {
			t.Fatal(err)
		}
		a, secret, err := st.EnrollSyncAgent(uid, machine, "test")
		if err != nil {
			t.Fatal(err)
		}
		if err = st.TouchEnvironmentFor(uid, machine, "agents", "", ""); err != nil {
			t.Fatal(err)
		}
		c := agentConfig{ID: a.ID, Secret: secret, Home: home, Host: machine, Server: srv.URL, Token: "remote-personal"}
		instance := strings.Repeat(string(rune('a'+index)), 48)
		if handled, err := processRemoteImportWith(context.Background(), c, "test", instance); err != nil || handled {
			t.Fatal(handled, err)
		}
		var request store.RemoteImport
		input := store.RemoteImportCreate{RequestKey: "browser_remote_request_" + machine, AgentID: a.ID, EnvironmentID: store.EnvironmentIDFor(uid, machine, "agents", ""), Name: "demo"}
		if code := browser("POST", "/api/v2/skill-imports", input, &request); code != 200 {
			t.Fatal(code, request)
		}
		if handled, err := processRemoteImportWith(context.Background(), c, "test", instance); err != nil || !handled {
			t.Fatal(handled, err)
		}
		if code := browser("GET", "/api/v2/skill-imports/"+request.ID, nil, &request); code != 200 || request.State != "reviewing" || request.Preview == nil {
			t.Fatal(code, request)
		}
		raw, _ := json.Marshal(request)
		if strings.Contains(string(raw), "EXCLUDED_SECRET") || strings.Contains(string(raw), `"data":`) {
			t.Fatal("browser preview received bytes")
		}
		bundles, err := st.PrivateSkillBundlesFor(uid)
		if err != nil || len(bundles) != min(index, 2) {
			t.Fatal("preview imported contents", bundles, err)
		}
		approve := map[string]any{"previewHash": request.PreviewHash, "excluded": []string{".env"}}
		if code := browser("POST", "/api/v2/skill-imports/"+request.ID+"/approve", approve, &request); code != 200 {
			t.Fatal(code, request)
		}
		if machine == "changed" {
			if err = os.WriteFile(filepath.Join(root, "run.bin"), []byte("edited after review"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		if handled, err := processRemoteImportWith(context.Background(), c, "test", instance); err != nil || !handled {
			t.Fatal(handled, err)
		}
		if code := browser("GET", "/api/v2/skill-imports/"+request.ID, nil, &request); code != 200 {
			t.Fatal(code)
		}
		if machine == "changed" {
			if request.State != "failed" || request.Error != "source_changed" {
				t.Fatal(request)
			}
			continue
		}
		if request.State != "completed" || request.Receipt == nil {
			t.Fatal(request)
		}
		full, err := st.PrivateSkillRevisionFor(uid, request.Receipt.Bundle.ID, request.Receipt.Revision.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(full.Files) != 2 {
			t.Fatal("wrong selection", full.Files)
		}
		for _, f := range full.Files {
			switch f.Path {
			case "SKILL.md":
				if string(f.Data) != string(instruction) {
					t.Fatal("CRLF changed")
				}
			case "run.bin":
				if string(f.Data) != machine+"\x00\xff" || !f.Executable {
					t.Fatal("binary/mode changed")
				}
			default:
				t.Fatal("unexpected/excluded file", f.Path)
			}
		}
	}
	bundles, err := st.PrivateSkillBundlesFor(uid)
	if err != nil || len(bundles) != 2 {
		t.Fatal(bundles, err)
	}
}
func TestRemoteImportUnsupportedControllerNeverReadsContent(t *testing.T) {
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		if r.URL.Path != "/api/v2/sync/skill-import/work" {
			t.Error("unsupported server received content request")
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	handled, err := processRemoteImportWith(context.Background(), agentConfig{Server: srv.URL, Home: "/unavailable"}, "test", strings.Repeat("a", 48))
	if err != nil || handled || posts != 1 {
		t.Fatal(handled, err, posts)
	}
}
