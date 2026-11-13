//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type modelRecoveryChat struct {
	*fakeChat
	model string
	apply string
	err   error
}

func (f *modelRecoveryChat) Model() string { return f.model }
func (f *modelRecoveryChat) SetModel(context.Context, string) error {
	f.model = f.apply
	return f.err
}

func TestModelReadbackUpdatesChatRestartRecord(t *testing.T) {
	for _, tc := range []struct {
		name, applied string
		err           error
	}{
		{"confirmed", "provider/new", nil},
		{"unconfirmed", "", acpagent.ErrModelUnconfirmed},
		{"different model", "provider/default", acpagent.ErrModel},
		{"refused", "provider/old", errors.New("invalid parameters")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, fake, _ := testChatHost(t)
			t.Cleanup(h.sup.Close)
			path := filepath.Join(t.TempDir(), "structured-chats.json")
			h.sup.registry = newChatRegistry(path)
			proc := &modelRecoveryChat{fakeChat: fake, model: "provider/old", apply: tc.applied, err: tc.err}
			h.sup.start = func(_ context.Context, opts acpagent.Options) (chatProcess, error) {
				fake.onEvent = opts.OnEvent
				return proc, nil
			}
			const id = "session-model0123456789"
			if _, err := h.open("request-model0123456789", id, terminalproto.Create{
				Title: "Chat", Tool: "opencode", Structured: true, Trust: true, Dir: h.sup.opts.Home, Model: "provider/old",
			}, true); err != nil {
				t.Fatal(err)
			}
			_ = h.setModel(t.Context(), id, "provider/new")
			records := newChatRegistry(path).snapshot()
			if len(records) != 1 || records[0].Model != tc.applied {
				t.Fatalf("restart record did not retain read-back %q: %+v", tc.applied, records)
			}
			if cs := h.find(id); cs.model != tc.applied {
				t.Fatalf("live recovery model=%q", cs.model)
			}
		})
	}
}
