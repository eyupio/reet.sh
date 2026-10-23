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
	conn, _, err := websocket.Dial(dialCtx, privateconn.Origin+"/private/v1/control", &websocket.DialOptions{
		HTTPClient: client.HTTPClient(),
		HTTPHeader: authHeaders(cfg),
	})
	dialCancel()
	if err != nil {
		return false, errors.New("private control connection unavailable")
	}
	defer conn.CloseNow()
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
	start(func() { readControl(sessionCtx, conn, generation, send, errorsSeen) })
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

func readConsent(read func() (bool, terminalproto.Readiness, error)) (bool, terminalproto.Readiness) {
	enabled, readiness, err := read()
	if err != nil || capabilitiesFor(AgentConfig{}, enabled, readiness).Validate() != nil {
		return false, terminalproto.DisabledLocally
	}
	return enabled, readiness
}

func readControl(ctx context.Context, conn *websocket.Conn, generation int64, send Sender, errorsSeen chan<- error) {
	for {
		readCtx, cancel := context.WithTimeout(ctx, controlReadTimeout)
		kind, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			errorsSeen <- errors.New("private control connection lost")
			return
		}
		if kind != websocket.MessageText {
			errorsSeen <- terminalproto.ErrInvalid
			return
		}
		message, err := terminalproto.Decode(data)
		if err != nil || message.Generation != generation {
			errorsSeen <- terminalproto.ErrInvalid
			return
		}
		probe, ok := message.Body.(*terminalproto.Probe)
		if !ok {
			errorsSeen <- terminalproto.ErrInvalid
			return
		}
		result := terminalproto.ProbeResult(*probe)
		if err := send(ctx, terminalproto.Message{
			Version: terminalproto.Version, RequestID: message.RequestID, Generation: generation, Body: result,
		}); err != nil {
			errorsSeen <- err
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
		if nextEnabled == enabled && nextReadiness == readiness {
			continue
		}
		caps := capabilitiesFor(cfg, nextEnabled, nextReadiness)
		if caps.Validate() != nil {
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
	return terminalproto.Capabilities{
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
