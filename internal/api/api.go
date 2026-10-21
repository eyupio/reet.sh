// Package api exposes the JSON API and the bookmarklet endpoint.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

type server struct {
	st      *store.Store
	f       meta.Fetcher
	token   string
	version string
	commit  string
	debug   bool
	topics  bool
	queue   chan ref.Ref
}

// Option configures the server.
type Option func(*server)

// WithVersion sets the build version and commit reported by /api/version and /healthz.
func WithVersion(version, commit string) Option {
	return func(s *server) {
		if version != "" {
			s.version = version
		}
		if commit != "" {
			s.commit = commit
		}
	}
}

// WithDebug enables per-fetch debug logging.
func WithDebug(debug bool) Option {
	return func(s *server) { s.debug = debug }
}

// WithTopics enables the daily skills.sh topic seed.
func WithTopics(enabled bool) Option {
	return func(s *server) { s.topics = enabled }
}

// New returns the API handler. An empty token disables authentication.
func New(st *store.Store, f meta.Fetcher, token string, opts ...Option) http.Handler {
	s := &server{st: st, f: f, token: token, version: "dev", commit: "unknown", queue: make(chan ref.Ref, 256)}
	for _, opt := range opts {
		opt(s)
	}
	go s.worker()
	if s.topics {
		go s.topicWorker()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/version", s.buildInfo)
	mux.HandleFunc("GET /api/skills", s.list)
	mux.HandleFunc("POST /api/skills", s.add)
	mux.HandleFunc("PATCH /api/skill", s.patch)
	mux.HandleFunc("DELETE /api/skill", s.remove)
	mux.HandleFunc("POST /api/skill/refresh", s.refresh)
	mux.HandleFunc("POST /api/skill/resolve", s.resolve)
	mux.HandleFunc("POST /api/skill/tags", s.skillTags)
	mux.HandleFunc("POST /api/skill/install-copy", s.installCopy)
	mux.HandleFunc("DELETE /api/skill/ignore", s.unignore)
	mux.HandleFunc("POST /api/import", s.importSkills)
	mux.HandleFunc("GET /api/environments", s.environments)
	mux.HandleFunc("GET /api/tags", s.tags)
	mux.HandleFunc("PATCH /api/tag", s.renameTag)
	mux.HandleFunc("POST /api/tags/merge", s.mergeTags)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/ignored", s.ignored)
	mux.HandleFunc("GET /api/export", s.export)
	mux.HandleFunc("POST /api/restore", s.restore)
	mux.HandleFunc("GET /quick", s.quick)
	return s.logRequests(s.auth(mux))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// logRequests records one line per write, never the query string (which can carry a token).
func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.Path != "/quick" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		source := "ui"
		switch {
		case r.URL.Path == "/quick":
			source = "quick"
		case r.Header.Get("X-Reet-Source") == "scan":
			source = "scan"
		}
		log.Printf("request method=%s path=%q status=%d duration=%s source=%s",
			r.Method, r.URL.Path, status, time.Since(start).Round(time.Millisecond), source)
	})
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	n, err := s.st.Health(ctx)
	if err != nil {
		log.Printf("health: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "version": s.version, "skills": n})
}

func (s *server) buildInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"version": s.version, "commit": s.commit})
}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead {
			// Stops other websites from writing to a token-less instance through the visitor's browser.
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				writeErr(w, http.StatusForbidden, "cross-site requests are not allowed")
				return
			}
			if (r.Method == http.MethodPost || r.Method == http.MethodPatch) &&
				!strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
				writeErr(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				return
			}
		}
		if s.token != "" && r.URL.Path != "/api/version" && (strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/quick") {
			got := r.Header.Get("X-Reet-Token")
			if b, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
				got = b
			}
			if got == "" {
				got = r.URL.Query().Get("t")
			}
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				writeErr(w, http.StatusUnauthorized, "missing or wrong token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) worker() {
	for r := range s.queue {
		s.fetchInto(r)
	}
}

func (s *server) fetchInto(r ref.Ref) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := s.f.Fetch(ctx, r)
	if m.Empty() {
		if s.debug {
			log.Printf("meta fetch id=%s result=empty", r.ID)
		}
		return
	}
	if err := s.st.SetMeta(r.ID, m.Name, m.Description, m.Installs, m.InstallsWeek, m.Trend); err != nil {
		if s.debug {
			log.Printf("meta store id=%s error=%v", r.ID, err)
		}
		return
	}
	if s.debug {
		log.Printf("meta fetch id=%s name=%q installs=%d", r.ID, m.Name, m.Installs)
	}
}

func (s *server) enqueue(r ref.Ref) {
	select {
	case s.queue <- r:
	default:
	}
}

func (s *server) topicWorker() {
	s.syncTopics()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.syncTopics()
	}
}

func (s *server) syncTopics() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	topics, err := s.f.Topics(ctx)
	if err != nil {
		_ = s.st.SetSetting("topics_last_error", err.Error())
		if s.debug {
			log.Printf("topics fetch error=%v", err)
		}
		return
	}
	converted := make([]store.Topic, len(topics))
	for i, t := range topics {
		converted[i] = store.Topic{Slug: t.Slug, Name: t.Name, Skills: t.Skills}
	}
	if err := s.st.ReplaceTopics(converted); err != nil {
		_ = s.st.SetSetting("topics_last_error", err.Error())
		if s.debug {
			log.Printf("topics sync error=%v", err)
		}
		return
	}
	if s.debug {
		log.Printf("topics synced count=%d", len(converted))
	}
}

func (s *server) event(source, action, skillID string, detail any) {
	if err := s.st.AddEvent(source, action, skillID, detail); err != nil && s.debug {
		log.Printf("event store action=%s error=%v", action, err)
	}
}

func stageLabel(sk store.Skill) string {
	if sk.Used {
		return "In use"
	}
	if sk.Bookmarked {
		return "To test"
	}
	return "On the shelf"
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.Filter{
		Tab:   q.Get("tab"),
		Query: q.Get("q"),
		Sort:  q.Get("sort"),
		Env:   q.Get("env"),
		Only:  q.Get("only"),
	}
	for _, raw := range q["tag"] {
		for _, part := range strings.Split(raw, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
				f.Tags = append(f.Tags, id)
			}
		}
	}
	skills, counts, err := s.st.List(f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"skills": skills, "counts": counts})
}

func (s *server) add(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Input string `json:"input"`
		store.Patch
	}
	if !decode(w, r, &in) {
		return
	}
	rf, err := ref.Parse(in.Input)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_, existed, err := s.st.Get(rf.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.st.Unignore(rf.ID)
	sk, err := s.st.Upsert(rf, in.Patch)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.enqueue(rf)
	s.event("ui", "add", sk.ID, map[string]any{"name": sk.Name, "created": !existed})
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
	old, _, _ := s.st.Get(in.ID)
	sk, err := s.st.Patch(in.ID, in.Patch)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "skill not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	action, detail := patchEvent(old, sk, in.Patch)
	s.event("ui", action, sk.ID, detail)
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

func (s *server) remove(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeErr(w, 400, "id required")
		return
	}
	sk, _, _ := s.st.Get(id)
	if r.URL.Query().Get("ignore") == "1" {
		if err := s.st.Ignore(id, sk.Name, "removed"); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.event("ui", "ignore", id, map[string]any{"name": sk.Name})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.st.Delete(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event("ui", "remove", id, map[string]any{"name": sk.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	rf, err := ref.Parse(in.ID)
	if err != nil {
		writeErr(w, 400, "skill has no skills.sh source to refresh from")
		return
	}
	s.fetchInto(rf)
	sk, ok, err := s.st.Get(in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "skill not found")
		return
	}
	s.event("ui", "refresh", sk.ID, map[string]any{"name": sk.Name})
	writeJSON(w, 200, sk)
}

func (s *server) resolve(w http.ResponseWriter, r *http.Request) {
	var in struct{ ID, Input string }
	if !decode(w, r, &in) {
		return
	}
	rf, err := ref.Parse(in.Input)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	sk, err := s.st.Resolve(in.ID, rf)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "skill not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.enqueue(rf)
	s.event("ui", "link", sk.ID, map[string]any{"name": sk.Name, "from": in.ID})
	writeJSON(w, 200, sk)
}

type importItem struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Host    string `json:"host"`
	Agent   string `json:"agent"`
	Project string `json:"project"`
	Path    string `json:"path"`
}

// importEntry marks one installed skill as used, keeping existing flags and notes.
func (s *server) importEntry(item importItem) (string, bool, bool, error) {
	name, source := strings.TrimSpace(item.Name), strings.TrimSpace(item.Source)
	if name == "" {
		return "", false, false, errors.New("empty name")
	}
	var rf ref.Ref
	local := false
	if source != "" {
		source = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(source, "https://github.com/"), "http://github.com/"), ".git")
		if r, err := ref.Parse(source + "/" + name); err == nil {
			rf = r
		}
	}
	if rf.ID == "" {
		if r, err := ref.Parse(name); err == nil {
			rf = r
		} else if strings.ContainsAny(name, "/\\ \t") {
			return "", false, false, fmt.Errorf("can't tell what %q is", name)
		} else {
			local = true
		}
	}
	id := rf.ID
	if local {
		id = "local/" + name
	}
	if ignored, err := s.st.IsIgnored(id); err != nil || ignored {
		return id, false, ignored, err
	}
	_, existed, err := s.st.Get(id)
	if err != nil {
		return id, false, false, err
	}
	used := true
	flags := store.Patch{Used: &used}
	var sk store.Skill
	if local {
		sk, err = s.st.UpsertLocal(name, flags)
	} else {
		sk, err = s.st.Upsert(rf, flags)
	}
	if err != nil {
		return id, false, false, err
	}
	if !local {
		s.enqueue(rf)
	}
	if item.Host != "" && item.Agent != "" {
		sourcePath := item.Path
		if sourcePath != "" {
			sourcePath = strings.TrimRight(sourcePath, "/") + "/" + name
		}
		if err := s.st.RecordEnvironment(sk.ID, item.Host, item.Agent, item.Project, item.Path, sourcePath); err != nil {
			return id, false, false, err
		}
	}
	return sk.ID, !existed, false, nil
}

func (s *server) importSkills(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Items        []importItem `json:"items"`
		Text         string       `json:"text"`
		Prune        bool         `json:"prune"`
		Environments []struct {
			Host    string   `json:"host"`
			Agent   string   `json:"agent"`
			Project string   `json:"project"`
			Path    string   `json:"path"`
			Skills  []string `json:"skills"`
		} `json:"environments"`
	}
	if !decode(w, r, &in) {
		return
	}
	imported, created, ignoredCount := 0, 0, 0
	errs := []string{}
	seen := map[string]bool{}
	itemIDs := map[string]string{}
	handle := func(item importItem) {
		id, wasCreated, wasIgnored, err := s.importEntry(item)
		switch {
		case err != nil:
			errs = append(errs, err.Error())
		case wasIgnored:
			ignoredCount++
		default:
			if !seen[id] {
				seen[id] = true
				imported++
				if wasCreated {
					created++
				}
			}
		}
		if id != "" && err == nil && !wasIgnored {
			key := store.EnvironmentID(item.Host, item.Agent, item.Project) + "\x00" + item.Name
			itemIDs[key] = id
		}
	}
	for _, item := range in.Items {
		handle(item)
	}
	for _, line := range strings.Split(in.Text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "-*"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		handle(importItem{Name: line})
	}
	pruned := 0
	if in.Prune {
		for _, env := range in.Environments {
			if err := s.st.TouchEnvironment(env.Host, env.Agent, env.Project, env.Path); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			envID := store.EnvironmentID(env.Host, env.Agent, env.Project)
			keep := []string{}
			for _, name := range env.Skills {
				if id, ok := itemIDs[envID+"\x00"+name]; ok {
					keep = append(keep, id)
				}
			}
			n, err := s.st.PruneEnvironment(envID, keep)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			pruned += int(n)
		}
	}
	source, action := "import", "import"
	if r.Header.Get("X-Reet-Source") == "scan" {
		source, action = "scan", "scan"
	}
	s.event(source, action, "", map[string]any{"imported": imported, "created": created, "ignored": ignoredCount, "pruned": pruned})
	writeJSON(w, 200, map[string]any{"imported": imported, "created": created, "ignored": ignoredCount, "pruned": pruned, "errors": errs})
}

func (s *server) environments(w http.ResponseWriter, r *http.Request) {
	envs, err := s.st.ListEnvironments()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"environments": envs})
}

func (s *server) tags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.st.ListTags()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"tags": tags})
}

func (s *server) skillTags(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     string   `json:"id"`
		Add    []string `json:"add"`
		Remove []int64  `json:"remove"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ID == "" {
		writeErr(w, 400, "id required")
		return
	}
	if len(in.Add) > 0 {
		if err := s.st.AddSkillTags(in.ID, in.Add); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	for _, id := range in.Remove {
		if err := s.st.RemoveSkillTag(in.ID, id); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	sk, ok, err := s.st.Get(in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "skill not found")
		return
	}
	s.event("ui", "tag", sk.ID, map[string]any{"name": sk.Name, "added": in.Add, "removed": in.Remove})
	writeJSON(w, 200, sk)
}

func (s *server) installCopy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	sk, ok, err := s.st.Get(in.ID)
	if err != nil || !ok {
		writeErr(w, 404, "skill not found")
		return
	}
	s.event("ui", "install-copy", sk.ID, map[string]any{"name": sk.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) renameTag(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.RenameTag(in.ID, in.Name); errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "tag not found")
		return
	} else if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, 409, "that tag already exists")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	s.event("ui", "tag-rename", "", map[string]any{"name": in.Name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *server) mergeTags(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From int64 `json:"from"`
		Into int64 `json:"into"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.MergeTags(in.From, in.Into); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event("ui", "tag-merge", "", map[string]any{"from": in.From, "into": in.Into})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 20
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	events, err := s.st.ListEvents(limit, q.Get("skill"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"events": events})
}

func (s *server) ignored(w http.ResponseWriter, r *http.Request) {
	items, err := s.st.ListIgnored()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ignored": items})
}

func (s *server) unignore(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeErr(w, 400, "id required")
		return
	}
	name := id
	if items, err := s.st.ListIgnored(); err == nil {
		for _, item := range items {
			if item.ID == id {
				name = item.Name
				break
			}
		}
	}
	if err := s.st.Unignore(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event("ui", "unignore", id, map[string]any{"name": name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) export(w http.ResponseWriter, r *http.Request) {
	includeEvents := r.URL.Query().Get("events") != "0"
	snap, err := s.st.ExportSnapshot(includeEvents)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event("ui", "export", "", map[string]any{"events": includeEvents})
	w.Header().Set("Content-Disposition", `attachment; filename="reet-export.json"`)
	writeJSON(w, 200, snap)
}

func (s *server) restore(w http.ResponseWriter, r *http.Request) {
	var in store.Snapshot
	if !decode(w, r, &in) {
		return
	}
	if in.Skills == nil {
		writeErr(w, 400, "backup has no skills")
		return
	}
	if err := s.st.RestoreSnapshot(in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.event("ui", "restore", "", map[string]any{"restored": len(in.Skills), "ignored": len(in.Ignored)})
	writeJSON(w, 200, map[string]int{"restored": len(in.Skills)})
}

func (s *server) quick(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	yes := true
	var p store.Patch
	var verb, alreadyVerb, actionName string
	switch q.Get("action") {
	case "like":
		p.Liked, verb, alreadyVerb, actionName = &yes, "Liked", "Already liked", "like"
	case "save":
		p.Bookmarked, verb, alreadyVerb, actionName = &yes, "Saved", "Already saved", "stage"
	case "used":
		p.Used, verb, alreadyVerb, actionName = &yes, "Marked as used", "Already marked as used", "stage"
	default:
		quickPage(w, 400, "Unknown action")
		return
	}
	rf, err := ref.Parse(q.Get("url"))
	if err != nil {
		quickPage(w, 400, "Not a skills.sh skill page")
		return
	}
	old, found, err := s.st.Get(rf.ID)
	if err != nil {
		quickPage(w, 500, "Could not save")
		return
	}
	already := found &&
		((p.Liked != nil && old.Liked) || (p.Bookmarked != nil && old.Bookmarked) || (p.Used != nil && old.Used))
	_ = s.st.Unignore(rf.ID)
	sk, err := s.st.Upsert(rf, p)
	if err != nil {
		quickPage(w, 500, "Could not save")
		return
	}
	s.enqueue(rf)
	detail := map[string]any{"name": sk.Name, "already": already}
	if actionName == "stage" {
		detail["from"] = stageLabel(old)
		detail["to"] = stageLabel(sk)
	}
	s.event("quick", actionName, sk.ID, detail)
	msg := verb + ": " + sk.Name
	if already {
		msg = alreadyVerb + ": " + sk.Name
	}
	quickPage(w, 200, msg)
}

func quickPage(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>reet.sh</title>
<body style="font:16px system-ui;display:grid;place-items:center;height:90vh;margin:0"><p>%s</p>
<script>setTimeout(function(){window.close()},1500)</script></body>`, html.EscapeString(msg))
}
