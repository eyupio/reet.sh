package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/store"
)

type mailTestSealer struct{}

func (mailTestSealer) SealSecret(v string) (string, error) { return "sealed:" + v, nil }
func (mailTestSealer) OpenSecret(v string) (string, error) {
	if !strings.HasPrefix(v, "sealed:") {
		return "", errors.New("bad seal")
	}
	return strings.TrimPrefix(v, "sealed:"), nil
}

type mailSrv struct {
	srv  *httptest.Server
	st   *store.Store
	fake *mail.Fake
	q    *mail.Queue
}

func newMailSrv(t *testing.T) mailSrv {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := &mail.Settings{Store: st, Sealer: mailTestSealer{}, Env: mail.Config{
		BaseURL: "https://reet.example", OutboxDir: t.TempDir(),
	}}
	if err := st.SaveMailSettings(store.MailSettings{
		TLS: mail.TLSStartTLS, Port: 587, From: "hello@example.com", FromName: "reet.sh", BaseURL: "https://reet.example",
	}, "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	fake := &mail.Fake{}
	q := &mail.Queue{Store: st, Settings: settings, Sender: fake}
	m := &mail.Mailer{Store: st, Settings: settings, Queue: q}
	srv := httptest.NewServer(New(st, deadFetcher(t), "bootstrap", WithAuthMode(AuthAccounts), WithMail(m)))
	t.Cleanup(func() { srv.Close(); st.Close() })
	return mailSrv{srv: srv, st: st, fake: fake, q: q}
}

func setupMailOwner(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	owner := &http.Client{Jar: mustJar(t)}
	code, body := doClient(t, owner, srv, "POST", "/api/v2/auth/setup",
		`{"name":"Owner","email":"owner@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap")
	if code != 200 {
		t.Fatalf("setup: %d %s", code, body)
	}
	return owner
}

func sendQueue(t *testing.T, q *mail.Queue) {
	t.Helper()
	if _, err := q.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func resetTokenFrom(t *testing.T, text string) string {
	t.Helper()
	_, rest, ok := strings.Cut(text, "/reset?token=")
	if !ok {
		t.Fatalf("no reset link in %q", text)
	}
	token := rest
	if i := strings.IndexAny(token, " \n\r<\"'"); i >= 0 {
		token = token[:i]
	}
	if token == "" {
		t.Fatal("empty reset token")
	}
	return token
}

func TestMailSettingsOutboxAndInvite(t *testing.T) {
	m := newMailSrv(t)
	owner := setupMailOwner(t, m.srv)

	code, body := doClient(t, owner, m.srv, "POST", "/api/v2/admin/invites", `{"email":"sam@example.com","role":"member"}`)
	if code != 200 || !strings.Contains(body, `"emailed":true`) {
		t.Fatalf("invite: %d %s", code, body)
	}
	if len(m.fake.Messages()) != 0 {
		t.Fatal("mail left before the queue ran")
	}
	sendQueue(t, m.q)
	msgs := m.fake.Messages()
	if len(msgs) != 1 || msgs[0].To != "sam@example.com" || !strings.Contains(msgs[0].Text, "/join?token=") {
		t.Fatalf("invitation message: %+v", msgs)
	}

	code, body = doClient(t, owner, m.srv, "GET", "/api/v2/admin/mail/outbox", "")
	var box struct {
		Mode     string           `json:"mode"`
		Messages []map[string]any `json:"messages"`
		Counts   map[string]int   `json:"counts"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &box) != nil || box.Mode != "outbox" || len(box.Messages) != 1 || box.Messages[0]["state"] != "delivered" {
		t.Fatalf("outbox: %d %s", code, body)
	}
	if _, ok := box.Messages[0]["id"]; !ok {
		t.Fatalf("outbox row has no id: %s", body)
	}

	code, body = doClient(t, owner, m.srv, "GET", "/api/v2/admin/mail", "")
	var settings struct {
		Settings mailSettingsView `json:"settings"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &settings) != nil || settings.Settings.Mode != "outbox" || settings.Settings.From != "hello@example.com" {
		t.Fatalf("settings: %d %s", code, body)
	}

	code, body = doClient(t, owner, m.srv, "PATCH", "/api/v2/admin/mail", `{
		"host":"smtp.example.com","port":587,"tls":"starttls","user":"me","password":"secret","passwordAction":"replace",
		"from":"hello@example.com","fromName":"reet.sh","replyTo":"","baseUrl":"https://reet.example","dkimSelector":"default"}`)
	if code != 200 || !strings.Contains(body, `"passwordSet":true`) || !strings.Contains(body, `"mode":"smtp"`) {
		t.Fatalf("save settings: %d %s", code, body)
	}
	if strings.Contains(body, "secret") {
		t.Fatal("the password came back")
	}
	code, body = doClient(t, owner, m.srv, "PATCH", "/api/v2/admin/mail", `{
		"host":"other.example.com","tls":"starttls","user":"me","passwordAction":"keep",
		"from":"hello@example.com","baseUrl":"https://reet.example"}`)
	if code != 422 || !strings.Contains(body, "saved password is for") {
		t.Fatalf("password followed a host change: %d %s", code, body)
	}
}

func TestForgotAndResetPassword(t *testing.T) {
	m := newMailSrv(t)
	setupMailOwner(t, m.srv)
	hash, _ := auth.HashPassword("old-enough-password")
	if _, err := m.st.CreateUser("sam@example.com", "Sam", "member", hash); err != nil {
		t.Fatal(err)
	}

	code, body := do(t, m.srv, "POST", "/api/v2/auth/forgot", `{"email":"sam@example.com"}`)
	if code != 200 || !strings.Contains(body, "If that address has an account") {
		t.Fatalf("forgot: %d %s", code, body)
	}
	sendQueue(t, m.q)
	msgs := m.fake.Messages()
	if len(msgs) != 1 || msgs[0].To != "sam@example.com" {
		t.Fatalf("reset messages: %+v", msgs)
	}
	token := resetTokenFrom(t, msgs[0].Text)

	code, body = do(t, m.srv, "POST", "/api/v2/auth/reset", `{"token":`+strconv.Quote(token)+`,"password":"new-enough-password"}`)
	if code != 200 {
		t.Fatalf("reset: %d %s", code, body)
	}
	if code, _ := do(t, m.srv, "POST", "/api/v2/auth/sign-in", `{"email":"sam@example.com","password":"old-enough-password"}`); code != 401 {
		t.Fatalf("old password still works: %d", code)
	}
	if code, body := do(t, m.srv, "POST", "/api/v2/auth/sign-in", `{"email":"sam@example.com","password":"new-enough-password"}`); code != 200 {
		t.Fatalf("new password: %d %s", code, body)
	}
	if code, _ := do(t, m.srv, "POST", "/api/v2/auth/reset", `{"token":`+strconv.Quote(token)+`,"password":"another-enough-password"}`); code != 400 {
		t.Fatalf("spent reset token worked again: %d", code)
	}
	// The security notice follows the reset.
	sendQueue(t, m.q)
	found := false
	for _, msg := range m.fake.Messages() {
		if strings.Contains(msg.Subject, "password changed") {
			found = true
		}
	}
	if !found {
		t.Fatal("no password-change notice")
	}
}

func TestTeamNotificationsFollowTheTeamsChoice(t *testing.T) {
	m := newMailSrv(t)
	owner := setupMailOwner(t, m.srv)
	hash, _ := auth.HashPassword("long-enough-password")
	if _, err := m.st.CreateUser("member@example.com", "Member", "member", hash); err != nil {
		t.Fatal(err)
	}

	code, body := doClient(t, owner, m.srv, "POST", "/api/v2/groups", `{"name":"Platform","kind":"team"}`)
	var groups struct {
		Groups []store.Group `json:"groups"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &groups) != nil || len(groups.Groups) != 1 {
		t.Fatalf("create group: %d %s", code, body)
	}
	id := groups.Groups[0].ID

	code, body = doClient(t, owner, m.srv, "GET", "/api/v2/groups/"+strconv.FormatInt(id, 10)+"/notifications", "")
	var view teamNotifyView
	if code != 200 || json.Unmarshal([]byte(body), &view) != nil || !view.Notify.Enabled || !view.MailReady || view.MailMode != "outbox" {
		t.Fatalf("notifications: %d %s", code, body)
	}
	code, body = doClient(t, owner, m.srv, "PATCH", "/api/v2/groups/"+strconv.FormatInt(id, 10)+"/notifications",
		`{"enabled":true,"events":["member_joined","list_shared"],"recipients":"managers","extraEmails":["team@example.com"]}`)
	if code != 200 {
		t.Fatalf("save notifications: %d %s", code, body)
	}

	code, body = doClient(t, owner, m.srv, "POST", "/api/v2/groups/"+strconv.FormatInt(id, 10)+"/members", `{"email":"member@example.com","role":"member"}`)
	if code != 200 {
		t.Fatalf("add member: %d %s", code, body)
	}
	sendQueue(t, m.q)
	msgs := m.fake.Messages()
	if len(msgs) != 2 {
		t.Fatalf("want owner and extra address, got %+v", msgs)
	}
	for _, msg := range msgs {
		if !strings.Contains(msg.Subject, "joined Platform") {
			t.Fatalf("wrong notice: %+v", msg)
		}
	}

	code, body = doClient(t, owner, m.srv, "POST", "/api/v2/lists", `{"name":"Toolkit","visibility":"team","audience":{"groupId":`+strconv.FormatInt(id, 10)+`,"groupRole":"viewer"}}`)
	if code != 200 {
		t.Fatalf("create team list: %d %s", code, body)
	}
	sendQueue(t, m.q)
	found := false
	for _, msg := range m.fake.Messages() {
		if strings.Contains(msg.Subject, "Toolkit is now shared with Platform") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no list-shared notice: %+v", m.fake.Messages())
	}

	member := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, member, m.srv, "POST", "/api/v2/auth/sign-in", `{"email":"member@example.com","password":"long-enough-password"}`); code != 200 {
		t.Fatalf("member sign-in: %d", code)
	}
	if code, _ := doClient(t, member, m.srv, "GET", "/api/v2/groups/"+strconv.FormatInt(id, 10)+"/notifications", ""); code != 403 {
		t.Fatalf("member read notification settings: %d", code)
	}
}

func TestAdminResetSendsALinkWhenMailIsReady(t *testing.T) {
	m := newMailSrv(t)
	owner := setupMailOwner(t, m.srv)
	hash, _ := auth.HashPassword("old-enough-password")
	u, err := m.st.CreateUser("sam@example.com", "Sam", "member", hash)
	if err != nil {
		t.Fatal(err)
	}
	code, body := doClient(t, owner, m.srv, "POST", "/api/v2/admin/users/reset", `{"id":`+strconv.FormatInt(u.ID, 10)+`}`)
	if code != 200 || !strings.Contains(body, `"emailed":true`) {
		t.Fatalf("reset: %d %s", code, body)
	}
	sendQueue(t, m.q)
	msgs := m.fake.Messages()
	if len(msgs) != 1 || msgs[0].To != "sam@example.com" {
		t.Fatalf("reset messages: %+v", msgs)
	}
	if token := resetTokenFrom(t, msgs[0].Text); token == "" {
		t.Fatal("no reset token")
	}
	// Nothing changes until the link is used.
	if code, _ := do(t, m.srv, "POST", "/api/v2/auth/sign-in", `{"email":"sam@example.com","password":"old-enough-password"}`); code != 200 {
		t.Fatalf("old password stopped working before the link was used: %d", code)
	}
}
