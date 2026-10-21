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

// New returns the API handler. An empty token disables authentication.
func New(st *store.Store, f meta.Fetcher, token string, opts ...Option) http.Handler {
	s := &server{st: st, f: f, token: token, version: "dev", commit: "unknown", queue: make(chan ref.Ref, 256)}
	for _, opt := range opts {
		opt(s)
	}
	go s.worker()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/version", s.buildInfo)
	mux.HandleFunc("GET /api/skills", s.list)
	mux.HandleFunc("POST /api/skills", s.add)
	mux.HandleFunc("PATCH /api/skill", s.patch)
	mux.HandleFunc("DELETE /api/skill", s.remove)
	mux.HandleFunc("POST /api/skill/refresh", s.refresh)
	mux.HandleFunc("POST /api/skill/resolve", s.resolve)
	mux.HandleFunc("POST /api/import", s.importSkills)
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
	skills, counts, err := s.st.List(store.Filter{Tab: q.Get("tab"), Query: q.Get("q"), Sort: q.Get("sort")})
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
	sk, err := s.st.Upsert(rf, in.Patch)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.enqueue(rf)
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
	sk, err := s.st.Patch(in.ID, in.Patch)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "skill not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, sk)
}

func (s *server) remove(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeErr(w, 400, "id required")
		return
	}
	if err := s.st.Delete(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
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
	writeJSON(w, 200, sk)
}

// importEntry marks one installed skill as used, keeping any existing flags and notes.
func (s *server) importEntry(name, source string) error {
	name, source = strings.TrimSpace(name), strings.TrimSpace(source)
	if name == "" {
		return errors.New("empty name")
	}
	used := true
	flags := store.Patch{Used: &used}
	up := func(r ref.Ref) error {
		_, err := s.st.Upsert(r, flags)
		if err == nil {
			s.enqueue(r)
		}
		return err
	}
	if source != "" {
		source = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(source, "https://github.com/"), "http://github.com/"), ".git")
		if r, err := ref.Parse(source + "/" + name); err == nil {
			return up(r)
		}
	}
	if r, err := ref.Parse(name); err == nil {
		return up(r)
	}
	if strings.ContainsAny(name, "/\\ \t") {
		return fmt.Errorf("can't tell what %q is", name)
	}
	_, err := s.st.UpsertLocal(name, flags)
	return err
}

func (s *server) importSkills(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Items []struct{ Name, Source string } `json:"items"`
		Text  string                          `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	imported, errs := 0, []string{}
	handle := func(name, source string) {
		if err := s.importEntry(name, source); err != nil {
			errs = append(errs, err.Error())
		} else {
			imported++
		}
	}
	for _, it := range in.Items {
		handle(it.Name, it.Source)
	}
	for _, line := range strings.Split(in.Text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "-*"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		handle(line, "")
	}
	writeJSON(w, 200, map[string]any{"imported": imported, "errors": errs})
}

func (s *server) export(w http.ResponseWriter, r *http.Request) {
	all, err := s.st.Export()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="reet-export.json"`)
	writeJSON(w, 200, map[string]any{"version": 1, "exportedAt": time.Now().UnixMilli(), "skills": all})
}

func (s *server) restore(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Skills []store.Skill `json:"skills"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.Restore(in.Skills); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]int{"restored": len(in.Skills)})
}

func (s *server) quick(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	yes := true
	var p store.Patch
	var verb, alreadyVerb string
	switch q.Get("action") {
	case "like":
		p.Liked, verb, alreadyVerb = &yes, "Liked", "Already liked"
	case "save":
		p.Bookmarked, verb, alreadyVerb = &yes, "Saved", "Already saved"
	case "used":
		p.Used, verb, alreadyVerb = &yes, "Marked as used", "Already marked as used"
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
	sk, err := s.st.Upsert(rf, p)
	if err != nil {
		quickPage(w, 500, "Could not save")
		return
	}
	s.enqueue(rf)
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
