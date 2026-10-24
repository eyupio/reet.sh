// Package skillbundle observes local skill contents without modifying them.
// A matching name or upstream source is not evidence of matching file bytes.
// These versioned manifests are independent of the existing setup snapshot digest.
package skillbundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SchemaVersion       = 1
	Algorithm           = "sha256"
	ModeSemantics       = "posix-executable-v1"
	MaxFiles            = 256
	MaxBytes      int64 = 8 << 20
	MaxEntries          = 1024
	MaxDepth            = 32
	MaxPathBytes        = 1024
)

var ErrIncomplete = errors.New("skill bundle observation is incomplete")
var ErrLimits = errors.New("skill bundle limits must be positive and within supported bounds")

// Limits may tighten, but never increase, the supported observation bounds.
// Zero fields select the defaults. Other ingestion formats may have lower limits.
type Limits struct {
	Files      int
	FileBytes  int64
	TotalBytes int64
	Entries    int
	Depth      int
	PathBytes  int
}

type File struct {
	Path       string `json:"path"`
	Bytes      int64  `json:"bytes"`
	Digest     string `json:"digest"`
	Executable bool   `json:"executable"`
}

type Issue struct {
	Path string `json:"path"` // Relative path only; "." identifies the selected root.
	Code string `json:"code"`
}

type Manifest struct {
	SchemaVersion int     `json:"schemaVersion"`
	Algorithm     string  `json:"algorithm"`
	ModeSemantics string  `json:"modeSemantics"`
	Complete      bool    `json:"complete"`
	Files         []File  `json:"files"`
	TotalBytes    int64   `json:"totalBytes"`
	Digest        string  `json:"digest,omitempty"` // Never set for an incomplete observation.
	Issues        []Issue `json:"issues,omitempty"`
}

// Scan hashes exact bytes and the presence of any executable bit on Linux/macOS.
// It never returns file contents or absolute host paths. Missing, unsafe, changing
// or unreadable entries produce ErrIncomplete, with a non-comparable manifest.
// Observations detect metadata/membership changes but are not atomic filesystem
// snapshots; a future apply must lock and revalidate its own preconditions.
func Scan(dir string, limits Limits) (Manifest, error) {
	return scan(dir, limits, nil)
}

type observation struct {
	path string
	info os.FileInfo
}

type scanner struct {
	contents  *[]ContentFile
	limits    Limits
	manifest  Manifest
	entries   int
	bytesRead int64
	seen      []string
	// Internal read boundary permits deterministic filesystem-race fixtures.
	afterRead    func(string)
	observations []observation
}

func scan(dir string, limits Limits, afterRead func(string)) (Manifest, error) {
	return scanInto(dir, limits, afterRead, nil)
}

func scanInto(dir string, limits Limits, afterRead func(string), contents *[]ContentFile) (Manifest, error) {
	m := Manifest{SchemaVersion: SchemaVersion, Algorithm: Algorithm, ModeSemantics: ModeSemantics, Files: []File{}}
	limits, err := checkedLimits(limits)
	if err != nil {
		return m, err
	}
	s := scanner{limits: limits, manifest: m, afterRead: afterRead, contents: contents}
	if !supportedModes {
		s.issue(".", "platform_unsupported")
		return s.result()
	}
	before, err := os.Lstat(dir)
	if err != nil {
		code := "root_unavailable"
		if errors.Is(err, os.ErrNotExist) {
			code = "root_missing"
		}
		s.issue(".", code)
		return s.result()
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		s.issue(".", "root_not_regular_directory")
		return s.result()
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		s.issue(".", "root_unavailable")
		return s.result()
	}
	defer root.Close()
	opened, err := root.Stat(".")
	bound, bindErr := os.Lstat(dir)
	if err != nil || bindErr != nil || !unchanged(before, opened) || !unchanged(before, bound) {
		s.issue(".", "root_changed")
		return s.result()
	}
	s.walk(root, "", 0)
	for _, observed := range s.observations {
		current, err := root.Lstat(observed.path)
		if err != nil || !unchanged(observed.info, current) {
			s.issue(observed.path, "entry_changed")
		}
	}
	after, err := os.Lstat(dir)
	if err != nil || !unchanged(before, after) {
		s.issue(".", "root_changed")
	}
	found := false
	for _, file := range s.manifest.Files {
		if file.Path == "SKILL.md" {
			found = true
		}
	}
	if !found && len(s.manifest.Issues) == 0 {
		// Only a complete traversal can establish absence. A failed read is unknown.
		s.issue("SKILL.md", "skill_manifest_missing")
	}
	return s.result()
}

func checkedLimits(l Limits) (Limits, error) {
	if l.Files == 0 {
		l.Files = MaxFiles
	}
	if l.FileBytes == 0 {
		l.FileBytes = MaxBytes
	}
	if l.TotalBytes == 0 {
		l.TotalBytes = MaxBytes
	}
	if l.Entries == 0 {
		l.Entries = MaxEntries
	}
	if l.Depth == 0 {
		l.Depth = MaxDepth
	}
	if l.PathBytes == 0 {
		l.PathBytes = MaxPathBytes
	}
	if l.Files < 1 || l.Files > MaxFiles || l.FileBytes < 1 || l.FileBytes > MaxBytes || l.TotalBytes < 1 || l.TotalBytes > MaxBytes || l.Entries < 1 || l.Entries > MaxEntries || l.Depth < 1 || l.Depth > MaxDepth || l.PathBytes < 1 || l.PathBytes > MaxPathBytes {
		return l, ErrLimits
	}
	return l, nil
}

func (s *scanner) issue(name, code string) {
	s.manifest.Issues = append(s.manifest.Issues, Issue{Path: name, Code: code})
}

func (s *scanner) result() (Manifest, error) {
	sort.Slice(s.manifest.Files, func(i, j int) bool { return s.manifest.Files[i].Path < s.manifest.Files[j].Path })
	sort.Slice(s.manifest.Issues, func(i, j int) bool {
		a, b := s.manifest.Issues[i], s.manifest.Issues[j]
		if a.Path == b.Path {
			return a.Code < b.Code
		}
		return a.Path < b.Path
	})
	if len(s.manifest.Issues) > 0 {
		return s.manifest, ErrIncomplete
	}
	s.manifest.Complete = true
	s.manifest.Digest = manifestDigest(s.manifest)
	return s.manifest, nil
}

func directoryEntries(root *os.Root, limit int) ([]os.DirEntry, error) {
	f, err := root.OpenFile(".", os.O_RDONLY|readFlags, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(limit + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (s *scanner) walk(root *os.Root, prefix string, depth int) {
	before, err := root.Stat(".")
	if err != nil {
		s.issue(display(prefix), "directory_unreadable")
		return
	}
	if prefix != "" {
		s.observations = append(s.observations, observation{prefix, before})
	}
	entries, err := directoryEntries(root, s.limits.Entries-s.entries)
	if err != nil {
		s.issue(display(prefix), "directory_unreadable")
		return
	}
	if len(entries) > s.limits.Entries-s.entries {
		s.issue(display(prefix), "entry_limit")
		return
	}
	// Count all entries before recursing: empty directories also consume the bound.
	s.entries += len(entries)
	for _, entry := range entries {
		name := entry.Name()
		relative := name
		if prefix != "" {
			relative = prefix + "/" + name
		}
		if !validPath(relative, s.limits.PathBytes) {
			s.issue(display(prefix), "unsafe_path")
			continue
		}
		collision := false
		for _, seen := range s.seen {
			if strings.EqualFold(seen, relative) {
				collision = true
				break
			}
		}
		if collision {
			s.issue(relative, "case_collision")
			continue
		}
		s.seen = append(s.seen, relative)
		info, err := root.Lstat(name)
		if err != nil {
			s.issue(relative, "metadata_unavailable")
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			s.issue(relative, "symlink")
			continue
		}
		if info.IsDir() {
			if depth >= s.limits.Depth {
				s.issue(relative, "depth_limit")
				continue
			}
			child, err := root.OpenRoot(name)
			if err != nil {
				s.issue(relative, "directory_unreadable")
				continue
			}
			opened, statErr := child.Stat(".")
			if statErr != nil || !unchanged(info, opened) {
				s.issue(relative, "directory_changed")
			} else {
				s.walk(child, relative, depth+1)
			}
			child.Close()
			after, err := root.Lstat(name)
			if err != nil || !unchanged(info, after) {
				s.issue(relative, "directory_changed")
			}
			continue
		}
		if !info.Mode().IsRegular() {
			s.issue(relative, "special_file")
			continue
		}
		if len(s.manifest.Files) >= s.limits.Files {
			s.issue(relative, "file_limit")
			continue
		}
		remaining := s.limits.TotalBytes - s.bytesRead
		limit := min(s.limits.FileBytes, remaining)
		if remaining < 0 || info.Size() > limit {
			s.issue(relative, "byte_limit")
			continue
		}
		var captured bytes.Buffer
		var capture io.Writer
		if s.contents != nil {
			capture = &captured
		}
		file, bytesRead, code := readFileInto(root, name, relative, info, limit, s.afterRead, capture)
		s.bytesRead += bytesRead
		if code != "" {
			s.issue(relative, code)
			continue
		}
		s.observations = append(s.observations, observation{relative, info})
		if s.contents != nil {
			*s.contents = append(*s.contents, ContentFile{Path: file.Path, Data: captured.Bytes(), Executable: file.Executable})
		}
		s.manifest.Files = append(s.manifest.Files, file)
		s.manifest.TotalBytes += file.Bytes
	}
	after, err := root.Stat(".")
	if err != nil || !unchanged(before, after) {
		s.issue(display(prefix), "directory_changed")
		return
	}
	// Recheck membership even on filesystems with coarse directory timestamps.
	current, err := directoryEntries(root, len(entries))
	if err != nil || len(entries) != len(current) {
		s.issue(display(prefix), "directory_changed")
		return
	}
	for i := range entries {
		if entries[i].Name() != current[i].Name() || entries[i].Type() != current[i].Type() {
			s.issue(display(prefix), "directory_changed")
			return
		}
	}
}

func readFile(root *os.Root, name, relative string, before os.FileInfo, limit int64, afterRead func(string)) (File, int64, string) {
	return readFileInto(root, name, relative, before, limit, afterRead, nil)
}

func readFileInto(root *os.Root, name, relative string, before os.FileInfo, limit int64, afterRead func(string), capture io.Writer) (File, int64, string) {
	f, err := root.OpenFile(name, os.O_RDONLY|readFlags, 0)
	if err != nil {
		return File{}, 0, "file_unreadable"
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !unchanged(before, opened) {
		return File{}, 0, "file_changed"
	}
	hash := sha256.New()
	var sink io.Writer = hash
	if capture != nil {
		sink = io.MultiWriter(hash, capture)
	}
	size, err := io.Copy(sink, io.LimitReader(f, limit+1))
	if err != nil {
		return File{}, size, "file_unreadable"
	}
	if size > limit {
		return File{}, size, "byte_limit"
	}
	if afterRead != nil {
		afterRead(relative)
	}
	after, err := f.Stat()
	bound, bindErr := root.Lstat(name)
	if err != nil || bindErr != nil || size != before.Size() || !unchanged(before, after) || !unchanged(before, bound) {
		return File{}, size, "file_changed"
	}
	return File{Path: relative, Bytes: size, Digest: hex.EncodeToString(hash.Sum(nil)), Executable: before.Mode().Perm()&0o111 != 0}, size, ""
}

func unchanged(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
func display(prefix string) string {
	if prefix == "" {
		return "."
	}
	return prefix
}

// Paths are bounded relative UTF-8 paths with portable-name restrictions. Case-fold collisions are rejected
// across both files and directories; no path or Unicode normalization rewrites bytes.
func validPath(name string, limit int) bool {
	if name == "" || len(name) > limit || !utf8.ValidString(name) || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.ContainsAny(name, `\:<>"|?*`) {
		return false
	}
	for _, c := range name {
		if unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || unicode.Is(unicode.Zl, c) || unicode.Is(unicode.Zp, c) {
			return false
		}
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || len(part) > 255 || strings.TrimRight(part, ". ") != part {
			return false
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
			return false
		}
	}
	return true
}

// Canonical v1 input is compact encoding/json of this fixed field-order struct,
// with files sorted by exact relative UTF-8 path; digests are lowercase SHA-256.
// Summaries, observations and issues are not content identity.
func manifestDigest(m Manifest) string {
	data, _ := json.Marshal(struct {
		SchemaVersion int    `json:"schemaVersion"`
		Algorithm     string `json:"algorithm"`
		ModeSemantics string `json:"modeSemantics"`
		Files         []File `json:"files"`
	}{m.SchemaVersion, m.Algorithm, m.ModeSemantics, m.Files})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
