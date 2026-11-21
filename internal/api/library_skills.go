package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

func stageLabel(sk store.Skill) string {
	switch sk.Status {
	case store.SkillActive:
		return "Active"
	case store.SkillArchived:
		return "Archived"
	}
	if sk.Used {
		return "In use"
	}
	if sk.Bookmarked {
		return "To test"
	}
	return "Parked"
}

func filterFromRequest(r *http.Request) store.Filter {
	q := r.URL.Query()
	f := store.Filter{
		Tab:     q.Get("tab"),
		Query:   q.Get("q"),
		Sort:    q.Get("sort"),
		Env:     q.Get("env"),
		Machine: q.Get("machine"),
		Only:    q.Get("only"),
		Stage:   q.Get("stage"),
	}
	// status (active or archived) is the library status the MCP tools filter
	// on; the store reads it as the stage of the same name.
	if status := q.Get("status"); status != "" {
		f.Stage = status
	}
	for _, raw := range q["tag"] {
		for _, part := range strings.Split(raw, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
				f.Tags = append(f.Tags, id)
			}
		}
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 1000 {
		f.Limit = n
	}
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n > 0 {
		f.Offset = n
	}
	return f
}

// canonicalShelfListing folds an unlinked machine copy under a linked skill of
// the same name only for the unfiltered shelf. A narrowed view can be the only
// one that holds the machine copy, so folding there could hide the skill
// entirely; the person asked for that machine or stage and still gets its card.
func canonicalShelfListing(f store.Filter) bool {
	return f.Machine == "" && f.Env == "" && f.Stage == "" && f.Only == "" &&
		len(f.Tags) == 0 && f.Query == "" && f.SourceType == "" && !f.Untagged &&
		(f.Tab == "" || f.Tab == "all")
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	compact := q.Get("compact") == "1" || q.Get("compact") == "true" || q.Get("fields") != ""
	filter := filterFromRequest(r)
	filter.Compact = compact
	filter.Canonical = canonicalShelfListing(filter)
	skills, counts, err := s.st.ListFor(s.uid(r), filter)
	if err == nil && q.Get("library") == "1" {
		skills, err = s.withLibraryOnlySkills(s.uid(r), filter, skills, &counts)
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if compact {
		fields := []string{"id", "name", "source", "sourceType", "stage", "liked", "bookmarked", "tags", "environment_count"}
		if raw := strings.TrimSpace(q.Get("fields")); raw != "" {
			fields = nil
			allowed := map[string]bool{"id": true, "name": true, "source": true, "sourceType": true, "status": true, "stage": true, "liked": true, "bookmarked": true, "tags": true, "environment_count": true}
			for _, field := range strings.Split(raw, ",") {
				field = strings.TrimSpace(field)
				if field == "" {
					continue
				}
				if !allowed[field] {
					writeErrCode(w, 400, errInvalidField, "fields", "unknown compact field: "+field, "Use id, name, source, sourceType, status, liked, tags or environment_count.")
					return
				}
				fields = append(fields, field)
			}
			if len(fields) == 0 {
				fields = []string{"id", "name"}
			}
		}
		if raw := strings.TrimSpace(q.Get("fields")); raw == "" {
			compactSkills := make([]compactSkill, len(skills))
			for i, sk := range skills {
				compactSkills[i] = compactSkill{
					ID: sk.ID, Name: sk.Name, Source: sk.Source, SourceType: sk.SourceType,
					Stage: stageKey(sk), Liked: sk.Liked, Bookmarked: sk.Bookmarked,
					Tags: compactTagNames(sk), EnvironmentCount: sk.EnvironmentCount,
				}
			}
			writeJSON(w, 200, map[string]any{"skills": compactSkills, "counts": counts})
			return
		}
		compactSkills := make([]map[string]any, 0, len(skills))
		for _, sk := range skills {
			row := make(map[string]any, len(fields))
			for _, field := range fields {
				switch field {
				case "id":
					row["id"] = sk.ID
				case "name":
					row["name"] = sk.Name
				case "source":
					row["source"] = sk.Source
				case "sourceType":
					row["sourceType"] = sk.SourceType
				case "status":
					row["status"] = sk.Status
				case "stage":
					row["stage"] = stageKey(sk)
				case "liked":
					row["liked"] = sk.Liked
				case "bookmarked":
					row["bookmarked"] = sk.Bookmarked
				case "tags":
					row["tags"] = compactTagNames(sk)
				case "environment_count":
					row["environment_count"] = sk.EnvironmentCount
				}
			}
			compactSkills = append(compactSkills, row)
		}
		writeJSON(w, 200, map[string]any{"skills": compactSkills, "counts": counts})
		return
	}
	writeJSON(w, 200, map[string]any{"skills": skills, "counts": counts})
}

// withLibraryOnlySkills adds, for library=1 (the MCP tools), the library
// skills no shelf row stands for, such as one written and never published,
// keyed by library id (ruling R47). They have no likes, tags or machines, so
// a filter on those leaves them out; text matches name, id and description.
// The whole list is put back in the listing's order. A paged listing is left
// as it is.
//
// counts, when given, are the listing's counts over the whole library, and
// count these skills too, whatever the filter.
func (s *server) withLibraryOnlySkills(uid int64, f store.Filter, skills []store.Skill, counts *store.LibraryCounts) ([]store.Skill, error) {
	extra, err := s.st.LibrarySkillsWithoutShelfFor(uid)
	if err != nil || len(extra) == 0 {
		return skills, err
	}
	if counts != nil {
		counts.Add(extra)
	}
	if f.Limit > 0 || f.Offset > 0 || (f.Tab != "" && f.Tab != "all") || len(f.Tags) > 0 || f.Env != "" || f.Machine != "" || f.Only != "" || f.SourceType != "" {
		return skills, nil
	}
	switch f.Stage {
	case "", store.SkillActive, store.SkillArchived:
	default:
		return skills, nil
	}
	term := strings.ToLower(strings.TrimSpace(f.Query))
	if strings.Contains(term, "#") {
		return skills, nil
	}
	for _, sk := range extra {
		if f.Stage != "" && sk.Status != f.Stage {
			continue
		}
		if term != "" && !strings.Contains(strings.ToLower(sk.Name+"\n"+sk.ID+"\n"+sk.Description), term) {
			continue
		}
		if f.Compact {
			sk.Tags = []store.Tag{}
		}
		skills = append(skills, sk)
	}
	sort.SliceStable(skills, func(i, j int) bool {
		a, b := skills[i], skills[j]
		switch f.Sort {
		case "name":
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "installs":
			if a.Installs != b.Installs {
				return a.Installs > b.Installs
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		if a.UpdatedAt != b.UpdatedAt {
			return a.UpdatedAt > b.UpdatedAt
		}
		return a.CreatedAt > b.CreatedAt
	})
	return skills, nil
}

// compactSkill is the default compact listing shape. A struct marshals much
// faster than a map per row; the custom fields= form keeps its map path.
type compactSkill struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Source           string   `json:"source"`
	SourceType       string   `json:"sourceType"`
	Stage            string   `json:"stage"`
	Liked            bool     `json:"liked"`
	Bookmarked       bool     `json:"bookmarked"`
	Tags             []string `json:"tags"`
	EnvironmentCount int      `json:"environment_count"`
}

func compactTagNames(sk store.Skill) []string {
	names := make([]string, 0, len(sk.Tags))
	for _, tag := range sk.Tags {
		names = append(names, tag.Name)
	}
	return names
}

// stageKey keeps the keys the Skills page reads until it shows Active and
// Archived: an Active skill reads as In use and an Archived one as Parked.
func stageKey(sk store.Skill) string {
	switch sk.Status {
	case store.SkillActive:
		return "use"
	case store.SkillArchived:
		return "shelf"
	}
	if sk.Used {
		return "use"
	}
	if sk.Bookmarked {
		return "try"
	}
	return "shelf"
}

func (s *server) add(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Input string `json:"input"`
		// Status is the library status the person chose, active or
		// archived, as Community's Save offers: a new skill starts with it,
		// and one the library has moves to it. Empty keeps today's add.
		Status string `json:"status"`
		store.Patch
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Status != "" && in.Status != store.SkillActive && in.Status != store.SkillArchived {
		writeErr(w, 400, "status must be active or archived")
		return
	}
	rf, err := s.parseSkillInput(r, in.Input)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_, existed, err := s.st.GetFor(s.uid(r), rf.ID)
	if err == nil && existed {
		// A row whose skill was deleted and waits for its purge is added
		// afresh: AddFor brings it back.
		existed, err = s.st.LiveLibrarySkillFor(s.uid(r), "", rf.ID)
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// A reet-hosted skill is something to try, not just a link to file away.
	hostedSkill := rf.SourceType == "hosted"
	noFlags := in.Bookmarked == nil && in.Used == nil
	if hostedSkill && noFlags {
		test := true
		in.Bookmarked = &test
	}
	var sk store.Skill
	if in.Status != "" {
		// The status the person chose, applied in the same transaction:
		// a new skill starts with it, one the library has moves to it.
		sk, err = s.st.AddWithStatusFor(s.uid(r), rf, in.Patch, in.Status)
	} else {
		sk, err = s.st.AddFor(s.uid(r), rf, in.Patch)
	}
	if errors.Is(err, store.ErrSkillNameTaken) {
		writeErr(w, 409, "A skill with that name is already in your library.")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.enqueue(rf)
	s.kickSkillCopies(s.uid(r))
	s.eventRequest(r, "add", sk.ID, map[string]any{"name": sk.Name, "created": !existed})
	writeJSON(w, 200, struct {
		store.Skill
		Created bool `json:"created"`
	}{sk, !existed})
}

func (s *server) patch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
		store.Patch
	}
	if !decode(w, r, &in) {
		return
	}
	old, _, _ := s.st.GetFor(s.uid(r), in.ID)
	sk, err := s.st.PatchFor(s.uid(r), in.ID, in.Patch)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "skill not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	action, detail := patchEvent(old, sk, in.Patch)
	s.eventRequest(r, action, sk.ID, detail)
	writeJSON(w, 200, sk)
}

func patchEvent(old, updated store.Skill, p store.Patch) (string, map[string]any) {
	detail := map[string]any{"name": updated.Name}
	switch {
	case p.Notes != nil && p.Liked == nil && p.Bookmarked == nil && p.Used == nil:
		return "notes", detail
	case p.Liked != nil && p.Bookmarked == nil && p.Used == nil:
		if *p.Liked {
			return "like", detail
		}
		return "unlike", detail
	case p.Bookmarked != nil || p.Used != nil:
		detail["from"] = stageLabel(old)
		detail["to"] = stageLabel(updated)
		return "stage", detail
	}
	return "patch", detail
}

// remove is the old shelf Remove of one shelf row. It deletes the row's
// library skill, as the Skills page's Delete does: it leaves every machine,
// and its name is free again. Nothing changes, with a 409, for a skill the
// person wrote ({"code":"written"}, deleted from Write a skill) or one reet
// holds no copy of, unless noCopyConfirmed=1 ({"code":"no-copy"}).
func (s *server) remove(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeErr(w, 400, "id required")
		return
	}
	sk, _, _ := s.st.GetFor(s.uid(r), id)
	if !s.removeShelfSkills(w, r, []string{id}, r.URL.Query().Get("noCopyConfirmed") == "1") {
		return
	}
	s.eventRequest(r, "remove", id, map[string]any{"name": sk.Name})
	w.WriteHeader(http.StatusNoContent)
}

// removeShelfSkills removes shelf rows and deletes their library skills, and
// says so on w when it cannot.
func (s *server) removeShelfSkills(w http.ResponseWriter, r *http.Request, ids []string, noCopyConfirmed bool) bool {
	uid := s.uid(r)
	_, refused, err := s.st.RemoveShelfSkillsFor(uid, ids, noCopyConfirmed, s.skillNow())
	if errors.Is(err, store.ErrSkillNoCopy) || errors.Is(err, store.ErrSkillWritten) {
		writeNotRemoved(w, err, refused)
		return false
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return false
	}
	s.kickSkillCopies(uid)
	return true
}

func (s *server) refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	before, _, _ := s.st.GetFor(s.uid(r), in.ID)
	rf, err := ref.Parse(in.ID)
	if before.SourceType == "hosted" {
		rf, err = ref.ParseAt(before.URL, r.Host)
	}
	if err != nil {
		writeErr(w, 400, "skill has no skills.sh source to refresh from")
		return
	}
	refreshed := s.fetchInto(rf)
	sk, ok, err := s.st.GetFor(s.uid(r), in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "skill not found")
		return
	}
	changed := refreshed && metaChanged(before, sk)
	s.eventRequest(r, "refresh", sk.ID, map[string]any{"name": sk.Name, "changed": changed, "ok": refreshed})
	writeJSON(w, 200, struct {
		store.Skill
		Changed   bool `json:"changed"`
		Refreshed bool `json:"refreshed"`
	}{sk, changed, refreshed})
}

func metaChanged(a, b store.Skill) bool {
	nameChanged := a.Name != b.Name || a.Description != b.Description
	countsChanged := a.Installs != b.Installs || a.InstallsWeek != b.InstallsWeek
	if nameChanged || countsChanged {
		return true
	}
	if len(a.Trend) != len(b.Trend) {
		return true
	}
	for i := range a.Trend {
		if a.Trend[i] != b.Trend[i] {
			return true
		}
	}
	return false
}

func (s *server) resolve(w http.ResponseWriter, r *http.Request) {
	var in struct{ ID, Input string }
	if !decode(w, r, &in) {
		return
	}
	rf, err := s.parseSkillInput(r, in.Input)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	sk, err := s.st.ResolveFor(s.uid(r), in.ID, rf)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "skill not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.enqueue(rf)
	s.eventRequest(r, "link", sk.ID, map[string]any{"name": sk.Name, "from": in.ID})
	writeJSON(w, 200, sk)
}

func (s *server) installCopy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	sk, ok, err := s.st.GetFor(s.uid(r), in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "skill not found")
		return
	}
	s.eventRequest(r, "install-copy", sk.ID, map[string]any{"name": sk.Name})
	w.WriteHeader(http.StatusNoContent)
}

func validIDs(ids []string) bool {
	if len(ids) == 0 || len(ids) > 500 {
		return false
	}
	for _, id := range ids {
		if id == "" {
			return false
		}
	}
	return true
}

func bulkAction(p store.Patch) (string, map[string]any) {
	detail := map[string]any{}
	switch {
	case p.Notes != nil:
		return "bulk-patch", detail
	case p.Liked != nil && p.Bookmarked == nil && p.Used == nil:
		detail["liked"] = *p.Liked
		return "bulk-like", detail
	case p.Bookmarked != nil || p.Used != nil:
		to := "Parked"
		if p.Used != nil && *p.Used {
			to = "In use"
		} else if p.Bookmarked != nil && *p.Bookmarked {
			to = "To test"
		}
		detail["to"] = to
		return "bulk-stage", detail
	}
	return "bulk-patch", detail
}

func (s *server) patchMany(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
		store.Patch
	}
	if !decode(w, r, &in) {
		return
	}
	if !validIDs(in.IDs) {
		writeErr(w, 400, "choose between 1 and 500 skills")
		return
	}
	n, err := s.st.PatchManyFor(s.uid(r), in.IDs, in.Patch)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	action, detail := bulkAction(in.Patch)
	detail["count"] = n
	s.eventRequest(r, action, "", detail)
	writeJSON(w, 200, map[string]int{"updated": n})
}

func (s *server) removeMany(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs             []string `json:"ids"`
		NoCopyConfirmed bool     `json:"noCopyConfirmed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validIDs(in.IDs) {
		writeErr(w, 400, "choose between 1 and 500 skills")
		return
	}
	uid := s.uid(r)
	n, refused, err := s.st.RemoveShelfSkillsFor(uid, in.IDs, in.NoCopyConfirmed, s.skillNow())
	if errors.Is(err, store.ErrSkillNoCopy) || errors.Is(err, store.ErrSkillWritten) {
		writeNotRemoved(w, err, refused)
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.kickSkillCopies(uid)
	s.eventRequest(r, "bulk-remove", "", map[string]any{"count": n})
	writeJSON(w, 200, map[string]int{"removed": n})
}
