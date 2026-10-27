package api

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// worktreeOperation says which request a refusal answers, so git's failure
// and an unexpected reason are worded for what was asked.
type worktreeOperation int

const (
	worktreeCreating worktreeOperation = iota
	worktreeChecking
	worktreeRemoving
)

const outsideHomeSentence = "The folder ~/.reet/worktrees on that machine is a link, belongs to another user or can be changed by others. Fix it, then try again."

// worktreeRefusals are the plain sentences for every reason the machine can
// give. Codes equal the reasons, except exists, which is worktree_exists.
var worktreeRefusals = map[terminalproto.WorktreeReason]struct{ code, sentence string }{
	terminalproto.WorktreeNotRepoRoot:         {"not_repo_root", "That folder isn't the top of a git repository."},
	terminalproto.WorktreeRepoBusy:            {"repo_busy", "That repository is in the middle of a merge, rebase or similar. Finish or abort it first."},
	terminalproto.WorktreeExists:              {"worktree_exists", "A worktree or branch with this run's name is already on that machine, so nothing was started."},
	terminalproto.WorktreeOutsideHome:         {"outside_home", outsideHomeSentence},
	terminalproto.WorktreeNotRegistered:       {"not_registered", "Git doesn't list that folder as this run's worktree, so reet won't touch it."},
	terminalproto.WorktreeHasChanges:          {"has_changes", "This worktree has changes you haven't saved. Commit, stash or discard them in the worktree, then check again."},
	terminalproto.WorktreeUnreferencedCommits: {"unreferenced_commits", "Some commits in this worktree are not on any branch. Put them on a branch first, then check again."},
	terminalproto.WorktreeNestedRepository:    {"nested_repository", "This worktree contains another git repository. Move or remove it first."},
	terminalproto.WorktreeHiddenChanges:       {"hidden_changes", "Git is not tracking some edits in this worktree. Review them before removing."},
	terminalproto.WorktreeCheckIncomplete:     {"check_incomplete", "reet couldn't check all of this worktree. Remove it yourself."},
}

// worktreeReasons lists which reasons each request can be refused with; any
// other reason gets the request's general sentence.
var worktreeReasons = map[worktreeOperation][]terminalproto.WorktreeReason{
	worktreeCreating: {terminalproto.WorktreeNotRepoRoot, terminalproto.WorktreeRepoBusy, terminalproto.WorktreeExists, terminalproto.WorktreeGitFailed, terminalproto.WorktreeOutsideHome},
	worktreeChecking: {terminalproto.WorktreeGitFailed, terminalproto.WorktreeOutsideHome, terminalproto.WorktreeNotRegistered},
	worktreeRemoving: {terminalproto.WorktreeGitFailed, terminalproto.WorktreeOutsideHome, terminalproto.WorktreeNotRegistered, terminalproto.WorktreeHasChanges,
		terminalproto.WorktreeUnreferencedCommits, terminalproto.WorktreeNestedRepository, terminalproto.WorktreeHiddenChanges, terminalproto.WorktreeCheckIncomplete},
}

// writeWorktreeRefusal answers a worktree_refused with 422, a stable code, a
// plain sentence and, for git_failed, git's first line as detail.
func writeWorktreeRefusal(w http.ResponseWriter, err error, op worktreeOperation) {
	var refusal *terminalbroker.WorktreeError
	reason := terminalproto.WorktreeReason("")
	detail := ""
	if errors.As(err, &refusal) {
		reason, detail = refusal.Reason, refusal.Detail
	}
	known := false
	for _, r := range worktreeReasons[op] {
		known = known || r == reason
	}
	code, sentence := "worktree_refused", ""
	switch {
	case !known:
		detail = ""
		sentence = map[worktreeOperation]string{
			worktreeCreating: "That machine refused to make a worktree for this request. Start it again to see why.",
			worktreeChecking: "That machine refused to check the worktree.",
			worktreeRemoving: "That machine refused to remove the worktree.",
		}[op]
	case reason == terminalproto.WorktreeGitFailed:
		code = "git_failed"
		sentence = map[worktreeOperation]string{
			worktreeCreating: "Git couldn't make a worktree there.",
			worktreeChecking: "Git couldn't check that worktree.",
			worktreeRemoving: "Git couldn't remove that worktree.",
		}[op]
	default:
		code, sentence = worktreeRefusals[reason].code, worktreeRefusals[reason].sentence
	}
	out := map[string]string{"error": sentence, "code": code, "message": sentence}
	if detail != "" {
		out["detail"] = detail
	}
	writeJSON(w, http.StatusUnprocessableEntity, out)
}

// worktreeError maps the broker's worktree outcomes; everything else keeps
// the existing terminal mapping.
func worktreeError(w http.ResponseWriter, err error, op worktreeOperation) {
	switch {
	case errors.Is(err, terminalbroker.ErrWorktreeUnsupported):
		writeErrCode(w, http.StatusConflict, "worktree_unsupported", "", "Update reet on that machine to start agents with worktrees.", "")
	case errors.Is(err, terminalbroker.ErrWorktreeChanged):
		writeErrCode(w, http.StatusConflict, "worktree_changed", "", "That worktree changed since you looked. Check again.", "")
	case errors.Is(err, terminalbroker.ErrWorktreeNotFound):
		writeErrCode(w, http.StatusNotFound, "not_found", "", "That worktree isn't on that machine. It may have been removed already.", "")
	case errors.Is(err, terminalbroker.ErrWorktreeRefused):
		writeWorktreeRefusal(w, err, op)
	case errors.Is(err, terminalbroker.ErrWorktreeMismatch):
		writeErrCode(w, http.StatusServiceUnavailable, "worktree_mismatch", "", "That machine's answer did not match the request. Check again.", "")
	case errors.Is(err, terminalbroker.ErrWorktreeBusy):
		writeErrCode(w, http.StatusConflict, "worktree_busy", "", "That machine is already working on a worktree. Try again in a minute.", "")
	case errors.Is(err, terminalbroker.ErrWorktreeUnavailable):
		writeErrCode(w, http.StatusServiceUnavailable, "worktree_unavailable", "", "That machine couldn't do that just now. Try again in a moment.", "")
	default:
		terminalError(w, err)
	}
}

// worktreeBranch is the branch the machine makes for a run: reet/<project>-<run>.
func worktreeBranch(project, run string) string { return "reet/" + project + "-" + run }

// worktreeTarget names one run's worktree on one machine. Repo is the
// repository folder's name on that machine; the machine slugs it exactly as
// it did when it made the worktree.
type worktreeTarget struct {
	AgentID string `json:"agentId"`
	Project string `json:"project"`
	Repo    string `json:"repo"`
	Run     string `json:"run"`
}

func worktreeStateJSON(state terminalproto.WorktreeState) map[string]any {
	paths := state.Paths
	if paths == nil {
		paths = []string{}
	}
	return map[string]any{
		"path": state.Path, "branch": state.Branch, "changed": state.Changed, "unpushed": state.Unpushed, "paths": paths,
		"unreferenced": state.Unreferenced, "hidden": state.Hidden, "nested": state.Nested, "incomplete": state.Incomplete,
	}
}

// terminalWorktreeStatus reports a run's worktree: counts, the first paths,
// and every condition that would block removing it. It only reads.
func (s *server) terminalWorktreeStatus(w http.ResponseWriter, r *http.Request) {
	var in worktreeTarget
	if !terminalJSON(w, r, &in) {
		return
	}
	state, err := s.terminals.WorktreeStatus(r.Context(), s.uid(r), in.AgentID, in.Project, in.Repo, in.Run)
	if err != nil {
		worktreeError(w, err, worktreeChecking)
		return
	}
	writeJSON(w, 200, worktreeStateJSON(state))
}

// terminalWorktreeRemove removes a run's worktree only when the machine's
// counts still equal the ones the person was shown. The branch is kept.
func (s *server) terminalWorktreeRemove(w http.ResponseWriter, r *http.Request) {
	var in struct {
		worktreeTarget
		// Both counts are required: a remove always states what was shown.
		Changed  *int `json:"changed"`
		Unpushed *int `json:"unpushed"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	if in.Changed == nil || in.Unpushed == nil {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	shown := terminalbroker.WorktreeCounts{Changed: *in.Changed, Unpushed: *in.Unpushed}
	if err := terminalbroker.CheckWorktreeRemove(in.Project, in.Repo, in.Run, shown); err != nil {
		terminalError(w, err)
		return
	}
	uid := s.uid(r)
	removed, err := s.terminals.WorktreeRemove(r.Context(), uid, in.AgentID, in.Project, in.Repo, in.Run, shown)
	if errors.Is(err, terminalbroker.ErrWorktreeReplyLost) {
		// The remove was sent but its answer never came. Ask for status once,
		// on a budget of its own: nothing there means the remove happened.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), terminalbroker.WorktreeCallTimeout)
		_, statusErr := s.terminals.WorktreeStatus(ctx, uid, in.AgentID, in.Project, in.Repo, in.Run)
		cancel()
		if errors.Is(statusErr, terminalbroker.ErrWorktreeNotFound) {
			branch := worktreeBranch(in.Project, in.Run)
			s.event(uid, "ui", "worktree-remove", "", map[string]string{"machine": in.AgentID, "host": s.machineHost(uid, in.AgentID), "branch": branch, "confirmedBy": "status"})
			s.recordRunWorktreeRemoved(uid, in.AgentID, in.Run)
			writeJSON(w, 200, map[string]any{"ok": true, "branch": branch, "confirmedBy": "status"})
			return
		}
	}
	if err != nil {
		worktreeError(w, err, worktreeRemoving)
		return
	}
	s.event(uid, "ui", "worktree-remove", "", map[string]string{"machine": in.AgentID, "host": s.machineHost(uid, in.AgentID), "path": removed.Path, "branch": removed.Branch})
	s.recordRunWorktreeRemoved(uid, in.AgentID, in.Run)
	writeJSON(w, 200, map[string]any{"ok": true, "path": removed.Path, "branch": removed.Branch})
}

// auditLateWorktreeRemoval records a removal whose ok answer reached the
// controller only after the request had given up. The broker has already
// checked that the folder and branch are the requested run's.
func (s *server) auditLateWorktreeRemoval(uid int64, target terminalbroker.WorktreeTarget, result terminalproto.WorktreeResult) {
	s.event(uid, "ui", "worktree-remove", "", map[string]string{"machine": target.Agent, "host": s.machineHost(uid, target.Agent), "path": result.Path, "branch": result.Branch, "late": "true"})
	s.recordRunWorktreeRemoved(uid, target.Agent, target.Run)
}

// recordRunWorktreeRemoved moves the account's run member that made this
// worktree to removed. A worktree that no run member made changes nothing.
func (s *server) recordRunWorktreeRemoved(uid int64, agent, run string) {
	if err := s.st.SetRunMemberRemovedFor(uid, agent, run); err != nil {
		log.Printf("projects: record removed worktree: %v", err)
	}
}
