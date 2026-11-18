package store

import (
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const usageDay = int64(24 * 60 * 60 * 1000)

// usageSession is a structured chat of the given agent on the fixture's machine.
func usageSession(t *testing.T, s *Store, uid int64, a SyncAgent, c RemoteAgentCapabilities, tool, model, title string) TerminalSession {
	t.Helper()
	session := createTerminal(t, s, uid, a, c)
	if _, err := s.db.Exec(`UPDATE terminal_sessions SET tool=?,model=?,title=?,structured=1 WHERE id=?`, tool, model, title, session.ID); err != nil {
		t.Fatal(err)
	}
	session.Tool, session.Model, session.Title, session.Structured = tool, model, title, true
	return session
}

func usageEventCount(t *testing.T, s *Store, uid int64) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM terminal_usage_events WHERE user_id=?`, uid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUsageEventsAreDatedAndOutliveTheSessionAndItsMachine(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	session := usageSession(t, s, uid, a, c, "opencode", "made-up-large", "Fix the login")

	// A report with no tokens (a context report alone) is not an event.
	if err := s.AddTerminalSessionUsage(uid, session.ID, 0, 0, 0, 100, 1000); err != nil {
		t.Fatal(err)
	}
	if n := usageEventCount(t, s, uid); n != 0 {
		t.Fatalf("a report without tokens was dated: %d events", n)
	}
	if err := s.AddTerminalSessionUsage(uid, session.ID, 10, 5, 2, 0, 0); err != nil {
		t.Fatal(err)
	}
	// The latest title is the one kept.
	if err := s.RenameTerminalFor(uid, session.ID, "Fix the sign-in"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTerminalSessionUsage(uid, session.ID, 1, 1, 0, 200, 1000); err != nil {
		t.Fatal(err)
	}
	if n := usageEventCount(t, s, uid); n != 2 {
		t.Fatalf("events %d, want 2", n)
	}
	from, to := now()-usageDay, now()+usageDay
	got, err := s.UsageFor(uid, from, to, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tools) != 1 || got.Tools[0].Tool != "opencode" || got.Tools[0].Input != 11 || got.Tools[0].Output != 6 || got.Tools[0].Cached != 2 || got.Tools[0].Sessions != 1 {
		t.Fatalf("tools: %+v", got.Tools)
	}
	if len(got.Sessions) != 1 {
		t.Fatalf("sessions: %+v", got.Sessions)
	}
	row := got.Sessions[0]
	if row.ID != session.ID || row.Title != "Fix the sign-in" || row.Tool != "opencode" || row.Model != "made-up-large" || row.AgentID != a.ID || row.Host != "laptop" || !row.Structured || row.Archived || row.Input != 11 || row.ContextUsed != 200 || row.ContextSize != 1000 {
		t.Fatalf("session row: %+v", row)
	}
	if got.Since == 0 || got.Since > now() {
		t.Fatalf("since: %d", got.Since)
	}

	// Archiving drops the session's running totals but not its dated events.
	if err := s.ObserveTerminalStateFor(uid, a.ID, session.ID, c.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	zero := 0
	if err := s.ObserveTerminalStateFor(uid, a.ID, session.ID, c.Generation, terminalproto.Exited, &zero); err != nil {
		t.Fatal(err)
	}
	if err := s.ArchiveTerminalFor(uid, session.ID); err != nil {
		t.Fatal(err)
	}
	got, err = s.UsageFor(uid, from, to, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sessions) != 1 || !got.Sessions[0].Archived || got.Sessions[0].Title != "Fix the sign-in" || got.Sessions[0].Input != 11 || got.Tools[0].Input != 11 {
		t.Fatalf("after archive: %+v %+v", got.Tools, got.Sessions)
	}

	// Revoking the machine deletes its sessions; the events keep the agent, title and machine name.
	if _, err := s.db.Exec(`DELETE FROM sync_agents WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TerminalSessionFor(uid, session.ID); err == nil {
		t.Fatal("the session outlived its machine; the test does not prove anything")
	}
	got, err = s.UsageFor(uid, from, to, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Tool != "opencode" || got.Sessions[0].Title != "Fix the sign-in" || got.Sessions[0].Host != "laptop" || !got.Sessions[0].Archived || got.Tools[0].Tool != "opencode" || got.Tools[0].Input != 11 {
		t.Fatalf("after the machine went: %+v %+v", got.Tools, got.Sessions)
	}
}

func TestUsageRangesAreHalfOpenAndOldEventsArePruned(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	session := usageSession(t, s, uid, a, c, "claude", "made-up-small", "Tidy")
	base := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC).UnixMilli()
	for i, at := range []int64{base - 1, base, base + 1000, base + usageDay} {
		if err := s.addTerminalSessionUsageAt(uid, session.ID, int64(i+1), 0, 0, 0, 0, at); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.UsageFor(uid, base, base+usageDay, 200)
	if err != nil {
		t.Fatal(err)
	}
	// base and base+1000 are in [base, base+day); base-1 and base+day are not.
	if len(got.Tools) != 1 || got.Tools[0].Input != 2+3 || got.Tools[0].FirstAt != base {
		t.Fatalf("half-open range: %+v", got.Tools)
	}
	if got.Since != base-1 {
		t.Fatalf("since %d, want %d", got.Since, base-1)
	}
	// A write 91 days on prunes what is older than 90 days.
	later := base + 91*usageDay + 1
	if err := s.addTerminalSessionUsageAt(uid, session.ID, 7, 0, 0, 0, 0, later); err != nil {
		t.Fatal(err)
	}
	if n := usageEventCount(t, s, uid); n != 1 {
		t.Fatalf("events after prune %d, want 1", n)
	}
	var titles int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM terminal_usage_sessions WHERE user_id=?`, uid).Scan(&titles); err != nil || titles != 1 {
		t.Fatalf("kept titles %d %v", titles, err)
	}
}

func TestUsageAggregatesPerAgentAndSessionWithGafferAndFactory(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	first := usageSession(t, s, uid, a, c, "claude", "made-up-small", "First")
	second := usageSession(t, s, uid, a, c, "claude", "made-up-large", "Second")
	third := usageSession(t, s, uid, a, c, "codex", "made-up-medium", "Third")
	base := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC).UnixMilli()
	for _, e := range []struct {
		id         string
		in, out, c int64
		at         int64
	}{{first.ID, 100, 10, 50, base + 1}, {second.ID, 200, 20, 0, base + 3}, {first.ID, 1, 1, 1, base + 2}, {third.ID, 7, 3, 0, base + 4}} {
		if err := s.addTerminalSessionUsageAt(uid, e.id, e.in, e.out, e.c, 0, 0, e.at); err != nil {
			t.Fatal(err)
		}
	}
	// The second session is a Factory stage's: an outcome keeps its session id.
	data := `{"id":"o1","state":"done","sessions":[{"key":"k","title":"T","prompt":"p","requestId":"r","state":"done","updatedAt":1,"stage":0,"stages":[{"kind":"build","state":"done","requestId":"r1","sessionId":"` + second.ID + `"}]}]}`
	if _, err := s.db.Exec(`INSERT INTO gaffer_outcomes(id,user_id,created_at,data) VALUES('o1',?,?,?)`, uid, base, data); err != nil {
		t.Fatal(err)
	}
	// Gaffer's own replies: two in range, one outside, one with no usage reported.
	if _, err := s.db.Exec(`INSERT INTO gaffer_conversations(id,user_id,session_id,provider_id,model,created_at,updated_at) VALUES('c1',?,'','p1','made-up-chat',?,?)`, uid, base, base); err != nil {
		t.Fatal(err)
	}
	for i, turn := range []struct {
		at       int64
		reported int
	}{{base + 5, 1}, {base + 6, 1}, {base - 5, 1}, {base + 7, 0}} {
		if _, err := s.db.Exec(`INSERT INTO gaffer_turns(conversation_id,id,request_revision,prompt,state,provider_id,provider_name,model,input_tokens,output_tokens,usage_reported,started_at,updated_at,finished_at) VALUES('c1',?,1,'q','done','p1','Made-up provider','made-up-chat',?,?,?,?,?,?)`,
			string(rune('a'+i)), 30, 4, turn.reported, turn.at, turn.at, turn.at); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.UsageFor(uid, base, base+usageDay, 200)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]UsageToolTotal{}
	for _, tool := range got.Tools {
		tools[tool.Tool] = tool
	}
	if cl := tools["claude"]; cl.Input != 301 || cl.Output != 31 || cl.Cached != 51 || cl.Sessions != 2 || cl.FactoryStages != 1 || cl.FirstAt != base+1 {
		t.Fatalf("claude: %+v", cl)
	}
	if cx := tools["codex"]; cx.Input != 7 || cx.Sessions != 1 || cx.FactoryStages != 0 {
		t.Fatalf("codex: %+v", cx)
	}
	if len(got.Sessions) != 3 || got.Sessions[0].ID != third.ID || got.Sessions[1].ID != second.ID || got.Sessions[2].ID != first.ID {
		t.Fatalf("session order: %+v", got.Sessions)
	}
	if !got.Sessions[1].Factory || got.Sessions[0].Factory || got.Sessions[2].Input != 101 || got.Sessions[2].LastAt != base+2 {
		t.Fatalf("session rows: %+v", got.Sessions)
	}
	if got.More {
		t.Fatal("more with every session listed")
	}
	if len(got.Gaffer) != 1 || got.Gaffer[0].Provider != "Made-up provider" || got.Gaffer[0].Model != "made-up-chat" || got.Gaffer[0].Input != 60 || got.Gaffer[0].Output != 8 || got.Gaffer[0].Replies != 2 {
		t.Fatalf("gaffer: %+v", got.Gaffer)
	}

	// The list is bounded and says so.
	got, err = s.UsageFor(uid, base, base+usageDay, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sessions) != 2 || !got.More || got.Sessions[0].ID != third.ID {
		t.Fatalf("bounded: %d more=%v", len(got.Sessions), got.More)
	}

	// Another account sees none of it, and a deleted account's events go with it.
	other, err := s.CreateUser("usage-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.UsageFor(other.ID, base, base+usageDay, 200); err != nil || len(got.Tools) != 0 || len(got.Sessions) != 0 || len(got.Gaffer) != 0 || got.Since != 0 {
		t.Fatalf("another account: %+v %v", got, err)
	}
	oa, oc := terminalAgent(t, s, other.ID, "desktop")
	theirs := usageSession(t, s, other.ID, oa, oc, "cline", "made-up-tiny", "Theirs")
	if err := s.AddTerminalSessionUsage(other.ID, theirs.ID, 3, 3, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if usageEventCount(t, s, other.ID) != 1 {
		t.Fatal("the other account's report was not dated")
	}
	if err := s.DeleteUser(other.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM terminal_usage_events WHERE user_id=?)+(SELECT COUNT(*) FROM terminal_usage_sessions WHERE user_id=?)`, other.ID, other.ID).Scan(&left); err != nil || left != 0 {
		t.Fatalf("a deleted account's usage stayed: %d %v", left, err)
	}
}
