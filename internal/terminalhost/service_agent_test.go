package terminalhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSyncServiceWithoutTmuxPreservesTerminal(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // a sync-only machine has no tmux
	layout := privateLayout(t)
	plan, err := PlanServices(context.Background(), ServiceOptions{Binary: "/bin/sh", ConfigPath: filepath.Join(filepath.Dir(layout.Dir), "agent.json"), Layout: layout, UnitDir: t.TempDir(), AllowMissingTmux: true})
	if err != nil {
		t.Fatal(err)
	}
	const terminal = "existing terminal service must be preserved"
	if err := os.WriteFile(plan.TerminalPath, []byte(terminal), 0600); err != nil {
		t.Fatal(err)
	}
	original := runServiceOutput
	defer func() { runServiceOutput = original }()
	var calls []string
	runServiceOutput = func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return "", nil
	}
	if err := InstallAgent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	for _, call := range calls {
		if strings.Contains(call, plan.TerminalName) || strings.Contains(call, plan.TerminalLabel) || strings.Contains(call, "tmux") {
			t.Fatal("sync installation touched terminal service", call)
		}
	}
	if data, err := os.ReadFile(plan.TerminalPath); err != nil || string(data) != terminal {
		t.Fatal("existing terminal unit changed", err)
	}
	if data, err := os.ReadFile(plan.AgentPath); err != nil || string(data) != plan.AgentText() {
		t.Fatal("agent definition missing", err)
	}
	// Re-running may refresh this enrollment, but cannot take over another one.
	if err := os.WriteFile(plan.AgentPath, []byte("another enrollment"), 0600); err != nil {
		t.Fatal(err)
	}
	calls = nil
	if err := InstallAgent(context.Background(), plan); err == nil || len(calls) != 0 {
		t.Fatal("unrelated enrollment was replaced", err)
	}
}

func TestSyncServiceWithoutManagerLeavesNoUnit(t *testing.T) {
	layout := privateLayout(t)
	plan, err := PlanServices(context.Background(), ServiceOptions{Binary: "/bin/sh", ConfigPath: filepath.Join(filepath.Dir(layout.Dir), "agent.json"), Layout: layout, UnitDir: t.TempDir(), AllowMissingTmux: true})
	if err != nil {
		t.Fatal(err)
	}
	original := runServiceOutput
	defer func() { runServiceOutput = original }()
	runServiceOutput = func(context.Context, string, ...string) (string, error) { return "", errors.New("no native manager") }
	if err := InstallAgent(context.Background(), plan); err == nil {
		t.Fatal("unavailable manager accepted")
	}
	if _, err := os.Stat(plan.AgentPath); !os.IsNotExist(err) {
		t.Fatal("unavailable manager left a unit that blocks background startup", err)
	}
}
