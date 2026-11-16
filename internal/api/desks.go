package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"

	"github.com/eyupio/reet.sh/internal/desk"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Desks: saved workspaces of tabs, split trees and panes, per account. The
// routes take the terminal access rules (accounts, cookie, same origin for
// writes). Every write names the version it read (expect) and a stale one is
// answered with 409 and the current record.

// maxDeskBodyBytes holds a 64 KiB document with room for its envelope, so an
// oversize document is refused as an invalid desk rather than as a large body.
const maxDeskBodyBytes = desk.MaxDocBytes*2 + 4096

func (s *server) registerDeskRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/desks", s.terminalAccess(s.deskList, true))
	mux.HandleFunc("POST /api/v2/desks", s.terminalAccess(s.deskCreate, true))
	mux.HandleFunc("POST /api/v2/desks/reorder", s.terminalAccess(s.deskReorder, true))
	mux.HandleFunc("GET /api/v2/desks/{id}", s.terminalAccess(s.deskGet, true))
	mux.HandleFunc("PATCH /api/v2/desks/{id}", s.terminalAccess(s.deskUpdate, true))
	mux.HandleFunc("DELETE /api/v2/desks/{id}", s.terminalAccess(s.deskDelete, true))
	// Reopen starts a shell, so it is refused while remote terminals are off.
	mux.HandleFunc("POST /api/v2/desks/{id}/panes/{pane}/reopen", s.terminalAccess(s.deskPaneReopen, false))
}

type deskJSON struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Colour   string   `json:"colour"`
	Template bool     `json:"template"`
	Position int      `json:"position"`
	Version  int64    `json:"version"`
	Schema   int      `json:"schema"`
	Doc      desk.Doc `json:"doc"`
}

func deskView(d store.Desk) deskJSON {
	return deskJSON{d.ID, d.Name, d.Colour, d.Template, d.Position, d.Version, d.Schema, d.Doc}
}

type deskSummary struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Colour   string   `json:"colour"`
	Template bool     `json:"template"`
	Position int      `json:"position"`
	Version  int64    `json:"version"`
	Tabs     int      `json:"tabs"`
	Panes    int      `json:"panes"`
	Sessions []string `json:"sessions"`
	Hosts    []string `json:"hosts"`
}

// deskHosts lists the distinct machines of a document's panes in document
// order: tabs in order, panes in tree order, then any pane the tree leaves out
// by id.
func deskHosts(doc desk.Doc) []string {
	var out []string
	seen := map[string]bool{}
	add := func(t desk.Tab, id string) {
		if h := t.Panes[id].Host; h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	for _, t := range doc.Tabs {
		listed := map[string]bool{}
		var walk func(n *desk.Node)
		walk = func(n *desk.Node) {
			if n == nil {
				return
			}
			if n.Split == "" {
				if _, ok := t.Panes[n.Pane]; ok && !listed[n.Pane] {
					listed[n.Pane] = true
					add(t, n.Pane)
				}
				return
			}
			walk(n.A)
			walk(n.B)
		}
		walk(t.Root)
		var rest []string
		for id := range t.Panes {
			if !listed[id] {
				rest = append(rest, id)
			}
		}
		sort.Strings(rest)
		for _, id := range rest {
			add(t, id)
		}
	}
	return out
}

func summariseDesk(d store.Desk) deskSummary {
	out := deskSummary{ID: d.ID, Name: d.Name, Colour: d.Colour, Template: d.Template, Position: d.Position, Version: d.Version,
		Tabs: len(d.Doc.Tabs), Sessions: d.Doc.Sessions(), Hosts: deskHosts(d.Doc)}
	if out.Sessions == nil {
		out.Sessions = []string{}
	}
	if out.Hosts == nil {
		out.Hosts = []string{}
	}
	for _, t := range d.Doc.Tabs {
		out.Panes += len(t.Panes)
	}
	return out
}

// deskBody decodes exactly one JSON object with no unknown fields; an empty
// body is allowed only when optional is set.
func deskBody(w http.ResponseWriter, r *http.Request, out any, optional bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxDeskBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err == nil && optional && len(bytes.TrimSpace(raw)) == 0 {
		return true
	}
	if err == nil {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err = d.Decode(out); err == nil {
			if extra := d.Decode(new(any)); extra != io.EOF {
				err = errors.New("more than one JSON value")
			}
		}
	}
	if err != nil {
		writeErrCode(w, http.StatusBadRequest, "desk_invalid", "", "That request isn't a desk reet can read.", "")
		return false
	}
	return true
}

func deskRefusal(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrDeskMissing):
		writeErrCode(w, 404, "desk_missing", "", "That desk isn't on this account.", "")
	case errors.Is(err, store.ErrDeskInvalid):
		writeErrCode(w, 400, "desk_invalid", "", "That desk can't be saved: "+err.Error(), "")
	case errors.Is(err, store.ErrDeskFull):
		writeErrCode(w, 409, "desk_full", "", err.Error()+". Remove one first.", "")
	case errors.Is(err, store.ErrDeskNameTaken):
		writeErrCode(w, 409, "desk_name_taken", "name", "Another desk already has that name.", "")
	case errors.Is(err, store.ErrDeskSessionTaken):
		writeErrCode(w, 409, "desk_session_taken", "", "A session can be in only one pane.", "")
	default:
		log.Printf("desks: store error: %v", err)
		writeErrCode(w, 500, "server_error", "", "reet couldn't do that just now. Try again.", "")
	}
}

// deskStale answers 409 with the desk as it is now.
func (s *server) deskStale(w http.ResponseWriter, uid int64, id string) {
	cur, err := s.st.DeskFor(uid, id)
	if err != nil {
		deskRefusal(w, err)
		return
	}
	writeJSON(w, 409, map[string]any{
		"error": "desk_stale", "code": "desk_stale",
		"message": "Someone changed this desk. Reload it, then try again.",
		"desk":    deskView(cur),
	})
}

func (s *server) deskList(w http.ResponseWriter, r *http.Request) {
	desks, err := s.st.ListDesksFor(s.uid(r))
	if err != nil {
		deskRefusal(w, err)
		return
	}
	out := make([]deskSummary, 0, len(desks))
	for _, d := range desks {
		out = append(out, summariseDesk(d))
	}
	writeJSON(w, 200, map[string]any{"desks": out})
}

func (s *server) deskGet(w http.ResponseWriter, r *http.Request) {
	d, err := s.st.DeskFor(s.uid(r), r.PathValue("id"))
	if err != nil {
		deskRefusal(w, err)
		return
	}
	writeJSON(w, 200, deskView(d))
}

func newDeskID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure randomness unavailable")
	}
	return hex.EncodeToString(b[:])
}

// copyDeskDoc copies a document with fresh tab and pane ids and every session
// empty.
func copyDeskDoc(src desk.Doc) desk.Doc {
	out := desk.Doc{Tabs: make([]desk.Tab, 0, len(src.Tabs))}
	for _, t := range src.Tabs {
		ids := map[string]string{}
		for id := range t.Panes {
			ids[id] = newDeskID()
		}
		var tree func(n *desk.Node) *desk.Node
		tree = func(n *desk.Node) *desk.Node {
			if n == nil {
				return nil
			}
			c := *n
			if c.Pane != "" {
				c.Pane = ids[c.Pane]
			}
			c.A, c.B = tree(n.A), tree(n.B)
			return &c
		}
		nt := t
		nt.ID = newDeskID()
		nt.Root = tree(t.Root)
		nt.Zoomed, nt.Active = ids[t.Zoomed], ids[t.Active]
		nt.Panes = make(map[string]desk.Pane, len(t.Panes))
		for id, p := range t.Panes {
			p.ID, p.Session = ids[id], ""
			nt.Panes[p.ID] = p
		}
		if src.Active == t.ID {
			out.Active = nt.ID
		}
		out.Tabs = append(out.Tabs, nt)
	}
	return out
}

func (s *server) deskCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Colour   string `json:"colour"`
		Template bool   `json:"template"`
		From     string `json:"from"`
	}
	if !deskBody(w, r, &in, false) {
		return
	}
	uid := s.uid(r)
	d := store.Desk{Name: in.Name, Colour: in.Colour, Template: in.Template, Doc: desk.Doc{Tabs: []desk.Tab{}}}
	if in.From != "" {
		src, err := s.st.DeskFor(uid, in.From)
		if err != nil {
			deskRefusal(w, err)
			return
		}
		d.Doc = copyDeskDoc(src.Doc)
	}
	created, err := s.st.CreateDeskFor(uid, d)
	if err != nil {
		deskRefusal(w, err)
		return
	}
	s.eventRequest(r, "desk-created", "", map[string]any{"desk": created.ID, "name": created.Name, "template": created.Template})
	writeJSON(w, 201, deskView(created))
}

func (s *server) deskUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Expect *int64          `json:"expect"`
		Name   *string         `json:"name"`
		Colour *string         `json:"colour"`
		Doc    json.RawMessage `json:"doc"`
	}
	if !deskBody(w, r, &in, false) {
		return
	}
	uid, id := s.uid(r), r.PathValue("id")
	cur, err := s.st.DeskFor(uid, id)
	if err != nil {
		deskRefusal(w, err)
		return
	}
	if in.Expect == nil || *in.Expect != cur.Version {
		s.deskStale(w, uid, id)
		return
	}
	if in.Name != nil {
		cur.Name = *in.Name
	}
	if in.Colour != nil {
		cur.Colour = *in.Colour
	}
	if len(in.Doc) > 0 && string(in.Doc) != "null" {
		doc, err := desk.Parse(in.Doc)
		if err != nil {
			writeErrCode(w, 400, "desk_invalid", "doc", "That desk can't be saved: "+err.Error(), "")
			return
		}
		cur.Doc = doc
	}
	saved, err := s.st.UpdateDeskFor(uid, cur, cur.Version)
	if errors.Is(err, store.ErrDeskStale) {
		s.deskStale(w, uid, id)
		return
	}
	if err != nil {
		deskRefusal(w, err)
		return
	}
	writeJSON(w, 200, deskView(saved))
}

// deskEndSession asks for a session to end with its history removed, and its
// desk pane's own command history on the machine with it (the pane is gone
// for good). A test replaces it to make an end fail.
var deskEndSession = func(s *server, ctx context.Context, uid int64, session, request string) error {
	_, err := s.terminals.EndDeskPane(ctx, uid, session, request, true)
	return err
}

func (s *server) deskDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Expect      *int64 `json:"expect"`
		EndSessions bool   `json:"endSessions"`
	}
	if !deskBody(w, r, &in, false) {
		return
	}
	uid, id := s.uid(r), r.PathValue("id")
	cur, err := s.st.DeskFor(uid, id)
	if err != nil {
		deskRefusal(w, err)
		return
	}
	if in.Expect == nil || *in.Expect != cur.Version {
		s.deskStale(w, uid, id)
		return
	}
	err = s.st.DeleteDeskFor(uid, id, cur.Version)
	if errors.Is(err, store.ErrDeskStale) {
		s.deskStale(w, uid, id)
		return
	}
	if err != nil {
		deskRefusal(w, err)
		return
	}
	// The desk is gone first, so a stale delete never leaves a desk whose shells
	// were already ended. A session that will not end is reported, not retried.
	left := []string{}
	if in.EndSessions {
		for _, sid := range cur.Doc.Sessions() {
			sess, err := s.st.TerminalSessionFor(uid, sid)
			if err != nil {
				continue // not a session reet knows: nothing to end
			}
			if sess.State != terminalproto.Creating && sess.State != terminalproto.Running {
				continue // already ended or ending
			}
			if err := deskEndSession(s, r.Context(), uid, sid, terminalbroker.ID()); err != nil {
				left = append(left, sid)
			}
		}
	}
	s.eventRequest(r, "desk-deleted", "", map[string]any{"desk": id, "name": cur.Name, "endedSessions": in.EndSessions, "left": len(left)})
	writeJSON(w, 200, map[string]any{"ok": true, "left": left})
}

func (s *server) deskReorder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if !deskBody(w, r, &in, false) {
		return
	}
	if err := s.st.ReorderDesksFor(s.uid(r), in.IDs); err != nil {
		deskRefusal(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// Reopen: a pane whose shell ended or was lost gets a fresh plain shell.
const (
	// reopenCols and reopenRows size the new shell until the pane attaches and
	// resizes it to fit.
	reopenCols, reopenRows = 80, 24
)

// deskReopenCreate is the create a reopen sends. A machine with desk-shell-v1
// is told the pane (Desk.Pane). The pane's folder goes as Desk.Dir, which the
// machine opens even outside home, only when it is the folder that machine
// itself last reported for this pane (trusted); otherwise as Dir, which keeps
// the home rule. The controller does not know the machine's home, so a trusted
// folder is always sent as Desk.Dir and the machine checks it
// (terminalhost.ResolveDeskDir); a folder a browser wrote never is.
func deskReopenCreate(pane desk.Pane, dir string, deskShell, trusted bool) terminalproto.Create {
	title, origin := terminalproto.ResolveTitle(pane.Name, "", "", "", dir)
	in := terminalproto.Create{Title: title, TitleOrigin: origin, Cols: reopenCols, Rows: reopenRows}
	switch {
	case deskShell && terminalproto.ValidID(pane.ID):
		in.Desk.Pane = pane.ID
		if trusted {
			in.Desk.Dir = dir
		} else {
			in.Dir = dir
		}
	default:
		// An older machine never sees a desk reference: the folder goes as
		// Dir under shell-dir-v1, which the broker requires.
		in.Dir = dir
	}
	return in
}

func (s *server) deskPaneReopen(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Expect *int64 `json:"expect"`
		Home   bool   `json:"home"`
	}
	if !deskBody(w, r, &in, false) {
		return
	}
	uid, id, paneID := s.uid(r), r.PathValue("id"), r.PathValue("pane")
	cur, err := s.st.DeskFor(uid, id)
	if err != nil {
		deskRefusal(w, err)
		return
	}
	if in.Expect == nil || *in.Expect != cur.Version {
		s.deskStale(w, uid, id)
		return
	}
	var pane desk.Pane
	found := false
	for _, t := range cur.Doc.Tabs {
		if p, ok := t.Panes[paneID]; ok {
			pane, found = p, true
		}
	}
	if !found {
		writeErrCode(w, 404, "pane_missing", "", "That pane isn't on this desk.", "")
		return
	}
	if cur.Template {
		writeErrCode(w, 400, "desk_invalid", "", "A template holds no shells.", "")
		return
	}
	if pane.Session != "" {
		if sess, err := s.st.TerminalSessionFor(uid, pane.Session); err == nil && sess.State.ReservesQuota() {
			writeErrCode(w, 409, "pane_live", "", "This pane's shell is still running.", "")
			return
		}
	}
	agent := pane.Host
	host := s.machineHost(uid, agent)
	if agent == "" || host == "" {
		writeErrCode(w, 409, "machine_gone", "", "This machine was removed from reet.", "")
		return
	}
	caps, capsErr := s.st.RemoteCapabilitiesFor(uid, agent)
	if capsErr != nil || !s.terminals.Ready(uid, agent) {
		writeErrCode(w, 409, "machine_offline", "", host+" is not reporting.", "")
		return
	}
	dir := pane.Dir
	if in.Home {
		dir = ""
	}
	trusted := false
	if dir != "" {
		reportedBy, reported, ok, err := s.st.DeskPaneReportedDirFor(uid, id, paneID)
		if err != nil {
			deskRefusal(w, err)
			return
		}
		trusted = ok && reportedBy == agent && reported == dir
	}
	create := deskReopenCreate(pane, dir, caps.Capabilities.DeskShell, trusted)
	bound := false
	session, _, err := s.terminals.CreateSessionReserved(r.Context(), uid, agent, terminalbroker.ID(), create, func(reserved store.TerminalSession) error {
		_, err := s.st.ReopenDeskPaneFor(uid, id, paneID, reserved.ID, cur.Version)
		bound = err == nil
		return err
	})
	if err != nil {
		if bound {
			// The pane keeps a session that may still start (no answer yet);
			// only one that is definitely not running is taken off it.
			latest, lerr := s.st.TerminalSessionFor(uid, session.ID)
			if lerr != nil || !latest.State.ReservesQuota() {
				_, _ = s.st.UnbindDeskPaneSessionFor(uid, id, paneID, session.ID)
			}
		}
		s.deskReopenError(w, err, uid, id, agent, host)
		return
	}
	s.eventRequest(r, "desk-pane-reopened", "", map[string]any{
		"desk": id, "pane": paneID, "machine": agent, "host": host, "session": session.ID,
		"dir": create.Dir + create.Desk.Dir, "outsideHome": create.Desk.Dir != "", "home": in.Home,
	})
	saved, err := s.st.DeskFor(uid, id)
	if err != nil {
		deskRefusal(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"desk": deskView(saved), "session": session})
}

// deskReopenError maps a reopen's refusal; anything else keeps the terminal
// create's mapping.
func (s *server) deskReopenError(w http.ResponseWriter, err error, uid int64, id, agent, host string) {
	switch {
	case errors.Is(err, store.ErrDeskStale):
		s.deskStale(w, uid, id)
	case errors.Is(err, store.ErrDeskMissing):
		deskRefusal(w, err)
	case errors.Is(err, terminalbroker.ErrFolderGone), errors.Is(err, terminalbroker.ErrLaunchFailed):
		// A plain shell's launch fails only on its folder.
		writeErrCode(w, 409, "folder_gone", "", "That folder no longer exists on "+host+".", "")
	case errors.Is(err, terminalbroker.ErrShellDirUnsupported), errors.Is(err, terminalbroker.ErrDeskShellUnsupported):
		writeErrCode(w, 409, "shell_dir_unsupported", "", "Update reet on that machine to keep this pane's folder and history.", "")
	case errors.Is(err, terminalbroker.ErrOffline), errors.Is(err, store.ErrTerminalNotReady):
		writeErrCode(w, 409, "machine_offline", "", host+" is not reporting.", "")
	default:
		s.terminalCreateError(w, err, uid, agent, "")
	}
}
