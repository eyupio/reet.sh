package agenthealth

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A machine claims each setup step only when the controller advertised it, so
// an older controller is never offered one, and only with terminal access on.
func TestSetupFeaturesAreNegotiatedOneByOne(t *testing.T) {
	cfg := AgentConfig{TerminalSession: testTerminalFactory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready}
	negotiateFeatures(&cfg, terminalproto.SoftwareApplyFeature)
	if caps := capabilitiesFor(cfg, true, terminalproto.Ready); caps.SetupRead || caps.SetupApply || caps.SetupRollback {
		t.Fatalf("claimed from an older controller: %+v", caps)
	}
	negotiateFeatures(&cfg, terminalproto.SetupReadFeature)
	caps := capabilitiesFor(cfg, true, terminalproto.Ready)
	if !caps.SetupRead || caps.SetupApply || caps.SetupRollback {
		t.Fatalf("read only: %+v", caps)
	}
	negotiateFeatures(&cfg, terminalproto.SetupReadFeature+","+terminalproto.SetupApplyFeature+","+terminalproto.SetupRollbackFeature)
	caps = capabilitiesFor(cfg, true, terminalproto.Ready)
	features := terminalFeatures(cfg)
	if !caps.SetupRead || !caps.SetupApply || !caps.SetupRollback || caps.Validate() != nil {
		t.Fatalf("all: %+v", caps)
	}
	set := terminalproto.ParseFeatures(features)
	if !set.Has(terminalproto.SetupReadFeature) || !set.Has(terminalproto.SetupApplyFeature) || !set.Has(terminalproto.SetupRollbackFeature) {
		t.Fatalf("the dispatcher is not told: %q", features)
	}
	if caps := capabilitiesFor(cfg, false, terminalproto.DisabledLocally); caps.SetupRead || caps.SetupApply || caps.SetupRollback {
		t.Fatal("claimed with terminal access off")
	}
}
