package scan

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

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
