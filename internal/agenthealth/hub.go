package agenthealth

import (
	"context"
	"errors"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	freshReportWindow = 90 * time.Second
	maxRollingRTT     = 120
	maxOutstanding    = 8
	maxConsecutive    = 3
)

// SummaryStore persists bounded controller-minute summaries and prunes expired
// rows. *store.Store satisfies it.
type SummaryStore interface {
	SaveRemoteHealthMinuteFor(uid int64, in store.RemoteHealthMinute) error
	PruneRemoteHealth() error
}

// Sender writes one typed control message to the agent connection.
type Sender func(context.Context, terminalproto.Message) error

// Status is a live or retained observation. Freshness is always computed from
// controller receipt time. RTT variability is the spread between p95 and p50
// over the retained rolling samples, not raw network latency.
type Status struct {
	AgentID              string                      `json:"agentId"`
	Connected            bool                        `json:"connected"`
	Unreachable          bool                        `json:"unreachable,omitempty"`
	LastKnown            bool                        `json:"lastKnown,omitempty"`
	ContactAt            int64                       `json:"contactAt,omitempty"`
	ReportAt             int64                       `json:"reportAt,omitempty"`
	Generation           int64                       `json:"generation,omitempty"`
	InstanceID           string                      `json:"agentInstanceId,omitempty"`
	Capabilities         terminalproto.Capabilities  `json:"capabilities"`
	Report               *terminalproto.HealthReport `json:"report,omitempty"`
	ReportFresh          bool                        `json:"reportFresh"`
	RTTMillis            *float64                    `json:"rttMillis,omitempty"`
	RTTP50Millis         *float64                    `json:"rttP50Millis,omitempty"`
	RTTP95Millis         *float64                    `json:"rttP95Millis,omitempty"`
	RTTVariabilityMillis *float64                    `json:"rttVariabilityMillis,omitempty"`
	MissedProbes         int                         `json:"missedProbes"`
	Reconnects           int                         `json:"reconnects"`
	Path                 string                      `json:"path"`
}

type HubConfig struct {
	Store         SummaryStore
	Now           func() time.Time
	ProbeInterval time.Duration
	ProbeTimeout  time.Duration
	PruneInterval time.Duration
	MaxLastKnown  int
	Logf          func(format string, args ...any)
}

// Hub owns live controller connections and a bounded last-known snapshot for
// machines that have disconnected. Persisted history remains in the store.
type Hub struct {
	st            SummaryStore
	now           func() time.Time
	probeInterval time.Duration
	probeTimeout  time.Duration
	pruneInterval time.Duration
	maxLastKnown  int
	logf          func(format string, args ...any)

	mu     sync.Mutex
	live   map[string]*Conn
	last   map[string]Status
	closed bool
	done   chan struct{}
	prune  sync.Once
}

func NewHub(cfg HubConfig) *Hub {
	h := &Hub{
		st: cfg.Store, now: cfg.Now, probeInterval: cfg.ProbeInterval, probeTimeout: cfg.ProbeTimeout,
		pruneInterval: cfg.PruneInterval, maxLastKnown: cfg.MaxLastKnown, logf: cfg.Logf,
		live: map[string]*Conn{}, last: map[string]Status{}, done: make(chan struct{}),
	}
	if h.now == nil {
		h.now = time.Now
	}
	if h.probeInterval <= 0 {
		h.probeInterval = 30 * time.Second
	}
	if h.probeTimeout <= 0 {
		h.probeTimeout = 5 * time.Second
	}
	if h.pruneInterval <= 0 {
		h.pruneInterval = time.Hour
	}
	if h.maxLastKnown <= 0 {
		h.maxLastKnown = 256
	}
	if h.logf == nil {
		h.logf = func(string, ...any) {}
	}
	return h
}

// Start begins the idle retention loop. The API starts it when private
// connections are configured; a direct Connect also starts it.
func (h *Hub) Start() {
	if h == nil {
		return
	}
	h.startPrune()
}

func hubKey(uid int64, agentID string) string {
	return agentID + "\x00" + strconv.FormatInt(uid, 10)
}

// Connect installs a new connection generation. Any previous connection for
// the same enrollment is fenced and its last observation is retained.
func (h *Hub) Connect(uid int64, agentID string, generation int64, instance string, caps terminalproto.Capabilities, send Sender) (*Conn, error) {
	if h == nil || send == nil || generation <= 0 {
		return nil, terminalproto.ErrInvalid
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Conn{
		hub: h, uid: uid, agentID: agentID, generation: generation, instance: instance, caps: caps,
		send: send, ctx: ctx, cancel: cancel, done: make(chan struct{}),
		acc:         NewAccumulator(h.now().Truncate(time.Minute).UnixMilli(), generation),
		outstanding: map[string]probeAttempt{},
	}
	if generation > 1 {
		c.acc.NoteReconnect()
		c.reconnects = 1
	}
	key := hubKey(uid, agentID)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		cancel()
		return nil, store.ErrRemoteNotFound
	}
	old := h.live[key]
	h.live[key] = c
	h.mu.Unlock()
	if old != nil {
		old.Close()
	}
	go c.probeLoop()
	return c, nil
}

func (h *Hub) startPrune() {
	if h.st == nil {
		return
	}
	h.prune.Do(func() {
		go func() {
			if err := h.st.PruneRemoteHealth(); err != nil {
				h.logf("health summary pruning failed")
			}
			ticker := time.NewTicker(h.pruneInterval)
			defer ticker.Stop()
			for {
				select {
				case <-h.done:
					return
				case <-ticker.C:
					if err := h.st.PruneRemoteHealth(); err != nil {
						h.logf("health summary pruning failed")
					}
				}
			}
		}()
	})
}

func (h *Hub) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	conns := make([]*Conn, 0, len(h.live))
	for _, c := range h.live {
		conns = append(conns, c)
	}
	close(h.done)
	h.mu.Unlock()
	for _, c := range conns {
		c.Close()
	}
}

// Status returns a live or last-known observation. It does not consult the
// store; the API combines this with persisted capabilities and history.
func (h *Hub) Status(uid int64, agentID string) (Status, bool) {
	if h == nil {
		return Status{}, false
	}
	key := hubKey(uid, agentID)
	h.mu.Lock()
	conn := h.live[key]
	last, hasLast := h.last[key]
	h.mu.Unlock()
	if conn != nil {
		status := conn.Status()
		if status.Connected || status.ContactAt != 0 {
			return status, true
		}
	}
	if hasLast {
		status := last
		status.LastKnown = true
		status.Connected = false
		status.ReportFresh = status.ReportAt != 0 && h.now().Sub(time.UnixMilli(status.ReportAt)) <= freshReportWindow
		return status, true
	}
	return Status{}, false
}

type probeAttempt struct {
	sequence int64
	at       time.Time
}

type Conn struct {
	hub        *Hub
	uid        int64
	agentID    string
	generation int64
	instance   string
	caps       terminalproto.Capabilities
	send       Sender
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}

	mu           sync.Mutex
	closed       bool
	unreachable  bool
	started      time.Time
	lastContact  time.Time
	lastReportAt time.Time
	lastReport   *terminalproto.HealthReport
	lastSequence int64
	lastRTT      *float64
	rtt          []float64
	misses       int
	reconnects   int
	outstanding  map[string]probeAttempt
	nextSequence int64
	acc          *Accumulator
	revision     int64
}

func (c *Conn) Done() <-chan struct{} { return c.done }

func (c *Conn) statusLocked(now time.Time) Status {
	status := Status{
		AgentID: c.agentID, Connected: !c.closed && !c.unreachable, ContactAt: unixMilli(c.lastContact),
		ReportAt: unixMilli(c.lastReportAt), Generation: c.generation, InstanceID: c.instance,
		Capabilities: c.caps, MissedProbes: c.misses, Reconnects: c.reconnects,
		Unreachable: c.unreachable, Path: "unknown",
	}
	if c.lastReport != nil {
		status.ReportFresh = now.Sub(c.lastReportAt) <= freshReportWindow
	}
	if c.lastRTT != nil {
		value := *c.lastRTT
		status.RTTMillis = &value
	}
	status.RTTP50Millis = Percentile(c.rtt, 0.5)
	status.RTTP95Millis = Percentile(c.rtt, 0.95)
	if status.RTTP50Millis != nil && status.RTTP95Millis != nil {
		spread := *status.RTTP95Millis - *status.RTTP50Millis
		status.RTTVariabilityMillis = &spread
	}
	if c.lastReport != nil {
		copy := *c.lastReport
		status.Report = &copy
	}
	return status
}

func (c *Conn) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statusLocked(c.hub.now())
}

// Report accepts an authenticated, ordered health report. Duplicate and
// out-of-order reports within an instance are ignored.
func (c *Conn) Report(report terminalproto.HealthReport, received time.Time) {
	if report.Validate() != nil || report.AgentInstanceID != c.instance {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.unreachable || report.Sequence <= c.lastSequence {
		return
	}
	c.lastSequence = report.Sequence
	c.lastContact = received
	c.lastReportAt = received
	copy := report
	c.lastReport = &copy
	c.acc.AddReport(report)
	c.persistLocked(received)
}

// ProbeResult accepts each matching probe once and measures RTT with the
// controller's receipt time. Host timestamps are never subtracted.
func (c *Conn) ProbeResult(result terminalproto.ProbeResult, received time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.unreachable {
		return
	}
	attempt, ok := c.outstanding[result.Nonce]
	if !ok || attempt.sequence != result.Sequence {
		return
	}
	delete(c.outstanding, result.Nonce)
	rtt := received.Sub(attempt.at)
	if rtt < 0 || rtt > 5*time.Second {
		return
	}
	millis := float64(rtt) / float64(time.Millisecond)
	c.lastContact = received
	c.misses = 0
	c.lastRTT = &millis
	if len(c.rtt) < maxRollingRTT {
		c.rtt = append(c.rtt, millis)
	} else {
		c.rtt = append(c.rtt[1:], millis)
	}
	c.acc.AddRTT(millis)
	c.persistLocked(received)
}

// ReportTraffic records actual terminal payload bytes once streams exist.
func (c *Conn) ReportTraffic(input, output int64, received time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.caps.HealthEnabled {
		return
	}
	c.acc.AddTraffic(input, output)
	c.persistLocked(received)
}

func (c *Conn) persistLocked(now time.Time) {
	if c.closed || !c.caps.HealthEnabled || c.hub.st == nil {
		return
	}
	minute := now.Truncate(time.Minute).UnixMilli()
	if c.acc.Minute != minute {
		scope := c.acc.Scope
		c.acc = NewAccumulator(minute, c.generation)
		c.acc.Scope = scope
		c.revision = 0
	}
	c.revision++
	if c.revision > terminalproto.MaxCounter {
		c.revision = 1
	}
	in := store.RemoteHealthMinute{
		AgentID: c.agentID, Minute: minute, Generation: c.generation, Revision: c.revision,
		Summary: c.acc.Summary(),
	}
	if err := c.hub.st.SaveRemoteHealthMinuteFor(c.uid, in); err != nil {
		c.hub.logf("health summary write failed")
		if errors.Is(err, store.ErrRemoteNotFound) || errors.Is(err, store.ErrTerminalNotReady) || errors.Is(err, store.ErrTerminalConflict) {
			go c.Close()
		}
	}
}

func (c *Conn) probeLoop() {
	delay := jitter(c.hub.probeInterval)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	c.sendProbe()
	check := time.NewTicker(time.Second)
	defer check.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-timer.C:
			c.sendProbe()
			timer.Reset(jitter(c.hub.probeInterval))
		case <-check.C:
			c.expireProbes()
		}
	}
}

func (c *Conn) sendProbe() {
	nonce, err := randomID()
	if err != nil {
		c.cancel()
		return
	}
	c.mu.Lock()
	if c.closed || c.unreachable || len(c.outstanding) >= maxOutstanding {
		c.mu.Unlock()
		return
	}
	c.nextSequence++
	if c.nextSequence <= 0 || c.nextSequence > terminalproto.MaxCounter {
		c.nextSequence = 1
	}
	sequence := c.nextSequence
	now := c.hub.now()
	c.outstanding[nonce] = probeAttempt{sequence: sequence, at: now}
	c.mu.Unlock()
	message := terminalproto.Message{
		Version: terminalproto.Version, RequestID: nonce, Generation: c.generation,
		Body: terminalproto.Probe{Sequence: sequence, Nonce: nonce},
	}
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	if err := c.send(ctx, message); err != nil {
		c.Close()
	}
}

func (c *Conn) expireProbes() {
	now := c.hub.now()
	var closeNow bool
	c.mu.Lock()
	if c.closed || c.unreachable {
		c.mu.Unlock()
		return
	}
	for nonce, attempt := range c.outstanding {
		if now.Sub(attempt.at) < c.hub.probeTimeout {
			continue
		}
		delete(c.outstanding, nonce)
		c.misses++
		c.acc.MissProbe()
		c.persistLocked(now)
		if c.misses >= maxConsecutive {
			c.unreachable = true
			closeNow = true
			break
		}
	}
	c.mu.Unlock()
	if closeNow {
		go c.Close()
	}
}

// Close detaches this connection, retains its last observation, and stops its
// probe loop. It never touches the host or a future terminal session.
func (c *Conn) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	status := c.statusLocked(c.hub.now())
	c.mu.Unlock()

	c.cancel()
	c.hub.mu.Lock()
	key := hubKey(c.uid, c.agentID)
	if c.hub.live[key] == c {
		delete(c.hub.live, key)
		c.hub.rememberLocked(key, status)
	}
	c.hub.mu.Unlock()
	close(c.done)
}

func (h *Hub) rememberLocked(key string, status Status) {
	if len(h.last) >= h.maxLastKnown {
		var oldestKey string
		oldest := time.Time{}
		for candidate, value := range h.last {
			at := time.UnixMilli(value.ReportAt)
			if oldest.IsZero() || at.Before(oldest) {
				oldestKey, oldest = candidate, at
			}
		}
		delete(h.last, oldestKey)
	}
	status.LastKnown = true
	h.last[key] = status
}

func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	factor := 0.9 + 0.2*rand.Float64()
	return time.Duration(float64(d) * factor)
}
