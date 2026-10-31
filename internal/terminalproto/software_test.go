package terminalproto

import (
	"strings"
	"testing"
)

func TestSupportsSoftwareToken(t *testing.T) {
	for _, value := range []string{"", "software-v10", "software-v1x", "prefix-software-v1", "Software-v1"} {
		if SupportsSoftware(value) {
			t.Fatalf("accepted %q", value)
		}
	}
	if !SupportsSoftware("browse-v1, software-v1") || !SupportsSoftware(" software-v1 ") || SoftwareFeature != "software-v1" {
		t.Fatal("did not accept software-v1")
	}
}

func installedSoftware(tool string) SoftwareEntry {
	return SoftwareEntry{
		Tool: tool, State: SoftwareInstalled, Version: "1.19.5", Manager: "node_package",
		Package: "opencode-ai", Path: "/usr/local/bin/opencode", Platform: "linux", Arch: "amd64",
		Source: "package.json", Evidence: "package_bin_matches_executable",
		Operations: []SoftwareOperation{SoftwarePlanUpdate, SoftwarePlanRemove}, CheckedAt: 1,
	}
}

func TestSoftwareEntryValidation(t *testing.T) {
	base := installedSoftware("opencode")
	if !base.Valid() {
		t.Fatal("valid installed entry rejected")
	}
	cases := map[string]func(*SoftwareEntry){
		"missing version":       func(e *SoftwareEntry) { e.Version = "" },
		"unknown reason":        func(e *SoftwareEntry) { e.Reason = "other" },
		"reason when installed": func(e *SoftwareEntry) { e.Reason = SoftwareNotInstalled },
		"bad operation":         func(e *SoftwareEntry) { e.Operations = []SoftwareOperation{"run"} },
		"repeated operation":    func(e *SoftwareEntry) { e.Operations = []SoftwareOperation{SoftwarePlanUpdate, SoftwarePlanUpdate} },
		"oversized path":        func(e *SoftwareEntry) { e.Path = strings.Repeat("a", 4097) },
		"control byte in path":  func(e *SoftwareEntry) { e.Path = "/usr/bin/open\x00code" },
		"missing check time":    func(e *SoftwareEntry) { e.CheckedAt = 0 },
		"unknown tool":          func(e *SoftwareEntry) { e.Tool = "vim" },
	}
	for name, change := range cases {
		entry := base
		change(&entry)
		if entry.Valid() {
			t.Fatalf("%s accepted", name)
		}
	}

	missing := SoftwareEntry{Tool: "opencode", State: SoftwareMissing, Platform: "linux", Arch: "amd64", Package: "opencode-ai", Source: "npm", Reason: SoftwareNotInstalled, Operations: []SoftwareOperation{SoftwarePlanInstall}, CheckedAt: 1}
	if !missing.Valid() {
		t.Fatal("valid missing entry rejected")
	}
	missing.Reason = SoftwareUnreadable
	if missing.Valid() {
		t.Fatal("missing entry with another reason accepted")
	}
	missing.Reason = SoftwareNotInstalled
	missing.Version = "1.19.5"
	if missing.Valid() {
		t.Fatal("missing entry with a version accepted")
	}
}

func TestValidSoftwareBoundsAndDuplicates(t *testing.T) {
	if !ValidSoftware(nil) || !ValidSoftware([]SoftwareEntry{installedSoftware("opencode")}) {
		t.Fatal("valid inventory rejected")
	}
	if ValidSoftware([]SoftwareEntry{installedSoftware("opencode"), installedSoftware("opencode")}) {
		t.Fatal("duplicate tool accepted")
	}
	if ValidSoftware(make([]SoftwareEntry, MaxSoftwareEntries+1)) {
		t.Fatal("inventory over the entry bound accepted")
	}
}

func TestCapabilitiesSoftwareNeedsTerminalControl(t *testing.T) {
	caps := Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "me", TerminalEnabled: true, HealthEnabled: true, Readiness: Ready, TerminalControl: true}
	caps.Software = []SoftwareEntry{installedSoftware("opencode")}
	if caps.Validate() != nil {
		t.Fatalf("software inventory with terminal control rejected: %v", caps.Validate())
	}
	caps.TerminalControl = false
	if caps.Validate() == nil {
		t.Fatal("software inventory without terminal control accepted")
	}
}
