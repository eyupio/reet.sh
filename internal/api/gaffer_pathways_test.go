package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

// pathwayStart is an account with a default provider, a project and a saved
// pathway, behind a server that has remote sessions on.
func pathwayStart(t *testing.T) (*httptest.Server, *http.Client, string, string) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithGafferProviders(seal), WithRemoteTerminals(true))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup %d", code)
	}
	if _, err := h.s.gafferProviders.Save(gaffer.Principal{UserID: 1, CanUseLocal: true}, gaffer.Input{Name: "Brains", Kind: "openai_compatible", BaseURL: srv.URL, LocalOnly: true, Enabled: true, MakeDefault: true, Model: "fake-model"}); err != nil {
		t.Fatal(err)
	}
	agent, _, err := st.EnrollSyncAgent(1, "machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateFolderProjectFor(1, store.FolderProject{Host: agent.ID, Dir: "/work/billing", Name: "billing", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.s.gafferProviders.SavePathway(1, store.GafferPathway{Name: "Careful", Enabled: true, Stages: []store.GafferPathwayStage{
		{Kind: "code"}, {Kind: "custom", Name: "Audit", Prompt: "Audit the logging."}, {Kind: "pr"},
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return srv, owner, project.ID, p.ID
}

func TestStartOutcomeOnAPathway(t *testing.T) {
	srv, owner, project, pathway := pathwayStart(t)
	body := `{"objective":"Tidy the logs.","projectId":"` + project + `","tool":"opencode","maxReviews":8,"hours":24,"confirmed":true,"pathway":"` + pathway + `","sessions":[{"title":"Logs","prompt":"Tidy the logs. Do not merge."}]}`
	code, got := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", body)
	if code != 201 {
		t.Fatalf("start: %d %s", code, got)
	}
	var o struct {
		Sessions []struct {
			PathwayName string `json:"pathwayName"`
			Stages      []struct{ Kind, Name string }
		}
	}
	if err := json.Unmarshal([]byte(got), &o); err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, st := range o.Sessions[0].Stages {
		if st.Name != "" {
			labels = append(labels, st.Name)
		} else {
			labels = append(labels, st.Kind)
		}
	}
	if strings.Join(labels, ",") != "code,Audit,pr" || o.Sessions[0].PathwayName != "Careful" {
		t.Fatalf("stages %v pathway %q: %s", labels, o.Sessions[0].PathwayName, got)
	}
}

func TestStartRefusesStagesWithAPathway(t *testing.T) {
	srv, owner, project, pathway := pathwayStart(t)
	body := `{"objective":"Tidy the logs.","projectId":"` + project + `","tool":"opencode","maxReviews":8,"hours":24,"confirmed":true,"pathway":"` + pathway + `","sessions":[{"title":"Logs","prompt":"Tidy the logs. Do not merge.","stages":[{"kind":"code"}]}]}`
	if code, got := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", body); code != 422 || !strings.Contains(got, "not both") {
		t.Fatalf("stages and a pathway: %d %s", code, got)
	}
}
