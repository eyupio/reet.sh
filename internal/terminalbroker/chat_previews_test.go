package terminalbroker

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestPreviewChunksVerifyOwnershipOffsetsAndContentHash(t *testing.T) {
	data := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("image"), 16000)...)
	id := fmt.Sprintf("%x", sha256.Sum256(data))
	for _, bad := range []string{"", "offset", "hash"} {
		t.Run(bad, func(t *testing.T) {
			b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPPreviews = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
				if in, ok := m.Body.(*terminalproto.ACPPreview); ok {
					end := min(int(in.Offset)+terminalproto.MaxACPPreviewChunk, len(data))
					part := append([]byte(nil), data[in.Offset:end]...)
					offset := in.Offset
					if bad == "offset" {
						offset++
					}
					if bad == "hash" {
						part[len(part)-1] ^= 1
					}
					return terminalproto.ACPPreviewResult{ID: id, Offset: offset, Size: int64(len(data)) + int64(boolInt(bad == "offset")), MIME: "image/png", Data: part}, true
				}
				return nil, true
			})
			s := chatSession(t, b, uid, agent)
			got, mime, err := b.ChatPreview(t.Context(), uid, s.ID, id)
			if bad != "" {
				if err == nil {
					t.Fatal("corrupt snapshot accepted")
				}
				return
			}
			if err != nil || mime != "image/png" || !bytes.Equal(got, data) {
				t.Fatal("valid preview lost", err)
			}
			if _, _, err = b.ChatPreview(t.Context(), uid+1, s.ID, id); err == nil {
				t.Fatal("foreign account")
			}
		})
	}
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
