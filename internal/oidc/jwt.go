package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"
)

// leeway is the clock difference tolerated between this server and the
// provider.
const leeway = 2 * time.Minute

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

// verified is a token whose signature has been checked.
type verified struct {
	claims map[string]json.RawMessage
}

// verify checks a compact JWT's signature against the provider's published
// keys. Only RS256 and ES256 are accepted: "none" and the HMAC family are
// refused outright, since a symmetric algorithm checked against a public key
// is the classic way to forge a token.
func (p *Provider) verify(ctx context.Context, raw string) (*verified, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || len(raw) > 16<<10 {
		return nil, errors.New("the token is not a signed JWT")
	}
	hb, err := b64(parts[0])
	if err != nil {
		return nil, errors.New("the token header is malformed")
	}
	var h jwtHeader
	if err := json.Unmarshal(hb, &h); err != nil {
		return nil, errors.New("the token header is malformed")
	}
	if h.Alg != "RS256" && h.Alg != "ES256" {
		return nil, errors.New("the token uses a signing algorithm this server does not accept")
	}
	sig, err := b64(parts[2])
	if err != nil {
		return nil, errors.New("the token signature is malformed")
	}
	key, err := p.key(ctx, h.Kid, h.Alg)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	switch pub := key.pub.(type) {
	case *rsa.PublicKey:
		if h.Alg != "RS256" || rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
			return nil, errors.New("the token signature does not verify")
		}
	case *ecdsa.PublicKey:
		if h.Alg != "ES256" || len(sig) != 64 ||
			!ecdsa.Verify(pub, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
			return nil, errors.New("the token signature does not verify")
		}
	default:
		return nil, errors.New("the token signature does not verify")
	}
	pb, err := b64(parts[1])
	if err != nil {
		return nil, errors.New("the token payload is malformed")
	}
	var claims map[string]json.RawMessage
	if err := json.Unmarshal(pb, &claims); err != nil {
		return nil, errors.New("the token payload is malformed")
	}
	return &verified{claims: claims}, nil
}

func (v *verified) str(name string) string {
	var s string
	if raw, ok := v.claims[name]; ok {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}

func (v *verified) number(name string) (int64, bool) {
	raw, ok := v.claims[name]
	if !ok {
		return 0, false
	}
	var f float64
	if json.Unmarshal(raw, &f) != nil {
		return 0, false
	}
	return int64(f), true
}

func (v *verified) has(name string) bool {
	_, ok := v.claims[name]
	return ok
}

// audience returns the aud claim, which may be one string or a list.
func (v *verified) audience() []string {
	raw, ok := v.claims["aud"]
	if !ok {
		return nil
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}
	}
	var many []string
	_ = json.Unmarshal(raw, &many)
	return many
}

// checkCommon holds what every token this package accepts must satisfy:
// issuer, audience (and authorised party when there are several), and a
// lifetime that has not ended.
func (p *Provider) checkCommon(v *verified, requireExp bool) error {
	if v.str("iss") != p.cfg.Issuer {
		return errors.New("the token was issued by a different issuer")
	}
	aud := v.audience()
	found := false
	for _, a := range aud {
		if a == p.cfg.ClientID {
			found = true
		}
	}
	if !found {
		return errors.New("the token is for a different client")
	}
	if len(aud) > 1 && v.str("azp") != p.cfg.ClientID {
		return errors.New("the token's authorised party is not this client")
	}
	now := p.now()
	if exp, ok := v.number("exp"); ok {
		if now.After(time.Unix(exp, 0).Add(leeway)) {
			return errors.New("the token has expired")
		}
	} else if requireExp {
		return errors.New("the token has no expiry")
	}
	if nbf, ok := v.number("nbf"); ok && now.Add(leeway).Before(time.Unix(nbf, 0)) {
		return errors.New("the token is not valid yet")
	}
	if iat, ok := v.number("iat"); ok && now.Add(leeway).Before(time.Unix(iat, 0)) {
		return errors.New("the token was issued in the future")
	}
	return nil
}
