package terminalbroker

import (
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ProgressStep is one launch stage a machine reported, and when it began.
type ProgressStep struct {
	Stage terminalproto.LaunchStage
	At    time.Time
}

const (
	progressKeep = 5 * time.Minute
	progressMax  = 256
)

type progressKey struct {
	uid     int64
	agent   string
	request string
}

// launchProgress keeps the stages of recent creates in memory only. It is a
// hint for the page's progress view: nothing decides on it, and it is lost on
// restart, which just shows the page its plain steps.
type launchProgress struct {
	mu    sync.Mutex
	steps map[progressKey][]ProgressStep
}

func (l *launchProgress) note(uid int64, agent, request string, stage terminalproto.LaunchStage, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.steps == nil {
		l.steps = map[progressKey][]ProgressStep{}
	}
	for k, v := range l.steps {
		if at.Sub(v[len(v)-1].At) > progressKeep {
			delete(l.steps, k)
		}
	}
	k := progressKey{uid, agent, request}
	if _, seen := l.steps[k]; !seen && len(l.steps) >= progressMax {
		return
	}
	l.steps[k] = append(l.steps[k], ProgressStep{stage, at})
}

// LaunchProgress returns the stages reported so far for one of this account's
// creates, oldest first; nil when the machine reported none.
func (b *Broker) LaunchProgress(uid int64, agent, request string) []ProgressStep {
	b.progress.mu.Lock()
	defer b.progress.mu.Unlock()
	return append([]ProgressStep(nil), b.progress.steps[progressKey{uid, agent, request}]...)
}
