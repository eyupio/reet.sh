package gaffer

import (
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// sessionGone reports whether a session can never be looked after again: its
// row is missing, or it was archived (hidden from history, transcript cleared).
// A session that has merely ended or been lost is not gone: the person can
// still read it and move its pipeline on, so only they decide about it.
func (s *Service) sessionGone(uid int64, sessionID string) bool {
	if _, err := s.store.TerminalSessionFor(uid, sessionID); err != nil {
		return errors.Is(err, store.ErrRemoteNotFound)
	}
	archived, err := s.store.TerminalArchivedFor(uid, sessionID)
	return err == nil && archived
}

// stopPipelineGoal closes the goal that followed a pipeline stage, so it does
// not keep asking for the person after the pipeline has stopped being followed.
func (s *Service) stopPipelineGoal(uid int64, conversation, note string) {
	if conversation == "" {
		return
	}
	g, err := s.store.GafferGoalFor(uid, conversation)
	if err != nil || g.State == "stopped" || g.State == "completed" || g.ActiveTurnID != "" {
		return
	}
	_ = s.store.ObserveGafferGoal(g, "stopped", note, 0)
}

const sessionGoneNote = "Its session no longer exists, so nothing is following this pipeline any more. Retry it to start a new session, or archive or delete the outcome."

// releaseGoneSessions stops following every pipeline whose session has been
// archived or removed. Without it the pipeline would stay "following" for
// good: the goal supervisor notices the session is gone and asks for the
// person, but nothing ever finishes the pipeline, so the outcome could be
// neither archived nor deleted.
func (s *Service) releaseGoneSessions() {
	rows, err := s.store.FollowedGafferOutcomes()
	if err != nil {
		return
	}
	for _, o := range rows {
		if _, err := s.chatPrincipal(o.OwnerID); err != nil {
			continue
		}
		for _, piece := range o.Sessions {
			if piece.State != "following" || piece.SessionID == "" || !s.sessionGone(o.OwnerID, piece.SessionID) {
				continue
			}
			key, session, conversation := piece.Key, piece.SessionID, piece.ConversationID
			released := false
			after, err := s.outcomeStep(o.OwnerID, o.ID, func(o *store.GafferOutcome) bool {
				for i := range o.Sessions {
					row := &o.Sessions[i]
					if row.Key == key && row.State == "following" && row.SessionID == session {
						row.State, row.Note, row.UpdatedAt = "unfollowed", sessionGoneNote, time.Now().UnixMilli()
						released = true
						return true
					}
				}
				return false
			})
			if err != nil || !released {
				continue
			}
			s.stopPipelineGoal(o.OwnerID, conversation, sessionGoneNote)
			s.auditPipeline(after, key)
		}
	}
}

const stoppedFollowingNote = "You stopped following this pipeline. Its session, if it still exists, keeps running."

// StopFollowingOutcome is the person ending an outcome that is not finished:
// every pipeline still moving is no longer followed and the controller starts
// nothing more for it. Sessions that exist keep running. It is how an outcome
// whose session was lost or ended is put away, so it can be archived or deleted.
func (s *Service) StopFollowingOutcome(uid int64, id string, revision int64) (store.GafferOutcome, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferOutcome{}, err
	}
	var conversations []string
	o, err := s.store.UpdateGafferOutcomeFor(uid, id, revision, func(o *store.GafferOutcome) bool {
		if o.Finished() {
			return false
		}
		now := time.Now().UnixMilli()
		for i := range o.Sessions {
			row := &o.Sessions[i]
			switch row.State {
			case "following", "ready_to_merge", "advancing", "starting", "waiting":
				if row.SessionID == "" {
					row.State, row.Note = "cancelled", "Not started: you stopped this outcome first."
				} else {
					row.State, row.Note = "unfollowed", stoppedFollowingNote
					conversations = append(conversations, row.ConversationID)
				}
				row.UpdatedAt = now
			}
		}
		if o.State == "starting" {
			o.State, o.Note = "stopped", "You stopped this outcome. Sessions already started keep running."
		}
		return true
	})
	if err != nil {
		return outcomeView(o), err
	}
	for _, conversation := range conversations {
		s.stopPipelineGoal(uid, conversation, stoppedFollowingNote)
	}
	return outcomeView(o), nil
}
