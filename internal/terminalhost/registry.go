//go:build linux || darwin

package terminalhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/eyupio/reet.sh/internal/filelock"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const maxRegistryBytes = 16 << 20

var (
	ErrRegistry          = errors.New("terminal registry is unavailable, invalid, or belongs to another enrollment")
	ErrSessionMissing    = errors.New("managed terminal session is missing")
	ErrOperationConflict = errors.New("terminal operation conflicts with its durable receipt")
	ErrTerminalDisabled  = errors.New("terminal access is disabled locally")
	ErrSessionCapacity   = errors.New("managed terminal session capacity reached")
	ErrManagerClosed     = errors.New("terminal manager is closed")
	ErrOutputLimit       = errors.New("terminal snapshot exceeds its byte limit; request fewer lines")
)

// ManagedSession contains operational metadata only. No bytes, environments,
// commands, credentials, or paths supplied by a controller are persisted.
type ManagedSession struct {
	ID              string                     `json:"id"`
	Title           string                     `json:"title"`
	Tool            string                     `json:"tool,omitempty"`
	State           terminalproto.SessionState `json:"state"`
	CreatedAt       int64                      `json:"createdAt"`
	UpdatedAt       int64                      `json:"updatedAt"`
	ActivityAt      int64                      `json:"activityAt,omitempty"`
	BellAt          int64                      `json:"bellAt,omitempty"`
	ExitCode        *int                       `json:"exitCode"`
	PaneID          string                     `json:"paneId,omitempty"`
	CreateRequestID string                     `json:"createRequestId"`
	// WorktreePath and WorktreeRepo record where a worktree create made the
	// worktree (the folder, and the repository slug status and remove take),
	// so a replay of the create reports them again.
	WorktreePath string `json:"worktreePath,omitempty"`
	WorktreeRepo string `json:"worktreeRepo,omitempty"`
}

type operationReceipt struct {
	RequestID string                `json:"requestId"`
	SessionID string                `json:"sessionId"`
	Kind      string                `json:"kind"`
	Create    *terminalproto.Create `json:"create,omitempty"`
	Complete  bool                  `json:"complete"`
}
type hostRegistry struct {
	Version      int                         `json:"version"`
	EnrollmentID string                      `json:"enrollmentId"`
	Sessions     map[string]ManagedSession   `json:"sessions"`
	Operations   map[string]operationReceipt `json:"operations"`
}

func (m *Manager) registryPath() string { return filepath.Join(m.layout.Dir, "registry.json") }
func receiptKey(kind, id string) string { return kind + ":" + id }

// Lock each transaction rather than holding a lifetime lock: a local CLI can
// inspect/end work while the agent is up. Every transaction reloads the file;
// no process can commit a stale in-memory registry over another writer.
func (m *Manager) registryLock(ctx context.Context) (*os.File, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lock, err := filelock.Acquire(filepath.Join(m.layout.Dir, "registry.lock"))
		if !errors.Is(err, filelock.ErrLocked) {
			return lock, err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *Manager) loadRegistry() (hostRegistry, error) {
	empty := hostRegistry{Version: 1, EnrollmentID: m.enrollment, Sessions: map[string]ManagedSession{}, Operations: map[string]operationReceipt{}}
	info, err := os.Lstat(m.registryPath())
	if os.IsNotExist(err) {
		return empty, nil
	}
	if err != nil {
		return empty, ErrRegistry
	}
	validPerm := info.Mode().IsRegular() && info.Mode().Perm()&0077 == 0
	validOwner := ownedByCurrentUser(info) == nil
	valid := validPerm && validOwner && info.Size() <= maxRegistryBytes
	if !valid {
		return empty, ErrRegistry
	}
	f, err := os.Open(m.registryPath())
	if err != nil {
		return empty, ErrRegistry
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return empty, ErrRegistry
	}
	data, err := io.ReadAll(io.LimitReader(f, maxRegistryBytes+1))
	if err != nil || len(data) > maxRegistryBytes {
		return empty, ErrRegistry
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var r hostRegistry
	decoded := d.Decode(&r) == nil && d.Decode(new(any)) == io.EOF
	if !decoded || m.validateRegistry(r) != nil {
		return empty, ErrRegistry
	}
	return r, nil
}

func (m *Manager) validateRegistry(r hostRegistry) error {
	validVersion := r.Version == 1 && r.EnrollmentID == m.enrollment
	validMaps := r.Sessions != nil && r.Operations != nil
	validCounts := len(r.Sessions) <= 4096 && len(r.Operations) <= 16384
	valid := validVersion && validMaps && validCounts
	if !valid {
		return ErrRegistry
	}
	for id, s := range r.Sessions {
		validID := id == s.ID && terminalproto.ValidID(id)
		validSession := terminalproto.ValidID(s.CreateRequestID) && terminalproto.ValidTitle(s.Title) && s.State.Valid()
		validTimes := s.CreatedAt > 0 && s.UpdatedAt >= s.CreatedAt && s.ActivityAt >= 0 && s.BellAt >= 0
		validExit := s.ExitCode == nil ||
			(s.State == terminalproto.Exited && *s.ExitCode >= 0 && *s.ExitCode <= 255)
		validPane := s.PaneID == "" || paneID.MatchString(s.PaneID)
		valid := validID && validSession && validTimes && validExit && validPane
		if !valid {
			return ErrRegistry
		}
		if op, ok := r.Operations[receiptKey("create", s.CreateRequestID)]; !ok || op.SessionID != id {
			return ErrRegistry
		}
		paired := (s.WorktreePath == "") == (s.WorktreeRepo == "")
		validPair := s.WorktreePath == "" || (filepath.IsAbs(s.WorktreePath) &&
			!hasControl(s.WorktreePath) && terminalproto.ValidSlug(s.WorktreeRepo))
		if !paired || !validPair {
			return ErrRegistry
		}
	}
	for key, op := range r.Operations {
		validKey := terminalproto.ValidID(op.RequestID) && key == receiptKey(op.Kind, op.RequestID)
		validKind := op.Kind == "create" || op.Kind == "end"
		if !validKey || !validKind {
			return ErrRegistry
		}
		s, ok := r.Sessions[op.SessionID]
		if !ok {
			return ErrRegistry
		}
		isCreate := op.Kind == "create"
		createValid := op.Create != nil && op.Create.Validate() == nil && s.CreateRequestID == op.RequestID
		isEnd := op.Kind == "end"
		endValid := op.Create == nil
		valid := (!isCreate || createValid) && (!isEnd || endValid)
		if !valid {
			return ErrRegistry
		}
	}
	return nil
}

func (m *Manager) saveRegistry(r hostRegistry) error {
	if m.validateRegistry(r) != nil {
		return ErrRegistry
	}
	data, err := json.Marshal(r)
	if err != nil || len(data) > maxRegistryBytes {
		return ErrRegistry
	}
	if err := writePrivateFile(m.registryPath(), data); err != nil {
		return ErrRegistry
	}
	// The rename, not just its temporary file contents, must precede an ACK.
	dir, err := os.Open(m.layout.Dir)
	if err != nil {
		return ErrRegistry
	}
	defer dir.Close()
	if dir.Sync() != nil {
		return ErrRegistry
	}
	close(m.changed)
	m.changed = make(chan struct{})
	return nil
}
