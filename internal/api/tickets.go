package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/notify"
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
	s.emitTicketOpened(t, u)
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
	s.emitTicketReply(t, kind, msg.ID, u)
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
		s.emitTicketReply(t, "admin", msg.ID, u)
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
	assignee, _, _ := s.st.UserByID(in.AssigneeID)
	t, _ := s.st.GetTicket(id)
	s.emitTicketAssigned(t, assignee, s.currentUser(r))
	s.eventRequest(r, "ticket-assign", "", map[string]any{"ticket": id, "assignee": in.AssigneeID})
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t})
}

// ---------------------------------------------------------------------------
// Notifications

func (s *server) emit(ev notify.Event) {
	if s.notify == nil {
		return
	}
	if err := s.notify.Emit(context.Background(), ev); err != nil {
		log.Printf("notifications: %s: %v", ev.Type, err)
	}
}

func (s *server) managerRecipients(t store.Ticket, includeAdmins bool) []notify.Recipient {
	if t.GroupID == 0 {
		return nil
	}
	rs, err := s.st.TeamNotifyRecipients(t.GroupID, "managers", nil)
	if err != nil {
		log.Printf("notifications: reading a team's managers: %v", err)
		return nil
	}
	out := []notify.Recipient{}
	for _, r := range rs {
		// When a staff kind is also emitted, an instance administrator
		// already receives that and does not need the team copy too.
		if !includeAdmins && (r.Role == "owner" || r.Role == "admin") {
			continue
		}
		out = append(out, notify.Recipient{UserID: r.ID, Email: r.Email, Name: r.Name, Role: r.Role,
			Admin: r.Role == "owner" || r.Role == "admin", Manager: true})
	}
	return out
}

func (s *server) openerRecipient(t store.Ticket) []notify.Recipient {
	if t.UserID == 0 {
		return nil
	}
	u, ok, err := s.st.UserByID(t.UserID)
	if err != nil || !ok || u.DisabledAt != 0 {
		return nil
	}
	return []notify.Recipient{{UserID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role}}
}

func ticketTitle(prefix string, t store.Ticket) string { return prefix + ": " + t.Subject }

func (s *server) emitTicketOpened(t store.Ticket, by store.User) {
	data := mail.Data{Ticket: t.Subject, Team: t.GroupName, Actor: ticketName(by), When: time.Now()}
	s.emit(notify.Event{
		Type: "staff_ticket_opened", Key: "ticket:" + strconv.FormatInt(t.ID, 10) + ":opened:staff",
		TeamID: t.GroupID, Title: ticketTitle("New support ticket", t), Body: ticketName(by) + " opened a support ticket.",
		Link: "/support/" + strconv.FormatInt(t.ID, 10), AdminLink: "/admin/support?ticket=" + strconv.FormatInt(t.ID, 10),
		Data: data, Except: by.ID,
	})
	if t.GroupID != 0 {
		s.emit(notify.Event{
			Type: "ticket_opened", Key: "ticket:" + strconv.FormatInt(t.ID, 10) + ":opened:team",
			TeamID: t.GroupID, Title: ticketTitle("New team ticket", t), Body: ticketName(by) + " opened a ticket for " + t.GroupName + ".",
			Link: "/support/" + strconv.FormatInt(t.ID, 10), Data: data, To: s.managerRecipients(t, false), Except: by.ID,
		})
	}
}

func (s *server) emitTicketReply(t store.Ticket, kind string, messageID int64, by store.User) {
	data := mail.Data{Ticket: t.Subject, Team: t.GroupName, Actor: ticketName(by), When: time.Now()}
	key := "ticket:" + strconv.FormatInt(t.ID, 10) + ":reply:" + strconv.FormatInt(messageID, 10)
	id := strconv.FormatInt(t.ID, 10)
	switch kind {
	case "user":
		s.emit(notify.Event{Type: "staff_ticket_replied", Key: key + ":staff", TeamID: t.GroupID,
			Title: ticketTitle("Requester replied", t), Body: ticketName(by) + " replied on ticket #" + id + ".",
			Link: "/support/" + id, AdminLink: "/admin/support?ticket=" + id, Data: data, Except: by.ID})
		if t.GroupID != 0 {
			s.emit(notify.Event{Type: "ticket_replied", Key: key + ":team", TeamID: t.GroupID,
				Title: ticketTitle("Reply on", t), Body: ticketName(by) + " replied on ticket #" + id + ".",
				Link: "/support/" + id, Data: data, To: s.managerRecipients(t, false), Except: by.ID})
		}
	case "team":
		if to := s.openerRecipient(t); len(to) > 0 {
			s.emit(notify.Event{Type: "ticket_replied", Key: key + ":user", TeamID: t.GroupID,
				Title: ticketTitle("Reply on", t), Body: ticketName(by) + " replied on ticket #" + id + ".",
				Link: "/support/" + id, Data: data, To: to, Except: by.ID})
		}
	case "admin":
		to := append(s.openerRecipient(t), s.managerRecipients(t, true)...)
		if len(to) > 0 {
			s.emit(notify.Event{Type: "ticket_replied", Key: key + ":user", TeamID: t.GroupID,
				Title: ticketTitle("Support replied", t), Body: ticketName(by) + " replied on ticket #" + id + ".",
				Link: "/support/" + id, Data: data, To: to, Except: by.ID})
		}
	}
}

func (s *server) emitTicketAssigned(t store.Ticket, assignee store.User, by store.User) {
	if assignee.ID == 0 {
		return
	}
	data := mail.Data{Ticket: t.Subject, Team: t.GroupName, Actor: ticketName(by), When: time.Now()}
	s.emit(notify.Event{Type: "staff_ticket_assigned",
		Key:    "ticket:" + strconv.FormatInt(t.ID, 10) + ":assigned:" + strconv.FormatInt(assignee.ID, 10) + ":" + strconv.FormatInt(t.UpdatedAt, 10),
		TeamID: t.GroupID, Title: ticketTitle("Ticket assigned", t), Body: ticketName(by) + " assigned ticket #" + strconv.FormatInt(t.ID, 10) + " to you.",
		Link: "/support/" + strconv.FormatInt(t.ID, 10), AdminLink: "/admin/support?ticket=" + strconv.FormatInt(t.ID, 10),
		Data: data, To: []notify.Recipient{{UserID: assignee.ID, Email: assignee.Email, Name: assignee.Name, Role: assignee.Role, Admin: true}}})
}
