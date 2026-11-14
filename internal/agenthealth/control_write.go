package agenthealth

import (
	"context"
	"time"

	"github.com/coder/websocket"
)

// WriteControl writes one message on a machine's control link for a caller,
// and is the only way either end writes there. The caller holds the link's
// write lock.
//
// The link carries every session on the machine, so one caller giving up must
// not end it. The websocket library closes the whole connection when the
// context a write runs under ends, even when it had already ended before the
// write began. A caller that has given up therefore sends nothing, and a write
// that has begun runs under its own limit rather than the caller's. A chat's
// pane abandons its request for the chat's live controls when the person
// leaves it, and on a busy controller that request was often still waiting to
// be sent; a launch-stage note goes with a one-second budget. Each now costs
// only its own message. A write that cannot finish within limit still ends
// the link, which is how a half-open connection is found.
func WriteControl(ctx context.Context, c *websocket.Conn, data []byte, limit time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), limit)
	defer cancel()
	return c.Write(writeCtx, websocket.MessageText, data)
}
