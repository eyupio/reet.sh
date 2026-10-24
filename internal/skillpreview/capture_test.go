//go:build linux || darwin

package skillpreview

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(data), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
}

func scopeRoot(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".agents", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustValid(t *testing.T, c Capture) Capture {
	t.Helper()
	if err := c.Validate(); err != nil {
		t.Fatalf("invalid capture: %v\n%+v", err, c)
	}
	return c
}

func issueCodes(c Capture) []string {
	out := []string{}
	for _, i := range c.Issues {
		out = append(out, i.Code)
	}
	return out
}

func TestObserveEstablishesAbsenceOnlyInsideAVerifiedExclusiveRoot(t *testing.T) {
	root := scopeRoot(t)
	writeTree(t, root, map[string]string{"someone-else/SKILL.md": "unrelated"})
	c := mustValid(t, Observe(Roots{Target: root, OthersComplete: true}, "demo"))
	if c.Presence != "absent" || !c.Complete || c.RootOwnership != "exclusive" || c.Manifest != nil || len(c.RootID) != 64 {
		t.Fatalf("%+v", c)
	}
	if _, err := os.Stat(filepath.Join(root, "demo")); !os.IsNotExist(err) {
		t.Fatal("observation created the skill folder")
	}
}

func TestObserveReportsEveryFilePreservingUnmanagedOnesWithoutContents(t *testing.T) {
	root := scopeRoot(t)
	writeTree(t, root, map[string]string{
		"demo/SKILL.md":       "---\nname: demo\n---\nSECRET_BODY_TEXT\n",
		"demo/run.sh":         "#!/bin/sh\necho SECRET_SCRIPT\n",
		"demo/notes/mine.txt": "my own unmanaged note",
	})
	c := mustValid(t, Observe(Roots{Target: root, OthersComplete: true}, "demo"))
	if c.Presence != "present" || !c.Complete || c.Manifest == nil || len(c.Manifest.Files) != 3 {
		t.Fatalf("%+v", c)
	}
	want, err := skillbundle.Scan(filepath.Join(root, "demo"), skillbundle.Limits{})
	if err != nil || c.Manifest.Digest != want.Digest {
		t.Fatalf("not the S1 identity: %v %v", err, c.Manifest)
	}
	byPath := map[string]skillbundle.File{}
	for _, f := range c.Manifest.Files {
		byPath[f.Path] = f
	}
	if !byPath["run.sh"].Executable || byPath["SKILL.md"].Executable || byPath["notes/mine.txt"].Bytes == 0 {
		t.Fatalf("mode or size identity lost: %+v", byPath)
	}
	raw, _ := json.Marshal(c)
	for _, leaked := range []string{"SECRET_BODY_TEXT", "SECRET_SCRIPT", "my own unmanaged note", root} {
		if strings.Contains(string(raw), leaked) {
			t.Fatalf("capture leaked %q", leaked)
		}
	}
}

func TestObserveRootIdentityIsStablePerDirectoryAndDistinctBetweenThem(t *testing.T) {
	a, b := scopeRoot(t), scopeRoot(t)
	first := Observe(Roots{Target: a, OthersComplete: true}, "demo")
	second := Observe(Roots{Target: a, OthersComplete: true}, "demo")
	other := Observe(Roots{Target: b, OthersComplete: true}, "demo")
	if first.RootID != second.RootID || first.RootID == other.RootID {
		t.Fatal(first.RootID, second.RootID, other.RootID)
	}
	if strings.Contains(first.RootID, filepath.Base(a)) {
		t.Fatal("root identity exposes a path")
	}
}

func TestObserveMissingScopeRootIsUnknownNotAbsent(t *testing.T) {
	c := mustValid(t, Observe(Roots{Target: filepath.Join(t.TempDir(), "never-created"), OthersComplete: true}, "demo"))
	if c.Presence != "unknown" || c.RootOwnership != "unknown" || c.RootID != Unavailable || c.Complete {
		t.Fatalf("%+v", c)
	}
	if got := issueCodes(c); len(got) != 1 || got[0] != "scope_root_missing" {
		t.Fatal(got)
	}
}

func TestObserveSymlinkedScopeRootIsSharedAndNeverRead(t *testing.T) {
	real := scopeRoot(t)
	writeTree(t, real, map[string]string{"demo/SKILL.md": "---\nname: demo\n---\n"})
	link := filepath.Join(t.TempDir(), "skills-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	c := mustValid(t, Observe(Roots{Target: link, OthersComplete: true}, "demo"))
	if c.RootOwnership != "shared" || c.Presence != "unknown" || c.Manifest != nil {
		t.Fatalf("%+v", c)
	}
	if got := issueCodes(c); len(got) != 1 || got[0] != "scope_root_symlink" {
		t.Fatal(got)
	}
}

func TestObserveAnotherScopeResolvingToTheSameDirectoryIsSharedAndNeverRead(t *testing.T) {
	real := scopeRoot(t)
	writeTree(t, real, map[string]string{"demo/SKILL.md": "---\nname: demo\n---\n"})
	alias := filepath.Join(t.TempDir(), "claude-skills")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	c := mustValid(t, Observe(Roots{Target: real, Others: []string{filepath.Join(t.TempDir(), "absent-scope"), alias}, OthersComplete: true}, "demo"))
	if c.RootOwnership != "shared" || c.Presence != "unknown" || c.Manifest != nil || c.Complete {
		t.Fatalf("%+v", c)
	}
	if got := issueCodes(c); len(got) != 1 || got[0] != "root_aliased" {
		t.Fatal(got)
	}
}

func TestObserveOtherMissingScopesDoNotMakeARootShared(t *testing.T) {
	root := scopeRoot(t)
	c := mustValid(t, Observe(Roots{Target: root, Others: []string{filepath.Join(t.TempDir(), "x"), filepath.Join(t.TempDir(), "y")}, OthersComplete: true}, "demo"))
	if c.RootOwnership != "exclusive" || c.Presence != "absent" {
		t.Fatalf("%+v", c)
	}
}

func TestObserveCannotProveExclusivityFromATruncatedAliasSet(t *testing.T) {
	root := scopeRoot(t)
	writeTree(t, root, map[string]string{"demo/SKILL.md": "---\nname: demo\n---\n"})
	c := mustValid(t, Observe(Roots{Target: root, OthersComplete: false}, "demo"))
	if c.RootOwnership != "unknown" || c.Presence != "unknown" || c.Manifest != nil {
		t.Fatalf("%+v", c)
	}
	if got := issueCodes(c); len(got) != 1 || got[0] != "alias_check_incomplete" {
		t.Fatal(got)
	}
}

func TestObserveSymlinkedSkillFolderIsUnknownEvenWhenItsTargetIsAValidSkill(t *testing.T) {
	root := scopeRoot(t)
	elsewhere := t.TempDir()
	writeTree(t, elsewhere, map[string]string{"SKILL.md": "---\nname: demo\n---\n"})
	if err := os.Symlink(elsewhere, filepath.Join(root, "demo")); err != nil {
		t.Fatal(err)
	}
	c := mustValid(t, Observe(Roots{Target: root, OthersComplete: true}, "demo"))
	if c.Presence != "unknown" || c.Manifest != nil || c.RootOwnership != "exclusive" {
		t.Fatalf("%+v", c)
	}
	if got := issueCodes(c); len(got) != 1 || got[0] != "skill_symlink" {
		t.Fatal(got)
	}
}

func TestObserveSkillPathThatIsAFileIsUnknown(t *testing.T) {
	root := scopeRoot(t)
	writeTree(t, root, map[string]string{"demo": "a file where a folder belongs"})
	c := mustValid(t, Observe(Roots{Target: root, OthersComplete: true}, "demo"))
	got := issueCodes(c)
	if c.Presence != "unknown" || len(got) != 1 || got[0] != "skill_not_directory" {
		t.Fatalf("%+v", c)
	}
}

func TestObserveUnsafeEntryInsideTheSkillMakesTheWholeCaptureUnknown(t *testing.T) {
	root := scopeRoot(t)
	writeTree(t, root, map[string]string{"demo/SKILL.md": "---\nname: demo\n---\n"})
	if err := os.Symlink("/etc/hostname", filepath.Join(root, "demo", "link")); err != nil {
		t.Fatal(err)
	}
	c := mustValid(t, Observe(Roots{Target: root, OthersComplete: true}, "demo"))
	if c.Presence != "unknown" || c.Complete || c.Manifest != nil {
		t.Fatalf("a partial tree must not look like a manifest: %+v", c)
	}
	found := false
	for _, i := range c.Issues {
		found = found || (i.Path == "link" && i.Code == "symlink")
	}
	if !found {
		t.Fatal(c.Issues)
	}
}

func TestObserveBoundsRejectedFilesNotSilentlyDropped(t *testing.T) {
	root := scopeRoot(t)
	files := map[string]string{"demo/SKILL.md": "---\nname: demo\n---\n"}
	for i := 0; i <= skillbundle.MaxFiles; i++ {
		files[fmt.Sprintf("demo/f%03d.txt", i)] = "x"
	}
	if len(files) <= skillbundle.MaxFiles {
		t.Fatal("fixture does not exceed the file limit", len(files))
	}
	writeTree(t, root, files)
	c := mustValid(t, Observe(Roots{Target: root, OthersComplete: true}, "demo"))
	if c.Presence != "unknown" || c.Manifest != nil {
		t.Fatalf("over-limit tree reported as complete: presence=%s files=%v", c.Presence, c.Manifest != nil)
	}
	if len(c.Issues) == 0 || len(c.Issues) > MaxIssues {
		t.Fatal("issues unbounded or missing", len(c.Issues))
	}
}

func TestObserveRefusesNamesThatCouldLeaveTheScopeRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "skills")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTree(t, parent, map[string]string{"sibling/SKILL.md": "---\nname: sibling\n---\n"})
	for _, name := range []string{"../sibling", "..", ".", "", "a/b", "Upper", "has space", strings.Repeat("a", 65), "-lead", "trail-", "dou--ble", "nul\x00byte"} {
		c := Observe(Roots{Target: root, OthersComplete: true}, name)
		if c.Presence != "unknown" || c.Manifest != nil || c.RootID != Unavailable {
			t.Fatalf("%q: %+v", name, c)
		}
		if mustValid(t, c); len(c.Issues) != 1 || c.Issues[0].Code != "invalid_skill_name" {
			t.Fatalf("%q: %v", name, c.Issues)
		}
	}
}

func TestObserveLeavesTheFilesystemExactlyAsFound(t *testing.T) {
	root := scopeRoot(t)
	writeTree(t, root, map[string]string{"demo/SKILL.md": "---\nname: demo\n---\n", "demo/run.sh": "#!/bin/sh\n", "other/SKILL.md": "x"})
	before := snapshot(t, root)
	Observe(Roots{Target: root, OthersComplete: true}, "demo")
	Observe(Roots{Target: root, OthersComplete: true}, "missing")
	if after := snapshot(t, root); after != before {
		t.Fatalf("observation changed the tree:\n%s\n---\n%s", before, after)
	}
}

func snapshot(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		b.WriteString(rel + " " + info.Mode().String() + " " + info.ModTime().UTC().String() + "\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestObserveDetectsTheScopeRootBeingReplacedWhileCapturing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
	}{
		{"present skill", map[string]string{"demo/SKILL.md": "---\nname: demo\n---\n"}},
		{"absent skill", map[string]string{"other/SKILL.md": "x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := scopeRoot(t)
			writeTree(t, root, tc.files)
			c := observe(Roots{Target: root, OthersComplete: true}, "demo", func() {
				// A different directory now answers to the same path.
				if err := os.Rename(root, root+".moved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0o755); err != nil {
					t.Fatal(err)
				}
			})
			mustValid(t, c)
			if got := issueCodes(c); c.Presence != "unknown" || c.Manifest != nil || len(got) != 1 || got[0] != "scope_root_changed" {
				t.Fatalf("%+v", c)
			}
		})
	}
}
