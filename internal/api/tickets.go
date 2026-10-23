package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/store"
)

// Support tickets are the requester's half and the instance administrator's
// half. A ticket can be personal or about a team; a team's owner and managers
// can read and answer its tickets. Internal notes never leave the admin side.

func (s *server) registerTicketRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/tickets", s.ticketList)
	mux.HandleFunc("POST /api/v2/tickets", s.ticketOpen)
	mux.HandleFunc("GET /api/v2/tickets/{id}", s.ticketDetail)
	mux.HandleFunc("POST /api/v2/tickets/{id}/reply", s.ticketReply)
	mux.HandleFunc("POST /api/v2/tickets/{id}/close", s.ticketClose)

	mux.HandleFunc("GET /api/v2/admin/tickets", s.requireAdmin(s.adminTicketQueue))
	mux.HandleFunc("GET /api/v2/admin/tickets/{id}", s.requireAdmin(s.adminTicketDetail))
	mux.HandleFunc("POST /api/v2/admin/tickets/{id}/reply", s.requireAdmin(s.adminTicketReply))
	mux.HandleFunc("POST /api/v2/admin/tickets/{id}/note", s.requireAdmin(s.adminTicketNote))
	mux.HandleFunc("POST /api/v2/admin/tickets/{id}/status", s.requireAdmin(s.adminTicketStatus))
	mux.HandleFunc("POST /api/v2/admin/tickets/{id}/assign", s.requireAdmin(s.adminTicketAssign))
}

func ticketName(u store.User) string {
	if strings.TrimSpace(u.Name) != "" {
		return u.Name
	}
	if strings.TrimSpace(u.Email) != "" {
		return u.Email
	}
	return "Someone"
}

func ticketWriteError(w http.ResponseWriter, err error) {
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = "the ticket could not be saved"
	}
	msg = strings.ToUpper(msg[:1]) + msg[1:]
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	writeErr(w, http.StatusUnprocessableEntity, msg)
}

func (s *server) ticketList(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !store.ValidTicketStatus(status) {
		writeErr(w, http.StatusBadRequest, "a ticket is open, waiting or closed")
		return
	}
	filter := store.TicketFilter{Status: status}
	if raw := r.URL.Query().Get("assignee"); raw != "" {
		filter.Assignee, _ = strconv.ParseInt(raw, 10, 64)
	}
	tickets, err := s.st.TicketsFor(s.uid(r), s.role(r), filter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets})
}

func (s *server) ticketOpen(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
		GroupID int64  `json:"groupId"`
	}
	if !decode(w, r, &in) {
		return
	}
	u := s.currentUser(r)
	groupName := ""
	if in.GroupID != 0 {
		g, err := s.st.GroupFor(u.ID, s.role(r), in.GroupID)
		if err != nil {
			writeErr(w, http.StatusForbidden, "choose a team you belong to")
			return
		}
		groupName = g.Name
	}
	t, err := s.st.OpenTicket(u.ID, ticketName(u), u.Email, in.GroupID, groupName, in.Subject, in.Body)
	if err != nil {
		ticketWriteError(w, err)
		return
	}
	s.eventRequest(r, "ticket-open", "", map[string]any{"ticket": t.ID, "subject": t.Subject, "group": t.GroupName})
	s.notifyTicket(r, t, "opened", "opened", true, true, false, u.Email, ticketName(u))
	writeJSON(w, http.StatusCreated, map[string]any{"ticket": t})
}

func (s *server) ticketAccess(w http.ResponseWriter, r *http.Request) (store.Ticket, store.User, bool) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "ticket not found")
		return store.Ticket{}, store.User{}, false
	}
	t, err := s.st.GetTicket(id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "ticket not found")
		return store.Ticket{}, store.User{}, false
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return store.Ticket{}, store.User{}, false
	}
	u := s.currentUser(r)
	if u.Role == "owner" || u.Role == "admin" || t.UserID == u.ID {
		return t, u, true
	}
	if t.GroupID != 0 {
		if g, err := s.st.GroupFor(u.ID, u.Role, t.GroupID); err == nil && (g.Role == "owner" || g.Role == "manager") {
			return t, u, true
		}
	}
	// Another person's ticket and one that never existed answer alike.
	writeErr(w, http.StatusNotFound, "ticket not found")
	return store.Ticket{}, store.User{}, false
}

func (s *server) ticketDetail(w http.ResponseWriter, r *http.Request) {
	t, _, ok := s.ticketAccess(w, r)
	if !ok {
		return
	}
	messages, err := s.st.TicketMessages(t.ID, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t, "messages": messages})
}

// ticketKind says how a reply from this person reads: an instance admin is
// support, a group owner/manager is the team, and everybody else is the
// requester.
func (s *server) ticketKind(t store.Ticket, u store.User, role string) string {
	if role == "owner" || role == "admin" {
		return "admin"
	}
	if t.GroupID != 0 {
		if g, err := s.st.GroupFor(u.ID, role, t.GroupID); err == nil && (g.Role == "owner" || g.Role == "manager") {
			return "team"
		}
	}
	return "user"
}

func (s *server) ticketReply(w http.ResponseWriter, r *http.Request) {
	t, u, ok := s.ticketAccess(w, r)
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &in) {
		return
	}
	kind := s.ticketKind(t, u, s.role(r))
	t, msg, err := s.st.AddTicketMessage(t.ID, u.ID, kind, ticketName(u), in.Body, false, "")
	if err != nil {
		ticketWriteError(w, err)
		return
	}
	s.eventRequest(r, "ticket-reply", "", map[string]any{"ticket": t.ID, "kind": kind, "message": msg.ID})
	s.notifyTicketForReply(r, t, kind, strconv.FormatInt(msg.ID, 10), u.Email, ticketName(u))
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t, "message": msg})
}

func (s *server) ticketClose(w http.ResponseWriter, r *http.Request) {
	t, u, ok := s.ticketAccess(w, r)
	if !ok {
		return
	}
	if t.Status != store.TicketClosed {
		if err := s.st.SetTicketStatus(t.ID, store.TicketClosed); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	s.eventRequest(r, "ticket-close", "", map[string]any{"ticket": t.ID, "by": ticketName(u)})
	t, _ = s.st.GetTicket(t.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t})
}

func (s *server) adminTicketQueue(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !store.ValidTicketStatus(status) {
		writeErr(w, http.StatusBadRequest, "a ticket is open, waiting or closed")
		return
	}
	filter := store.TicketFilter{Status: status, Limit: 500}
	if raw := r.URL.Query().Get("assignee"); raw != "" {
		filter.Assignee, _ = strconv.ParseInt(raw, 10, 64)
	}
	tickets, err := s.st.TicketsFor(s.uid(r), s.role(r), filter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	counts, err := s.st.TicketCounts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": tickets, "counts": counts})
}

func (s *server) adminTicketDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "ticket not found")
		return
	}
	t, err := s.st.GetTicket(id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "ticket not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	messages, err := s.st.TicketMessages(t.ID, true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t, "messages": messages})
}

func (s *server) adminTicketReply(w http.ResponseWriter, r *http.Request) {
	s.adminTicketMessage(w, r, false)
}

func (s *server) adminTicketNote(w http.ResponseWriter, r *http.Request) {
	s.adminTicketMessage(w, r, true)
}

func (s *server) adminTicketMessage(w http.ResponseWriter, r *http.Request, internal bool) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "ticket not found")
		return
	}
	var in struct {
		Body   string `json:"body"`
		Status string `json:"status"`
	}
	if !decode(w, r, &in) {
		return
	}
	t, err := s.st.GetTicket(id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "ticket not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	u := s.currentUser(r)
	status := ""
	if !internal {
		status = strings.TrimSpace(in.Status)
		if status == "" {
			status = store.TicketWaiting
		}
	}
	t, msg, err := s.st.AddTicketMessage(t.ID, u.ID, "admin", ticketName(u), in.Body, internal, status)
	if err != nil {
		ticketWriteError(w, err)
		return
	}
	action := "ticket-reply"
	if internal {
		action = "ticket-note"
	}
	s.eventRequest(r, action, "", map[string]any{"ticket": t.ID, "message": msg.ID})
	if !internal {
		s.notifyTicketForReply(r, t, "admin", strconv.FormatInt(msg.ID, 10), u.Email, ticketName(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t, "message": msg})
}

func (s *server) adminTicketStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "ticket not found")
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.SetTicketStatus(id, in.Status); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "ticket not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eventRequest(r, "ticket-status", "", map[string]any{"ticket": id, "status": in.Status})
	t, _ := s.st.GetTicket(id)
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t})
}

func (s *server) adminTicketAssign(w http.ResponseWriter, r *http.Request) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "ticket not found")
		return
	}
	var in struct {
		AssigneeID int64 `json:"assigneeId"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.st.AssignTicket(id, in.AssigneeID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "ticket not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eventRequest(r, "ticket-assign", "", map[string]any{"ticket": id, "assignee": in.AssigneeID})
	t, _ := s.st.GetTicket(id)
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t})
}

// ---------------------------------------------------------------------------
// Notifications

func uniqueRecipients(in []store.MailRecipient, skipEmail string) []store.MailRecipient {
	seen := map[string]bool{}
	out := []store.MailRecipient{}
	for _, r := range in {
		email := strings.ToLower(strings.TrimSpace(r.Email))
		if email == "" || email == strings.ToLower(skipEmail) || seen[email] {
			continue
		}
		seen[email] = true
		r.Email = email
		out = append(out, r)
	}
	return out
}

func (s *server) ticketRecipients(t store.Ticket, admins, team, opener bool) ([]store.MailRecipient, error) {
	out := []store.MailRecipient{}
	if admins {
		rs, err := s.st.AdminRecipients()
		if err != nil {
			return nil, err
		}
		out = append(out, rs...)
	}
	if team && t.GroupID != 0 {
		rs, err := s.st.TeamNotifyRecipients(t.GroupID, "managers", nil)
		if err != nil {
			return nil, err
		}
		for i := range rs {
			rs[i].Admin = false
			rs[i].Manager = true
		}
		out = append(out, rs...)
	}
	if opener && t.UserID != 0 {
		if u, ok, err := s.st.UserByID(t.UserID); err != nil {
			return nil, err
		} else if ok && u.DisabledAt == 0 {
			out = append(out, store.MailRecipient{Email: u.Email, Name: ticketName(u)})
		}
	}
	return out, nil
}

func (s *server) notifyTicket(r *http.Request, t store.Ticket, event, dedupe string, admins, team, opener bool, authorEmail, author string) {
	if s.mail == nil || !s.mail.Delivers() {
		return
	}
	recipients, err := s.ticketRecipients(t, admins, team, opener)
	if err != nil {
		log.Printf("email: reading support ticket recipients: %v", err)
		return
	}
	recipients = uniqueRecipients(recipients, authorEmail)
	base := s.mailBase(r)
	for _, rc := range recipients {
		link := base + "/support/" + strconv.FormatInt(t.ID, 10)
		if rc.Admin {
			link = base + "/admin/support?ticket=" + strconv.FormatInt(t.ID, 10)
		}
		d := mail.Data{Ticket: t.Subject, Team: t.GroupName, Actor: author, Link: link, When: time.Now()}
		if err := s.mail.TicketNotice(r.Context(), t.ID, t.GroupID, event, dedupe, rc, d); err != nil {
			log.Printf("email: queueing a support ticket notice: %v", err)
		}
	}
}

func (s *server) notifyTicketForReply(r *http.Request, t store.Ticket, kind, dedupe, authorEmail, author string) {
	switch kind {
	case "user":
		s.notifyTicket(r, t, "reply", dedupe, true, true, false, authorEmail, author)
	case "team":
		s.notifyTicket(r, t, "reply", dedupe, false, false, true, authorEmail, author)
	case "admin":
		s.notifyTicket(r, t, "reply", dedupe, false, true, true, authorEmail, author)
	}
}
