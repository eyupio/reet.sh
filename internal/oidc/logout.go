package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	logoutEvent   = "http://schemas.openid.net/event/backchannel-logout"
	jtiLife       = 10 * time.Minute
	maxSeenTokens = 10000
	logoutPast    = 5 * time.Minute
)

// LogoutClaims names whose sessions a logout token ends: all of a subject's,
// or only those begun under one provider session.
type LogoutClaims struct {
	Subject   string
	SessionID string
}

// VerifyLogoutToken checks a back-channel logout token as OpenID Connect
// Back-Channel Logout 1.0 section 2.6 asks, and remembers its jti so the same
// token cannot be played twice.
func (p *Provider) VerifyLogoutToken(ctx context.Context, raw string) (LogoutClaims, error) {
	v, err := p.verify(ctx, raw)
	if err != nil {
		return LogoutClaims{}, err
	}
	if err := p.checkCommon(v, false); err != nil {
		return LogoutClaims{}, err
	}
	if v.has("nonce") {
		return LogoutClaims{}, errors.New("a logout token must not carry a nonce")
	}
	var events map[string]json.RawMessage
	if rawEv, ok := v.claims["events"]; ok {
		_ = json.Unmarshal(rawEv, &events)
	}
	if _, ok := events[logoutEvent]; !ok {
		return LogoutClaims{}, errors.New("the token is not a logout token")
	}
	c := LogoutClaims{Subject: v.str("sub"), SessionID: v.str("sid")}
	if c.Subject == "" && c.SessionID == "" {
		return LogoutClaims{}, errors.New("the logout token names neither a subject nor a session")
	}
	iat, ok := v.number("iat")
	if !ok {
		return LogoutClaims{}, errors.New("the logout token has no issue time")
	}
	now := p.now()
	if now.Sub(time.Unix(iat, 0)) > logoutPast {
		return LogoutClaims{}, errors.New("the logout token is too old")
	}
	jti := v.str("jti")
	if jti == "" {
		return LogoutClaims{}, errors.New("the logout token has no jti")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, at := range p.seenLogin {
		if now.Sub(at) > jtiLife {
			delete(p.seenLogin, k)
		}
	}
	if _, seen := p.seenLogin[jti]; seen {
		return LogoutClaims{}, errors.New("the logout token was already used")
	}
	if len(p.seenLogin) >= maxSeenTokens {
		return LogoutClaims{}, errors.New("too many logout tokens are being tracked; try again shortly")
	}
	p.seenLogin[jti] = now
	return c, nil
}
