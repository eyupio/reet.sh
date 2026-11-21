package terminalbroker

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var ErrPreviewUnavailable = errors.New("This image preview is unavailable or has expired.")
var previewReads = make(chan struct{}, 4)

// previewShare is how many of the preview slots one account may hold, so a
// slow machine of one account cannot keep every slot from the others.
const previewShare = 2

var previewAccounts = struct {
	sync.Mutex
	slots map[int64]*previewSlots
}{slots: map[int64]*previewSlots{}}

type previewSlots struct {
	held chan struct{}
	refs int
}

// acquirePreview takes one of the account's slots, then a shared one. The
// returned release must be called once.
func acquirePreview(ctx context.Context, uid int64) (func(), error) {
	previewAccounts.Lock()
	a := previewAccounts.slots[uid]
	if a == nil {
		a = &previewSlots{held: make(chan struct{}, previewShare)}
		previewAccounts.slots[uid] = a
	}
	a.refs++
	previewAccounts.Unlock()
	forget := func() {
		previewAccounts.Lock()
		if a.refs--; a.refs == 0 {
			delete(previewAccounts.slots, uid)
		}
		previewAccounts.Unlock()
	}
	select {
	case a.held <- struct{}{}:
	case <-ctx.Done():
		forget()
		return nil, ctx.Err()
	}
	select {
	case previewReads <- struct{}{}:
	case <-ctx.Done():
		<-a.held
		forget()
		return nil, ctx.Err()
	}
	return func() { <-previewReads; <-a.held; forget() }, nil
}

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
	release, err := acquirePreview(ctx, uid)
	if err != nil {
		return nil, "", err
	}
	defer release()
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
