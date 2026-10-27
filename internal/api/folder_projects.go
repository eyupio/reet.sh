package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Folder projects (docs/superpowers/specs/2026-10-26-agents-home-design.md,
// "Project registry"): one folder on one machine, per account. The routes
// take the Projects access rules (projectAccess), strict JSON, ETag/If-Match
// and one-sentence refusals naming the field. Nothing here asks a machine
// anything: the dialog browses beforehand, so isRepo and baseBranch are the
// client's hints, checked for shape only.

// folderProjectWriteLimit is how many folder-project writes one account may
// make per minute.
const folderProjectWriteLimit = 30

// maxFolderProjectBodyBytes bounds a request: the longest folder, name,
// branch, model and starter prompt, each JSON-escaped, fit well inside it, as
// does a reorder of 100 ids and versions.
const maxFolderProjectBodyBytes = 64 << 10

var folderAccessWords = accessWords{
	accounts:  "Using projects needs an account signed in through the browser.",
	signedOut: "Sign in to use projects.",
	readOnly:  "This account is read-only, so it can't use projects.",
}

// folderAccess is the Projects access rule with sentences naming projects.
func (s *server) folderAccess(next http.HandlerFunc) http.HandlerFunc {
	return s.featureAccess(folderAccessWords, next)
}

func (s *server) registerFolderProjectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/folder-projects", s.folderAccess(s.folderProjectList))
	mux.HandleFunc("POST /api/v2/folder-projects", s.folderAccess(s.folderProjectWrite(s.folderProjectCreate)))
	mux.HandleFunc("POST /api/v2/folder-projects/reorder", s.folderAccess(s.folderProjectWrite(s.folderProjectReorder)))
	mux.HandleFunc("POST /api/v2/folder-projects/import-saved", s.folderAccess(s.folderProjectWrite(s.folderProjectImportSaved)))
	mux.HandleFunc("GET /api/v2/folder-projects/{id}", s.folderAccess(s.folderProjectGet))
	mux.HandleFunc("PATCH /api/v2/folder-projects/{id}", s.folderAccess(s.folderProjectWrite(s.folderProjectUpdate)))
	mux.HandleFunc("DELETE /api/v2/folder-projects/{id}", s.folderAccess(s.folderProjectWrite(s.folderProjectDelete)))
}

// folderProjectWrite admits at most folderProjectWriteLimit writes per
// account per minute. Every write that reaches it counts, refused or not.
func (s *server) folderProjectWrite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strconv.FormatInt(s.uid(r), 10)
		s.folderWritesMu.Lock()
		allowed := s.folderWrites.Allow(key)
		if allowed {
			s.folderWrites.Hit(key)
		}
		s.folderWritesMu.Unlock()
		if !allowed {
			writeErrCode(w, http.StatusTooManyRequests, "project_rate_limited", "", sentenceSlowDown, "")
			return
		}
		next(w, r)
	}
}

// folderProjectBody decodes exactly one JSON object with no unknown fields.
func folderProjectBody(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFolderProjectBodyBytes)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(out)
	if err == nil {
		if extra := d.Decode(new(any)); extra != io.EOF {
			err = extra
			if err == nil {
				err = errors.New("more than one JSON value")
			}
		}
	}
	if err == nil {
		return true
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeErrCode(w, http.StatusRequestEntityTooLarge, "too_large", "", "That request is too large for a project.", "")
	} else {
		writeErrCode(w, http.StatusBadRequest, "invalid_request", "", "That request isn't a project reet can read.", "")
	}
	return false
}

// folderProjectInput is the body of a create or an update. On an update a
// field that is absent or null keeps its saved value, and "" clears a text
// field (name falls back to the folder's name).
type folderProjectInput struct {
	Host            *string `json:"host"`
	Dir             *string `json:"dir"`
	Name            *string `json:"name"`
	IsRepo          *bool   `json:"isRepo"`
	BaseBranch      *string `json:"baseBranch"`
	WorktreeDefault *string `json:"worktreeDefault"`
	DefaultTool     *string `json:"defaultTool"`
	DefaultModel    *string `json:"defaultModel"`
	StarterPrompt   *string `json:"starterPrompt"`
	// Note can only be cleared (""): it is written by the migration of
	// saved Projects and shown once.
	Note *string `json:"note"`
}

// checkNote refuses a note other than "": only the migration writes one.
func (in folderProjectInput) checkNote() *projectRefusal {
	if in.Note != nil && *in.Note != "" {
		return &projectRefusal{422, "invalid_note", "note", "The note can only be cleared.", ""}
	}
	return nil
}

func (in folderProjectInput) apply(p store.FolderProject) store.FolderProject {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	set(&p.Host, in.Host)
	set(&p.Dir, in.Dir)
	set(&p.Name, in.Name)
	set(&p.BaseBranch, in.BaseBranch)
	set(&p.WorktreeDefault, in.WorktreeDefault)
	set(&p.DefaultTool, in.DefaultTool)
	set(&p.DefaultModel, in.DefaultModel)
	set(&p.StarterPrompt, in.StarterPrompt)
	set(&p.Note, in.Note)
	if in.IsRepo != nil {
		p.IsRepo = *in.IsRepo
	}
	return p
}

const (
	sentenceFolderNotFound = "That project isn't on this account."
	sentenceFolderChanged  = "Someone changed this project. Reload it, then try again."
	sentenceFolderDir      = "The folder must be an absolute path starting with /, at most 4096 bytes, without control characters or line separators."
	sentenceFolderName     = "Give the project a name of 1 to 80 characters, without line breaks or control characters."
	sentenceFolderPrompt   = "The starter prompt must be valid text of at most 2,000 bytes, without control characters other than line breaks, and must not start with a dash."
	sentenceFolderMachine  = "That machine isn't enrolled on this account."
)

// folderRefusal maps every store error for folder projects to a code and one
// sentence. machine is the machine's name for project_exists (see
// machineLabel). Unknown errors are
// a 500 and are logged without request content.
func folderRefusal(err error, machine string) *projectRefusal {
	switch {
	case errors.Is(err, store.ErrFolderProjectNotFound):
		// The same code and sentence as a terminal create naming the project.
		return &projectRefusal{404, "project_not_found", "", sentenceFolderNotFound, ""}
	case errors.Is(err, store.ErrFolderProjectChanged):
		return &projectRefusal{409, "project_changed", "", sentenceFolderChanged, ""}
	case errors.Is(err, store.ErrFolderProjectExists):
		return &projectRefusal{409, "project_exists", "dir", machine + " already has a project for that folder.", ""}
	case errors.Is(err, store.ErrFolderProjectLimit):
		return &projectRefusal{409, "project_limit", "", fmt.Sprintf("You have %d projects, the most reet keeps. Remove one first.", store.MaxFolderProjects), ""}
	case errors.Is(err, store.ErrFolderProjectName):
		// The name's shape is checked before saving, so here it is a clash.
		return &projectRefusal{409, "name_taken", "name", "Another project on this account already has that name.", ""}
	case errors.Is(err, store.ErrFolderProjectInvalid):
		return &projectRefusal{422, "invalid_project", "", "Something in that project can't be saved. Check each field and try again.", ""}
	}
	log.Printf("folder projects: store error: %v", err)
	return &projectRefusal{500, "server_error", "", "reet couldn't do that just now. Try again.", ""}
}

// folderNameShape is the store's name rule: 1 to 80 characters of plain text.
func folderNameShape(name string) bool {
	if n := utf8.RuneCountInString(name); n < 1 || n > store.MaxProjectNameRunes || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return false
		}
	}
	return true
}

// machineNames maps each of the account's enrolled machines (its enrollment
// id, which a project's Host holds) to its current name.
func (s *server) machineNames(uid int64) (map[string]string, error) {
	agents, err := s.st.SyncAgentsFor(uid)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(agents))
	for _, a := range agents {
		names[a.ID] = a.Host
	}
	return names, nil
}

// machineLabel is the machine's name for a sentence, or "that machine" when
// it is no longer enrolled.
func machineLabel(names map[string]string, host string) string {
	if name := names[host]; name != "" {
		return name
	}
	return "that machine"
}

// checkFolderProject refuses, with a precise code and field, everything the
// store would refuse only as a general invalid project, and a machine that is
// not one of the account's enrolled machines. Host is the machine's
// enrollment id, matched exactly; p.Dir comes back cleaned. saved is the
// project being updated (nil on create): its own machine and folder stay
// acceptable after the machine is forgotten; anything else on it does not.
func (s *server) checkFolderProject(names map[string]string, p *store.FolderProject, saved *store.FolderProject) *projectRefusal {
	dir, err := store.CleanProjectDir(p.Dir)
	if err != nil {
		return &projectRefusal{422, "invalid_dir", "dir", sentenceFolderDir, ""}
	}
	p.Dir = dir
	if saved == nil || p.Host != saved.Host || p.Dir != saved.Dir {
		if _, enrolled := names[p.Host]; !enrolled {
			return &projectRefusal{422, "unknown_machine", "host", sentenceFolderMachine, ""}
		}
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name != "" && !folderNameShape(p.Name) {
		return &projectRefusal{422, "invalid_name", "name", sentenceFolderName, ""}
	}
	if p.BaseBranch != "" && !terminalproto.ValidRefName(p.BaseBranch) {
		return &projectRefusal{422, "invalid_branch", "baseBranch", "The base branch must be blank or a valid git branch name of at most 255 bytes.", ""}
	}
	switch p.WorktreeDefault {
	case "", "inherit", "yes", "no":
	default:
		return &projectRefusal{422, "invalid_worktree_default", "worktreeDefault", "Choose whether each session gets a worktree: inherit, yes or no.", ""}
	}
	if p.DefaultTool != "" && !terminalproto.ValidTool(p.DefaultTool) {
		return &projectRefusal{422, "invalid_tool", "defaultTool", sentenceToolChoice, ""}
	}
	if p.DefaultModel != "" && !terminalproto.ValidModel(p.DefaultModel) {
		return &projectRefusal{422, "invalid_model", "defaultModel", sentenceModel, ""}
	}
	// A saved prompt is checked again only when it changes, so one that
	// today's rules refuse does not block editing anything else.
	if p.StarterPrompt != "" && (saved == nil || p.StarterPrompt != saved.StarterPrompt) {
		if len(p.StarterPrompt) > terminalproto.MaxPromptBytes || strings.HasPrefix(p.StarterPrompt, "-") || !terminalproto.ValidPrompt(p.StarterPrompt) {
			return &projectRefusal{422, "invalid_prompt", "starterPrompt", sentenceFolderPrompt, ""}
		}
		if terminalproto.PromptNeedsWords(p.DefaultTool, p.StarterPrompt) {
			return &projectRefusal{422, "prompt_unsupported", "starterPrompt", "Write the starter prompt as at least two words.", ""}
		}
	}
	return nil
}

// folderProjectView is a project as the API shows it. StarterPrompt is set
// only by the single read, for its owner's edit form.
type folderProjectView struct {
	ID string `json:"id"`
	// Host is the machine's enrollment id (the agentId of browse, branches
	// and terminal create); Machine is its current name, "" once forgotten.
	Host             string  `json:"host"`
	Machine          string  `json:"machine"`
	Dir              string  `json:"dir"`
	Name             string  `json:"name"`
	IsRepo           bool    `json:"isRepo"`
	BaseBranch       string  `json:"baseBranch"`
	WorktreeDefault  string  `json:"worktreeDefault"`
	DefaultTool      string  `json:"defaultTool"`
	DefaultModel     string  `json:"defaultModel"`
	HasStarterPrompt bool    `json:"hasStarterPrompt"`
	StarterPrompt    *string `json:"starterPrompt,omitempty"`
	// Note is the migration's one-time notice: the saved Projects (by name)
	// with this folder whose shared prompts were not kept. "" when none.
	Note      string `json:"note"`
	Position  int    `json:"position"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	Version   int64  `json:"version"`
}

func viewFolderProject(p store.FolderProject, names map[string]string) folderProjectView {
	return folderProjectView{ID: p.ID, Host: p.Host, Machine: names[p.Host], Dir: p.Dir, Name: p.Name, IsRepo: p.IsRepo, BaseBranch: p.BaseBranch, WorktreeDefault: p.WorktreeDefault,
		DefaultTool: p.DefaultTool, DefaultModel: p.DefaultModel, HasStarterPrompt: p.StarterPrompt != "", Note: p.Note, Position: p.Position,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Version: p.Version}
}

// auditFolderProject records a change: the project, its machine (id, and
// name as host, as a terminal launch records them), folder and name. The
// starter prompt is never part of it.
func (s *server) auditFolderProject(uid int64, action string, p store.FolderProject, names map[string]string) {
	s.event(uid, "ui", action, "", map[string]any{"project": p.ID, "machine": p.Host, "host": names[p.Host], "folder": p.Dir, "name": p.Name})
}

func (s *server) writeFolderProjects(w http.ResponseWriter, uid int64) {
	projects, err := s.st.ListFolderProjectsFor(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	names, err := s.machineNames(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	waiting, err := s.st.SavedWaitingFor(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	writeJSON(w, http.StatusOK, s.folderListAnswer(projects, names, waiting, nil))
}

// folderListAnswer is the list answer: the projects in rail order and
// savedWaiting, how many saved Project repositories wait for room. extra
// adds fields (the import's counts and sentence).
func (s *server) folderListAnswer(projects []store.FolderProject, names map[string]string, waiting int, extra map[string]any) map[string]any {
	out := make([]folderProjectView, 0, len(projects))
	for _, p := range projects {
		out = append(out, viewFolderProject(p, names))
	}
	answer := map[string]any{"projects": out, "savedWaiting": waiting}
	for k, v := range extra {
		answer[k] = v
	}
	return answer
}

// folderProjectImportSaved brings in the account's saved Project
// repositories that waited for room (MigrateProjectsToFoldersFor), with the
// migration's rules and audit. The body is empty or {}. Nothing waiting, or
// still no room, is a 200 that creates nothing and says so.
func (s *server) folderProjectImportSaved(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFolderProjectBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeErrCode(w, http.StatusRequestEntityTooLarge, "too_large", "", "That request is too large for a project.", "")
		return
	}
	if strings.TrimSpace(string(raw)) != "" {
		r.Body = io.NopCloser(strings.NewReader(string(raw)))
		var in struct{}
		if !folderProjectBody(w, r, &in) {
			return
		}
	}
	uid := s.uid(r)
	rep, err := s.st.MigrateProjectsToFoldersFor(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	projects, err := s.st.ListFolderProjectsFor(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	names, err := s.machineNames(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	waiting, err := s.st.SavedWaitingFor(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	var message string
	switch {
	case rep.Created > 0 && waiting > 0:
		message = fmt.Sprintf("Added %s as projects. %s still waiting for room.", savedRepositories(rep.Created), savedRepositoriesAre(waiting))
	case rep.Created > 0:
		message = fmt.Sprintf("Added %s as projects.", savedRepositories(rep.Created))
	case waiting > 0:
		message = fmt.Sprintf("%s waiting for room. Remove a project, then try again.", savedRepositoriesAre(waiting))
	default:
		message = "Nothing is waiting to be added."
	}
	writeJSON(w, http.StatusOK, s.folderListAnswer(projects, names, waiting, map[string]any{"created": rep.Created, "skipped": rep.Skipped, "message": message}))
}

func savedRepositories(n int) string {
	if n == 1 {
		return "1 saved repository"
	}
	return fmt.Sprintf("%d saved repositories", n)
}

func savedRepositoriesAre(n int) string {
	if n == 1 {
		return "1 saved repository is"
	}
	return fmt.Sprintf("%d saved repositories are", n)
}

func (s *server) folderProjectList(w http.ResponseWriter, r *http.Request) {
	s.writeFolderProjects(w, s.uid(r))
}

func (s *server) folderProjectGet(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	p, err := s.st.FolderProjectFor(uid, r.PathValue("id"))
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	names, err := s.machineNames(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	v := viewFolderProject(p, names)
	prompt := p.StarterPrompt
	v.StarterPrompt = &prompt
	w.Header().Set("ETag", projectETag(p.Version))
	writeJSON(w, http.StatusOK, v)
}

func (s *server) folderProjectCreate(w http.ResponseWriter, r *http.Request) {
	var in folderProjectInput
	if !folderProjectBody(w, r, &in) {
		return
	}
	uid := s.uid(r)
	names, err := s.machineNames(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	if refusal := in.checkNote(); refusal != nil {
		refusal.write(w)
		return
	}
	p := in.apply(store.FolderProject{})
	if refusal := s.checkFolderProject(names, &p, nil); refusal != nil {
		refusal.write(w)
		return
	}
	saved, err := s.st.CreateFolderProjectFor(uid, p)
	if err != nil {
		folderRefusal(err, machineLabel(names, p.Host)).write(w)
		return
	}
	s.auditFolderProject(uid, "project-add", saved, names)
	w.Header().Set("ETag", projectETag(saved.Version))
	writeJSON(w, http.StatusCreated, viewFolderProject(saved, names))
}

func folderVersionRefused(w http.ResponseWriter) {
	writeErrCode(w, http.StatusBadRequest, "invalid_version", "", "The If-Match header must be the project's version number.", "")
}

// folderProjectUpdate merges the body into the saved project and saves it.
// With If-Match the store saves only if the stored version is still that one
// (409 project_changed otherwise); without it the last write wins, as with
// Projects. A project removed meanwhile is 404 and is not recreated.
func (s *server) folderProjectUpdate(w http.ResponseWriter, r *http.Request) {
	var in folderProjectInput
	if !folderProjectBody(w, r, &in) {
		return
	}
	expect, ok := ifMatch(r)
	if !ok {
		folderVersionRefused(w)
		return
	}
	uid := s.uid(r)
	current, err := s.st.FolderProjectFor(uid, r.PathValue("id"))
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	names, err := s.machineNames(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	if refusal := in.checkNote(); refusal != nil {
		refusal.write(w)
		return
	}
	p := in.apply(current)
	if refusal := s.checkFolderProject(names, &p, &current); refusal != nil {
		refusal.write(w)
		return
	}
	saved, err := s.st.UpdateFolderProjectFor(uid, p, expect)
	if err != nil {
		folderRefusal(err, machineLabel(names, p.Host)).write(w)
		return
	}
	s.auditFolderProject(uid, "project-update", saved, names)
	w.Header().Set("ETag", projectETag(saved.Version))
	writeJSON(w, http.StatusOK, viewFolderProject(saved, names))
}

// folderProjectDelete removes the registry entry only; sessions and worktrees
// stay on the machine. If-Match works as for an update.
func (s *server) folderProjectDelete(w http.ResponseWriter, r *http.Request) {
	expect, ok := ifMatch(r)
	if !ok {
		folderVersionRefused(w)
		return
	}
	uid := s.uid(r)
	id := r.PathValue("id")
	p, err := s.st.FolderProjectFor(uid, id)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	names, err := s.machineNames(uid)
	if err != nil {
		folderRefusal(err, "").write(w)
		return
	}
	if err := s.st.DeleteFolderProjectFor(uid, id, expect); err != nil {
		folderRefusal(err, machineLabel(names, p.Host)).write(w)
		return
	}
	s.auditFolderProject(uid, "project-remove", p, names)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Project removed. Its sessions and any worktrees stay on " + machineLabel(names, p.Host) + "."})
}

// folderProjectReorder puts the account's projects in the order given. ids
// names every project once and versions[i] is the version read for ids[i]; a
// stale or partial view (including one naming a project another tab
// removed) is 409 project_changed and nothing moves. It answers
// the list in its new order, with the new versions.
func (s *server) folderProjectReorder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs      []string `json:"ids"`
		Versions []int64  `json:"versions"`
	}
	if !folderProjectBody(w, r, &in) {
		return
	}
	if in.IDs == nil || len(in.IDs) != len(in.Versions) || len(in.IDs) > store.MaxFolderProjects {
		writeErrCode(w, http.StatusUnprocessableEntity, "invalid_order", "ids", "List every project once, each with the version you read.", "")
		return
	}
	uid := s.uid(r)
	if err := s.st.ReorderFolderProjectsFor(uid, in.IDs, in.Versions); err != nil {
		if errors.Is(err, store.ErrFolderProjectInvalid) {
			writeErrCode(w, http.StatusUnprocessableEntity, "invalid_order", "ids", "List every project once, each with the version you read.", "")
			return
		}
		folderRefusal(err, "").write(w)
		return
	}
	s.writeFolderProjects(w, uid)
}
