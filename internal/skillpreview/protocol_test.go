package skillpreview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

func demoManifest(t *testing.T, files ...skillbundle.ContentFile) skillbundle.Manifest {
	t.Helper()
	m, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func presentCapture(t *testing.T) Capture {
	t.Helper()
	m := demoManifest(t, skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("---\nname: demo\n---\n")}, skillbundle.ContentFile{Path: "run.sh", Data: []byte("#!/bin/sh\n"), Executable: true})
	return Capture{SchemaVersion: Version, Adapter: Adapter, RootID: strings.Repeat("a", 64), RootOwnership: "exclusive", Presence: "present", Complete: true, Manifest: &m, Issues: []Issue{}}
}

func TestCaptureValidateAcceptsOnlyTruthfulStates(t *testing.T) {
	absent := Capture{SchemaVersion: Version, Adapter: Adapter, RootID: strings.Repeat("b", 64), RootOwnership: "exclusive", Presence: "absent", Complete: true, Issues: []Issue{}}
	unknown := Capture{SchemaVersion: Version, Adapter: Adapter, RootID: Unavailable, RootOwnership: "unknown", Presence: "unknown", Issues: []Issue{{Path: ".", Code: "scope_root_missing"}}}
	for name, c := range map[string]Capture{"present": presentCapture(t), "absent": absent, "unknown": unknown} {
		if err := c.Validate(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	cases := map[string]func(*Capture){
		"future schema":              func(c *Capture) { c.SchemaVersion = 2 },
		"other adapter":              func(c *Capture) { c.Adapter = "windows-acl-v1" },
		"unknown ownership word":     func(c *Capture) { c.RootOwnership = "mine" },
		"unknown presence word":      func(c *Capture) { c.Presence = "maybe" },
		"short root id":              func(c *Capture) { c.RootID = "abc" },
		"uppercase root id":          func(c *Capture) { c.RootID = strings.Repeat("A", 64) },
		"present without manifest":   func(c *Capture) { c.Manifest = nil },
		"present but incomplete":     func(c *Capture) { c.Complete = false },
		"present with issues":        func(c *Capture) { c.Issues = []Issue{{Path: ".", Code: "entry_changed"}} },
		"present with forged digest": func(c *Capture) { c.Manifest.Digest = strings.Repeat("0", 64) },
		"present with unsafe path":   func(c *Capture) { c.Manifest.Files[0].Path = "../escape" },
		"nil issue list":             func(c *Capture) { c.Issues = nil },
		"too many issues": func(c *Capture) {
			c.Presence, c.Complete, c.Manifest = "unknown", false, nil
			c.Issues = make([]Issue, MaxIssues+1)
			for i := range c.Issues {
				c.Issues[i] = Issue{Path: ".", Code: "symlink"}
			}
		},
		"unknown without explanation": func(c *Capture) { c.Presence, c.Complete, c.Manifest, c.Issues = "unknown", false, nil, []Issue{} },
		"unknown claiming complete": func(c *Capture) {
			c.Presence, c.Complete, c.Manifest, c.Issues = "unknown", true, nil, []Issue{{Path: ".", Code: "symlink"}}
		},
		"unknown carrying a manifest": func(c *Capture) {
			c.Presence, c.Complete, c.Issues = "unknown", false, []Issue{{Path: ".", Code: "symlink"}}
		},
		"bad issue code": func(c *Capture) {
			c.Presence, c.Complete, c.Manifest, c.Issues = "unknown", false, nil, []Issue{{Path: ".", Code: "Symlink!"}}
		},
		"control character in path": func(c *Capture) {
			c.Presence, c.Complete, c.Manifest, c.Issues = "unknown", false, nil, []Issue{{Path: "a\nb", Code: "symlink"}}
		},
		"oversized issue path": func(c *Capture) {
			c.Presence, c.Complete, c.Manifest, c.Issues = "unknown", false, nil, []Issue{{Path: strings.Repeat("a", 1025), Code: "symlink"}}
		},
		"unavailable root but exclusive": func(c *Capture) {
			c.RootID, c.Presence, c.Complete, c.Manifest, c.Issues = Unavailable, "unknown", false, nil, []Issue{{Path: ".", Code: "scope_root_missing"}}
		},
		"unavailable root claiming presence": func(c *Capture) { c.RootID, c.RootOwnership = Unavailable, "unknown" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := presentCapture(t)
			mutate(&c)
			if err := c.Validate(); err == nil {
				t.Fatalf("accepted %+v", c)
			}
		})
	}
	t.Run("absent carrying files", func(t *testing.T) {
		c := absent
		m := demoManifest(t, skillbundle.ContentFile{Path: "SKILL.md", Data: []byte("x")})
		c.Manifest = &m
		if c.Validate() == nil {
			t.Fatal("absence with a manifest")
		}
	})
	t.Run("absent but incomplete", func(t *testing.T) {
		c := absent
		c.Complete = false
		if c.Validate() == nil {
			t.Fatal("absence without a complete check")
		}
	})
}

func TestCaptureHashIsCanonicalAndStateSensitive(t *testing.T) {
	a := presentCapture(t)
	b := presentCapture(t)
	b.Manifest.Files[0], b.Manifest.Files[1] = b.Manifest.Files[1], b.Manifest.Files[0]
	ha, err := a.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hb, err := b.Hash()
	if err != nil || ha != hb {
		t.Fatal("file order changed identity", ha, hb, err)
	}
	b.RootID = strings.Repeat("c", 64)
	if hc, _ := b.Hash(); hc == ha {
		t.Fatal("root identity not bound")
	}
	b = presentCapture(t)
	b.RootOwnership = "shared"
	if hd, _ := b.Hash(); hd == ha {
		t.Fatal("ownership not bound")
	}
	b = presentCapture(t)
	b.Manifest.Files[1].Executable = false
	if _, err = b.Hash(); err == nil {
		t.Fatal("hashed an inconsistent manifest")
	}
}

func TestCaptureValidateEnforcesTheS1ManifestBoundsOnAnUntrustedReport(t *testing.T) {
	over := make([]skillbundle.File, skillbundle.MaxFiles+1)
	for i := range over {
		over[i] = skillbundle.File{Path: fmt.Sprintf("f%03d.txt", i), Bytes: 1, Digest: strings.Repeat("a", 64)}
	}
	for name, files := range map[string][]skillbundle.File{
		"too many files":    over,
		"too many bytes":    {{Path: "SKILL.md", Bytes: skillbundle.MaxBytes + 1, Digest: strings.Repeat("a", 64)}},
		"negative size":     {{Path: "SKILL.md", Bytes: -1, Digest: strings.Repeat("a", 64)}},
		"case collision":    {{Path: "SKILL.md", Bytes: 1, Digest: strings.Repeat("a", 64)}, {Path: "skill.md", Bytes: 1, Digest: strings.Repeat("b", 64)}},
		"deep traversal":    {{Path: "a/../../etc/passwd", Bytes: 1, Digest: strings.Repeat("a", 64)}},
		"overlong path":     {{Path: strings.Repeat("a", skillbundle.MaxPathBytes+1), Bytes: 1, Digest: strings.Repeat("a", 64)}},
		"short file digest": {{Path: "SKILL.md", Bytes: 1, Digest: "abc"}},
	} {
		t.Run(name, func(t *testing.T) {
			c := presentCapture(t)
			m := *c.Manifest
			m.Files = files
			c.Manifest = &m
			if err := c.Validate(); err == nil {
				t.Fatal("accepted an out-of-bounds manifest")
			}
		})
	}
}
