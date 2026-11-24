//go:build linux || darwin

package terminalagent

import "path/filepath"

// Running counts the chats this agent process is running.
func (c *ChatSupervisor) Running() int {
	if c == nil || c.s == nil {
		return 0
	}
	return len(c.running())
}

func (c *ChatSupervisor) running() []string {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	var ids []string
	for id, cs := range c.s.sessions {
		if cs.state.ReservesQuota() && !cs.quiet {
			ids = append(ids, id)
		}
	}
	return ids
}

// EndAll ends every chat this agent process is running, as the controller's
// end does (its process group goes and it is not resumed), and reports how
// many it ended. It is for reet agent lock --end-sessions on the machine.
func (c *ChatSupervisor) EndAll() int {
	if c == nil || c.s == nil {
		return 0
	}
	c.s.mu.Lock()
	link := c.s.link
	c.s.mu.Unlock()
	if link == nil {
		link = &chatLink{}
	}
	h := &chatHost{sup: c.s, link: link}
	ended := 0
	for _, id := range c.running() {
		if _, err := h.end(id); err == nil {
			ended++
		}
	}
	return ended
}

func recordedChats(layoutDir string) *chatRegistry {
	return newChatRegistry(filepath.Join(layoutDir, "structured-chats.json"))
}

// RecordedChats counts the chats a stopped agent would resume when it
// starts, from the record in its private terminal folder.
func RecordedChats(layoutDir string) int { return len(recordedChats(layoutDir).snapshot()) }

// ForgetRecordedChats drops the chats a stopped agent would resume, so none
// comes back when it starts, and reports how many it dropped.
func ForgetRecordedChats(layoutDir string) int {
	r := recordedChats(layoutDir)
	records := r.snapshot()
	for _, rec := range records {
		r.remove(rec.ID)
	}
	return len(records)
}
