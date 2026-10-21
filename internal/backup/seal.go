package backup

import (
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
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	archiveMagic = "REETBK1\n"
	argonTime    = 2
	argonMemory  = 32 * 1024
	argonThreads = 2
	keyLen       = 32
	saltLen      = 16
	nonceLen     = 12
)

// Manifest is the unsealed header of a copy: what it is, when it was taken,
// and the digest of the payload inside. It is readable before the copy is
// opened, which is what lets a listing say what each file holds.
type Manifest struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	OwnerID   int64  `json:"ownerId"`
	Target    string `json:"target"`
	TakenAt   int64  `json:"takenAt"`
	Version   string `json:"version,omitempty"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Encrypted bool   `json:"encrypted"`
	Cipher    string `json:"cipher,omitempty"`
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

// Seal wraps a payload with its manifest. The manifest is readable without
// the key; the payload is not.
func (s *Sealer) Seal(m Manifest, payload []byte) ([]byte, error) {
	if m.TakenAt == 0 {
		m.TakenAt = time.Now().UnixMilli()
	}
	m.Size = int64(len(payload))
	sum := sha256.Sum256(payload)
	m.SHA256 = hex.EncodeToString(sum[:])
	m.Encrypted = true
	m.Cipher = "AES-256-GCM/Argon2id"

	salt := make([]byte, saltLen)
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(derive(s.passphrase, salt))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sealed := aead.Seal(nil, nonce, payload, []byte(m.ID))
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

// Open reads a copy, checks its digest, and gives back the manifest and the
// bytes inside. A wrong key, a damaged file and a file that is not a reet
// backup each fail with words that say which.
func (s *Sealer) Open(data []byte) (Manifest, []byte, error) {
	var m Manifest
	if len(data) < len(archiveMagic)+4 || string(data[:len(archiveMagic)]) != archiveMagic {
		return m, nil, errors.New("this is not a reet backup")
	}
	at := len(archiveMagic)
	n := int(binary.BigEndian.Uint32(data[at : at+4]))
	at += 4
	if n < 0 || n > 1<<20 || len(data) < at+n {
		return m, nil, errors.New("this backup's header is damaged")
	}
	if err := json.Unmarshal(data[at:at+n], &m); err != nil {
		return m, nil, errors.New("this backup's header is damaged")
	}
	at += n
	body := data[at:]
	if !m.Encrypted {
		if err := verifyPayload(m, body); err != nil {
			return m, nil, err
		}
		return m, body, nil
	}
	if len(body) < saltLen+nonceLen {
		return m, nil, errors.New("this backup is truncated")
	}
	block, err := aes.NewCipher(derive(s.passphrase, body[:saltLen]))
	if err != nil {
		return m, nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return m, nil, err
	}
	plain, err := aead.Open(nil, body[saltLen:saltLen+nonceLen], body[saltLen+nonceLen:], []byte(m.ID))
	if err != nil {
		return m, nil, errors.New("this backup cannot be opened with this key, or it is damaged")
	}
	if err := verifyPayload(m, plain); err != nil {
		return m, nil, err
	}
	return m, plain, nil
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
	var m Manifest
	if len(data) < len(archiveMagic)+4 || string(data[:len(archiveMagic)]) != archiveMagic {
		return m, errors.New("this is not a reet backup")
	}
	at := len(archiveMagic)
	n := int(binary.BigEndian.Uint32(data[at : at+4]))
	at += 4
	if n < 0 || n > 1<<20 || len(data) < at+n {
		return m, errors.New("this backup's header is damaged")
	}
	if err := json.Unmarshal(data[at:at+n], &m); err != nil {
		return m, errors.New("this backup's header is damaged")
	}
	return m, nil
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
