package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Machine maintenance: housekeeping that is safe to repeat. Today it has one
// task, cleaning up the worktrees of finished sessions; each task has its own
// settings and last-run summary so others (log pruning, update checks) can sit
// beside it. A manual sweep shows what it found and removes only what the
// person picked. The scheduled sweep is a safety net and removes only a
// worktree that holds nothing the person has not already saved: no
// uncommitted changes, no commits on no branch, no hidden edits and no nested
// repository. Anything else is left for a person to review.
const (
	maintWorktreeEnabledKey  = "maintenance_worktrees_enabled"
	maintWorktreeIntervalKey = "maintenance_worktrees_interval_hours"
	maintWorktreeAgeKey      = "maintenance_worktrees_min_age_days"
	maintWorktreeLastKey     = "maintenance_worktrees_last_run"
	maintWorktreeByKey       = "maintenance_worktrees_changed_by"

	maintDefaultIntervalHours = 24
	maintDefaultMinAgeDays    = 7
	maintMaxCandidates        = 100
	maintMaxClean             = 50
	maintSweepTimeout         = 10 * time.Minute
	maintCheckEvery           = 15 * time.Minute
)

type worktreeMaintenanceSettings struct {
	Enabled       bool   `json:"enabled"`
	IntervalHours int    `json:"intervalHours"`
	MinAgeDays    int    `json:"minAgeDays"`
	ChangedBy     string `json:"changedBy,omitempty"`
}

// worktreeSweepSummary is what the last scheduled sweep did.
type worktreeSweepSummary struct {
	At          int64 `json:"at"`
	Removed     int   `json:"removed"`
	Review      int   `json:"review"`
	Unreachable int   `json:"unreachable"`
	Failed      int   `json:"failed"`
}

func (s *server) worktreeMaintenanceSettings() worktreeMaintenanceSettings {
	out := worktreeMaintenanceSettings{IntervalHours: maintDefaultIntervalHours, MinAgeDays: maintDefaultMinAgeDays}
	if v, _ := s.st.GetSetting(maintWorktreeEnabledKey); v == "1" {
		out.Enabled = true
	}
	if n, err := strconv.Atoi(settingOr(s, maintWorktreeIntervalKey)); err == nil && n >= 1 && n <= 24*30 {
		out.IntervalHours = n
	}
	if n, err := strconv.Atoi(settingOr(s, maintWorktreeAgeKey)); err == nil && n >= 0 && n <= 365 {
		out.MinAgeDays = n
	}
	out.ChangedBy, _ = s.st.GetSetting(maintWorktreeByKey)
	return out
}

func settingOr(s *server, key string) string { v, _ := s.st.GetSetting(key); return v }

func (s *server) worktreeSweepLast() *worktreeSweepSummary {
	raw, _ := s.st.GetSetting(maintWorktreeLastKey)
	if raw == "" {
		return nil
	}
	var out worktreeSweepSummary
	if json.Unmarshal([]byte(raw), &out) != nil {
		return nil
	}
	return &out
}

func (s *server) maintenanceGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"worktrees": s.worktreeMaintenanceSettings(), "lastRun": s.worktreeSweepLast()})
}

func (s *server) maintenancePut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled       *bool `json:"enabled"`
		IntervalHours *int  `json:"intervalHours"`
		MinAgeDays    *int  `json:"minAgeDays"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Enabled == nil && in.IntervalHours == nil && in.MinAgeDays == nil {
		writeErr(w, http.StatusBadRequest, "nothing to change")
		return
	}
	if in.IntervalHours != nil && (*in.IntervalHours < 1 || *in.IntervalHours > 24*30) {
		writeErr(w, http.StatusBadRequest, "intervalHours must be between 1 and 720")
		return
	}
	if in.MinAgeDays != nil && (*in.MinAgeDays < 0 || *in.MinAgeDays > 365) {
		writeErr(w, http.StatusBadRequest, "minAgeDays must be between 0 and 365")
		return
	}
	set := map[string]string{maintWorktreeByKey: s.currentUser(r).Name}
	action := "maintenance-settings"
	if in.Enabled != nil {
		set[maintWorktreeEnabledKey] = map[bool]string{true: "1", false: "0"}[*in.Enabled]
		action = map[bool]string{true: "maintenance-worktrees-on", false: "maintenance-worktrees-off"}[*in.Enabled]
	}
	if in.IntervalHours != nil {
		set[maintWorktreeIntervalKey] = strconv.Itoa(*in.IntervalHours)
	}
	if in.MinAgeDays != nil {
		set[maintWorktreeAgeKey] = strconv.Itoa(*in.MinAgeDays)
	}
	for k, v := range set {
		if err := s.st.SetSetting(k, v); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.event(s.uid(r), "ui", action, "", map[string]any{"by": s.currentUser(r).Name})
	s.maintenanceGet(w, r)
}

// Sweep statuses for one worktree.
const (
	sweepSafe        = "safe"        // nothing unsaved: may be removed
	sweepReview      = "review"      // uncommitted changes: a person decides
	sweepBlocked     = "blocked"     // another check refuses it; a person looks
	sweepUnreachable = "unreachable" // the machine could not be asked
	sweepGone        = "gone"        // not on the machine any more
)

type sweepItem struct {
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId"`
	Host      string `json:"host"`
	Title     string `json:"title"`
	Project   string `json:"project"`
	Repo      string `json:"repo"`
	Run       string `json:"run"`
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	EndedAt   int64  `json:"endedAt"`
	Status    string `json:"status"`
	Changed   int    `json:"changed"`
	Unpushed  int    `json:"unpushed"`
	Reason    string `json:"reason,omitempty"`
	Removed   bool   `json:"removed,omitempty"`
}

// worktreeParts reads the project slug and run token from a worktree folder
// of the form <home>/.reet/worktrees/<project>/<repo>/<run>.
func worktreeParts(p string) (project, run string, ok bool) {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	n := len(parts)
	if n < 5 || parts[n-4] != "worktrees" || parts[n-5] != ".reet" {
		return "", "", false
	}
	project, run = parts[n-3], parts[n-1]
	return project, run, terminalproto.ValidSlug(project) && terminalproto.ValidRunToken(run) && path.Base(p) == run
}

// classifyWorktree says what may be done with a worktree whose state the
// machine reported.
func classifyWorktree(st terminalproto.WorktreeState) (string, string) {
	switch {
	case st.Unreferenced > 0:
		return sweepBlocked, "unreferenced_commits"
	case st.Hidden > 0:
		return sweepBlocked, "hidden_changes"
	case st.Nested:
		return sweepBlocked, "nested_repository"
	case st.Incomplete:
		return sweepBlocked, "check_incomplete"
	case st.Changed > 0:
		return sweepReview, "has_changes"
	}
	return sweepSafe, ""
}

// sweepWorktrees looks at the account's finished sessions' worktrees one
// machine at a time. With apply, it removes the ones that are safe; it never
// removes anything else, and never forces.
func (s *server) sweepWorktrees(ctx context.Context, uid int64, minAgeDays int, apply bool, automatic bool) ([]sweepItem, bool, error) {
	olderThan := time.Now().Add(-time.Duration(minAgeDays) * 24 * time.Hour).UnixMilli()
	cands, err := s.st.WorktreeCandidatesFor(uid, olderThan, maintMaxCandidates+1)
	if err != nil {
		return nil, false, err
	}
	more := len(cands) > maintMaxCandidates
	if more {
		cands = cands[:maintMaxCandidates]
	}
	down := map[string]bool{}
	out := make([]sweepItem, 0, len(cands))
	for _, c := range cands {
		project, run, ok := worktreeParts(c.Path)
		if !ok {
			continue
		}
		item := sweepItem{SessionID: c.SessionID, AgentID: c.AgentID, Host: s.machineHost(uid, c.AgentID), Title: c.Title, Project: project, Repo: c.Repo,
			Run: run, Path: c.Path, Branch: c.Branch, EndedAt: c.EndedAt}
		if down[c.AgentID] {
			item.Status, item.Reason = sweepUnreachable, "offline"
			out = append(out, item)
			continue
		}
		if ctx.Err() != nil {
			item.Status, item.Reason = sweepUnreachable, "timed_out"
			out = append(out, item)
			continue
		}
		state, err := s.terminals.WorktreeStatus(ctx, uid, c.AgentID, project, c.Repo, run)
		switch {
		case errors.Is(err, terminalbroker.ErrWorktreeNotFound):
			_ = s.st.MarkWorktreeGone(uid, c.AgentID, c.Path)
			continue
		case errors.Is(err, terminalbroker.ErrWorktreeRefused):
			item.Status = sweepBlocked
			var we *terminalbroker.WorktreeError
			if errors.As(err, &we) {
				item.Reason = string(we.Reason)
			}
		case err != nil:
			item.Status, item.Reason = sweepUnreachable, "unavailable"
			if errors.Is(err, terminalbroker.ErrOffline) || errors.Is(err, terminalbroker.ErrWorktreeUnsupported) || errors.Is(err, terminalbroker.ErrWorktreeUnavailable) {
				down[c.AgentID] = true
			}
		default:
			item.Status, item.Reason = classifyWorktree(state)
			item.Changed, item.Unpushed = state.Changed, state.Unpushed
			if state.Path != "" {
				item.Path = state.Path
			}
		}
		if apply && item.Status == sweepSafe {
			item = s.removeSwept(ctx, uid, item, automatic)
		}
		out = append(out, item)
	}
	return out, more, nil
}

// removeSwept removes one safe worktree with the counts just read, so a change
// made in between is refused by the machine rather than lost. The branch is
// kept. A failure leaves the item as it was, with the reason.
func (s *server) removeSwept(ctx context.Context, uid int64, item sweepItem, automatic bool) sweepItem {
	shown := terminalbroker.WorktreeCounts{Changed: item.Changed, Unpushed: item.Unpushed}
	removed, err := s.terminals.WorktreeRemove(ctx, uid, item.AgentID, item.Project, item.Repo, item.Run, shown)
	if errors.Is(err, terminalbroker.ErrWorktreeNotFound) {
		_ = s.st.MarkWorktreeGone(uid, item.AgentID, item.Path)
		item.Status, item.Removed = sweepGone, true
		return item
	}
	if err != nil {
		item.Status, item.Reason = sweepUnreachable, "remove_failed"
		var we *terminalbroker.WorktreeError
		switch {
		case errors.Is(err, terminalbroker.ErrWorktreeChanged):
			item.Status, item.Reason = sweepReview, "changed_since_checked"
		case errors.As(err, &we):
			item.Status, item.Reason = sweepBlocked, string(we.Reason)
		}
		return item
	}
	_ = s.st.MarkWorktreeGone(uid, item.AgentID, item.Path)
	item.Removed, item.Status = true, sweepGone
	detail := map[string]string{"machine": item.AgentID, "host": item.Host, "path": removed.Path, "branch": removed.Branch}
	if automatic {
		detail["automatic"] = "true"
	}
	s.event(uid, "maintenance", "worktree-remove", "", detail)
	s.recordRunWorktreeRemoved(uid, item.AgentID, item.Run)
	s.autoPruneAfterWorktree(uid, item.AgentID, item.Run)
	return item
}

func (s *server) maintenanceWorktreeScan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MinAgeDays *int `json:"minAgeDays"`
	}
	if !terminalOptionalJSON(w, r, &in) {
		return
	}
	age := s.worktreeMaintenanceSettings().MinAgeDays
	if in.MinAgeDays != nil {
		if *in.MinAgeDays < 0 || *in.MinAgeDays > 365 {
			writeErr(w, http.StatusBadRequest, "minAgeDays must be between 0 and 365")
			return
		}
		age = *in.MinAgeDays
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	items, more, err := s.sweepWorktrees(ctx, s.uid(r), age, false, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "more": more, "minAgeDays": age})
}

// maintenanceWorktreeClean removes the worktrees a person picked from a scan.
// Each is checked again on its machine; one that now has anything unsaved is
// left and reported. Unsaved work is never removed here: that goes through the
// worktree's own dialog, one at a time, with its warning.
func (s *server) maintenanceWorktreeClean(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Items []struct {
			AgentID string `json:"agentId"`
			Project string `json:"project"`
			Repo    string `json:"repo"`
			Run     string `json:"run"`
		} `json:"items"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	if len(in.Items) == 0 || len(in.Items) > maintMaxClean {
		writeErr(w, http.StatusBadRequest, "pick between 1 and "+strconv.Itoa(maintMaxClean)+" worktrees")
		return
	}
	uid := s.uid(r)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	out := make([]sweepItem, 0, len(in.Items))
	for _, t := range in.Items {
		item := sweepItem{AgentID: t.AgentID, Host: s.machineHost(uid, t.AgentID), Project: t.Project, Repo: t.Repo, Run: t.Run}
		if terminalbroker.CheckWorktreeRemove(t.Project, t.Repo, t.Run, terminalbroker.WorktreeCounts{}) != nil {
			item.Status, item.Reason = sweepBlocked, "invalid"
			out = append(out, item)
			continue
		}
		state, err := s.terminals.WorktreeStatus(ctx, uid, t.AgentID, t.Project, t.Repo, t.Run)
		switch {
		case errors.Is(err, terminalbroker.ErrWorktreeNotFound):
			_ = s.st.MarkWorktreeGoneByRun(uid, t.AgentID, t.Run)
			item.Status, item.Removed = sweepGone, true
		case err != nil:
			item.Status, item.Reason = sweepUnreachable, "unavailable"
		default:
			item.Path, item.Branch = state.Path, state.Branch
			item.Changed, item.Unpushed = state.Changed, state.Unpushed
			item.Status, item.Reason = classifyWorktree(state)
			if item.Status == sweepSafe {
				item = s.removeSwept(ctx, uid, item, false)
			}
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// maintenanceWorker runs the scheduled sweeps. It wakes every few minutes and
// sweeps only when the setting is on and the interval has passed.
func (s *server) maintenanceWorker() {
	ticker := time.NewTicker(maintCheckEvery)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.runScheduledMaintenance(time.Now())
		}
	}
}

var maintMu sync.Mutex

func (s *server) runScheduledMaintenance(now time.Time) {
	if !maintMu.TryLock() {
		return
	}
	defer maintMu.Unlock()
	// A fenced controller runs no scheduled sweeps on machines.
	if s.st.Fenced() {
		return
	}
	cfg := s.worktreeMaintenanceSettings()
	if !cfg.Enabled {
		return
	}
	if last := s.worktreeSweepLast(); last != nil && now.Sub(time.UnixMilli(last.At)) < time.Duration(cfg.IntervalHours)*time.Hour {
		return
	}
	users, err := s.st.ListUsers()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.baseContext(), maintSweepTimeout)
	defer cancel()
	sum := worktreeSweepSummary{At: now.UnixMilli()}
	for _, u := range users {
		if u.DisabledAt != 0 || u.Role == "viewer" {
			continue
		}
		items, _, err := s.sweepWorktrees(ctx, u.ID, cfg.MinAgeDays, true, true)
		if err != nil {
			sum.Failed++
			continue
		}
		for _, it := range items {
			switch {
			case it.Removed:
				sum.Removed++
			case it.Status == sweepReview || it.Status == sweepBlocked:
				sum.Review++
			case it.Status == sweepUnreachable:
				sum.Unreachable++
			}
		}
		if len(items) > 0 {
			s.event(u.ID, "maintenance", "worktree-sweep", "", map[string]int{"removed": countRemoved(items), "needsReview": len(items) - countRemoved(items)})
		}
	}
	if raw, err := json.Marshal(sum); err == nil {
		_ = s.st.SetSetting(maintWorktreeLastKey, string(raw))
	}
}

func countRemoved(items []sweepItem) int {
	n := 0
	for _, it := range items {
		if it.Removed {
			n++
		}
	}
	return n
}
