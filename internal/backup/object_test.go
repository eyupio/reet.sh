package backup

import (
	"bytes"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/objectstore"
)

func TestObjectHeaderRoundTripsAndRefusesDamage(t *testing.T) {
	m := ObjectManifest{Target: "instance", TakenAt: time.Now().UnixMilli(),
		PlaintextSize: 10, PlaintextSHA256: "abc", Compression: CompressionGzip, KeyFingerprint: "1234"}
	object, err := EncodeObject(m, []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	got, payload, err := DecodeObject(object)
	if err != nil || string(payload) != "payload" {
		t.Fatalf("decode: %v %q", err, payload)
	}
	if got.Target != "instance" || got.Compression != CompressionGzip {
		t.Fatalf("manifest = %+v", got)
	}
	bad := append([]byte(nil), object...)
	bad[headerFixed+1] ^= 0xff
	if _, _, err := DecodeObject(bad); err == nil {
		t.Fatal("a corrupted manifest was accepted")
	}
	bad = append([]byte(nil), object...)
	bad[len(bad)-1] ^= 0xff
	if _, _, err := DecodeObject(bad); err == nil {
		t.Fatal("a corrupted payload was accepted")
	}
}

func TestDiscoverCopiesPrefersTheOneObjectAndPairsTheOldLayout(t *testing.T) {
	now := time.Now().UTC()
	objects := []objectstore.Object{
		{Key: "pre/instance/instance-20260912T030004Z-aaaaaaaa.gz.enc", Size: 10, LastModified: now},
		{Key: "pre/instance/instance-20260912T030004Z-aaaaaaaa.sealed", Size: 9, LastModified: now},
		{Key: "pre/instance/instance-20260912T030004Z-aaaaaaaa.json", Size: 5, LastModified: now},
		{Key: "pre/user-1/user-1-20260911T030004Z-bbbbbbbb.sealed", Size: 8, LastModified: now},
		{Key: "pre/user-1/user-1-20260911T030004Z-bbbbbbbb.json", Size: 5, LastModified: now},
		{Key: "pre/user-1/user-1-20260910T030004Z-cccccccc.reetbackup", Size: 7, LastModified: now},
	}
	copies := DiscoverCopies("pre", objects)
	if len(copies) != 3 {
		t.Fatalf("copies = %+v", copies)
	}
	if copies[0].Name != "instance-20260912T030004Z-aaaaaaaa" || copies[0].Layout != LayoutObject {
		t.Fatalf("one-object copy not preferred: %+v", copies[0])
	}
	layouts := map[string]string{}
	for _, c := range copies {
		layouts[c.Name] = c.Layout
	}
	if layouts["user-1-20260911T030004Z-bbbbbbbb"] != LayoutPair || layouts["user-1-20260910T030004Z-cccccccc"] != LayoutLegacy {
		t.Fatalf("old layouts = %+v", layouts)
	}
}

func TestObjectNameUsesCompressionSuffix(t *testing.T) {
	if got := ObjectKey("pre", "instance", "instance-20260912T030004Z-aaaaaaaa", CompressionGzip); got != "pre/instance/instance-20260912T030004Z-aaaaaaaa.gz.enc" {
		t.Fatalf("compressed key = %q", got)
	}
	if got := ObjectKey("pre", "instance", "instance-20260912T030004Z-aaaaaaaa", CompressionNone); got != "pre/instance/instance-20260912T030004Z-aaaaaaaa.enc" {
		t.Fatalf("plain key = %q", got)
	}
}

func TestOuterSealIsVerifiedAndUnwrapped(t *testing.T) {
	payload := []byte("already encrypted")
	m := ObjectManifest{Target: "instance", CopyID: "instance-20260912T030004Z-aaaaaaaa", TakenAt: time.Now().UnixMilli()}
	object, err := sealObject(m, payload, "a destination passphrase")
	if err != nil {
		t.Fatal(err)
	}
	got, outer, err := DecodeObject(object)
	if err != nil {
		t.Fatal(err)
	}
	if !got.PassphraseSealed || got.OuterSize != int64(len(outer)) {
		t.Fatalf("outer manifest = %+v", got)
	}
	inner, err := openOuter(outer, "a destination passphrase")
	if err != nil || !bytes.Equal(inner, payload) {
		t.Fatalf("open outer: %q %v", inner, err)
	}
	if _, err := openOuter(outer, "the wrong destination passphrase"); err == nil {
		t.Fatal("a wrong destination passphrase opened the copy")
	}
}
