package gaffer

import (
	"errors"
	"sort"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// Overview contains retained evidence, never a newly generated report or live
// worker/GitHub observation. Delivery acknowledges handoff, not task completion.
type GoalOverview struct {
	Goal      store.GafferGoal     `json:"goal"`
	Session   *GoalSession         `json:"session,omitempty"`
	Available bool                 `json:"available"`
	Attention bool                 `json:"attention"`
	Reason    string               `json:"reason,omitempty"`
	Delivery  *store.WorkerMessage `json:"delivery,omitempty"`
}

type GoalSession struct {
	ID      string `json:"id"`
	AgentID string `json:"agentId"`
	Title   string `json:"title"`
	State   string `json:"state"`
}

type GoalOverviewView struct {
	Goals  []GoalOverview `json:"goals"`
	ReadAt int64          `json:"readAt"`
}

func (s *Service) Goals(uid int64) (GoalOverviewView, error) {
	v := GoalOverviewView{Goals: []GoalOverview{}}
	if _, err := s.chatPrincipal(uid); err != nil {
		return v, err
	}
	goals, err := s.store.GafferGoalsFor(uid)
	if err != nil {
		return v, err
	}
	for _, g := range goals {
		row := GoalOverview{Goal: g, Attention: g.State == "needs_user" || g.State == "limit"}
		session, err := s.store.TerminalSessionFor(uid, g.SessionID)
		if err != nil && !errors.Is(err, store.ErrRemoteNotFound) && !errors.Is(err, store.ErrNotFound) {
			return v, err
		}
		if err == nil && session.Structured {
			archived, err := s.store.TerminalArchivedFor(uid, g.SessionID)
			if err != nil {
				return v, err
			}
			row.Session = &GoalSession{ID: session.ID, AgentID: session.AgentID, Title: session.Title, State: string(session.State)}
			row.Available = !archived
		}
		if !row.Available {
			row.Reason = "This session is archived or unavailable. Its retained goal can be reviewed here; use Conversation history to remove the saved chat."
			row.Attention = row.Attention || g.State == "active"
		} else {
			if g.State == "active" {
				if g.ExpiresAt <= time.Now().UnixMilli() {
					row.Reason = "The time limit has passed. No further goal instructions are authorised."
				} else if err := s.store.CheckGafferGoal(uid, g.ConversationID, g.ID, g.Revision, ""); err != nil {
					row.Reason = "Goal permissions changed. Review the session and its permissions before resuming."
				}
				row.Attention = row.Attention || row.Reason != ""
			}
			if g.PendingID != "" {
				// Reconcile only the retained queue; this never probes/wakes the worker.
				if s.worker != nil {
					if err := s.worker.ReconcileWorkerQueue(uid, g.SessionID); err != nil {
						return v, err
					}
				}
				m, err := s.store.WorkerMessageFor(uid, "gaffer", g.ConversationID, g.SessionID, g.PendingID)
				if err != nil && !errors.Is(err, store.ErrNotFound) {
					return v, err
				}
				if err == nil {
					row.Delivery = &m
					row.Attention = row.Attention || m.State == "uncertain" || m.State == "blocked"
				} else {
					row.Attention = true
					row.Reason = "The last instruction's receipt is unavailable. Check the working conversation before continuing."
				}
			}
		}
		v.Goals = append(v.Goals, row)
	}
	// Scheduler observations update frequently. Sort by actual review evidence,
	// not by polling timestamps, so cards remain steady while being read.
	sort.Slice(v.Goals, func(i, j int) bool {
		a, b := v.Goals[i], v.Goals[j]
		rank := func(r GoalOverview) int {
			if r.Attention {
				return 0
			}
			if r.Goal.State == "active" {
				return 1
			}
			if r.Goal.State == "paused" {
				return 2
			}
			return 3
		}
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		stamp := func(g store.GafferGoal) int64 {
			if g.LastReviewedAt > 0 {
				return g.LastReviewedAt
			}
			return g.CreatedAt
		}
		if stamp(a.Goal) != stamp(b.Goal) {
			return stamp(a.Goal) > stamp(b.Goal)
		}
		return a.Goal.ConversationID < b.Goal.ConversationID
	})
	v.ReadAt = time.Now().UnixMilli()
	return v, nil
}
