package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// maxProjectBodyBytes bounds a Project request: twelve folders of the longest
// allowed length, a full shared prompt and JSON escaping fit well inside it.
const maxProjectBodyBytes = 128 << 10

func (s *server) registerProjectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/projects", s.projectAccess(s.projectList))
	// Saved Projects moved into folder projects (MigrateProjectsToFolders):
	// they can no longer be made, changed or deleted, so the two cannot
	// drift apart. Reading them, their runs, starting and retrying a run and
	// removing a run's worktree keep working until runs move to the rail.
	mux.HandleFunc("POST /api/v2/projects", s.projectAccess(s.projectsMoved))
	mux.HandleFunc("GET /api/v2/projects/{id}", s.projectAccess(s.projectGet))
	mux.HandleFunc("PATCH /api/v2/projects/{id}", s.projectAccess(s.projectsMoved))
	mux.HandleFunc("DELETE /api/v2/projects/{id}", s.projectAccess(s.projectsMoved))
	mux.HandleFunc("GET /api/v2/projects/{id}/runs", s.projectAccess(s.projectRuns))
	// A run lives under its own prefix: /projects/runs/{runId} would clash
	// with /projects/{id}/runs in the mux (both match /projects/runs/runs).
	mux.HandleFunc("GET /api/v2/project-runs/{runId}", s.projectAccess(s.projectRun))
}

// accessWords are the three refusal sentences of featureAccess, naming the
// feature the routes belong to.
type accessWords struct{ accounts, signedOut, readOnly string }

var projectsAccessWords = accessWords{
	accounts:  "Projects need an account signed in through the browser.",
	signedOut: "Sign in to use Projects.",
	readOnly:  "This account is read-only, so it can't use Projects.",
}

// projectAccess admits only a signed-in browser account that may write, as
// terminals do: Projects start agents on the person's machines. API tokens,
// single-token and open modes, and viewers are refused; every change must come
// from the canonical origin.
func (s *server) projectAccess(next http.HandlerFunc) http.HandlerFunc {
	return s.featureAccess(projectsAccessWords, next)
}

// featureAccess is projectAccess with the feature's own refusal sentences.
func (s *server) featureAccess(words accessWords, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if s.authMode != AuthAccounts || requestToken(r) != "" {
			writeErrCode(w, http.StatusForbidden, "accounts_required", "", words.accounts, "")
			return
		}
		u, ok := s.sessionUser(r)
		if !ok {
			writeErrCode(w, http.StatusUnauthorized, "signed_out", "", words.signedOut, "")
			return
		}
		if u.Role == "viewer" {
			writeErrCode(w, http.StatusForbidden, "read_only", "", words.readOnly, "")
			return
		}
		if r.Method != http.MethodGet && !s.terminalOrigin(r) {
			writeErrCode(w, http.StatusForbidden, "origin_refused", "", "That request didn't come from this reet's own address.", "")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, u)))
	}
}

// projectBody decodes exactly one JSON object with no unknown fields.
func projectBody(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxProjectBodyBytes)
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
		writeErrCode(w, http.StatusRequestEntityTooLarge, "too_large", "", "That request is too large for a Project.", "")
	} else {
		writeErrCode(w, http.StatusBadRequest, "invalid_request", "", "That request isn't a Project reet can read.", "")
	}
	return false
}

// projectRefusal is a refusal with a stable code and one plain sentence.
// Statuses: 422 for a field the person can correct in the form (including
// member_limit and duplicate_member, which are about this request's list);
// 409 for a conflict with what is already saved (name_taken, project_limit,
// which depends on the account's other Projects, and project_changed).
type projectRefusal struct {
	status         int
	code, field    string
	sentence, hint string
}

func (e projectRefusal) write(w http.ResponseWriter) {
	writeErrCode(w, e.status, e.code, e.field, e.sentence, e.hint)
}

const (
	sentenceToolChoice = "Choose one of the agents reet can start."
	sentenceModel      = "That model name isn't one reet can pass on. Use letters, digits and . _ : / @ + -, up to 100 characters, starting with a letter or digit."
	sentenceTwoWords   = "Write the opening prompt as at least two words."
	sentenceNotFound   = "That Project isn't on this account."
	sentenceRunMissing = "That run isn't on this account."
	sentenceChanged    = "Someone changed this Project. Reload it and try again."
)

func sentenceDir(n int) string {
	return fmt.Sprintf("Repository %d: the folder must be an absolute path starting with /, at most %d bytes, without control characters or line separators.", n, terminalproto.MaxDirBytes)
}

// storeRefusal maps every store error for Projects and runs to a stable code
// and a plain sentence. Unknown errors are a 500 and are logged without
// request content; store errors never carry the prompt.
func storeRefusal(err error) *projectRefusal {
	var member *store.ProjectMemberError
	if errors.As(err, &member) && member.Field == "dir" {
		return &projectRefusal{422, "invalid_dir", fmt.Sprintf("members.%d.dir", member.Index), sentenceDir(member.Index + 1), ""}
	}
	switch {
	case errors.Is(err, store.ErrProjectStale):
		return &projectRefusal{409, "project_changed", "", sentenceChanged, ""}
	case errors.Is(err, store.ErrProjectNotFound):
		return &projectRefusal{404, "not_found", "", sentenceNotFound, ""}
	case errors.Is(err, store.ErrProjectNameTaken):
		return &projectRefusal{409, "name_taken", "name", "Another Project on this account already has that name.", ""}
	case errors.Is(err, store.ErrProjectName):
		return &projectRefusal{422, "invalid_name", "name", fmt.Sprintf("Give the Project a name of 1 to %d characters, without line breaks or control characters.", store.MaxProjectNameRunes), ""}
	case errors.Is(err, store.ErrProjectLimit):
		return &projectRefusal{409, "project_limit", "", fmt.Sprintf("This account already has %d Projects, the most it can have. Delete one first.", store.MaxProjectsPerAccount), ""}
	case errors.Is(err, store.ErrMemberLimit):
		return &projectRefusal{422, "member_limit", "members", fmt.Sprintf("A Project can have at most %d repositories.", store.MaxProjectMembers), ""}
	case errors.Is(err, store.ErrDuplicateMember):
		return &projectRefusal{422, "duplicate_member", "members", "Each machine and folder can be in a Project only once.", ""}
	case errors.Is(err, store.ErrProjectInvalid):
		return &projectRefusal{422, "invalid_project", "", "Something in that Project can't be saved. Check each field and try again.", ""}
	case errors.Is(err, store.ErrRunNotFound):
		return &projectRefusal{404, "run_not_found", "", sentenceRunMissing, ""}
	case errors.Is(err, store.ErrRunNoMembers):
		return &projectRefusal{422, "no_members", "members", "Choose at least one repository to start.", ""}
	case errors.Is(err, store.ErrRunMemberNotPending):
		return &projectRefusal{409, "member_not_pending", "", "That repository isn't waiting to start, or is already being started.", ""}
	case errors.Is(err, store.ErrRunMemberTransition):
		return &projectRefusal{409, "member_state", "", "That repository can't move to that state.", ""}
	case errors.Is(err, store.ErrRunMemberStale):
		return &projectRefusal{409, "member_stale", "", "Something changed. Check again.", ""}
	}
	log.Printf("projects: store error: %v", err)
	return &projectRefusal{500, "server_error", "", "reet couldn't do that just now. Try again.", ""}
}

type projectMemberView struct {
	Host     string `json:"host"`
	Dir      string `json:"dir"`
	Tool     string `json:"tool"`
	Model    string `json:"model"`
	Position int    `json:"position"`
}

type projectRunSummary struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	StartedAt int64  `json:"startedAt"`
}

// projectView is a Project as the API shows it. SharedPrompt is set only by
// the single-Project read, for its owner's edit form.
type projectView struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	DefaultTool     string              `json:"defaultTool"`
	DefaultModel    string              `json:"defaultModel"`
	UseWorktrees    bool                `json:"useWorktrees"`
	HasSharedPrompt bool                `json:"hasSharedPrompt"`
	SharedPrompt    *string             `json:"sharedPrompt,omitempty"`
	Members         []projectMemberView `json:"members"`
	CreatedAt       int64               `json:"createdAt"`
	UpdatedAt       int64               `json:"updatedAt"`
	Version         int64               `json:"version"`
	LastRun         *projectRunSummary  `json:"lastRun,omitempty"`
}

func viewProject(p store.Project) projectView {
	v := projectView{ID: p.ID, Name: p.Name, DefaultTool: p.DefaultTool, DefaultModel: p.DefaultModel, UseWorktrees: p.UseWorktrees,
		HasSharedPrompt: p.SharedPrompt != "", Members: make([]projectMemberView, 0, len(p.Members)), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Version: p.Version}
	for _, m := range p.Members {
		v.Members = append(v.Members, projectMemberView{Host: m.Host, Dir: m.Dir, Tool: m.Tool, Model: m.Model, Position: m.Position})
	}
	return v
}

type runMemberView struct {
	Position       int    `json:"position"`
	Host           string `json:"host"`
	Dir            string `json:"dir"`
	Tool           string `json:"tool"`
	Model          string `json:"model"`
	State          string `json:"state"`
	Code           string `json:"code"`
	Sentence       string `json:"sentence"`
	SessionID      string `json:"sessionId"`
	WorktreePath   string `json:"worktreePath"`
	WorktreeRepo   string `json:"worktreeRepo"`
	WorktreeBranch string `json:"worktreeBranch"`
	// WorktreeRun and WorktreeProject, with Host and WorktreeRepo, are what
	// the worktree status and remove routes take (agentId, run, project,
	// repo). WorktreeRun is empty for a run without worktrees.
	WorktreeRun     string `json:"worktreeRun"`
	WorktreeProject string `json:"worktreeProject"`
	// SharesCheckout is true when the run made no worktrees: the agent works
	// in the saved folder itself, which any other run or session there shares.
	SharesCheckout bool `json:"sharesCheckout"`
	Attempt        int  `json:"attempt"`
}

// runView is a run as the API shows it. It has no prompt field of any kind.
type runView struct {
	ID           string `json:"id"`
	ProjectID    string `json:"projectId"`
	ProjectName  string `json:"projectName"`
	State        string `json:"state"`
	StartedAt    int64  `json:"startedAt"`
	UseWorktrees bool   `json:"useWorktrees"`
	// WorktreeProject is the Project slug the run's worktrees were made under,
	// from the name when the run started (a later rename does not change it).
	WorktreeProject string          `json:"worktreeProject"`
	Members         []runMemberView `json:"members"`
}

// runMemberSentences are the plain sentences for a failed member's code: the
// ones the single-repository dialog shows, and the worktree ones.
var runMemberSentences = func() map[string]string {
	out := map[string]string{
		"launch_unsupported":   "Update reet on that machine to start agents.",
		"tool_not_installed":   "That agent isn't installed on that machine.",
		"folder_unavailable":   "That folder isn't available on that machine.",
		"prompt_unsupported":   sentenceTwoWords,
		"worktree_unsupported": "Update reet on that machine to start agents with worktrees.",
		"git_failed":           "Git couldn't make a worktree there.",
		"worktree_mismatch":    "That machine's answer did not match the request. Check again.",
		"session_limit":        "The terminal session limit has been reached. End a session and try again.",
		"machine_unavailable":  "That machine is reconnecting. Try again in a moment.",
		"unknown_machine":      "That machine isn't enrolled on this account.",
		// The codes only a run's orchestrator stores (internal/projectrun).
		"unknown_outcome":    "We didn't hear back, so the agent may have started. Check your sessions before retrying.",
		"session_lost":       "That machine lost the session before it started. Start it again.",
		"machine_refused":    "That machine refused to start the agent. Check the machine, then try again.",
		"terminals_disabled": sentenceTerminalsOff,
		"worktree_refused":   "That machine refused to make a worktree for this request. Start it again to see why.",
		"invalid_launch":     "reet couldn't send that start request. Check the Project's settings and try again.",
		"internal_error":     "Something went wrong on our side. Check your sessions, then retry.",
	}
	for _, r := range worktreeRefusals {
		out[r.code] = r.sentence
	}
	return out
}()

func runMemberSentence(state, code string) string {
	if state != store.RunMemberFailed {
		return ""
	}
	if s, ok := runMemberSentences[code]; ok {
		return s
	}
	return "Something changed. Check again."
}

func viewRun(r store.Run) runView {
	v := runView{ID: r.ID, ProjectID: r.ProjectID, ProjectName: r.ProjectName, State: r.State, StartedAt: r.CreatedAt, UseWorktrees: r.UseWorktrees,
		Members: make([]runMemberView, 0, len(r.Members))}
	if r.UseWorktrees {
		v.WorktreeProject = terminalproto.Slug(r.ProjectName, "project")
	}
	for _, m := range r.Members {
		v.Members = append(v.Members, runMemberView{Position: m.Position, Host: m.Host, Dir: m.Dir, Tool: m.Tool, Model: m.Model, State: m.State,
			Code: m.Code, Sentence: runMemberSentence(m.State, m.Code), SessionID: m.SessionID, WorktreePath: m.WorktreePath,
			WorktreeRepo: m.WorktreeRepo, WorktreeBranch: m.WorktreeBranch, WorktreeRun: m.WorktreeToken, WorktreeProject: v.WorktreeProject,
			SharesCheckout: !r.UseWorktrees, Attempt: m.Attempt})
	}
	return v
}

func (s *server) projectList(w http.ResponseWriter, r *http.Request) {
	uid := s.uid(r)
	projects, err := s.st.ListProjectsFor(uid)
	if err != nil {
		storeRefusal(err).write(w)
		return
	}
	out := make([]projectView, 0, len(projects))
	for _, p := range projects {
		v := viewProject(p)
		runs, err := s.st.RunsFor(uid, p.ID, 1)
		if err != nil {
			storeRefusal(err).write(w)
			return
		}
		if len(runs) > 0 {
			v.LastRun = &projectRunSummary{ID: runs[0].ID, State: runs[0].State, StartedAt: runs[0].CreatedAt}
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

func (s *server) projectGet(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.ProjectFor(s.uid(r), r.PathValue("id"))
	if err != nil {
		storeRefusal(err).write(w)
		return
	}
	v := viewProject(p)
	prompt := p.SharedPrompt
	v.SharedPrompt = &prompt
	w.Header().Set("ETag", projectETag(p.Version))
	writeJSON(w, http.StatusOK, v)
}

func projectETag(version int64) string { return `"` + strconv.FormatInt(version, 10) + `"` }

// ifMatch reads an optional If-Match header: the Project version the editor
// read, bare or quoted. Absent means no precondition (0).
func ifMatch(r *http.Request) (int64, bool) {
	raw := strings.TrimSpace(r.Header.Get("If-Match"))
	if raw == "" {
		return 0, true
	}
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	return v, err == nil && v > 0
}

// projectsMoved answers a create, change or delete of a saved Project: they
// now live in the rail on Home as folder projects. Nothing is read or saved.
func (s *server) projectsMoved(w http.ResponseWriter, r *http.Request) {
	writeErrCode(w, http.StatusGone, "projects_moved", "", "Projects now live in the rail on Home.", "")
}

// projectRuns lists a Project's newest 20 runs, newest first.
func (s *server) projectRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.st.RunsFor(s.uid(r), r.PathValue("id"), store.MaxRunsListed)
	if err != nil {
		storeRefusal(err).write(w)
		return
	}
	out := make([]runView, 0, len(runs))
	for _, run := range runs {
		out = append(out, viewRun(run))
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": out})
}

// projectRun reads one run, which stays readable after its Project is deleted.
func (s *server) projectRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.st.RunFor(s.uid(r), r.PathValue("runId"))
	if err != nil {
		storeRefusal(err).write(w)
		return
	}
	writeJSON(w, http.StatusOK, viewRun(run))
}
