package backup

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/eyupio/reet.sh/internal/sealed"
)

// The sealed archive format is shared with local setup snapshots.
type Manifest = sealed.Manifest
type Sealer = sealed.Sealer
type SealOptions = sealed.SealOptions

const (
	ProtectionKey      = sealed.ProtectionKey
	ProtectionPassword = sealed.ProtectionPassword
	CompressionNone    = sealed.CompressionNone
	CompressionGzip    = sealed.CompressionGzip
	MinPassword        = sealed.MinPassword
	maxObjectBytes     = sealed.MaxObjectBytes
)

func NewSealer(key, source string) (*Sealer, error) { return sealed.NewSealer(key, source) }

func OpenArchive(data []byte, password, key string) (Manifest, []byte, error) {
	return sealed.OpenArchive(data, password, key)
}

func ManifestOf(data []byte) (Manifest, error) { return sealed.ManifestOf(data) }

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
