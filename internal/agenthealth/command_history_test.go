package agenthealth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestCommandHistoryCapabilityIsOmittedWithoutNegotiation(t *testing.T) {
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	for _, negotiated := range []bool{false, true} {
		caps := capabilities(AgentConfig{TerminalSession: factory, commandHistory: negotiated})
		data, err := json.Marshal(caps)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "commandHistory") != negotiated || caps.ProtocolVersion != 1 {
			t.Fatal("legacy handshake changed", string(data))
		}
	}
}
