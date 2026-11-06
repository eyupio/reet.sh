package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestoredCopyPausesGoalsAndDoesNotAdoptOldGoalInstructions(t *testing.T) {
	st, uid, session := queueFixture(t)
	cid := strings.Repeat("a", 32)
	goalID := strings.Repeat("b", 32)
	turnID := strings.Repeat("c", 32)
	c, err := st.CreateGafferConversationFor(uid, GafferConversation{ID: cid, SessionID: session, ProviderID: strings.Repeat("d", 32)})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{strings.Repeat("e", 32), strings.Repeat("f", 32)} {
		if err = st.EnqueueTerminalFor(uid, session, id, "An instruction", nil); err != nil {
			t.Fatal(err)
		}
	}
	goal := GafferGoal{OwnerID: uid, ID: goalID, ConversationID: cid, SessionID: session, Revision: 1, State: "active", Text: "PRIVATE_GOAL_TEXT", ExpiresAt: time.Now().Add(time.Hour).UnixMilli(), MaxReviews: 12}
	err = st.withTx(func(tx *sql.Tx) error {
		if err := saveGoal(tx, goal); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO gaffer_turns(conversation_id,id,request_revision,prompt,state,provider_id,provider_name,model,started_at,updated_at,goal_id,goal_revision) VALUES(?,?,1,'Goal review','completed',?,'Fake','fake-model',?,?,?,1)`, cid, turnID, c.ProviderID, now(), now(), goalID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE terminal_chat_queue SET origin='companion' WHERE id=?`, strings.Repeat("e", 32)); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO worker_message_receipts VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, uid, "gaffer", cid, session, turnID, strings.Repeat("e", 32), turnID, "An instruction", "hash", 1, "queued", "", now())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.Dir(), "reet.db")
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := AdaptDatabaseFile(context.Background(), path, AdaptOptions{})
	if err != nil || result.GafferGoalsPaused != 1 {
		t.Fatal(result, err)
	}
	reopened, err := Open(st.Dir())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	g, err := reopened.GafferGoalFor(uid, cid)
	if err != nil || g.State != "needs_user" || g.Revision != 2 {
		t.Fatal(g, err)
	}
	receipt, err := reopened.WorkerMessageFor(uid, "gaffer", cid, session, turnID)
	if err != nil || receipt.State != "removed" {
		t.Fatal(receipt, err)
	}
	list, err := reopened.TerminalQueueFor(uid, session)
	if err != nil || len(list) != 1 || list[0].Origin != "" {
		t.Fatal("restore touched a manual instruction", list, err)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "PRIVATE_GOAL_TEXT") {
		t.Fatal("goal text leaked to restore report")
	}
}
