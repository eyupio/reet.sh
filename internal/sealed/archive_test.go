package sealed

import (
	"strings"
	"testing"
)

func TestSealOpenAndManifest(t *testing.T) {
	sealer, err := NewSealer("0123456789abcdef0123456789abcdef", "test")
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{ID: "setup-1", Kind: "setup", Target: "setup"}
	data, err := sealer.Seal(manifest, []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	head, err := ManifestOf(data)
	if err != nil || head.ID != manifest.ID || head.Kind != "setup" || !head.Encrypted || head.Protection != ProtectionKey {
		t.Fatalf("manifest: %+v %v", head, err)
	}
	got, payload, err := sealer.Open(data)
	if err != nil || string(payload) != "payload" || got.ID != manifest.ID {
		t.Fatalf("open: %+v %q %v", got, payload, err)
	}
	other, _ := NewSealer(strings.Repeat("z", 32), "test")
	if _, _, err := other.Open(data); err == nil {
		t.Fatal("a different key opened the archive")
	}
}

func TestPasswordCompressionAndTamper(t *testing.T) {
	sealer, _ := NewSealer(strings.Repeat("0", 32), "test")
	manifest := Manifest{ID: "setup-2", Kind: "setup", Target: "setup"}
	data, err := sealer.SealArchive(manifest, []byte(strings.Repeat("payload", 100)), SealOptions{Compress: true, Password: "a password long enough"})
	if err != nil {
		t.Fatal(err)
	}
	head, err := ManifestOf(data)
	if err != nil || head.Protection != ProtectionPassword || head.Compression != CompressionGzip {
		t.Fatalf("manifest: %+v %v", head, err)
	}
	if _, _, err := sealer.Open(data); err == nil {
		t.Fatal("the key opened a password archive")
	}
	if _, _, err := OpenArchive(data, "the wrong password", ""); err == nil {
		t.Fatal("a wrong password opened the archive")
	}
	_, payload, err := OpenArchive(data, "a password long enough", "")
	if err != nil || !strings.HasPrefix(string(payload), "payloadpayload") {
		t.Fatalf("open: %q %v", payload, err)
	}
	data[len(data)-1] ^= 0xff
	if _, _, err := OpenArchive(data, "a password long enough", ""); err == nil {
		t.Fatal("a damaged archive opened")
	}
	if _, err := ManifestOf(data[:len(data)/2]); err == nil {
		t.Fatal("a truncated archive reported a manifest")
	}
}
