package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
)

func (c *codeController) importsSeen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.imports...)
}

// enrolByCode enrols this machine at the default enrolment path, as the
// step-by-step flow does, and gives it one installed skill.
func enrolByCode(t *testing.T, c *codeController, name string) {
	t.Helper()
	if err := RunAgentInput([]string{"enroll", "--server", c.srv.URL, "--code", c.code(t), "--env", name, "--skip-terminal-setup"}, strings.NewReader(""), &bytes.Buffer{}, "test"); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	skill := filepath.Join(home, ".claude", "skills", "alpha")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: alpha\ndescription: A test skill\n---\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// reet scan on a machine enrolled by code, with no personal token anywhere,
// reports with the machine's own enrolment: no token is sent or asked for,
// --prune works for its own name, and --reconcile is skipped with a line.
func TestScanOnACodeEnrolledMachineUsesItsEnrolment(t *testing.T) {
	c := newCodeController(t)
	enrolByCode(t, c, "studio")
	var out bytes.Buffer
	if err := RunVersion([]string{"--server", c.srv.URL, "--env", "studio", "--prune", "--reconcile"}, &out, "test"); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got := c.importsSeen(); len(got) != 1 || got[0] != "machine" {
		t.Fatalf("reports: %v", got)
	}
	if len(c.tokenRequests()) != 0 {
		t.Fatalf("a token was sent: %v", c.tokenRequests())
	}
	if !strings.Contains(out.String(), "pruned on studio") {
		t.Fatalf("prune: %s", out.String())
	}
	envs, _ := c.st.ListEnvironmentsFor(c.uid)
	scanned := false
	for _, e := range envs {
		if e.Host != "studio" {
			t.Fatalf("environments: %+v", envs)
		}
		scanned = scanned || e.Agent == "claude"
	}
	if !scanned {
		t.Fatalf("the scan did not arrive: %+v", envs)
	}
	if !strings.Contains(out.String(), "--reconcile skipped") {
		t.Fatalf("no reconcile line: %s", out.String())
	}
	// With no --server or --env, the enrolment supplies both.
	if err := RunVersion(nil, &bytes.Buffer{}, "test"); err != nil {
		t.Fatal(err)
	}
	if got := c.importsSeen(); len(got) != 2 || got[1] != "machine" {
		t.Fatalf("reports: %v", got)
	}
}

// The machine's secret goes only to the controller it was enrolled with, and
// only for its own name; nothing is sent otherwise.
func TestScanSendsTheEnrolmentNowhereElse(t *testing.T) {
	c := newCodeController(t)
	enrolByCode(t, c, "studio")
	home := os.Getenv("HOME")
	other := newCodeController(t)
	t.Setenv("HOME", home)
	for name, args := range map[string][]string{
		"another server": {"--server", other.srv.URL, "--env", "studio"},
		"another name":   {"--server", c.srv.URL, "--env", "laptop"},
	} {
		err := RunVersion(args, &bytes.Buffer{}, "test")
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	if len(c.importsSeen())+len(other.importsSeen()) != 0 {
		t.Fatalf("sent: %v %v", c.importsSeen(), other.importsSeen())
	}
}

func TestScanStillPrefersAPersonalToken(t *testing.T) {
	c := newCodeController(t)
	enrolByCode(t, c, "studio")
	if _, err := c.st.CreateAPIToken(c.uid, "cli", auth.HashToken("personal")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REET_TOKEN", "personal")
	if err := RunVersion([]string{"--server", c.srv.URL, "--env", "studio"}, &bytes.Buffer{}, "test"); err != nil {
		t.Fatal(err)
	}
	if got := c.importsSeen(); len(got) != 1 || got[0] != "token" {
		t.Fatalf("reports: %v", got)
	}
}

func TestScanWithNeitherEnrolmentNorTokenSaysWhatToDo(t *testing.T) {
	c := newCodeController(t)
	err := RunVersion([]string{"--server", c.srv.URL, "--env", "studio"}, &bytes.Buffer{}, "test")
	if err == nil || !strings.Contains(err.Error(), "reet agent enroll") || !strings.Contains(err.Error(), "token") {
		t.Fatalf("%v", err)
	}
}

// A machine enrolled with a personal token reports, and reconciles, with that
// token, as before.
func TestScanOnATokenEnrolledMachineUsesItsToken(t *testing.T) {
	c := newCodeController(t)
	if _, err := c.st.CreateAPIToken(c.uid, "old way", auth.HashToken("personal")); err != nil {
		t.Fatal(err)
	}
	if err := RunAgentInput([]string{"enroll", "--server", c.srv.URL, "--token", "personal", "--env", "studio", "--skip-terminal-setup"}, strings.NewReader(""), &bytes.Buffer{}, "test"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunVersion([]string{"--env", "studio", "--reconcile"}, &out, "test"); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got := c.importsSeen(); len(got) != 1 || got[0] != "token" || strings.Contains(out.String(), "skipped") {
		t.Fatalf("reports: %v\n%s", got, out.String())
	}
}
