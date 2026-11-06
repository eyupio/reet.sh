package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SyncSourceFix is one desired skill whose source will follow the copy already
// installed on the machine, so a shared agent folder is not overwritten.
type SyncSourceFix struct {
	TargetID string `json:"targetId"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	From     string `json:"from"`
	To       string `json:"to"`
	Agent    string `json:"agent"`
}

// canonicalSyncSource compares two reported sources the way the enrolled agent
// does, so "https://github.com/o/r.git" and "github.com/o/r" are one source.
func canonicalSyncSource(source string) string {
	source = strings.TrimPrefix(strings.TrimPrefix(source, "https://"), "http://")
	source = strings.TrimPrefix(source, "github.com/")
	return strings.TrimSuffix(strings.TrimRight(source, "/"), ".git")
}

// resolveSyncSourceFixes finds desired skills that disagree with a copy another
// agent holds on the same machine and project. When every installed copy of the
// name agrees on one source, the plan adopts it: that is the copy sync refuses
// to overwrite, so following it is the non-destructive resolution. More than one
// installed source is genuinely ambiguous and is left for a person.
func resolveSyncSourceFixes(target SyncTarget, envs []SyncEnvironment) ([]SyncSourceFix, []SyncSkill, error) {
	type copyRef struct {
		skill SyncSkill
		agent string
	}
	installed := map[string][]copyRef{}
	for _, env := range envs {
		if !strings.EqualFold(env.Host, target.Target.Host) || env.Project != target.Target.Project {
			continue
		}
		for _, sk := range env.Skills {
			if sk.Source == "" || sk.SourceType == "local" || sk.SourceType == "private" {
				continue
			}
			installed[sk.Slug] = append(installed[sk.Slug], copyRef{skill: sk, agent: env.Agent})
		}
	}
	fixes := []SyncSourceFix{}
	desired := make([]SyncSkill, len(target.Desired))
	copy(desired, target.Desired)
	changed := map[string]bool{}
	for i := range desired {
		sk := &desired[i]
		if sk.Source == "" || sk.SourceType == "local" || sk.SourceType == "private" || changed[sk.Slug] {
			continue
		}
		copies := installed[sk.Slug]
		if len(copies) == 0 {
			continue
		}
		bySource := map[string]copyRef{}
		for _, copy := range copies {
			key := canonicalSyncSource(copy.skill.Source)
			if _, ok := bySource[key]; !ok {
				bySource[key] = copy
			}
		}
		if len(bySource) > 1 {
			sources := make([]string, 0, len(bySource))
			for _, copy := range bySource {
				sources = append(sources, copy.skill.Source)
			}
			return nil, nil, fmt.Errorf("%s is installed from more than one source (%s); unlink one copy on the machine before syncing", sk.Name, strings.Join(sources, ", "))
		}
		for _, copy := range bySource {
			if canonicalSyncSource(copy.skill.Source) == canonicalSyncSource(sk.Source) {
				continue
			}
			fixes = append(fixes, SyncSourceFix{TargetID: target.Target.ID, ID: sk.ID, Name: sk.Name, Slug: sk.Slug, From: sk.Source, To: copy.skill.Source, Agent: copy.agent})
			sk.Source = copy.skill.Source
			sk.SourceType = copy.skill.SourceType
			sk.URL = copy.skill.URL
			changed[sk.Slug] = true
		}
	}
	if len(fixes) == 0 {
		return nil, nil, nil
	}
	return fixes, desired, nil
}

// ResolveSyncSourceConflictsFor adopts the installed source for each desired
// skill that a different agent already holds on the machine. A dry run reports
// the same changes and writes nothing.
func (s *Store) ResolveSyncSourceConflictsFor(uid int64, targetIDs []string, dryRun bool) ([]SyncSourceFix, error) {
	wanted := map[string]bool{}
	for _, id := range targetIDs {
		if id != "" {
			wanted[id] = true
		}
	}
	if len(wanted) == 0 {
		return nil, errors.New("choose at least one sync plan")
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		return nil, err
	}
	envs, err := s.SyncEnvironmentsFor(uid)
	if err != nil {
		return nil, err
	}
	fixes := []SyncSourceFix{}
	plans := map[string][]SyncSkill{}
	for _, target := range targets {
		if !wanted[target.Target.ID] {
			continue
		}
		targetFixes, desired, e := resolveSyncSourceFixes(target, envs)
		if e != nil {
			return nil, e
		}
		if len(targetFixes) == 0 {
			continue
		}
		fixes = append(fixes, targetFixes...)
		plans[target.Target.ID] = desired
	}
	if dryRun || len(fixes) == 0 {
		return fixes, nil
	}
	err = s.withTx(func(tx *sql.Tx) error {
		for id, desired := range plans {
			data, e := json.Marshal(desired)
			if e != nil {
				return e
			}
			actual, e := environmentSkills(tx, uid, id)
			if e != nil {
				return e
			}
			baseline, e := json.Marshal(actual)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(`UPDATE sync_targets SET desired=?,revision=?,updated_at=? WHERE user_id=? AND target_id=?`, string(data), syncID(), now(), uid, id); e != nil {
				return e
			}
			if _, e = tx.Exec(`INSERT INTO sync_options(target_id,mode,baseline) VALUES(?,'mirror',?) ON CONFLICT(target_id) DO UPDATE SET baseline=excluded.baseline`, id, string(baseline)); e != nil {
				return e
			}
			if _, e = tx.Exec(`UPDATE sync_policies SET problem='' WHERE target_id=?`, id); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fixes, nil
}
