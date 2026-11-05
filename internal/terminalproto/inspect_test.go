package terminalproto

import (
	"encoding/json"
	"fmt"
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

func fullInspectResult() InspectResult {
	p := &InspectPull{
		Number: 42, Title: "Show the session overview", URL: "https://github.com/eyupio/reet.sh/pull/42",
		State: "OPEN", Review: "REVIEW_REQUIRED", Mergeable: "MERGEABLE", Added: 10, Removed: 2,
		Base: "main", Head: "claude/session-overview-machine",
		CreatedAt: 1_700_000_000_000, MergedAt: 1_700_000_100_000, ClosedAt: 1_700_000_100_000, LastPushAt: 1_700_000_050_000,
		AutoMerge:   true,
		Reviews:     &InspectReviews{Approved: 1, ChangesRequested: 2, Commented: 3, Requested: 4},
		Threads:     &InspectThreads{Open: 5, Resolved: 6, More: true},
		DetailKnown: true,
	}
	return InspectResult{CountsKnown: true, CheckedAt: 1_700_000_000_000, Remote: "eyupio/reet.sh", Pull: p}
}

func TestInspectPullDetailBoundaries(t *testing.T) {
	if !fullInspectResult().valid() {
		t.Fatal("a fully populated result was refused")
	}
	over := strings.Repeat("x", 256)
	for name, mutate := range map[string]func(*InspectResult){
		"negative approved":         func(r *InspectResult) { r.Pull.Reviews.Approved = -1 },
		"negative changes":          func(r *InspectResult) { r.Pull.Reviews.ChangesRequested = -1 },
		"negative commented":        func(r *InspectResult) { r.Pull.Reviews.Commented = -1 },
		"negative requested":        func(r *InspectResult) { r.Pull.Reviews.Requested = -1 },
		"too many approved":         func(r *InspectResult) { r.Pull.Reviews.Approved = 10001 },
		"too many requested":        func(r *InspectResult) { r.Pull.Reviews.Requested = 10001 },
		"negative open threads":     func(r *InspectResult) { r.Pull.Threads.Open = -1 },
		"negative resolved threads": func(r *InspectResult) { r.Pull.Threads.Resolved = -1 },
		"too many open threads":     func(r *InspectResult) { r.Pull.Threads.Open = 10001 },
		"too many resolved threads": func(r *InspectResult) { r.Pull.Threads.Resolved = 10001 },
		"long base":                 func(r *InspectResult) { r.Pull.Base = over },
		"long head":                 func(r *InspectResult) { r.Pull.Head = over },
		"multibyte head over bytes": func(r *InspectResult) { r.Pull.Head = strings.Repeat("é", 128) },
		"control in base":           func(r *InspectResult) { r.Pull.Base = "ma\nin" },
		"control in head":           func(r *InspectResult) { r.Pull.Head = "a\x00b" },
		"negative created":          func(r *InspectResult) { r.Pull.CreatedAt = -1 },
		"huge merged":               func(r *InspectResult) { r.Pull.MergedAt = MaxCounter + 1 },
		"huge closed":               func(r *InspectResult) { r.Pull.ClosedAt = MaxCounter + 1 },
		"negative push":             func(r *InspectResult) { r.Pull.LastPushAt = -1 },
		"remote without owner":      func(r *InspectResult) { r.Remote = "reet.sh" },
		"remote with space":         func(r *InspectResult) { r.Remote = "eyupio/reet sh" },
		"remote with extra slash":   func(r *InspectResult) { r.Remote = "a/b/c" },
		"remote with newline":       func(r *InspectResult) { r.Remote = "a/b\n" },
		"remote too long":           func(r *InspectResult) { r.Remote = "a/" + strings.Repeat("b", 139) },
	} {
		r := fullInspectResult()
		mutate(&r)
		if r.valid() {
			t.Errorf("%s: accepted", name)
		}
	}
	// Absent is allowed: zero times, no remote, no reviews or threads.
	r := fullInspectResult()
	r.Remote, r.Pull.Reviews, r.Pull.Threads = "", nil, nil
	r.Pull.CreatedAt, r.Pull.MergedAt, r.Pull.ClosedAt, r.Pull.LastPushAt = 0, 0, 0, 0
	r.Pull.Base, r.Pull.Head = "", ""
	if !r.valid() {
		t.Error("a result with the detail absent was refused")
	}
	r = fullInspectResult()
	r.Pull.Reviews.Approved, r.Pull.Threads.Open = 10000, 10000
	r.Remote = strings.Repeat("a", 70) + "/" + strings.Repeat("b", 69)
	r.Pull.Base = strings.Repeat("x", 255)
	if !r.valid() {
		t.Error("values at the bound were refused")
	}
}

func TestAnOlderMachinesPullReadsAsDetailNotKnown(t *testing.T) {
	old := `{"ahead":0,"behind":0,"countsKnown":true,"changed":0,"conflicts":0,` +
		`"pull":{"number":7,"title":"t","url":"https://github.com/a/b/pull/7","state":"OPEN","draft":false,` +
		`"review":"APPROVED","mergeable":"MERGEABLE","added":1,"removed":1,"checks":[{"name":"ci","state":"SUCCESS"}]}}`
	var r InspectResult
	if err := json.Unmarshal([]byte(old), &r); err != nil {
		t.Fatal(err)
	}
	if !r.valid() {
		t.Fatal("an older machine's result was refused")
	}
	if r.Pull.DetailKnown || r.Pull.Reviews != nil || r.Pull.Threads != nil || r.Remote != "" {
		t.Fatalf("absent fields read as present: %+v", r.Pull)
	}
}

func TestAFullFortyCheckResultWithEveryNewFieldFitsTheFrame(t *testing.T) {
	r := fullInspectResult()
	for i := 0; i < 40; i++ {
		r.Pull.Checks = append(r.Pull.Checks, InspectCheck{
			Name:  fmt.Sprintf("build / matrix (linux, go-1.2%d) / unit-tests-%02d", i%10, i),
			State: "SUCCESS",
			URL:   fmt.Sprintf("https://github.com/eyupio/reet.sh/actions/runs/123456789%02d/job/987654321%02d", i, i),
		})
	}
	r.Pull.More = true
	if !r.valid() {
		b, _ := json.Marshal(r)
		t.Fatalf("refused at %d bytes (limit %d)", len(b), MaxInspectBytes)
	}
}
