//go:build linux || darwin

package scan

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillfetch"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// e2eFetcher stands for GitHub: every public skill it is asked for is one
// SKILL.md named after the skill.
type e2eFetcher struct{}

func (e2eFetcher) Fetch(_ context.Context, r ref.Ref) (skillfetch.Result, error) {
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: " + r.Slug + "\ndescription: Fetched for the test\n---\nInstructions for " + r.Slug + ".\n")}}
	m, err := skillbundle.FromContents(files)
	return skillfetch.Result{Files: files, Manifest: m, Origin: "https://github.com/" + r.Source}, err
}

// e2eMachine is one enrolled machine: its home folder and enrollment file.
type e2eMachine struct{ name, home, config string }

func (m e2eMachine) skills() string { return filepath.Join(m.home, ".agents", "skills") }

// present reports whether the machine has name in ~/.agents/skills.
func (m e2eMachine) present(t *testing.T, name string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(m.skills(), name))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

// backups counts the machine's backups of name from ~/.agents/skills.
func (m e2eMachine) backups(t *testing.T, name string) int {
	t.Helper()
	listed, err := listBackups(m.home)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, b := range listed {
		if b.Name == name && b.Folder == m.skills() {
			n++
		}
	}
	return n
}

// TestSkillsEndToEnd drives a real controller and real agents, one cycle at a
// time, each in a temporary home folder, and checks each step on disk.
func TestSkillsEndToEnd(t *testing.T) {
	t.Setenv("REET_TOKEN", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := st.DefaultOwner()
	// Skill content is private, so it needs an account: the owner's token.
	if _, err = st.CreateAPIToken(uid, "owner", auth.HashToken("e2e-owner")); err != nil {
		t.Fatal(err)
	}
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "offline", 500) }))
	h := api.New(st, meta.Fetcher{Client: dead.Client(), PageBase: dead.URL + "/", APIBase: dead.URL}, "", api.WithAuthMode(api.AuthAccounts), api.WithSkillFetcher(e2eFetcher{}))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close(); dead.Close() })
	enroll := func(name string) e2eMachine {
		t.Helper()
		m := e2eMachine{name: name, home: t.TempDir()}
		m.config = filepath.Join(m.home, ".config", "reet", "agent.json")
		t.Setenv("HOME", m.home)
		if err := RunAgent([]string{"enroll", "--server", srv.URL, "--token", "e2e-owner", "--env", name, "--config", m.config, "--allow-apply", "--skip-terminal-setup"}, io.Discard, "test"); err != nil {
			t.Fatal(err)
		}
		return m
	}
	cycle := func(machines ...e2eMachine) {
		t.Helper()
		for _, m := range machines {
			t.Setenv("HOME", m.home)
			if err := RunAgent([]string{"run", "--once", "--config", m.config}, io.Discard, "test"); err != nil {
				t.Fatalf("%s: %v", m.name, err)
			}
		}
	}
	healthOf := func(item string) store.SkillHealth {
		t.Helper()
		m, err := st.SkillHealthFor(uid, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return m[item]
	}
	// A change made this long ago has held for the grace period, so the next
	// poll carries it.
	settled := func() time.Time { return time.Now().Add(-store.SkillGrace - time.Second) }
	studio := enroll("studio")

	// 1. Add a skill by link: the controller fetches it, and one cycle
	// installs it.
	code, body := agentTestRequest(t, srv.URL, "POST", "/api/skills", `{"input":"https://www.skills.sh/vercel-labs/skills/find-skills"}`)
	if code != 200 {
		t.Fatalf("add: %d %s", code, body)
	}
	var item, digest string
	for deadline := time.Now().Add(10 * time.Second); item == ""; {
		d, err := st.DesiredSkillsFor(uid, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range d.Active {
			if a.Name == "find-skills" {
				item, digest = a.Item, a.Digest
			}
		}
		if item == "" && time.Now().After(deadline) {
			t.Fatalf("the controller never got the skill's files: %+v", d)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cycle(studio)
	if !studio.present(t, "find-skills") || folderDigest(t, filepath.Join(studio.skills(), "find-skills")) != digest {
		t.Fatal("step 1: the added skill was not installed from reet's copy")
	}
	if h := healthOf(item); h.State != "ok" || h.Total != 1 {
		t.Fatalf("step 1: health %+v", h)
	}

	// 2. Archive it. Once the grace period is over, one cycle backs it up
	// and removes it.
	if err = st.SetSkillStatusFor(uid, []string{item}, store.SkillArchived, settled()); err != nil {
		t.Fatal(err)
	}
	cycle(studio)
	if studio.present(t, "find-skills") || studio.backups(t, "find-skills") != 1 {
		t.Fatalf("step 2: archived skill present=%t, backups=%d", studio.present(t, "find-skills"), studio.backups(t, "find-skills"))
	}
	if h := healthOf(item); h.State != "ok" {
		t.Fatalf("step 2: health %+v", h)
	}

	// 3. Restore it: it is installed again.
	if err = st.SetSkillStatusFor(uid, []string{item}, store.SkillActive, settled()); err != nil {
		t.Fatal(err)
	}
	cycle(studio)
	if !studio.present(t, "find-skills") || folderDigest(t, filepath.Join(studio.skills(), "find-skills")) != digest {
		t.Fatal("step 3: the restored skill was not installed again")
	}

	// 4. A skill put on a machine by hand is asked about, and left alone:
	// it is never copied into reet nor spread to another machine.
	cq := filepath.Join(studio.skills(), "cq")
	if err = os.MkdirAll(cq, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(cq, "SKILL.md"), []byte("---\nname: cq\ndescription: Made on this machine\n---\nAsk questions.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	before := folderDigest(t, cq)
	cycle(studio)
	newOn := func(when string) {
		t.Helper()
		found, err := st.NewOnMachinesFor(uid)
		if err != nil || len(found) != 1 || found[0].Name != "cq" || len(found[0].Copies) != 1 || found[0].Copies[0].Host != "studio" {
			t.Fatalf("step 4, %s: new on a machine %+v %v", when, found, err)
		}
	}
	newOn("after one cycle")
	laptop := enroll("laptop")
	for range 3 {
		cycle(studio, laptop)
	}
	newOn("three cycles later")
	items, err := st.LibraryFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if strings.EqualFold(it.Name, "cq") {
			t.Fatalf("step 4: cq became a library item: %+v", it)
		}
	}
	needs, err := st.SkillsNeedingCopyFor(uid)
	if err != nil || len(needs) != 0 {
		t.Fatalf("step 4: copies planned %+v %v", needs, err)
	}
	if requests, _, err := st.RemoteImportsFor(uid); err != nil || len(requests) != 0 {
		t.Fatalf("step 4: captures asked for %+v %v", requests, err)
	}
	if d, err := st.DesiredSkillsFor(uid, time.Now()); err != nil || len(d.Active) != 1 || d.Active[0].Name != "find-skills" {
		t.Fatalf("step 4: desired %+v %v", d, err)
	}
	if !studio.present(t, "cq") || folderDigest(t, cq) != before {
		t.Fatal("step 4: the skill found on the machine was changed")
	}
	if laptop.present(t, "cq") || !laptop.present(t, "find-skills") {
		t.Fatalf("step 4: laptop has cq=%t find-skills=%t", laptop.present(t, "cq"), laptop.present(t, "find-skills"))
	}

	// 5. Keep it; a second home gets it. Task 9 adds this step with
	// POST /api/v2/skills/new.

	// 6. Delete the skill: it is gone from disk on both machines, with a
	// backup, and once both have confirmed it, its name is forgotten.
	if err = st.DeleteSkillsFor(uid, []string{item}, false, settled()); err != nil {
		t.Fatal(err)
	}
	if !onRemovalList(t, st, uid, "find-skills") {
		t.Fatal("step 6: the deleted name is not on the removal list")
	}
	cycle(studio)
	if studio.present(t, "find-skills") || studio.backups(t, "find-skills") != 2 {
		t.Fatalf("step 6: deleted skill present=%t, backups=%d", studio.present(t, "find-skills"), studio.backups(t, "find-skills"))
	}
	if !onRemovalList(t, st, uid, "find-skills") {
		t.Fatal("step 6: the name was forgotten before every machine confirmed it")
	}
	cycle(laptop)
	if laptop.present(t, "find-skills") || laptop.backups(t, "find-skills") != 1 {
		t.Fatalf("step 6: laptop present=%t, backups=%d", laptop.present(t, "find-skills"), laptop.backups(t, "find-skills"))
	}
	if onRemovalList(t, st, uid, "find-skills") {
		t.Fatal("step 6: the name was not forgotten after both machines confirmed it")
	}
	if !studio.present(t, "cq") {
		t.Fatal("step 6: the skill found on the machine went too")
	}
}

func onRemovalList(t *testing.T, st *store.Store, uid int64, name string) bool {
	t.Helper()
	removals, err := st.SkillRemovalsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range removals {
		if strings.EqualFold(r.Name, name) {
			return true
		}
	}
	return false
}

func agentTestRequest(t *testing.T, server, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, server+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reet-Token", "e2e-owner")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
