package softwarerun

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func softwareRunFixture(t *testing.T, hosts ...string) (*store.Store, int64, []store.SyncAgent) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	agents := make([]store.SyncAgent, 0, len(hosts))
	for _, host := range hosts {
		a, _, err := st.EnrollSyncAgent(uid, host, "test")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), softwareRunCaps("1.18.0")); err != nil {
			t.Fatal(err)
		}
		agents = append(agents, a)
	}
	return st, uid, agents
}

func softwareRunCaps(version string) terminalproto.Capabilities {
	return terminalproto.Capabilities{
		ProtocolVersion: terminalproto.Version, OS: "linux", OSUser: "tester",
		TerminalEnabled: true, TerminalControl: true, HealthEnabled: true,
		Readiness: terminalproto.Ready, SoftwareApply: true,
		Software: []terminalproto.SoftwareEntry{{
			Tool: "opencode", State: terminalproto.SoftwareInstalled, Version: version,
			Manager: "reet", Package: "opencode-ai", Source: "verified_tarball",
			Platform: "linux", Arch: "amd64", Managed: true, CheckedAt: 1,
			Path:       "/home/tester/.reet/software/opencode/opencode",
			Operations: []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanUpdate, terminalproto.SoftwarePlanRemove},
		}},
	}
}

func softwareRunBatch(t *testing.T, st *store.Store, uid int64, agents ...store.SyncAgent) store.SoftwareBatch {
	t.Helper()
	members := make([]store.SoftwareBatchMemberInput, 0, len(agents))
	for i, a := range agents {
		members = append(members, store.SoftwareBatchMemberInput{
			Position: i, AgentID: a.ID, Host: a.Host, Action: "update", Target: "1.19.5",
			ExpectedState: "installed", ExpectedVersion: "1.18.0", ExpectedManaged: true,
		})
	}
	b, _, err := st.CreateSoftwareBatchFor(uid, "", "opencode", members)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type fakeBroker struct {
	mu    sync.Mutex
	calls []string
	apply func(agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error)
}

func (b *fakeBroker) SoftwareApply(_ context.Context, _ int64, agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error) {
	b.mu.Lock()
	b.calls = append(b.calls, agentID)
	apply := b.apply
	b.mu.Unlock()
	return apply(agentID, in)
}

func (b *fakeBroker) called() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string{}, b.calls...)
}

func applied(in terminalproto.SoftwareApply) terminalproto.SoftwareApplyResult {
	return terminalproto.SoftwareApplyResult{Tool: in.Tool, Action: in.Action, State: terminalproto.SoftwareApplyApplied, Version: "1.19.5"}
}

func refused(in terminalproto.SoftwareApply, reason terminalproto.SoftwareApplyReason) terminalproto.SoftwareApplyResult {
	return terminalproto.SoftwareApplyResult{Tool: in.Tool, Action: in.Action, State: terminalproto.SoftwareApplyRefused, Reason: reason}
}

func failed(in terminalproto.SoftwareApply, reason terminalproto.SoftwareApplyReason) terminalproto.SoftwareApplyResult {
	return terminalproto.SoftwareApplyResult{Tool: in.Tool, Action: in.Action, State: terminalproto.SoftwareApplyFailed, Reason: reason}
}

func waitBatch(t *testing.T, st *store.Store, uid int64, batchID string, want ...string) store.SoftwareBatch {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last store.SoftwareBatch
	for time.Now().Before(deadline) {
		b, err := st.SoftwareBatchFor(uid, batchID)
		if err != nil {
			t.Fatal(err)
		}
		last = b
		for _, state := range want {
			if b.State == state {
				return b
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("batch stayed %s, wanted %v: %+v", last.State, want, last)
	return last
}

func TestRunAppliesInOrderAndStopsOnFailure(t *testing.T) {
	st, uid, agents := softwareRunFixture(t, "alpha", "beta", "gamma")
	batch := softwareRunBatch(t, st, uid, agents...)
	broker := &fakeBroker{apply: func(agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error) {
		if agentID == agents[1].ID {
			return failed(in, terminalproto.SoftwareApplyInstall), nil
		}
		return applied(in), nil
	}}
	o := New(Config{Store: st, Broker: broker, Logf: t.Logf})
	defer o.Close()
	o.Start(uid, batch.ID)

	stopped := waitBatch(t, st, uid, batch.ID, store.SoftwareBatchStopped)
	if stopped.Members[0].State != store.SoftwareMemberApplied {
		t.Fatalf("first member: %+v", stopped.Members[0])
	}
	if stopped.Members[1].State != store.SoftwareMemberFailed || stopped.Members[1].Code != "install_failed" {
		t.Fatalf("second member: %+v", stopped.Members[1])
	}
	if stopped.Members[2].State != store.SoftwareMemberPending {
		t.Fatalf("later machines must wait: %+v", stopped.Members[2])
	}
	if calls := broker.called(); len(calls) != 2 || calls[0] != agents[0].ID || calls[1] != agents[1].ID {
		t.Fatalf("calls: %v", calls)
	}

	// Starting a stopped batch again must not continue past its failure.
	o.Start(uid, batch.ID)
	time.Sleep(50 * time.Millisecond)
	still := waitBatch(t, st, uid, batch.ID, store.SoftwareBatchStopped)
	if still.Members[2].State != store.SoftwareMemberPending {
		t.Fatalf("a stopped batch resumed itself: %+v", still.Members[2])
	}
	if calls := broker.called(); len(calls) != 2 {
		t.Fatalf("a stopped batch applied more machines: %v", calls)
	}
}

func TestRetryThenContinue(t *testing.T) {
	st, uid, agents := softwareRunFixture(t, "alpha", "beta")
	batch := softwareRunBatch(t, st, uid, agents...)
	var mu sync.Mutex
	failedOnce := false
	broker := &fakeBroker{apply: func(agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error) {
		mu.Lock()
		defer mu.Unlock()
		if agentID == agents[0].ID && !failedOnce {
			failedOnce = true
			return failed(in, terminalproto.SoftwareApplyDownload), nil
		}
		return applied(in), nil
	}}
	o := New(Config{Store: st, Broker: broker, Logf: t.Logf})
	defer o.Close()
	o.Start(uid, batch.ID)
	waitBatch(t, st, uid, batch.ID, store.SoftwareBatchStopped)

	if _, err := o.Retry(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	done := waitBatch(t, st, uid, batch.ID, store.SoftwareBatchDone)
	if done.Members[0].State != store.SoftwareMemberApplied || done.Members[1].State != store.SoftwareMemberApplied {
		t.Fatalf("done: %+v", done)
	}
}

func TestSkipContinuesAndFinishes(t *testing.T) {
	st, uid, agents := softwareRunFixture(t, "alpha", "beta")
	batch := softwareRunBatch(t, st, uid, agents...)
	broker := &fakeBroker{apply: func(agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error) {
		if agentID == agents[0].ID {
			return failed(in, terminalproto.SoftwareApplyChecksum), nil
		}
		return applied(in), nil
	}}
	o := New(Config{Store: st, Broker: broker, Logf: t.Logf})
	defer o.Close()
	o.Start(uid, batch.ID)
	waitBatch(t, st, uid, batch.ID, store.SoftwareBatchStopped)

	if err := o.Skip(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	done := waitBatch(t, st, uid, batch.ID, store.SoftwareBatchDone)
	if done.Members[0].State != store.SoftwareMemberSkipped || done.Members[1].State != store.SoftwareMemberApplied {
		t.Fatalf("done: %+v", done)
	}
}

func TestOfflineStopsWithCode(t *testing.T) {
	st, uid, agents := softwareRunFixture(t, "alpha", "beta")
	batch := softwareRunBatch(t, st, uid, agents...)
	broker := &fakeBroker{apply: func(agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error) {
		return terminalproto.SoftwareApplyResult{}, terminalbroker.ErrOffline
	}}
	o := New(Config{Store: st, Broker: broker, Logf: t.Logf})
	defer o.Close()
	o.Start(uid, batch.ID)
	stopped := waitBatch(t, st, uid, batch.ID, store.SoftwareBatchStopped)
	if stopped.Members[0].Code != CodeOffline {
		t.Fatalf("offline code: %+v", stopped.Members[0])
	}
}

func TestRefusedButAlreadyAtTargetIsApplied(t *testing.T) {
	st, uid, agents := softwareRunFixture(t, "alpha")
	batch := softwareRunBatch(t, st, uid, agents...)
	broker := &fakeBroker{apply: func(agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error) {
		// The machine reached the target but the reply was lost; the retry
		// sees it already installed and refuses as changed.
		if _, err := st.BeginRemoteConnectionFor(uid, agentID, terminalbroker.ID(), softwareRunCaps("1.19.5")); err != nil {
			t.Errorf("update evidence: %v", err)
		}
		return refused(in, terminalproto.SoftwareApplyChanged), nil
	}}
	o := New(Config{Store: st, Broker: broker, Logf: t.Logf})
	defer o.Close()
	o.Start(uid, batch.ID)
	done := waitBatch(t, st, uid, batch.ID, store.SoftwareBatchDone)
	if done.Members[0].State != store.SoftwareMemberApplied || done.Members[0].Version != "1.19.5" {
		t.Fatalf("reconciled member: %+v", done.Members[0])
	}
}

func TestResumeResetsRunningMember(t *testing.T) {
	st, uid, agents := softwareRunFixture(t, "alpha")
	batch := softwareRunBatch(t, st, uid, agents...)
	st.SoftwareBatchMemberStart(uid, batch.ID, 0) // as if a controller stopped mid-apply

	broker := &fakeBroker{apply: func(agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error) {
		return applied(in), nil
	}}
	o := New(Config{Store: st, Broker: broker, Logf: t.Logf})
	defer o.Close()
	if err := o.Resume(); err != nil {
		t.Fatal(err)
	}
	done := waitBatch(t, st, uid, batch.ID, store.SoftwareBatchDone)
	if done.Members[0].State != store.SoftwareMemberApplied {
		t.Fatalf("resumed member: %+v", done.Members[0])
	}
}

func TestStartTwiceDoesNotDoubleApply(t *testing.T) {
	st, uid, agents := softwareRunFixture(t, "alpha")
	batch := softwareRunBatch(t, st, uid, agents...)
	var mu sync.Mutex
	calls := 0
	broker := &fakeBroker{apply: func(agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		return applied(in), nil
	}}
	o := New(Config{Store: st, Broker: broker, Logf: t.Logf})
	defer o.Close()
	o.Start(uid, batch.ID)
	o.Start(uid, batch.ID)
	waitBatch(t, st, uid, batch.ID, store.SoftwareBatchDone)
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("applied %d times, want 1", calls)
	}
}

func TestAbandonBetweenMachines(t *testing.T) {
	st, uid, agents := softwareRunFixture(t, "alpha", "beta")
	batch := softwareRunBatch(t, st, uid, agents...)
	// Nothing has started, so abandoning skips both pending machines.
	if _, err := st.AbandonSoftwareBatch(uid, batch.ID); err != nil {
		t.Fatal(err)
	}
	abandoned, _ := st.SoftwareBatchFor(uid, batch.ID)
	if abandoned.State != store.SoftwareBatchAbandoned {
		t.Fatalf("state: %s", abandoned.State)
	}
	for _, m := range abandoned.Members {
		if m.State != store.SoftwareMemberSkipped {
			t.Fatalf("member: %+v", m)
		}
	}
	// A failed member stays failed for the record.
	second := softwareRunBatch(t, st, uid, agents...)
	st.SoftwareBatchMemberStart(uid, second.ID, 0)
	st.SetSoftwareBatchMemberState(uid, second.ID, 0, store.SoftwareMemberFailed, "offline", "")
	st.AbandonSoftwareBatch(uid, second.ID)
	after, _ := st.SoftwareBatchFor(uid, second.ID)
	if after.Members[0].State != store.SoftwareMemberFailed || after.Members[1].State != store.SoftwareMemberSkipped {
		t.Fatalf("abandon records: %+v", after.Members)
	}
}

func TestRetryRejectsFinishedBatch(t *testing.T) {
	st, uid, agents := softwareRunFixture(t, "alpha")
	batch := softwareRunBatch(t, st, uid, agents...)
	if _, err := st.AbandonSoftwareBatch(uid, batch.ID); err != nil {
		t.Fatal(err)
	}
	o := New(Config{Store: st, Broker: &fakeBroker{apply: func(string, terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error) {
		return terminalproto.SoftwareApplyResult{}, errors.New("no")
	}}, Logf: t.Logf})
	defer o.Close()
	if _, err := o.Retry(uid, batch.ID, 0); !errors.Is(err, store.ErrSoftwareBatchFinished) {
		t.Fatalf("retry on an abandoned batch: %v", err)
	}
}
