package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPausedGoalCannotResumePastTheActiveGoalLimit(t *testing.T) {
	st, uid, session := queueFixture(t)
	cid := strings.Repeat("a", 32)
	if _, err := st.CreateGafferConversationFor(uid, GafferConversation{ID: cid, SessionID: session, ProviderID: strings.Repeat("b", 32)}); err != nil {
		t.Fatal(err)
	}
	p, err := st.SaveGafferProviderFor(uid, GafferProvider{ID: strings.Repeat("b", 32), Name: "Resume fixture", Kind: "openai_compatible", BaseURL: "http://127.0.0.1:1/v1", Model: "fake", Enabled: true, AllowSessions: true, AllowCode: true})
	if err != nil {
		t.Fatal(err)
	}
	sharing, err := st.SetContextGrantFor(uid, "gaffer", cid, 0, ContextGrant{SessionID: session, ContextFlags: ContextFlags{History: true, Text: true}, ProviderID: p.ID, ProviderRevision: p.Revision, Model: p.Model})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := st.SetWorkerGrantFor(uid, "gaffer", cid, 0, WorkerGrant{SessionID: session, Enabled: true, ProviderID: p.ID, ProviderRevision: p.Revision, Model: p.Model})
	if err != nil {
		t.Fatal(err)
	}
	pins := GafferGoal{SessionID: session, ProviderID: p.ID, ProviderRevision: p.Revision, Model: p.Model, ContextRevision: sharing.Revision, WorkerRevision: worker.Revision}
	err = st.withTx(func(tx *sql.Tx) error {
		goal := pins
		goal.OwnerID = uid
		goal.ID = cid
		goal.ConversationID = cid
		goal.Revision = 1
		goal.State = "paused"
		goal.MaxReviews = 12
		goal.ExpiresAt = now() + 3600000
		if err := saveGoal(tx, goal); err != nil {
			return err
		}
		for i := 0; i < 10; i++ {
			id := fmt.Sprintf("%032x", i+1)
			if _, err := tx.Exec(`INSERT INTO gaffer_conversations(id,user_id,session_id,provider_id,model,revision,created_at,updated_at) VALUES(?,?,?,?,'',1,1,1)`, id, uid, fmt.Sprintf("limit-session-%d", i), strings.Repeat("b", 32)); err != nil {
				return err
			}
			if err := saveGoal(tx, GafferGoal{OwnerID: uid, ID: id, ConversationID: id, SessionID: fmt.Sprintf("limit-session-%d", i), State: "active"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ControlGafferGoal(uid, cid, 1, "resume", pins); !errors.Is(err, ErrGafferChatBusy) {
		t.Fatal("resume bypassed the active goal limit", err)
	}
	g, err := st.GafferGoalFor(uid, cid)
	if err != nil || g.State != "paused" || g.Revision != 1 {
		t.Fatal("failed resume changed the goal", g, err)
	}
	if _, err := st.db.Exec(`DELETE FROM gaffer_goals WHERE conversation_id=?`, fmt.Sprintf("%032x", 1)); err != nil {
		t.Fatal(err)
	}
	g, err = st.ControlGafferGoal(uid, cid, 1, "resume", pins)
	if err != nil || g.State != "active" {
		t.Fatal("the authorised goal could not resume once room was available", g, err)
	}
}
