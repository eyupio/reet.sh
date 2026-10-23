package terminalproto

import (
	"bytes"
	"strings"
	"testing"
)

func testHandshake() Handshake {
	return Handshake{Version, testID, testID, testCapabilities()}
}

func TestHandshakeRoundTrip(t *testing.T) {
	data, err := EncodeHandshake(testHandshake())
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeHandshake(data)
	if err != nil || got.Validate() != nil || got.AgentInstanceID != testID || got.RequestID != testID || !got.Capabilities.TerminalEnabled {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	reply, err := EncodeHandshakeReply(HandshakeReply{Version, testID, 7})
	if err != nil {
		t.Fatal(err)
	}
	gotReply, err := DecodeHandshakeReply(reply)
	if err != nil || gotReply.Generation != 7 || gotReply.RequestID != testID {
		t.Fatalf("reply round trip: %+v %v", gotReply, err)
	}
}

func TestHandshakeHealthOnlyWithoutTerminalOptIn(t *testing.T) {
	h := testHandshake()
	h.Capabilities.TerminalEnabled = false
	h.Capabilities.HealthEnabled = true
	h.Capabilities.Readiness = DisabledLocally
	data, err := EncodeHandshake(h)
	if err != nil {
		t.Fatalf("health-only handshake rejected: %v", err)
	}
	if _, err := DecodeHandshake(data); err != nil {
		t.Fatalf("health-only handshake decode: %v", err)
	}
}

func TestHandshakeRejectsMalformedInput(t *testing.T) {
	valid, err := EncodeHandshake(testHandshake())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"invalid UTF-8", append(append([]byte{}, valid...), 0xff)},
		{"oversized", bytes.Repeat([]byte("x"), MaxControlBytes+1)},
		{"unknown field", []byte(strings.Replace(string(valid), `"requestId"`, `"unknown":1,"requestId"`, 1))},
		{"duplicate field", []byte(strings.Replace(string(valid), `"requestId"`, `"requestId":"`+testID+`","requestId"`, 1))},
		{"trailing value", append(append([]byte{}, valid...), []byte(` {}`)...)},
		{"null", []byte(`null`)},
		{"wrong protocol", []byte(strings.Replace(string(valid), `"protocolVersion":1`, `"protocolVersion":2`, 1))},
		{"missing instance", []byte(strings.Replace(string(valid), `"agentInstanceId":"`+testID+`"`, `"agentInstanceId":""`, 1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeHandshake(tc.data); err == nil {
				t.Fatal("malformed handshake accepted")
			}
		})
	}
	if _, err := DecodeHandshake([]byte(`{"protocolVersion":1,"requestId":"` + testID + `","agentInstanceId":"` + testID + `","capabilities":{"protocolVersion":1,"os":"plan9","osUser":"tester","terminalEnabled":false,"healthEnabled":true,"readiness":"disabled_locally"}}`)); err == nil {
		t.Fatal("unsupported OS accepted")
	}
}

func TestHandshakeReplyRejectsInvalidGeneration(t *testing.T) {
	for _, g := range []int64{0, -1, MaxCounter + 1} {
		if _, err := EncodeHandshakeReply(HandshakeReply{Version, testID, g}); err == nil {
			t.Fatalf("generation %d accepted", g)
		}
	}
	if _, err := DecodeHandshakeReply([]byte(`{"protocolVersion":1,"requestId":"short","generation":1}`)); err == nil {
		t.Fatal("short request id accepted")
	}
}
