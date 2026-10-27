package agenthealth

import (
	"context"
	"errors"
	"fmt"
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
		readControlWithTerminal(ctx, client, 1, clientSender(client), errorsSeen, "", terminal)
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
	go readControlWithTerminal(t.Context(), client, 1, clientSender(client), errorsSeen, "", func(context.Context, terminalproto.Message) error { return failure })
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

// When the queue in front of a stuck terminal handler is full, the reader
// answers the next request unavailable itself and keeps the connection: the
// controller gives up on a call after ten seconds while the message still
// waits here, so retries during a slow worktree create can fill the queue.
func TestFullTerminalQueueAnswersUnavailableAndKeepsTheConnection(t *testing.T) {
	client, peer := controlPair(t)
	errorsSeen := make(chan error, 3)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	release := make(chan struct{})
	var mu sync.Mutex
	var handled []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		readControlWithTerminal(ctx, client, 1, clientSender(client), errorsSeen, "", func(ctx context.Context, m terminalproto.Message) error {
			mu.Lock()
			handled = append(handled, m.RequestID)
			first := len(handled) == 1
			mu.Unlock()
			if first {
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			return nil
		})
	}()
	id := func(i int) string { return fmt.Sprintf("request-%012d", i) }
	write := func(i int, body terminalproto.Body, session string) {
		t.Helper()
		data, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: id(i), SessionID: session, Generation: 1, Body: body})
		if err != nil {
			t.Fatal(err)
		}
		if err := peer.Write(t.Context(), websocket.MessageText, data); err != nil {
			t.Fatal(err)
		}
	}
	// One message in the handler, terminalQueueSize waiting. The first is
	// written alone and must be held before the rest are written: with one
	// CPU the reader can read a whole burst before the dispatcher runs, and
	// would then find the queue full early.
	write(0, &terminalproto.Reconcile{}, "")
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		mu.Lock()
		n := len(handled)
		mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the handler never took the first message")
		}
	}
	for i := 1; i <= terminalQueueSize; i++ {
		write(i, &terminalproto.Reconcile{}, "")
	}
	read := func() terminalproto.Message {
		t.Helper()
		readCtx, readCancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer readCancel()
		_, data, err := peer.Read(readCtx)
		if err != nil {
			t.Fatal("no answer", err)
		}
		m, err := terminalproto.Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	// The queue is full: a session request and a worktree request are each
	// answered unavailable at once, by the reader.
	overflowA, overflowB := terminalQueueSize+1, terminalQueueSize+2
	write(overflowA, &terminalproto.End{}, "session-0123456789ab")
	if m := read(); m.RequestID != id(overflowA) || m.SessionID != "session-0123456789ab" || *m.Body.(*terminalproto.Result) != (terminalproto.Result{Code: terminalproto.Unavailable}) {
		t.Fatal("overflowing session request", m)
	}
	write(overflowB, &terminalproto.WorktreeStatus{Run: "0123456789abcdef0123", Project: "p", Repo: "site"}, "")
	if m := read(); m.RequestID != id(overflowB) || *m.Body.(*terminalproto.WorktreeResult) != (terminalproto.WorktreeResult{Code: terminalproto.Unavailable}) {
		t.Fatal("overflowing worktree request", m)
	}
	// Probes are still answered and the session goes on.
	write(overflowB+1, &terminalproto.Probe{Sequence: 1, Nonce: "nonce-0123456789ab"}, "")
	if m := read(); m.RequestID != id(overflowB+1) {
		t.Fatal("probe", m)
	}
	select {
	case err := <-errorsSeen:
		t.Fatal("a full queue ended the session", err)
	default:
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(handled)
		mu.Unlock()
		if n == terminalQueueSize+1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	got := append([]string(nil), handled...)
	mu.Unlock()
	if len(got) != terminalQueueSize+1 {
		t.Fatal("queued requests handled", len(got))
	}
	for i, r := range got {
		if r != id(i) {
			t.Fatal("queued requests out of order at", i, r)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the control reader did not stop")
	}
}
