package terminalbroker

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func BenchmarkPublishChatEvent(b *testing.B) {
	log := newChatLog()
	event := terminalproto.ACPEvent{Kind: "agent", Text: "a short streamed chunk"}
	b.ResetTimer()
	for range b.N {
		log.publish(event)
	}
}
