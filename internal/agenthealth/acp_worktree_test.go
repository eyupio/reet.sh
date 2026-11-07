package agenthealth

import (
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"testing"
)

func TestStructuredWorktreeCapabilityFollowsNegotiationAndConsent(t *testing.T) {
	cfg := AgentConfig{TerminalSession: testTerminalFactory, GitAvailable: func() bool { return true }, LaunchTools: func() []string { return []string{"opencode"} }}
	base := terminalproto.LaunchFeature + "," + terminalproto.WorktreeFeature + "," + terminalproto.ACPFeature
	negotiateFeatures(&cfg, base)
	if capabilitiesFor(cfg, true, terminalproto.Ready).ACPWorktree {
		t.Fatal("new capability leaked to old controller")
	}
	features := base + "," + terminalproto.ACPWorktreeFeature
	negotiateFeatures(&cfg, features)
	caps := capabilitiesFor(cfg, true, terminalproto.Ready)
	if !caps.ACPWorktree {
		t.Fatal("structured worktree support was not advertised")
	}
	if err := caps.ValidateFor(features); err != nil {
		t.Fatal(err)
	}
	if capabilitiesFor(cfg, false, terminalproto.Ready).ACPWorktree {
		t.Fatal("disabled machine advertised structured worktrees")
	}
}
