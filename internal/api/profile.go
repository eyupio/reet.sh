package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

func (s *server) registerProfileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/profile", s.profileGet)
	mux.HandleFunc("PATCH /api/v2/profile", s.profileSave)
	mux.HandleFunc("POST /api/v2/profile/password", s.requireWriter(s.profilePassword))
	mux.HandleFunc("GET /api/v2/profile/sessions", s.profileSessions)
	mux.HandleFunc("DELETE /api/v2/profile/sessions/{id}", s.requireWriter(s.profileEndSession))
	mux.HandleFunc("POST /api/v2/profile/sessions/end-others", s.requireWriter(s.profileEndOtherSessions))
	mux.HandleFunc("GET /api/v2/profile/activity", s.profileActivity)
}

func (s *server) profileGet(w http.ResponseWriter, r *http.Request) {
	user := s.currentUser(r)
	profile, err := s.st.ProfileFor(user.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	sessions, err := s.st.SessionsFor(user.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"user":         user,
		"profile":      profile,
		"sessionCount": len(sessions),
		"accounts":     s.authMode == AuthAccounts,
	})
}

func (s *server) profileSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name                  *string `json:"name"`
		Theme                 *string `json:"theme"`
		Timezone              *string `json:"timezone"`
		DateFormat            *string `json:"dateFormat"`
		ShelfView             *string `json:"shelfView"`
		CardsExpanded         *bool   `json:"cardsExpanded"`
		TerminalRemoveOnClose *bool   `json:"terminalRemoveOnClose"`
		HostedPrefix          *string `json:"hostedPrefix"`
	}
	if !decode(w, r, &in) {
		return
	}
	user := s.currentUser(r)
	if user.Role == "viewer" && in.Name != nil {
		writeErr(w, http.StatusForbidden, "read-only accounts can change their display preferences, but not their name")
		return
	}
	// Validate all new defaults before applying any profile changes.
	if in.ShelfView != nil && *in.ShelfView != "list" && *in.ShelfView != "board" {
		writeErr(w, http.StatusBadRequest, "shelfView must be list or board")
		return
	}
	hostedPrefix := ""
	if in.HostedPrefix != nil {
		var err error
		hostedPrefix, err = store.NormalizeHostedPrefix(*in.HostedPrefix)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	profile, err := s.st.ProfileFor(user.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	detail := map[string]any{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if len(name) > 100 {
			writeErr(w, 400, "name is too long")
			return
		}
		if name != user.Name {
			if err := s.st.UpdateUserName(user.ID, name); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			detail["name"] = name
		}
	}
	if in.Theme != nil {
		theme := strings.ToLower(strings.TrimSpace(*in.Theme))
		switch theme {
		case "system", "light", "dark":
			profile.Theme = theme
			detail["theme"] = theme
		default:
			writeErr(w, 400, "theme must be system, light or dark")
			return
		}
	}
	if in.Timezone != nil {
		tz := strings.TrimSpace(*in.Timezone)
		if len(tz) > 64 {
			writeErr(w, 400, "time zone is too long")
			return
		}
		profile.Timezone = tz
		detail["timezone"] = tz
	}
	if in.DateFormat != nil {
		format := strings.ToLower(strings.TrimSpace(*in.DateFormat))
		switch format {
		case "", "locale", "dmy", "mdy", "ymd":
			if format == "" {
				format = "locale"
			}
			profile.DateFormat = format
			detail["dateFormat"] = format
		default:
			writeErr(w, 400, "dateFormat must be locale, dmy, mdy or ymd")
			return
		}
	}
	if in.ShelfView != nil {
		profile.ShelfView = *in.ShelfView
		detail["shelfView"] = *in.ShelfView
	}
	if in.CardsExpanded != nil {
		profile.CardsExpanded = *in.CardsExpanded
		detail["cardsExpanded"] = *in.CardsExpanded
	}
	if in.TerminalRemoveOnClose != nil {
		profile.TerminalRemoveOnClose = *in.TerminalRemoveOnClose
		detail["terminalRemoveOnClose"] = *in.TerminalRemoveOnClose
	}
	if in.HostedPrefix != nil {
		profile.HostedPrefix = hostedPrefix
		detail["hostedPrefix"] = hostedPrefix
	}
	if err := s.st.SaveProfileFor(user.ID, profile); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if saved, err := s.st.ProfileFor(user.ID); err == nil {
		profile = saved
	}
	if len(detail) > 0 {
		s.event(user.ID, "ui", "profile", "", detail)
	}
	updated, _, _ := s.st.UserByID(user.ID)
	writeJSON(w, 200, map[string]any{"user": updated, "profile": profile})
}

func (s *server) profilePassword(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthAccounts {
		writeErr(w, http.StatusNotFound, "accounts are not enabled")
		return
	}
	var in struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	user := s.currentUser(r)
	if user.PasswordHash == "" {
		writeErr(w, 400, "this account has no password; ask an admin to reset it")
		return
	}
	keys := []string{clientIP(r), "profile:" + strconv.FormatInt(user.ID, 10)}
	if !s.limiter.Allow(keys...) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	s.limiter.Hit(keys...)
	if !auth.VerifyPassword(user.PasswordHash, in.Current) {
		writeErr(w, http.StatusUnprocessableEntity, "that is not your current password")
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, 400, "the new password must be at least 8 characters")
		return
	}
	if len(in.Password) > 1024 {
		writeErr(w, 400, "the new password is too long")
		return
	}
	if strings.EqualFold(strings.TrimSpace(in.Password), user.Email) {
		writeErr(w, 400, "choose a password that is not your email address")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, 500, "could not change the password")
		return
	}
	if err := s.st.SetUserPasswordKeep(user.ID, hash); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// This browser gets a fresh session and every other one ends, so a
	// password change really does put a stranger out.
	token, err := s.newSessionFor(w, r, user)
	if err != nil {
		writeErr(w, 500, "the password changed but a new session could not be started")
		return
	}
	_, _ = s.st.DeleteOtherSessions(user.ID, auth.HashToken(token))
	s.event(user.ID, "ui", "password-change", "", map[string]any{"name": user.Name})
	s.emitPasswordChanged(user, time.Now())
	writeJSON(w, 200, map[string]any{"ok": true})
}

// newSessionFor is newSession for an already-resolved user, without returning
// a second copy of the record.
func (s *server) newSessionFor(w http.ResponseWriter, r *http.Request, user store.User) (string, error) {
	token, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.st.CreateSession(auth.HashToken(token), user.ID, expires.UnixMilli(), r.UserAgent()); err != nil {
		return "", err
	}
	s.setSession(w, r, token, expires)
	return token, nil
}

func (s *server) profileSessions(w http.ResponseWriter, r *http.Request) {
	user := s.currentUser(r)
	sessions, err := s.st.SessionsFor(user.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	current := ""
	if c, err := r.Cookie(sessionCookie); err == nil {
		current = auth.HashToken(c.Value)
	}
	out := make([]map[string]any, 0, len(sessions))
	for _, x := range sessions {
		out = append(out, map[string]any{
			"id": x.ID, "device": x.UserAgent, "createdAt": x.CreatedAt,
			"lastSeen": x.LastSeen, "expiresAt": x.ExpiresAt, "current": x.ID == current,
		})
	}
	writeJSON(w, 200, map[string]any{"sessions": out})
}

func (s *server) profileEndSession(w http.ResponseWriter, r *http.Request) {
	user := s.currentUser(r)
	id := r.PathValue("id")
	if c, err := r.Cookie(sessionCookie); err == nil && auth.HashToken(c.Value) == id {
		writeErr(w, 400, "that is the session you are using; sign out instead")
		return
	}
	if err := s.st.DeleteSessionFor(user.ID, id); errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "that session has already ended")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(user.ID, "ui", "session-end", "", map[string]any{"session": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) profileEndOtherSessions(w http.ResponseWriter, r *http.Request) {
	user := s.currentUser(r)
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		writeErr(w, 400, "this request carries no session to keep")
		return
	}
	n, err := s.st.DeleteOtherSessions(user.ID, auth.HashToken(c.Value))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.event(user.ID, "ui", "session-end-others", "", map[string]any{"ended": n})
	writeJSON(w, 200, map[string]any{"ended": n})
}

func (s *server) profileActivity(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	events, err := s.st.ListEventsFor(s.uid(r), limit, "")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"events": events})
}
