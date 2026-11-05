# A session Overview that answers "what did I ask, how far has it got, what do I do next"

Status: proposed, awaiting review. Date: 2026-11-05.

## Problem

The Overview tab of the session workspace panel (`ContextPanel.svelte`, lines 151-171) reports what the agent is doing
this second: a state pill, one headline, tool counts, the latest agent update, a changed-files card and a small
"Where it's working" list. It does not tell a person who comes back to a session:

1. **What they asked.** The controller deliberately keeps no opening prompt (`store_terminal.go`: "The opening prompt is
   deliberately absent: it is kept only with the create request"). A Gaffer chat's first message is only there while
   the early history is loaded, and a terminal session has nothing at all.
2. **How far the work has got.** Pull request facts (checks, review decision, conflicts, merged or closed) are read by
   the machine and shown in full, but only on the GitHub tab, and only once it has been opened (the navigation's
   background read keeps just the PR number). Nothing says "coded, PR open, two checks failing, three open review
   threads".
3. **How long it has run.** "Started 14m ago" is shown, with no clock time, no running length and no final length.
4. **What to do next.** The one thing a person can do when something is red (open the failing check, ask the agent to
   fix it) is two tabs away, and review comments are not read at all.

## Goal

Opening a session answers three questions without scrolling or hunting: **what I asked, how far it has got, what I
should do next.** In a 300 to 520 px panel, in both themes, on a phone sheet, from the keyboard, with the same promise
the Overview already makes: nothing below a block moves when the agent moves on.

Success looks like this:

- The opening prompt is one tap away in a modal, for terminals and chats, on any device.
- Start time, running or final length, repo, branch, worktree and machine are visible and copyable or linkable.
- A five-step tracker (Code, PR, Checks, Review, Merge) shows where the work is, from facts only, and one **Next** line
  names the single most useful thing to do, with its button.
- A merged PR collapses the whole thing to "Merged 12m ago" with a link.
- Nothing is invented: a stage with no evidence says "not read" and never reads as healthy.

## Decisions taken (owner, 2026-11-05)

| Question | Decision |
| --- | --- |
| Where does the prompt come from? | Stored on the controller, reversing the current "never kept" rule. |
| How deep are the PR stages? | The machine's GitHub read is extended: review threads, merge and last-push times, auto-merge. |
| What is "babysit"? | The tracker shows what the PR needs, from facts. No inference from tool names, no agent-declared state. |
| Which actions? | Links, copy buttons, and "Ask the agent to fix..." shortcuts that **prefill** the composer. Nothing is sent for the person. |

## Scope

- The Overview tab, its components and pure derivation modules; the GitHub tab keeps its detail view and gains a status
  dot.
- Controller: store and serve the opening prompt (capped, owner-only).
- Machine: extend the read-only GitHub read behind `view=github`.
- Out of scope: token or cost figures, a shareable session summary, an agent-declared "watching" state (see Deferred),
  GitHub Enterprise hosts (the read is github.com only today), and anything that edits, stages, commits, pushes or
  merges. Inspection stays strictly read-only.

## Design

### 1. Layout

Top to bottom. Every block that can change content has a fixed-height slot, so the existing contract
("`docs/terminal-workspace.md`: the Overview never moves its sections when the agent moves on") still holds.

1. **Status header** (existing). State pill, headline, notice slot (input needed, chat error, since-last-visit, calm).
   New: a **run clock** line, `Started 14:02 · running 1h 12m`, which becomes `Ran 1h 12m` when the session has
   finished. A `<time>` element, not announced by screen readers.
2. **You asked.** A two-line quote of the opening prompt, with **Read full prompt** opening the prompt dialog (3).
3. **Stage tracker and Next line** (4).
4. **Pull request card** (5): a summary of what the GitHub tab shows, with a link into it.
5. **Live activity** (existing): In progress, failures, latest agent update.
6. **Workspace changes** (existing).
7. **Where it's working**: repo (linked), branch (copy), worktree (copy), machine, project, agent, model and effort,
   start time.
8. **Skills and decisions** (existing, collapsed).

The GitHub tab shows a small status dot (bad, warn or good, taken from the same verdict as the PR card) so trouble is
visible from any tab. The dot is never colour alone: the tab's accessible name gains ", needs attention" or similar.

### 2. Components and pure modules

`ContextPanel.svelte` is one dense file already; the Overview moves out of it. New files in `web/src/lib/sessions/`:

| File | Purpose | Depends on |
| --- | --- | --- |
| `OverviewTab.svelte` | Composes the blocks above; owns nothing but layout | the components below |
| `PromptCard.svelte`, `PromptDialog.svelte` | The quote and the modal | `dialogShell.ts`, `promptSource.ts` |
| `StageTracker.svelte` | Stepper and Next line | `overviewStages.ts` |
| `PullCard.svelte` | Compact PR summary and buttons | `pullVerdict.ts` |
| `WhereCard.svelte` | Repo, branch, worktree, machine, agent facts, copy buttons | `runClock.ts` |
| `overviewStages.ts` | **Pure.** Facts in, five steps and one Next out | `sessionInsights.ts` types |
| `pullVerdict.ts` | **Pure.** The verdict logic now inline in `GitHubPane.svelte`, extracted so the card, the tab dot and the pane agree | |
| `runClock.ts` | **Pure.** Start label and run length | |
| `fixMessages.ts` | **Pure.** The prefilled message for conflicts, failing checks and open threads | |
| `promptSource.ts` | **Pure.** Chooses stored excerpt, loaded chat message or "not kept" | `chat.ts` types |

Pure modules import no Svelte, as `contextLayout.ts` does, so `node:test` runs them. `GitHubPane.svelte` is changed only to
import `pullVerdict.ts` in place of its inline copy (a targeted improvement that serves this work, not a refactor).
`ContextPanel.svelte` keeps the header, tabs, footer, polling and the non-Overview tabs.

### 3. The prompt

**Controller.**

- `terminal_sessions` gains `prompt TEXT NOT NULL DEFAULT ''`, added with the same column-ensure list that already
  carries `worktree_path`, `tool` and the rest (`store_terminal.go` around line 49). It is written in the same
  `INSERT` that reserves the session (line 585), so a retried create (same request id) cannot change it. The existing
  cap, `terminalproto.MaxPromptBytes` (8000), already bounds it; nothing new is accepted.
- The session list does **not** carry the full text. `terminalSessionCols` gains `substr(prompt,1,300)`, serialised as
  `promptExcerpt` (omitted when empty) with `promptMore: true` when the stored text is longer.
- New `GET /api/v2/terminal/sessions/{id}/prompt` beside `.../inspect`, through the same `s.terminal(...)` wrapper, so
  the same account and ownership checks apply. It answers `{ "prompt": "..." }`, `Cache-Control: no-store`; a session
  that is not the caller's is a 404 exactly like any other session route; an empty string means "not kept".
- The prompt is never logged and never written into an audit detail. The controller never deletes a session row
  (archiving hides it and keeps it), so the prompt is **cleared when the session is archived**, on both archive paths
  (`ArchiveTerminalFor`, and `archiveTerminalOnClose` for a session ended with history removal). This is the lifecycle a
  chat's stored transcript already has: `DeleteTerminalChatEventsFor` drops it "when it is archived or its live history
  is closed". An archived session therefore shows "not kept" for its prompt, like any session that never had one.
- The comment in `store_terminal.go`, and any doc or README sentence that says prompts are not retained, change in the
  same pull request. The implementation plan starts by searching for those sentences.

**Browser.**

- `TerminalSession` gains `promptExcerpt?`, `promptMore?` and the already-serialised `completedAt?`.
- `promptSource.ts` picks, in order: the stored excerpt (full text fetched when the dialog opens); else the first `user`
  item in the loaded chat when `chat.truncated` is false; else the honest line "This session started before reet kept
  opening prompts." (terminals) or "Earlier messages are not loaded." (truncated chats).
- The card is a quote with a left rule, two lines clamped, in a fixed-height slot that also holds the loading and "not
  kept" states. The dialog is a native `<dialog>` with `use:shell`, so Escape, focus return and phone behaviour match
  the other dialogs. It shows the full text (`white-space: pre-wrap`, scrolls inside its body), a **Copy prompt**
  button with a "Copied" confirmation, and chips for agent, model, effort, start time and, where set, the auto-approve
  decision the session records today.

### 4. Stage tracker (`overviewStages.ts`)

Five fixed steps so the row never changes width: **Code, PR, Checks, Review, Merge**. Each step is one of `done`,
`current`, `attention`, `upcoming` or `unknown`, rendered as an ordered list with an icon *and* a word (colour is never
the only signal) and `aria-current="step"` on the current step.

| Step | done | current | attention | upcoming | unknown |
| --- | --- | --- | --- | --- | --- |
| Code | a PR exists | changed files or tool activity, session live | session finished with changes and no PR (Next explains) | nothing yet | n/a |
| PR | PR open (label `#42`, `draft` noted) | draft PR | n/a | `no_pr` | `github_unavailable`, with the reason |
| Checks | all reported checks passed or skipped | any check running | any failing | no PR | **no checks reported (never `done`)** |
| Review | approved and no open threads | review required or awaiting | changes requested, or any open thread | draft, or no PR | threads not read on this machine |
| Merge | merged | auto-merge on, or ready | conflicts | not yet | closed without merge |

Plan is not a step. When the session reports a plan-like agent mode (`agentMode` contains "plan"), Code reads
**Planning** and is `current`. Once the mode changes it reads Code again. Nothing says a plan *was* made unless the mode
says so now, because history is not observable.

The **Next line** is one sentence and at most one primary button, chosen by the first rule that matches:
input needed (`Review` opens the session) > conflicts > failing checks (`Open failing check`) > changes requested >
open review threads > running checks ("Waiting on X and Y") > draft > review required > merged > nothing. A secondary
**Ask the agent to fix...** button appears for conflicts, failing checks and open threads (section 6). "Babysit" lives
here: the line says what the PR needs, from facts, not what the agent is doing. The tracker never claims a PR can merge;
it says "Ready. GitHub decides." and keeps the existing note about required checks and branch rules.

When the PR is merged, the tracker and Next line collapse to one row: "Merged 12m ago" with **Open pull request**.

### 5. Pull request card and the extended GitHub read

**Card (`PullCard.svelte`).** `#42 · open`, title, verdict (the shared `pullVerdict.ts` text), the checks meter and
failing, running, passed chips, a reviews line ("1 approved · changes requested by 1"), open threads, conflicts, `+/-`
totals, last push ("pushed 4m ago"), and buttons **Open pull request**, **Open failing check**, **Open review** and
**Full detail** (switches to the GitHub tab). A fixed-height slot while loading and when there is no PR (a 69 px row,
the height of the changes card), so presence of a PR changes the slot once, not on every refresh.

**Reading.** The Overview reads `view=github` on the GitHub tab's cadence (two minutes while Live updates is on),
sharing the navigation's read through `insightStore.ts` rather than starting its own, so a visible session costs
one read per cadence, as today.

**Machine (`internal/terminalhost/inspect.go`, `internal/terminalproto/inspect.go`).**

- `gh pr view --json` adds `createdAt,mergedAt,closedAt,baseRefName,headRefName,autoMergeRequest,latestReviews,
  reviewRequests`. `commits` is **not** requested: a long PR would overrun the 128 KiB output cap and break parsing.
- A second bounded call, `gh api graphql`, reads `reviewThreads(first:100){totalCount nodes{isResolved isOutdated}}`
  and the last commit's `committedDate`, for **open pull requests only**. It has the same safeguards as the first:
  absolute `gh` path, `GH_PROMPT_DISABLED`, `GH_HOST=github.com`, the process-group kill, a size-capped buffer and the
  request's deadline. If it fails, the first call's result is still returned and threads are "not read".
- `InspectPull` gains optional fields: `base`, `head`, `createdAt`, `mergedAt`, `closedAt`, `lastPushAt`, `autoMerge`,
  `reviews{approved,changesRequested,commented,requested}`, `threads{open,resolved,more}` and `detailKnown`.
  `InspectResult` gains `remote` ("owner/name" for a github.com origin, set by the status view too, so the repo link
  works before any PR exists). `valid()` and `fitInspect` bound every new field.
- `InspectFeature` stays `session-inspect-v1`: every field is additive and optional. `detailKnown` plays the part
  `countsKnown` plays today: absent means "this machine has not updated", so the UI says that instead of showing zero.

### 6. Actions

| Action | Where | Notes |
| --- | --- | --- |
| Open pull request, failing check, review, repo | Pull card, Next line, Where card | `https:` only, through `safeInsightURL` |
| Copy prompt, branch, worktree path, PR link | Prompt dialog, Where card, Pull card | Per-button "Copied" confirmation |
| **Ask the agent to fix...** | Next line | Conflicts, failing checks, open threads |

"Ask the agent to fix..." builds a plain message in `fixMessages.ts`, for example *"These checks are failing on the pull
request: "build", "lint" (and 2 more). Please look into why and fix them."*. In a Gaffer chat it **prefills the
composer** (appended on a new line, never replacing what the person typed) and focuses it; they press send. In a
terminal session there is no composer, so the same button reads **Copy message for the agent**. Check names come from
GitHub and are untrusted: they are quoted, stripped of control characters, capped at 60 characters, limited to three
names and shown to the person before anything is sent. No URL from GitHub enters the message. The seam into
`ChatPane.svelte` is one prop and one effect (`SessionScreen` forwards `{ sessionId, text, nonce }`), kept small
because that file is already large.

### 7. Run clock (`runClock.ts`)

Start label: `14:02` when today, `4 Nov, 14:02` otherwise, with the full date and time in the `title`. Length: while the
session is `creating`, `running` or `end_requested`, `now - createdAt`; when `completedAt` is set, `completedAt -
createdAt`; a finished session without `completedAt` shows the start only and no length (never a guess). Formats:
`under a minute`, `7m`, `1h 12m`, `2d 3h`. The panel adds no timer: it follows the `now` that `SessionsPage` already
ticks.

### 8. Error handling and degraded modes

| Situation | What the Overview shows |
| --- | --- |
| Machine offline or paused | Last known tracker, marked "Last known", as the footer does today |
| Machine without `session-inspect-v1` | Prompt, clock and Where card still work; tracker and PR card say "Update reet on this machine" |
| Machine without the extended read (`detailKnown` absent) | PR, checks, review decision and conflicts as today; threads, merged time and last push "not read on this machine" |
| `gh` missing or not signed in (`github_unavailable`) | PR, Checks, Review, Merge are `unknown` with the reason sentence |
| GraphQL call fails | Everything from the first call stays; only threads are "not read" |
| Controller without `promptExcerpt` | "not kept" line, no error |
| Prompt fetch fails when the dialog opens | The excerpt is shown with "Couldn't load the full prompt. Try again." and a retry |

Copy follows `docs/voice-and-copy.md`: familiar state labels, no dialect in errors, and no cheerful reassurance around a
failure.

### 9. Quality bar

- Tokens only (`--good`, `--warn`, `--bad`, `--accent`, `--tint`, `--line`), correct in both themes, no new library.
- 44 px targets on `pointer: coarse`, a visible focus ring on every control, full keyboard use of the tabs, tracker
  buttons, dialog and copy buttons, and `prefers-reduced-motion` honoured (the one animation, a soft pulse on the
  current step, is off under it).
- Checked at 300, 340 and 520 px and in the phone sheet. Below 340 px the step labels shorten (the tracker keeps its
  icons and words).
- The panel's existing rule holds: long text is cut with an ellipsis or scrolls inside its slot.

## Delivery

One spec, three pull requests, in this order. Each is shippable alone, and the UI degrades if an earlier one is not yet
deployed.

1. **Controller: keep and serve the opening prompt.** Column, excerpt, endpoint, deletion, docs and the removed claim.
2. **Machine: extend the GitHub read.** `gh` fields, the GraphQL call, wire types, `valid()`, docs.
3. **UI: the Overview.** Components, modules, the GitHub tab dot, e2e, docs, `handoff.md`.

Each pull request adds its `handoff.md` entry (`OVERVIEW-1`, `OVERVIEW-2`, `OVERVIEW-3`) and updates
`docs/terminal-workspace.md` and `docs/ui-verification.md` where behaviour changes.

## Testing

- **`node:test`** beside the pure modules, as `agentLaunch.test.mjs` sits beside `agentLaunch.ts`: `overviewStages`
  (a table of facts to steps and Next, including "no checks is never done", "threads unknown is not zero", merged
  collapse and plan-mode relabel), `runClock` (formats, `completedAt`, finished without it), `fixMessages` (caps,
  control characters, quotes, no URLs) and `promptSource` (order of fallbacks).
- **Go.** Migration adds the column to an old database; the cap holds; `promptExcerpt` and `promptMore` in the list;
  the endpoint answers the owner, 404s another account's session and never logs the text; archiving by either path
  clears it, and the audit rows for a launch contain no prompt text.
  `inspect_test.go` fixtures for the new `gh` JSON, a GraphQL failure, a truncated oversize output, and `valid()`
  refusing out-of-range counts.
- **e2e** (`session-workspace.e2e.mjs` and its synthetic machine): prompt dialog opens, copies, closes with Escape and
  returns focus; tracker states for conflict, failing, running, approved and merged fixtures; the prefill lands in the
  composer without overwriting a draft; the headline and notice change without moving later blocks (the existing
  layout-stability check, extended to the new slots); an older-machine fixture shows the "update" copy; phone sheet at
  340 px and a 300 px panel.
- Each new behaviour is shown failing with its rule removed before the pull request is opened, and the pull request says
  which.

## Open risks

1. **Prompts can contain secrets.** Storing them reverses a deliberate rule. Mitigations: 8000-byte cap, owner-only,
   never logged or audited, deleted with the session. A person may paste a token into a prompt; the docs say prompts are
   kept. A per-account "do not keep prompts" setting is deferred, not forgotten. The plan checks whether controller
   backups or exports include the column and updates their documentation.
2. **Rows are kept, so retention is explicit.** Session rows are never deleted; the prompt is cleared on archive (section
   3). A session that is never archived keeps its prompt for as long as its row is visible, which matches how long the
   person can still open it. The plan confirms that deleting an account removes its session rows (and so the prompts).
3. **Extra `gh` processes.** The GraphQL call roughly doubles `gh` runs per read, for open PRs only. The navigation's
   concurrency cap (3) and cadence (two or ten minutes) already bound the load; a `detail=0` request for the background
   read is the escape hatch if measurement shows a problem.
4. **Token scopes and GitHub Enterprise.** A signed-in `gh` without GraphQL access degrades to "threads not read".
   Enterprise hosts stay unsupported.
5. **Plan mode names differ per agent.** A mode is called plan-like only when its name contains "plan"; otherwise
   Planning never appears, which is the safe failure.
6. **`ChatPane.svelte` is large.** The prefill seam is one prop and one effect, reviewed on its own.
7. **Mixed versions.** New UI with an old controller or machine is a supported state and is tested (section 8).

## Deferred

Token and cost figures; a copyable standup-style session summary; an agent-declared "watching this PR" state; a
`detail=0` background read; a "do not keep prompts" account setting.
