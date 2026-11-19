package store

import (
	"testing"
)

func schemaObjectExists(t *testing.T, s *Store, name string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type IN ('table','trigger') AND name=?`, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func shelfHasColumn(t *testing.T, s *Store, name string) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM pragma_table_info('shelf') WHERE name=?`, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// legacyEngine gives a store the old engine's tables, trigger and shelf flags
// as a database upgraded from before skills-engine-drop-v1 has them, with a
// row in each table it can hold one in.
func legacyEngine(t *testing.T, s *Store, uid int64, agent string) {
	t.Helper()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS sync_targets(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,target_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,source_id TEXT NOT NULL,source_name TEXT NOT NULL,desired TEXT NOT NULL,revision TEXT NOT NULL,updated_at INTEGER NOT NULL,PRIMARY KEY(user_id,target_id))`,
		`CREATE TABLE IF NOT EXISTS sync_options(target_id TEXT PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE,mode TEXT NOT NULL DEFAULT 'mirror',baseline TEXT NOT NULL DEFAULT '[]',applied_revision TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS sync_policies(target_id TEXT PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE,follow_source INTEGER NOT NULL DEFAULT 0,problem TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS sync_library(user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,origin TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sync_runs(id TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,agent_id TEXT NOT NULL REFERENCES sync_agents(id) ON DELETE CASCADE,targets TEXT NOT NULL,cursor INTEGER NOT NULL DEFAULT 0,status TEXT NOT NULL,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sync_run_jobs(job_id TEXT PRIMARY KEY REFERENCES sync_jobs(id) ON DELETE CASCADE,run_id TEXT NOT NULL REFERENCES sync_runs(id) ON DELETE CASCADE,position INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS sync_receipts(job_id TEXT NOT NULL REFERENCES sync_jobs(id) ON DELETE CASCADE,status TEXT NOT NULL,hash TEXT NOT NULL,PRIMARY KEY(job_id,status,hash))`,
		`CREATE TABLE IF NOT EXISTS skill_installs(job_id TEXT PRIMARY KEY REFERENCES sync_jobs(id) ON DELETE CASCADE,user_id INTEGER NOT NULL,target_id TEXT NOT NULL,slug TEXT NOT NULL,created_target INTEGER NOT NULL DEFAULT 0,added INTEGER NOT NULL DEFAULT 0,revision TEXT NOT NULL,prev_revision TEXT NOT NULL DEFAULT '',replaced TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS canonical_library_selections(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,target_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,item_ids TEXT NOT NULL,version TEXT NOT NULL,PRIMARY KEY(user_id,target_id))`,
		`CREATE TABLE IF NOT EXISTS library_selection_accounts(user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS library_selections(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,target_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,selected TEXT NOT NULL,PRIMARY KEY(user_id,target_id))`,
		`CREATE TABLE IF NOT EXISTS library_bases(user_id INTEGER NOT NULL,enrollment_id TEXT NOT NULL,binding_id TEXT NOT NULL,item_id TEXT NOT NULL,revision_id TEXT NOT NULL,PRIMARY KEY(user_id,enrollment_id,binding_id))`,
		`CREATE TABLE IF NOT EXISTS library_connectors(user_id INTEGER NOT NULL,enrollment_id TEXT NOT NULL,protocol INTEGER NOT NULL,PRIMARY KEY(user_id,enrollment_id))`,
		`CREATE TABLE IF NOT EXISTS library_edit_events(user_id INTEGER NOT NULL,event_key TEXT NOT NULL,item_id TEXT NOT NULL,PRIMARY KEY(user_id,event_key))`,
		`CREATE TABLE IF NOT EXISTS library_difference_choices(user_id INTEGER NOT NULL,item_id TEXT NOT NULL,env_id TEXT NOT NULL,PRIMARY KEY(user_id,item_id,env_id))`,
		`CREATE TABLE IF NOT EXISTS skill_content_targets(id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS skill_content_previews(id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS skill_content_preview_capabilities(agent_id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS skill_content_receipts(id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS ignored(user_id INTEGER NOT NULL DEFAULT 0,id TEXT NOT NULL,name TEXT NOT NULL DEFAULT '',reason TEXT NOT NULL DEFAULT '',at INTEGER NOT NULL,PRIMARY KEY(user_id,id))`,
		`CREATE TRIGGER IF NOT EXISTS library_intent_fence AFTER UPDATE OF status ON library_items BEGIN UPDATE sync_targets SET revision='moved' WHERE user_id=NEW.user_id; END`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	for _, column := range []string{"bookmarked INTEGER NOT NULL DEFAULT 0", "used INTEGER NOT NULL DEFAULT 0"} {
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM pragma_table_info('shelf') WHERE name=?`, column[:len(column)-len(" INTEGER NOT NULL DEFAULT 0")]).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			if _, err := s.db.Exec(`ALTER TABLE shelf ADD COLUMN ` + column); err != nil {
				t.Fatal(err)
			}
		}
	}
	at := now()
	for _, stmt := range []string{
		`INSERT INTO environments(id,user_id,name,kind,agent,host,first_seen,last_seen) VALUES('env-engine',?1,'laptop agents','machine','agents','laptop',?3,?3)`,
		`INSERT INTO sync_targets VALUES(?1,'env-engine','src','Source','[]','r1',?3)`,
		`INSERT INTO sync_options(target_id) VALUES('env-engine')`,
		`INSERT INTO sync_policies(target_id) VALUES('env-engine')`,
		`INSERT INTO sync_library VALUES(?1,'library')`,
		`INSERT INTO sync_runs(id,user_id,agent_id,targets,status,created_at,updated_at) VALUES('run-1',?1,?2,'[]','running',?3,?3)`,
		`INSERT INTO sync_run_jobs(job_id,run_id) VALUES('skill-queued','run-1')`,
		`INSERT INTO sync_receipts VALUES('skill-done','succeeded','hash')`,
		`INSERT INTO skill_installs(job_id,user_id,target_id,slug,revision) VALUES('skill-done',?1,'env-engine','seo-audit','r1')`,
		`INSERT INTO canonical_library_selections VALUES(?1,'env-engine','[]','v1')`,
		`INSERT INTO library_selection_accounts VALUES(?1)`,
		`INSERT INTO library_selections VALUES(?1,'env-engine','[]')`,
		`INSERT INTO ignored(user_id,id,name,reason,at) VALUES(?1,'owner/repo@gone','gone','removed',?3)`,
	} {
		if _, err := s.db.Exec(stmt, uid, agent, at); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

// engineJobs adds a binary update and old-engine skill jobs in each status.
func engineJobs(t *testing.T, s *Store, uid int64, agent string) {
	t.Helper()
	for _, job := range [][3]string{
		{"update-queued", "update", "queued"},
		{"update-done", "update", "succeeded"},
		{"skill-queued", "apply", "queued"},
		{"skill-approval", "apply", "awaiting-approval"},
		{"preview-queued", "preview", "queued"},
		{"skill-done", "apply", "succeeded"},
	} {
		if _, err := s.db.Exec(`INSERT INTO sync_jobs(id,user_id,agent_id,kind,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, job[0], uid, agent, job[1], job[2], now(), now()); err != nil {
			t.Fatal(err)
		}
	}
}

func syncJobIDs(t *testing.T, s *Store) map[string]string {
	t.Helper()
	rows, err := s.db.Query(`SELECT id,status FROM sync_jobs`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, status string
		if err = rows.Scan(&id, &status); err != nil {
			t.Fatal(err)
		}
		out[id] = status
	}
	return out
}

func TestEngineDropMigration(t *testing.T) {
	s := newStore(t)
	uid, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	agent, _, err := s.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	engineJobs(t, s, uid, agent.ID)
	legacyEngine(t, s, uid, agent.ID)
	if _, err = s.db.Exec(`INSERT INTO skills(id,source,slug,source_type,name,created_at,updated_at) VALUES('owner/repo@seo-audit','owner/repo','seo-audit','github','seo-audit',1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO shelf(user_id,skill_id,liked,used,bookmarked,notes,created_at,updated_at) VALUES(?,'owner/repo@seo-audit',1,1,0,'kept',1,1)`, uid); err != nil {
		t.Fatal(err)
	}

	for run := 1; run <= 2; run++ {
		if err = s.migrateSkillsEngineDrop(); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		for _, table := range append(append([]string{}, engineDropTables...), "library_intent_fence") {
			if schemaObjectExists(t, s, table) {
				t.Fatalf("run %d: %s is still there", run, table)
			}
		}
		for _, column := range []string{"used", "bookmarked"} {
			if shelfHasColumn(t, s, column) {
				t.Fatalf("run %d: shelf.%s is still there", run, column)
			}
		}
		jobs := syncJobIDs(t, s)
		want := map[string]string{"update-queued": "queued", "update-done": "succeeded", "skill-done": "succeeded"}
		if len(jobs) != len(want) {
			t.Fatalf("run %d: jobs %v, want %v", run, jobs, want)
		}
		for id, status := range want {
			if jobs[id] != status {
				t.Fatalf("run %d: jobs %v, want %v", run, jobs, want)
			}
		}
	}
	// What the shelf row says that is not a stage is kept, and its library
	// item still stands for it.
	var liked int
	var notes string
	if err = s.db.QueryRow(`SELECT liked,notes FROM shelf WHERE user_id=? AND skill_id='owner/repo@seo-audit'`, uid).Scan(&liked, &notes); err != nil || liked != 1 || notes != "kept" {
		t.Fatalf("shelf row after the drop: liked=%d notes=%q %v", liked, notes, err)
	}
	if got := skillStateNamed(t, s, uid, "seo-audit"); got.status != SkillActive {
		t.Fatalf("library item after the drop: %+v", got)
	}
	var marked int
	if err = s.db.QueryRow(`SELECT count(*) FROM library_migrations WHERE name='skills-engine-drop-v1'`).Scan(&marked); err != nil || marked != 1 {
		t.Fatalf("migration record %d, %v", marked, err)
	}
}
