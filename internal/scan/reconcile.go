package scan

import (
	"context"
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

// These are what the pass uses to touch disk; tests replace them to stand
// for a disk that does not keep files exactly, or a move that fails.
var (
	readBackDigest  = privateSkillDigest
	reconcileBackup = backupSkill
	reconcileMove   = moveEntry
)

// reconcileOrder visits ~/.agents/skills first, because Cursor and OpenCode
// link to the copy there, and Claude's own copies last.
var reconcileOrder = []int{0, 2, 3, 1}

// A skill whose install failed is tried again after installRetryFirst, then
// after twice as long each time, up to installRetryMost.
const (
	installRetryFirst = 5 * time.Second
	installRetryMost  = 5 * time.Minute
)

type installBackoff struct {
	digest, detail string
	failures       int
	next           time.Time
}

// installBackoffs remembers, per home and lower-case name, an install that
// failed, so a bundle that cannot be installed is not fetched every cycle.
// It lasts as long as the process; a restarted agent tries at once.
var installBackoffs = struct {
	sync.Mutex
	m map[string]installBackoff
}{m: map[string]installBackoff{}}

// reconcileSkills makes the managed folders under home match d and reports
// what it found and did, one entry per skill and folder. installed maps
// "<folder>/<lower-case name>" to the digest reet last installed there; the
// map returned replaces it.
//
// ~/.agents/skills is created when an Active skill must go there. The other
// folders are used only when they exist: Cursor and OpenCode get links to the
// ~/.agents/skills copy, Claude gets its own. A folder that is a link to
// another managed folder is handled once, under the first of them, and one
// that leads into Codex's skills folder is left alone. Anything removed or
// replaced is moved into a dated backup first. Names are matched ignoring
// case. A failure stops only that skill in that folder.
func reconcileSkills(ctx context.Context, home string, d store.DesiredState, installed map[string]string, fetch bundleFetcher, now time.Time) (store.SkillReport, map[string]string) {
	r := &reconciler{ctx: ctx, home: home, now: now, fetch: fetch, installed: installed,
		next: map[string]string{}, bundles: map[string]fetchedBundle{}, digests: map[string]copyDigest{}, shared: map[string]string{}}
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
	claimed := claimedNames(active, removals, refused)
	roots := managedRoots(home)
	for _, i := range reconcileOrder {
		if !roots[i].skip {
			r.folder(managedFolders[i].Dir, managedFolders[i].Agent, roots[i].real, active, removals, claimed)
		}
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

// claimedNames are the lower-case names the desired state speaks for, refused
// ones included, so a copy under one of them is never reported unknown.
func claimedNames(active []store.DesiredSkill, removals []store.DesiredRemoval, refused []store.SkillReportEntry) map[string]bool {
	claimed := map[string]bool{}
	for _, a := range active {
		claimed[strings.ToLower(a.Name)] = true
	}
	for _, rm := range removals {
		claimed[strings.ToLower(rm.Name)] = true
	}
	for _, e := range refused {
		claimed[strings.ToLower(e.Name)] = true
	}
	return claimed
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

type managedRoot struct {
	real string // where the folder really is; "" when it does not exist
	skip bool   // handled under another managed folder, or not at all
}

// managedRoots resolves the managed folders, which may be links, as a
// dotfiles setup makes them. One that is the same folder as an earlier
// managed folder, or lies inside another, or is or holds Codex's skills
// folder, is skipped.
func managedRoots(home string) []managedRoot {
	codex := filepath.Join(home, ".codex", "skills")
	if real, err := filepath.EvalSymlinks(codex); err == nil {
		codex = real
	}
	roots := make([]managedRoot, len(managedFolders))
	for i, f := range managedFolders {
		roots[i].real, _ = filepath.EvalSymlinks(filepath.Join(home, filepath.FromSlash(f.Dir)))
	}
	for i := range roots {
		real := roots[i].real
		if real == "" {
			continue
		}
		roots[i].skip = within(real, codex) || within(codex, real)
		for j, other := range roots {
			if j == i || other.real == "" {
				continue
			}
			if real == other.real && j < i || real != other.real && within(real, other.real) {
				roots[i].skip = true
			}
		}
	}
	return roots
}

// within says whether p is dir or lies inside it.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// copyDigest is a copy's digest over all its files, and over all but the files
// an operating system leaves in folders on its own, which no one put in the
// skill. Either matching reet's digest makes the copy reet's.
type copyDigest struct{ all, sans string }

func (c copyDigest) is(digest string) bool {
	return digest != "" && (c.all == digest || c.sans == digest)
}

// osFile says whether a skill file is one Finder or Windows Explorer leaves on
// its own: .DS_Store, Thumbs.db, or a ._ AppleDouble file.
func osFile(p string) bool {
	base := p[strings.LastIndex(p, "/")+1:]
	return strings.EqualFold(base, ".DS_Store") || strings.EqualFold(base, "Thumbs.db") || strings.HasPrefix(base, "._")
}

func readCopyDigest(dir string) copyDigest {
	m, err := skillbundle.Scan(dir, skillbundle.Limits{})
	if err != nil || !m.Complete {
		return copyDigest{}
	}
	c := copyDigest{all: m.Digest, sans: m.Digest}
	kept := make([]skillbundle.File, 0, len(m.Files))
	for _, f := range m.Files {
		if !osFile(f.Path) {
			kept = append(kept, f)
		}
	}
	if len(kept) != len(m.Files) {
		c.sans = ""
		if stripped, err := skillbundle.FromMetadata(kept); err == nil {
			c.sans = stripped.Digest
		}
	}
	return c
}

type fetchedBundle struct {
	files []skillbundle.ContentFile
	err   error
}

type reconciler struct {
	ctx        context.Context
	home       string
	now        time.Time
	fetch      bundleFetcher
	installed  map[string]string
	next       map[string]string
	bundles    map[string]fetchedBundle // by lower-case name: one fetch a pass
	digests    map[string]copyDigest    // by path, for real folders read this pass
	shared     map[string]string        // lower-case name → reet's copy in ~/.agents/skills
	sharedReal string                   // where ~/.agents/skills really is, once it exists
	entries    []store.SkillReportEntry
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

// folder reconciles one managed folder. dir is where it is under home and
// real where it really is, "" when it does not exist yet.
func (r *reconciler) folder(rel, agent, real string, active []store.DesiredSkill, removals []store.DesiredRemoval, claimed map[string]bool) {
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
		r.activeSkill(dir, real, rel, label, linked, a, names[strings.ToLower(a.Name)])
	}
	if rel == sharedSkillsDir {
		r.sharedReal, _ = filepath.EvalSymlinks(dir)
	}
	for _, rm := range removals {
		for _, name := range names[strings.ToLower(rm.Name)] {
			dest, err := r.backup(dir, name)
			if err != nil {
				r.add(label, rm.Name, "failed", "", "could not back up "+name+": "+err.Error(), "")
				continue
			}
			r.add(label, rm.Name, "removed", "", "", r.tilde(dest))
		}
	}
	for _, lower := range slices.Sorted(maps.Keys(names)) {
		if claimed[lower] {
			continue
		}
		for _, name := range names[lower] {
			if digest, ok := r.skillAt(dir, real, name); ok {
				r.add(label, name, "unknown", digest, "", "")
			}
		}
	}
}

// movedCopy is a copy moved into a backup during this pass.
type movedCopy struct{ original, backup string }

// activeSkill keeps one copy of a in dir that is reet's: the present one when
// its digest matches, or a new one. Every other copy under the same name, in
// any case, is backed up first. If the new copy cannot then be put in place,
// the copies backed up for it are put back.
func (r *reconciler) activeSkill(dir, real, rel, label string, linked bool, a store.DesiredSkill, matches []string) {
	key := rel + "/" + strings.ToLower(a.Name)
	keep := ""
	for _, name := range matches {
		if r.copyAt(dir, real, name).is(a.Digest) && (keep == "" || name == a.Name) {
			keep = name
		}
	}
	installing := keep == "" && !linked
	staging, source := "", ""
	if keep == "" {
		var err error
		if linked {
			if source = r.shared[strings.ToLower(a.Name)]; source == "" {
				r.fail(label, rel, a, "not installed in ~/"+sharedSkillsDir+" to link to", "")
				return
			}
		} else if detail, waiting := r.waiting(a); waiting {
			r.fail(label, rel, a, detail, "")
			return
		} else if staging, err = r.stage(dir, a); err != nil {
			r.failInstall(label, rel, a, err.Error(), nil, installing)
			return
		}
	}
	if staging != "" {
		defer os.RemoveAll(staging)
	}
	prev, hadPrev := r.installed[key]
	state := "installed"
	var moved []movedCopy
	for _, name := range matches {
		if name == keep {
			continue
		}
		was := r.copyAt(dir, real, name)
		dest, err := r.backup(dir, name)
		if err != nil {
			detail := "could not back up " + name + ": " + err.Error()
			if keep != "" {
				// reet's copy is in place; only this other copy stays.
				r.settle(rel, a, keep)
				r.add(label, a.Name, "failed", "", detail, r.firstBackup(moved))
				return
			}
			r.failInstall(label, rel, a, detail, moved, installing)
			return
		}
		moved = append(moved, movedCopy{original: filepath.Join(dir, name), backup: dest})
		// A readable copy that is neither this revision nor the one reet
		// last put here was changed on this machine.
		if was.all != "" && !was.is(a.Digest) && !(hadPrev && was.is(prev)) {
			state = "edited"
		}
	}
	switch {
	case linked && keep == "":
		if err := r.link(real, source); err != nil {
			r.failInstall(label, rel, a, err.Error(), moved, false)
			return
		}
		keep = source
	case keep == "":
		dest := filepath.Join(dir, a.Name)
		if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
			r.failInstall(label, rel, a, "another copy appeared while installing", moved, true)
			return
		}
		if err := reconcileMove(staging, dest); err != nil {
			r.failInstall(label, rel, a, err.Error(), moved, true)
			return
		}
		r.digests[dest] = copyDigest{all: a.Digest, sans: a.Digest}
		keep = a.Name
		r.recovered(a)
	}
	r.settle(rel, a, keep)
	r.add(label, a.Name, state, a.Digest, "", r.firstBackup(moved))
}

// settle records that reet's copy of a is in place as keep.
func (r *reconciler) settle(rel string, a store.DesiredSkill, keep string) {
	if rel == sharedSkillsDir {
		r.shared[strings.ToLower(a.Name)] = keep
	}
	r.next[rel+"/"+strings.ToLower(a.Name)] = a.Digest
}

func (r *reconciler) firstBackup(moved []movedCopy) string {
	if len(moved) == 0 {
		return ""
	}
	return r.tilde(moved[0].backup)
}

// fail reports a failed skill and keeps what reet last installed there.
func (r *reconciler) fail(label, rel string, a store.DesiredSkill, detail, backup string) {
	key := rel + "/" + strings.ToLower(a.Name)
	if prev, ok := r.installed[key]; ok {
		r.next[key] = prev
	}
	r.add(label, a.Name, "failed", "", detail, backup)
}

// failInstall puts back the copies moved for an install that could not
// finish, reports it, and, for a copy of reet's files, waits before trying
// again.
func (r *reconciler) failInstall(label, rel string, a store.DesiredSkill, detail string, moved []movedCopy, backoff bool) {
	var left []movedCopy
	for i := len(moved) - 1; i >= 0; i-- {
		m := moved[i]
		err := errors.New(m.original + " is taken")
		if _, statErr := os.Lstat(m.original); errors.Is(statErr, fs.ErrNotExist) {
			err = moveEntry(m.backup, m.original)
		}
		if err != nil {
			detail += "; the copy that was there stays in " + r.tilde(m.backup) + ": " + err.Error()
			left = append([]movedCopy{m}, left...)
		}
		delete(r.digests, m.original)
	}
	if backoff {
		r.failedInstall(a, detail)
	}
	r.fail(label, rel, a, detail, r.firstBackup(left))
}

// waiting says whether a's install failed lately for this revision, and why.
func (r *reconciler) waiting(a store.DesiredSkill) (string, bool) {
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	b, ok := installBackoffs.m[r.home+"\x00"+strings.ToLower(a.Name)]
	if !ok || b.digest != a.Digest || r.now.After(b.next) {
		return "", false
	}
	return b.detail, true
}

func (r *reconciler) failedInstall(a store.DesiredSkill, detail string) {
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	key := r.home + "\x00" + strings.ToLower(a.Name)
	b := installBackoffs.m[key]
	if b.digest != a.Digest {
		b = installBackoff{digest: a.Digest}
	}
	b.failures++
	wait := installRetryMost
	if b.failures <= 6 {
		wait = min(installRetryFirst<<(b.failures-1), installRetryMost)
	}
	b.next, b.detail = r.now.Add(wait), detail
	installBackoffs.m[key] = b
}

func (r *reconciler) recovered(a store.DesiredSkill) {
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	delete(installBackoffs.m, r.home+"\x00"+strings.ToLower(a.Name))
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
	if got, err := readBackDigest(staging); err != nil || got != a.Digest {
		os.RemoveAll(staging)
		return "", errors.New("These files don't read back as reet's copy on this disk")
	}
	return staging, nil
}

// link makes a link in the folder that is really at real to reet's copy
// source in ~/.agents/skills, relative from where the folder really is, so
// it holds when the folder is itself a link.
func (r *reconciler) link(real, source string) error {
	if r.sharedReal == "" {
		return errors.New("~/" + sharedSkillsDir + " is not there to link to")
	}
	canonical := filepath.Join(r.sharedReal, source)
	if _, err := os.Stat(filepath.Join(canonical, "SKILL.md")); err != nil {
		return fmt.Errorf("installed %s not found in the shared skills folder: %w", source, err)
	}
	target, err := filepath.Rel(real, canonical)
	if err != nil {
		target = canonical
	}
	return os.Symlink(target, filepath.Join(real, source))
}

// backup moves dir/name into the dated backups beside dir and returns where
// it went.
func (r *reconciler) backup(dir, name string) (string, error) {
	p := filepath.Join(dir, name)
	dest, err := reconcileBackup(dir, name, r.now)
	delete(r.digests, p)
	if dest == "" {
		return "", err
	}
	// A move whose index could not be written still moved the copy; it is
	// listed by its folder name instead.
	return dest, nil
}

// tilde writes p as a ~/ path when it is under home.
func (r *reconciler) tilde(p string) string {
	rel, err := filepath.Rel(r.home, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return p
	}
	return "~/" + filepath.ToSlash(rel)
}

// copyAt reads the digests of the copy at dir/name, a folder really at real,
// or nothing when it cannot be read as a skill. A link is followed only into
// ~/.agents/skills, where reet keeps the copy Cursor, OpenCode and the skills
// CLI link to; nothing outside the managed folders is read.
func (r *reconciler) copyAt(dir, real, name string) copyDigest {
	p := filepath.Join(dir, name)
	st, err := os.Lstat(p)
	if err != nil {
		return copyDigest{}
	}
	if st.Mode()&os.ModeSymlink != 0 {
		if p = r.sharedTarget(real, p); p == "" {
			return copyDigest{}
		}
		if st, err = os.Lstat(p); err != nil {
			return copyDigest{}
		}
	}
	if !st.IsDir() {
		return copyDigest{}
	}
	if c, ok := r.digests[p]; ok {
		return c
	}
	c := readCopyDigest(p)
	r.digests[p] = c
	return c
}

// sharedTarget returns the copy in ~/.agents/skills the link at p leads to,
// when p is in another folder, or "".
func (r *reconciler) sharedTarget(real, p string) string {
	if r.sharedReal == "" || real == r.sharedReal {
		return ""
	}
	target, err := os.Readlink(p)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(real, target)
	}
	target = filepath.Clean(target)
	shared := filepath.Join(r.home, filepath.FromSlash(sharedSkillsDir))
	if parent := filepath.Dir(target); parent != shared && parent != r.sharedReal {
		if parent, err = filepath.EvalSymlinks(parent); err != nil || parent != r.sharedReal {
			return ""
		}
	}
	return filepath.Join(shared, filepath.Base(target))
}

// skillAt says whether dir/name is a skill to report, with its digest: a
// folder holding a SKILL.md, or a link, which is never followed out of the
// managed folders.
func (r *reconciler) skillAt(dir, real, name string) (string, bool) {
	p := filepath.Join(dir, name)
	st, err := os.Lstat(p)
	if err != nil {
		return "", false
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return r.copyAt(dir, real, name).all, true
	}
	if !st.IsDir() {
		return "", false
	}
	if md, err := os.Lstat(filepath.Join(p, "SKILL.md")); err != nil || !md.Mode().IsRegular() {
		return "", false
	}
	return r.copyAt(dir, real, name).all, true
}
