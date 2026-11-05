package store

import (
	"fmt"
	"testing"
	"time"
)

func benchTerminalSessionsStore(b *testing.B) (*Store, int64) {
	b.Helper()
	s, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = s.Close() })
	u, err := s.CreateUser("terminal-bench@example.com", "terminal-bench", "owner", "hash")
	if err != nil {
		b.Fatal(err)
	}
	agentID := "bench-agent-0001"
	at := time.Now().UnixMilli()
	if _, err := s.db.Exec(`INSERT INTO sync_agents(id,user_id,host,secret_hash,version,last_seen) VALUES(?,?,?,?,?,?)`,
		agentID, u.ID, "bench-host", "hash", "dev", at); err != nil {
		b.Fatal(err)
	}
	for i := range 100 {
		id := fmt.Sprintf("bench-session-%04d", i)
		if _, err := s.db.Exec(`INSERT INTO terminal_sessions(
			id,user_id,agent_id,title,state,created_at,updated_at,exit_code,worktree_path,worktree_repo,worktree_branch,dir,
			tool,model,effort,yolo,project_id,structured,acp_session_id,activity_at,bell_at,attention_observed,completed_at,chat_status,agent_mode)
			VALUES(?,?,?,?,?,?,?,NULL,'','','',?,?,?,'',0,NULL,0,'',0,0,0,0,'','')`,
			id, u.ID, agentID, fmt.Sprintf("Session %d", i), "running", at-int64(i), at-int64(i), "/tmp/bench", "", "", ""); err != nil {
			b.Fatal(err)
		}
		if i%2 == 0 {
			if _, err := s.db.Exec(`INSERT INTO terminal_session_seen(user_id,session_id,seen_at) VALUES(?,?,?)`, u.ID, id, at); err != nil {
				b.Fatal(err)
			}
		}
	}
	return s, u.ID
}

func BenchmarkTerminalSessionsFor(b *testing.B) {
	s, uid := benchTerminalSessionsStore(b)
	b.ResetTimer()
	for range b.N {
		if _, err := s.TerminalSessionsFor(uid); err != nil {
			b.Fatal(err)
		}
	}
}
