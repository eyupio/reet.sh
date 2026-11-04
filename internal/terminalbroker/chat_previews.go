package terminalbroker

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var ErrPreviewUnavailable = errors.New("This image preview is unavailable or has expired.")
var previewReads = make(chan struct{}, 4)

// ChatPreview reads a connector-captured snapshot belonging to this account and
// session. Every part and the final content hash are checked before serving bytes.
func (b *Broker) ChatPreview(ctx context.Context, uid int64, session, id string) ([]byte, string, error) {
	if !terminalproto.ValidACPPreviewID(id) {
		return nil, "", ErrPreviewUnavailable
	}
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil || !s.Structured {
		return nil, "", ErrPreviewUnavailable
	}
	select {
	case previewReads <- struct{}{}:
		defer func() { <-previewReads }()
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return nil, "", err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, s.AgentID)
	valid := err == nil && caps.Generation == p.Generation && caps.Capabilities.ACPPreviews
	if !valid {
		return nil, "", ErrPreviewUnavailable
	}
	var data []byte
	var size int64
	var mime string
	for {
		in := terminalproto.ACPPreview{ID: id, Offset: int64(len(data))}
		reply, err := p.callWithin(ctx, terminalproto.Message{RequestID: ID(), SessionID: session, Body: &in}, 3*time.Second)
		if err != nil {
			return nil, "", err
		}
		part, ok := reply.Body.(*terminalproto.ACPPreviewResult)
		if !ok {
			return nil, "", ErrPreviewUnavailable
		}
		validPart := part.ID == id && part.Offset == in.Offset
		validSize := part.Size >= 1 && part.Size <= terminalproto.MaxACPPreviewBytes &&
			len(part.Data) > 0 && len(part.Data) <= terminalproto.MaxACPPreviewChunk
		validOffset := in.Offset+int64(len(part.Data)) <= part.Size
		valid := validPart && validSize && validOffset
		if !valid {
			return nil, "", ErrPreviewUnavailable
		}
		if data == nil {
			size, mime = part.Size, part.MIME
			data = make([]byte, 0, int(size))
		} else if part.Size != size || part.MIME != mime {
			return nil, "", ErrPreviewUnavailable
		}
		data = append(data, part.Data...)
		if int64(len(data)) == size {
			break
		}
	}
	detected, _, ok := terminalproto.SniffUploadMIME(data)
	validImage := ok && mime == detected && fmt.Sprintf("%x", sha256.Sum256(data)) == id
	if !validImage {
		return nil, "", ErrPreviewUnavailable
	}
	return data, mime, nil
}
