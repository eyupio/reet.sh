//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// PaneProcesses is refused while terminal access is withheld, as History is.
func TestPaneProcessesRefusedWithoutConsent(t *testing.T) {
	m, id, _ := uploadTestManager(t, terminalproto.Running)
	m.consent = func() error { return errors.New("terminal access is off") }
	if _, err := m.PaneProcesses(context.Background(), id); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatalf("err = %v, want ErrTerminalDisabled", err)
	}
	if _, err := m.History(context.Background(), id, 10); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatalf("History err = %v, want ErrTerminalDisabled", err)
	}
}

func uploadHeld(m *Manager, sessionID string) uploadReservation {
	m.uploadMu.Lock()
	defer m.uploadMu.Unlock()
	return m.uploadReserved[sessionID]
}

// An upload reservation lives exactly as long as its transfer: a session id
// forgotten and created again while an upload to its previous incarnation is
// still streaming counts that transfer only until it ends, and inherits no
// reservation afterwards.
func TestForgetLeavesNoStaleUploadReservation(t *testing.T) {
	m, _ := managerFixture(t)
	ctx := context.Background()
	id := testSessionID(1)
	create := terminalproto.Create{Title: "Build", Cols: 80, Rows: 24}
	if _, err := m.Create(ctx, testRequestID(1), id, create); err != nil {
		t.Fatal(err)
	}
	payload := uploadPNG()
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		_, err := m.WriteUpload(ctx, id, uploadRequest(payload, "image/png"), pr)
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for uploadHeld(m, id).files != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the upload never reserved quota")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if s, err := m.End(ctx, testRequestID(2), id); err != nil || s.State.ReservesQuota() {
		t.Fatal("end", s, err)
	}
	if err := m.Forget(ctx, id); err != nil {
		t.Fatal(err)
	}
	if s, err := m.Create(ctx, testRequestID(3), id, create); err != nil || s.State != terminalproto.Running {
		t.Fatal("create after forget", s, err)
	}
	_ = pw.CloseWithError(errors.New("transfer cancelled"))
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the cancelled upload succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the upload never ended")
	}
	if held := uploadHeld(m, id); held.files != 0 || held.bytes != 0 {
		t.Fatalf("the reused id inherited a reservation: %+v", held)
	}
	writeUpload(t, m, id, payload, "image/png")
	if held := uploadHeld(m, id); held.files != 0 || held.bytes != 0 {
		t.Fatalf("a finished upload left a reservation: %+v", held)
	}
}
