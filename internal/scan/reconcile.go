package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

// managedFolders are the home skills folders that follow reet, with the agent
// each belongs to. Reconcile reads and writes nothing else: not project
// folders, not .codex/skills, and not a custom CLAUDE_CONFIG_DIR or
// XDG_CONFIG_HOME.
var managedFolders = []struct{ Dir, Agent string }{{".agents/skills", "agents"}, {".claude/skills", "claude"}, {".cursor/skills", "cursor"}, {".config/opencode/skills", "opencode"}}

// bundleFetcher returns the files of an Active skill's revision.
type bundleFetcher func(ctx context.Context, s store.DesiredSkill) ([]skillbundle.ContentFile, error)

const sharedSkillsDir = ".agents/skills"

var skillDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// reconcileDigest reads the digest of a skill folder on disk. Tests replace
// it to stand for a disk that does not keep files exactly.
var reconcileDigest = privateSkillDigest

// reconcileOrder visits ~/.agents/skills first, because Cursor and OpenCode
// link to the copy there, and Claude's own copies last.
var reconcileOrder = []int{0, 2, 3, 1}

// reconcileSkills makes the managed folders under home match d and reports
// what it found and did, one entry per skill and folder. installed maps
// "<folder>/<lower-case name>" to the digest reet last installed there; the
// map returned replaces it.
//
// ~/.agents/skills is created when an Active skill must go there. The other
// folders are used only when they exist: Cursor and OpenCode get links to the
// ~/.agents/skills copy, Claude gets its own. Anything removed or replaced is
// moved into a dated backup first. Names are matched ignoring case. A failure
// stops only that skill in that folder.
func reconcileSkills(ctx context.Context, home string, d store.DesiredState, installed map[string]string, fetch bundleFetcher, now time.Time) (store.SkillReport, map[string]string) {
	r := &reconciler{ctx: ctx, home: home, now: now, fetch: fetch, installed: installed,
		next: map[string]string{}, bundles: map[string]fetchedBundle{}, digests: map[string]string{}, shared: map[string]string{}}
	rep := store.SkillReport{Version: d.Version, Entries: []store.SkillReportEntry{}}
	active, removals, refused := checkDesired(d)
	r.entries = refused
	if !filepath.IsAbs(home) {
		// Without a home folder every path would be relative to wherever the
		// agent runs.
		for _, a := range active {
			r.add("", a.Name, "failed", "", "this machine's home folder is not known", "")
		}
		for _, rm := range removals {
			r.add("", rm.Name, "failed", "", "this machine's home folder is not known", "")
		}
		maps.Copy(r.next, installed)
		rep.Entries = r.entries
		return rep, r.next
	}
	r.clearStaging()
	claimed := map[string]bool{}
	for _, a := range active {
		claimed[strings.ToLower(a.Name)] = true
	}
	for _, rm := range removals {
		claimed[strings.ToLower(rm.Name)] = true
	}
	for _, i := range reconcileOrder {
		r.folder(managedFolders[i].Dir, managedFolders[i].Agent, active, removals, claimed)
	}
	rep.Entries = r.entries
	return rep, r.next
}

// checkDesired keeps the names that can safely name a folder entry, and the
// Active skills with a digest to check their files against. Everything else
// becomes a failed entry and touches nothing. A name listed twice is used
// once, and a name both Active and removed stays Active.
func checkDesired(d store.DesiredState) ([]store.DesiredSkill, []store.DesiredRemoval, []store.SkillReportEntry) {
	refused := []store.SkillReportEntry{}
	seen := map[string]bool{}
	var active []store.DesiredSkill
	for _, a := range d.Active {
		lower := strings.ToLower(a.Name)
		switch problem := skillNameProblem(a.Name); {
		case problem != "":
			refused = append(refused, store.SkillReportEntry{Name: a.Name, State: "failed", Detail: problem})
		case !skillDigestPattern.MatchString(a.Digest):
			refused = append(refused, store.SkillReportEntry{Name: a.Name, State: "failed", Detail: "refused: no digest to check the files against"})
		case seen[lower]:
			refused = append(refused, store.SkillReportEntry{Name: a.Name, State: "failed", Detail: "listed twice"})
		default:
			seen[lower] = true
			active = append(active, a)
		}
	}
	var removals []store.DesiredRemoval
	for _, rm := range d.Remove {
		lower := strings.ToLower(rm.Name)
		switch problem := skillNameProblem(rm.Name); {
		case problem != "":
			refused = append(refused, store.SkillReportEntry{Name: rm.Name, State: "failed", Detail: problem})
		case !seen[lower]:
			seen[lower] = true
			removals = append(removals, rm)
		}
	}
	return active, removals, refused
}

// skillNameProblem says why a name from the controller cannot name an entry
// in a skills folder, or "" when it can: it must be one plain path element.
func skillNameProblem(name string) string {
	switch {
	case !validBackupName(name):
		return "refused: not a plain skill name"
	case len(name) > 255:
		return "refused: the name is longer than 255 bytes"
	case strings.HasPrefix(name, "."):
		return "refused: a skill name cannot start with a dot"
	}
	return ""
}

type fetchedBundle struct {
	files []skillbundle.ContentFile
	err   error
}

type reconciler struct {
	ctx       context.Context
	home      string
	now       time.Time
	fetch     bundleFetcher
	installed map[string]string
	next      map[string]string
	bundles   map[string]fetchedBundle // by lower-case name: one fetch a pass
	digests   map[string]string        // by path, for real folders read this pass
	shared    map[string]string        // lower-case name → reet's copy in ~/.agents/skills
	entries   []store.SkillReportEntry
}

func (r *reconciler) add(folder, name, state, digest, detail, backup string) {
	r.entries = append(r.entries, store.SkillReportEntry{Folder: folder, Name: name, State: state, Digest: digest, Detail: detail, Backup: backup})
}

// clearStaging removes .reet-private-* folders an interrupted install left
// beside the managed folders.
func (r *reconciler) clearStaging() {
	for _, f := range managedFolders {
		parent := filepath.Dir(filepath.Join(r.home, filepath.FromSlash(f.Dir)))
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".reet-private-") {
				// RemoveAll removes a link without following it.
				_ = os.RemoveAll(filepath.Join(parent, e.Name()))
			}
		}
	}
}

// skillEntries lists a skills folder's entries by lower-case name, leaving out
// hidden ones. A missing folder gives nil.
func skillEntries(dir string) (map[string][]string, error) {
	list, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, e := range list {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		lower := strings.ToLower(e.Name())
		out[lower] = append(out[lower], e.Name())
	}
	return out, nil
}

func (r *reconciler) folder(rel, agent string, active []store.DesiredSkill, removals []store.DesiredRemoval, claimed map[string]bool) {
	dir := filepath.Join(r.home, filepath.FromSlash(rel))
	label := "~/" + rel
	names, err := skillEntries(dir)
	if err != nil {
		for _, a := range active {
			r.fail(label, rel, a, "cannot read "+label+": "+err.Error(), "")
		}
		for _, rm := range removals {
			r.add(label, rm.Name, "failed", "", "cannot read "+label+": "+err.Error(), "")
		}
		return
	}
	if names == nil && rel != sharedSkillsDir {
		return
	}
	linked := agent == "cursor" || agent == "opencode"
	for _, a := range active {
		r.activeSkill(dir, rel, label, linked, a, names[strings.ToLower(a.Name)])
	}
	for _, rm := range removals {
		for _, name := range names[strings.ToLower(rm.Name)] {
			dest, err := r.backup(dir, name)
			if err != nil {
				r.add(label, rm.Name, "failed", "", "could not back up "+name+": "+err.Error(), "")
				continue
			}
			r.add(label, rm.Name, "removed", "", "", dest)
		}
	}
	for _, lower := range slices.Sorted(maps.Keys(names)) {
		if claimed[lower] {
			continue
		}
		for _, name := range names[lower] {
			if digest, ok := r.skillAt(dir, name); ok {
				r.add(label, name, "unknown", digest, "", "")
			}
		}
	}
}

// activeSkill keeps one copy of a in dir that is reet's: the present one when
// its digest matches, or a new one. Every other copy under the same name, in
// any case, is backed up first.
func (r *reconciler) activeSkill(dir, rel, label string, linked bool, a store.DesiredSkill, matches []string) {
	key := rel + "/" + strings.ToLower(a.Name)
	keep := ""
	for _, name := range matches {
		if r.digest(dir, name) == a.Digest && (keep == "" || name == a.Name) {
			keep = name
		}
	}
	staging, source := "", ""
	if keep == "" {
		var err error
		if linked {
			if source = r.shared[strings.ToLower(a.Name)]; source == "" {
				r.fail(label, rel, a, "not installed in ~/"+sharedSkillsDir+" to link to", "")
				return
			}
		} else if staging, err = r.stage(dir, a); err != nil {
			r.fail(label, rel, a, err.Error(), "")
			return
		}
	}
	if staging != "" {
		defer os.RemoveAll(staging)
	}
	prev, hadPrev := r.installed[key]
	state, backup := "installed", ""
	for _, name := range matches {
		if name == keep {
			continue
		}
		was := r.digest(dir, name)
		dest, err := r.backup(dir, name)
		if err != nil {
			r.fail(label, rel, a, "could not back up "+name+": "+err.Error(), backup)
			return
		}
		if backup == "" {
			backup = dest
		}
		// A readable copy that is neither this revision nor the one reet
		// last put here was changed on this machine.
		if was != "" && was != a.Digest && (!hadPrev || was != prev) {
			state = "edited"
		}
	}
	switch {
	case linked && keep == "":
		if err := linkSharedInstall(r.home, dir, source); err != nil {
			r.fail(label, rel, a, err.Error(), backup)
			return
		}
	case keep == "":
		dest := filepath.Join(dir, a.Name)
		if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
			r.fail(label, rel, a, "another copy appeared while installing", backup)
			return
		}
		if err := os.Rename(staging, dest); err != nil {
			r.fail(label, rel, a, err.Error(), backup)
			return
		}
		r.digests[dest] = a.Digest
		keep = a.Name
	}
	if rel == sharedSkillsDir {
		r.shared[strings.ToLower(a.Name)] = keep
	}
	r.next[key] = a.Digest
	r.add(label, a.Name, state, a.Digest, "", backup)
}

// fail reports a failed skill and keeps what reet last installed there.
func (r *reconciler) fail(label, rel string, a store.DesiredSkill, detail, backup string) {
	key := rel + "/" + strings.ToLower(a.Name)
	if prev, ok := r.installed[key]; ok {
		r.next[key] = prev
	}
	r.add(label, a.Name, "failed", "", detail, backup)
}

// stage fetches and checks a's files, once a pass, and writes them into a
// staging folder beside dir, creating dir when it is missing. The staged copy
// must read back with a's digest.
func (r *reconciler) stage(dir string, a store.DesiredSkill) (string, error) {
	lower := strings.ToLower(a.Name)
	b, ok := r.bundles[lower]
	if !ok {
		if b.err = r.ctx.Err(); b.err == nil {
			b.files, b.err = r.fetch(r.ctx, a)
		}
		if b.err == nil {
			b.files, b.err = checkSkillFiles(b.files, a.Digest)
		}
		r.bundles[lower] = b
	}
	if b.err != nil {
		return "", b.err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	staging, err := stageSkillFiles(filepath.Dir(dir), b.files)
	if err != nil {
		return "", err
	}
	// On a disk that does not keep files exactly (fixed modes on SMB, exFAT
	// or some FUSE mounts), reet's copy would read as changed on every pass
	// and be replaced again each time. Nothing present is touched then.
	if got, err := reconcileDigest(staging); err != nil || got != a.Digest {
		os.RemoveAll(staging)
		return "", errors.New("These files don't read back as reet's copy on this disk")
	}
	return staging, nil
}

// backup moves dir/name into the dated backups beside dir and returns where
// it went, as a ~/ path.
func (r *reconciler) backup(dir, name string) (string, error) {
	p := filepath.Join(dir, name)
	dest, err := backupSkill(dir, name, r.now)
	delete(r.digests, p)
	if dest == "" {
		return "", err
	}
	// A move whose index could not be written still moved the copy; it is
	// listed by its folder name instead.
	return r.tilde(dest), nil
}

func (r *reconciler) tilde(p string) string {
	rel, err := filepath.Rel(r.home, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return p
	}
	return "~/" + filepath.ToSlash(rel)
}

// digest is the digest of the copy at dir/name, or "" when it cannot be read
// as a skill. A link is followed only into ~/.agents/skills, where reet keeps
// the copy Cursor, OpenCode and the skills CLI link to; nothing outside the
// managed folders is read.
func (r *reconciler) digest(dir, name string) string {
	p := filepath.Join(dir, name)
	st, err := os.Lstat(p)
	if err != nil {
		return ""
	}
	if st.Mode()&os.ModeSymlink != 0 {
		if p = r.sharedTarget(dir, p); p == "" {
			return ""
		}
		if st, err = os.Lstat(p); err != nil {
			return ""
		}
	}
	if !st.IsDir() {
		return ""
	}
	if d, ok := r.digests[p]; ok {
		return d
	}
	d, err := reconcileDigest(p)
	if err != nil {
		d = ""
	}
	r.digests[p] = d
	return d
}

// sharedTarget returns where the link at p points when that is an entry of
// ~/.agents/skills and p is in another folder, or "".
func (r *reconciler) sharedTarget(dir, p string) string {
	shared := filepath.Join(r.home, filepath.FromSlash(sharedSkillsDir))
	if filepath.Clean(dir) == shared {
		return ""
	}
	target, err := os.Readlink(p)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	target = filepath.Clean(target)
	if filepath.Dir(target) != shared {
		return ""
	}
	return target
}

// skillAt says whether dir/name is a skill to report, with its digest: a
// folder holding a SKILL.md, or a link, which is never followed out of the
// managed folders.
func (r *reconciler) skillAt(dir, name string) (string, bool) {
	p := filepath.Join(dir, name)
	st, err := os.Lstat(p)
	if err != nil {
		return "", false
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return r.digest(dir, name), true
	}
	if !st.IsDir() {
		return "", false
	}
	if md, err := os.Lstat(filepath.Join(p, "SKILL.md")); err != nil || !md.Mode().IsRegular() {
		return "", false
	}
	return r.digest(dir, name), true
}

// skillInventory is the report of a machine that may not write: what the
// managed folders hold, changing nothing. Every copy of an Active or removed
// name is reported under that name, in any case: an Active copy that is reet's
// is installed and one that differs has failed; a removed name still present
// is installed, so the controller does not take it for gone. Only other
// skills are unknown.
func skillInventory(home string, d store.DesiredState) store.SkillReport {
	active, removals, _ := checkDesired(d)
	want := map[string]store.DesiredSkill{}
	for _, a := range active {
		want[strings.ToLower(a.Name)] = a
	}
	gone := map[string]string{}
	for _, rm := range removals {
		gone[strings.ToLower(rm.Name)] = rm.Name
	}
	r := &reconciler{home: home, digests: map[string]string{}}
	rep := store.SkillReport{Version: d.Version, Blocked: "allow-apply", Entries: []store.SkillReportEntry{}}
	if !filepath.IsAbs(home) {
		return rep
	}
	for _, i := range reconcileOrder {
		rel := managedFolders[i].Dir
		dir := filepath.Join(home, filepath.FromSlash(rel))
		label := "~/" + rel
		names, err := skillEntries(dir)
		if err != nil {
			continue
		}
		for _, lower := range slices.Sorted(maps.Keys(names)) {
			for _, name := range names[lower] {
				if a, ok := want[lower]; ok {
					if digest := r.digest(dir, name); digest == a.Digest {
						r.add(label, a.Name, "installed", digest, "", "")
					} else {
						r.add(label, a.Name, "failed", "", "allow-apply is off", "")
					}
				} else if removed, ok := gone[lower]; ok {
					r.add(label, removed, "installed", r.digest(dir, name), "", "")
				} else if digest, ok := r.skillAt(dir, name); ok {
					r.add(label, name, "unknown", digest, "", "")
				}
			}
		}
	}
	if r.entries != nil {
		rep.Entries = r.entries
	}
	return rep
}

// controllerBundles fetches an Active skill's revision from the controller,
// checking it as every private install does.
func controllerBundles(c agentConfig) bundleFetcher {
	return func(ctx context.Context, s store.DesiredSkill) ([]skillbundle.ContentFile, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		source := "private:" + s.Bundle + ":" + s.Revision + ":" + s.Digest
		return privateSkillFiles(c.Server, c.Token, store.SyncSkill{Slug: s.Name, SourceType: "private", Source: source})
	}
}

// desiredCache is <config>.desired: the last desired state a poll carried,
// with the controller and machine it was for.
type desiredCache struct {
	Server  string             `json:"server"`
	Agent   string             `json:"agent"`
	Desired store.DesiredState `json:"desired"`
}

// readDesiredSkills returns the desired state the last poll that carried one
// left, or nil. One kept for another controller or enrollment is not used.
func readDesiredSkills(config string, c agentConfig) *store.DesiredState {
	b, err := os.ReadFile(config + ".desired")
	if err != nil {
		return nil
	}
	var kept desiredCache
	if json.Unmarshal(b, &kept) != nil || kept.Server != c.Server || kept.Agent != c.ID {
		return nil
	}
	return &kept.Desired
}

// unsentCache is <config>.unsent: entries no report has delivered yet that a
// later pass would not report again, with the controller and machine they
// are for.
type unsentCache struct {
	Server  string                   `json:"server"`
	Agent   string                   `json:"agent"`
	Entries []store.SkillReportEntry `json:"entries"`
}

// unsentKept caps the entries kept while reports fail, keeping the newest.
const unsentKept = 500

func readUnsentSkills(config string, c agentConfig) []store.SkillReportEntry {
	b, err := os.ReadFile(config + ".unsent")
	if err != nil {
		return nil
	}
	var kept unsentCache
	if json.Unmarshal(b, &kept) != nil || kept.Server != c.Server || kept.Agent != c.ID {
		return nil
	}
	return kept.Entries
}

// newsOf returns what a pass did that the next pass cannot see: an edit it
// backed up and replaced, and a copy it removed, each with its backup.
func newsOf(rep store.SkillReport) []store.SkillReportEntry {
	var news []store.SkillReportEntry
	for _, e := range rep.Entries {
		if e.State == "edited" || e.State == "removed" && e.Backup != "" {
			news = append(news, e)
		}
	}
	return news
}

// withUnsent adds entries earlier reports carried but never delivered. Each
// goes just before this report's entry for the same skill and folder, which
// the controller records last, or at the end. An installed entry of the same
// revision says nothing an unsent edit does not, so it is left out.
func withUnsent(rep store.SkillReport, unsent []store.SkillReportEntry) store.SkillReport {
	if len(unsent) == 0 {
		return rep
	}
	key := func(e store.SkillReportEntry) string { return e.Folder + "\x00" + strings.ToLower(e.Name) }
	waiting := map[string][]store.SkillReportEntry{}
	for _, u := range unsent {
		waiting[key(u)] = append(waiting[key(u)], u)
	}
	entries := make([]store.SkillReportEntry, 0, len(rep.Entries)+len(unsent))
	for _, e := range rep.Entries {
		before := waiting[key(e)]
		delete(waiting, key(e))
		entries = append(entries, before...)
		if n := len(before); n > 0 && e.State == "installed" && before[n-1].State == "edited" && before[n-1].Digest == e.Digest {
			continue
		}
		entries = append(entries, e)
	}
	for _, u := range unsent {
		if _, left := waiting[key(u)]; left {
			entries = append(entries, u)
		}
	}
	rep.Entries = entries
	return rep
}

func readInstalledSkills(config string) map[string]string {
	installed := map[string]string{}
	b, err := os.ReadFile(config + ".installed")
	if err != nil || json.Unmarshal(b, &installed) != nil || installed == nil {
		return map[string]string{}
	}
	return installed
}

// skillReportsSent holds, per enrollment file, the last skill report this
// process posted, so an unchanged report is not sent again.
var skillReportsSent = struct {
	sync.Mutex
	last map[string]string
}{last: map[string]string{}}

// reconcileAgentSkills runs one reconcile with fresh, the desired state this
// poll carried, or else cached, the one an earlier poll left. With neither it
// does nothing. With allow-apply off it only reports what is there.
func reconcileAgentSkills(ctx context.Context, c agentConfig, config string, fresh, cached *store.DesiredState, version string, now time.Time) error {
	var errs []error
	d := cached
	if fresh != nil {
		d = fresh
		was, _ := json.Marshal(cached)
		is, _ := json.Marshal(fresh)
		if !bytes.Equal(was, is) {
			if err := writePrivate(config+".desired", desiredCache{Server: c.Server, Agent: c.ID, Desired: *fresh}); err != nil {
				errs = append(errs, fmt.Errorf("could not keep the skills this machine should have: %w", err))
			}
		}
	}
	if d == nil {
		return nil
	}
	// Read the local opt-in fresh, so allow-apply takes effect on the next
	// cycle without restarting the agent.
	local, err := loadAgentConfig(config)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	earlier := readUnsentSkills(config, c)
	unsent := earlier
	var rep store.SkillReport
	if local.AllowApply {
		installed := readInstalledSkills(config)
		var next map[string]string
		rep, next = reconcileSkills(ctx, c.Home, *d, installed, controllerBundles(c), now)
		// Kept before anything else is written, so a report that never
		// arrives cannot lose an edit's backup or a removal.
		if news := newsOf(rep); len(news) > 0 {
			unsent = append(unsent, news...)
			if len(unsent) > unsentKept {
				unsent = unsent[len(unsent)-unsentKept:]
			}
			if err := writePrivate(config+".unsent", unsentCache{Server: c.Server, Agent: c.ID, Entries: unsent}); err != nil {
				errs = append(errs, fmt.Errorf("could not keep what this pass did until it is reported: %w", err))
			}
		}
		if !maps.Equal(installed, next) {
			if err := writePrivate(config+".installed", next); err != nil {
				errs = append(errs, err)
			}
		}
	} else {
		rep = skillInventory(c.Home, *d)
	}
	if err := postSkillReport(ctx, c, config, withUnsent(rep, earlier), version); err != nil {
		errs = append(errs, err)
	} else if len(unsent) > 0 {
		if err := os.Remove(config + ".unsent"); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if local.AllowApply {
		errs = append(errs, pruneBackups(c.Home, now))
	}
	return errors.Join(errs...)
}

// postSkillReport sends rep unless this process already sent the same one.
// It returns nil once the controller has rep.
func postSkillReport(ctx context.Context, c agentConfig, config string, rep store.SkillReport, version string) error {
	b, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	skillReportsSent.Lock()
	same := skillReportsSent.last[config] == string(b)
	skillReportsSent.Unlock()
	if same {
		return nil
	}
	if err := agentRequestContext(ctx, c, "/api/v2/sync/skills/report", json.RawMessage(b), nil, version); err != nil {
		return fmt.Errorf("skills report not sent; it is sent again next cycle: %w", err)
	}
	skillReportsSent.Lock()
	skillReportsSent.last[config] = string(b)
	skillReportsSent.Unlock()
	return nil
}
