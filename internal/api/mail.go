package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/store"
)

// Email is two layers: the instance owner names the one mail server every
// message goes through, on Admin, Email; each team's owner or managers choose
// what their own team is emailed about, under Teams & organisations. The outbox
// is written before anything sends, so a broken mail server costs a retry, not
// an email.

type mailSettingsView struct {
	Source      string `json:"source"`
	Mode        string `json:"mode"`
	Summary     string `json:"summary"`
	Problem     string `json:"problem,omitempty"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	TLS         string `json:"tls"`
	User        string `json:"user"`
	PasswordSet bool   `json:"passwordSet"`
	From        string `json:"from"`
	FromName    string `json:"fromName"`
	ReplyTo     string `json:"replyTo"`
	BaseURL     string `json:"baseUrl"`
	// BaseURLInUse is where links start now: the saved address, REET_PUBLIC_URL,
	// or the address the page was opened at. Kept apart so saving the page
	// never pins a guessed address.
	BaseURLInUse string `json:"baseUrlInUse"`
	DKIMSelector string `json:"dkimSelector"`
	UpdatedAt    int64  `json:"updatedAt,omitempty"`
	UpdatedBy    string `json:"updatedBy,omitempty"`
}

type mailEventView struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

var teamMailEvents = []mailEventView{
	{Key: "member_joined", Label: "People join", Description: "When somebody is added to the team."},
	{Key: "member_left", Label: "People leave", Description: "When somebody is removed from the team."},
	{Key: "member_role", Label: "Role changes", Description: "When somebody becomes a manager, or stops being one."},
	{Key: "list_shared", Label: "Lists shared", Description: "When a list is shared with the team."},
}

func (s *server) registerMailRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/admin/mail", s.requireAdmin(s.mailSettings))
	mux.HandleFunc("PATCH /api/v2/admin/mail", s.requireOwner(s.saveMailSettings))
	mux.HandleFunc("POST /api/v2/admin/mail/check", s.requireOwner(s.checkMailSettings))
	mux.HandleFunc("POST /api/v2/admin/mail/test", s.requireOwner(s.testMail))
	mux.HandleFunc("GET /api/v2/admin/mail/dns", s.requireAdmin(s.mailDNS))
	mux.HandleFunc("GET /api/v2/admin/mail/outbox", s.requireAdmin(s.mailOutbox))
	mux.HandleFunc("POST /api/v2/admin/mail/outbox/resend", s.requireOwner(s.resendMail))
	mux.HandleFunc("POST /api/v2/auth/forgot", s.forgotPassword)
	mux.HandleFunc("POST /api/v2/auth/reset", s.resetPassword)
	mux.HandleFunc("GET /api/v2/groups/{id}/notifications", s.groupNotifications)
	mux.HandleFunc("PATCH /api/v2/groups/{id}/notifications", s.requireWriter(s.saveGroupNotifications))
}

func (s *server) mailReady(w http.ResponseWriter) bool {
	if s.mail == nil || s.mail.Settings == nil {
		writeErr(w, http.StatusServiceUnavailable, "email is not wired into this instance")
		return false
	}
	return true
}

func (s *server) mailSettingsView(r *http.Request) (mailSettingsView, error) {
	v := mailSettingsView{Source: "none", Mode: "held", Summary: "Email is not wired into this instance.",
		TLS: mail.TLSStartTLS, Port: 587, FromName: "reet.sh", BaseURLInUse: requestOrigin(r)}
	if s.mail == nil || s.mail.Settings == nil {
		return v, nil
	}
	snap := s.mail.Settings.Snapshot()
	v.Source, v.Mode, v.Summary, v.Problem = snap.Source, snap.Config.Mode(), snap.Config.Summary(), snap.Problem
	if snap.Config.BaseURL != "" {
		v.BaseURLInUse = snap.Config.BaseURL
	}
	saved, ok, err := s.mail.Store.GetMailSettings()
	if err != nil {
		return v, err
	}
	if ok {
		v.Host, v.Port, v.TLS, v.User, v.PasswordSet = saved.Host, saved.Port, saved.TLS, saved.User, saved.PasswordSet
		v.From, v.FromName, v.ReplyTo, v.BaseURL = saved.From, saved.FromName, saved.ReplyTo, saved.BaseURL
		v.DKIMSelector, v.UpdatedAt, v.UpdatedBy = saved.DKIMSelector, saved.UpdatedAt, saved.UpdatedBy
		if v.Port == 0 {
			v.Port = 587
		}
		if v.TLS == "" {
			v.TLS = mail.TLSStartTLS
		}
		if saved.BaseURL != "" {
			v.BaseURLInUse = saved.BaseURL
		}
	}
	return v, nil
}

func (s *server) mailSettings(w http.ResponseWriter, r *http.Request) {
	v, err := s.mailSettingsView(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": v})
}

func mailFormError(w http.ResponseWriter, err error) {
	if errors.Is(err, mail.ErrNoSealer) {
		writeErr(w, http.StatusConflict, "this instance has no encryption key, so it will not store a mail password. Set REET_BACKUP_KEY and restart, then save again")
		return
	}
	msg := strings.TrimPrefix(err.Error(), "email: ")
	if msg == "" {
		msg = "the settings could not be saved"
	}
	msg = strings.ToUpper(msg[:1]) + msg[1:]
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	writeErr(w, http.StatusUnprocessableEntity, msg)
}

func (s *server) saveMailSettings(w http.ResponseWriter, r *http.Request) {
	if !s.mailReady(w) {
		return
	}
	var f mail.Form
	if !decode(w, r, &f) {
		return
	}
	c, err := s.mail.Settings.Save(f, s.currentUser(r).Email)
	if err != nil {
		mailFormError(w, err)
		return
	}
	s.eventRequest(r, "mail-settings", "", map[string]any{"host": c.Host, "from": c.From.Address, "baseUrl": c.BaseURL})
	v, _ := s.mailSettingsView(r)
	writeJSON(w, http.StatusOK, map[string]any{"settings": v, "done": "Saved. The next message is sent with these settings."})
}

func (s *server) checkMailSettings(w http.ResponseWriter, r *http.Request) {
	if !s.mailReady(w) {
		return
	}
	var f mail.Form
	if !decode(w, r, &f) {
		return
	}
	if strings.TrimSpace(f.BaseURL) == "" {
		f.BaseURL = requestOrigin(r)
	}
	c, _, err := s.mail.Settings.Resolve(f)
	if err != nil {
		mailFormError(w, err)
		return
	}
	if !c.SMTP() {
		writeErr(w, http.StatusUnprocessableEntity, "enter the mail server's host name to check it")
		return
	}
	tr, err := (&mail.SMTP{Config: c}).Check(r.Context())
	detail := "connected, secured and signed in to " + c.Host
	if err != nil {
		detail = "check failed against " + c.Host + ": " + err.Error()
	}
	s.eventRequest(r, "mail-check", "", map[string]any{"host": c.Host, "ok": err == nil, "detail": detail})
	out := mailCheckResult(tr, err, "The server accepted the connection, the encryption and the sign-in. Nothing was sent.")
	writeJSON(w, http.StatusOK, out)
}

func mailCheckResult(tr mail.Transcript, err error, ok string) map[string]any {
	lines := []string(tr)
	if lines == nil {
		lines = []string{}
	}
	if err != nil {
		msg := strings.TrimPrefix(err.Error(), "smtp: ")
		if msg == "" {
			msg = "The check failed."
		} else {
			msg = strings.ToUpper(msg[:1]) + msg[1:]
			if !strings.HasSuffix(msg, ".") {
				msg += "."
			}
		}
		return map[string]any{"ok": false, "message": msg, "transcript": lines}
	}
	return map[string]any{"ok": true, "message": ok, "transcript": lines}
}

func (s *server) testMail(w http.ResponseWriter, r *http.Request) {
	if !s.mailReady(w) {
		return
	}
	u := s.currentUser(r)
	c := s.mail.Current()
	if !c.SMTP() {
		writeErr(w, http.StatusUnprocessableEntity, "save a mail server first; the test is sent with the settings in force")
		return
	}
	if c.BaseURL == "" {
		c.BaseURL = requestOrigin(r)
	}
	rendered, err := mail.Compose(mail.TplTest, mail.Data{Name: u.Name, Email: u.Email, Instance: c.Host, Base: c.BaseURL})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	tr, err := (&mail.SMTP{Config: c}).SendTraced(r.Context(), mail.Message{
		ID: "test-" + strconv.FormatInt(time.Now().UnixNano(), 10), From: c.From, To: u.Email, ReplyTo: c.ReplyTo,
		Subject: rendered.Subject, Text: rendered.Text, HTML: rendered.HTML, Date: time.Now(),
	})
	detail := "sent to " + u.Email + " through " + c.Host
	if err != nil {
		detail = "failed to send to " + u.Email + " through " + c.Host + ": " + err.Error()
	}
	s.eventRequest(r, "mail-test", "", map[string]any{"to": u.Email, "host": c.Host, "ok": err == nil, "detail": detail})
	out := mailCheckResult(tr, err, "The server accepted the test email for "+u.Email+". Check that it arrived, and not in spam.")
	writeJSON(w, http.StatusOK, out)
}

func (s *server) mailDNS(w http.ResponseWriter, r *http.Request) {
	if !s.mailReady(w) {
		return
	}
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	if from == "" {
		from = s.mail.Current().From.Address
	}
	if !strings.Contains(from, "@") || strings.HasSuffix(from, "@localhost") {
		writeErr(w, http.StatusUnprocessableEntity, "enter the from address whose domain should be checked")
		return
	}
	selector := r.URL.Query().Get("selector")
	if selector == "" {
		selector = s.mail.Current().DKIMSelector
	}
	writeJSON(w, http.StatusOK, mail.CheckDNS(r.Context(), nil, from, selector))
}

func (s *server) mailOutbox(w http.ResponseWriter, r *http.Request) {
	if s.mail == nil || s.mail.Settings == nil {
		writeJSON(w, http.StatusOK, map[string]any{"mode": "off", "summary": "Email is not wired into this instance.",
			"messages": []any{}, "counts": map[string]int{}, "warning": "Email is not wired into this instance."})
		return
	}
	ms, err := s.st.MailListing(r.URL.Query().Get("state"), 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	counts, err := s.st.MailCounts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg := s.mail.Current()
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, mailMessageView(m, cfg.Delivers()))
	}
	resp := map[string]any{"mode": cfg.Mode(), "summary": cfg.Summary(), "messages": out, "counts": counts}
	if !cfg.Delivers() {
		resp["warning"] = "No mail server is configured. Password resets hand over a temporary password and invitations show a link to copy; welcome and password-change messages are held here and nobody receives them. Save a server above; it applies at once."
	}
	writeJSON(w, http.StatusOK, resp)
}

func mailMessageView(m store.Mail, delivers bool) map[string]any {
	resendable := delivers && m.State != store.MailQueued && m.State != store.MailSending && !m.Spent()
	return map[string]any{
		"id": m.ID, "template": m.Template, "to": m.To, "subject": m.Subject, "state": m.State,
		"attempts": m.Attempts, "lastError": m.LastError, "via": m.Via, "createdAt": m.CreatedAt,
		"nextAttemptAt": m.NextAttemptAt, "deliveredAt": m.DeliveredAt, "secret": m.Secret, "resendable": resendable,
	}
}

func (s *server) resendMail(w http.ResponseWriter, r *http.Request) {
	if !s.mailReady(w) {
		return
	}
	if !s.mail.Delivers() {
		writeErr(w, http.StatusConflict, "no mail server is configured, so a resend would be held again. Save one first")
		return
	}
	var in struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	m, err := s.st.RequeueMail(strings.TrimSpace(in.ID))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "message not found")
		return
	case errors.Is(err, store.ErrMailWaiting):
		writeErr(w, http.StatusConflict, "that message is already waiting to be sent")
		return
	case errors.Is(err, store.ErrMailSpent):
		writeErr(w, http.StatusUnprocessableEntity, "that message carried a sign-in link that is spent; send a new one")
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eventRequest(r, "mail-resend", "", map[string]any{"id": m.ID, "to": m.To, "template": m.Template})
	if s.mail.Queue != nil {
		s.mail.Queue.Kick()
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": mailMessageView(m, true), "done": "Queued to send again to " + m.To + "."})
}

// ---------------------------------------------------------------------------
// Password resets

func (s *server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		writeErr(w, http.StatusBadRequest, "enter your email address")
		return
	}
	keys := []string{"forgot:" + email, "forgot-ip:" + clientIP(r)}
	if !s.limiter.Allow(keys...) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	s.limiter.Hit(keys...)
	// The answer is the same whether or not the account exists.
	answer := map[string]string{"message": "If that address has an account here, a reset link is on its way. It works once, for 30 minutes."}
	if s.mail == nil || !s.mail.Delivers() {
		writeJSON(w, http.StatusOK, answer)
		return
	}
	u, ok, err := s.st.UserByEmail(email)
	if err != nil {
		log.Printf("email: reading an account for a reset: %v", err)
		writeJSON(w, http.StatusOK, answer)
		return
	}
	if ok && u.DisabledAt == 0 {
		if err := s.mail.PasswordReset(r.Context(), u, mail.ResetTTL); err != nil {
			log.Printf("email: queueing a password reset: %v", err)
		} else {
			s.event(u.ID, "ui", "password-reset-request", "", map[string]any{"email": u.Email})
		}
	}
	writeJSON(w, http.StatusOK, answer)
}

func (s *server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	keys := []string{"reset-ip:" + clientIP(r)}
	if !s.limiter.Allow(keys...) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	s.limiter.Hit(keys...)
	token := strings.TrimSpace(in.Token)
	if token == "" {
		writeErr(w, http.StatusBadRequest, "that reset link is not complete")
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "the new password must be at least 8 characters")
		return
	}
	if len(in.Password) > 1024 {
		writeErr(w, http.StatusBadRequest, "the new password is too long")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not set the new password")
		return
	}
	u, ok, err := s.st.ResetPasswordWithToken(auth.HashToken(token), hash, time.Now().UnixMilli())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "that reset link is invalid, has expired, or has already been used. Ask for a new one")
		return
	}
	s.event(u.ID, "ui", "password-reset", "", map[string]any{"name": u.Name, "email": u.Email})
	s.emitPasswordChanged(u, time.Now())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Your password changed. Every other session was signed out."})
}

// ---------------------------------------------------------------------------
// Team notifications

type teamNotifyView struct {
	Notify    store.TeamNotify `json:"notify"`
	Events    []mailEventView  `json:"events"`
	MailReady bool             `json:"mailReady"`
	MailMode  string           `json:"mailMode"`
	GroupName string           `json:"groupName"`
}

func (s *server) groupNotifyAccess(w http.ResponseWriter, r *http.Request) (store.Group, bool) {
	id, ok := listIDFrom(r)
	if !ok {
		writeErr(w, http.StatusNotFound, "group not found")
		return store.Group{}, false
	}
	g, err := s.st.GroupFor(s.uid(r), s.role(r), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "group not found")
		return store.Group{}, false
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return store.Group{}, false
	}
	if g.Role != "owner" && g.Role != "manager" {
		writeErr(w, http.StatusForbidden, "only this group's owner or managers can change its notifications")
		return store.Group{}, false
	}
	return g, true
}

func (s *server) groupNotifyView(g store.Group) teamNotifyView {
	n, _ := s.st.TeamNotifyFor(g.ID)
	v := teamNotifyView{Notify: n, Events: teamMailEvents, GroupName: g.Name, MailMode: "off"}
	if s.mail != nil && s.mail.Settings != nil {
		cfg := s.mail.Current()
		v.MailReady, v.MailMode = cfg.Delivers(), cfg.Mode()
	}
	return v
}

func (s *server) groupNotifications(w http.ResponseWriter, r *http.Request) {
	g, ok := s.groupNotifyAccess(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.groupNotifyView(g))
}

func (s *server) saveGroupNotifications(w http.ResponseWriter, r *http.Request) {
	g, ok := s.groupNotifyAccess(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled     *bool    `json:"enabled"`
		Events      []string `json:"events"`
		Recipients  string   `json:"recipients"`
		ExtraEmails []string `json:"extraEmails"`
	}
	if !decode(w, r, &in) {
		return
	}
	n, err := s.st.TeamNotifyFor(g.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if in.Enabled != nil {
		n.Enabled = *in.Enabled
	}
	if in.Events != nil {
		n.Events = in.Events
	}
	if in.Recipients != "" {
		n.Recipients = in.Recipients
	}
	if in.ExtraEmails != nil {
		n.ExtraEmails = in.ExtraEmails
	}
	if err := s.st.SaveTeamNotify(n, s.uid(r)); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, strings.ToUpper(err.Error()[:1])+err.Error()[1:])
		return
	}
	s.eventRequest(r, "group-notify", "", map[string]any{"group": g.ID, "enabled": n.Enabled, "recipients": n.Recipients, "events": n.Events})
	writeJSON(w, http.StatusOK, s.groupNotifyView(g))
}

// notifyTeam queues one team notice when that group has it turned on and mail
// is delivered. It never blocks the change that caused it: a mail failure is a
// log line, not a failed request.
func (s *server) notifyTeam(r *http.Request, groupID int64, event, dedupe string, d mail.Data) {
	if s.mail == nil || !s.mail.Delivers() {
		return
	}
	n, err := s.st.TeamNotifyFor(groupID)
	if err != nil || !n.Enabled {
		return
	}
	on := false
	for _, e := range n.Events {
		if e == event {
			on = true
			break
		}
	}
	if !on {
		return
	}
	recipients, err := s.st.TeamNotifyRecipients(groupID, n.Recipients, n.ExtraEmails)
	if err != nil {
		log.Printf("email: reading a team's recipients: %v", err)
		return
	}
	if name, err := s.st.GroupName(groupID); err == nil {
		d.Team = name
	}
	d.Audience = n.Recipients
	if d.Link == "" {
		d.Link = s.mailBase(r) + "/spaces"
	}
	if d.When.IsZero() {
		d.When = time.Now()
	}
	if err := s.mail.TeamNotice(r.Context(), groupID, event, dedupe, recipients, d); err != nil {
		log.Printf("email: queueing a team notice: %v", err)
	}
}

func (s *server) mailBase(r *http.Request) string {
	if s.mail != nil {
		if base := s.mail.Base(); base != "" {
			return base
		}
	}
	return requestOrigin(r)
}

func (s *server) notifyListShared(r *http.Request, groupID, listID int64, listName string, updatedAt int64) {
	if groupID == 0 {
		return
	}
	d := mail.Data{List: listName, Actor: s.currentUser(r).Name, Link: s.mailBase(r) + fmt.Sprintf("/lists/%d", listID)}
	s.notifyTeam(r, groupID, "list_shared", fmt.Sprintf("list:%d:%d", listID, updatedAt), d)
}
