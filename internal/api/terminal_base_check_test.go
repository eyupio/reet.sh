package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// baseCheckCaps is a machine that fetches a worktree's base from GitHub, so
// it can be asked where a session would start.
func baseCheckCaps() terminalproto.Capabilities {
	caps := browseCaps()
	caps.BaseFresh = true
	return caps
}

func baseCheckPath(agent, dir, base string) string {
	q := url.Values{"dir": {dir}}
	if base != "" {
		q.Set("base", base)
	}
	return "/api/v2/machines/" + agent + "/base-check?" + q.Encode()
}

// baseCheckProject saves the repository the checks below ask about, with base
// as the base work on it starts from.
func baseCheckProject(t *testing.T, f *browseAPI, base string) {
	t.Helper()
	_, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent, Dir: browseTestHome + "/site", Name: "Site", IsRepo: true, BaseBranch: base})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBaseCheckRouteRefusesAnOlderMachine(t *testing.T) {
	f := browseAPIFixture(t, browseCaps()) // no base-fresh-v1
	code, out, raw := f.get(t, f.cookie, baseCheckPath(f.agent, browseTestHome+"/site", "main"))
	if code != http.StatusConflict || out["code"] != "base_check_unsupported" {
		t.Fatalf("older machine = %d %v %s", code, out, raw)
	}
	if out["error"] != "Update reet on that machine to check a branch against GitHub." {
		t.Fatalf("sentence = %v", out["error"])
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatalf("a check reached an older machine: %+v", sent)
	}
}

func TestBaseCheckRouteFillsTheProjectsBase(t *testing.T) {
	f := browseAPIFixture(t, baseCheckCaps())
	baseCheckProject(t, f, "develop")
	local, start := strings.Repeat("cd", 20), strings.Repeat("ab", 20)
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BaseCheckResult{State: terminalproto.BaseState{
			Base: "develop", Relation: terminalproto.BaseBehind, Local: local, Remote: start, Start: start}}
	})
	code, out, raw := f.get(t, f.cookie, baseCheckPath(f.agent, browseTestHome+"/site", ""))
	if code != 200 {
		t.Fatalf("check = %d %s", code, raw)
	}
	sent := f.sentRequests()
	if len(sent) != 1 {
		t.Fatalf("sent %+v", sent)
	}
	check, ok := sent[0].(terminalproto.BaseCheck)
	if !ok || check.Base != "develop" || check.Dir != browseTestHome+"/site" {
		t.Fatalf("the project's base did not reach the machine: %+v", sent[0])
	}
	want := map[string]any{"base": "develop", "relation": "behind", "local": local, "remote": start, "start": start}
	if len(out) != len(want) {
		t.Fatalf("answer has fields beyond the five named ones: %s", raw)
	}
	for k, v := range want {
		if out[k] != v {
			t.Fatalf("%s = %v, want %v", k, out[k], v)
		}
	}
	// A base the browser names is sent as it stands, project or no project.
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BaseCheckResult{State: terminalproto.BaseState{Base: "release", Relation: terminalproto.BaseSame, Start: start}}
	})
	if code, _, raw := f.get(t, f.cookie, baseCheckPath(f.agent, browseTestHome+"/site", "release")); code != 200 {
		t.Fatalf("named base = %d %s", code, raw)
	}
	if sent = f.sentRequests(); len(sent) != 2 || sent[1].(terminalproto.BaseCheck).Base != "release" {
		t.Fatalf("a named base was overwritten: %+v", sent)
	}
}

// A project that saved no base of its own falls back to the account's
// default, as an isolated launch does, so the branch the browser asks about
// is the one the session will start from.
func TestBaseCheckRouteFallsBackToTheAccountsDefaultBase(t *testing.T) {
	f := browseAPIFixture(t, baseCheckCaps())
	baseCheckProject(t, f, "")
	profile, err := f.st.ProfileFor(f.uid)
	if err != nil {
		t.Fatal(err)
	}
	profile.DefaultBaseBranch = "trunk"
	if err := f.st.SaveProfileFor(f.uid, profile); err != nil {
		t.Fatal(err)
	}
	start := strings.Repeat("ab", 20)
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BaseCheckResult{State: terminalproto.BaseState{Base: "trunk", Relation: terminalproto.BaseSame, Start: start}}
	})
	if code, _, raw := f.get(t, f.cookie, baseCheckPath(f.agent, browseTestHome+"/site", "")); code != 200 {
		t.Fatalf("check = %d %s", code, raw)
	}
	sent := f.sentRequests()
	if len(sent) != 1 || sent[0].(terminalproto.BaseCheck).Base != "trunk" {
		t.Fatalf("the check did not ask about the account's default base: %+v", sent)
	}
}

// The answer carries only what the machine sent: a state with no local or
// remote commit leaves those fields out altogether.
func TestBaseCheckRouteOmitsWhatTheMachineDidNotSend(t *testing.T) {
	f := browseAPIFixture(t, baseCheckCaps())
	start := strings.Repeat("ab", 20)
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BaseCheckResult{State: terminalproto.BaseState{
			Base: "main", Relation: terminalproto.BaseNoLocal, Remote: start, Start: start}}
	})
	code, out, raw := f.get(t, f.cookie, baseCheckPath(f.agent, browseTestHome+"/site", "main"))
	if code != 200 {
		t.Fatalf("check = %d %s", code, raw)
	}
	if _, ok := out["local"]; ok || len(out) != 4 {
		t.Fatalf("empty fields were not left out: %s", raw)
	}
	if out["relation"] != "no_local" || out["remote"] != start || out["start"] != start {
		t.Fatalf("answer = %s", raw)
	}
}

// A branch neither the repository nor GitHub has is the same refusal an
// isolated launch gets, with the same sentence.
func TestBaseCheckRouteAnswersAMissingBranch(t *testing.T) {
	f := browseAPIFixture(t, baseCheckCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BaseCheckResult{Reason: terminalproto.BranchesBaseMissing}
	})
	code, out, raw := f.get(t, f.cookie, baseCheckPath(f.agent, browseTestHome+"/site", "gone"))
	if code != http.StatusUnprocessableEntity || out["code"] != "base_missing" {
		t.Fatalf("missing base = %d %v %s", code, out, raw)
	}
	if out["error"] != "That branch isn't in the repository on studio." {
		t.Fatalf("sentence = %v", out["error"])
	}
}

func TestBaseCheckRouteRefusesAViewer(t *testing.T) {
	f := browseAPIFixture(t, baseCheckCaps())
	viewer, err := f.st.CreateUser("base-check-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	path := baseCheckPath(f.agent, browseTestHome+"/site", "main")
	code, _, raw := f.get(t, terminalLogin(t, f.st, viewer.ID), path)
	if code != http.StatusForbidden {
		t.Fatalf("viewer = %d %s", code, raw)
	}
	if code, _, raw := f.get(t, "", path); code != 401 && code != 403 {
		t.Fatalf("signed out = %d %s", code, raw)
	}
	if sent := f.sentRequests(); len(sent) != 0 {
		t.Fatalf("a refused request reached the machine: %+v", sent)
	}
}

// A machine already running git over a repository is busy, not broken: the
// browser gets the same answer a branch listing or a prune gets.
func TestBaseCheckRouteSaysBusyWhenTheMachineIsBusy(t *testing.T) {
	f := browseAPIFixture(t, baseCheckCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BaseCheckResult{Reason: terminalproto.BranchesConflict}
	})
	code, out, raw := f.get(t, f.cookie, baseCheckPath(f.agent, browseTestHome+"/site", "main"))
	if code != http.StatusConflict || out["code"] != "branches_busy" {
		t.Fatalf("busy machine = %d %v %s", code, out, raw)
	}
	if out["error"] != sentenceBranchesBusy {
		t.Fatalf("sentence = %v, want %q", out["error"], sentenceBranchesBusy)
	}
}

// A git that could not answer says so about this check, never about listing
// branches, which is not what was asked for.
func TestBaseCheckRouteSaysGitCouldNotCheckTheBranch(t *testing.T) {
	f := browseAPIFixture(t, baseCheckCaps())
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.BaseCheckResult{Reason: terminalproto.BranchesGitFailed}
	})
	code, out, raw := f.get(t, f.cookie, baseCheckPath(f.agent, browseTestHome+"/site", "main"))
	if code != http.StatusUnprocessableEntity || out["code"] != "git_failed" {
		t.Fatalf("git failure = %d %v %s", code, out, raw)
	}
	if out["error"] != "Git couldn't check that branch there." {
		t.Fatalf("sentence = %v", out["error"])
	}
}
