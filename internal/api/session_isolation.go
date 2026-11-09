package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// isolateWorktree makes a launch in a saved git repository run in a worktree
// of its own, whatever the client asked for. Two agents can then never share
// one checkout of a repository reet knows about.
//
// The run token comes from the request id, never from a clock or a random
// source: a retried create must carry the same token so the machine adopts
// the worktree it already made. A launch that is not an agent start, that
// already names a worktree, that reopens an earlier conversation, or that is
// already inside a reet worktree is left alone. It reports whether it added a
// worktree.
func (s *server) isolateWorktree(uid int64, agent, requestID string, c *terminalproto.Create) bool {
	if c.Tool == "" || c.Dir == "" || c.Worktree != "" || c.Resume || c.ResumeACP != "" {
		return false
	}
	if strings.Contains(path.Clean(c.Dir), "/.reet/worktrees/") {
		return false
	}
	// Reuse the controller-derived identity from the original receipt. A
	// project renamed/removed while a reply was uncertain cannot change a retry.
	if operation, err := s.st.TerminalOperationFor(uid, agent, "create", requestID); err == nil && operation.Create != nil {
		c.Worktree, c.Project = operation.Create.Worktree, operation.Create.Project
		if c.Worktree != "" && c.ProjectID == "" {
			if session, err := s.st.TerminalSessionFor(uid, operation.SessionID); err == nil && session.ProjectID != nil {
				c.ProjectID = *session.ProjectID
			}
		}
		return c.Worktree != ""
	}
	p, ok, err := s.st.RepoProjectForDir(uid, agent, c.Dir)
	if err != nil || !ok {
		return false
	}
	c.Worktree = worktreeRunToken(requestID)
	c.Project = terminalproto.Slug(p.Name, "project")
	if c.ProjectID == "" {
		c.ProjectID = p.ID
	}
	return true
}

// worktreeRunToken derives a valid run token (32 lower-case hex digits) from a
// create's request id.
func worktreeRunToken(requestID string) string {
	sum := sha256.Sum256([]byte("reet-session-worktree|" + requestID))
	return hex.EncodeToString(sum[:16])
}

// worktreeHold says why removing a session was refused: its worktree is still
// there. Code is the stable error code; State is set when a person can still
// confirm the removal (uncommitted changes), so the reply can show what would
// be lost and carry the fingerprint the confirmation must repeat.
type worktreeHold struct {
	Status int
	Code   string
	Msg    string
	State  *terminalproto.WorktreeState
}

// worktreeConfirm is what a person confirmed when removing a session whose
// worktree has uncommitted changes: the counts and fingerprint they were shown.
type worktreeConfirm struct {
	Discard     bool   `json:"discard"`
	Changed     *int   `json:"changed"`
	Unpushed    *int   `json:"unpushed"`
	Fingerprint string `json:"fingerprint"`
}

// releaseSessionWorktree makes sure a session being removed from history
// leaves no worktree behind. It returns nil only when the worktree is gone (or
// never existed). Clean worktrees are removed at once. One with uncommitted
// changes is removed only when the person confirmed deleting exactly those
// changes. Anything else (the session still running, the machine offline or
// failing, a check the machine refuses) holds the removal: the session stays,
// and nothing is lost. The branch is always kept.
func (s *server) releaseSessionWorktree(ctx context.Context, uid int64, sess store.TerminalSession, c worktreeConfirm) *worktreeHold {
	if sess.WorktreePath == "" || sess.WorktreeRepo == "" {
		return nil
	}
	project, run, ok := worktreeParts(sess.WorktreePath)
	if !ok {
		return nil
	}
	switch sess.State {
	case terminalproto.Ended, terminalproto.Exited, terminalproto.Lost:
	default:
		return &worktreeHold{Status: http.StatusConflict, Code: "session_running", Msg: "End the session and wait for it to stop before removing it. Its worktree is still in use."}
	}
	state, err := s.terminals.WorktreeStatus(ctx, uid, sess.AgentID, project, sess.WorktreeRepo, run)
	switch {
	case errors.Is(err, terminalbroker.ErrWorktreeNotFound):
		_ = s.st.MarkWorktreeGone(uid, sess.AgentID, sess.WorktreePath)
		return nil
	case err != nil:
		return &worktreeHold{Status: http.StatusServiceUnavailable, Code: "worktree_unreachable", Msg: "Couldn't reach " + s.machineHost(uid, sess.AgentID) + " to remove this session's worktree. The session is kept; try again when the machine is online."}
	}
	status, reason := classifyWorktree(state)
	shown := terminalbroker.WorktreeCounts{Changed: state.Changed, Unpushed: state.Unpushed}
	switch status {
	case sweepSafe:
	case sweepReview:
		confirmed := c.Discard && c.Changed != nil && c.Unpushed != nil && *c.Changed == state.Changed && *c.Unpushed == state.Unpushed &&
			state.Fingerprint != "" && c.Fingerprint == state.Fingerprint
		if !confirmed {
			return &worktreeHold{Status: http.StatusConflict, Code: "worktree_unsaved", State: &state,
				Msg: "This session's worktree has uncommitted changes. Removing the session deletes them. Confirm to delete them, or commit them first."}
		}
		shown.Discard, shown.Fingerprint = true, state.Fingerprint
	default:
		return &worktreeHold{Status: http.StatusConflict, Code: "worktree_blocked", Msg: "This session's worktree can't be removed safely (" + reason + "). The session is kept; sort the worktree out first."}
	}
	removed, err := s.terminals.WorktreeRemove(ctx, uid, sess.AgentID, project, sess.WorktreeRepo, run, shown)
	switch {
	case errors.Is(err, terminalbroker.ErrWorktreeNotFound):
	case err != nil:
		return &worktreeHold{Status: http.StatusConflict, Code: "worktree_not_removed", Msg: "The worktree couldn't be removed (it may have changed just now). The session is kept; check again."}
	}
	detail := map[string]string{"machine": sess.AgentID, "host": s.machineHost(uid, sess.AgentID), "path": removed.Path, "branch": removed.Branch, "session": sess.ID}
	if shown.Discard {
		detail["discardedChanges"] = strconv.Itoa(state.Changed)
	}
	s.event(uid, "ui", "worktree-remove", "", detail)
	_ = s.st.MarkWorktreeGone(uid, sess.AgentID, sess.WorktreePath)
	s.recordRunWorktreeRemoved(uid, sess.AgentID, run)
	s.autoPruneAfterWorktree(uid, sess.AgentID, run)
	return nil
}

// onSessionClosed runs after a session row has been observed closed. When the
// person ended it with remove from history and it worked in a worktree, the
// worktree is checked and removed now that the machine is no longer using it,
// and only then is the session hidden from history. Uncommitted changes are
// never discarded automatically: a worktree that needs a person's decision
// keeps the session in history, where the existing removal flow asks.
func (s *server) onSessionClosed(uid int64, agentID, sessionID string) {
	sess, err := s.st.TerminalSessionFor(uid, sessionID)
	if err != nil || sess.WorktreePath == "" || !sess.RemoveHistory {
		return
	}
	if archived, err := s.st.TerminalArchivedFor(uid, sessionID); err != nil || archived {
		return
	}
	if !s.claimSessionCleanup(sessionID) {
		return
	}
	defer s.releaseSessionCleanup(sessionID)
	ctx, cancel := context.WithTimeout(s.baseContext(), 3*time.Minute)
	defer cancel()
	hold := s.releaseSessionWorktree(ctx, uid, sess, worktreeConfirm{})
	if hold != nil {
		// The worktree could not go safely, so the session stays visible with
		// the reason; the person can confirm unsaved work from its menu.
		_ = s.st.AddEventOnceFor(uid, "system", "worktree-kept", "", map[string]string{"machine": sess.AgentID, "host": s.machineHost(uid, sess.AgentID), "path": sess.WorktreePath, "code": hold.Code})
		return
	}
	if err := s.st.ArchiveTerminalFor(uid, sessionID); err != nil {
		return
	}
	s.terminals.CloseChat(uid, sessionID)
	s.event(uid, "system", "session-remove", "", map[string]string{"machine": sess.AgentID, "host": s.machineHost(uid, sess.AgentID), "session": sessionID})
}

// onPeerReady resumes cleanups that could not reach the machine before, such
// as one observed while the connection was still synchronizing, or one
// interrupted by a controller restart. It only looks at this machine's closed
// remove-history sessions that still hold a worktree.
func (s *server) onPeerReady(uid int64, agentID string) {
	const maxCleanups = 50
	sessions, err := s.st.TerminalCleanupCandidatesFor(uid, agentID, maxCleanups)
	if err != nil {
		return
	}
	for _, sess := range sessions {
		s.onSessionClosed(uid, agentID, sess.ID)
	}
}

func (s *server) claimSessionCleanup(sessionID string) bool {
	s.sessionCleanupMu.Lock()
	defer s.sessionCleanupMu.Unlock()
	if s.sessionCleanup[sessionID] {
		return false
	}
	if s.sessionCleanup == nil {
		s.sessionCleanup = map[string]bool{}
	}
	s.sessionCleanup[sessionID] = true
	return true
}

func (s *server) releaseSessionCleanup(sessionID string) {
	s.sessionCleanupMu.Lock()
	delete(s.sessionCleanup, sessionID)
	s.sessionCleanupMu.Unlock()
}
