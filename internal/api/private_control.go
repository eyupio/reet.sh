package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

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
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-hubConn.Done():
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
		default:
			return
		}
	}
}
