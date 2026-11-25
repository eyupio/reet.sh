package agenthealth

import (
	"context"
	"net/http"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func testTerminalFactory(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
	return nil, nil
}

func testSoftwareEntry(tool, version string) terminalproto.SoftwareEntry {
	return terminalproto.SoftwareEntry{
		Tool: tool, State: terminalproto.SoftwareInstalled, Version: version, Manager: "node_package",
		Package: "opencode-ai", Path: "/usr/local/bin/opencode", Platform: "linux", Arch: "amd64",
		Source: "package.json", Evidence: "package_bin_matches_executable",
		Operations: []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanUpdate}, CheckedAt: 1,
	}
}

func TestSoftwareCapabilityOnlyWhenNegotiated(t *testing.T) {
	calls := 0
	cfg := AgentConfig{
		TerminalSession: testTerminalFactory,
		Software: func() []terminalproto.SoftwareEntry {
			calls++
			return []terminalproto.SoftwareEntry{testSoftwareEntry("opencode", "1.19.5")}
		},
	}
	negotiateFeatures(&cfg, terminalproto.LaunchFeature)
	if caps := capabilitiesFor(cfg, true, terminalproto.Ready); len(caps.Software) != 0 || calls != 0 {
		t.Fatalf("reported software without software-v1: %+v calls=%d", caps.Software, calls)
	}
	negotiateFeatures(&cfg, terminalproto.LaunchFeature+","+terminalproto.SoftwareFeature)
	caps := capabilitiesFor(cfg, true, terminalproto.Ready)
	if len(caps.Software) != 1 || caps.Software[0].Version != "1.19.5" || calls != 1 {
		t.Fatalf("software not reported: %+v calls=%d", caps.Software, calls)
	}
	if err := caps.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSoftwareCapabilityNeedsTerminalControl(t *testing.T) {
	cfg := AgentConfig{
		Software: func() []terminalproto.SoftwareEntry {
			return []terminalproto.SoftwareEntry{testSoftwareEntry("opencode", "1.19.5")}
		},
	}
	negotiateFeatures(&cfg, terminalproto.SoftwareFeature)
	if caps := capabilitiesFor(cfg, true, terminalproto.Ready); len(caps.Software) != 0 {
		t.Fatalf("software reported without terminal control: %+v", caps.Software)
	}
}

func TestSoftwareCapabilityDropsInvalidAndDuplicateEntries(t *testing.T) {
	invalid := testSoftwareEntry("opencode", "1.19.5")
	invalid.Version = ""
	other := testSoftwareEntry("claude", "2.0.0")
	cfg := AgentConfig{
		TerminalSession: testTerminalFactory,
		Software: func() []terminalproto.SoftwareEntry {
			return []terminalproto.SoftwareEntry{invalid, testSoftwareEntry("opencode", "1.19.5"), testSoftwareEntry("opencode", "1.18.0"), other}
		},
	}
	negotiateFeatures(&cfg, terminalproto.SoftwareFeature)
	caps := capabilitiesFor(cfg, true, terminalproto.Ready)
	if len(caps.Software) != 2 {
		t.Fatalf("kept wrong entries: %+v", caps.Software)
	}
	if caps.Software[0].Tool != "opencode" || caps.Software[0].Version != "1.19.5" || caps.Software[1].Tool != "claude" {
		t.Fatalf("kept the wrong values: %+v", caps.Software)
	}
	if err := caps.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSoftwareApplyCapabilityNeedsNegotiationAndTerminalAccess(t *testing.T) {
	cfg := AgentConfig{
		TerminalSession: testTerminalFactory,
		Software: func() []terminalproto.SoftwareEntry {
			return []terminalproto.SoftwareEntry{testSoftwareEntry("opencode", "1.19.5")}
		},
	}
	negotiateFeatures(&cfg, terminalproto.SoftwareFeature)
	if caps := capabilitiesFor(cfg, true, terminalproto.Ready); caps.SoftwareApply {
		t.Fatal("apply capability without software-apply-v1")
	}
	negotiateFeatures(&cfg, terminalproto.SoftwareFeature+","+terminalproto.SoftwareApplyFeature)
	caps := capabilitiesFor(cfg, true, terminalproto.Ready)
	if !caps.SoftwareApply {
		t.Fatal("apply capability not reported")
	}
	if err := caps.Validate(); err != nil {
		t.Fatal(err)
	}
	if caps := capabilitiesFor(cfg, false, terminalproto.DisabledLocally); caps.SoftwareApply {
		t.Fatal("apply capability with terminal access off")
	}
	noControl := AgentConfig{Software: cfg.Software}
	negotiateFeatures(&noControl, terminalproto.SoftwareFeature+","+terminalproto.SoftwareApplyFeature)
	if caps := capabilitiesFor(noControl, true, terminalproto.Ready); caps.SoftwareApply {
		t.Fatal("apply capability without terminal control")
	}
}

func TestSoftwareMirrorFirstIsNegotiatedBesideApply(t *testing.T) {
	cfg := AgentConfig{TerminalSession: testTerminalFactory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready}
	negotiateFeatures(&cfg, terminalproto.SoftwareApplyFeature)
	if caps := capabilitiesFor(cfg, true, terminalproto.Ready); caps.SoftwareMirrorFirst {
		t.Fatal("mirror first claimed without software-mirror-first-v1")
	}
	negotiateFeatures(&cfg, terminalproto.SoftwareMirrorFirstFeature)
	if caps := capabilitiesFor(cfg, true, terminalproto.Ready); caps.SoftwareMirrorFirst {
		t.Fatal("mirror first claimed without software-apply-v1")
	}
	negotiateFeatures(&cfg, terminalproto.SoftwareApplyFeature+","+terminalproto.SoftwareMirrorFirstFeature)
	caps := capabilitiesFor(cfg, true, terminalproto.Ready)
	if !caps.SoftwareApply || !caps.SoftwareMirrorFirst {
		t.Fatalf("not claimed: %+v", caps)
	}
	features := terminalFeatures(cfg)
	if err := caps.ValidateFor(features); err != nil {
		t.Fatal(err)
	}
	// The terminal dispatcher serves an apply only for what was negotiated.
	if !terminalproto.SupportsSoftwareApply(features) || !terminalproto.SupportsSoftwareMirrorFirst(features) {
		t.Fatalf("terminal features %q", features)
	}
	if caps := capabilitiesFor(cfg, false, terminalproto.DisabledLocally); caps.SoftwareMirrorFirst {
		t.Fatal("mirror first with terminal access off")
	}
}

// A machine that negotiated software-apply-v1 lists it among the features its dispatcher serves; without it the dispatcher
// refuses every apply as invalid.
func TestSoftwareApplyIsListedForTheDispatcher(t *testing.T) {
	cfg := AgentConfig{TerminalSession: testTerminalFactory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready}
	if terminalproto.SupportsSoftwareApply(terminalFeatures(cfg)) {
		t.Fatal("listed before it was negotiated")
	}
	negotiateFeatures(&cfg, terminalproto.SoftwareApplyFeature)
	features := terminalFeatures(cfg)
	if !terminalproto.SupportsSoftwareApply(features) {
		t.Fatalf("negotiated but not listed: %q", features)
	}
	if err := capabilitiesFor(cfg, true, terminalproto.Ready).ValidateFor(features); err != nil {
		t.Fatal(err)
	}
}
