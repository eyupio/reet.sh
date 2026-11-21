package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/ref"
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
	expect(alice.ID, "POST", "/api/v2/hosting/username", map[string]string{"username": "docs"}, 400)
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
	if strings.Contains(page, "<script>alert") || !strings.Contains(page, "npx skills@1.7.4 add "+srv.URL+"/alice/reet-demo") {
		t.Fatalf("unsafe page or wrong install command: %s", page)
	}
	for _, want := range []string{
		`name="robots" content="index,follow"`,
		`rel="canonical" href="` + srv.URL + `/alice/reet-demo"`,
		`property="og:url" content="` + srv.URL + `/alice/reet-demo"`,
		`property="og:image" content="` + srv.URL + `/og-image.png"`,
		`"@type":"SoftwareSourceCode"`,
		`"@type":"BreadcrumbList"`,
		`href="/community"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("hosted page missing %s", want)
		}
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
	// Bob's library already has a reet-demo of his own, so he cannot add
	// Alice's under the same name until he deletes his.
	expect(bob.ID, "POST", "/api/skills", map[string]string{"input": srv.URL + "/alice/reet-demo"}, 409)
	expect(bob.ID, "DELETE", "/api/v2/hosting/skills/reet-demo", nil, 200)
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
	if aliceSkill.ID == "" || aliceSkill.SourceType != "hosted" || aliceSkill.Status != store.SkillActive {
		t.Fatalf("hosted references not deduplicated or not Active: %s", raw)
	}
	if got := installCommand(aliceSkill); got != "npx skills@1.7.4 add "+srv.URL+"/alice/reet-demo --skill reet-demo" {
		t.Fatal(got)
	}
	// The skills CLI matches on the skill's own name, so a name that is not the
	// folder name is offered as well, quoted for a shell.
	named := store.Skill{Name: "SVG Logo Designer", Slug: "svg-logo-designer", Source: "rknall/claude-skills", SourceType: "github"}
	if got := installCommand(named); got != "npx skills@1.7.4 add https://github.com/rknall/claude-skills --skill svg-logo-designer 'SVG Logo Designer'" {
		t.Fatal(got)
	}
	shelfHas := func(uid int64, skillID string) bool {
		t.Helper()
		return strings.Contains(expect(uid, "GET", "/api/skills", nil, 200), `"id":"`+skillID+`"`)
	}
	if !shelfHas(alice.ID, aliceSkill.ID) || !shelfHas(bob.ID, aliceSkill.ID) {
		t.Fatal("published skill missing from a shelf")
	}
	expect(bob.ID, "POST", "/api/skill/refresh", map[string]string{"id": aliceSkill.ID}, 200)
	sk.Published = false
	expect(alice.ID, "PUT", "/api/v2/hosting/skills/reet-demo", sk, 200)
	if shelfHas(alice.ID, aliceSkill.ID) || shelfHas(bob.ID, aliceSkill.ID) {
		t.Fatal("unpublished skill stayed on a shelf")
	}
	for _, path := range paths {
		expect(0, "GET", path, nil, 404)
	}
	expect(bob.ID, "POST", "/api/skills", map[string]string{"input": srv.URL + "/alice/reet-demo"}, 400)
	sk.Published = true
	expect(alice.ID, "PUT", "/api/v2/hosting/skills/reet-demo", sk, 200)
	if !shelfHas(alice.ID, aliceSkill.ID) || !shelfHas(bob.ID, aliceSkill.ID) {
		t.Fatal("republished skill did not return to its shelves")
	}
	if err := st.SetUserDisabled(alice.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		expect(0, "GET", path, nil, 404)
	}
	st.SetUserDisabled(alice.ID, false)
	expect(alice.ID, "DELETE", "/api/v2/hosting/skills/reet-demo", nil, 200)
	if shelfHas(alice.ID, aliceSkill.ID) || shelfHas(bob.ID, aliceSkill.ID) {
		t.Fatal("deleted skill stayed on a shelf")
	}
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
	for path, want := range map[string]string{"/": "spa", "/account": "spa", "/machines": "spa", "/machines?view=compare": "spa", "/sync": "spa", "/sync/advanced": "spa", "/sync/advanced?machine=dev": "spa", "/sync/other": "hosted", "/sync/advanced/more": "hosted", "/setups": "spa", "/machines/demo": "hosted", "/projects": "spa", "/projects/0123456789abcdef0123456789abcdef": "spa", "/projects/abc/": "spa", "/hosting": "spa", "/lists/1": "spa", "/desks": "spa", "/desks/0123456789abcdef0123456789abcdef": "spa", "/desks/0123456789abcdef0123456789abcdef/": "spa", "/api/skills": "spa", "/alice/reet-demo": "hosted", "/alice/reet-demo/.well-known/agent-skills/index.json": "hosted"} {
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
	if len(shelf.Skills) != 1 || shelf.Skills[0].Status != store.SkillActive || shelf.Skills[0].SourceType != "hosted" {
		t.Fatalf("published skill not added as Active: %+v", shelf.Skills)
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

// One live library item per name: adding a link, writing a skill or uploading
// content under a name the library already has is refused with 409.
func TestLibraryNameAlreadyTakenIsAConflict(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "")
	owner, _ := st.CreateUser("names@example.com", "Names", "member", "hash")
	st.CreateAPIToken(owner.ID, "test", auth.HashToken("names-token"))
	call := func(method, path string, in any) (int, string) {
		t.Helper()
		body := ""
		if s, ok := in.(string); ok {
			body = s
		} else if in != nil {
			b, _ := json.Marshal(in)
			body = string(b)
		}
		return do(t, srv, method, path, body, "X-Reet-Token", "names-token")
	}
	// Written skills take the account's default name prefix, so the name is one.
	name := store.DefaultHostedPrefix + "notes"
	if code, body := call("POST", "/api/skills", `{"input":"https://www.skills.sh/someone/skills/`+name+`"}`); code != 200 {
		t.Fatalf("first add: %d %s", code, body)
	}
	if code, body := call("POST", "/api/skills", `{"input":"https://www.skills.sh/someone-else/skills/`+name+`"}`); code != 409 || !strings.Contains(body, "already in your library") {
		t.Fatalf("a second source of the name: %d %s", code, body)
	}
	if code, body := call("POST", "/api/v2/hosting/username", map[string]string{"username": "names"}); code != 200 {
		t.Fatalf("username: %d %s", code, body)
	}
	written := store.HostedSkill{Content: "---\nname: " + name + "\ndescription: Mine\n---\nBody\n"}
	if code, body := call("PUT", "/api/v2/hosting/skills/"+name, written); code != 409 {
		t.Fatalf("a written skill took a library name: %d %s", code, body)
	}
}

// A machine copy of an Archived skill, reported with a source other than the
// skill's own, is a sighting: no scan, first or repeated, makes it Active or
// takes its name off the removal list. Only a person's stage action does.
func TestRepeatScanOfAnArchivedSkillLeavesItArchived(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "")
	owner, _ := st.CreateUser("scans@example.com", "Scans", "member", "hash")
	st.CreateAPIToken(owner.ID, "test", auth.HashToken("scan-token"))
	if err := st.ClaimPublishingUsername(owner.ID, "scanner"); err != nil {
		t.Fatal(err)
	}
	name := store.DefaultHostedPrefix + "scanned"
	if _, err := st.SaveHostedSkill(owner.ID, store.HostedSkill{Slug: name, Content: "---\nname: " + name + "\ndescription: Written\n---\nBody\n"}); err != nil {
		t.Fatal(err)
	}
	items, err := st.LibraryFor(owner.ID)
	if err != nil || len(items) != 1 || items[0].RevisionID == "" {
		t.Fatal("written skill missing its copy", items, err)
	}
	item := items[0]
	if err = st.SetSkillStatusFor(owner.ID, []string{item.ID}, store.SkillArchived, time.Now()); err != nil {
		t.Fatal(err)
	}
	scan := `{"items":[{"name":"` + name + `","source":"someone/else","host":"laptop","agent":"claude","path":"/home/u/.claude/skills"}]}`
	for i := 1; i <= 3; i++ {
		if code, body := do(t, srv, "POST", "/api/import", scan, "X-Reet-Token", "scan-token"); code != 200 {
			t.Fatalf("scan %d: %d %s", i, code, body)
		}
		got, err := st.LibraryItemFor(owner.ID, item.ID)
		if err != nil || got.Status != store.SkillArchived {
			t.Fatalf("scan %d changed the status: %+v %v", i, got, err)
		}
		removals, err := st.SkillRemovalsFor(owner.ID)
		if err != nil || len(removals) != 1 || removals[0].Name != name || removals[0].Reason != "archived" {
			t.Fatalf("scan %d changed the removal list: %+v %v", i, removals, err)
		}
	}
}

// A written skill cannot take a name the library already has: saving a new one
// and renaming one onto it are both 409.
func TestHostedSaveAndRenameRefuseATakenLibraryName(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "")
	alice, _ := st.CreateUser("author@example.com", "Author", "member", "hash")
	token := "hosted-taken-token"
	st.CreateAPIToken(alice.ID, "test", auth.HashToken(token))
	expect := func(method, path, body string, want int) {
		t.Helper()
		if code, out := do(t, srv, method, path, body, "X-Reet-Token", token); code != want {
			t.Fatalf("%s %s: %d %s; want %d", method, path, code, out, want)
		}
	}
	expect("POST", "/api/v2/hosting/username", `{"username":"alice"}`, 200)
	expect("PATCH", "/api/v2/profile", `{"hostedPrefix":""}`, 200)
	if _, err := st.UpsertFor(alice.ID, ref.Ref{ID: "a/b/Taken-Name", Source: "a/b", Slug: "Taken-Name", SourceType: "github", URL: "https://www.skills.sh/a/b/Taken-Name"}, store.Patch{}); err != nil {
		t.Fatal(err)
	}
	content := func(name string) string {
		return fmt.Sprintf(`{"content":"---\nname: %s\ndescription: A skill.\n---\n\nInstructions.\n","files":{}}`, name)
	}
	expect("PUT", "/api/v2/hosting/skills/taken-name", content("taken-name"), 409)
	expect("PUT", "/api/v2/hosting/skills/mine", content("mine"), 200)
	expect("POST", "/api/v2/hosting/skills/mine/rename", `{"name":"taken-name"}`, 409)
}
