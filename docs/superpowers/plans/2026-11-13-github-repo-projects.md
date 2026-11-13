# Projects from a GitHub Repository Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a project by naming a GitHub repository and a machine, have every task start from GitHub's current base, and let Remove delete the clone reet made.

**Architecture:** Three phases, each its own pull request and useful alone. Phase 1 hands the machine's `gh` sign-in to git and makes a worktree start from a freshly fetched base. Phase 2 adds one machine request that clones into `~/reet/repos/<owner>/<repo>` and saves an ordinary folder project that records its origin. Phase 3 adds a status and a remove request for that clone. Every new machine request copies the shape of `branch-sync-v1`: a `terminalproto` message pair, a `terminalhost` function, a `terminalbroker` method, a negotiated feature and a capability.

**Tech Stack:** Go (controller and machine connector, SQLite store), Svelte 5 and TypeScript (web), Playwright e2e in `web/tests`.

**Spec:** `docs/superpowers/specs/2026-11-13-github-repo-projects-design.md`

## Global Constraints

- The controller never holds, receives or forwards a GitHub credential.
- github.com only. No GitHub Enterprise, no other host.
- Full, normal clone: no `--bare`, `--mirror`, `--filter` or `--depth`.
- Every git call stays an argument vector through a `GitRunner`, with hooks off (`-c core.hooksPath=/dev/null`) and the hardened environment (`HOME`, `PATH`, `LANG` only, plus what `pinnedGitEnv` adds). Network calls also get `GIT_TERMINAL_PROMPT=0`.
- New wire fields and requests go only to a connection that negotiated the new feature; both sides decode strictly. A machine without the capability is never sent the request.
- The clone lives at `<home>/reet/repos/<owner>/<repo>`. reet never deletes a folder without its marker.
- Time bounds: fetch before a task 30 s inside the 60 s worktree create; clone 10 minutes.
- Copy follows `docs/voice-and-copy.md`. The sentences in this plan are exact.
- Each phase ends with `go vet ./...`, `go test -race ./...` and, where `web/` changed, `npm run check` in `web/` passing, docs and `CHANGELOG.md` updated, and a row in `docs/ui-verification.md` for any UI change.

## Review Focus

1. **`gh` is installed but signed out, or its token expired.** A task still starts, from the last known base, with the "couldn't reach GitHub" notice; nothing prompts or hangs. Pinned in Task 3.
2. **The local base has unpushed commits.** The task starts from local and says so; that work is never silently dropped. Pinned in Task 3.
3. **A project with no saved base branch, started by the Factory.** It starts from the remote's default branch, not from whatever `HEAD` the folder is on. Pinned in Task 4.
4. **Adding the same repository twice, or after a clone that was interrupted.** The second add adopts the folder or refuses; no half-made folder is ever left or adopted. Pinned in Task 8.
5. **Removing a project whose clone gained work between the dialog and the click.** The machine refuses and the folder is kept. Pinned in Task 12.

---

## Phase 1: fresh base for every task

### Task 1: Prove the `gh` sign-in reaches hardened git (gate)

This is the spec's main risk. Do it before anything else and stop if it fails.

**Files:**
- Create: `internal/terminalhost/gh_credentials.go`
- Test: `internal/terminalhost/gh_credentials_test.go`

**Interfaces:**
- Produces: `func GitHubRemote(url string) (owner, repo string, ok bool)`; `func ghCredentialArgs() []string`; `func networkGitEnv() []string` (returns `[]string{"GIT_TERMINAL_PROMPT=0"}`).

- [ ] **Step 1: Write the failing tests**

```go
func TestGitHubRemoteAcceptsOnlyGitHubDotCom(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/eyupio/reet.sh.git": "eyupio/reet.sh",
		"https://github.com/eyupio/reet.sh":     "eyupio/reet.sh",
		"git@github.com:eyupio/reet.sh.git":     "eyupio/reet.sh",
		"ssh://git@github.com/eyupio/reet.sh":   "eyupio/reet.sh",
	} {
		o, r, ok := GitHubRemote(in)
		if !ok || o+"/"+r != want { t.Fatal(in, o, r, ok) }
	}
	for _, in := range []string{"", "https://github.example.com/a/b", "https://gitlab.com/a/b", "https://github.com/a", "https://github.com/a/b/c", "https://evil@github.com.x/a/b", "https://github.com/-a/b", "https://github.com/a/.."} {
		if _, _, ok := GitHubRemote(in); ok { t.Fatal("accepted", in) }
	}
}

// With gh at an absolute path, the args reset inherited helpers and name gh for github.com only.
func TestGhCredentialArgsNameOnlyGitHub(t *testing.T) {
	dir := t.TempDir()
	gh := filepath.Join(dir, "gh")
	os.WriteFile(gh, []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", dir)
	want := []string{"-c", "credential.helper=", "-c", "credential.https://github.com.helper=!" + gh + " auth git-credential"}
	if got := ghCredentialArgs(); !slices.Equal(got, want) { t.Fatal(got) }
	t.Setenv("PATH", t.TempDir())
	if got := ghCredentialArgs(); got != nil { t.Fatal("args without gh", got) }
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./internal/terminalhost -run 'TestGitHubRemote|TestGhCredentialArgs'`
Expected: FAIL, undefined `GitHubRemote`.

- [ ] **Step 3: Implement the three functions in `gh_credentials.go`**

Owner matches `^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`; repo matches `^[A-Za-z0-9._-]{1,100}$`, is not `.` or `..`, and a trailing `.git` is stripped first. `ghCredentialArgs` uses `exec.LookPath("gh")` and returns nil unless the path is absolute.

- [ ] **Step 4: Run the tests and see them pass**

- [ ] **Step 5: Prove it on a real machine, by hand**

On a machine with `gh auth login` done and no other git credential set up, against a private repository you can read:

```bash
env -i HOME="$HOME" PATH="$PATH" LANG=C GIT_TERMINAL_PROMPT=0 git -c core.hooksPath=/dev/null -c credential.helper= -c "credential.https://github.com.helper=!$(command -v gh) auth git-credential" ls-remote https://github.com/OWNER/PRIVATE.git HEAD
```

Expected: one line with a commit id. Then run it again after `gh auth logout`: expected a non-zero exit within seconds and no prompt. Record both results in `docs/device-checks.md`. If the first fails, stop and report: the rest of the plan depends on it.

- [ ] **Step 6: Commit** (`Add the gh credential arguments for hardened git`)

### Task 2: The base rule

**Files:**
- Create: `internal/terminalproto/base_state.go`, `internal/terminalhost/base_start.go`
- Test: `internal/terminalhost/base_start_test.go`

**Interfaces:**
- Consumes: `GitHubRemote`, `ghCredentialArgs`, `networkGitEnv` (Task 1); `syncDefaultBase(ctx, dir, git) string` (existing, `branch_sync.go`).
- Produces, in `terminalproto`:

```go
type BaseRelation string
const (
	BaseSame        BaseRelation = "same"
	BaseBehind      BaseRelation = "behind"       // local is an ancestor of GitHub's
	BaseNoLocal     BaseRelation = "no_local"
	BaseAhead       BaseRelation = "ahead"        // local has commits GitHub lacks, and all of GitHub's
	BaseDiverged    BaseRelation = "diverged"
	BaseFetchFailed BaseRelation = "fetch_failed"
	BaseNoRemote    BaseRelation = "no_remote"    // no github.com origin; today's local lookup applies
)
// BaseState is what a worktree started from. Start is always set on success.
type BaseState struct {
	Base     string       `json:"base,omitempty"`
	Relation BaseRelation `json:"relation,omitempty"`
	Local    string       `json:"local,omitempty"`  // commit ids, 40 or 64 hex
	Remote   string       `json:"remote,omitempty"`
	Start    string       `json:"start,omitempty"`
}
func (s BaseState) Valid() bool
```

- Produces, in `terminalhost`: `func ResolveBase(ctx context.Context, repo, base string, fetch bool, git GitRunner) (terminalproto.BaseState, error)`. `repo` is the resolved repository root. An empty `base` becomes `syncDefaultBase`; if that is empty too the state is `{Relation: BaseNoRemote, Start: <HEAD commit>}`. With `fetch` false no network call is made and the last fetched `refs/remotes/origin/<base>` is used. Returns `refuseWorktree(terminalproto.WorktreeBaseMissing, "")` when neither side has the branch.

- [ ] **Step 1: Write the failing tests** against real repositories made in `t.TempDir()` with `ExecGit`, the "GitHub" side being a second local repository whose URL is rewritten with `-c url.<path>.insteadOf=https://github.com/o/r` in the test's runner. One test per row, each asserting `Relation` and that `Start` equals the named commit:

| Test | Setup | Relation | Start |
| --- | --- | --- | --- |
| `TestBaseSame` | local main == remote main | `same` | that commit |
| `TestBaseBehindStartsFromGitHub` | remote one commit ahead | `behind` | remote's |
| `TestBaseNoLocalStartsFromGitHub` | local branch deleted | `no_local` | remote's |
| `TestBaseAheadStartsFromLocal` | local one commit ahead | `ahead` | local's |
| `TestBaseDivergedStartsFromLocal` | one commit each side | `diverged` | local's |
| `TestBaseFetchFailedUsesLastKnown` | remote folder removed | `fetch_failed` | local's |
| `TestBaseFetchFailedWithoutLocalUsesRemoteTracking` | remote removed, local branch deleted | `fetch_failed` | `refs/remotes/origin/main` |
| `TestBaseNonGitHubOriginIsNotFetched` | origin `https://gitlab.com/a/b` | `no_remote` | local's; the runner saw no `fetch` |
| `TestBaseMissingEverywhere` | base `nope` | error is `WorktreeBaseMissing` | |
| `TestBaseFetchIsBoundedAt30Seconds` | runner blocks on `fetch` until its ctx ends | `fetch_failed`; the ctx deadline the runner saw is ≤ 30 s away | local's |
| `TestBaseEmptyUsesTheRemoteDefault` | base `""`, `origin/HEAD` → `trunk` | `Base == "trunk"` | |

- [ ] **Step 2: Run and see them fail** (`go test ./internal/terminalhost -run TestBase`)

- [ ] **Step 3: Implement `BaseState`, `Valid` and `ResolveBase`**

The fetch is exactly `git -c core.hooksPath=/dev/null <ghCredentialArgs...> fetch --no-tags --no-write-fetch-head origin +refs/heads/<base>:refs/remotes/origin/<base>`, with `networkGitEnv()` added to the call's environment and its own 30-second context. Ancestry uses `git merge-base --is-ancestor`. The environment addition needs `runGit` to accept extra variables: carry them in the context the way `pinGitDir` does (`func withGitEnv(ctx context.Context, env ...string) context.Context`, read in `pinnedGitEnv`).

- [ ] **Step 4: Run and see them pass**
- [ ] **Step 5: Commit** (`Choose a worktree's start from GitHub's base unless local is ahead`)

### Task 3: Use the rule when a worktree is made, and report it

**Files:**
- Modify: `internal/terminalhost/worktree.go` (the `start` lookup in `prepareWorktree`, about lines 385-410; `Worktree` struct), `internal/terminalhost/branch_sync.go` (its fetch), `internal/terminalhost/manager.go:255`, `internal/terminalagent/chat.go:469`, `internal/terminalproto/protocol.go` (`SessionChanged`, feature list), `internal/terminalproto/base_state.go`, `internal/agenthealth/agent.go` (features and capability, beside `BranchSyncFeature`), `internal/api/private_control.go:19`
- Test: `internal/terminalhost/worktree_test.go`, `internal/terminalhost/branch_sync_test.go`, `internal/terminalproto/protocol_test.go`

**Interfaces:**
- Consumes: `ResolveBase` (Task 2).
- Produces: `const BaseFreshFeature = "base-fresh-v1"`; `func SupportsBaseFresh(features string) bool`; `Capabilities.BaseFresh bool` (`json:"baseFresh,omitempty"`, true only with `Worktree` and the negotiated feature); `Worktree.Base terminalproto.BaseState`; `SessionChanged.Base *terminalproto.BaseState` (`json:"base,omitempty"`), set only in answer to a worktree create on a connection that negotiated the feature. `PrepareWorktree` gains no parameter: it always calls `ResolveBase(..., fetch=true, ...)`.

- [ ] **Step 1: Write the failing tests**
  - `TestPrepareWorktreeStartsFromGitHubWhenLocalIsBehind`: the new worktree's `HEAD` is the remote's commit and `w.Base.Relation == BaseBehind`.
  - `TestPrepareWorktreeKeepsUnpushedLocalWork` (Review Focus 2): local ahead; `HEAD` is local's; `Relation == BaseAhead`.
  - `TestPrepareWorktreeStartsWhenGhIsSignedOut` (Review Focus 1): the runner fails every `fetch` with exit 128; the worktree is still made, `Relation == BaseFetchFailed`, and the call returns in under 5 seconds.
  - `TestPrepareWorktreeAdoptedRetryDoesNotFetch`: a second call with the same run token makes no `fetch` call.
  - `TestSyncBranchFetchCarriesTheGhHelper`: the recorded `fetch` argv contains `credential.https://github.com.helper=` when the origin is github.com and `gh` is on `PATH`, and not otherwise.
  - `TestSessionChangedBaseIsOmittedWithoutTheFeature`: a `SessionChanged` with nil `Base` encodes without `"base"`; one with an invalid `BaseState` fails `valid()`.

- [ ] **Step 2: Run and see them fail**
- [ ] **Step 3: Implement.** Replace the `refs/heads/` then `refs/remotes/` loop with `ResolveBase`; keep the adopt path above it untouched. Add the helper args to `SyncBranch`'s fetch only for a github.com origin.
- [ ] **Step 4: Run `go test ./internal/terminalhost ./internal/terminalproto ./internal/agenthealth ./internal/terminalagent` and see them pass**
- [ ] **Step 5: Commit** (`Start each worktree from a freshly fetched base`)

### Task 4: The controller names the base, stores what happened and records a notice

**Files:**
- Modify: `internal/api/session_isolation.go` (`isolateWorktree`), `internal/store/store_terminal.go` (`TerminalSession`, `terminalSessionCols`, a migration adding `base_relation TEXT NOT NULL DEFAULT ''` and `base_start TEXT NOT NULL DEFAULT ''`), `internal/terminalbroker/broker.go` (where `WorktreePath` is taken from `session_changed`), `internal/api/terminal.go` (session JSON)
- Test: `internal/api/session_isolation_test.go`, `internal/store/store_terminal_test.go`

**Interfaces:**
- Consumes: `SessionChanged.Base`, `Capabilities.BaseFresh` (Task 3).
- Produces: `TerminalSession.BaseRelation, BaseStart string`; session JSON fields `baseRelation` and `baseStart` (omitted when empty); an Activity event `s.event(uid, source, "worktree-base", "", map[string]string{"machine", "host", "session", "base", "relation", "start"})` written once per session when the relation is `behind`, `ahead`, `diverged` or `fetch_failed`.

- [ ] **Step 1: Write the failing tests**
  - `TestIsolatedLaunchWithoutABaseUsesTheProjectsBase` (Review Focus 3): project `BaseBranch: "develop"`, a create with `Base: ""` through `isolateWorktree` leaves `c.Base == "develop"`. With a blank project base `c.Base` stays `""` (the machine then uses the remote default).
  - `TestIsolatedLaunchKeepsANamedBase`: `Base: "release"` is not overwritten.
  - `TestIsolatedRetryKeepsTheOriginalBase`: a retried request id reuses the base from the stored operation.
  - `TestGafferOutcomeLaunchCarriesTheProjectsBase`: a launch through `internal/api/gaffer_outcomes.go` for a project with base `develop` sends `Base: "develop"`.
  - `TestSessionRecordsItsBase`: a `session_changed` carrying `Base{Relation: ahead, Start: <oid>}` is stored, returned in the session JSON, and writes exactly one `worktree-base` event; a relation of `same` writes none.

- [ ] **Step 2: Run and see them fail**
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run `go test ./internal/api ./internal/store ./internal/terminalbroker` and see them pass**
- [ ] **Step 5: Commit** (`Name the project's base on isolated launches and record where a task started`)

### Task 5: A base check the browser can ask for

**Files:**
- Modify: `internal/terminalproto/base_state.go`, `internal/terminalproto/protocol.go` (decode cases `base_check`, `base_check_result`), `internal/agenthealth/agent.go`, `internal/terminalagent/agent.go` (dispatch, beside `*terminalproto.BranchSync`), `internal/api/terminal.go` (route)
- Create: `internal/terminalbroker/base_check.go`, `internal/api/terminal_base_check.go`
- Test: `internal/terminalhost/base_start_test.go`, `internal/api/terminal_base_check_test.go`

**Interfaces:**
- Consumes: `ResolveBase` (Task 2), `branchesRepo(home, dir)` (existing folder check used by `SyncBranch`).
- Produces:
  - `terminalproto.BaseCheck{Dir, Base string}` (kind `base_check`) and `BaseCheckResult{Reason BranchesReason; State BaseState}` (kind `base_check_result`), sent only when `caps.BaseFresh`.
  - `func CheckBase(ctx context.Context, home string, req terminalproto.BaseCheck, git GitRunner) terminalproto.BaseCheckResult` in `terminalhost/base_start.go`.
  - `func (b *Broker) CheckBase(ctx context.Context, uid int64, agent string, req terminalproto.BaseCheck) (terminalproto.BaseState, error)`, call timeout 40 s, `ErrBaseCheckUnsupported` without the capability.
  - `GET /api/v2/machines/{agentId}/base-check?dir=&base=` behind `s.terminal(...)`, answering `{"base","relation","local","remote","start"}`; 409 `base_check_unsupported` with "Update reet on that machine to check a branch against GitHub."; a blank `base` is filled from the project found by `RepoProjectForDir`.

- [ ] **Step 1: Write the failing tests**: `TestCheckBaseReportsWithoutMakingAWorktree` (state as in Task 2; `git worktree list` unchanged); `TestBaseCheckRouteRefusesAnOlderMachine` (409 and the sentence); `TestBaseCheckRouteFillsTheProjectsBase`; `TestBaseCheckRouteRefusesAViewer` (403 `read_only`).
- [ ] **Step 2: Run and see them fail**
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run and see them pass**
- [ ] **Step 5: Commit** (`Let the browser ask where a task would start`)

### Task 6: Notice and confirmation in the browser

**Files:**
- Modify: `web/src/lib/terminal/api.ts`, `web/src/lib/agentLaunch.ts`, `web/src/lib/home/LaunchDialog.svelte`, `web/src/lib/home/NewSessionDialog.svelte`
- Create: `web/src/lib/home/BaseConfirm.svelte`, `web/src/lib/baseNotice.ts`
- Test: `web/src/lib/baseNotice.test.mjs`, `web/tests/launch-base.e2e.mjs` (add it to the workflow that runs `web/tests/launch.e2e.mjs`)
- Docs: `docs/start-agent-session.md`, `docs/terminal-worktrees.md`, `docs/projects.md`, `docs/ui-verification.md`, `CHANGELOG.md`

**Interfaces:**
- Consumes: the base-check route (Task 5); session `baseRelation`, `baseStart` (Task 4).
- Produces: `export async function checkBase(agentId: string, dir: string, base?: string, signal?: AbortSignal): Promise<BaseState>` in `api.ts`; `export function baseSentence(s: BaseState, host: string): string | null` and `export function needsConfirm(s: BaseState, project: { cloned?: boolean }): boolean` in `baseNotice.ts`.

`baseSentence` returns exactly (short id is the first 7 characters of `start`):

| Relation | Sentence |
| --- | --- |
| `same`, `no_remote` | `null` |
| `behind`, `no_local` | `main on studio is behind GitHub. This session starts from GitHub's main (abc1234).` |
| `ahead`, `diverged` | `main on studio has commits GitHub doesn't have. This session starts from the local main (abc1234).` |
| `fetch_failed` | `Couldn't reach GitHub from studio, so main may be out of date. This session starts from the last known main (abc1234).` |

`needsConfirm` is true when the sentence is not `null` and the project is not `cloned`. (`cloned` does not exist until Phase 2, so in this phase every repository project asks.)

- [ ] **Step 1: Write the failing unit tests** for each row of the table and for `needsConfirm`, in `baseNotice.test.mjs`.
- [ ] **Step 2: Add `node src/lib/baseNotice.test.mjs` to the `test` script in `web/package.json`, run `node src/lib/baseNotice.test.mjs` in `web/` and see it fail**
- [ ] **Step 3: Implement `baseNotice.ts` and `checkBase`**
- [ ] **Step 4: Write the failing e2e** `launch-base.e2e.mjs`: with the fake machine answering `ahead`, **Start** shows a dialog titled `Check where this starts` with the sentence and buttons `Start session` and `Cancel`; Cancel makes no create request and returns focus to Start; `Start session` makes one. With `same` no dialog appears. With the route answering 409 the session starts without a dialog. The dialog traps focus and closes on Escape. Run at 390 px and 1280 px, light and dark.
- [ ] **Step 5: Implement `BaseConfirm.svelte`** on the existing dialog component the other Home dialogs use, and call `checkBase` in the one launch path in `agentLaunch.ts` before `createTerminal` when the launch will be isolated (a repository project, an agent, not a resume). A failed or unsupported check never blocks the start.
- [ ] **Step 6: Show the stored notice**: the session's Overview "Where it's working" card adds one line with `baseSentence` when `baseRelation` is set.
- [ ] **Step 7: Run `npm run check` and the e2e and see them pass**
- [ ] **Step 8: Update the docs and changelog; correct "resolved to a local branch commit, then a remote-tracking branch commit" in `terminal-worktrees.md` and the `PrepareWorktree` comment**
- [ ] **Step 9: Commit** (`Say and confirm where a session starts when local and GitHub differ`), open the Phase 1 pull request.

---

## Phase 2: add a project from GitHub

### Task 7: Name validation and the wire messages

**Files:**
- Create: `internal/terminalproto/repo_clone.go`
- Modify: `internal/terminalproto/protocol.go` (decode cases; `Capabilities.RepoClone bool` `json:"repoClone,omitempty"`)
- Test: `internal/terminalproto/repo_clone_test.go`

**Interfaces:**
- Produces:

```go
const RepoCloneFeature = "repo-clone-v1"
func SupportsRepoClone(features string) bool
// ParseGitHubRepo accepts "owner/repo" and "https://github.com/owner/repo", with an optional ".git" or trailing slash.
func ParseGitHubRepo(input string) (owner, repo string, ok bool)
type RepoClone struct{ Owner, Repo string }            // kind "repo_clone"
type RepoCloneReason string
const (
	RepoCloneNotSignedIn RepoCloneReason = "not_signed_in"
	RepoCloneNoAccess    RepoCloneReason = "no_access"   // missing, or not visible to this sign-in
	RepoCloneExists      RepoCloneReason = "exists"      // the folder is there and is something else
	RepoCloneTimedOut    RepoCloneReason = "timed_out"
	RepoCloneFailed      RepoCloneReason = "failed"
	RepoCloneDenied      RepoCloneReason = "denied"
)
type RepoCloneResult struct {                            // kind "repo_clone_result"
	Reason        RepoCloneReason `json:"reason,omitempty"`
	Dir           string          `json:"dir,omitempty"`
	DefaultBranch string          `json:"defaultBranch,omitempty"`
	Adopted       bool            `json:"adopted,omitempty"`
	Marked        bool            `json:"marked,omitempty"` // the reet marker is present
}
```

- [ ] **Step 1: Write the failing tests**: `TestParseGitHubRepo` (accepts `eyupio/reet.sh`, `https://github.com/eyupio/reet.sh`, `…/reet.sh.git`, `…/reet.sh/`, surrounding spaces; rejects `""`, `a`, `a/b/c`, `a/..`, `-a/b`, `a/b c`, `http://github.com/a/b`, `https://gitlab.com/a/b`, `a/b?x=1`, a 40-character owner, anything with a control character); `TestRepoCloneValid` (a `RepoClone` with an invalid name fails `valid()`; a result with `Dir` and a `Reason` together fails).
- [ ] **Step 2: Run and see them fail**
- [ ] **Step 3: Implement**, sharing the owner and repo patterns with `GitHubRemote` by moving them here and calling them from `terminalhost`.
- [ ] **Step 4: Run and see them pass**
- [ ] **Step 5: Commit** (`Add the repo-clone-v1 messages`)

### Task 8: Clone on the machine

**Files:**
- Create: `internal/terminalhost/repo_clone.go`
- Modify: `internal/agenthealth/agent.go` (feature, capability: `launch`, `worktree`, the negotiated feature, and `git` and `gh` both found at absolute paths), `internal/terminalagent/agent.go` (dispatch)
- Test: `internal/terminalhost/repo_clone_test.go`

**Interfaces:**
- Consumes: Task 7's types; `ghCredentialArgs`, `networkGitEnv`, `withGitEnv` (Tasks 1-2); `newExecGit(timeout)`.
- Produces:
  - `type GHRunner func(ctx context.Context, args ...string) ([]byte, error)` and `func ExecGH(ctx context.Context, args ...string) ([]byte, error)` (absolute `gh`, the hardened environment, 15 s, 4 KiB).
  - `func CloneRepo(ctx context.Context, home string, req terminalproto.RepoClone, git GitRunner, gh GHRunner) terminalproto.RepoCloneResult`
  - `const cloneMarker = "reet-clone"`: a file at `<clone>/.git/reet-clone` whose content is `owner/repo\n`. `func cloneMarked(dir string) (origin string, ok bool)`.

Behaviour, in order: `gh auth status --hostname github.com` must exit 0, else `not_signed_in`. The destination is `<home>/reet/repos/<owner>/<repo>`; every folder above it is made 0700 and must be a real directory owned by this user, as `ensureWorktreeDirs` does. If the destination exists: adopt only when `.git` is a real directory and `GitHubRemote(origin)` equals the request (case-insensitive), answering `Adopted: true` and `Marked` as found; otherwise `exists`. Otherwise clone into `<owner>/.<repo>.cloning-<16 random hex>` with `git -c core.hooksPath=/dev/null <ghCredentialArgs...> clone --no-tags https://github.com/<owner>/<repo>.git <temp>`, write the marker, then `os.Rename` to the destination. Any failure removes the temp folder and nothing else. A clone error whose output contains `Repository not found` or `could not read Username` is `no_access`; a context deadline is `timed_out`. The default branch is `syncDefaultBase`, falling back to the checked-out branch.

- [ ] **Step 1: Write the failing tests**, using a local bare repository reached through `insteadOf` as in Task 2 and a fake `GHRunner`:
  - `TestCloneRepoMakesAMarkedNormalClone`: `Dir` is `<home>/reet/repos/o/r`, `.git` is a directory, the marker reads `o/r`, `DefaultBranch == "main"`, no `--depth`, `--filter`, `--bare` or `--mirror` in the recorded argv, and hooks were disabled.
  - `TestCloneRepoRefusesWhenSignedOut`: `gh` exits 1 → `not_signed_in`, no folder made.
  - `TestCloneRepoAdoptsItsOwnEarlierClone` (Review Focus 4): a second call answers `Adopted` and `Marked`, and makes no `clone` call.
  - `TestCloneRepoAdoptsAPersonsCloneUnmarked`: a hand-made clone of the same origin → `Adopted`, `Marked == false`, no marker written.
  - `TestCloneRepoRefusesAnotherFolder`: destination is a plain folder, or a clone of another origin → `exists`, folder untouched.
  - `TestCloneRepoLeavesNothingAfterAFailure` (Review Focus 4): the runner fails `clone` after creating files → `<home>/reet/repos/o` holds no entry.
  - `TestCloneRepoIgnoresALeftoverTempFolder`: a `.r.cloning-…` folder from a killed run is not adopted and does not block a fresh clone.
  - `TestCloneRepoRefusesASymlinkedParent`: `<home>/reet` is a symlink → `failed`.
  - `TestCloneRepoTimesOut`: the runner blocks until ctx ends → `timed_out`, nothing left.
- [ ] **Step 2: Run and see them fail**
- [ ] **Step 3: Implement.** The dispatch in `terminalagent` calls it with `newExecGit(10*time.Minute)` and `ExecGH`.
- [ ] **Step 4: Run `go test ./internal/terminalhost ./internal/agenthealth ./internal/terminalagent` and see them pass**
- [ ] **Step 5: Commit** (`Clone a GitHub repository on the machine`)

### Task 9: Broker, store and the add route

**Files:**
- Create: `internal/terminalbroker/repo_clone.go`, `internal/api/folder_projects_github.go`
- Modify: `internal/api/private_control.go:19`, `internal/store/store_folder_projects.go` (`FolderProject.Origin string`, `Cloned bool`; migration adding `origin TEXT NOT NULL DEFAULT ''` and `cloned INTEGER NOT NULL DEFAULT 0`, in the style of the `colour` column at about line 150), `internal/api/folder_projects.go` (`folderProjectView` gains `origin`, `cloned`; routes)
- Test: `internal/api/folder_projects_github_test.go`, `internal/store/store_folder_projects_test.go`

**Interfaces:**
- Consumes: Tasks 7-8.
- Produces:
  - `func (b *Broker) CloneRepo(ctx context.Context, uid int64, agent string, req terminalproto.RepoClone) (terminalproto.RepoCloneResult, error)`; `var RepoCloneCallTimeout = 10*time.Minute + 30*time.Second`; `ErrRepoCloneUnsupported`.
  - `POST /api/v2/folder-projects/from-github` with body `{"host": "<agentId>", "repo": "owner/repo or link", "name": ""}`, behind `folderAccess(folderProjectWrite(...))`. It validates, checks the limits a normal create checks, then starts the clone in a goroutine that outlives the request and answers `202 {"cloneId": "<32 hex>", "dir": "~/reet/repos/owner/repo"}`.
  - `GET /api/v2/folder-projects/clones/{id}` answering `{"state": "cloning"}`, `{"state": "done", "project": <folderProjectView>}` or `{"state": "failed", "code": "...", "sentence": "..."}`. Clone jobs live in memory per account, at most 5 running per account (429 `clone_limit`, "Five repositories are already being cloned. Wait for one to finish."), kept 10 minutes after they end.
  - On success the saved project has `IsRepo: true`, `Dir`, `BaseBranch: DefaultBranch`, `Origin: "owner/repo"`, `Cloned: result.Marked`, and a name from the `name` field or the repository name (the existing ` (2)` rule on a clash). The audit row is `folder-project-add` with `origin` and `cloned`.

Refusal sentences, exact (`studio` is the machine's name):

| Code | Status | Sentence |
| --- | --- | --- |
| `invalid_repo` | 422 | Write the repository as owner/name, or paste its github.com link. |
| `repo_clone_unsupported` | 409 | Update reet on studio to add projects from GitHub. It also needs git and the GitHub CLI. |
| `machine_offline` | 409 | studio is offline. Try again when it's back. |
| `not_signed_in` | (job) | studio isn't signed in to GitHub. Run gh auth login there, then try again. |
| `no_access` | (job) | GitHub has no owner/name that studio's sign-in can read. |
| `exists` | (job) | ~/reet/repos/owner/name on studio already holds something else. Move it, or add that folder as a project. |
| `timed_out` | (job) | Cloning owner/name took longer than 10 minutes and was stopped. Nothing was kept. |
| `failed` | (job) | Couldn't clone owner/name on studio. |
| `project_exists` | (job) | studio already has a project for that folder. |

- [ ] **Step 1: Write the failing tests**: `TestAddFromGitHubClonesThenSavesARepoProject` (poll to `done`; the project has the fields above; one audit row); `TestAddFromGitHubSurvivesTheRequestEnding` (cancel the request context; the job still reaches `done`); `TestAddFromGitHubReportsEachRefusal` (table over the sentences); `TestAddFromGitHubNeverSendsToAnOlderMachine` (nothing sent, 409); `TestAddFromGitHubOfAnAdoptedUnmarkedFolderIsNotCloned` (`cloned == false`); `TestAddFromGitHubCountsTowardsTheProjectLimit` (the 101st is refused before any clone); `TestCloneStatusIsPerAccount` (another account gets 404); `TestFolderProjectOriginMigratesWithDefaults`.
- [ ] **Step 2: Run and see them fail**
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run `go test ./internal/api ./internal/store ./internal/terminalbroker` and see them pass**
- [ ] **Step 5: Commit** (`Add a project from a GitHub repository`)

### Task 10: The Add dialog

**Files:**
- Modify: `web/src/lib/home/AddProjectDialog.svelte`, `web/src/lib/terminal/api.ts` (or the file the existing folder-project calls live in), `web/src/lib/baseNotice.ts` callers (pass `cloned`)
- Create: `web/src/lib/home/AddFromGitHub.svelte`, `web/src/lib/githubRepo.ts`
- Test: `web/src/lib/githubRepo.test.mjs`, `web/tests/add-project-github.e2e.mjs` (beside `add-project.e2e.mjs` in its workflow)
- Docs: `docs/projects.md`, `docs/quick-start.md`, `README.md`, `docs/ui-verification.md`, `CHANGELOG.md`

**Interfaces:**
- Consumes: Task 9's routes; machine capability `repoClone` from `/api/v2/terminal/machines`.
- Produces: `export function parseGitHubRepo(input: string): { owner: string; repo: string } | null` (same rules as the Go one); `addFromGitHub(host, repo, name, signal)` and `cloneStatus(id, signal)`.

The dialog gains two choices at its top, `Folder on a machine` (today's content, the default) and `From GitHub`. From GitHub shows: a field labelled `Repository` with the hint `owner/name, or a github.com link`; a machine picker listing only machines with `repoClone`, and, when there are none, the line `No machine here can clone from GitHub yet. It needs a current reet, git and the GitHub CLI.`; an optional `Name`; and, once the repository parses, the line `reet will clone it to ~/reet/repos/owner/name on studio.` The button is `Clone and add`. While cloning: `Cloning owner/name on studio… You can close this; the project appears when it's done.` with a `Close` button. On `done` the dialog closes and the project is selected in the rail. On `failed` the sentence shows in the dialog's error slot and the form is usable again.

- [ ] **Step 1: Write the failing unit tests** for `parseGitHubRepo` (the Task 7 cases).
- [ ] **Step 2: Add `node src/lib/githubRepo.test.mjs` to the `test` script in `web/package.json`; run it and see it fail; implement; see it pass**
- [ ] **Step 3: Write the failing e2e**: the happy path ends with the project in the rail showing the repository diamond and base `main`; closing mid-clone and reopening Home later still shows the project; each refusal sentence is shown; the button is disabled until the repository parses and a machine is chosen; keyboard only, focus returns to Add; 390 px and 1280 px, light and dark; a project with `cloned: true` starts a session with relation `behind` without the Task 6 confirmation.
- [ ] **Step 4: Implement**, polling `cloneStatus` every 2 seconds while the dialog or Home is open.
- [ ] **Step 5: Run `npm run check` and the e2e and see them pass; update the docs**
- [ ] **Step 6: Commit** (`Add From GitHub to the Add project dialog`), open the Phase 2 pull request.

---

## Phase 3: remove the clone with the project

### Task 11: Clone status and remove on the machine

**Files:**
- Modify: `internal/terminalproto/repo_clone.go`, `internal/terminalproto/protocol.go`, `internal/terminalhost/repo_clone.go`, `internal/terminalagent/agent.go`, `internal/agenthealth/agent.go`
- Create: `internal/terminalbroker/repo_clone_remove.go`
- Test: `internal/terminalhost/repo_clone_test.go`

**Interfaces:**
- Consumes: `cloneMarked`, `GitHubRemote`, `worktreeRefTips` and `changeFingerprint` (existing, `worktree.go`).
- Produces: `const RepoCloneRemoveFeature = "repo-clone-remove-v1"`, `func SupportsRepoCloneRemove(features string) bool` and `Capabilities.RepoCloneRemove bool` (`json:"repoCloneRemove,omitempty"`). A machine that shipped Phase 2 alone decodes strictly and would refuse these kinds, so they need their own token; they are sent only with `caps.RepoCloneRemove`. And:

```go
type RepoCloneStatus struct{ Dir, Origin string }      // kind "repo_clone_status"; Origin is "owner/repo"
type RepoCloneState struct {                           // kind "repo_clone_state"
	Reason      RepoCloneReason `json:"reason,omitempty"`
	Marked      bool   `json:"marked,omitempty"`
	Worktrees   int    `json:"worktrees,omitempty"`    // linked worktrees, excluding the clone itself
	Changed     int    `json:"changed,omitempty"`      // uncommitted and untracked entries
	Stashes     int    `json:"stashes,omitempty"`
	Unpushed    int    `json:"unpushed,omitempty"`     // local branches with commits on no remote-tracking branch
	Fingerprint string `json:"fingerprint,omitempty"`
}
func (s RepoCloneState) Removable() bool  // Marked && all four counts zero && Reason == ""
type RepoCloneRemove struct{ Dir, Origin, Fingerprint string } // kind "repo_clone_remove"
type RepoCloneRemoved struct {                         // kind "repo_clone_removed"
	Reason RepoCloneReason `json:"reason,omitempty"`   // adds "changed" and "not_removable"
	Dir    string          `json:"dir,omitempty"`
}
func CloneStatus(ctx context.Context, home string, req terminalproto.RepoCloneStatus, git GitRunner) terminalproto.RepoCloneState
func RemoveClone(ctx context.Context, home string, req terminalproto.RepoCloneRemove, git GitRunner) terminalproto.RepoCloneRemoved
func (b *Broker) CloneStatus(...) (terminalproto.RepoCloneState, error)
func (b *Broker) RemoveClone(...) (terminalproto.RepoCloneRemoved, error)
```

`Dir` must resolve to exactly `<home>/reet/repos/<owner>/<repo>` for the request's `Origin`; anything else is `denied`. `RemoveClone` recomputes the state, refuses with `changed` when the fingerprint differs and `not_removable` when `Removable()` is false, then removes the folder and, if it is then empty, `<home>/reet/repos/<owner>`. It never removes `<home>/reet/repos` or anything above it.

- [ ] **Step 1: Write the failing tests**: `TestCloneStatusOfAFreshCloneIsRemovable`; one test each showing a linked worktree, an untracked file, a stash, and a local branch with an unpushed commit make it not removable with the right count; `TestCloneStatusCountsAPushedReetBranchAsSafe`; `TestRemoveCloneRefusesAnUnmarkedFolder`; `TestRemoveCloneRefusesAnotherOrigin`; `TestRemoveCloneRefusesAPathOutsideReetRepos` (`Dir` of `<home>/work/r`, and of a symlink into `reet/repos`); `TestRemoveCloneRefusesWhenChangedSinceShown` (Review Focus 5: a file added after `CloneStatus` → `changed`, folder intact); `TestRemoveCloneDeletesTheFolderAndAnEmptyOwner`; `TestRemoveCloneKeepsAnOwnerWithAnotherRepo`.
- [ ] **Step 2: Run and see them fail**
- [ ] **Step 3: Implement**
- [ ] **Step 4: Run and see them pass**
- [ ] **Step 5: Commit** (`Report and remove a clone reet made, only when nothing would be lost`)

### Task 12: Remove from reet offers to delete the clone

**Files:**
- Modify: `internal/api/folder_projects.go` (`folderProjectDelete`, a new `GET /api/v2/folder-projects/{id}/clone`), `web/src/lib/home/ProjectRail.svelte` or the Remove dialog it opens, `web/src/lib/terminal/api.ts`
- Test: `internal/api/folder_projects_github_test.go`, `web/tests/add-project-github.e2e.mjs`
- Docs: `docs/projects.md`, `docs/machine-maintenance.md`, `docs/ui-verification.md`, `CHANGELOG.md`

**Interfaces:**
- Consumes: Task 11's broker methods; `FolderProject.Cloned`, `Origin`.
- Produces: `GET /api/v2/folder-projects/{id}/clone` → `{"removable": bool, "worktrees", "changed", "stashes", "unpushed", "fingerprint", "sentence"}` (404 for a project that is not `cloned`); `DELETE /api/v2/folder-projects/{id}` accepts an optional body `{"deleteClone": true, "fingerprint": "..."}`. With it, the clone is removed first; only if that succeeds is the project deleted. Without it, today's behaviour. The audit row is `folder-project-remove` with `cloneDeleted: true` and the path.

The Remove dialog of a `cloned` project adds a tick, off by default: `Also delete ~/reet/repos/owner/name from studio`. When not removable the tick is absent and one of these shows, first match:

| Condition | Sentence |
| --- | --- |
| machine offline or older | The clone on studio is kept: studio can't be asked right now. |
| worktrees > 0 | The clone on studio is kept: 2 sessions still have worktrees from it. Remove those sessions first. |
| changed, stashes or unpushed > 0 | The clone on studio is kept: it holds work that isn't on GitHub. |
| not marked | The clone on studio is kept: reet didn't make that folder. |

On a refused delete (`changed`): `The clone changed just now, so nothing was removed. Check again.` and both the project and the folder stay.

- [ ] **Step 1: Write the failing API tests**: `TestRemoveProjectDeletesItsCloneWhenAsked`; `TestRemoveProjectKeepsTheProjectWhenTheCloneChanged` (Review Focus 5); `TestRemoveProjectWithoutTheFlagNeverTouchesTheFolder` (no `repo_clone_remove` sent); `TestRemoveAnOrdinaryProjectIgnoresDeleteClone` (a non-`cloned` project with the flag: 422 `not_cloned`, nothing sent); `TestCloneRouteGivesEachSentence`.
- [ ] **Step 2: Run and see them fail; implement; see them pass**
- [ ] **Step 3: Extend the e2e**: the tick appears only when removable, is off by default, each sentence shows for its condition, a refused delete keeps the project in the rail; 390 px and 1280 px, light and dark.
- [ ] **Step 4: Implement the dialog; run `npm run check` and the e2e**
- [ ] **Step 5: Update the docs; commit** (`Offer to delete the clone when a project from GitHub is removed`), open the Phase 3 pull request.

---

## What this plan leaves out, on purpose

- Automatic machine choice, moving a project, setup commands, secrets, hosts other than github.com, and repository search: the spec's non-goals.
- Removing a worktree when a pull request merges: the owner chose session removal and the scheduled clean-up.
