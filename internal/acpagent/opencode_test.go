//go:build linux || darwin

package acpagent

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenCodePromptErrorRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, schema := range []string{
		`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL)`,
		`CREATE TABLE session_message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, type TEXT NOT NULL, seq INTEGER NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(schema); err != nil {
			t.Fatal(err)
		}
	}
	insertV1 := func(id, session string, created int64, message string) {
		_, err := db.Exec(`INSERT INTO message (id,session_id,time_created,time_updated,data) VALUES (?,?,?,?,?)`,
			id, session, created, created, `{"role":"assistant","time":{"created":`+strconv.FormatInt(created, 10)+`},"error":{"data":{"message":"`+message+`"}}}`)
		if err != nil {
			t.Fatal(err)
		}
	}
	insertV2 := func(id, session string, created int64, message string) {
		_, err := db.Exec(`INSERT INTO session_message (id,session_id,type,seq,time_created,time_updated,data) VALUES (?,?,?,?,?,?,?)`,
			id, session, "assistant", created, created, created, `{"time":{"created":`+strconv.FormatInt(created, 10)+`},"error":{"message":"`+message+`"}}`)
		if err != nil {
			t.Fatal(err)
		}
	}
	insertV1("old", "ses-1", 100, "too early")
	insertV1("same", "ses-1", 200, "from v1")
	insertV1("other", "ses-2", 300, "wrong session")
	insertV2("new", "ses-1", 400, "newest from v2")

	if got := openCodePromptErrorAt(path, "ses-1", 200); got != "newest from v2" {
		t.Fatalf("recovered %q", got)
	}
	if got := openCodePromptErrorAt(path, "ses-1", 401); got != "" {
		t.Fatalf("old error recovered: %q", got)
	}
	if got := openCodePromptErrorAt(filepath.Join(dir, "missing.db"), "ses-1", 0); got != "" {
		t.Fatalf("missing database recovered %q", got)
	}
}
