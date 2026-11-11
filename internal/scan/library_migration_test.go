package scan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

type migrationEnv struct {
	home, config, root string
	c                  agentConfig
	target             store.SyncTarget
	mu                 sync.Mutex
	asked              []string
	// answer decides the controller's reply per item id; before runs inside the request.
	answer func(itemID string, w http.ResponseWriter)
	before func(itemID string)
}

func (e *migrationEnv) requests() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.asked...)
}

func newMigrationEnv(t *testing.T) *migrationEnv {
	t.Helper()
	e := &migrationEnv{}
	e.home = t.TempDir()
	t.Setenv("HOME", e.home)
	e.root = filepath.Join(e.home, ".claude", "skills")
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var check store.LibraryDeploymentCheck
		_ = json.NewDecoder(r.Body).Decode(&check)
		if r.URL.Path != "/api/v2/library/install-evidence" {
			http.NotFound(w, r)
			return
		}
		e.mu.Lock()
		e.asked = append(e.asked, check.ItemID)
		e.mu.Unlock()
		if e.before != nil {
			e.before(check.ItemID)
		}
		e.answer(check.ItemID, w)
	}))
	t.Cleanup(controller.Close)
	hostname, _ := os.Hostname()
	e.c = agentConfig{Server: controller.URL, ID: strings.Repeat("b", 32), Hostname: hostname, Home: e.home, Token: "fixture", Secret: "fixture", AllowApply: true}
	e.config = filepath.Join(e.home, "agent.json")
	if err := writePrivate(e.config, e.c); err != nil {
		t.Fatal(err)
	}
	e.target = store.SyncTarget{Canonical: true, Target: store.Environment{ID: "target", Host: "desktop", Agent: "claude"}}
	return e
}

func (e *migrationEnv) legacy(t *testing.T, name, item string) libraryBinding {
	t.Helper()
	dir := filepath.Join(e.root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: Legacy\n---\nBody "+name+".\n"), 0644); err != nil {
		t.Fatal(err)
	}
	digest, err := privateSkillDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A legacy binding: an edit base with no deployment receipt and no source flag.
	return libraryBinding{Path: dir, Paths: []string{"SKILL.md"}, ItemID: strings.Repeat(item, 32), Revision: strings.Repeat("d", 32), Digest: digest}
}

func (e *migrationEnv) run(t *testing.T, bindings ...libraryBinding) libraryLocalState {
	t.Helper()
	state := libraryLocalState{Server: e.c.Server, Enrollment: e.c.ID, ManagedRemovals: managedRemovalVersion, Bindings: bindings}
	if err := writePrivate(e.config+".library", state); err != nil {
		t.Fatal(err)
	}
	if _, _, err := planLibraryRemovals(context.Background(), e.config, e.c, e.target, e.root, "test"); err != nil {
		t.Fatal(err)
	}
	got, err := loadLibraryState(e.config, e.c)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func installed(w http.ResponseWriter, yes bool) {
	_ = json.NewEncoder(w).Encode(store.LibraryInstallEvidence{Installed: yes, JobID: "job"})
}

func TestLegacyBindingBecomesAVerifiedDeploymentOnlyWithInstallEvidence(t *testing.T) {
	e := newMigrationEnv(t)
	e.answer = func(item string, w http.ResponseWriter) { installed(w, item == strings.Repeat("1", 32)) }
	proven, unproven := e.legacy(t, "proven", "1"), e.legacy(t, "unproven", "2")
	state := e.run(t, proven, unproven)

	got := state.Bindings[0].Deployment
	if got == nil || !got.Verified || got.TargetID != "target" || got.Revision != proven.Revision || got.Digest != proven.Digest {
		t.Fatal("a binding with install evidence was not migrated to a verified deployment", got)
	}
	if !managedLibraryPath(state.Bindings[0], "target", e.root) {
		t.Fatal("a migrated binding is not recognised as managed")
	}
	if state.Bindings[1].Deployment != nil {
		t.Fatal("a binding with no install evidence was given ownership", state.Bindings[1].Deployment)
	}
	if _, err := os.Stat(filepath.Join(unproven.Path, "SKILL.md")); err != nil {
		t.Fatal("an unproven copy was touched", err)
	}
}

func TestMigrationNeverClaimsSourcesOwnedOrEditedCopies(t *testing.T) {
	e := newMigrationEnv(t)
	e.answer = func(_ string, w http.ResponseWriter) { installed(w, true) }
	source := e.legacy(t, "source", "1")
	source.Source = true
	owned := e.legacy(t, "owned", "2")
	owned.Deployment = &libraryDeployment{TargetID: "other", Revision: owned.Revision, Digest: owned.Digest, Verified: true}
	edited := e.legacy(t, "edited", "3")
	if err := os.WriteFile(filepath.Join(edited.Path, "SKILL.md"), []byte("locally edited after the base"), 0644); err != nil {
		t.Fatal(err)
	}
	state := e.run(t, source, owned, edited)

	if state.Bindings[0].Deployment != nil || !state.Bindings[0].Source {
		t.Fatal("an imported source became removable", state.Bindings[0])
	}
	if d := state.Bindings[1].Deployment; d == nil || d.TargetID != "other" {
		t.Fatal("an existing deployment record was rewritten", d)
	}
	if state.Bindings[2].Deployment != nil {
		t.Fatal("a locally edited copy was claimed", state.Bindings[2].Deployment)
	}
	if asked := e.requests(); len(asked) != 0 {
		t.Fatal("ineligible bindings asked the controller for evidence", asked)
	}
}

func TestMigrationOnlyConsidersTheTargetsOwnFolder(t *testing.T) {
	e := newMigrationEnv(t)
	e.answer = func(_ string, w http.ResponseWriter) { installed(w, true) }
	nested := e.legacy(t, "demo", "1")
	other := filepath.Join(e.home, ".agents", "skills", "elsewhere")
	if err := os.MkdirAll(other, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "SKILL.md"), []byte("elsewhere"), 0644); err != nil {
		t.Fatal(err)
	}
	digest, _ := privateSkillDigest(other)
	foreignFolder := libraryBinding{Path: other, Paths: []string{"SKILL.md"}, ItemID: strings.Repeat("2", 32), Revision: strings.Repeat("d", 32), Digest: digest}
	state := e.run(t, nested, foreignFolder)
	if state.Bindings[0].Deployment == nil || state.Bindings[1].Deployment != nil {
		t.Fatal("a binding outside this target's folder was migrated", state.Bindings)
	}
}

func TestMigrationFailuresAreIsolatedAndNeverFatal(t *testing.T) {
	// A controller that does not offer the endpoint (older) or refuses one binding leaves copies unowned and breaks nothing.
	e := newMigrationEnv(t)
	e.answer = func(item string, w http.ResponseWriter) {
		switch item {
		case strings.Repeat("1", 32):
			http.Error(w, "not found", http.StatusNotFound)
		case strings.Repeat("2", 32):
			http.Error(w, "refused", http.StatusForbidden)
		default:
			installed(w, true)
		}
	}
	state := e.run(t, e.legacy(t, "a", "1"), e.legacy(t, "b", "2"), e.legacy(t, "c", "3"))
	if state.Bindings[0].Deployment != nil || state.Bindings[1].Deployment != nil {
		t.Fatal("a refused or unavailable evidence request granted ownership")
	}
	if state.Bindings[2].Deployment == nil || !state.Bindings[2].Deployment.Verified {
		t.Fatal("a healthy binding was blocked by unrelated failures", state.Bindings[2])
	}
}

func TestMigrationRequiresLocalConsentAndAStableCopy(t *testing.T) {
	// Withdrawn consent: nothing is asked and nothing is granted.
	off := newMigrationEnv(t)
	off.answer = func(_ string, w http.ResponseWriter) { installed(w, true) }
	c2 := off.c
	c2.AllowApply = false
	if err := writePrivate(off.config, c2); err != nil {
		t.Fatal(err)
	}
	binding := off.legacy(t, "demo", "1")
	if err := writePrivate(off.config+".library", libraryLocalState{Server: off.c.Server, Enrollment: off.c.ID, ManagedRemovals: managedRemovalVersion, Bindings: []libraryBinding{binding}}); err != nil {
		t.Fatal(err)
	}
	// Planning itself refuses without consent; what matters is that nothing was granted.
	_, _, _ = planLibraryRemovals(context.Background(), off.config, off.c, off.target, off.root, "test")
	state, _ := loadLibraryState(off.config, off.c)
	if state.Bindings[0].Deployment != nil || len(off.requests()) != 0 {
		t.Fatal("migration ran without local consent", state.Bindings[0].Deployment, off.requests())
	}

	// A copy that changes while the controller is being asked is not claimed.
	race := newMigrationEnv(t)
	race.answer = func(_ string, w http.ResponseWriter) { installed(w, true) }
	racing := race.legacy(t, "demo", "1")
	race.before = func(string) {
		_ = os.WriteFile(filepath.Join(racing.Path, "SKILL.md"), []byte("changed during the request"), 0644)
	}
	state = race.run(t, racing)
	if state.Bindings[0].Deployment != nil {
		t.Fatal("a copy that changed during verification was claimed")
	}
}

func TestMigratedCopyThenFollowsTheNormalRemovalGate(t *testing.T) {
	// Migration only grants the right to be considered. Removal still needs
	// the negotiated capability and an authoritative remove intent.
	e := newMigrationEnv(t)
	e.answer = func(_ string, w http.ResponseWriter) { installed(w, true) }
	state := e.run(t, e.legacy(t, "demo", "1"))
	if state.Bindings[0].Deployment == nil {
		t.Fatal("not migrated")
	}
	// With the capability withdrawn, a migrated copy plans nothing.
	state.ManagedRemovals = 0
	if err := writePrivate(e.config+".library", state); err != nil {
		t.Fatal(err)
	}
	actions, _, err := planLibraryRemovals(context.Background(), e.config, e.c, e.target, e.root, "test")
	if err != nil || len(actions) != 0 {
		t.Fatal("a migrated copy was planned for removal without the capability", actions, err)
	}
}
