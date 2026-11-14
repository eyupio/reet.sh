package agenthealth

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// A caller that has given up before its message goes out sends nothing and
// leaves the link up. Before, the websocket library closed the whole
// connection about one time in four in this case.
func TestWriteControlForACallerThatGaveUpLeavesTheLinkUp(t *testing.T) {
	client, peer := controlPair(t)
	gaveUp, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 40; i++ {
		if err := WriteControl(gaveUp, client, []byte(`{"gaveUp":true}`), time.Second); !errors.Is(err, context.Canceled) {
			t.Fatalf("write %d for a caller that gave up: %v, want context.Canceled", i, err)
		}
	}
	if err := WriteControl(t.Context(), client, []byte(`{"after":true}`), time.Second); err != nil {
		t.Fatalf("the link was ended by callers that gave up: %v", err)
	}
	ctx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	if _, data, err := peer.Read(ctx); err != nil || string(data) != `{"after":true}` {
		t.Fatalf("the peer read %q, %v; want only the message whose caller was still waiting", data, err)
	}
}

// A write that has begun runs under its own limit, not the caller's: a caller
// whose deadline passes in the middle of a slow write gets its message through
// instead of ending the link.
func TestWriteControlOutlivesTheCallersDeadline(t *testing.T) {
	client, peer := controlPair(t)
	peer.SetReadLimit(64 << 20)
	// Larger than the socket buffers, so the write waits for the peer to read.
	big := bytes.Repeat([]byte("x"), 32<<20)
	read := make(chan error, 1)
	go func() {
		time.Sleep(300 * time.Millisecond) // until well after the caller's deadline
		_, data, err := peer.Read(t.Context())
		if err == nil && len(data) != len(big) {
			err = errors.New("the message arrived cut short")
		}
		read <- err
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if err := WriteControl(ctx, client, big, 10*time.Second); err != nil {
		t.Fatalf("a write that began before the caller's deadline was cut off by it: %v", err)
	}
	if err := <-read; err != nil {
		t.Fatalf("the peer did not read the whole message: %v", err)
	}
	if err := WriteControl(t.Context(), client, []byte(`{"after":true}`), time.Second); err != nil {
		t.Fatalf("the link did not survive the caller's deadline: %v", err)
	}
}
