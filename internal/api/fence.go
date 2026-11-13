package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// The fence is the owner's way to say "this controller has moved". While it is
// up the controller refuses every write with 423 (Locked) and its background
// work stands still; reads keep working. It changes no data, so lifting it puts
// the controller back exactly as it was.

const fencePath = "/api/v2/admin/fence"

func (s *server) registerFenceRoutes(mux *http.ServeMux) {
	// An admin may read the state, so the pages they work in can say why every
	// change is refused before they try one. It tells them nothing the 423
	// does not. Raising and lifting stay the owner's alone.
	mux.HandleFunc("GET "+fencePath, s.requireAdmin(s.fenceGet))
	mux.HandleFunc("POST "+fencePath, s.requireOwner(s.fenceRaise))
	mux.HandleFunc("DELETE "+fencePath, s.requireOwner(s.fenceLift))
}

type fenceView struct {
	Fenced  bool   `json:"fenced"`
	Since   int64  `json:"since"`
	MovedTo string `json:"movedTo"`
}

func viewOfFence(f store.Fence) fenceView {
	return fenceView{Fenced: f.Fenced, Since: f.Since, MovedTo: f.MovedTo}
}

func (s *server) fenceGet(w http.ResponseWriter, r *http.Request) {
	f, err := s.st.Fence()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, viewOfFence(f))
}

func (s *server) fenceRaise(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MovedTo string `json:"movedTo"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, `the body is {"movedTo": "https://..."}; movedTo is optional`)
		return
	}
	to, err := store.ValidMovedTo(in.MovedTo)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	before, err := s.st.Fence()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	f, err := s.st.SetFence(to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !before.Fenced {
		s.event(s.uid(r), "backup", "fence-raised", "", map[string]any{"movedTo": f.MovedTo})
	}
	writeJSON(w, http.StatusOK, viewOfFence(f))
}

func (s *server) fenceLift(w http.ResponseWriter, r *http.Request) {
	before, err := s.st.Fence()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !before.Fenced {
		writeJSON(w, http.StatusOK, viewOfFence(store.Fence{}))
		return
	}
	f, err := s.st.LiftFence()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.event(s.uid(r), "backup", "fence-lifted", "", nil)
	s.resumeAfterFence()
	writeJSON(w, http.StatusOK, viewOfFence(f))
}

// resumeAfterFence starts again the work that stood still: members of project
// runs and software batches that were left pending. The mail queue, the
// notifier, the schedules and the goal and chat loops check the fence on their
// own next pass, so they need no nudge.
func (s *server) resumeAfterFence() {
	if s.runs != nil {
		if err := s.runs.Resume(); err != nil {
			log.Printf("projects: resume runs after the fence was lifted: %v", err)
		}
	}
	if s.softwareRuns != nil {
		if err := s.softwareRuns.Resume(); err != nil {
			log.Printf("software batches: resume after the fence was lifted: %v", err)
		}
	}
}

// fenceMessage says that the controller has moved, when, and to where.
func fenceMessage(f store.Fence) string {
	msg := "This controller has moved, so it no longer accepts changes. It was fenced on " +
		time.UnixMilli(f.Since).UTC().Format("2 January 2006 at 15:04 UTC")
	if f.MovedTo != "" {
		return msg + "; use " + f.MovedTo + " instead."
	}
	return msg + "; use the controller it moved to instead."
}

// fenceWrite is whether a request would change something. Reads, including the
// private-connection upgrade, are not writes. /quick saves through a GET, so it
// counts; /mcp carries every tool call as a POST and is checked per tool.
func fenceWrite(r *http.Request) bool {
	if r.URL.Path == "/mcp" {
		return false
	}
	if r.URL.Path == "/quick" {
		return true
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// fenceExempt names the writes a fenced controller still takes: raising and
// lifting the fence, the owner's backup settings, run and verify (so a final
// copy can be taken and checked), and signing in or out, without which an owner
// whose session had lapsed could not reach the rest.
func fenceExempt(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case p == fencePath:
		return true
	case p == ssoLogoutPath:
		return true
	case r.Method == http.MethodPatch && p == "/api/v2/admin/backups":
		return true
	case r.Method == http.MethodPost && (p == "/api/v2/admin/backups/run" ||
		(strings.HasPrefix(p, "/api/v2/admin/backups/items/") && strings.HasSuffix(p, "/verify"))):
		return true
	case r.Method == http.MethodPost && (p == "/api/v2/auth/sign-in" || p == "/api/v2/auth/sign-out"):
		return true
	}
	return false
}

// fenceGate refuses a write with 423 while the controller is fenced.
func (s *server) fenceGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !fenceWrite(r) || fenceExempt(r) {
			next.ServeHTTP(w, r)
			return
		}
		f, err := s.st.Fence()
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "this controller cannot tell whether it has moved, so it is not accepting changes")
			return
		}
		if !f.Fenced {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusLocked, map[string]any{
			"error": fenceMessage(f), "code": "controller_moved",
			"fenced": true, "since": f.Since, "movedTo": f.MovedTo,
		})
	})
}

// fenceInterval is how often the server looks for a fence lifted from outside
// it, by `reet backup fence --lift` run beside the server.
const fenceInterval = 5 * time.Second

// fenceWatcher continues the paused project runs and software batches when a
// fence is lifted by the command line, which cannot tell the server. A lift
// through the API continues them at once and this then finds nothing to do.
func (s *server) fenceWatcher() {
	was := s.st.Fenced()
	t := time.NewTicker(fenceInterval)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
		}
		now := s.st.Fenced()
		if was && !now {
			s.resumeAfterFence()
		}
		was = now
	}
}
