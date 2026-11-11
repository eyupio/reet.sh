package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

type recoveryEnv struct {
	home, config, root, path, backup, digest string
	c                                        agentConfig
	target                                   store.SyncTarget
}

// recoveryFixture lays out a managed copy and the journal a removal leaves
// behind. crash decides how far the interrupted operation got.
func recoveryFixture(t *testing.T, crash string) recoveryEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".claude", "skills")
	path := filepath.Join(root, "demo")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: demo\ndescription: Recovery fixture\n---\nInstructions.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	digest, err := privateSkillDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	backupRoot := filepath.Join(home, ".claude", ".reet-sync-backups", "removed-1")
	backup := filepath.Join(backupRoot, "demo")
	if err := os.MkdirAll(backupRoot, 0700); err != nil {
		t.Fatal(err)
	}
	switch crash {
	case "before_move": // journal written, rename never happened
	case "after_move": // renamed, final record never written
		if err := os.Rename(path, backup); err != nil {
			t.Fatal(err)
		}
	case "both": // ambiguous: a copy in both places
		if err := os.MkdirAll(backup, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(backup, "SKILL.md"), []byte("---\nname: demo\ndescription: Recovery fixture\n---\nInstructions.\n"), 0644); err != nil {
			t.Fatal(err)
		}
	case "neither":
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	case "backup_changed":
		if err := os.Rename(path, backup); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(backup, "SKILL.md"), []byte("changed after the move"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	hostname, _ := os.Hostname()
	c := agentConfig{Server: "http://127.0.0.1:1", ID: strings.Repeat("b", 32), Hostname: hostname, Home: home, Token: "fixture", Secret: "fixture", AllowApply: true}
	config := filepath.Join(home, "agent.json")
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	binding := libraryBinding{Path: path, Paths: []string{"SKILL.md"}, ItemID: strings.Repeat("c", 32), Revision: strings.Repeat("d", 32), Digest: digest, Deployment: &libraryDeployment{TargetID: "target", Revision: strings.Repeat("d", 32), Digest: digest, Verified: true, Removal: &libraryRemoval{Generation: strings.Repeat("a", 64), Backup: backup, Phase: "prepared", Digest: digest}}}
	state := libraryLocalState{Server: c.Server, Enrollment: c.ID, ManagedRemovals: managedRemovalVersion, Bindings: []libraryBinding{binding}}
	if err := writePrivate(config+".library", state); err != nil {
		t.Fatal(err)
	}
	return recoveryEnv{home, config, root, path, backup, digest, c, store.SyncTarget{Canonical: true, Target: store.Environment{ID: "target", Host: "desktop", Agent: "claude"}}}
}

func TestInterruptedManagedRemovalRecovery(t *testing.T) {
	cases := []struct {
		crash       string
		wantProblem bool
		wantPhase   string // "" means the journal is cleared
		wantVerify  bool   // ownership still proven
		wantLocal   bool
		wantBackup  bool
	}{
		{"before_move", false, "", true, true, false},
		{"after_move", false, "removed", false, false, true},
		{"both", true, "prepared", true, true, true},
		{"neither", true, "prepared", true, false, false},
		{"backup_changed", true, "prepared", true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.crash, func(t *testing.T) {
			e := recoveryFixture(t, tc.crash)
			// Recovery needs no controller: the controller address above is unreachable.
			actions, problems, err := planLibraryRemovals(context.Background(), e.config, e.c, e.target, e.root, "test")
			if err != nil {
				t.Fatal(err)
			}
			if len(actions) != 0 {
				t.Fatal("recovery planned a new removal", actions)
			}
			// A cleared journal still leaves the ordinary "intent unverifiable"
			// retention here (no controller); only recovery problems are counted.
			interrupted := false
			for _, message := range problems {
				interrupted = interrupted || strings.Contains(message, "interrupted removal")
			}
			if interrupted != tc.wantProblem {
				t.Fatalf("problems=%v want recovery problem=%v", problems, tc.wantProblem)
			}
			state, err := loadLibraryState(e.config, e.c)
			if err != nil {
				t.Fatal(err)
			}
			deployment := state.Bindings[0].Deployment
			phase := ""
			if deployment.Removal != nil {
				phase = deployment.Removal.Phase
			}
			if phase != tc.wantPhase || deployment.Verified != tc.wantVerify {
				t.Fatalf("journal phase=%q verified=%v: %+v", phase, deployment.Verified, deployment)
			}
			if _, err := os.Lstat(e.path); (err == nil) != tc.wantLocal {
				t.Fatalf("local copy present=%v want=%v", err == nil, tc.wantLocal)
			}
			if _, err := os.Lstat(e.backup); (err == nil) != tc.wantBackup {
				t.Fatalf("backup present=%v want=%v", err == nil, tc.wantBackup)
			}
			// Running again must change nothing further.
			before, _ := os.ReadFile(e.config + ".library")
			if _, _, err := planLibraryRemovals(context.Background(), e.config, e.c, e.target, e.root, "test"); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(e.config + ".library")
			if string(before) != string(after) {
				t.Fatal("recovery is not idempotent")
			}
		})
	}
}

func TestInterruptedRemovalRecoveryRunsEvenWithoutNegotiatedCapability(t *testing.T) {
	e := recoveryFixture(t, "after_move")
	state, _ := loadLibraryState(e.config, e.c)
	state.ManagedRemovals = 0 // controller downgraded after the crash
	if err := writePrivate(e.config+".library", state); err != nil {
		t.Fatal(err)
	}
	if _, _, err := planLibraryRemovals(context.Background(), e.config, e.c, e.target, e.root, "test"); err != nil {
		t.Fatal(err)
	}
	state, _ = loadLibraryState(e.config, e.c)
	if state.Bindings[0].Deployment.Removal.Phase != "removed" || state.Bindings[0].Deployment.Verified {
		t.Fatal("a downgraded controller left the interrupted removal unresolved", state.Bindings[0].Deployment)
	}
}
