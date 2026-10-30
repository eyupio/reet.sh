//go:build linux || darwin

package acpagent

import "testing"

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
