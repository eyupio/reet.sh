package store

import (
	"encoding/json"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func chatEventJSON(text string) []byte {
	data, _ := json.Marshal(terminalproto.ACPEvent{Kind: "user", Text: text})
	return data
}

func TestTerminalChatTranscriptIsBoundedAndOrdered(t *testing.T) {
	s, uid, _, _ := terminalFixture(t)
	const session = "session-0123456789"

	for i := int64(1); i <= 3; i++ {
		if err := s.AppendTerminalChatEventFor(uid, session, i, i, chatEventJSON("one")); err != nil {
			t.Fatal(err)
		}
	}
	// A retried sequence is idempotent.
	if err := s.AppendTerminalChatEventFor(uid, session, 3, 3, chatEventJSON("one")); err != nil {
		t.Fatal(err)
	}
	events, err := s.TerminalChatEventsFor(uid, session)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Seq != 1 || events[2].Seq != 3 {
		t.Fatalf("transcript: %+v", events)
	}

	if err := s.DeleteTerminalChatEventsFor(uid, session); err != nil {
		t.Fatal(err)
	}
	events, err = s.TerminalChatEventsFor(uid, session)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("transcript after delete: %+v", events)
	}
}

func TestTerminalChatTranscriptRefusesInvalidEvents(t *testing.T) {
	s, uid, _, _ := terminalFixture(t)
	const session = "session-0123456789"
	if err := s.AppendTerminalChatEventFor(uid, session, 1, 1, []byte("not json")); err == nil {
		t.Fatal("invalid event accepted")
	}
	if err := s.AppendTerminalChatEventFor(uid, session, 0, 1, chatEventJSON("one")); err == nil {
		t.Fatal("zero sequence accepted")
	}
	if err := s.AppendTerminalChatEventFor(uid, "short", 1, 1, chatEventJSON("one")); err == nil {
		t.Fatal("invalid session accepted")
	}
}
