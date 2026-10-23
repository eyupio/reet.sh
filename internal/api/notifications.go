package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/notify"
	"github.com/eyupio/reet.sh/internal/store"
)

// The notification centre is one list per person: their ticket replies,
// account security notices, and -- for the owner and administrators -- the
// staff events they chose to receive. The owner's platform defaults and each
// person's own choices live here too.

func (s *server) registerNotificationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/notifications", s.notificationList)
	mux.HandleFunc("GET /api/v2/notifications/count", s.notificationCount)
	mux.HandleFunc("POST /api/v2/notifications/read", s.notificationRead)
	mux.HandleFunc("GET /api/v2/notifications/preferences", s.notificationPreferences)
	mux.HandleFunc("PATCH /api/v2/notifications/preferences", s.saveNotificationPreferences)
	mux.HandleFunc("GET /api/v2/admin/notifications/defaults", s.requireOwner(s.notificationDefaults))
	mux.HandleFunc("PATCH /api/v2/admin/notifications/defaults", s.requireOwner(s.saveNotificationDefaults))
}

func (s *server) notificationsReady(w http.ResponseWriter) bool {
	if s.notify == nil {
		writeErr(w, http.StatusServiceUnavailable, "notifications are not wired into this instance")
		return false
	}
	return true
}

func (s *server) notificationList(w http.ResponseWriter, r *http.Request) {
	if !s.notificationsReady(w) {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	unreadOnly := r.URL.Query().Get("unread") == "1" || strings.EqualFold(r.URL.Query().Get("unread"), "true")
	rows, err := s.st.NotificationsFor(s.uid(r), unreadOnly, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	unread, err := s.st.UnreadNotifications(s.uid(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": rows, "unread": unread})
}

func (s *server) notificationCount(w http.ResponseWriter, r *http.Request) {
	if !s.notificationsReady(w) {
		return
	}
	unread, err := s.st.UnreadNotifications(s.uid(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unread": unread})
}

func (s *server) notificationRead(w http.ResponseWriter, r *http.Request) {
	if !s.notificationsReady(w) {
		return
	}
	var in struct {
		IDs  []int64 `json:"ids"`
		Read *bool   `json:"read"`
	}
	if !decode(w, r, &in) {
		return
	}
	read := true
	if in.Read != nil {
		read = *in.Read
	}
	if _, err := s.st.MarkNotificationsRead(s.uid(r), in.IDs, read); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	unread, _ := s.st.UnreadNotifications(s.uid(r))
	writeJSON(w, http.StatusOK, map[string]any{"unread": unread})
}

func (s *server) notificationPreferences(w http.ResponseWriter, r *http.Request) {
	if !s.notificationsReady(w) {
		return
	}
	views, err := s.notify.Preferences(s.role(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preferences": views})
}

func (s *server) saveNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	if !s.notificationsReady(w) {
		return
	}
	var in struct {
		Preferences []notify.Preference `json:"preferences"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.notify.SetPreferences(s.uid(r), s.role(r), in.Preferences); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.eventRequest(r, "notification-prefs", "", map[string]any{"changed": len(in.Preferences)})
	views, err := s.notify.Preferences(s.role(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preferences": views, "done": "Saved. Your choices apply to the next notification."})
}

func (s *server) notificationDefaults(w http.ResponseWriter, r *http.Request) {
	if !s.notificationsReady(w) {
		return
	}
	views, err := s.notify.Defaults()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"defaults": views})
}

func (s *server) saveNotificationDefaults(w http.ResponseWriter, r *http.Request) {
	if !s.notificationsReady(w) {
		return
	}
	var in struct {
		Defaults []notify.Default `json:"defaults"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.notify.SetDefaults(s.currentUser(r).Email, in.Defaults); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.eventRequest(r, "notification-defaults", "", map[string]any{"changed": len(in.Defaults)})
	views, err := s.notify.Defaults()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"defaults": views, "done": "Saved. These are the defaults for everybody who has not chosen otherwise."})
}

// emitNewAccount tells the owner and administrators that somebody new has
// an account. The new person is left out: they know.
func (s *server) emitNewAccount(u store.User) {
	s.emit(notify.Event{
		Type: "staff_new_account", Key: "account:" + strconv.FormatInt(u.ID, 10) + ":new",
		Title: "New account: " + ticketName(u), Body: ticketName(u) + " (" + u.Email + ") created an account.",
		Link: "/admin/users", Data: mail.Data{Actor: ticketName(u), Email: u.Email, Role: u.Role, When: time.Now()},
		Except: u.ID,
	})
}

// emitPasswordChanged records and sends a security notice. The notifier owns
// it when wired; a direct email is the fallback for a caller with only mail.
func (s *server) emitPasswordChanged(u store.User, at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}
	if s.notify != nil {
		s.emit(notify.Event{
			Type: "security_password_changed", Key: fmt.Sprintf("pwchanged:%d:%d", u.ID, at.UnixMilli()),
			Title: "Your password changed", Body: "Your password was changed and every other session was signed out.",
			Link: "/account/profile", Tone: "warning", Data: mail.Data{When: at},
			To: []notify.Recipient{{UserID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role}},
		})
		return
	}
	if s.mail != nil {
		if err := s.mail.PasswordChanged(context.Background(), u, at); err != nil {
			log.Printf("email: queueing a password-change notice: %v", err)
		}
	}
}
