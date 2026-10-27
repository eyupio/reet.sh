package agenthealth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	defaultBootstrapTimeout = 20 * time.Second
	defaultReportInterval   = 30 * time.Second
	defaultCollectTimeout   = 3 * time.Second
	defaultBackoffMin       = time.Second
	defaultBackoffMax       = 15 * time.Second
	controlReadTimeout      = 90 * time.Second
)

// AgentConfig is the installed agent's private monitoring configuration. Health
// reporting is independent of terminal opt-in.
type AgentConfig struct {
	commandHistory    bool
	launch            bool
	worktree          bool
	browse            bool
	branches          bool
	browseAllowed     bool
	OnConnection      func(bool)
	TerminalSession   TerminalFactory
	control           func(context.Context, terminalproto.Message) error
	ServerURL         string
	Token             string
	AgentID           string
	Secret            string
	Version           string
	InstanceID        string
	Home              string
	TerminalEnabled   bool
	TerminalReadiness terminalproto.Readiness
	// Consent is polled for local terminal opt-in and readiness changes.
	Consent func() (enabled bool, readiness terminalproto.Readiness, err error)
	// LaunchTools lists the agent CLIs this connector can start, from its
	// built-in presets. Nil means the connector cannot start agents.
	LaunchTools func() []string
	// GitAvailable reports whether git is on this connector's PATH, for run
	// worktrees. Nil means the connector cannot make worktrees.
	GitAvailable func() bool
	// BrowseAllowed re-reads the machine's local folder-browse switch
	// (reet terminal browse on|off). It is polled with Consent. Nil means
	// browsing is refused.
	BrowseAllowed func() bool
	// TerminalAllowed re-reads the local terminal opt-in alone (the
	// enrollment is still this one and terminal access is on), without
	// probing readiness. Browse and branches requests are gated on it; nil
	// refuses them.
	TerminalAllowed func() bool

	Collector        Collector
	HTTPClient       *http.Client
	Logf             func(format string, args ...any)
	BootstrapTimeout time.Duration
	ReportInterval   time.Duration
	CollectTimeout   time.Duration
	BackoffMin       time.Duration
	BackoffMax       time.Duration
	ConsentInterval  time.Duration
}

// TerminalFactory is given only the authenticated private client's HTTP
// transport. Its returned cleanup must cancel and join all generation workers.
type TerminalFactory func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func())

// RunAgent maintains the private control connection and reports host health.
// Transient failures are retried with bounded backoff; only context
// cancellation stops the loop. It never changes the process working directory.
func RunAgent(ctx context.Context, cfg AgentConfig) error {
	if err := cfg.normalise(); err != nil {
		return err
	}
	sequence := int64(0)
	backoff := cfg.BackoffMin
	lastError := ""
	for {
		if ctx.Err() != nil {
			return nil
		}
		connected, err := runPrivateSession(ctx, cfg, &sequence)
		if ctx.Err() != nil {
			return nil
		}
		if connected {
			backoff = cfg.BackoffMin
			lastError = ""
		}
		if err != nil {
			message := err.Error()
			if message != lastError {
				cfg.Logf("private monitoring: %v", err)
				lastError = message
			}
		}
		timer := time.NewTimer(jitter(backoff))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		if !connected {
			backoff *= 2
			if backoff > cfg.BackoffMax {
				backoff = cfg.BackoffMax
			}
		}
	}
}

func (cfg *AgentConfig) normalise() error {
	u, err := url.Parse(cfg.ServerURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return errors.New("private monitoring requires the controller origin")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return errors.New("private monitoring requires HTTPS")
	}
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")
	if cfg.Token == "" || cfg.AgentID == "" || cfg.Secret == "" {
		return errors.New("private monitoring requires the controller token and enrollment credentials")
	}
	if !terminalproto.ValidID(cfg.AgentID) {
		return errors.New("private monitoring needs a valid enrollment ID")
	}
	if cfg.InstanceID == "" {
		instance, err := randomID()
		if err != nil {
			return err
		}
		cfg.InstanceID = instance
	}
	if !terminalproto.ValidID(cfg.InstanceID) {
		return errors.New("private monitoring needs a valid agent instance ID")
	}
	if cfg.Home == "" {
		cfg.Home, _ = os.UserHomeDir()
	}
	if cfg.Collector == nil {
		cfg.Collector = NewCollector()
	}
	cfg.Collector = &boundedCollector{inner: cfg.Collector, busy: make(chan struct{}, 1)}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{
			Timeout: defaultBootstrapTimeout,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if cfg.BootstrapTimeout <= 0 {
		cfg.BootstrapTimeout = defaultBootstrapTimeout
	}
	if cfg.ReportInterval <= 0 {
		cfg.ReportInterval = defaultReportInterval
	}
	if cfg.CollectTimeout <= 0 {
		cfg.CollectTimeout = defaultCollectTimeout
	}
	if cfg.BackoffMin <= 0 {
		cfg.BackoffMin = defaultBackoffMin
	}
	if cfg.BackoffMax < cfg.BackoffMin {
		cfg.BackoffMax = defaultBackoffMax
	}
	if cfg.BackoffMax < cfg.BackoffMin {
		cfg.BackoffMax = cfg.BackoffMin
	}
	if cfg.ConsentInterval <= 0 {
		cfg.ConsentInterval = 5 * time.Second
	}
	return nil
}

func runPrivateSession(ctx context.Context, cfg AgentConfig, sequence *int64) (bool, error) {
	// Refresh consent before EVERY handshake, including reconnects. Never
	// advertise the opt-in snapshot captured when the process first started.
	if cfg.Consent != nil {
		cfg.TerminalEnabled, cfg.TerminalReadiness = readConsent(cfg.Consent)
	}
	cfg.browseAllowed = readBrowseAllowed(cfg)
	address, err := bootstrap(ctx, cfg)
	if err != nil {
		return false, err
	}
	client, err := privateconn.NewClient(address)
	if err != nil {
		return false, err
	}
	defer client.Close()
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	dialCtx, dialCancel := context.WithTimeout(sessionCtx, 15*time.Second)
	conn, response, err := websocket.Dial(dialCtx, privateconn.Origin+"/private/v1/control", &websocket.DialOptions{
		HTTPClient: client.HTTPClient(),
		HTTPHeader: authHeaders(cfg),
	})
	dialCancel()
	if err != nil {
		return false, errors.New("private control connection unavailable")
	}
	defer conn.CloseNow()
	advertised := ""
	if response != nil {
		advertised = response.Header.Get(terminalproto.FeatureHeader)
	}
	negotiateFeatures(&cfg, advertised)
	conn.SetReadLimit(terminalproto.MaxControlBytes)

	handshakeID, err := randomID()
	if err != nil {
		return false, err
	}
	handshake, err := terminalproto.EncodeHandshake(terminalproto.Handshake{
		ProtocolVersion: terminalproto.Version,
		RequestID:       handshakeID,
		AgentInstanceID: cfg.InstanceID,
		Capabilities:    capabilities(cfg),
	})
	if err != nil {
		return false, err
	}
	var writeMu sync.Mutex
	send := func(ctx context.Context, message terminalproto.Message) error {
		data, err := terminalproto.Encode(message)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
		defer writeCancel()
		return conn.Write(writeCtx, websocket.MessageText, data)
	}
	writeCtx, writeCancel := context.WithTimeout(sessionCtx, 5*time.Second)
	err = conn.Write(writeCtx, websocket.MessageText, handshake)
	writeCancel()
	if err != nil {
		return false, errors.New("private handshake was not accepted")
	}
	readCtx, readCancel := context.WithTimeout(sessionCtx, 5*time.Second)
	kind, replyData, err := conn.Read(readCtx)
	readCancel()
	if err != nil || kind != websocket.MessageText {
		return false, errors.New("private handshake was not acknowledged")
	}
	reply, err := terminalproto.DecodeHandshakeReply(replyData)
	if err != nil || reply.ProtocolVersion != terminalproto.Version || reply.RequestID != handshakeID {
		return false, errors.New("this agent protocol is not supported; update the agent or controller")
	}
	generation := reply.Generation
	if cfg.OnConnection != nil {
		cfg.OnConnection(true)
		defer cfg.OnConnection(false)
	}
	if cfg.TerminalSession != nil {
		var closeTerminal func()
		terminalHeaders := authHeaders(cfg)
		setTerminalFeatures(terminalHeaders, cfg)
		cfg.control, closeTerminal = cfg.TerminalSession(sessionCtx, client.HTTPClient(), terminalHeaders, generation, send)
		defer closeTerminal()
	}

	return true, runControlWorkers(sessionCtx, cfg, conn, generation, sequence, send)
}

// Join the old generation's workers before another generation can reuse its
// collector or sequence. Bounded collection makes cancellation prompt even if
// an OS collector is stuck in an uninterruptible filesystem operation.
func runControlWorkers(ctx context.Context, cfg AgentConfig, conn *websocket.Conn, generation int64, sequence *int64, send Sender) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	errorsSeen := make(chan error, 3)
	var workers sync.WaitGroup
	start := func(fn func()) { workers.Add(1); go func() { defer workers.Done(); fn() }() }
	defer func() { cancel(); conn.CloseNow(); workers.Wait() }()
	control := cfg.control
	if control != nil {
		control = gateMachineRequests(cfg, control, send)
	}
	start(func() {
		readControlWithTerminal(sessionCtx, conn, generation, send, errorsSeen, terminalFeatures(cfg), control)
	})
	start(func() { reportHealth(sessionCtx, cfg, generation, sequence, send, errorsSeen) })
	if cfg.Consent != nil {
		initial := capabilities(cfg)
		start(func() {
			consentControl(sessionCtx, cfg, generation, initial.TerminalEnabled, initial.Readiness, send, errorsSeen)
		})
	}
	select {
	case <-sessionCtx.Done():
		return nil
	case err := <-errorsSeen:
		cancel()
		return err
	}
}

// gateMachineRequests serves a browse or branches request whose feature was
// negotiated only while the connector would advertise the matching
// capability, worked out afresh (terminal consent and the browse switch are
// re-read) for each request; otherwise it answers denied itself, so the
// connector never serves more than it offers. Everything else, including a
// browse or branches request that was not negotiated (a protocol error
// there), goes to control.
func gateMachineRequests(cfg AgentConfig, control func(context.Context, terminalproto.Message) error, send Sender) func(context.Context, terminalproto.Message) error {
	return func(ctx context.Context, m terminalproto.Message) error {
		switch m.Body.(type) {
		case *terminalproto.Browse:
			if cfg.browse && !currentCapabilities(cfg).Browse {
				m.Body = terminalproto.BrowseResult{Reason: terminalproto.BrowseDenied}
				return send(ctx, m)
			}
		case *terminalproto.Branches:
			if cfg.branches && !currentCapabilities(cfg).Branches {
				m.Body = terminalproto.BranchesResult{Reason: terminalproto.BranchesDenied}
				return send(ctx, m)
			}
		}
		return control(ctx, m)
	}
}

// currentCapabilities is the browse and branches part of what the
// connector would advertise now, from cheap local reads only: the terminal
// opt-in (TerminalAllowed) and the browse switch are read again, git is
// looked up on PATH. It never calls Consent, whose readiness probe runs
// tmux and can take seconds; neither capability depends on readiness.
func currentCapabilities(cfg AgentConfig) terminalproto.Capabilities {
	enabled := cfg.TerminalAllowed != nil && cfg.TerminalAllowed()
	readiness := terminalproto.DisabledLocally
	if enabled {
		readiness = terminalproto.Ready
	}
	cfg.browseAllowed = readBrowseAllowed(cfg)
	return capabilitiesFor(cfg, enabled, readiness)
}

// readBrowseAllowed reads the local folder-browse switch; without one,
// browsing is refused.
func readBrowseAllowed(cfg AgentConfig) bool { return cfg.BrowseAllowed != nil && cfg.BrowseAllowed() }

func readConsent(read func() (bool, terminalproto.Readiness, error)) (bool, terminalproto.Readiness) {
	enabled, readiness, err := read()
	if err != nil || capabilitiesFor(AgentConfig{}, enabled, readiness).Validate() != nil {
		return false, terminalproto.DisabledLocally
	}
	return enabled, readiness
}

// terminalQueueSize bounds the terminal messages waiting for the terminal
// dispatcher. The controller holds at most 32 calls per machine at once, but
// it gives up on a call after its timeout while the message may still wait
// here (behind a slow worktree create, for example), so its retries can fill
// the queue. A request that finds the queue full is answered unavailable by
// the reader; the connection is kept.
const terminalQueueSize = 64

// unavailable is the answer to a request the dispatcher cannot take: a
// worktree request gets a worktree_result, a browse or branches request its
// own result as conflict (busy), every other request a result, with the
// request's own envelope. A browse or branches request whose feature is not
// in features (what this connection negotiated) has no answer: it is a
// protocol error, as the dispatcher would treat it, and false is returned.
func unavailable(m terminalproto.Message, features string) (terminalproto.Message, bool) {
	switch m.Body.(type) {
	case *terminalproto.WorktreeStatus, *terminalproto.WorktreeRemove:
		m.Body = terminalproto.WorktreeResult{Code: terminalproto.Unavailable}
	case *terminalproto.Browse:
		if !terminalproto.SupportsBrowse(features) {
			return m, false
		}
		m.Body = terminalproto.BrowseResult{Reason: terminalproto.BrowseConflict}
	case *terminalproto.Branches:
		if !terminalproto.SupportsBranches(features) {
			return m, false
		}
		m.Body = terminalproto.BranchesResult{Reason: terminalproto.BranchesConflict}
	default:
		m.Body = terminalproto.Result{Code: terminalproto.Unavailable}
	}
	return m, true
}

func readControl(ctx context.Context, conn *websocket.Conn, generation int64, send Sender, errorsSeen chan<- error) {
	readControlWithTerminal(ctx, conn, generation, send, errorsSeen, "", nil)
}

// readControlWithTerminal answers probes on the reader itself and hands every
// other message to one dispatcher goroutine, in arrival order, over a bounded
// queue. A slow terminal request (a worktree's git, for example) therefore
// never delays a probe answer, and the terminal handler still sees one
// message at a time, as its attachment bookkeeping requires.
func readControlWithTerminal(ctx context.Context, conn *websocket.Conn, generation int64, send Sender, errorsSeen chan<- error, features string, terminal func(context.Context, terminalproto.Message) error) {
	report := func(err error) {
		select {
		case errorsSeen <- err:
		default: // an error is already waiting; the session ends on the first
		}
	}
	var queue chan terminalproto.Message
	if terminal != nil {
		queue = make(chan terminalproto.Message, terminalQueueSize)
		var dispatcher sync.WaitGroup
		dispatcher.Add(1)
		go func() {
			defer dispatcher.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case message := <-queue:
					if err := terminal(ctx, message); err != nil {
						report(err)
						return
					}
				}
			}
		}()
		// The reader reports its own error before this wait, so the session
		// cancels ctx and the dispatcher's current request ends.
		defer dispatcher.Wait()
	}
	for {
		readCtx, cancel := context.WithTimeout(ctx, controlReadTimeout)
		kind, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			report(errors.New("private control connection lost"))
			return
		}
		if kind != websocket.MessageText {
			report(terminalproto.ErrInvalid)
			return
		}
		message, err := terminalproto.Decode(data)
		if err != nil || message.Generation != generation {
			report(terminalproto.ErrInvalid)
			return
		}
		probe, ok := message.Body.(*terminalproto.Probe)
		if !ok {
			if queue == nil {
				report(terminalproto.ErrInvalid)
				return
			}
			select {
			case queue <- message:
			default:
				answer, ok := unavailable(message, features)
				if !ok {
					report(terminalproto.ErrInvalid)
					return
				}
				if err := send(ctx, answer); err != nil {
					report(err)
					return
				}
			}
			continue
		}
		result := terminalproto.ProbeResult(*probe)
		if err := send(ctx, terminalproto.Message{
			Version: terminalproto.Version, RequestID: message.RequestID, Generation: generation, Body: result,
		}); err != nil {
			report(err)
			return
		}
	}
}

func consentControl(ctx context.Context, cfg AgentConfig, generation int64, enabled bool, readiness terminalproto.Readiness, send Sender, errorsSeen chan<- error) {
	ticker := time.NewTicker(cfg.ConsentInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		nextEnabled, nextReadiness := readConsent(cfg.Consent)
		nextBrowse := readBrowseAllowed(cfg)
		if nextEnabled == enabled && nextReadiness == readiness && nextBrowse == cfg.browseAllowed {
			continue
		}
		previousBrowse := cfg.browseAllowed
		cfg.browseAllowed = nextBrowse
		caps := capabilitiesFor(cfg, nextEnabled, nextReadiness)
		if caps.Validate() != nil {
			cfg.browseAllowed = previousBrowse
			continue
		}
		requestID, err := randomID()
		if err != nil {
			errorsSeen <- err
			return
		}
		if err := send(ctx, terminalproto.Message{
			Version: terminalproto.Version, RequestID: requestID, Generation: generation,
			Body: terminalproto.Hello{AgentInstanceID: cfg.InstanceID, Capabilities: caps},
		}); err != nil {
			errorsSeen <- err
			return
		}
		enabled, readiness = nextEnabled, nextReadiness
	}
}

func reportHealth(ctx context.Context, cfg AgentConfig, generation int64, sequence *int64, send Sender, errorsSeen chan<- error) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		collectCtx, cancel := context.WithTimeout(ctx, cfg.CollectTimeout)
		sample := cfg.Collector.Collect(collectCtx, cfg.Home)
		cancel()
		*sequence += 1
		if *sequence <= 0 || *sequence > terminalproto.MaxCounter {
			*sequence = 1
		}
		report := sample.HealthReport(cfg.InstanceID, *sequence)
		if report.Validate() != nil {
			errorsSeen <- terminalproto.ErrInvalid
			return
		}
		requestID, err := randomID()
		if err != nil {
			errorsSeen <- err
			return
		}
		if err := send(ctx, terminalproto.Message{
			Version: terminalproto.Version, RequestID: requestID, Generation: generation, Body: report,
		}); err != nil {
			errorsSeen <- err
			return
		}
		timer.Reset(jitter(cfg.ReportInterval))
	}
}

func bootstrap(ctx context.Context, cfg AgentConfig) (string, error) {
	if cfg.HTTPClient == nil {
		return "", errors.New("private bootstrap requires an HTTP client")
	}
	timeout := cfg.BootstrapTimeout
	if timeout <= 0 {
		timeout = defaultBootstrapTimeout
	}
	body, _ := json.Marshal(map[string]string{})
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, cfg.ServerURL+"/api/v2/sync/private", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, values := range authHeaders(cfg) {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return "", errors.New("private bootstrap could not reach the controller")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("private bootstrap was refused (HTTP %d)", resp.StatusCode)
	}
	var reply struct {
		ProtocolVersion int    `json:"protocolVersion"`
		Address         string `json:"address"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&reply); err != nil || reply.ProtocolVersion != terminalproto.Version || reply.Address == "" {
		return "", errors.New("private bootstrap reply is invalid")
	}
	return reply.Address, nil
}

func authHeaders(cfg AgentConfig) http.Header {
	return http.Header{
		"X-Reet-Token":     {cfg.Token},
		"X-Reet-Agent":     {cfg.AgentID},
		"X-Reet-Agent-Key": {cfg.Secret},
		"X-Reet-Version":   {cfg.Version},
	}
}

// negotiateFeatures records the optional extensions the controller advertised
// on the control upgrade. A feature it did not advertise is never claimed.
func negotiateFeatures(cfg *AgentConfig, advertised string) {
	cfg.commandHistory = terminalproto.SupportsCommandHistory(advertised)
	cfg.launch = terminalproto.SupportsLaunch(advertised)
	cfg.worktree = terminalproto.SupportsWorktree(advertised)
	cfg.browse = terminalproto.SupportsBrowse(advertised)
	cfg.branches = terminalproto.SupportsBranches(advertised)
}

// setTerminalFeatures tells the terminal dispatcher which negotiated
// extensions it may serve on this connection generation.
func setTerminalFeatures(headers http.Header, cfg AgentConfig) {
	if features := terminalFeatures(cfg); features != "" {
		headers.Set(terminalproto.FeatureHeader, features)
	}
}

// terminalFeatures lists the negotiated extensions the terminal dispatcher
// may serve, comma-separated, or "" for none.
func terminalFeatures(cfg AgentConfig) string {
	var features []string
	if cfg.commandHistory {
		features = append(features, terminalproto.CommandHistoryFeature)
	}
	if cfg.launch {
		features = append(features, terminalproto.LaunchFeature)
	}
	if cfg.worktree {
		features = append(features, terminalproto.WorktreeFeature)
	}
	if cfg.browse {
		features = append(features, terminalproto.BrowseFeature)
	}
	if cfg.branches {
		features = append(features, terminalproto.BranchesFeature)
	}
	return strings.Join(features, ",")
}

// launchTools returns the installed tools to report, keeping only known ids
// in sorted order and within the protocol's bound.
func launchTools(cfg AgentConfig) []string {
	var tools []string
	for _, tool := range cfg.LaunchTools() {
		if terminalproto.ValidTool(tool) && !slices.Contains(tools, tool) {
			tools = append(tools, tool)
		}
	}
	sort.Strings(tools)
	return tools
}

func capabilities(cfg AgentConfig) terminalproto.Capabilities {
	if cfg.TerminalEnabled && cfg.TerminalReadiness == "" {
		cfg.TerminalReadiness = terminalproto.SupervisorUnavailable
	}
	return capabilitiesFor(cfg, cfg.TerminalEnabled, cfg.TerminalReadiness)
}

func capabilitiesFor(cfg AgentConfig, enabled bool, readiness terminalproto.Readiness) terminalproto.Capabilities {
	osName := runtime.GOOS
	if osName != "linux" && osName != "darwin" {
		osName = "unknown"
		if !enabled {
			readiness = terminalproto.Unsupported
		}
	}
	if !enabled {
		if readiness == "" || readiness == terminalproto.Ready || readiness == terminalproto.TmuxUnavailable || readiness == terminalproto.SupervisorUnavailable || readiness == terminalproto.CapacityReached || readiness == terminalproto.Reconnecting {
			if osName == "unknown" {
				readiness = terminalproto.Unsupported
			} else {
				readiness = terminalproto.DisabledLocally
			}
		}
	}
	if enabled && readiness == terminalproto.DisabledLocally {
		readiness = terminalproto.SupervisorUnavailable
	}
	username := "unknown"
	if current, err := user.Current(); err == nil && current.Username != "" {
		username = current.Username
	}
	launch := cfg.launch && cfg.TerminalSession != nil && cfg.LaunchTools != nil && (osName == "linux" || osName == "darwin")
	var tools []string
	if launch {
		tools = launchTools(cfg)
	}
	// Worktrees need launch (a run starts an agent in one), the negotiated
	// worktree-v1 and git on PATH.
	worktree := launch && cfg.worktree && cfg.GitAvailable != nil && cfg.GitAvailable()
	// Browse and branches act on this machine's folders for the controller,
	// so they need terminal access on as well as their negotiated feature:
	// browse also the local folder-browse switch on, branches worktrees.
	control := cfg.TerminalSession != nil && (osName == "linux" || osName == "darwin")
	return terminalproto.Capabilities{
		Launch:          launch,
		Tools:           tools,
		Worktree:        worktree,
		Browse:          cfg.browse && control && enabled && cfg.browseAllowed,
		Branches:        cfg.branches && worktree && enabled,
		TerminalControl: cfg.TerminalSession != nil,
		CommandHistory:  cfg.commandHistory && cfg.TerminalSession != nil && (osName == "linux" || osName == "darwin"),
		ProtocolVersion: terminalproto.Version,
		OS:              osName,
		OSUser:          username,
		TerminalEnabled: enabled,
		HealthEnabled:   true,
		Readiness:       readiness,
	}
}

// HealthReport converts a sample at the given sequence. SampledAt is host time
// for diagnostics only; the controller uses receipt time for freshness.
func (s Sample) HealthReport(instance string, sequence int64) terminalproto.HealthReport {
	sampledAt := s.SampledAt
	if sampledAt.IsZero() {
		sampledAt = time.Now()
	}
	scope := s.Scope
	if scope != "host" && scope != "container" {
		scope = "host"
	}
	cpu := s.CPUPercent
	if cpu.Value == nil && cpu.Reason == "" {
		cpu.Reason = terminalproto.CollectionFailed
	}
	memory := fillCapacity(s.Memory)
	disk := fillCapacity(s.Disk)
	uptime := s.HostUptimeSeconds
	if uptime.Value == nil && uptime.Reason == "" {
		uptime.Reason = terminalproto.CollectionFailed
	}
	return terminalproto.HealthReport{
		SchemaVersion:     terminalproto.Version,
		Sequence:          sequence,
		AgentInstanceID:   instance,
		SampledAt:         sampledAt.UnixMilli(),
		Scope:             scope,
		CPUPercent:        cpu,
		Memory:            memory,
		Disk:              disk,
		HostUptimeSeconds: uptime,
	}
}

func fillCapacity(capacity terminalproto.CapacityReading) terminalproto.CapacityReading {
	if capacity.Total.Value == nil && capacity.Total.Reason == "" {
		capacity.Total.Reason = terminalproto.CollectionFailed
	}
	if capacity.Available.Value == nil && capacity.Available.Reason == "" {
		capacity.Available.Reason = terminalproto.CollectionFailed
	}
	return capacity
}
