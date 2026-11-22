package agytranscript

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func line(index int, typ, status, content string, tools int) string {
	calls := ""
	for i := 0; i < tools; i++ {
		calls += `{"name":"run_command","args":{"CommandLine":"x"}}`
		if i < tools-1 {
			calls += ","
		}
	}
	return fmt.Sprintf(`{"step_index":%d,"type":%q,"status":%q,"content":%q,"tool_calls":[%s]}`+"\n", index, typ, status, content, calls)
}

func replay(t *testing.T, ledger string) []Step {
	t.Helper()
	steps, err := Replay(strings.NewReader(ledger))
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

func indices(steps []Step) []int {
	out := make([]int, len(steps))
	for i, s := range steps {
		out[i] = s.Index
	}
	return out
}

func TestReplayKeepsOrder(t *testing.T) {
	steps := replay(t, line(0, "USER_INPUT", "DONE", "go", 0)+line(1, "PLANNER_RESPONSE", "DONE", "done", 0))
	if got := indices(steps); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatal(got)
	}
	if steps[1].Content == nil || *steps[1].Content != "done" {
		t.Fatalf("content lost: %+v", steps[1])
	}
}

func TestReplayRewindReplacesSuffix(t *testing.T) {
	steps := replay(t,
		line(0, "USER_INPUT", "DONE", "one", 0)+
			line(1, "PLANNER_RESPONSE", "DONE", "a", 0)+
			line(2, "USER_INPUT", "DONE", "two", 0)+
			line(3, "PLANNER_RESPONSE", "DONE", "b", 0)+
			// a rewind to step 2 replaces 2 and 3
			line(2, "USER_INPUT", "DONE", "two again", 0))
	if got := indices(steps); len(got) != 3 || got[2] != 2 {
		t.Fatal(got)
	}
	if *steps[2].Content != "two again" {
		t.Fatalf("rewound step not replaced: %+v", steps[2])
	}
	// a repeated index replaces only itself when it is the last
	steps = replay(t, line(0, "USER_INPUT", "RUNNING", "x", 0)+line(0, "USER_INPUT", "DONE", "x", 0))
	if len(steps) != 1 || !steps[0].Done() {
		t.Fatalf("repeat not replaced: %+v", steps)
	}
}

func TestReplayIgnoresAnUnfinishedLastLineAndBlankLines(t *testing.T) {
	ledger := line(0, "USER_INPUT", "DONE", "one", 0) + "\n  \n" + `{"step_index":1,"type":"PLANNER_RESP`
	if got := indices(replay(t, ledger)); len(got) != 1 || got[0] != 0 {
		t.Fatalf("partial line not left for the writer: %v", got)
	}
}

func TestReplayRefusesWhatCannotBeTrusted(t *testing.T) {
	for name, ledger := range map[string]string{
		"not json":           "garbage\n",
		"no step index":      `{"type":"USER_INPUT","status":"DONE"}` + "\n",
		"negative index":     `{"step_index":-1,"type":"USER_INPUT"}` + "\n",
		"content not text":   `{"step_index":0,"content":{"a":1}}` + "\n",
		"oversized record":   `{"step_index":0,"content":"` + strings.Repeat("a", MaxLineBytes) + `"}` + "\n",
		"oversized no \\n":   strings.Repeat("a", MaxLineBytes+1),
		"malformed after ok": line(0, "USER_INPUT", "DONE", "ok", 0) + "{\n",
	} {
		if _, err := Replay(strings.NewReader(ledger)); !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestReplayBoundsTheStepCount(t *testing.T) {
	var b strings.Builder
	for i := 0; i <= MaxSteps; i++ {
		fmt.Fprintf(&b, `{"step_index":%d,"type":"GENERIC","status":"DONE"}`+"\n", i)
	}
	if _, err := Replay(strings.NewReader(b.String())); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestCompletedWaitsForAnUnfinishedLastStep(t *testing.T) {
	steps := replay(t, line(0, "USER_INPUT", "DONE", "one", 0)+line(1, "GENERIC", "RUNNING", "", 0))
	if got := Completed(steps); len(got) != 1 {
		t.Fatalf("unfinished last step shown: %v", indices(got))
	}
	// an unfinished historical step does not hide a later completed one
	steps = replay(t, line(0, "GENERIC", "RUNNING", "", 0)+line(1, "USER_INPUT", "DONE", "two", 0))
	if got := Completed(steps); len(got) != 2 {
		t.Fatalf("later completed step hidden: %v", indices(got))
	}
	if Completed(nil) != nil {
		t.Fatal("empty ledger")
	}
}

func TestActivityFollowsTheNativeTurn(t *testing.T) {
	cases := []struct {
		name   string
		ledger string
		want   Activity
	}{
		{"empty", "", Idle},
		{"user prompt starts the turn", line(0, "USER_INPUT", "DONE", "go", 0), Working},
		{"planner with tools continues", line(0, "USER_INPUT", "DONE", "go", 0) + line(1, "PLANNER_RESPONSE", "DONE", "running", 1), Working},
		{"tool results do not end it", line(0, "USER_INPUT", "DONE", "go", 0) + line(1, "PLANNER_RESPONSE", "DONE", "running", 1) + line(2, "GENERIC", "DONE", "ok", 0), Working},
		{"tool-free answer ends it", line(0, "USER_INPUT", "DONE", "go", 0) + line(1, "PLANNER_RESPONSE", "DONE", "Done.", 0), Idle},
		{"empty planner text does not end it", line(0, "USER_INPUT", "DONE", "go", 0) + line(1, "PLANNER_RESPONSE", "DONE", "  ", 0), Working},
		{"a tool result after the answer does not restart it", line(0, "USER_INPUT", "DONE", "go", 0) + line(1, "PLANNER_RESPONSE", "DONE", "Done.", 0) + line(2, "GENERIC", "DONE", "late", 0), Idle},
		{"unfinished steps are not evidence", line(0, "USER_INPUT", "DONE", "go", 0) + line(1, "PLANNER_RESPONSE", "RUNNING", "Done.", 0), Working},
		{"missing content is not an answer", line(0, "USER_INPUT", "DONE", "go", 0) + `{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE"}` + "\n", Working},
	}
	for _, c := range cases {
		if got := ActivityOf(replay(t, c.ledger)); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}
