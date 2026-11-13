package terminalproto

import (
	"bytes"
	"testing"
)

func TestBaseStateValid(t *testing.T) {
	const sha1 = "0123456789abcdef0123456789abcdef01234567"
	const sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []struct {
		name  string
		state BaseState
		want  bool
	}{
		{"start only", BaseState{Relation: BaseNoRemote, Start: sha1}, true},
		{"every field", BaseState{Base: "main", Relation: BaseSame, Local: sha1, Remote: sha1, Start: sha1}, true},
		{"sha-256", BaseState{Relation: BaseAhead, Local: sha256, Start: sha256}, true},
		{"no relation", BaseState{Start: sha1}, false},
		{"unknown relation", BaseState{Relation: BaseRelation("stale"), Start: sha1}, false},
		{"no start", BaseState{Relation: BaseNoRemote}, false},
		{"short start", BaseState{Relation: BaseNoRemote, Start: sha1[:7]}, false},
		{"upper-case start", BaseState{Relation: BaseNoRemote, Start: "ABCDEF" + sha1[6:]}, false},
		{"non-hex local", BaseState{Relation: BaseAhead, Local: "z" + sha1[1:], Start: sha1}, false},
		{"non-hex remote", BaseState{Relation: BaseBehind, Remote: "z" + sha1[1:], Start: sha1}, false},
		{"invalid base", BaseState{Base: "-dash", Relation: BaseSame, Start: sha1}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.state.Valid(); got != c.want {
				t.Fatalf("Valid() = %v for %+v, want %v", got, c.state, c.want)
			}
		})
	}
}

func TestBaseCheckRoundTripAndLimits(t *testing.T) {
	const sha1 = "0123456789abcdef0123456789abcdef01234567"
	for _, body := range []Body{
		BaseCheck{Dir: "/home/u/api"},
		BaseCheck{Dir: "/home/u/api", Base: "main"},
		BaseCheckResult{State: BaseState{Base: "main", Relation: BaseBehind, Local: sha1, Remote: sha1, Start: sha1}},
		BaseCheckResult{State: BaseState{Relation: BaseNoRemote, Start: sha1}},
		BaseCheckResult{Reason: BranchesBaseMissing},
		BaseCheckResult{Reason: BranchesNotRepoRoot},
	} {
		encoded, err := Encode(testMessage(body))
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		if _, err = Encode(decoded); err != nil {
			t.Fatal(err)
		}
	}
	encoded, _ := Encode(testMessage(BaseCheck{Dir: "/home/u/api"}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"dir":`), []byte(`"fetch":true,"dir":`), 1)); err == nil {
		t.Fatal("unknown field accepted")
	}
	for name, c := range map[string]BaseCheck{"no dir": {}, "relative": {Dir: "api"}, "bad base": {Dir: "/a", Base: "ma\nin"}} {
		if c.valid() {
			t.Error("check accepted", name)
		}
	}
	for name, r := range map[string]BaseCheckResult{
		"empty":                 {},
		"unknown reason":        {Reason: BranchesReason("stale")},
		"reason and state":      {Reason: BranchesGitFailed, State: BaseState{Relation: BaseSame, Start: sha1}},
		"state without a start": {State: BaseState{Relation: BaseSame}},
	} {
		if r.valid() {
			t.Error("result accepted", name)
		}
	}
}
