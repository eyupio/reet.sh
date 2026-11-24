package scan

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// codeController is an accounts controller that remembers which requests
// carried a personal token. refuseMachines makes it act as an older
// controller, which knows no machine authentication.
type codeController struct {
	srv            *httptest.Server
	st             *store.Store
	uid            int64
	mu             sync.Mutex
	withToken      []string
	refuseMachines bool
}

func newCodeController(t *testing.T) *codeController {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("REET_TOKEN", "")
	t.Setenv("REET_ENROL_CODE", "")
	t.Setenv("REET_ENV_FILE", "")
	t.Chdir(t.TempDir())
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	c := &codeController{st: st}
	c.uid, _ = st.DefaultOwner()
	h := api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthAccounts))
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-Reet-Token") + r.Header.Get("Authorization")
		c.mu.Lock()
		if tok != "" && !strings.Contains(tok, clientconfig.EnrolCodePrefix) {
			c.withToken = append(c.withToken, r.URL.Path)
		}
		refuse := c.refuseMachines
		c.mu.Unlock()
		if refuse && tok == "" && r.URL.Path != "/api/v2/sync/agent-proof" {
			http.Error(w, `{"error":"sign in required"}`, 401)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(func() { c.srv.Close(); h.Close(); st.Close() })
	return c
}

func (c *codeController) code(t *testing.T) string {
	t.Helper()
	raw, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	code := clientconfig.EnrolCodePrefix + raw
	if err = c.st.CreateEnrolCode(c.uid, auth.HashToken(code), time.Now().Add(time.Minute).UnixMilli(), 20); err != nil {
		t.Fatal(err)
	}
	return code
}

func (c *codeController) tokenRequests() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.withToken...)
}

func (c *codeController) agent(t *testing.T) store.SyncAgent {
	t.Helper()
	agents, err := c.st.SyncAgentsFor(c.uid)
	if err != nil || len(agents) != 1 {
		t.Fatalf("machines: %+v %v", agents, err)
	}
	return agents[0]
}

func noPersonalTokenOnDisk(t *testing.T, config string) {
	t.Helper()
	raw, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"token"`)) {
		t.Fatalf("agent.json holds a token field: %s", raw)
	}
	if path, _ := clientconfig.TokenPath(); fileExists(path) {
		t.Fatalf("enrolment wrote %s", path)
	}
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

// A machine enrolled with a code keeps only its id and secret, and every
// request it makes afterwards, its inventory scan included, is authenticated
// by them alone.
func TestEnrolWithACodeKeepsNoPersonalToken(t *testing.T) {
	c := newCodeController(t)
	config := filepath.Join(t.TempDir(), "agent.json")
	var out bytes.Buffer
	if err := RunAgentInput([]string{"enroll", "--server", c.srv.URL, "--code", c.code(t), "--env", "studio", "--config", config, "--skip-terminal-setup"}, strings.NewReader(""), &out, "test"); err != nil {
		t.Fatal(err)
	}
	noPersonalTokenOnDisk(t, config)
	saved, err := loadAgentConfig(config)
	if err != nil || saved.ID == "" || saved.Secret == "" || saved.Token != "" {
		t.Fatalf("enrolment: %+v %v", saved, err)
	}
	if err = RunAgent([]string{"run", "--once", "--config", config}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	if err = RunAgent([]string{"status", "--remote", "--config", config}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	if sent := c.tokenRequests(); len(sent) != 0 {
		t.Fatalf("requests carried a token: %v", sent)
	}
	if a := c.agent(t); a.LegacyToken || a.Host != "studio" {
		t.Fatalf("machine: %+v", a)
	}
	envs, _ := c.st.ListEnvironmentsFor(c.uid)
	scanned := false
	for _, e := range envs {
		scanned = scanned || e.Host == "studio"
	}
	if !scanned {
		t.Fatalf("the inventory scan did not arrive: %+v", envs)
	}
}

func TestEnrolCodeFromTheEnvironmentAndARefusedCode(t *testing.T) {
	c := newCodeController(t)
	t.Setenv("REET_ENROL_CODE", c.code(t))
	config := filepath.Join(t.TempDir(), "agent.json")
	if err := RunAgentInput([]string{"enroll", "--server", c.srv.URL, "--env", "studio", "--config", config, "--skip-terminal-setup"}, strings.NewReader(""), io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	noPersonalTokenOnDisk(t, config)
	// The same code again is refused, and the person is told what to do.
	other := filepath.Join(t.TempDir(), "agent.json")
	err := RunAgentInput([]string{"enroll", "--server", c.srv.URL, "--env", "laptop", "--config", other, "--skip-terminal-setup"}, strings.NewReader(""), io.Discard, "test")
	if err == nil || !strings.Contains(err.Error(), "Add a machine") {
		t.Fatalf("spent code: %v", err)
	}
	if fileExists(other) {
		t.Fatal("a refused enrolment wrote its configuration")
	}
}

// A personal token still enrols. Where the controller takes the machine's
// own secret, the token is not kept; an older controller keeps it working.
func TestEnrolWithAPersonalTokenStillWorks(t *testing.T) {
	c := newCodeController(t)
	if _, err := c.st.CreateAPIToken(c.uid, "old way", auth.HashToken("personal")); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "agent.json")
	// (Its warning, on standard error, is checked in clientconfig.)
	if err := RunAgentInput([]string{"enroll", "--server", c.srv.URL, "--token", "personal", "--env", "studio", "--config", config, "--skip-terminal-setup"}, strings.NewReader(""), io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	noPersonalTokenOnDisk(t, config)
	if err := RunAgent([]string{"run", "--once", "--config", config}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	if a := c.agent(t); a.LegacyToken {
		t.Fatal("machine still marked after leaving the token behind")
	}

	older := newCodeController(t)
	if _, err := older.st.CreateAPIToken(older.uid, "old way", auth.HashToken("personal")); err != nil {
		t.Fatal(err)
	}
	older.refuseMachines = true
	config = filepath.Join(t.TempDir(), "agent.json")
	if err := RunAgentInput([]string{"enroll", "--server", older.srv.URL, "--token", "personal", "--env", "studio", "--config", config, "--skip-terminal-setup"}, strings.NewReader(""), io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	if saved, _ := loadAgentConfig(config); saved.Token != "personal" {
		t.Fatalf("an older controller's machine lost its token: %+v", saved)
	}
	if err := RunAgent([]string{"run", "--once", "--config", config}, io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	if a := older.agent(t); !a.LegacyToken {
		t.Fatal("a machine using a personal token is not marked")
	}
}
