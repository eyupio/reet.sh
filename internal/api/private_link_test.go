package api

import (
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Every answer and note a machine can send on the private connection must
// leave the connection up. The controller ends the link on a message it does
// not accept as a peer reply, so a kind missing from that list turns an
// ordinary answer into a dropped machine.
func TestPrivateControlKeepsTheLinkForEveryMachineAnswer(t *testing.T) {
	for name, body := range map[string]terminalproto.Body{
		"launch_progress":          terminalproto.LaunchProgress{Stage: terminalproto.StageWorktree},
		"base_check_result":        terminalproto.BaseCheckResult{Reason: terminalproto.BranchesGitFailed},
		"branch_sync_result":       terminalproto.BranchSyncResult{Reason: terminalproto.BranchesGitFailed},
		"branch_prune_result":      terminalproto.BranchPruneResult{Reason: terminalproto.BranchesGitFailed},
		"branch_prune_plan_result": terminalproto.BranchPrunePlanResult{Reason: terminalproto.BranchesGitFailed},
		"native_view_result":       terminalproto.NativeViewResult{Reason: "not available"},
		"software_apply_result":    terminalproto.SoftwareApplyResult{Tool: "claude", Action: terminalproto.SoftwareActionRemove, State: terminalproto.SoftwareApplyRefused, Reason: terminalproto.SoftwareApplyBusy},
		"repo_clone_result":        terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneFailed},
		"repo_clone_state":         terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneFailed},
		"repo_clone_removed":       terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneChanged},
		"accounts_result":          terminalproto.AccountsResult{Reason: terminalproto.AccountsTimedOut},
	} {
		t.Run(name, func(t *testing.T) {
			h := newGM6Harness(t)
			caps, err := h.st.RemoteCapabilitiesFor(h.uid, h.agent.ID)
			if err != nil {
				t.Fatal(err)
			}
			h.linkMu.Lock()
			c, done := h.conn, h.workerDone
			h.linkMu.Unlock()
			msg := terminalproto.Message{Version: terminalproto.Version, RequestID: terminalbroker.ID(), Generation: caps.Generation, Body: body}
			if name == "native_view_result" {
				// A view answer belongs to a session, so it must name one.
				msg.SessionID = terminalbroker.ID()
			}
			if err := h.send(c, msg); err != nil {
				t.Fatal("the note could not be sent:", err)
			}
			select {
			case <-done:
				h.linkMu.Lock()
				why := h.linkErr
				h.linkMu.Unlock()
				t.Fatalf("the controller ended the machine's connection on a %s: %v", name, why)
			case <-time.After(700 * time.Millisecond):
			}
			if !h.current.Load().s.terminals.Ready(h.uid, h.agent.ID) {
				t.Fatalf("the machine is no longer ready after a %s", name)
			}
		})
	}
}
