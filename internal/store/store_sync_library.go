package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// SetSyncLibraryOrigin remembers the controller for background work. It contains
// no credentials; private revisions continue to require the owning account.
func (s *Store) SetSyncLibraryOrigin(uid int64, origin string) error {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/") {
		return errors.New("controller origin unavailable")
	}
	_, err = s.db.Exec(`INSERT INTO sync_library(user_id,origin) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET origin=excluded.origin`, uid, strings.TrimRight(origin, "/"))
	return err
}

// PrivateSyncIdentityFor identifies a reported folder by its saved private
// install, rather than creating a second local/<name> skill on every scan.
// Content observations still decide whether the installed bytes match.
func (s *Store) PrivateSyncIdentityFor(uid int64, host, agent, project, name string) (SyncSkill, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT t.desired FROM sync_targets t JOIN environments e ON e.id=t.target_id AND e.user_id=t.user_id WHERE t.user_id=? AND e.host=? COLLATE NOCASE AND e.agent=? AND e.project=?`, uid, host, agent, project).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncSkill{}, false, nil
	}
	if err != nil {
		return SyncSkill{}, false, err
	}
	var skills []SyncSkill
	if err = json.Unmarshal([]byte(raw), &skills); err != nil {
		return SyncSkill{}, false, err
	}
	for _, skill := range skills {
		if skill.Slug == name && skill.SourceType == "private" {
			return skill, true, nil
		}
	}
	return SyncSkill{}, false, nil
}

// SyncLibraryFor is the shared collection: shelf skills and every written skill,
// including unpublished drafts. Own written content is pinned privately, even
// when published, so an edit changes its revision and never depends on a public
// URL cache. An unlinked scan of the same name is not another library entry.
func (s *Store) SyncLibraryFor(uid int64, role string) ([]SyncSkill, error) {
	var origin string
	if err := s.db.QueryRow(`SELECT origin FROM sync_library WHERE user_id=?`, uid).Scan(&origin); err != nil {
		return nil, err
	}
	shelf, err := s.SyncCandidatesFor(uid, role, 0)
	if err != nil {
		return nil, err
	}
	written, err := s.SyncHostedFor(uid, origin)
	if err != nil {
		return nil, err
	}
	hosted, err := s.HostedSkillsFor(uid)
	if err != nil {
		return nil, err
	}
	bySlug := map[string]HostedSkill{}
	for _, sk := range hosted {
		bySlug[sk.Slug] = sk
	}
	byID := map[string]SyncSkill{}
	for _, sk := range shelf {
		byID[sk.ID] = sk
	}
	for _, sk := range written {
		byID[sk.ID] = s.privateDraftCandidate(uid, sk.ID, bySlug[sk.Slug])
	}
	linked := map[string]bool{}
	sources := map[string]string{}
	conflicts := map[string]bool{}
	for _, sk := range byID {
		if sk.Source != "" && sk.SourceType != "local" {
			linked[sk.Slug] = true
			key := sk.SourceType + ":" + sk.Source
			if prior, ok := sources[sk.Slug]; ok && prior != key {
				conflicts[sk.Slug] = true
			}
			sources[sk.Slug] = key
		}
	}
	out := []SyncSkill{}
	for _, sk := range byID {
		if sk.SourceType == "local" && sk.Problem == "" && linked[sk.Slug] {
			continue
		}
		if conflicts[sk.Slug] {
			sk.Problem = fmt.Sprintf("More than one source uses the name %s. Remove the duplicate from your shelf before syncing.", sk.Slug)
		}
		out = append(out, sk)
	}
	slices.SortFunc(out, func(a, b SyncSkill) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// prepareLibraryTargets enrols newly scanned home folders in the shared library.
// Project folders remain explicit. Automatic sync owns home-folder plans;
// turning it off allows a custom plan. reset also retries settled failures.
func (s *Store) prepareLibraryTargets(uid int64, role string, agent SyncAgent, reset bool) error {
	var origin string
	err := s.db.QueryRow(`SELECT origin FROM sync_library WHERE user_id=?`, uid).Scan(&origin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // legacy installations keep their explicit policies
	}
	if err != nil {
		return err
	}
	rows, err := s.db.Query(`SELECT e.id,COALESCE(t.revision,'') FROM environments e JOIN environment_scans sc ON sc.env_id=e.id LEFT JOIN sync_targets t ON t.user_id=e.user_id AND t.target_id=e.id LEFT JOIN sync_policies p ON p.target_id=e.id LEFT JOIN sync_options o ON o.target_id=e.id WHERE e.user_id=? AND e.host=? COLLATE NOCASE AND e.project='' AND e.agent IN ('agents','claude','cursor','opencode') AND sc.complete=1 AND sc.scanned_at>=? AND (? OR t.target_id IS NULL OR t.source_id!='library' OR COALESCE(p.follow_source,0)!=1 OR COALESCE(o.mode,'')!='additive')`, uid, agent.Host, time.Now().Add(-14*24*time.Hour).UnixMilli(), reset)
	if err != nil {
		return err
	}
	type location struct{ id, revision string }
	var locations []location
	for rows.Next() {
		var item location
		if err = rows.Scan(&item.id, &item.revision); err != nil {
			break
		}
		locations = append(locations, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if len(locations) == 0 {
		return nil
	}
	// A settled machine does not need its entire library rebuilt merely to
	// discover that all its folders were already prepared.
	var pending int
	if err = s.db.QueryRow(`SELECT count(*) FROM sync_jobs WHERE user_id=? AND agent_id=? AND status IN ('queued','running','awaiting-approval')`, uid, agent.ID).Scan(&pending); err != nil {
		return err
	}
	if pending > 0 {
		return nil
	}
	candidates, err := s.SyncLibraryFor(uid, role)
	if err != nil {
		return err
	}
	ids := []string{}
	for _, sk := range candidates {
		if sk.Source != "" && sk.SourceType != "local" && sk.Problem == "" {
			ids = append(ids, sk.ID)
		}
	}
	for _, item := range locations {
		// Pending work keeps its exact revision until it settles. A later poll
		// catches up without invalidating an in-flight plan.
		if err = s.SaveDesiredFor(uid, role, DesiredSelection{TargetID: item.id, SourceID: "library", SkillIDs: ids, Mode: "additive", Revision: item.revision, FollowPending: true}); err != nil {
			return err
		}
	}
	return nil
}

// SyncLibraryMachinesFor establishes one durable policy, including offline
// machines. Polls finish setup after the first complete scan, without a browser.
func (s *Store) SyncLibraryMachinesFor(uid int64, role, origin string) error {
	return s.syncLibraryMachinesFor(uid, role, origin, "")
}

// SyncLibraryMachineFor retries one destination without changing other
// enrollments, custom plans, or private-copy requests.
func (s *Store) SyncLibraryMachineFor(uid int64, role, origin, agentID string) error {
	if agentID == "" {
		return errors.New("choose an enrolled machine")
	}
	return s.syncLibraryMachinesFor(uid, role, origin, agentID)
}

func (s *Store) syncLibraryMachinesFor(uid int64, role, origin, agentID string) error {
	if role == "viewer" {
		return errors.New("read-only account")
	}
	agents, err := s.SyncAgentsFor(uid)
	if err != nil {
		return err
	}
	if agentID != "" {
		selected := []SyncAgent{}
		for _, agent := range agents {
			if agent.ID == agentID {
				selected = append(selected, agent)
			}
		}
		if len(selected) == 0 {
			return ErrNotFound
		}
		agents = selected
	}
	if err := s.SetSyncLibraryOrigin(uid, origin); err != nil {
		return err
	}
	candidates, err := s.SyncLibraryFor(uid, role)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if candidate.Problem != "" {
			return errors.New(candidate.Problem)
		}
	}
	// Explicit retry releases terminal capture keys, keeping their audit trail.
	// Polls alone never create a stream of retries for a broken source.
	if err := s.withTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT r.data FROM machine_import_requests r WHERE r.user_id=? AND r.request_key LIKE 'library_%' AND r.state IN ('failed','expired','cancelled') AND (?='' OR json_extract(r.data,'$.destinationId') IN (SELECT e.id FROM environments e JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE WHERE a.id=? AND a.user_id=?))`, uid, agentID, agentID, uid)
		if err != nil {
			return err
		}
		var requests []RemoteImport
		for rows.Next() {
			var raw string
			if err = rows.Scan(&raw); err != nil {
				break
			}
			var request RemoteImport
			if err = json.Unmarshal([]byte(raw), &request); err != nil {
				break
			}
			requests = append(requests, request)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		for _, request := range requests {
			request.RequestKey = "retried_" + request.ID
			if _, err = tx.Exec(`UPDATE machine_import_requests SET request_key=? WHERE id=? AND user_id=?`, request.RequestKey, request.ID, uid); err != nil {
				return err
			}
			if err = writeImport(tx, uid, request); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, agent := range agents {
		if err = s.ControlSyncAgent(uid, agent.ID, "keep-sync-on"); err != nil {
			return err
		}
		agent.KeepSynced = true
		if agent.Paused {
			continue
		}
		if err = s.prepareLibraryTargets(uid, role, agent, true); err != nil {
			return err
		}
		if err = s.MaintainSync(uid, role, agent); err != nil {
			return err
		}
	}
	return nil
}
