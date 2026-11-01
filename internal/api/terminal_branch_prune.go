package api

import (
	"net/http"
	"strconv"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Pruning merged local branches (docs/remote-machine-operations.md): a plan
// the person reads, then a prune of exactly the branches in it. Both are sent
// to the machine; only the prune writes an audit row.

type branchPruneBranchJSON struct {
	Name string `json:"name"`
	OID  string `json:"oid"`
	Skip string `json:"skip"`
}

type branchPrunePlanJSON struct {
	Current  string                  `json:"current"`
	Default  string                  `json:"default"`
	Base     string                  `json:"base"`
	Branches []branchPruneBranchJSON `json:"branches"`
	More     bool                    `json:"more"`
}

type branchPruneSkippedJSON struct {
	Name string `json:"name"`
	Skip string `json:"skip"`
}

type branchPruneResultJSON struct {
	Deleted []string                 `json:"deleted"`
	Skipped []branchPruneSkippedJSON `json:"skipped"`
}

func (s *server) machineBranchPrunePlan(w http.ResponseWriter, r *http.Request) {
	q, ok := machineQuery(r, "dir", "base")
	if !ok {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	req := terminalproto.BranchPrunePlan{Dir: q.Get("dir"), Base: q.Get("base")}
	agent := r.PathValue("agentId")
	if !terminalproto.ValidID(agent) || terminalbroker.CheckBranchPrunePlan(req) != nil {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	if !s.terminals.Ready(uid, agent) {
		s.machineRequestError(w, terminalbroker.ErrOffline, uid, agent)
		return
	}
	limits, _ := s.browseState()
	if !limits.take(func(l *browseLimits) *auth.Limiter { return l.prune }, strconv.FormatInt(uid, 10)+"/"+agent) {
		writeErrCode(w, http.StatusTooManyRequests, "branches_rate_limited", "", sentenceSlowDown, "")
		return
	}
	result, err := s.terminals.BranchPrunePlan(r.Context(), uid, agent, req)
	if err != nil {
		s.machineRequestError(w, err, uid, agent)
		return
	}
	out := branchPrunePlanJSON{Current: result.Current, Default: result.Default, Base: result.Base, Branches: []branchPruneBranchJSON{}, More: result.More}
	for _, b := range result.Branches {
		out.Branches = append(out.Branches, branchPruneBranchJSON{Name: b.Name, OID: b.OID, Skip: string(b.Skip)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) machineBranchPrune(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Dir      string `json:"dir"`
		Base     string `json:"base"`
		Branches []struct {
			Name string `json:"name"`
			OID  string `json:"oid"`
		} `json:"branches"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	req := terminalproto.BranchPrune{Dir: in.Dir, Base: in.Base}
	for _, b := range in.Branches {
		req.Branches = append(req.Branches, terminalproto.BranchPruneTarget{Name: b.Name, OID: b.OID})
	}
	agent := r.PathValue("agentId")
	if !terminalproto.ValidID(agent) || terminalbroker.CheckBranchPrune(req) != nil {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	if !s.terminals.Ready(uid, agent) {
		s.machineRequestError(w, terminalbroker.ErrOffline, uid, agent)
		return
	}
	limits, _ := s.browseState()
	if !limits.take(func(l *browseLimits) *auth.Limiter { return l.prune }, strconv.FormatInt(uid, 10)+"/"+agent) {
		writeErrCode(w, http.StatusTooManyRequests, "branches_rate_limited", "", sentenceSlowDown, "")
		return
	}
	result, err := s.terminals.PruneBranches(r.Context(), uid, agent, req)
	if err != nil {
		s.machineRequestError(w, err, uid, agent)
		return
	}
	s.event(uid, "ui", "branches-prune", "", map[string]any{
		"machine": agent, "host": s.machineHost(uid, agent), "dir": in.Dir,
		"deleted": result.Deleted, "skipped": len(result.Skipped),
	})
	out := branchPruneResultJSON{Deleted: result.Deleted, Skipped: []branchPruneSkippedJSON{}}
	if out.Deleted == nil {
		out.Deleted = []string{}
	}
	for _, s := range result.Skipped {
		out.Skipped = append(out.Skipped, branchPruneSkippedJSON{Name: s.Name, Skip: string(s.Skip)})
	}
	writeJSON(w, http.StatusOK, out)
}
