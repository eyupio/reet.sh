package terminalproto

import (
	"bytes"
	"testing"
)

func TestBranchSyncFeatureToken(t *testing.T) {
	for _, value := range []string{"", "branch-prune-v1", "branch-sync-v10", "prefix-branch-sync-v1"} {
		if SupportsBranchSync(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsBranchSync("branch-prune-v1, branch-sync-v1") || BranchSyncFeature != "branch-sync-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestBranchSyncRoundTripAndLimits(t *testing.T) {
	for _, body := range []Body{
		BranchSync{Dir: "/home/u/api"},
		BranchSync{Dir: "/home/u/api", Base: "main"},
		BranchSyncResult{Status: BranchSyncUpdated, Base: "main"},
		BranchSyncResult{Status: BranchSyncDirty},
		BranchSyncResult{Reason: BranchesNotRepoRoot},
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
	encoded, _ := Encode(testMessage(BranchSync{Dir: "/home/u/api"}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"dir":`), []byte(`"force":true,"dir":`), 1)); err == nil {
		t.Fatal("unknown field accepted")
	}
	for name, p := range map[string]BranchSync{"no dir": {}, "relative": {Dir: "api"}, "bad base": {Dir: "/a", Base: "ma\nin"}} {
		if p.valid() {
			t.Error("sync accepted", name)
		}
	}
	for name, r := range map[string]BranchSyncResult{
		"empty":             {},
		"unknown status":    {Status: "forced"},
		"reason and status": {Reason: BranchesGitFailed, Status: BranchSyncUpdated},
	} {
		if r.valid() {
			t.Error("result accepted", name)
		}
	}
}
