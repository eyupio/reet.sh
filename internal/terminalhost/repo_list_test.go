package terminalhost

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// signedIn is a sign-in check that passes.
func signedIn(context.Context, ...string) ([]byte, error) { return nil, nil }

// pages answers each listing page from its slice, recording the arguments.
func pages(calls *[][]string, page ...string) GHRunner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		*calls = append(*calls, args)
		n := len(*calls)
		if n > len(page) {
			return []byte("[]"), nil
		}
		return []byte(page[n-1]), nil
	}
}

// fullPage is a page of repoListPage entries named o/<prefix><i>.
func fullPage(prefix string) string {
	entries := make([]string, repoListPage)
	for i := range entries {
		entries[i] = fmt.Sprintf(`{"name":"o/%s%d","private":false,"pushedAt":"2026-11-14T10:00:00Z"}`, prefix, i)
	}
	return "[" + strings.Join(entries, ",") + "]"
}

func TestListReposAsksGitHubForEveryRepositoryTheSignInReaches(t *testing.T) {
	var calls [][]string
	got := ListRepos(context.Background(), signedIn, pages(&calls,
		`[{"name":"eyupio/reet.sh","private":true,"pushedAt":"2026-11-14T10:00:00Z"},{"name":"someone/shared","private":false,"pushedAt":null}]`))
	want := terminalproto.RepoListResult{Repos: []terminalproto.RepoListEntry{
		{Name: "eyupio/reet.sh", Private: true, PushedAt: time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC).Unix()},
		{Name: "someone/shared"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListRepos = %+v, want %+v", got, want)
	}
	// One page: it was short, so there is nothing after it.
	wantArgs := []string{"api", "--hostname", "github.com", "--method", "GET", "user/repos",
		"-f", "affiliation=owner,collaborator,organization_member", "-f", "sort=pushed", "-f", "direction=desc",
		"-f", "per_page=100", "-f", "page=1", "--jq", repoListJQ}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], wantArgs) {
		t.Fatalf("gh was run as %q, want once as %q", calls, wantArgs)
	}
	if !got.Repos[0].Private || got.More {
		t.Fatal(got)
	}
}

func TestListReposStopsAtTheLimitAndSaysThereMayBeMore(t *testing.T) {
	var calls [][]string
	got := ListRepos(context.Background(), signedIn, pages(&calls, fullPage("a"), fullPage("b")))
	if len(calls) != 2 || !slices.Contains(calls[1], "page=2") {
		t.Fatalf("pages asked for: %q", calls)
	}
	if len(got.Repos) != terminalproto.MaxRepoListEntries || !got.More || got.Reason != "" {
		t.Fatalf("got %d repos, more=%v, reason=%q", len(got.Repos), got.More, got.Reason)
	}
	// A full list goes out in one control message.
	if _, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: strings.Repeat("a", 24), Generation: 1, Body: got}); err != nil {
		t.Fatal("a full list does not encode:", err)
	}
	// A second page that is not full ends the list without More.
	calls = nil
	got = ListRepos(context.Background(), signedIn, pages(&calls, fullPage("a"), `[{"name":"o/last"}]`))
	if len(got.Repos) != repoListPage+1 || got.More {
		t.Fatalf("got %d repos, more=%v", len(got.Repos), got.More)
	}
}

func TestListReposLeavesOutNamesItCannotUseAndRepeats(t *testing.T) {
	var calls [][]string
	got := ListRepos(context.Background(), signedIn, pages(&calls,
		`[{"name":"a/b"},{"name":"A/B"},{"name":"no-slash"},{"name":"-bad/x"},{"name":"a/b/c"},{"name":"c/d","pushedAt":"not a time"}]`))
	want := []terminalproto.RepoListEntry{{Name: "a/b"}, {Name: "c/d"}}
	if !reflect.DeepEqual(got.Repos, want) || got.Reason != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestListReposRefusals(t *testing.T) {
	failing := func(err error) GHRunner {
		return func(context.Context, ...string) ([]byte, error) { return nil, err }
	}
	answering := func(out string) GHRunner {
		return func(context.Context, ...string) ([]byte, error) { return []byte(out), nil }
	}
	for name, tc := range map[string]struct {
		check, list GHRunner
		want        terminalproto.RepoListReason
	}{
		"not signed in":         {failing(errors.New("exit status 1")), answering("[]"), terminalproto.RepoListNotSignedIn},
		"gh missing":            {failing(errGHMissing), answering("[]"), terminalproto.RepoListDenied},
		"a page fails":          {signedIn, failing(errors.New("exit status 1")), terminalproto.RepoListFailed},
		"output cut short":      {signedIn, failing(errGHOutputTooLong), terminalproto.RepoListFailed},
		"not JSON":              {signedIn, answering("HTTP 502"), terminalproto.RepoListFailed},
		"a page over the limit": {signedIn, answering(fullPage("a")[:len(fullPage("a"))-1] + `,{"name":"x/y"}]`), terminalproto.RepoListFailed},
		"no runners":            {nil, nil, terminalproto.RepoListFailed},
	} {
		if got := ListRepos(context.Background(), tc.check, tc.list); got.Reason != tc.want || len(got.Repos) != 0 {
			t.Errorf("%s: got %+v, want reason %q", name, got, tc.want)
		}
	}
}

func TestListReposTimesOutWithTheRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	waiting := func(ctx context.Context, _ ...string) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() }
	if got := ListRepos(ctx, waiting, waiting); got.Reason != terminalproto.RepoListTimedOut {
		t.Fatalf("sign-in check: %+v", got)
	}
	if got := ListRepos(ctx, signedIn, waiting); got.Reason != terminalproto.RepoListTimedOut {
		t.Fatalf("listing: %+v", got)
	}
}
