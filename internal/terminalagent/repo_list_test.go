//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const repoListFeatures = allFeatures + "," + terminalproto.RepoListFeature

// fakeListRepos replaces the connector's listing for one test: each call is
// counted, waits for release (when given) or its context, then answers result.
func fakeListRepos(t *testing.T, result terminalproto.RepoListResult, release chan struct{}) *atomic.Int32 {
	t.Helper()
	original := listRepos
	var calls atomic.Int32
	listRepos = func(ctx context.Context, check, list terminalhost.GHRunner) terminalproto.RepoListResult {
		calls.Add(1)
		if check == nil || list == nil {
			t.Error("listing run without gh")
		}
		if release != nil {
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
		return result
	}
	t.Cleanup(func() { listRepos = original })
	return &calls
}

func TestRepoListRequiresTheNegotiatedFeature(t *testing.T) {
	calls := fakeListRepos(t, terminalproto.RepoListResult{}, nil)
	handle, _, _ := browseConnector(t, allFeatures+","+terminalproto.RepoCloneFeature, nil)
	if err := handle(t.Context(), message("repo-list-unasked-01", &terminalproto.RepoList{})); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("listed")
	}
}

// With terminal access off the listing is refused as denied, as a clone is.
func TestRepoListRequiresTerminalAccess(t *testing.T) {
	calls := fakeListRepos(t, terminalproto.RepoListResult{}, nil)
	for _, consent := range []func() error{nil, func() error { return errors.New("off") }} {
		handle, got, _ := browseConnector(t, repoListFeatures, func(o *terminalhost.ManagerOptions) { o.Consent = consent })
		if err := handle(t.Context(), message("repo-list-denied-001", &terminalproto.RepoList{})); err != nil {
			t.Fatal(err)
		}
		reply := next(t, got, "repo-list-denied-001")
		if !reflect.DeepEqual(reply.Body, terminalproto.RepoListResult{Reason: terminalproto.RepoListDenied}) {
			t.Fatal(reply.Body)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("listed")
	}
}

// A listing is answered with what the machine's gh gave.
func TestRepoListAnswersTheList(t *testing.T) {
	want := terminalproto.RepoListResult{Repos: []terminalproto.RepoListEntry{{Name: "eyupio/reet.sh", Private: true, PushedAt: 1700000000}}, More: true}
	calls := fakeListRepos(t, want, nil)
	handle, got, _ := browseConnector(t, repoListFeatures, nil)
	if err := handle(t.Context(), message("repo-list-request-01", &terminalproto.RepoList{})); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "repo-list-request-01"); !reflect.DeepEqual(reply.Body, want) {
		t.Fatal(reply.Body)
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}

// One listing runs at a time on a connection: a second is answered busy at
// once, and the first still gets its list.
func TestRepoListOneAtATime(t *testing.T) {
	release := make(chan struct{})
	want := terminalproto.RepoListResult{Repos: []terminalproto.RepoListEntry{{Name: "a/b"}}}
	calls := fakeListRepos(t, want, release)
	handle, got, _ := browseConnector(t, repoListFeatures, nil)
	if err := handle(t.Context(), message("repo-list-first-0001", &terminalproto.RepoList{})); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if err := handle(t.Context(), message("repo-list-second-001", &terminalproto.RepoList{})); err != nil {
		t.Fatal(err)
	}
	if reply := next(t, got, "repo-list-second-001"); !reflect.DeepEqual(reply.Body, terminalproto.RepoListResult{Reason: terminalproto.RepoListBusy}) {
		t.Fatal(reply.Body)
	}
	close(release)
	if reply := next(t, got, "repo-list-first-0001"); !reflect.DeepEqual(reply.Body, want) {
		t.Fatal(reply.Body)
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}

// The connector answers before the controller gives up.
func TestRepoListAnswerBoundSitsUnderTheControllers(t *testing.T) {
	if repoListAnswerAfter <= 30*time.Second {
		t.Fatal("the connector answers before the listing's own deadline", repoListAnswerAfter)
	}
	if repoListAnswerAfter >= 40*time.Second {
		t.Fatal("the connector answers after the controller gives up", repoListAnswerAfter)
	}
}
