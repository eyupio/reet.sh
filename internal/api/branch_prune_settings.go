package api

import (
	"net/http"
	"strconv"
	"time"
)

// The account-wide automation setting for branch pruning (docs/branch-pruning.md):
// after a worktree is removed, prune the merged local branches in the
// repository it came from. Off unless an owner or admin turns it on, and every
// run is audited like a manual prune.
const (
	branchPruneSettingKey = "branch_prune_after_worktree"
	branchPruneByKey      = "branch_prune_changed_by"
	branchPruneAtKey      = "branch_prune_changed_at"
)

type branchPruneSettingView struct {
	AfterWorktree bool   `json:"afterWorktree"`
	ChangedBy     string `json:"changedBy,omitempty"`
	ChangedAt     int64  `json:"changedAt,omitempty"`
}

// branchPruneAfterWorktree reports the setting as it is now.
func (s *server) branchPruneAfterWorktree() bool {
	v, _ := s.st.GetSetting(branchPruneSettingKey)
	return v == "1"
}

func (s *server) branchPruneSetting() (branchPruneSettingView, error) {
	var out branchPruneSettingView
	v, err := s.st.GetSetting(branchPruneSettingKey)
	if err != nil {
		return out, err
	}
	out.AfterWorktree = v == "1"
	out.ChangedBy, _ = s.st.GetSetting(branchPruneByKey)
	if raw, _ := s.st.GetSetting(branchPruneAtKey); raw != "" {
		out.ChangedAt, _ = strconv.ParseInt(raw, 10, 64)
	}
	return out, nil
}

func (s *server) branchPruneSettingGet(w http.ResponseWriter, r *http.Request) {
	out, err := s.branchPruneSetting()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) branchPruneSettingPut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AfterWorktree *bool `json:"afterWorktree"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.AfterWorktree == nil {
		writeErr(w, http.StatusBadRequest, "afterWorktree is required")
		return
	}
	value := "0"
	action := "branch-prune-automation-off"
	if *in.AfterWorktree {
		value, action = "1", "branch-prune-automation-on"
	}
	name := s.currentUser(r).Name
	at := time.Now().UnixMilli()
	for key, v := range map[string]string{
		branchPruneSettingKey: value,
		branchPruneByKey:      name,
		branchPruneAtKey:      strconv.FormatInt(at, 10),
	} {
		if err := s.st.SetSetting(key, v); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.eventRequest(r, action, "", map[string]any{"afterWorktree": *in.AfterWorktree})
	out, err := s.branchPruneSetting()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
