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
	reconcileMove   = moveEntryCopying
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
// folders are used only when they exist when the pass starts: Cursor and
// OpenCode get links to the ~/.agents/skills copy, Claude gets its own. A
// folder that is a link to another managed folder is handled once, under the
// first of them, and one that leads into Codex's skills folder is left alone
// and says so. Anything removed or replaced is moved into a dated backup
// first. Names are matched ignoring case. A failure stops only that skill in
// that folder.
func reconcileSkills(ctx context.Context, home string, d store.DesiredState, installed map[string]string, fetch bundleFetcher, now time.Time) (store.SkillReport, map[string]string) {
	return reconcileWith(ctx, home, d, installed, fetch, now, reconcileOptions{})
}

// reconcileOptions are what the agent adds to a pass.
type reconcileOptions struct {
	// unmanaged are folders, such as enrolled projects, that no managed
	// folder may lead into.
	unmanaged []string
	// news is told each edit or removal as soon as its backup is made, and
	// retract an edit whose replacement then failed and was put back.
	news, retract func(store.SkillReportEntry)
}

// reconcileWith is reconcileSkills with what the agent adds.
func reconcileWith(ctx context.Context, home string, d store.DesiredState, installed map[string]string, fetch bundleFetcher, now time.Time, opts reconcileOptions) (store.SkillReport, map[string]string) {
	r := &reconciler{ctx: ctx, home: home, now: now, fetch: fetch, installed: installed, news: opts.news, retract: opts.retract,
		next: map[string]string{}, bundles: map[string]fetchedBundle{}, digests: map[string]copyDigest{}, shared: map[string]string{}, allowed: map[string]bool{}}
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
	r.forgetWaits(active)
	claimed := claimedNames(active, removals, refused)
	roots := managedRoots(home, opts.unmanaged)
	r.clearStaging(roots)
	for _, root := range roots {
		if root.handled() {
			r.allowed[root.real] = true
		}
	}
	if roots[0].handled() {
		r.sharedReal = roots[0].real
	}
	for _, i := range reconcileOrder {
		f, root := newFolderRun(home, i, roots[i]), roots[i]
		switch {
		case root.why != "":
			r.leftAlone(f, root.why, active, removals)
		case root.handled() || f.rel == sharedSkillsDir && !root.skip:
			r.folder(f, active, removals, claimed)
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
	real   string // where the folder really is, or would be made
	exists bool
	skip   bool   // handled under an earlier managed folder, or left alone
	why    string // why it is left alone, as the report says it; "" when handled elsewhere
}

// handled says the pass works in this folder.
func (m managedRoot) handled() bool { return m.exists && !m.skip }

// managedRoots resolves the managed folders, which may be links, as a
// dotfiles setup makes them. One that is the same folder as an earlier
// managed folder is handled under that one. One that leads into another
// managed folder, Codex's skills folder, an enrolled project or the folder a
// custom CLAUDE_CONFIG_DIR or XDG_CONFIG_HOME names, or that holds one of
// those places, is left alone; so is one whose parent, where its backups and
// staging go, leads into such a place.
func managedRoots(home string, unmanaged []string) []managedRoot {
	type place struct{ path, name string }
	places := []place{{resolvePath(filepath.Join(home, ".codex", "skills")), "Codex's skills folder"}}
	for _, p := range unmanaged {
		if filepath.IsAbs(p) {
			places = append(places, place{resolvePath(p), "a project folder"})
		}
	}
	// As sync refused before: a custom folder is the person's own setup.
	for _, custom := range []struct{ env, usual string }{{"CLAUDE_CONFIG_DIR", ".claude"}, {"XDG_CONFIG_HOME", ".config"}} {
		dir := os.Getenv(custom.env)
		if filepath.IsAbs(dir) && resolvePath(dir) != resolvePath(filepath.Join(home, custom.usual)) {
			places = append(places, place{resolvePath(dir), "the folder " + custom.env + " names"})
		}
	}
	realHome := resolvePath(home)
	// into says whether p lies in pl. A place that is home, or holds it,
	// would cover every managed folder, so only what lies in it outside home
	// counts.
	into := func(p string, pl place) bool {
		return within(p, pl.path) && !(within(realHome, pl.path) && within(p, realHome))
	}
	roots := make([]managedRoot, len(managedFolders))
	parents := make([]string, len(managedFolders))
	for i, f := range managedFolders {
		p := filepath.Join(home, filepath.FromSlash(f.Dir))
		if real, err := filepath.EvalSymlinks(p); err == nil {
			roots[i].real, roots[i].exists = real, true
		} else {
			roots[i].real = resolvePath(p)
		}
		parents[i] = resolvePath(filepath.Dir(p))
	}
	for i := range roots {
		label, real := "~/"+managedFolders[i].Dir, roots[i].real
		for _, pl := range places {
			switch {
			case into(real, pl) || into(parents[i], pl):
				roots[i].why = label + " leads into " + pl.name + ", so reet leaves it alone"
			case within(pl.path, real):
				roots[i].why = label + " holds " + pl.name + ", so reet leaves it alone"
			default:
				continue
			}
			roots[i].skip = true
			break
		}
		for j, other := range roots {
			if roots[i].skip {
				break
			}
			switch {
			case j == i:
			case real == other.real:
				roots[i].skip = j < i
			case within(real, other.real):
				roots[i].skip, roots[i].why = true, label+" leads into ~/"+managedFolders[j].Dir+", so reet leaves it alone"
			}
		}
	}
	return roots
}

// resolvePath resolves the links in p as far as p exists, and keeps the rest
// as it is, so a folder not made yet compares with ones that exist.
func resolvePath(p string) string {
	p = filepath.Clean(p)
	var rest []string
	for {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(append([]string{real}, rest...)...)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(append([]string{p}, rest...)...)
		}
		rest = append([]string{filepath.Base(p)}, rest...)
		p = parent
	}
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
	ctx           context.Context
	home          string
	now           time.Time
	fetch         bundleFetcher
	installed     map[string]string
	next          map[string]string
	news, retract func(store.SkillReportEntry)
	bundles       map[string]fetchedBundle // by lower-case name: one fetch a pass
	digests       map[string]copyDigest    // by path, for real folders read this pass
	shared        map[string]string        // lower-case name → reet's copy in ~/.agents/skills
	sharedReal    string                   // where ~/.agents/skills really is, once it exists
	allowed       map[string]bool          // the real folders this pass may write in
	entries       []store.SkillReportEntry
}

// folderRun is one managed folder during a pass.
type folderRun struct {
	rel, label, dir string
	linked          bool   // Cursor and OpenCode link to ~/.agents/skills
	real            string // where it really is; "" until it exists
	planned         string // where it would be made
}

func newFolderRun(home string, i int, root managedRoot) *folderRun {
	rel := managedFolders[i].Dir
	f := &folderRun{rel: rel, label: "~/" + rel, dir: filepath.Join(home, filepath.FromSlash(rel)), planned: root.real,
		linked: managedFolders[i].Agent == "cursor" || managedFolders[i].Agent == "opencode"}
	if root.exists {
		f.real = root.real
	}
	return f
}

func (r *reconciler) add(folder, name, state, digest, detail, backup string) {
	r.entries = append(r.entries, store.SkillReportEntry{Folder: folder, Name: name, State: state, Digest: digest, Detail: detail, Backup: backup})
}

// tell passes on news a later pass could not see again.
func (r *reconciler) tell(e store.SkillReportEntry) {
	if r.news != nil {
		r.news(e)
	}
}

// clearStaging removes what an interrupted install left, only around the
// folders this pass works in: .reet-private-* staging beside where each is
// under home and beside where it really is, and a cross-device copy's hidden
// .<name>.reet-copy-* folder inside it.
func (r *reconciler) clearStaging(roots []managedRoot) {
	beside := map[string]bool{}
	inside := map[string]bool{}
	for i, f := range managedFolders {
		if roots[i].handled() {
			beside[filepath.Dir(filepath.Join(r.home, filepath.FromSlash(f.Dir)))] = true
			beside[filepath.Dir(roots[i].real)] = true
			inside[roots[i].real] = true
		}
	}
	clear := func(dir string, ours func(string) bool) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if ours(e.Name()) {
				// RemoveAll removes a link without following it.
				_ = os.RemoveAll(filepath.Join(dir, e.Name()))
			}
		}
	}
	for dir := range beside {
		clear(dir, func(name string) bool { return strings.HasPrefix(name, ".reet-private-") })
	}
	for dir := range inside {
		clear(dir, func(name string) bool { return strings.HasPrefix(name, ".") && strings.Contains(name, ".reet-copy-") })
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

// leftAlone reports the Active and removed names a folder left alone still
// holds as failed, saying why, so the controller does not count a removed
// name there as gone. Only the folder's names are read.
func (r *reconciler) leftAlone(f *folderRun, why string, active []store.DesiredSkill, removals []store.DesiredRemoval) {
	names, err := skillEntries(f.dir)
	if err != nil {
		r.failAll(f, active, removals, why+"; reet cannot read it to see what it holds: "+err.Error())
		return
	}
	for _, a := range active {
		if len(names[strings.ToLower(a.Name)]) > 0 {
			r.fail(f.label, f.rel, a, why, "")
		}
	}
	for _, rm := range removals {
		if len(names[strings.ToLower(rm.Name)]) > 0 {
			r.add(f.label, rm.Name, "failed", "", why, "")
		}
	}
}

// failAll reports every Active and removed name as failed in a folder the
// pass could not work in.
func (r *reconciler) failAll(f *folderRun, active []store.DesiredSkill, removals []store.DesiredRemoval, detail string) {
	for _, a := range active {
		r.fail(f.label, f.rel, a, detail, "")
	}
	for _, rm := range removals {
		r.add(f.label, rm.Name, "failed", "", detail, "")
	}
}

// folder reconciles one managed folder.
func (r *reconciler) folder(f *folderRun, active []store.DesiredSkill, removals []store.DesiredRemoval, claimed map[string]bool) {
	if f.real != "" {
		if err := r.writable(f); err != nil {
			r.failAll(f, active, removals, err.Error())
			return
		}
	}
	names, err := skillEntries(f.dir)
	if err != nil {
		r.failAll(f, active, removals, "cannot read "+f.label+": "+err.Error())
		return
	}
	for _, a := range active {
		r.activeSkill(f, a, names[strings.ToLower(a.Name)])
	}
	if f.rel == sharedSkillsDir && f.real != "" {
		r.sharedReal = f.real
	}
	for _, rm := range removals {
		for _, name := range names[strings.ToLower(rm.Name)] {
			if err := r.writable(f); err != nil {
				r.add(f.label, rm.Name, "failed", "", err.Error(), "")
				continue
			}
			dest, err := r.backup(f.dir, name)
			if err != nil {
				r.add(f.label, rm.Name, "failed", "", "could not back up "+name+": "+err.Error(), "")
				continue
			}
			r.add(f.label, rm.Name, "removed", "", "", r.tilde(dest))
			r.tell(r.entries[len(r.entries)-1])
		}
	}
	for _, lower := range slices.Sorted(maps.Keys(names)) {
		if claimed[lower] {
			continue
		}
		for _, name := range names[lower] {
			if digest, ok := r.skillAt(f, name); ok {
				r.add(f.label, name, "unknown", digest, "", "")
			}
		}
	}
}

// movedCopy is a copy moved into a backup during this pass.
type movedCopy struct{ original, backup string }

// errNoReadBack is the reason a copy of reet's files that does not read back
// as itself is not used.
var errNoReadBack = errors.New("These files don't read back as reet's copy on this disk")

// activeSkill keeps one copy of a in the folder that is reet's: the present
// one when its digest matches, or a new one. Every other copy under the same
// name, in any case, is backed up first. If the new copy cannot then be put
// in place, the copies backed up for it are put back.
func (r *reconciler) activeSkill(f *folderRun, a store.DesiredSkill, matches []string) {
	key := f.rel + "/" + strings.ToLower(a.Name)
	keep := ""
	for _, name := range matches {
		if r.copyAt(f, name).is(a.Digest) && (keep == "" || name == a.Name) {
			keep = name
		}
	}
	staging, source := "", ""
	if keep == "" {
		var err error
		if f.linked {
			if source = r.shared[strings.ToLower(a.Name)]; source == "" {
				r.fail(f.label, f.rel, a, "not installed in ~/"+sharedSkillsDir+" to link to", "")
				return
			}
		} else if detail, waiting := r.waiting(f, a); waiting {
			r.fail(f.label, f.rel, a, detail, "")
			return
		} else if staging, err = r.stage(f, a); err != nil {
			r.failInstall(f, a, err.Error(), nil, nil)
			return
		}
	}
	if staging != "" {
		defer os.RemoveAll(staging)
	}
	prev, hadPrev := r.installed[key]
	state, edit := "installed", (*store.SkillReportEntry)(nil)
	var moved []movedCopy
	for _, name := range matches {
		if name == keep {
			continue
		}
		was := r.copyAt(f, name)
		if err := r.writable(f); err != nil {
			r.failInstall(f, a, err.Error(), moved, edit)
			return
		}
		dest, err := r.backup(f.dir, name)
		if err != nil {
			detail := "could not back up " + name + ": " + err.Error()
			if keep != "" {
				// reet's copy is in place; only this other copy stays.
				r.settle(f, a, keep)
				r.add(f.label, a.Name, "failed", "", detail, r.firstBackup(moved))
				return
			}
			r.failInstall(f, a, detail, moved, edit)
			return
		}
		moved = append(moved, movedCopy{original: filepath.Join(f.dir, name), backup: dest})
		// A readable copy that is neither this revision nor the one reet
		// last put here was changed on this machine. The first such backup
		// is the one the report names, and it is passed on at once.
		if was.all != "" && !was.is(a.Digest) && !(hadPrev && was.is(prev)) {
			state = "edited"
			if edit == nil {
				edit = &store.SkillReportEntry{Folder: f.label, Name: a.Name, State: "edited", Digest: a.Digest, Backup: r.tilde(dest)}
				r.tell(*edit)
			}
		}
	}
	switch {
	case f.linked && keep == "":
		err := r.writable(f)
		if err == nil {
			err = r.link(f.real, source)
		}
		if err != nil {
			r.failInstall(f, a, err.Error(), moved, edit)
			return
		}
		keep = source
	case keep == "":
		dest := filepath.Join(f.dir, a.Name)
		if err := r.install(f, staging, dest, a.Digest); err != nil {
			r.failInstall(f, a, err.Error(), moved, edit)
			return
		}
		r.digests[dest] = copyDigest{all: a.Digest, sans: a.Digest}
		keep = a.Name
		r.recovered(f, a)
	}
	r.settle(f, a, keep)
	backup := r.firstBackup(moved)
	if edit != nil {
		backup = edit.Backup
	}
	r.add(f.label, a.Name, state, a.Digest, "", backup)
}

// install moves the staged copy to dest. Staging sits beside the folder, so
// this is normally a rename; when it had to copy, the copy that landed must
// read back as reet's too, or it is removed again.
func (r *reconciler) install(f *folderRun, staging, dest, digest string) error {
	if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
		return errors.New("another copy appeared while installing")
	}
	if err := r.writable(f); err != nil {
		return err
	}
	copied, err := reconcileMove(staging, dest)
	if err != nil || !copied {
		return err
	}
	if got, err := readBackDigest(dest); err != nil || got != digest {
		// Only reet's own copy, made just now, is removed.
		os.RemoveAll(dest)
		return errNoReadBack
	}
	return nil
}

// writable refuses to write into a folder that is not one this pass works
// in, or that has moved since the pass started: the last guard that reet
// writes only inside the managed folders.
func (r *reconciler) writable(f *folderRun) error {
	real, err := filepath.EvalSymlinks(f.dir)
	if err != nil || f.real == "" || real != f.real || !r.allowed[real] {
		return errors.New(f.label + " changed while reet was working on it; it is tried again next time")
	}
	return nil
}

// settle records that reet's copy of a is in place as keep.
func (r *reconciler) settle(f *folderRun, a store.DesiredSkill, keep string) {
	if f.rel == sharedSkillsDir {
		r.shared[strings.ToLower(a.Name)] = keep
	}
	r.next[f.rel+"/"+strings.ToLower(a.Name)] = a.Digest
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
// finish, takes back the edit it passed on, reports the failure and, for a
// copy of reet's files, waits before trying this folder again.
func (r *reconciler) failInstall(f *folderRun, a store.DesiredSkill, detail string, moved []movedCopy, edit *store.SkillReportEntry) {
	var left []movedCopy
	restored := map[string]bool{}
	// Nothing goes back through a folder that has moved.
	guard := r.writable(f)
	for i := len(moved) - 1; i >= 0; i-- {
		m := moved[i]
		err := guard
		if err == nil {
			err = errors.New(m.original + " is taken")
			if _, statErr := os.Lstat(m.original); errors.Is(statErr, fs.ErrNotExist) {
				err = moveEntry(m.backup, m.original)
			}
		}
		if err != nil {
			detail += "; the copy that was there stays in " + r.tilde(m.backup) + ": " + err.Error()
			left = append([]movedCopy{m}, left...)
		} else {
			restored[r.tilde(m.backup)] = true
		}
		delete(r.digests, m.original)
	}
	// An edit whose copy is back in place was never replaced. One whose copy
	// stays in its backup is still news: the person's edit lives only there.
	if edit != nil && restored[edit.Backup] && r.retract != nil {
		r.retract(*edit)
	}
	if !f.linked {
		r.failedInstall(f, a, detail)
	}
	r.fail(f.label, f.rel, a, detail, r.firstBackup(left))
}

func (r *reconciler) waitKey(f *folderRun, a store.DesiredSkill) string {
	return r.home + "\x00" + f.rel + "\x00" + strings.ToLower(a.Name)
}

// waiting says whether a's install in this folder failed lately for this
// revision, and why.
func (r *reconciler) waiting(f *folderRun, a store.DesiredSkill) (string, bool) {
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	b, ok := installBackoffs.m[r.waitKey(f, a)]
	if !ok || b.digest != a.Digest || r.now.After(b.next) {
		return "", false
	}
	return b.detail, true
}

func (r *reconciler) failedInstall(f *folderRun, a store.DesiredSkill, detail string) {
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	key := r.waitKey(f, a)
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

func (r *reconciler) recovered(f *folderRun, a store.DesiredSkill) {
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	delete(installBackoffs.m, r.waitKey(f, a))
}

// forgetWaits drops the waits of skills that are no longer Active.
func (r *reconciler) forgetWaits(active []store.DesiredSkill) {
	still := map[string]bool{}
	for _, a := range active {
		still[strings.ToLower(a.Name)] = true
	}
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	for key := range installBackoffs.m {
		home, rest, _ := strings.Cut(key, "\x00")
		_, name, _ := strings.Cut(rest, "\x00")
		if home == r.home && !still[name] {
			delete(installBackoffs.m, key)
		}
	}
}

// stage fetches and checks a's files, once a pass, and writes them into a
// staging folder beside where the folder really is, so the install is a
// rename. ~/.agents/skills is made first when it is missing, only where the
// pass found it would be. The staged copy must read back with a's digest.
func (r *reconciler) stage(f *folderRun, a store.DesiredSkill) (string, error) {
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
	if f.real == "" {
		moved := errors.New(f.label + " would not be made where reet expected; it is tried again next time")
		if resolvePath(f.dir) != f.planned {
			return "", moved
		}
		if err := os.MkdirAll(f.dir, 0755); err != nil {
			return "", err
		}
		real, err := filepath.EvalSymlinks(f.dir)
		if err != nil || real != f.planned {
			return "", moved
		}
		f.real, r.allowed[real] = real, true
		if f.rel == sharedSkillsDir {
			r.sharedReal = real
		}
	}
	staging, err := stageSkillFiles(filepath.Dir(f.real), b.files)
	if err != nil && filepath.Dir(f.dir) != filepath.Dir(f.real) {
		// The real folder may be a mount root, or sit in a folder this user
		// cannot write in. Beside the folder under home works then, and the
		// install reads back what lands if it had to copy.
		staging, err = stageSkillFiles(filepath.Dir(f.dir), b.files)
	}
	if err != nil {
		return "", err
	}
	// On a disk that does not keep files exactly (fixed modes on SMB, exFAT
	// or some FUSE mounts), reet's copy would read as changed on every pass
	// and be replaced again each time. Nothing present is touched then.
	if got, err := readBackDigest(staging); err != nil || got != a.Digest {
		os.RemoveAll(staging)
		return "", errNoReadBack
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
		return err
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

// copyAt reads the digests of the copy at name in the folder, or nothing when
// it cannot be read as a skill. A link is followed only into
// ~/.agents/skills, where reet keeps the copy Cursor, OpenCode and the skills
// CLI link to; nothing outside the managed folders is read.
func (r *reconciler) copyAt(f *folderRun, name string) copyDigest {
	p := filepath.Join(f.dir, name)
	st, err := os.Lstat(p)
	if err != nil {
		return copyDigest{}
	}
	if st.Mode()&os.ModeSymlink != 0 {
		if p = r.sharedTarget(f.real, p); p == "" {
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
// when p is in another folder, really at real, or "".
func (r *reconciler) sharedTarget(real, p string) string {
	if r.sharedReal == "" || real == "" || real == r.sharedReal {
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

// skillAt says whether name in the folder is a skill to report, with its
// digest: a folder holding a SKILL.md, or a link, which is never followed out
// of the managed folders.
func (r *reconciler) skillAt(f *folderRun, name string) (string, bool) {
	p := filepath.Join(f.dir, name)
	st, err := os.Lstat(p)
	if err != nil {
		return "", false
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return r.copyAt(f, name).all, true
	}
	if !st.IsDir() {
		return "", false
	}
	if md, err := os.Lstat(filepath.Join(p, "SKILL.md")); err != nil || !md.Mode().IsRegular() {
		return "", false
	}
	return r.copyAt(f, name).all, true
}
