package api

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (s *server) registerSessionBacklogRoutes(mux *http.ServeMux) {
	access := func(h http.HandlerFunc) http.HandlerFunc {
		return s.featureAccess(accessWords{
			accounts: "Using the session backlog needs a browser account.", signedOut: "Sign in to use your session backlog.", readOnly: "This account is read-only, so it can't manage a session backlog.",
		}, h)
	}
	mux.HandleFunc("GET /api/v2/session-backlog", access(s.sessionBacklogList))
	mux.HandleFunc("POST /api/v2/session-backlog", access(s.folderProjectWrite(s.sessionBacklogSave)))
	mux.HandleFunc("GET /api/v2/session-backlog/{id}", access(s.sessionBacklogGet))
	mux.HandleFunc("PUT /api/v2/session-backlog/{id}", access(s.folderProjectWrite(s.sessionBacklogSave)))
	mux.HandleFunc("DELETE /api/v2/session-backlog/{id}", access(s.folderProjectWrite(s.sessionBacklogDelete)))
	mux.HandleFunc("POST /api/v2/session-backlog/{id}/start", s.terminal(s.folderProjectWrite(s.sessionBacklogStart)))
}

func backlogBody(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 15<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(out)
	if err == nil && d.Decode(new(any)) == io.EOF {
		return true
	}
	writeJSON(w, 400, map[string]string{"error": "That backlog request is invalid or too large."})
	return false
}

func backlogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeJSON(w, 404, map[string]string{"error": "That backlog item isn't on this account."})
	case errors.Is(err, store.ErrDraftConflict), errors.Is(err, store.ErrDraftLimit):
		writeJSON(w, 409, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, 500, map[string]string{"error": "Couldn't read or save the backlog just now."})
	}
}

func (s *server) sessionBacklogList(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.SessionDraftsFor(s.uid(r))
	if err != nil {
		backlogError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"drafts": list})
}

func (s *server) sessionBacklogGet(w http.ResponseWriter, r *http.Request) {
	d, err := s.st.SessionDraftFor(s.uid(r), r.PathValue("id"))
	if err != nil {
		backlogError(w, err)
		return
	}
	d.Starting = d.PreparedPrompt != ""
	writeJSON(w, 200, d)
}

type backlogInput struct {
	Title     string             `json:"title"`
	Prompt    string             `json:"prompt"`
	AgentID   string             `json:"agentId"`
	ProjectID string             `json:"projectId"`
	Dir       string             `json:"dir"`
	Tool      string             `json:"tool"`
	Model     string             `json:"model"`
	Effort    string             `json:"effort"`
	Idea      bool               `json:"idea"`
	Yolo      bool               `json:"yolo"`
	Worktree  bool               `json:"worktree"`
	Base      string             `json:"base"`
	Chat      bool               `json:"chat"`
	Trust     bool               `json:"trust"`
	Images    []store.DraftImage `json:"images"`
	Version   int64              `json:"version"`
}

func (s *server) sessionBacklogSave(w http.ResponseWriter, r *http.Request) {
	var in backlogInput
	if !backlogBody(w, r, &in) {
		return
	}
	d := store.SessionDraft{ID: r.PathValue("id"), Title: strings.TrimSpace(in.Title), Prompt: in.Prompt, AgentID: in.AgentID, ProjectID: in.ProjectID, Dir: in.Dir, Tool: in.Tool, Model: strings.TrimSpace(in.Model), Effort: in.Effort, Idea: in.Idea, Yolo: in.Yolo, Worktree: in.Worktree, Base: strings.TrimSpace(in.Base), Chat: in.Chat, Trust: in.Trust, Images: in.Images, Version: in.Version}
	bad := func(message string) { writeJSON(w, 422, map[string]string{"error": message}) }
	emptyIdea := d.Title == "" && d.Prompt == ""
	if d.Idea && emptyIdea {
		bad("Give the idea a title or a note.")
		return
	}
	if !terminalproto.ValidTitle(d.Title) {
		bad("Use a session title of 1 to 80 plain characters.")
		return
	}
	validEffort := terminalproto.ValidEffort(d.Effort)
	if d.Chat {
		validEffort = d.Effort == "" || terminalproto.ValidModel(d.Effort)
	}
	validTool := terminalproto.ValidToolV2(d.Tool) && validEffort
	validModel := d.Model == "" || terminalproto.ValidModel(d.Model)
	if !validTool || !validModel {
		bad("Choose a supported agent, model and effort.")
		return
	}
	if !(d.Idea && d.Prompt == "") && !terminalproto.ValidPrompt(d.Prompt) {
		bad(fmt.Sprintf("Write a prompt of at most %d bytes without control characters.", terminalproto.MaxPromptBytes))
		return
	}
	if d.Dir != "" && !terminalproto.ValidDir(d.Dir) {
		bad("Use an absolute folder path without control characters.")
		return
	}
	machines, err := s.st.SyncAgentsFor(s.uid(r))
	if err != nil {
		backlogError(w, err)
		return
	}
	found := false
	for _, m := range machines {
		if m.ID == d.AgentID {
			found = true
		}
	}
	if !found {
		bad("Choose a machine on this account.")
		return
	}
	if d.ProjectID != "" {
		p, err := s.st.FolderProjectFor(s.uid(r), d.ProjectID)
		matches := err == nil && p.Host == d.AgentID && p.Dir == d.Dir
		if !matches {
			bad("Choose the project's own machine and folder.")
			return
		}
	}
	baseWithoutWorktree := d.Base != "" && !d.Worktree
	invalidBase := d.Base != "" && !terminalproto.ValidRefName(d.Base)
	if baseWithoutWorktree || invalidBase {
		bad("Choose a base branch only with a worktree, and use a valid branch name.")
		return
	}
	chatTool := d.Chat && !terminalproto.ReviewedChatTool(d.Tool)
	trustWithoutChat := d.Trust && (!d.Chat || d.Tool != "opencode")
	if chatTool || trustWithoutChat {
		bad("Choose an agent with Gaffer support. Plugin trust applies only to an OpenCode chat.")
		return
	}
	if len(d.Images) > 4 {
		bad("Keep at most four images in a backlog item.")
		return
	}
	total := 0
	for _, im := range d.Images {
		header, encoded, ok := strings.Cut(im.Data, ",")
		mime := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
		validMime := mime == "image/png" || mime == "image/jpeg" || mime == "image/gif" || mime == "image/webp"
		validHeader := ok && header == "data:"+mime+";base64"
		if !validHeader || !validMime {
			bad("Attach PNG, JPEG, GIF or WebP images.")
			return
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		total += len(data)
		validSize := len(data) > 0 && len(data) <= 5<<20 && total <= 10<<20
		validMeta := http.DetectContentType(data) == mime && len(im.Name) <= 255
		validImage := validSize && validMeta
		if err != nil || !validImage {
			bad("Images must match their type, be at most 5 MB each and 10 MB together.")
			return
		}
	}
	create := r.Method == http.MethodPost
	if !create && in.Version < 1 {
		bad("Reload the backlog item before saving.")
		return
	}
	d, err = s.st.SaveSessionDraftFor(s.uid(r), d, create)
	if err != nil {
		backlogError(w, err)
		return
	}
	status := 200
	if create {
		status = 201
	}
	writeJSON(w, status, d)
}

func (s *server) sessionBacklogDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version int64 `json:"version"`
	}
	if !backlogBody(w, r, &in) {
		return
	}
	if err := s.st.DeleteSessionDraftFor(s.uid(r), r.PathValue("id"), in.Version); err != nil {
		backlogError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// Freeze before dispatch. All browsers and retries dispatch the frozen payload,
// including the exact uploaded image references. The normal terminal handler
// retains ownership, origin, capability and project gates and its audit behavior.
func (s *server) sessionBacklogStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version int64  `json:"version"`
		Prompt  string `json:"prompt"`
	}
	if !backlogBody(w, r, &in) {
		return
	}
	if !terminalproto.ValidPrompt(in.Prompt) {
		writeJSON(w, 422, map[string]string{"error": fmt.Sprintf("The prompt including image paths must fit in %d bytes.", terminalproto.MaxPromptBytes)})
		return
	}
	key := fnv.New32a()
	_, _ = key.Write([]byte(r.PathValue("id")))
	lock := &s.backlogStarts[key.Sum32()%uint32(len(s.backlogStarts))]
	lock.Lock()
	defer lock.Unlock()
	// An idea is only a placeholder; it must gain a prompt (and lose the flag) before it can start.
	cur, curErr := s.st.SessionDraftFor(s.uid(r), r.PathValue("id"))
	if curErr == nil && cur.Idea {
		writeJSON(w, 422, map[string]string{"error": "This is an idea. Add a prompt and switch off Idea before starting it.", "code": "idea_not_startable"})
		return
	}
	// The worktree slug is resolved once and frozen with the prompt; a retry
	// must not re-read a project that was renamed or changed since.
	wtProject := ""
	worktreeProject := curErr == nil && cur.Worktree
	hasChatProject := !cur.Chat && cur.ProjectID != ""
	if worktreeProject && hasChatProject {
		if p, err := s.st.FolderProjectFor(s.uid(r), cur.ProjectID); err == nil && p.IsRepo {
			wtProject = terminalproto.Slug(p.Name, "project")
		}
	}
	d, err := s.st.PrepareSessionDraftFor(s.uid(r), r.PathValue("id"), in.Version, in.Prompt, wtProject)
	if err != nil {
		backlogError(w, err)
		return
	}
	if d.SessionID != "" && !s.terminalCreateRecorded(s.uid(r), d.AgentID, d.ID) {
		writeJSON(w, 404, map[string]string{"error": "That session is no longer available. Create a new backlog item to start new work."})
		return
	}
	launch := map[string]any{"agentId": d.AgentID, "requestId": d.ID, "title": d.Title, "cols": 100, "rows": 30, "tool": d.Tool, "model": d.Model, "effort": d.Effort, "dir": d.Dir, "projectId": d.ProjectID, "prompt": d.PreparedPrompt}
	// YOLO is checked by the normal create path (account setting, machine and
	// agent) and audited there, exactly as for an active launch.
	if d.Yolo {
		launch["yolo"] = true
	}
	if d.Chat {
		launch["structured"] = true
		if d.Trust {
			launch["trust"] = true
		}
	} else if d.PreparedProject != "" && terminalproto.ValidRunToken(d.ID) {
		// The draft ID is the stable run token, so a retried start adopts the
		// worktree the machine already made.
		launch["worktree"] = d.ID
		launch["project"] = d.PreparedProject
		if d.Base != "" {
			launch["base"] = d.Base
		}
	}
	body, _ := json.Marshal(launch)
	next := r.Clone(r.Context())
	next.Body = io.NopCloser(bytes.NewReader(body))
	next.ContentLength = int64(len(body))
	response := &statusWriter{ResponseWriter: w}
	s.terminalCreate(response, next)
	// A refusal before the broker reserved anything is safe to edit again.
	// Lost replies with a recorded operation stay frozen and retry identically.
	if response.status >= 400 {
		_ = s.st.ReleaseSessionDraftFor(s.uid(r), d.ID, d.Version)
	}
	_ = s.st.CompleteSessionDraftFor(s.uid(r), d.ID)
}
