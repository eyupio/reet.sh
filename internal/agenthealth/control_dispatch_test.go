package agenthealth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// controlPair connects a client (the connector side) to a server peer (the
// controller side) over a real websocket.
func controlPair(t *testing.T) (client, peer *websocket.Conn) {
	t.Helper()
	peers := make(chan *websocket.Conn, 1)
	finish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		peers <- c
		<-finish
	}))
	t.Cleanup(func() { close(finish); server.Close() })
	client, _, err := websocket.Dial(t.Context(), strings.Replace(server.URL, "http:", "ws:", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.CloseNow() })
	return client, <-peers
}

func controlMessage(t *testing.T, id string, body terminalproto.Body) []byte {
	t.Helper()
	data, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: id, Generation: 1, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func clientSender(c *websocket.Conn) Sender {
	var mu sync.Mutex
	return func(ctx context.Context, m terminalproto.Message) error {
		data, err := terminalproto.Encode(m)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		return c.Write(ctx, websocket.MessageText, data)
	}
}

// A terminal request that takes a long time must not stop the connector
// answering the controller's probes, or the controller marks the machine
// unreachable and disconnects it.
func TestProbeIsAnsweredWhileATerminalRequestRuns(t *testing.T) {
	client, peer := controlPair(t)
	release := make(chan struct{})
	started := make(chan struct{})
	var order []string
	var mu sync.Mutex
	terminal := func(ctx context.Context, m terminalproto.Message) error {
		mu.Lock()
		order = append(order, m.RequestID)
		first := len(order) == 1
		mu.Unlock()
		if first {
			close(started)
			select {
			case <-release:
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
			}
		}
		return nil
	}
	errorsSeen := make(chan error, 3)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		readControlWithTerminal(ctx, client, 1, clientSender(client), errorsSeen, terminal)
	}()
	ids := []string{"request-aaaaaaaaaaaa", "request-bbbbbbbbbbbb", "request-cccccccccccc"}
	for _, id := range ids {
		if err := peer.Write(t.Context(), websocket.MessageText, controlMessage(t, id, &terminalproto.Reconcile{})); err != nil {
			t.Fatal(err)
		}
	}
	<-started
	probe := terminalproto.Probe{Sequence: 1, Nonce: "nonce-0123456789ab"}
	if err := peer.Write(t.Context(), websocket.MessageText, controlMessage(t, "request-probe-00001", &probe)); err != nil {
		t.Fatal(err)
	}
	readCtx, readCancel := context.WithTimeout(t.Context(), time.Second)
	_, data, err := peer.Read(readCtx)
	readCancel()
	if err != nil {
		t.Fatal("no probe answer while a terminal request was running", err)
	}
	reply, err := terminalproto.Decode(data)
	if r, ok := reply.Body.(*terminalproto.ProbeResult); err != nil || !ok || terminalproto.Probe(*r) != probe {
		t.Fatal("probe answer", reply, err)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(order)
		mu.Unlock()
		if n == len(ids) || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if strings.Join(got, ",") != strings.Join(ids, ",") {
		t.Fatal("terminal requests were not handled once each, in order", got)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the control reader did not stop")
	}
}

// A terminal handler error still ends the connection, as before.
func TestTerminalHandlerErrorEndsTheConnection(t *testing.T) {
	client, peer := controlPair(t)
	errorsSeen := make(chan error, 3)
	failure := errors.New("handler failed")
	go readControlWithTerminal(t.Context(), client, 1, clientSender(client), errorsSeen, func(context.Context, terminalproto.Message) error { return failure })
	if err := peer.Write(t.Context(), websocket.MessageText, controlMessage(t, "request-aaaaaaaaaaaa", &terminalproto.Reconcile{})); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errorsSeen:
		if !errors.Is(err, failure) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler error was not reported")
	}
}

// The queue in front of the terminal handler is bounded: a controller that
// floods a stuck handler ends the connection instead of growing memory.
func TestTerminalQueueIsBounded(t *testing.T) {
	client, peer := controlPair(t)
	errorsSeen := make(chan error, 3)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		readControlWithTerminal(ctx, client, 1, clientSender(client), errorsSeen, func(ctx context.Context, _ terminalproto.Message) error {
			<-ctx.Done()
			return nil
		})
	}()
	for i := 0; i < terminalQueueSize+2; i++ {
		id := "request-" + strings.Repeat("a", 8) + string(rune('a'+i%26)) + strings.Repeat("b", 4) + string(rune('a'+i/26))
		if peer.Write(t.Context(), websocket.MessageText, controlMessage(t, id, &terminalproto.Reconcile{})) != nil {
			break
		}
	}
	select {
	case err := <-errorsSeen:
		if err == nil {
			t.Fatal("nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("an overflowing queue was not reported")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the control reader did not stop")
	}
}
