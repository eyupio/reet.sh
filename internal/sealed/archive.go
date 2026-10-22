package sealed

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	// MaxObjectBytes bounds an archive this build will open in memory.
	MaxObjectBytes = 512 << 20
	// MaxHeaderBytes bounds the clear manifest header.
	MaxHeaderBytes = 1 << 20

	archiveMagic = "REETBK1\n"
	argonTime    = 2
	argonMemory  = 32 * 1024
	argonThreads = 2
	keyLen       = 32
	saltLen      = 16
	nonceLen     = 12
)

// What protects a copy's payload, and how it is stored.
const (
	ProtectionKey      = "key"
	ProtectionPassword = "password"

	CompressionNone = "none"
	CompressionGzip = "gzip"
)

// MinPassword is the shortest password accepted for a copy, matching the
// shortest a Zoomies export takes.
const MinPassword = 12

// Manifest is the unsealed header of a copy: what it is, when it was taken,
// and the digest of the payload inside. It is readable before the copy is
// opened, which is what lets a listing say what each file holds.
type Manifest struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	OwnerID     int64  `json:"ownerId"`
	Target      string `json:"target"`
	TakenAt     int64  `json:"takenAt"`
	Version     string `json:"version,omitempty"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	Encrypted   bool   `json:"encrypted"`
	Cipher      string `json:"cipher,omitempty"`
	Protection  string `json:"protection,omitempty"`
	Compression string `json:"compression,omitempty"`
}

// SealOptions chooses how a copy is stored. A password replaces the
// instance key for that copy, so it opens with the password alone; Compress
// gzips the payload before it is sealed.
type SealOptions struct {
	Compress bool
	Password string
}

// Sealer seals every archive and every stored destination secret with one key.
type Sealer struct {
	passphrase  []byte
	source      string
	fingerprint string
}

// NewSealer checks the key and records where it came from.
func NewSealer(key, source string) (*Sealer, error) {
	key = strings.TrimSpace(key)
	if len(key) < 32 {
		return nil, errors.New("the backup key is shorter than 32 characters")
	}
	sum := sha256.Sum256([]byte(key))
	return &Sealer{passphrase: []byte(key), source: source, fingerprint: hex.EncodeToString(sum[:4])}, nil
}

func (s *Sealer) Source() string      { return s.source }
func (s *Sealer) Fingerprint() string { return s.fingerprint }

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func derive(passphrase, salt []byte) []byte {
	return argon2.IDKey(passphrase, salt, argonTime, argonMemory, argonThreads, keyLen)
}

// Seal wraps a payload with its manifest under the instance key.
func (s *Sealer) Seal(m Manifest, payload []byte) ([]byte, error) {
	return s.SealArchive(m, payload, SealOptions{})
}

// SealArchive wraps a payload with its manifest, optionally compressed and
// optionally sealed under a password instead of the instance key.
func (s *Sealer) SealArchive(m Manifest, payload []byte, opts SealOptions) ([]byte, error) {
	if m.TakenAt == 0 {
		m.TakenAt = time.Now().UnixMilli()
	}
	m.Size = int64(len(payload))
	sum := sha256.Sum256(payload)
	m.SHA256 = hex.EncodeToString(sum[:])
	m.Encrypted = true
	m.Cipher = "AES-256-GCM/Argon2id"
	m.Protection = ProtectionKey
	m.Compression = ""

	stored := payload
	if opts.Compress {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(payload); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
		stored = buf.Bytes()
		m.Compression = CompressionGzip
	}
	key := s.passphrase
	if opts.Password != "" {
		key = []byte(opts.Password)
		m.Protection = ProtectionPassword
	}

	salt := make([]byte, saltLen)
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(derive(key, salt))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sealed := aead.Seal(nil, nonce, stored, []byte(m.ID))
	head, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var out []byte
	out = append(out, archiveMagic...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(head)))
	out = append(out, head...)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// Open reads a copy sealed under the instance key, checks its digest and
// decompresses it when it was stored compressed.
func (s *Sealer) Open(data []byte) (Manifest, []byte, error) {
	m, body, err := parseArchive(data)
	if err != nil {
		return m, nil, err
	}
	if m.Protection == ProtectionPassword {
		return m, nil, errors.New("this copy is protected with a password; enter it to open it")
	}
	return s.openBody(m, body, s.passphrase, "this backup cannot be opened with this key, or it is damaged")
}

// OpenArchive opens a downloaded copy with the password or the instance
// key it needs, for a machine that may have no database.
func OpenArchive(data []byte, password, key string) (Manifest, []byte, error) {
	m, err := ManifestOf(data)
	if err != nil {
		return m, nil, err
	}
	if m.Protection == ProtectionPassword {
		if password == "" {
			return m, nil, errors.New("this copy is protected with a password; give it with --password or --password-file")
		}
		sealer, _ := NewSealer(strings.Repeat("0", 32), "cli")
		return sealer.OpenWithPassword(data, password)
	}
	if strings.TrimSpace(key) == "" {
		return m, nil, errors.New("this copy is sealed under the instance key; set REET_BACKUP_KEY (or give a password-protected copy its password)")
	}
	sealer, err := NewSealer(key, "cli")
	if err != nil {
		return m, nil, err
	}
	return sealer.Open(data)
}

// OpenWithPassword opens a copy sealed under a password. A copy sealed under
// the instance key opens as Open would, with the password ignored, so a
// caller that has both never has to choose.
func (s *Sealer) OpenWithPassword(data []byte, password string) (Manifest, []byte, error) {
	m, body, err := parseArchive(data)
	if err != nil {
		return m, nil, err
	}
	if m.Protection != ProtectionPassword {
		return s.openBody(m, body, s.passphrase, "this backup cannot be opened with this key, or it is damaged")
	}
	if password == "" {
		return m, nil, errors.New("this copy is protected with a password; enter it to open it")
	}
	return s.openBody(m, body, []byte(password), "the password does not open this copy, or it is damaged")
}

func (s *Sealer) openBody(m Manifest, body, key []byte, wrong string) (Manifest, []byte, error) {
	if len(body) < saltLen+nonceLen {
		return m, nil, errors.New("this backup is truncated")
	}
	block, err := aes.NewCipher(derive(key, body[:saltLen]))
	if err != nil {
		return m, nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return m, nil, err
	}
	plain, err := aead.Open(nil, body[saltLen:saltLen+nonceLen], body[saltLen+nonceLen:], []byte(m.ID))
	if err != nil {
		return m, nil, errors.New(wrong)
	}
	if m.Compression == CompressionGzip {
		gz, err := gzip.NewReader(bytes.NewReader(plain))
		if err != nil {
			return m, nil, fmt.Errorf("this backup's compressed payload is damaged: %w", err)
		}
		defer gz.Close()
		out, err := io.ReadAll(io.LimitReader(gz, MaxObjectBytes+1))
		if err != nil {
			return m, nil, fmt.Errorf("reading this backup's compressed payload: %w", err)
		}
		if int64(len(out)) > MaxObjectBytes {
			return m, nil, errors.New("this backup opens to more than this build can hold in memory")
		}
		plain = out
	}
	if err := verifyPayload(m, plain); err != nil {
		return m, nil, err
	}
	return m, plain, nil
}

// parseArchive reads the header and returns the body after it.
func parseArchive(data []byte) (Manifest, []byte, error) {
	var m Manifest
	if len(data) < len(archiveMagic)+4 || string(data[:len(archiveMagic)]) != archiveMagic {
		return m, nil, errors.New("this is not a reet backup")
	}
	at := len(archiveMagic)
	n := int(binary.BigEndian.Uint32(data[at : at+4]))
	at += 4
	if n < 0 || n > MaxHeaderBytes || len(data) < at+n {
		return m, nil, errors.New("this backup's header is damaged")
	}
	if err := json.Unmarshal(data[at:at+n], &m); err != nil {
		return m, nil, errors.New("this backup's header is damaged")
	}
	return m, data[at+n:], nil
}

func verifyPayload(m Manifest, payload []byte) error {
	sum := sha256.Sum256(payload)
	if m.SHA256 != "" && !strings.EqualFold(hex.EncodeToString(sum[:]), m.SHA256) {
		return errors.New("this backup does not match its recorded digest; it is damaged")
	}
	return nil
}

// ManifestOf reads only the header, for a listing that must not open every
// copy with the key.
func ManifestOf(data []byte) (Manifest, error) {
	m, _, err := parseArchive(data)
	return m, err
}

// SealSecret keeps a destination's secret access key unreadable in the
// database, under the same passphrase that opens the copies. Each secret gets
// its own salt, so the Argon2id work is not shared between them.
func (s *Sealer) SealSecret(secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	salt := make([]byte, saltLen)
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	block, err := aes.NewCipher(derive(s.passphrase, salt))
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	var out []byte
	out = append(out, salt...)
	out = append(out, nonce...)
	out = aead.Seal(out, nonce, []byte(secret), nil)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

func (s *Sealer) OpenSecret(sealed string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil || len(raw) < saltLen+nonceLen {
		return "", errors.New("the stored secret is damaged")
	}
	salt := raw[:saltLen]
	nonce := raw[saltLen : saltLen+nonceLen]
	block, err := aes.NewCipher(derive(s.passphrase, salt))
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := aead.Open(nil, nonce, raw[saltLen+nonceLen:], nil)
	if err != nil {
		return "", fmt.Errorf("the stored secret was sealed under another backup key; enter it again")
	}
	return string(plain), nil
}
