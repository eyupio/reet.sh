package store

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

func TestGoalOverviewListIsBoundedAndOwnedThroughConversation(t *testing.T) {
	st, uid, session := queueFixture(t)
	other, err := st.CreateUser("other-goals@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	// Seed above the normal conversation quota to verify the read independently
	// bounds old/imported databases as well as current application writes.
	err = st.withTx(func(tx *sql.Tx) error {
		for i := 0; i < 102; i++ {
			id := fmt.Sprintf("%032x", i+1)
			if _, err := tx.Exec(`INSERT INTO gaffer_conversations(id,user_id,session_id,provider_id,model,revision,created_at,updated_at) VALUES(?,?,?,?,'',1,1,1)`, id, uid, fmt.Sprintf("session-%d", i), strings.Repeat("a", 32)); err != nil {
				return err
			}
			owner := uid
			if i == 0 {
				owner = other.ID
			} // mismatched parent ownership must never leak
			if err := saveGoal(tx, GafferGoal{OwnerID: owner, ID: id, ConversationID: id, SessionID: fmt.Sprintf("session-%d", i), Text: session, State: "paused"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := st.GafferGoalsFor(uid)
	if err != nil || len(list) != 100 {
		t.Fatal(len(list), err)
	}
	for _, g := range list {
		if g.OwnerID != uid || g.ID == fmt.Sprintf("%032x", 1) {
			t.Fatal("foreign goal included", g)
		}
	}
	list, err = st.GafferGoalsFor(other.ID)
	if err != nil || len(list) != 0 {
		t.Fatal("mismatched conversation ownership", list, err)
	}
}
