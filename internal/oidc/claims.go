package oidc

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"strings"
)

// Claims is what a sign-in learns about a person.
type Claims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	// Groups is what the groups claim held, and HasGroups says whether the
	// claim was there at all: a token with no groups claim must leave an
	// existing person's role alone, where an empty list means "no group".
	Groups    []string
	HasGroups bool
	SessionID string
}

func (p *Provider) claimsFrom(v *verified) (Claims, error) {
	sub := v.str("sub")
	if sub == "" || len(sub) > 255 {
		return Claims{}, errors.New("the token has no usable subject")
	}
	c := Claims{Subject: sub, SessionID: v.str("sid")}
	c.Email = strings.TrimSpace(v.str(p.cfg.EmailClaim))
	if raw, ok := v.claims["email_verified"]; ok {
		var b bool
		var s string
		switch {
		case json.Unmarshal(raw, &b) == nil:
			c.EmailVerified = b
		case json.Unmarshal(raw, &s) == nil:
			c.EmailVerified = strings.EqualFold(s, "true")
		}
	}
	for _, name := range []string{p.cfg.NameClaim, "name", "preferred_username"} {
		if n := strings.TrimSpace(v.str(name)); n != "" {
			c.Name = n
			break
		}
	}
	if c.Name == "" && c.Email != "" {
		c.Name = strings.SplitN(c.Email, "@", 2)[0]
	}
	if raw, ok := v.claims[p.cfg.GroupsClaim]; ok {
		c.HasGroups = true
		var list []string
		var one string
		switch {
		case json.Unmarshal(raw, &list) == nil:
			c.Groups = list
		case json.Unmarshal(raw, &one) == nil && one != "":
			c.Groups = strings.FieldsFunc(one, func(r rune) bool { return r == ',' || r == ' ' })
		}
	}
	return c, nil
}

// RoleFor maps a person's groups to a role. The highest of admin, member and
// viewer that any group matches wins, and the owner is never one of them.
// mapped is false when no groups are mapped at all, which tells the caller to
// leave an existing person's role alone and use the default for a new one.
func (p *Provider) RoleFor(groups []string) (role string, mapped bool) {
	if len(p.cfg.AdminGroups)+len(p.cfg.MemberGroups)+len(p.cfg.ViewerGroups) == 0 {
		return p.cfg.DefaultRole, false
	}
	has := func(list []string) bool {
		for _, want := range list {
			for _, g := range groups {
				if strings.EqualFold(g, want) {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has(p.cfg.AdminGroups):
		return RoleAdmin, true
	case has(p.cfg.MemberGroups):
		return RoleMember, true
	case has(p.cfg.ViewerGroups):
		return RoleViewer, true
	}
	// A typo in a group name gives read-only, never more.
	return p.cfg.DefaultRole, true
}

func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
