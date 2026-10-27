package skillbundle

import "sort"

// ReadContents explicitly captures exact bytes using Scan's bounded, no-follow
// reads and filesystem revalidation. A changed, unsafe or partial observation
// returns no contents. The expected digest must come from the caller's preview.
func ReadContents(dir, expectedDigest string) (Manifest, []ContentFile, error) {
	return readContents(dir, expectedDigest, nil)
}
func readContents(dir, expectedDigest string, afterRead func(string)) (Manifest, []ContentFile, error) {
	files := []ContentFile{}
	m, err := scanInto(dir, Limits{}, afterRead, &files)
	if err != nil {
		return m, nil, err
	}
	if expectedDigest == "" || m.Digest != expectedDigest {
		return m, nil, ErrIncomplete
	}
	verified, err := FromContents(files)
	if err != nil || verified.Digest != m.Digest {
		return m, nil, ErrIncomplete
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return m, files, nil
}
