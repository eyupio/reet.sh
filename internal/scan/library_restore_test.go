package scan

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

func restoreBinding(t *testing.T, root string) (libraryBinding, string) {
	t.Helper()
	path := filepath.Join(root, "demo")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"SKILL.md": "---\nname: demo\ndescription: Restore fixture\n---\nBody.\n", "reference.txt": "support file"} {
		if err := os.WriteFile(filepath.Join(path, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	digest, err := privateSkillDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	return libraryBinding{Path: path, Paths: []string{"SKILL.md", "reference.txt"}, ItemID: strings.Repeat("c", 32), Revision: strings.Repeat("d", 32), Digest: digest, Deployment: &libraryDeployment{TargetID: "target", Revision: strings.Repeat("d", 32), Digest: digest, Verified: true}}, path
}

func TestDeletionObservationIsDebounced(t *testing.T) {
	root := t.TempDir()
	binding, path := restoreBinding(t, root)
	state := libraryLocalState{Bindings: []libraryBinding{binding}}
	start := time.Unix(1_800_000_000, 0)
	window := libraryRestoreDebounce

	// An intact bundle records nothing.
	observeLibraryDeletions(&state, start)
	if len(state.Missing) != 0 {
		t.Fatal("intact bundle recorded as missing", state.Missing)
	}

	// An atomic editor save: the file is briefly absent in one observation and
	// back by the next, so it can never become stable.
	missing := filepath.Join(path, "reference.txt")
	if err := os.Rename(missing, missing+".tmp"); err != nil {
		t.Fatal(err)
	}
	observeLibraryDeletions(&state, start)
	if len(state.Missing) != 1 || state.Missing[0].stable(window) {
		t.Fatal("first sighting was already stable", state.Missing)
	}
	if err := os.Rename(missing+".tmp", missing); err != nil {
		t.Fatal(err)
	}
	observeLibraryDeletions(&state, start.Add(window*2))
	if len(state.Missing) != 0 {
		t.Fatal("a file that came back kept its deletion record", state.Missing)
	}

	// A real deletion stays missing across observations spanning the window.
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	observeLibraryDeletions(&state, start.Add(window*3))
	observeLibraryDeletions(&state, start.Add(window*3+window/2))
	if state.Missing[0].stable(window) || state.Missing[0].Count != 2 {
		t.Fatal("stable before the window elapsed", state.Missing)
	}
	observeLibraryDeletions(&state, start.Add(window*4+time.Second))
	if len(state.Missing) != 1 || !state.Missing[0].stable(window) || state.Missing[0].Whole || len(state.Missing[0].Files) != 1 || state.Missing[0].Files[0] != "reference.txt" {
		t.Fatal("a persisting deletion was not classified as stable", state.Missing)
	}

	// A different missing set is a new episode: the clock starts again.
	if err := os.Remove(filepath.Join(path, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	observeLibraryDeletions(&state, start.Add(window*5))
	if len(state.Missing) != 1 || state.Missing[0].Count != 1 || state.Missing[0].stable(window) || len(state.Missing[0].Files) != 2 {
		t.Fatal("a changed deletion did not restart the debounce", state.Missing)
	}
}

func TestWholeDirectoryDeletionAndOwnershipBoundaries(t *testing.T) {
	root := t.TempDir()
	binding, path := restoreBinding(t, root)
	unowned := binding
	unowned.Path = filepath.Join(root, "imported")
	unowned.Source = true
	removed := binding
	removed.Path = filepath.Join(root, "removed")
	removed.Deployment = &libraryDeployment{TargetID: "target", Verified: true, Removal: &libraryRemoval{Phase: "prepared"}}
	unverified := binding
	unverified.Path = filepath.Join(root, "unverified")
	unverified.Deployment = &libraryDeployment{TargetID: "target"}
	state := libraryLocalState{Bindings: []libraryBinding{binding, unowned, removed, unverified}}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	observeLibraryDeletions(&state, now)
	// Only the verified, non-source, non-removing binding is a deletion; the
	// others were never present on disk and must not be treated as managed.
	if len(state.Missing) != 1 || state.Missing[0].Path != path || !state.Missing[0].Whole {
		t.Fatal("wrong deletion records", state.Missing)
	}
}

func TestDebouncedRestoreReinstallsAndRecordsVerifiedReceipt(t *testing.T) {
	old := libraryRestoreDebounce
	libraryRestoreDebounce = 40 * time.Millisecond
	defer func() { libraryRestoreDebounce = old }()
	home := t.TempDir()
	t.Setenv("HOME", home)
	directory := filepath.Join(home, ".agents", "skills")
	config := filepath.Join(home, "agent.json")
	files := []skillbundle.ContentFile{
		{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Restore fixture\n---\nInstructions.\n")},
		{Path: "reference.txt", Data: []byte("support file")},
	}
	manifest, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	revision := store.PrivateSkillRevision{ID: strings.Repeat("b", 32), BundleID: strings.Repeat("c", 32), ManifestDigest: manifest.Digest, Files: files}
	skill := store.SyncSkill{ID: "local/demo", ItemID: strings.Repeat("a", 32), Name: "demo", Slug: "demo", SourceType: "private", Source: "private:" + revision.BundleID + ":" + revision.ID + ":" + manifest.Digest}
	target := store.SyncTarget{Canonical: true, Target: store.Environment{ID: "target", Name: "Desktop", Host: "desktop", Agent: "agents"}, Mode: "additive", Revision: "revision", Desired: []store.SyncSkill{skill}}
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/library/bases":
			var base store.LibraryBase
			_ = json.NewDecoder(r.Body).Decode(&base)
			_ = json.NewEncoder(w).Encode(base)
		case strings.HasPrefix(r.URL.Path, "/api/v2/skill-bundles/"):
			_ = json.NewEncoder(w).Encode(revision)
		case r.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]any{"canWrite": true, "targets": []store.SyncTarget{target}})
		default:
			_, _ = w.Write([]byte(`{"errors":[]}`))
		}
	}))
	defer controller.Close()
	hostname, _ := os.Hostname()
	c := agentConfig{Server: controller.URL, ID: strings.Repeat("d", 32), Token: "fixture", Secret: "fixture", AllowApply: true, Home: home, Hostname: hostname}
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	var plan SyncPlan
	var backups []string
	opts := syncOptions{onInstalled: func(s store.SyncSkill, path string, files []skillbundle.ContentFile) error {
		return prepareLibraryInstall(context.Background(), config, c, s, path, files, "test", target.Target.ID)
	}, onInstallVerified: func(s store.SyncSkill, path string) error {
		return finishLibraryInstall(config, c, s, path, target.Target.ID)
	}, managedDeletions: func(target store.SyncTarget, path string) (map[string]bool, map[string]bool, error) {
		return planLibraryRestores(config, c, target, path)
	}, onPlan: func(p SyncPlan) { plan = p }, onBackup: func(path string) { backups = append(backups, path) }}
	run := func(apply bool) {
		t.Helper()
		args := []string{"--server", c.Server, "--token", c.Token, "--env", "desktop", "--target", target.Target.ID}
		if apply {
			args = append(args, "--apply")
		}
		plan = SyncPlan{}
		if err := runSyncWithOptions(args, strings.NewReader("yes\n"), io.Discard, func([]string, string) error { t.Fatal("private install invoked npx"); return nil }, "test", opts); err != nil {
			t.Fatal(err)
		}
	}
	run(true) // the first install is not a restore and is never held back
	state, err := loadLibraryState(config, c)
	if err != nil || len(state.Restores) != 0 || state.Bindings[0].Deployment == nil || !state.Bindings[0].Deployment.Verified {
		t.Fatal("first install", state, err)
	}
	if err := os.Remove(filepath.Join(directory, "demo", "reference.txt")); err != nil {
		t.Fatal(err)
	}

	run(false) // first sighting
	if len(plan.Actions) != 0 {
		t.Fatal("a first sighting planned a restore", plan.Actions)
	}
	time.Sleep(80 * time.Millisecond)
	run(false) // the same deletion, still missing after the window
	if len(plan.Actions) != 1 || !plan.Actions[0].Add || !plan.Actions[0].Replace || plan.Actions[0].Skill.Slug != "demo" {
		t.Fatal("a stable deletion was not planned for restore", plan.Actions)
	}
	state, _ = loadLibraryState(config, c)
	if len(state.Restores) != 0 {
		t.Fatal("a receipt was recorded before any restore was verified")
	}

	run(true)
	got, err := os.ReadFile(filepath.Join(directory, "demo", "reference.txt"))
	if err != nil || string(got) != "support file" {
		t.Fatal("deleted file was not restored from canonical bytes", string(got), err)
	}
	if len(backups) != 1 {
		t.Fatal("surviving local files were not preserved before replacing", backups)
	}
	if _, err := os.Stat(filepath.Join(backups[0], "SKILL.md")); err != nil {
		t.Fatal("survivor missing from backup", err)
	}
	state, _ = loadLibraryState(config, c)
	if len(state.Missing) != 0 || len(state.Restores) != 1 || state.Restores[0].Files != 1 || state.Restores[0].Whole || state.Restores[0].ItemID != skill.ItemID || state.Restores[0].Episode == "" {
		t.Fatal("verified restore receipt is wrong", state.Missing, state.Restores)
	}

	// Repeating the apply with nothing missing creates no second receipt.
	run(true)
	again, _ := loadLibraryState(config, c)
	if len(again.Restores) != 1 {
		t.Fatal("a repeated sync duplicated the restore receipt", again.Restores)
	}
}

func TestRestoreReceiptsAreSentUntilAcceptedAndOnlyStableDeletionsAskForASync(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "agent.json")
	var mu sync.Mutex
	status := map[string]int{}
	seen := map[string][]string{}
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Episode   string `json:"episode"`
			BindingID string `json:"bindingId"`
			ItemID    string `json:"itemId"`
		}
		_ = json.Unmarshal(body, &payload)
		key := payload.Episode
		if r.URL.Path == "/api/v2/library/deletion-observed" {
			key = payload.ItemID
		}
		mu.Lock()
		seen[r.URL.Path] = append(seen[r.URL.Path], key+"|"+payload.BindingID)
		code := status[key]
		mu.Unlock()
		if code != 0 && code != 200 {
			http.Error(w, "refused", code)
			return
		}
		if r.URL.Path == "/api/v2/library/restore-receipt" {
			_, _ = w.Write([]byte(`{"accepted":true,"notified":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"queued":true}`))
	}))
	defer controller.Close()
	c := agentConfig{Server: controller.URL, ID: strings.Repeat("d", 32), Token: "fixture", Secret: "fixture", Home: home}
	ep := func(ch string) string { return strings.Repeat(ch, 32) }
	status[ep("2")] = http.StatusConflict  // transient: paused or changed
	status[ep("3")] = http.StatusForbidden // permanent: the binding is gone
	restore := func(ch string) libraryRestore {
		return libraryRestore{Episode: ep(ch), Path: filepath.Join(home, "skills", ch), TargetID: "target", ItemID: strings.Repeat(ch, 32), Revision: strings.Repeat("d", 32), Files: 1}
	}
	state := libraryLocalState{Restores: []libraryRestore{restore("1"), restore("2"), restore("3")}}
	reportLibraryRestores(context.Background(), c, config, &state, "test", io.Discard)
	if len(state.Restores) != 1 || state.Restores[0].Episode != ep("2") {
		t.Fatal("accepted and permanently refused receipts must be dropped, a transient refusal kept", state.Restores)
	}
	persisted, _ := loadLibraryState(config, c)
	if len(persisted.Restores) != 1 {
		t.Fatal("receipt bookkeeping was not persisted", persisted.Restores)
	}
	status[ep("2")] = 0
	reportLibraryRestores(context.Background(), c, config, &state, "test", io.Discard)
	if len(state.Restores) != 0 {
		t.Fatal("a receipt was not dropped once accepted", state.Restores)
	}

	// Only a stable deletion with a known binding asks for a sync, with the binding id the controller knows.
	now := time.Now()
	window := libraryRestoreDebounce
	stable := libraryMissing{Path: filepath.Join(home, "skills", "a"), TargetID: "target", ItemID: strings.Repeat("a", 32), Files: []string{"x"}, FirstSeen: now.Add(-3 * window).UnixMilli(), LastSeen: now.UnixMilli(), Count: 3}
	young := libraryMissing{Path: filepath.Join(home, "skills", "b"), TargetID: "target", ItemID: strings.Repeat("b", 32), Files: []string{"x"}, FirstSeen: now.UnixMilli(), LastSeen: now.UnixMilli(), Count: 1}
	state = libraryLocalState{Missing: []libraryMissing{stable, young}, Bindings: []libraryBinding{{Path: stable.Path, ItemID: stable.ItemID, Revision: strings.Repeat("d", 32)}, {Path: young.Path, ItemID: young.ItemID, Revision: strings.Repeat("d", 32)}}}
	requestLibraryRestoreSync(context.Background(), c, &state, "test", io.Discard)
	calls := seen["/api/v2/library/deletion-observed"]
	want := stable.ItemID + "|" + libraryDeletionCheck(stable.Path, "target", stable.ItemID, strings.Repeat("d", 32)).BindingID
	if len(calls) != 1 || calls[0] != want {
		t.Fatal("only the stable deletion should ask for a sync", calls, want)
	}
}

// Deleting the entry file leaves a folder the planner cannot read as a skill.
// That must be held while the deletion is unproven, then restored by replacing
// the folder (which backs up whatever survived), never left blocked forever.
func TestDeletedEntryFileIsHeldThenRestored(t *testing.T) {
	old := libraryRestoreDebounce
	libraryRestoreDebounce = 40 * time.Millisecond
	defer func() { libraryRestoreDebounce = old }()
	home := t.TempDir()
	t.Setenv("HOME", home)
	directory := filepath.Join(home, ".agents", "skills")
	config := filepath.Join(home, "agent.json")
	files := []skillbundle.ContentFile{
		{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Restore fixture\n---\nInstructions.\n")},
		{Path: "reference.txt", Data: []byte("support file")},
	}
	manifest, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	revision := store.PrivateSkillRevision{ID: strings.Repeat("b", 32), BundleID: strings.Repeat("c", 32), ManifestDigest: manifest.Digest, Files: files}
	skill := store.SyncSkill{ID: "local/demo", ItemID: strings.Repeat("a", 32), Name: "demo", Slug: "demo", SourceType: "private", Source: "private:" + revision.BundleID + ":" + revision.ID + ":" + manifest.Digest}
	target := store.SyncTarget{Canonical: true, Target: store.Environment{ID: "target", Name: "Desktop", Host: "desktop", Agent: "agents"}, Mode: "additive", Revision: "revision", Desired: []store.SyncSkill{skill}}
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/library/bases":
			var base store.LibraryBase
			_ = json.NewDecoder(r.Body).Decode(&base)
			_ = json.NewEncoder(w).Encode(base)
		case strings.HasPrefix(r.URL.Path, "/api/v2/skill-bundles/"):
			_ = json.NewEncoder(w).Encode(revision)
		case r.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]any{"canWrite": true, "targets": []store.SyncTarget{target}})
		default:
			_, _ = w.Write([]byte(`{"errors":[]}`))
		}
	}))
	defer controller.Close()
	hostname, _ := os.Hostname()
	c := agentConfig{Server: controller.URL, ID: strings.Repeat("d", 32), Token: "fixture", Secret: "fixture", AllowApply: true, Home: home, Hostname: hostname}
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	var plan SyncPlan
	opts := syncOptions{onInstalled: func(s store.SyncSkill, path string, files []skillbundle.ContentFile) error {
		return prepareLibraryInstall(context.Background(), config, c, s, path, files, "test", target.Target.ID)
	}, onInstallVerified: func(s store.SyncSkill, path string) error {
		return finishLibraryInstall(config, c, s, path, target.Target.ID)
	}, managedDeletions: func(target store.SyncTarget, path string) (map[string]bool, map[string]bool, error) {
		return planLibraryRestores(config, c, target, path)
	}, onPlan: func(p SyncPlan) { plan = p }}
	run := func(apply bool) {
		t.Helper()
		args := []string{"--server", c.Server, "--token", c.Token, "--env", "desktop", "--target", target.Target.ID}
		if apply {
			args = append(args, "--apply")
		}
		plan = SyncPlan{}
		if err := runSyncWithOptions(args, strings.NewReader("yes\n"), io.Discard, func([]string, string) error { t.Fatal("private install invoked npx"); return nil }, "test", opts); err != nil {
			t.Fatal(err)
		}
	}
	run(true)
	if err := os.Remove(filepath.Join(directory, "demo", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	run(false)
	if len(plan.Actions) != 0 || len(plan.Problems) != 0 {
		t.Fatal("an unproven deletion of the entry file was planned or reported as a problem", plan.Actions, plan.Problems)
	}
	time.Sleep(80 * time.Millisecond)
	run(false)
	if len(plan.Actions) != 1 || !plan.Actions[0].Add || !plan.Actions[0].Replace {
		t.Fatal("a stable deletion of the entry file was not planned as a replace", plan.Actions, plan.Problems)
	}
	run(true)
	got, err := os.ReadFile(filepath.Join(directory, "demo", "SKILL.md"))
	if err != nil || string(got) != string(files[0].Data) {
		t.Fatal("the entry file was not restored from canonical bytes", string(got), err)
	}
	state, _ := loadLibraryState(config, c)
	if len(state.Restores) != 1 || len(state.Missing) != 0 {
		t.Fatal("restore receipt", state.Missing, state.Restores)
	}
}
