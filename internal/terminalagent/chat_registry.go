//go:build linux || darwin

package terminalagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// chatRecord is one structured chat the connector should try to reopen after
// its own process restarts (an upgrade, a crash or a reboot). It is written
// beside the terminal state, private to the agent's OS user. The OpenCode
// store holds the conversation; this only remembers which conversation belongs
// to which reet session and how it was started.
type chatRecord struct {
	WorktreePath string `json:"worktreePath,omitempty"`
	WorktreeRepo string `json:"worktreeRepo,omitempty"`
	ID           string `json:"id"`
	Title        string `json:"title"`
	Tool         string `json:"tool"`
	Dir          string `json:"dir"`
	Model        string `json:"model"`
	Effort       string `json:"effort,omitempty"`
	Yolo         bool   `json:"yolo,omitempty"`
	Trust        bool   `json:"trust,omitempty"`
	ACPSID       string `json:"acpSessionId"`
	UpdatedAt    int64  `json:"updatedAt"`
	Attempts     int    `json:"attempts,omitempty"`
}

type chatRegistryFile struct {
	Version int          `json:"version"`
	Chats   []chatRecord `json:"chats"`
}

// maxChatResumeAge bounds how stale a record may be before it is dropped. A
// chat left running by a machine that has been off for longer than this is
// history, not work to reopen.
const maxChatResumeAge = 7 * 24 * time.Hour

// maxChatResumeAttempts bounds repeated failed reopens so a broken OpenCode
// store is not retried on every connector start forever.
const maxChatResumeAttempts = 3

// chatRegistry is the connector-side list of structured chats to reopen after
// a process restart. Writes are atomic; a corrupt file is ignored rather than
// failing the agent, and is replaced by the next successful write.
type chatRegistry struct {
	mu    sync.Mutex
	path  string
	chats map[string]chatRecord
}

func newChatRegistry(path string) *chatRegistry {
	r := &chatRegistry{path: path, chats: map[string]chatRecord{}}
	r.load()
	return r
}

func (r *chatRegistry) load() {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return
	}
	var file chatRegistryFile
	if json.Unmarshal(data, &file) != nil || file.Version != 1 {
		return
	}
	for _, rec := range file.Chats {
		if r.valid(rec) {
			r.chats[rec.ID] = rec
		}
	}
}

func (r *chatRegistry) valid(rec chatRecord) bool {
	if (rec.WorktreePath == "") != (rec.WorktreeRepo == "") || rec.WorktreePath != "" && (rec.Dir != rec.WorktreePath || !terminalproto.ValidDir(rec.WorktreePath) || !terminalproto.ValidSlug(rec.WorktreeRepo)) {
		return false
	}
	return terminalproto.ValidID(rec.ID) &&
		terminalproto.ValidTitle(rec.Title) &&
		(rec.Model == "" || terminalproto.ValidModel(rec.Model)) &&
		(rec.Effort == "" || terminalproto.ValidModel(rec.Effort)) &&
		terminalproto.ReviewedChatTool(rec.Tool) &&
		acpagent.Describe(rec.Tool, true, "").History &&
		terminalproto.ValidDir(rec.Dir) &&
		terminalproto.ValidACPSessionIDFor(rec.Tool, rec.ACPSID)
}

// snapshot returns the records worth reopening, oldest first, and drops entries
// that are too old. The caller must not mutate the returned values.
func (r *chatRegistry) snapshot() []chatRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	out := make([]chatRecord, 0, len(r.chats))
	changed := false
	for id, rec := range r.chats {
		stale := rec.UpdatedAt <= 0 || now.Sub(time.UnixMilli(rec.UpdatedAt)) > maxChatResumeAge
		if stale || rec.Attempts >= maxChatResumeAttempts {
			delete(r.chats, id)
			changed = true
			continue
		}
		out = append(out, rec)
	}
	if changed {
		r.saveLocked()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt < out[j].UpdatedAt })
	return out
}

func (r *chatRegistry) remember(rec chatRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// A manual continuation replaces the failed row's restart record. Keeping
	// both would load the same conversation twice on the next connector start.
	for id, previous := range r.chats {
		sameChat := previous.Tool == rec.Tool && previous.ACPSID == rec.ACPSID
		if id != rec.ID && sameChat {
			delete(r.chats, id)
		}
	}
	rec.UpdatedAt = time.Now().UnixMilli()
	rec.Attempts = 0
	r.chats[rec.ID] = rec
	r.saveLocked()
}

// touch refreshes an existing record, for example after the owner switched
// approval mode. It does nothing for a session that is not recorded.
func (r *chatRegistry) touch(id string, yolo bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.chats[id]
	if !ok {
		return
	}
	rec.Yolo = yolo
	rec.UpdatedAt = time.Now().UnixMilli()
	r.chats[id] = rec
	r.saveLocked()
}

// setModel updates an existing restart record from the process's read-back.
// It never recreates a record concurrently removed by End or a mode handoff.
func (r *chatRegistry) setModel(id, model string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.chats[id]
	if !ok {
		return
	}
	rec.Model = model
	rec.UpdatedAt = time.Now().UnixMilli()
	r.chats[id] = rec
	r.saveLocked()
}

// setControls never recreates a record removed by End or mode handoff.
func (r *chatRegistry) setControls(id string, state terminalproto.ACPControls) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.chats[id]
	if !ok {
		return
	}
	rec.Model = state.Model
	rec.Effort = ""
	for _, o := range state.Options {
		if o.Category == "thought_level" {
			rec.Effort = o.Current
		}
	}
	rec.UpdatedAt = time.Now().UnixMilli()
	r.chats[id] = rec
	r.saveLocked()
}

// seen refreshes when a recorded chat last did something, so an active chat is
// not dropped by the age bound while it is still in use.
func (r *chatRegistry) seen(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.chats[id]
	if !ok {
		return
	}
	rec.UpdatedAt = time.Now().UnixMilli()
	r.chats[id] = rec
	r.saveLocked()
}

// failed records one failed reopen. The record is dropped after the bounded
// number of attempts.
func (r *chatRegistry) failed(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.chats[id]
	if !ok {
		return
	}
	rec.Attempts++
	if rec.Attempts >= maxChatResumeAttempts {
		delete(r.chats, id)
	} else {
		r.chats[id] = rec
	}
	r.saveLocked()
}

func (r *chatRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.chats[id]; !ok {
		return
	}
	delete(r.chats, id)
	r.saveLocked()
}

func (r *chatRegistry) saveLocked() {
	file := chatRegistryFile{Version: 1, Chats: make([]chatRecord, 0, len(r.chats))}
	for _, rec := range r.chats {
		file.Chats = append(file.Chats, rec)
	}
	sort.Slice(file.Chats, func(i, j int) bool { return file.Chats[i].ID < file.Chats[j].ID })
	data, err := json.Marshal(file)
	if err != nil {
		return
	}
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, r.path)
}

// remember records a live chat so the next connector process can reopen it.
func (s *chatSupervisor) remember(cs *chatSession) {
	if s.registry == nil {
		return
	}
	s.mu.Lock()
	rec := chatRecord{
		ID: cs.id, Title: cs.title, Tool: cs.tool, Dir: cs.dir, Model: cs.model, Effort: cs.effort,
		Yolo: cs.yolo, Trust: cs.trust, ACPSID: cs.acpID,
		WorktreePath: cs.worktreePath, WorktreeRepo: cs.worktreeRepo,
	}
	s.mu.Unlock()
	valid := rec.ACPSID != "" && acpagent.Describe(rec.Tool, true, "").History &&
		terminalproto.ValidACPSessionIDFor(rec.Tool, rec.ACPSID) && terminalproto.ValidDir(rec.Dir)
	if !valid {
		return
	}
	s.registry.remember(rec)
}

// touchRecord updates the approval mode stored for a chat that is already
// recorded, so a restart does not restore a different mode than the person
// last chose. It never creates a record.
func (s *chatSupervisor) touchRecord(id string, yolo bool) {
	if s.registry != nil {
		s.registry.touch(id, yolo)
	}
}

// forgetRecord removes a chat the person ended or handed to the OpenCode TUI.
func (s *chatSupervisor) forgetRecord(id string) {
	if s.registry != nil {
		s.registry.remove(id)
	}
}

// chatBinary resolves the executable a chat runs, preferring reet's managed
// installation over PATH as a terminal launch does.
func (s *chatSupervisor) chatBinary(tool string) (acpagent.Spec, string, error) {
	spec, ok := acpagent.Lookup(tool)
	if !ok {
		return acpagent.Spec{}, "", acpagent.ErrUnsupported
	}
	binary, err := s.lookPath(spec.Command)
	managed, found, managedErr := agentsoftware.ManagedBinary(s.opts.Home, tool)
	if managedErr != nil {
		return acpagent.Spec{}, "", &acpagent.VersionError{Agent: spec.DisplayName, Required: spec.MinVersion, Reason: "reet's managed installation is broken"}
	}
	if found {
		binary, err = managed, nil
	}
	if err != nil {
		return acpagent.Spec{}, "", &acpagent.VersionError{Agent: spec.DisplayName, Required: spec.MinVersion, Reason: "the agent is not installed"}
	}
	return spec, binary, nil
}

// resumePersisted reopens the chats this connector was running when its
// previous process stopped. OpenCode keeps the conversation; reet only needs
// to start its ACP process again with session/load and the same reet session
// id, so a controller reconnect adopts the row instead of marking it lost.
func (s *chatSupervisor) resumePersisted() {
	for _, rec := range s.registry.snapshot() {
		if s.ctx.Err() != nil {
			return
		}
		s.resumeChat(rec)
	}
}

func (s *chatSupervisor) resumeChat(rec chatRecord) {
	if s.opts.Consent != nil && s.opts.Consent() != nil {
		s.registry.remove(rec.ID)
		return
	}
	dir, err := terminalhost.ResolveDir(s.opts.Home, rec.Dir)
	if err != nil {
		s.registry.remove(rec.ID)
		return
	}
	spec, binary, err := s.chatBinary(rec.Tool)
	if err != nil {
		s.registry.remove(rec.ID)
		return
	}
	yolo := rec.Yolo
	consent := s.opts.YoloConsent == nil || s.opts.YoloConsent() != nil
	if yolo && consent {
		yolo = false
	}

	cs := &chatSession{id: rec.ID, title: rec.Title, tool: rec.Tool, dir: dir, model: rec.Model, effort: rec.Effort, acpID: rec.ACPSID,
		state: terminalproto.Running, busy: true, yolo: yolo, trust: rec.Trust, worktreePath: rec.WorktreePath, worktreeRepo: rec.WorktreeRepo}
	s.mu.Lock()
	if s.closed || s.quotaLocked() >= terminalproto.MaxSessionsPerAgent {
		s.mu.Unlock()
		return
	}
	if _, exists := s.sessions[rec.ID]; exists || s.conversationInUseLocked(rec.Tool, rec.ACPSID) {
		s.mu.Unlock()
		return
	}
	s.sessions[rec.ID] = cs
	s.mu.Unlock()

	startCtx, cancel := context.WithTimeout(s.ctx, 60*time.Second)
	defer cancel()
	s.mu.Lock()
	cs.cancel = cancel
	s.mu.Unlock()

	// session/load replays the whole conversation to OnEvent. The controller
	// already shows that conversation (its live ring, or its stored one), so
	// the replay is dropped here and only events after startup are forwarded.
	var restored atomic.Bool
	process, err := s.start(startCtx, acpagent.Options{
		NativeTerminal: rec.Tool == "opencode",
		Agent:          spec, Binary: binary, Dir: dir, Home: s.opts.Home, Model: rec.Model, Effort: rec.Effort, Trust: rec.Trust, Resume: rec.ACPSID,
		BaseEnv: os.Environ(),
		OnEvent: func(event acpagent.Event) {
			if restored.Load() {
				s.publish(cs, event)
			}
		},
	})
	if err != nil {
		s.mu.Lock()
		cs.busy = false
		cs.state = terminalproto.Exited
		cs.cancel = nil
		s.mu.Unlock()
		s.registry.failed(rec.ID)
		s.publish(cs, acpagent.Event{Kind: "notice", Bad: true,
			Text: "This chat could not be restored after reet restarted on the machine. You can try Continue with history again, or start a new chat."})
		s.ended(cs, -1)
		return
	}

	restored.Store(true)
	acpID := process.SessionID()
	s.mu.Lock()
	if s.closed {
		delete(s.sessions, rec.ID)
		s.mu.Unlock()
		process.Close()
		return
	}
	cs.process = process
	if acpID != "" {
		cs.acpID = acpID
	}
	acpID = cs.acpID
	cs.state = terminalproto.Running
	cs.busy = false
	cs.cancel = nil
	s.wg.Add(1)
	s.mu.Unlock()

	s.registry.remember(chatRecord{ID: rec.ID, Title: rec.Title, Tool: rec.Tool, Dir: rec.Dir,
		Model: rec.Model, Effort: rec.Effort, Yolo: yolo, Trust: rec.Trust, ACPSID: acpID, WorktreePath: rec.WorktreePath, WorktreeRepo: rec.WorktreeRepo})
	s.announceRunning(cs)
	s.publish(cs, acpagent.Event{Kind: "notice",
		Text: "This chat was restored after reet restarted on the machine. A reply that was still running then was stopped."})
	s.publish(cs, acpagent.Event{Kind: "status", State: "idle"})
	go func() {
		defer s.wg.Done()
		<-process.Done()
		s.ended(cs, 0)
	}()
}
