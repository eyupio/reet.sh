package agenthealth

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Control-loop resilience. These cover the failure modes behind an
// intermittent drop: a connection whose handshake succeeded and then died at
// once used to reconnect every second forever (each reconnect fences the
// previous generation and detaches live terminals and chats); a half-open
// connection was not noticed until the next read or write; and every drop was
// reported with the same sentence, so none of it was diagnosable.

func TestRetryDelayGrowsWhileConnectionsKeepDyingAtOnce(t *testing.T) {
	// A session that was never accepted, or died inside one stable period, is
	// delayed over [backoff/2, backoff) and the backoff keeps growing, so a
	// flapping link cannot reconnect at the base interval forever.
	backoff := defaultBackoffMin
	for i := 0; i < 12; i++ {
		delay := retryDelay(backoff, false)
		if delay < backoff/2 || delay >= backoff {
			t.Fatalf("unstable retry %d: delay %v outside [%v, %v)", i, delay, backoff/2, backoff)
		}
		backoff = min(2*backoff, defaultBackoffMax)
	}
	if backoff != defaultBackoffMax {
		t.Fatalf("backoff did not climb to its ceiling, stopped at %v", backoff)
	}
	// A session that stayed up keeps the original ±10% jitter.
	stable := retryDelay(defaultBackoffMin, true)
	if stable < defaultBackoffMin*9/10 || stable > defaultBackoffMin*11/10 {
		t.Fatalf("stable retry jitter out of range: %v", stable)
	}
}

// The regression this guards: before the fix the loop reset its delay to the
// minimum as soon as a handshake succeeded, so a link that died immediately
// reconnected about once a second indefinitely. Each reconnect fences the
// previous generation and detaches that machine's live terminals and chats, so
// that showed up as sporadic drops that no single action explained.
func TestFlappingConnectionBacksOffInsteadOfReconnectingEverySecond(t *testing.T) {
	cfg := AgentConfig{BackoffMin: 5 * time.Millisecond, BackoffMax: 100 * time.Millisecond, Logf: func(string, ...any) {}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := make(chan time.Time, 512)
	var count atomic.Int64
	session := func(context.Context, *int64) (bool, error) {
		// Accepted, then lost at once, every time.
		attempts <- time.Now()
		if count.Add(1) >= 9 {
			cancel()
		}
		return true, errors.New("private control connection lost: use of closed network connection")
	}
	done := make(chan struct{})
	go func() { defer close(done); _ = reconnectLoop(ctx, cfg, session) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("reconnect loop did not stop when its context was cancelled")
	}
	close(attempts)
	var times []time.Time
	for at := range attempts {
		times = append(times, at)
	}
	if len(times) < 6 {
		t.Fatalf("only %d attempts recorded", len(times))
	}
	// The last gap must be much longer than the first. With the old reset the
	// gap stayed at the minimum for every attempt.
	first := times[1].Sub(times[0])
	last := times[len(times)-1].Sub(times[len(times)-2])
	if last < 3*first {
		t.Fatalf("retry gap did not grow: first %v, last %v", first, last)
	}
}

// A connection that stays up resets the delay; one that dies instantly must
// not. Without the stable window both looked the same to the loop.
func TestAStableSessionResetsTheDelayAndAFlappingOneDoesNot(t *testing.T) {
	cfg := AgentConfig{BackoffMin: 5 * time.Millisecond, BackoffMax: 40 * time.Millisecond, StableSession: 30 * time.Millisecond, Logf: func(string, ...any) {}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var count atomic.Int64
	attempts := make(chan time.Duration, 64)
	var previous atomic.Int64
	session := func(ctx context.Context, _ *int64) (bool, error) {
		now := time.Now().UnixNano()
		if earlier := previous.Swap(now); earlier != 0 {
			attempts <- time.Duration(now - earlier)
		}
		// A long healthy session, then a run of instantly dying ones.
		if count.Add(1) == 1 {
			select {
			case <-time.After(60 * time.Millisecond):
			case <-ctx.Done():
			}
			return true, nil
		}
		if count.Load() >= 14 {
			cancel()
		}
		return true, errors.New("lost")
	}
	_ = reconnectLoop(ctx, cfg, session)
	close(attempts)
	var gaps []time.Duration
	for gap := range attempts {
		gaps = append(gaps, gap)
	}
	if len(gaps) < 8 {
		t.Fatalf("only %d attempts recorded", len(gaps))
	}
	// The gap right after the stable session resets to the base interval.
	if gaps[1] > 20*time.Millisecond {
		t.Fatalf("a stable session did not reset the delay: next attempt after %v", gaps[1])
	}
	// The gaps after the flapping run must grow towards the ceiling.
	if last := gaps[len(gaps)-1]; last < 3*gaps[1] {
		t.Fatalf("a flapping connection kept the base delay: after stable %v, last %v", gaps[1], last)
	}
}

// A dropped socket must be named and its cause kept. Before the fix every drop
// read "private control connection lost" with nothing to distinguish a reset,
// a deadline or a protocol error.
func TestControlReaderReportsWhyTheConnectionWasLost(t *testing.T) {
	client, peer := controlPair(t)
	errorsSeen := make(chan error, 3)
	done := make(chan struct{})
	go func() {
		defer close(done)
		readControlWithTerminal(t.Context(), client, 1, clientSender(client), errorsSeen, "", time.Hour, nil)
	}()
	peer.CloseNow()
	select {
	case err := <-errorsSeen:
		const sentence = "private control connection lost"
		if !strings.Contains(err.Error(), sentence) {
			t.Fatalf("lost connection was not named: %v", err)
		}
		// A bare sentence was the old behaviour and told a log nothing. The
		// cause must be present after it, and must be the transport error so a
		// reset can be told from a deadline or a protocol error.
		if err.Error() == sentence {
			t.Fatalf("cause was swallowed, every drop looks identical: %v", err)
		}
		var closeErr websocket.CloseError
		if !errors.As(err, &closeErr) && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
			t.Fatalf("cause was not a transport error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a closed peer did not end the control reader")
	}
	<-done
}

// A peer that stays open but says nothing is a half-open connection. It must be
// detected by the reader's own liveness window rather than only failing on the
// next request, and the agent used to have no such check before its 90-second
// read deadline.
func TestControlReaderTreatsSilenceAsADeadConnection(t *testing.T) {
	client, _ := controlPair(t)
	errorsSeen := make(chan error, 3)
	done := make(chan struct{})
	go func() {
		defer close(done)
		readControlWithTerminal(t.Context(), client, 1, clientSender(client), errorsSeen, "", 50*time.Millisecond, nil)
	}()
	select {
	case err := <-errorsSeen:
		if !strings.Contains(err.Error(), "liveness window") {
			t.Fatalf("a silent connection was not reported as dead: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a silent control connection was never noticed")
	}
	<-done
}

// Repeated disconnects must not multiply connections or leak timers and
// listeners: each generation's workers are joined before the next begins, so
// the sequence counter they mutate is safe to reuse (checked under -race) and
// no second reader can exist for one machine.
func TestRepeatedDisconnectsJoinWorkersBeforeTheNextGeneration(t *testing.T) {
	sequence := int64(0)
	cfg := AgentConfig{
		InstanceID: testInstance(), ReportInterval: time.Hour, CollectTimeout: time.Hour,
		Collector: collectorFunc(func(context.Context, string) Sample { return Sample{} }),
	}
	for generation := int64(1); generation <= 5; generation++ {
		client, peer := controlPair(t)
		done := make(chan error, 1)
		go func() {
			done <- runControlWorkers(t.Context(), cfg, client, generation, &sequence, clientSender(client))
		}()
		peer.CloseNow()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("generation %d did not release its workers after a drop", generation)
		}
		sequence++
	}
}

// Cancelling the run must stop the loop promptly and leave nothing running;
// goleak in TestMain proves no timer or listener outlives it.
func TestReconnectLoopStopsWhenCancelled(t *testing.T) {
	cfg := AgentConfig{BackoffMin: 50 * time.Millisecond, BackoffMax: time.Second, Logf: func(string, ...any) {}}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 1)
	session := func(context.Context, *int64) (bool, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		return false, errors.New("unreachable")
	}
	done := make(chan struct{})
	go func() { defer close(done); _ = reconnectLoop(ctx, cfg, session) }()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelling the retry loop did not stop it")
	}
}
