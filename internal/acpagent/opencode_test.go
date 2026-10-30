//go:build linux || darwin

package acpagent

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestOpenCodeLogErrorRecovery(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.log")
	now := time.Now().UTC()
	line := func(at time.Time, session, level, message, small, text string) string {
		return `timestamp=` + at.Format(time.RFC3339Nano) + ` level=` + level + ` run=x message="` + message + `" providerID=opencode modelID=stub session.id=` + session + ` small=` + small + ` agent=build mode=primary error.error=` + strconv.Quote(text)
	}
	content := strings.Join([]string{
		line(now.Add(-3*time.Minute), "ses-1", "ERROR", "stream error", "false", "too early"),
		line(now.Add(-2*time.Minute), "ses-2", "ERROR", "stream error", "false", "wrong session"),
		line(now.Add(-2*time.Minute), "ses-1", "WARN", "stream error", "false", "wrong level"),
		line(now.Add(-2*time.Minute), "ses-1", "ERROR", "stream error", "true", "title model"),
		line(now.Add(-90*time.Second), "ses-1", "ERROR", "stream error", "false", "AI_APICallError: older"),
		line(now.Add(-30*time.Second), "ses-1", "ERROR", "stream error", "false", `AI_APICallError: quota "gone"`),
		line(now, "ses-1", "ERROR", "other error", "false", "not a stream"),
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if got := openCodeLogErrorAt(path, "ses-1", now.Add(-time.Minute).UnixMilli()); got != `quota "gone"` {
		t.Fatalf("recovered %q", got)
	}
	home := t.TempDir()
	homeLog := openCodeLogPath(home)
	if err := os.MkdirAll(filepath.Dir(homeLog), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(homeLog, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if got := openCodePromptError(home, "ses-1", now.Add(-time.Minute).UnixMilli()); got != `quota "gone"` {
		t.Fatalf("prompt fallback recovered %q", got)
	}
	if got := openCodeLogErrorAt(filepath.Join(dir, "missing.log"), "ses-1", 0); got != "" {
		t.Fatalf("missing log recovered %q", got)
	}
}

func TestOpenCodeConfigCarriesTheChosenModel(t *testing.T) {
	for name, model := range map[string]string{"default": "", "chosen": "ollama-cloud/deepseek-v4.1-flash:1017"} {
		var config map[string]any
		if err := json.Unmarshal([]byte(openCodeConfigContent(model)), &config); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got, ok := config["model"]
		if model == "" {
			if ok {
				t.Fatalf("default config model %v", got)
			}
			continue
		}
		if got != model {
			t.Fatalf("%s config model %v", name, got)
		}
	}
}
