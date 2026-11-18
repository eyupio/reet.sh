package scan

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

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
		if parent, err = resolveLinks(parent); err != nil || parent != r.sharedReal {
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
