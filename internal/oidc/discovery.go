package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	discoveryLife  = time.Hour
	discoveryRetry = 30 * time.Second
)

type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// ErrUnreachable is returned when the provider cannot be reached or does not
// describe itself properly. A sign-in that gets it can be tried again later.
var ErrUnreachable = errors.New("the identity provider is not available")

// discover returns the provider's endpoints, fetched at most once an hour
// and, after a failure, at most once every half minute.
func (p *Provider) discover(ctx context.Context) (*discovery, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if p.disc != nil && now.Sub(p.discAt) < discoveryLife {
		return p.disc, nil
	}
	if !p.discTry.IsZero() && now.Sub(p.discTry) < discoveryRetry {
		if p.disc != nil {
			return p.disc, nil
		}
		return nil, ErrUnreachable
	}
	p.discTry = now
	d, err := p.fetchDiscovery(ctx)
	if err != nil {
		if p.disc != nil {
			return p.disc, nil // a stale answer beats none while it is down
		}
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	p.disc, p.discAt = d, now
	return d, nil
}

func (p *Provider) fetchDiscovery(ctx context.Context) (*discovery, error) {
	// The terminating slash is dropped for this request only (OpenID Connect
	// Discovery 4.1); the issuer itself is compared exactly as configured.
	body, err := p.get(ctx, strings.TrimRight(p.cfg.Issuer, "/")+"/.well-known/openid-configuration")
	if err != nil {
		return nil, err
	}
	var d discovery
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, errors.New("the discovery document is not JSON")
	}
	// The document must name the issuer it was fetched for. A provider that
	// names another is either misconfigured or being impersonated, and tokens
	// are checked against the configured issuer in either case.
	if d.Issuer != p.cfg.Issuer {
		return nil, errors.New("the discovery document names a different issuer")
	}
	for name, v := range map[string]string{
		"authorization_endpoint": d.AuthorizationEndpoint,
		"token_endpoint":         d.TokenEndpoint,
		"jwks_uri":               d.JWKSURI,
	} {
		if v == "" {
			return nil, fmt.Errorf("the discovery document has no %s", name)
		}
		if err := checkURL(v, name); err != nil {
			return nil, err
		}
	}
	return &d, nil
}

// Check says whether the provider can be reached and describes itself
// properly, for a status page. It uses the same cached answer a sign-in would.
func (p *Provider) Check(ctx context.Context) error {
	_, err := p.discover(ctx)
	return err
}
