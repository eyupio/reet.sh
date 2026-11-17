package oidc

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// memStore is the settings table, in memory.
type memStore struct {
	m       map[string]string
	getErr  error
	setErrs int
}

func (s *memStore) GetSetting(k string) (string, error) { return s.m[k], s.getErr }
func (s *memStore) SetSetting(k, v string) error {
	if s.setErrs > 0 {
		s.setErrs--
		return errors.New("disk full")
	}
	s.m[k] = v
	return nil
}

func newMem() *memStore { return &memStore{m: map[string]string{}} }

type logs struct{ lines []string }

func (l *logs) logf(f string, a ...any) { l.lines = append(l.lines, fmt.Sprintf(f, a...)) }
func (l *logs) all() string             { return strings.Join(l.lines, "\n") }

func TestTheFirstStartStoresWhatTheEnvironmentSaid(t *testing.T) {
	e := goodEnv()
	e["REET_OIDC_AUTO_CREATE"], e["REET_OIDC_DEFAULT_ROLE"], e["REET_OIDC_ADMIN_GROUPS"] = "on", "member", "ops, root"
	p, err := FromEnv(env(e))
	if err != nil {
		t.Fatal(err)
	}
	st, l := newMem(), &logs{}
	p.UseStoredBehaviour(st, l.logf)
	got, err := DecodeBehaviour(st.m[SettingKey])
	if err != nil {
		t.Fatalf("nothing usable was stored: %v (%q)", err, st.m[SettingKey])
	}
	if !got.AutoCreate || got.DefaultRole != "member" || strings.Join(got.AdminGroups, ",") != "ops,root" || !got.PasswordLogin {
		t.Fatalf("stored = %+v", got)
	}
	if !strings.Contains(l.all(), "stored from the environment") {
		t.Fatalf("the seeding was not logged: %q", l.all())
	}
}

func TestASecondStartKeepsTheStoredChoicesAndNamesTheVariableNotItsValue(t *testing.T) {
	first, _ := FromEnv(env(func() map[string]string {
		e := goodEnv()
		e["REET_OIDC_LINK_BY_EMAIL"] = "on"
		return e
	}()))
	st := newMem()
	first.UseStoredBehaviour(st, (&logs{}).logf)

	// The owner turns it off on the admin page; the environment still says on,
	// and says a button label of its own that the stored choice does not.
	b := first.Behaviour()
	b.LinkByEmail = false
	enc, _ := EncodeBehaviour(b)
	st.m[SettingKey] = enc

	e := goodEnv()
	e["REET_OIDC_LINK_BY_EMAIL"], e["REET_OIDC_BUTTON_LABEL"] = "on", "Secret-looking label 8f3a"
	second, err := FromEnv(env(e))
	if err != nil {
		t.Fatal(err)
	}
	l := &logs{}
	second.UseStoredBehaviour(st, l.logf)
	if second.LinkByEmail() {
		t.Fatal("the environment overrode the stored choice")
	}
	out := l.all()
	for _, want := range []string{"REET_OIDC_LINK_BY_EMAIL", "REET_OIDC_BUTTON_LABEL"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%s is not named: %q", want, out)
		}
	}
	if strings.Contains(out, "8f3a") || strings.Contains(out, "Secret-looking") {
		t.Fatalf("a value reached the log: %q", out)
	}
	if st.m[SettingKey] != enc {
		t.Fatal("a later start rewrote the stored choices")
	}
}

func TestAVariableThatAgreesWithTheStoredChoiceIsNotMentioned(t *testing.T) {
	e := goodEnv()
	e["REET_OIDC_AUTO_CREATE"] = "on"
	p, _ := FromEnv(env(e))
	st := newMem()
	p.UseStoredBehaviour(st, (&logs{}).logf)
	again, _ := FromEnv(env(e))
	l := &logs{}
	again.UseStoredBehaviour(st, l.logf)
	if l.all() != "" {
		t.Fatalf("a quiet restart logged %q", l.all())
	}
}

func TestABadEnvironmentValueStopsTheStartBeforeAnythingIsStored(t *testing.T) {
	for name, set := range map[string]map[string]string{
		"role":  {"REET_OIDC_DEFAULT_ROLE": "admin"},
		"flag":  {"REET_OIDC_LINK_BY_EMAIL": "maybe"},
		"label": {"REET_OIDC_BUTTON_LABEL": strings.Repeat("x", 101)},
	} {
		e := goodEnv()
		for k, v := range set {
			e[k] = v
		}
		if p, err := FromEnv(env(e)); err == nil || p != nil {
			t.Fatalf("%s: started anyway (%v)", name, err)
		}
	}
}

func TestAnUnusableStoredValueFallsBackToTheEnvironmentAndSaysSo(t *testing.T) {
	p, _ := FromEnv(env(goodEnv()))
	st, l := newMem(), &logs{}
	st.m[SettingKey] = `{"defaultRole":"owner"}`
	p.UseStoredBehaviour(st, l.logf)
	if p.Behaviour().DefaultRole != "viewer" || !strings.Contains(l.all(), "not usable") {
		t.Fatalf("role %q, log %q", p.Behaviour().DefaultRole, l.all())
	}
	if st.m[SettingKey] != `{"defaultRole":"owner"}` {
		t.Fatal("an unusable value was overwritten at start")
	}
}

func okBehaviour() Behaviour {
	return Behaviour{PasswordLogin: true, DefaultRole: "viewer", Scopes: []string{"openid", "profile"}}
}

func TestSetBehaviourRefusesWhatTheEnvironmentWouldRefuse(t *testing.T) {
	p, _ := FromEnv(env(goodEnv()))
	cases := map[string]func(*Behaviour){
		"defaultRole": func(b *Behaviour) { b.DefaultRole = "admin" },
		"scopes":      func(b *Behaviour) { b.Scopes = []string{"profile"} },
		"nameClaim":   func(b *Behaviour) { b.NameClaim = "two words" },
		"adminGroups": func(b *Behaviour) { b.AdminGroups = []string{""} },
		"label":       func(b *Behaviour) { b.Label = "line\nbreak" },
	}
	for field, mutate := range cases {
		b := okBehaviour()
		mutate(&b)
		err := p.SetBehaviour(b)
		var fe *FieldError
		if !errors.As(err, &fe) || fe.Field != field {
			t.Fatalf("%s: %v", field, err)
		}
		if p.Behaviour().DefaultRole != "viewer" || p.Behaviour().Label != "" {
			t.Fatalf("%s: a refused set was applied", field)
		}
	}
	good := okBehaviour()
	good.DefaultRole, good.AutoCreate = "member", true
	if err := p.SetBehaviour(good); err != nil || !p.AutoCreate() || p.Behaviour().DefaultRole != "member" {
		t.Fatalf("a good set: %v", err)
	}
}

func TestChangingTheBehaviourLeavesTheTrustRelationshipAlone(t *testing.T) {
	p, _ := FromEnv(env(goodEnv()))
	before := p.Config()
	b := okBehaviour()
	b.LinkByEmail = true
	if err := p.SetBehaviour(b); err != nil {
		t.Fatal(err)
	}
	after := p.Config()
	if after.Issuer != before.Issuer || after.ClientID != before.ClientID || after.ClientSecret != before.ClientSecret || after.RedirectURL != before.RedirectURL {
		t.Fatalf("the trust relationship moved: %+v -> %+v", before, after)
	}
}

func TestTheBehaviourACallerHoldsIsACopy(t *testing.T) {
	p, _ := FromEnv(env(goodEnv()))
	b := p.Behaviour()
	b.Scopes[0] = "tampered"
	if p.Behaviour().Scopes[0] != "openid" {
		t.Fatal("editing a returned Behaviour changed the one in force")
	}
}
