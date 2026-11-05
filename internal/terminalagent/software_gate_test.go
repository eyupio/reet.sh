//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSoftwareGateWriteWaitsForLaunch(t *testing.T) {
	releaseRead, err := lockSoftwareRead(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			releaseRead()
		}
	}()

	writeErr := make(chan error, 1)
	go func() {
		releaseWrite, err := lockSoftwareWrite(context.Background())
		if err == nil {
			releaseWrite()
		}
		writeErr <- err
	}()

	// Wait for the writer to queue. Once it has, a new launch waits behind it.
	deadline := time.Now().Add(2 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		release, err := lockSoftwareRead(ctx)
		cancel()
		if err != nil {
			break
		}
		release()
		if time.Now().After(deadline) {
			t.Fatal("writer never took its place in the queue")
		}
		time.Sleep(time.Millisecond)
	}

	select {
	case err := <-writeErr:
		t.Fatalf("writer acquired while a launch was open: %v", err)
	default:
	}

	releaseRead()
	released = true
	select {
	case err := <-writeErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("writer did not acquire after the launch released")
	}

	release, err := lockSoftwareRead(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestSoftwareGateWriteIsCancelable(t *testing.T) {
	releaseRead, err := lockSoftwareRead(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer releaseRead()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	release, err := lockSoftwareWrite(ctx)
	if err == nil {
		release()
		t.Fatal("write acquired while a launch was open")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestSoftwareGateReadIsCancelable(t *testing.T) {
	releaseWrite, err := lockSoftwareWrite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer releaseWrite()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	release, err := lockSoftwareRead(ctx)
	if err == nil {
		release()
		t.Fatal("launch acquired while an apply was open")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}
