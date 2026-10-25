package oidc

import (
	"context"
	"errors"
	"testing"
)

func TestCheckSaysWhetherTheProviderDescribesItself(t *testing.T) {
	idp := newFakeIDP(t)
	p, _ := idp.provider(nil)
	if err := p.Check(context.Background()); err != nil {
		t.Fatalf("a reachable provider: %v", err)
	}
	idp.discoveryIssuer = "https://someone-else.example"
	down, _ := idp.provider(nil)
	if err := down.Check(context.Background()); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a provider describing another issuer: %v", err)
	}
}
