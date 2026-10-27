package agenthealth

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestWorktreeCapabilityNeedsLaunchGitAndNegotiation(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("launch is Linux and macOS only")
	}
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	tools := func() []string { return []string{"claude"} }
	git := func() bool { return true }
	noGit := func() bool { return false }
	for _, tc := range []struct {
		name     string
		cfg      AgentConfig
		worktree bool
	}{
		{"everything", AgentConfig{TerminalSession: factory, launch: true, worktree: true, LaunchTools: tools, GitAvailable: git}, true},
		{"worktree not negotiated", AgentConfig{TerminalSession: factory, launch: true, LaunchTools: tools, GitAvailable: git}, false},
		{"launch not negotiated", AgentConfig{TerminalSession: factory, worktree: true, LaunchTools: tools, GitAvailable: git}, false},
		{"no launch helper", AgentConfig{TerminalSession: factory, launch: true, worktree: true, GitAvailable: git}, false},
		{"no terminal control", AgentConfig{launch: true, worktree: true, LaunchTools: tools, GitAvailable: git}, false},
		{"git not found", AgentConfig{TerminalSession: factory, launch: true, worktree: true, LaunchTools: tools, GitAvailable: noGit}, false},
		{"no git helper", AgentConfig{TerminalSession: factory, launch: true, worktree: true, LaunchTools: tools}, false},
	} {
		caps := capabilities(tc.cfg)
		if err := caps.Validate(); err != nil {
			t.Fatal(tc.name, err)
		}
		data, _ := json.Marshal(caps)
		if caps.Worktree != tc.worktree || strings.Contains(string(data), `"worktree"`) != tc.worktree {
			t.Fatal(tc.name, string(data))
		}
	}
}

func TestWorktreeFeatureFollowsTheControllerAdvertisement(t *testing.T) {
	for _, tc := range []struct {
		advertised       string
		launch, worktree bool
	}{
		{"", false, false},
		{terminalproto.LaunchFeature, true, false},
		{terminalproto.WorktreeFeature, false, true},
		{terminalproto.CommandHistoryFeature + "," + terminalproto.LaunchFeature + ", " + terminalproto.WorktreeFeature, true, true},
		{"worktree-v2", false, false},
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, tc.advertised)
		if cfg.launch != tc.launch || cfg.worktree != tc.worktree {
			t.Fatal("negotiated", tc.advertised, cfg.launch, cfg.worktree)
		}
		headers := http.Header{}
		setTerminalFeatures(headers, cfg)
		value := headers.Get(terminalproto.FeatureHeader)
		if terminalproto.SupportsLaunch(value) != tc.launch || terminalproto.SupportsWorktree(value) != tc.worktree {
			t.Fatal("terminal features header", tc.advertised, value)
		}
	}
}
