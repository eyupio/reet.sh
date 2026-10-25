package oidc

import (
	"net/http"
	"sync"
	"time"
)

// Provider is one configured identity provider and what has been learned
// about it: its endpoints, its signing keys and the logout tokens already
// seen.
type Provider struct {
	cfg    Config
	client *http.Client
	now    func() time.Time

	mu        sync.Mutex
	disc      *discovery
	discAt    time.Time
	discTry   time.Time
	keys      map[string]jwk
	keysAt    time.Time
	keysTry   time.Time
	seenLogin map[string]time.Time
}

func newProvider(cfg Config) *Provider {
	return &Provider{cfg: cfg, client: newHTTPClient(), now: time.Now, seenLogin: map[string]time.Time{}}
}

// Config returns the settings, with the secret redacted by its type.
func (p *Provider) Config() Config { return p.cfg }

// Label is the text on the sign-in button.
func (p *Provider) Label() string {
	if p.cfg.ButtonLabel != "" {
		return p.cfg.ButtonLabel
	}
	return "Sign in with single sign-on"
}

// RedirectURL is the address the provider must be told to send people back to.
func (p *Provider) RedirectURL() string { return p.cfg.RedirectURL }

// PasswordLogin says whether the sign-in page still offers a password.
func (p *Provider) PasswordLogin() bool { return p.cfg.PasswordLogin }

// AutoCreate and LinkByEmail say what happens to a person with no account
// here yet.
func (p *Provider) AutoCreate() bool  { return p.cfg.AutoCreate }
func (p *Provider) LinkByEmail() bool { return p.cfg.LinkByEmail }
func (p *Provider) Issuer() string    { return p.cfg.Issuer }
