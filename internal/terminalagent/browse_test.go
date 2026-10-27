//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const allFeatures = terminalproto.LaunchFeature + "," + terminalproto.WorktreeFeature + "," + terminalproto.BrowseFeature + "," + terminalproto.BranchesFeature

// browseConnector is a connector with home holding one repository root,
// site. No tmux is needed: browse and branches never use the manager.
func browseConnector(t *testing.T, features string, edit func(*terminalhost.ManagerOptions)) (func(context.Context, terminalproto.Message) error, replies, string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "site", ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "site", ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := terminalhost.ManagerOptions{
		EnrollmentID:  "enrollment-0123456789",
		Home:          home,
		Consent:       func() error { return nil },
		BrowseConsent: func() error { return nil },
		Git:           cannedBranches(nil),
	}
	if edit != nil {
		edit(&opts)
	}
	headers := http.Header{}
	if features != "" {
		headers.Set(terminalproto.FeatureHeader, features)
	}
	got := make(replies, 64)
	handle, cleanup := Factory(opts)(t.Context(), http.DefaultClient, headers, 1, got.send)
	t.Cleanup(cleanup)
	return handle, got, home
}

// cannedBranches answers a listing with main as the only branch; a
// for-each-ref first waits for hold, when given, or its context.
func cannedBranches(hold chan struct{}) terminalhost.GitRunner {
	return func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		switch args[0] {
		case "for-each-ref":
			if hold != nil {
				select {
				case <-hold:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return []byte("refs/heads/main\n"), nil
		case "symbolic-ref":
			if args[len(args)-1] == "HEAD" {
				return []byte("main\n"), nil
			}
		}
		return nil, errors.New("exit status 1")
	}
}

// gatedBrowse replaces the connector's browse for one test. Each call waits
// for its release (or its context), saying it started first.
type gatedBrowse struct {
	mu      sync.Mutex
	calls   int
	started chan int
	release chan struct{}
	all     bool // hold every call, not only the first
	once    sync.Once
}

func holdBrowse(t *testing.T, all bool) *gatedBrowse {
	g := &gatedBrowse{started: make(chan int, 64), release: make(chan struct{}), all: all}
	original := browse
	browse = func(ctx context.Context, home string, req terminalproto.Browse, fsys terminalhost.FS, now func() time.Time) terminalproto.BrowseResult {
		g.mu.Lock()
		g.calls++
		n := g.calls
		g.mu.Unlock()
		if n == 1 || g.all {
			g.started <- n
			// A stuck syscall does not see its context: wait for release only.
			<-g.release
		}
		return original(ctx, home, req, fsys, now)
	}
	t.Cleanup(func() { browse = original; g.open() })
	return g
}

func (g *gatedBrowse) open() { g.once.Do(func() { close(g.release) }) }

func (g *gatedBrowse) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func waitStarted(t *testing.T, g *gatedBrowse) {
	t.Helper()
	select {
	case <-g.started:
	case <-time.After(10 * time.Second):
		t.Fatal("browse never started")
	}
}

func next(t *testing.T, got replies, id string) terminalproto.Message {
	t.Helper()
	select {
	case m := <-got:
		if m.RequestID != id {
			t.Fatalf("reply to %s, want %s: %+v", m.RequestID, id, m)
		}
		if _, err := terminalproto.Encode(m); err != nil {
			t.Fatalf("reply is not a valid message: %+v", m.Body)
		}
		return m
	case <-time.After(10 * time.Second):
		t.Fatal("no reply to", id)
	}
	return terminalproto.Message{}
}

func nothingSent(t *testing.T, got replies) {
	t.Helper()
	select {
	case m := <-got:
		t.Fatalf("sent %+v", m)
	case <-time.After(100 * time.Millisecond):
	}
}

// waitIdle waits until no browse or branches work runs in this process.
func waitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for browsesRunning.Load() != 0 || branchesRunning.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("browse or branches work still running")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A browse is answered from its own goroutine; a second one while it runs is
// refused as conflict at once, and other messages are still served.
func TestAgentBrowseOneAtATime(t *testing.T) {
	waitIdle(t)
	g := holdBrowse(t, false)
	handle, got, home := browseConnector(t, allFeatures, nil)
	start := time.Now()
	if err := handle(t.Context(), message("browse-first-000001", &terminalproto.Browse{})); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("the dispatcher waited for the browse")
	}
	waitStarted(t, g)
	start = time.Now()
	if err := handle(t.Context(), message("browse-second-00001", &terminalproto.Browse{Path: home})); err != nil {
		t.Fatal(err)
	}
	if m := next(t, got, "browse-second-00001"); !reflect.DeepEqual(m.Body, terminalproto.BrowseResult{Reason: terminalproto.BrowseConflict}) {
		t.Fatalf("second browse: %+v", m.Body)
	}
	if time.Since(start) > time.Second {
		t.Fatal("conflict was not at once")
	}
	if g.count() != 1 {
		t.Fatal("the second browse ran", g.count())
	}
	// Branches has its own slot: a listing is not held up by a browse.
	if err := handle(t.Context(), message("branches-during-01", &terminalproto.Branches{Dir: filepath.Join(home, "site")})); err != nil {
		t.Fatal(err)
	}
	if m := next(t, got, "branches-during-01"); !reflect.DeepEqual(m.Body, terminalproto.BranchesResult{Current: "main", Branches: []string{"main"}}) {
		t.Fatalf("branches during a browse: %+v", m.Body)
	}
	// Other messages are still served.
	if err := handle(t.Context(), message("reconcile-during-01", &terminalproto.Reconcile{})); err != nil {
		t.Fatal(err)
	}
	next(t, got, "reconcile-during-01")
	g.open()
	m := next(t, got, "browse-first-000001")
	r, ok := m.Body.(terminalproto.BrowseResult)
	if !ok || r.Reason != "" || r.Path != home || len(r.Entries) != 1 || r.Entries[0].Name != "site" || !r.Entries[0].RepoRoot {
		t.Fatalf("first browse: %+v", m.Body)
	}
	// The slot is free again.
	if err := handle(t.Context(), message("browse-third-000001", &terminalproto.Browse{})); err != nil {
		t.Fatal(err)
	}
	if r := next(t, got, "browse-third-000001").Body.(terminalproto.BrowseResult); r.Reason != "" || r.Path != home {
		t.Fatalf("third browse: %+v", r)
	}
}

// A browse stuck in a call the deadline cannot cut short (a hung mount) is
// answered unreadable at the connector's deadline and its slot freed, so the
// next browse runs. Stuck browses are bounded: past the cap a browse is
// refused as conflict until they finish.
func TestAgentBrowseFreesItsSlotAtTheDeadline(t *testing.T) {
	waitIdle(t)
	original := browseAnswerAfter
	browseAnswerAfter = 200 * time.Millisecond
	t.Cleanup(func() { browseAnswerAfter = original })
	if original != 4*time.Second || maxAsideRunning != 4 {
		t.Fatal("bounds changed", original, maxAsideRunning)
	}
	g := holdBrowse(t, true)
	handle, got, _ := browseConnector(t, allFeatures, nil)
	for i := range maxAsideRunning {
		id := "browse-stuck-00000" + string(rune('0'+i))
		start := time.Now()
		if err := handle(t.Context(), message(id, &terminalproto.Browse{})); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, g)
		m := next(t, got, id)
		if !reflect.DeepEqual(m.Body, terminalproto.BrowseResult{Reason: terminalproto.BrowseUnreadable}) {
			t.Fatalf("stuck browse %d: %+v", i, m.Body)
		}
		if d := time.Since(start); d < 150*time.Millisecond || d > 5*time.Second {
			t.Fatal("not answered at the deadline", d)
		}
	}
	// The slot is free, but the cap of stuck browses is reached.
	if err := handle(t.Context(), message("browse-over-cap-001", &terminalproto.Browse{})); err != nil {
		t.Fatal(err)
	}
	if m := next(t, got, "browse-over-cap-001"); !reflect.DeepEqual(m.Body, terminalproto.BrowseResult{Reason: terminalproto.BrowseConflict}) {
		t.Fatalf("over the cap: %+v", m.Body)
	}
	if g.count() != maxAsideRunning {
		t.Fatal("a browse over the cap ran", g.count())
	}
	// When the stuck ones finish, their answers are dropped and browsing
	// works again.
	g.open()
	waitIdle(t)
	nothingSent(t, got)
	if err := handle(t.Context(), message("browse-after-00001", &terminalproto.Browse{})); err != nil {
		t.Fatal(err)
	}
	if r := next(t, got, "browse-after-00001").Body.(terminalproto.BrowseResult); r.Reason != "" {
		t.Fatalf("after: %+v", r)
	}
}

func TestAgentBranchesOneAtATime(t *testing.T) {
	waitIdle(t)
	hold := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(hold) }) })
	handle, got, home := browseConnector(t, allFeatures, func(o *terminalhost.ManagerOptions) { o.Git = cannedBranches(hold) })
	req := &terminalproto.Branches{Dir: filepath.Join(home, "site")}
	if err := handle(t.Context(), message("branches-first-0001", req)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for branchesRunning.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := handle(t.Context(), message("branches-second-001", req)); err != nil {
		t.Fatal(err)
	}
	if m := next(t, got, "branches-second-001"); !reflect.DeepEqual(m.Body, terminalproto.BranchesResult{Reason: terminalproto.BranchesConflict}) {
		t.Fatalf("second listing: %+v", m.Body)
	}
	once.Do(func() { close(hold) })
	if m := next(t, got, "branches-first-0001"); !reflect.DeepEqual(m.Body, terminalproto.BranchesResult{Current: "main", Branches: []string{"main"}}) {
		t.Fatalf("first listing: %+v", m.Body)
	}
	if m := branchesCall(t, handle, got, req); !reflect.DeepEqual(m, terminalproto.BranchesResult{Current: "main", Branches: []string{"main"}}) {
		t.Fatalf("slot not released: %+v", m)
	}
	if branchesAnswerAfter != 8*time.Second {
		t.Fatal(branchesAnswerAfter)
	}
}

func branchesCall(t *testing.T, handle func(context.Context, terminalproto.Message) error, got replies, req *terminalproto.Branches) terminalproto.Body {
	t.Helper()
	if err := handle(t.Context(), message("branches-call-00001", req)); err != nil {
		t.Fatal(err)
	}
	return next(t, got, "branches-call-00001").Body
}

// A browse or branches message on a connection that did not negotiate its
// feature is a protocol error: nothing is sent back (an old peer drops the
// connection on a kind it does not know) and nothing runs.
func TestAgentBrowseNotNegotiatedIsNeverAnswered(t *testing.T) {
	waitIdle(t)
	g := holdBrowse(t, false)
	g.open()
	for _, features := range []string{"", terminalproto.LaunchFeature + "," + terminalproto.WorktreeFeature, terminalproto.BranchesFeature, "browse-v2"} {
		ran := false
		handle, got, _ := browseConnector(t, features, func(o *terminalhost.ManagerOptions) {
			o.BrowseConsent = func() error { ran = true; return nil }
		})
		if err := handle(t.Context(), message("browse-unasked-0001", &terminalproto.Browse{})); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%q: %v", features, err)
		}
		nothingSent(t, got)
		if g.count() != 0 || ran {
			t.Fatalf("%q: a browse ran", features)
		}
	}
	for _, features := range []string{"", terminalproto.LaunchFeature + "," + terminalproto.WorktreeFeature, terminalproto.BrowseFeature, "branches-v2"} {
		listed := false
		handle, got, home := browseConnector(t, features, func(o *terminalhost.ManagerOptions) {
			o.Git = func(context.Context, string, ...string) ([]byte, error) { listed = true; return nil, nil }
		})
		if err := handle(t.Context(), message("branches-unasked-01", &terminalproto.Branches{Dir: filepath.Join(home, "site")})); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%q: %v", features, err)
		}
		nothingSent(t, got)
		if listed {
			t.Fatalf("%q: git ran", features)
		}
	}
	// Each is answered once negotiated on its own.
	handle, got, _ := browseConnector(t, terminalproto.BrowseFeature, nil)
	if err := handle(t.Context(), message("browse-asked-000001", &terminalproto.Browse{})); err != nil {
		t.Fatal(err)
	}
	next(t, got, "browse-asked-000001")
	handle, got, home := browseConnector(t, terminalproto.BranchesFeature, nil)
	if body := branchesCall(t, handle, got, &terminalproto.Branches{Dir: filepath.Join(home, "site")}); !reflect.DeepEqual(body, terminalproto.BranchesResult{Current: "main", Branches: []string{"main"}}) {
		t.Fatalf("%+v", body)
	}
}

// A machine that refuses browsing locally (reet terminal browse off, or
// terminal access off) answers denied and runs nothing; branches follow
// terminal access only.
func TestAgentBrowseRefusedLocally(t *testing.T) {
	waitIdle(t)
	g := holdBrowse(t, false)
	g.open()
	off := errors.New("off")
	for name, edit := range map[string]func(*terminalhost.ManagerOptions){
		"browse off":         func(o *terminalhost.ManagerOptions) { o.BrowseConsent = func() error { return off } },
		"no browse switch":   func(o *terminalhost.ManagerOptions) { o.BrowseConsent = nil },
		"terminal off":       func(o *terminalhost.ManagerOptions) { o.Consent = func() error { return off } },
		"no terminal switch": func(o *terminalhost.ManagerOptions) { o.Consent = nil },
	} {
		handle, got, home := browseConnector(t, allFeatures, edit)
		if err := handle(t.Context(), message("browse-refused-0001", &terminalproto.Browse{})); err != nil {
			t.Fatal(name, err)
		}
		if m := next(t, got, "browse-refused-0001"); !reflect.DeepEqual(m.Body, terminalproto.BrowseResult{Reason: terminalproto.BrowseOff}) {
			t.Fatalf("%s: %+v", name, m.Body)
		}
		if g.count() != 0 {
			t.Fatal(name, "a refused browse ran")
		}
		want := terminalproto.BranchesResult{Reason: terminalproto.BranchesDenied}
		if strings.HasPrefix(name, "browse") || name == "no browse switch" {
			want = terminalproto.BranchesResult{Current: "main", Branches: []string{"main"}}
		}
		if body := branchesCall(t, handle, got, &terminalproto.Branches{Dir: filepath.Join(home, "site")}); !reflect.DeepEqual(body, want) {
			t.Fatalf("%s: branches %+v", name, body)
		}
	}
}
