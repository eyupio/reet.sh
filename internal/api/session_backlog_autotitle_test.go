package api

import "testing"

// A saved draft or idea left without a title is named from what it says, as the
// start dialog would have suggested; one with nothing to name it from is still
// refused.
func TestSessionBacklogBlankTitleIsNamedFromThePrompt(t *testing.T) {
	f := projectFixture(t)
	path := "/api/v2/session-backlog"
	draft := func(extra map[string]any) map[string]any {
		in := map[string]any{"title": "", "prompt": "", "agentId": f.agent.ID, "projectId": "", "dir": "/home/tester/site", "tool": "claude", "model": "", "effort": "", "images": []any{}, "version": 0}
		for k, v := range extra {
			in[k] = v
		}
		return in
	}
	for _, c := range []struct {
		name, want string
		in         map[string]any
	}{
		{"a prompt", "Make the filters clearer", draft(map[string]any{"prompt": "Please make the filters clearer. Start with the machine list."})},
		{"an idea's note", "Maybe add a dark mode toggle", draft(map[string]any{"idea": true, "prompt": "Maybe add a dark mode toggle"})},
		{"a typed title is kept", "Polish the board", draft(map[string]any{"title": "Polish the board", "prompt": "Please make the filters clearer"})},
	} {
		code, out, raw := f.do(t, f.cookie, "POST", path, c.in)
		if code != 201 || out["title"] != c.want {
			t.Fatalf("%s: %d %s, want the title %q", c.name, code, raw, c.want)
		}
	}
	for name, in := range map[string]map[string]any{
		"no prompt":                draft(nil),
		"a prompt with nothing in": draft(map[string]any{"prompt": "hi"}),
	} {
		if code, _, raw := f.do(t, f.cookie, "POST", path, in); code != 422 {
			t.Fatalf("%s: %d %s, want 422", name, code, raw)
		}
	}
}
