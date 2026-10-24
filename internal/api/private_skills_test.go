package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

func privateSkillBody(key, base, text string) string {
	in := store.PrivateSkillWrite{RequestID: key, ExpectedBase: &base, Name: "demo", Files: []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Private instructions\n---\n" + text + "\n")}, {Path: "data.bin", Data: []byte{0, 255, 1}}}}
	raw, _ := json.Marshal(in)
	return string(raw)
}
func privateSkillHTTP(t *testing.T, s *Server, method, path, body, cookie, origin, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "https://reet.example"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", origin)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestPrivateSkillAPIAccountContractAndIsolation(t *testing.T) {
	srv, st, _, _, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	srv.SetRemoteTerminals(false)
	viewer, err := st.CreateUser("bundle-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateUser("bundle-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherCookie := terminalLogin(t, st, other.ID)
	const root = "/api/v2/skill-bundles"
	body := privateSkillBody("api_create_000001", "", "PRIVATE_BODY")
	for _, tc := range []struct {
		name, method, path, body, cookie, origin, token string
		status                                          int
	}{
		{"anonymous", "GET", root, "", "", "", "", 401},
		{"viewer", "GET", root, "", terminalLogin(t, st, viewer.ID), "", "", 403},
		{"missing origin", "POST", root, body, cookie, "", "", 403},
		{"foreign origin", "POST", root, body, cookie, "https://foreign.example", "", 403},
		{"origin path", "POST", root, body, cookie, "https://reet.example/path", "", 403},
		{"origin query", "POST", root, body, cookie, "https://reet.example?x=1", "", 403},
		{"unknown owner", "POST", root, strings.TrimSuffix(body, "}") + `,"userId":999}`, cookie, "https://reet.example", "", 400},
		{"publish flag", "POST", root, strings.TrimSuffix(body, "}") + `,"published":true}`, cookie, "https://reet.example", "", 400},
		{"missing base", "POST", root, strings.Replace(body, `"expectedBase":"",`, "", 1), cookie, "https://reet.example", "", 400},
		{"extra JSON", "POST", root, body + `{}`, cookie, "https://reet.example", "", 400},
		{"URL credential", "GET", root + "?t=" + token, "", "", "", "", 400},
		{"oversize", "POST", root, strings.Repeat(" ", 16<<20) + body, cookie, "https://reet.example", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := privateSkillHTTP(t, srv, tc.method, tc.path, tc.body, tc.cookie, tc.origin, tc.token)
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	w := privateSkillHTTP(t, srv, "POST", root, body, cookie, "https://reet.example", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var first store.PrivateSkillResult
	if err = json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), "PRIVATE_BODY") {
		t.Fatal("write receipt contains payload")
	}
	bundle := root + "/" + first.Bundle.ID
	revision := bundle + "/revisions/" + first.Revision.ID
	for _, path := range []string{bundle, bundle + "/revisions", revision, bundle + "/revisions/" + first.Revision.ManifestDigest} {
		w = privateSkillHTTP(t, srv, "GET", path, "", otherCookie, "", "")
		if w.Code != 404 {
			t.Fatal("foreign or guessed lookup", path, w.Code, w.Body.String())
		}
	}
	// Personal API tokens operate only as their owning account, without browser Origin.
	if _, err = st.CreateAPIToken(other.ID, "other", auth.HashToken("private-other-token")); err != nil {
		t.Fatal(err)
	}
	w = privateSkillHTTP(t, srv, "GET", revision, "", "", "", "private-other-token")
	if w.Code != 404 {
		t.Fatal("foreign token", w.Code)
	}
	w = privateSkillHTTP(t, srv, "GET", revision, "", "", "", token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("cache/encoding headers", w.Header())
	}
	var contents store.PrivateSkillRevision
	if err = json.Unmarshal(w.Body.Bytes(), &contents); err != nil || len(contents.Files) != 2 {
		t.Fatal(contents, err)
	}
	if !strings.Contains(string(contents.Files[0].Data), "PRIVATE_BODY") {
		t.Fatal("payload missing", contents.Files)
	}
	w = privateSkillHTTP(t, srv, "POST", bundle+"/revisions", privateSkillBody("foreign_edit_0001", first.Revision.ID, "OTHER"), "", "", "private-other-token")
	if w.Code != 404 {
		t.Fatal("foreign mutation", w.Code, w.Body.String())
	}
	nextBody := privateSkillBody("api_append_000001", first.Revision.ID, "NEXT")
	w = privateSkillHTTP(t, srv, "POST", bundle+"/revisions", nextBody, "", "", token)
	if w.Code != 200 {
		t.Fatal("personal write", w.Code, w.Body.String())
	}
	var next store.PrivateSkillResult
	if err = json.Unmarshal(w.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	w = privateSkillHTTP(t, srv, "POST", bundle+"/revisions", privateSkillBody("api_stale_0000001", first.Revision.ID, "STALE"), cookie, "https://reet.example", "")
	if w.Code != 409 {
		t.Fatal("stale", w.Code, w.Body.String())
	}
	w = privateSkillHTTP(t, srv, "POST", root, body, cookie, "https://reet.example", "")
	var retry store.PrivateSkillResult
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &retry) != nil || retry.Bundle.Head != first.Revision.ID || retry.Revision.ID != first.Revision.ID {
		t.Fatal("historical retry", w.Code, w.Body.String())
	}
	w = privateSkillHTTP(t, srv, "GET", bundle, "", cookie, "", "")
	var current store.PrivateSkillBundle
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &current) != nil || current.Head != next.Revision.ID {
		t.Fatal("retry moved head", w.Code, w.Body.String())
	}
	for _, method := range []string{"GET", "POST"} {
		if mcpAPIPath(method, root) {
			t.Fatal("new private route exposed through MCP")
		}
	}
	if err = st.SetUserDisabled(uid, true); err != nil {
		t.Fatal(err)
	}
	w = privateSkillHTTP(t, srv, "GET", revision, "", cookie, "", "")
	if w.Code != 401 {
		t.Fatal("disabled cookie", w.Code)
	}
}
func TestPrivateSkillAPIDoesNotExposeDefaultOwnerInLegacyModes(t *testing.T) {
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		t.Run(string(mode), func(t *testing.T) {
			srv, _, _, _, _ := privateFixture(t, mode, "")
			for _, method := range []string{"GET", "POST"} {
				w := privateSkillHTTP(t, srv, method, "/api/v2/skill-bundles", privateSkillBody("legacy_request_01", "", "NEVER"), "", "https://reet.example", "")
				if w.Code != 403 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
		})
	}
}
