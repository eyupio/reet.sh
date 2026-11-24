//go:build linux || darwin

package terminalhost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ErrLocked refuses work because the machine is locked there (reet agent
// lock). It is a kind of ErrTerminalDisabled, so it is answered denied where
// no locked answer is negotiated.
var ErrLocked = fmt.Errorf("this machine is locked: %w", ErrTerminalDisabled)

// lockCutEvery is how often an attachment checks for the lock on its own, so
// it is cut within a second even while a reconcile holds the manager.
const lockCutEvery = 250 * time.Millisecond

func (m *Manager) isLocked() bool { return m.locked != nil && m.locked() }

// open is allowed for work that starts, attaches, types or reads: it is
// refused while the machine is locked as well as with terminal access off.
// The caller holds m.mu.
func (m *Manager) open() error {
	if err := m.allowed(); err != nil {
		return err
	}
	if m.isLocked() {
		return ErrLocked
	}
	return nil
}

// cut reports whether a live attachment must close: terminal access is off
// or the machine is locked. It takes no lock.
func (m *Manager) cut() bool { return m.consent() != nil || m.isLocked() }

// watchCut closes a once the machine is locked or its terminal access is
// switched off, without waiting for the manager.
func (m *Manager) watchCut(a *Attachment) {
	t := time.NewTicker(lockCutEvery)
	defer t.Stop()
	for {
		select {
		case <-a.done:
			return
		case <-t.C:
			if m.cut() {
				_ = a.Close()
				return
			}
		}
	}
}

// whileUnlocked fails a read once the machine is locked, so an upload that
// was streaming when the lock was taken stops at its next read.
func (m *Manager) whileUnlocked(r io.Reader) io.Reader { return lockedReader{r, m} }

type lockedReader struct {
	r io.Reader
	m *Manager
}

func (l lockedReader) Read(p []byte) (int, error) {
	if l.m.isLocked() {
		return 0, ErrLocked
	}
	return l.r.Read(p)
}

// Running counts the sessions the manager has that run or are starting.
func (m *Manager) Running(ctx context.Context) (int, error) {
	ids, err := m.runningIDs(ctx)
	return len(ids), err
}

func (m *Manager) runningIDs(ctx context.Context) ([]string, error) {
	sessions, err := m.List(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, s := range sessions {
		if s.State == terminalproto.Running || s.State == terminalproto.Creating {
			ids = append(ids, s.ID)
		}
	}
	return ids, nil
}

// EndAll ends every running session as the controller's end does (locked or
// not), and reports how many it ended. It is for reet agent lock
// --end-sessions on the machine.
func (m *Manager) EndAll(ctx context.Context) (int, error) {
	ids, err := m.runningIDs(ctx)
	if err != nil {
		return 0, err
	}
	ended := 0
	var last error
	for _, id := range ids {
		request, err := randomRequestID()
		if err != nil {
			return ended, err
		}
		if _, err := m.End(ctx, request, id); err != nil {
			last = err
			continue
		}
		ended++
	}
	return ended, last
}

func randomRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "end-all-" + hex.EncodeToString(b), nil
}
