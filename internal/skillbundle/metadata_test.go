package skillbundle

import "testing"

func TestMetadataIdentityMatchesExactContentManifest(t *testing.T) {
	original, err := FromContents([]ContentFile{{Path: "SKILL.md", Data: []byte("instructions\r\n")}, {Path: "run.bin", Data: []byte{0, 255, 1}, Executable: true}})
	if err != nil {
		t.Fatal(err)
	}
	fromMetadata, err := FromMetadata(original.Files)
	if err != nil || fromMetadata.Digest != original.Digest {
		t.Fatal(fromMetadata, err)
	}
	for _, change := range []func([]File){func(f []File) { f[0].Path = "../outside" }, func(f []File) { f[0].Digest = "unknown" }, func(f []File) { f[0].Bytes = MaxBytes + 1 }, func(f []File) { f[0].Bytes = -1 }, func(f []File) { f[1].Path = f[0].Path }} {
		files := append([]File{}, original.Files...)
		change(files)
		if _, err = FromMetadata(files); err == nil {
			t.Fatal("unsafe metadata accepted", files)
		}
	}
	changed := append([]File{}, original.Files...)
	changed[1].Executable = false
	if m, err := FromMetadata(changed); err != nil || m.Digest == original.Digest {
		t.Fatal("mode not bound", m, err)
	}
}
