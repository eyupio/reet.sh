package agenthealth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type fakeSummaryStore struct {
	mu     sync.Mutex
	saves  []savedSummary
	prunes int
	err    error
}

type savedSummary struct {
	uid int64
	in  store.RemoteHealthMinute
}

func (f *fakeSummaryStore) SaveRemoteHealthMinuteFor(uid int64, in store.RemoteHealthMinute) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.saves = append(f.saves, savedSummary{uid, in})
	return nil
}

func (f *fakeSummaryStore) PruneRemoteHealth() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prunes++
	return nil
}

func (f *fakeSummaryStore) pruneCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prunes
}

func (f *fakeSummaryStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.saves)
}

func (f *fakeSummaryStore) last() savedSummary {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saves[len(f.saves)-1]
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 23, 12, 0, 10, 0, time.UTC)}
}
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type sentMessage struct {
	message terminalproto.Message
	err     error
}

func testHub(t *testing.T) (*Hub, *fakeSummaryStore, *fakeClock, chan sentMessage) {
	t.Helper()
	summary := &fakeSummaryStore{}
	clock := newFakeClock()
	hub := NewHub(HubConfig{
		Store: summary, Now: clock.Now, ProbeInterval: time.Hour, ProbeTimeout: time.Second,
		PruneInterval: time.Hour,
	})
	t.Cleanup(hub.Close)
	sent := make(chan sentMessage, 16)
	return hub, summary, clock, sent
}

func testSender(sent chan sentMessage) Sender {
	return func(ctx context.Context, message terminalproto.Message) error {
		select {
		case sent <- sentMessage{message: message}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func testCaps() terminalproto.Capabilities {
	return terminalproto.Capabilities{
		ProtocolVersion: terminalproto.Version, OS: "linux", OSUser: "tester",
		HealthEnabled: true, TerminalEnabled: false, Readiness: terminalproto.DisabledLocally,
	}
}

func nextProbe(t *testing.T, sent chan sentMessage) (terminalproto.Probe, string) {
	t.Helper()
	select {
	case got := <-sent:
		probe, ok := got.message.Body.(terminalproto.Probe)
		if !ok {
			t.Fatalf("message body = %T", got.message.Body)
		}
		return probe, got.message.RequestID
	case <-time.After(2 * time.Second):
		t.Fatal("no probe was sent")
		return terminalproto.Probe{}, ""
	}
}

func healthReport(sequence int64) terminalproto.HealthReport {
	out := testReport(float64(sequence), true)
	out.Sequence = sequence
	return out
}

func TestHubConnectsSendsProbeAndMeasuresRTT(t *testing.T) {
	hub, _, clock, sent := testHub(t)
	conn, err := hub.Connect(1, "agent0123456789abcdef", 1, testInstance(), testCaps(), testSender(sent))
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := nextProbe(t, sent)
	clock.Advance(42 * time.Millisecond)
	conn.ProbeResult(terminalproto.ProbeResult{Sequence: probe.Sequence, Nonce: probe.Nonce}, clock.Now())
	status, ok := hub.Status(1, "agent0123456789abcdef")
	if !ok || !status.Connected || status.RTTMillis == nil || *status.RTTMillis != 42 {
		t.Fatalf("status: %+v", status)
	}
	if status.RTTP50Millis == nil || *status.RTTP50Millis != 42 || status.ReportFresh {
		t.Fatalf("rtt trend: %+v", status)
	}
	if status.Path != "unknown" || status.Generation != 1 || status.Capabilities.Readiness != terminalproto.DisabledLocally {
		t.Fatalf("status fields: %+v", status)
	}
}

func TestHubIgnoresDuplicateAndOutOfOrderReports(t *testing.T) {
	hub, summary, clock, sent := testHub(t)
	conn, err := hub.Connect(7, "agent0123456789abcdef", 1, testInstance(), testCaps(), testSender(sent))
	if err != nil {
		t.Fatal(err)
	}
	nextProbe(t, sent)
	conn.Report(healthReport(2), clock.Now())
	saves := summary.count()
	conn.Report(healthReport(2), clock.Now())
	conn.Report(healthReport(1), clock.Now())
	if summary.count() != saves {
		t.Fatal("duplicate/out-of-order report was accepted")
	}
	foreign := healthReport(3)
	foreign.AgentInstanceID = "different0123456789abc"
	conn.Report(foreign, clock.Now())
	if summary.count() != saves {
		t.Fatal("foreign instance report was accepted")
	}
	conn.Report(healthReport(3), clock.Now())
	if summary.count() != saves+1 {
		t.Fatal("ordered report was not accepted")
	}
	last := summary.last()
	if last.uid != 7 || last.in.Generation != 1 || last.in.Summary.CPUPercent.Valid != 2 {
		t.Fatalf("stored summary: %+v", last)
	}
}

func TestHubRollsMinutesAndReconnects(t *testing.T) {
	hub, summary, clock, sent := testHub(t)
	first, err := hub.Connect(1, "agent0123456789abcdef", 1, testInstance(), testCaps(), testSender(sent))
	if err != nil {
		t.Fatal(err)
	}
	nextProbe(t, sent)
	first.Report(healthReport(1), clock.Now())
	firstMinute := summary.last().in.Minute
	clock.Advance(time.Minute)
	first.Report(healthReport(2), clock.Now())
	if summary.last().in.Minute == firstMinute || summary.last().in.Revision != 1 {
		t.Fatalf("minute rollover: %+v", summary.last().in)
	}
	second, err := hub.Connect(1, "agent0123456789abcdef", 2, testInstance(), testCaps(), testSender(sent))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Fatal("previous generation was not fenced")
	}
	nextProbe(t, sent)
	status, ok := hub.Status(1, "agent0123456789abcdef")
	if !ok || !status.Connected || status.Reconnects != 1 || status.Generation != 2 {
		t.Fatalf("reconnect status: %+v", status)
	}
	second.Report(healthReport(3), clock.Now())
	last := summary.last()
	if last.in.Generation != 2 || last.in.Summary.Reconnects != 1 {
		t.Fatalf("reconnect summary: %+v", last.in.Summary)
	}
}

func TestHubMarksUnreachableAfterThreeMissedProbes(t *testing.T) {
	hub, summary, clock, sent := testHub(t)
	conn, err := hub.Connect(1, "agent0123456789abcdef", 1, testInstance(), testCaps(), testSender(sent))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxConsecutive; i++ {
		nextProbe(t, sent)
		clock.Advance(2 * time.Second)
		conn.expireProbes()
		if i < maxConsecutive-1 {
			conn.sendProbe()
		}
	}
	select {
	case <-conn.Done():
	case <-time.After(time.Second):
		t.Fatal("unreachable connection was not closed")
	}
	if summary.count() != maxConsecutive {
		t.Fatalf("missed probe summaries = %d", summary.count())
	}
	if last := summary.last().in.Summary; last.MissedProbes != maxConsecutive {
		t.Fatalf("missed probe count: %+v", last)
	}
	status, ok := hub.Status(1, "agent0123456789abcdef")
	if !ok || !status.LastKnown || !status.Unreachable || status.Connected {
		t.Fatalf("last known status: %+v", status)
	}
}

func TestHubClosesOnRevocationAndRetainsLastKnown(t *testing.T) {
	hub, summary, clock, sent := testHub(t)
	conn, err := hub.Connect(1, "agent0123456789abcdef", 1, testInstance(), testCaps(), testSender(sent))
	if err != nil {
		t.Fatal(err)
	}
	nextProbe(t, sent)
	conn.Report(healthReport(1), clock.Now())
	summary.mu.Lock()
	summary.err = store.ErrRemoteNotFound
	summary.mu.Unlock()
	conn.Report(healthReport(2), clock.Now())
	select {
	case <-conn.Done():
	case <-time.After(time.Second):
		t.Fatal("revoked connection was not closed")
	}
	status, ok := hub.Status(1, "agent0123456789abcdef")
	if !ok || !status.LastKnown || status.ContactAt == 0 {
		t.Fatalf("retained status: %+v", status)
	}
}

func TestHubFreshnessUsesControllerReceiptTime(t *testing.T) {
	hub, _, clock, sent := testHub(t)
	conn, err := hub.Connect(1, "agent0123456789abcdef", 1, testInstance(), testCaps(), testSender(sent))
	if err != nil {
		t.Fatal(err)
	}
	nextProbe(t, sent)
	conn.Report(healthReport(1), clock.Now())
	status, _ := hub.Status(1, "agent0123456789abcdef")
	if !status.ReportFresh {
		t.Fatal("fresh report was marked stale")
	}
	clock.Advance(freshReportWindow + time.Second)
	status, _ = hub.Status(1, "agent0123456789abcdef")
	if status.ReportFresh {
		t.Fatal("old report was marked fresh")
	}
}

func TestHubIsolatesAccountsWithTheSameAgentID(t *testing.T) {
	hub, _, _, sent := testHub(t)
	if _, err := hub.Connect(1, "shared0123456789abcdef", 1, testInstance(), testCaps(), testSender(sent)); err != nil {
		t.Fatal(err)
	}
	if _, ok := hub.Status(2, "shared0123456789abcdef"); ok {
		t.Fatal("another account saw the connection")
	}
	if _, err := hub.Connect(2, "shared0123456789abcdef", 1, testInstance(), testCaps(), testSender(sent)); err != nil {
		t.Fatal(err)
	}
	if status, ok := hub.Status(1, "shared0123456789abcdef"); !ok || !status.Connected {
		t.Fatal("first account's connection was replaced by another account")
	}
}

func TestHubCloseStopsConnections(t *testing.T) {
	hub, _, _, sent := testHub(t)
	conn, err := hub.Connect(1, "agent0123456789abcdef", 1, testInstance(), testCaps(), testSender(sent))
	if err != nil {
		t.Fatal(err)
	}
	nextProbe(t, sent)
	hub.Close()
	select {
	case <-conn.Done():
	case <-time.After(time.Second):
		t.Fatal("hub close left the connection running")
	}
	if _, err := hub.Connect(1, "agent0123456789abcdef", 2, testInstance(), testCaps(), testSender(sent)); err == nil {
		t.Fatal("closed hub accepted a connection")
	}
}

func TestHubPersistenceFailureIsNotReportedAsFreshSuccess(t *testing.T) {
	hub, summary, clock, sent := testHub(t)
	summary.err = errors.New("database unavailable")
	conn, err := hub.Connect(1, "agent0123456789abcdef", 1, testInstance(), testCaps(), testSender(sent))
	if err != nil {
		t.Fatal(err)
	}
	nextProbe(t, sent)
	conn.Report(healthReport(1), clock.Now())
	// The live connection remains usable; only the summary is missing.
	status, ok := hub.Status(1, "agent0123456789abcdef")
	if !ok || !status.Connected || summary.count() != 0 {
		t.Fatalf("status: %+v saves=%d", status, summary.count())
	}
}

func TestHubPrunesWhileIdle(t *testing.T) {
	hub, summary, _, _ := testHub(t)
	hub.Start()
	deadline := time.Now().Add(time.Second)
	for summary.pruneCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if summary.pruneCount() == 0 {
		t.Fatal("idle hub did not prune")
	}
}
