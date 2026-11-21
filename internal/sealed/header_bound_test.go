package sealed

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

// reheader writes a different header in front of the same sealed body.
func reheader(t *testing.T, data []byte, edit func(map[string]any)) []byte {
	t.Helper()
	_, head, body, err := parseArchive(data)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	dec := json.NewDecoder(bytes.NewReader(head))
	dec.UseNumber()
	if err := dec.Decode(&fields); err != nil {
		t.Fatal(err)
	}
	edit(fields)
	head, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	out := []byte(archiveMagic)
	out = binary.BigEndian.AppendUint32(out, uint32(len(head)))
	out = append(out, head...)
	return append(out, body...)
}

func TestEditedHeaderDoesNotOpen(t *testing.T) {
	sealer, _ := NewSealer(strings.Repeat("0", 32), "test")
	fenced := true
	m := Manifest{ID: "instance-1", Kind: "instance", Target: "instance", OwnerID: 7, TakenAt: 1000, Fenced: &fenced}
	data, err := sealer.Seal(m, []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	if _, payload, err := sealer.Open(data); err != nil || string(payload) != "payload" {
		t.Fatalf("an untouched copy: %q %v", payload, err)
	}
	edits := map[string]func(map[string]any){
		"fenced":  func(f map[string]any) { f["fenced"] = false },
		"kind":    func(f map[string]any) { f["kind"] = "user" },
		"ownerId": func(f map[string]any) { f["ownerId"] = 1 },
		"takenAt": func(f map[string]any) { f["takenAt"] = 2000 },
		// Claiming the old format must not get an edited header through.
		"format": func(f map[string]any) { delete(f, "format"); f["fenced"] = false },
	}
	for name, edit := range edits {
		if _, _, err := sealer.Open(reheader(t, data, edit)); err == nil {
			t.Fatalf("a copy with an edited %s opened", name)
		}
	}
}

func TestEditedHeaderOfPasswordCopyDoesNotOpen(t *testing.T) {
	sealer, _ := NewSealer(strings.Repeat("0", 32), "test")
	const password = "a password long enough"
	data, err := sealer.SealArchive(Manifest{ID: "user-1", Kind: "user", OwnerID: 3}, []byte("payload"), SealOptions{Password: password})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sealer.OpenWithPassword(data, password); err != nil {
		t.Fatal(err)
	}
	edited := reheader(t, data, func(f map[string]any) { f["ownerId"] = 9 })
	if _, _, err := sealer.OpenWithPassword(edited, password); err == nil {
		t.Fatal("a password copy with an edited owner opened")
	}
}

// A copy taken before the header was bound is sealed over its id alone, and
// must keep opening.
func TestOldFormatCopyStillOpens(t *testing.T) {
	key := strings.Repeat("0", 32)
	sealer, _ := NewSealer(key, "test")
	m := Manifest{ID: "instance-old", Kind: "instance", Target: "instance", TakenAt: 1000,
		Size: 7, Encrypted: true, Cipher: "AES-256-GCM/Argon2id", Protection: ProtectionKey}
	salt := bytes.Repeat([]byte{1}, saltLen)
	nonce := bytes.Repeat([]byte{2}, nonceLen)
	block, err := aes.NewCipher(derive([]byte(key), salt))
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	head, _ := json.Marshal(m)
	if bytes.Contains(head, []byte("format")) {
		t.Fatalf("an old header names a format: %s", head)
	}
	data := []byte(archiveMagic)
	data = binary.BigEndian.AppendUint32(data, uint32(len(head)))
	data = append(data, head...)
	data = append(data, salt...)
	data = append(data, nonce...)
	data = aead.Seal(data, nonce, []byte("payload"), []byte(m.ID))

	got, payload, err := sealer.Open(data)
	if err != nil || string(payload) != "payload" || got.ID != m.ID {
		t.Fatalf("open an old copy: %+v %q %v", got, payload, err)
	}
	if _, payload, err := OpenArchive(data, "", key); err != nil || string(payload) != "payload" {
		t.Fatalf("OpenArchive on an old copy: %q %v", payload, err)
	}
}
