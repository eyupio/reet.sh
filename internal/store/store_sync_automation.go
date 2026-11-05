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
	enabled := agent.KeepSynced && !agent.Paused
	if !enabled || role == "viewer" {
		return nil
	}
	if err := s.prepareLibraryTargets(uid, role, agent, false); err != nil {
		return err
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
	// A failed private copy is reported on its own skill, not as a change to the
	// saved selection, so it must not make every poll rewrite the desired set.
	captureProblems, err := s.libraryCaptureProblems(uid)
	if err != nil {
		return err
	}
	var libraryCandidates []SyncSkill
	libraryRead := false
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
			if target.SourceID == "library" {
				if !libraryRead {
					libraryCandidates, err = s.SyncLibraryFor(uid, role)
					libraryRead = err == nil
				}
				candidates = libraryCandidates
				if libraryRead {
					err = nil
				}
			} else if target.SourceID == "shelf" {
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
					matching := env.ID == target.SourceID && env.Complete
					if matching && env.ScannedAt > time.Now().Add(-14*24*time.Hour).UnixMilli() {
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
			// Following the whole source also covers skills discovered without an
			// upstream URL. Capture one private copy through their enrolled host;
			// the durable transfer queues this destination when the bytes arrive.
			preparing := false
			for _, sk := range candidates {
				shelfSource := target.SourceID == "shelf" || target.SourceID == "library"
				customSource := sk.Source != "" && sk.SourceType != "local"
				if !shelfSource || customSource || sk.Problem != "" {
					continue
				}
				key := "follow_" + secretHash(target.Target.ID+"\x00"+sk.ID)
				if target.SourceID == "library" {
					// One private capture can serve the whole fleet. Other folders
					// pick up its immutable source on their next poll.
					key = "library_" + secretHash(sk.ID)
				}
				request, e := s.RequestMachineSkillSync(uid, sk.ID, agent.Host, "", key, target.Target.ID, true)
				if e == nil && (request.State == "failed" || request.State == "expired" || request.State == "cancelled") {
					e = fmt.Errorf("private copy %s: %s", request.State, request.Error)
				}
				if e != nil {
					// One skill that cannot be copied is reported and skipped; it
					// must not hold every other skill on the machine hostage.
					if e = problem("Could not copy " + sk.Name + " privately: " + e.Error() + ". Use Sync all machines to retry."); e != nil {
						return e
					}
					continue
				}
				preparing = true
				if e = problem("Copying machine-only skills privately before installation."); e != nil {
					return e
				}
			}
			if preparing {
				continue
			}
			ordered := func(skills []SyncSkill) []SyncSkill {
				out := slices.DeleteFunc(slices.Clone(skills), func(sk SyncSkill) bool {
					_, failed := captureProblems[sk.ID]
					return failed
				})
				slices.SortFunc(out, func(a, b SyncSkill) int { return strings.Compare(a.ID, b.ID) })
				return out
			}
			if !slices.Equal(ordered(candidates), ordered(target.Desired)) {
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
		noMissing := len(target.Missing) == 0
		additive := target.Mode == "additive" || len(target.Extra) == 0
		if noMissing && additive {
			continue
		}
		if _, err = s.StartSyncRun(uid, agent.ID, []string{target.Target.ID}); err != nil {
			return fmt.Errorf("maintain sync: %w", err)
		}
		return nil // one machine operation at a time
	}
	return nil
}
