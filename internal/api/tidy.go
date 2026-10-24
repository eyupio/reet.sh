package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
)

func uniqueStrings(values ...[]string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, group := range values {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func (s *server) deleteEnvironments(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID      string   `json:"id"`
		IDs     []string `json:"ids"`
		Force   bool     `json:"force"`
		Confirm bool     `json:"confirm"`
		DryRun  bool     `json:"dry_run"`
	}
	if !decode(w, r, &in) {
		return
	}
	ids := uniqueStrings([]string{in.ID}, in.IDs)
	if len(ids) == 0 || len(ids) > 500 {
		writeErrCode(w, 400, errInvalidArgument, "id", "choose between 1 and 500 environments", "Pass id or ids from reet_list_environments.")
		return
	}
	if in.DryRun {
		envs, err := s.st.ListEnvironmentsFor(s.uid(r))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		receipts := map[string]int{}
		if !in.Force {
			if receipts, err = s.st.ReceiptCountsFor(s.uid(r), ids); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
		index := map[string]store.Environment{}
		for _, env := range envs {
			index[env.ID] = env
		}
		would := []store.Environment{}
		blocked := []store.EnvironmentChange{}
		for _, id := range ids {
			env, ok := index[id]
			if !ok {
				blocked = append(blocked, store.EnvironmentChange{ID: id, Code: errEnvironmentNotFound, Message: "environment not found"})
				continue
			}
			if env.SkillCount > 0 && !in.Force {
				blocked = append(blocked, store.EnvironmentChange{ID: id, Name: env.Name, SkillCount: env.SkillCount, Code: errEnvironmentHasSkills, Message: "environment still has skills"})
				continue
			}
			if receipts[id] > 0 {
				blocked = append(blocked, store.EnvironmentChange{ID: id, Name: env.Name, Code: errEnvironmentHasReceipts, Message: "environment still holds ownership receipts; release them first"})
				continue
			}
			would = append(would, env)
		}
		writeJSON(w, 200, map[string]any{"dry_run": true, "would_delete": would, "blocked": blocked})
		return
	}
	if !in.Confirm {
		writeErrCode(w, 400, errConfirmRequired, "confirm", "set confirm=true to delete environments", "Run once with dry_run=true to preview, then repeat with confirm=true.")
		return
	}
	deleted, blocked, err := s.st.DeleteEnvironmentsFor(s.uid(r), ids, in.Force)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(deleted) == 0 && len(blocked) > 0 {
		first := blocked[0]
		hint := "Use force=true to remove the report links, or remove its skills first."
		switch first.Code {
		case errEnvironmentNotFound:
			hint = "Use IDs from reet_list_environments."
		case errEnvironmentHasReceipts:
			hint = "Release the ownership records first, or use force=true to forget them with the report."
		}
		writeJSON(w, 400, map[string]any{"error": first.Message, "code": first.Code, "message": first.Message, "hint": hint, "blocked": blocked})
		return
	}
	s.eventRequest(r, "environment-delete", "", map[string]any{"count": len(deleted), "ids": ids, "force": in.Force})
	writeJSON(w, 200, map[string]any{"dry_run": false, "deleted": len(deleted), "environments": deleted, "blocked": blocked})
}

func (s *server) mergeEnvironments(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FromID  string `json:"from_id"`
		ToID    string `json:"to_id"`
		From    string `json:"from"`
		To      string `json:"to"`
		Confirm bool   `json:"confirm"`
		DryRun  bool   `json:"dry_run"`
	}
	if !decode(w, r, &in) {
		return
	}
	from, to := strings.TrimSpace(in.FromID), strings.TrimSpace(in.ToID)
	if from == "" {
		from = strings.TrimSpace(in.From)
	}
	if to == "" {
		to = strings.TrimSpace(in.To)
	}
	if from == "" || to == "" || from == to {
		writeErrCode(w, 400, errInvalidArgument, "from_id", "choose two different environment IDs", "Use IDs from reet_list_environments.")
		return
	}
	if in.DryRun {
		envs, err := s.st.ListEnvironmentsFor(s.uid(r))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		var source, target *store.Environment
		for i := range envs {
			if envs[i].ID == from {
				source = &envs[i]
			}
			if envs[i].ID == to {
				target = &envs[i]
			}
		}
		if source == nil || target == nil {
			writeErrCode(w, 404, errEnvironmentNotFound, "from_id", "environment not found", "Use IDs from reet_list_environments.")
			return
		}
		// The preview answers a refusal the confirmed merge would, in its order: pins, then receipts.
		pins, err := s.st.PinCountsFor(s.uid(r), []string{from})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if pins[from] > 0 {
			writeEnvironmentMergeError(w, store.ErrSkillContentTargetScopeMerge)
			return
		}
		receipts, err := s.st.ReceiptCountsFor(s.uid(r), []string{from})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if receipts[from] > 0 {
			writeEnvironmentMergeError(w, store.ErrSkillContentReceiptScope)
			return
		}
		writeJSON(w, 200, map[string]any{"dry_run": true, "from": source, "to": target, "skills_moved": source.SkillCount})
		return
	}
	if !in.Confirm {
		writeErrCode(w, 400, errConfirmRequired, "confirm", "set confirm=true to merge environments", "Run once with dry_run=true to preview, then repeat with confirm=true.")
		return
	}
	moved, err := s.st.MergeEnvironmentsFor(s.uid(r), from, to)
	if err != nil {
		writeEnvironmentMergeError(w, err)
		return
	}
	s.eventRequest(r, "environment-merge", "", map[string]any{"from": from, "to": to, "skills_moved": moved})
	writeJSON(w, 200, map[string]any{"dry_run": false, "from": from, "to": to, "skills_moved": moved})
}

// writeEnvironmentMergeError answers what MergeEnvironmentsFor refuses, for the
// confirmed merge and for its preview alike.
func writeEnvironmentMergeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrSkillContentTargetScopeMerge):
		writeErrCode(w, 409, errEnvironmentHasPins, "from_id", err.Error(), "Remove the private skill pin first.")
	case errors.Is(err, store.ErrSkillContentReceiptScope):
		writeErrCode(w, 409, errEnvironmentHasReceipts, "from_id", err.Error(), "Release the ownership record first.")
	case strings.Contains(strings.ToLower(err.Error()), "not found"):
		writeErrCode(w, 404, errEnvironmentNotFound, "from_id", err.Error(), "Use IDs from reet_list_environments.")
	default:
		writeErr(w, 500, err.Error())
	}
}

type listResolveError struct {
	code, field, message, hint string
}

func (e *listResolveError) Error() string { return e.message }

type listSkillInput struct {
	ID    string `json:"id"`
	Input string `json:"input"`
	Note  string `json:"note"`
}

func (s *server) resolveListSkill(r *http.Request, list store.SharedList, in listSkillInput) (store.Skill, *listResolveError) {
	value := strings.TrimSpace(in.ID)
	if value == "" {
		value = strings.TrimSpace(in.Input)
	}
	if value == "" {
		return store.Skill{}, &listResolveError{errInvalidArgument, "id", "choose a skill or paste a reference", "Pass id for a shelf skill or input for owner/repo/skill or a skills.sh URL."}
	}
	sk, matches, err := s.st.MatchShelfSkillFor(s.uid(r), value)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.Skill{}, &listResolveError{"internal_error", "", err.Error(), ""}
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			ids = append(ids, match.ID)
		}
		return store.Skill{}, &listResolveError{errAmbiguousSkillID, "id", "more than one skill matches that name", "Use one exact ID: " + strings.Join(ids, ", ")}
	}
	if err == nil {
		if sk.SourceType == "local" && list.Visibility != "personal" {
			return store.Skill{}, &listResolveError{errLocalSkillInList, "id", "local skills can only be added to personal lists", "Use a personal list, or link the local skill to its upstream source with reet_resolve first."}
		}
		return sk, nil
	}
	input := strings.TrimSpace(in.Input)
	if input == "" || value != input {
		return store.Skill{}, &listResolveError{errSkillNotFound, "id", "skill not found on this account's shelf", "Use an exact ID from reet_list_skills."}
	}
	rf, err := s.parseSkillInput(r, input)
	if err != nil {
		return store.Skill{}, &listResolveError{errSkillNotFound, "input", "could not resolve that skill reference", "Use an exact shelf ID, owner/repo/skill, or a skills.sh URL."}
	}
	if err := s.st.EnsureSkill(rf); err != nil {
		return store.Skill{}, &listResolveError{"internal_error", "input", err.Error(), ""}
	}
	s.enqueue(rf)
	sk, ok, err := s.st.Get(rf.ID)
	if err != nil {
		return store.Skill{}, &listResolveError{"internal_error", "input", err.Error(), ""}
	}
	if !ok {
		return store.Skill{}, &listResolveError{errSkillNotFound, "input", "skill not found", "Use an exact shelf ID or a valid skills.sh reference."}
	}
	return sk, nil
}

func (s *server) addListSkills(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanEdit {
		writeErr(w, 403, "you can read this list but not change it")
		return
	}
	var in struct {
		Inputs []listSkillInput `json:"inputs"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Inputs) == 0 || len(in.Inputs) > 100 {
		writeErrCode(w, 400, errInvalidArgument, "inputs", "choose between 1 and 100 skills", "Pass inputs from reet_list_skills or valid references.")
		return
	}
	results := make([]map[string]any, 0, len(in.Inputs))
	added := 0
	for _, item := range in.Inputs {
		sk, resolveErr := s.resolveListSkill(r, access.List, item)
		if resolveErr != nil {
			results = append(results, map[string]any{"ok": false, "id": item.ID, "input": item.Input, "code": resolveErr.code, "message": resolveErr.message})
			continue
		}
		if err := s.st.AddListSkill(access.List.ID, sk.ID, item.Note, s.uid(r)); err != nil {
			results = append(results, map[string]any{"ok": false, "id": sk.ID, "code": "list_add_failed", "message": err.Error()})
			continue
		}
		added++
		results = append(results, map[string]any{"ok": true, "id": sk.ID, "name": sk.Name})
	}
	s.eventRequest(r, "list-skill-bulk-add", "", map[string]any{"list": access.List.Name, "id": access.List.ID, "added": added, "failed": len(in.Inputs) - added})
	list := access.List
	if fresh, err := s.st.SharedListByID(access.List.ID); err == nil {
		list = fresh
	}
	writeJSON(w, 200, map[string]any{"added": added, "failed": len(in.Inputs) - added, "results": results, "list": list})
}

func (s *server) deleteListConfirmed(w http.ResponseWriter, r *http.Request) {
	access, ok := s.listAccess(w, r)
	if !ok {
		return
	}
	if !access.CanManage {
		writeErr(w, 403, "the list owner or an admin can delete it")
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
		DryRun  bool `json:"dry_run"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.DryRun {
		writeJSON(w, 200, map[string]any{"dry_run": true, "list": access.List})
		return
	}
	if !in.Confirm {
		writeErrCode(w, 400, errConfirmRequired, "confirm", "set confirm=true to delete the list", "Run once with dry_run=true to preview, then repeat with confirm=true.")
		return
	}
	if err := s.st.DeleteList(access.List.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "list-delete", "", map[string]any{"list": access.List.Name, "id": access.List.ID})
	writeJSON(w, 200, map[string]any{"deleted": true, "dry_run": false, "list": access.List})
}

func (s *server) createTag(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	tag, err := s.st.CreateTag(in.Name)
	if err != nil {
		writeErrCode(w, 400, errInvalidArgument, "name", err.Error(), "Use 1 to 100 characters.")
		return
	}
	s.eventRequest(r, "tag-create", "", map[string]any{"id": tag.ID, "name": tag.Name})
	writeJSON(w, 200, tag)
}

func (s *server) renameTagScoped(w http.ResponseWriter, r *http.Request) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErrCode(w, 404, errTagNotFound, "id", "tag not found", "Use tag IDs from reet_list_tags.")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	affected, err := s.st.TagUsageFor(s.uid(r), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErrCode(w, 404, errTagNotFound, "id", "tag not found", "Use tag IDs from reet_list_tags.")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if role := s.role(r); role == "owner" || role == "admin" {
		err = s.st.RenameTag(id, in.Name)
	} else {
		affected, err = s.st.RenameTagFor(s.uid(r), id, in.Name)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeErrCode(w, 404, errTagNotFound, "id", "tag not found", "Use tag IDs from reet_list_tags.")
		return
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeErrCode(w, 409, errConflict, "name", "that tag already exists", "Merge the tags or choose another name.")
			return
		}
		writeErrCode(w, 400, errInvalidArgument, "name", err.Error(), "Use 1 to 100 characters.")
		return
	}
	s.eventRequest(r, "tag-rename", "", map[string]any{"id": id, "name": strings.TrimSpace(in.Name), "affected": affected})
	writeJSON(w, 200, map[string]any{"id": id, "name": strings.TrimSpace(in.Name), "affected": affected})
}

func (s *server) deleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErrCode(w, 404, errTagNotFound, "id", "tag not found", "Use tag IDs from reet_list_tags.")
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
		DryRun  bool `json:"dry_run"`
	}
	if !decode(w, r, &in) {
		return
	}
	affected, err := s.st.TagUsageFor(s.uid(r), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErrCode(w, 404, errTagNotFound, "id", "tag not found", "Use tag IDs from reet_list_tags.")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if in.DryRun {
		writeJSON(w, 200, map[string]any{"dry_run": true, "id": id, "affected": affected})
		return
	}
	if !in.Confirm {
		writeErrCode(w, 400, errConfirmRequired, "confirm", "set confirm=true to delete the tag", "Run once with dry_run=true to preview, then repeat with confirm=true.")
		return
	}
	affected, err = s.st.DeleteTagFor(s.uid(r), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.eventRequest(r, "tag-delete", "", map[string]any{"id": id, "affected": affected})
	writeJSON(w, 200, map[string]any{"deleted": true, "dry_run": false, "id": id, "affected": affected})
}

func (s *server) bulkUpdateSkills(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Filter struct {
			Q          string  `json:"q"`
			SourceType string  `json:"source_type"`
			Env        string  `json:"env"`
			Stage      string  `json:"stage"`
			Sort       string  `json:"sort"`
			Tags       []int64 `json:"tags"`
			Untagged   bool    `json:"untagged"`
		} `json:"filter"`
		AddTags      []string `json:"add_tags"`
		RemoveTagIDs []int64  `json:"remove_tag_ids"`
		Stage        string   `json:"stage"`
		Liked        *bool    `json:"liked"`
		Bookmarked   *bool    `json:"bookmarked"`
		AddToListID  int64    `json:"add_to_list_id"`
		DryRun       bool     `json:"dry_run"`
	}
	if !decode(w, r, &in) {
		return
	}
	ids := uniqueStrings(in.IDs)
	filtered := in.Filter.Q != "" || in.Filter.SourceType != "" || in.Filter.Env != "" || in.Filter.Stage != "" || len(in.Filter.Tags) > 0 || in.Filter.Untagged
	if len(ids) == 0 && !filtered {
		writeErrCode(w, 400, errInvalidArgument, "ids", "supply ids or a filter", "Use ids from reet_list_skills, or filter by q, source_type, env, stage, tags or untagged.")
		return
	}
	if len(ids) > 500 {
		writeErrCode(w, 400, errInvalidArgument, "ids", "choose at most 500 skills", "Narrow the filter or split the work.")
		return
	}
	if len(ids) == 0 {
		skills, _, err := s.st.ListFor(s.uid(r), store.Filter{
			Query: in.Filter.Q, SourceType: in.Filter.SourceType, Env: in.Filter.Env,
			Stage: in.Filter.Stage, Sort: in.Filter.Sort, Tags: in.Filter.Tags, Untagged: in.Filter.Untagged,
		})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		for _, sk := range skills {
			ids = append(ids, sk.ID)
		}
		if len(ids) > 500 {
			writeErrCode(w, 400, errInvalidArgument, "filter", "the filter matches more than 500 skills", "Narrow the filter or send explicit ids in batches.")
			return
		}
	}
	var change store.BulkChange
	change.Liked = in.Liked
	change.Bookmarked = in.Bookmarked
	change.AddTags = in.AddTags
	change.RemoveTagIDs = in.RemoveTagIDs
	change.AddToListID = in.AddToListID
	if in.Stage != "" {
		switch in.Stage {
		case "shelf":
			used, bookmarked := false, false
			change.Used, change.Bookmarked = &used, &bookmarked
		case "try":
			used, bookmarked := false, true
			change.Used, change.Bookmarked = &used, &bookmarked
		case "use":
			used := true
			change.Used = &used
		default:
			writeErrCode(w, 400, errInvalidArgument, "stage", "stage must be shelf, try or use", "Use shelf, try or use.")
			return
		}
	}
	if change.AddToListID > 0 {
		access, err := s.st.ListAccessFor(s.uid(r), s.role(r), change.AddToListID)
		if errors.Is(err, store.ErrNotFound) {
			writeErrCode(w, 404, errListNotFound, "add_to_list_id", "list not found", "Use a list ID from reet_list_lists.")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if !access.CanEdit {
			writeErr(w, 403, "you can read this list but not change it")
			return
		}
	}
	updated, results, err := s.st.BulkUpdateFor(s.uid(r), ids, change, in.DryRun)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	failed := 0
	for _, result := range results {
		if !result.OK {
			failed++
		}
	}
	if !in.DryRun {
		s.eventRequest(r, "bulk-update", "", map[string]any{"count": updated, "failed": failed, "add_tags": in.AddTags, "remove_tag_ids": in.RemoveTagIDs, "list": in.AddToListID})
	}
	writeJSON(w, 200, map[string]any{"dry_run": in.DryRun, "updated": updated, "failed": failed, "results": results})
}

func (s *server) findDuplicates(w http.ResponseWriter, r *http.Request) {
	groups, err := s.st.FindDuplicatesFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	total := 0
	for _, group := range groups {
		total += len(group.Skills)
	}
	writeJSON(w, 200, map[string]any{"groups": groups, "total_groups": len(groups), "total_skills": total})
}
