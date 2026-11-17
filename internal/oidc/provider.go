package oidc

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Provider is one configured identity provider and what has been learned
// about it: its endpoints, its signing keys and the logout tokens already
// seen.
type Provider struct {
	// cfg holds what stays in the environment: issuer, client, secret and
	// redirect. Its behaviour fields are zeroed; beh is the one place they live.
	cfg    Config
	beh    atomic.Pointer[Behaviour]
	envSet map[string]bool // REET_OIDC_* switches that were set when this started
	client *http.Client
	now    func() time.Time

	mu        sync.Mutex
	disc      *discovery
	discAt    time.Time
	discTry   time.Time
	keys      map[string]jwk
	keysAt    time.Time
	keysTry   time.Time
	keysGen   uint64     // counts successful key fetches
	fetchMu   sync.Mutex // one key fetch at a time
	seenLogin map[string]time.Time
}

func newProvider(cfg Config) *Provider {
	p := &Provider{cfg: cfg, client: newHTTPClient(), now: time.Now, seenLogin: map[string]time.Time{}}
	b := behaviourFromConfig(cfg)
	p.beh.Store(&b)
	p.cfg.Scopes, p.cfg.NameClaim, p.cfg.EmailClaim, p.cfg.GroupsClaim = nil, "", "", ""
	p.cfg.AdminGroups, p.cfg.MemberGroups, p.cfg.ViewerGroups = nil, nil, nil
	p.cfg.DefaultRole, p.cfg.AutoCreate, p.cfg.LinkByEmail, p.cfg.PasswordLogin, p.cfg.ButtonLabel = "", false, false, false, ""
	return p
}

// Config returns the settings, with the secret redacted by its type. The
// behaviour fields are those in force now.
func (p *Provider) Config() Config {
	c, b := p.cfg, p.snapshot()
	c.Scopes, c.NameClaim, c.EmailClaim, c.GroupsClaim = b.Scopes, b.NameClaim, b.EmailClaim, b.GroupsClaim
	c.AdminGroups, c.MemberGroups, c.ViewerGroups = b.AdminGroups, b.MemberGroups, b.ViewerGroups
	c.DefaultRole, c.AutoCreate, c.LinkByEmail, c.PasswordLogin, c.ButtonLabel = b.DefaultRole, b.AutoCreate, b.LinkByEmail, b.PasswordLogin, b.Label
	return c
}

// Label is the text on the sign-in button.
func (p *Provider) Label() string { return p.snapshot().ResolvedLabel() }

// RedirectURL is the address the provider must be told to send people back to.
func (p *Provider) RedirectURL() string { return p.cfg.RedirectURL }

// PasswordLogin says whether the sign-in page still offers a password.
func (p *Provider) PasswordLogin() bool { return p.snapshot().PasswordLogin }

// AutoCreate and LinkByEmail say what happens to a person with no account
// here yet.
func (p *Provider) AutoCreate() bool  { return p.snapshot().AutoCreate }
func (p *Provider) LinkByEmail() bool { return p.snapshot().LinkByEmail }
func (p *Provider) Issuer() string    { return p.cfg.Issuer }
