package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestCommunityAndGroupAudienceAPI(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "")
	creator, err := st.CreateUser("creator@example.com", "Creator", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	member, _ := st.CreateUser("member@example.com", "Member", "member", "hash")
	excluded, _ := st.CreateUser("excluded@example.com", "Excluded", "member", "hash")
	viewer, _ := st.CreateUser("viewer@example.com", "Viewer", "viewer", "hash")
	tokens := map[int64]string{}
	for _, u := range []store.User{creator, member, excluded, viewer} {
		token := fmt.Sprintf("token-%d", u.ID)
		tokens[u.ID] = token
		st.CreateAPIToken(u.ID, "test", auth.HashToken(token))
	}
	call := func(userID int64, method, path, body string) (int, string) {
		t.Helper()
		if userID == 0 {
			return doClient(t, srv.Client(), srv, method, path, body)
		}
		return doClient(t, srv.Client(), srv, method, path, body, "X-Reet-Token", tokens[userID])
	}
	code, body := call(creator.ID, "POST", "/api/v2/groups", `{"name":"Engineering","kind":"team"}`)
	if code != 200 {
		t.Fatal(body)
	}
	var groups struct{ Groups []store.Group }
	json.Unmarshal([]byte(body), &groups)
	id := groups.Groups[0].ID
	for _, email := range []string{member.Email, excluded.Email} {
		code, body = call(creator.ID, "POST", fmt.Sprintf("/api/v2/groups/%d/members", id), `{"email":"`+email+`","role":"member"}`)
		if code != 200 {
			t.Fatal(body)
		}
	}
	code, body = call(member.ID, "POST", fmt.Sprintf("/api/v2/groups/%d/members", id), `{"email":"viewer@example.com","role":"member"}`)
	if code == 200 {
		t.Fatal("ordinary group member changed membership")
	}
	code, body = call(creator.ID, "POST", "/api/v2/lists", fmt.Sprintf(`{"name":"Team private","visibility":"team","audience":{"groupId":%d,"groupRole":"editor","excluded":["excluded@example.com"]}}`, id))
	if code != 200 {
		t.Fatal(body)
	}
	var list store.ListDetail
	json.Unmarshal([]byte(body), &list)
	path := fmt.Sprintf("/api/v2/lists/%d", list.ID)
	if code, body = call(member.ID, "GET", path, ""); code != 200 {
		t.Fatal(body)
	}
	for _, method := range []string{"GET", "PATCH"} {
		if code, body = call(excluded.ID, method, path, `{"name":"No"}`); code != 404 {
			t.Fatalf("excluded %s got %d %s", method, code, body)
		}
	}
	if code, body = call(0, "GET", "/api/community", ""); code != 200 || strings.Contains(body, "Team private") {
		t.Fatalf("anonymous community leaked private list: %s", body)
	}
	if code, _ = call(viewer.ID, "POST", "/api/v2/groups", `{"name":"No","kind":"team"}`); code != 403 {
		t.Fatal("read-only account created group")
	}
	// The default audience is personal; publication is a separate explicit choice.
	code, body = call(creator.ID, "POST", "/api/v2/lists", `{"name":"Personal by default"}`)
	if code != 200 {
		t.Fatal(body)
	}
	var personal store.ListDetail
	json.Unmarshal([]byte(body), &personal)
	if personal.Visibility != "personal" {
		t.Fatal("new list default is too broad")
	}
	if code, _ = call(member.ID, "GET", fmt.Sprintf("/api/v2/lists/%d", personal.ID), ""); code != 404 {
		t.Fatal("personal list is readable by another account")
	}
	if code, body = call(creator.ID, "PATCH", path, `{"visibility":"public","audience":{"people":[],"excluded":[]}}`); code != 200 {
		t.Fatal(body)
	}
	if code, body = call(0, "GET", "/api/community", ""); code != 200 || !strings.Contains(body, "Team private") || strings.Contains(body, "creator@example.com") || strings.Contains(body, "excluded@example.com") {
		t.Fatalf("public discovery %d %s", code, body)
	}
	if code, body = call(0, "GET", "/api/v2/public/lists/"+list.Slug, ""); code != 200 || strings.Contains(body, "creator@example.com") || strings.Contains(body, "audience") {
		t.Fatalf("anonymous list %d %s", code, body)
	}
	if code, _ = call(0, "GET", "/api/v2/groups", ""); code != 401 {
		t.Fatal("anonymous group directory exposed")
	}
	if code, body = call(creator.ID, "PATCH", path, `{"visibility":"personal","audience":{"people":[],"excluded":[]}}`); code != 200 {
		t.Fatal(body)
	}
	if code, _ = call(0, "GET", "/api/v2/public/lists/"+list.Slug, ""); code != 404 {
		t.Fatal("unpublished link still public")
	}
}
