package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// restoreFixture is a controller with a known amount of everything a restore
// reports, closed so the file can be read the way a copy is.
func restoreFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateUser("owner@example.com", "Owner", "owner", "x")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := st.CreateUser("admin@example.com", "Admin", "admin", "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("m1@example.com", "M1", "member", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("m2@example.com", "M2", "member", "x"); err != nil {
		t.Fatal(err)
	}
	// An owner who cannot sign in does not count as a way in.
	gone, err := st.CreateUser("gone@example.com", "Gone", "owner", "x")
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := st.db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`UPDATE users SET disabled_at=1 WHERE id=?`, gone.ID)
	for _, id := range []string{"s1", "s2", "s3"} {
		exec(`INSERT INTO skills(id,created_at,updated_at) VALUES(?,1,1)`, id)
	}
	exec(`INSERT INTO shelf(user_id,skill_id,created_at,updated_at) VALUES(?,?,1,1),(?,?,1,1),(?,?,1,1)`,
		owner.ID, "s1", owner.ID, "s2", admin.ID, "s3")
	for _, n := range []string{"one", "two"} {
		if _, err := st.CreateList(owner.ID, n, "", "members"); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"a", "b", "c"} {
		if _, err := st.CreateTag(n); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO environments(id,name,first_seen,last_seen) VALUES('e1','one',1,1),('e2','two',1,1)`)
	exec(`INSERT INTO sync_agents(id,user_id,host,secret_hash,version,last_seen) VALUES('a1',?, 'laptop','h','v',1)`, owner.ID)
	for _, n := range []string{"offsite", "second"} {
		if _, err := st.CreateBackupRemote(BackupRemote{Name: n, SecretSealed: "sealed-" + n, PathStyle: -1}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO backup_settings(owner_id,password_sealed,updated_at) VALUES(0,'sealed-pw',1)`)
	future := time.Now().Add(time.Hour).UnixMilli()
	for _, id := range []string{"sess-1", "sess-2"} {
		if err := st.CreateSession(id, owner.ID, future, "t"); err != nil {
			t.Fatal(err)
		}
	}
	for _, h := range []string{"t1", "t2"} {
		if _, err := st.CreateAPIToken(owner.ID, "tok", h); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.CreateMCPConnection(owner.ID, "agent", "read", "m1", future); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "reet.db")
}

func rows(t *testing.T, path, q string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestTheInventoryCountsMatchAKnownCopy(t *testing.T) {
	sum, err := InspectDatabaseFile(context.Background(), restoreFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Users != 5 || sum.UsersByRole["owner"] != 2 || sum.UsersByRole["admin"] != 1 || sum.UsersByRole["member"] != 2 {
		t.Fatalf("accounts: %+v", sum)
	}
	if sum.Shelves != 2 || sum.Skills != 3 || sum.Lists != 2 || sum.Tags != 3 || sum.Environments != 2 || sum.Machines != 1 || sum.Destinations != 2 {
		t.Fatalf("inventory: %+v", sum)
	}
	if len(sum.Owners) != 1 || sum.Owners[0] != "owner@example.com" {
		t.Fatalf("a disabled owner is not a way in: %v", sum.Owners)
	}
	if sum.Sessions != 2 || sum.APITokens != 2 || sum.MCPConnections != 1 {
		t.Fatalf("credentials: %+v", sum)
	}
}

func TestAdaptingByDefaultLeavesSessionsAndTokensAlone(t *testing.T) {
	path := restoreFixture(t)
	ad, err := AdaptDatabaseFile(context.Background(), path, AdaptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ad.SessionsEnded != 0 || ad.TokensRevoked != 0 || ad.MCPConnectionsRevoked != 0 {
		t.Fatalf("adapt: %+v", ad)
	}
	if rows(t, path, `SELECT COUNT(*) FROM sessions`) != 2 ||
		rows(t, path, `SELECT COUNT(*) FROM api_tokens WHERE revoked_at=0`) != 2 ||
		rows(t, path, `SELECT COUNT(*) FROM mcp_connections WHERE revoked_at=0`) != 1 {
		t.Fatal("a default restore changed sessions or tokens")
	}
}

func TestDroppingSessionsEmptiesOnlyTheSessions(t *testing.T) {
	path := restoreFixture(t)
	if _, err := AdaptDatabaseFile(context.Background(), path, AdaptOptions{DropSessions: true}); err != nil {
		t.Fatal(err)
	}
	if rows(t, path, `SELECT COUNT(*) FROM sessions`) != 0 {
		t.Fatal("sessions survived --drop-sessions")
	}
	if rows(t, path, `SELECT COUNT(*) FROM api_tokens WHERE revoked_at=0`) != 2 || rows(t, path, `SELECT COUNT(*) FROM users`) != 5 {
		t.Fatal("--drop-sessions changed more than sessions")
	}
}

func TestDroppingTokensRevokesOnlyTheTokens(t *testing.T) {
	path := restoreFixture(t)
	ad, err := AdaptDatabaseFile(context.Background(), path, AdaptOptions{DropTokens: true})
	if err != nil {
		t.Fatal(err)
	}
	if ad.TokensRevoked != 2 || ad.MCPConnectionsRevoked != 1 {
		t.Fatalf("adapt: %+v", ad)
	}
	if rows(t, path, `SELECT COUNT(*) FROM api_tokens WHERE revoked_at=0`) != 0 || rows(t, path, `SELECT COUNT(*) FROM mcp_connections WHERE revoked_at=0`) != 0 {
		t.Fatal("tokens survived --drop-tokens")
	}
	if rows(t, path, `SELECT COUNT(*) FROM sessions`) != 2 || rows(t, path, `SELECT COUNT(*) FROM users`) != 5 {
		t.Fatal("--drop-tokens changed more than tokens")
	}
}

func TestUnreadableSecretsAreClearedAndTheirDestinationsSwitchedOff(t *testing.T) {
	path := restoreFixture(t)
	ad, err := AdaptDatabaseFile(context.Background(), path, AdaptOptions{
		Unreadable: func(sealed string) bool { return sealed != "sealed-second" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ad.Cleared) != 2 || !strings.Contains(ad.Cleared[0], `"offsite"`) || !strings.Contains(ad.Cleared[1], "copy password") {
		t.Fatalf("cleared: %v", ad.Cleared)
	}
	if rows(t, path, `SELECT COUNT(*) FROM backup_remotes WHERE name='offsite' AND secret_sealed='' AND disabled=1 AND last_error<>''`) != 1 {
		t.Fatal("the unreadable destination was not cleared and switched off")
	}
	if rows(t, path, `SELECT COUNT(*) FROM backup_remotes WHERE name='second' AND secret_sealed='sealed-second' AND disabled=0`) != 1 {
		t.Fatal("a readable destination was touched")
	}
	if rows(t, path, `SELECT COUNT(*) FROM backup_settings WHERE password_sealed<>''`) != 0 {
		t.Fatal("the unreadable copy password was kept")
	}
}

func TestTheChoicesAreWrittenToTheActivityLogWithoutASecret(t *testing.T) {
	path := restoreFixture(t)
	if _, err := AdaptDatabaseFile(context.Background(), path, AdaptOptions{
		DropSessions: true, EventSource: "cli", EventAction: "backup-restored",
		EventDetail: map[string]any{"id": "copy-1"},
	}); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var detail string
	if err := db.QueryRow(`SELECT detail FROM events WHERE action='backup-restored'`).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id":"copy-1"`, `"sessionsDropped":true`, `"tokensDropped":false`} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail %s lacks %s", detail, want)
		}
	}
}
