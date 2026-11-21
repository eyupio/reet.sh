package store

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
)

func auditUser(t *testing.T, s *Store, email, role string) User {
	t.Helper()
	u, err := s.CreateUser(email, "", role, "hash")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// A restore adds a missing catalogue row and never rewrites one another
// account also sees.
func TestRestoreLeavesTheSharedSkillRowAlone(t *testing.T) {
	s := newStore(t)
	a, b := auditUser(t, s, "a@example.com", "member"), auditUser(t, s, "b@example.com", "member")
	rf, err := ref.Parse("vercel-labs/skills/find-skills")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFor(b.ID, rf, Patch{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE skills SET name='Real',description='Real text',installs=7 WHERE id=?`, rf.ID); err != nil {
		t.Fatal(err)
	}
	forged := Skill{ID: rf.ID, Name: "Forged", Description: "Forged text", Installs: 999999}
	if err := s.RestoreSnapshotFor(a.ID, Snapshot{Version: 2, Skills: []Skill{forged}}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetFor(b.ID, rf.ID)
	if err != nil || !ok || got.Name != "Real" || got.Description != "Real text" || got.Installs != 7 {
		t.Fatalf("the other account's view changed: %+v %v %v", got, ok, err)
	}
	if _, ok, _ := s.GetFor(a.ID, rf.ID); !ok {
		t.Fatal("the restore did not put the skill on the restoring account's shelf")
	}
	// A skill nobody has yet is still created from the snapshot.
	fresh := Skill{ID: "acme/tools/fresh", Name: "Fresh"}
	if err := s.RestoreSnapshotFor(a.ID, Snapshot{Version: 2, Skills: []Skill{fresh}}); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := s.GetFor(a.ID, "acme/tools/fresh"); !ok || got.Name != "Fresh" {
		t.Fatalf("a new skill was not restored: %+v %v", got, ok)
	}
}

func TestPasswordResetRevokesTokensAndMCPConnections(t *testing.T) {
	s := newStore(t)
	u := auditUser(t, s, "a@example.com", "member")
	if _, err := s.CreateAPIToken(u.ID, "cli", "token-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO mcp_access(user_id,enabled) VALUES(?,1)`, u.ID); err != nil {
		t.Fatal(err)
	}
	far := time.Now().Add(time.Hour).UnixMilli()
	if _, err := s.CreateMCPConnection(u.ID, "desk", "read", "mcp-hash", far); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UnixMilli()
	if _, ok, _ := s.APITokenUser("token-hash", at); !ok {
		t.Fatal("fixture: the token does not work")
	}
	if _, _, ok, _ := s.MCPUser("mcp-hash", at); !ok {
		t.Fatal("fixture: the connection does not work")
	}
	if err := s.CreatePasswordReset("reset-hash", u.ID, far); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.ResetPasswordWithToken("reset-hash", "new-hash", at); err != nil || !ok {
		t.Fatalf("reset: %v %v", ok, err)
	}
	if _, ok, _ := s.APITokenUser("token-hash", at); ok {
		t.Fatal("an API token outlived the password reset")
	}
	if _, _, ok, _ := s.MCPUser("mcp-hash", at); ok {
		t.Fatal("an MCP connection outlived the password reset")
	}
}

func mcpClientCount(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM mcp_oauth_clients`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMCPClientsArePrunedAndCapped(t *testing.T) {
	s := newStore(t)
	u := auditUser(t, s, "a@example.com", "member")
	conn, err := s.CreateMCPConnection(u.ID, "desk", "read", "mcp-hash", time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-mcpClientUnusedAge - time.Hour).UnixMilli()
	for _, id := range []string{"old-unused", "old-with-code", "old-with-refresh"} {
		if _, err := s.db.Exec(`INSERT INTO mcp_oauth_clients(id,name,redirect_uris,created_at) VALUES(?,'c','[]',?)`, id, old); err != nil {
			t.Fatal(err)
		}
	}
	far := time.Now().Add(time.Hour).UnixMilli()
	if _, err := s.db.Exec(`INSERT INTO mcp_oauth_codes(hash,client_id,connection_id,redirect_uri,challenge,resource,scope,expires_at) VALUES('c','old-with-code',?,'','','','read',?)`, conn.ID, far); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO mcp_oauth_refresh(hash,connection_id,client_id,resource,scope,expires_at) VALUES('r',?,'old-with-refresh','','read',?)`, conn.ID, far); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMCPClient(MCPClient{ID: "new", Name: "n", RedirectURIs: []string{"https://x.example/cb"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MCPClient("old-unused"); err == nil {
		t.Fatal("an old client nobody connected was kept")
	}
	for _, id := range []string{"old-with-code", "old-with-refresh", "new"} {
		if _, err := s.MCPClient(id); err != nil {
			t.Fatalf("client %s was dropped: %v", id, err)
		}
	}

	// Fill the table with fresh clients: the next registration is refused.
	have := mcpClientCount(t, s)
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	recent := time.Now().UnixMilli()
	for i := have; i < mcpClientLimit; i++ {
		if _, err := tx.Exec(`INSERT INTO mcp_oauth_clients(id,name,redirect_uris,created_at) VALUES(?,'c','[]',?)`, fmt.Sprintf("fill-%d", i), recent); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMCPClient(MCPClient{ID: "over", Name: "n"}); !errors.Is(err, ErrMCPClientLimit) {
		t.Fatalf("registration past the limit: %v", err)
	}
	// Once the unused ones are over an hour old they make room again.
	if _, err := s.db.Exec(`UPDATE mcp_oauth_clients SET created_at=? WHERE id LIKE 'fill-%'`, time.Now().Add(-2*mcpClientFullAge).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMCPClient(MCPClient{ID: "after", Name: "n"}); err != nil {
		t.Fatalf("registration after the unused clients aged: %v", err)
	}
	if n := mcpClientCount(t, s); n != 4 {
		t.Fatalf("clients left: %d", n)
	}
}

func inviteCount(t *testing.T, s *Store, email string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM invites WHERE email=?`, email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestANewInviteReplacesEarlierOnesForTheAddress(t *testing.T) {
	s := newStore(t)
	owner, _ := s.DefaultOwner()
	far := time.Now().Add(time.Hour).UnixMilli()
	if err := s.CreateInvite("first", "New@Example.com", "admin", owner, far); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateInvite("other", "other@example.com", "member", owner, far); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateInvite("second", "new@example.com", "viewer", owner, far); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UnixMilli()
	if _, ok, _ := s.InviteByHash("first", at); ok {
		t.Fatal("the earlier invite, with its admin role, still works")
	}
	if got, ok, _ := s.InviteByHash("second", at); !ok || got.Role != "viewer" {
		t.Fatalf("the new invite: %+v %v", got, ok)
	}
	if _, ok, _ := s.InviteByHash("other", at); !ok {
		t.Fatal("an invite for another address was removed")
	}
	if err := s.DeleteInvitesFor(" NEW@example.com "); err != nil || inviteCount(t, s, "new@example.com") != 0 {
		t.Fatalf("delete by address: %v", err)
	}
}

func TestDeleteUserRemovesInvitesAddressedToThem(t *testing.T) {
	s := newStore(t)
	owner, _ := s.DefaultOwner()
	u := auditUser(t, s, "gone@example.com", "member")
	if _, err := s.db.Exec(`INSERT INTO invites(token_hash,email,role,created_by,created_at,expires_at) VALUES('stale','gone@example.com','admin',?,1,?)`, owner, time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	if inviteCount(t, s, "gone@example.com") != 0 {
		t.Fatal("an invite for the deleted account's address survived")
	}
}

func TestTransferOwnerChecksTheActorIsStillTheOwner(t *testing.T) {
	s := newStore(t)
	owner, _ := s.DefaultOwner()
	admin, member := auditUser(t, s, "admin@example.com", "admin"), auditUser(t, s, "member@example.com", "member")
	if err := s.TransferOwner(admin.ID, member.ID); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("a transfer by a non-owner: %v", err)
	}
	for id, want := range map[int64]string{owner: "owner", admin.ID: "admin", member.ID: "member"} {
		if u, _, _ := s.UserByID(id); u.Role != want {
			t.Fatalf("user %d is %s, want %s", id, u.Role, want)
		}
	}
	if err := s.TransferOwner(owner, admin.ID); err != nil {
		t.Fatal(err)
	}
	// The old owner's second, stale request must not take ownership back.
	if err := s.TransferOwner(owner, member.ID); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("a stale transfer: %v", err)
	}
	if u, _, _ := s.UserByID(admin.ID); u.Role != "owner" {
		t.Fatalf("the new owner is %s", u.Role)
	}
}

// A session, token or machine writes its last-seen time on first use and then
// not again inside the stamp interval; expiry and revocation still apply at once.
func TestLastSeenIsStampedAtMostOncePerInterval(t *testing.T) {
	s := newStore(t)
	u := auditUser(t, s, "a@example.com", "member")
	far := time.Now().Add(time.Hour).UnixMilli()
	if err := s.CreateSession("sess", u.ID, far, "ua"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAPIToken(u.ID, "cli", "tok"); err != nil {
		t.Fatal(err)
	}
	seen := func(query string) (at int64) {
		t.Helper()
		if err := s.db.QueryRow(query).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return
	}
	for _, at := range []int64{1000, 2000} {
		if _, ok, err := s.SessionUser("sess", at); !ok || err != nil {
			t.Fatalf("session: %v %v", ok, err)
		}
		if _, ok, err := s.APITokenUser("tok", at); !ok || err != nil {
			t.Fatalf("token: %v %v", ok, err)
		}
	}
	if got := seen(`SELECT last_seen FROM sessions WHERE id='sess'`); got != 1000 {
		t.Fatalf("session last_seen %d, want the first stamp only", got)
	}
	if got := seen(`SELECT last_used_at FROM api_tokens WHERE hash='tok'`); got != 1000 {
		t.Fatalf("token last_used_at %d, want the first stamp only", got)
	}
	// Once the interval has passed the next request stamps again.
	s.stamps.mu.Lock()
	for k := range s.stamps.at {
		s.stamps.at[k] = time.Now().Add(-2 * stampEvery)
	}
	s.stamps.mu.Unlock()
	if _, ok, _ := s.SessionUser("sess", 3000); !ok {
		t.Fatal("session")
	}
	if got := seen(`SELECT last_seen FROM sessions WHERE id='sess'`); got != 3000 {
		t.Fatalf("session last_seen %d after the interval", got)
	}
	// Expiry, sign-out and revocation are read every time.
	if _, ok, _ := s.SessionUser("sess", far+1); ok {
		t.Fatal("an expired session was accepted")
	}
	if err := s.DeleteSession("sess"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.SessionUser("sess", 4000); ok {
		t.Fatal("a deleted session was accepted")
	}
	if err := s.RevokeUserTokens(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.APITokenUser("tok", 4000); ok {
		t.Fatal("a revoked token was accepted")
	}

	a, secret, err := s.EnrollSyncAgent(u.ID, "laptop", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateSyncAgentBuild(u.ID, a.ID, secret, "v1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE sync_agents SET last_seen=5 WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateSyncAgentBuild(u.ID, a.ID, secret, "v1", ""); err != nil {
		t.Fatal(err)
	}
	if got := seen(`SELECT last_seen FROM sync_agents`); got != 5 {
		t.Fatalf("machine last_seen %d: a second poll inside the interval wrote", got)
	}
	// A new build is recorded at once.
	if _, err := s.AuthenticateSyncAgentBuild(u.ID, a.ID, secret, "v2", "abc"); err != nil {
		t.Fatal(err)
	}
	agents, err := s.SyncAgentsFor(u.ID)
	if err != nil || len(agents) != 1 || agents[0].Version != "v2" || agents[0].Commit != "abc" || agents[0].LastSeen == 5 {
		t.Fatalf("after a new build: %+v %v", agents, err)
	}
	if _, err := s.AuthenticateSyncAgentBuild(u.ID, a.ID, "wrong", "v2", ""); err == nil {
		t.Fatal("a wrong secret was accepted")
	}
}

func TestStampTimesStayBounded(t *testing.T) {
	var st stampTimes
	for i := 0; i < 3*stampKeep; i++ {
		st.due(fmt.Sprint(i), stampEvery)
	}
	if len(st.at) > stampKeep {
		t.Fatalf("%d stamp entries kept", len(st.at))
	}
}
