package skillbundle

import (
	"errors"
	"regexp"
	"sort"
)

// FromMetadata validates a metadata-only manifest using the same path/mode
// identity as FromContents. It does not attest that a host has the claimed bytes;
// a transfer must recompute identity from actual contents before storage.
func FromMetadata(files []File) (Manifest, error) {
	var empty Manifest
	placeholders := make([]ContentFile, len(files))
	var total int64
	validDigest := regexp.MustCompile(`^[a-f0-9]{64}$`)
	for i, f := range files {
		if f.Bytes < 0 || f.Bytes > MaxBytes || !validDigest.MatchString(f.Digest) {
			return empty, errors.New("invalid manifest metadata")
		}
		total += f.Bytes
		if total > MaxBytes {
			return empty, errors.New("manifest byte limit exceeded")
		}
		placeholders[i] = ContentFile{Path: f.Path, Executable: f.Executable}
	}
	m, err := FromContents(placeholders)
	if err != nil {
		return empty, err
	}
	m.Files = append([]File{}, files...)
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	m.TotalBytes = total
	m.Digest = manifestDigest(m)
	return m, nil
}
