package store

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// MaintainSync runs on authenticated agent polls, so saved policies continue
// working after the browser closes. It never replays failed or approved work.
func (s *Store) MaintainSync(uid int64, role string, agent SyncAgent) error {
	if !agent.KeepSynced || agent.Paused || role == "viewer" {
		return nil
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		return err
	}
	var pending int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE user_id=? AND agent_id=? AND status IN ('queued','running','awaiting-approval')`, uid, agent.ID).Scan(&pending); err != nil {
		return err
	}
	if pending > 0 {
		return nil
	}
	envs, err := s.SyncEnvironmentsFor(uid)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if !strings.EqualFold(target.Target.Host, agent.Host) {
			continue
		}
		fresh := false
		for _, env := range envs {
			if env.ID == target.Target.ID {
				fresh = env.Complete && env.ScannedAt > time.Now().Add(-14*24*time.Hour).UnixMilli()
			}
		}
		problem := func(message string) error {
			_, e := s.db.Exec(`UPDATE sync_policies SET problem=? WHERE target_id=?`, message, target.Target.ID)
			return e
		}
		if !fresh {
			if err := problem("A fresh complete scan is needed before sync can continue."); err != nil {
				return err
			}
			continue
		}
		if target.FollowSource {
			in := DesiredSelection{TargetID: target.Target.ID, SourceID: target.SourceID, Mode: target.Mode, Revision: target.Revision, FollowSource: true}
			var candidates []SyncSkill
			if target.SourceID == "shelf" {
				candidates, err = s.SyncCandidatesFor(uid, role, 0)
			} else if strings.HasPrefix(target.SourceID, "list:") {
				in.SourceID = "shelf"
				in.ListID, err = strconv.ParseInt(strings.TrimPrefix(target.SourceID, "list:"), 10, 64)
				if err == nil {
					candidates, err = s.SyncCandidatesFor(uid, role, in.ListID)
				}
			} else {
				err = errors.New("source needs a recent complete scan")
				for _, env := range envs {
					if env.ID == target.SourceID && env.Complete && env.ScannedAt > time.Now().Add(-14*24*time.Hour).UnixMilli() {
						candidates, err = env.Skills, nil
					}
				}
			}
			// A missing source never becomes an empty desired set. Leave the last
			// reviewed set alone until access or the source inventory is restored.
			if err != nil {
				if e := problem("Sync source is unavailable: " + err.Error()); e != nil {
					return e
				}
				continue
			}
			ids := func(skills []SyncSkill) []string {
				out := []string{}
				for _, sk := range skills {
					out = append(out, sk.ID)
				}
				slices.Sort(out)
				return out
			}
			if !slices.Equal(ids(candidates), ids(target.Desired)) {
				if err = s.SaveDesiredFor(uid, role, in); err != nil {
					if e := problem("Sync selection needs attention: " + err.Error()); e != nil {
						return e
					}
					continue
				}
				updated, e := s.SyncTargetsFor(uid)
				if e != nil {
					return e
				}
				for _, item := range updated {
					if item.Target.ID == target.Target.ID {
						target = item
					}
				}
			}
		}
		if err := problem(""); err != nil {
			return err
		}
		// A failed attempt needs a deliberate fix/retry. Continuous sync must not
		// turn a permissions or provenance problem into hundreds of failed jobs.
		var latestStatus string
		err := s.db.QueryRow(`SELECT status FROM sync_jobs WHERE user_id=? AND agent_id=? AND target_id=? AND revision=? ORDER BY created_at DESC,updated_at DESC LIMIT 1`, uid, agent.ID, target.Target.ID, target.Revision).Scan(&latestStatus)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if latestStatus == "failed" || latestStatus == "cancelled" {
			continue
		}
		if len(target.Missing) == 0 && (target.Mode == "additive" || len(target.Extra) == 0) {
			continue
		}
		if _, err = s.StartSyncRun(uid, agent.ID, []string{target.Target.ID}); err != nil {
			return fmt.Errorf("maintain sync: %w", err)
		}
		return nil // one machine operation at a time
	}
	return nil
}
