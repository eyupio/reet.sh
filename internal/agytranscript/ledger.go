// Package agytranscript reads Antigravity (agy) conversation ledgers.
//
// Agy appends one JSON object per step to a conversation's transcript_full.jsonl. A step has a
// step_index, a type (USER_INPUT, PLANNER_RESPONSE, GENERIC, ...) and a status (DONE once the step
// is complete). The file is append-only, but a rewind re-writes from an earlier index: a step whose
// index is not above the last one replaces that step and everything after it.
//
// This package only reads bytes it is handed. It never opens a path, so finding the right file
// (and proving it belongs to a session) stays with the caller.
package agytranscript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const (
	// MaxLineBytes bounds one step record.
	MaxLineBytes = 16 << 20
	// MaxSteps bounds the active history of one conversation.
	MaxSteps = 100_000
)

// ErrUnavailable means the history cannot be trusted: a malformed or oversized record, or more
// steps than MaxSteps. The caller shows the terminal instead of a guessed conversation.
var ErrUnavailable = errors.New("agy history is unavailable")

// ToolCall is one tool a planner response asked for.
type ToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Step is one ledger record.
type Step struct {
	Index     int        `json:"step_index"`
	Type      string     `json:"type"`
	Status    string     `json:"status"`
	Content   *string    `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls"`
	Model     string     `json:"model"`
}

// Done reports whether the step is complete.
func (s Step) Done() bool { return s.Status == "DONE" }

// Replay reads every complete line of a ledger and applies agy's rewind rule: a step whose index
// is not above the last kept one replaces that step and everything after it. A final line with no
// newline is still being written and is left out. Blank lines are skipped.
func Replay(r io.Reader) ([]Step, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	var steps []Step
	for {
		line, err := readLine(br)
		if err == io.EOF {
			return steps, nil
		}
		if err != nil {
			return nil, err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		step, err := decode(line)
		if err != nil {
			return nil, err
		}
		for len(steps) > 0 && steps[len(steps)-1].Index >= step.Index {
			steps = steps[:len(steps)-1]
		}
		steps = append(steps, step)
		if len(steps) > MaxSteps {
			return nil, ErrUnavailable
		}
	}
}

// readLine returns one newline-terminated line. A trailing fragment without a newline yields
// io.EOF: it is incomplete and waits for the writer.
func readLine(br *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, err := br.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > MaxLineBytes {
			return nil, ErrUnavailable
		}
		switch {
		case err == nil:
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case err == io.EOF:
			return nil, io.EOF
		default:
			return nil, ErrUnavailable
		}
	}
}

func decode(line []byte) (Step, error) {
	var raw struct {
		Index *int `json:"step_index"`
	}
	if json.Unmarshal(line, &raw) != nil || raw.Index == nil || *raw.Index < 0 {
		return Step{}, ErrUnavailable
	}
	var s Step
	if json.Unmarshal(line, &s) != nil {
		return Step{}, ErrUnavailable
	}
	return s, nil
}

// Completed drops a final step that is not DONE: it is still being written, so it waits for the
// next read. An unfinished step earlier in the ledger does not hide later completed ones.
func Completed(steps []Step) []Step {
	if n := len(steps); n > 0 && !steps[n-1].Done() {
		return steps[:n-1]
	}
	return steps
}

// Activity is whether the native turn is still running.
type Activity string

const (
	Idle    Activity = "idle"
	Working Activity = "working"
)

// ActivityOf follows the captured agy 1.3.3 sequence over completed steps: a finished user step
// starts a turn, a planner response with tools (or with no text) continues it, and a tool-free
// planner answer ends it. Tool results and other steps do not change it. This is display
// feedback, not input readiness: an interrupted turn with no final answer stays working.
func ActivityOf(steps []Step) Activity {
	activity := Idle
	for _, s := range steps {
		if !s.Done() {
			continue
		}
		switch s.Type {
		case "USER_INPUT":
			activity = Working
		case "PLANNER_RESPONSE":
			if len(s.ToolCalls) > 0 || s.Content == nil || strings.TrimSpace(*s.Content) == "" {
				activity = Working
			} else {
				activity = Idle
			}
		}
	}
	return activity
}
