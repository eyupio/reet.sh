package agentsoftware

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func installed(version string) terminalproto.SoftwareEntry {
	return terminalproto.SoftwareEntry{
		Tool: "opencode", State: terminalproto.SoftwareInstalled, Version: version,
		Manager: "node_package", Package: "opencode-ai", Path: "/usr/local/bin/opencode",
		Platform: "linux", Arch: "amd64", Source: "package.json", CheckedAt: 1,
	}
}

func missing() terminalproto.SoftwareEntry {
	return terminalproto.SoftwareEntry{
		Tool: "opencode", State: terminalproto.SoftwareMissing, Package: "opencode-ai",
		Source: "npm", Platform: "linux", Arch: "amd64", Reason: terminalproto.SoftwareNotInstalled, CheckedAt: 1,
	}
}

func TestUpdatePlanNamesTheReviewedVersionAndRunsNothing(t *testing.T) {
	plan, err := For(installed("1.18.0"), Update, "studio", 0)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "ready" || plan.Current != "1.18.0" || plan.Target != OpenCodeReviewedVersion {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.Command != "npm install -g opencode-ai@1.19.5" || plan.Executes {
		t.Fatalf("command or executes wrong: %+v", plan)
	}
	if len(plan.Steps) == 0 || len(plan.Effects) == 0 {
		t.Fatalf("plan lacks steps or effects: %+v", plan)
	}
}

func TestUpdatePlanIsANoopAtTheReviewedVersion(t *testing.T) {
	plan, err := For(installed(OpenCodeReviewedVersion), Update, "studio", 3)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "noop" || plan.Command != "" {
		t.Fatalf("expected noop: %+v", plan)
	}
}

func TestInstallPlanOnlyWhenMissing(t *testing.T) {
	plan, err := For(missing(), Install, "studio", 0)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "ready" || plan.Command != "npm install -g opencode-ai@1.19.5" {
		t.Fatalf("unexpected install plan: %+v", plan)
	}
	if _, err := For(installed("1.19.5"), Install, "studio", 0); !errors.Is(err, ErrAction) {
		t.Fatalf("installed install plan error = %v", err)
	}
}

func TestRemovePlanKeepsConfigurationAndNamesRunningSessions(t *testing.T) {
	plan, err := For(installed("1.19.5"), Remove, "studio", 2)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Command != "npm uninstall -g opencode-ai" || plan.Target != "" {
		t.Fatalf("unexpected remove plan: %+v", plan)
	}
	if plan.Effects[0] != "2 OpenCode sessions are running on studio; finish or close them before you remove." {
		t.Fatalf("unexpected effect: %q", plan.Effects[0])
	}
	if _, err := For(missing(), Remove, "studio", 0); !errors.Is(err, ErrAction) {
		t.Fatalf("missing remove plan error = %v", err)
	}
}

func TestPlanRefusesUnreviewedEvidence(t *testing.T) {
	cases := map[string]terminalproto.SoftwareEntry{
		"another tool":     {Tool: "claude", State: terminalproto.SoftwareInstalled, Version: "1", Manager: "node_package", Package: "x", Path: "/x", Platform: "linux", Arch: "amd64", Source: "package.json", CheckedAt: 1},
		"another platform": {Tool: "opencode", State: terminalproto.SoftwareInstalled, Version: "1", Manager: "node_package", Package: "opencode-ai", Path: "/x", Platform: "darwin", Arch: "arm64", Source: "package.json", CheckedAt: 1},
		"another manager":  {Tool: "opencode", State: terminalproto.SoftwareInstalled, Version: "1", Manager: "brew", Package: "opencode-ai", Path: "/x", Platform: "linux", Arch: "amd64", Source: "package.json", CheckedAt: 1},
		"unknown state":    {Tool: "opencode", State: terminalproto.SoftwareUnknown, Reason: terminalproto.SoftwareUnreadable, Platform: "linux", Arch: "amd64", CheckedAt: 1},
	}
	for name, entry := range cases {
		if _, err := For(entry, Update, "studio", 0); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	if _, err := For(installed("1.19.5"), Action("run"), "studio", 0); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown action error = %v", err)
	}
}
