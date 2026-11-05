package store

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func chatStoreFixture(t *testing.T) (*Store, int64, GafferConversation) {
	t.Helper()
	s := newStore(t)
	u, err := s.CreateUser("buddy@example.test", "Buddy", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateGafferConversationFor(u.ID, GafferConversation{ID: strings.Repeat("c", 32), ProviderID: strings.Repeat("p", 32)})
	// Provider IDs use the same hexadecimal shape as controller-generated IDs.
	if err == nil {
		t.Fatal("invalid provider ID accepted")
	}
	c, err = s.CreateGafferConversationFor(u.ID, GafferConversation{ID: strings.Repeat("c", 32), ProviderID: strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal(err)
	}
	return s, u.ID, c
}
func TestGafferChatReceiptsAreOwnedDurableAndNeverReplayed(t *testing.T) {
	s, uid, c := chatStoreFixture(t)
	other, err := s.CreateUser("other-buddy@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	in := GafferTurn{ID: strings.Repeat("b", 32), ConversationID: c.ID, RequestRevision: c.Revision, Prompt: "How are we doing?", ProviderID: c.ProviderID, ProviderName: "My provider", Model: "model-a"}
	first, created, err := s.StartGafferTurnFor(uid, in)
	if err != nil || !created {
		t.Fatalf("admit: %v %v", created, err)
	}
	if _, _, err := s.StartGafferTurnFor(other.ID, in); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account write: %v", err)
	}
	if _, err := s.GafferTurnFor(other.ID, c.ID, in.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account read: %v", err)
	}
	again, created, err := s.StartGafferTurnFor(uid, in)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("duplicate: %v %v", created, err)
	}
	altered := in
	altered.Prompt = "Different instruction"
	if _, _, err := s.StartGafferTurnFor(uid, altered); !errors.Is(err, ErrGafferConflict) {
		t.Fatalf("reused ID: %v", err)
	}
	first.Answer = "Reet, partial progress."
	if err := s.UpdateGafferTurnFor(uid, first); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverGafferTurns(); err != nil {
		t.Fatal(err)
	}
	recovered, created, err := s.StartGafferTurnFor(uid, in)
	if err != nil || created || recovered.State != "interrupted" || recovered.Answer != first.Answer {
		t.Fatalf("recovery: %+v %v %v", recovered, created, err)
	}
	first.State = "completed"
	if err := s.UpdateGafferTurnFor(uid, first); !errors.Is(err, ErrGafferConflict) {
		t.Fatalf("late completion: %v", err)
	}
	if err := s.DeleteUser(uid); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM gaffer_turns WHERE conversation_id=?`, c.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("account cascade %d %v", n, err)
	}
}
func TestGafferConversationScopeAndConcurrentAdmission(t *testing.T) {
	s, uid, c := chatStoreFixture(t)
	existing, err := s.CreateGafferConversationFor(uid, GafferConversation{ID: strings.Repeat("d", 32), ProviderID: strings.Repeat("e", 32)})
	if err != nil || existing.ID != c.ID || existing.ProviderID != c.ProviderID {
		t.Fatalf("scope retry changed binding: %+v %v", existing, err)
	}
	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	admitted := make(chan bool, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_, created, err := s.StartGafferTurnFor(uid, GafferTurn{ID: strings.Repeat(string(rune('0'+i)), 32), ConversationID: c.ID, RequestRevision: 1, ProviderID: c.ProviderID, Prompt: "Hello", Model: "a"})
			admitted <- created && err == nil
		}(i)
	}
	wg.Wait()
	close(admitted)
	count := 0
	for ok := range admitted {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("concurrent admissions %d", count)
	}
	current, _ := s.GafferConversationFor(uid, c.ID)
	if err := s.ConfigureGafferConversationFor(uid, c.ID, current.Revision, c.ProviderID, "other-model"); !errors.Is(err, ErrGafferChatBusy) {
		t.Fatalf("reconfigured active turn: %v", err)
	}
	if err := s.DeleteGafferConversationFor(uid, c.ID, 1); !errors.Is(err, ErrGafferConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if err := s.DeleteGafferConversationFor(uid, c.ID, current.Revision); err != nil {
		t.Fatal(err)
	}
}
func TestGafferChatRetentionRejectsNewTurnsWithoutDroppingReceipts(t *testing.T) {
	s, uid, c := chatStoreFixture(t)
	for i := 0; i < MaxGafferTurns; i++ {
		id := strings.Repeat("0", 30) + string("0123456789abcdef"[i/16]) + string("0123456789abcdef"[i%16])
		turn, created, err := s.StartGafferTurnFor(uid, GafferTurn{ID: id, ConversationID: c.ID, RequestRevision: c.Revision, ProviderID: c.ProviderID, Prompt: "hello", Model: "a"})
		if err != nil || !created {
			t.Fatalf("turn %d: %v", i, err)
		}
		turn.State = "completed"
		turn.Answer = "hello back"
		if err = s.UpdateGafferTurnFor(uid, turn); err != nil {
			t.Fatal(err)
		}
		c, _ = s.GafferConversationFor(uid, c.ID)
	}
	if _, _, err := s.StartGafferTurnFor(uid, GafferTurn{ID: strings.Repeat("f", 32), ConversationID: c.ID, RequestRevision: c.Revision, ProviderID: c.ProviderID, Prompt: "overflow", Model: "a"}); !errors.Is(err, ErrGafferChatFull) {
		t.Fatalf("retention limit: %v", err)
	}
	turns, err := s.GafferTurnsFor(uid, c.ID)
	if err != nil || len(turns) != MaxGafferTurns {
		t.Fatalf("receipts trimmed: %d %v", len(turns), err)
	}
}

func TestGafferPartialTurnSurvivesDatabaseReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	user, err := s.CreateUser("reopen@example.test", "Person", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateGafferConversationFor(user.ID, GafferConversation{ID: strings.Repeat("a", 32), ProviderID: strings.Repeat("b", 32)})
	if err != nil {
		t.Fatal(err)
	}
	request := GafferTurn{ID: strings.Repeat("c", 32), ConversationID: c.ID, RequestRevision: 1, ProviderID: c.ProviderID, Prompt: "one request", Model: "fake"}
	turn, _, err := s.StartGafferTurnFor(user.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	turn.Answer = "partial before shutdown"
	if err = s.UpdateGafferTurnFor(user.ID, turn); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverGafferTurns(); err != nil {
		t.Fatal(err)
	}
	old, created, err := s.StartGafferTurnFor(user.ID, request)
	if err != nil || created || old.State != "interrupted" || old.Answer != turn.Answer {
		t.Fatalf("reopen: %+v %v %v", old, created, err)
	}
}
