package terminalbroker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func bridgeFixture(t *testing.T, flood bool) (*Broker, *Peer, *websocket.Conn, <-chan error, *atomic.Bool) {
	t.Helper()
	b, p, uid, agent := fixture(t)
	s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Stream", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	dataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		_, data, err := c.Read(r.Context())
		if err != nil {
			return
		}
		h, err := terminalproto.DecodeStreamHello(data)
		if err != nil {
			return
		}
		_ = b.Bind(r.Context(), uid, agent, h, c)
	}))
	t.Cleanup(dataServer.Close)
	original := p.send
	p.send = func(ctx context.Context, m terminalproto.Message) error {
		if a, ok := m.Body.(terminalproto.Attach); ok {
			c, _, err := websocket.Dial(t.Context(), dataServer.URL, nil)
			if err != nil {
				return err
			}
			h := terminalproto.StreamHello{Version: 1, RequestID: m.RequestID, SessionID: m.SessionID, AttachmentID: m.AttachmentID, Generation: m.Generation, AttachmentGeneration: m.AttachmentGeneration, Token: a.StreamToken, Operation: "attach"}
			data, _ := json.Marshal(h)
			if err = c.Write(ctx, websocket.MessageText, data); err != nil {
				c.CloseNow()
				return err
			}
			go func() {
				defer c.CloseNow()
				if flood {
					buf := make([]byte, terminalproto.MaxBinaryBytes)
					for range 1024 {
						if write(t.Context(), c, websocket.MessageBinary, buf) != nil {
							return
						}
					}
				} else {
					_, _, _ = c.Read(t.Context())
				}
			}()
		}
		return original(ctx, m)
	}
	ticket, err := b.Ticket(uid, "login", s.ID, Options{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	authorized := &atomic.Bool{}
	authorized.Store(true)
	done := make(chan error, 1)
	browserServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		done <- b.Serve(r.Context(), uid, "login", ticket, c, authorized.Load)
	}))
	t.Cleanup(browserServer.Close)
	browser, _, err := websocket.Dial(t.Context(), browserServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { browser.CloseNow(); b.Close() })
	return b, p, browser, done, authorized
}
func TestSlowConsumerClosesBoundedBridge(t *testing.T) {
	_, _, _, done, _ := bridgeFixture(t, true)
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("slow browser kept unbounded attachment alive")
	}
}
func TestRevokedLoginStopsLeaseRenewal(t *testing.T) {
	_, _, c, done, authorized := bridgeFixture(t, false)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	_, _, err := c.Read(ctx)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	authorized.Store(false)
	select {
	case <-done:
	case <-time.After(7 * time.Second):
		t.Fatal("revoked browser continued renewing lease")
	}
}
func TestTerminalPartitionClosesBothBridgeDirections(t *testing.T) {
	_, p, c, done, _ := bridgeFixture(t, false)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	_, _, err := c.Read(ctx)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	p.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("control partition kept data attachment alive")
	}
}
