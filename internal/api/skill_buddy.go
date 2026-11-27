package api

import (
	"errors"
	"fmt"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"github.com/eyupio/reet.sh/internal/store"
	"net/http"
	"strings"
)

type buddyContext struct {
	SessionID string `json:"sessionId"`
	OutcomeID string `json:"outcomeId"`
	Pipeline  string `json:"pipeline"`
	Stage     int    `json:"stage"`
	Task      string `json:"task"`
	SkillID   string `json:"skillId"`
	Action    string `json:"action"`
	Revision  string `json:"revision"`
}

func (s *server) registerSkillBuddy(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/admin/system-skills", s.terminalAccess(s.requireAdmin(s.sameOriginRead(s.buddyAdminGet)), true))
	mux.HandleFunc("PUT /api/v2/admin/system-skills", s.terminalAccess(s.requireAdmin(s.buddyAdminSave), true))
	mux.HandleFunc("POST /api/v2/skill-buddy/recommendations", s.terminalAccess(s.buddyRecommendations, true))
	mux.HandleFunc("POST /api/v2/skill-buddy/choose", s.terminalAccess(s.buddyChoose, true))
	mux.HandleFunc("GET /api/v2/skill-buddy/insights", s.terminalAccess(s.sameOriginRead(s.buddyInsights), true))
}

func (s *server) buddyScope(uid int64, in buddyContext) (scope, stage, task string, editable bool, err error) {
	if len(in.Task) > 4000 || (in.SessionID == "") == (in.OutcomeID == "") {
		err = errors.New("Choose one session or Factory stage.")
		return
	}
	if in.SessionID != "" {
		var row store.TerminalSession
		row, err = s.st.TerminalSessionFor(uid, in.SessionID)
		if err != nil {
			return
		}
		scope = "session:" + row.ID
		task = row.Title + " " + in.Task
		editable = (row.State == "running" || row.State == "creating")
		// A Factory session uses its actual stage, not a guess from its title.
		outcomes, e := s.st.GafferOutcomesFor(uid)
		if e != nil {
			err = e
			return
		}
		for _, o := range outcomes {
			for _, p := range o.Sessions {
				for _, st := range p.Stages {
					if st.SessionID == row.ID {
						stage = st.Kind
						task += " " + p.Title
						if st.Kind == gaffer.CustomStage {
							stage, task = "", task+" "+st.Name+" "+st.Prompt
						}
						break
					}
				}
			}
		}
		return
	}
	var o store.GafferOutcome
	o, err = s.st.GafferOutcomeFor(uid, in.OutcomeID)
	if err != nil {
		return
	}
	for _, p := range o.Sessions {
		if p.Key == in.Pipeline && in.Stage >= 0 && in.Stage < len(p.Stages) {
			st := p.Stages[in.Stage]
			scope = fmt.Sprintf("factory:%s:%s:%d", o.ID, p.Key, in.Stage)
			stage = st.Kind
			task = p.Title + " " + p.Prompt + " " + st.Brief + " " + o.Objective
			// A pathway's own stage has no kind word to rank by; its name and prompt stand in (ranked locally only).
			if st.Kind == gaffer.CustomStage {
				stage, task = "", task+" "+st.Name+" "+st.Prompt
			}
			editable = o.ArchivedAt == 0 && st.State == "pending" && in.Stage > p.Stage
			return
		}
	}
	err = errors.New("That Factory stage is not available.")
	return
}

func (s *server) buddyCatalogue(uid int64) ([]skillbuddy.Skill, error) {
	out, _, err := s.buddySystem()
	if err != nil {
		return nil, err
	}
	items, err := s.st.LibraryFor(uid)
	if err != nil {
		return nil, err
	}
	ownedReferences := map[string]bool{}
	for _, it := range items {
		if it.Metadata != nil {
			ownedReferences[it.Metadata.ID] = true
		}
		if it.ItemType != store.LibrarySkill || it.Status != store.SkillActive {
			continue
		}
		desc := ""
		if it.Metadata != nil {
			desc = it.Metadata.Description
		}
		out = append(out, skillbuddy.Skill{ID: "library:" + it.ID, Name: it.Name, Description: desc, Source: "personal", Revision: it.RevisionID})
	}
	// Only explicitly public community metadata is searched; no private notes or files.
	_, community, err := s.st.Community()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, sk := range community {
		if seen[sk.ID] || ownedReferences[sk.ID] {
			continue
		}
		seen[sk.ID] = true
		out = append(out, skillbuddy.Skill{ID: "community:" + sk.ID, Name: sk.Name, Description: sk.Description, Source: "community", URL: "/community"})
	}
	return out, nil
}

// skillCannotLoad is a load refused for the skill itself (switched off, not
// Active, too large, no SKILL.md), as against a read that failed.
type skillCannotLoad string

func (e skillCannotLoad) Error() string { return string(e) }

// chatGuidanceBytes keeps room for the person's task in the session composer's 2,000 bytes.
const chatGuidanceBytes = 1500

// buddyLoadProblem says why a load failed without passing a storage error to the person.
func buddyLoadProblem(err error) (int, string) {
	var refused skillCannotLoad
	switch {
	case errors.As(err, &refused):
		return 400, refused.Error()
	case errors.Is(err, store.ErrPrivateSkillMissing):
		return 404, "That skill is no longer in your library."
	}
	return 500, "That skill could not be read."
}

func (s *server) buddyLoad(uid int64, id string) (skillbuddy.Skill, error) {
	system, _, err := s.buddySystem()
	if err != nil {
		return skillbuddy.Skill{}, err
	}
	for _, sk := range system {
		if sk.ID == id && !sk.Disabled && sk.Source == "builtin" {
			return sk, nil
		}
	}
	if !strings.HasPrefix(id, "library:") {
		return skillbuddy.Skill{}, skillCannotLoad("Add community skills to your library before using them.")
	}
	it, err := s.st.LibraryItemFor(uid, strings.TrimPrefix(id, "library:"))
	if err != nil {
		return skillbuddy.Skill{}, err
	}
	if it.Status != store.SkillActive || it.ItemType != store.LibrarySkill || it.RevisionID == "" {
		return skillbuddy.Skill{}, skillCannotLoad("This skill needs an Active saved copy in your library first.")
	}
	rev, err := s.st.PrivateSkillRevisionFor(uid, it.BundleID, it.RevisionID)
	if err != nil {
		return skillbuddy.Skill{}, err
	}
	for _, f := range rev.Files {
		if f.Path == "SKILL.md" {
			text := string(f.Data)
			if len(text) > 12000 {
				return skillbuddy.Skill{}, skillCannotLoad("This skill is too large for inline guidance. Use its installed version through Skills.")
			}
			return skillbuddy.Skill{ID: id, Name: it.Name, Source: "personal", Revision: rev.ID, Instructions: text}, nil
		}
	}
	return skillbuddy.Skill{}, skillCannotLoad("This saved skill has no SKILL.md.")
}

func (s *server) buddyRecommendations(w http.ResponseWriter, r *http.Request) {
	var in buddyContext
	if !terminalJSON(w, r, &in) {
		return
	}
	uid := s.uid(r)
	scope, stage, task, editable, err := s.buddyScope(uid, in)
	if err != nil {
		writeErr(w, 404, "That session or Factory stage is not available.")
		return
	}
	catalogue, err := s.buddyCatalogue(uid)
	if err != nil {
		writeErr(w, 500, "Skills could not be read.")
		return
	}
	recommendations := skillbuddy.Recommend(task, stage, catalogue)
	if err = s.st.RecordSkillBuddySuggestionsFor(uid, scope, in.SessionID, stage, recommendations); err != nil {
		writeErr(w, 500, "Suggestions could not be saved.")
		return
	}
	records, err := s.st.SkillBuddyRecordsFor(uid, scope)
	if err != nil {
		writeErr(w, 500, "Skill choices could not be read.")
		return
	}
	byID := map[string]store.SkillBuddyRecord{}
	for _, rec := range records {
		byID[rec.Skill.ID] = rec
	}
	for i := range recommendations {
		recommendations[i].Instructions = ""
		if rec, ok := byID[recommendations[i].ID]; ok {
			recommendations[i].State = rec.Skill.State
			delete(byID, recommendations[i].ID)
		}
	}
	// Keep selected guidance visible even after a task's wording changes.
	for _, rec := range records {
		if _, ok := byID[rec.Skill.ID]; ok && (rec.Skill.State == "selected" || rec.Skill.State == "drafted" || rec.Skill.State == "prepared") {
			sk := rec.Skill
			sk.Instructions = ""
			recommendations = append(recommendations, sk)
		}
	}
	writeJSON(w, 200, map[string]any{"skills": recommendations, "stage": stage, "editable": editable, "scope": scope})
}

func (s *server) buddyChoose(w http.ResponseWriter, r *http.Request) {
	var in buddyContext
	if !terminalJSON(w, r, &in) {
		return
	}
	uid := s.uid(r)
	scope, stage, _, editable, err := s.buddyScope(uid, in)
	if err != nil {
		writeErr(w, 404, "That session or Factory stage is not available.")
		return
	}
	if in.Action == "inspect" {
		records, e := s.st.SkillBuddyRecordsFor(uid, scope)
		if e != nil {
			writeErr(w, 500, "Skill choices could not be read.")
			return
		}
		for _, rec := range records {
			if rec.Skill.ID == in.SkillID && rec.Skill.Instructions != "" && (rec.Skill.State == "selected" || rec.Skill.State == "prepared" || rec.Skill.State == "drafted") {
				writeJSON(w, 200, map[string]any{"text": skillbuddy.Prompt(rec.Skill), "skill": rec.Skill})
				return
			}
		}
		sk, err := s.buddyLoad(uid, in.SkillID)
		if err != nil {
			code, problem := buddyLoadProblem(err)
			writeErr(w, code, problem)
			return
		}
		text := skillbuddy.Prompt(sk)
		writeJSON(w, 200, map[string]any{"text": text, "skill": sk, "tooLong": in.SessionID != "" && len(text) > chatGuidanceBytes})
		return
	}
	if !editable {
		if in.SessionID != "" {
			writeErr(w, 409, "This session is not running. Continue it before adding guidance.")
		} else {
			writeErr(w, 409, "This stage has already started. Open its session to add guidance.")
		}
		return
	}
	if in.Action != "drafted" && in.Action != "selected" && in.Action != "dismissed" {
		writeErr(w, 400, "Choose or dismiss a skill.")
		return
	}
	if in.OutcomeID != "" && in.Action == "drafted" || in.SessionID != "" && in.Action == "selected" {
		writeErr(w, 400, "Invalid skill action for this work.")
		return
	}
	sk, err := s.buddyLoad(uid, in.SkillID)
	if err != nil && in.Action == "dismissed" {
		records, e := s.st.SkillBuddyRecordsFor(uid, scope)
		if e != nil {
			writeErr(w, 500, "Skill choices could not be read.")
			return
		}
		for _, rec := range records {
			if rec.Skill.ID == in.SkillID {
				sk = rec.Skill
				err = nil
				break
			}
		}
	}
	if err != nil {
		code, problem := buddyLoadProblem(err)
		writeErr(w, code, problem)
		return
	}
	if in.Action != "dismissed" && in.Revision != "" && in.Revision != sk.Revision {
		writeErr(w, 409, "This skill changed. Review its current guidance before choosing it.")
		return
	}
	text := skillbuddy.Prompt(sk)
	if in.SessionID != "" && in.Action == "drafted" && len(text) > chatGuidanceBytes {
		writeErr(w, 400, "Use this skill through the session's Skills picker; its instructions are too long for the composer.")
		return
	}
	if in.OutcomeID != "" {
		o, e := s.st.GafferOutcomeFor(uid, in.OutcomeID)
		if e != nil {
			writeErr(w, 404, "Outcome unavailable.")
			return
		}
		changed := false
		_, e = s.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(next *store.GafferOutcome) bool {
			for i := range next.Sessions {
				p := &next.Sessions[i]
				if p.Key != in.Pipeline || in.Stage <= p.Stage || in.Stage >= len(p.Stages) {
					continue
				}
				st := &p.Stages[in.Stage]
				if st.State != "pending" {
					return false
				}
				guidance := []skillbuddy.Skill{}
				size := 0
				for _, old := range st.SkillGuidance {
					if old.ID != sk.ID {
						guidance = append(guidance, old)
						size += len(skillbuddy.Prompt(old))
					}
				}
				if in.Action == "selected" {
					if len(guidance) >= 2 || size+len(text) > gaffer.MaxStageGuidanceBytes {
						return false
					}
					guidance = append(guidance, sk)
				}
				st.SkillGuidance = guidance
				changed = true
				return true
			}
			return false
		})
		if e != nil || !changed {
			writeErr(w, 409, "The stage changed, or its guidance exceeds two skills / 6,000 bytes. Refresh and choose again.")
			return
		}
	}
	sk.State = in.Action
	if err = s.st.RecordSkillBuddyFor(uid, scope, in.SessionID, stage, sk); err != nil {
		writeErr(w, 500, "The skill receipt could not be saved. Refresh before trying again.")
		return
	}
	writeJSON(w, 200, map[string]any{"text": text, "skill": sk})
}

func (s *server) buddyInsights(w http.ResponseWriter, r *http.Request) {
	records, err := s.st.SkillBuddyRecordsFor(s.uid(r), "")
	if err != nil {
		writeErr(w, 500, "Skill insights could not be read.")
		return
	}
	usage := map[string]store.TerminalSessionUsage{}
	for i := range records {
		records[i].Skill.Instructions = ""
		if id := records[i].SessionID; id != "" {
			if _, exists := usage[id]; !exists {
				u, e := s.st.TerminalSessionUsageFor(s.uid(r), id)
				if e != nil {
					writeErr(w, 500, "Session usage could not be read.")
					return
				}
				usage[id] = u
			}
		}
	}
	writeJSON(w, 200, map[string]any{"records": records, "limit": 500, "retentionDays": 90, "sessionUsage": usage})
}
