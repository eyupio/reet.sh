//go:build linux || darwin

package acpagent

import "testing"

func TestDescribeInstalledAdapterEvidence(t *testing.T) {
	for _, tc := range []struct {
		tool           string
		installed      bool
		version, state string
	}{
		{"opencode", false, "", "missing"},
		{"opencode", true, "1.19.5", "available"},
		{"opencode", true, "", "available"}, // executable/handshake are checked at launch
		{"opencode", true, "1.19.4", "incompatible"},
		{"opencode", true, "garbled", "incompatible"},
		{"claude", true, "", "unverified"},
		{"codex", true, "", "unverified"},
		{"gemini", false, "", "missing"},
	} {
		got := Describe(tc.tool, tc.installed, tc.version)
		if got.State != tc.state || !got.Valid() {
			t.Fatalf("%+v: %+v", tc, got)
		}
		if got.State != "available" && (got.History || got.Images || got.Model || got.Approval) {
			t.Fatalf("unverified features: %+v", got)
		}
	}
}
