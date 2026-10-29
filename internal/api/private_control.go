package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// agreedControlFeatures is the feature list this connection negotiates: every
// private control feature, with agents-v2 only for a machine that claims it.
func agreedControlFeatures(c terminalproto.Capabilities) string {
	features := []string{terminalproto.CommandHistoryFeature, terminalproto.LaunchFeature, terminalproto.WorktreeFeature, terminalproto.BrowseFeature, terminalproto.BranchesFeature, terminalproto.ClipboardFeature, terminalproto.UploadFeature, terminalproto.ModelsFeature}
	if c.AgentsV2 {
		features = append(features, terminalproto.AgentsFeature)
	}
	return strings.Join(features, ",")
}

// privateControlFeatures is what the route advertises: the most a machine can agree.
var privateControlFeatures = agreedControlFeatures(terminalproto.Capabilities{AgentsV2: true})

// privateControl upgrades the exact private control route, authenticates the
// enrollment, assigns a connection generation, and then services typed control
// messages. The listener context, not a header, proves the transport.
func (s *server) privateControl(w http.ResponseWriter, r *http.Request) {
	if !s.privateCredentials() {
		writeErr(w, http.StatusForbidden, "private connections require an authenticated instance")
		return
	}
	uid, agent, err := s.st.AuthenticateRemoteAgent(r.Header.Get("X-Reet-Agent"), r.Header.Get("X-Reet-Agent-Key"), r.Header.Get("X-Reet-Version"))
	if err != nil {
		writeErr(w, http.StatusForbidden, "machine enrollment is missing or revoked")
		return
	}
	w.Header().Set(terminalproto.FeatureHeader, privateControlFeatures)
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(terminalproto.MaxControlBytes)
	defer conn.CloseNow()

	readCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	kind, data, err := conn.Read(readCtx)
	cancel()
	if err != nil || kind != websocket.MessageText {
		_ = conn.Close(websocket.StatusPolicyViolation, "handshake required")
		return
	}
	handshake, err := terminalproto.DecodeHandshake(data)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "invalid handshake")
		return
	}
	// Agree the connection's features first and validate the hello against
	// them: a machine that does not claim agents-v2 cannot store v2 tools or an
	// OS version.
	connectionFeatures := agreedControlFeatures(handshake.Capabilities)
	if handshake.Capabilities.ValidateFor(connectionFeatures) != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "invalid capabilities")
		return
	}
	if !handshake.Capabilities.HealthEnabled {
		_ = conn.Close(websocket.StatusPolicyViolation, "health reporting is disabled")
		return
	}
	caps, err := s.st.BeginRemoteConnectionFor(uid, agent.ID, handshake.AgentInstanceID, handshake.Capabilities)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "connection rejected")
		return
	}
	reply, err := terminalproto.EncodeHandshakeReply(terminalproto.HandshakeReply{
		ProtocolVersion: terminalproto.Version,
		RequestID:       handshake.RequestID,
		Generation:      caps.Generation,
	})
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "connection rejected")
		return
	}
	var writeMu sync.Mutex
	write := func(ctx context.Context, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
		defer writeCancel()
		return conn.Write(writeCtx, websocket.MessageText, payload)
	}
	if err := write(r.Context(), reply); err != nil {
		return
	}
	send := func(ctx context.Context, message terminalproto.Message) error {
		payload, err := terminalproto.Encode(message)
		if err != nil {
			return err
		}
		return write(ctx, payload)
	}
	hubConn, err := s.healthHub.Connect(uid, agent.ID, caps.Generation, handshake.AgentInstanceID, caps.Capabilities, send)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "connection rejected")
		return
	}
	defer hubConn.Close()
	peer, err := s.terminals.Connect(uid, agent.ID, caps.Generation, s.terminalEnabled() && caps.Capabilities.TerminalControl && caps.Capabilities.TerminalEnabled && caps.Capabilities.Readiness == terminalproto.Ready, terminalbroker.Sender(send))
	if err != nil {
		return
	}
	defer peer.Close()
	// Re-read after registration: a live start/stop can race the initial
	// capability calculation and its reset snapshot.
	peer.Capabilities(s.terminalEnabled() && caps.Capabilities.TerminalControl && caps.Capabilities.TerminalEnabled && caps.Capabilities.Readiness == terminalproto.Ready)
	peer.SetTrafficObserver(hubConn.ReportTraffic)
	peerCtx, peerCancel := context.WithCancel(r.Context())
	var peerWorkers sync.WaitGroup
	defer func() { peerCancel(); peerWorkers.Wait() }()
	if s.terminalEnabled() && caps.Capabilities.TerminalControl {
		peerWorkers.Add(1)
		go func() { defer peerWorkers.Done(); _ = peer.Synchronize(peerCtx) }()
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-hubConn.Done():
			conn.CloseNow()
		case <-peer.Done():
			conn.CloseNow()
		case <-done:
		}
	}()
	for {
		kind, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		if kind != websocket.MessageText {
			return
		}
		message, err := terminalproto.Decode(data)
		if err != nil || message.Generation != caps.Generation {
			return
		}
		switch body := message.Body.(type) {
		case *terminalproto.HealthReport:
			if body.AgentInstanceID != handshake.AgentInstanceID {
				return
			}
			hubConn.Report(*body, time.Now())
		case *terminalproto.ProbeResult:
			hubConn.ProbeResult(*body, time.Now())
		case *terminalproto.Hello:
			if body.Capabilities.ValidateFor(connectionFeatures) != nil {
				return
			}
			if body.AgentInstanceID != handshake.AgentInstanceID {
				return
			}
			if err := s.st.UpdateRemoteCapabilitiesFor(uid, agent.ID, body.AgentInstanceID, caps.Generation, body.Capabilities); err != nil {
				return
			}
			hubConn.UpdateCapabilities(body.Capabilities)
			peer.Capabilities(s.terminalEnabled() && body.Capabilities.TerminalControl && body.Capabilities.TerminalEnabled && body.Capabilities.Readiness == terminalproto.Ready)
			if s.terminalEnabled() && body.Capabilities.TerminalControl {
				peerWorkers.Add(1)
				go func() { defer peerWorkers.Done(); _ = peer.Synchronize(peerCtx) }()
			}
		case *terminalproto.Result, *terminalproto.Reconcile, *terminalproto.SessionChanged, *terminalproto.WorktreeState, *terminalproto.WorktreeResult,
			*terminalproto.BrowseResult, *terminalproto.BranchesResult, *terminalproto.ModelsResult:
			// A browse or branches answer that comes after its request gave
			// up is dropped by the broker; it does not end the connection.
			if !s.terminalEnabled() {
				continue
			}
			if err := peer.Receive(message); err != nil {
				return
			}
		default:
			return
		}
	}
}
