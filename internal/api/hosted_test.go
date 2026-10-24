package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestHostedSkillsPublicationAndOwnership(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "")
	alice, _ := st.CreateUser("author@example.com", "Author", "member", "hash")
	bob, _ := st.CreateUser("other@example.com", "Other", "member", "hash")
	viewer, _ := st.CreateUser("viewer@example.com", "Viewer", "viewer", "hash")
	tokens := map[int64]string{}
	for _, u := range []store.User{alice, bob, viewer} {
		token := fmt.Sprintf("hosted-%d", u.ID)
		tokens[u.ID] = token
		st.CreateAPIToken(u.ID, "test", auth.HashToken(token))
	}
	call := func(uid int64, method, path string, in any) (int, string) {
		t.Helper()
		body := ""
		if in != nil {
			b, _ := json.Marshal(in)
			body = string(b)
		}
		if uid == 0 {
			return do(t, srv, method, path, body)
		}
		return do(t, srv, method, path, body, "X-Reet-Token", tokens[uid])
	}
	expect := func(uid int64, method, path string, in any, want int) string {
		t.Helper()
		code, body := call(uid, method, path, in)
		if code != want {
			t.Fatalf("%s %s as %d: %d %s; want %d", method, path, uid, code, body, want)
		}
		return body
	}
	expect(0, "GET", "/api/v2/hosting", nil, 401)
	expect(0, "POST", "/api/v2/hosting/username", map[string]string{"username": "anon"}, 401)
	expect(viewer.ID, "POST", "/api/v2/hosting/username", map[string]string{"username": "viewer"}, 403)
	expect(alice.ID, "POST", "/api/v2/hosting/username", map[string]string{"username": "api"}, 400)
	expect(alice.ID, "POST", "/api/v2/hosting/username", map[string]string{"username": "alice"}, 200)
	expect(bob.ID, "POST", "/api/v2/hosting/username", map[string]string{"username": "alice"}, 409)
	expect(alice.ID, "POST", "/api/v2/hosting/username", map[string]string{"username": "new-name"}, 409)
	expect(bob.ID, "POST", "/api/v2/hosting/username", map[string]string{"username": "bob"}, 200)
	sk := store.HostedSkill{Content: "---\nname: reet-demo\ndescription: |\n  A hosted skill.\n---\n\nFollow these instructions. <script>alert(1)</script>\n", Files: map[string][]byte{"references/help.md": []byte("Supporting text"), "assets/data.bin": {0, 1, 255}}}
	expect(alice.ID, "PUT", "/api/v2/hosting/skills/reet-demo", sk, 200)
	expect(viewer.ID, "PUT", "/api/v2/hosting/skills/reet-demo", sk, 403)
	if body := expect(bob.ID, "GET", "/api/v2/hosting", nil, 200); strings.Contains(body, "Follow these instructions") {
		t.Fatal("another account read a draft")
	}
	paths := []string{"/alice/reet-demo", "/alice/reet-demo/SKILL.md", "/alice/reet-demo/.well-known/agent-skills/index.json", "/alice/reet-demo/.well-known/agent-skills/reet-demo/references/help.md"}
	for _, path := range paths {
		expect(0, "GET", path, nil, 404)
	}
	expect(bob.ID, "DELETE", "/api/v2/hosting/skills/reet-demo", nil, 404)
	sk.Published = true
	expect(alice.ID, "PUT", "/api/v2/hosting/skills/reet-demo", sk, 200)
	page := expect(0, "GET", "/alice/reet-demo", nil, 200)
	if strings.Contains(page, "<script>alert") || !strings.Contains(page, "npx skills add "+srv.URL+"/alice/reet-demo") {
		t.Fatalf("unsafe page or wrong install command: %s", page)
	}
	if body := expect(0, "GET", "/alice/reet-demo/SKILL.md", nil, 200); body != sk.Content {
		t.Fatal("changed skill contents")
	}
	raw := expect(0, "GET", "/alice/reet-demo/.well-known/agent-skills/index.json", nil, 200)
	var index struct {
		Skills []struct {
			Name, Description string
			Files             []string
		}
	}
	if json.Unmarshal([]byte(raw), &index) != nil || len(index.Skills) != 1 || index.Skills[0].Name != "reet-demo" || len(index.Skills[0].Files) != 3 {
		t.Fatalf("invalid CLI index: %s", raw)
	}
	if body := expect(0, "GET", paths[3], nil, 200); body != "Supporting text" {
		t.Fatal(body)
	}
	if body := expect(0, "GET", "/alice/reet-demo/.well-known/agent-skills/reet-demo/assets/data.bin", nil, 200); body != string([]byte{0, 1, 255}) {
		t.Fatal("binary file was corrupted")
	}
	expect(0, "GET", "/alice/reet-demo/.well-known/agent-skills/wrong/SKILL.md", nil, 404)
	// The same name in another namespace cannot replace the first author's skill.
	other := sk
	other.Content = strings.Replace(sk.Content, "Follow these instructions.", "Other author's instructions.", 1)
	expect(bob.ID, "PUT", "/api/v2/hosting/skills/reet-demo", other, 200)
	if body := expect(0, "GET", "/alice/reet-demo/SKILL.md", nil, 200); body != sk.Content {
		t.Fatal("another author replaced the skill")
	}
	// Both the URL and its copied command add the same hosted reference.
	for _, input := range []string{srv.URL + "/alice/reet-demo", "npx skills add " + srv.URL + "/alice/reet-demo"} {
		body := expect(bob.ID, "POST", "/api/skills", map[string]string{"input": input}, 200)
		if !strings.Contains(body, `"sourceType":"hosted"`) {
			t.Fatal(body)
		}
	}
	var shelf listResp
	raw = expect(bob.ID, "GET", "/api/skills", nil, 200)
	json.Unmarshal([]byte(raw), &shelf)
	var aliceSkill store.Skill
	for _, item := range shelf.Skills {
		if strings.HasSuffix(item.Source, "/alice") {
			aliceSkill = item
		}
	}
	if aliceSkill.ID == "" || aliceSkill.SourceType != "hosted" || !aliceSkill.Bookmarked {
		t.Fatalf("hosted references not deduplicated or not in To test: %s", raw)
	}
	if got := installCommand(aliceSkill); got != "npx skills add "+srv.URL+"/alice/reet-demo --skill reet-demo" {
		t.Fatal(got)
	}
	expect(bob.ID, "POST", "/api/skill/refresh", map[string]string{"id": aliceSkill.ID}, 200)
	sk.Published = false
	expect(alice.ID, "PUT", "/api/v2/hosting/skills/reet-demo", sk, 200)
	for _, path := range paths {
		expect(0, "GET", path, nil, 404)
	}
	expect(bob.ID, "POST", "/api/skills", map[string]string{"input": srv.URL + "/alice/reet-demo"}, 400)
	sk.Published = true
	expect(alice.ID, "PUT", "/api/v2/hosting/skills/reet-demo", sk, 200)
	if err := st.SetUserDisabled(alice.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		expect(0, "GET", path, nil, 404)
	}
	st.SetUserDisabled(alice.ID, false)
	expect(alice.ID, "DELETE", "/api/v2/hosting/skills/reet-demo", nil, 200)
	for _, path := range paths {
		expect(0, "GET", path, nil, 404)
	}
	// Account deletion leaves the username reserved against install-link takeover.
	if err := st.DeleteUser(alice.ID); err != nil {
		t.Fatal(err)
	}
	newcomer, _ := st.CreateUser("new@example.com", "New", "member", "hash")
	if err := st.ClaimPublishingUsername(newcomer.ID, "alice"); err != store.ErrNamespaceTaken {
		t.Fatalf("deleted username was reusable: %v", err)
	}
}

func TestHostedRoutingPreservesApplicationPages(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hosted")) })
	spa := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("spa")) })
	handler := WithHostedPages(api, spa)
	for path, want := range map[string]string{"/": "spa", "/account": "spa", "/machines": "spa", "/machines?view=compare": "spa", "/setups": "spa", "/machines/demo": "hosted", "/hosting": "spa", "/lists/1": "spa", "/api/skills": "spa", "/alice/reet-demo": "hosted", "/alice/reet-demo/.well-known/agent-skills/index.json": "hosted"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Body.String() != want {
			t.Errorf("%s routed to %s", path, response.Body.String())
		}
	}
}

func TestHostedPrefixPreferenceRenameAndShelf(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "")
	alice, _ := st.CreateUser("author@example.com", "Author", "member", "hash")
	token := "hosted-prefix-token"
	st.CreateAPIToken(alice.ID, "test", auth.HashToken(token))
	call := func(method, path, body string) (int, string) {
		return do(t, srv, method, path, body, "X-Reet-Token", token)
	}
	expect := func(method, path, body string, want int) string {
		t.Helper()
		code, out := call(method, path, body)
		if code != want {
			t.Fatalf("%s %s: %d %s; want %d", method, path, code, out, want)
		}
		return out
	}
	expect("POST", "/api/v2/hosting/username", `{"username":"alice"}`, 200)
	// The default profile prefixes new hosted skills with reet-.
	expect("PUT", "/api/v2/hosting/skills/demo", `{"content":"---\nname: demo\ndescription: A skill.\n---\n\nInstructions.\n","files":{}}`, 400)
	// A custom prefix replaces the default.
	expect("PATCH", "/api/v2/profile", `{"hostedPrefix":"acme"}`, 200)
	expect("PUT", "/api/v2/hosting/skills/acme-demo", `{"content":"---\nname: acme-demo\ndescription: A skill.\n---\n\nInstructions.\n","files":{}}`, 200)
	expect("PUT", "/api/v2/hosting/skills/acme-demo", `{"content":"---\nname: acme-demo\ndescription: A skill.\n---\n\nInstructions.\n","files":{},"published":true}`, 200)
	var shelf listResp
	json.Unmarshal([]byte(expect("GET", "/api/skills", "", 200)), &shelf)
	if len(shelf.Skills) != 1 || !shelf.Skills[0].Bookmarked || shelf.Skills[0].SourceType != "hosted" {
		t.Fatalf("published skill not added to To test: %+v", shelf.Skills)
	}
	oldID := shelf.Skills[0].ID
	var created struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal([]byte(expect("POST", "/api/v2/lists", `{"name":"Try list"}`, 200)), &created)
	expect("POST", fmt.Sprintf("/api/v2/lists/%d/skills", created.ID), fmt.Sprintf(`{"input":%q}`, srv.URL+"/alice/acme-demo"), 200)
	// Renaming keeps the prefix, rewrites the bundle name and moves references.
	body := expect("POST", "/api/v2/hosting/skills/acme-demo/rename", `{"name":"acme-renamed"}`, 200)
	if !strings.Contains(body, `"slug":"acme-renamed"`) || !strings.Contains(body, "name: acme-renamed") {
		t.Fatalf("rename did not rewrite the bundle: %s", body)
	}
	expect("GET", "/alice/acme-demo", "", 404)
	expect("GET", "/alice/acme-renamed", "", 200)
	shelf = listResp{}
	json.Unmarshal([]byte(expect("GET", "/api/skills", "", 200)), &shelf)
	if len(shelf.Skills) != 1 || shelf.Skills[0].ID == oldID || shelf.Skills[0].Slug != "acme-renamed" {
		t.Fatalf("rename did not move the shelf reference: %+v", shelf.Skills)
	}
	var detail struct {
		Skills []store.Skill `json:"skills"`
	}
	json.Unmarshal([]byte(expect("GET", fmt.Sprintf("/api/v2/lists/%d", created.ID), "", 200)), &detail)
	if len(detail.Skills) != 1 || detail.Skills[0].Slug != "acme-renamed" {
		t.Fatalf("rename did not move the list reference: %+v", detail.Skills)
	}
	expect("POST", "/api/v2/hosting/skills/acme-renamed/rename", `{"name":"bare"}`, 400)
	// Turning the prefix off allows bare names for new skills.
	expect("PATCH", "/api/v2/profile", `{"hostedPrefix":""}`, 200)
	expect("PUT", "/api/v2/hosting/skills/bare", `{"content":"---\nname: bare\ndescription: A skill.\n---\n\nInstructions.\n","files":{}}`, 200)
}
