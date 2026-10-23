package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/notify"
	"github.com/eyupio/reet.sh/internal/store"
)

type notificationList struct {
	Notifications []store.Notification `json:"notifications"`
	Unread        int                  `json:"unread"`
}

func ownerNotifications(t *testing.T, c *http.Client, srv *httptest.Server) notificationList {
	t.Helper()
	code, body := doClient(t, c, srv, "GET", "/api/v2/notifications", "")
	var out notificationList
	if code != 200 || json.Unmarshal([]byte(body), &out) != nil {
		t.Fatalf("notifications: %d %s", code, body)
	}
	return out
}

func hasNotification(rows []store.Notification, kind, title string) bool {
	for _, n := range rows {
		if n.Kind == kind && strings.Contains(n.Title, title) {
			return true
		}
	}
	return false
}

func TestNotificationCentreTicketsAndPreferences(t *testing.T) {
	m := newMailSrv(t)
	owner := setupMailOwner(t, m.srv)
	hash, _ := auth.HashPassword("long-enough-password")
	if _, err := m.st.CreateUser("sam@example.com", "Sam", "member", hash); err != nil {
		t.Fatal(err)
	}
	sam := ticketSignIn(t, m.srv, "sam@example.com", "long-enough-password")

	code, body := doClient(t, sam, m.srv, "POST", "/api/v2/tickets", `{"subject":"Printer","body":"It will not print"}`)
	var opened struct {
		Ticket store.Ticket `json:"ticket"`
	}
	if code != 201 || json.Unmarshal([]byte(body), &opened) != nil {
		t.Fatalf("open ticket: %d %s", code, body)
	}
	if rows := ownerNotifications(t, owner, m.srv); !hasNotification(rows.Notifications, "staff_ticket_opened", "New support ticket") || rows.Unread == 0 {
		t.Fatalf("owner missing ticket notification: %+v", rows)
	}
	if rows := ownerNotifications(t, sam, m.srv); len(rows.Notifications) != 0 {
		t.Fatalf("opener was told about their own ticket: %+v", rows)
	}

	id := strconv.FormatInt(opened.Ticket.ID, 10)
	if code, body := doClient(t, owner, m.srv, "POST", "/api/v2/admin/tickets/"+id+"/reply", `{"body":"Try the other tray","status":"waiting"}`); code != 200 {
		t.Fatalf("admin reply: %d %s", code, body)
	}
	if rows := ownerNotifications(t, sam, m.srv); !hasNotification(rows.Notifications, "ticket_replied", "Support replied") || rows.Unread != 1 {
		t.Fatalf("requester missing reply notification: %+v", rows)
	}
	if code, body := doClient(t, sam, m.srv, "POST", "/api/v2/notifications/read", `{}`); code != 200 || !strings.Contains(body, `"unread":0`) {
		t.Fatalf("mark all read: %d %s", code, body)
	}

	// Turning the reply kind off stops both the centre row and its email.
	code, body = doClient(t, sam, m.srv, "GET", "/api/v2/notifications/preferences", "")
	var prefs struct {
		Preferences []notify.PreferenceView `json:"preferences"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &prefs) != nil {
		t.Fatalf("preferences: %d %s", code, body)
	}
	found := false
	for _, p := range prefs.Preferences {
		if p.Key == "ticket_replied" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ticket reply kind missing: %s", body)
	}
	sendQueue(t, m.q)
	before := len(m.fake.Messages())
	code, body = doClient(t, sam, m.srv, "PATCH", "/api/v2/notifications/preferences", `{"preferences":[{"key":"ticket_replied","enabled":false,"mode":"none"}]}`)
	if code != 200 {
		t.Fatalf("save preference: %d %s", code, body)
	}
	if code, _ := doClient(t, owner, m.srv, "POST", "/api/v2/admin/tickets/"+id+"/reply", `{"body":"Any luck?","status":"waiting"}`); code != 200 {
		t.Fatalf("second admin reply: %d", code)
	}
	if rows := ownerNotifications(t, sam, m.srv); len(rows.Notifications) != 1 {
		t.Fatalf("disabled preference still made a notification: %+v", rows)
	}
	sendQueue(t, m.q)
	if len(m.fake.Messages()) != before {
		t.Fatalf("disabled preference still emailed: %+v", m.fake.Messages())
	}

	// A locked security notice cannot be turned off.
	if code, _ := doClient(t, sam, m.srv, "PATCH", "/api/v2/notifications/preferences", `{"preferences":[{"key":"security_password_changed","enabled":false,"mode":"none"}]}`); code != 422 {
		t.Fatalf("locked security notice was changed: %d", code)
	}

	// The owner's platform default can stop staff notices entirely.
	code, body = doClient(t, owner, m.srv, "GET", "/api/v2/admin/notifications/defaults", "")
	var defaults struct {
		Defaults []notify.DefaultView `json:"defaults"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &defaults) != nil {
		t.Fatalf("defaults: %d %s", code, body)
	}
	if code, body := doClient(t, owner, m.srv, "PATCH", "/api/v2/admin/notifications/defaults", `{"defaults":[{"kind":"staff_ticket_opened","enabled":false,"mode":"immediate","owner":false,"admin":false}]}`); code != 200 {
		t.Fatalf("save defaults: %d %s", code, body)
	}
	before = len(ownerNotifications(t, owner, m.srv).Notifications)
	if code, body := doClient(t, sam, m.srv, "POST", "/api/v2/tickets", `{"subject":"Second","body":"Another issue"}`); code != 201 {
		t.Fatalf("second ticket: %d %s", code, body)
	}
	if rows := ownerNotifications(t, owner, m.srv); len(rows.Notifications) != before {
		t.Fatalf("staff default did not suppress the notice: %+v", rows)
	}
}

func TestNewAccountNotificationsAndCount(t *testing.T) {
	m := newMailSrv(t)
	owner := setupMailOwner(t, m.srv)
	code, body := doClient(t, owner, m.srv, "POST", "/api/v2/admin/invites", `{"email":"new@example.com","role":"member"}`)
	var invite struct {
		Token string `json:"token"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &invite) != nil || invite.Token == "" {
		t.Fatalf("invite: %d %s", code, body)
	}
	newcomer := &http.Client{Jar: mustJar(t)}
	if code, body := doClient(t, newcomer, m.srv, "POST", "/api/v2/auth/join", `{"token":"`+invite.Token+`","name":"Newcomer","password":"long-enough-password"}`); code != 200 {
		t.Fatalf("join: %d %s", code, body)
	}
	rows := ownerNotifications(t, owner, m.srv)
	if !hasNotification(rows.Notifications, "staff_new_account", "New account") {
		t.Fatalf("owner missing new-account notification: %+v", rows)
	}
	code, body = doClient(t, owner, m.srv, "GET", "/api/v2/notifications/count", "")
	if code != 200 || !strings.Contains(body, `"unread":`) {
		t.Fatalf("count: %d %s", code, body)
	}
	if code, body := doClient(t, owner, m.srv, "POST", "/api/v2/notifications/read", `{}`); code != 200 || !strings.Contains(body, `"unread":0`) {
		t.Fatalf("mark all read: %d %s", code, body)
	}
}

func TestPasswordChangeCreatesALockedNotification(t *testing.T) {
	m := newMailSrv(t)
	setupMailOwner(t, m.srv)
	hash, _ := auth.HashPassword("long-enough-password")
	if _, err := m.st.CreateUser("sam@example.com", "Sam", "member", hash); err != nil {
		t.Fatal(err)
	}
	sam := ticketSignIn(t, m.srv, "sam@example.com", "long-enough-password")
	if code, body := doClient(t, sam, m.srv, "POST", "/api/v2/profile/password", `{"current":"long-enough-password","password":"another-long-password"}`); code != 200 {
		t.Fatalf("change password: %d %s", code, body)
	}
	rows := ownerNotifications(t, sam, m.srv)
	if !hasNotification(rows.Notifications, "security_password_changed", "Your password changed") {
		t.Fatalf("missing security notification: %+v", rows)
	}
}
