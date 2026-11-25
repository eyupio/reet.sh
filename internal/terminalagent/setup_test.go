//go:build linux || darwin

package terminalagent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const allSetupFeatures = terminalproto.SetupReadFeature + "," + terminalproto.SetupApplyFeature + "," + terminalproto.SetupRollbackFeature

// setupConnector is a connector with a home holding one Claude instruction
// file, the journals in a temporary folder and allow-apply as given.
func setupConnector(t *testing.T, features string, allowApply bool) (func(m terminalproto.Message) terminalproto.SetupFleetResult, func(m terminalproto.Message) error, string) {
	t.Helper()
	for _, name := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "XDG_CONFIG_HOME", "OPENCODE_CONFIG_DIR", "OPENCODE_CONFIG", "PI_CODING_AGENT_DIR"} {
		t.Setenv(name, "")
	}
	journals := t.TempDir()
	original := setupJournalDir
	setupJournalDir = func() (string, error) { return journals, nil }
	t.Cleanup(func() { setupJournalDir = original })
	var home string
	handle, got := modelsConnector(t, features, func(o *terminalhost.ManagerOptions) {
		home = o.Home
		o.ApplyConsent = func() error {
			if !allowApply {
				return errors.New("off")
			}
			return nil
		}
	})
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte("Be brief.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	call := func(m terminalproto.Message) terminalproto.SetupFleetResult {
		t.Helper()
		if err := handle(t.Context(), m); err != nil {
			t.Fatal(err)
		}
		select {
		case reply := <-got:
			result, ok := reply.Body.(terminalproto.SetupFleetResult)
			if !ok {
				t.Fatalf("reply %T", reply.Body)
			}
			return result
		case <-time.After(5 * time.Second):
			t.Fatal("no reply")
		}
		return terminalproto.SetupFleetResult{}
	}
	return call, func(m terminalproto.Message) error {
		err := handle(t.Context(), m)
		nothingSent(t, got)
		return err
	}, home
}

// Each operation is served only on a generation that negotiated its own
// feature; an older controller never sends one, and a stray one is a
// protocol error with no answer.
func TestSetupFleetNeedsEachOperationsFeature(t *testing.T) {
	_, refused, _ := setupConnector(t, terminalproto.SetupReadFeature, true)
	item := terminalproto.SetupItem{Agent: "claude", Scope: "global", Kind: "instructions", Name: "CLAUDE.md", Format: "text", Content: "x"}
	item.SHA256 = terminalproto.SetupItemSHA256(item.Content)
	plan := &terminalproto.SetupFleet{Op: terminalproto.SetupOpPlan, Items: []terminalproto.SetupItem{item}}
	if err := refused(message("setup-fleet-000001", plan)); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatalf("plan without setup-apply-v1: %v", err)
	}
	rollback := &terminalproto.SetupFleet{Op: terminalproto.SetupOpRollback, Journal: "restore-fleet-x"}
	if err := refused(message("setup-fleet-000002", rollback)); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatalf("rollback without setup-rollback-v1: %v", err)
	}
	_, refused, _ = setupConnector(t, "", true)
	if err := refused(message("setup-fleet-000003", &terminalproto.SetupFleet{Op: terminalproto.SetupOpRead})); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatalf("read without setup-read-v1: %v", err)
	}
}

func TestSetupFleetReadsPlansAppliesAndUndoesWithAllowApply(t *testing.T) {
	call, _, home := setupConnector(t, allSetupFeatures, true)
	read := call(message("setup-fleet-000010", &terminalproto.SetupFleet{Op: terminalproto.SetupOpRead}))
	if read.State != terminalproto.SetupOK || len(read.Items) != 1 || read.Items[0].Content != "Be brief.\n" {
		t.Fatalf("read: %+v", read)
	}
	items := read.Items
	items[0].Content = "Be brief and plain.\n"
	items[0].SHA256 = terminalproto.SetupItemSHA256(items[0].Content)
	plan := call(message("setup-fleet-000011", &terminalproto.SetupFleet{Op: terminalproto.SetupOpPlan, Items: items}))
	if plan.State != terminalproto.SetupOK || len(plan.Files) != 1 || plan.Files[0].State != "merge" || plan.Fingerprint == "" {
		t.Fatalf("plan: %+v", plan)
	}
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	if data, _ := os.ReadFile(path); string(data) != "Be brief.\n" {
		t.Fatal("a plan wrote the file")
	}
	stale := call(message("setup-fleet-000012", &terminalproto.SetupFleet{Op: terminalproto.SetupOpApply, Items: items, Fingerprint: "0000000000000000000000000000000000000000000000000000000000000000", Label: "fleet-b1"}))
	if stale.State != terminalproto.SetupRefused || stale.Reason != terminalproto.SetupReasonChanged {
		t.Fatalf("stale fingerprint: %+v", stale)
	}
	applied := call(message("setup-fleet-000013", &terminalproto.SetupFleet{Op: terminalproto.SetupOpApply, Items: items, Fingerprint: plan.Fingerprint, Label: "fleet-b1"}))
	if applied.State != terminalproto.SetupOK || !terminalproto.ValidSetupJournal(applied.Journal) {
		t.Fatalf("apply: %+v", applied)
	}
	if data, _ := os.ReadFile(path); string(data) != "Be brief and plain.\n" {
		t.Fatalf("applied: %q", data)
	}
	undo := call(message("setup-fleet-000014", &terminalproto.SetupFleet{Op: terminalproto.SetupOpRollback, Journal: applied.Journal}))
	if undo.State != terminalproto.SetupOK {
		t.Fatalf("undo: %+v", undo)
	}
	if data, _ := os.ReadFile(path); string(data) != "Be brief.\n" {
		t.Fatalf("undone: %q", data)
	}
}

// Writing needs the machine's allow-apply opt-in as well as terminal access;
// reading needs terminal access only.
func TestSetupFleetRefusesWritesWithoutAllowApply(t *testing.T) {
	call, _, _ := setupConnector(t, allSetupFeatures, false)
	read := call(message("setup-fleet-000020", &terminalproto.SetupFleet{Op: terminalproto.SetupOpRead}))
	if read.State != terminalproto.SetupOK {
		t.Fatalf("read: %+v", read)
	}
	plan := call(message("setup-fleet-000021", &terminalproto.SetupFleet{Op: terminalproto.SetupOpPlan, Items: read.Items}))
	if plan.State != terminalproto.SetupRefused || plan.Reason != terminalproto.SetupReasonNotAllowed {
		t.Fatalf("plan: %+v", plan)
	}
	undo := call(message("setup-fleet-000022", &terminalproto.SetupFleet{Op: terminalproto.SetupOpRollback, Journal: "restore-fleet-x"}))
	if undo.Reason != terminalproto.SetupReasonNotAllowed {
		t.Fatalf("undo: %+v", undo)
	}
}
