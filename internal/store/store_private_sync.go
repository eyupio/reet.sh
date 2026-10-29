package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Private sync sources keep local skill identities intact. File bytes remain
// in account-owned immutable revisions; they never become public hosted skills.
func (s *Store) privateSyncCandidate(uid int64, sk SyncSkill) (SyncSkill, error) {
	if sk.Source != "" && sk.SourceType != "local" {
		return sk, nil
	}
	var bundle, revision, digest string
	err := s.db.QueryRow(`SELECT bundle_id,revision_id,digest FROM sync_private_sources WHERE user_id=? AND skill_id=?`, uid, sk.ID).Scan(&bundle, &revision, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return sk, nil
	}
	if err != nil {
		return sk, err
	}
	sk.SourceType = "private"
	sk.Source = "private:" + bundle + ":" + revision + ":" + digest
	return sk, nil
}

// RequestMachineSkillSync chooses only a scope that actually reported this
// owned skill. The explicit sync request authorises a private file transfer.
func (s *Store) RequestMachineSkillSync(uid int64, skillID, destination, sourceID, requestKey, destinationID string, followShelf bool) (RemoteImport, error) {
	sk, ok, err := s.GetFor(uid, skillID)
	if err != nil {
		return RemoteImport{}, err
	}
	if !ok {
		return RemoteImport{}, ErrNotFound
	}
	envs, err := s.SyncEnvironmentsFor(uid)
	if err != nil {
		return RemoteImport{}, err
	}
	agents, err := s.SyncAgentsFor(uid)
	if err != nil {
		return RemoteImport{}, err
	}
	var target *SyncEnvironment
	for i := range envs {
		e := &envs[i]
		if strings.EqualFold(e.Host, destination) && Syncable(e.Agent) && (destinationID == "" && e.Project == "" || e.ID == destinationID) {
			if target == nil || e.Agent == "agents" {
				target = e
			}
		}
	}
	if target == nil {
		return RemoteImport{}, errors.New("scan the destination machine so reet can find its skills folder")
	}
	var targetAgent *SyncAgent
	for i := range agents {
		if strings.EqualFold(agents[i].Host, target.Host) {
			targetAgent = &agents[i]
		}
	}
	if targetAgent == nil {
		return RemoteImport{}, errors.New("connect the destination reet agent first")
	}
	if targetAgent.Paused {
		return RemoteImport{}, errors.New("resume the destination machine first")
	}
	var source *SyncEnvironment
	for i := range envs {
		e := &envs[i]
		if sourceID != "" && sourceID != e.ID {
			continue
		}
		enrolled := false
		for _, a := range agents {
			if strings.EqualFold(a.Host, e.Host) && !a.Paused {
				enrolled = true
			}
		}
		if !enrolled {
			continue
		}
		for _, have := range e.Skills {
			if have.ID == skillID {
				if source == nil || e.Agent == "agents" {
					source = e
				}
			}
		}
	}
	if source == nil {
		return RemoteImport{}, errors.New("no other machine reports this skill; scan its source machine first")
	}
	var sourceAgent *SyncAgent
	for i := range agents {
		if strings.EqualFold(agents[i].Host, source.Host) {
			sourceAgent = &agents[i]
		}
	}
	if sourceAgent == nil {
		return RemoteImport{}, errors.New("connect reet on the source machine to transfer its skill")
	}
	return s.CreateRemoteImportFor(uid, RemoteImportCreate{FollowShelf: followShelf, RequestKey: requestKey, AgentID: sourceAgent.ID, EnvironmentID: source.ID, Name: sk.Slug, SyncSkillID: skillID, DestinationID: target.ID})
}

// AdvanceMachineSkillSyncs is driven by agent polls as well as browser reads.
// Closing the browser does not interrupt a requested source -> reet -> target transfer.
func (s *Store) AdvanceMachineSkillSyncs(uid int64, role string) error {
	rows, err := s.db.Query(`SELECT id FROM machine_import_requests WHERE user_id=? AND state='completed'`, uid)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		r, e := s.RemoteImportFor(uid, id)
		if e != nil {
			return e
		}
		if r.DestinationID == "" || r.SyncRunID != "" || r.Receipt == nil {
			continue
		}
		// A disconnected/stale destination must not prevent other machines
		// from polling. Keep its transfer durable and surface the exact fix.
		e = s.advanceMachineSkillSync(uid, role, r)
		if err := s.withTx(func(tx *sql.Tx) error {
			current, err := readImport(tx, uid, id)
			if err != nil {
				return err
			}
			current.SyncError = ""
			if e != nil {
				current.SyncError = e.Error()
			}
			return writeImport(tx, uid, current)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) advanceMachineSkillSync(uid int64, role string, r RemoteImport) error {
	id := r.ID
	var e error

	// Recover a committed run after a crash before its import receipt was
	// marked queued. Do not rewrite its desired revision or replay it.
	var existing string
	e = s.db.QueryRow(`SELECT id FROM sync_runs WHERE id=? AND user_id=?`, "transfer_"+r.ID, uid).Scan(&existing)
	if e == nil {
		if e = s.withTx(func(tx *sql.Tx) error {
			current, e := readImport(tx, uid, id)
			if e != nil {
				return e
			}
			current.SyncRunID = existing
			return writeImport(tx, uid, current)
		}); e != nil {
			return e
		}
		return nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}

	// Serialize destination work, including reviews. Updating a desired
	// revision while a prior transfer applies would invalidate its plan.
	var busy int
	e = s.db.QueryRow(`SELECT count(*) FROM sync_jobs j JOIN sync_agents a ON a.id=j.agent_id JOIN environments e ON e.user_id=a.user_id AND e.host=a.host COLLATE NOCASE WHERE e.id=? AND a.user_id=? AND j.status IN ('queued','running','awaiting-approval')`, r.DestinationID, uid).Scan(&busy)
	if e != nil {
		return e
	}
	if busy > 0 {
		return nil
	}
	// Use the latest revision as a concurrency guard; preserve an existing
	// machine's selected skills and removal mode instead of replacing its set.
	targets, e := s.SyncTargetsFor(uid)
	if e != nil {
		return e
	}
	wantFollow := r.FollowShelf
	in := DesiredSelection{TargetID: r.DestinationID, SourceID: "shelf", Mode: "additive", SkillIDs: []string{r.SyncSkillID}}
	for _, target := range targets {
		if target.Target.ID == r.DestinationID {
			in.Revision = target.Revision
			in.Mode = target.Mode
			in.SourceID = target.SourceID
			if target.SourceID == "shelf" && target.FollowSource {
				wantFollow = true
			}
			for _, sk := range target.Desired {
				if sk.ID != r.SyncSkillID {
					in.SkillIDs = append(in.SkillIDs, sk.ID)
				}
			}
			if strings.HasPrefix(in.SourceID, "list:") {
				fmt.Sscanf(in.SourceID, "list:%d", &in.ListID)
				in.SourceID = "shelf"
			}
		}
	}
	if wantFollow && in.SourceID == "shelf" && in.ListID == 0 {
		candidates, e := s.SyncCandidatesFor(uid, role, 0)
		if e != nil {
			return e
		}
		in.FollowSource = true
		for _, sk := range candidates {
			if sk.Source == "" || sk.SourceType == "local" {
				in.FollowSource = false
				in.FollowPending = true
			}
		}
	}
	if e = s.SaveDesiredFor(uid, role, in); e != nil {
		return e
	}
	envs, e := s.SyncEnvironmentsFor(uid)
	if e != nil {
		return e
	}
	host := ""
	for _, env := range envs {
		if env.ID == r.DestinationID {
			host = env.Host
		}
	}
	agents, e := s.SyncAgentsFor(uid)
	if e != nil {
		return e
	}
	agentID := ""
	for _, a := range agents {
		if strings.EqualFold(a.Host, host) {
			agentID = a.ID
		}
	}
	run, e := s.startSyncRun(uid, agentID, []string{r.DestinationID}, "transfer_"+r.ID)
	if e != nil {
		return e
	}
	if e = s.withTx(func(tx *sql.Tx) error {
		current, e := readImport(tx, uid, id)
		if e != nil {
			return e
		}
		current.SyncRunID = run.ID
		return writeImport(tx, uid, current)
	}); e != nil {
		return e
	}
	return nil
}

func bindPrivateSync(tx *sql.Tx, uid int64, r RemoteImport, result PrivateSkillResult) error {
	if r.SyncSkillID == "" {
		return nil
	}
	_, err := tx.Exec(`INSERT INTO sync_private_sources(user_id,skill_id,bundle_id,revision_id,digest) VALUES(?,?,?,?,?) ON CONFLICT(user_id,skill_id) DO UPDATE SET bundle_id=excluded.bundle_id,revision_id=excluded.revision_id,digest=excluded.digest`, uid, r.SyncSkillID, result.Bundle.ID, result.Revision.ID, result.Revision.ManifestDigest)
	return err
}

func (s *Store) validateSyncImport(tx *sql.Tx, uid int64, in RemoteImportCreate) error {
	if in.DestinationID == "" && in.SyncSkillID == "" {
		if in.FollowShelf {
			return ErrRemoteImport
		}
		return nil
	}
	var count int
	err := tx.QueryRow(`SELECT count(*) FROM skill_envs se JOIN environments e ON e.id=se.env_id JOIN skills sk ON sk.id=se.skill_id WHERE se.skill_id=? AND e.id=? AND e.user_id=? AND sk.slug=?`, in.SyncSkillID, in.EnvironmentID, uid, in.Name).Scan(&count)
	if err != nil || count != 1 {
		return ErrRemoteImport
	}
	err = tx.QueryRow(`SELECT count(*) FROM environments e JOIN sync_agents a ON a.host=e.host COLLATE NOCASE AND a.user_id=e.user_id WHERE e.id=? AND e.user_id=? AND a.paused=0 AND e.agent IN ('agents','claude','cursor','opencode')`, in.DestinationID, uid).Scan(&count)
	if err != nil || count != 1 {
		return ErrRemoteImport
	}
	return nil
}
