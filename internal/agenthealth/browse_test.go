package agenthealth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestBrowseCapabilityNeedsNegotiationTerminalAndTheLocalSwitch(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	tools := func() []string { return []string{"claude"} }
	git := func() bool { return true }
	full := func() AgentConfig {
		return AgentConfig{TerminalSession: factory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready, launch: true, worktree: true, browse: true, branches: true, browseAllowed: true, LaunchTools: tools, GitAvailable: git}
	}
	for _, tc := range []struct {
		name             string
		edit             func(*AgentConfig)
		browse, branches bool
	}{
		{"everything", func(*AgentConfig) {}, true, true},
		{"browse not negotiated", func(c *AgentConfig) { c.browse = false }, false, true},
		{"branches not negotiated", func(c *AgentConfig) { c.branches = false }, true, false},
		{"browse off locally", func(c *AgentConfig) { c.browseAllowed = false }, false, true},
		{"terminal access off", func(c *AgentConfig) { c.TerminalEnabled = false }, false, false},
		{"no terminal control", func(c *AgentConfig) { c.TerminalSession = nil }, false, false},
		{"no worktrees (git not found)", func(c *AgentConfig) { c.GitAvailable = func() bool { return false } }, true, false},
		{"no worktrees (not negotiated)", func(c *AgentConfig) { c.worktree = false }, true, false},
	} {
		cfg := full()
		tc.edit(&cfg)
		caps := capabilities(cfg)
		if err := caps.Validate(); err != nil {
			t.Fatal(tc.name, err)
		}
		data, _ := json.Marshal(caps)
		if caps.Browse != tc.browse || strings.Contains(string(data), `"browse"`) != tc.browse {
			t.Fatal(tc.name, "browse", string(data))
		}
		if caps.Branches != tc.branches || strings.Contains(string(data), `"branches"`) != tc.branches {
			t.Fatal(tc.name, "branches", string(data))
		}
	}
}

func TestBrowseFeaturesFollowTheControllerAdvertisement(t *testing.T) {
	for _, tc := range []struct {
		advertised       string
		browse, branches bool
	}{
		{"", false, false},
		{terminalproto.WorktreeFeature, false, false},
		{terminalproto.BrowseFeature, true, false},
		{terminalproto.BranchesFeature, false, true},
		{terminalproto.LaunchFeature + "," + terminalproto.WorktreeFeature + ", " + terminalproto.BrowseFeature + "," + terminalproto.BranchesFeature, true, true},
		{"browse-v2,branches-v2", false, false},
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, tc.advertised)
		if cfg.browse != tc.browse || cfg.branches != tc.branches {
			t.Fatal("negotiated", tc.advertised, cfg.browse, cfg.branches)
		}
		headers := http.Header{}
		setTerminalFeatures(headers, cfg)
		value := headers.Get(terminalproto.FeatureHeader)
		if terminalproto.SupportsBrowse(value) != tc.browse || terminalproto.SupportsBranches(value) != tc.branches {
			t.Fatal("terminal features header", tc.advertised, value)
		}
	}
}

// Turning browse off (or on) on the machine reaches the controller without a
// reconnect, as terminal consent does. A missing switch is off.
func TestBrowseSwitchChangeSendsCapabilities(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	if readBrowseAllowed(AgentConfig{}) {
		t.Fatal("no switch must mean no browse")
	}
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	var on atomic.Bool
	on.Store(true)
	cfg := AgentConfig{
		InstanceID: testInstance(), ConsentInterval: 5 * time.Millisecond, TerminalSession: factory, browse: true, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready,
		Consent:       func() (bool, terminalproto.Readiness, error) { return true, terminalproto.Ready, nil },
		BrowseAllowed: func() bool { return on.Load() },
	}
	cfg.browseAllowed = readBrowseAllowed(cfg)
	if !capabilities(cfg).Browse {
		t.Fatal("browse on is not reported")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan terminalproto.Message, 4)
	errs := make(chan error, 2)
	go consentControl(ctx, cfg, 3, true, terminalproto.Ready, func(_ context.Context, m terminalproto.Message) error { sent <- m; return nil }, errs)
	select {
	case m := <-sent:
		t.Fatal("sent with nothing changed", m)
	case <-time.After(50 * time.Millisecond):
	}
	for _, want := range []bool{false, true} {
		on.Store(want)
		select {
		case m := <-sent:
			hello, ok := m.Body.(terminalproto.Hello)
			if !ok || hello.Capabilities.Browse != want || !hello.Capabilities.TerminalEnabled {
				t.Fatalf("browse %t: %+v", want, m)
			}
		case err := <-errs:
			t.Fatal(err)
		case <-time.After(5 * time.Second):
			t.Fatal("no capability update for browse", want)
		}
	}
}

// A browse or branches request the dispatcher cannot take (its queue is
// full) is answered with its own kind, as busy, but only when its feature was
// negotiated: otherwise it is a protocol error and nothing is sent back, as
// the dispatcher itself would treat it.
func TestFullTerminalQueueAnswersBrowseOnlyWhenNegotiated(t *testing.T) {
	branches := &terminalproto.Branches{Dir: "/home/someone/site"}
	for _, tc := range []struct {
		features         string
		body             terminalproto.Body
		answered, ending bool
	}{
		{terminalproto.BrowseFeature, &terminalproto.Browse{}, true, false},
		{terminalproto.BranchesFeature, branches, true, false},
		{terminalproto.BrowseFeature + "," + terminalproto.BranchesFeature, branches, true, false},
		{"", &terminalproto.Browse{}, false, true},
		{terminalproto.BranchesFeature, &terminalproto.Browse{}, false, true},
		{"", branches, false, true},
		{terminalproto.BrowseFeature, branches, false, true},
	} {
		client, peer := controlPair(t)
		errorsSeen := make(chan error, 3)
		ctx, cancel := context.WithCancel(t.Context())
		release := make(chan struct{})
		var handled atomic.Int32
		done := make(chan struct{})
		go func() {
			defer close(done)
			readControlWithTerminal(ctx, client, 1, clientSender(client), errorsSeen, tc.features, func(ctx context.Context, m terminalproto.Message) error {
				if handled.Add(1) == 1 {
					select {
					case <-release:
					case <-ctx.Done():
					}
				}
				return nil
			})
		}()
		for i := 0; i <= terminalQueueSize; i++ {
			if err := peer.Write(t.Context(), websocket.MessageText, controlMessage(t, fmt.Sprintf("request-%012d", i), &terminalproto.Reconcile{})); err != nil {
				t.Fatal(err)
			}
		}
		for deadline := time.Now().Add(2 * time.Second); handled.Load() == 0 && time.Now().Before(deadline); {
			time.Sleep(5 * time.Millisecond)
		}
		if err := peer.Write(t.Context(), websocket.MessageText, controlMessage(t, "request-overflowing", tc.body)); err != nil {
			t.Fatal(err)
		}
		readCtx, readCancel := context.WithTimeout(t.Context(), time.Second)
		_, data, err := peer.Read(readCtx)
		readCancel()
		if tc.answered {
			if err != nil {
				t.Fatal(tc.features, "no answer", err)
			}
			m, err := terminalproto.Decode(data)
			if err != nil || m.RequestID != "request-overflowing" {
				t.Fatal(tc.features, m, err)
			}
			switch r := m.Body.(type) {
			case *terminalproto.BrowseResult:
				if !browseConflict(*r) {
					t.Fatalf("%s: %+v", tc.features, r)
				}
			case *terminalproto.BranchesResult:
				if !reflect.DeepEqual(*r, terminalproto.BranchesResult{Reason: terminalproto.BranchesConflict}) {
					t.Fatalf("%s: %+v", tc.features, r)
				}
			default:
				t.Fatalf("%s: answered %T", tc.features, m.Body)
			}
		} else if err == nil {
			t.Fatalf("%q: answered %s", tc.features, data)
		}
		select {
		case err := <-errorsSeen:
			if !tc.ending || !errors.Is(err, terminalproto.ErrInvalid) {
				t.Fatal(tc.features, "session ended", err)
			}
		case <-time.After(100 * time.Millisecond):
			if tc.ending {
				t.Fatal(tc.features, "an unnegotiated request did not end the session")
			}
		}
		close(release)
		cancel()
		<-done
	}
}

func browseConflict(r terminalproto.BrowseResult) bool {
	return r.Reason == terminalproto.BrowseConflict && r.Path == "" && len(r.Entries) == 0
}

// The control workers give the reader the features this connection
// negotiated, so a full queue answers a negotiated browse as busy.
func TestControlWorkersAnswerAnOverflowingBrowse(t *testing.T) {
	client, peer := controlPair(t)
	release := make(chan struct{})
	defer close(release)
	var handled atomic.Int32
	cfg := AgentConfig{
		InstanceID: testInstance(), CollectTimeout: time.Hour, ReportInterval: time.Hour, browse: true,
		Collector: collectorFunc(func(ctx context.Context, _ string) Sample { <-ctx.Done(); return Sample{} }),
		control: func(ctx context.Context, m terminalproto.Message) error {
			if handled.Add(1) == 1 {
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			return nil
		},
	}
	sequence := int64(0)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runControlWorkers(ctx, cfg, client, 1, &sequence, clientSender(client)) }()
	for i := 0; i <= terminalQueueSize; i++ {
		if err := peer.Write(t.Context(), websocket.MessageText, controlMessage(t, fmt.Sprintf("request-%012d", i), &terminalproto.Reconcile{})); err != nil {
			t.Fatal(err)
		}
	}
	for deadline := time.Now().Add(2 * time.Second); handled.Load() == 0 && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	if err := peer.Write(t.Context(), websocket.MessageText, controlMessage(t, "request-overflowing", &terminalproto.Browse{})); err != nil {
		t.Fatal(err)
	}
	readCtx, readCancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer readCancel()
	_, data, err := peer.Read(readCtx)
	if err != nil {
		t.Fatal("no answer", err)
	}
	m, err := terminalproto.Decode(data)
	if r, ok := m.Body.(*terminalproto.BrowseResult); err != nil || !ok || m.RequestID != "request-overflowing" || !browseConflict(*r) {
		t.Fatalf("%s %v", data, err)
	}
	cancel()
	<-done
}

// The connector serves a negotiated browse or branches request only while it
// advertises the matching capability, re-read for each request; otherwise it
// answers denied itself. Anything else, including a request whose feature
// was not negotiated (a protocol error there), goes to the terminal handler.
func TestMachineRequestsFollowTheAdvertisedCapabilities(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	full := func() AgentConfig {
		return AgentConfig{
			TerminalSession: factory, launch: true, worktree: true, browse: true, branches: true,
			LaunchTools:  func() []string { return []string{"claude"} },
			GitAvailable: func() bool { return true },
			// Consent probes readiness (tmux) and must never be used here.
			Consent: func() (bool, terminalproto.Readiness, error) {
				t.Error("the readiness probe ran for a browse or branches request")
				return true, terminalproto.Ready, nil
			},
			TerminalAllowed: func() bool { return true },
			BrowseAllowed:   func() bool { return true },
		}
	}
	browse := terminalproto.Message{Version: terminalproto.Version, RequestID: "browse-request-00001", Generation: 1, Body: &terminalproto.Browse{}}
	branches := terminalproto.Message{Version: terminalproto.Version, RequestID: "branches-request-0001", Generation: 1, Body: &terminalproto.Branches{Dir: "/home/someone/site"}}
	other := terminalproto.Message{Version: terminalproto.Version, RequestID: "reconcile-request-001", Generation: 1, Body: &terminalproto.Reconcile{}}
	browseDenied := terminalproto.BrowseResult{Reason: terminalproto.BrowseDenied}
	branchesDenied := terminalproto.BranchesResult{Reason: terminalproto.BranchesDenied}
	for _, tc := range []struct {
		name                     string
		edit                     func(*AgentConfig)
		browseServed, branchesOK bool
	}{
		{"everything on", func(*AgentConfig) {}, true, true},
		{"browse off locally", func(c *AgentConfig) { c.BrowseAllowed = func() bool { return false } }, false, true},
		{"no browse switch", func(c *AgentConfig) { c.BrowseAllowed = nil }, false, true},
		{"git not found", func(c *AgentConfig) { c.GitAvailable = func() bool { return false } }, true, false},
		{"worktree not negotiated", func(c *AgentConfig) { c.worktree = false }, true, false},
		{"launch not negotiated", func(c *AgentConfig) { c.launch = false }, true, false},
		{"terminal access off", func(c *AgentConfig) { c.TerminalAllowed = func() bool { return false } }, false, false},
		{"no terminal switch", func(c *AgentConfig) { c.TerminalAllowed = nil }, false, false},
	} {
		cfg := full()
		tc.edit(&cfg)
		var forwarded []string
		var sent []terminalproto.Message
		gate := gateMachineRequests(cfg, func(_ context.Context, m terminalproto.Message) error {
			forwarded = append(forwarded, m.RequestID)
			return nil
		}, func(_ context.Context, m terminalproto.Message) error { sent = append(sent, m); return nil })
		for _, m := range []terminalproto.Message{browse, branches, other} {
			if err := gate(t.Context(), m); err != nil {
				t.Fatal(tc.name, err)
			}
		}
		want := []string{}
		var wantSent []terminalproto.Body
		if tc.browseServed {
			want = append(want, browse.RequestID)
		} else {
			wantSent = append(wantSent, browseDenied)
		}
		if tc.branchesOK {
			want = append(want, branches.RequestID)
		} else {
			wantSent = append(wantSent, branchesDenied)
		}
		want = append(want, other.RequestID)
		if strings.Join(forwarded, ",") != strings.Join(want, ",") {
			t.Fatalf("%s: forwarded %q want %q", tc.name, forwarded, want)
		}
		var gotSent []terminalproto.Body
		for _, m := range sent {
			if m.Generation != 1 || (m.RequestID != browse.RequestID && m.RequestID != branches.RequestID) {
				t.Fatalf("%s: envelope %+v", tc.name, m)
			}
			gotSent = append(gotSent, m.Body)
		}
		if !reflect.DeepEqual(gotSent, wantSent) {
			t.Fatalf("%s: sent %+v want %+v", tc.name, gotSent, wantSent)
		}
	}
	// A request whose feature was not negotiated is not answered here.
	cfg := full()
	cfg.browse, cfg.branches, cfg.BrowseAllowed, cfg.GitAvailable = false, false, nil, nil
	var forwarded int
	gate := gateMachineRequests(cfg, func(context.Context, terminalproto.Message) error { forwarded++; return terminalproto.ErrInvalid },
		func(_ context.Context, m terminalproto.Message) error { t.Fatal("answered", m); return nil })
	for _, m := range []terminalproto.Message{browse, branches} {
		if err := gate(t.Context(), m); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(err)
		}
	}
	if forwarded != 2 {
		t.Fatal("unnegotiated requests not passed on", forwarded)
	}
}

// The control workers put that gate in front of the terminal handler.
func TestControlWorkersRefuseBrowseWhenOffLocally(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	client, peer := controlPair(t)
	var handled atomic.Int32
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	cfg := AgentConfig{
		InstanceID: testInstance(), CollectTimeout: time.Hour, ReportInterval: time.Hour, ConsentInterval: time.Hour, browse: true,
		TerminalSession: factory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready,
		TerminalAllowed: func() bool { return true },
		BrowseAllowed:   func() bool { return false },
		Collector:       collectorFunc(func(ctx context.Context, _ string) Sample { <-ctx.Done(); return Sample{} }),
		control:         func(context.Context, terminalproto.Message) error { handled.Add(1); return nil },
	}
	sequence := int64(0)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runControlWorkers(ctx, cfg, client, 1, &sequence, clientSender(client)) }()
	if err := peer.Write(t.Context(), websocket.MessageText, controlMessage(t, "request-browse-off01", &terminalproto.Browse{})); err != nil {
		t.Fatal(err)
	}
	readCtx, readCancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer readCancel()
	_, data, err := peer.Read(readCtx)
	if err != nil {
		t.Fatal("no answer", err)
	}
	m, err := terminalproto.Decode(data)
	if r, ok := m.Body.(*terminalproto.BrowseResult); err != nil || !ok || m.RequestID != "request-browse-off01" || r.Reason != terminalproto.BrowseDenied {
		t.Fatalf("%s %v", data, err)
	}
	if handled.Load() != 0 {
		t.Fatal("the terminal handler saw a browse the connector does not offer")
	}
	cancel()
	<-done
}

// Deciding whether to serve a browse or branches request reads only cheap
// local state: it never runs the readiness probe (tmux, seconds when tmux is
// wedged) on the dispatcher, so the connection's other requests never wait.
func TestGateNeverWaitsOnTheReadinessProbe(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	var probes atomic.Int32
	wedged := make(chan struct{})
	defer close(wedged)
	cfg := AgentConfig{
		TerminalSession: factory, launch: true, worktree: true, browse: true, branches: true,
		LaunchTools:  func() []string { return []string{"claude"} },
		GitAvailable: func() bool { return true },
		Consent: func() (bool, terminalproto.Readiness, error) {
			probes.Add(1)
			<-wedged
			return true, terminalproto.Ready, nil
		},
		TerminalAllowed: func() bool { return true },
		BrowseAllowed:   func() bool { return false },
	}
	var forwarded atomic.Int32
	gate := gateMachineRequests(cfg, func(context.Context, terminalproto.Message) error { forwarded.Add(1); return nil },
		func(context.Context, terminalproto.Message) error { return nil })
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, body := range []terminalproto.Body{&terminalproto.Browse{}, &terminalproto.Branches{Dir: "/home/someone/site"}} {
			_ = gate(context.Background(), terminalproto.Message{Version: terminalproto.Version, RequestID: "gate-request-000001", Generation: 1, Body: body})
		}
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("a browse or branches request waited on the readiness probe")
	}
	if n := probes.Load(); n != 0 {
		t.Fatal("readiness probed", n)
	}
	if forwarded.Load() != 1 {
		t.Fatal("branches not served, or browse served while off", forwarded.Load())
	}
}
