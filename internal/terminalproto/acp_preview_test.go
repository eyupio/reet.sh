package terminalproto

import (
	"strings"
	"testing"
)

func TestPreviewNegotiationAndWireBounds(t *testing.T) {
	id := strings.Repeat("a", 64)
	ref := ACPPreviewRef{ID: id, Name: "Screenshot.png", MIME: "image/png", Size: 100}
	for _, body := range []Body{ACPEvent{Kind: "preview", Preview: &ref}, ACPPreview{ID: id}, ACPPreviewResult{ID: id, Size: MaxACPPreviewChunk, MIME: "image/png", Data: make([]byte, MaxACPPreviewChunk)}} {
		raw, err := Encode(Message{Version: Version, RequestID: "request-preview0123456789", SessionID: "session-preview0123456789", Generation: 1, Body: body})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = Decode(raw); err != nil {
			t.Fatal(err)
		}
	}
	if (ACPPreview{ID: "../../other"}).valid() || (ACPPreviewResult{ID: id, Size: MaxACPPreviewBytes + 1, MIME: "image/png", Data: []byte{1}}).valid() {
		t.Fatal("invalid preview bounds accepted")
	}
	if (ACPEvent{Kind: "agent", MessageID: "message-preview", Preview: &ref}).valid() {
		t.Fatal("preview fields on text event")
	}
	c := Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "dev", TerminalEnabled: true, TerminalControl: true, ACP: true, ACPPreviews: true, Readiness: Ready}
	if c.ValidateFor(ACPFeature) == nil {
		t.Fatal("unnegotiated previews")
	}
	if err := c.ValidateFor(ACPFeature + "," + ACPPreviewsFeature); err != nil {
		t.Fatal(err)
	}
}
