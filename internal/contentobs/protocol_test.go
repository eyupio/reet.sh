package contentobs

import (
	"strings"
	"testing"
)

func TestReportRejectsAmbiguousOrUnboundedObservations(t *testing.T) {
	valid := func() Report {
		return Report{SchemaVersion: 1, ScanID: strings.Repeat("a", 48), ObservedAt: 1, Scopes: []Scope{{Agent: "agents", Complete: true, Skills: []Skill{{Name: "demo", SchemaVersion: 1, Algorithm: "sha256", ModeSemantics: "posix-executable-v1", Complete: true, Digest: strings.Repeat("a", 64), Files: 1, Bytes: 12}}}}}
	}
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Report){
		"future schema":        func(r *Report) { r.SchemaVersion++ },
		"missing scopes":       func(r *Report) { r.Scopes = nil },
		"duplicate scope":      func(r *Report) { r.Scopes = append(r.Scopes, r.Scopes[0]) },
		"duplicate name":       func(r *Report) { r.Scopes[0].Skills = append(r.Scopes[0].Skills, r.Scopes[0].Skills[0]) },
		"root traversal":       func(r *Report) { r.Scopes[0].Skills[0].Name = "../demo" },
		"control character":    func(r *Report) { r.Scopes[0].Project = "/root\nprivate" },
		"bad digest":           func(r *Report) { r.Scopes[0].Skills[0].Digest = "sha256:abc" },
		"incomplete digest":    func(r *Report) { r.Scopes[0].Complete = false; r.Scopes[0].Skills[0].Complete = false },
		"scope hides partial":  func(r *Report) { r.Scopes[0].Skills[0].Complete = false; r.Scopes[0].Skills[0].Digest = "" },
		"overlarge file count": func(r *Report) { r.Scopes[0].Skills[0].Files = 257 },
		"issue path":           func(r *Report) { r.Scopes[0].Skills[0].Issues = []string{"/etc/secret"} },
	} {
		t.Run(name, func(t *testing.T) {
			r := valid()
			change(&r)
			if err := r.Validate(); err == nil {
				t.Fatal("accepted invalid observation")
			}
		})
	}
}
