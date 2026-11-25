package api

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

// An agent drops its finished result on a 403, so only a refused enrolment may
// answer one: a storage failure answers 500 and the agent sends the result again.
func TestAgentIdentityAnswers403ForARefusalAnd500ForAStorageFailure(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := st.DefaultOwner()
	if _, err = st.CreateAPIToken(uid, "owner", auth.HashToken("fixture-owner")); err != nil {
		t.Fatal(err)
	}
	agent, secret, err := st.EnrollSyncAgent(uid, "laptop", "test")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(st, deadFetcher(t), "", WithAuthMode(AuthAccounts)))
	defer srv.Close()
	identity := func(key string) (int, string) {
		return do(t, srv, "POST", "/api/v2/sync/result", `{"id":"no-such-job","status":"succeeded"}`,
			"X-Reet-Token", "fixture-owner", "X-Reet-Agent", agent.ID, "X-Reet-Agent-Key", key)
	}

	if code, body := identity("wrong"); code != 403 || !strings.Contains(body, "missing or revoked") {
		t.Fatalf("wrong secret: %d %s", code, body)
	}
	if code, body := identity(secret); code != 409 {
		t.Fatalf("right secret: %d %s", code, body)
	}

	db, err := sql.Open("sqlite", "file:"+filepath.Join(st.Path(), "reet.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`ALTER TABLE sync_agents RENAME COLUMN keep_synced TO keep_synced_lost`); err != nil {
		t.Fatal(err)
	}
	// The body names no column or table: a storage error is not the caller's to read.
	if code, body := identity(secret); code != 500 || strings.Contains(body, "missing or revoked") || strings.Contains(body, "keep_synced") {
		t.Fatalf("storage failure: %d %s", code, body)
	}
}
