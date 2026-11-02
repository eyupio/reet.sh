package agenthealth

import (
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"testing"
)

func TestSessionInspectionOnlyAdvertisedAfterNegotiationAndConsent(t *testing.T) {
	cfg := AgentConfig{TerminalSession: testTerminalFactory}
	negotiateFeatures(&cfg, "")
	if capabilitiesFor(cfg, true, terminalproto.Ready).Inspect || terminalproto.SupportsInspect(terminalFeatures(cfg)) {
		t.Fatal("inspection leaked to old controller")
	}
	negotiateFeatures(&cfg, terminalproto.InspectFeature)
	caps := capabilitiesFor(cfg, true, terminalproto.Ready)
	if !caps.Inspect || !terminalproto.SupportsInspect(terminalFeatures(cfg)) || caps.Validate() != nil {
		t.Fatal("inspection was not negotiated", caps)
	}
	if capabilitiesFor(cfg, false, terminalproto.DisabledLocally).Inspect {
		t.Fatal("inspection exposed without local consent")
	}
}
