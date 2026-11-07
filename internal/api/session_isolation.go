package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
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

// cleanupSessionWorktree removes the worktree of a session that has just been
// removed from history, but only when nothing in it is unsaved. A worktree with
// uncommitted changes, unreferenced commits or anything else the machine
// refuses is left in place and stays listed in Account → Maintenance for a
// person to review, as is one on a machine that is offline. The branch is kept.
func (s *server) cleanupSessionWorktree(uid int64, sess store.TerminalSession) {
	if sess.WorktreePath == "" || sess.WorktreeRepo == "" {
		return
	}
	project, run, ok := worktreeParts(sess.WorktreePath)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(s.baseContext(), 3*time.Minute)
	defer cancel()
	state, err := s.terminals.WorktreeStatus(ctx, uid, sess.AgentID, project, sess.WorktreeRepo, run)
	if errors.Is(err, terminalbroker.ErrWorktreeNotFound) {
		_ = s.st.MarkWorktreeGone(uid, sess.AgentID, sess.WorktreePath)
		return
	}
	if err != nil {
		return
	}
	status, reason := classifyWorktree(state)
	if status != sweepSafe {
		s.event(uid, "ui", "worktree-kept", "", map[string]string{"machine": sess.AgentID, "session": sess.ID, "path": state.Path, "reason": reason})
		return
	}
	item := sweepItem{SessionID: sess.ID, AgentID: sess.AgentID, Host: s.machineHost(uid, sess.AgentID), Title: sess.Title, Project: project,
		Repo: sess.WorktreeRepo, Run: run, Path: sess.WorktreePath, Branch: sess.WorktreeBranch, Status: status, Changed: state.Changed, Unpushed: state.Unpushed}
	s.removeSwept(ctx, uid, item, true)
}
