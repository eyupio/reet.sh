package api

import (
	"encoding/json"
	"github.com/eyupio/reet.sh/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/oidc"
)

// settingsBody is a whole, valid set; change takes the field to alter.
func settingsBody(t *testing.T, change map[string]any) string {
	t.Helper()
	set := map[string]any{
		"label": "", "passwordLogin": true, "autoCreate": false, "linkByEmail": false, "defaultRole": "viewer",
		"nameClaim": "", "emailClaim": "", "groupsClaim": "", "scopes": []string{"openid", "profile", "email"},
		"adminGroups": []string{}, "memberGroups": []string{}, "viewerGroups": []string{},
	}
	for k, v := range change {
		if v == nil {
			delete(set, k)
		} else {
			set[k] = v
		}
	}
	b, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (e *ssoEnv) ownerClient() *http.Client {
	e.t.Helper()
	c := e.client()
	e.signIn(c, "owner@example.com", ownerPassword)
	return c
}

func (e *ssoEnv) adminClient() *http.Client {
	e.t.Helper()
	e.memberWithPassword("admin@example.com", "admin", "admin password 123")
	c := e.client()
	e.signIn(c, "admin@example.com", "admin password 123")
	return c
}

func (e *ssoEnv) storedSettings() string {
	e.t.Helper()
	v, err := e.st.GetSetting(oidc.SettingKey)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

func TestOnlyTheOwnerCanChangeTheSingleSignOnSettings(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	before := e.storedSettings()
	if code, _ := e.body(e.adminClient(), "PATCH", "/api/v2/admin/sso", settingsBody(t, map[string]any{"autoCreate": true})); code != http.StatusForbidden {
		t.Fatalf("an administrator's change = %d", code)
	}
	if code, _ := e.body(e.client(), "PATCH", "/api/v2/admin/sso", settingsBody(t, nil)); code != http.StatusUnauthorized && code != http.StatusForbidden {
		t.Fatalf("an anonymous change = %d", code)
	}
	if e.storedSettings() != before {
		t.Fatal("a refused change was stored")
	}
	code, body := e.body(e.ownerClient(), "PATCH", "/api/v2/admin/sso", settingsBody(t, map[string]any{"autoCreate": true}))
	if code != 200 || !strings.Contains(body, `"autoCreate":true`) {
		t.Fatalf("the owner's change = %d %s", code, body)
	}
}

func TestTheSettingsAreOnlyTheOwnersWhereSingleSignOnIsOff(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{noSSO: true})
	if code, _ := e.body(e.adminClient(), "PATCH", "/api/v2/admin/sso", settingsBody(t, nil)); code != http.StatusForbidden {
		t.Fatalf("an administrator, single sign-on off = %d (the owner check comes first)", code)
	}
	code, body := e.body(e.ownerClient(), "PATCH", "/api/v2/admin/sso", settingsBody(t, nil))
	if code != http.StatusNotFound || !strings.Contains(body, "single sign-on is not enabled") {
		t.Fatalf("the owner, single sign-on off = %d %s", code, body)
	}
}

func TestAChangeTheOwnerSavesIsObeyedByTheNextSignInWithNoRestart(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	member := e.member("ada@example.com", "member")
	oc := e.ownerClient()

	if loc := e.login(e.client(), ada("sub-ada"), ""); errorCode(loc) != "exists" {
		t.Fatalf("link-by-email off at first = %q", loc)
	}
	if code, body := e.body(oc, "PATCH", "/api/v2/admin/sso", settingsBody(t, map[string]any{"linkByEmail": true})); code != 200 {
		t.Fatalf("turning link-by-email on = %d %s", code, body)
	}
	if loc := e.login(e.client(), ada("sub-ada"), ""); loc != "/" {
		t.Fatalf("after turning it on, the next sign-in = %q", loc)
	}
	if u, ok, _ := e.st.SSOIdentityUser(e.idp.Issuer, "sub-ada"); !ok || u.ID != member.ID {
		t.Fatal("the unlinked member was not linked on the next sign-in")
	}

	other := e.member("bob@example.com", "member")
	if code, _ := e.body(oc, "PATCH", "/api/v2/admin/sso", settingsBody(t, map[string]any{"linkByEmail": false})); code != 200 {
		t.Fatalf("turning it off = %d", code)
	}
	bob := ada("sub-bob")
	bob.Email = "bob@example.com"
	if loc := e.login(e.client(), bob, ""); errorCode(loc) != "exists" {
		t.Fatalf("after turning it off, the next sign-in = %q", loc)
	}
	if _, ok, _ := e.st.SSOIdentityUser(e.idp.Issuer, "sub-bob"); ok {
		t.Fatalf("%s was linked with link-by-email off", other.Email)
	}
}

func TestTheOtherSwitchesAreObeyedByTheNextSignInToo(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	oc := e.ownerClient()
	if loc := e.login(e.client(), ada("sub-ada"), ""); errorCode(loc) != "no_account" {
		t.Fatalf("auto-create off = %q", loc)
	}
	set := settingsBody(t, map[string]any{"autoCreate": true, "defaultRole": "member", "label": "Sign in with Acme", "passwordLogin": false})
	if code, body := e.body(oc, "PATCH", "/api/v2/admin/sso", set); code != 200 {
		t.Fatalf("save = %d %s", code, body)
	}
	if loc := e.login(e.client(), ada("sub-ada"), ""); loc != "/" {
		t.Fatalf("auto-create on = %q", loc)
	}
	u, ok, _ := e.st.SSOIdentityUser(e.idp.Issuer, "sub-ada")
	if !ok || u.Role != "member" {
		t.Fatalf("created %+v, found %v; want a member", u, ok)
	}
	_, body := e.body(e.client(), "GET", "/api/v2/auth/status", "")
	if !strings.Contains(body, `"label":"Sign in with Acme"`) || !strings.Contains(body, `"passwordLogin":false`) {
		t.Fatalf("the sign-in page was not told: %s", body)
	}
}

func TestWithLinkByEmailOnAnOwnerAndAnAdministratorAreStillNotLinkable(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	e.member("admin@example.com", "admin")
	if code, _ := e.body(e.ownerClient(), "PATCH", "/api/v2/admin/sso", settingsBody(t, map[string]any{"linkByEmail": true})); code != 200 {
		t.Fatal("could not turn link-by-email on")
	}
	for _, email := range []string{"owner@example.com", "admin@example.com"} {
		p := ada("sub-" + email)
		p.Email = email
		if loc := e.login(e.client(), p, ""); errorCode(loc) != "not_linkable" {
			t.Fatalf("%s = %q", email, loc)
		}
	}
	// The settings add no way around the other refusal either: a link that
	// already points at the owner is still no way in.
	if err := e.st.LinkSSOIdentity(e.idp.Issuer, "sub-to-owner", e.owner.ID); err != nil {
		t.Fatal(err)
	}
	p := ada("sub-to-owner")
	if loc := e.login(e.client(), p, ""); errorCode(loc) != "not_linkable" {
		t.Fatalf("a link to the owner = %q", loc)
	}
}

func TestARefusedSaveStoresNothingAndNamesTheField(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	oc := e.ownerClient()
	good := settingsBody(t, map[string]any{"autoCreate": true, "label": "Acme"})
	if code, body := e.body(oc, "PATCH", "/api/v2/admin/sso", good); code != 200 {
		t.Fatalf("setup = %d %s", code, body)
	}
	stored := e.storedSettings()
	_, view := e.body(oc, "GET", "/api/v2/admin/sso", "")

	cases := []struct{ name, body, field string }{
		{"a partial body", `{"autoCreate":false}`, "label"},
		{"a blank body", ` `, ""},
		{"an array", `[]`, ""},
		{"null", `null`, ""},
		{"an unknown role", settingsBody(t, map[string]any{"defaultRole": "admin"}), "defaultRole"},
		{"the owner as a role", settingsBody(t, map[string]any{"defaultRole": "owner"}), "defaultRole"},
		{"scopes without openid", settingsBody(t, map[string]any{"scopes": []string{"profile"}}), "scopes"},
		{"scopes that are null", settingsBody(t, map[string]any{"scopes": nil, "x": 1}), ""},
		{"a list that is null", strings.Replace(settingsBody(t, nil), `"adminGroups":[]`, `"adminGroups":null`, 1), "adminGroups"},
		{"a flag that is text", strings.Replace(settingsBody(t, nil), `"autoCreate":false`, `"autoCreate":"yes"`, 1), "autoCreate"},
		{"an unknown field", strings.Replace(settingsBody(t, nil), `{`, `{"clientSecret":"x",`, 1), "clientSecret"},
		{"a claim with a space", settingsBody(t, map[string]any{"groupsClaim": "my groups"}), "groupsClaim"},
		{"an empty group", settingsBody(t, map[string]any{"viewerGroups": []string{"ok", ""}}), "viewerGroups"},
		{"a body over the limit", `{"label":"` + strings.Repeat("x", ssoSettingsLimit) + `"}`, ""},
		// Valid in every field, so only its size can refuse it.
		{"a whole set padded past the limit", settingsBody(t, map[string]any{"autoCreate": true}) + strings.Repeat(" ", ssoSettingsLimit), ""},
	}
	for _, c := range cases {
		code, body := e.body(oc, "PATCH", "/api/v2/admin/sso", c.body)
		if code != http.StatusBadRequest {
			t.Errorf("%s = %d %s", c.name, code, body)
			continue
		}
		var out map[string]string
		_ = json.Unmarshal([]byte(body), &out)
		if c.name == "a partial body" && !strings.Contains(out["error"], "missing") {
			t.Errorf("a partial body is refused for the wrong reason: %s", body)
		}
		if out["error"] == "" || (c.field != "" && out["field"] != c.field) {
			t.Errorf("%s: answer %s, want an error beside %q", c.name, body, c.field)
		}
		if got := e.storedSettings(); got != stored {
			t.Errorf("%s changed what is stored: %s", c.name, got)
		}
	}
	if _, after := e.body(oc, "GET", "/api/v2/admin/sso", ""); after != view {
		t.Fatalf("what is in force changed after refused saves:\n%s\n%s", view, after)
	}
}

func TestTwoOverlappingSavesLeaveOneWholeSet(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	oc := e.ownerClient()
	a := settingsBody(t, map[string]any{"autoCreate": true, "linkByEmail": true, "defaultRole": "member", "label": "A", "adminGroups": []string{"a1", "a2"}})
	b := settingsBody(t, map[string]any{"autoCreate": false, "linkByEmail": false, "defaultRole": "viewer", "label": "B", "adminGroups": []string{"b1"}})
	for i := 0; i < 60; i++ {
		var wg sync.WaitGroup
		for n := 0; n < 8; n++ {
			body := []string{a, b}[n%2]
			wg.Add(1)
			go func(body string) {
				defer wg.Done()
				if code, out := e.body(oc, "PATCH", "/api/v2/admin/sso", body); code != 200 {
					t.Errorf("save = %d %s", code, out)
				}
			}(body)
		}
		wg.Wait()
		stored, err := oidc.DecodeBehaviour(e.storedSettings())
		if err != nil {
			t.Fatal(err)
		}
		inForce := e.sso(t).Behaviour()
		if len(stored.Changed(inForce)) != 0 {
			t.Fatalf("round %d: stored %+v but in force %+v", i, stored, inForce)
		}
		whole := (stored.Label == "A" && stored.AutoCreate && stored.LinkByEmail && stored.DefaultRole == "member" && len(stored.AdminGroups) == 2) ||
			(stored.Label == "B" && !stored.AutoCreate && !stored.LinkByEmail && stored.DefaultRole == "viewer" && len(stored.AdminGroups) == 1)
		if !whole {
			t.Fatalf("round %d left a mix: %+v", i, stored)
		}
	}
}

func TestNeitherTheAnswerNorTheEventCarriesTheClientSecret(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	oc := e.ownerClient()
	code, body := e.body(oc, "PATCH", "/api/v2/admin/sso", settingsBody(t, map[string]any{"linkByEmail": true, "adminGroups": []string{"ops"}}))
	if code != 200 {
		t.Fatalf("save = %d %s", code, body)
	}
	_, view := e.body(oc, "GET", "/api/v2/admin/sso", "")
	evs, err := e.st.ListEvents(50, "")
	if err != nil {
		t.Fatal(err)
	}
	var detail string
	for _, ev := range evs {
		if ev.Action == "sso-settings" {
			detail = ev.Detail
		}
	}
	if detail == "" {
		t.Fatal("no sso-settings event was recorded")
	}
	for name, text := range map[string]string{"answer": body, "status": view, "event": detail} {
		if strings.Contains(text, "s3cret") || strings.Contains(strings.ToLower(text), "secret") {
			t.Errorf("the %s shows the client secret: %s", name, text)
		}
	}
	var d struct {
		Changed []string       `json:"changed"`
		Values  map[string]any `json:"values"`
	}
	if err := json.Unmarshal([]byte(detail), &d); err != nil {
		t.Fatalf("%v: %s", err, detail)
	}
	if strings.Join(d.Changed, ",") != "linkByEmail,adminGroups" || d.Values["linkByEmail"] != true {
		t.Fatalf("the event = %s", detail)
	}
	if _, has := d.Values["autoCreate"]; has {
		t.Fatalf("the event lists a field that did not change: %s", detail)
	}
	// Saving the same set again changes nothing, so records nothing.
	e.body(oc, "PATCH", "/api/v2/admin/sso", settingsBody(t, map[string]any{"linkByEmail": true, "adminGroups": []string{"ops"}}))
	n := 0
	evs, _ = e.st.ListEvents(50, "")
	for _, ev := range evs {
		if ev.Action == "sso-settings" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d sso-settings events after a repeat save; want 1", n)
	}
}

func TestTheAnswerHoldsAllTwelveAndNeverNull(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	_, body := e.body(e.adminClient(), "GET", "/api/v2/admin/sso", "")
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	for _, f := range append([]string{"labelSetting", "label"}, ssoSettingFields...) {
		if v, ok := got[f]; !ok || string(v) == "null" {
			t.Errorf("%s = %s in %s", f, v, body)
		}
	}
	if string(got["adminGroups"]) != "[]" || string(got["label"]) != `"Sign in with single sign-on"` || string(got["labelSetting"]) != `""` {
		t.Fatalf("answer = %s", body)
	}
}

func TestTheStoredChoicesOutliveARestartAndWinOverTheEnvironment(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	if code, _ := e.body(e.ownerClient(), "PATCH", "/api/v2/admin/sso", settingsBody(t, map[string]any{"autoCreate": true, "defaultRole": "member"})); code != 200 {
		t.Fatal("save failed")
	}
	vars := map[string]string{
		"REET_OIDC_ISSUER": e.idp.Issuer, "REET_OIDC_CLIENT_ID": "client", "REET_OIDC_CLIENT_SECRET": "s3cret",
		"REET_OIDC_REDIRECT_URL": "http://localhost/api/v2/auth/oidc/callback",
		"REET_OIDC_AUTO_CREATE":  "off", "REET_OIDC_DEFAULT_ROLE": "viewer",
	}
	p, err := oidc.FromEnv(func(k string) string { return vars[k] })
	if err != nil || p == nil {
		t.Fatalf("%v %v", p, err)
	}
	restarted := newServer(e.st, deadFetcher(t), "bootstrap", WithAuthMode(AuthAccounts), WithOIDC(p))
	if !p.AutoCreate() || p.Behaviour().DefaultRole != "member" {
		t.Fatal("a restart under a different environment used the environment")
	}
	_ = restarted
}

func TestVersionReportsTheSettingsRoute(t *testing.T) {
	_, body := do(t, newSrv(t, deadFetcher(t), ""), "GET", "/api/version", "")
	var doc struct {
		Features []string `json:"features"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	for _, f := range doc.Features {
		if f == "sso-settings" {
			return
		}
	}
	t.Fatalf("features = %v", doc.Features)
}

func (e *ssoEnv) sso(t *testing.T) *oidc.Provider {
	t.Helper()
	return e.srvr.sso
}

// Without the lock, one save can be stored, another stored and applied, and the
// first applied last: the database says one thing and the controller does
// another until the next restart. The hook holds the first save between its
// store and its effect so the second can overtake it, if it is allowed to.
func TestASaveCannotBeOvertakenBetweenBeingStoredAndTakingEffect(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	oc := e.ownerClient()
	var mu sync.Mutex
	calls := 0
	secondStored := make(chan struct{})
	e.srvr.afterSSOStore = func() {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 2 {
			close(secondStored)
			return
		}
		select {
		case <-secondStored:
			time.Sleep(100 * time.Millisecond) // let the overtaking save take effect first
		case <-time.After(400 * time.Millisecond):
		}
	}
	a := settingsBody(t, map[string]any{"label": "A", "autoCreate": true})
	b := settingsBody(t, map[string]any{"label": "B"})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); e.body(oc, "PATCH", "/api/v2/admin/sso", a) }()
	time.Sleep(100 * time.Millisecond) // the first save is the one held
	go func() { defer wg.Done(); e.body(oc, "PATCH", "/api/v2/admin/sso", b) }()
	wg.Wait()
	stored, err := oidc.DecodeBehaviour(e.storedSettings())
	if err != nil {
		t.Fatal(err)
	}
	if now := e.sso(t).Behaviour(); len(stored.Changed(now)) != 0 {
		t.Fatalf("stored %+v but in force %+v", stored, now)
	}
}

// A sign-in follows the choices its claims were read under, even if the owner
// saves new ones before it finishes: here the claims were read while
// auto-create was on and "staff" mapped to member, and the controller now says
// neither.
func TestASignInFollowsTheChoicesItsClaimsWereReadUnder(t *testing.T) {
	e := newSSOEnv(t, ssoOpts{})
	read := e.sso(t).Behaviour()
	read.AutoCreate, read.MemberGroups = true, []string{"staff"}
	if code, _ := e.body(e.ownerClient(), "PATCH", "/api/v2/admin/sso", settingsBody(t, map[string]any{"autoCreate": false})); code != 200 {
		t.Fatal("could not save the newer settings")
	}
	claims := oidc.Claims{Subject: "sub-late", Email: "late@example.com", EmailVerified: true, Name: "Late", Groups: []string{"staff"}, HasGroups: true, Read: read}
	w := httptest.NewRecorder()
	e.srvr.ssoSignIn(w, httptest.NewRequest("GET", "/api/v2/auth/oidc/callback", nil), store.SSOLogin{Next: "/"}, claims)
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Fatalf("the sign-in followed the newer choices: %q", loc)
	}
	u, ok, _ := e.st.SSOIdentityUser(e.idp.Issuer, "sub-late")
	if !ok || u.Role != "member" {
		t.Fatalf("created %+v, found %v; want a member from the old mapping", u, ok)
	}
}
