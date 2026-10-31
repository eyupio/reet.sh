//go:build linux || darwin

package acpagent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSemverFloor(t *testing.T) {
	for _, tc := range []struct {
		found, min string
		ok         bool
	}{
		{"1.19.5", "1.19.5", true},
		{"1.20.0", "1.19.5", true},
		{"v1.19.5", "1.19.5", true},
		{"1.19", "1.19.0", true},
		{"1.19.5-alpha.1", "1.19.5", false},
		{"1.19.4", "1.19.5", false},
		{"not-a-version", "1.19.5", false},
	} {
		found, ok := parseSemver(tc.found)
		min, okMin := parseSemver(tc.min)
		if !okMin {
			t.Fatalf("parse floor %q", tc.min)
		}
		if !ok {
			if tc.ok {
				t.Fatalf("%q should parse", tc.found)
			}
			continue
		}
		if got := found.atLeast(min); got != tc.ok {
			t.Fatalf("%q at least %q: got=%v want=%v", tc.found, tc.min, got, tc.ok)
		}
	}
}

func TestProviderAllowlistIsPerAgent(t *testing.T) {
	spec, ok := Lookup("opencode")
	if !ok || spec.ExpectedName != "OpenCode" || spec.MinVersion != "1.19.5" || spec.RequireMode != "build" {
		t.Fatalf("opencode spec: %+v %v", spec, ok)
	}
	allowed := map[string]bool{}
	for _, key := range spec.EnvAllowlist {
		allowed[key] = true
	}
	for _, key := range []string{"OPENCODE_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY"} {
		if !allowed[key] {
			t.Fatalf("provider key %s is not allowlisted", key)
		}
	}
	if allowed["OPENCODE_CONFIG"] || allowed["SSH_AUTH_SOCK"] {
		t.Fatal("unsafe key allowlisted")
	}
}

func TestSessionRequestMeta(t *testing.T) {
	method, params := sessionRequest(Spec{}, "/p", "")
	if _, has := params["_meta"]; method != "session/new" || has || params["sessionId"] != nil {
		t.Fatalf("plain new %s %v", method, params)
	}
	method, params = sessionRequest(Spec{SessionMeta: ClaudeSessionMeta}, "/p", "abc")
	if method != "session/load" || params["sessionId"] != "abc" || params["cwd"] != "/p" {
		t.Fatalf("load %s %v", method, params)
	}
	raw, err := json.Marshal(params["_meta"])
	if err != nil || string(raw) != `{"claudeCode":{"options":{"settingSources":["user"]}}}` {
		t.Fatalf("meta %s %v", raw, err)
	}
	// The registered OpenCode profile is unchanged and Claude is not registered.
	if _, ok := Lookup("claude"); ok || OpenCode.SessionMeta != nil {
		t.Fatal("profiles changed")
	}
}

func TestClaudeCandidateEnvironment(t *testing.T) {
	env := agentEnvironment(ClaudeCandidate, "/home/u", []string{
		"PATH=/bin", "HOME=/elsewhere", "LANG=C.UTF-8",
		"ANTHROPIC_API_KEY=sk-ant-test", "CLAUDE_CODE_OAUTH_TOKEN=tok",
		"ANTHROPIC_BASE_URL=https://evil.example", "ANTHROPIC_AUTH_TOKEN=x", "CLAUDE_CODE_USE_BEDROCK=1",
		"CLAUDE_CODE_USE_VERTEX=1", "CLAUDE_CONFIG_DIR=/tmp/other", "ANTHROPIC_MODEL=x",
		"OPENAI_API_KEY=sk-other", "SSH_AUTH_SOCK=/tmp/ssh", "GH_TOKEN=secret",
	}, LaunchEnv{})
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, bad := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_USE_", "CLAUDE_CONFIG_DIR", "ANTHROPIC_MODEL", "OPENAI_API_KEY", "SSH_AUTH_SOCK", "GH_TOKEN", "HOME=/elsewhere"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("kept %s:\n%s", bad, joined)
		}
	}
	for _, good := range []string{"\nHOME=/home/u\n", "\nANTHROPIC_API_KEY=sk-ant-test\n", "\nCLAUDE_CODE_OAUTH_TOKEN=tok\n", "\nDISABLE_TELEMETRY=1\n", "\nDISABLE_AUTOUPDATER=1\n"} {
		if !strings.Contains(joined, good) {
			t.Fatalf("missing %q:\n%s", good, joined)
		}
	}
	if ClaudeCandidate.RequireMode != "default" || ClaudeCandidate.SessionMeta == nil {
		t.Fatal("approval contract not pinned")
	}
	if _, registered := Lookup("claude"); registered {
		t.Fatal("candidate must stay unregistered")
	}
}
