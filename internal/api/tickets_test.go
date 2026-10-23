package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

func ticketSignIn(t *testing.T, srv *httptest.Server, email, password string) *http.Client {
	t.Helper()
	c := &http.Client{Jar: mustJar(t)}
	code, body := doClient(t, c, srv, "POST", "/api/v2/auth/sign-in", `{"email":"`+email+`","password":"`+password+`"}`)
	if code != 200 {
		t.Fatalf("sign-in %s: %d %s", email, code, body)
	}
	return c
}

func TestPersonalTicketLifecycleAndNotifications(t *testing.T) {
	m := newMailSrv(t)
	owner := setupMailOwner(t, m.srv)
	hash, _ := auth.HashPassword("long-enough-password")
	u, err := m.st.CreateUser("sam@example.com", "Sam", "member", hash)
	if err != nil {
		t.Fatal(err)
	}
	sam := ticketSignIn(t, m.srv, "sam@example.com", "long-enough-password")

	code, body := doClient(t, sam, m.srv, "POST", "/api/v2/tickets", `{"subject":"Printer","body":"It will not print"}`)
	var opened struct {
		Ticket store.Ticket `json:"ticket"`
	}
	if code != 201 || json.Unmarshal([]byte(body), &opened) != nil || opened.Ticket.ID == 0 {
		t.Fatalf("open: %d %s", code, body)
	}
	sendQueue(t, m.q)
	found := false
	for _, msg := range m.fake.Messages() {
		if msg.To == "owner@example.com" && strings.Contains(msg.Subject, "Printer") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no admin notification: %+v", m.fake.Messages())
	}

	if code, body := doClient(t, owner, m.srv, "GET", "/api/v2/admin/tickets", ""); code != 200 || !strings.Contains(body, `"status":"open"`) {
		t.Fatalf("queue: %d %s", code, body)
	}
	code, body = doClient(t, owner, m.srv, "POST", "/api/v2/admin/tickets/"+strconv.FormatInt(opened.Ticket.ID, 10)+"/reply", `{"body":"Try the other tray","status":"waiting"}`)
	if code != 200 || !strings.Contains(body, `"status":"waiting"`) {
		t.Fatalf("admin reply: %d %s", code, body)
	}
	sendQueue(t, m.q)
	found = false
	for _, msg := range m.fake.Messages() {
		if msg.To == "sam@example.com" && strings.Contains(msg.Subject, "Reply on support ticket") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no requester notification: %+v", m.fake.Messages())
	}

	code, body = doClient(t, sam, m.srv, "POST", "/api/v2/tickets/"+strconv.FormatInt(opened.Ticket.ID, 10)+"/reply", `{"body":"It is still stuck"}`)
	if code != 200 || !strings.Contains(body, `"status":"open"`) {
		t.Fatalf("requester reply: %d %s", code, body)
	}
	code, body = doClient(t, owner, m.srv, "POST", "/api/v2/admin/tickets/"+strconv.FormatInt(opened.Ticket.ID, 10)+"/note", `{"body":"Known firmware bug"}`)
	if code != 200 {
		t.Fatalf("note: %d %s", code, body)
	}
	code, body = doClient(t, sam, m.srv, "GET", "/api/v2/tickets/"+strconv.FormatInt(opened.Ticket.ID, 10), "")
	if code != 200 || strings.Contains(body, "Known firmware bug") {
		t.Fatalf("internal note leaked to requester: %d %s", code, body)
	}
	code, body = doClient(t, owner, m.srv, "GET", "/api/v2/admin/tickets/"+strconv.FormatInt(opened.Ticket.ID, 10), "")
	if code != 200 || !strings.Contains(body, "Known firmware bug") {
		t.Fatalf("internal note missing for admin: %d %s", code, body)
	}

	ownerUser, _, _ := m.st.OwnerUser()
	code, body = doClient(t, owner, m.srv, "POST", "/api/v2/admin/tickets/"+strconv.FormatInt(opened.Ticket.ID, 10)+"/assign", `{"assigneeId":`+strconv.FormatInt(ownerUser.ID, 10)+`}`)
	if code != 200 || !strings.Contains(body, `"assignee":"Owner"`) {
		t.Fatalf("assign: %d %s", code, body)
	}
	if code, _ := doClient(t, owner, m.srv, "POST", "/api/v2/admin/tickets/"+strconv.FormatInt(opened.Ticket.ID, 10)+"/status", `{"status":"closed"}`); code != 200 {
		t.Fatalf("close: %d", code)
	}
	code, body = doClient(t, sam, m.srv, "GET", "/api/v2/tickets", "")
	if code != 200 || !strings.Contains(body, `"status":"closed"`) {
		t.Fatalf("closed ticket: %d %s", code, body)
	}
	if u.ID == 0 {
		t.Fatal("user missing")
	}
}

func TestTeamTicketIsVisibleToManagersNotOtherMembers(t *testing.T) {
	m := newMailSrv(t)
	owner := setupMailOwner(t, m.srv)
	hash, _ := auth.HashPassword("long-enough-password")
	manager, _ := m.st.CreateUser("manager@example.com", "Manager", "member", hash)
	member, _ := m.st.CreateUser("member@example.com", "Member", "member", hash)
	other, _ := m.st.CreateUser("other@example.com", "Other", "member", hash)

	code, body := doClient(t, owner, m.srv, "POST", "/api/v2/groups", `{"name":"Platform","kind":"team"}`)
	var groups struct {
		Groups []store.Group `json:"groups"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &groups) != nil || len(groups.Groups) != 1 {
		t.Fatalf("create group: %d %s", code, body)
	}
	groupID := groups.Groups[0].ID
	gid := strconv.FormatInt(groupID, 10)
	if code, body := doClient(t, owner, m.srv, "POST", "/api/v2/groups/"+gid+"/members", `{"email":"manager@example.com","role":"manager"}`); code != 200 {
		t.Fatalf("add manager: %d %s", code, body)
	}
	if code, body := doClient(t, owner, m.srv, "POST", "/api/v2/groups/"+gid+"/members", `{"email":"member@example.com","role":"member"}`); code != 200 {
		t.Fatalf("add member: %d %s", code, body)
	}

	memberClient := ticketSignIn(t, m.srv, "member@example.com", "long-enough-password")
	managerClient := ticketSignIn(t, m.srv, "manager@example.com", "long-enough-password")
	otherClient := ticketSignIn(t, m.srv, "other@example.com", "long-enough-password")

	code, body = doClient(t, memberClient, m.srv, "POST", "/api/v2/tickets", `{"subject":"Shared laptop","body":"The team laptop will not boot","groupId":`+gid+`}`)
	var opened struct {
		Ticket store.Ticket `json:"ticket"`
	}
	if code != 201 || json.Unmarshal([]byte(body), &opened) != nil {
		t.Fatalf("open team ticket: %d %s", code, body)
	}
	if !strings.Contains(body, `"groupName":"Platform"`) {
		t.Fatalf("team not recorded: %s", body)
	}
	sendQueue(t, m.q)
	to := map[string]bool{}
	for _, msg := range m.fake.Messages() {
		to[msg.To] = true
	}
	if !to["owner@example.com"] || !to["manager@example.com"] || to["member@example.com"] || to["other@example.com"] {
		t.Fatalf("wrong new-ticket recipients: %+v", to)
	}

	id := strconv.FormatInt(opened.Ticket.ID, 10)
	if code, _ := doClient(t, managerClient, m.srv, "GET", "/api/v2/tickets/"+id, ""); code != 200 {
		t.Fatalf("manager cannot read team ticket: %d", code)
	}
	if code, _ := doClient(t, otherClient, m.srv, "GET", "/api/v2/tickets/"+id, ""); code != 404 {
		t.Fatalf("unrelated member read team ticket: %d", code)
	}
	if code, _ := doClient(t, otherClient, m.srv, "GET", "/api/v2/admin/tickets", ""); code != 403 {
		t.Fatalf("non-admin read admin queue: %d", code)
	}

	code, body = doClient(t, managerClient, m.srv, "POST", "/api/v2/tickets/"+id+"/reply", `{"body":"Try charging it"}`)
	if code != 200 || !strings.Contains(body, `"status":"waiting"`) {
		t.Fatalf("manager reply: %d %s", code, body)
	}
	sendQueue(t, m.q)
	last := m.fake.Messages()[len(m.fake.Messages())-1]
	if last.To != "member@example.com" || !strings.Contains(last.Subject, "Reply on support ticket") {
		t.Fatalf("opener was not told about the team reply: %+v", last)
	}
	if member.ID == 0 || manager.ID == 0 || other.ID == 0 {
		t.Fatal("users missing")
	}
}

func TestTicketValidationAndGroupAccess(t *testing.T) {
	m := newMailSrv(t)
	setupMailOwner(t, m.srv)
	hash, _ := auth.HashPassword("long-enough-password")
	if _, err := m.st.CreateUser("sam@example.com", "Sam", "member", hash); err != nil {
		t.Fatal(err)
	}
	sam := ticketSignIn(t, m.srv, "sam@example.com", "long-enough-password")
	if code, _ := doClient(t, sam, m.srv, "POST", "/api/v2/tickets", `{"subject":"","body":"x"}`); code != 422 {
		t.Fatalf("empty subject: %d", code)
	}
	if code, _ := doClient(t, sam, m.srv, "POST", "/api/v2/tickets", `{"subject":"Help","body":"x","groupId":999}`); code != 403 {
		t.Fatalf("unknown group: %d", code)
	}
}
