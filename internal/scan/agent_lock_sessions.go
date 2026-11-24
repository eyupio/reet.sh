//go:build linux || darwin

package scan

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
)

// localSessions is this machine's terminal sessions, through their own
// manager, and its chats: from the running agent over its local control
// socket, or, with the agent stopped, the chats it would resume.
type localSessions struct {
	config string
	c      agentConfig
}

func localSessionsFor(config string, c agentConfig) machineSessions { return localSessions{config, c} }

func (l localSessions) layout() (terminalhost.Layout, error) {
	return terminalhost.NewEnrollmentLayout(l.config, l.c.ID)
}

// shells opens a manager on the machine's tmux server; with no terminal
// set up, or no server running, there are none.
func (l localSessions) shells(ctx context.Context) (*terminalhost.Manager, bool) {
	layout, err := l.layout()
	if err != nil {
		return nil, false
	}
	m, err := terminalhost.NewManager(ctx, terminalhost.ManagerOptions{EnrollmentID: l.c.ID, Layout: layout, Home: l.c.Home, Consent: func() error { return nil }})
	if err != nil {
		return nil, false
	}
	return m, true
}

func (l localSessions) running() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n := 0
	if m, ok := l.shells(ctx); ok {
		shells, err := m.Running(ctx)
		_ = m.Close()
		if err != nil && !errors.Is(err, terminalhost.ErrNotRunning) {
			return 0, err
		}
		n += shells
	}
	chats, err := l.chats("GET", "/chats")
	return n + chats, err
}

func (l localSessions) endAll() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ended := 0
	var errs []error
	if m, ok := l.shells(ctx); ok {
		n, err := m.EndAll(ctx)
		_ = m.Close()
		ended += n
		if err != nil && !errors.Is(err, terminalhost.ErrNotRunning) {
			errs = append(errs, err)
		}
	}
	n, err := l.chats("POST", "/chats/end")
	return ended + n, errors.Join(append(errs, err)...)
}

// chats asks the running agent, which owns them, or with the agent
// stopped reads (or, for an end, forgets) the chats it would resume.
func (l localSessions) chats(method, path string) (int, error) {
	if running, err := agentRunning(l.config); err == nil && running {
		var reply struct{ Count int }
		if err := localAgentCallWithin(l.config, method, path, &reply, time.Minute); err != nil {
			return 0, err
		}
		return reply.Count, nil
	}
	layout, err := l.layout()
	if err != nil {
		return 0, nil
	}
	if method == "POST" {
		return terminalagent.ForgetRecordedChats(layout.Dir), nil
	}
	return terminalagent.RecordedChats(layout.Dir), nil
}
