package api

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

const hostedDemo = "---\nname: reet-demo\ndescription: A hosted skill.\n---\n\nFollow these instructions.\n"

// publishAsBob makes another account publish the skill reet-demo under bob and
// returns the link a person adds it by, and bob's account.
func publishAsBob(t *testing.T, e copyEnv) (string, int64) {
	t.Helper()
	bob, err := e.st.CreateUser("bob@example.com", "Bob", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.st.ClaimPublishingUsername(bob.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	sk := store.HostedSkill{Slug: "reet-demo", Content: hostedDemo, Files: map[string][]byte{"references/help.md": []byte("Supporting text")}, Published: true}
	if _, err = e.st.SaveHostedSkill(bob.ID, sk); err != nil {
		t.Fatal(err)
	}
	return e.srv.URL + "/bob/reet-demo", bob.ID
}

// A skill added from someone else's published page has no machine to copy it
// from and no download to make: reet copies it from the page.
func TestForeignHostedSkillGetsTheFilesOfItsPublishedPage(t *testing.T) {
	e := newCopyEnv(t)
	link, _ := publishAsBob(t, e)
	if code, body := do(t, e.srv, "POST", "/api/skills", `{"input":"`+link+`"}`); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	e.settle()
	d, err := e.st.DesiredSkillsFor(e.uid, time.Now())
	if err != nil || len(d.Active) != 1 || d.Active[0].Name != "reet-demo" || d.Active[0].Revision == "" {
		t.Fatalf("desired: %+v %v health %+v", d, err, e.health(t))
	}
	rev, err := e.st.PrivateSkillRevisionFor(e.uid, d.Active[0].Bundle, d.Active[0].Revision)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range rev.Files {
		got = append(got, f.Path)
	}
	if strings.Join(got, ",") != "SKILL.md,references/help.md" {
		t.Fatalf("files = %v", got)
	}
	origin, reference, err := e.st.SkillOriginFor(e.uid, d.Active[0].Item)
	if err != nil || origin != "imported" || reference != link {
		t.Fatalf("origin %q reference %q (want %q) %v", origin, reference, link, err)
	}
	if h := e.health(t); h.State == "getting" || h.State == "fetch-failed" || h.State == "copy-failed" {
		t.Fatalf("health: %+v", h)
	}
	if n := e.fetch.callCount(); n != 0 {
		t.Fatalf("a page of this server is not downloaded: %d fetches", n)
	}
	// There is no original to download for it.
	if code, _ := do(t, e.srv, "GET", "/api/v2/skills/"+d.Active[0].Item+"/original", ""); code != 404 {
		t.Fatalf("original of a hosted skill: %d", code)
	}
}

// A page unpublished between adding the skill and the copy says so on the
// skill instead of leaving it silent.
func TestForeignHostedSkillUnpublishedBeforeTheCopySaysSo(t *testing.T) {
	e := newCopyEnv(t)
	link, bob := publishAsBob(t, e)
	rf, err := ref.ParseAt(link, strings.TrimPrefix(e.srv.URL, "http://"))
	if err != nil || rf.SourceType != "hosted" {
		t.Fatalf("ref %+v %v", rf, err)
	}
	if _, err = e.st.AddFor(e.uid, rf, store.Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err = e.st.SaveHostedSkill(bob, store.HostedSkill{Slug: "reet-demo", Content: hostedDemo}); err != nil {
		t.Fatal(err)
	}
	e.api.s.kickSkillCopies(e.uid)
	e.settle()
	if h := e.health(t); h.State != "fetch-failed" || !strings.Contains(h.Reason, "no longer published") {
		t.Fatalf("health: %+v", h)
	}
	if d, _ := e.st.DesiredSkillsFor(e.uid, time.Now()); len(d.Active) != 0 {
		t.Fatalf("nothing is saved: %+v", d)
	}
}
