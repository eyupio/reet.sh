package terminalbroker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestCommandHistoryNeverDispatchesNewOperationToLegacyAgent(t *testing.T) {
	b, p, uid, agent := fixture(t)
	session, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Legacy", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	var dispatched atomic.Int64
	original := p.send
	p.send = func(ctx context.Context, m terminalproto.Message) error {
		if _, ok := m.Body.(terminalproto.CommandHistory); ok {
			dispatched.Add(1)
		}
		return original(ctx, m)
	}
	if _, err = b.CommandHistory(t.Context(), uid, session.ID); !errors.Is(err, ErrCommandHistoryUnsupported) {
		t.Fatal(err)
	}
	if dispatched.Load() != 0 || !b.Ready(uid, agent) {
		t.Fatal("legacy connection was changed", dispatched.Load())
	}
}
