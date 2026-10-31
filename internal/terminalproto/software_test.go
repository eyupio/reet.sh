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

func TestSoftwareEntryManagedOnlyWhenInstalled(t *testing.T) {
	managed := installedSoftware("opencode")
	managed.Managed = true
	if !managed.Valid() {
		t.Fatal("managed installed entry rejected")
	}
	managed.State = SoftwareMissing
	managed.Reason = SoftwareNotInstalled
	managed.Version, managed.Manager, managed.Path = "", "", ""
	if managed.Valid() {
		t.Fatal("managed missing entry accepted")
	}
}

func TestSoftwareApplyValidation(t *testing.T) {
	install := SoftwareApply{Tool: "opencode", Action: SoftwareActionInstall, ExpectedState: SoftwareMissing}
	if !install.valid() {
		t.Fatal("valid install request rejected")
	}
	alongside := SoftwareApply{Tool: "opencode", Action: SoftwareActionInstall, ExpectedState: SoftwareInstalled, ExpectedManaged: false}
	if !alongside.valid() {
		t.Fatal("valid install-alongside request rejected")
	}
	update := SoftwareApply{Tool: "opencode", Action: SoftwareActionUpdate, ExpectedState: SoftwareInstalled, ExpectedVersion: "1.18.0", ExpectedManaged: true}
	if !update.valid() {
		t.Fatal("valid update request rejected")
	}
	for name, request := range map[string]SoftwareApply{
		"update a system copy": {Tool: "opencode", Action: SoftwareActionUpdate, ExpectedState: SoftwareInstalled},
		"install over managed": {Tool: "opencode", Action: SoftwareActionInstall, ExpectedState: SoftwareInstalled, ExpectedManaged: true},
		"remove":               {Tool: "opencode", Action: SoftwareActionRemove, ExpectedState: SoftwareInstalled, ExpectedManaged: true},
		"unknown action":       {Tool: "opencode", Action: "run", ExpectedState: SoftwareMissing},
		"unknown tool":         {Tool: "vim", Action: SoftwareActionInstall, ExpectedState: SoftwareMissing},
	} {
		if request.valid() {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestSoftwareApplyResultValidation(t *testing.T) {
	entry := installedSoftware("opencode")
	entry.Managed = true
	entry.Manager, entry.Source = "reet", "verified_tarball"
	applied := SoftwareApplyResult{Tool: "opencode", Action: SoftwareActionUpdate, State: SoftwareApplyApplied, Version: "1.19.5", Previous: "1.18.0", Entry: &entry}
	if !applied.valid() {
		t.Fatal("valid applied result rejected")
	}
	refused := SoftwareApplyResult{Tool: "opencode", Action: SoftwareActionUpdate, State: SoftwareApplyRefused, Reason: SoftwareApplySessions}
	if !refused.valid() {
		t.Fatal("valid refused result rejected")
	}
	applied.Entry = nil
	if applied.valid() {
		t.Fatal("applied result without an entry accepted")
	}
	applied.Entry = &entry
	applied.Reason = SoftwareApplyBusy
	if applied.valid() {
		t.Fatal("applied result with a reason accepted")
	}
	refused.Entry = &entry
	if refused.valid() {
		t.Fatal("refused result with an entry accepted")
	}
	refused.Entry = nil
	refused.Reason = "other"
	if refused.valid() {
		t.Fatal("refused result with an unknown reason accepted")
	}
}

func TestSupportsSoftwareApplyToken(t *testing.T) {
	for _, value := range []string{"", "software-apply-v10", "prefix-software-apply-v1", "Software-apply-v1"} {
		if SupportsSoftwareApply(value) {
			t.Fatalf("accepted %q", value)
		}
	}
	if !SupportsSoftwareApply("software-v1, software-apply-v1") || SoftwareApplyFeature != "software-apply-v1" {
		t.Fatal("did not accept software-apply-v1")
	}
}
