package mail

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Dir is the development sender: each message is written to a directory as
// ID.eml (what a server would have been sent) and ID.html (to open in a
// browser). Only sign-in messages are pruned after LinkRetention: the
// directory is for the day's work, not a record.
type Dir struct{ Path string }

// Name implements Sender.
func (d Dir) Name() string { return "outbox" }

// Send implements Sender.
func (d Dir) Send(_ context.Context, m Message) error {
	raw, err := Encode(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d.Path, 0o700); err != nil {
		return fmt.Errorf("outbox: %w", err)
	}
	d.prune(time.Now().Add(-LinkRetention))
	name := m.ID
	if name == "" {
		name = "message"
	}
	if err := os.WriteFile(filepath.Join(d.Path, name+".eml"), raw, 0o600); err != nil {
		return fmt.Errorf("outbox: %w", err)
	}
	return os.WriteFile(filepath.Join(d.Path, name+".html"), []byte(m.HTML), 0o600)
}

// prune removes the outbox's files last written before cutoff.
func (d Dir) prune(cutoff time.Time) {
	entries, err := os.ReadDir(d.Path)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(d.Path, e.Name()))
		}
	}
}

// OutboxWarning is what serve says when the local outbox directory is set on
// an instance that is not on a laptop: the directory holds every sign-in link
// in the clear. It is empty when there is no outbox, or when the instance is a
// loopback address.
func OutboxWarning(c Config, portal string) string {
	if c.OutboxDir == "" || portal == "" {
		return ""
	}
	u, err := url.Parse(portal)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return ""
	}
	return EnvOutboxDir + " is set on an instance served at " + portal + ": every sign-in link it sends is written to " + c.OutboxDir +
		" in the clear. The outbox is for a developer's machine; unset it and save a mail server on Admin, Email."
}

// Fake records what it is given, for tests. Fail, when set, is returned for
// every send until it is cleared.
type Fake struct {
	mu   sync.Mutex
	Sent []Message
	Fail error
}

// Name implements Sender.
func (f *Fake) Name() string { return "fake" }

// Send implements Sender.
func (f *Fake) Send(_ context.Context, m Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	if _, err := Encode(m); err != nil {
		return err
	}
	f.Sent = append(f.Sent, m)
	return nil
}

// Messages is a copy of what was sent.
func (f *Fake) Messages() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Message(nil), f.Sent...)
}

// SetFail sets or clears the failure.
func (f *Fake) SetFail(err error) {
	f.mu.Lock()
	f.Fail = err
	f.mu.Unlock()
}
