package terminalproto

import (
	"strings"
	"testing"
)

func TestInspectProtocolBoundaries(t *testing.T) {
	for _, v := range []string{"status", "files", "file", "diff", "github"} {
		r := Inspect{Dir: "/home/dev/repo", View: v}
		if v == "file" || v == "diff" {
			r.Path = "src/main.go"
		}
		m := Message{Version: Version, RequestID: strings.Repeat("a", 16), Generation: 1, Body: r}
		data, err := Encode(m)
		if err != nil {
			t.Fatal(v, err)
		}
		if _, err = Decode(data); err != nil {
			t.Fatal(v, err)
		}
	}
	for _, p := range []string{"../secret", "a/../../secret", "/etc/passwd", "a/../b", ".git/config", "dir/.git/config", "a\\b", "a\nfile"} {
		if ValidInspectPath(p) {
			t.Errorf("accepted %q", p)
		}
	}
	for _, v := range []Inspect{{Dir: "/home/dev/repo", View: "exec"}, {Dir: "/home/dev/repo", View: "status", Path: "file"}, {Dir: "/home/dev/repo", View: "file"}, {View: "status"}} {
		if v.valid() {
			t.Errorf("accepted %+v", v)
		}
	}
	if (InspectResult{Text: strings.Repeat("x", MaxInspectBytes)}).valid() {
		t.Fatal("oversized result accepted")
	}
	if (Capabilities{Inspect: true}).Validate() == nil {
		t.Fatal("inspection without terminal control accepted")
	}
}
