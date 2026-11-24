package scan

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/clientconfig"
)

// tokenEnrolled enrols a machine with a personal token, then gives it the
// local settings its owner chose, and stops the agent being restarted.
func tokenEnrolled(t *testing.T, c *codeController, allow bool) (string, *[]string) {
	t.Helper()
	if _, err := c.st.CreateAPIToken(c.uid, "old way", auth.HashToken("personal")); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "agent.json")
	if err := RunAgentInput([]string{"enroll", "--server", c.srv.URL, "--token", "personal", "--env", "studio", "--config", config, "--skip-terminal-setup"}, strings.NewReader(""), io.Discard, "test"); err != nil {
		t.Fatal(err)
	}
	saved, err := loadAgentConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	saved.AllowApply, saved.TerminalEnabled, saved.FollowOff, saved.BrowseOff = allow, true, true, true
	saved.Projects = []string{t.TempDir()}
	if err = writePrivate(config, saved); err != nil {
		t.Fatal(err)
	}
	restarts := &[]string{}
	old := controlAgentFn
	controlAgentFn = func(action, cfg, prefix string, out io.Writer) error {
		*restarts = append(*restarts, action+" "+cfg)
		return nil
	}
	t.Cleanup(func() { controlAgentFn = old })
	return config, restarts
}

func TestEnrolAgainKeepsEveryLocalSetting(t *testing.T) {
	for _, allow := range []bool{true, false} {
		c := newCodeController(t)
		config, restarts := tokenEnrolled(t, c, allow)
		before, _ := loadAgentConfig(config)
		var out bytes.Buffer
		if err := RunAgentInput([]string{"enroll", "--again", "--code", c.code(t), "--config", config}, strings.NewReader(""), &out, "test"); err != nil {
			t.Fatal(err)
		}
		after, err := loadAgentConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		if after.Token != "" || after.Secret == "" || after.Secret == before.Secret || after.ID == "" {
			t.Fatalf("credentials: %+v", after)
		}
		want := before
		want.ID, want.Secret, want.Token = after.ID, after.Secret, ""
		if !strings.EqualFold(want.Host, after.Host) {
			t.Fatalf("host changed: %q -> %q", want.Host, after.Host)
		}
		want.Host = after.Host
		if got, w := strings.Join(after.Projects, ","), strings.Join(want.Projects, ","); got != w {
			t.Fatalf("projects: %q, want %q", got, w)
		}
		want.Projects = after.Projects
		if !sameConfig(after, want) {
			t.Fatalf("settings changed:\n%+v\n%+v", after, want)
		}
		onOff := map[bool]string{true: "on", false: "off"}[allow]
		if !strings.Contains(out.String(), "remote apply: "+onOff) || !strings.Contains(out.String(), "Revoke the old token under Account in the web.") {
			t.Fatalf("output: %q", out.String())
		}
		raw, _ := os.ReadFile(config)
		if bytes.Contains(raw, []byte(`"token"`)) {
			t.Fatalf("token kept: %s", raw)
		}
		if len(*restarts) != 1 || !strings.HasPrefix((*restarts)[0], "restart ") {
			t.Fatalf("restarts: %v", *restarts)
		}
		if a := c.agent(t); a.LegacyToken || a.ID != after.ID {
			t.Fatalf("controller record: %+v", a)
		}
		// It now works by its own secret alone.
		if err = RunAgent([]string{"status", "--remote", "--config", config}, io.Discard, "test"); err != nil {
			t.Fatal(err)
		}
	}
}

func sameConfig(a, b agentConfig) bool { return reflect.DeepEqual(a, b) }

func TestEnrolAgainChangesNothingWhenItFails(t *testing.T) {
	c := newCodeController(t)
	config, restarts := tokenEnrolled(t, c, true)
	before, _ := os.ReadFile(config)
	t.Setenv("REET_TOKEN", "personal")
	for name, args := range map[string][]string{
		"spent code":   {"--code", clientconfig.EnrolCodePrefix + "unknown"},
		"a token":      {"--token", "personal"},
		"an env file":  {"--env-file", filepath.Join(t.TempDir(), ".env")},
		"no code":      {},
		"a new server": {"--code", c.code(t), "--server", "https://elsewhere.example"},
		"a new name":   {"--code", c.code(t), "--env", "other"},
		"apply flag":   {"--code", c.code(t), "--allow-apply"},
	} {
		err := RunAgentInput(append([]string{"enroll", "--again", "--config", config}, args...), strings.NewReader(""), io.Discard, "test")
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		if name == "a token" && !strings.Contains(err.Error(), "enrolment code") {
			t.Fatalf("token refusal: %v", err)
		}
		if after, _ := os.ReadFile(config); !bytes.Equal(after, before) {
			t.Fatalf("%s changed the configuration", name)
		}
	}
	if len(*restarts) != 0 {
		t.Fatalf("restarted: %v", *restarts)
	}
	if a := c.agent(t); !a.LegacyToken {
		t.Fatal("the controller record changed")
	}
}

func TestEnrolAgainNeedsAnEnrolment(t *testing.T) {
	c := newCodeController(t)
	err := RunAgentInput([]string{"enroll", "--again", "--code", c.code(t), "--config", filepath.Join(t.TempDir(), "agent.json")}, strings.NewReader(""), io.Discard, "test")
	if err == nil || !strings.Contains(err.Error(), "reet agent enroll") {
		t.Fatalf("no enrolment: %v", err)
	}
}

// A token reet saved for its other commands is left in place, and the person
// is told where it is and how to remove it once revoked.
func TestEnrolAgainLeavesASavedTokenAndSaysSo(t *testing.T) {
	c := newCodeController(t)
	config, _ := tokenEnrolled(t, c, false)
	path, err := clientconfig.SaveToken("personal")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = RunAgentInput([]string{"enroll", "--again", "--code", c.code(t), "--config", config}, strings.NewReader(""), &out, "test"); err != nil {
		t.Fatal(err)
	}
	if saved, _ := clientconfig.SavedToken(); saved != "personal" {
		t.Fatal("the saved token was removed")
	}
	if !strings.Contains(out.String(), path) || !strings.Contains(out.String(), "reet configure --forget-token") {
		t.Fatalf("output: %q", out.String())
	}
}
