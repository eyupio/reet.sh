package store

import (
	"sort"
	"strings"
)

// RollupMember is an active account with its team-wide totals.
type RollupMember struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Used      int    `json:"used"`
	Installed int    `json:"installed"`
}

// RollupMemberUse is one member's use of one skill.
type RollupMemberUse struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Used         bool     `json:"used"`
	Environments []string `json:"environments,omitempty"`
}

// RollupSkill is one skill across the team.
type RollupSkill struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Source           string            `json:"source"`
	Tags             []string          `json:"tags,omitempty"`
	Lists            []ListRef         `json:"lists,omitempty"`
	UsedBy           int               `json:"usedBy"`
	OnShelf          int               `json:"onShelf"`
	InstalledBy      int               `json:"installedBy"`
	EnvironmentLinks int               `json:"environmentLinks"`
	Members          []RollupMemberUse `json:"members,omitempty"`
}

// RollupUnused is a skill on a list that nobody has marked in use.
type RollupUnused struct {
	ListID   int64  `json:"listId"`
	ListName string `json:"listName"`
	SkillID  string `json:"skillId"`
	Name     string `json:"name"`
	Source   string `json:"source"`
}

// RollupDrift names the gaps between what is used, shelved and listed.
type RollupDrift struct {
	Unlisted []string       `json:"unlisted"`
	Unused   []RollupUnused `json:"unused"`
}

// Rollup is the team view: adoption by skill, member totals and list drift.
type Rollup struct {
	GeneratedAt int64          `json:"generatedAt"`
	Individual  bool           `json:"individual"`
	Members     []RollupMember `json:"members"`
	Lists       []ListRef      `json:"lists"`
	Skills      []RollupSkill  `json:"skills"`
	Drift       RollupDrift    `json:"drift"`
}

// RollupFor builds the team rollup. includeIndividual leaves member names and
// environment chips on each skill; counts are always included.
func (s *Store) RollupFor(includeIndividual bool) (Rollup, error) {
	out := Rollup{
		GeneratedAt: now(),
		Individual:  includeIndividual,
		Members:     []RollupMember{},
		Lists:       []ListRef{},
		Skills:      []RollupSkill{},
		Drift:       RollupDrift{Unlisted: []string{}, Unused: []RollupUnused{}},
	}

	memberRows, err := s.db.Query(`SELECT id,name,email,role FROM users WHERE disabled_at=0 ORDER BY lower(name), lower(email)`)
	if err != nil {
		return out, err
	}
	for memberRows.Next() {
		var m RollupMember
		if err := memberRows.Scan(&m.ID, &m.Name, &m.Email, &m.Role); err != nil {
			memberRows.Close()
			return out, err
		}
		out.Members = append(out.Members, m)
	}
	if err := memberRows.Err(); err != nil {
		memberRows.Close()
		return out, err
	}
	memberRows.Close()

	listRows, err := s.db.Query(`SELECT id,name FROM lists ORDER BY lower(name), id`)
	if err != nil {
		return out, err
	}
	for listRows.Next() {
		var l ListRef
		if err := listRows.Scan(&l.ID, &l.Name); err != nil {
			listRows.Close()
			return out, err
		}
		out.Lists = append(out.Lists, l)
	}
	if err := listRows.Err(); err != nil {
		listRows.Close()
		return out, err
	}
	listRows.Close()

	shelfRows, err := s.db.Query(`SELECT sh.skill_id,sh.user_id,sh.used FROM shelf sh
		JOIN users u ON u.id=sh.user_id WHERE u.disabled_at=0`)
	if err != nil {
		return out, err
	}
	shelfPresent := map[string]map[int64]bool{}
	shelfUsed := map[string]map[int64]bool{}
	for shelfRows.Next() {
		var skillID string
		var userID int64
		var used int
		if err := shelfRows.Scan(&skillID, &userID, &used); err != nil {
			shelfRows.Close()
			return out, err
		}
		if shelfPresent[skillID] == nil {
			shelfPresent[skillID] = map[int64]bool{}
			shelfUsed[skillID] = map[int64]bool{}
		}
		shelfPresent[skillID][userID] = true
		if used != 0 {
			shelfUsed[skillID][userID] = true
		}
	}
	if err := shelfRows.Err(); err != nil {
		shelfRows.Close()
		return out, err
	}
	shelfRows.Close()

	envRows, err := s.db.Query(`SELECT se.skill_id,e.user_id,e.name FROM skill_envs se
		JOIN environments e ON e.id=se.env_id JOIN users u ON u.id=e.user_id WHERE u.disabled_at=0`)
	if err != nil {
		return out, err
	}
	envNames := map[string]map[int64]map[string]bool{}
	envLinks := map[string]int{}
	for envRows.Next() {
		var skillID, name string
		var userID int64
		if err := envRows.Scan(&skillID, &userID, &name); err != nil {
			envRows.Close()
			return out, err
		}
		envLinks[skillID]++
		if envNames[skillID] == nil {
			envNames[skillID] = map[int64]map[string]bool{}
		}
		if envNames[skillID][userID] == nil {
			envNames[skillID][userID] = map[string]bool{}
		}
		envNames[skillID][userID][name] = true
	}
	if err := envRows.Err(); err != nil {
		envRows.Close()
		return out, err
	}
	envRows.Close()

	tagRows, err := s.db.Query(`SELECT st.skill_id,t.name FROM skill_tags st
		JOIN tags t ON t.id=st.tag_id JOIN users u ON u.id=st.user_id
		WHERE u.disabled_at=0 AND st.hidden=0`)
	if err != nil {
		return out, err
	}
	tagsBySkill := map[string]map[string]bool{}
	for tagRows.Next() {
		var skillID, name string
		if err := tagRows.Scan(&skillID, &name); err != nil {
			tagRows.Close()
			return out, err
		}
		if tagsBySkill[skillID] == nil {
			tagsBySkill[skillID] = map[string]bool{}
		}
		tagsBySkill[skillID][name] = true
	}
	if err := tagRows.Err(); err != nil {
		tagRows.Close()
		return out, err
	}
	tagRows.Close()

	listSkillRows, err := s.db.Query(`SELECT ls.skill_id,l.id,l.name FROM list_skills ls
		JOIN lists l ON l.id=ls.list_id ORDER BY lower(l.name), l.id`)
	if err != nil {
		return out, err
	}
	listsBySkill := map[string][]ListRef{}
	for listSkillRows.Next() {
		var skillID string
		var list ListRef
		if err := listSkillRows.Scan(&skillID, &list.ID, &list.Name); err != nil {
			listSkillRows.Close()
			return out, err
		}
		listsBySkill[skillID] = append(listsBySkill[skillID], list)
	}
	if err := listSkillRows.Err(); err != nil {
		listSkillRows.Close()
		return out, err
	}
	listSkillRows.Close()

	skillRows, err := s.db.Query(`SELECT s.id,s.name,s.source FROM skills s
		WHERE EXISTS (SELECT 1 FROM shelf sh JOIN users u ON u.id=sh.user_id
			WHERE sh.skill_id=s.id AND u.disabled_at=0)
		OR EXISTS (SELECT 1 FROM list_skills ls WHERE ls.skill_id=s.id)
		OR EXISTS (SELECT 1 FROM skill_envs se JOIN environments e ON e.id=se.env_id
			JOIN users u ON u.id=e.user_id WHERE se.skill_id=s.id AND u.disabled_at=0)
		ORDER BY lower(s.name), s.id`)
	if err != nil {
		return out, err
	}
	memberIndex := map[int64]int{}
	for i, m := range out.Members {
		memberIndex[m.ID] = i
	}
	memberUsed := make([]map[string]bool, len(out.Members))
	memberInstalled := make([]map[string]bool, len(out.Members))
	for i := range out.Members {
		memberUsed[i] = map[string]bool{}
		memberInstalled[i] = map[string]bool{}
	}
	for skillRows.Next() {
		var sk RollupSkill
		if err := skillRows.Scan(&sk.ID, &sk.Name, &sk.Source); err != nil {
			skillRows.Close()
			return out, err
		}
		sk.Lists = append(sk.Lists, listsBySkill[sk.ID]...)
		sk.EnvironmentLinks = envLinks[sk.ID]
		if names := tagsBySkill[sk.ID]; len(names) > 0 {
			for name := range names {
				sk.Tags = append(sk.Tags, name)
			}
			sort.Slice(sk.Tags, func(i, j int) bool { return strings.ToLower(sk.Tags[i]) < strings.ToLower(sk.Tags[j]) })
		}
		for _, m := range out.Members {
			idx := memberIndex[m.ID]
			used := shelfUsed[sk.ID][m.ID]
			envs := []string{}
			for name := range envNames[sk.ID][m.ID] {
				envs = append(envs, name)
			}
			sort.Slice(envs, func(i, j int) bool { return strings.ToLower(envs[i]) < strings.ToLower(envs[j]) })
			if shelfPresent[sk.ID][m.ID] {
				sk.OnShelf++
			}
			if used {
				sk.UsedBy++
				memberUsed[idx][sk.ID] = true
			}
			if len(envs) > 0 {
				sk.InstalledBy++
				memberInstalled[idx][sk.ID] = true
			}
			if includeIndividual {
				sk.Members = append(sk.Members, RollupMemberUse{ID: m.ID, Name: m.Name, Used: used, Environments: envs})
			}
		}
		if sk.UsedBy > 0 && len(sk.Lists) == 0 {
			out.Drift.Unlisted = append(out.Drift.Unlisted, sk.ID)
		}
		if sk.UsedBy == 0 {
			for _, list := range sk.Lists {
				out.Drift.Unused = append(out.Drift.Unused, RollupUnused{
					ListID: list.ID, ListName: list.Name, SkillID: sk.ID, Name: sk.Name, Source: sk.Source,
				})
			}
		}
		out.Skills = append(out.Skills, sk)
	}
	if err := skillRows.Err(); err != nil {
		skillRows.Close()
		return out, err
	}
	skillRows.Close()

	for i := range out.Members {
		out.Members[i].Used = len(memberUsed[i])
		out.Members[i].Installed = len(memberInstalled[i])
	}
	return out, nil
}
