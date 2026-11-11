package terminalbroker

import (
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// yoloRollupWindow is the longest one audit row keeps counting. A turn that
// runs longer gets a new row, so a row never hides more than this much time.
const yoloRollupWindow = 10 * time.Minute

// yoloRollup is the audit row the approvals YOLO gave in one turn of one chat
// are counted on.
type yoloRollup struct {
	id      int64
	count   int
	first   string
	started time.Time
}

// auditYoloAllow records that YOLO allowed one request. Every approval is
// counted and saved as it happens; the approvals of one turn share one audit
// row (action yolo-allow) that carries how many there were and when, instead
// of a row each. The row is written on the first approval, so nothing waits
// in memory to be recorded.
func (b *Broker) auditYoloAllow(uid int64, s store.TerminalSession, permission string) {
	key := chatKey{uid, s.ID}
	now := time.Now()
	b.yoloMu.Lock()
	defer b.yoloMu.Unlock()
	r := b.yoloAllows[key]
	if r != nil && now.Sub(r.started) >= yoloRollupWindow {
		r = nil
	}
	if r == nil {
		r = &yoloRollup{first: permission, started: now}
	}
	r.count++
	detail := map[string]any{
		"machine": s.AgentID, "tool": s.Tool, "session": s.ID, "count": r.count,
		"permission": r.first, "lastPermission": permission,
		"from": r.started.UnixMilli(), "to": now.UnixMilli(),
	}
	var err error
	if r.id == 0 {
		r.id, err = b.st.AddEventIDFor(uid, "machine", "yolo-allow", "", detail)
	} else {
		err = b.st.UpdateEventDetailFor(uid, r.id, "yolo-allow", detail)
	}
	if err != nil || r.id == 0 {
		// Never lose the record of an approval: fall back to a row of its own.
		_ = b.st.AddEventFor(uid, "machine", "yolo-allow", "", map[string]any{
			"machine": s.AgentID, "tool": s.Tool, "session": s.ID, "count": 1, "permission": permission,
		})
		delete(b.yoloAllows, key)
		return
	}
	if b.yoloAllows == nil {
		b.yoloAllows = map[chatKey]*yoloRollup{}
	}
	b.yoloAllows[key] = r
}

// endYoloRollup closes the chat's current row: the next approval starts another.
func (b *Broker) endYoloRollup(uid int64, session string) {
	b.yoloMu.Lock()
	delete(b.yoloAllows, chatKey{uid, session})
	b.yoloMu.Unlock()
}
