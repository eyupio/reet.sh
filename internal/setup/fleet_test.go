package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestFleetAllowlistDropsModelDefaultsAndCommandFields(t *testing.T) {
	cases := []struct {
		agent, base string
		want        []string
	}{
		{"claude", "settings.json", []string{"alwaysThinkingEnabled", "cleanupPeriodDays", "forceLoginMethod", "includeCoAuthoredBy", "outputStyle", "permissions"}},
		{"claude", ".claude.json", nil},
		{"claude", "settings.local.json", nil},
		{"codex", "config.toml", []string{"approval_policy", "file_opener", "hide_agent_reasoning", "sandbox_mode", "tools"}},
		{"opencode", "opencode.json", []string{"autoupdate", "compaction", "default_agent", "disabled_providers", "enabled_providers", "instructions", "permission", "share", "theme", "watcher"}},
		{"pi", "settings.json", []string{"theme"}},
		{"cursor", "mcp.json", nil},
	}
	for _, c := range cases {
		if got := FleetSettingsFields(c.agent, c.base); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %s: %v, want %v", c.agent, c.base, got, c.want)
		}
	}
	// The portable policy still takes a model; the fleet one never does.
	model := []byte(`{"model":"m","permissions":{"allow":["Read"]}}`)
	if err := ValidatePortableSettings("claude", "json", "settings.json", model); err != nil {
		t.Fatalf("portable: %v", err)
	}
	if err := ValidateFleetSettings("claude", "json", "settings.json", model); err == nil {
		t.Fatal("the fleet allowlist took a model default")
	}
	for _, refused := range []string{`{"formatter":{}}`, `{"small_model":"x"}`} {
		if ValidateFleetSettings("opencode", "json", "opencode.json", []byte(refused)) == nil {
			t.Fatalf("opencode took %s", refused)
		}
	}
	if ValidateFleetSettings("pi", "json", "settings.json", []byte(`{"packages":["x"]}`)) == nil {
		t.Fatal("pi took packages")
	}
	if ValidateFleetSettings("codex", "toml", "config.toml", []byte("model_reasoning_effort = \"high\"\n")) == nil {
		t.Fatal("codex took a model default")
	}
	if err := ValidateFleetSettings("codex", "toml", "config.toml", []byte("approval_policy = \"never\"\n")); err != nil {
		t.Fatalf("codex approval policy: %v", err)
	}
}

func TestFleetWideningPerAgent(t *testing.T) {
	list := func(v ...string) []any {
		out := []any{}
		for _, s := range v {
			out = append(out, s)
		}
		return out
	}
	perm := func(kv ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	cases := []struct {
		name          string
		agent         string
		before, after map[string]any
		widens        bool
	}{
		{"claude new allow rule", "claude", perm("permissions", perm("allow", list("Read"))), perm("permissions", perm("allow", list("Read", "Bash"))), true},
		{"claude allow rule into an empty file", "claude", map[string]any{}, perm("permissions", perm("allow", list("Read"))), true},
		{"claude fewer allow rules", "claude", perm("permissions", perm("allow", list("Read", "Bash"))), perm("permissions", perm("allow", list("Read"))), false},
		{"claude deny dropped", "claude", perm("permissions", perm("deny", list("Bash"))), perm("permissions", perm()), true},
		{"claude deny added", "claude", perm("permissions", perm()), perm("permissions", perm("deny", list("Bash"))), false},
		{"claude ask dropped", "claude", perm("permissions", perm("ask", list("Bash"))), perm("permissions", perm("ask", list())), true},
		{"claude bypass mode", "claude", perm("permissions", perm("defaultMode", "default")), perm("permissions", perm("defaultMode", "bypassPermissions")), true},
		{"claude plan mode", "claude", perm("permissions", perm("defaultMode", "acceptEdits")), perm("permissions", perm("defaultMode", "plan")), false},
		{"claude unknown mode", "claude", perm("permissions", perm()), perm("permissions", perm("defaultMode", "something")), true},
		{"claude extra directory", "claude", perm("permissions", perm()), perm("permissions", perm("additionalDirectories", list("/srv"))), true},
		{"claude untouched", "claude", perm("permissions", perm("allow", list("Read"))), perm("outputStyle", "x"), false},
		{"codex never asks", "codex", perm("approval_policy", "on-request"), perm("approval_policy", "never"), true},
		{"codex asks more", "codex", perm("approval_policy", "never"), perm("approval_policy", "untrusted"), false},
		{"codex policy where none was", "codex", map[string]any{}, perm("approval_policy", "on-request"), true},
		{"codex full access", "codex", perm("sandbox_mode", "workspace-write"), perm("sandbox_mode", "danger-full-access"), true},
		{"codex read only", "codex", perm("sandbox_mode", "workspace-write"), perm("sandbox_mode", "read-only"), false},
		{"opencode allow everything", "opencode", perm("permission", "ask"), perm("permission", "allow"), true},
		{"opencode new allow tool", "opencode", perm("permission", perm("edit", "ask")), perm("permission", perm("edit", "ask", "bash", "allow")), true},
		{"opencode new ask tool", "opencode", perm("permission", perm("edit", "ask")), perm("permission", perm("edit", "ask", "bash", "ask")), false},
		{"opencode deny dropped", "opencode", perm("permission", perm("bash", perm("rm *", "deny"))), perm("permission", perm("bash", perm())), true},
		{"opencode deny everything", "opencode", perm("permission", "allow"), perm("permission", "deny"), false},
		{"pi has no permissions", "pi", map[string]any{}, perm("theme", "dark"), false},
	}
	for _, c := range cases {
		got := len(fleetWidens(c.agent, c.before, c.after)) > 0
		if got != c.widens {
			t.Errorf("%s: widens %v, want %v", c.name, got, c.widens)
		}
	}
}

// fleetSource writes a source machine's setup and returns its report.
func fleetSource(t *testing.T) Report {
	t.Helper()
	e := fixture(t)
	writeConfig(t, filepath.Join(e.home, ".claude", "CLAUDE.md"), "Use British spelling.\nRun the tests.\n")
	writeConfig(t, filepath.Join(e.home, ".claude", "settings.json"), `{"model":"big","permissions":{"allow":["Read","Bash(go test:*)"]},"outputStyle":"plain","hooks":{"x":"y"},"env":{"K":"v"},"enabledMcpjsonServers":["a"]}`)
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"permissions":{"allow":["Read"]},"oauthAccount":{"x":"y"}}`)
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), "model = \"m\"\napproval_policy = \"on-request\"\n[mcp_servers.x]\ncommand = \"x\"\n")
	writeConfig(t, filepath.Join(e.home, ".pi", "agent", "settings.json"), `{"defaultModel":"m","packages":["p"]}`)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func itemNamed(t *testing.T, items []terminalproto.SetupItem, agent, name string) terminalproto.SetupItem {
	t.Helper()
	for _, item := range items {
		if item.Agent == agent && item.Name == name {
			return item
		}
	}
	t.Fatalf("no %s %s in %+v", agent, name, items)
	return terminalproto.SetupItem{}
}

func TestReadFleetCarriesOnlyAllowlistedGlobalSetup(t *testing.T) {
	items, err := readFleet(fleetSource(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("items: %+v", items)
	}
	md := itemNamed(t, items, "claude", "CLAUDE.md")
	if md.Kind != "instructions" || md.Format != "text" || md.Content != "Use British spelling.\nRun the tests.\n" || md.SHA256 != terminalproto.SetupItemSHA256(md.Content) || md.Scope != "global" {
		t.Fatalf("instructions: %+v", md)
	}
	settings := itemNamed(t, items, "claude", "settings.json")
	for _, never := range []string{"model", "hooks", "env", "enabledMcpjsonServers", "big"} {
		if strings.Contains(settings.Content, never) {
			t.Fatalf("settings carried %q: %s", never, settings.Content)
		}
	}
	if !strings.Contains(settings.Content, "Bash(go test:*)") || !strings.Contains(settings.Content, "outputStyle") {
		t.Fatalf("settings: %s", settings.Content)
	}
	codex := itemNamed(t, items, "codex", "config.toml")
	if codex.Format != "toml" || strings.Contains(codex.Content, "mcp") || strings.Contains(codex.Content, "model") || !strings.Contains(codex.Content, "approval_policy") {
		t.Fatalf("codex: %+v", codex)
	}
	if err := CheckFleetItems(items); err != nil {
		t.Fatalf("own read does not pass its own check: %v", err)
	}
}

func TestReadFleetRefusesCredentialWithoutEchoingIt(t *testing.T) {
	e := fixture(t)
	token := "ghp_" + strings.Repeat("A1b2", 9)
	writeConfig(t, filepath.Join(e.home, ".codex", "AGENTS.md"), "Use this: "+token+"\n")
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	_, err = readFleet(r)
	var refusal *FleetRefusal
	if !errors.As(err, &refusal) || refusal.Reason != "credential" || refusal.Agent != "codex" || refusal.Name != "AGENTS.md" {
		t.Fatalf("refusal: %v", err)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatal("the refusal echoed the credential")
	}
	// The controller's check refuses the same item, also without echoing it.
	item := terminalproto.SetupItem{Agent: "codex", Scope: "global", Kind: "instructions", Name: "AGENTS.md", Format: "text", Content: token}
	item.SHA256 = terminalproto.SetupItemSHA256(item.Content)
	err = CheckFleetItems([]terminalproto.SetupItem{item})
	if !errors.As(err, &refusal) || refusal.Reason != "credential" || strings.Contains(err.Error(), token) {
		t.Fatalf("check: %v", err)
	}
}

func fleetTarget(t *testing.T) (environment, Report) {
	t.Helper()
	e := fixture(t)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	return e, r
}

func rediscover(t *testing.T, e environment) Report {
	t.Helper()
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPlanFleetMapsByNameIgnoringSourcePathsAndBlocksSymlinks(t *testing.T) {
	items, err := readFleet(fleetSource(t))
	if err != nil {
		t.Fatal(err)
	}
	for i := range items {
		items[i].Label = "/somewhere/else/" + items[i].Name
	}
	e, _ := fleetTarget(t)
	// The target's Claude instructions are a symlink: blocked, never followed.
	mkdir(t, filepath.Join(e.home, ".claude"))
	outside := filepath.Join(t.TempDir(), "outside.md")
	writeConfig(t, outside, "elsewhere\n")
	if err := os.Symlink(outside, filepath.Join(e.home, ".claude", "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	plan, err := planFleet(rediscover(t, e), items)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, f := range plan.Files {
		states[f.Agent+" "+f.Name] = f.State
		if f.Destination != "" && !strings.HasPrefix(f.Destination, e.home) {
			t.Fatalf("destination outside the target's own locations: %+v", f)
		}
	}
	want := map[string]string{"claude CLAUDE.md": "blocked", "claude settings.json": "create", "codex config.toml": "create"}
	if !reflect.DeepEqual(states, want) {
		t.Fatalf("states %v, want %v", states, want)
	}
	if _, err := applyFleet(rediscover(t, e), t.TempDir(), "fleet-test", plan.Fingerprint, items); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(outside); string(data) != "elsewhere\n" {
		t.Fatal("the symlink's target was written")
	}
	if _, err := os.Stat("/somewhere/else"); err == nil {
		t.Fatal("a source path was used")
	}
	// An instruction item never lands on a settings file of the same name.
	bad := terminalproto.SetupItem{Agent: "codex", Scope: "global", Kind: "instructions", Name: "config.toml", Format: "text", Content: "x"}
	bad.SHA256 = terminalproto.SetupItemSHA256(bad.Content)
	plan, err = planFleet(rediscover(t, e), []terminalproto.SetupItem{bad})
	if err != nil || plan.Files[0].State != "blocked" || plan.Files[0].Reason != "no_location" {
		t.Fatalf("instructions onto settings: %+v %v", plan.Files, err)
	}
}

func settingsItem(t *testing.T, agent, name, format, content string) terminalproto.SetupItem {
	t.Helper()
	item := terminalproto.SetupItem{Agent: agent, Scope: "global", Kind: "settings", Name: name, Format: format, Content: content}
	item.SHA256 = terminalproto.SetupItemSHA256(content)
	return item
}

func TestApplyFleetMergesFieldsKeepsTargetFieldsAndUndoes(t *testing.T) {
	e, _ := fleetTarget(t)
	path := filepath.Join(e.home, ".claude", "settings.json")
	original := `{"theme":"dark","hooks":{"PreToolUse":[]},"permissions":{"allow":["Read"]},"outputStyle":"plain"}`
	writeConfig(t, path, original)
	mdPath := filepath.Join(e.home, ".claude", "CLAUDE.md")
	writeConfig(t, mdPath, "Old line.\nKept line.\n")
	items := []terminalproto.SetupItem{
		settingsItem(t, "claude", "settings.json", "json", `{"permissions":{"allow":["Read","Bash"]},"outputStyle":"plain","cleanupPeriodDays":30}`),
		{Agent: "claude", Scope: "global", Kind: "instructions", Name: "CLAUDE.md", Format: "text", Content: "Kept line.\nNew line.\n"},
	}
	items[1].SHA256 = terminalproto.SetupItemSHA256(items[1].Content)
	plan, err := planFleet(rediscover(t, e), items)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := planFleet(rediscover(t, e), items)
	if again.Fingerprint != plan.Fingerprint || len(plan.Fingerprint) != 64 {
		t.Fatalf("fingerprint not stable: %s %s", plan.Fingerprint, again.Fingerprint)
	}
	var settings, md terminalproto.SetupFilePlan
	for _, f := range plan.Files {
		if f.Kind == "settings" {
			settings = f
		} else {
			md = f
		}
	}
	if settings.State != "merge" || !reflect.DeepEqual(settings.Fields, []string{"cleanupPeriodDays", "permissions"}) || !reflect.DeepEqual(settings.Widens, []string{"permissions"}) {
		t.Fatalf("settings plan: %+v", settings)
	}
	if md.State != "merge" || !reflect.DeepEqual(md.Added, []string{"New line."}) || !reflect.DeepEqual(md.Removed, []string{"Old line."}) {
		t.Fatalf("instructions plan: %+v", md)
	}
	journals := t.TempDir()
	id, err := applyFleet(rediscover(t, e), journals, "fleet-batch1", plan.Fingerprint, items)
	if err != nil {
		t.Fatal(err)
	}
	if !terminalproto.ValidSetupJournal(id) {
		t.Fatalf("journal id %q", id)
	}
	merged, err := parseSource(mustRead(t, path), "json")
	if err != nil {
		t.Fatal(err)
	}
	if merged["theme"] != "dark" || merged["hooks"] == nil || !sameValue(merged["cleanupPeriodDays"], 30) {
		t.Fatalf("target-only fields were not kept: %v", merged)
	}
	if !sameValue(merged["permissions"], map[string]any{"allow": []any{"Read", "Bash"}}) {
		t.Fatalf("permissions: %v", merged["permissions"])
	}
	if string(mustRead(t, mdPath)) != "Kept line.\nNew line.\n" {
		t.Fatal("instructions not replaced")
	}
	// The journal is one reet setup journals lists.
	list, err := ListRestoreJournals(journals)
	if err != nil || len(list.Journals) != 1 || list.Journals[0].State != "completed" {
		t.Fatalf("journals: %+v %v", list, err)
	}
	// A repeated apply after a lost reply returns the same journal and writes nothing.
	repeat, err := applyFleet(rediscover(t, e), journals, "fleet-batch1", plan.Fingerprint, items)
	if err != nil || repeat != id {
		t.Fatalf("repeat: %q %v", repeat, err)
	}
	// Now everything is unchanged.
	after, _ := planFleet(rediscover(t, e), items)
	for _, f := range after.Files {
		if f.State != "unchanged" {
			t.Fatalf("after apply: %+v", f)
		}
	}
	var refusal *FleetRefusal
	if _, err := applyFleet(rediscover(t, e), journals, "fleet-batch2", after.Fingerprint, items); !errors.As(err, &refusal) || refusal.Reason != "nothing" {
		t.Fatalf("nothing to do: %v", err)
	}

	// Undo restores the exact bytes.
	result, err := RollbackFleet(journals, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Restored) != 2 || string(mustRead(t, path)) != original || string(mustRead(t, mdPath)) != "Old line.\nKept line.\n" {
		t.Fatalf("undo: %+v", result)
	}
	if _, err := RollbackFleet(journals, "restore-nope"); !errors.Is(err, ErrNoJournal) {
		t.Fatalf("missing journal: %v", err)
	}
}

func TestRollbackFleetRefusesAFileChangedSinceTheApply(t *testing.T) {
	e, r := fleetTarget(t)
	items := []terminalproto.SetupItem{settingsItem(t, "codex", "config.toml", "toml", "approval_policy = \"never\"\n")}
	plan, err := planFleet(r, items)
	if err != nil {
		t.Fatal(err)
	}
	journals := t.TempDir()
	id, err := applyFleet(rediscover(t, e), journals, "fleet-x", plan.Fingerprint, items)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.home, ".codex", "config.toml")
	applied := mustRead(t, path)
	writeConfig(t, path, "approval_policy = \"untrusted\"\n")
	if _, err := RollbackFleet(journals, id); !errors.Is(err, ErrRollbackChanged) {
		t.Fatalf("changed file: %v", err)
	}
	if string(mustRead(t, path)) != "approval_policy = \"untrusted\"\n" {
		t.Fatal("a refused undo changed the file")
	}
	// The CLI reverses the same journal from the journal alone once the
	// person's edit is gone again.
	writeConfig(t, path, string(applied))
	var out bytes.Buffer
	if err := Run([]string{"rollback", "--journal", filepath.Join(journals, id+".json")}, &out); err != nil {
		t.Fatalf("cli: %v %s", err, out.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the created file was not removed")
	}
}

func TestApplyFleetRefusesAChangedTargetAndTheMergeChecksItsBase(t *testing.T) {
	e, _ := fleetTarget(t)
	path := filepath.Join(e.home, ".config", "opencode", "opencode.json")
	writeConfig(t, path, `{"theme":"a","permission":"ask"}`)
	items := []terminalproto.SetupItem{settingsItem(t, "opencode", "opencode.json", "json", `{"theme":"b"}`)}
	plan, err := planFleet(rediscover(t, e), items)
	if err != nil {
		t.Fatal(err)
	}
	// A person edits the file after the review: the fingerprint no longer matches.
	writeConfig(t, path, `{"theme":"a","permission":"deny"}`)
	if _, err := applyFleet(rediscover(t, e), t.TempDir(), "fleet-y", plan.Fingerprint, items); !errors.Is(err, ErrFleetChanged) {
		t.Fatalf("changed target: %v", err)
	}
	// The restore merge itself re-checks the base digest before writing.
	plan, err = planFleet(rediscover(t, e), items)
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, path, `{"theme":"a","permission":"allow"}`)
	plan.restore.SnapshotID = "fleet-z"
	if _, err := ApplyRestorePlan(plan.restore, RestoreApplyOptions{JournalDir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "changed after the plan") {
		t.Fatalf("merge base: %v", err)
	}
	if string(mustRead(t, path)) != `{"theme":"a","permission":"allow"}` {
		t.Fatal("a refused merge wrote the file")
	}
}

// A target file that holds what looks like a credential is never quoted in
// its plan: no removed lines, no field values, and the file is marked so the
// review can say why its lines are not shown.
func TestPlanFleetNeverQuotesATargetFileThatHoldsACredential(t *testing.T) {
	token := "ghp_" + strings.Repeat("A1b2", 9)
	e, _ := fleetTarget(t)
	writeConfig(t, filepath.Join(e.home, ".claude", "CLAUDE.md"), "Old line.\ntoken "+token+"\n")
	writeConfig(t, filepath.Join(e.home, ".claude", "settings.json"), `{"outputStyle":"`+token+`","theme":"dark"}`)
	md := terminalproto.SetupItem{Agent: "claude", Scope: "global", Kind: "instructions", Name: "CLAUDE.md", Format: "text", Content: "New line.\n"}
	md.SHA256 = terminalproto.SetupItemSHA256(md.Content)
	items := []terminalproto.SetupItem{md, settingsItem(t, "claude", "settings.json", "json", `{"outputStyle":"plain"}`)}
	plan, err := planFleet(rediscover(t, e), items)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(plan.Files)
	if strings.Contains(string(data), token) || strings.Contains(string(data), "Old line.") {
		t.Fatalf("the plan quotes the target: %s", data)
	}
	for _, f := range plan.Files {
		if f.State != "merge" || !f.HoldsCredential || len(f.Removed) != 0 {
			t.Fatalf("file not marked: %+v", f)
		}
	}
	// Applying still works: the reviewed plan says the file is replaced.
	if _, err := applyFleet(rediscover(t, e), t.TempDir(), "fleet-cred", plan.Fingerprint, items); err != nil {
		t.Fatal(err)
	}
}
