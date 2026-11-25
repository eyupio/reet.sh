package setuprun

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func fixture(t *testing.T, hosts ...string) (*store.Store, int64, store.SetupBatch) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	var members []store.SetupBatchMemberInput
	for i, host := range hosts {
		a, _, err := st.EnrollSyncAgent(uid, host, "test")
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, store.SetupBatchMemberInput{Position: i, AgentID: a.ID, Host: host, Fingerprint: "fp-" + host})
	}
	item := terminalproto.SetupItem{Agent: "claude", Scope: "global", Kind: "instructions", Name: "CLAUDE.md", Format: "text", Content: "x\n"}
	item.SHA256 = terminalproto.SetupItemSHA256(item.Content)
	items, _ := json.Marshal([]terminalproto.SetupItem{item})
	b, _, err := st.CreateSetupBatchFor(uid, "", store.SetupBatchSource{AgentID: "src", Host: "source"}, items, members)
	if err != nil {
		t.Fatal(err)
	}
	return st, uid, b
}

type fakeBroker struct {
	mu    sync.Mutex
	calls []terminalproto.SetupFleet
	hosts []string
	reply func(host string) (terminalproto.SetupFleetResult, error)
	host  map[string]string
}

func (b *fakeBroker) SetupFleet(_ context.Context, _ int64, agentID string, in terminalproto.SetupFleet) (terminalproto.SetupFleetResult, error) {
	b.mu.Lock()
	b.calls = append(b.calls, in)
	host := b.host[agentID]
	b.hosts = append(b.hosts, host)
	reply := b.reply
	b.mu.Unlock()
	return reply(host)
}

func newBroker(batch store.SetupBatch, reply func(host string) (terminalproto.SetupFleetResult, error)) *fakeBroker {
	b := &fakeBroker{reply: reply, host: map[string]string{}}
	for _, m := range batch.Members {
		b.host[m.AgentID] = m.Host
	}
	return b
}

func applied(string) (terminalproto.SetupFleetResult, error) {
	return terminalproto.SetupFleetResult{Op: terminalproto.SetupOpApply, State: terminalproto.SetupOK, Journal: "restore-fleet-1"}, nil
}

func waitState(t *testing.T, st *store.Store, uid int64, id string, want string) store.SetupBatch {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := st.SetupBatchFor(uid, id)
		if err != nil {
			t.Fatal(err)
		}
		if b.State == want {
			return b
		}
		time.Sleep(5 * time.Millisecond)
	}
	b, _ := st.SetupBatchFor(uid, id)
	t.Fatalf("batch %s, want %s: %+v", b.State, want, b)
	return b
}

func TestSetupBatchAppliesInOrderWithTheReviewedPlan(t *testing.T) {
	st, uid, batch := fixture(t, "one", "two", "three")
	broker := newBroker(batch, applied)
	settled := make(chan store.SetupBatch, 4)
	o := New(Config{Store: st, Broker: broker, Settled: func(_ int64, b store.SetupBatch) { settled <- b }})
	defer o.Close()
	o.Start(uid, batch.ID)
	done := waitState(t, st, uid, batch.ID, store.SoftwareBatchDone)
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if len(broker.hosts) != 3 || broker.hosts[0] != "one" || broker.hosts[1] != "two" || broker.hosts[2] != "three" {
		t.Fatalf("order: %v", broker.hosts)
	}
	for i, call := range broker.calls {
		if call.Op != terminalproto.SetupOpApply || call.Fingerprint != "fp-"+broker.hosts[i] || call.Label != "fleet-"+batch.ID || len(call.Items) != 1 {
			t.Fatalf("call %d: %+v", i, call)
		}
	}
	for _, m := range done.Members {
		if m.State != store.SoftwareMemberApplied || m.Journal != "restore-fleet-1" {
			t.Fatalf("member: %+v", m)
		}
	}
	if done.ContentHeld {
		t.Fatal("a done batch still holds the copied text")
	}
	select {
	case <-settled:
	case <-time.After(time.Second):
		t.Fatal("not audited")
	}
}

func TestSetupBatchStopsAtTheFirstFailureAndSkipsOffline(t *testing.T) {
	st, uid, batch := fixture(t, "one", "two", "three")
	broker := newBroker(batch, func(host string) (terminalproto.SetupFleetResult, error) {
		switch host {
		case "one":
			return terminalproto.SetupFleetResult{}, terminalbroker.ErrNotSent
		case "two":
			return terminalproto.SetupFleetResult{Op: terminalproto.SetupOpApply, State: terminalproto.SetupRefused, Reason: terminalproto.SetupReasonChanged}, nil
		}
		return applied(host)
	})
	o := New(Config{Store: st, Broker: broker})
	defer o.Close()
	o.Start(uid, batch.ID)
	stopped := waitState(t, st, uid, batch.ID, store.SoftwareBatchStopped)
	got := []string{}
	for _, m := range stopped.Members {
		got = append(got, m.State+"/"+m.Code)
	}
	if got[0] != "skipped/offline" || got[1] != "failed/changed" || got[2] != "pending/" {
		t.Fatalf("members: %v", got)
	}
	if !stopped.ContentHeld {
		t.Fatal("a stopped batch lost its text before it settled")
	}
	if err := o.Skip(uid, batch.ID, 1); err != nil {
		t.Fatal(err)
	}
	done := waitState(t, st, uid, batch.ID, store.SoftwareBatchDone)
	if done.Members[2].State != store.SoftwareMemberApplied {
		t.Fatalf("after skip: %+v", done.Members[2])
	}
}

func TestSetupBatchResumesAMemberLeftRunning(t *testing.T) {
	st, uid, batch := fixture(t, "one", "two")
	if _, err := st.SetupBatchMemberStart(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	broker := newBroker(batch, applied)
	o := New(Config{Store: st, Broker: broker})
	defer o.Close()
	if err := o.Resume(); err != nil {
		t.Fatal(err)
	}
	waitState(t, st, uid, batch.ID, store.SoftwareBatchDone)
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if len(broker.hosts) != 2 || broker.hosts[0] != "one" {
		t.Fatalf("resumed calls: %v", broker.hosts)
	}
}

func TestSetupBatchWithoutItsTextFailsTheMember(t *testing.T) {
	st, uid, batch := fixture(t, "one")
	clock := time.Now().Add(25 * time.Hour)
	broker := newBroker(batch, applied)
	o := New(Config{Store: st, Broker: broker, Now: func() time.Time { return clock }})
	defer o.Close()
	o.Purge()
	got, _ := st.SetupBatchFor(uid, batch.ID)
	if got.State != store.SoftwareBatchAbandoned || got.ContentHeld || got.Members[0].Code != "expired" {
		t.Fatalf("after a day: %+v", got)
	}
	o.Start(uid, batch.ID)
	time.Sleep(50 * time.Millisecond)
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if len(broker.calls) != 0 {
		t.Fatal("an expired batch reached a machine")
	}
}
