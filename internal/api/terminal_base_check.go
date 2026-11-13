package api

import (
	"net/http"
	"strconv"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Where a session would start: one read-only question to a machine that
// fetches a worktree's base from GitHub, asked before anything is started.
// The machine makes no worktree and the controller keeps nothing of the
// answer.

// baseCheckJSON is the answer, and nothing else: what the machine said about
// the base, with the fields it left empty left out.
type baseCheckJSON struct {
	Base     string `json:"base,omitempty"`
	Relation string `json:"relation,omitempty"`
	Local    string `json:"local,omitempty"`
	Remote   string `json:"remote,omitempty"`
	Start    string `json:"start,omitempty"`
}

func (s *server) machineBaseCheck(w http.ResponseWriter, r *http.Request) {
	q, ok := machineQuery(r, "dir", "base")
	agent := r.PathValue("agentId")
	if !ok || !terminalproto.ValidID(agent) {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	req := terminalproto.BaseCheck{Dir: q.Get("dir"), Base: q.Get("base")}
	if req.Base == "" {
		// A browser that names no base is asking about the project's own: the
		// branch an isolated session in that folder would start from
		// (isolateWorktree names the same one, and no other). A folder with no
		// saved project, and a project that saved no base, leave the choice to
		// the machine, and the answer is about the branch it would choose. The
		// account's default base belongs to the starts the browser sends it
		// with, which name it in this request themselves.
		if p, found, err := s.st.RepoProjectForDir(uid, agent, req.Dir); err == nil && found {
			req.Base = p.BaseBranch
		}
	}
	if terminalbroker.CheckBaseCheck(req) != nil {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	if !s.terminals.Ready(uid, agent) {
		s.machineRequestError(w, terminalbroker.ErrOffline, uid, agent)
		return
	}
	// A check fetches from GitHub on the machine, so it shares the limit the
	// other requests that run git over a whole repository take.
	limits, _ := s.browseState()
	if !limits.take(func(l *browseLimits) *auth.Limiter { return l.prune }, strconv.FormatInt(uid, 10)+"/"+agent) {
		writeErrCode(w, http.StatusTooManyRequests, "branches_rate_limited", "", sentenceSlowDown, "")
		return
	}
	state, err := s.terminals.CheckBase(r.Context(), uid, agent, req)
	if err != nil {
		s.machineRequestError(w, err, uid, agent)
		return
	}
	writeJSON(w, http.StatusOK, baseCheckJSON{Base: state.Base, Relation: string(state.Relation),
		Local: state.Local, Remote: state.Remote, Start: state.Start})
}
