package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestSkillContentTargetAPIPrivateConditionalIntentOnly(t *testing.T) {
	srv, st, a, _, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	if e := st.TouchEnvironmentFor(uid, a.Host, "agents", "", ""); e != nil {
		t.Fatal(e)
	}
	env := store.EnvironmentIDFor(uid, a.Host, "agents", "")
	if e := st.RecordInventoryFor(uid, env, true, store.MachineInfo{}); e != nil {
		t.Fatal(e)
	}
	w := privateSkillHTTP(t, srv, "POST", "/api/v2/skill-bundles", privateSkillBody("pin_api_bundle_0001", "", "PRIVATE_PIN_CONTENT"), "", "", token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var bundle store.PrivateSkillResult
	if e := json.Unmarshal(w.Body.Bytes(), &bundle); e != nil {
		t.Fatal(e)
	}
	empty := ""
	in := store.SkillContentTargetWrite{EnvironmentID: env, AgentID: a.ID, BundleID: bundle.Bundle.ID, RevisionID: bundle.Revision.ID, Mode: "managed", ExpectedVersion: &empty}
	raw, _ := json.Marshal(in)
	body := string(raw)
	const root = "/api/v2/skill-content-targets"
	cookie := terminalLogin(t, st, uid)
	viewer, e := st.CreateUser("pin-api-viewer@example.test", "Reader", "viewer", "hash")
	if e != nil {
		t.Fatal(e)
	}
	other, e := st.CreateUser("pin-api-other@example.test", "Other", "member", "hash")
	if e != nil {
		t.Fatal(e)
	}
	otherCookie := terminalLogin(t, st, other.ID)
	if _, e = st.CreateAPIToken(other.ID, "other", auth.HashToken("pin-other-personal-token")); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name, method, path, body, cookie, origin, token string
		status                                          int
	}{
		{"anonymous", "GET", root, "", "", "", "", 401},
		{"viewer", "GET", root, "", terminalLogin(t, st, viewer.ID), "", "", 403},
		{"viewer mutation", "POST", root, body, terminalLogin(t, st, viewer.ID), "https://reet.example", "", 403},
		{"missing origin", "POST", root, body, cookie, "", "", 403},
		{"foreign origin", "POST", root, body, cookie, "https://foreign.example", "", 403},
		{"origin path", "POST", root, body, cookie, "https://reet.example/path", "", 403},
		{"query", "GET", root + "?x=1", "", cookie, "", "", 400},
		{"empty query", "GET", root + "?", "", cookie, "", "", 400},
		{"missing expected version", "POST", root, strings.Replace(body, `,"expectedVersion":""`, "", 1), cookie, "https://reet.example", "", 400},
		{"null expected version", "POST", root, strings.Replace(body, `"expectedVersion":""`, `"expectedVersion":null`, 1), cookie, "https://reet.example", "", 400},
		{"unknown owner", "POST", root, strings.TrimSuffix(body, "}") + `,"userId":99}`, cookie, "https://reet.example", "", 400},
		{"attempted file upload", "POST", root, strings.TrimSuffix(body, "}") + `,"files":[]}`, cookie, "https://reet.example", "", 400},
		{"trailing JSON", "POST", root, body + `{}`, cookie, "https://reet.example", "", 400},
		{"oversized", "POST", root, strings.Repeat(" ", 16<<10) + body, cookie, "https://reet.example", "", 400},
		{"unsupported mode", "POST", root, strings.Replace(body, `"managed"`, `"mirror"`, 1), cookie, "https://reet.example", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := privateSkillHTTP(t, srv, tc.method, tc.path, tc.body, tc.cookie, tc.origin, tc.token)
			if r.Code != tc.status {
				t.Fatal(r.Code, r.Body.String())
			}
		})
	}
	w = privateSkillHTTP(t, srv, "POST", root, body, "", "", token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var pin store.SkillContentTarget
	if e = json.Unmarshal(w.Body.Bytes(), &pin); e != nil {
		t.Fatal(e)
	}
	if pin.Verification != "not_verified" || pin.ManifestDigest != bundle.Revision.ManifestDigest {
		t.Fatal(pin)
	}
	path := root + "/" + pin.ID
	for _, tc := range []struct {
		method, path, body, cookie, token string
		status                            int
	}{
		{"GET", path, "", otherCookie, "", 404},
		{"GET", path, "", "", "pin-other-personal-token", 404},
		{"DELETE", path, fmt.Sprintf(`{"expectedVersion":%q}`, pin.Version), otherCookie, "", 404},
		{"DELETE", path, `{}`, cookie, "", 400},
		{"DELETE", path, fmt.Sprintf(`{"expectedVersion":%q}`, strings.Repeat("a", 48)), cookie, "", 409},
		{"GET", root + "/" + strings.Repeat("0", 48), "", cookie, "", 404},
	} {
		r := privateSkillHTTP(t, srv, tc.method, tc.path, tc.body, tc.cookie, "https://reet.example", tc.token)
		if r.Code != tc.status {
			t.Fatal(tc.method, tc.path, r.Code, r.Body.String())
		}
	}
	for _, path := range []string{root, path} {
		w = privateSkillHTTP(t, srv, "GET", path, "", cookie, "", "")
		if w.Code != 200 || strings.Contains(w.Body.String(), "PRIVATE_PIN_CONTENT") || strings.Contains(w.Body.String(), `"files":`) || strings.Contains(w.Body.String(), `"data":`) {
			t.Fatal("non-metadata response", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private response cached")
		}
	}
	w = privateSkillHTTP(t, srv, "POST", root, body, cookie, "https://reet.example", "")
	if w.Code != 200 {
		t.Fatal("browser same-state save", w.Code, w.Body.String())
	}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		if mcpAPIPath(method, root) || mcpAPIPath(method, path) {
			t.Fatal("MCP allowlist expanded")
		}
	}
	if e = st.ControlSyncAgent(uid, a.ID, "pause"); e != nil {
		t.Fatal(e)
	}
	w = privateSkillHTTP(t, srv, "DELETE", path, fmt.Sprintf(`{"expectedVersion":%q}`, pin.Version), cookie, "https://reet.example", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if revision, e := st.PrivateSkillRevisionFor(uid, bundle.Bundle.ID, bundle.Revision.ID); e != nil || len(revision.Files) == 0 {
		t.Fatal("intent deletion removed private content", e)
	}
	if work, e := st.ClaimSyncJob(uid, a); e != nil || work != nil {
		t.Fatal("metadata dispatched machine work", work, e)
	}
}
func TestSkillContentTargetAPIRejectsLegacyAuthModes(t *testing.T) {
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		t.Run(string(mode), func(t *testing.T) {
			srv, _, _, _, _ := privateFixture(t, mode, "legacy-token")
			for _, method := range []string{"GET", "POST", "DELETE"} {
				path := "/api/v2/skill-content-targets"
				if method == "DELETE" {
					path += "/" + strings.Repeat("a", 48)
				}
				w := privateSkillHTTP(t, srv, method, path, `{}`, "", "https://reet.example", "legacy-token")
				if w.Code != 403 {
					t.Fatal(method, w.Code, w.Body.String())
				}
			}
		})
	}
}
