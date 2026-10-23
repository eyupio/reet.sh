// Package privateconn adapts Tailcat to reet's private agent connection.
// It does not expose an application handler or grant access to a shell.
package privateconn

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/tailscale/tailcat"
	"tailscale.com/tailcfg"
)

const (
	identityFormat = "reet-tailcat-v1"
	setupTimeout   = 20 * time.Second
	maxIdentity    = 64 << 10
)

var (
	ErrIdentity    = errors.New("private connection identity is unavailable or invalid")
	ErrUnavailable = errors.New("private connection unavailable")
)

// Identity contains secret key material, including the connection capability.
// Persist it only with Seal; it must not be included in API responses or logs.
type Identity struct {
	key *tailcat.PrivateKey
}

func (*Identity) String() string   { return "[private connection identity]" }
func (*Identity) GoString() string { return "[private connection identity]" }

// NewIdentity discovers a relay with bounded setup work. An empty relayMapURL
// uses Tailcat's default; a configured map must use HTTPS without credentials.
func NewIdentity(ctx context.Context, relayMapURL string) (*Identity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if relayMapURL != "" {
		u, err := url.Parse(relayMapURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return nil, ErrIdentity
		}
	}
	ctx, cancel := context.WithTimeout(ctx, setupTimeout)
	defer cancel()
	info := &tailcat.ConnInfo{RegionID: -1}
	opts := []any{tailcat.ExpandForServer}
	if relayMapURL != "" {
		opts = append(opts, tailcat.DERPMapURL(relayMapURL))
	}
	if err := info.Expand(ctx, opts...); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	if len(info.Region) == 0 {
		return nil, ErrUnavailable
	}
	return identityForRegion(info.Region[0])
}

func identityForRegion(region *tailcfg.DERPRegion) (*Identity, error) {
	i := &Identity{key: tailcat.NewPrivateKey()}
	i.key.Public.Region = []*tailcfg.DERPRegion{region}
	if !i.valid() {
		return nil, ErrIdentity
	}
	return i, nil
}

func (i *Identity) valid() bool {
	return i != nil && i.key != nil && !i.key.Private.IsZero() &&
		i.key.Public.ServerPublic.NodePublic == i.key.Private.Public() &&
		!i.key.Public.ServerDiscoPublic.IsZero() &&
		!i.key.Public.PresharedKey.IsZero() &&
		len(i.key.Public.Region) == 1 && i.key.Public.Region[0] != nil &&
		i.key.Public.Region[0].RegionID != 0 && len(i.key.Public.Region[0].Nodes) != 0
}

type identityEnvelope struct {
	Format string              `json:"format"`
	Key    *tailcat.PrivateKey `json:"key"`
}

// Seal encrypts the identity using reet's existing instance sealing key.
func (i *Identity) Seal(sealer *sealed.Sealer) (string, error) {
	if sealer == nil || !i.valid() {
		return "", ErrIdentity
	}
	b, err := json.Marshal(identityEnvelope{Format: identityFormat, Key: i.key})
	if err != nil || len(b) > maxIdentity/2 {
		return "", ErrIdentity
	}
	result, err := sealer.SealSecret(string(b))
	if err != nil {
		return "", ErrIdentity
	}
	return result, nil
}

// OpenIdentity restores an identity without replacing an unreadable one.
func OpenIdentity(sealer *sealed.Sealer, ciphertext string) (*Identity, error) {
	if sealer == nil || ciphertext == "" || len(ciphertext) > maxIdentity {
		return nil, ErrIdentity
	}
	raw, err := sealer.OpenSecret(ciphertext)
	if err != nil {
		return nil, ErrIdentity
	}
	var envelope identityEnvelope
	if json.Unmarshal([]byte(raw), &envelope) != nil || envelope.Format != identityFormat {
		return nil, ErrIdentity
	}
	i := &Identity{key: envelope.Key}
	if !i.valid() {
		return nil, ErrIdentity
	}
	return i, nil
}
