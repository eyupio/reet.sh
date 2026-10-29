package terminalbroker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// modelsPeer is a connected peer that answers models requests with answer; a
// nil answer holds the request until the test delivers a late reply.
type modelsPeer struct {
	b      *Broker
	st     *store.Store
	uid    int64
	agent  string
	peer   *Peer
	mu     sync.Mutex
	asked  []terminalproto.Body
	answer func(terminalproto.Body) terminalproto.Body
	held   []terminalproto.Message
}

func modelsCapabilities() terminalproto.Capabilities {
	caps := launchCapabilities("opencode")
	caps.Models = true
	return caps
}

func modelsFixture(t *testing.T, caps terminalproto.Capabilities) *modelsPeer {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	a, _, err := st.EnrollSyncAgent(uid, "studio", "test")
	if err != nil {
		t.Fatal(err)
	}
	record, err := st.BeginRemoteConnectionFor(uid, a.ID, ID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	f := &modelsPeer{b: New(st), st: st, uid: uid, agent: a.ID}
	t.Cleanup(f.b.Close)
	var p *Peer
	p, err = f.b.Connect(uid, a.ID, record.Generation, true, func(ctx context.Context, m terminalproto.Message) error {
		reply := m
		switch op := m.Body.(type) {
		case terminalproto.Reconcile:
			reply.Body = terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
		case terminalproto.Models:
			f.mu.Lock()
			f.asked = append(f.asked, op)
			answer := f.answer
			f.mu.Unlock()
			if answer == nil {
				f.mu.Lock()
				f.held = append(f.held, reply)
				f.mu.Unlock()
				return nil
			}
			reply.Body = answer(op)
		default:
			reply.Body = terminalproto.Result{Code: terminalproto.OK}
		}
		data, err := terminalproto.Encode(reply)
		if err != nil {
			return err
		}
		decoded, err := terminalproto.Decode(data)
		if err != nil {
			return err
		}
		return p.Receive(decoded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Synchronize(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.peer = p
	return f
}

func (f *modelsPeer) answering(fn func(terminalproto.Body) terminalproto.Body) {
	f.mu.Lock()
	f.answer = fn
	f.mu.Unlock()
}

func (f *modelsPeer) sentRequests() []terminalproto.Body {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Body(nil), f.asked...)
}

// replyLate delivers a result for the oldest held request, as a reader does
// after the call has already given up.
func (f *modelsPeer) replyLate(t *testing.T, result terminalproto.ModelsResult) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.held) == 0 {
		t.Fatal("no held models request")
	}
	m := f.held[0]
	f.held = f.held[1:]
	m.Body = &result
	data, err := terminalproto.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := terminalproto.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.peer.Receive(decoded); err != nil {
		t.Fatal(err)
	}
}

func listedModels() terminalproto.ModelsResult {
	return terminalproto.ModelsResult{Tool: "opencode", Models: []terminalproto.ModelEntry{{ID: "anthropic/claude-sonnet-4-5", Provider: "anthropic"}}, Version: "1.19.5"}
}

func TestModelsIsNeverSentToAPeerWithoutTheCapability(t *testing.T) {
	for name, tc := range map[string]struct {
		caps terminalproto.Capabilities
		want error
	}{
		"old peer": {launchCapabilities("opencode"), ErrModelsUnsupported},
		"legacy peer": {
			legacyCapabilities(), ErrModelsUnsupported,
		},
		"models switched off": {
			func() terminalproto.Capabilities {
				c := modelsCapabilities()
				c.Models, c.ModelsOff = false, true
				return c
			}(),
			ErrModelsOff,
		},
	} {
		f := modelsFixture(t, tc.caps)
		f.answering(func(terminalproto.Body) terminalproto.Body { return listedModels() })
		_, err := f.b.Models(t.Context(), f.uid, f.agent, "opencode")
		if !errors.Is(err, tc.want) {
			t.Fatal(name, err)
		}
		if sent := f.sentRequests(); len(sent) != 0 {
			t.Fatal(name, "a models request reached a peer without the capability", sent)
		}
		select {
		case <-f.peer.Done():
			t.Fatal(name, "a refused models request ended the connection")
		default:
		}
	}
}

func TestModelsValidatesTheToolBeforeAnyPeerState(t *testing.T) {
	f := modelsFixture(t, modelsCapabilities())
	f.answering(func(terminalproto.Body) terminalproto.Body { return listedModels() })
	for _, tool := range []string{"", "pi", "OPENCODE", "../opencode"} {
		if _, err := f.b.Models(t.Context(), f.uid, f.agent, tool); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(tool, err)
		}
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatal("an invalid models request reached the machine", sent)
	}
}

func TestModelsSendsTheExactRequestAndReturnsOneAnswer(t *testing.T) {
	f := modelsFixture(t, modelsCapabilities())
	f.answering(func(terminalproto.Body) terminalproto.Body { return listedModels() })
	got, err := f.b.Models(t.Context(), f.uid, f.agent, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "anthropic/claude-sonnet-4-5" || got.Version != "1.19.5" {
		t.Fatal(got)
	}
	sent := f.sentRequests()
	if len(sent) != 1 {
		t.Fatal("one request", sent)
	}
	if sent[0] != (terminalproto.Models{Tool: "opencode"}) {
		t.Fatal("request changed", sent[0])
	}
}

func TestModelsReturnsARefusalReason(t *testing.T) {
	f := modelsFixture(t, modelsCapabilities())
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.ModelsResult{Tool: "opencode", Reason: terminalproto.ModelsUnsupported}
	})
	got, err := f.b.Models(t.Context(), f.uid, f.agent, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if got.Reason != terminalproto.ModelsUnsupported || len(got.Models) != 0 {
		t.Fatal(got)
	}
}

func TestModelsRejectsAnUnexpectedAnswer(t *testing.T) {
	f := modelsFixture(t, modelsCapabilities())
	for name, answer := range map[string]terminalproto.Body{
		"wrong kind":       terminalproto.Result{Code: terminalproto.OK},
		"wrong tool":       terminalproto.ModelsResult{Tool: "claude", Models: listedModels().Models},
		"bare models body": terminalproto.Models{Tool: "opencode"},
	} {
		f.answering(func(terminalproto.Body) terminalproto.Body { return answer })
		if _, err := f.b.Models(t.Context(), f.uid, f.agent, "opencode"); !errors.Is(err, ErrModelsAnswer) {
			t.Fatal(name, err)
		}
	}
}

func TestModelsTimesOutAndDropsALateResult(t *testing.T) {
	old := ModelsCallTimeout
	ModelsCallTimeout = 50 * time.Millisecond
	t.Cleanup(func() { ModelsCallTimeout = old })

	f := modelsFixture(t, modelsCapabilities())
	f.answering(nil)
	start := time.Now()
	if _, err := f.b.Models(t.Context(), f.uid, f.agent, "opencode"); !errors.Is(err, ErrMachineTimeout) {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatal("a held models request took", elapsed)
	}
	select {
	case <-f.peer.Done():
		t.Fatal("a timed-out models request ended the connection")
	default:
	}
	f.replyLate(t, listedModels())
	if got := f.b.LateReplies(); got != 1 {
		t.Fatal("late models result counted", got)
	}
	select {
	case <-f.peer.Done():
		t.Fatal("a late models result ended the connection")
	default:
	}
	// The connection still answers a later request.
	f.answering(func(terminalproto.Body) terminalproto.Body { return listedModels() })
	if _, err := f.b.Models(t.Context(), f.uid, f.agent, "opencode"); err != nil {
		t.Fatal("after a late result", err)
	}
}

func TestModelsRequiresTheCurrentCapabilityGeneration(t *testing.T) {
	f := modelsFixture(t, modelsCapabilities())
	f.answering(func(terminalproto.Body) terminalproto.Body { return listedModels() })
	if _, err := f.st.BeginRemoteConnectionFor(f.uid, f.agent, ID(), modelsCapabilities()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.Models(t.Context(), f.uid, f.agent, "opencode"); !errors.Is(err, ErrOffline) {
		t.Fatal(err)
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatal("a stale-generation request reached the machine", sent)
	}
}
