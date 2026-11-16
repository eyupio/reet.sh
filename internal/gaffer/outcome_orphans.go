package gaffer

import (
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
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

// sessionRunning reports whether a session is still there to be followed.
func (s *Service) sessionRunning(uid int64, sessionID string) bool {
	row, err := s.store.TerminalSessionFor(uid, sessionID)
	if err != nil || row.State != terminalproto.Running {
		return false
	}
	archived, err := s.store.TerminalArchivedFor(uid, sessionID)
	return err == nil && !archived
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

// What stopping a session may do to a Factory outcome whose pipeline still
// works in it. "leave" is the empty string: the outcome is not touched.
const (
	SessionEndLeave   = ""
	SessionEndArchive = "archive"
	SessionEndDelete  = "delete"
)

// EndSessionOutcome applies the person's choice when they stop a session that
// a Factory pipeline is still working in. The outcome is stopped following
// first, so an archive or delete is allowed, and its goal is closed. Only the
// pipeline's session in hand counts: an earlier stage's session, already
// finished with, leaves the outcome alone.
func (s *Service) EndSessionOutcome(uid int64, sessionID, mode string) error {
	if mode == SessionEndLeave {
		return nil
	}
	if mode != SessionEndArchive && mode != SessionEndDelete {
		return invalid("session-end", "Choose leave, archive or delete for a Factory outcome.")
	}
	if _, err := s.chatPrincipal(uid); err != nil {
		return err
	}
	rows, err := s.store.GafferOutcomesFor(uid)
	if err != nil {
		return err
	}
	var firstErr error
	for _, o := range rows {
		if !outcomeWorksIn(o, sessionID) {
			continue
		}
		if err := s.endOutcomeForSession(uid, o.ID, mode); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// outcomeWorksIn reports whether sessionID is the session a pipeline is
// working in now, rather than one an earlier stage has finished with.
func outcomeWorksIn(o store.GafferOutcome, sessionID string) bool {
	for _, piece := range o.Sessions {
		if piece.SessionID != sessionID {
			continue
		}
		switch piece.State {
		case "following", "starting", "advancing", "waiting", "ready_to_merge":
			return true
		}
	}
	return false
}

// endOutcomeForSession stops following an outcome and then archives or deletes
// it. The controller may settle the outcome between the two writes, so a busy
// revision is read again and retried.
func (s *Service) endOutcomeForSession(uid int64, id, mode string) error {
	for attempt := 0; attempt < 4; attempt++ {
		o, err := s.store.GafferOutcomeFor(uid, id)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !o.Finished() {
			if o, err = s.StopFollowingOutcome(uid, id, o.Revision); errors.Is(err, store.ErrGafferOutcomeBusy) {
				continue
			} else if err != nil {
				return err
			}
		}
		switch mode {
		case SessionEndArchive:
			if o.ArchivedAt > 0 {
				return nil
			}
			_, err = s.ArchiveOutcome(uid, id, o.Revision, true)
		case SessionEndDelete:
			err = s.DeleteOutcome(uid, id, o.Revision)
		}
		switch {
		case err == nil, errors.Is(err, store.ErrNotFound):
			return nil
		case errors.Is(err, store.ErrGafferOutcomeBusy), errors.Is(err, store.ErrGafferOutcomeActive):
			continue
		default:
			return err
		}
	}
	return store.ErrGafferOutcomeBusy
}

// WorkDirInUse reports whether a pipeline still has work to do in a folder:
// the worktree its first stage was given, which every later stage and any
// retry works in. The session list uses it to keep that worktree when the
// session that made it is removed from history.
func (s *Service) WorkDirInUse(uid int64, dir string) bool {
	if dir == "" {
		return false
	}
	rows, err := s.store.GafferOutcomesFor(uid)
	if err != nil {
		return false
	}
	for _, o := range rows {
		if o.ArchivedAt > 0 {
			continue
		}
		for _, piece := range o.Sessions {
			if piece.WorkDir != dir {
				continue
			}
			switch piece.State {
			case "following", "starting", "advancing", "ready_to_merge", "unfollowed":
				return true
			}
		}
	}
	return false
}
