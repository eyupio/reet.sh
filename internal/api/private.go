package api

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// privateTransport is the lazily started controller listener. Tests replace it
// to exercise bootstrap responses without starting a relay or network stack.
type privateTransport interface {
	Ensure(context.Context) (string, error)
	Close() error
}

func (s *server) privateCredentials() bool {
	if s.authMode == AuthAccounts {
		return true
	}
	return s.authMode == AuthToken && s.token != ""
}

func (s *server) configurePrivate() {
	if s.privateSealer == nil || !s.privateCredentials() {
		return
	}
	s.private = privateconn.NewRuntime(privateconn.RuntimeConfig{
		Sealer:      s.privateSealer,
		RelayMapURL: s.privateRelayURL,
		Handler:     s.privateHandler(),
		LoadIdentity: func() (int, string, bool, error) {
			record, found, err := s.st.PrivateIdentity()
			return record.FormatVersion, record.Ciphertext, found, err
		},
		SaveIdentity: func(formatVersion int, ciphertext string) error {
			return s.st.SavePrivateIdentityIfAbsent(store.PrivateIdentityRecord{FormatVersion: formatVersion, Ciphertext: ciphertext})
		},
		Logf: func(format string, args ...any) {
			log.Printf("private connection: "+format, args...)
		},
	})
	s.healthHub.Start()
}

// privateBootstrap returns the controller capability to an authenticated
// enrollment over ordinary HTTPS. The response is never part of a browser
// machine payload and must not be cached.
func (s *server) privateBootstrap(w http.ResponseWriter, r *http.Request) {
	if !s.privateCredentials() {
		writeErr(w, http.StatusForbidden, "private connections require an authenticated instance")
		return
	}
	if s.private == nil {
		writeErr(w, http.StatusServiceUnavailable, "private connections are not configured on this instance")
		return
	}
	if _, ok := s.agentIdentity(w, r); !ok {
		return
	}
	address, err := s.private.Ensure(r.Context())
	if err != nil {
		// Neither the capability nor upstream diagnostics are logged here.
		log.Printf("private bootstrap failed")
		writeErr(w, http.StatusServiceUnavailable, "private connection is unavailable; try again when the controller relay is reachable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(w, http.StatusOK, map[string]any{"protocolVersion": terminalproto.Version, "address": address})
}

// privateHandler is the complete route allowlist served by the Tailcat
// listener. It is not mounted on the public mux. Anything other than the exact
// handshake path and method is absent, and no query string is accepted.
func (s *server) privateHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !privateconn.IsPrivateTransport(r.Context()) || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/private/v1/hello":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			s.privateHello(w, r)
		case "/private/v1/control":
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			s.privateControl(w, r)
		case "/private/v1/terminal/data":
			if r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			s.privateTerminalData(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func (s *server) privateHello(w http.ResponseWriter, r *http.Request) {
	if !s.privateCredentials() {
		writeErr(w, http.StatusForbidden, "private connections require an authenticated instance")
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeErr(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	uid, agent, err := s.st.AuthenticateRemoteAgent(r.Header.Get("X-Reet-Agent"), r.Header.Get("X-Reet-Agent-Key"), r.Header.Get("X-Reet-Version"))
	if err != nil {
		writeErr(w, http.StatusForbidden, "machine enrollment is missing or revoked")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, terminalproto.MaxControlBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil || len(data) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid private message")
		return
	}
	hello, err := terminalproto.DecodeHandshake(data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid private message")
		return
	}
	caps, err := s.st.BeginRemoteConnectionFor(uid, agent.ID, hello.AgentInstanceID, hello.Capabilities)
	switch {
	case err == nil:
	case errors.Is(err, store.ErrRemoteNotFound):
		writeErr(w, http.StatusForbidden, "machine enrollment is missing or revoked")
		return
	case errors.Is(err, store.ErrTerminalConflict):
		writeErr(w, http.StatusConflict, "private connection generation rejected")
		return
	case errors.Is(err, terminalproto.ErrInvalid):
		writeErr(w, http.StatusBadRequest, "invalid private message")
		return
	default:
		log.Printf("private handshake failed for an enrollment")
		writeErr(w, http.StatusInternalServerError, "private connection is unavailable")
		return
	}
	reply, err := terminalproto.EncodeHandshakeReply(terminalproto.HandshakeReply{
		ProtocolVersion: terminalproto.Version,
		RequestID:       hello.RequestID,
		Generation:      caps.Generation,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "private connection is unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(reply)
}
