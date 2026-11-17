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
	// Read is the choices these claims were read under. A sign-in uses it for
	// everything that follows, so an owner saving new settings in between
	// cannot pair claims read by one set of names with another set's roles.
	Read Behaviour
}

func (p *Provider) claimsFrom(v *verified) (Claims, error) {
	sub := v.str("sub")
	if sub == "" || len(sub) > 255 {
		return Claims{}, errors.New("the token has no usable subject")
	}
	b := p.snapshot()
	c := Claims{Subject: sub, SessionID: v.str("sid"), Read: b.normalised()}
	c.Email = strings.TrimSpace(v.str(b.EmailClaimName()))
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
	for _, name := range []string{b.NameClaimName(), "name", "preferred_username"} {
		if n := strings.TrimSpace(v.str(name)); n != "" {
			c.Name = n
			break
		}
	}
	if c.Name == "" && c.Email != "" {
		c.Name = strings.SplitN(c.Email, "@", 2)[0]
	}
	if raw, ok := v.claims[b.GroupsClaimName()]; ok {
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

// RoleFor maps a person's groups to a role by the choices now in force; see
// Behaviour.RoleFor.
func (p *Provider) RoleFor(groups []string) (role string, mapped bool) {
	return p.snapshot().RoleFor(groups)
}

func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
