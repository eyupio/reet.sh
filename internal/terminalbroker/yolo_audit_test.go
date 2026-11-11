package terminalbroker

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// yoloRows returns the yolo-allow audit details, oldest first.
func yoloRows(t *testing.T, b *Broker, uid int64) []map[string]any {
	t.Helper()
	events, err := b.st.ListEventsFor(uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Action != "yolo-allow" {
			continue
		}
		if events[i].Source != "machine" {
			t.Fatalf("source %q", events[i].Source)
		}
		meta := map[string]any{}
		if err := json.Unmarshal([]byte(events[i].Detail), &meta); err != nil {
			t.Fatal(err)
		}
		out = append(out, meta)
	}
	return out
}

// The approvals YOLO gives in one turn share one audit row that counts them.
// Every approval is still recorded, as it happens; a new turn, a long turn
// and another chat each get a row of their own.
func TestYoloApprovalsAreCountedOnOneAuditRowPerTurn(t *testing.T) {
	b, _, uid, agent, _ := switchFixture(t, true)
	chat := switchChat(t, b, uid, agent)
	other := switchChat(t, b, uid, agent)
	plain := switchChat(t, b, uid, agent)
	for _, id := range []string{chat.ID, other.ID} {
		if err := b.st.SetTerminalYoloFor(uid, id, true); err != nil {
			t.Fatal(err)
		}
	}
	yes, no := true, false
	allow := func(session, permission string) {
		b.publishChat(uid, session, terminalproto.ACPEvent{Kind: "permission_resolved", PermissionID: permission, Allow: &yes})
	}
	allow(chat.ID, "permission-a")
	if rows := yoloRows(t, b, uid); len(rows) != 1 || rows[0]["count"] != float64(1) {
		t.Fatalf("the first approval was not recorded at once: %+v", rows)
	}
	allow(chat.ID, "permission-b")
	allow(chat.ID, "permission-c")
	// A refusal and an approval in a chat that is not on YOLO are not YOLO approvals.
	b.publishChat(uid, chat.ID, terminalproto.ACPEvent{Kind: "permission_resolved", PermissionID: "permission-d", Allow: &no})
	allow(plain.ID, "permission-e")
	allow(other.ID, "permission-f")
	rows := yoloRows(t, b, uid)
	if len(rows) != 2 {
		t.Fatalf("rows %+v", rows)
	}
	first := rows[0]
	if first["count"] != float64(3) || first["session"] != chat.ID || first["machine"] != agent || first["tool"] != "opencode" ||
		first["permission"] != "permission-a" || first["lastPermission"] != "permission-c" || first["from"].(float64) > first["to"].(float64) {
		t.Fatalf("rolled-up row %+v", first)
	}
	if rows[1]["count"] != float64(1) || rows[1]["session"] != other.ID {
		t.Fatalf("other chat's row %+v", rows[1])
	}

	// The turn ends: the next approval is a new row.
	b.publishChat(uid, chat.ID, terminalproto.ACPEvent{Kind: "status", State: "idle"})
	allow(chat.ID, "permission-g")
	allow(chat.ID, "permission-h")
	rows = yoloRows(t, b, uid)
	if len(rows) != 3 || rows[0]["count"] != float64(3) || rows[2]["count"] != float64(2) || rows[2]["permission"] != "permission-g" {
		t.Fatalf("after a new turn %+v", rows)
	}

	// A turn that outlasts the window starts another row rather than hiding hours in one.
	b.yoloMu.Lock()
	b.yoloAllows[chatKey{uid, chat.ID}].started = time.Now().Add(-yoloRollupWindow - time.Second)
	b.yoloMu.Unlock()
	allow(chat.ID, "permission-i")
	rows = yoloRows(t, b, uid)
	if len(rows) != 4 || rows[2]["count"] != float64(2) || rows[3]["count"] != float64(1) {
		t.Fatalf("after the window %+v", rows)
	}
	total := 0
	for _, r := range rows {
		total += int(r["count"].(float64))
	}
	if total != 7 {
		t.Fatalf("7 approvals were allowed, %d recorded", total)
	}

	// A closed chat keeps nothing in memory.
	b.closeChat(uid, chat.ID)
	b.yoloMu.Lock()
	_, kept := b.yoloAllows[chatKey{uid, chat.ID}]
	b.yoloMu.Unlock()
	if kept {
		t.Fatal("a closed chat kept its rollup")
	}
}
