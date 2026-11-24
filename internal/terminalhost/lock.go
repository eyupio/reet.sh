//go:build linux || darwin

package terminalhost

import (
	"fmt"
	"io"
	"time"
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
