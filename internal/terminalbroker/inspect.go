package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var ErrInspectUnsupported = errors.New("Update reet on this machine to inspect session files and GitHub status.")

// InspectSession binds every view to an account-owned session's recorded workspace.
func (b *Broker) InspectSession(ctx context.Context, uid int64, session, view, path string) (terminalproto.InspectResult, error) {
	if (terminalproto.Inspect{Dir: "/", View: view, Path: path}).Validate() != nil {
		return terminalproto.InspectResult{}, terminalproto.ErrInvalid
	}
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		return terminalproto.InspectResult{}, ErrDenied
	}
	dir := s.WorktreePath
	if dir == "" {
		dir = s.Dir
	}
	if dir == "" {
		return terminalproto.InspectResult{Reason: "unavailable"}, nil
	}
	req := terminalproto.Inspect{Dir: dir, View: view, Path: path}
	p, caps, err := b.machinePeer(uid, s.AgentID, req)
	if err != nil {
		return terminalproto.InspectResult{}, err
	}
	if !caps.Inspect {
		return terminalproto.InspectResult{}, ErrInspectUnsupported
	}
	body, err := p.ask(ctx, req, 16*time.Second)
	if err != nil {
		return terminalproto.InspectResult{}, err
	}
	result, ok := body.(*terminalproto.InspectResult)
	if !ok {
		return terminalproto.InspectResult{}, ErrMachineAnswer
	}
	return *result, nil
}

// InspectFolder reads the status of a folder on the account's machine: for a
// saved project's folder (the controller's own record, never a browser path),
// to learn which GitHub repository its origin is. Sent only to a connection
// that reported the inspect capability.
func (b *Broker) InspectFolder(ctx context.Context, uid int64, agent, dir string) (terminalproto.InspectResult, error) {
	return b.InspectFolderView(ctx, uid, agent, dir, "status")
}

// InspectFolderView is InspectFolder for the status or the GitHub view. The
// folder is the controller's own record of where a pipeline works, on a
// machine the account owns.
func (b *Broker) InspectFolderView(ctx context.Context, uid int64, agent, dir, view string) (terminalproto.InspectResult, error) {
	if view != "status" && view != "github" {
		return terminalproto.InspectResult{}, terminalproto.ErrInvalid
	}
	req := terminalproto.Inspect{Dir: dir, View: view}
	p, caps, err := b.machinePeer(uid, agent, req)
	if err != nil {
		return terminalproto.InspectResult{}, err
	}
	if !caps.Inspect {
		return terminalproto.InspectResult{}, ErrInspectUnsupported
	}
	body, err := p.ask(ctx, req, 16*time.Second)
	if err != nil {
		return terminalproto.InspectResult{}, err
	}
	result, ok := body.(*terminalproto.InspectResult)
	if !ok {
		return terminalproto.InspectResult{}, ErrMachineAnswer
	}
	return *result, nil
}
