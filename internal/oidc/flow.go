package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Random returns n random bytes as URL-safe text, for state, nonce and the
// PKCE verifier.
func Random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("oidc: no randomness: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Challenge is the PKCE S256 challenge for a verifier.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthURL is where to send the person to sign in.
func (p *Provider) AuthURL(ctx context.Context, state, nonce, verifier string) (string, error) {
	d, err := p.discover(ctx)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(d.AuthorizationEndpoint)
	if err != nil {
		return "", ErrUnreachable
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", p.cfg.ClientID)
	q.Set("redirect_uri", p.cfg.RedirectURL)
	q.Set("scope", strings.Join(p.cfg.Scopes, " "))
	q.Set("state", state)
	q.Set("nonce", nonce)
	q.Set("code_challenge", Challenge(verifier))
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Exchange spends an authorisation code and returns the person it signed in.
// The token endpoint's own error text is never passed on: it can echo what
// was sent.
func (p *Provider) Exchange(ctx context.Context, code, verifier, nonce string) (Claims, error) {
	d, err := p.discover(ctx)
	if err != nil {
		return Claims{}, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.cfg.RedirectURL},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Claims{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	// client_secret_basic: id and secret are form-encoded, then base64'd
	// (RFC 6749 section 2.3.1).
	req.SetBasicAuth(url.QueryEscape(p.cfg.ClientID), url.QueryEscape(string(p.cfg.ClientSecret)))
	resp, err := p.client.Do(req)
	if err != nil {
		return Claims{}, ErrUnreachable
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return Claims{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Claims{}, errors.New("the identity provider refused the sign-in")
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.IDToken == "" {
		return Claims{}, errors.New("the identity provider returned no ID token")
	}
	v, err := p.verify(ctx, tok.IDToken)
	if err != nil {
		return Claims{}, err
	}
	if err := p.checkCommon(v, true); err != nil {
		return Claims{}, err
	}
	if nonce == "" || !equal(v.str("nonce"), nonce) {
		return Claims{}, errors.New("the ID token's nonce does not match this sign-in")
	}
	return p.claimsFrom(v)
}
