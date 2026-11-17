package oidc

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Behaviour is how single sign-on treats the people who come through it: who
// is matched or made at a first sign-in, which role they get, and whether
// passwords still work. These are choices about the operator's own people, so
// the owner changes them while the controller runs. The provider's address,
// client and secret are a trust relationship and stay in the environment.
//
// A Behaviour is a plain value. The provider holds one and replaces it as a
// whole, so a sign-in that has read it never sees half of an update.
type Behaviour struct {
	// Label is the stored button text; empty means the default words.
	Label         string `json:"label"`
	PasswordLogin bool   `json:"passwordLogin"`
	AutoCreate    bool   `json:"autoCreate"`
	LinkByEmail   bool   `json:"linkByEmail"`
	DefaultRole   string `json:"defaultRole"`
	// The claim names are stored as given; empty means name, email and groups.
	NameClaim    string   `json:"nameClaim"`
	EmailClaim   string   `json:"emailClaim"`
	GroupsClaim  string   `json:"groupsClaim"`
	Scopes       []string `json:"scopes"`
	AdminGroups  []string `json:"adminGroups"`
	MemberGroups []string `json:"memberGroups"`
	ViewerGroups []string `json:"viewerGroups"`
}

// SettingKey is where the owner's choices live in the settings table: one JSON
// value under one key, so a write is atomic.
const SettingKey = "sso.behaviour"

const (
	// DefaultLabel is the button text when none is set.
	DefaultLabel = "Sign in with single sign-on"

	maxLabel      = 100
	maxClaimName  = 100
	maxScopeLen   = 100
	maxGroupLen   = 200
	maxListLength = 200
)

// ResolvedLabel is the text on the sign-in button.
func (b Behaviour) ResolvedLabel() string {
	if b.Label != "" {
		return b.Label
	}
	return DefaultLabel
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// NameClaimName, EmailClaimName and GroupsClaimName are the claims actually
// read, with the defaults filled in.
func (b Behaviour) NameClaimName() string   { return orDefault(b.NameClaim, "name") }
func (b Behaviour) EmailClaimName() string  { return orDefault(b.EmailClaim, "email") }
func (b Behaviour) GroupsClaimName() string { return orDefault(b.GroupsClaim, "groups") }

// RoleFor maps a person's groups to a role. The highest of admin, member and
// viewer that any group matches wins, and the owner is never one of them.
// mapped is false when no groups are mapped at all, which tells the caller to
// leave an existing person's role alone and use the default for a new one.
func (b Behaviour) RoleFor(groups []string) (role string, mapped bool) {
	if len(b.AdminGroups)+len(b.MemberGroups)+len(b.ViewerGroups) == 0 {
		return b.DefaultRole, false
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
	case has(b.AdminGroups):
		return RoleAdmin, true
	case has(b.MemberGroups):
		return RoleMember, true
	case has(b.ViewerGroups):
		return RoleViewer, true
	}
	// A typo in a group name gives read-only, never more.
	return b.DefaultRole, true
}

// FieldError is a refused value, with the name of the field it belongs to and
// a plain sentence an owner can act on.
type FieldError struct {
	Field string
	Msg   string
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Msg }

func bad(field, format string, args ...any) error {
	return &FieldError{Field: field, Msg: fmt.Sprintf(format, args...)}
}

// Validate says whether b can be used, and returns the first problem as a
// *FieldError. It is the one rule set for the environment's first read and for
// every later change, so the two cannot drift apart.
func (b Behaviour) Validate() error {
	if utf8.RuneCountInString(b.Label) > maxLabel || hasControl(b.Label) {
		return bad("label", "must be plain text of at most %d characters", maxLabel)
	}
	if b.Label != strings.TrimSpace(b.Label) {
		return bad("label", "must not start or end with a space")
	}
	switch b.DefaultRole {
	case RoleMember, RoleViewer:
	default:
		return bad("defaultRole", "must be member or viewer")
	}
	for _, c := range []struct{ field, v string }{{"nameClaim", b.NameClaim}, {"emailClaim", b.EmailClaim}, {"groupsClaim", b.GroupsClaim}} {
		if c.v == "" {
			continue
		}
		if utf8.RuneCountInString(c.v) > maxClaimName || strings.IndexFunc(c.v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return bad(c.field, "must be one claim name with no spaces, or empty for the default")
		}
	}
	if err := checkList("scopes", b.Scopes, maxScopeLen, isScopeToken); err != nil {
		return err
	}
	hasOpenID := false
	for _, s := range b.Scopes {
		hasOpenID = hasOpenID || s == "openid"
	}
	if !hasOpenID {
		return bad("scopes", "must include openid, or the provider will not send an identity")
	}
	for _, g := range []struct {
		field string
		v     []string
	}{{"adminGroups", b.AdminGroups}, {"memberGroups", b.MemberGroups}, {"viewerGroups", b.ViewerGroups}} {
		if err := checkList(g.field, g.v, maxGroupLen, func(s string) bool { return !hasControl(s) && strings.TrimSpace(s) == s }); err != nil {
			return err
		}
	}
	return nil
}

func checkList(field string, v []string, maxLen int, ok func(string) bool) error {
	if len(v) > maxListLength {
		return bad(field, "can hold at most %d entries", maxListLength)
	}
	for _, s := range v {
		if s == "" {
			return bad(field, "must not contain an empty entry")
		}
		if utf8.RuneCountInString(s) > maxLen || !ok(s) {
			return bad(field, "has an entry that is too long or has characters it cannot hold: %q", clip(s))
		}
	}
	return nil
}

func clip(s string) string {
	if r := []rune(s); len(r) > 20 {
		return string(r[:20]) + "…"
	}
	return s
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

// isScopeToken is RFC 6749's scope-token: printable ASCII without a space,
// a double quote or a backslash.
func isScopeToken(s string) bool {
	for _, r := range s {
		if r < 0x21 || r > 0x7e || r == '"' || r == '\\' {
			return false
		}
	}
	return s != ""
}

// normalised returns b with every list non-nil, so it always reads as [] and
// never as null.
func (b Behaviour) normalised() Behaviour {
	b.Scopes = append([]string{}, b.Scopes...)
	b.AdminGroups = append([]string{}, b.AdminGroups...)
	b.MemberGroups = append([]string{}, b.MemberGroups...)
	b.ViewerGroups = append([]string{}, b.ViewerGroups...)
	return b
}

// Changed names, in the order the interface shows them, the fields on which a
// and b differ.
func (a Behaviour) Changed(b Behaviour) []string {
	var out []string
	add := func(name string, same bool) {
		if !same {
			out = append(out, name)
		}
	}
	add("label", a.Label == b.Label)
	add("passwordLogin", a.PasswordLogin == b.PasswordLogin)
	add("autoCreate", a.AutoCreate == b.AutoCreate)
	add("linkByEmail", a.LinkByEmail == b.LinkByEmail)
	add("defaultRole", a.DefaultRole == b.DefaultRole)
	add("nameClaim", a.NameClaim == b.NameClaim)
	add("emailClaim", a.EmailClaim == b.EmailClaim)
	add("groupsClaim", a.GroupsClaim == b.GroupsClaim)
	add("scopes", sameList(a.Scopes, b.Scopes))
	add("adminGroups", sameList(a.AdminGroups, b.AdminGroups))
	add("memberGroups", sameList(a.MemberGroups, b.MemberGroups))
	add("viewerGroups", sameList(a.ViewerGroups, b.ViewerGroups))
	return out
}

func sameList(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// behaviourFromConfig is what the environment said, as a Behaviour.
func behaviourFromConfig(c Config) Behaviour {
	return Behaviour{
		Label: c.ButtonLabel, PasswordLogin: c.PasswordLogin, AutoCreate: c.AutoCreate, LinkByEmail: c.LinkByEmail,
		DefaultRole: c.DefaultRole, NameClaim: c.NameClaim, EmailClaim: c.EmailClaim, GroupsClaim: c.GroupsClaim,
		Scopes: c.Scopes, AdminGroups: c.AdminGroups, MemberGroups: c.MemberGroups, ViewerGroups: c.ViewerGroups,
	}.normalised()
}

// Behaviour returns the choices now in force. The value is a copy.
func (p *Provider) Behaviour() Behaviour { return p.beh.Load().normalised() }

// snapshot is the choices now in force without a copy, for a request to read
// once and keep. Callers must not change it.
func (p *Provider) snapshot() *Behaviour { return p.beh.Load() }

// SetBehaviour replaces the choices as a whole after checking them with the
// rules the environment's values pass through. It takes effect at the next
// request that reads them and never restarts anything.
func (p *Provider) SetBehaviour(b Behaviour) error {
	if err := b.Validate(); err != nil {
		return err
	}
	n := b.normalised()
	p.beh.Store(&n)
	return nil
}

// SettingsStore is the part of the store that keeps the owner's choices.
type SettingsStore interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

// EncodeBehaviour is the one stored form of the choices.
func EncodeBehaviour(b Behaviour) (string, error) {
	raw, err := json.Marshal(b.normalised())
	return string(raw), err
}

// DecodeBehaviour reads the stored form, refusing anything that would not
// pass Validate or that carries a field this build does not know.
func DecodeBehaviour(raw string) (Behaviour, error) {
	var b Behaviour
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return Behaviour{}, err
	}
	if err := b.Validate(); err != nil {
		return Behaviour{}, err
	}
	return b.normalised(), nil
}

// envBehaviourVars says which environment variable each field was seeded from,
// so a start that finds the environment disagreeing with the stored choice can
// name the variable and nothing else.
var envBehaviourVars = []struct{ field, variable string }{
	{"label", "REET_OIDC_BUTTON_LABEL"},
	{"passwordLogin", "REET_OIDC_PASSWORD_LOGIN"},
	{"autoCreate", "REET_OIDC_AUTO_CREATE"},
	{"linkByEmail", "REET_OIDC_LINK_BY_EMAIL"},
	{"defaultRole", "REET_OIDC_DEFAULT_ROLE"},
	{"nameClaim", "REET_OIDC_NAME_CLAIM"},
	{"emailClaim", "REET_OIDC_EMAIL_CLAIM"},
	{"groupsClaim", "REET_OIDC_GROUPS_CLAIM"},
	{"scopes", "REET_OIDC_SCOPES"},
	{"adminGroups", "REET_OIDC_ADMIN_GROUPS"},
	{"memberGroups", "REET_OIDC_MEMBER_GROUPS"},
	{"viewerGroups", "REET_OIDC_VIEWER_GROUPS"},
}

// UseStoredBehaviour makes the stored choices the ones in force.
//
// The environment seeds once: with nothing stored, what the environment said
// (already checked by FromEnv, which refuses a bad one) is stored. From then
// on the stored choices are the only ones used. A variable that is still set
// and differs from what is stored is named in one log line, never its value,
// so an operator who edits the environment and sees no effect learns why.
//
// A store that cannot be read or written leaves the environment's choices in
// force and is reported through logf, so single sign-on still works.
func (p *Provider) UseStoredBehaviour(st SettingsStore, logf func(format string, args ...any)) {
	raw, err := st.GetSetting(SettingKey)
	if err != nil {
		logf("single sign-on: could not read the stored settings, using the environment's: %v", err)
		return
	}
	seed := p.Behaviour()
	if raw == "" {
		enc, err := EncodeBehaviour(seed)
		if err == nil {
			err = st.SetSetting(SettingKey, enc)
		}
		if err != nil {
			logf("single sign-on: could not store the settings, using the environment's: %v", err)
			return
		}
		logf("single sign-on: settings stored from the environment; from now on change them on the admin page, and the REET_OIDC_* switches for them are ignored")
		return
	}
	stored, err := DecodeBehaviour(raw)
	if err != nil {
		logf("single sign-on: the stored settings are not usable (%v); using the environment's until an owner saves new ones", err)
		return
	}
	p.beh.Store(&stored)
	differs := map[string]bool{}
	for _, f := range stored.Changed(seed) {
		differs[f] = true
	}
	for _, v := range envBehaviourVars {
		if differs[v.field] && p.envSet[v.variable] {
			logf("single sign-on: %s is set and differs from the stored setting; the stored setting is used", v.variable)
		}
	}
}
