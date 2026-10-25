// Package oidc signs people in through an OpenID Connect identity provider.
//
// It is a client of the provider: it never issues identities. Everything is
// written with the standard library so the binary stays small, and it accepts
// only what a sign-in needs: the authorisation-code flow with PKCE, ID tokens
// signed RS256 or ES256, and back-channel logout.
package oidc

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Secret is a value that must not reach a log line or an error.
type Secret string

func (Secret) String() string   { return "[redacted]" }
func (Secret) GoString() string { return "[redacted]" }

// Roles an identity provider's groups can map to. The owner is never one of
// them: ownership is only ever given by setup and by transfer.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"
)

// Config is what the environment says about single sign-on.
type Config struct {
	Issuer       string
	ClientID     string
	ClientSecret Secret
	RedirectURL  string
	Scopes       []string
	NameClaim    string
	EmailClaim   string
	GroupsClaim  string
	AdminGroups  []string
	MemberGroups []string
	ViewerGroups []string
	// DefaultRole is given to a new person who matches no group, and to
	// anyone when groups are mapped but none matches.
	DefaultRole   string
	AutoCreate    bool
	LinkByEmail   bool
	PasswordLogin bool
	ButtonLabel   string
}

// readFile is os.ReadFile, replaced in tests.
var readFile = os.ReadFile

// lookup reads NAME or the file NAME_FILE names, refusing both at once.
func lookup(getenv func(string) string, name string) (string, error) {
	v, file := getenv(name), getenv(name+"_FILE")
	switch {
	case v != "" && file != "":
		return "", fmt.Errorf("both %s and %s_FILE are set; set one", name, name)
	case file != "":
		raw, err := readFile(file)
		if err != nil {
			return "", fmt.Errorf("reading %s_FILE: %w", name, err)
		}
		return strings.TrimSpace(string(raw)), nil
	}
	return strings.TrimSpace(v), nil
}

// FromEnv reads the REET_OIDC_* settings. It returns a nil provider and no
// error when single sign-on is not configured or is switched off, and an
// error that names what to fix when it is configured badly: a half-set
// configuration must not start up looking like no single sign-on at all.
func FromEnv(getenv func(string) string) (*Provider, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if off := strings.ToLower(strings.TrimSpace(getenv("REET_OIDC"))); off == "off" || off == "false" || off == "0" {
		return nil, nil
	}
	var errs []string
	str := func(name string) string {
		v, err := lookup(getenv, name)
		if err != nil {
			errs = append(errs, err.Error())
		}
		return v
	}
	list := func(name, def string) []string {
		v := str(name)
		if v == "" {
			v = def
		}
		var out []string
		for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
			if f != "" {
				out = append(out, f)
			}
		}
		return out
	}
	flag := func(name string, def bool) bool {
		switch strings.ToLower(str(name)) {
		case "":
			return def
		case "1", "true", "on", "yes":
			return true
		case "0", "false", "off", "no":
			return false
		}
		errs = append(errs, name+" must be on or off")
		return def
	}
	cfg := Config{
		Issuer:        strings.TrimRight(str("REET_OIDC_ISSUER"), "/"),
		ClientID:      str("REET_OIDC_CLIENT_ID"),
		ClientSecret:  Secret(str("REET_OIDC_CLIENT_SECRET")),
		RedirectURL:   str("REET_OIDC_REDIRECT_URL"),
		Scopes:        list("REET_OIDC_SCOPES", "openid profile email"),
		NameClaim:     str("REET_OIDC_NAME_CLAIM"),
		EmailClaim:    str("REET_OIDC_EMAIL_CLAIM"),
		GroupsClaim:   str("REET_OIDC_GROUPS_CLAIM"),
		AdminGroups:   list("REET_OIDC_ADMIN_GROUPS", ""),
		MemberGroups:  list("REET_OIDC_MEMBER_GROUPS", ""),
		ViewerGroups:  list("REET_OIDC_VIEWER_GROUPS", ""),
		DefaultRole:   strings.ToLower(str("REET_OIDC_DEFAULT_ROLE")),
		AutoCreate:    flag("REET_OIDC_AUTO_CREATE", false),
		LinkByEmail:   flag("REET_OIDC_LINK_BY_EMAIL", false),
		PasswordLogin: flag("REET_OIDC_PASSWORD_LOGIN", true),
		ButtonLabel:   str("REET_OIDC_BUTTON_LABEL"),
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("single sign-on: %s", strings.Join(errs, "; "))
	}
	if cfg.Issuer == "" && cfg.ClientID == "" && cfg.ClientSecret == "" {
		return nil, nil
	}
	if cfg.RedirectURL == "" {
		if base := strings.TrimRight(strings.TrimSpace(getenv("REET_PUBLIC_URL")), "/"); base != "" {
			cfg.RedirectURL = base + "/api/v2/auth/oidc/callback"
		}
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("single sign-on: %w", err)
	}
	return newProvider(cfg), nil
}

func (c *Config) validate() error {
	var errs []string
	if c.Issuer == "" {
		errs = append(errs, "REET_OIDC_ISSUER is required")
	} else if err := checkURL(c.Issuer, "REET_OIDC_ISSUER"); err != nil {
		errs = append(errs, err.Error())
	}
	if c.ClientID == "" {
		errs = append(errs, "REET_OIDC_CLIENT_ID is required")
	}
	if c.ClientSecret == "" {
		errs = append(errs, "REET_OIDC_CLIENT_SECRET (or _FILE) is required")
	}
	if c.RedirectURL == "" {
		errs = append(errs, "set REET_OIDC_REDIRECT_URL, or REET_PUBLIC_URL so the callback address can be worked out")
	} else if err := checkURL(c.RedirectURL, "REET_OIDC_REDIRECT_URL"); err != nil {
		errs = append(errs, err.Error())
	}
	switch c.DefaultRole {
	case "":
		c.DefaultRole = RoleViewer
	case RoleMember, RoleViewer:
	default:
		errs = append(errs, "REET_OIDC_DEFAULT_ROLE must be member or viewer")
	}
	hasOpenID := false
	for _, s := range c.Scopes {
		if s == "openid" {
			hasOpenID = true
		}
	}
	if !hasOpenID {
		c.Scopes = append([]string{"openid"}, c.Scopes...)
	}
	if c.NameClaim == "" {
		c.NameClaim = "name"
	}
	if c.EmailClaim == "" {
		c.EmailClaim = "email"
	}
	if c.GroupsClaim == "" {
		c.GroupsClaim = "groups"
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// checkURL accepts an https URL, or http only on this machine, with no
// credentials or fragment in it.
func checkURL(raw, name string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%s is not a URL", name)
	}
	if u.User != nil || u.Fragment != "" {
		return fmt.Errorf("%s must not carry credentials or a fragment", name)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
	}
	return fmt.Errorf("%s must be an https URL (http is allowed only for localhost)", name)
}
