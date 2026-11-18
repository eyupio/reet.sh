package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestReconcileRefreshUsesHostnameAndDoesNotTouchFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REET_ENV", "")
	mkSkill(t, filepath.Join(os.Getenv("HOME"), ".agents", "skills"), "alpha", true)
	hostname, _ := os.Hostname()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("X-Reet-Token") != "private-token" {
			t.Error("credential missing")
		}
		if r.URL.Path == "/api/v2/machines/merge" {
			var v struct{ From, To string }
			if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
				t.Error(err)
			}
			if v.From != hostname || v.To != "PROD_Snicket" {
				t.Errorf("identity: %+v", v)
			}
			w.Write([]byte(`{}`))
			return
		}
		var v struct {
			Prune   bool
			Items   []struct{ Name, Host string }
			Machine struct{ Host string }
		}
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			t.Error(err)
		}
		if !v.Prune || v.Machine.Host != "PROD_Snicket" {
			t.Errorf("inventory: %+v", v)
		}
		if len(v.Items) == 0 {
			t.Error("installed skill not reported")
		}
		w.Write([]byte(`{"imported":1,"errors":[]}`))
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := RunVersion([]string{"--server", srv.URL, "--token", "private-token", "--env", "PROD_Snicket", "--prune", "--reconcile"}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/api/v2/machines/merge,/api/import" {
		t.Fatalf("repair must precede report: %v", paths)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".agents", "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatal("refresh changed installed files")
	}
	if strings.Contains(out.String(), "private-token") {
		t.Fatal("token leaked")
	}
	paths = nil
	if err := RunVersion([]string{"--server", srv.URL, "--token", "private-token", "--env", "PROD_Snicket", "--prune", "--reconcile", "--dry-run"}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatal("dry run changed controller state")
	}
	if err := RunVersion([]string{"--server", srv.URL, "--reconcile"}, &out, "test"); err == nil {
		t.Fatal("repair accepted an implicit preferred name")
	}
	if len(paths) != 0 {
		t.Fatal("missing explicit name still sent requests")
	}
}

func TestReconcileConflictStopsBeforeInventoryImport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"conflicting references"}`))
	}))
	defer srv.Close()
	err := RunVersion([]string{"--server", srv.URL, "--env", "preferred", "--reconcile", "--prune"}, &bytes.Buffer{}, "test")
	if err == nil || !strings.Contains(err.Error(), "conflicting references") || requests != 1 {
		t.Fatalf("conflict not stopped: %v %d", err, requests)
	}
}

func TestSyncAcceptsMergedHostAndTargetAliases(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".claude", "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	posted := false
	target := store.SyncTarget{Target: store.Environment{ID: "new-target", Host: "preferred", Agent: "claude", Name: "preferred · Claude Code"}, HostAliases: []string{"old-host"}, TargetAliases: []string{"old-target"}, Desired: []store.SyncSkill{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"targets": []store.SyncTarget{target}, "canWrite": true})
			return
		}
		var v struct{ Machine store.MachineInfo }
		json.NewDecoder(r.Body).Decode(&v)
		if v.Machine.Host != "preferred" {
			t.Error("report did not use preferred name")
		}
		posted = true
		w.Write([]byte(`{"imported":0,"errors":[]}`))
	}))
	defer srv.Close()
	if err := runSync([]string{"--server", srv.URL, "--env", "old-host", "--target", "old-target"}, strings.NewReader(""), &bytes.Buffer{}, func([]string, string) error { t.Fatal("must not install/remove skills"); return nil }, "test"); err != nil {
		t.Fatal(err)
	}
	if !posted {
		t.Fatal("inventory was not refreshed")
	}
	if err := runSync([]string{"--server", srv.URL, "--env", "unrelated-host", "--target", "old-target"}, strings.NewReader(""), &bytes.Buffer{}, func([]string, string) error { t.Fatal("must not execute"); return nil }, "test"); err == nil {
		t.Fatal("unrelated host accepted")
	}
}

var reconcileNow = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// stubBundles is a bundleFetcher that counts its calls.
type stubBundles struct {
	mu      sync.Mutex
	files   map[string][]skillbundle.ContentFile
	fail    map[string]error
	calls   int
	onFetch func() // runs during each fetch, standing for what happens meanwhile
}

func (s *stubBundles) fetch(_ context.Context, d store.DesiredSkill) ([]skillbundle.ContentFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.onFetch != nil {
		s.onFetch()
	}
	if err := s.fail[d.Name]; err != nil {
		return nil, err
	}
	files, ok := s.files[d.Name]
	if !ok {
		return nil, errors.New("no such bundle")
	}
	return files, nil
}

// skillBundle makes an Active skill and the files its digest names.
func skillBundle(t *testing.T, name, body string) (store.DesiredSkill, []skillbundle.ContentFile) {
	t.Helper()
	files := []skillbundle.ContentFile{
		{Path: "SKILL.md", Data: []byte("---\nname: " + name + "\ndescription: " + body + "\n---\n" + body + "\n")},
		{Path: "scripts/run.sh", Data: []byte("echo " + body + "\n"), Executable: true},
	}
	m, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	return store.DesiredSkill{Name: name, Item: "item-" + name, Revision: strings.Repeat("a", 32), Bundle: strings.Repeat("b", 32), Digest: m.Digest}, files
}

func digestOfFiles(t *testing.T, files []skillbundle.ContentFile) string {
	t.Helper()
	m, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	return m.Digest
}

func placeFiles(t *testing.T, dir string, files []skillbundle.ContentFile) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0644)
		if f.Executable {
			mode = 0755
		}
		if err := os.WriteFile(p, f.Data, mode); err != nil {
			t.Fatal(err)
		}
	}
}

func mkdirs(t *testing.T, home string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(d)), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func folderDigest(t *testing.T, dir string) string {
	t.Helper()
	d, err := privateSkillDigest(dir)
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	return d
}

// homeTree records every entry under root, with its kind, content or link
// target, mode and modification time, so a test can tell whether anything
// changed.
func homeTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && p == root {
			return filepath.SkipDir
		}
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		desc := fmt.Sprintf("%v %d", info.Mode(), info.ModTime().UnixNano())
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			desc += " -> " + target
		case info.Mode().IsRegular():
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			desc += " " + string(b)
		}
		out[p] = desc
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// withoutFolderTimes drops folders' modification times from a homeTree, for
// a pass that may stage files and remove them again.
func withoutFolderTimes(m map[string]string) map[string]string {
	out := map[string]string{}
	for p, v := range m {
		if strings.HasPrefix(v, "d") {
			v = strings.Fields(v)[0]
		}
		out[p] = v
	}
	return out
}

func sameTree(t *testing.T, what string, before, after map[string]string) {
	t.Helper()
	for p, v := range after {
		if before[p] != v {
			t.Errorf("%s: %s changed or appeared: %q", what, p, v)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			t.Errorf("%s: %s disappeared", what, p)
		}
	}
}

func entryOf(t *testing.T, rep store.SkillReport, folder, name string) store.SkillReportEntry {
	t.Helper()
	var found []store.SkillReportEntry
	for _, e := range rep.Entries {
		if e.Folder == folder && e.Name == name {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one entry for %s in %s, got %+v in %+v", name, folder, found, rep.Entries)
	}
	return found[0]
}

func stagingLeft(t *testing.T, home string) []string {
	t.Helper()
	var left []string
	parents := map[string]bool{}
	for _, f := range managedFolders {
		dir := filepath.Join(home, filepath.FromSlash(f.Dir))
		parents[filepath.Dir(dir)] = true
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			parents[filepath.Dir(real)] = true
		}
	}
	for parent := range parents {
		entries, _ := os.ReadDir(parent)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".reet-private-") {
				left = append(left, filepath.Join(parent, e.Name()))
			}
		}
	}
	return left
}

func TestReconcileInstallsMissing(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".cursor/skills", ".claude/skills")
	a, aFiles := skillBundle(t, "alpha", "first")
	b, bFiles := skillBundle(t, "beta", "second")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles, "beta": bFiles}}
	rep, installed := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a, b}}, map[string]string{}, stub.fetch, reconcileNow)
	if stub.calls != 2 {
		t.Fatalf("fetched %d times, want once per skill", stub.calls)
	}
	if rep.Version != "v1" || rep.Blocked != "" || len(rep.Entries) != 6 {
		t.Fatalf("report %+v", rep)
	}
	for _, s := range []store.DesiredSkill{a, b} {
		if got := folderDigest(t, filepath.Join(home, ".agents", "skills", s.Name)); got != s.Digest {
			t.Fatalf("%s in .agents/skills has digest %s", s.Name, got)
		}
		target, err := os.Readlink(filepath.Join(home, ".cursor", "skills", s.Name))
		if err != nil || target != filepath.Join("..", "..", ".agents", "skills", s.Name) {
			t.Fatalf("cursor link for %s: %q %v", s.Name, target, err)
		}
		if st, err := os.Lstat(filepath.Join(home, ".claude", "skills", s.Name)); err != nil || !st.IsDir() {
			t.Fatalf("claude copy of %s is not a folder: %v %v", s.Name, st, err)
		}
		if got := folderDigest(t, filepath.Join(home, ".claude", "skills", s.Name)); got != s.Digest {
			t.Fatalf("%s in .claude/skills has digest %s", s.Name, got)
		}
		for _, folder := range []string{".agents/skills", ".cursor/skills", ".claude/skills"} {
			e := entryOf(t, rep, "~/"+folder, s.Name)
			if e.State != "installed" || e.Digest != s.Digest || e.Backup != "" || e.Detail != "" {
				t.Fatalf("entry %+v", e)
			}
			if installed[folder+"/"+s.Name] != s.Digest {
				t.Fatalf("installed %v", installed)
			}
		}
	}
	if len(installed) != 6 {
		t.Fatalf("installed %v", installed)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
		t.Fatal("OpenCode's folder was created although it did not exist")
	}
	if left := stagingLeft(t, home); len(left) != 0 {
		t.Fatalf("staging left behind: %v", left)
	}
}

func TestReconcileRefusesBadDigest(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".cursor/skills")
	a, _ := skillBundle(t, "alpha", "first")
	_, older := skillBundle(t, "alpha", "older")
	_, tampered := skillBundle(t, "alpha", "tampered")
	placeFiles(t, filepath.Join(home, ".claude", "skills", "alpha"), older)
	installed := map[string]string{".claude/skills/alpha": digestOfFiles(t, older)}
	before := homeTree(t, home)
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": tampered}}
	rep, next := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, installed, stub.fetch, reconcileNow)
	sameTree(t, "home", before, homeTree(t, home))
	if stub.calls != 1 {
		t.Fatalf("fetched %d times", stub.calls)
	}
	for _, folder := range []string{"~/.agents/skills", "~/.claude/skills", "~/.cursor/skills"} {
		e := entryOf(t, rep, folder, "alpha")
		if e.State != "failed" || e.Detail == "" || e.Backup != "" {
			t.Fatalf("entry %+v", e)
		}
	}
	if e := entryOf(t, rep, "~/.agents/skills", "alpha"); !strings.Contains(e.Detail, "checksum") {
		t.Fatalf("detail %q", e.Detail)
	}
	if !maps.Equal(next, installed) {
		t.Fatalf("a failed install changed what reet last installed: %v", next)
	}
}

func TestReconcileReplacesEditedCopy(t *testing.T) {
	home := t.TempDir()
	a, aFiles := skillBundle(t, "alpha", "first")
	b, bFiles := skillBundle(t, "beta", "new")
	_, bOld := skillBundle(t, "beta", "old")
	shared := filepath.Join(home, ".agents", "skills")
	placeFiles(t, filepath.Join(shared, "alpha"), aFiles)
	if err := os.WriteFile(filepath.Join(shared, "alpha", "SKILL.md"), []byte("---\nname: alpha\n---\nmy own edit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	placeFiles(t, filepath.Join(shared, "beta"), bOld)
	installed := map[string]string{".agents/skills/alpha": a.Digest, ".agents/skills/beta": digestOfFiles(t, bOld)}
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles, "beta": bFiles}}
	rep, next := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v2", Active: []store.DesiredSkill{a, b}}, installed, stub.fetch, reconcileNow)

	edited := entryOf(t, rep, "~/.agents/skills", "alpha")
	if edited.State != "edited" || edited.Digest != a.Digest || edited.Backup != "~/.agents/.reet-backups/2026-11-18/.agents-alpha" {
		t.Fatalf("edited entry %+v", edited)
	}
	kept, err := os.ReadFile(filepath.Join(home, ".agents", ".reet-backups", "2026-11-18", ".agents-alpha", "SKILL.md"))
	if err != nil || !strings.Contains(string(kept), "my own edit") {
		t.Fatalf("the edit was not backed up: %q %v", kept, err)
	}
	if got := folderDigest(t, filepath.Join(shared, "alpha")); got != a.Digest {
		t.Fatal("reet's copy of alpha is not in place")
	}
	// An older revision reet installed is replaced too, but it is no edit.
	update := entryOf(t, rep, "~/.agents/skills", "beta")
	if update.State != "installed" || update.Digest != b.Digest || update.Backup != "~/.agents/.reet-backups/2026-11-18/.agents-beta" {
		t.Fatalf("update entry %+v", update)
	}
	if got := folderDigest(t, filepath.Join(shared, "beta")); got != b.Digest {
		t.Fatal("reet's new revision of beta is not in place")
	}
	if next[".agents/skills/alpha"] != a.Digest || next[".agents/skills/beta"] != b.Digest || len(next) != 2 {
		t.Fatalf("installed %v", next)
	}
}

func TestReconcileRemovesWhatReetNeverInstalled(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".agents", "skills", "plain"))
	outside := filepath.Join(home, "dev", "linked")
	writeSkill(t, outside)
	symlink(t, outside, filepath.Join(home, ".claude", "skills", "linked"))
	_, files := skillBundle(t, "tweaked", "reet's")
	placeFiles(t, filepath.Join(home, ".claude", "skills", "tweaked"), files)
	if err := os.WriteFile(filepath.Join(home, ".claude", "skills", "tweaked", "SKILL.md"), []byte("edited here"), 0644); err != nil {
		t.Fatal(err)
	}
	installed := map[string]string{".claude/skills/tweaked": digestOfFiles(t, files)}
	devBefore := homeTree(t, filepath.Join(home, "dev"))
	stub := &stubBundles{}
	d := store.DesiredState{Version: "v3", Remove: []store.DesiredRemoval{{Name: "plain", Reason: "archived"}, {Name: "linked", Reason: "archived"}, {Name: "tweaked", Reason: "deleted"}}}
	rep, next := reconcileSkills(context.Background(), home, d, installed, stub.fetch, reconcileNow)

	for _, c := range []struct{ folder, name, backup string }{
		{".agents/skills", "plain", "~/.agents/.reet-backups/2026-11-18/.agents-plain"},
		{".claude/skills", "linked", "~/.claude/.reet-backups/2026-11-18/.claude-linked"},
		{".claude/skills", "tweaked", "~/.claude/.reet-backups/2026-11-18/.claude-tweaked"},
	} {
		e := entryOf(t, rep, "~/"+c.folder, c.name)
		if e.State != "removed" || e.Backup != c.backup {
			t.Fatalf("entry %+v", e)
		}
		if _, err := os.Lstat(filepath.Join(home, c.folder, c.name)); !os.IsNotExist(err) {
			t.Fatalf("%s is still in %s", c.name, c.folder)
		}
		if _, err := os.Lstat(filepath.Join(home, strings.TrimPrefix(c.backup, "~/"))); err != nil {
			t.Fatalf("no backup of %s: %v", c.name, err)
		}
	}
	link := filepath.Join(home, ".claude", ".reet-backups", "2026-11-18", ".claude-linked")
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link was not moved as a link: %v %v", st, err)
	}
	if target, _ := os.Readlink(link); target != outside {
		t.Fatalf("link now points at %q", target)
	}
	sameTree(t, "link target", devBefore, homeTree(t, filepath.Join(home, "dev")))
	if len(rep.Entries) != 3 || len(next) != 0 || stub.calls != 0 {
		t.Fatalf("entries %+v, installed %v, fetches %d", rep.Entries, next, stub.calls)
	}
}

func TestReconcileLeavesUnknownAlone(t *testing.T) {
	home := t.TempDir()
	cq := filepath.Join(home, ".agents", "skills", "cq")
	writeSkill(t, cq)
	mkdirs(t, home, ".agents/skills/notes-without-skill-md", ".cursor/skills")
	symlink(t, filepath.Join("..", "..", ".agents", "skills", "cq"), filepath.Join(home, ".cursor", "skills", "cq"))
	if err := os.WriteFile(filepath.Join(home, ".agents", "skills", ".DS_Store"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	before := homeTree(t, home)
	stub := &stubBundles{}
	rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Remove: []store.DesiredRemoval{{Name: "other", Reason: "archived"}}}, map[string]string{}, stub.fetch, reconcileNow)
	sameTree(t, "home", before, homeTree(t, home))
	want := folderDigest(t, cq)
	for _, folder := range []string{"~/.agents/skills", "~/.cursor/skills"} {
		if e := entryOf(t, rep, folder, "cq"); e.State != "unknown" || e.Digest != want || e.Backup != "" {
			t.Fatalf("entry %+v, want unknown with digest %s", e, want)
		}
	}
	if len(rep.Entries) != 2 {
		t.Fatalf("only skills are reported: %+v", rep.Entries)
	}
}

func TestReconcileRecoversFromInterruptedInstall(t *testing.T) {
	home := t.TempDir()
	a, aFiles := skillBundle(t, "alpha", "first")
	_, older := skillBundle(t, "alpha", "older")
	// An earlier pass staged reet's copy and backed up the old one, then was
	// killed before moving the new copy into place.
	placeFiles(t, filepath.Join(home, ".agents", ".reet-private-1234"), aFiles[:1])
	backup := filepath.Join(home, ".agents", ".reet-backups", "2026-11-18", ".agents-alpha")
	placeFiles(t, backup, older)
	mkdirs(t, home, ".agents/skills", ".claude/skills")
	placeFiles(t, filepath.Join(home, ".claude", ".reet-private-99"), aFiles)
	placeFiles(t, filepath.Join(home, ".claude", "skills", "alpha"), aFiles)
	installed := map[string]string{".agents/skills/alpha": digestOfFiles(t, older), ".claude/skills/alpha": a.Digest}
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	rep, next := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v2", Active: []store.DesiredSkill{a}}, installed, stub.fetch, reconcileNow)

	if left := stagingLeft(t, home); len(left) != 0 {
		t.Fatalf("staging left behind: %v", left)
	}
	if got := folderDigest(t, filepath.Join(home, ".agents", "skills", "alpha")); got != a.Digest {
		t.Fatal("alpha was not installed")
	}
	if e := entryOf(t, rep, "~/.agents/skills", "alpha"); e.State != "installed" || e.Digest != a.Digest || e.Backup != "" {
		t.Fatalf("entry %+v", e)
	}
	if got := folderDigest(t, backup); got != digestOfFiles(t, older) {
		t.Fatal("the earlier backup was disturbed")
	}
	if stub.calls != 1 || next[".agents/skills/alpha"] != a.Digest {
		t.Fatalf("fetches %d, installed %v", stub.calls, next)
	}
}

// caseSensitive says whether dir's file system keeps names differing only in
// case apart.
func caseSensitive(t *testing.T, dir string) bool {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "case-probe"), 0755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Join(dir, "case-probe"))
	_, err := os.Lstat(filepath.Join(dir, "CASE-PROBE"))
	return os.IsNotExist(err)
}

func TestReconcileMatchesNamesIgnoringCase(t *testing.T) {
	t.Run("a copy under another case is the same skill", func(t *testing.T) {
		home := t.TempDir()
		shared := filepath.Join(home, ".agents", "skills")
		seo, seoFiles := skillBundle(t, "seo-audit", "audit")
		ask, askFiles := skillBundle(t, "Askme", "ask")
		_, askOld := skillBundle(t, "Askme", "old")
		placeFiles(t, filepath.Join(shared, "SEO-Audit"), seoFiles)
		symlink(t, filepath.Join("..", "..", ".agents", "skills", "SEO-Audit"), filepath.Join(home, ".cursor", "skills", "SEO-Audit"))
		placeFiles(t, filepath.Join(shared, "askme"), askOld)
		writeSkill(t, filepath.Join(shared, "Old-Thing"))
		stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"seo-audit": seoFiles, "Askme": askFiles}}
		d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{ask, seo}, Remove: []store.DesiredRemoval{{Name: "old-thing", Reason: "archived"}}}
		rep, next := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow)

		if stub.calls != 1 {
			t.Fatalf("fetched %d times; SEO-Audit is already reet's copy", stub.calls)
		}
		var names []string
		entries, _ := os.ReadDir(shared)
		for _, e := range entries {
			names = append(names, e.Name())
		}
		if strings.Join(names, " ") != "Askme SEO-Audit" {
			t.Fatalf("skills folder holds %v", names)
		}
		for _, e := range rep.Entries {
			if e.State == "unknown" {
				t.Fatalf("a case variant was reported unknown: %+v", e)
			}
		}
		if e := entryOf(t, rep, "~/.agents/skills", "seo-audit"); e.State != "installed" || e.Digest != seo.Digest || e.Backup != "" {
			t.Fatalf("entry %+v", e)
		}
		if e := entryOf(t, rep, "~/.cursor/skills", "seo-audit"); e.State != "installed" || e.Backup != "" {
			t.Fatalf("cursor entry %+v", e)
		}
		if e := entryOf(t, rep, "~/.agents/skills", "Askme"); e.State != "edited" || e.Backup != "~/.agents/.reet-backups/2026-11-18/.agents-askme" {
			t.Fatalf("Askme entry %+v", e)
		}
		if e := entryOf(t, rep, "~/.agents/skills", "old-thing"); e.State != "removed" || e.Backup != "~/.agents/.reet-backups/2026-11-18/.agents-Old-Thing" {
			t.Fatalf("removal entry %+v", e)
		}
		if next[".agents/skills/seo-audit"] != seo.Digest || next[".agents/skills/askme"] != ask.Digest {
			t.Fatalf("installed %v", next)
		}
	})
	t.Run("two copies differing only in case", func(t *testing.T) {
		home := t.TempDir()
		shared := filepath.Join(home, ".agents", "skills")
		mkdirs(t, home, ".agents/skills")
		if !caseSensitive(t, shared) {
			t.Skip("this file system cannot hold both names")
		}
		seo, seoFiles := skillBundle(t, "seo-audit", "audit")
		placeFiles(t, filepath.Join(shared, "seo-audit"), seoFiles)
		writeSkill(t, filepath.Join(shared, "SEO-Audit"))
		stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"seo-audit": seoFiles}}
		rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{seo}}, map[string]string{}, stub.fetch, reconcileNow)
		entries, _ := os.ReadDir(shared)
		if len(entries) != 1 || entries[0].Name() != "seo-audit" || stub.calls != 0 {
			t.Fatalf("folder %v, fetches %d", entries, stub.calls)
		}
		if len(rep.Entries) != 1 {
			t.Fatalf("entries %+v", rep.Entries)
		}
		if e := rep.Entries[0]; e.Name != "seo-audit" || e.State != "edited" || e.Backup != "~/.agents/.reet-backups/2026-11-18/.agents-SEO-Audit" {
			t.Fatalf("entry %+v", e)
		}
	})
}

func TestReconcileIsIdempotent(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".cursor/skills", ".claude/skills", ".config/opencode/skills")
	writeSkill(t, filepath.Join(home, ".claude", "skills", "cq"))
	a, aFiles := skillBundle(t, "alpha", "first")
	b, bFiles := skillBundle(t, "beta", "second")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles, "beta": bFiles}}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a, b}, Remove: []store.DesiredRemoval{{Name: "gone", Reason: "deleted"}}}
	first, installed := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow)
	fetched := stub.calls
	before := homeTree(t, home)
	second, again := reconcileSkills(context.Background(), home, d, installed, stub.fetch, reconcileNow.Add(5*time.Second))
	if stub.calls != fetched {
		t.Fatalf("the second pass fetched %d more times", stub.calls-fetched)
	}
	sameTree(t, "home", before, homeTree(t, home))
	if !reflect.DeepEqual(first, second) || !maps.Equal(installed, again) {
		t.Fatalf("second pass differs:\n%+v\n%+v\n%v\n%v", first, second, installed, again)
	}
}

func TestReconcileContinuesPastFailedDownload(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".cursor/skills")
	a, aFiles := skillBundle(t, "a", "first")
	b, bFiles := skillBundle(t, "b", "second")
	c, cFiles := skillBundle(t, "c", "third")
	stub := &stubBundles{
		files: map[string][]skillbundle.ContentFile{"a": aFiles, "b": bFiles, "c": cFiles},
		fail:  map[string]error{"b": errors.New("unexpected EOF after 4096 bytes")},
	}
	rep, next := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a, b, c}}, map[string]string{}, stub.fetch, reconcileNow)
	for _, s := range []store.DesiredSkill{a, c} {
		for _, folder := range []string{".agents/skills", ".cursor/skills"} {
			if e := entryOf(t, rep, "~/"+folder, s.Name); e.State != "installed" {
				t.Fatalf("entry %+v", e)
			}
			if _, err := os.Stat(filepath.Join(home, folder, s.Name, "SKILL.md")); err != nil {
				t.Fatalf("%s is not in %s: %v", s.Name, folder, err)
			}
		}
	}
	for _, folder := range []string{".agents/skills", ".cursor/skills"} {
		e := entryOf(t, rep, "~/"+folder, "b")
		if e.State != "failed" || e.Detail == "" {
			t.Fatalf("entry %+v", e)
		}
		if _, err := os.Lstat(filepath.Join(home, folder, "b")); !os.IsNotExist(err) {
			t.Fatalf("b appeared in %s", folder)
		}
	}
	if e := entryOf(t, rep, "~/.agents/skills", "b"); !strings.Contains(e.Detail, "unexpected EOF") {
		t.Fatalf("detail %q", e.Detail)
	}
	if left := stagingLeft(t, home); len(left) != 0 {
		t.Fatalf("staging left behind: %v", left)
	}
	if _, ok := next[".agents/skills/b"]; ok || len(next) != 4 || stub.calls != 3 {
		t.Fatalf("installed %v, fetches %d", next, stub.calls)
	}
}

func TestReconcileSkipsUnmanagedFolders(t *testing.T) {
	home := t.TempDir()
	unmanaged := []string{
		filepath.Join(home, ".codex"),
		filepath.Join(home, "work", "app"),
		filepath.Join(home, "custom-claude"),
		filepath.Join(home, "xdg"),
	}
	writeSkill(t, filepath.Join(home, ".codex", "skills", "legacy"))
	writeSkill(t, filepath.Join(home, ".codex", "skills", "alpha"))
	writeSkill(t, filepath.Join(home, "work", "app", ".claude", "skills", "legacy"))
	writeSkill(t, filepath.Join(home, "work", "app", ".agents", "skills", "legacy"))
	writeSkill(t, filepath.Join(home, "custom-claude", "skills", "legacy"))
	writeSkill(t, filepath.Join(home, "xdg", "opencode", "skills", "legacy"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "custom-claude"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	mkdirs(t, home, ".claude/skills")
	symlink(t, filepath.Join("..", "..", ".codex", "skills", "legacy"), filepath.Join(home, ".claude", "skills", "codex-link"))
	before := map[string]map[string]string{}
	for _, dir := range unmanaged {
		before[dir] = homeTree(t, dir)
	}
	a, aFiles := skillBundle(t, "alpha", "first")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}, Remove: []store.DesiredRemoval{{Name: "legacy", Reason: "archived"}}}
	rep, _ := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow)
	for _, dir := range unmanaged {
		sameTree(t, dir, before[dir], homeTree(t, dir))
	}
	for _, e := range rep.Entries {
		if e.Folder != "~/.agents/skills" && e.Folder != "~/.claude/skills" {
			t.Fatalf("entry outside the existing managed folders: %+v", e)
		}
		if e.Name == "legacy" {
			t.Fatalf("a skill outside the managed folders was reported: %+v", e)
		}
	}
	// The link is reported, but what it points at is never read.
	if e := entryOf(t, rep, "~/.claude/skills", "codex-link"); e.State != "unknown" || e.Digest != "" {
		t.Fatalf("link entry %+v", e)
	}
	for _, folder := range []string{"~/.agents/skills", "~/.claude/skills"} {
		if e := entryOf(t, rep, folder, "alpha"); e.State != "installed" {
			t.Fatalf("entry %+v", e)
		}
	}
	if len(rep.Entries) != 3 {
		t.Fatalf("entries %+v", rep.Entries)
	}
}

func TestReconcileRefusesUnsafeNames(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".agents/skills", ".claude/skills")
	writeSkill(t, filepath.Join(home, ".agents", "escape"))
	writeSkill(t, filepath.Join(home, ".agents", "skills", ".hidden"))
	bad := []string{"../x", "a/b", `a\b`, ".", "..", "", "nul\x00name", strings.Repeat("n", 256), ".hidden"}
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{}}
	var d store.DesiredState
	d.Version = "v1"
	for _, name := range bad {
		s, files := skillBundle(t, "x", "x")
		s.Name = name
		stub.files[name] = files
		d.Active = append(d.Active, s)
	}
	unchecked, files := skillBundle(t, "unchecked", "x")
	unchecked.Digest = ""
	stub.files["unchecked"] = files
	d.Active = append(d.Active, unchecked)
	for _, name := range []string{"../escape", "..", "/etc", ".hidden"} {
		d.Remove = append(d.Remove, store.DesiredRemoval{Name: name, Reason: "archived"})
	}
	before := homeTree(t, home)
	rep, next := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow)
	sameTree(t, "home", before, homeTree(t, home))
	if stub.calls != 0 || len(next) != 0 {
		t.Fatalf("fetches %d, installed %v", stub.calls, next)
	}
	if len(rep.Entries) != len(d.Active)+len(d.Remove) {
		t.Fatalf("want one refusal per name, got %+v", rep.Entries)
	}
	for _, e := range rep.Entries {
		if e.State != "failed" || e.Detail == "" || e.Backup != "" {
			t.Fatalf("entry %+v", e)
		}
	}
}

// On a disk that does not keep files exactly (fixed modes on SMB, exFAT or
// some FUSE mounts), reet's copy never reads back as itself. Installing it
// would replace it again on every poll, so nothing is replaced at all.
func TestReconcileRefusesACopyThatDoesNotReadBack(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".cursor/skills")
	a, aFiles := skillBundle(t, "alpha", "first")
	_, older := skillBundle(t, "alpha", "older")
	placeFiles(t, filepath.Join(home, ".agents", "skills", "alpha"), older)
	installed := map[string]string{".agents/skills/alpha": digestOfFiles(t, older)}
	was := readBackDigest
	t.Cleanup(func() { readBackDigest = was })
	readBacks := 0
	readBackDigest = func(string) (string, error) {
		readBacks++
		return strings.Repeat("0", 64), nil
	}
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}
	before := withoutFolderTimes(homeTree(t, home))
	for pass := 1; pass <= 2; pass++ {
		// The second pass comes after the wait, so it reads back again.
		at := reconcileNow.Add(time.Duration(pass-1) * time.Minute)
		rep, next := reconcileSkills(context.Background(), home, d, installed, stub.fetch, at)
		if readBacks != pass {
			t.Fatalf("pass %d: %d read-backs", pass, readBacks)
		}
		sameTree(t, "home", before, withoutFolderTimes(homeTree(t, home)))
		e := entryOf(t, rep, "~/.agents/skills", "alpha")
		if e.State != "failed" || e.Detail != "These files don't read back as reet's copy on this disk" || e.Backup != "" {
			t.Fatalf("pass %d: entry %+v", pass, e)
		}
		if !maps.Equal(next, installed) {
			t.Fatalf("pass %d: installed %v", pass, next)
		}
	}
}

func TestReconcileBacksOffAFailingInstall(t *testing.T) {
	home := t.TempDir()
	a, aFiles := skillBundle(t, "alpha", "first")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}, fail: map[string]error{"alpha": errors.New("controller replied 502 Bad Gateway")}}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}
	pass := func(at time.Duration, fetches int, state string) {
		t.Helper()
		rep, _ := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow.Add(at))
		e := entryOf(t, rep, "~/.agents/skills", "alpha")
		if e.State != state || state == "failed" && !strings.Contains(e.Detail, "502") {
			t.Fatalf("at %v: entry %+v, want %s", at, e, state)
		}
		if stub.calls != fetches {
			t.Fatalf("at %v: %d fetches, want %d", at, stub.calls, fetches)
		}
	}
	pass(0, 1, "failed")
	pass(5*time.Second, 1, "failed") // the next cycle waits, and still says why
	pass(6*time.Second, 2, "failed")
	pass(16*time.Second, 2, "failed") // the wait doubles
	pass(17*time.Second, 3, "failed")
	// A new revision is tried at once.
	b, bFiles := skillBundle(t, "alpha", "second")
	d.Active = []store.DesiredSkill{b}
	pass(18*time.Second, 4, "failed")
	stub.files["alpha"], stub.fail = bFiles, nil
	pass(19*time.Second, 4, "failed")
	pass(24*time.Second, 5, "installed")
}

func TestReconcileFetchesNamesThatAreNotSlugs(t *testing.T) {
	srv := newSkillController(t)
	home := t.TempDir()
	a, aFiles := skillBundle(t, "My Skill+ü", "spaced")
	srv.serve(t, a, aFiles)
	fetch := controllerBundles(agentConfig{Server: srv.URL, Token: "t"})
	rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, fetch, reconcileNow)
	if e := entryOf(t, rep, "~/.agents/skills", a.Name); e.State != "installed" {
		t.Fatalf("entry %+v", e)
	}
	if got := folderDigest(t, filepath.Join(home, ".agents", "skills", a.Name)); got != a.Digest {
		t.Fatal("the skill is not in a folder under its name")
	}
}

func TestReconcilePutsTheCopyBackWhenTheInstallFails(t *testing.T) {
	home := t.TempDir()
	a, aFiles := skillBundle(t, "alpha", "first")
	_, older := skillBundle(t, "alpha", "older")
	present := filepath.Join(home, ".agents", "skills", "alpha")
	placeFiles(t, present, older)
	installed := map[string]string{".agents/skills/alpha": digestOfFiles(t, older)}
	was := reconcileMove
	t.Cleanup(func() { reconcileMove = was })
	reconcileMove = func(src, dest string) (bool, error) {
		return false, &os.LinkError{Op: "rename", Old: src, New: dest, Err: syscall.EXDEV}
	}
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	rep, next := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, installed, stub.fetch, reconcileNow)
	e := entryOf(t, rep, "~/.agents/skills", "alpha")
	if e.State != "failed" || !strings.Contains(e.Detail, "cross-device") || e.Backup != "" {
		t.Fatalf("entry %+v", e)
	}
	if got := folderDigest(t, present); got != digestOfFiles(t, older) {
		t.Fatal("the copy that was there is not back in place")
	}
	if kept, _ := filepath.Glob(filepath.Join(home, ".agents", ".reet-backups", "*", ".agents-alpha*")); len(kept) != 0 {
		t.Fatalf("backups left: %v", kept)
	}
	if left := stagingLeft(t, home); len(left) != 0 {
		t.Fatalf("staging left behind: %v", left)
	}
	if !maps.Equal(next, installed) {
		t.Fatalf("installed %v", next)
	}
}

func TestReconcileHandlesSymlinkedFoldersOnce(t *testing.T) {
	t.Run("a folder linked to the shared folder", func(t *testing.T) {
		home := t.TempDir()
		mkdirs(t, home, ".agents/skills")
		symlink(t, filepath.Join("..", ".agents", "skills"), filepath.Join(home, ".cursor", "skills"))
		writeSkill(t, filepath.Join(home, ".agents", "skills", "cq"))
		a, aFiles := skillBundle(t, "alpha", "first")
		stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
		rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, stub.fetch, reconcileNow)
		if len(rep.Entries) != 2 || stub.calls != 1 {
			t.Fatalf("entries %+v, fetches %d", rep.Entries, stub.calls)
		}
		for _, e := range rep.Entries {
			if e.Folder != "~/.agents/skills" {
				t.Fatalf("the same folder was handled twice: %+v", e)
			}
		}
		entries, _ := os.ReadDir(filepath.Join(home, ".agents", "skills"))
		for _, e := range entries {
			if e.Type()&os.ModeSymlink != 0 {
				t.Fatalf("%s links to itself", e.Name())
			}
		}
	})
	t.Run("Claude's folder linked to Codex's", func(t *testing.T) {
		home := t.TempDir()
		writeSkill(t, filepath.Join(home, ".codex", "skills", "legacy"))
		writeSkill(t, filepath.Join(home, ".codex", "skills", "alpha"))
		writeSkill(t, filepath.Join(home, ".codex", "skills", "cq"))
		symlink(t, filepath.Join("..", ".codex", "skills"), filepath.Join(home, ".claude", "skills"))
		before := homeTree(t, filepath.Join(home, ".codex"))
		a, aFiles := skillBundle(t, "alpha", "first")
		stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
		d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}, Remove: []store.DesiredRemoval{{Name: "legacy", Reason: "archived"}}}
		rep, _ := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow)
		sameTree(t, ".codex", before, homeTree(t, filepath.Join(home, ".codex")))
		if e := entryOf(t, rep, "~/.agents/skills", "alpha"); e.State != "installed" {
			t.Fatalf("entry %+v", e)
		}
		// What Claude can still see there is reported, so the archived
		// skill is not taken for gone.
		for _, name := range []string{"alpha", "legacy"} {
			e := entryOf(t, rep, "~/.claude/skills", name)
			if e.State != "failed" || e.Detail != "~/.claude/skills leads into Codex's skills folder, so reet leaves it alone" {
				t.Fatalf("entry %+v", e)
			}
		}
		if len(rep.Entries) != 3 {
			t.Fatalf("entries %+v", rep.Entries)
		}
	})
	t.Run("a folder linked from elsewhere", func(t *testing.T) {
		home := t.TempDir()
		elsewhere := filepath.Join(home, "dotfiles", "editors", "cursor", "skills")
		mkdirs(t, home, "dotfiles/editors/cursor/skills")
		symlink(t, elsewhere, filepath.Join(home, ".cursor", "skills"))
		a, aFiles := skillBundle(t, "alpha", "first")
		stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
		d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}
		first, installed := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow)
		if e := entryOf(t, first, "~/.cursor/skills", "alpha"); e.State != "installed" {
			t.Fatalf("entry %+v", e)
		}
		if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "alpha", "SKILL.md")); err != nil {
			t.Fatalf("Cursor's link does not reach the shared copy: %v", err)
		}
		before := homeTree(t, home)
		second, _ := reconcileSkills(context.Background(), home, d, installed, stub.fetch, reconcileNow)
		sameTree(t, "home", before, homeTree(t, home))
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("second pass %+v", second)
		}
	})
}

func TestReconcileReportsARefusedNameOnlyAsFailed(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".agents", "skills", "alpha"))
	a, _ := skillBundle(t, "alpha", "first")
	a.Digest = ""
	rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, (&stubBundles{}).fetch, reconcileNow)
	if len(rep.Entries) != 1 || rep.Entries[0].State != "failed" {
		t.Fatalf("entries %+v", rep.Entries)
	}
}

func failingBackup(t *testing.T, name string) {
	t.Helper()
	was := reconcileBackup
	t.Cleanup(func() { reconcileBackup = was })
	reconcileBackup = func(folder, n string, now time.Time) (string, error) {
		if n == name {
			return "", errors.New("device or resource busy")
		}
		return backupSkill(folder, n, now)
	}
}

func TestReconcileLinksToTheCopyLeftWhenABackupFails(t *testing.T) {
	t.Run("a duplicate that cannot move", func(t *testing.T) {
		home := t.TempDir()
		shared := filepath.Join(home, ".agents", "skills")
		mkdirs(t, home, ".agents/skills", ".cursor/skills")
		if !caseSensitive(t, shared) {
			t.Skip("this file system cannot hold both names")
		}
		seo, seoFiles := skillBundle(t, "seo-audit", "audit")
		placeFiles(t, filepath.Join(shared, "seo-audit"), seoFiles)
		writeSkill(t, filepath.Join(shared, "SEO-Audit"))
		failingBackup(t, "SEO-Audit")
		stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"seo-audit": seoFiles}}
		rep, next := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{seo}}, map[string]string{}, stub.fetch, reconcileNow)
		if e := entryOf(t, rep, "~/.agents/skills", "seo-audit"); e.State != "failed" || !strings.Contains(e.Detail, "SEO-Audit") {
			t.Fatalf("entry %+v", e)
		}
		if e := entryOf(t, rep, "~/.cursor/skills", "seo-audit"); e.State != "installed" {
			t.Fatalf("cursor entry %+v", e)
		}
		if got := folderDigest(t, filepath.Join(shared, "seo-audit")); got != seo.Digest {
			t.Fatal("reet's copy moved")
		}
		if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "seo-audit", "SKILL.md")); err != nil {
			t.Fatalf("Cursor's link: %v", err)
		}
		if next[".agents/skills/seo-audit"] != seo.Digest || stub.calls != 0 {
			t.Fatalf("installed %v, fetches %d", next, stub.calls)
		}
	})
	t.Run("a removal that cannot move", func(t *testing.T) {
		home := t.TempDir()
		gone := filepath.Join(home, ".agents", "skills", "gone")
		writeSkill(t, gone)
		failingBackup(t, "gone")
		d := store.DesiredState{Version: "v1", Remove: []store.DesiredRemoval{{Name: "gone", Reason: "archived"}}}
		rep, _ := reconcileSkills(context.Background(), home, d, map[string]string{}, (&stubBundles{}).fetch, reconcileNow)
		if e := entryOf(t, rep, "~/.agents/skills", "gone"); e.State != "failed" || !strings.Contains(e.Detail, "busy") || e.Backup != "" {
			t.Fatalf("entry %+v", e)
		}
		if _, err := os.Stat(filepath.Join(gone, "SKILL.md")); err != nil {
			t.Fatal("the skill went although its backup failed")
		}
	})
}

func TestReconcileIgnoresOSFilesInACopy(t *testing.T) {
	for _, recorded := range []bool{true, false} {
		home := t.TempDir()
		mkdirs(t, home, ".cursor/skills")
		a, aFiles := skillBundle(t, "alpha", "first")
		copyDir := filepath.Join(home, ".agents", "skills", "alpha")
		placeFiles(t, copyDir, aFiles)
		placeFiles(t, copyDir, []skillbundle.ContentFile{{Path: ".DS_Store", Data: []byte("finder")}, {Path: "scripts/._run.sh", Data: []byte("fork")}, {Path: "Thumbs.db", Data: []byte("thumbs")}})
		symlink(t, filepath.Join("..", "..", ".agents", "skills", "alpha"), filepath.Join(home, ".cursor", "skills", "alpha"))
		installed := map[string]string{}
		if recorded {
			installed = map[string]string{".agents/skills/alpha": a.Digest, ".cursor/skills/alpha": a.Digest}
		}
		before := homeTree(t, home)
		stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
		rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, installed, stub.fetch, reconcileNow)
		sameTree(t, "home", before, homeTree(t, home))
		for _, folder := range []string{"~/.agents/skills", "~/.cursor/skills"} {
			if e := entryOf(t, rep, folder, "alpha"); e.State != "installed" || e.Backup != "" {
				t.Fatalf("recorded %v: entry %+v", recorded, e)
			}
		}
		if stub.calls != 0 {
			t.Fatalf("recorded %v: fetched %d times", recorded, stub.calls)
		}
	}
}

func TestReconcileKeepsInstallingClaudeWhenTheSharedFolderFails(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".claude/skills")
	a, aFiles := skillBundle(t, "alpha", "first")
	was := readBackDigest
	t.Cleanup(func() { readBackDigest = was })
	shared := string(filepath.Separator) + ".agents" + string(filepath.Separator)
	readBackDigest = func(p string) (string, error) {
		if strings.Contains(p, shared) {
			return strings.Repeat("0", 64), nil
		}
		return privateSkillDigest(p)
	}
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}
	claude := filepath.Join(home, ".claude", "skills", "alpha")
	for pass := 0; pass < 2; pass++ {
		// The second pass falls inside the shared folder's wait.
		rep, _ := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow.Add(time.Duration(pass)*time.Second))
		if e := entryOf(t, rep, "~/.agents/skills", "alpha"); e.State != "failed" || !strings.Contains(e.Detail, "read back") {
			t.Fatalf("pass %d: shared entry %+v", pass, e)
		}
		if e := entryOf(t, rep, "~/.claude/skills", "alpha"); e.State != "installed" || e.Detail != "" {
			t.Fatalf("pass %d: Claude's entry %+v", pass, e)
		}
		if got := folderDigest(t, claude); got != a.Digest {
			t.Fatalf("pass %d: Claude's copy is not reet's", pass)
		}
		if err := os.RemoveAll(claude); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReconcileLeavesAFolderMadeDuringThePassForTheNextPass(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	home := t.TempDir()
	a, aFiles := skillBundle(t, "alpha", "first")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}, onFetch: func() {
		if err := os.MkdirAll(filepath.Join(home, ".cursor", "skills"), 0755); err != nil {
			t.Error(err)
		}
	}}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}
	first, installed := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow)
	if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
		t.Fatalf("wrote %v in the working folder", entries)
	}
	if len(first.Entries) != 1 || first.Entries[0].Folder != "~/.agents/skills" {
		t.Fatalf("entries %+v", first.Entries)
	}
	second, _ := reconcileSkills(context.Background(), home, d, installed, stub.fetch, reconcileNow)
	if e := entryOf(t, second, "~/.cursor/skills", "alpha"); e.State != "installed" {
		t.Fatalf("entry %+v", e)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatalf("Cursor's link: %v", err)
	}
	if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
		t.Fatalf("wrote %v in the working folder", entries)
	}
}

// A managed folder on another device is copied into, not renamed into. The
// copy that lands must read back as reet's, or reet's copy would look edited
// and be replaced on every pass.
func TestReconcileReadsBackACopyMovedAcrossDevices(t *testing.T) {
	home := t.TempDir()
	a, aFiles := skillBundle(t, "alpha", "first")
	_, older := skillBundle(t, "alpha", "older")
	present := filepath.Join(home, ".agents", "skills", "alpha")
	placeFiles(t, present, older)
	installed := map[string]string{".agents/skills/alpha": digestOfFiles(t, older)}
	wasMove, wasRead := reconcileMove, readBackDigest
	t.Cleanup(func() { reconcileMove, readBackDigest = wasMove, wasRead })
	reconcileMove = func(src, dest string) (bool, error) {
		if err := copyEntry(src, dest); err != nil {
			return true, err
		}
		return true, os.RemoveAll(src)
	}
	readBackDigest = func(p string) (string, error) {
		if !strings.Contains(p, ".reet-private-") {
			return strings.Repeat("0", 64), nil
		}
		return privateSkillDigest(p)
	}
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}
	for pass := 0; pass < 2; pass++ {
		rep, next := reconcileSkills(context.Background(), home, d, installed, stub.fetch, reconcileNow.Add(time.Duration(pass)*time.Minute))
		e := entryOf(t, rep, "~/.agents/skills", "alpha")
		if e.State != "failed" || !strings.Contains(e.Detail, "read back") || e.Backup != "" {
			t.Fatalf("pass %d: entry %+v", pass, e)
		}
		if got := folderDigest(t, present); got != digestOfFiles(t, older) {
			t.Fatalf("pass %d: the copy that was there is not back", pass)
		}
		if kept, _ := filepath.Glob(filepath.Join(home, ".agents", ".reet-backups", "*", ".agents-alpha*")); len(kept) != 0 {
			t.Fatalf("pass %d: backups left: %v", pass, kept)
		}
		if left := stagingLeft(t, home); len(left) != 0 {
			t.Fatalf("pass %d: staging left: %v", pass, left)
		}
		if !maps.Equal(next, installed) {
			t.Fatalf("pass %d: installed %v", pass, next)
		}
	}
}

func backoffsFor(home string) int {
	installBackoffs.Lock()
	defer installBackoffs.Unlock()
	n := 0
	for key := range installBackoffs.m {
		if strings.HasPrefix(key, home+"\x00") {
			n++
		}
	}
	return n
}

func TestReconcileForgetsWaitsForSkillsNoLongerActive(t *testing.T) {
	home := t.TempDir()
	a, _ := skillBundle(t, "alpha", "first")
	stub := &stubBundles{fail: map[string]error{"alpha": errors.New("controller replied 502 Bad Gateway")}}
	reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, stub.fetch, reconcileNow)
	if n := backoffsFor(home); n == 0 {
		t.Fatal("a failed install was not remembered")
	}
	reconcileSkills(context.Background(), home, store.DesiredState{Version: "v2"}, map[string]string{}, stub.fetch, reconcileNow.Add(time.Second))
	if n := backoffsFor(home); n != 0 {
		t.Fatalf("%d waits kept for a skill no longer Active", n)
	}
}

func TestReconcileLeavesFoldersLeadingIntoProjectsOrCustomConfigAlone(t *testing.T) {
	for _, c := range []struct {
		name, folder, place, env string
		want                     string
	}{
		{"project", ".claude/skills", "work/app/.claude/skills", "", "~/.claude/skills leads into a project folder, so reet leaves it alone"},
		{"CLAUDE_CONFIG_DIR", ".claude/skills", "claude-work/skills", "CLAUDE_CONFIG_DIR=claude-work", "~/.claude/skills leads into the folder CLAUDE_CONFIG_DIR names, so reet leaves it alone"},
		{"XDG_CONFIG_HOME", ".config/opencode/skills", "xdg/opencode/skills", "XDG_CONFIG_HOME=xdg", "~/.config/opencode/skills leads into the folder XDG_CONFIG_HOME names, so reet leaves it alone"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			place := filepath.Join(home, filepath.FromSlash(c.place))
			writeSkill(t, filepath.Join(place, "legacy"))
			symlink(t, place, filepath.Join(home, filepath.FromSlash(c.folder)))
			if name, value, ok := strings.Cut(c.env, "="); ok {
				t.Setenv(name, filepath.Join(home, value))
			}
			before := homeTree(t, place)
			d := store.DesiredState{Version: "v1", Remove: []store.DesiredRemoval{{Name: "legacy", Reason: "archived"}}}
			opts := reconcileOptions{unmanaged: []string{filepath.Join(home, "work", "app")}}
			rep, _ := reconcileWith(context.Background(), home, d, map[string]string{}, (&stubBundles{}).fetch, reconcileNow, opts)
			sameTree(t, c.place, before, homeTree(t, place))
			if e := entryOf(t, rep, "~/"+c.folder, "legacy"); e.State != "failed" || e.Detail != c.want {
				t.Fatalf("entry %+v", e)
			}
		})
	}
}

// A project or custom folder that is home itself would cover every managed
// folder; it is no reason to leave them alone.
func TestReconcileIgnoresAProjectThatIsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	a, aFiles := skillBundle(t, "alpha", "first")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	opts := reconcileOptions{unmanaged: []string{home, filepath.Dir(home)}}
	rep, _ := reconcileWith(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, stub.fetch, reconcileNow, opts)
	if e := entryOf(t, rep, "~/.agents/skills", "alpha"); e.State != "installed" {
		t.Fatalf("entry %+v", e)
	}
}

// The last guard: a folder that turns into a link to somewhere else while the
// pass runs is not written through.
func TestReconcileRefusesAFolderThatMovesDuringThePass(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".claude/skills", "elsewhere")
	claude := filepath.Join(home, ".claude", "skills")
	a, aFiles := skillBundle(t, "alpha", "first")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}, onFetch: func() {
		if err := os.Rename(claude, claude+"-old"); err != nil {
			t.Error(err)
		}
		if err := os.Symlink(filepath.Join(home, "elsewhere"), claude); err != nil {
			t.Error(err)
		}
	}}
	rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, stub.fetch, reconcileNow)
	if e := entryOf(t, rep, "~/.claude/skills", "alpha"); e.State != "failed" || !strings.Contains(e.Detail, "changed while reet was working") {
		t.Fatalf("entry %+v", e)
	}
	if entries, _ := os.ReadDir(filepath.Join(home, "elsewhere")); len(entries) != 0 {
		t.Fatalf("wrote %v through the new link", entries)
	}
	if e := entryOf(t, rep, "~/.agents/skills", "alpha"); e.State != "installed" {
		t.Fatalf("entry %+v", e)
	}
}

func TestReconcileFindsCodexThroughALinkedHome(t *testing.T) {
	realHome := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	symlink(t, realHome, home)
	writeSkill(t, filepath.Join(realHome, ".codex", "legacy"))
	symlink(t, filepath.Join("..", ".codex"), filepath.Join(realHome, ".claude", "skills"))
	before := homeTree(t, filepath.Join(realHome, ".codex"))
	d := store.DesiredState{Version: "v1", Remove: []store.DesiredRemoval{{Name: "legacy", Reason: "archived"}}}
	rep, _ := reconcileSkills(context.Background(), home, d, map[string]string{}, (&stubBundles{}).fetch, reconcileNow)
	sameTree(t, ".codex", before, homeTree(t, filepath.Join(realHome, ".codex")))
	if e := entryOf(t, rep, "~/.claude/skills", "legacy"); e.State != "failed" {
		t.Fatalf("entry %+v", e)
	}
}

// A removal named like a project must never move the project into a backup
// that is deleted after 30 days.
func TestReconcileLeavesAFolderHoldingAProjectAlone(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "work", "app")
	writeSkill(t, project)
	symlink(t, filepath.Join(home, "work"), filepath.Join(home, ".claude", "skills"))
	before := homeTree(t, filepath.Join(home, "work"))
	d := store.DesiredState{Version: "v1", Remove: []store.DesiredRemoval{{Name: "app", Reason: "archived"}}}
	rep, _ := reconcileWith(context.Background(), home, d, map[string]string{}, (&stubBundles{}).fetch, reconcileNow, reconcileOptions{unmanaged: []string{project}})
	sameTree(t, "work", before, homeTree(t, filepath.Join(home, "work")))
	if e := entryOf(t, rep, "~/.claude/skills", "app"); e.State != "failed" || e.Detail != "~/.claude/skills holds a project folder, so reet leaves it alone" {
		t.Fatalf("entry %+v", e)
	}
}

// A project that holds home covers every managed folder, but a folder that
// leads into it outside home is still in the project.
func TestReconcileLeavesAFolderInAProjectHoldingHomeAlone(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "me")
	inProject := filepath.Join(base, "app", ".claude", "skills")
	writeSkill(t, filepath.Join(inProject, "legacy"))
	symlink(t, inProject, filepath.Join(home, ".claude", "skills"))
	before := homeTree(t, filepath.Join(base, "app"))
	d := store.DesiredState{Version: "v1", Remove: []store.DesiredRemoval{{Name: "legacy", Reason: "archived"}}}
	rep, _ := reconcileWith(context.Background(), home, d, map[string]string{}, (&stubBundles{}).fetch, reconcileNow, reconcileOptions{unmanaged: []string{base}})
	sameTree(t, "project", before, homeTree(t, filepath.Join(base, "app")))
	if e := entryOf(t, rep, "~/.claude/skills", "legacy"); e.State != "failed" || e.Detail != "~/.claude/skills leads into a project folder, so reet leaves it alone" {
		t.Fatalf("entry %+v", e)
	}
}

func TestReconcileCleansStagingOnlyBesideFoldersItWorksIn(t *testing.T) {
	for _, c := range []struct{ name, skills string }{
		// ~/.claude is a link into a project, and so is its skills folder.
		{"folder and parent in a project", ""},
		// Only ~/.claude leads into the project; its skills folder is a
		// link out of it, but backups and staging would land beside it.
		{"only the parent in a project", "elsewhere/skills"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			project := filepath.Join(home, "work", "app")
			claudeDir := filepath.Join(project, ".claude")
			mkdirs(t, home, "work/app/.claude/.reet-private-7")
			if c.skills == "" {
				writeSkill(t, filepath.Join(claudeDir, "skills", "legacy"))
			} else {
				writeSkill(t, filepath.Join(home, filepath.FromSlash(c.skills), "legacy"))
				symlink(t, filepath.Join(home, filepath.FromSlash(c.skills)), filepath.Join(claudeDir, "skills"))
			}
			symlink(t, claudeDir, filepath.Join(home, ".claude"))
			before := homeTree(t, home)
			d := store.DesiredState{Version: "v1", Remove: []store.DesiredRemoval{{Name: "legacy", Reason: "archived"}}}
			rep, _ := reconcileWith(context.Background(), home, d, map[string]string{}, (&stubBundles{}).fetch, reconcileNow, reconcileOptions{unmanaged: []string{project}})
			sameTree(t, "home", before, homeTree(t, home))
			if e := entryOf(t, rep, "~/.claude/skills", "legacy"); e.State != "failed" {
				t.Fatalf("entry %+v", e)
			}
		})
	}
}

// A folder pointed elsewhere while the pass runs is neither installed into
// nor backed up from.
func TestReconcileBacksUpNothingThroughAFolderRepointedMidPass(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude", "skills")
	writeSkill(t, filepath.Join(claude, "gone"))
	writeSkill(t, filepath.Join(home, "elsewhere", "gone"))
	before := homeTree(t, filepath.Join(home, "elsewhere"))
	a, aFiles := skillBundle(t, "alpha", "first")
	was := readBackDigest
	t.Cleanup(func() { readBackDigest = was })
	moved := false
	readBackDigest = func(p string) (string, error) {
		// Claude's copy is staged after its folder was listed: point the
		// folder elsewhere now.
		if !moved && strings.Contains(p, string(filepath.Separator)+".claude"+string(filepath.Separator)) {
			moved = true
			if err := os.Rename(claude, claude+"-old"); err != nil {
				t.Error(err)
			}
			if err := os.Symlink(filepath.Join(home, "elsewhere"), claude); err != nil {
				t.Error(err)
			}
		}
		return privateSkillDigest(p)
	}
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}, Remove: []store.DesiredRemoval{{Name: "gone", Reason: "archived"}}}
	rep, _ := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow)
	if !moved {
		t.Fatal("the folder was never pointed elsewhere")
	}
	sameTree(t, "elsewhere", before, homeTree(t, filepath.Join(home, "elsewhere")))
	for _, name := range []string{"alpha", "gone"} {
		if e := entryOf(t, rep, "~/.claude/skills", name); e.State != "failed" || !strings.Contains(e.Detail, "changed while reet was working") {
			t.Fatalf("entry %+v", e)
		}
	}
}

// repoint points the folder at dir elsewhere, as someone might while a pass
// runs, keeping what it held in dir-old.
func repoint(t *testing.T, dir, elsewhere string) {
	t.Helper()
	if err := os.Rename(dir, dir+"-old"); err != nil {
		t.Error(err)
	}
	if err := os.Symlink(elsewhere, dir); err != nil {
		t.Error(err)
	}
}

// A folder pointed elsewhere after the pass started, before its turn, is not
// read at all: what the new place holds is not this folder's to report.
func TestReconcileReadsNothingInAFolderMovedBeforeItsTurn(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, ".claude/skills")
	elsewhere := filepath.Join(home, "elsewhere")
	writeSkill(t, filepath.Join(elsewhere, "cq"))
	before := homeTree(t, elsewhere)
	a, aFiles := skillBundle(t, "alpha", "first")
	// The bundle is fetched for ~/.agents/skills, before Claude's turn.
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}, onFetch: func() {
		repoint(t, filepath.Join(home, ".claude", "skills"), elsewhere)
	}}
	rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, stub.fetch, reconcileNow)
	sameTree(t, "elsewhere", before, homeTree(t, elsewhere))
	var claude []string
	for _, e := range rep.Entries {
		if e.Folder == "~/.claude/skills" {
			claude = append(claude, e.Name+" "+e.State+" "+e.Detail)
		}
	}
	if want := "alpha failed ~/.claude/skills changed while reet was working on it; it is tried again next time"; strings.Join(claude, "|") != want {
		t.Fatalf("Claude's entries %q, want %q", claude, want)
	}
}

// A folder pointed elsewhere while reet's copy is staged, after its copies
// were listed, has none of them backed up: the backup would move what the
// new place holds.
func TestReconcileBacksUpNoCopyThroughAFolderRepointedWhileStaging(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude", "skills")
	_, older := skillBundle(t, "alpha", "older")
	placeFiles(t, filepath.Join(claude, "alpha"), older)
	elsewhere := filepath.Join(home, "elsewhere")
	writeSkill(t, filepath.Join(elsewhere, "alpha"))
	before := homeTree(t, elsewhere)
	a, aFiles := skillBundle(t, "alpha", "first")
	was := readBackDigest
	t.Cleanup(func() { readBackDigest = was })
	moved := false
	readBackDigest = func(p string) (string, error) {
		if !moved && strings.Contains(p, string(filepath.Separator)+".claude"+string(filepath.Separator)) {
			moved = true
			repoint(t, claude, elsewhere)
		}
		return privateSkillDigest(p)
	}
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, stub.fetch, reconcileNow)
	if !moved {
		t.Fatal("the folder was never pointed elsewhere")
	}
	sameTree(t, "elsewhere", before, homeTree(t, elsewhere))
	if got := folderDigest(t, filepath.Join(claude+"-old", "alpha")); got != digestOfFiles(t, older) {
		t.Fatal("the copy in the folder as it was is not as it was")
	}
	if e := entryOf(t, rep, "~/.claude/skills", "alpha"); e.State != "failed" || !strings.Contains(e.Detail, "changed while reet was working") || e.Backup != "" {
		t.Fatalf("entry %+v", e)
	}
}

// A folder pointed elsewhere after its copy was backed up, before reet's copy
// went in, has nothing put back through it: the copy stays in its backup and
// the new place is left as it is.
func TestReconcilePutsNothingBackThroughAFolderRepointedMidInstall(t *testing.T) {
	home := t.TempDir()
	shared := filepath.Join(home, ".agents", "skills")
	_, older := skillBundle(t, "alpha", "older")
	placeFiles(t, filepath.Join(shared, "alpha"), older)
	elsewhere := filepath.Join(home, "elsewhere")
	mkdirs(t, home, "elsewhere")
	before := homeTree(t, elsewhere)
	was := reconcileMove
	t.Cleanup(func() { reconcileMove = was })
	reconcileMove = func(string, string) (bool, error) {
		repoint(t, shared, elsewhere)
		return false, errors.New("no space left on device")
	}
	a, aFiles := skillBundle(t, "alpha", "first")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, stub.fetch, reconcileNow)
	sameTree(t, "elsewhere", before, homeTree(t, elsewhere))
	backup := "~/.agents/.reet-backups/2026-11-18/.agents-alpha"
	e := entryOf(t, rep, "~/.agents/skills", "alpha")
	if e.State != "failed" || e.Backup != backup || !strings.Contains(e.Detail, "stays in "+backup+": ~/.agents/skills changed while reet was working") {
		t.Fatalf("entry %+v", e)
	}
	if got := folderDigest(t, filepath.Join(home, strings.TrimPrefix(backup, "~/"))); got != digestOfFiles(t, older) {
		t.Fatal("the copy is not in its backup")
	}
}

func TestReconcileReportsAFolderLeftAloneThatCannotBeRead(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0755); err != nil {
		t.Fatal(err)
	}
	// Codex's skills folder is a file here, so listing it fails.
	if err := os.WriteFile(filepath.Join(home, ".codex", "skills"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	symlink(t, filepath.Join("..", ".codex", "skills"), filepath.Join(home, ".claude", "skills"))
	a, aFiles := skillBundle(t, "alpha", "first")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}, Remove: []store.DesiredRemoval{{Name: "legacy", Reason: "archived"}}}
	rep, _ := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow)
	for _, name := range []string{"alpha", "legacy"} {
		e := entryOf(t, rep, "~/.claude/skills", name)
		if e.State != "failed" || !strings.HasPrefix(e.Detail, "~/.claude/skills leads into Codex's skills folder, so reet leaves it alone") || !strings.Contains(e.Detail, "cannot read") {
			t.Fatalf("entry %+v", e)
		}
	}
}

// Through a linked home, the usual ~/.claude named in its other form is not
// a custom folder.
func TestReconcileTreatsTheUsualClaudeFolderAsUsualThroughALinkedHome(t *testing.T) {
	realHome := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	symlink(t, realHome, home)
	writeSkill(t, filepath.Join(realHome, ".claude", "skills", "gone"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(realHome, ".claude"))
	d := store.DesiredState{Version: "v1", Remove: []store.DesiredRemoval{{Name: "gone", Reason: "archived"}}}
	rep, _ := reconcileSkills(context.Background(), home, d, map[string]string{}, (&stubBundles{}).fetch, reconcileNow)
	if e := entryOf(t, rep, "~/.claude/skills", "gone"); e.State != "removed" {
		t.Fatalf("entry %+v", e)
	}
}

// A folder whose real parent reet cannot write in, such as a mount root, is
// staged for beside where it is under home instead.
func TestReconcileStagesBesideTheFolderWhenItsRealParentIsClosed(t *testing.T) {
	home := t.TempDir()
	mount := filepath.Join(home, "mnt")
	mkdirs(t, home, "mnt/skills")
	symlink(t, filepath.Join(mount, "skills"), filepath.Join(home, ".claude", "skills"))
	was := makeStaging
	t.Cleanup(func() { makeStaging = was })
	makeStaging = func(dir, pattern string) (string, error) {
		if dir == mount {
			return "", &os.PathError{Op: "mkdirtemp", Path: dir, Err: syscall.EACCES}
		}
		return os.MkdirTemp(dir, pattern)
	}
	a, aFiles := skillBundle(t, "alpha", "first")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, stub.fetch, reconcileNow)
	if e := entryOf(t, rep, "~/.claude/skills", "alpha"); e.State != "installed" {
		t.Fatalf("entry %+v", e)
	}
	if got := folderDigest(t, filepath.Join(mount, "skills", "alpha")); got != a.Digest {
		t.Fatal("reet's copy is not on the mount")
	}
	if left := stagingLeft(t, home); len(left) != 0 {
		t.Fatalf("staging left: %v", left)
	}
}

func TestReconcileCleansStagingBesideALinkedFolder(t *testing.T) {
	home := t.TempDir()
	elsewhere := filepath.Join(home, "dotfiles", "editors", "cursor", "skills")
	// A cross-device copy cut short leaves its hidden folder inside, named
	// by an earlier agent after the skill, or now apart from it.
	mkdirs(t, home, "dotfiles/editors/cursor/skills/.alpha.reet-copy-5", "dotfiles/editors/cursor/skills/.reet-copy-6", "dotfiles/editors/cursor/.reet-private-9")
	symlink(t, elsewhere, filepath.Join(home, ".cursor", "skills"))
	reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1"}, map[string]string{}, (&stubBundles{}).fetch, reconcileNow)
	if left := stagingLeft(t, home); len(left) != 0 {
		t.Fatalf("staging left: %v", left)
	}
	for _, cut := range []string{".alpha.reet-copy-5", ".reet-copy-6"} {
		if _, err := os.Lstat(filepath.Join(elsewhere, cut)); !os.IsNotExist(err) {
			t.Fatalf("a cut-short copy was left: %s", cut)
		}
	}
}

// A copy moved across devices is made in a hidden folder named apart from
// the skill, so a name as long as a skill name may be still fits.
func TestReconcileInstallsALongNameAcrossDevices(t *testing.T) {
	home := t.TempDir()
	was := renameEntry
	t.Cleanup(func() { renameEntry = was })
	renameEntry = func(src, dest string) error {
		return &os.LinkError{Op: "rename", Old: src, New: dest, Err: syscall.EXDEV}
	}
	name := strings.Repeat("a", 250)
	a, aFiles := skillBundle(t, name, "long")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{name: aFiles}}
	rep, _ := reconcileSkills(context.Background(), home, store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}}, map[string]string{}, stub.fetch, reconcileNow)
	if e := entryOf(t, rep, "~/.agents/skills", name); e.State != "installed" {
		t.Fatalf("entry %+v", e)
	}
	shared := filepath.Join(home, ".agents", "skills")
	if got := folderDigest(t, filepath.Join(shared, name)); got != a.Digest {
		t.Fatal("reet's copy is not in place")
	}
	if entries, _ := os.ReadDir(shared); len(entries) != 1 {
		t.Fatalf("left %v", entries)
	}
}

// A folder whose parent is pointed elsewhere while the pass runs, though the
// folder still resolves to where it was, gets nothing more: its backups and
// fallback staging would land in the parent's new place.
func TestReconcileBacksUpNothingBesideAParentRepointedMidPass(t *testing.T) {
	home := t.TempDir()
	skills := filepath.Join(home, "dotfiles", "claude-skills")
	writeSkill(t, filepath.Join(skills, "gone"))
	for _, parent := range []string{"claude-a", "claude-b"} {
		symlink(t, skills, filepath.Join(home, parent, "skills"))
	}
	claude := filepath.Join(home, ".claude")
	symlink(t, filepath.Join(home, "claude-a"), claude)
	before := homeTree(t, filepath.Join(home, "claude-b"))
	a, aFiles := skillBundle(t, "alpha", "first")
	was := readBackDigest
	t.Cleanup(func() { readBackDigest = was })
	moved := false
	readBackDigest = func(p string) (string, error) {
		// Claude's copy is staged beside its real folder: point ~/.claude
		// at the other parent now.
		if !moved && strings.Contains(p, string(filepath.Separator)+"dotfiles"+string(filepath.Separator)) {
			moved = true
			if err := os.Remove(claude); err != nil {
				t.Error(err)
			}
			if err := os.Symlink(filepath.Join(home, "claude-b"), claude); err != nil {
				t.Error(err)
			}
		}
		return privateSkillDigest(p)
	}
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}, Remove: []store.DesiredRemoval{{Name: "gone", Reason: "archived"}}}
	rep, _ := reconcileSkills(context.Background(), home, d, map[string]string{}, stub.fetch, reconcileNow)
	if !moved {
		t.Fatal("the parent was never pointed elsewhere")
	}
	sameTree(t, "claude-b", before, homeTree(t, filepath.Join(home, "claude-b")))
	if _, err := os.Stat(filepath.Join(skills, "gone", "SKILL.md")); err != nil {
		t.Fatalf("gone was moved: %v", err)
	}
	for _, name := range []string{"alpha", "gone"} {
		if e := entryOf(t, rep, "~/.claude/skills", name); e.State != "failed" || !strings.Contains(e.Detail, "changed while reet was working") {
			t.Fatalf("entry %+v", e)
		}
	}
}

// Two names for one folder are one folder: a project's protection goes by the
// folder it is, not the name it was enrolled under.
func TestReconcileComparesFoldersByIdentity(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "work/app/.claude/skills", "work/apple")
	// "Work" is a second name for "work", as another case is on a disk that
	// ignores case.
	symlink(t, filepath.Join(root, "work"), filepath.Join(root, "Work"))
	for _, c := range []struct {
		p, dir string
		want   bool
	}{
		{"work/app/.claude/skills", "Work/app", true},
		{"work/app", "Work/app", true},
		{"work/app/not-made-yet/x", "Work/app", true},
		{"work/apple", "Work/app", false},
		{"work", "Work/app", false},
		// Where the folder does not exist, the paths' text decides.
		{"gone/app/x", "gone/app", true},
		{"gone/apple", "gone/app", false},
	} {
		p, dir := filepath.Join(root, filepath.FromSlash(c.p)), filepath.Join(root, filepath.FromSlash(c.dir))
		if got := within(p, dir); got != c.want {
			t.Errorf("within(%s, %s) = %v, want %v", c.p, c.dir, got, c.want)
		}
	}
	if !samePath(filepath.Join(root, "work", "app"), filepath.Join(root, "Work", "app")) {
		t.Error("two names for one folder are not the same folder")
	}
	if samePath(filepath.Join(root, "work", "app"), filepath.Join(root, "work", "apple")) {
		t.Error("two folders are the same folder")
	}
}

// A managed folder that reaches an enrolled project under another name for
// its folder is still left alone. On a disk that ignores case, ~/Work/app and
// ~/work/app are one folder, and resolving links keeps the case each path was
// given in, so the paths' text alone does not show it.
func TestReconcileKnowsAProjectByItsFolderNotItsName(t *testing.T) {
	for _, c := range []struct{ name, skills, remove, why string }{
		{"leads into", "work/app/.claude/skills", "legacy", "~/.claude/skills leads into a project folder, so reet leaves it alone"},
		{"holds", "work", "app", "~/.claude/skills holds a project folder, so reet leaves it alone"},
	} {
		for _, second := range []string{"a linked name", "another case"} {
			t.Run(c.name+" under "+second, func(t *testing.T) {
				home := t.TempDir()
				work := filepath.Join(home, "work")
				writeSkill(t, filepath.Join(work, "app", ".claude", "skills", "legacy"))
				symlink(t, filepath.Join(home, filepath.FromSlash(c.skills)), filepath.Join(home, ".claude", "skills"))
				enrolled := filepath.Join(home, "Work", "app")
				if second == "another case" {
					if caseSensitive(t, work) {
						t.Skip("this file system keeps names differing only in case apart")
					}
				} else {
					// "projects" stands for "Work": a second name for the
					// folder that resolving keeps as it was given.
					alias := filepath.Join(home, "projects")
					enrolled = filepath.Join(alias, "app")
					symlink(t, work, alias)
					realWork, err := filepath.EvalSymlinks(work)
					if err != nil {
						t.Fatal(err)
					}
					was := resolveLinks
					t.Cleanup(func() { resolveLinks = was })
					resolveLinks = func(p string) (string, error) {
						r, err := filepath.EvalSymlinks(p)
						if rest, ok := strings.CutPrefix(p, alias); err == nil && ok && (rest == "" || rest[0] == filepath.Separator) {
							r = alias + strings.TrimPrefix(r, realWork)
						}
						return r, err
					}
				}
				before := homeTree(t, work)
				d := store.DesiredState{Version: "v1", Remove: []store.DesiredRemoval{{Name: c.remove, Reason: "archived"}}}
				rep, _ := reconcileWith(context.Background(), home, d, map[string]string{}, (&stubBundles{}).fetch, reconcileNow, reconcileOptions{unmanaged: []string{enrolled}})
				sameTree(t, "work", before, homeTree(t, work))
				if e := entryOf(t, rep, "~/.claude/skills", c.remove); e.State != "failed" || e.Detail != c.why {
					t.Fatalf("entry %+v", e)
				}
			})
		}
	}
}

func TestReconcileNeedsAHomeFolder(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	a, aFiles := skillBundle(t, "alpha", "first")
	stub := &stubBundles{files: map[string][]skillbundle.ContentFile{"alpha": aFiles}}
	installed := map[string]string{".agents/skills/alpha": a.Digest}
	d := store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}, Remove: []store.DesiredRemoval{{Name: "gone", Reason: "archived"}}}
	rep, next := reconcileSkills(context.Background(), "", d, installed, stub.fetch, reconcileNow)
	if entries, _ := os.ReadDir(cwd); len(entries) != 0 || stub.calls != 0 {
		t.Fatalf("wrote %v relative to the working folder; fetches %d", entries, stub.calls)
	}
	if len(rep.Entries) != 2 || rep.Entries[0].State != "failed" || rep.Entries[1].State != "failed" {
		t.Fatalf("entries %+v", rep.Entries)
	}
	if !maps.Equal(next, installed) {
		t.Fatalf("installed %v", next)
	}
}

// skillController is a controller that serves the poll, the bundle download
// and the skills report, and records what the agent sent.
type skillController struct {
	*httptest.Server
	mu           sync.Mutex
	desired      *store.DesiredState
	revisions    map[string]store.PrivateSkillRevision
	reportStatus int
	polls        []polled
	attempts     int
	reports      []store.SkillReport
	fetches      int
}

type polled struct{ protocol, skillsVersion string }

func newSkillController(t *testing.T) *skillController {
	t.Helper()
	c := &skillController{revisions: map[string]store.PrivateSkillRevision{}}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		switch {
		case r.URL.Path == "/api/v2/sync/poll":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("poll body: %v", err)
			}
			c.polls = append(c.polls, polled{r.Header.Get("X-Reet-Skills-Protocol"), body["skillsVersion"]})
			reply := map[string]any{"agent": map[string]any{"id": testMachineID, "host": "studio"}}
			if c.desired != nil {
				reply["desired"] = c.desired
			}
			json.NewEncoder(w).Encode(reply)
		case r.URL.Path == "/api/v2/sync/skills/report":
			c.attempts++
			var rep store.SkillReport
			if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
				t.Errorf("report body: %v", err)
			}
			if c.reportStatus != 0 {
				w.WriteHeader(c.reportStatus)
				return
			}
			c.reports = append(c.reports, rep)
			json.NewEncoder(w).Encode(map[string]bool{"recorded": true})
		case strings.HasPrefix(r.URL.Path, "/api/v2/skill-bundles/"):
			c.fetches++
			rev, ok := c.revisions[path.Base(r.URL.Path)]
			if !ok {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(rev)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(c.Close)
	return c
}

// serve makes s's files available as its revision.
func (c *skillController) serve(t *testing.T, s store.DesiredSkill, files []skillbundle.ContentFile) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.revisions[s.Revision] = store.PrivateSkillRevision{ID: s.Revision, BundleID: s.Bundle, ManifestDigest: s.Digest, Files: files}
}

func (c *skillController) setDesired(d *store.DesiredState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.desired = d
}

func (c *skillController) counts() (polls, attempts, reports, fetches int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.polls), c.attempts, len(c.reports), c.fetches
}

func (c *skillController) lastReport(t *testing.T) store.SkillReport {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.reports) == 0 {
		t.Fatal("no report was posted")
	}
	return c.reports[len(c.reports)-1]
}

func (c *skillController) lastPoll(t *testing.T) polled {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.polls) == 0 {
		t.Fatal("the agent did not poll")
	}
	return c.polls[len(c.polls)-1]
}

// skillCycle runs one agent cycle with no inventory scan due.
func skillCycle(t *testing.T, config string) error {
	t.Helper()
	_, err := skillCycleOut(t, config)
	return err
}

// skillCycleOut is skillCycle, also returning what the cycle printed.
func skillCycleOut(t *testing.T, config string) (string, error) {
	t.Helper()
	c := loaded(t, config)
	last := time.Now()
	var out bytes.Buffer
	err := agentCycle(context.Background(), c, config, &out, "test", &last, false)
	return out.String(), err
}

func TestAgentCycleReconcilesFromCachedDesired(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	home := loaded(t, config).Home
	a, aFiles := skillBundle(t, "alpha", "first")
	a.Bundle, a.Revision = strings.Repeat("1", 32), strings.Repeat("2", 32)
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}, Remove: []store.DesiredRemoval{{Name: "gone", Reason: "archived"}}})
	expired := filepath.Join(home, ".agents", ".reet-backups", "2020-01-01")
	writeSkill(t, filepath.Join(expired, ".agents-old"))
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(expired); !os.IsNotExist(err) {
		t.Fatal("backups older than 30 days were not pruned after the reconcile")
	}
	if p := srv.lastPoll(t); p.protocol != "2" || p.skillsVersion != "" {
		t.Fatalf("first poll %+v", p)
	}
	if got := folderDigest(t, filepath.Join(home, ".agents", "skills", "alpha")); got != a.Digest {
		t.Fatal("alpha was not installed from the controller's bundle")
	}
	if rep := srv.lastReport(t); rep.Version != "v1" || len(rep.Entries) != 1 || rep.Entries[0].State != "installed" {
		t.Fatalf("report %+v", rep)
	}
	for _, suffix := range []string{".desired", ".installed"} {
		st, err := os.Stat(config + suffix)
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatalf("%s: %v %v", suffix, st, err)
		}
	}

	// The controller leaves desired out once the machine has its version, and
	// the machine still puts back what is on disk by hand.
	srv.setDesired(nil)
	writeSkill(t, filepath.Join(home, ".agents", "skills", "gone"))
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	if p := srv.lastPoll(t); p.skillsVersion != "v1" {
		t.Fatalf("second poll %+v", p)
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "gone")); !os.IsNotExist(err) {
		t.Fatal("the archived skill put back by hand was not removed")
	}
	backups, _ := filepath.Glob(filepath.Join(home, ".agents", ".reet-backups", "*", ".agents-gone"))
	if len(backups) != 1 {
		t.Fatalf("backups %v", backups)
	}
	if rep := srv.lastReport(t); rep.Version != "v1" || len(rep.Entries) != 2 || rep.Entries[1].State != "removed" {
		t.Fatalf("report %+v", rep)
	}
	if _, _, _, fetches := srv.counts(); fetches != 1 {
		t.Fatalf("fetched %d times", fetches)
	}
}

func TestAgentCycleIgnoresDesiredKeptForAnotherController(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	home := loaded(t, config).Home
	writeSkill(t, filepath.Join(home, ".agents", "skills", "gone"))
	kept := desiredCache{Server: "https://other.example", Agent: testMachineID, Desired: store.DesiredState{Version: "v9", Remove: []store.DesiredRemoval{{Name: "gone", Reason: "archived"}}}}
	if err := writePrivate(config+".desired", kept); err != nil {
		t.Fatal(err)
	}
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	if p := srv.lastPoll(t); p.skillsVersion != "" {
		t.Fatalf("poll %+v", p)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "gone", "SKILL.md")); err != nil {
		t.Fatal("another controller's desired state was applied")
	}
	if _, attempts, _, _ := srv.counts(); attempts != 0 {
		t.Fatalf("%d reports without a desired state", attempts)
	}
}

func TestAgentCycleMovesACorruptInstalledFileAside(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	a, aFiles := skillBundle(t, "alpha", "first")
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}})
	if err := os.WriteFile(config+".installed", []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := skillCycleOut(t, config)
	if err != nil {
		t.Fatal(err)
	}
	aside, _ := filepath.Glob(config + ".installed.corrupt-*")
	if len(aside) != 1 {
		t.Fatalf("moved aside: %v", aside)
	}
	if b, _ := os.ReadFile(aside[0]); string(b) != "{not json" {
		t.Fatalf("aside holds %q", b)
	}
	if !strings.Contains(out, filepath.Base(aside[0])) {
		t.Fatalf("not logged: %q", out)
	}
	var got map[string]string
	if b, err := os.ReadFile(config + ".installed"); err != nil || json.Unmarshal(b, &got) != nil || got[".agents/skills/alpha"] != a.Digest {
		t.Fatalf("installed %v (%v)", got, err)
	}
}

func TestAgentCycleSecondCycleWritesNothing(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	home := loaded(t, config).Home
	mkdirs(t, home, ".cursor/skills")
	a, aFiles := skillBundle(t, "alpha", "first")
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}, Remove: []store.DesiredRemoval{{Name: "gone", Reason: "deleted"}}})
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	stat := func() []os.FileInfo {
		t.Helper()
		var infos []os.FileInfo
		for _, suffix := range []string{".installed", ".desired"} {
			st, err := os.Stat(config + suffix)
			if err != nil {
				t.Fatal(err)
			}
			infos = append(infos, st)
		}
		return infos
	}
	files, tree := stat(), homeTree(t, home)
	_, _, reports, fetches := srv.counts()
	// The same desired state again, then the controller leaving it out.
	for cycle := 0; cycle < 2; cycle++ {
		if err := skillCycle(t, config); err != nil {
			t.Fatal(err)
		}
		srv.setDesired(nil)
	}
	for i, st := range stat() {
		if !os.SameFile(files[i], st) || !files[i].ModTime().Equal(st.ModTime()) {
			t.Fatalf("%s was written again", st.Name())
		}
	}
	sameTree(t, "home", tree, homeTree(t, home))
	if _, _, r, f := srv.counts(); r != reports || f != fetches {
		t.Fatalf("reports %d→%d, fetches %d→%d", reports, r, fetches, f)
	}
}

// An agent stopped after it backed up an edited copy and before reet's copy
// went in still reports the edit after it starts again.
func TestAgentCycleReportsAnEditWhenStoppedBeforeTheInstall(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	home := loaded(t, config).Home
	a, aFiles := skillBundle(t, "alpha", "first")
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}})
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".agents", "skills", "alpha", "SKILL.md"), []byte("my own edit"), 0644); err != nil {
		t.Fatal(err)
	}
	was := reconcileMove
	reconcileMove = func(string, string) (bool, error) { panic("the agent was stopped") }
	func() {
		defer func() { recover() }()
		skillCycle(t, config)
	}()
	reconcileMove = was
	skillReportsSent.Lock()
	delete(skillReportsSent.last, config)
	skillReportsSent.Unlock()
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	e := entryOf(t, srv.lastReport(t), "~/.agents/skills", "alpha")
	if e.State != "edited" || !strings.HasPrefix(e.Backup, "~/.agents/.reet-backups/") {
		t.Fatalf("entry %+v", e)
	}
	if b, err := os.ReadFile(filepath.Join(home, strings.TrimPrefix(e.Backup, "~/"), "SKILL.md")); err != nil || string(b) != "my own edit" {
		t.Fatalf("backup %q %v", b, err)
	}
}

// An edit whose copy could not go back after reet's copy failed to go in
// lives only in its backup; the controller must still hear of it.
func TestAgentCycleReportsAnEditWhoseCopyCouldNotGoBack(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	home := loaded(t, config).Home
	a, aFiles := skillBundle(t, "alpha", "first")
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}})
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".agents", "skills", "alpha", "SKILL.md"), []byte("my own edit"), 0644); err != nil {
		t.Fatal(err)
	}
	was := reconcileMove
	t.Cleanup(func() { reconcileMove = was })
	// The move fails after something took the skill's place, so the edited
	// copy cannot be put back.
	reconcileMove = func(src, dest string) (bool, error) {
		if err := os.WriteFile(dest, []byte("in the way"), 0644); err != nil {
			return false, err
		}
		return false, errors.New("no space left on device")
	}
	skillCycle(t, config)
	rep := srv.lastReport(t)
	var edited []store.SkillReportEntry
	for _, e := range rep.Entries {
		if e.Name == "alpha" && e.State == "edited" {
			edited = append(edited, e)
		}
	}
	if len(edited) != 1 || !strings.HasPrefix(edited[0].Backup, "~/.agents/.reet-backups/") {
		t.Fatalf("report %+v", rep)
	}
	if b, err := os.ReadFile(filepath.Join(home, strings.TrimPrefix(edited[0].Backup, "~/"), "SKILL.md")); err != nil || string(b) != "my own edit" {
		t.Fatalf("backup %q %v", b, err)
	}
}

// An edit passed on before another copy of the same skill could not be
// backed up still reaches the controller, though the folder reports failed.
func TestAgentCycleReportsAnEditWhenAnotherCopyCannotMove(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	shared := filepath.Join(loaded(t, config).Home, ".agents", "skills")
	mkdirs(t, loaded(t, config).Home, ".agents/skills")
	if !caseSensitive(t, shared) {
		t.Skip("this file system cannot hold names differing only in case")
	}
	a, aFiles := skillBundle(t, "alpha", "first")
	placeFiles(t, filepath.Join(shared, "alpha"), aFiles)
	placeFiles(t, filepath.Join(shared, "ALPHA"), []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("my own edit")}})
	writeSkill(t, filepath.Join(shared, "Alpha"))
	failingBackup(t, "Alpha")
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}})
	skillCycle(t, config)
	rep := srv.lastReport(t)
	var states []string
	for _, e := range rep.Entries {
		if strings.EqualFold(e.Name, "alpha") {
			states = append(states, e.Name+" "+e.State+" "+e.Backup)
		}
	}
	date := time.Now().Format(backupDateLayout)
	// The edit is reported under the name its copy had.
	want := []string{"ALPHA edited ~/.agents/.reet-backups/" + date + "/.agents-ALPHA", "alpha failed ~/.agents/.reet-backups/" + date + "/.agents-ALPHA"}
	if strings.Join(states, "|") != strings.Join(want, "|") {
		t.Fatalf("alpha entries %q, want %q", states, want)
	}
	if _, err := os.Lstat(config + ".unsent"); !os.IsNotExist(err) {
		t.Fatal("delivered entries were kept")
	}
}

func TestAgentCycleBlockedWithoutAllowApply(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	c := loaded(t, config)
	c.AllowApply = false
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	home := c.Home
	writeSkill(t, filepath.Join(home, ".agents", "skills", "gone"))
	writeSkill(t, filepath.Join(home, ".agents", ".reet-backups", "2020-01-01", ".agents-old"))
	a, aFiles := skillBundle(t, "alpha", "first")
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}, Remove: []store.DesiredRemoval{{Name: "gone", Reason: "archived"}}})
	before := homeTree(t, filepath.Join(home, ".agents"))
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	sameTree(t, ".agents", before, homeTree(t, filepath.Join(home, ".agents")))
	for _, dir := range []string{".claude", ".cursor", ".config"} {
		if _, err := os.Lstat(filepath.Join(home, dir)); !os.IsNotExist(err) {
			t.Fatalf("%s was created", dir)
		}
	}
	if _, err := os.Lstat(config + ".installed"); !os.IsNotExist(err) {
		t.Fatal("what reet installed was written although nothing was")
	}
	rep := srv.lastReport(t)
	if rep.Blocked != "allow-apply" || rep.Version != "v1" {
		t.Fatalf("report %+v", rep)
	}
	// The archived skill is still there, so the controller must not take it
	// for gone.
	if e := entryOf(t, rep, "~/.agents/skills", "gone"); e.State != "installed" {
		t.Fatalf("entry %+v", e)
	}
	if _, _, _, fetches := srv.counts(); fetches != 0 {
		t.Fatalf("fetched %d times", fetches)
	}
}

func TestAgentCycleBlockedReportsUnderDesiredNames(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	c := loaded(t, config)
	c.AllowApply = false
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(c.Home, ".agents", "skills")
	seo, seoFiles := skillBundle(t, "seo-audit", "audit")
	ask, _ := skillBundle(t, "askme", "ask")
	_, askOld := skillBundle(t, "askme", "old")
	placeFiles(t, filepath.Join(shared, "SEO-Audit"), seoFiles)
	placeFiles(t, filepath.Join(shared, "askme"), askOld)
	writeSkill(t, filepath.Join(shared, "Old-Thing"))
	writeSkill(t, filepath.Join(shared, "cq"))
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{ask, seo}, Remove: []store.DesiredRemoval{{Name: "old-thing", Reason: "archived"}}})
	before := homeTree(t, shared)
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	sameTree(t, "skills", before, homeTree(t, shared))
	rep := srv.lastReport(t)
	if rep.Blocked != "allow-apply" || len(rep.Entries) != 4 {
		t.Fatalf("report %+v", rep)
	}
	if e := entryOf(t, rep, "~/.agents/skills", "seo-audit"); e.State != "installed" || e.Digest != seo.Digest {
		t.Fatalf("case variant of an Active skill: %+v", e)
	}
	if e := entryOf(t, rep, "~/.agents/skills", "askme"); e.State != "failed" || e.Detail != "allow-apply is off" {
		t.Fatalf("Active copy that differs: %+v", e)
	}
	if e := entryOf(t, rep, "~/.agents/skills", "old-thing"); e.State != "installed" {
		t.Fatalf("archived name still present: %+v", e)
	}
	if e := entryOf(t, rep, "~/.agents/skills", "cq"); e.State != "unknown" || e.Digest != folderDigest(t, filepath.Join(shared, "cq")) {
		t.Fatalf("unknown skill: %+v", e)
	}
}

// An edit backed up and replaced, and a removal, reach the controller even
// when the report that carried them was never delivered.
func TestAgentCycleKeepsUnsentNewsUntilItIsReported(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	home := loaded(t, config).Home
	a, aFiles := skillBundle(t, "alpha", "first")
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}, Remove: []store.DesiredRemoval{{Name: "gone", Reason: "archived"}}})
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".agents", "skills", "alpha", "SKILL.md"), []byte("my own edit"), 0644); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(home, ".agents", "skills", "gone"))
	srv.mu.Lock()
	srv.reportStatus = http.StatusServiceUnavailable
	srv.mu.Unlock()
	if err := skillCycle(t, config); err == nil {
		t.Fatal("a refused report was not an error")
	}
	if st, err := os.Stat(config + ".unsent"); err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("unsent entries were not kept: %v %v", st, err)
	}
	// The agent restarts, and the controller takes reports again.
	skillReportsSent.Lock()
	delete(skillReportsSent.last, config)
	skillReportsSent.Unlock()
	srv.mu.Lock()
	srv.reportStatus = 0
	srv.mu.Unlock()
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	date := time.Now().Format(backupDateLayout)
	rep := srv.lastReport(t)
	if e := entryOf(t, rep, "~/.agents/skills", "alpha"); e.State != "edited" || e.Digest != a.Digest || e.Backup != "~/.agents/.reet-backups/"+date+"/.agents-alpha" {
		t.Fatalf("edited entry %+v", e)
	}
	if e := entryOf(t, rep, "~/.agents/skills", "gone"); e.State != "removed" || e.Backup != "~/.agents/.reet-backups/"+date+"/.agents-gone" {
		t.Fatalf("removed entry %+v", e)
	}
	if _, err := os.Lstat(config + ".unsent"); !os.IsNotExist(err) {
		t.Fatal("delivered entries were kept")
	}
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	if rep := srv.lastReport(t); len(rep.Entries) != 1 || rep.Entries[0].State != "installed" || rep.Entries[0].Backup != "" {
		t.Fatalf("delivered entries were sent again: %+v", rep)
	}
}

func TestAgentCycleSendsTheReportOnlyWhenItChanges(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	home := loaded(t, config).Home
	srv.setDesired(&store.DesiredState{Version: "v1", Remove: []store.DesiredRemoval{{Name: "gone", Reason: "archived"}}})
	posted := func(want int) {
		t.Helper()
		if err := skillCycle(t, config); err != nil {
			t.Fatal(err)
		}
		if _, _, reports, _ := srv.counts(); reports != want {
			t.Fatalf("%d reports posted, want %d", reports, want)
		}
	}
	posted(1) // the first cycle after start always reports
	posted(1)
	writeSkill(t, filepath.Join(home, ".agents", "skills", "gone"))
	posted(2) // it removed gone
	posted(3) // gone is no longer there to report
	posted(3)
	skillReportsSent.Lock()
	delete(skillReportsSent.last, config)
	skillReportsSent.Unlock()
	posted(4) // a restarted agent reports again
}

func TestAgentCycleRetriesAFailedReport(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	srv.setDesired(&store.DesiredState{Version: "v1"})
	srv.mu.Lock()
	srv.reportStatus = http.StatusServiceUnavailable
	srv.mu.Unlock()
	for i := 1; i <= 2; i++ {
		err := skillCycle(t, config)
		if err == nil || !strings.Contains(err.Error(), "report") {
			t.Fatalf("cycle %d: %v", i, err)
		}
		if _, attempts, _, _ := srv.counts(); attempts != i {
			t.Fatalf("cycle %d: %d attempts", i, attempts)
		}
	}
	srv.mu.Lock()
	srv.reportStatus = 0
	srv.mu.Unlock()
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	if _, attempts, reports, _ := srv.counts(); attempts != 3 || reports != 1 {
		t.Fatalf("attempts %d, reports %d", attempts, reports)
	}
	if rep := srv.lastReport(t); rep.Version != "v1" {
		t.Fatalf("report %+v", rep)
	}
}

// Old backups are pruned only beside the folders a pass works in. One reached
// through a link into a project lies in the project, and stays.
func TestAgentCyclePrunesOnlyBesideFoldersItWorksIn(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	c := loaded(t, config)
	project := filepath.Join(c.Home, "work", "app")
	c.Projects = []string{project}
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	mkdirs(t, c.Home, "work/app/.claude/skills", ".agents/skills")
	writeSkill(t, filepath.Join(project, ".claude", ".reet-backups", "2020-01-01", ".claude-old"))
	symlink(t, filepath.Join(project, ".claude"), filepath.Join(c.Home, ".claude"))
	ours := filepath.Join(c.Home, ".agents", ".reet-backups", "2020-01-01")
	writeSkill(t, filepath.Join(ours, ".agents-old"))
	before := homeTree(t, project)
	srv.setDesired(&store.DesiredState{Version: "v1"})
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	sameTree(t, "project", before, homeTree(t, project))
	if _, err := os.Lstat(ours); !os.IsNotExist(err) {
		t.Fatalf("an expired backup beside a folder reet works in was kept: %v", err)
	}
}

// Every copy reet backs up and replaces that was not its own reaches the
// controller with its backup, though the report that first carried them was
// never delivered: each case variant under its own name, and a copy reet
// cannot read as a skill too.
func TestAgentCycleReportsEveryCopyThatWasNotReets(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	home := loaded(t, config).Home
	shared := filepath.Join(home, ".agents", "skills")
	mkdirs(t, home, ".agents/skills")
	if !caseSensitive(t, shared) {
		t.Skip("this file system cannot hold names differing only in case")
	}
	placeFiles(t, filepath.Join(shared, "ALPHA"), []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("first edit")}})
	placeFiles(t, filepath.Join(shared, "Alpha"), []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("second edit")}})
	// A link in the shared folder is never followed, so reet cannot read it.
	theirs := filepath.Join(home, "dev", "alpha")
	writeSkill(t, theirs)
	symlink(t, theirs, filepath.Join(shared, "alpha"))
	a, aFiles := skillBundle(t, "alpha", "first")
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}})
	srv.mu.Lock()
	srv.reportStatus = http.StatusServiceUnavailable
	srv.mu.Unlock()
	if err := skillCycle(t, config); err == nil {
		t.Fatal("a refused report was not an error")
	}
	// The agent restarts, and the controller takes reports again.
	skillReportsSent.Lock()
	delete(skillReportsSent.last, config)
	skillReportsSent.Unlock()
	srv.mu.Lock()
	srv.reportStatus = 0
	srv.mu.Unlock()
	if err := skillCycle(t, config); err != nil {
		t.Fatal(err)
	}
	backups := "~/.agents/.reet-backups/" + time.Now().Format(backupDateLayout) + "/"
	var got []string
	for _, e := range srv.lastReport(t).Entries {
		if e.State == "edited" {
			got = append(got, e.Name+" "+e.Backup+" "+e.Detail)
		}
	}
	want := []string{
		"ALPHA " + backups + ".agents-ALPHA ",
		"Alpha " + backups + ".agents-Alpha ",
		"alpha " + backups + ".agents-alpha reet could not read this copy",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("edited entries\n%q\nwant\n%q", got, want)
	}
	at := func(name string) string {
		return filepath.Join(home, strings.TrimPrefix(backups, "~/"), ".agents-"+name)
	}
	for name, body := range map[string]string{"ALPHA": "first edit", "Alpha": "second edit"} {
		if b, err := os.ReadFile(filepath.Join(at(name), "SKILL.md")); err != nil || string(b) != body {
			t.Fatalf("backup of %s: %q %v", name, b, err)
		}
	}
	if target, err := os.Readlink(at("alpha")); err != nil || target != theirs {
		t.Fatalf("backup of the link: %q %v", target, err)
	}
	if _, err := os.Lstat(config + ".unsent"); !os.IsNotExist(err) {
		t.Fatal("delivered entries were kept")
	}
}

// Copies put back after an install fails were never replaced, so the edits
// passed on for them are taken back, every one.
func TestAgentCycleTakesBackEveryEditWhoseCopyWentBack(t *testing.T) {
	srv := newSkillController(t)
	config := enrolledMachine(t, srv.URL)
	shared := filepath.Join(loaded(t, config).Home, ".agents", "skills")
	mkdirs(t, loaded(t, config).Home, ".agents/skills")
	if !caseSensitive(t, shared) {
		t.Skip("this file system cannot hold names differing only in case")
	}
	placeFiles(t, filepath.Join(shared, "ALPHA"), []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("first edit")}})
	placeFiles(t, filepath.Join(shared, "Alpha"), []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("second edit")}})
	before := homeTree(t, shared)
	was := reconcileMove
	t.Cleanup(func() { reconcileMove = was })
	reconcileMove = func(string, string) (bool, error) { return false, errors.New("no space left on device") }
	a, aFiles := skillBundle(t, "alpha", "first")
	srv.serve(t, a, aFiles)
	srv.setDesired(&store.DesiredState{Version: "v1", Active: []store.DesiredSkill{a}})
	skillCycle(t, config)
	sameTree(t, "skills", withoutFolderTimes(before), withoutFolderTimes(homeTree(t, shared)))
	var got []string
	for _, e := range srv.lastReport(t).Entries {
		got = append(got, e.Name+" "+e.State+" "+e.Backup)
	}
	if strings.Join(got, "|") != "alpha failed " {
		t.Fatalf("entries %q", got)
	}
	if _, err := os.Lstat(config + ".unsent"); !os.IsNotExist(err) {
		t.Fatal("edits taken back were kept")
	}
}
