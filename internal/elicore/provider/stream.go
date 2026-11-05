// SPDX-License-Identifier: AGPL-3.0-only
// Adapted from eyupio/zoomies, 3c4d09627f78afab72a6575e94624fb0e7710f3f.
// Original source: internal/assistant/provider/. See ../NOTICE.md for provenance.
package provider

import (
	"context"
	"errors"
	"iter"
	"net/http"

	"github.com/eyupio/reet.sh/internal/elicore"
)

// These bounds apply across frames: limiting individual SSE lines alone
// cannot bound a tool call delivered as many small argument fragments.
const (
	maxToolCalls         = 32
	maxToolArgumentBytes = 1 << 20
)

// Decoder turns one server event into the assistant's events: none for a
// keep-alive or a frame the adapter ignores, several when one frame carries
// a delta and the usage, and one with Done when the server is finished.
type Decoder func(ServerEvent) ([]elicore.Event, error)

// NewStream wraps a streaming response. Events are read in the caller's
// goroutine on Next, so there is nothing to leak; cancellation closes the
// body from a context.AfterFunc, which is what makes a blocked read return.
func NewStream(resp *http.Response, decode Decoder) elicore.Stream {
	s := &stream{resp: resp, decode: decode}
	s.next, s.stop = iter.Pull2(ReadEvents(resp.Body))
	return s
}

type stream struct {
	resp   *http.Response
	decode Decoder
	next   func() (ServerEvent, error, bool)
	stop   func()
	queue  []elicore.Event
	ended  bool
}

func (s *stream) Next(ctx context.Context) (elicore.Event, bool) {
	if s.ended {
		return elicore.Event{}, false
	}
	if err := ctx.Err(); err != nil {
		return s.end(elicore.Event{Err: err})
	}
	for len(s.queue) == 0 {
		if err := ctx.Err(); err != nil {
			return s.end(elicore.Event{Err: err})
		}
		// The read below blocks on the body; closing the body is what frees
		// it when the context ends first.
		stopWatch := context.AfterFunc(ctx, func() { s.resp.Body.Close() })
		ev, err, ok := s.next()
		stopWatch()
		if ctx.Err() != nil {
			return s.end(elicore.Event{Err: ctx.Err()})
		}
		if err != nil {
			return s.end(elicore.Event{Err: err})
		}
		if !ok {
			// The server went away before it said it was done. A reply that
			// happens to be short is not the same thing, so this is an error.
			return s.end(elicore.Event{Err: errors.New("the provider closed the stream before it finished answering")})
		}
		events, err := s.decode(ev)
		if err != nil {
			return s.end(elicore.Event{Err: err})
		}
		s.queue = append(s.queue, events...)
	}
	e := s.queue[0]
	s.queue = s.queue[1:]
	if e.Done || e.Err != nil {
		return s.end(e)
	}
	return e, true
}

func (s *stream) end(e elicore.Event) (elicore.Event, bool) {
	s.ended = true
	s.stop()
	s.resp.Body.Close()
	return e, true
}

// Close releases the connection. After the stream ended it is a no-op, and
// before, it drains nothing: a reply nobody is reading is not worth the
// bytes.
func (s *stream) Close() error {
	if !s.ended {
		s.ended = true
		s.stop()
	}
	s.resp.Body.Close()
	return nil
}
