package api

import (
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// endedWorktreeSession makes a finished session that made the test worktree.
func endedWorktreeSession(t *testing.T, f *launchAPI) string {
	t.Helper()
	rec, err := f.st.RemoteCapabilitiesFor(f.uid, f.agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	create := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Dir: "/home/tester/site", Worktree: worktreeTestRun, Project: worktreeTestProject}
	session, _, err := f.st.ReserveTerminalCreateFor(f.uid, f.agent.ID, terminalbroker.ID(), rec.Generation, create)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.RecordTerminalWorktreeFor(f.uid, f.agent.ID, session.ID, worktreeTestPath, "site"); err != nil {
		t.Fatal(err)
	}
	zero := 0
	if err := f.st.ObserveTerminalStateFor(f.uid, f.agent.ID, session.ID, rec.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.st.ObserveTerminalStateFor(f.uid, f.agent.ID, session.ID, rec.Generation, terminalproto.Exited, &zero); err != nil {
		t.Fatal(err)
	}
	return session.ID
}

func TestMaintenanceCleansOnlyWhatIsSaved(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	endedWorktreeSession(t, f)
	state := terminalproto.WorktreeState{Path: worktreeTestPath, Branch: worktreeTestBranch, Unpushed: 1}
	f.answer(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.WorktreeRemove); ok {
			return terminalproto.WorktreeResult{Code: terminalproto.OK, Path: worktreeTestPath, Branch: worktreeTestBranch}
		}
		return state
	})
	scan := func() map[string]any {
		code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/maintenance/worktrees/scan", map[string]any{"minAgeDays": 0})
		if code != 200 {
			t.Fatal(code, raw)
		}
		return out
	}
	statusOf := func(out map[string]any) string {
		items, _ := out["items"].([]any)
		if len(items) != 1 {
			t.Fatal("items", out)
		}
		return items[0].(map[string]any)["status"].(string)
	}
	if got := statusOf(scan()); got != sweepSafe {
		t.Fatal("clean worktree is", got)
	}
	// Unsaved changes are shown for review and never removed by a clean.
	state.Changed = 2
	if got := statusOf(scan()); got != sweepReview {
		t.Fatal("changed worktree is", got)
	}
	req := map[string]any{"items": []map[string]any{{"agentId": f.agent.ID, "project": worktreeTestProject, "repo": "site", "run": worktreeTestRun}}}
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/maintenance/worktrees/clean", req)
	if code != 200 || statusOf(out) != sweepReview {
		t.Fatal(code, raw)
	}
	for _, b := range f.worktreeSent() {
		if _, ok := b.(terminalproto.WorktreeRemove); ok {
			t.Fatal("a worktree with unsaved changes was sent a remove")
		}
	}
	// Once saved, the clean removes it with the counts it just read, and it is not asked about again.
	state.Changed = 0
	code, out, raw = f.do(t, f.cookie, "POST", "/api/v2/maintenance/worktrees/clean", req)
	if code != 200 || statusOf(out) != sweepGone {
		t.Fatal(code, raw)
	}
	var removes []terminalproto.WorktreeRemove
	for _, b := range f.worktreeSent() {
		if r, ok := b.(terminalproto.WorktreeRemove); ok {
			removes = append(removes, r)
		}
	}
	if len(removes) != 1 || removes[0].Discard || removes[0].Unpushed != 1 {
		t.Fatal("remove", removes)
	}
	if items, _ := scan()["items"].([]any); len(items) != 0 {
		t.Fatal("a removed worktree is still listed", items)
	}
}

func TestMaintenanceScheduleRemovesOnlySafeWorktreesAndOnlyWhenOn(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	endedWorktreeSession(t, f)
	f.answer(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.WorktreeRemove); ok {
			return terminalproto.WorktreeResult{Code: terminalproto.OK, Path: worktreeTestPath, Branch: worktreeTestBranch}
		}
		return terminalproto.WorktreeState{Path: worktreeTestPath, Branch: worktreeTestBranch, Changed: 1}
	})
	removed := func() int {
		n := 0
		for _, b := range f.worktreeSent() {
			if _, ok := b.(terminalproto.WorktreeRemove); ok {
				n++
			}
		}
		return n
	}
	f.srv.s.runScheduledMaintenance(time.Now())
	if removed() != 0 || len(f.worktreeSent()) != 0 {
		t.Fatal("a sweep ran while switched off")
	}
	code, _, raw := f.do(t, f.cookie, "PUT", "/api/v2/maintenance/worktrees", map[string]any{"enabled": true, "minAgeDays": 0})
	if code != 200 {
		t.Fatal(code, raw)
	}
	f.srv.s.runScheduledMaintenance(time.Now())
	if removed() != 0 {
		t.Fatal("the schedule removed a worktree with unsaved changes")
	}
	last := f.srv.s.worktreeSweepLast()
	if last == nil || last.Review != 1 || last.Removed != 0 {
		t.Fatal("summary", last)
	}
}

// A fenced controller runs no scheduled sweep on any machine, and sweeps again
// once the fence is lifted.
func TestAFencedControllerRunsNoScheduledSweepUntilTheLift(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	endedWorktreeSession(t, f)
	f.answer(func(body terminalproto.Body) terminalproto.Body {
		return terminalproto.WorktreeState{Path: worktreeTestPath, Branch: worktreeTestBranch, Changed: 1}
	})
	if code, _, raw := f.do(t, f.cookie, "PUT", "/api/v2/maintenance/worktrees", map[string]any{"enabled": true, "minAgeDays": 0}); code != 200 {
		t.Fatal(code, raw)
	}
	if _, err := f.srv.s.st.SetFence(""); err != nil {
		t.Fatal(err)
	}
	f.srv.s.runScheduledMaintenance(time.Now())
	if len(f.worktreeSent()) != 0 || f.srv.s.worktreeSweepLast() != nil {
		t.Fatal("a sweep ran while fenced")
	}
	if _, err := f.srv.s.st.LiftFence(); err != nil {
		t.Fatal(err)
	}
	f.srv.s.runScheduledMaintenance(time.Now())
	if f.srv.s.worktreeSweepLast() == nil {
		t.Fatal("no sweep ran after the lift")
	}
}
