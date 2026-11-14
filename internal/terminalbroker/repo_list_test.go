package terminalbroker

import (
	"errors"
	"reflect"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func repoListCapabilities() terminalproto.Capabilities {
	caps := browseCapabilities()
	caps.RepoList = true
	return caps
}

func TestListReposIsForwardedAndNeedsTheCapability(t *testing.T) {
	f := browseFixture(t, repoListCapabilities())
	want := terminalproto.RepoListResult{Repos: []terminalproto.RepoListEntry{{Name: "acme/site", Private: true, PushedAt: 1700000000}}, More: true}
	f.answering(func(terminalproto.Body) terminalproto.Body { return want })
	got, err := f.b.ListRepos(t.Context(), f.uid, f.agent)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
	if sent := f.sentRequests(); len(sent) != 1 || !reflect.DeepEqual(sent[0], terminalproto.RepoList{}) {
		t.Fatalf("request not forwarded exactly: %+v", sent)
	}

	// A refusal comes back as the machine's reason, for the caller to word.
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.RepoListResult{Reason: terminalproto.RepoListNotSignedIn}
	})
	if got, err := f.b.ListRepos(t.Context(), f.uid, f.agent); err != nil || got.Reason != terminalproto.RepoListNotSignedIn {
		t.Fatal(got, err)
	}

	// A machine that can clone but shipped before the list is never asked.
	old := browseFixture(t, repoCloneCapabilities())
	if _, err := old.b.ListRepos(t.Context(), old.uid, old.agent); !errors.Is(err, ErrRepoListUnsupported) {
		t.Fatalf("err = %v, want unsupported", err)
	}
	if len(old.sentRequests()) != 0 {
		t.Fatal("a listing was sent to a machine that did not negotiate it")
	}
}
