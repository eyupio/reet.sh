package terminalhost

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testPlan(t *testing.T, env []string) ServicePlan {
	t.Helper()
	layout := privateLayout(t)
	config := filepath.Join(filepath.Dir(layout.Dir), "agent.json")
	plan, err := PlanServices(context.Background(), ServiceOptions{
		Binary: "/bin/sh", ConfigPath: config, Layout: layout, UnitDir: t.TempDir(), Env: env,
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestPlanServicesSeparatesUnitsAndFiltersEnvironment(t *testing.T) {
	plan := testPlan(t, []string{
		"PATH=/usr/bin:/bin", "HOME=/home/tester", "USER=tester", "LOGNAME=tester", "SHELL=/bin/sh",
		"TERM=xterm-256color", "REET_TOKEN=must-not-leak", "SSH_AUTH_SOCK=/tmp/agent.sock", "BAD\nKEY=x",
	})
	agent, terminal := plan.AgentText(), plan.TerminalText()
	if strings.Contains(agent+terminal, "must-not-leak") || strings.Contains(agent+terminal, "SSH_AUTH_SOCK") {
		t.Fatal("service environment inherited a credential")
	}
	if runtime.GOOS == "darwin" {
		if !strings.Contains(agent, "<key>RunAtLoad</key>") || !strings.Contains(terminal, "<string>"+plan.TmuxPath+"</string>") {
			t.Fatalf("plist content: %s\n%s", agent, terminal)
		}
		if plan.AgentLabel != "sh.reet.agent" || plan.AgentName != "sh.reet.agent.plist" {
			t.Fatalf("labels: %+v", plan)
		}
	} else {
		if !strings.Contains(agent, "ExecStart=") || !strings.Contains(agent, "Description=reet agent") {
			t.Fatalf("agent unit: %s", agent)
		}
		if !strings.Contains(agent, "UMask=0077") || strings.Contains(agent, "PartOf=") || strings.Contains(terminal, "PartOf=") {
			t.Fatalf("unit safety: %s\n%s", agent, terminal)
		}
		if !strings.Contains(terminal, "-D") || !strings.Contains(terminal, plan.Layout.Socket) || !strings.Contains(terminal, "-f") {
			t.Fatalf("terminal unit: %s", terminal)
		}
		if plan.AgentName != "reet-agent.service" || plan.TerminalName != "reet-terminal.service" {
			t.Fatalf("names: %+v", plan)
		}
	}
	if !strings.Contains(agent, "-i") || !strings.Contains(agent, plan.Binary) || !strings.Contains(agent, "agent") || !strings.Contains(agent, "run") {
		t.Fatalf("agent command: %s", agent)
	}
}

func TestPlanServicesRejectsUnsafeConfiguration(t *testing.T) {
	layout := privateLayout(t)
	config := filepath.Join(filepath.Dir(layout.Dir), "agent.json")
	for _, options := range []ServiceOptions{
		{Prefix: "Bad Prefix", Binary: "/bin/sh", ConfigPath: config, Layout: layout, UnitDir: t.TempDir()},
		{Prefix: "reet", Binary: "/does/not/exist", ConfigPath: config, Layout: layout, UnitDir: t.TempDir()},
		{Prefix: "reet", Binary: "/bin/sh", ConfigPath: filepath.Join(t.TempDir(), "missing.json"), Layout: layout, UnitDir: t.TempDir()},
	} {
		if _, err := PlanServices(context.Background(), options); err == nil {
			t.Fatalf("unsafe options accepted: %+v", options)
		}
	}
}

func TestInstallAndUninstallUseIndependentServices(t *testing.T) {
	plan := testPlan(t, []string{"PATH=/usr/bin:/bin", "HOME=/home/tester", "USER=tester", "SHELL=/bin/sh"})
	original := runServiceCommand
	defer func() { runServiceCommand = original }()
	var calls []string
	runServiceCommand = func(ctx context.Context, name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}
	if err := Install(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{plan.AgentPath, plan.TerminalPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing unit %s", path)
		}
	}
	if runtime.GOOS == "linux" {
		terminal := indexOf(calls, "systemctl --user enable --now "+plan.TerminalName)
		agent := indexOf(calls, "systemctl --user enable --now "+plan.AgentName)
		if terminal < 0 || agent < 0 || terminal > agent {
			t.Fatalf("service order: %v", calls)
		}
	}
	status := ReadStatus(context.Background(), plan)
	if !status.Installed && runtime.GOOS != "darwin" {
		t.Fatalf("status: %+v", status)
	}
	if err := Uninstall(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{plan.AgentPath, plan.TerminalPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unit not removed: %s", path)
		}
	}
}

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}

func TestValidateEnvironmentAllowsOnlySafeKeys(t *testing.T) {
	got := validateEnvironment([]string{
		"PATH=/bin", "HOME=/home/tester", "REET_TOKEN=secret", "SSH_AUTH_SOCK=/tmp/x",
		"LANG=en_GB.UTF-8", "WITH\nNEWLINE=x", "NOEQUALS",
	})
	joined := strings.Join(got, ",")
	if strings.Contains(joined, "secret") || strings.Contains(joined, "SSH_AUTH_SOCK") || strings.Contains(joined, "NOEQUALS") || strings.Contains(joined, "NEWLINE") {
		t.Fatalf("unsafe environment: %v", got)
	}
	if !strings.Contains(joined, "PATH=/bin") || !strings.Contains(joined, "LANG=en_GB.UTF-8") {
		t.Fatalf("safe environment missing: %v", got)
	}
}
