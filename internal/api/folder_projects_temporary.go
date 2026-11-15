package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A temporary project is a repository picked From GitHub in Start an agent or
// a Factory outcome: reet cloned it for that start and saved it as a project
// so the start could use it like any other. Once nothing uses it and nothing
// in the clone would be lost, reet removes the project and deletes the clone,
// with the same checks Remove from reet makes. While something would be lost
// it stays, and the project says why.

var (
	// temporaryCheckEvery is how often temporary projects are looked at.
	temporaryCheckEvery = 5 * time.Minute
	// temporaryGrace is how long a new temporary project is left alone: the
	// start it was made for begins only after its clone is saved.
	temporaryGrace = 30 * time.Minute
)

var temporaryMu sync.Mutex

// temporaryGate keeps the clean-up and the person's own changes to a project
// apart. While the clean-up removes a temporary project it holds the
// project's mark; Keep and Edit write only under the gate and are refused
// while the mark is held. So a Keep either lands before the clean-up reads
// the project again (which then finds it no longer temporary and stops) or
// is refused while the removal runs: a kept project is never removed.
var temporaryGate struct {
	sync.Mutex
	removing map[string]bool
}

const sentenceTemporaryRemoving = "reet is removing this project and its clone right now. Try again in a moment."

func temporaryKey(uid int64, id string) string { return strconv.FormatInt(uid, 10) + "/" + id }

// claimTemporary marks the project as being removed; false when it already is.
func claimTemporary(uid int64, id string) bool {
	temporaryGate.Lock()
	defer temporaryGate.Unlock()
	if temporaryGate.removing[temporaryKey(uid, id)] {
		return false
	}
	if temporaryGate.removing == nil {
		temporaryGate.removing = map[string]bool{}
	}
	temporaryGate.removing[temporaryKey(uid, id)] = true
	return true
}

func releaseTemporary(uid int64, id string) {
	temporaryGate.Lock()
	defer temporaryGate.Unlock()
	delete(temporaryGate.removing, temporaryKey(uid, id))
}

// unlessRemoving runs change under the gate, unless the clean-up is removing
// the project: then nothing is changed and false is returned.
func unlessRemoving(uid int64, id string, change func()) bool {
	temporaryGate.Lock()
	defer temporaryGate.Unlock()
	if temporaryGate.removing[temporaryKey(uid, id)] {
		return false
	}
	change()
	return true
}

func (s *server) temporaryProjectWorker() {
	ticker := time.NewTicker(temporaryCheckEvery)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.sweepTemporaryProjects(time.Now())
		}
	}
}

// sweepTemporaryProjects looks at every account's temporary projects once.
// Only one sweep runs at a time; a fenced controller touches no machine.
func (s *server) sweepTemporaryProjects(now time.Time) {
	if !temporaryMu.TryLock() {
		return
	}
	defer temporaryMu.Unlock()
	if s.st.Fenced() {
		return
	}
	list, err := s.st.TemporaryFolderProjects()
	if err != nil {
		return
	}
	for _, item := range list {
		select {
		case <-s.stop:
			return
		default:
		}
		s.sweepTemporaryProject(item.UserID, item.Project, now)
	}
}

// temporaryInUse reports whether anything still uses the project: a session
// that has not stopped, started on it or running in its folder, or a Factory
// outcome on it that has not finished. A list that cannot be read counts as
// in use, so nothing is removed on a guess.
func (s *server) temporaryInUse(uid int64, p store.FolderProject) bool {
	sessions, err := s.st.TerminalSessionsFor(uid)
	if err != nil {
		return true
	}
	for _, t := range sessions {
		if t.State.Closed() {
			continue
		}
		onProject := t.ProjectID != nil && *t.ProjectID == p.ID
		inside := t.AgentID == p.Host && (t.Dir == p.Dir || strings.HasPrefix(t.Dir, p.Dir+"/"))
		if onProject || inside {
			return true
		}
	}
	outcomes, err := s.st.GafferOutcomesFor(uid)
	if err != nil {
		return true
	}
	for _, o := range outcomes {
		if o.ProjectID == p.ID && !o.Finished() {
			return true
		}
	}
	return false
}

// sweepTemporaryProject removes one temporary project with its clone when it
// is safe now. Nothing happens while it is new or in use, or while its
// machine cannot be asked (it is tried again later). When the machine says
// deleting the clone would lose something, the project stays and says why.
func (s *server) sweepTemporaryProject(uid int64, p store.FolderProject, now time.Time) {
	if !p.Temporary || now.Sub(time.UnixMilli(p.CreatedAt)) < temporaryGrace {
		return
	}
	keep := func(because string) {
		if because != p.KeptBecause {
			_ = s.st.SetFolderProjectKeptFor(uid, p.ID, because)
		}
	}
	if s.temporaryInUse(uid, p) {
		keep("")
		return
	}
	names, err := s.machineNames(uid)
	if err != nil {
		return
	}
	machine := machineLabel(names, p.Host)
	dir, origin, ok := cloneFolder(p)
	if !ok {
		keep(cloneNotFound(machine, p))
		return
	}
	if !s.cloneReachable(uid, p.Host) || s.cloneHeldBack(uid, p, machine) != "" {
		return
	}
	// From here the project is marked: Keep and Edit wait their turn. It is
	// read again under the mark, and the removal goes on only if it is still
	// this temporary project, unchanged since the sweep listed it.
	if !claimTemporary(uid, p.ID) {
		return
	}
	defer releaseTemporary(uid, p.ID)
	current, err := s.st.FolderProjectFor(uid, p.ID)
	if err != nil || !current.Temporary || current.Version != p.Version || current.Host != p.Host || current.Dir != p.Dir {
		return
	}
	ctx, cancel := context.WithTimeout(s.baseContext(), terminalbroker.RepoCloneStatusCallTimeout)
	state, err := s.terminals.CloneStatus(ctx, uid, p.Host, terminalproto.RepoCloneStatus{Dir: dir, Origin: origin})
	cancel()
	if err != nil {
		return
	}
	if !state.Removable() {
		keep(s.cloneStateSentence(uid, p, machine, state))
		return
	}
	detail := folderAuditDetail(p, names)
	detail["origin"], detail["cloned"], detail["temporary"] = p.Origin, p.Cloned, true
	removed, refusal := s.removeProjectClone(uid, p, machine, state.Fingerprint)
	if refusal != nil {
		if refusal.code == "clone_unconfirmed" {
			detail["unconfirmed"], detail["clonePath"] = true, p.Dir
			s.event(uid, "system", "project-clone-delete", "", detail)
		}
		if refusal.code != "clone_changed" {
			keep(refusal.sentence)
		}
		return
	}
	detail["cloneDeleted"], detail["clonePath"] = true, removed
	if err := s.st.DeleteFolderProjectFor(uid, p.ID, p.Version); err != nil && err != store.ErrFolderProjectNotFound {
		// The clone is gone but the project could not be removed: the next
		// sweep finds no clone and says so on the project.
		s.event(uid, "system", "project-clone-delete", "", detail)
		return
	}
	s.event(uid, "system", "project-remove", "", detail)
}

// folderProjectKeep answers POST /api/v2/folder-projects/{id}/keep: the person
// keeps a temporary project, so reet never removes it or its clone.
func (s *server) folderProjectKeep(w http.ResponseWriter, r *http.Request) {
	if !folderProjectOptionalBody(w, r, &struct{}{}) {
		return
	}
	uid := s.uid(r)
	var kept store.FolderProject
	var err error
	if !unlessRemoving(uid, r.PathValue("id"), func() { kept, err = s.st.KeepFolderProjectFor(uid, r.PathValue("id")) }) {
		writeErrCode(w, http.StatusConflict, "project_removing", "", sentenceTemporaryRemoving, "")
		return
	}
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	names, err := s.machineNames(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	detail := folderAuditDetail(kept, names)
	detail["kept"] = true
	s.event(uid, "ui", "project-keep", "", detail)
	writeJSON(w, http.StatusOK, viewFolderProject(kept, names))
}
