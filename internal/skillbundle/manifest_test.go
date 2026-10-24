package skillbundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	if !supportedModes {
		t.Skip("local executable-mode observation is only supported on Linux/macOS")
	}
	root := t.TempDir()
	for name, data := range files {
		write(t, root, name, data)
	}
	return root
}
func write(t *testing.T, root, name, data string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
func observe(t *testing.T, root string) Manifest {
	t.Helper()
	m, err := Scan(root, Limits{})
	if err != nil {
		t.Fatalf("observation failed: %+v %v", m.Issues, err)
	}
	if !validManifest(&m) {
		t.Fatalf("scanner produced a non-comparable manifest: %+v", m)
	}
	return m
}
func assertIncomplete(t *testing.T, m Manifest, err error, code string) {
	t.Helper()
	if !errors.Is(err, ErrIncomplete) || m.Complete || m.Digest != "" {
		t.Fatalf("incomplete data accepted: %+v %v", m, err)
	}
	found := false
	for _, issue := range m.Issues {
		if issue.Code == code {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing %s: %+v", code, m.Issues)
	}
	if Compare(&m, &m).State != "unknown" || Compare(nil, &m).State != "unknown" {
		t.Fatal("incomplete data compared as known")
	}
}

func TestDeterministicManifestIsReadOnlyAndDoesNotExposeContents(t *testing.T) {
	const secret = "SYNTHETIC-PRIVATE-CONTENT"
	first := fixture(t, map[string]string{"SKILL.md": secret, "scripts/run.sh": "#!/bin/sh\necho test\n", "empty": ""})
	second := fixture(t, map[string]string{"empty": "", "scripts/run.sh": "#!/bin/sh\necho test\n", "SKILL.md": secret})
	for _, root := range []string{first, second} {
		if err := os.Chmod(filepath.Join(root, "scripts/run.sh"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a, b := observe(t, first), observe(t, second)
	if !reflect.DeepEqual(a, b) || Compare(&a, &b).State != "equal" {
		t.Fatal("order/location changed bundle identity")
	}
	if len(a.Files) != 3 || a.TotalBytes != int64(len(secret)+len("#!/bin/sh\necho test\n")) {
		t.Fatalf("wrong totals: %+v", a)
	}
	for _, file := range a.Files {
		data, err := os.ReadFile(filepath.Join(first, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if file.Digest != hex.EncodeToString(sum[:]) {
			t.Fatal("not an exact-byte digest")
		}
		if file.Executable != (file.Path == "scripts/run.sh") {
			t.Fatal("executable bit lost")
		}
	}
	encoded, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(first)) || bytes.Contains(encoded, []byte(secret)) || bytes.Contains(encoded, []byte("#!/bin/sh")) {
		t.Fatal("manifest exposed root/content")
	}
	again := observe(t, first)
	if !reflect.DeepEqual(a, again) {
		t.Fatal("observation changed source")
	}
	if info, err := os.Stat(filepath.Join(first, "scripts/run.sh")); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatal("source mode changed", err)
	}
}

func TestSameSourceEditsScriptsModesAndLineEndingsChangeIdentity(t *testing.T) {
	root := fixture(t, map[string]string{"SKILL.md": "same upstream\n", "scripts/run.sh": "echo old\n"})
	before := observe(t, root)
	for _, tc := range []struct {
		name, file, value string
		mode              os.FileMode
	}{
		{name: "instructions", file: "SKILL.md", value: "same upstream but local edit\n"},
		{name: "supporting script", file: "scripts/run.sh", value: "echo new\n"},
		{name: "line endings", file: "SKILL.md", value: "same upstream\r\n"},
		{name: "executable only", file: "scripts/run.sh", mode: 0o700},
	} {
		t.Run(tc.name, func(t *testing.T) {
			write(t, root, "SKILL.md", "same upstream\n")
			write(t, root, "scripts/run.sh", "echo old\n")
			if err := os.Chmod(filepath.Join(root, "scripts/run.sh"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.mode != 0 {
				if err := os.Chmod(filepath.Join(root, tc.file), tc.mode); err != nil {
					t.Fatal(err)
				}
			} else {
				write(t, root, tc.file, tc.value)
			}
			after := observe(t, root)
			diff := Compare(&before, &after)
			if after.Digest == before.Digest || diff.State != "changed" || len(diff.Changes) != 1 || diff.Changes[0].Path != tc.file || diff.Changes[0].Kind != "modified" {
				t.Fatalf("local change missed: %+v", diff)
			}
		})
	}
}

func TestDiffIsDirectionalAndRenamesAreRemovePlusAdd(t *testing.T) {
	first := fixture(t, map[string]string{"SKILL.md": "one", "a": "same", "old": "renamed", "remove": "gone"})
	second := fixture(t, map[string]string{"SKILL.md": "two", "a": "same", "new": "renamed", "add": "added"})
	a, b := observe(t, first), observe(t, second)
	diff := Compare(&a, &b)
	var changes []string
	for _, change := range diff.Changes {
		changes = append(changes, change.Path+":"+change.Kind)
	}
	want := []string{"SKILL.md:modified", "add:added", "new:added", "old:removed", "remove:removed"}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("got %v want %v", changes, want)
	}
	reverse := Compare(&b, &a)
	if reverse.Changes[1].Kind != "removed" {
		t.Fatal("diff direction lost")
	}
	original := a.Files[0]
	diff.Changes[0].Before.Bytes++
	if a.Files[0] != original {
		t.Fatal("diff aliases its input")
	}
}

func TestMissingUnsafeAndUnreadableTreesRemainUnknown(t *testing.T) {
	t.Run("missing root", func(t *testing.T) {
		root := fixture(t, map[string]string{"SKILL.md": "x"})
		m, err := Scan(filepath.Join(root, "missing"), Limits{})
		assertIncomplete(t, m, err, "root_missing")
	})
	t.Run("empty directory", func(t *testing.T) {
		m, err := Scan(fixture(t, nil), Limits{})
		assertIncomplete(t, m, err, "skill_manifest_missing")
	})
	t.Run("empty instructions are bytes not an omission", func(t *testing.T) {
		m := observe(t, fixture(t, map[string]string{"SKILL.md": ""}))
		if len(m.Files) != 1 || m.TotalBytes != 0 {
			t.Fatal("empty file omitted")
		}
	})
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "unreadable file", true: "unreadable directory"}[directory], func(t *testing.T) {
			root := fixture(t, map[string]string{"SKILL.md": "x", "hidden/private": "secret"})
			target := filepath.Join(root, "SKILL.md")
			if directory {
				target = filepath.Join(root, "hidden")
			}
			if err := os.Chmod(target, 0); err != nil {
				t.Fatal(err)
			}
			defer os.Chmod(target, 0o700)
			if f, err := os.Open(target); err == nil {
				f.Close()
				t.Skip("current OS user can read despite permission bits")
			}
			m, err := Scan(root, Limits{})
			code := "file_unreadable"
			if directory {
				code = "directory_unreadable"
			}
			assertIncomplete(t, m, err, code)
		})
	}
	for _, name := range []string{"CON", "bad\\name", "bad:name", "trailing.", "control\x01", "direction\u202e"} {
		t.Run("unsafe "+name, func(t *testing.T) {
			root := fixture(t, map[string]string{"SKILL.md": "x", name: "y"})
			m, err := Scan(root, Limits{})
			assertIncomplete(t, m, err, "unsafe_path")
			encoded, _ := json.Marshal(m)
			if bytes.Contains(encoded, []byte(root)) {
				t.Fatal("diagnostic leaked absolute root")
			}
		})
	}
	t.Run("case-fold collision", func(t *testing.T) {
		root := fixture(t, map[string]string{"SKILL.md": "x", "Readme": "one", "README": "two"})
		entries, _ := os.ReadDir(root)
		if len(entries) != 3 {
			t.Skip("filesystem itself is case insensitive")
		}
		m, err := Scan(root, Limits{})
		assertIncomplete(t, m, err, "case_collision")
	})
	t.Run("case-fold directory collision", func(t *testing.T) {
		root := fixture(t, map[string]string{"SKILL.md": "x", "Tools/a": "a", "tools/b": "b"})
		entries, _ := os.ReadDir(root)
		if len(entries) != 3 {
			t.Skip("filesystem itself is case insensitive")
		}
		m, err := Scan(root, Limits{})
		assertIncomplete(t, m, err, "case_collision")
	})
}

func TestTraversalAndByteBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		files  map[string]string
		limits Limits
		code   string
	}{
		{"files", map[string]string{"SKILL.md": "", "a": "", "b": ""}, Limits{Files: 2}, "file_limit"},
		{"one file", map[string]string{"SKILL.md": "1234"}, Limits{FileBytes: 3}, "byte_limit"},
		{"total", map[string]string{"SKILL.md": "12", "a": "12"}, Limits{TotalBytes: 3}, "byte_limit"},
		{"entries", map[string]string{"SKILL.md": "", "a/one": "", "b/two": ""}, Limits{Entries: 3}, "entry_limit"},
		{"depth", map[string]string{"SKILL.md": "", "one/two/leaf": ""}, Limits{Depth: 1}, "depth_limit"},
		{"path bytes", map[string]string{"SKILL.md": "", "longer-name": ""}, Limits{PathBytes: 8}, "unsafe_path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Scan(fixture(t, tc.files), tc.limits)
			assertIncomplete(t, m, err, tc.code)
		})
	}
	t.Run("empty directories consume entries", func(t *testing.T) {
		root := fixture(t, map[string]string{"SKILL.md": ""})
		for _, name := range []string{"a", "b", "c"} {
			if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		m, err := Scan(root, Limits{Entries: 3})
		assertIncomplete(t, m, err, "entry_limit")
	})
	for _, limits := range []Limits{{Files: -1}, {Files: MaxFiles + 1}, {FileBytes: MaxBytes + 1}, {TotalBytes: -1}, {Entries: -1}, {Depth: MaxDepth + 1}, {PathBytes: MaxPathBytes + 1}} {
		m, err := Scan("never-opened", limits)
		if !errors.Is(err, ErrLimits) || m.Complete || m.Digest != "" {
			t.Fatalf("bad limits accepted: %+v %v", limits, err)
		}
	}
}

func TestSymlinkRootsFilesDirectoriesAndDanglingLinksAreIncomplete(t *testing.T) {
	for _, kind := range []string{"root", "file", "directory", "dangling"} {
		t.Run(kind, func(t *testing.T) {
			root := fixture(t, map[string]string{"SKILL.md": "local"})
			outside := fixture(t, map[string]string{"SKILL.md": "outside"})
			link := filepath.Join(root, "link")
			target := filepath.Join(outside, "SKILL.md")
			switch kind {
			case "root":
				link = filepath.Join(t.TempDir(), "link")
				target = root
			case "directory":
				target = outside
			case "dangling":
				target = filepath.Join(outside, "absent")
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skip(err)
			}
			if kind == "root" {
				root = link
			}
			m, err := Scan(root, Limits{})
			code := "symlink"
			if kind == "root" {
				code = "root_not_regular_directory"
			}
			assertIncomplete(t, m, err, code)
		})
	}
}

func TestReadTimeChangesAndLaterEditsInvalidateWholeObservation(t *testing.T) {
	for _, kind := range []string{"current file", "earlier file", "membership", "directory replacement", "root replacement"} {
		t.Run(kind, func(t *testing.T) {
			root := fixture(t, map[string]string{"SKILL.md": "original", "scripts/run.sh": "echo original"})
			fired := false
			m, err := scan(root, Limits{}, func(name string) {
				if fired {
					return
				}
				if kind == "earlier file" && name != "scripts/run.sh" {
					return
				}
				fired = true
				switch kind {
				case "current file":
					write(t, root, name, "changed and different length")
				case "earlier file":
					write(t, root, "SKILL.md", "changed and different length")
				case "membership":
					write(t, root, "added", "new")
				case "directory replacement":
					if err := os.Rename(filepath.Join(root, "scripts"), filepath.Join(root, "old-scripts")); err != nil {
						t.Fatal(err)
					}
					write(t, root, "scripts/new", "new")
				case "root replacement":
					if err := os.Rename(root, root+"-moved"); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { os.RemoveAll(root + "-moved") })
					if err := os.Mkdir(root, 0o700); err != nil {
						t.Fatal(err)
					}
				}
			})
			if !fired {
				t.Fatal("race fixture not reached")
			}
			if !errors.Is(err, ErrIncomplete) || m.Complete || m.Digest != "" || Compare(&m, &m).State != "unknown" {
				t.Fatalf("changing tree accepted: %+v %v", m, err)
			}
		})
	}
}

func TestMalformedOrIncompatibleManifestsAreUnknown(t *testing.T) {
	original := observe(t, fixture(t, map[string]string{"SKILL.md": "x", "a": "y"}))
	tests := []struct {
		name string
		edit func(*Manifest)
	}{
		{"incomplete", func(m *Manifest) { m.Complete = false }},
		{"unknown schema", func(m *Manifest) { m.SchemaVersion++ }},
		{"unknown hash", func(m *Manifest) { m.Algorithm = "unknown" }},
		{"unsupported modes", func(m *Manifest) { m.ModeSemantics = "unknown" }},
		{"unreadable", func(m *Manifest) { m.Issues = []Issue{{Path: "a", Code: "file_unreadable"}} }},
		{"empty", func(m *Manifest) { m.Files = nil }},
		{"no instructions", func(m *Manifest) { m.Files = m.Files[1:] }},
		{"bad total", func(m *Manifest) { m.TotalBytes++ }},
		{"bad tree hash", func(m *Manifest) { m.Digest = strings.Repeat("0", 64) }},
		{"file hash", func(m *Manifest) { m.Files[0].Digest = "not-a-digest" }},
		{"negative bytes", func(m *Manifest) { m.Files[0].Bytes = -1 }},
		{"oversized", func(m *Manifest) { m.Files[0].Bytes = MaxBytes + 1 }},
		{"absolute", func(m *Manifest) { m.Files[0].Path = "/SKILL.md" }},
		{"traversal", func(m *Manifest) { m.Files[0].Path = "../SKILL.md" }},
		{"duplicate", func(m *Manifest) { m.Files = append(m.Files, m.Files[len(m.Files)-1]) }},
		{"out of order", func(m *Manifest) { m.Files[0], m.Files[1] = m.Files[1], m.Files[0] }},
		{"file and directory alias", func(m *Manifest) { m.Files[1].Path = "skill.MD/child" }},
		{"directory case aliases", func(m *Manifest) {
			a, b := m.Files[1], m.Files[1]
			a.Path = "tools/a"
			b.Path = "Tools/b"
			m.Files = append(m.Files[:1], b, a)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			changed := original
			changed.Files = append([]File(nil), original.Files...)
			tc.edit(&changed)
			// A self-consistent hash cannot make invalid metadata or incompatible schemas comparable.
			if tc.name != "bad tree hash" {
				changed.Digest = manifestDigest(changed)
			}
			if Compare(&original, &changed).State != "unknown" {
				t.Fatalf("invalid input accepted: %+v", changed)
			}
		})
	}
	if Compare(nil, &original).State != "unknown" || Compare(&original, nil).State != "unknown" {
		t.Fatal("missing baseline treated as empty")
	}
}

func TestVersionOneCanonicalDigestAndPartialReadBudget(t *testing.T) {
	t.Run("golden digest", func(t *testing.T) {
		m := observe(t, fixture(t, map[string]string{"SKILL.md": "x"}))
		const want = "dd0ffe9c59919dd4b7f9669a57a34820eb2d649010f9d1df4187def0f74a60b8"
		if m.Digest != want {
			t.Fatalf("v1 canonical format changed: %s", m.Digest)
		}
	})
	t.Run("failed reads consume aggregate budget", func(t *testing.T) {
		root := fixture(t, map[string]string{"SKILL.md": "1234", "a": "1234", "b": "1234"})
		reads := 0
		m, err := scan(root, Limits{TotalBytes: 4}, func(name string) { reads++; write(t, root, name, "changed") })
		assertIncomplete(t, m, err, "byte_limit")
		if reads != 1 {
			t.Fatalf("aggregate bound ignored failed read bytes: %d reads", reads)
		}
	})
}

func TestRelativePathContractAndZeroBudgetEmptyFiles(t *testing.T) {
	for _, name := range []string{"../a", "/a", "a//b", "a/./b", "a/../b", "a\\b", "a:b", "NUL.txt", "COM1.log", "a?b", "a*b", "a|b", "a<b", "a>b", "a\"b", "line\u2028break", "line\u2029break", string([]byte{0xff})} {
		if validPath(name, MaxPathBytes) {
			t.Fatalf("unsafe path accepted: %q", name)
		}
	}
	for _, name := range []string{"SKILL.md", "scripts/run.sh", "a valid name.txt", "references/café.md"} {
		if !validPath(name, MaxPathBytes) {
			t.Fatalf("safe path refused: %q", name)
		}
	}
	root := fixture(t, map[string]string{"SKILL.md": "x", "empty": ""})
	m, err := Scan(root, Limits{TotalBytes: 1})
	if err != nil || !m.Complete || len(m.Files) != 2 || m.TotalBytes != 1 {
		t.Fatalf("empty file after exact budget refused: %+v %v", m, err)
	}
}

func TestIncompleteTraversalDoesNotClaimInstructionsAreAbsent(t *testing.T) {
	root := fixture(t, map[string]string{"SKILL.md": "exists", "a": "x"})
	m, err := Scan(root, Limits{Entries: 1})
	assertIncomplete(t, m, err, "entry_limit")
	for _, issue := range m.Issues {
		if issue.Code == "skill_manifest_missing" {
			t.Fatal("incomplete enumeration claimed a confirmed absence")
		}
	}
}
