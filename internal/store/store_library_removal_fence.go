package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// SetManagedRemovals is the controller's internal rollout control for managed
// removals. It is off by default: while it is off the controller does not
// advertise the capability and a plan containing a removal is never approved
// automatically. It is not a user-facing setting.
func (s *Store) SetManagedRemovals(on bool) { s.managedRemovals.Store(on) }

// ManagedRemovalsEnabled reports whether managed removals are advertised.
func (s *Store) ManagedRemovalsEnabled() bool { return s.managedRemovals.Load() }

type removalVerdict int

const (
	// removalReview keeps the existing behaviour: a person reviews the plan.
	removalReview removalVerdict = iota
	// removalApproved means every removal is currently authorised by the
	// canonical status, selection and deletion state.
	removalApproved
	// removalStale means authoritative intent changed since the machine
	// planned; the plan must be made again, never applied or acknowledged.
	removalStale
)

type planRemovalAction struct {
	Add           bool                    `json:"add"`
	Replace       bool                    `json:"replace"`
	Skill         SyncSkill               `json:"skill"`
	RemovalIntent *LibraryDeploymentCheck `json:"removalIntent"`
}

// planRemovalVerdict checks every non-additive action against current
// controller intent in the caller's transaction. A plan is only automatically
// approvable when each such action is an explicit managed removal for this
// target whose generation is still current and whose intent is still "remove".
// Additive actions are not judged here. Whatever the plan says, an unknown
// shape, a missing proof, or another target's removal is left for review.
func planRemovalVerdict(tx *sql.Tx, uid int64, agentID, targetID string, plan json.RawMessage, enabled bool) (removalVerdict, error) {
	var parsed struct {
		Actions []planRemovalAction `json:"actions"`
	}
	if len(plan) == 0 || json.Unmarshal(plan, &parsed) != nil {
		return removalReview, nil
	}
	verdict := removalApproved
	for _, action := range parsed.Actions {
		if action.Add {
			continue
		}
		check := action.RemovalIntent
		if !enabled || action.Replace || check == nil || check.Generation == "" || check.TargetID != targetID || action.Skill.ItemID == "" || action.Skill.ItemID != check.ItemID {
			return removalReview, nil
		}
		intent, err := libraryDeploymentIntentTx(tx, uid, agentID, *check)
		switch {
		case errors.Is(err, ErrLibraryConflict), errors.Is(err, ErrRemoteNotFound):
			verdict = removalStale
		case err != nil && errors.Is(err, ErrPrivateSkillInvalid):
			return removalReview, nil
		case err != nil:
			return removalReview, err
		case intent.Action != "remove":
			verdict = removalStale
		}
	}
	return verdict, nil
}

// removalAcknowledgementCurrent reports whether every managed removal in the
// approved plan is still authorised. A superseded result is not an
// acknowledgement of success: the item may have been reselected or reactivated
// while the machine worked, and the next plan must reinstall it.
func removalAcknowledgementCurrent(tx *sql.Tx, uid int64, agentID, targetID string, plan json.RawMessage) (bool, error) {
	var parsed struct {
		Actions []planRemovalAction `json:"actions"`
	}
	if len(plan) == 0 || json.Unmarshal(plan, &parsed) != nil {
		return true, nil
	}
	for _, action := range parsed.Actions {
		if action.Add || action.RemovalIntent == nil {
			continue
		}
		check := *action.RemovalIntent
		if check.TargetID != targetID {
			return false, nil
		}
		intent, err := libraryDeploymentIntentTx(tx, uid, agentID, check)
		if errors.Is(err, ErrLibraryConflict) || errors.Is(err, ErrRemoteNotFound) || errors.Is(err, ErrPrivateSkillInvalid) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if intent.Action != "remove" {
			return false, nil
		}
	}
	return true, nil
}
