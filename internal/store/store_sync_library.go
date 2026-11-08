package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"sort"
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

// SyncLibraryView is the shared library, and the copies that were left out of it
// because another copy of the same name was chosen.
type SyncLibraryView struct {
	Skills     []SyncSkill          `json:"skills"`
	Duplicates []SyncDuplicateGroup `json:"duplicates"`
}

// SyncLibraryFor is the shared collection: shelf skills and every written skill,
// including unpublished drafts. Own written content is pinned privately, even
// when published, so an edit changes its revision and never depends on a public
// URL cache. An unlinked scan of the same name is not another library entry.
// Every name appears once: see libraryDuplicates.
func (s *Store) SyncLibraryFor(uid int64, role string) ([]SyncSkill, error) {
	view, err := s.SyncLibraryViewFor(uid, role)
	return view.Skills, err
}

// SyncLibraryViewFor is SyncLibraryFor plus the names that had more than one
// copy, for showing what was left out.
func (s *Store) SyncLibraryViewFor(uid int64, role string) (SyncLibraryView, error) {
	var origin string
	if err := s.db.QueryRow(`SELECT origin FROM sync_library WHERE user_id=?`, uid).Scan(&origin); err != nil {
		return SyncLibraryView{}, err
	}
	shelf, err := s.SyncCandidatesFor(uid, role, 0)
	if err != nil {
		return SyncLibraryView{}, err
	}
	written, err := s.SyncHostedFor(uid, origin)
	if err != nil {
		return SyncLibraryView{}, err
	}
	hosted, err := s.HostedSkillsFor(uid)
	if err != nil {
		return SyncLibraryView{}, err
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
	problems, err := s.libraryCaptureProblems(uid)
	if err != nil {
		return SyncLibraryView{}, err
	}
	linked := map[string]bool{}
	claims := map[string][]SyncSkill{}
	for _, sk := range byID {
		if sk.Source != "" && sk.SourceType != "local" {
			linked[sk.Slug] = true
			claims[sk.Slug] = append(claims[sk.Slug], sk)
		}
	}
	duplicates, err := s.libraryDuplicates(uid, claims)
	if err != nil {
		return SyncLibraryView{}, err
	}
	leftOut := map[string]bool{}
	for _, group := range duplicates {
		for _, sk := range group.Remove {
			leftOut[sk.ID] = true
		}
	}
	for id, problem := range problems {
		sk, ok := byID[id]
		// A machine-only copy is already replaced by the linked skill of the same
		// name, so its capture failure is not a problem for the library.
		if !ok || sk.Problem != "" || linked[sk.Slug] || (sk.Source != "" && sk.SourceType != "local") {
			continue
		}
		sk.Problem = problem
		byID[id] = sk
	}
	out := []SyncSkill{}
	for _, sk := range byID {
		if leftOut[sk.ID] || (sk.SourceType == "local" && sk.Problem == "" && linked[sk.Slug]) {
			continue
		}
		out = append(out, sk)
	}
	slices.SortFunc(out, func(a, b SyncSkill) int { return strings.Compare(a.ID, b.ID) })
	return SyncLibraryView{Skills: out, Duplicates: duplicates}, nil
}

// libraryDuplicates settles every name that more than one source claims. Two
// copies cannot share a skills folder, so a person has no useful decision to
// make: the library keeps the copy a saved plan already installs, then the oldest
// on the shelf, and leaves the others where they are. Nothing is deleted. A
// written skill with no shelf entry has no age and counts as the oldest, so a
// draft the account wrote outranks a shelf copy of its name. DedupeLibraryConflictsFor
// uses the same order, so removing the extras removes exactly the copies left out.
func (s *Store) libraryDuplicates(uid int64, claims map[string][]SyncSkill) ([]SyncDuplicateGroup, error) {
	groups := []SyncDuplicateGroup{}
	contested := []string{}
	for slug, members := range claims {
		sources := map[string]bool{}
		for _, sk := range members {
			sources[sk.SourceType+":"+sk.Source] = true
		}
		if len(sources) > 1 {
			contested = append(contested, slug)
		}
	}
	if len(contested) == 0 {
		return groups, nil
	}
	sort.Strings(contested)
	planned, err := s.desiredSkillCounts(uid)
	if err != nil {
		return nil, err
	}
	shelf, _, err := s.ListFor(uid, Filter{})
	if err != nil {
		return nil, err
	}
	created := map[string]int64{}
	for _, sk := range shelf {
		created[sk.ID] = sk.CreatedAt
	}
	for _, slug := range contested {
		members := slices.Clone(claims[slug])
		sort.SliceStable(members, func(i, j int) bool {
			a, b := members[i], members[j]
			if planned[a.ID] != planned[b.ID] {
				return planned[a.ID] > planned[b.ID]
			}
			if created[a.ID] != created[b.ID] {
				return created[a.ID] < created[b.ID]
			}
			return a.ID < b.ID
		})
		groups = append(groups, SyncDuplicateGroup{Slug: slug, Keep: members[0], Remove: members[1:]})
	}
	return groups, nil
}

// SyncDuplicateGroup is one name that more than one shelf source claims. Keep
// is the copy that stays; Remove is what deduping takes off the shelf.
type SyncDuplicateGroup struct {
	Slug   string      `json:"slug"`
	Keep   SyncSkill   `json:"keep"`
	Remove []SyncSkill `json:"remove"`
}

// DedupeLibraryConflictsFor removes the extra shelf entries that make the
// shared library ambiguous. It keeps the copy a saved plan already references,
// then the oldest copy, so the fix follows the plan rather than inventing one.
// A dry run reports the same groups and removes nothing.
func (s *Store) DedupeLibraryConflictsFor(uid int64, dryRun bool) ([]SyncDuplicateGroup, []string, error) {
	skills, _, err := s.ListFor(uid, Filter{})
	if err != nil {
		return nil, nil, err
	}
	desired, err := s.desiredSkillCounts(uid)
	if err != nil {
		return nil, nil, err
	}
	bySlug := map[string][]Skill{}
	slugs := []string{}
	for _, sk := range skills {
		if sk.Source == "" || sk.SourceType == "local" {
			continue
		}
		if _, ok := bySlug[sk.Slug]; !ok {
			slugs = append(slugs, sk.Slug)
		}
		bySlug[sk.Slug] = append(bySlug[sk.Slug], sk)
	}
	sort.Strings(slugs)
	groups := []SyncDuplicateGroup{}
	removed := []string{}
	for _, slug := range slugs {
		members := bySlug[slug]
		sources := map[string]bool{}
		for _, sk := range members {
			sources[sk.SourceType+":"+sk.Source] = true
		}
		if len(members) < 2 || len(sources) < 2 {
			continue
		}
		sort.SliceStable(members, func(i, j int) bool {
			if desired[members[i].ID] != desired[members[j].ID] {
				return desired[members[i].ID] > desired[members[j].ID]
			}
			if members[i].CreatedAt != members[j].CreatedAt {
				return members[i].CreatedAt < members[j].CreatedAt
			}
			return members[i].ID < members[j].ID
		})
		group := SyncDuplicateGroup{Slug: slug, Keep: asSync(members[0])}
		for _, sk := range members[1:] {
			group.Remove = append(group.Remove, asSync(sk))
			removed = append(removed, sk.ID)
		}
		groups = append(groups, group)
	}
	if dryRun || len(removed) == 0 {
		return groups, removed, nil
	}
	err = s.withTx(func(tx *sql.Tx) error {
		for _, id := range removed {
			if err := s.removeShelfTx(tx, uid, id, false); err != nil {
				return err
			}
		}
		// The dedupe just answered this exact complaint; a later poll recomputes
		// the policy problem from whatever is left on the shelf.
		_, err := tx.Exec(`UPDATE sync_policies SET problem='' WHERE problem LIKE '%More than one source uses the name%' AND target_id IN (SELECT id FROM environments WHERE user_id=?)`, uid)
		return err
	})
	return groups, removed, err
}

// desiredSkillCounts counts how many saved plans reference each skill, so a
// duplicate that a plan still points at is the one kept.
func (s *Store) desiredSkillCounts(uid int64) (map[string]int, error) {
	rows, err := s.db.Query(`SELECT desired FROM sync_targets WHERE user_id=?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var skills []SyncSkill
		if err := json.Unmarshal([]byte(raw), &skills); err != nil {
			return nil, err
		}
		for _, sk := range skills {
			counts[sk.ID]++
		}
	}
	return counts, rows.Err()
}

// libraryCaptureProblems explains machine-only skills whose private copy has
// already failed. A skill that cannot be captured is reported and skipped rather
// than blocking the rest of the library; an explicit retry releases its request
// key first, so these problems clear when the person asks for another attempt.
func (s *Store) libraryCaptureProblems(uid int64) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT json_extract(data,'$.syncSkillId'),json_extract(data,'$.error') FROM machine_import_requests WHERE user_id=? AND request_key LIKE 'library\_%' ESCAPE '\' AND state IN ('failed','expired','cancelled')`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	problems := map[string]string{}
	for rows.Next() {
		var id, code sql.NullString
		if err = rows.Scan(&id, &code); err != nil {
			return nil, err
		}
		if id.String != "" {
			problems[id.String] = privateCopyProblem(code.String)
		}
	}
	return problems, rows.Err()
}

func privateCopyProblem(code string) string {
	switch code {
	case "capture_failed":
		return "This skill folder could not be copied from its machine; it may contain unsupported files, or be larger than 256 files or 8 MiB."
	case "invalid_skill_manifest":
		return "SKILL.md needs valid matching metadata and instructions, so this skill cannot be copied."
	case "source_changed":
		return "The skill files changed while they were being copied. Retry to copy the current files."
	case "scope_unavailable":
		return "The source machine no longer has this skill folder."
	case "transfer_failed":
		return "This skill could not be saved privately. Private skill storage may be full."
	case "preview_expired":
		return "The private copy request expired before the files arrived. Retry to copy it."
	default:
		return "This skill could not be copied from its machine."
	}
}

// prepareLibraryTargets enrols newly scanned home folders in the shared library.
// Project folders remain explicit. Automatic sync owns home-folder plans;
// turning it off allows a custom plan. reset also retries settled failures.
func (s *Store) prepareLibraryTargets(uid int64, role string, agent SyncAgent, reset bool) error {
	if enabled, err := s.selectedLibraryEnabled(uid); err != nil {
		return err
	} else if enabled {
		return s.refreshSelectedLibrary(uid, role, agent)
	}
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
	// A queued binary update is not skill work. Counting it here left a machine
	// that was offline when an update was queued with no shared-library plan for
	// as long as that update waited.
	if err = s.db.QueryRow(`SELECT count(*) FROM sync_jobs WHERE user_id=? AND agent_id=? AND kind<>'update' AND status IN ('queued','running','awaiting-approval')`, uid, agent.ID).Scan(&pending); err != nil {
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
	// Explicit retry releases terminal capture keys, keeping their audit trail.
	// Polls alone never create a stream of retries for a broken source. This runs
	// before the candidates are read so a retry also clears the per-skill problem
	// that a previous capture failure put on the library.
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
	// A skill that cannot be synced is reported on that skill and left out. It
	// never stops this action: pressing Sync all machines switches every machine
	// on and brings across everything that can be brought.
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
