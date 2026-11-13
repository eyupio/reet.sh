package agenthealth

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// After a string of failures the delay has climbed to its ceiling. A session
// that then stays up must reset it, or every later drop would be retried at the
// slowest interval: the machine would recover sluggishly for the rest of its
// life because of a burst of failures that is long over.
func TestAStableSessionResetsADelayThatHadClimbed(t *testing.T) {
	const retryMin = 50 * time.Millisecond
	cfg := AgentConfig{BackoffMin: retryMin, BackoffMax: 400 * time.Millisecond, StableSession: 20 * time.Millisecond, Logf: func(string, ...any) {}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var count atomic.Int64
	var endedAt atomic.Int64
	waits := make(chan time.Duration, 16)
	session := func(ctx context.Context, _ *int64) (bool, error) {
		now := time.Now().UnixNano()
		if ended := endedAt.Load(); ended != 0 {
			waits <- time.Duration(now - ended)
		}
		n := count.Add(1)
		switch {
		case n <= 6:
			// Six instantly-dying sessions: the delay climbs to its ceiling.
			endedAt.Store(time.Now().UnixNano())
			return true, errors.New("private control connection lost")
		case n == 7:
			// One healthy session, well past the stable window.
			select {
			case <-time.After(60 * time.Millisecond):
			case <-ctx.Done():
			}
			endedAt.Store(time.Now().UnixNano())
			return true, nil
		default:
			cancel()
			return true, errors.New("private control connection lost")
		}
	}
	_ = reconnectLoop(ctx, cfg, session)
	close(waits)
	var all []time.Duration
	for w := range waits {
		all = append(all, w)
	}
	if len(all) < 7 {
		// Six waits follow the six dying sessions; the seventh follows the
		// healthy one. The final session is cancelled, so it adds no wait.
		t.Fatalf("only %d waits recorded", len(all))
	}
	// The waits before the healthy session must have climbed well past base.
	if all[5] < 2*retryMin {
		t.Fatalf("the delay did not climb during the failures: %v", all[5])
	}
	// The wait after the healthy session must be back at the base interval, not
	// still at the ceiling. Jitter is ±10%, so the ceiling is unmistakable.
	if after := all[6]; after > 2*retryMin {
		t.Fatalf("a healthy session did not reset a climbed delay: waited %v (base %v, ceiling %v)", after, retryMin, cfg.BackoffMax)
	}
}

// Exactly one connection attempt may be in flight at a time, however often the
// session fails or however fast it returns. Overlapping attempts would give the
// controller two live connections for one machine, and the second would fence
// the first and detach whatever it had attached. The peak assertion catches
// attempts that genuinely overlap; -race is what catches an attempt started in
// a goroutine and not joined, which is the realistic way this regresses.
func TestReconnectLoopNeverRunsTwoAttemptsAtOnce(t *testing.T) {
	cfg := AgentConfig{BackoffMin: time.Millisecond, BackoffMax: 2 * time.Millisecond, StableSession: 20 * time.Millisecond, Logf: func(string, ...any) {}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var inFlight, peak atomic.Int64
	var count atomic.Int64
	session := func(context.Context, *int64) (bool, error) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			high := peak.Load()
			if n <= high || peak.CompareAndSwap(high, n) {
				break
			}
		}
		if count.Add(1) >= 40 {
			cancel()
		}
		return true, errors.New("private control connection lost")
	}
	started := time.Now()
	_ = reconnectLoop(ctx, cfg, session)
	if got := peak.Load(); got != 1 {
		t.Fatalf("%d connection attempts ran at once; the controller would hold duplicate connections", got)
	}
	if inFlight.Load() != 0 {
		t.Fatalf("an attempt was still in flight after the loop returned")
	}
	if count.Load() < 40 {
		t.Fatalf("loop stopped early after %d attempts in %v", count.Load(), time.Since(started))
	}
}

// A revoked or expired enrollment must not be retried at the base interval and
// must not be reported as a generic refusal. It is a permanent condition: the
// retry goes straight to the ceiling and the reason says re-enrolling is the
// fix, so an operator is not left watching an agent that never connects.
func TestRefusedEnrollmentBacksOffToTheCeilingAndSaysWhy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "machine enrollment is missing or revoked", http.StatusForbidden)
	}))
	defer server.Close()
	cfg := AgentConfig{
		ServerURL: server.URL, Token: "token", AgentID: "agent0123456789abcdef",
		Secret: "secret", Version: "test", HTTPClient: server.Client(),
		BackoffMin: 5 * time.Millisecond, BackoffMax: 2 * time.Second,
		StableSession: 10 * time.Millisecond, Logf: func(string, ...any) {},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var count atomic.Int64
	var endedAt atomic.Int64
	waits := make(chan time.Duration, 8)
	var seen []string
	var mu sync.Mutex
	cfg.OnError = func(message string) {
		mu.Lock()
		seen = append(seen, message)
		mu.Unlock()
	}
	session := func(ctx context.Context, sequence *int64) (bool, error) {
		if ended := endedAt.Load(); ended != 0 {
			waits <- time.Duration(time.Now().UnixNano() - ended)
		}
		if count.Add(1) >= 3 {
			cancel()
		}
		connected, err := runPrivateSession(ctx, cfg, sequence)
		endedAt.Store(time.Now().UnixNano())
		return connected, err
	}
	_ = reconnectLoop(ctx, cfg, session)
	close(waits)
	var all []time.Duration
	for w := range waits {
		all = append(all, w)
	}
	if len(all) < 2 {
		t.Fatalf("only %d waits recorded", len(all))
	}
	// A permanent refusal must go to the ceiling on the first retry, not climb
	// from the base interval. Jitter is ±10%, so base and ceiling are
	// unmistakable here.
	if all[0] < cfg.BackoffMax/2 {
		t.Fatalf("a refused enrollment was retried after %v, not at the ceiling %v", all[0], cfg.BackoffMax)
	}
	if all[len(all)-1] < cfg.BackoffMax/2 {
		t.Fatalf("the refused retry did not stay at the ceiling: %v", all[len(all)-1])
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 {
		t.Fatal("the refusal was never surfaced through OnError")
	}
	joined := strings.Join(seen, "\n")
	if !strings.Contains(joined, "re-enroll") {
		t.Fatalf("the surfaced reason does not say retrying cannot fix it: %q", joined)
	}
	if strings.Contains(joined, "private bootstrap was refused") && !strings.Contains(joined, "re-enroll") {
		t.Fatalf("the refusal was reported generically: %q", joined)
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
