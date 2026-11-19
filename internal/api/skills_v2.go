package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/store"
)

const (
	// skillsPageMaxIDs bounds the skills one request names.
	skillsPageMaxIDs = 1000
	// skillsPageIDLimit bounds a body that names library items: up to 1,000
	// ids of 32 characters.
	skillsPageIDLimit = 64 << 10
	// skillsPageNameLimit bounds a body that names skills: up to 1,000 names
	// of up to 255 bytes.
	skillsPageNameLimit = 320 << 10
	// skillsPageSmallLimit bounds a body with one field.
	skillsPageSmallLimit = 4 << 10
)

// WithSkillClock sets when the Skills page's changes are made. Only tests need
// it: one set earlier than the grace period stands for a change machines are
// told of at once.
func WithSkillClock(now func() time.Time) Option { return func(s *server) { s.skillClock = now } }

func (s *server) skillNow() time.Time {
	if s.skillClock != nil {
		return s.skillClock()
	}
	return time.Now()
}

// registerSkillsPage serves the Skills page from one API keyed by library
// item. Retry and Update from original are in skill_copies.go; likes, notes,
// tags and lists keep their endpoints, keyed by each row's shelfId.
func (s *server) registerSkillsPage(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/skills", s.skillsPage)
	mux.HandleFunc("POST /api/v2/skills/status", s.requireWriter(s.skillsSetStatus))
	mux.HandleFunc("POST /api/v2/skills/delete", s.requireWriter(s.skillsDelete))
	mux.HandleFunc("POST /api/v2/skills/undelete", s.requireWriter(s.skillsUndelete))
	mux.HandleFunc("POST /api/v2/skills/new", s.requireWriter(s.skillsNew))
	mux.HandleFunc("POST /api/v2/skills/notice", s.requireWriter(s.skillsNotice))
	mux.HandleFunc("POST /api/v2/skills/{item}/use-version", s.requireWriter(s.skillUseVersion))
}

// skillsPageData is the page for the tab status, with each published
// skill's public link.
func (s *server) skillsPageData(r *http.Request, status string) (store.SkillsPage, error) {
	uid := s.uid(r)
	page, err := s.st.SkillsPageFor(uid, status, time.Now())
	if err != nil {
		return page, err
	}
	username := ""
	for i := range page.Skills {
		row := &page.Skills[i]
		if !row.Published || row.HostedSlug == "" {
			continue
		}
		if username == "" {
			if username, err = s.st.PublishingUsername(uid); err != nil {
				return page, err
			}
		}
		if username != "" {
			row.PublicURL = requestOrigin(r) + "/" + username + "/" + row.HostedSlug
		}
	}
	return page, nil
}

func (s *server) writeSkillsPage(w http.ResponseWriter, r *http.Request, status string) {
	page, err := s.skillsPageData(r, status)
	if err != nil {
		skillsPageError(w, err)
		return
	}
	writeJSON(w, 200, page)
}

// skillsPage is GET /api/v2/skills?status=active|archived; no status is the
// Active tab.
func (s *server) skillsPage(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = store.SkillActive
	}
	if status != store.SkillActive && status != store.SkillArchived {
		writeErrCode(w, 400, errInvalidArgument, "status", "Choose active or archived.", "")
		return
	}
	s.writeSkillsPage(w, r, status)
}

func skillsPageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrPrivateSkillMissing):
		writeErr(w, 404, "That skill is not in your library.")
	case errors.Is(err, store.ErrSkillNameTaken):
		writeErr(w, 409, "Another skill in your library has that name now.")
	case errors.Is(err, store.ErrPrivateSkillInvalid):
		writeErr(w, 400, "That is not a change the Skills page can make.")
	default:
		writeErr(w, 500, err.Error())
	}
}

// pageIDs checks the ids a request names: between 1 and skillsPageMaxIDs
// library item ids. It returns them without repeats.
func pageIDs(w http.ResponseWriter, ids []string) ([]string, bool) {
	if len(ids) > skillsPageMaxIDs {
		writeErrCode(w, 400, errInvalidArgument, "ids", fmt.Sprintf("Choose at most %d skills at a time.", skillsPageMaxIDs), "")
		return nil, false
	}
	for _, id := range ids {
		if id == "" || len(id) > 64 {
			writeErrCode(w, 400, errInvalidArgument, "ids", "That is not a skill id.", "")
			return nil, false
		}
	}
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		writeErrCode(w, 400, errInvalidArgument, "ids", "Choose at least one skill.", "")
		return nil, false
	}
	return ids, true
}

// skillsSetStatus is Archive and Restore, one skill or many: body
// {"ids":[…],"status":"active"|"archived","tab":"active"|"archived"}. It
// answers with the page for tab, the tab the person is on, so pruning never
// switches tabs. Without tab it answers with the tab the skills went to,
// where an Undo gets them back in their places.
func (s *server) skillsSetStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Status string   `json:"status"`
		Tab    string   `json:"tab"`
	}
	if !decodeStrictJSON(w, r, skillsPageIDLimit, &in, "Invalid or oversized status change.") {
		return
	}
	if in.Status != store.SkillActive && in.Status != store.SkillArchived {
		writeErrCode(w, 400, errInvalidArgument, "status", "Choose active or archived.", "")
		return
	}
	if in.Tab == "" {
		in.Tab = in.Status
	}
	if in.Tab != store.SkillActive && in.Tab != store.SkillArchived {
		writeErrCode(w, 400, errInvalidArgument, "tab", "Choose active or archived.", "")
		return
	}
	ids, ok := pageIDs(w, in.IDs)
	if !ok {
		return
	}
	uid := s.uid(r)
	items, err := s.st.SkillItemsFor(uid, ids)
	if err == nil {
		err = s.st.SetSkillStatusFor(uid, ids, in.Status, s.skillNow())
	}
	if err != nil {
		skillsPageError(w, err)
		return
	}
	action := "archive"
	if in.Status == store.SkillActive {
		action = "restore"
	}
	s.eventRequest(r, action, "", map[string]any{"names": itemNames(items)})
	// An archived skill reet has no copy of is copied in before it leaves
	// any machine.
	s.kickSkillCopies(uid)
	s.writeSkillsPage(w, r, in.Tab)
}

func itemNames(items []store.SkillItemRef) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Name)
	}
	return out
}

// skillsDelete is Delete: body {"ids":[…],"noCopyConfirmed":bool,
// "writtenConfirmed":bool}. It deletes nothing, and says which skills with a
// 409, when one of them is
//   - a skill the person wrote, unless writtenConfirmed
//     ({"code":"written","names":[…]}): their writing and its public page go
//     with it, so Delete asks once;
//   - one reet holds no copy of, unless noCopyConfirmed
//     ({"code":"no-copy","names":[…]}): the machines then keep only their
//     backup.
func (s *server) skillsDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs              []string `json:"ids"`
		NoCopyConfirmed  bool     `json:"noCopyConfirmed"`
		WrittenConfirmed bool     `json:"writtenConfirmed"`
	}
	if !decodeStrictJSON(w, r, skillsPageIDLimit, &in, "Invalid or oversized delete.") {
		return
	}
	ids, ok := pageIDs(w, in.IDs)
	if !ok {
		return
	}
	uid := s.uid(r)
	items, err := s.st.SkillItemsFor(uid, ids)
	if err != nil {
		skillsPageError(w, err)
		return
	}
	noCopy, written, pages := []string{}, []string{}, 0
	for _, it := range items {
		if !it.HasCopy {
			noCopy = append(noCopy, it.Name)
		}
		if it.Written {
			written = append(written, it.Name)
			if it.Published {
				pages++
			}
		}
	}
	if len(written) > 0 && !in.WrittenConfirmed {
		// A public page is mentioned only where there is one.
		them := "it"
		if len(written) > 1 {
			them = "them"
		}
		msg := "You wrote " + nameList(written) + ". Deleting " + them + " deletes your writing"
		switch {
		case pages == 0:
			msg += " too."
		case len(written) == 1:
			msg += " and its public page too."
		case pages == 1:
			msg += " and a public page too."
		default:
			msg += " and their public pages too."
		}
		writeJSON(w, 409, map[string]any{"code": "written", "names": written, "error": msg, "message": msg})
		return
	}
	if len(noCopy) > 0 && !in.NoCopyConfirmed {
		writeNoCopy(w, noCopy)
		return
	}
	if err = s.st.DeleteSkillsFor(uid, ids, in.NoCopyConfirmed, s.skillNow()); errors.Is(err, store.ErrSkillNoCopy) {
		// A copy went in the meantime.
		noCopy = noCopy[:0]
		if now, e := s.st.SkillItemsFor(uid, ids); e == nil {
			for _, it := range now {
				if !it.HasCopy {
					noCopy = append(noCopy, it.Name)
				}
			}
		}
		writeNoCopy(w, noCopy)
		return
	}
	if err != nil {
		skillsPageError(w, err)
		return
	}
	s.eventRequest(r, "delete", "", map[string]any{"names": itemNames(items)})
	s.kickSkillCopies(uid)
	writeJSON(w, 200, map[string]int{"deleted": len(ids)})
}

// writeNoCopy refuses a delete of skills reet holds no copy of, naming them.
func writeNoCopy(w http.ResponseWriter, names []string) {
	msg := "reet has no copy of " + strings.Join(names, ", ") + ". Deleting removes it from every machine, which keeps a backup for 30 days."
	if len(names) > 1 {
		msg = fmt.Sprintf("reet has no copy of %d of these skills. Deleting removes them from every machine, which keeps a backup for 30 days.", len(names))
	}
	writeJSON(w, 409, map[string]any{"code": "no-copy", "names": names, "error": msg, "message": msg})
}

// writeNotRemoved is the old Remove's refusal, which changed nothing: of a
// skill the person wrote, or of one reet holds no copy of (err says which).
// It says plainly that nothing was removed, and how to go ahead where one can.
func writeNotRemoved(w http.ResponseWriter, err error, names []string) {
	list := nameList(names)
	if errors.Is(err, store.ErrSkillWritten) {
		msg := "You wrote " + list + ". Delete it from Write a skill."
		if len(names) > 1 {
			msg = "You wrote " + list + ". Delete them from Write a skill."
		}
		writeJSON(w, 409, map[string]any{"code": "written", "names": names, "error": msg, "message": msg})
		return
	}
	msg := "reet has no copy of " + list + ", so it was not removed. Removing it deletes it from every machine, which keeps a backup for 30 days."
	if len(names) > 1 {
		msg = "reet has no copy of " + list + ", so they were not removed. Removing them deletes them from every machine, which keeps a backup for 30 days."
	}
	hint := "Remove it again with noCopyConfirmed to delete it from every machine."
	if len(names) > 1 {
		hint = "Remove them again with noCopyConfirmed to delete them from every machine."
	}
	writeJSON(w, 409, map[string]any{"code": "no-copy", "names": names, "error": msg, "message": msg, "hint": hint})
}

// nameList names skills in a sentence: up to three, then how many more.
// The full list goes in a refusal's names.
func nameList(names []string) string {
	switch {
	case len(names) <= 1:
		return strings.Join(names, "")
	case len(names) <= 3:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
	return strings.Join(names[:3], ", ") + fmt.Sprintf(" and %d more", len(names)-3)
}

// skillsUndelete is Undo of Delete: body {"ids":[…]}.
func (s *server) skillsUndelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if !decodeStrictJSON(w, r, skillsPageIDLimit, &in, "Invalid or oversized undo.") {
		return
	}
	ids, ok := pageIDs(w, in.IDs)
	if !ok {
		return
	}
	uid := s.uid(r)
	err := s.st.DeletedSkillItemsFor(uid, ids)
	if err == nil {
		err = s.st.UndeleteSkillsFor(uid, ids)
	}
	if errors.Is(err, store.ErrPrivateSkillMissing) {
		writeErr(w, 404, "It's too late to bring that back: it is gone from your library.")
		return
	}
	if err != nil {
		skillsPageError(w, err)
		return
	}
	s.eventRequest(r, "undelete", "", map[string]any{"count": len(ids)})
	writeJSON(w, 200, map[string]int{"restored": len(ids)})
}

// skillsNew decides skills new on a machine: body {"names":[…],
// "action":"keep"|"archive","from":"<agent id>"}. Keep makes each an Active
// skill and Archive an Archived one, with reet taking a copy of its files
// from the first machine listed, or from the machine from names. When the
// machines' copies differ Keep needs from (409 {"code":"choose-copy"}). A
// name no machine reports as new is refused (409 {"code":"not-new"}).
func (s *server) skillsNew(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Names  []string `json:"names"`
		Action string   `json:"action"`
		From   string   `json:"from"`
	}
	if !decodeStrictJSON(w, r, skillsPageNameLimit, &in, "Invalid or oversized decision.") {
		return
	}
	status := map[string]string{"keep": store.SkillActive, "archive": store.SkillArchived}[in.Action]
	if status == "" {
		writeErrCode(w, 400, errInvalidArgument, "action", "Choose keep or archive.", "")
		return
	}
	if len(in.From) > 64 {
		writeErrCode(w, 400, errInvalidArgument, "from", "That is not a machine.", "")
		return
	}
	if len(in.Names) > skillsPageMaxIDs {
		writeErrCode(w, 400, errInvalidArgument, "names", fmt.Sprintf("Choose at most %d skills at a time.", skillsPageMaxIDs), "")
		return
	}
	for _, name := range in.Names {
		if name == "" || len(name) > 255 || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\\x00\r\n") {
			writeErrCode(w, 400, errInvalidArgument, "names", "That is not a skill name.", "")
			return
		}
	}
	names := uniqueStrings(in.Names)
	if len(names) == 0 {
		writeErrCode(w, 400, errInvalidArgument, "names", "Choose at least one skill.", "")
		return
	}
	uid := s.uid(r)
	kept, err := s.st.KeepNewSkillsFor(uid, names, status, in.From, s.skillNow())
	var refused *store.NewSkillError
	if errors.As(err, &refused) {
		name := refused.Name
		switch {
		case errors.Is(err, store.ErrSkillNotNew):
			writeJSON(w, 409, map[string]any{"code": "not-new", "names": []string{name}, "error": name + " is not new on any machine."})
			return
		case errors.Is(err, store.ErrSkillChooseCopy):
			writeJSON(w, 409, map[string]any{"code": "choose-copy", "names": []string{name}, "error": "The machines have different copies of " + name + ". Choose which machine's copy to keep."})
			return
		case errors.Is(err, store.ErrSkillCopyNotThere):
			writeErrCode(w, 400, errInvalidArgument, "from", "That machine has no copy of "+name+".", "")
			return
		}
	}
	if err != nil {
		skillsPageError(w, err)
		return
	}
	s.kickNewSkillCaptures(uid, kept)
	action := "keep"
	if status == store.SkillArchived {
		action = "archive-new"
	}
	items := make([]string, 0, len(kept))
	for _, k := range kept {
		items = append(items, k.Item)
	}
	s.eventRequest(r, action, "", map[string]any{"names": names})
	writeJSON(w, 200, map[string]any{"items": items})
}

// kickNewSkillCaptures asks each kept skill's chosen machine for its files,
// through the existing machine capture, then makes sure every skill without
// content is being copied. A machine that cannot take the request now is
// asked again by the copy worker, which tries the chosen machine first.
func (s *server) kickNewSkillCaptures(uid int64, kept []store.KeptNewSkill) {
	for _, k := range kept {
		err := s.st.RequestSkillCaptureFor(uid, k.Item, k.AgentID, k.Folder+"/"+k.Name)
		if err != nil && !errors.Is(err, store.ErrRemoteImport) && !errors.Is(err, store.ErrRemoteImportCapacity) && !errors.Is(err, store.ErrSkillNoCaptureSource) && !errors.Is(err, store.ErrContentObservationStale) {
			log.Printf("skills: capture %s: %v", k.Name, err)
		}
	}
	s.kickSkillCopies(uid)
}

// skillsNotice answers the one-time upgrade notice: body
// {"action":"dismiss"|"restore-parked"}.
func (s *server) skillsNotice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"`
	}
	if !decodeStrictJSON(w, r, skillsPageSmallLimit, &in, "Invalid or oversized notice answer.") {
		return
	}
	uid := s.uid(r)
	var err error
	switch in.Action {
	case "dismiss":
		err = s.st.DismissSkillUpgradeNoticeFor(uid)
	case "restore-parked":
		err = s.st.RestoreParkedSkillsFor(uid, s.skillNow())
	default:
		writeErrCode(w, 400, errInvalidArgument, "action", "Choose dismiss or restore-parked.", "")
		return
	}
	if err != nil {
		skillsPageError(w, err)
		return
	}
	s.eventRequest(r, "skills-notice", "", map[string]any{"action": in.Action})
	if in.Action == "restore-parked" {
		s.kickSkillCopies(uid)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// skillUseVersion is Use this version: body {"agentId":"…"}. It asks that
// machine for its backup of the copy of the skill it edited, and the files
// become a new revision that every machine gets. It answers 202 once the
// machine has been asked.
func (s *server) skillUseVersion(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AgentID string `json:"agentId"`
	}
	if !decodeStrictJSON(w, r, skillsPageSmallLimit, &in, "Invalid or oversized request.") {
		return
	}
	if in.AgentID == "" || len(in.AgentID) > 64 {
		writeErrCode(w, 400, errInvalidArgument, "agentId", "Choose the machine whose version to use.", "")
		return
	}
	uid, item := s.uid(r), r.PathValue("item")
	if len(item) > 64 {
		writeErrCode(w, 400, errInvalidArgument, "item", "That is not a skill id.", "")
		return
	}
	req, err := s.st.RequestSkillUseVersionFor(uid, item, in.AgentID, time.Now())
	if err != nil {
		name := "this skill"
		if items, e := s.st.SkillItemsFor(uid, []string{item}); e == nil {
			name = items[0].Name
		}
		host := "That machine"
		if req.Host != "" {
			host = req.Host
		}
		switch {
		case errors.Is(err, store.ErrPrivateSkillMissing):
			writeErr(w, 404, "That skill is not in your library.")
		case errors.Is(err, store.ErrSkillMachineMissing):
			writeErr(w, 404, "That machine is not one of yours.")
		case errors.Is(err, store.ErrSkillNoBackup):
			writeErr(w, 404, fmt.Sprintf("reet has no edit to %s from %s to use. A machine's edit is kept for 30 days after reet puts its own copy back.", name, host))
		case errors.Is(err, store.ErrSkillBackupPath):
			writeErr(w, 409, fmt.Sprintf("The backup %s reported is not in its skills backups, so reet won't read it.", host))
		case errors.Is(err, store.ErrSkillMachineOld):
			writeErr(w, 409, fmt.Sprintf("Update reet on %s to use its version.", host))
		case errors.Is(err, store.ErrSkillMachinePaused), errors.Is(err, store.ErrContentObservationStale):
			writeErr(w, 409, fmt.Sprintf("Sync is paused on %s. Turn it back on to use its version.", host))
		case errors.Is(err, store.ErrRemoteImport), errors.Is(err, store.ErrRemoteNotFound):
			writeErr(w, 409, fmt.Sprintf("%s isn't connected right now. Try again when it's back.", host))
		case errors.Is(err, store.ErrRemoteImportCapacity):
			remoteImportError(w, err)
		case errors.Is(err, store.ErrPrivateSkillConflict):
			writeErr(w, 409, "The skill changed just now. Try again.")
		default:
			privateSkillError(w, err)
		}
		return
	}
	s.eventRequest(r, "use-version", "", map[string]any{"item": item, "host": req.Host})
	writeJSON(w, 202, map[string]any{"requested": true, "id": req.ID})
}
