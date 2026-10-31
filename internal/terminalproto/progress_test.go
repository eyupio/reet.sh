package terminalproto

import "testing"

func TestLaunchProgressRoundTripsAndRejectsUnknownStage(t *testing.T) {
	if !SupportsProgress("launch-v1, "+ProgressFeature) || SupportsProgress("launch-v1") {
		t.Fatal("feature negotiation is wrong")
	}
	if !(LaunchProgress{Stage: StageStarting}).valid() || (LaunchProgress{Stage: "bogus"}).valid() {
		t.Fatal("stage validation is wrong")
	}
}
