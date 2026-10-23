package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestoreLargePayloadsAndFailedVerificationRollback(t *testing.T) {
	for _, format := range []string{"skill", "sqlite"} {
		t.Run(format, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "payload")
			original := []byte("original")
			os.WriteFile(dest, original, 0600)
			data := bytes.Repeat([]byte("x"), (1<<20)+1)
			plan := RestorePlan{SnapshotID: "review", Fingerprint: "review", Actions: []RestoreAction{{Destination: dest, State: "conflict", Format: format}}, files: map[string]restoreFile{dest: {format: format, data: data}}}
			if _, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), Conflict: "replace"}); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(dest)
			if !bytes.Equal(got, data) {
				t.Fatal("large payload not restored")
			}
			// Deliberately exceed the format's limit to exercise rollback after a write.
			limit := maxSkillBytes
			if format == "sqlite" {
				limit = maxMemoryBytes
			}
			plan.files[dest] = restoreFile{format: format, data: bytes.Repeat([]byte("y"), limit+1)}
			_, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), Conflict: "replace"})
			if err == nil || !strings.Contains(err.Error(), "file_too_large") {
				t.Fatalf("expected verification failure: %v", err)
			}
			got, _ = os.ReadFile(dest)
			if !bytes.Equal(got, data) {
				t.Fatal("current failed replacement was not rolled back")
			}
		})
	}
}

func TestMemoryRestoreUsesEffectiveRootAndRechecksSidecars(t *testing.T) {
	e := fixture(t)
	custom := filepath.Join(e.home, "custom-codex")
	e.getenv = func(k string) string {
		if k == "CODEX_HOME" {
			return custom
		}
		return ""
	}
	report, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := SetupSnapshot{Memories: []SetupMemory{{Name: "memories_1.sqlite", AgentID: "codex", Scope: "global", Data: []byte("db")}}}
	plan := BuildRestorePlan(snapshot, testManifest("review"), report)
	dest := filepath.Join(custom, "memories_1.sqlite")
	findRestoreAction(t, plan, dest)
	_, err = ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), beforeWrite: func(path string) error {
		mkdir(t, filepath.Dir(path))
		return os.WriteFile(path+"-wal", []byte("wal"), 0600)
	}})
	if err == nil || !strings.Contains(err.Error(), "sidecars_present") {
		t.Fatalf("sidecar race accepted: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("database written despite sidecar")
	}
	result := VerifySetupSnapshot(snapshot, testManifest("review"), report)
	if result.Summary.Blocked != 1 || result.Components[0].Destination != dest {
		t.Fatalf("wrong verification: %+v", result)
	}
}

func TestMemoryVerificationContinuesAfterBlockedDatabase(t *testing.T) {
	e := fixture(t)
	root := filepath.Join(e.home, ".codex")
	mkdir(t, root)
	os.WriteFile(filepath.Join(root, "memories_1.sqlite-wal"), []byte("wal"), 0600)
	snapshot := SetupSnapshot{Memories: []SetupMemory{{Name: "memories_1.sqlite", AgentID: "codex"}, {Name: "memories_2.sqlite", AgentID: "codex"}}}
	result := SetupVerifyResult{}
	verifySetupMemories(snapshot, Report{Home: e.home}, &result)
	if len(result.Components) != 2 || result.Summary.Blocked != 1 || result.Summary.Missing != 1 {
		t.Fatalf("incomplete verification: %+v", result)
	}
}

func TestSkillVerificationReportsOneBlockedResult(t *testing.T) {
	e, _ := skillFixture(t)
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRestorePlan(planForSelection(t, selection, report), RestoreApplyOptions{JournalDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(destination.home, ".claude", "skills", "alpha", "SKILL.md")
	os.Remove(path)
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	result := VerifySetupSnapshot(BuildSetupSnapshot(selection), testManifest("review"), report)
	if result.Summary.Blocked != 1 || result.Summary.Verified != 0 || len(result.Components) != 1 {
		t.Fatalf("contradictory verification: %+v", result)
	}
}

func TestVerificationFailureIsNotSuccess(t *testing.T) {
	for _, summary := range []SetupVerifySummary{{Missing: 1}, {Changed: 1}, {Blocked: 1}} {
		if setupVerificationError(SetupVerifyResult{Summary: summary}) == nil {
			t.Fatal("incomplete verification reported success")
		}
	}
	if err := setupVerificationError(SetupVerifyResult{Summary: SetupVerifySummary{Verified: 1, Unsupported: 1}}); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreWizardReviewsPathsAndChoicesBeforeApplying(t *testing.T) {
	for _, tc := range []struct {
		name, input   string
		apply, verify bool
	}{
		{"cancel after review", "replace\nn\n", false, false},
		{"replace and verify", "replace\ny\n", true, true},
		{"skip reports incomplete verification", "skip\ny\n", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolatedCLI(t)
			selection, _, home := applyFixture(t)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			dest := filepath.Join(home, ".claude.json")
			writeConfig(t, dest, `{"model":"old"}`)
			dir := t.TempDir()
			snapshot, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: testSetupPassphrase})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := SaveSetupProfile("work", testSetupPassphrase, snapshot.ID, dir, time.Now()); err != nil {
				t.Fatal(err)
			}
			pass := writePassphrase(t, t.TempDir(), testSetupPassphrase)
			input := filepath.Join(t.TempDir(), "answers")
			os.WriteFile(input, []byte(tc.input), 0600)
			in, err := os.Open(input)
			if err != nil {
				t.Fatal(err)
			}
			defer in.Close()
			previous := os.Stdin
			os.Stdin = in
			defer func() { os.Stdin = previous }()
			var out bytes.Buffer
			err = runRestoreWizard(&out, dir, "", "", restoreWizardOptions{PasswordFile: pass})
			if tc.name == "skip reports incomplete verification" {
				if err == nil || !strings.Contains(err.Error(), "verification incomplete") {
					t.Fatalf("skip reported success: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			text := out.String()
			pathAt := strings.Index(text, dest)
			confirmAt := strings.Index(text, "Apply the reviewed plan")
			if pathAt < 0 || confirmAt < pathAt {
				t.Fatalf("confirmation preceded destination review: %s", text)
			}
			if !strings.Contains(text, "skip, replace or cancel") {
				t.Fatal("per-path conflict choice missing")
			}
			data, _ := os.ReadFile(dest)
			if tc.apply == bytes.Contains(data, []byte(`"old"`)) {
				t.Fatalf("unexpected destination after %s", tc.name)
			}
			if strings.Contains(text, "File verification passed") != tc.verify {
				t.Fatalf("incorrect success wording: %s", text)
			}
		})
	}
}
