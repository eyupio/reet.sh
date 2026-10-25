package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"time"
)

const (
	keysLife  = time.Hour
	keysRetry = time.Minute
)

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`

	pub crypto.PublicKey
}

func b64(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

// parse turns the JSON members into a public key, or says why it cannot.
func (k *jwk) parse() error {
	switch k.Kty {
	case "RSA":
		n, err := b64(k.N)
		if err != nil || len(n) < 256 { // under 2048 bits
			return errors.New("an RSA key is too short or malformed")
		}
		e, err := b64(k.E)
		if err != nil || len(e) == 0 || len(e) > 4 {
			return errors.New("an RSA exponent is malformed")
		}
		exp := 0
		for _, b := range e {
			exp = exp<<8 | int(b)
		}
		k.pub = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp}
	case "EC":
		if k.Crv != "P-256" {
			return errors.New("an EC key is not on P-256")
		}
		x, errx := b64(k.X)
		y, erry := b64(k.Y)
		if errx != nil || erry != nil {
			return errors.New("an EC key is malformed")
		}
		pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !pub.Curve.IsOnCurve(pub.X, pub.Y) {
			return errors.New("an EC key is not on its curve")
		}
		k.pub = pub
	default:
		return errors.New("unsupported key type")
	}
	return nil
}

// key finds the signing key a token names. An unknown kid makes one refetch
// of the key set, at most once a minute, so a rotation is picked up without a
// stranger being able to make this server call out on demand.
func (p *Provider) key(ctx context.Context, kid, alg string) (*jwk, error) {
	p.mu.Lock()
	known := p.keys
	fresh := known != nil && p.now().Sub(p.keysAt) < keysLife
	p.mu.Unlock()
	if fresh {
		if k := pick(known, kid, alg); k != nil {
			return k, nil
		}
	}
	if err := p.refreshKeys(ctx); err != nil {
		if k := pick(known, kid, alg); k != nil {
			return k, nil
		}
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if k := pick(p.keys, kid, alg); k != nil {
		return k, nil
	}
	return nil, errors.New("the token is signed with a key the provider does not publish")
}

func pick(keys map[string]jwk, kid, alg string) *jwk {
	want := "RSA"
	if alg == "ES256" {
		want = "EC"
	}
	if kid != "" {
		if k, ok := keys[kid]; ok && k.Kty == want && (k.Alg == "" || k.Alg == alg) {
			return &k
		}
		return nil
	}
	// A token with no kid is acceptable only when exactly one key could have
	// signed it.
	var found *jwk
	for _, k := range keys {
		if k.Kty == want && (k.Alg == "" || k.Alg == alg) {
			if found != nil {
				return nil
			}
			c := k
			found = &c
		}
	}
	return found
}

func (p *Provider) refreshKeys(ctx context.Context) error {
	d, err := p.discover(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	now := p.now()
	if !p.keysTry.IsZero() && now.Sub(p.keysTry) < keysRetry {
		p.mu.Unlock()
		return errors.New("the signing keys were fetched a moment ago")
	}
	p.keysTry = now
	p.mu.Unlock()
	body, err := p.get(ctx, d.JWKSURI)
	if err != nil {
		return err
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return errors.New("the provider's key set is not JSON")
	}
	keys := map[string]jwk{}
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		if err := k.parse(); err != nil {
			continue // one bad key must not hide the good ones
		}
		keys[k.Kid] = k
	}
	if len(keys) == 0 {
		return errors.New("the provider publishes no usable signing key")
	}
	p.mu.Lock()
	p.keys, p.keysAt = keys, now
	p.mu.Unlock()
	return nil
}
