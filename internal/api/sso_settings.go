package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/eyupio/reet.sh/internal/oidc"
)

// How single sign-on treats the people who come through it is the owner's to
// change while the controller runs: the twelve switches in oidc.Behaviour.
// The provider's address, client and secret stay in the environment and are
// neither shown nor changed here.

// ssoSettingsLimit is far more than twelve fields need; a larger body is
// refused rather than read.
const ssoSettingsLimit = 32 << 10

var ssoSettingFields = []string{
	"label", "passwordLogin", "autoCreate", "linkByEmail", "defaultRole", "nameClaim", "emailClaim", "groupsClaim",
	"scopes", "adminGroups", "memberGroups", "viewerGroups",
}

// ssoSettingsView is the twelve choices in the wire form, plus the resolved
// button text beside the stored one. Lists are never null.
func ssoSettingsView(b oidc.Behaviour) map[string]any {
	return map[string]any{
		"label": b.ResolvedLabel(), "labelSetting": b.Label,
		"passwordLogin": b.PasswordLogin, "autoCreate": b.AutoCreate, "linkByEmail": b.LinkByEmail,
		"defaultRole": b.DefaultRole, "nameClaim": b.NameClaim, "emailClaim": b.EmailClaim, "groupsClaim": b.GroupsClaim,
		"scopes": nonNil(b.Scopes), "adminGroups": nonNil(b.AdminGroups),
		"memberGroups": nonNil(b.MemberGroups), "viewerGroups": nonNil(b.ViewerGroups),
	}
}

// ssoSettingsValues is what the change event records: the new value of each
// field that changed. None of the twelve is a secret.
func ssoSettingsValues(b oidc.Behaviour, fields []string) map[string]any {
	view := ssoSettingsView(b)
	view["label"] = b.Label // the stored text, as the owner typed it
	out := map[string]any{}
	for _, f := range fields {
		out[f] = view[f]
	}
	return out
}

func ssoBadField(w http.ResponseWriter, field, msg string) {
	writeErrCode(w, http.StatusBadRequest, "invalid_"+field, field, msg, "")
}

// ssoSettingsUpdate stores a whole new set of choices. Every field must be
// present, so a partial write can never half-apply, and nothing is stored
// unless all of it is valid. The next sign-in obeys it; nothing restarts.
func (s *server) ssoSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.ssoGone(w) {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, ssoSettingsLimit))
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("the body must be a JSON object of at most %d KB", ssoSettingsLimit>>10))
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		writeErr(w, http.StatusBadRequest, "the body must be a JSON object with every setting in it")
		return
	}
	known := map[string]bool{}
	for _, f := range ssoSettingFields {
		known[f] = true
		if _, ok := fields[f]; !ok {
			ssoBadField(w, f, f+" is missing: send every setting each time")
			return
		}
	}
	for f := range fields {
		// labelSetting is what the answer calls the stored label; a client that
		// sends the answer back may carry the read-only fields with it.
		if !known[f] && !ssoReadOnly[f] {
			ssoBadField(w, f, f+" is not a setting")
			return
		}
	}
	var b oidc.Behaviour
	typed := []struct {
		field string
		into  any
		what  string
	}{
		{"label", &b.Label, "text"}, {"passwordLogin", &b.PasswordLogin, "true or false"},
		{"autoCreate", &b.AutoCreate, "true or false"}, {"linkByEmail", &b.LinkByEmail, "true or false"},
		{"defaultRole", &b.DefaultRole, "text"}, {"nameClaim", &b.NameClaim, "text"}, {"emailClaim", &b.EmailClaim, "text"},
		{"groupsClaim", &b.GroupsClaim, "text"}, {"scopes", &b.Scopes, "a list of text"},
		{"adminGroups", &b.AdminGroups, "a list of text"}, {"memberGroups", &b.MemberGroups, "a list of text"},
		{"viewerGroups", &b.ViewerGroups, "a list of text"},
	}
	for _, t := range typed {
		v := bytes.TrimSpace(fields[t.field])
		dec := json.NewDecoder(bytes.NewReader(v))
		if bytes.Equal(v, []byte("null")) || dec.Decode(t.into) != nil {
			ssoBadField(w, t.field, t.field+" must be "+t.what)
			return
		}
	}
	if err := b.Validate(); err != nil {
		var fe *oidc.FieldError
		if errors.As(err, &fe) {
			ssoBadField(w, fe.Field, fe.Field+" "+fe.Msg)
		} else {
			writeErr(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	// One save at a time: the stored value and the one in force change
	// together, so two overlapping saves leave one whole set and not a mix.
	s.ssoSaveMu.Lock()
	defer s.ssoSaveMu.Unlock()
	before := s.sso.Behaviour()
	enc, err := oidc.EncodeBehaviour(b)
	if err == nil {
		err = s.st.SetSetting(oidc.SettingKey, enc)
	}
	if err != nil {
		log.Printf("single sign-on: save settings: %v", err)
		writeErr(w, http.StatusInternalServerError, "could not save the settings")
		return
	}
	if s.afterSSOStore != nil {
		s.afterSSOStore()
	}
	if err := s.sso.SetBehaviour(b); err != nil {
		// Validated above, so this is a programming error; the stored value is
		// put back rather than left to be read at the next start.
		log.Printf("single sign-on: apply settings: %v", err)
		if old, e := oidc.EncodeBehaviour(before); e == nil {
			_ = s.st.SetSetting(oidc.SettingKey, old)
		}
		writeErr(w, http.StatusInternalServerError, "could not apply the settings")
		return
	}
	now := s.sso.Behaviour()
	if changed := before.Changed(now); len(changed) > 0 {
		u := s.currentUser(r)
		s.event(u.ID, "ui", "sso-settings", "", map[string]any{
			"name": u.Name, "email": u.Email, "changed": changed, "values": ssoSettingsValues(now, changed),
		})
	}
	writeJSON(w, http.StatusOK, ssoSettingsView(now))
}

var ssoReadOnly = map[string]bool{
	"labelSetting": true, "enabled": true, "issuer": true, "redirectUrl": true, "linked": true, "reachable": true, "problem": true,
}
