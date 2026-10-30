package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// objectManifestOf describes an already-sealed copy so its S3 object carries
// the clear facts a listing or a recovery needs. The payload itself is the
// existing sealed archive, unchanged.
func (s *Service) objectManifestOf(m Manifest, sealed []byte) (ObjectManifest, error) {
	compression := m.Compression
	if compression == "" {
		compression = CompressionNone
	}
	fingerprint := ""
	if s.cfg.Sealer != nil {
		fingerprint = s.cfg.Sealer.Fingerprint()
	}
	return ObjectManifest{
		Target:          m.Target,
		TakenAt:         m.TakenAt,
		PlaintextSize:   m.Size,
		PlaintextSHA256: m.SHA256,
		Compression:     compression,
		KeyFingerprint:  fingerprint,
		Version:         s.cfg.Version,
		CopyID:          m.ID,
		Kind:            m.Kind,
		Protection:      m.Protection,
	}, nil
}

// sealObject writes the header and, when a destination has a passphrase, the
// second seal around the already-encrypted payload.
func sealObject(m ObjectManifest, payload []byte, passphrase string) ([]byte, error) {
	if strings.TrimSpace(passphrase) == "" {
		return EncodeObject(m, payload)
	}
	outer, err := NewSealer(strings.Repeat("0", 32), "destination")
	if err != nil {
		return nil, err
	}
	sealed, err := outer.SealArchive(Manifest{ID: m.CopyID, Kind: m.Kind, Target: m.Target, TakenAt: m.TakenAt},
		payload, SealOptions{Password: passphrase})
	if err != nil {
		return nil, err
	}
	return EncodeObjectWithOuter(m, payload, sealed)
}

func openOuter(payload []byte, passphrase string) ([]byte, error) {
	if strings.TrimSpace(passphrase) == "" {
		return nil, errors.New("this copy is sealed under a destination passphrase, and this destination has none")
	}
	_, inner, err := OpenArchive(payload, passphrase, "")
	if err != nil {
		return nil, fmt.Errorf("the destination passphrase does not open this copy, or it is damaged: %w", err)
	}
	return inner, nil
}

// fetchRemotePayload returns the encrypted payload as this machine would hold
// it, unwrapping a destination's outer passphrase when one was used.
func (s *Service) fetchRemotePayload(ctx context.Context, r runtimeRemote, ref CopyRef) ([]byte, error) {
	cl, err := newS3Client(r.S3)
	if err != nil {
		return nil, err
	}
	key := ref.Key
	if ref.Layout == LayoutPair && strings.HasSuffix(key, ".json") {
		key = strings.TrimSuffix(key, ".json") + ".sealed"
	}
	data, err := cl.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if ref.Layout != LayoutObject {
		return data, nil
	}
	m, payload, err := DecodeObject(data)
	if err != nil {
		return nil, err
	}
	if m.PassphraseSealed {
		payload, err = openOuter(payload, r.S3.Passphrase)
		if err != nil {
			return nil, err
		}
	}
	return payload, nil
}

// findRemoteRef lists one destination and returns the copy with this id.
func (s *Service) findRemoteRef(ctx context.Context, r runtimeRemote, ownerID int64, id string) (CopyRef, bool) {
	cl, err := newS3Client(r.S3)
	if err != nil {
		return CopyRef{}, false
	}
	objects, err := cl.List(ctx, listingPrefix(r.S3.Prefix, scopeOf(ownerID), ownerID))
	if err != nil {
		return CopyRef{}, false
	}
	for _, ref := range DiscoverCopies(strings.Trim(r.S3.Prefix, "/"), objects) {
		if ref.Name == id {
			return ref, true
		}
	}
	return CopyRef{}, false
}

// fetchRemoteSealed finds a copy at any enabled destination and returns its
// encrypted payload in the form this machine keeps.
func (s *Service) fetchRemoteSealed(ctx context.Context, ownerID int64, id string) ([]byte, error) {
	remotes, err := s.loadRemotes(ownerID)
	if err != nil {
		return nil, err
	}
	for _, r := range remotes {
		if r.Disabled || r.Problem != "" || !r.S3.Configured() {
			continue
		}
		ref, ok := s.findRemoteRef(ctx, r, ownerID, id)
		if !ok {
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		data, err := s.fetchRemotePayload(rctx, r, ref)
		cancel()
		if err == nil {
			return data, nil
		}
	}
	return nil, os.ErrNotExist
}
