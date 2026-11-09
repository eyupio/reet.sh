package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// movingFixture is restoreFixture plus what moving a controller touches: saved
// addresses, MCP grants for the old address, a project run, a software batch
// and queued mail. It returns the path of the copy.
func movingFixture(t *testing.T) string {
	t.Helper()
	path := restoreFixture(t)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	var owner, conn int64
	if err := db.QueryRow(`SELECT id FROM users WHERE email='owner@example.com'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM mcp_connections LIMIT 1`).Scan(&conn); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO mail_settings(id,host,from_addr,base_url,updated_at) VALUES(1,'smtp.example.com','hi@example.com','https://old.example.com',1)`)
	exec(`INSERT INTO sync_library(user_id,origin) VALUES(?,'https://old.example.com')`, owner)
	exec(`INSERT INTO mcp_oauth_access(hash,connection_id,scope,resource,expires_at) VALUES('a1',?,'read','https://old.example.com/mcp',9999999999999)`, conn)
	exec(`INSERT INTO mcp_oauth_refresh(hash,connection_id,client_id,resource,scope,expires_at) VALUES('r1',?,'c','https://old.example.com/mcp','read',9999999999999)`, conn)
	exec(`INSERT INTO project_runs(id,user_id,project_name,use_worktrees,created_at) VALUES('run1',?,'Shop Front',0,1)`, owner)
	exec(`INSERT INTO project_run_members(run_id,user_id,position,host,dir,tool,state) VALUES('run1',?,0,'laptop','/a','claude','pending'),('run1',?,1,'laptop','/b','claude','started')`, owner, owner)
	exec(`INSERT INTO software_batches(id,user_id,tool,state,created_at,updated_at) VALUES('batch1',?,'opencode','running',1,1)`, owner)
	exec(`INSERT INTO software_batch_members(batch_id,user_id,position,agent_id,host,state,updated_at) VALUES
		('batch1',?,0,'a1','laptop','applied',1),('batch1',?,1,'a1','laptop','running',1),('batch1',?,2,'a1','desk','pending',1)`, owner, owner, owner)
	exec(`INSERT INTO mail_outbox(id,to_addr,state,created_at,updated_at) VALUES('m1','sam@example.com','queued',1,1),('m2','kim@example.com','sending',1,1),('m3','lee@example.com','delivered',1,1)`)
	return path
}

func copyOf(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "reet.db")
	if err := os.WriteFile(dst, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

// dump is every table's rows, as text, by table.
func dump(t *testing.T, path string) map[string]string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	names, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for names.Next() {
		var n string
		names.Scan(&n)
		tables = append(tables, n)
	}
	names.Close()
	out := map[string]string{}
	for _, tb := range tables {
		rows, err := db.Query(`SELECT * FROM "` + tb + `" ORDER BY rowid`)
		if err != nil {
			// A WITHOUT ROWID table has no rowid; read it unordered.
			if rows, err = db.Query(`SELECT * FROM "` + tb + `"`); err != nil {
				t.Fatal(err)
			}
		}
		cols, _ := rows.Columns()
		var b strings.Builder
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			rows.Scan(ptrs...)
			fmt.Fprintln(&b, vals...)
		}
		rows.Close()
		out[tb] = b.String()
	}
	return out
}

func scalar(t *testing.T, path, q string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var s string
	if err := db.QueryRow(q).Scan(&s); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return s
}

// With --public-url the two saved addresses change and the MCP grants go; with
// nothing else touched. Without it nothing about addresses is touched.
func TestAPublicURLChangesTheTwoAddressesAndGrantsAndNothingElse(t *testing.T) {
	src := movingFixture(t)
	with, without := copyOf(t, src), copyOf(t, src)
	ctx := context.Background()
	ad, err := AdaptDatabaseFile(ctx, with, AdaptOptions{PublicURL: "https://new.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AdaptDatabaseFile(ctx, without, AdaptOptions{}); err != nil {
		t.Fatal(err)
	}
	if !ad.MailBaseRewritten || ad.SyncOriginsRewritten != 1 || ad.MCPGrantsStale != 2 {
		t.Fatalf("adaptation: %+v", ad)
	}
	if got := scalar(t, with, `SELECT base_url FROM mail_settings`); got != "https://new.example.com" {
		t.Fatalf("mail base URL %q", got)
	}
	if got := scalar(t, with, `SELECT origin FROM sync_library`); got != "https://new.example.com" {
		t.Fatalf("sync origin %q", got)
	}
	// With no option they are untouched.
	if got := scalar(t, without, `SELECT base_url FROM mail_settings`); got != "https://old.example.com" {
		t.Fatalf("mail base URL changed without the option: %q", got)
	}
	if got := scalar(t, without, `SELECT origin FROM sync_library`); got != "https://old.example.com" {
		t.Fatalf("sync origin changed without the option: %q", got)
	}
	if n := rows(t, without, `SELECT COUNT(*) FROM mcp_oauth_access`) + rows(t, without, `SELECT COUNT(*) FROM mcp_oauth_refresh`); n != 2 {
		t.Fatalf("grants were removed without the option: %d left", n)
	}
	if n := rows(t, with, `SELECT COUNT(*) FROM mcp_oauth_access`) + rows(t, with, `SELECT COUNT(*) FROM mcp_oauth_refresh`); n != 0 {
		t.Fatalf("%d stale grants remain", n)
	}
	// Every other table is identical between the two, apart from the activity
	// log, which records the choice, and the tables whose work is held back in
	// both (their timestamps differ between the two runs; see the test of the
	// held work).
	a, b := dump(t, with), dump(t, without)
	changed := map[string]bool{"mail_settings": true, "sync_library": true, "mcp_oauth_access": true, "mcp_oauth_refresh": true, "mcp_oauth_codes": true, "events": true,
		"mail_outbox": true, "software_batches": true, "software_batch_members": true}
	for table, want := range b {
		if !changed[table] && a[table] != want {
			t.Errorf("table %s changed although only the two addresses and the grants should", table)
		}
	}
}

func TestMCPConnectionsAreKeptWhenOnlyTheirGrantsGoStale(t *testing.T) {
	path := movingFixture(t)
	before := rows(t, path, `SELECT COUNT(*) FROM mcp_connections WHERE revoked_at=0`)
	if _, err := AdaptDatabaseFile(context.Background(), path, AdaptOptions{PublicURL: "https://new.example.com"}); err != nil {
		t.Fatal(err)
	}
	if got := rows(t, path, `SELECT COUNT(*) FROM mcp_connections WHERE revoked_at=0`); got != before {
		t.Fatalf("connections: %d, want %d", got, before)
	}
}

func TestRestoringHoldsBackRunsBatchesAndMailUntilTheyAreRetried(t *testing.T) {
	path := movingFixture(t)
	ad, err := AdaptDatabaseFile(context.Background(), path, AdaptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// A pending run member waits as failed with a code, so the retry the product
	// already has starts it; a member already started is left alone.
	if got := scalar(t, path, `SELECT state||'/'||code FROM project_run_members WHERE position=0`); got != "failed/paused_by_restore" {
		t.Fatalf("pending member: %s", got)
	}
	if got := scalar(t, path, `SELECT state FROM project_run_members WHERE position=1`); got != "started" {
		t.Fatalf("a started member was changed: %s", got)
	}
	// A software batch stops on its first unfinished machine (the one that was
	// running), and the rest wait.
	if got := scalar(t, path, `SELECT state FROM software_batches`); got != "stopped" {
		t.Fatalf("batch: %s", got)
	}
	if got := scalar(t, path, `SELECT group_concat(state||'/'||code,',') FROM (SELECT * FROM software_batch_members ORDER BY position)`); got != "applied/,failed/paused_by_restore,pending/" {
		t.Fatalf("batch members: %s", got)
	}
	// Queued and sending mail is held; delivered mail is not.
	if got := scalar(t, path, `SELECT group_concat(state,',') FROM (SELECT state FROM mail_outbox ORDER BY id)`); got != "held,held,delivered" {
		t.Fatalf("mail: %s", got)
	}
	joined := strings.Join(ad.Paused, "\n")
	for _, want := range []string{"project run run1 (Shop Front): 1 member(s)", "software batch batch1 (opencode)", "2 queued email(s)"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the list does not say %q:\n%s", want, joined)
		}
	}
}

func TestTheHeldWorkIsWhatTheOrchestratorsDoNotResume(t *testing.T) {
	path := movingFixture(t)
	if _, err := AdaptDatabaseFile(context.Background(), path, AdaptOptions{}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if runs, err := st.PendingRuns(); err != nil || len(runs) != 0 {
		t.Fatalf("a held run would resume: %v %v", runs, err)
	}
	if batches, err := st.ResumeSoftwareBatches(); err != nil || len(batches) != 0 {
		t.Fatalf("a held batch would resume: %v %v", batches, err)
	}
	// And the existing retry resumes it.
	owner, _, _ := st.OwnerUser()
	if m, err := st.RetryRunMember(owner.ID, "run1", 0); err != nil || m.State != RunMemberPending {
		t.Fatalf("retry: %+v %v", m, err)
	}
	if m, err := st.RetrySoftwareBatchMember(owner.ID, "batch1", 1); err != nil || m.State != SoftwareMemberPending {
		t.Fatalf("batch retry: %+v %v", m, err)
	}
	if mail, err := st.RequeueMail("m1"); err != nil || mail.State != MailQueued {
		t.Fatalf("mail retry: %+v %v", mail, err)
	}
}

func TestAPublicURLThatIsNotAPlainOriginIsRefused(t *testing.T) {
	for _, bad := range []string{"", "reet.example.com", "ftp://x.example", "https://u:p@x.example", "https://x.example/path", "https://x.example?a=b", "https://x.example#f", "https://", "mailto:a@b.c"} {
		if _, err := ValidatePublicURL(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	for in, want := range map[string]string{"https://reet.example.com": "https://reet.example.com", "https://reet.example.com/": "https://reet.example.com", " http://10.0.0.5:8080 ": "http://10.0.0.5:8080"} {
		if got, err := ValidatePublicURL(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
}
