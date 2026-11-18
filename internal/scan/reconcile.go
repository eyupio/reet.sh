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

func (r *reconciler) add(folder, name, state, digest, detail, backup string) {
	r.entries = append(r.entries, store.SkillReportEntry{Folder: folder, Name: name, State: state, Digest: digest, Detail: detail, Backup: backup})
}

// tell passes on news a later pass could not see again.
func (r *reconciler) tell(e store.SkillReportEntry) {
	if r.news != nil {
		r.news(e)
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
	var moved []movedCopy
	var edits []store.SkillReportEntry
	for _, name := range matches {
		if name == keep {
			continue
		}
		was := r.copyAt(f, name)
		if err := r.writable(f); err != nil {
			r.failInstall(f, a, err.Error(), moved, edits)
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
			r.failInstall(f, a, detail, moved, edits)
			return
		}
		moved = append(moved, movedCopy{original: filepath.Join(f.dir, name), backup: dest})
		// A copy that is neither this revision nor the one reet last put
		// here was changed on this machine, or is one reet cannot read to
		// tell. Its backup may hold the only copy of someone's work, so each
		// one is passed on at once, under the name it had here.
		if !was.is(a.Digest) && !(hadPrev && was.is(prev)) {
			e := store.SkillReportEntry{Folder: f.label, Name: name, State: "edited", Digest: a.Digest, Backup: r.tilde(dest)}
			if was.all == "" {
				e.Detail = "reet could not read this copy"
			}
			edits = append(edits, e)
			r.tell(e)
		}
	}
	switch {
	case f.linked && keep == "":
		err := r.writable(f)
		if err == nil {
			err = r.link(f.real, source)
		}
		if err != nil {
			r.failInstall(f, a, err.Error(), moved, edits)
			return
		}
		keep = source
	case keep == "":
		dest := filepath.Join(f.dir, a.Name)
		if err := r.install(f, staging, dest, a.Digest); err != nil {
			r.failInstall(f, a, err.Error(), moved, edits)
			return
		}
		r.digests[dest] = copyDigest{all: a.Digest, sans: a.Digest}
		keep = a.Name
		r.recovered(f, a)
	}
	r.settle(f, a, keep)
	// The skill's own entry names the first copy that was not reet's.
	state, detail, backup := "installed", "", r.firstBackup(moved)
	if len(edits) > 0 {
		state, detail, backup = "edited", edits[0].Detail, edits[0].Backup
	}
	r.add(f.label, a.Name, state, a.Digest, detail, backup)
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
// finish, takes back the edits it passed on whose copies went back, reports
// the failure and, for a copy of reet's files, waits before trying this
// folder again.
func (r *reconciler) failInstall(f *folderRun, a store.DesiredSkill, detail string, moved []movedCopy, edits []store.SkillReportEntry) {
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
	for _, e := range edits {
		if restored[e.Backup] && r.retract != nil {
			r.retract(e)
		}
	}
	if !f.linked {
		r.failedInstall(f, a, detail)
	}
	r.fail(f.label, f.rel, a, detail, r.firstBackup(left))
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
		real, err := resolveLinks(f.dir)
		if err != nil || real != f.planned {
			return "", moved
		}
		f.real, r.allowed[real] = real, true
		if f.rel == sharedSkillsDir {
			r.sharedReal = real
		}
	}
	staging, err := r.stageIn(f, filepath.Dir(f.real), b.files)
	if err != nil && f.parent != filepath.Dir(f.real) {
		// The real folder may be a mount root, or sit in a folder this user
		// cannot write in. Beside the folder under home works then, and the
		// install reads back what lands if it had to copy. It is staged in
		// the parent by the resolved path stageIn checks, so a parent pointed
		// elsewhere after the check does not get it.
		staging, err = r.stageIn(f, f.parent, b.files)
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

// stageIn writes files into a new staging folder in dir, beside the folder,
// only while the folder and its parent are still where the pass found them:
// the fetch, or a first try elsewhere, may have given them time to move.
func (r *reconciler) stageIn(f *folderRun, dir string, files []skillbundle.ContentFile) (string, error) {
	if err := r.writable(f); err != nil {
		return "", err
	}
	return stageSkillFiles(dir, files)
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
