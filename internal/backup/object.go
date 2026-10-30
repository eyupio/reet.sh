package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/objectstore"
)

// The object format is one S3 object per copy: a short clear header, then the
// encrypted payload. The header is inside the object rather than in S3 user
// metadata or tags, so every S3-compatible store can serve it.
const (
	// ObjectMagic starts every bundled object.
	ObjectMagic = "ZPBKUP01"
	// MaxManifestBytes bounds the clear manifest this build will read.
	MaxManifestBytes = 1 << 20

	headerFixed = len(ObjectMagic) + 4 + sha256.Size
)

// Layout names how a copy is held at a destination.
const (
	LayoutObject = "object" // PREFIX/TARGET/NAME.gz.enc or NAME.enc
	LayoutLegacy = "legacy" // the previous one-object NAME.reetbackup or NAME.db
	LayoutPair   = "pair"   // the old NAME.json plus NAME.sealed
)

// ObjectManifest is the clear header of a bundled copy. It never holds a
// secret: no passphrase, access key, token, password or private key.
type ObjectManifest struct {
	Target          string `json:"target"`
	Instance        string `json:"instance,omitempty"`
	Name            string `json:"name,omitempty"`
	TakenAt         int64  `json:"taken_at"`
	PlaintextSize   int64  `json:"plaintext_size"`
	PlaintextSHA256 string `json:"plaintext_sha256"`
	PayloadSize     int64  `json:"payload_size"`
	PayloadSHA256   string `json:"payload_sha256"`
	Compression     string `json:"compression"`
	KeyFingerprint  string `json:"key_fingerprint,omitempty"`
	Version         string `json:"version,omitempty"`
	CopyID          string `json:"copy_id,omitempty"`
	Kind            string `json:"kind,omitempty"`
	Protection      string `json:"protection,omitempty"`
	Encryption      string `json:"encryption,omitempty"`
	// The optional outer seal: the payload was already encrypted and is then
	// sealed again under a destination passphrase.
	PassphraseSealed bool   `json:"passphrase_sealed,omitempty"`
	OuterSize        int64  `json:"outer_size,omitempty"`
	OuterSHA256      string `json:"outer_sha256,omitempty"`
}

// Extension is the key suffix a copy with this manifest takes.
func (m ObjectManifest) Extension() string {
	if m.Compression == "gzip" {
		return ".gz.enc"
	}
	return ".enc"
}

// EncodeObject writes the header and payload. It fills the payload size and
// digest from the bytes it is given.
func EncodeObject(m ObjectManifest, payload []byte) ([]byte, error) {
	m.PayloadSize = int64(len(payload))
	sum := sha256.Sum256(payload) // lgtm[go/weak-sensitive-data-hashing]
	m.PayloadSHA256 = hex.EncodeToString(sum[:])
	return encodeObjectWithHeader(m, payload, nil)
}

// EncodeObjectWithOuter writes a header for a payload that has already been
// sealed again under a destination passphrase.
func EncodeObjectWithOuter(m ObjectManifest, payload, outer []byte) ([]byte, error) {
	m.PayloadSize = int64(len(payload))
	sum := sha256.Sum256(payload) // lgtm[go/weak-sensitive-data-hashing]
	m.PayloadSHA256 = hex.EncodeToString(sum[:])
	m.PassphraseSealed = true
	m.OuterSize = int64(len(outer))
	outerSum := sha256.Sum256(outer)
	m.OuterSHA256 = hex.EncodeToString(outerSum[:])
	return encodeObjectWithHeader(m, outer, nil)
}

func encodeObjectWithHeader(m ObjectManifest, body []byte, _ []byte) ([]byte, error) {
	head, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if len(head) > MaxManifestBytes {
		return nil, fmt.Errorf("the manifest is %d bytes, larger than this build writes (%d)", len(head), MaxManifestBytes)
	}
	digest := sha256.Sum256(head)
	out := make([]byte, 0, headerFixed+len(head))
	out = append(out, ObjectMagic...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(head)))
	out = append(out, digest[:]...)
	out = append(out, head...)
	out = append(out, body...)
	return out, nil
}

// DecodeObjectHeader reads only the clear header. The reader is left at the
// first byte of the payload.
func DecodeObjectHeader(r io.Reader) (ObjectManifest, error) {
	var m ObjectManifest
	fixed := make([]byte, len(ObjectMagic)+4)
	if _, err := io.ReadFull(r, fixed); err != nil {
		return m, fmt.Errorf("reading the object header: %w", err)
	}
	if string(fixed[:len(ObjectMagic)]) != ObjectMagic {
		return m, errors.New("that object is not a reet cloud backup: its header is missing")
	}
	n := binary.BigEndian.Uint32(fixed[len(ObjectMagic):])
	if n == 0 || n > MaxManifestBytes {
		return m, fmt.Errorf("the object's manifest length %d is not one this build reads", n)
	}
	want := make([]byte, sha256.Size)
	if _, err := io.ReadFull(r, want); err != nil {
		return m, fmt.Errorf("reading the manifest digest: %w", err)
	}
	head := make([]byte, n)
	if _, err := io.ReadFull(r, head); err != nil {
		return m, fmt.Errorf("reading the manifest: %w", err)
	}
	got := sha256.Sum256(head)
	if !bytes.Equal(got[:], want) {
		return m, errors.New("the object's manifest does not match its digest; the object is damaged")
	}
	if err := json.Unmarshal(head, &m); err != nil {
		return m, fmt.Errorf("reading the object's manifest: %w", err)
	}
	if m.Target == "" || m.PayloadSize < 0 || m.PayloadSHA256 == "" {
		return m, errors.New("the object's manifest is missing what a copy needs")
	}
	return m, nil
}

// DecodeObject reads a whole bundled object and verifies the header digest,
// the payload size and the payload digest before returning the payload.
func DecodeObject(data []byte) (ObjectManifest, []byte, error) {
	var m ObjectManifest
	r := bytes.NewReader(data)
	head, err := DecodeObjectHeader(r)
	if err != nil {
		return m, nil, err
	}
	payload, err := io.ReadAll(r)
	if err != nil {
		return m, nil, err
	}
	if head.PassphraseSealed {
		if int64(len(payload)) != head.OuterSize {
			return head, nil, fmt.Errorf("the object holds %d outer bytes, but its manifest says %d", len(payload), head.OuterSize)
		}
		sum := sha256.Sum256(payload)
		if hex.EncodeToString(sum[:]) != head.OuterSHA256 {
			return head, nil, errors.New("the object's outer seal does not match its manifest")
		}
		return head, payload, nil
	}
	if int64(len(payload)) != head.PayloadSize {
		return head, nil, fmt.Errorf("the object holds %d payload bytes, but its manifest says %d", len(payload), head.PayloadSize)
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != head.PayloadSHA256 {
		return head, nil, errors.New("the object's payload does not match its manifest")
	}
	return head, payload, nil
}

// HeaderLength is the number of header bytes at the front of data, or 0 when
// there is not yet a whole header.
func HeaderLength(data []byte) int {
	if len(data) < len(ObjectMagic)+4 {
		return 0
	}
	if string(data[:len(ObjectMagic)]) != ObjectMagic {
		return 0
	}
	n := binary.BigEndian.Uint32(data[len(ObjectMagic):])
	if n == 0 || n > MaxManifestBytes {
		return 0
	}
	total := headerFixed + int(n)
	if total > len(data) {
		return 0
	}
	return total
}

// ObjectName strips a local file's own suffix and gives the name the object
// carries for a target and time.
func ObjectName(local string) string {
	base := path.Base(local)
	for _, suffix := range []string{".reetbackup", ".db", ".gz.enc", ".enc", ".sealed", ".json"} {
		if strings.HasSuffix(base, suffix) {
			return strings.TrimSuffix(base, suffix)
		}
	}
	return base
}

// ObjectKey builds PREFIX/TARGET/NAME.gz.enc or PREFIX/TARGET/NAME.enc.
func ObjectKey(prefix, target, name, compression string) string {
	if compression == "gzip" {
		name += ".gz.enc"
	} else {
		name += ".enc"
	}
	return joinKey(prefix, target, name)
}

func joinKey(prefix, target, name string) string {
	parts := []string{strings.Trim(prefix, "/"), strings.Trim(target, "/"), strings.TrimLeft(name, "/")}
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "/")
}

// CopyRef is one copy read from a key listing. It is built without a GET.
type CopyRef struct {
	Key         string
	Target      string
	Instance    string
	Name        string
	Compression string
	Layout      string
	Size        int64
	Modified    time.Time
	TakenAt     time.Time
}

var copyTimePattern = regexp.MustCompile(`(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z`)

// CopyTime reads the time a copy's name carries, falling back to its last
// modification. Retention is ordered by this, not by when an upload landed.
func CopyTime(name string, modified time.Time) time.Time {
	if m := copyTimePattern.FindStringSubmatch(name); m != nil {
		if t, err := time.Parse("20060102T150405Z", m[1]+m[2]+m[3]+"T"+m[4]+m[5]+m[6]+"Z"); err == nil {
			return t.UTC()
		}
	}
	if n, err := strconv.ParseInt(name, 10, 64); err == nil && n > 0 {
		return time.UnixMilli(n).UTC()
	}
	return modified.UTC()
}

// RefFromKey turns one listed key into a copy reference. A key that is not a
// copy under this prefix is not a reference.
func RefFromKey(prefix, key string, size int64, modified time.Time) (CopyRef, bool) {
	rel := strings.TrimPrefix(strings.TrimPrefix(key, "/"), strings.Trim(prefix, "/"))
	rel = strings.TrimPrefix(rel, "/")
	parts := strings.Split(rel, "/")
	var target string
	switch {
	case len(parts) >= 2 && parts[0] == "instance":
		target = "instance"
	case len(parts) >= 2 && strings.HasPrefix(parts[0], "user-"):
		target = parts[0]
	default:
		return CopyRef{}, false
	}
	base := parts[len(parts)-1]
	ref := CopyRef{Key: key, Target: target, Size: size, Modified: modified.UTC()}
	switch {
	case strings.HasSuffix(base, ".gz.enc"):
		ref.Name, ref.Compression, ref.Layout = strings.TrimSuffix(base, ".gz.enc"), "gzip", LayoutObject
	case strings.HasSuffix(base, ".enc"):
		ref.Name, ref.Compression, ref.Layout = strings.TrimSuffix(base, ".enc"), "none", LayoutObject
	case strings.HasSuffix(base, ".reetbackup"):
		ref.Name, ref.Layout = strings.TrimSuffix(base, ".reetbackup"), LayoutLegacy
	case strings.HasSuffix(base, ".sealed"):
		ref.Name, ref.Layout = strings.TrimSuffix(base, ".sealed"), LayoutPair
	case strings.HasSuffix(base, ".json"):
		ref.Name, ref.Layout = strings.TrimSuffix(base, ".json"), "manifest"
	default:
		return CopyRef{}, false
	}
	ref.TakenAt = CopyTime(ref.Name, modified)
	return ref, true
}

// DiscoverCopies groups a listing into copies. A name is old-style only when
// both its .json manifest and .sealed payload exist. New one-object keys win
// over either old layout when both are present.
func DiscoverCopies(prefix string, objects []objectstore.Object) []CopyRef {
	byName := map[string]map[string]CopyRef{}
	for _, o := range objects {
		ref, ok := RefFromKey(prefix, o.Key, o.Size, o.LastModified)
		if !ok {
			continue
		}
		group := ref.Target + "/" + ref.Instance + "/" + ref.Name
		if byName[group] == nil {
			byName[group] = map[string]CopyRef{}
		}
		// A manifest on its own is not a copy; it is read with its .sealed.
		if ref.Layout == "manifest" {
			byName[group]["manifest"] = ref
			continue
		}
		byName[group][ref.Layout] = ref
	}
	var out []CopyRef
	for _, layouts := range byName {
		if r, ok := layouts[LayoutObject]; ok {
			out = append(out, r)
			continue
		}
		if r, ok := layouts[LayoutPair]; ok {
			if manifest, hasManifest := layouts["manifest"]; hasManifest {
				r.Size += manifest.Size
				out = append(out, r)
			}
			continue
		}
		if r, ok := layouts[LayoutLegacy]; ok {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TakenAt.Equal(out[j].TakenAt) {
			return out[i].Key > out[j].Key
		}
		return out[i].TakenAt.After(out[j].TakenAt)
	})
	return out
}

// KeysForCopy names every object a copy of this target and name could have in
// either layout, so deleting one removes the copy and its old companions.
func KeysForCopy(prefix string, ref CopyRef) []string {
	target := ref.Target
	if ref.Instance != "" {
		target = ref.Target + "/" + ref.Instance
	}
	return []string{
		joinKey(prefix, target, ref.Name+".gz.enc"),
		joinKey(prefix, target, ref.Name+".enc"),
		joinKey(prefix, target, ref.Name+".sealed"),
		joinKey(prefix, target, ref.Name+".json"),
		joinKey(prefix, target, ref.Name+".reetbackup"),
	}
}
