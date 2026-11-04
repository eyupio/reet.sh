package terminalproto

import (
	"encoding/json"
	"unicode/utf8"
)

// Handshake is the first authenticated message over a new private connection.
// The controller assigns the connection generation in HandshakeReply, so the
// request cannot choose one. A handshake registers capabilities; it does not
// authorize a session or prove that a terminal is usable.
type Handshake struct {
	ProtocolVersion int          `json:"protocolVersion"`
	RequestID       string       `json:"requestId"`
	AgentInstanceID string       `json:"agentInstanceId"`
	Capabilities    Capabilities `json:"capabilities"`
}

func (h Handshake) Validate() error {
	validIDs := ValidID(h.RequestID) && ValidID(h.AgentInstanceID)
	valid := h.ProtocolVersion == Version && validIDs && h.Capabilities.Validate() == nil
	if !valid {
		return ErrInvalid
	}
	return nil
}

// HandshakeReply echoes the request ID and carries the controller-assigned
// fencing generation. Later control messages must use that generation.
type HandshakeReply struct {
	ProtocolVersion int    `json:"protocolVersion"`
	RequestID       string `json:"requestId"`
	Generation      int64  `json:"generation"`
}

func (h HandshakeReply) Validate() error {
	valid := h.ProtocolVersion == Version && ValidID(h.RequestID) && ValidCounter(h.Generation)
	if !valid {
		return ErrInvalid
	}
	return nil
}

func EncodeHandshake(h Handshake) ([]byte, error) {
	if h.Validate() != nil {
		return nil, ErrInvalid
	}
	b, err := encodeStrict(h)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func DecodeHandshake(data []byte) (Handshake, error) {
	var h Handshake
	if err := decodeStrict(data, &h); err != nil {
		return Handshake{}, err
	}
	if h.Validate() != nil {
		return Handshake{}, ErrInvalid
	}
	return h, nil
}

func EncodeHandshakeReply(h HandshakeReply) ([]byte, error) {
	if h.Validate() != nil {
		return nil, ErrInvalid
	}
	return encodeStrict(h)
}

func DecodeHandshakeReply(data []byte) (HandshakeReply, error) {
	var h HandshakeReply
	if err := decodeStrict(data, &h); err != nil {
		return HandshakeReply{}, err
	}
	if h.Validate() != nil {
		return HandshakeReply{}, ErrInvalid
	}
	return h, nil
}

func encodeStrict(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil || len(b) > MaxControlBytes {
		return nil, ErrInvalid
	}
	return b, nil
}

func decodeStrict(data []byte, v any) error {
	validData := len(data) > 0 && len(data) <= MaxControlBytes && utf8.Valid(data)
	if !validData || !uniqueKeys(data) {
		return ErrInvalid
	}
	if strictJSON(data, v) != nil {
		return ErrInvalid
	}
	return nil
}
