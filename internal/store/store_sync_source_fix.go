package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/eyupio/reet.sh/internal/ref"
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
	return resolveSyncSourceFixesFor(target, envs, false)
}

// resolveSyncSourceFixesFor is resolveSyncSourceFixes with a choice about the
// target's own scope. Automatic repair only follows a copy another agent holds,
// because a differing copy in the target's own scope is what a normal sync is
// meant to replace with the selected source.
func resolveSyncSourceFixesFor(target SyncTarget, envs []SyncEnvironment, onlyOtherAgents bool) ([]SyncSourceFix, []SyncSkill, error) {
	type copyRef struct {
		skill SyncSkill
		agent string
	}
	installed := map[string][]copyRef{}
	for _, env := range envs {
		if !strings.EqualFold(env.Host, target.Target.Host) || env.Project != target.Target.Project {
			continue
		}
		if onlyOtherAgents && env.ID == target.Target.ID {
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
		if len(copies) == 0 && sk.Name != "" {
			// The skills CLI installs into a folder made from the skill's name,
			// which is not always its slug. A copy held under that folder is the
			// one sync will not overwrite.
			if folder := ref.InstalledFolder(sk.Name); folder != sk.Slug {
				copies = installed[folder]
			}
		}
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
			changed[sk.Slug] = true
			// The plan now asks for the installed skill itself, not the old one
			// with another source: once it is installed the two must be the same
			// skill, or it stays listed as missing beside an extra.
			*sk = copy.skill
			changed[sk.Slug] = true
		}
	}
	if len(fixes) == 0 {
		return nil, nil, nil
	}
	// A plan that already asked for the installed skill keeps one entry for it.
	seen := map[string]bool{}
	kept := desired[:0]
	for _, sk := range desired {
		if !seen[sk.ID] {
			seen[sk.ID] = true
			kept = append(kept, sk)
		}
	}
	return fixes, kept, nil
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
	if err = s.applySyncSourceFixes(uid, plans); err != nil {
		return nil, err
	}
	return fixes, nil
}

// applySyncSourceFixes saves the adopted sources. It changes the saved plan and
// its baseline, never files on a machine.
func (s *Store) applySyncSourceFixes(uid int64, plans map[string][]SyncSkill) error {
	return s.withTx(func(tx *sql.Tx) error {
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
}

// AutoResolveSyncSourceConflictsFor repairs one saved plan that an installed
// copy has blocked. It only follows a copy another agent holds and only when
// that name has exactly one source there; anything ambiguous is returned as an
// error so the caller can report why it was left alone.
func (s *Store) AutoResolveSyncSourceConflictsFor(uid int64, targetID string) ([]SyncSourceFix, error) {
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		return nil, err
	}
	envs, err := s.SyncEnvironmentsFor(uid)
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		if target.Target.ID != targetID || target.Canonical {
			continue
		}
		fixes, desired, err := resolveSyncSourceFixesFor(target, envs, true)
		if err != nil {
			return nil, err
		}
		if len(fixes) == 0 {
			return nil, nil
		}
		if err = s.applySyncSourceFixes(uid, map[string][]SyncSkill{targetID: desired}); err != nil {
			return nil, err
		}
		return fixes, nil
	}
	return nil, nil
}
