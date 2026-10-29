package terminalbroker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// ErrUploadUnsupported means the machine did not negotiate upload-v1 (an
	// older reet there); nothing was sent.
	ErrUploadUnsupported = errors.New("machine does not support image uploads")
	// ErrUploadsOff means a current machine has image uploads switched off.
	ErrUploadsOff = errors.New("machine has image uploads switched off")
	// ErrUploadTooLarge means the image is outside the per-file limit.
	ErrUploadTooLarge = errors.New("image is too large to upload")
	// ErrUploadType means the machine did not accept the image's type.
	ErrUploadType = errors.New("image type is not supported")
	// ErrUploadQuota means the session or machine has no room for more images.
	ErrUploadQuota = errors.New("image upload quota is full")
	// ErrUploadBusy means the machine is already serving two uploads on this
	// connection, or a third stream could not be reserved.
	ErrUploadBusy = errors.New("machine is already accepting images")
	// ErrUploadTimeout means the transfer did not finish by its deadline. A
	// late stream or result is dropped, never retried.
	ErrUploadTimeout = errors.New("image upload timed out")
	// ErrUploadMismatch means the stream or its hash did not match the
	// declared size and type.
	ErrUploadMismatch = errors.New("image upload did not match its declaration")
	// ErrUploadFailed means the machine reported a failure or the stream ended
	// without a usable result.
	ErrUploadFailed = errors.New("image upload failed")
)

// uploadTransferTimeout bounds the reverse stream after the machine accepted
// the request. Five MiB over a normal link leaves a wide margin.
var uploadTransferTimeout = 30 * time.Second

func supportedUploadMIME(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

// uploadCallError maps a failed control call. A lost reply, like a deadline,
// means the machine may have accepted the upload: it is a timeout, not a
// plain failure, and is never retried.
func uploadCallError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrUploadTimeout
	}
	var lost errReplyLost
	if errors.As(err, &lost) {
		return ErrUploadTimeout
	}
	if errors.Is(err, ErrOffline) {
		return ErrOffline
	}
	return ErrUploadFailed
}

// Upload streams one image to the owner's running session on the machine and
// returns the absolute path the machine stored it at. It never retries and
// never replays bytes: a timeout or disconnect drops the expectation, so a
// late stream is refused.
func (b *Broker) Upload(ctx context.Context, uid int64, sessionID string, data []byte, mime string) (string, error) {
	if !b.start() {
		return "", ErrOffline
	}
	defer b.wg.Done()
	if !terminalproto.ValidID(sessionID) {
		return "", ErrDenied
	}
	if len(data) < 1 || len(data) > terminalproto.MaxUploadBytes {
		return "", ErrUploadTooLarge
	}
	if !supportedUploadMIME(mime) {
		return "", ErrUploadType
	}
	s, err := b.st.TerminalSessionFor(uid, sessionID)
	if err != nil || s.State != terminalproto.Running {
		return "", ErrDenied
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return "", err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, s.AgentID)
	if err != nil || caps.Generation != p.Generation {
		return "", ErrOffline
	}
	if !caps.Capabilities.Uploads {
		// A connector that negotiated upload-v1 but has the switch off says
		// so; without either, it is an older reet and never sees the request.
		if caps.Capabilities.UploadsOff {
			return "", ErrUploadsOff
		}
		return "", ErrUploadUnsupported
	}
	h, e, err := p.expect(sessionID, ID(), terminalproto.UploadOperation)
	if err != nil {
		return "", ErrUploadBusy
	}
	defer p.forget(h)
	defer e.close()
	sum := sha256.Sum256(data)
	req := terminalproto.Upload{StreamToken: h.Token, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), MIME: mime}
	reply, err := p.call(ctx, message(h, req))
	if err != nil {
		return "", uploadCallError(err)
	}
	if r, ok := reply.Body.(*terminalproto.Result); ok {
		switch r.Code {
		case terminalproto.OK:
		case terminalproto.UploadsOff:
			return "", ErrUploadsOff
		case terminalproto.UploadBusy:
			return "", ErrUploadBusy
		case terminalproto.NotFound, terminalproto.Denied:
			return "", ErrDenied
		default:
			return "", ErrUploadFailed
		}
	} else {
		return "", ErrUploadFailed
	}
	transferCtx, cancel := context.WithTimeout(ctx, uploadTransferTimeout)
	defer cancel()
	var c *websocket.Conn
	select {
	case c = <-e.conn:
	case <-transferCtx.Done():
		return "", ErrUploadTimeout
	case <-p.done:
		return "", ErrOffline
	}
	defer c.CloseNow()
	c.SetReadLimit(terminalproto.MaxControlBytes)
	for offset := 0; offset < len(data); offset += terminalproto.MaxBinaryBytes {
		end := offset + terminalproto.MaxBinaryBytes
		if end > len(data) {
			end = len(data)
		}
		if err := write(transferCtx, c, websocket.MessageBinary, data[offset:end]); err != nil {
			select {
			case <-p.done:
				return "", ErrOffline
			default:
			}
			if transferCtx.Err() != nil {
				return "", ErrUploadTimeout
			}
			return "", ErrUploadFailed
		}
		p.recordTraffic(end-offset, 0)
	}
	kind, frame, err := c.Read(transferCtx)
	if err != nil {
		select {
		case <-p.done:
			return "", ErrOffline
		default:
		}
		if transferCtx.Err() != nil {
			return "", ErrUploadTimeout
		}
		return "", ErrUploadFailed
	}
	if kind != websocket.MessageText || len(frame) > terminalproto.MaxControlBytes {
		return "", ErrUploadFailed
	}
	result, err := terminalproto.DecodeUploadResult(frame)
	if err != nil {
		return "", ErrUploadFailed
	}
	_ = c.Close(websocket.StatusNormalClosure, "")
	switch result.Code {
	case terminalproto.UploadOK:
		return result.Path, nil
	case terminalproto.UploadDisabled:
		return "", ErrUploadsOff
	case terminalproto.UploadTooLarge:
		return "", ErrUploadTooLarge
	case terminalproto.UploadType:
		return "", ErrUploadType
	case terminalproto.UploadQuota:
		return "", ErrUploadQuota
	case terminalproto.UploadMismatch:
		return "", ErrUploadMismatch
	case terminalproto.UploadConflict:
		return "", ErrUploadBusy
	}
	return "", ErrUploadFailed
}
