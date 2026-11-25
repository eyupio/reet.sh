package setup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/eyupio/reet.sh/internal/skillimport"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A fleet copy carries one machine's global instruction files and portable
// settings to other machines. It never carries skills, MCP servers, memory,
// project files, model defaults or anything the export policy keeps back
// without explicit credential selection.

// FleetRefusal is a copy refused for a closed reason. Agent and Name say
// which file, for a credential; the matched text is never kept.
type FleetRefusal struct {
	Reason, Agent, Name string
}

func (e *FleetRefusal) Error() string {
	if e.Name != "" {
		return "setup copy refused (" + e.Reason + "): " + e.Agent + " " + e.Name
	}
	return "setup copy refused (" + e.Reason + ")"
}

// ErrFleetChanged means the target's files changed since its plan was made.
var ErrFleetChanged = errors.New("the machine's setup changed since the plan was reviewed")

// fleetExcluded are fields the portable policy selects that a fleet copy
// never does: model defaults, and fields that run or enable commands.
var fleetExcluded = map[string]bool{
	"model": true, "defaultModel": true, "small_model": true, "model_provider": true,
	"model_reasoning_effort": true, "model_reasoning_summary": true, "model_verbosity": true,
	"model_context_window": true, "model_max_output_tokens": true, "defaultProvider": true,
	"formatter": true, "packages": true,
	"enabledMcpjsonServers": true, "enableAllProjectMcpServers": true, "disabledMcpjsonServers": true,
}

// fleetSettingsPolicy is the portable settings policy narrowed for a fleet
// copy. Claude's account file (.claude.json) holds machine state the agent
// rewrites constantly, and settings.local.json is machine-specific, so only
// settings.json is copied for Claude.
func fleetSettingsPolicy(agent, base string) ComponentExportPolicy {
	policy := settingsExportPolicy(agent, base)
	if agent == "claude" && base != "settings.json" {
		policy.Selection, policy.Handling = "unsupported", "not_implemented"
		return policy
	}
	fields := make([]FieldExportPolicy, 0, len(policy.Fields))
	for _, field := range policy.Fields {
		if field.State != "include" || fleetExcluded[field.Path] {
			field.State = "exclude"
		}
		fields = append(fields, field)
	}
	policy.Fields = fields
	return policy
}

// FleetSettingsFields lists the top-level fields a fleet copy carries for one
// settings file, sorted.
func FleetSettingsFields(agent, base string) []string {
	policy := fleetSettingsPolicy(agent, base)
	var out []string
	if policy.Selection != "include" {
		return out
	}
	for _, field := range policy.Fields {
		if field.State == "include" {
			out = append(out, field.Path)
		}
	}
	sort.Strings(out)
	return out
}

// ValidateFleetSettings is ValidatePortableSettings with the fleet allowlist:
// every field must be one a fleet copy carries.
func ValidateFleetSettings(agent, format, base string, data []byte) error {
	return validateSettingsPolicy(fleetSettingsPolicy(agent, base), agent, format, data)
}

// CheckFleetItems checks items from another machine before anything uses
// them: shape, digest, the fleet allowlist and credential-like content. A
// refusal names the file, never the matched text.
func CheckFleetItems(items []terminalproto.SetupItem) error {
	if len(items) == 0 || len(items) > terminalproto.MaxSetupItems {
		return &FleetRefusal{Reason: terminalproto.SetupReasonInvalid}
	}
	seen := map[string]bool{}
	for _, item := range items {
		key := item.Agent + "\x00" + item.Kind + "\x00" + item.Name
		validKind := item.Kind == "instructions" && item.Format == "text" ||
			item.Kind == "settings" && supportedFormat(item.Format, item.Agent)
		valid := terminalproto.ValidSetupAgent(item.Agent) && item.Scope == "global" && validKind &&
			item.Name != "" && filepath.Base(item.Name) == item.Name && !strings.ContainsAny(item.Name, `/\`) &&
			item.SHA256 == terminalproto.SetupItemSHA256(item.Content) && !seen[key]
		if !valid {
			return &FleetRefusal{Reason: terminalproto.SetupReasonInvalid, Agent: item.Agent}
		}
		seen[key] = true
		if skillimport.LooksLikeCredential([]byte(item.Content)) {
			return &FleetRefusal{Reason: terminalproto.SetupReasonCredential, Agent: item.Agent, Name: item.Name}
		}
		if item.Kind == "settings" && ValidateFleetSettings(item.Agent, item.Format, item.Name, []byte(item.Content)) != nil {
			return &FleetRefusal{Reason: terminalproto.SetupReasonInvalid, Agent: item.Agent, Name: item.Name}
		}
	}
	return nil
}

// ReadFleet reads this machine's global instruction files and the portable
// settings a fleet copy carries. It refuses the whole read when any file
// looks like it holds a credential.
func ReadFleet(home string) ([]terminalproto.SetupItem, error) {
	report, err := Discover(home, "")
	if err != nil {
		return nil, err
	}
	return readFleet(report)
}

func readFleet(report Report) ([]terminalproto.SetupItem, error) {
	cp := report
	cp.Agents = append([]Agent(nil), report.Agents...)
	ApplyExportPolicies(&cp)
	items := []terminalproto.SetupItem{}
	seen := map[string]bool{}
	for _, agent := range cp.Agents {
		if agent.ExportPolicy == nil || !terminalproto.ValidSetupAgent(agent.ID) {
			continue
		}
		sources := append([]SourceExportPolicy(nil), agent.ExportPolicy.Sources...)
		sort.Slice(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })
		for _, source := range sources {
			if source.Scope != "global" || source.Observed != "present" || len(source.Components) == 0 {
				continue
			}
			item := terminalproto.SetupItem{Agent: agent.ID, Scope: "global", Name: filepath.Base(source.Path), Label: source.Path}
			if len(source.Components) == 1 && source.Components[0].Kind == "instructions" {
				text, state, _ := readInstructionText(source.Path)
				if state != "parsed" {
					continue
				}
				item.Kind, item.Format, item.Content = "instructions", "text", text
			} else {
				policy := fleetSettingsPolicy(agent.ID, item.Name)
				if policy.Selection != "include" || !supportedFormat(source.Format, agent.ID) {
					continue
				}
				object, state, _ := readConfigObject(source.Path, source.Format, agent.ID)
				if state != "parsed" {
					continue
				}
				selected, _ := selectSettingsContent(object, policy, false)
				if len(selected) == 0 {
					continue
				}
				data, err := restoreContent(restoreFile{format: source.Format, settings: selected})
				if err != nil {
					continue
				}
				item.Kind, item.Format, item.Content = "settings", source.Format, string(data)
			}
			key := item.Agent + "\x00" + item.Kind + "\x00" + item.Name
			if seen[key] {
				continue // an override and a default with the same name: the first path wins
			}
			seen[key] = true
			item.SHA256 = terminalproto.SetupItemSHA256(item.Content)
			if skillimport.LooksLikeCredential([]byte(item.Content)) {
				return nil, &FleetRefusal{Reason: terminalproto.SetupReasonCredential, Agent: item.Agent, Name: item.Name}
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Agent != b.Agent {
			return a.Agent < b.Agent
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
	return items, nil
}

// FleetPlan is what a target would do with a copy. The restore plan inside it
// is what ApplyFleet writes, with the journal and rollback copies a CLI
// restore keeps.
type FleetPlan struct {
	Files       []terminalproto.SetupFilePlan
	Fingerprint string
	restore     RestorePlan
}

type fleetCandidate struct {
	agent, kind, path, state string
}

// fleetCandidates are this machine's own global instruction and settings
// locations. A copy is only ever written to one of them.
func fleetCandidates(report Report) []fleetCandidate {
	var out []fleetCandidate
	for _, agent := range report.Agents {
		for _, loc := range agent.Locations {
			kind := ""
			switch loc.Kind {
			case "instructions", "shared_instructions":
				kind = "instructions"
			case "configuration":
				kind = "settings"
			}
			if kind == "" || loc.Scope != "global" {
				continue
			}
			for _, path := range configPaths(agent.ID, loc) {
				state := loc.State
				if state == "present" {
					state = observeSource(path)
				}
				out = append(out, fleetCandidate{agent: agent.ID, kind: kind, path: path, state: state})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// PlanFleet maps each item to this machine's own location by agent, kind and
// file name (the source's path is never used) and says what applying would
// do. It writes nothing.
func PlanFleet(home string, items []terminalproto.SetupItem) (FleetPlan, error) {
	report, err := Discover(home, "")
	if err != nil {
		return FleetPlan{}, err
	}
	return planFleet(report, items)
}

func planFleet(report Report, items []terminalproto.SetupItem) (FleetPlan, error) {
	if err := CheckFleetItems(items); err != nil {
		return FleetPlan{}, err
	}
	candidates := fleetCandidates(report)
	plan := FleetPlan{Files: []terminalproto.SetupFilePlan{}}
	plan.restore = RestorePlan{
		SchemaVersion: restorePlanSchemaVersion, Actions: []RestoreAction{}, Unsupported: []RestoreUnsupported{},
		files: map[string]restoreFile{},
	}
	type fingerprintEntry struct {
		Agent, Kind, Name, Destination, State, Reason, Item, Base string
		Fields                                                    []string
	}
	var entries []fingerprintEntry
	for _, item := range items {
		f := terminalproto.SetupFilePlan{Agent: item.Agent, Kind: item.Kind, Name: item.Name}
		var candidate *fleetCandidate
		for i := range candidates {
			c := candidates[i]
			if c.agent == item.Agent && c.kind == item.Kind && filepath.Base(c.path) == item.Name {
				candidate = &c
				break
			}
		}
		file := restoreFile{agentID: item.Agent, format: item.Format}
		var source map[string]any
		if item.Kind == "instructions" {
			file.format, file.text = "text", item.Content
		} else {
			var err error
			if source, err = parseSource([]byte(item.Content), item.Format); err != nil {
				return FleetPlan{}, &FleetRefusal{Reason: terminalproto.SetupReasonInvalid, Agent: item.Agent, Name: item.Name}
			}
			file.settings = source
		}
		switch {
		case candidate == nil:
			f.State, f.Reason = "blocked", "no_location"
		case candidate.state == "missing":
			f.Destination, f.State = candidate.path, "create"
			if source != nil {
				f.Fields = sortedKeys(source)
				f.Widens = fleetWidens(item.Agent, map[string]any{}, source)
			} else {
				f.Added, _, f.Truncated = lineDiff("", item.Content)
			}
		case candidate.state == "present":
			f.Destination = candidate.path
			info, err := os.Lstat(candidate.path)
			var current []byte
			issue := "metadata_unavailable"
			if err == nil && info.Mode().IsRegular() {
				current, issue = readConfig(candidate.path, info)
			}
			if issue != "" {
				f.State, f.Reason = "blocked", "unreadable"
				break
			}
			file.baseDigest = sha256Bytes(current)
			if source == nil {
				if string(current) == item.Content {
					f.State = "unchanged"
					break
				}
				f.State = "merge"
				f.Added, f.Removed, f.Truncated = lineDiff(string(current), item.Content)
				break
			}
			target, err := parseSource(current, item.Format)
			if err != nil {
				f.State, f.Reason = "blocked", "unreadable"
				break
			}
			for _, key := range sortedKeys(source) {
				if !sameValue(target[key], source[key]) {
					f.Fields = append(f.Fields, key)
				}
			}
			if len(f.Fields) == 0 {
				f.State = "unchanged"
				break
			}
			f.State, file.base = "merge", target
			f.Widens = fleetWidens(item.Agent, target, source)
		default:
			// A symlink, an unreadable path or something that is not a file.
			f.Destination, f.State, f.Reason = candidate.path, "blocked", "destination_"+candidate.state
		}
		if f.State == "create" || f.State == "merge" {
			plan.restore.Actions = append(plan.restore.Actions, RestoreAction{
				AgentID: item.Agent, Components: []string{item.Kind}, Sources: []string{item.Name},
				Destination: f.Destination, Scope: "global", Format: file.format, State: f.State,
			})
			plan.restore.files[f.Destination] = file
		}
		plan.Files = append(plan.Files, f)
		entries = append(entries, fingerprintEntry{item.Agent, item.Kind, item.Name, f.Destination, f.State, f.Reason, item.SHA256, file.baseDigest, f.Fields})
	}
	sort.Slice(plan.Files, func(i, j int) bool {
		a, b := plan.Files[i], plan.Files[j]
		if a.Agent != b.Agent {
			return a.Agent < b.Agent
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Agent+"\x00"+entries[i].Kind+"\x00"+entries[i].Name < entries[j].Agent+"\x00"+entries[j].Kind+"\x00"+entries[j].Name
	})
	data, _ := json.Marshal(entries)
	sum := sha256.Sum256(data)
	plan.Fingerprint = hex.EncodeToString(sum[:])
	plan.restore.Fingerprint = plan.Fingerprint
	return plan, nil
}

// ApplyFleet rebuilds the plan, refuses it when it no longer matches the
// reviewed fingerprint, and writes it through the restore journal: rollback
// copies of every file it changes, a re-check of each file before it is
// written, and its own writes undone on failure. label names the journal; an
// apply repeated after a lost reply finds its own completed journal and
// returns it instead of writing again. It returns the journal id.
func ApplyFleet(home, journalDir, label, fingerprint string, items []terminalproto.SetupItem) (string, error) {
	report, err := Discover(home, "")
	if err != nil {
		return "", err
	}
	return applyFleet(report, journalDir, label, fingerprint, items)
}

func applyFleet(report Report, journalDir, label, fingerprint string, items []terminalproto.SetupItem) (string, error) {
	if journalDir == "" {
		dir, err := SnapshotDir()
		if err != nil {
			return "", err
		}
		journalDir = dir
	}
	if list, err := ListRestoreJournals(journalDir); err == nil {
		for _, journal := range list.Journals {
			if journal.SnapshotID == label && journal.State == "completed" && journal.Fingerprint == fingerprint {
				return strings.TrimSuffix(filepath.Base(journal.Path), ".json"), nil
			}
		}
	}
	plan, err := planFleet(report, items)
	if err != nil {
		return "", err
	}
	if plan.Fingerprint != fingerprint {
		return "", ErrFleetChanged
	}
	if len(plan.restore.Actions) == 0 {
		return "", &FleetRefusal{Reason: terminalproto.SetupReasonNothing}
	}
	plan.restore.SnapshotID = label
	result, err := ApplyRestorePlan(plan.restore, RestoreApplyOptions{JournalDir: journalDir})
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(filepath.Base(result.JournalPath), ".json"), nil
}

// RollbackFleet undoes one applied copy from its journal id, checking that
// each file is still exactly what the apply wrote.
func RollbackFleet(journalDir, id string) (RestoreRollbackResult, error) {
	if !terminalproto.ValidSetupJournal(id) {
		return RestoreRollbackResult{}, fmt.Errorf("%w: not a journal id", ErrNoJournal)
	}
	if journalDir == "" {
		dir, err := SnapshotDir()
		if err != nil {
			return RestoreRollbackResult{}, err
		}
		journalDir = dir
	}
	path := filepath.Join(journalDir, id+".json")
	if _, err := os.Lstat(path); err != nil {
		return RestoreRollbackResult{}, fmt.Errorf("%w: %s", ErrNoJournal, id)
	}
	return RollbackJournal(path, RestoreRollbackOptions{})
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// sameValue compares decoded values by their JSON form, so a TOML integer and
// the same number decoded elsewhere compare equal.
func sameValue(a, b any) bool {
	x, errX := json.Marshal(a)
	y, errY := json.Marshal(b)
	return errX == nil && errY == nil && bytes.Equal(x, y)
}

// fleetWidens names the permission fields whose new value would let agents
// do more without asking. It is deliberately conservative: anything it cannot
// show to be the same or narrower counts as wider.
func fleetWidens(agent string, before, after map[string]any) []string {
	var out []string
	switch agent {
	case "claude":
		if value, ok := after["permissions"]; ok && claudePermissionsWiden(before["permissions"], value) {
			out = append(out, "permissions")
		}
	case "codex":
		if value, ok := after["approval_policy"]; ok && rankWidens([]string{"untrusted", "on-request", "on-failure", "never"}, before["approval_policy"], value) {
			out = append(out, "approval_policy")
		}
		if value, ok := after["sandbox_mode"]; ok && rankWidens([]string{"read-only", "workspace-write", "danger-full-access"}, before["sandbox_mode"], value) {
			out = append(out, "sandbox_mode")
		}
	case "opencode":
		if value, ok := after["permission"]; ok && openCodePermissionWidens(before["permission"], value) {
			out = append(out, "permission")
		}
	}
	return out
}

// rankWidens reports whether after is further along order than before. A
// before that is missing or unknown counts as the narrowest; an after that is
// unknown counts as wider unless it is unchanged.
func rankWidens(order []string, before, after any) bool {
	if sameValue(before, after) {
		return false
	}
	index := func(v any) int {
		s, ok := v.(string)
		if !ok {
			return -1
		}
		for i, o := range order {
			if o == s {
				return i
			}
		}
		return -1
	}
	a, b := index(after), index(before)
	if a < 0 {
		return true
	}
	if b < 0 {
		b = 0
	}
	return a > b
}

// subsetOf reports whether every entry of list x is in list y. A missing x is
// empty; anything other than a list is not a subset.
func subsetOf(x, y any) bool {
	if x == nil {
		return true
	}
	xs, ok := x.([]any)
	if !ok {
		return false
	}
	ys, _ := y.([]any)
	for _, v := range xs {
		found := false
		for _, w := range ys {
			if sameValue(v, w) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// claudePermissionsWiden: a new allow rule or extra directory, a deny or ask
// rule dropped, a more permissive default mode, or any other change.
func claudePermissionsWiden(before, after any) bool {
	b, _ := before.(map[string]any)
	a, ok := after.(map[string]any)
	if !ok {
		return !sameValue(before, after)
	}
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	for k := range keys {
		switch k {
		case "allow", "additionalDirectories":
			if !subsetOf(a[k], b[k]) {
				return true
			}
		case "deny", "ask":
			if !subsetOf(b[k], a[k]) {
				return true
			}
		case "defaultMode":
			mode := func(v any) any {
				if v == nil {
					return "default"
				}
				return v
			}
			if rankWidens([]string{"plan", "default", "acceptEdits", "bypassPermissions"}, mode(b[k]), mode(a[k])) {
				return true
			}
		default:
			if !sameValue(b[k], a[k]) {
				return true
			}
		}
	}
	return false
}

// openCodePermissionWidens compares OpenCode's permission rules, a single
// action or a tree of tool and pattern names to deny, ask or allow. A rule
// that becomes more permissive, a new allow, or a deny or ask that is dropped
// counts as wider.
func openCodePermissionWidens(before, after any) bool {
	var flatten func(v any, prefix string, out map[string]any)
	flatten = func(v any, prefix string, out map[string]any) {
		if m, ok := v.(map[string]any); ok {
			for k, c := range m {
				flatten(c, prefix+"/"+k, out)
			}
			return
		}
		out[prefix] = v
	}
	fb, fa := map[string]any{}, map[string]any{}
	if before != nil {
		flatten(before, "", fb)
	}
	flatten(after, "", fa)
	order := []string{"deny", "ask", "allow"}
	for path, value := range fa {
		previous, ok := fb[path]
		if !ok {
			previous = "ask"
		}
		if rankWidens(order, previous, value) {
			return true
		}
	}
	for path, value := range fb {
		if _, kept := fa[path]; !kept && value != "allow" {
			return true
		}
	}
	return false
}

// lineDiff lists the lines after adds and the lines it drops, as a bounded,
// order-keeping multiset difference. Tabs become spaces and other control
// characters are dropped, so each line is plain text.
func lineDiff(before, after string) (added, removed []string, truncated bool) {
	split := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n"), "\n")
	}
	clean := func(line string) string {
		line = strings.Map(func(r rune) rune {
			if r == '\t' {
				return ' '
			}
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ' {
				return -1
			}
			return r
		}, line)
		if runes := []rune(line); len(runes) > 200 {
			truncated = true
			return string(runes[:200]) + "…"
		}
		return line
	}
	count := map[string]int{}
	old := split(before)
	for _, line := range old {
		count[line]++
	}
	for _, line := range split(after) {
		if count[line] > 0 {
			count[line]--
			continue
		}
		if len(added) == terminalproto.MaxSetupDiffLines {
			truncated = true
			continue
		}
		added = append(added, clean(line))
	}
	for _, line := range old {
		if count[line] == 0 {
			continue
		}
		count[line]--
		if len(removed) == terminalproto.MaxSetupDiffLines {
			truncated = true
			continue
		}
		removed = append(removed, clean(line))
	}
	return added, removed, truncated
}
