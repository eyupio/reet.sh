//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const modelsFeatures = terminalproto.ModelsFeature

// modelsConnector builds a connector from a private temp layout and home. No
// tmux server or session is needed: a Models request never launches an agent.
func modelsConnector(t *testing.T, features string, edit func(*terminalhost.ManagerOptions)) (func(context.Context, terminalproto.Message) error, replies) {
	t.Helper()
	base, err := os.MkdirTemp("/tmp", "reet-models-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	if base, err = filepath.EvalSymlinks(base); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	config := filepath.Join(base, "agent.json")
	if err := os.WriteFile(config, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	id := "enrollment-0123456789"
	layout, err := terminalhost.NewEnrollmentLayout(config, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := terminalhost.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	opts := terminalhost.ManagerOptions{EnrollmentID: id, Layout: layout, Home: home, Consent: func() error { return nil }, ModelsConsent: func() error { return nil }}
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
	return handle, got
}

func replaceListModels(t *testing.T, fn func(*terminalhost.Manager, context.Context, string) terminalproto.ModelsResult) {
	t.Helper()
	original := listModels
	listModels = fn
	t.Cleanup(func() { listModels = original })
}

func waitModelsIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for modelsRunning.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("models work still running")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestModelsNeedsNegotiation(t *testing.T) {
	replaceListModels(t, func(*terminalhost.Manager, context.Context, string) terminalproto.ModelsResult {
		t.Fatal("ran without models-v1")
		return terminalproto.ModelsResult{}
	})
	for _, features := range []string{"", terminalproto.BrowseFeature} {
		handle, got := modelsConnector(t, features, nil)
		if err := handle(t.Context(), message("models-guard-000001", &terminalproto.Models{Tool: "opencode"})); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("features %q: %v", features, err)
		}
		nothingSent(t, got)
	}
}

func TestModelsSwitchOff(t *testing.T) {
	replaceListModels(t, func(*terminalhost.Manager, context.Context, string) terminalproto.ModelsResult {
		t.Fatal("ran while switched off")
		return terminalproto.ModelsResult{}
	})
	for name, consent := range map[string]func() error{
		"off": func() error { return errors.New("models disabled locally") },
		"nil": nil,
	} {
		t.Run(name, func(t *testing.T) {
			handle, got := modelsConnector(t, modelsFeatures, func(opts *terminalhost.ManagerOptions) {
				opts.ModelsConsent = consent
			})
			if err := handle(t.Context(), message("models-off-0000001", &terminalproto.Models{Tool: "opencode"})); err != nil {
				t.Fatal(err)
			}
			body := next(t, got, "models-off-0000001").Body
			want := terminalproto.ModelsResult{Tool: "opencode", Reason: terminalproto.ModelsOff}
			if !reflect.DeepEqual(body, want) {
				t.Fatalf("body: %+v", body)
			}
		})
	}
}

func TestModelsUnsupportedToolNeverRuns(t *testing.T) {
	replaceListModels(t, func(*terminalhost.Manager, context.Context, string) terminalproto.ModelsResult {
		t.Fatal("ran a tool with no listing")
		return terminalproto.ModelsResult{}
	})
	handle, got := modelsConnector(t, modelsFeatures, nil)
	if err := handle(t.Context(), message("models-claude-00001", &terminalproto.Models{Tool: "claude"})); err != nil {
		t.Fatal(err)
	}
	want := terminalproto.ModelsResult{Tool: "claude", Reason: terminalproto.ModelsUnsupported}
	if body := next(t, got, "models-claude-00001").Body; !reflect.DeepEqual(body, want) {
		t.Fatalf("body: %+v", body)
	}
}

func TestModelsOneAtATime(t *testing.T) {
	waitModelsIdle(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	replaceListModels(t, func(_ *terminalhost.Manager, ctx context.Context, tool string) terminalproto.ModelsResult {
		started <- struct{}{}
		<-release
		return terminalproto.ModelsResult{Tool: tool, Models: []terminalproto.ModelEntry{{ID: "a/b", Provider: "a"}}}
	})
	handle, got := modelsConnector(t, modelsFeatures, nil)
	if err := handle(t.Context(), message("models-first-000001", &terminalproto.Models{Tool: "opencode"})); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first listing never started")
	}
	if err := handle(t.Context(), message("models-second-00001", &terminalproto.Models{Tool: "opencode"})); err != nil {
		t.Fatal(err)
	}
	want := terminalproto.ModelsResult{Tool: "opencode", Reason: terminalproto.ModelsConflict}
	if body := next(t, got, "models-second-00001").Body; !reflect.DeepEqual(body, want) {
		t.Fatalf("second: %+v", body)
	}
	close(release)
	if body := next(t, got, "models-first-000001").Body.(terminalproto.ModelsResult); body.Reason != "" || len(body.Models) != 1 {
		t.Fatalf("first: %+v", body)
	}
	waitModelsIdle(t)
}

func TestModelsFloorReturnsTheLastResult(t *testing.T) {
	original := modelListFloor
	modelListFloor = 100 * time.Millisecond
	t.Cleanup(func() { modelListFloor = original })
	var calls atomic.Int32
	replaceListModels(t, func(_ *terminalhost.Manager, _ context.Context, tool string) terminalproto.ModelsResult {
		n := calls.Add(1)
		return terminalproto.ModelsResult{Tool: tool, Models: []terminalproto.ModelEntry{{ID: "a/" + string(rune('0'+n)), Provider: "a"}}}
	})
	handle, got := modelsConnector(t, modelsFeatures, nil)
	if err := handle(t.Context(), message("models-floor-000001", &terminalproto.Models{Tool: "opencode"})); err != nil {
		t.Fatal(err)
	}
	first := next(t, got, "models-floor-000001").Body.(terminalproto.ModelsResult)
	if err := handle(t.Context(), message("models-floor-000002", &terminalproto.Models{Tool: "opencode"})); err != nil {
		t.Fatal(err)
	}
	second := next(t, got, "models-floor-000002").Body.(terminalproto.ModelsResult)
	if !reflect.DeepEqual(first, second) || calls.Load() != 1 {
		t.Fatalf("within the floor: first %+v second %+v calls %d", first, second, calls.Load())
	}
	time.Sleep(150 * time.Millisecond)
	if err := handle(t.Context(), message("models-floor-000003", &terminalproto.Models{Tool: "opencode"})); err != nil {
		t.Fatal(err)
	}
	third := next(t, got, "models-floor-000003").Body.(terminalproto.ModelsResult)
	if reflect.DeepEqual(first, third) || calls.Load() != 2 {
		t.Fatalf("after the floor: first %+v third %+v calls %d", first, third, calls.Load())
	}
}

func TestModelsReplyShapeAndOperation(t *testing.T) {
	var gotTool atomic.Value
	replaceListModels(t, func(_ *terminalhost.Manager, _ context.Context, tool string) terminalproto.ModelsResult {
		gotTool.Store(tool)
		return terminalproto.ModelsResult{Tool: tool, Models: []terminalproto.ModelEntry{{ID: "anthropic/x", Provider: "anthropic"}}, More: true}
	})
	handle, got := modelsConnector(t, modelsFeatures, nil)
	if err := handle(t.Context(), message("models-shape-000001", &terminalproto.Models{Tool: "opencode"})); err != nil {
		t.Fatal(err)
	}
	body := next(t, got, "models-shape-000001").Body
	if gotTool.Load() != "opencode" {
		t.Fatalf("listing ran for %v", gotTool.Load())
	}
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: "models-shape-000001", Generation: 1, Body: body}); err != nil {
		t.Fatalf("reply is not a valid message: %+v %v", body, err)
	}
	want := terminalproto.ModelsResult{Tool: "opencode", Models: []terminalproto.ModelEntry{{ID: "anthropic/x", Provider: "anthropic"}}, More: true}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("body: %+v", body)
	}
	waitModelsIdle(t)
}

func TestModelsLateRunAnswersTimedOut(t *testing.T) {
	waitModelsIdle(t)
	original := modelsAnswerAfter
	modelsAnswerAfter = 100 * time.Millisecond
	t.Cleanup(func() { modelsAnswerAfter = original })
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	replaceListModels(t, func(_ *terminalhost.Manager, _ context.Context, tool string) terminalproto.ModelsResult {
		started <- struct{}{}
		<-release
		return terminalproto.ModelsResult{Tool: tool}
	})
	handle, got := modelsConnector(t, modelsFeatures, nil)
	if err := handle(t.Context(), message("models-late-0000001", &terminalproto.Models{Tool: "opencode"})); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the listing never started")
	}
	want := terminalproto.ModelsResult{Tool: "opencode", Reason: terminalproto.ModelsTimedOut}
	if body := next(t, got, "models-late-0000001").Body; !reflect.DeepEqual(body, want) {
		t.Fatalf("body: %+v", body)
	}
	close(release)
	waitModelsIdle(t)
}
