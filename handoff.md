# Reet central-control implementation handoff

reet.sh is a self-hosted AI fleet control room: machines, the AI agents installed on them, skills,
reviewed sync, browser terminals and, now, an agents home with folder projects. This file is the
current state and the standing rules; start here. Where things are: [README.md](README.md) and
CONTRIBUTING.md; [docs/](docs/) for shipped behaviour (for example
[control centre](docs/control-centre.md), [terminal workspace](docs/terminal-workspace.md),
[projects](docs/projects.md), [folder browse](docs/folder-browse.md),
[remote machine operations](docs/remote-machine-operations.md),
[controller terminal commands](docs/terminal-controller-commands.md),
[OpenCode in Gaffer](docs/structured-sessions.md) and
[voice and copy](docs/voice-and-copy.md)); the agents-home mock-up in
[docs/design/agents-home/](docs/design/agents-home/), its
[plan](docs/superpowers/plans/2026-10-26-agents-home.md) and
[spec](docs/superpowers/specs/2026-10-26-agents-home-design.md); the 2 November UI refresh contracts in
`docs/superpowers/specs/2026-11-02-*.md`; what each UI change was checked against in
[docs/ui-verification.md](docs/ui-verification.md); the research behind the queued work in
[docs/research/](docs/research/); and older narrative in
[handoff-history.md](handoff-history.md).

## Gaffer AI session-companion MVP: agreed direction (5 November 2026)

The user selected the [Gaffer session-companion MVP](docs/work-orchestration-handoff.md#active-direction-gaffer-session-companion-mvp-5-november-2026)
as the initial AI-chat direction. Outside a session Gaffer is a general
Zoomies-style chat; each reet session has its own Gaffer conversation. Gaffer is
the user's quirky critical buddy: it reads permitted context and messages the
existing working model, which performs the coding and PR actions. Explicit goals
can continue for several selected sessions while the user is offline and recover
after controller restart. Initial follow-up supports verified structured chats.

The companion handoff records all choices, source evidence, provider/MCP
limitations and proposed GM0-GM6 slices alongside the broader KZ roadmap. The
plan [#600](https://github.com/eyupio/reet.sh/pull/600) and HTTP provider foundation
[#601](https://github.com/eyupio/reet.sh/pull/601) are merged. Tests use local fake
providers, with real credentials supplied later through Gaffer settings.

GM1B and GM2A now share [open PR #603](https://github.com/eyupio/reet.sh/pull/603),
`feat/gaffer-provider-settings`. The owner merged conversation backend #605 into
that branch, not into main. It contains account-owned providers, encrypted keys,
live model selection and Account → Gaffer AI, plus separate durable general/session
conversations, versioned model choices, turn receipts, SSE, cancellation and
restart recovery without replay. Only typed companion messages reach providers.
The [conversation/access contract](docs/handoffs/gaffer-conversations.md) records
worker eligibility, grants and queue receipt/recovery boundaries for later slices.

GM2B is [draft PR #606](https://github.com/eyupio/reet.sh/pull/606),
`feat/gaffer-floating-chat`, based on #603. Its floating general/session widget,
model choices, stream recovery and scope isolation are published separately;
read that branch's verification record before continuing its browser work. Do not
merge either PR without owner direction. Scoped context tools, worker messages
and durable offline supervision remain ahead; Gaffer has no coding or PR authority.

The 5 November #603 review follow-up fixes first-address-only provider dialing:
try all validated literal addresses, with dual-stack fallback and bounded,
cancelled/joined racers. A localhost HTTP regression fails on the old transport
and passes with the fix. The recurring offline-browse CI failure now uses fake
time for the request while retaining its 100 ms limit, six offline/access cases
and no-send/no-audit assertions; an injected 101 ms wait still fails. No browse
production source changes. Focused repeated race checks, web check/build, vet,
16 MCP checks, strict docs and the provider browser matrix pass. The new full race
run passes API (409 s), Gaffer (40 s), store (197 s), broker (66 s) and every package
except the same five baseline terminal-host failures. Final-head CI remains the
merge gate; see the companion handoff's dated follow-up record for exact limits.

Keep that handoff current in every implementation batch. Changes remain confined
to reet.sh; reference repositories are read-only.

## OVERVIEW-3: the Overview in the browser (5 November 2026)

Part C of the session overview, on branch `claude/session-overview-ui`. The right-hand panel's Overview now
answers where a session has got to and what is next. It reads the prompt route from OVERVIEW-1 and the extra pull
request fields from OVERVIEW-2; contents and rules are in `docs/terminal-workspace.md` ("Session workspace details").

- **Shipped.** A run clock line (`runClock.ts`); a "You asked" prompt card with a full-prompt dialog (`PromptCard`,
  `PromptDialog`, `promptLoader.ts`, `promptSource.ts`); a five-step tracker with one Next line built from facts only
  (`overviewStages.ts`, `StageTracker`); a pull request card (`PullCard`, `pullFacts.ts`, `pullVerdict.ts` shared with the
  GitHub tab); a Where card with copy buttons and a repository link (`WhereCard`); a status dot on the GitHub tab; and one
  automatic GitHub read per cadence that respects Live updates (`githubReadDue`, `insightStore`). The Overview itself is its
  own component (`OverviewTab`), moved out of `ContextPanel` with no change to what it showed. The final review's fix wave
  added a `Last known · checked …` marker on the tracker's Next line and the pull request card (`githubStale`), a Copy pull
  request link button, a prompt dialog that stays open across the 15 s session refresh, and the phone sheet closing after
  a fix request fills the composer.
- **The fix shortcut never sends.** "Ask the agent to fix..." fills the chat composer through `composerPrefill.ts`
  and `ChatPane`; a terminal, or a composer that is nearly full, disabled or off screen, gets a copy to the clipboard
  instead (`fixMessages.ts`, `fixDelivery.ts`). A repeated request does not stack. Check and branch names are cleaned
  before they are quoted because GitHub's users write them.
- **Honesty rules.** No reported checks is never green; threads not read are never zero; Merge is never claimed from green
  checks alone. An older machine or controller degrades to "not read on this machine" and "started before reet kept opening
  prompts" instead of a guess.
- **Validation.** See "Session workspace: the Overview" in `docs/ui-verification.md` for the commands, the files that
  ran, the one that could not (`terminal.e2e.mjs`, through the Go fixture, fails on the emoji line as it does without this
  change) and the tests seen failing with their rule removed. This task added a stability block to
  `session-workspace.e2e.mjs` covering every Overview slot at 300, 340 and 520 px and the 340 px phone sheet, both themes, and
  restored `toHaveText` on the Branch assertion in `sessions.e2e.mjs`.
- **Known deferrals (from the task reports).**
  - Check detail reads "plus 2 mores" with more than two failing or running checks; the test records it. Fix the wording and
    the test together.
  - `OverviewTab`'s scoped `section` rules apply to every `section` in it; use `div` for nested cards.
  - `newEvents` and `terminalAdvanced` are still derived in `ContextPanel` and passed down; they could move inside.
  - The prompt dialog's switching guards cannot be told apart by the browser test, because the session screen remounts per
    session; they are held by `promptLoader.test.mjs`.
  - The Live updates gate on the mount effect cannot be exercised end to end.
  - The Overview is about 250 px longer than before the two new cards (their heights are fixed, so nothing moves).
  - The fake clock in the cadence test only moves `Date`, so the Live-off test waits for the panel's real 5 s tick.
  - `answer` on `composerPrefill` is an extra field beyond the original brief, so a chat can say whether it took the text.
  - Wording not covered rule by rule: the fix messages (checks and singular threads wording were chosen in Task 7).
- **Compact at 14px (Task 16).** At the default 360 px panel the five steps are one row, the Next buttons share a row
  above the fix note's line, and the pull request card is a summary with a native **Details** fold for the secondary facts
  and actions (closed by default, kept open across polls; its summary row is two lines below about 388 px); the 300 px
  panel sets the steps three over two. Measurements
  and thresholds are in `docs/ui-verification.md` and the components' comments.
- **Not done.** Firefox, Safari and physical devices; screenshots; `terminal.e2e.mjs` passing in this container.

## OVERVIEW-2: the machine reads the extra pull request fields (5 November 2026)

Part B of the session overview. `internal/terminalhost/inspect.go` now fills the optional fields added to the
inspect wire in `3580786`: base and head branch, created, merged and closed times, reviews, review requests,
auto-merge, review threads, the last commit date and `DetailKnown`, plus `Remote` (`owner/name`) on the status
and GitHub views, so the page can link the repository before any pull request exists.

- The first `gh pr view --json` call gains only scalar fields (`createdAt,mergedAt,closedAt,baseRefName,headRefName`).
  Review bodies and commit lists could overrun its 128 KiB output cap and break the whole read, so reviews, review
  requests, auto-merge, threads and the last commit date come from one fixed-query `gh api graphql` call (owner, name
  and number as flag variables), for open pull requests only. A merged or closed pull request makes one `gh` call.
- Absent means "not read": a failed or `null` GraphQL reply leaves reviews, threads, auto-merge and last push unset
  while `DetailKnown` stays true; a first reply without `createdAt` is not `DetailKnown`.
- Rollout: the controller decodes machine replies with `DisallowUnknownFields`, so a machine sending these fields to
  an older controller has its inspect reply refused. Update controllers before machines (documented in
  `docs/terminal-workspace.md`).
- The 12 KiB reply cap still trims checks first; the new fields are small and a test holds that.

## CONCURRENCY-1: background work is stopped and joined (5 November 2026)

A Go concurrency audit of the connector, controller and agent loops found work that could outlive its owner or
wait without a deadline. Three focused batches merged on `main`:

- [#559](https://github.com/eyupio/reet.sh/pull/559) (`63f55857`): `Server.Close` stops the API queue, topic and
  refresh workers; health probe loops are joined; the backup-import pipe reader closes on every path; long-lived
  scan and stray-chat waits use tickers; a prompt, model change or mode change refuses once the chat supervisor
  is closed.
- [#569](https://github.com/eyupio/reet.sh/pull/569) (`60de2b79`): the software gate is now a context-aware
  weighted semaphore, so a launch waiting behind an apply gives up with its request, and the apply's work has a
  21-minute deadline; the chat ring evicts its oldest subscriber, as its comment promised.
- [#575](https://github.com/eyupio/reet.sh/pull/575) (`84bad853`): chat resume is tracked and canceled by
  `Close`; the API workers, health-prune loop and private-connection retry goroutine are joined, and their
  network and sweep contexts follow the server lifetime; model and mode controls check `closed` under the same
  lock as their session lookup.

`golang.org/x/sync` was already required indirectly; #569 imports its `semaphore` directly. Checks: `go vet
./...`; `go test -race` for `internal/terminalagent`, `internal/terminalbroker`, `internal/agenthealth`,
`internal/privateconn` and `internal/api` (549–575 s for API); the full CI race suite, browser matrix and Codex
review were green on each final head. Known limit: an apply runs the host's install step without a context, so a
hung filesystem syscall can still hold its goroutine, though launches now stop waiting for it.

## GAFFER-TIMEOUT-1: quiet commands survive the silence deadline (5 November 2026)

The turn watchdog treated a tool that did not stream output as a silent provider, stopping it after ten minutes.
It now excludes outstanding tools from both the provider-retry notice and the silence deadline. The two-hour hard
cap still applies, and the silence deadline resumes after the final tool finishes. An output-only update retains
the tool's last known state so late output cannot reopen a completed tool and suppress timeout recovery.

Three regressions failed before the watchdog fix: quiet command completion and a subsequent prompt, silence after
command completion, and the hard deadline. Removing the sparse-state fix separately makes the completion/silence
regression fail. After rebasing onto current main (`c594d81f`), full
`go test -race ./internal/acpagent ./internal/terminalagent -count=1`, `go vet ./...` and
`npm --prefix web run check` pass. The web check reports zero errors and two existing unused CSS selector warnings
in `LaunchBar.svelte` and `SessionHoverCard.svelte`. No Svelte files changed.
[PR #577](https://github.com/eyupio/reet.sh/pull/577), branch `fix/gaffer-quiet-command-timeout`, carries the fix;
final-head CI and review remain the merge gate. No live connector has been updated or restarted.

Review batches: [process recovery #480](https://github.com/eyupio/reet.sh/pull/480),
[retained state #485](https://github.com/eyupio/reet.sh/pull/485), and
[transcript retention #484](https://github.com/eyupio/reet.sh/pull/484). Eight reproduced defects are addressed across the three
focused batches below. Each PR records its checks and merge state. All handoff entries are retained when integrating the stack.

## OVERVIEW-1: the controller keeps and serves a session's opening prompt (5 November 2026)

First part of the session Overview. The controller now keeps the opening prompt with the session, so the
Overview can show it after the dialog is gone. Nothing in the browser reads it yet.

- **Kept.** A `prompt` column on `terminal_sessions`, capped at `terminalproto.MaxPromptBytes` (8,000 bytes).
  The session list and create replies carry only `promptExcerpt` (300 characters) and `promptMore`; the full
  text is read with `Store.TerminalPromptFor` and served by `GET /api/v2/terminal/sessions/{id}/prompt`
  (`internal/api/terminal_prompt.go`) as `{"prompt": ...}` with `Cache-Control: no-store`. A session that is
  not the caller's is a 404, a malformed id a 400.
- **Cleared.** Both archive paths remove the prompt from the row and from the create operation's stored
  parameters. An archived session answers an empty prompt. A retry of an archived session's create that
  carried a prompt now answers a conflict instead of replaying; this is deliberate and pinned by a test.
- **Never logged.** `auditTerminalLaunch` records no prompt; `TestLaunchAuditNeverCarriesThePrompt` fails
  when the prompt is added to the detail.
- **Docs.** `docs/terminal-launch.md` ("What is kept") and `docs/start-agent-session.md`, whose
  "the prompt is never kept" sentence was no longer true.

## AUTOTITLE-1: automatic session names (5 November 2026)

A session started without a title now gets a name that says what it is, instead of the first 80 characters of
the prompt or the same `Claude Code · studio` for every session started without one. The name is offline and
deterministic: no model, no network, no schema change and no setting. Choosing between this, the agent's own
title after its first turn and a model-written title was the one real decision; the first was picked because
the controller makes no outbound calls today and the README promises none.

- **Rules.** `terminalproto.PromptTitle` and `AutoTitle` (`internal/terminalproto/autotitle.go`), mirrored by
  `promptTitle` and `titleFor` in `web/src/lib/agentLaunch.ts`. The first line with something to say; blank
  lines, code fences, the skills picker's `Use the {name} skill.` line and punctuation-only lines are skipped;
  heading and bullet markers and English filler (a greeting, "please", "can you", "I'd like you to", "help me",
  "let's") are removed; the name ends at the first sentence (`e.g.` does not end one, and nothing under 8
  characters does); an ASCII first letter is capitalised; over 60 characters it is cut at the last space (not
  before the 20th character) with an ellipsis, else at 60. With nothing usable it is `<Agent> · <folder>`, or
  `Terminal` and `Terminal · <folder>` for a plain shell. Stated in full in
  [Launch requests → Session titles](docs/terminal-launch.md#session-titles).
- **One list of cases for both languages.** `internal/terminalproto/testdata/autotitle.json` (38 prompt cases,
  14 whole-title cases) is read by `autotitle_test.go` and `web/src/lib/agentLaunch.test.mjs`, so the title the
  dialog suggests cannot drift from the one the controller would give. The expected values were worked out by
  hand from the rules before the code existed. Five deliberate breakages of the rules (the limit scan, the
  abbreviation guard, the minimum word, the minimum sentence, the skills line) each failed the fixture.
- **Where it applies.** `POST /api/v2/terminal/sessions` names a blank or whitespace-only title (an absent one
  used to be a 400); `POST`/`PUT /api/v2/session-backlog` names a draft that has a prompt or note but no title
  (nothing to go on is still a 422); `projectrun.SelectionTitle` calls `PromptTitle`. The plain-terminal dialog's
  **New session name** is optional (it was pre-filled with `Shell` and required) and its placeholder and hint
  follow the folder; Home's machine-card shell takes the same name. A title that is sent or typed is kept as
  typed, Rename is untouched and sessions that already exist are not renamed.
- **Visible changes.** An unnamed plain shell is now `Terminal` or `Terminal · <folder>`, not `Shell`, matching
  the `Terminal · <folder>` that "Open a terminal here…" already used; the `home`, `start-agent` and
  `terminal-organisation` e2e expectations moved with it and `terminal-start` gained the folder case. A long
  prompt now gives a 60-character name ending in an ellipsis, not a hard cut at 80, so `defaultTitle`'s unit
  expectations changed.
- **For the next author.** `terminalhost.TestPackageSourceHasNoInvisibleCharacters` also scans
  `internal/terminalproto/*.go`. Write zero-width, bidi and line-separator characters as `\u` escapes, and check
  a new file's bytes afterwards: tooling can turn an escape into the literal character, which is what that test
  caught here.
- **Not covered.** A session started with no prompt, or one whose prompt is typed into the terminal afterwards,
  can only be named from its agent and folder; a name from what the agent actually did needs the agent's own
  title or a model, both ruled out above. The filler list is English only. Go's `(?i)` folds a few non-ASCII
  letters that JavaScript's `/i` does not, so a prompt written with, say, a long s could be named differently by
  the two; no real prompt is affected and the fixture holds no such case.

## GAFFER-CLINE-1: Cline chats in Gaffer (5 November 2026)

Cline CLI 3.0.74 runs its ACP server with `cline --acp`, and reet now offers it as a Gaffer chat beside
OpenCode. The profile (`internal/acpagent/cline.go`) launches `cline --acp --auto-approve false`, requires
`act` mode, reads the boolean `auto_approve` back as `false` before any turn (`Spec.RequireOptions`), applies a
chosen model with a confirmed `session/set_config_option` because Cline ignores one at `session/new`
(`Spec.ConfirmModelBySwitch`), and routes the person's own `~/.cline` through `CLINE_DIR` while keeping the
private chat home. The reviewed tool set, the Cline handle shape (`<ms>_<5 base64url>_cli`), the descriptor
(history, images, model, approvals, act/plan modes) and the scan probe are wired through
`internal/terminalproto`, `internal/scan`, the browser's chat adapter table and both start dialogs. Cards now
show a read's file, an editor's `path`/`new_text` as a diff and a `commands` list as the command; a card with
nothing to review stays unapprovable. Opt-in `TestRealClineStart` passes against the installed binary with no
prompt: version floor, identity, act mode, a confirmed plan/act switch and a confirmed model switch. Two prompts
with every permission rejected recorded the cards, and a stored conversation replayed through `session/load`.
The evidence and limits are in [the Cline ACP probe](docs/research/cline-acp-probe.md). Not covered: images, MCP
tools or subagents in a turn. A provider failure Cline logs without an ACP frame is now shown as that turn's error
([PR #573](https://github.com/eyupio/reet.sh/pull/573)); in-place reconnect for a lost Cline chat is in review
([PR #567](https://github.com/eyupio/reet.sh/pull/567)).
Merged as [PR #554](https://github.com/eyupio/reet.sh/pull/554) (`3919deef`); all 18 checks passed, including the
browser suite that runs the new `chat-cline.e2e.mjs`.

### The model picker shows the agent's whole catalogue (5 November 2026)

Cline reports 307 models; a live control reply capped a model selector at 16 values inside 10 KiB, so most of the
catalogue was unreachable without typing an id. `acp-model-catalogue-v1` widens the model selector to 512 ids in a
snapshot bounded at 56 KiB — the worst case a valid snapshot can reach, and still one frame because `MaxControlBytes`
is 64 KiB — while the effort selector keeps its 16-value bound. The session keeps every value the agent offers;
`chatHost.forConnection` trims the model list to 16 for a connection that did not negotiate the feature, on a copy, so
an older controller never receives a frame its decoder rejects, the session snapshot stays whole, and a reconnect with
the feature gets the full list without asking the agent again. Tests: `TestModelCatalogueBoundsTheModelSelectorNotEffort`
(proto, both directions on the wire), `TestModelCatalogueKeepsEveryOfferedModel`, `TestModelCatalogueIsTrimmedPerConnection`
and a 307-entry case in `web/src/lib/models.test.mjs`. In review as
[PR #586](https://github.com/eyupio/reet.sh/pull/586).

## SEO-1: public search surface and documentation (4 November 2026)

The public-search audit and its current status are in [SEO_AUDIT.md](SEO_AUDIT.md). The continuation state, remaining
tasks and a copy-paste starting prompt are in [the SEO follow-up handoff](docs/handoffs/seo-follow-up.md). Read that
file before changing the public routes, metadata, sitemap or docs.

Merged on `main`: real 404s and private-route noindex (#479, `e54cd9c`); install-first landing CTAs (#481, `aed7d36`);
server-rendered community and public lists (#483, `187a5c7`); hosted skill page metadata and sitemap entries (#487,
`7f713c2`); documentation first published at `/docs/` (#489, `3d8d1bb`) and now a standalone MkDocs site built by
`.github/workflows/docs.yml`; AI-crawler files and public nav pointing at the docs (#494,
`5b4050a`); the first guides and skills.sh comparison (#496, `f507b6f`); the sitemap crawl contract test (#497,
`b6b6ea4`); readable public list addresses for new lists (#502, `35c2182`); Open WebUI and LibreChat comparisons plus
the no-VPN guide (#505, `e95cb81`); and landing-page links to the guides (#507, `701e1cd`).

The docs site is no longer embedded in the Go binary. `mkdocs.yml`, `docs/requirements.txt`, `hooks/docs.py`,
`overrides/main.html` and `.github/workflows/docs.yml` build it from the same `docs/*.md` files and publish it to GitHub
Pages. The docs own `https://reet.sh/`; `DOCS_URL` points there, and the repository variable `DOCS_SITE_URL` overrides
the canonical origin for another deployment. The app keeps only the public routes, sitemap and AI-crawler files.

Remaining, in order: a readable-link action with permanent redirects for existing public lists; external measurement
(Search Console, Bing, and an explicit analytics decision); the marketing/app bundle split; and more intent content.
Do not treat the lab timings in the audit as field data, and do not add a third-party tracker while "no telemetry"
remains the default.

## GAFFER-PREVIEWS-1: agent image previews and continuation (4 November 2026)

Start a new environment with [the current continuation handoff](docs/handoffs/gaffer-continuation.md). It records the
delivered Gaffer PRs, their final heads, validation results and merge shas using repository-relative paths. The image
preview implementation is in [PR #498](https://github.com/eyupio/reet.sh/pull/498) on `feat/chat-image-previews`, merged as
`0de8f7d`; its [feature handoff](docs/handoffs/chat-image-previews.md) covers the wire contract, private snapshot cache, UI,
recovery, limits and checked screenshots. [PR #490](https://github.com/eyupio/reet.sh/pull/490) (session actions) merged as
`99c7fd8` and [PR #493](https://github.com/eyupio/reet.sh/pull/493) (startup and backlog settings) merged as `918f452`.
The handoff refresh that records all three landed as #503 (`4539386`) and #504 (`80ae4cc`). No Gaffer pull request remains
open; follow the "Starting a fresh session" checklist in the continuation handoff before changing anything.

## GAFFER-START-1: launch and backlog model/effort selection (4 November 2026)

Implementation and verification are tracked in [the startup-options handoff](docs/handoffs/start-agent-options.md).
Both start dialogs and backlog creation/editing retain model and native effort choices. The new `acp-start-options-v1`
contract prevents older live-control connectors receiving unsupported create fields. Runtime confirmation precedes readiness
and the opening prompt. The local web, browser and focused race checks pass. [PR #493](https://github.com/eyupio/reet.sh/pull/493) merged as
`918f452`, after the toolbar PR #490 merged as `99c7fd8`; its final head passed every check and no review findings were open.

## GAFFER-LIVE-1: live settings and durable message delivery (4 November 2026)

Implementation and verification are tracked in [the live-controls handoff](docs/handoffs/live-chat-controls.md).
This covers negotiated runtime model/effort selectors, capability-aware steering, durable shared queues and recovery
from uncertain delivery. [PR #488](https://github.com/eyupio/reet.sh/pull/488) merged as `ac5938d` after final-head checks and review fixes passed.

## GAFFER-BOUNDS-1: keep history bounded around waiting cards (4 November 2026)

Final asynchronous review follow-up: the process waiter could mark the chat closed before buffered final ACP frames reached
a slow event sender. The connector now owns its stdout pipe, observes exit independently, retires descendants and gives the
reader up to two seconds to drain before ending the session. A permanently blocked sender still cannot hide an exited agent.
Tests reproduce loss of a flushed final chunk before the fix and cover bounded shutdown with a blocked callback. The fake
child disables the race runtime's exit sleep for this case so that sleep cannot hide the shutdown race. This resolves the
post-merge review on #485 as part of #484; independent exit detection and the inherited-stderr bound are preserved.

Branch `fix/gaffer-transcript-bounds`. The browser reducer skips individual pending approvals and questions when evicting
old history, so one unanswered card no longer pins every later item. Tool commands, paths and diffs now count towards the
256 KiB text budget. Pending cards remain answerable; if cards alone exceed the budget they are retained. This is a reducer
change, with no new UI controls, wire fields or dependency. `docs/structured-sessions.md` describes the retention rule.

`chatBounds.test.mjs` covers both failures and is included in `npm test`: a waiting approval and question followed by a
long stream retain the cards and newest output within the limits; tool-only command/path/diff payloads also trim. Both
tests failed against the unchanged reducer and pass with the fix. Validation and merge status are recorded in the PR.

CI follow-up: the parallel terminal-upload fixture used arrival-order filenames for identical files, then asserted selection
order, so reversed request arrival caused a false failure. Distinct file bytes now determine fixture paths; the second
response deliberately completes first and the test still requires insertion in selection order. No upload runtime change.
## GAFFER-STATE-1: retain live choices and actionable replay (4 November 2026)

Branch `fix/gaffer-recovery-state`. `chatHost.setModel` now copies the process's actual model read-back into the live
session and its existing restart record, even on a failed or unconfirmed switch. It does not guess from the requested model
or recreate a removed record. A plain refusal keeps the old read-back; an unknown model clears the old launch choice.

The controller's live chat ring evicts ordinary history before pending approval/question cards and the latest status,
matching the connector's detached queue. Count and byte limits remain hard; pending state alone can still fill them.
Resolved cards become evictable again. Live subscribers receive the newly published event even when that event itself
cannot be retained in a ring full of pending state. The stored transcript remains bounded and connector reconnect replay
continues to restore retained pending cards after a controller restart.

Regression tests first reproduced stale restart models (confirmed, unknown and different-model replies) and loss of a
waiting approval/status at both ring limits. Tests also cover explicit refusal, question cancellation and a ring filled
entirely with pending state. Validation and merge status are recorded in the PR. No wire or dependency changes.
## GAFFER-RECOVERY-1: retire unusable agent processes (4 November 2026)

Branch `fix/gaffer-process-recovery`, integrated by `fix/gaffer-recovery-state`. An EOF or oversized frame in the ACP stdout
reader retires the process group. Process exit is observed independently because children may inherit stdout. A broken output stream can no longer leave a chat
waiting on a live process until the turn timeout. Provider errors recovered from the local log also mark the turn as
cancelled for incoming requests and arm the existing forced-stop grace, while keeping the provider error visible.

Regression tests reproduce closed stdout, an oversized frame and a provider failure whose adapter ignores cancel.
The integration regression also covers a leader exiting while a child keeps stdout open; waiting for the reader first
delayed that exit, so the integration fix keeps the independent process waiter and kills the group when reading ends.
A one-second `exec.Cmd.WaitDelay` also bounds inherited stderr held by a descendant after leader exit; both stdout-only
and stdout-plus-stderr cases are covered. Without that bound, exec's stderr copier can hide the leader's exit indefinitely.
All three failed before the fix and pass under the race detector afterwards. The full ACP race suite passes. The orphan
cleanup test now checks live process-group retirement, matching production, because a container's PID 1 may retain exited
zombies; the old test also failed on the unchanged baseline here. Full validation and merge status are
recorded in the PR. Follow-on review batches cover retained chat state and browser transcript limits.

Recovery still requires the agent's saved conversation. A framing failure ends the process; it does not retry a prompt
automatically. No wire format, dependency or agent availability changes.

## AUTH-CARD-1: the account card agrees with the landing page (3 November 2026)

Branch `claude/fervent-wright-5omayg`. Copy only. The card on the sign-in, sign-up and first-run owner-setup pages
(`AuthCard.svelte`) now says "Your whole AI setup. Ready to control." instead of "...On every machine.", so it agrees with the
landing page's h1. The page title and meta description keep "on every machine" on purpose. `first-run.e2e.mjs` (`authfit`)
holds the words. Checks and screenshots in [UI verification](docs/ui-verification.md).

## LANDING-1: the control-room landing page (3 November 2026)

Branch `claude/fervent-wright-5omayg`. Copy and UI only; no API, database or server change. The signed-out page now says
what reet.sh is for: command agents on any machine, sync skills and setups to one machine or all, back them up, and bring
an empty machine up to speed with one command. The h1 second line is "Your whole AI setup shows up, ready to control."
(`copy.ts`, `index.html` h1, meta description and crawler copy, README, `docs/yorkshire-copy.md`, tests). New:
`ControlRoom.svelte` (hero diagram with a wide and a compact drawing, replay, terminal built from `enrollCommands` with env
`fresh-vps`; final state at once under reduced motion, no Replay button), a copy chip under the buttons, a four-tile strip,
three new steps, and `StartFresh.svelte` (before and after slider, native range input with a label and `aria-valuetext`).
The terminal palette lives in `landingTerminal.css` because `theme.test.mjs` forbids literal colours in components. The "On
the workbench" line now says "Agent software: OpenCode today, more agents to come." and the sync card says a machine applies
a plan only when its owner has opted in. The sign-in card headline was left alone at first; see AUTH-CARD-1 below.

Checks: `npm run check` (unit suites, svelte-check 0 errors, 0 warnings), `public-workspace.e2e.mjs` and `first-run.e2e.mjs`
against `vite preview`, `go test ./web/`. Screenshots and scope in [UI verification](docs/ui-verification.md). The copy chip
omits `--server` as approved: the command takes the server from `REET_URL`, so it works on a machine where that is set.
The boot-shell skeleton in `index.html` still draws the old hero shape; it is decoration and was not changed.

## CHAT-REATTACH-1: reconnect a lost chat in place (3 November 2026)

PRs [#451](https://github.com/eyupio/reet.sh/pull/451) (merged: design, plan, protocol, negotiation, store, machine and
controller) and [#459](https://github.com/eyupio/reet.sh/pull/459) (route, web, browser contract, docs), branch
`claude/tender-johnson-k3xmf1`. A lost or crashed (`exited`) structured OpenCode chat now reconnects **in place**: same reet
session row, URL and transcript, through `session/load`. Feature `chat-reattach-v1` (requires `acp-v1` and `acp-history-v1`),
wire body `chat_reattach` carrying the row's launch data, capability `chatReattach`. The machine drops the replay (the
controller holds the transcript), forwards the post-load idle status and interrupted-tool closes, adopts a live process
instead of restarting it, and refuses a conversation another session owns ("This conversation is open in another session.").

Controller `Broker.ReattachChat` is single-flight per session (concurrent callers join one machine call), bounded at 75 s,
leaves the row untouched on error and never creates a row. The store allows `lost|exited -> running` for structured chats
only (exit code cleared), and a continuation row whose create operation failed can no longer itself be continued, closing
the many-rows path in the legacy flow. Route `POST /api/v2/terminal/sessions/{id}/reattach` (204; codes
`chat_reattach_unsupported`, `chat_reattach_not_closed`, `chat_reattach_no_conversation`, `chat_reattach_conflict` and `chat_reattach_trust` (with a `detail`) answer 409, `chat_reattach_failed` 502; a timeout or offline machine answers 503 "Reconnecting took too long. Try again.").
Web: one automatic attempt per session per page load; "Reconnecting to your conversation…"; failure shows the reason with
**Try again** and **Start a new chat**; manual **Reconnect**; the header menu no longer offers the new-row "Continue in
Gaffer with history…" for a chat that can reconnect. `ended` chats and older connectors keep the new-row path, now with a
single-flight guard and an in-pane "Starting a new session…". Docs: `docs/structured-sessions.md`, spec and plan under
`docs/superpowers/`.

Checks. Full run, earlier in Task 10: `go vet ./...` exit 0 and `go test -race -timeout 40m ./...` exit 0, 40 packages `ok`, including `internal/api` 791.503s and `internal/store` 339.278s. Those two slow packages were not re-run afterwards. Re-run in the fix round (raw logs `.superpowers/sdd/2026-11-03-chat-reattach/task-10-go.log` and `task-10-web.log`, not committed): `go vet ./...` exit 0 and `go test -race -count=1 ./cmd/... ./internal/terminalproto/ ./internal/terminalagent/ ./internal/terminalbroker/` printed `ok .../cmd/reet 16.283s`, `ok .../internal/terminalproto 1.184s`, `ok .../internal/terminalagent 8.255s`, `ok .../internal/terminalbroker 63.251s`. In `web/`, `npm run check` ran its unit tests (53 `# fail 0` summaries, no non-zero `# fail`) and svelte-check printed `COMPLETED 360 FILES 0 ERRORS 0 WARNINGS 0 FILES_WITH_PROBLEMS`; `npm run build` printed `built in 9.58s`.

Known limitations: auto-reattach sends no folder-trust confirmation, so a folder with OpenCode plugins refuses with the
trust message (resolved by follow-up F3 below: the person can now reconnect in place with **Trust this folder and
reconnect**, beside Continue with history and Start a new chat); YOLO chats are
refused (not downgraded) if the account or the machine no longer allows YOLO; while the machine is offline the pane shows the idle ended note (not a waiting state) and a failure consumes the one
automatic attempt; Claude/Codex chats are not covered. Not exercised: a live OpenCode provider reload and production chats.

Final-review fix wave (commits `ae23404` Go, `c4cd2b0` web, and this docs commit). A person's End of a Gaffer chat is
answered by the machine with `exited` (code 0), so ended chats were stored `exited` and auto-reattached; the store now
records that close of an `end_requested` structured row as `ended`, and `ReattachChat` refuses any row with an end
operation (older rows stored `exited`/`lost`) with `chat_reattach_ended` ("You ended this chat. Continue it with history as
a new session."), which the browser treats quietly on an automatic attempt. Reattach checks the account's Allow-YOLO setting
first (`chat_reattach_yolo_not_allowed`), keeps the machine's YOLO refusal (`chat_reattach_yolo_refused`) and capacity
(`chat_reattach_capacity`) instead of the retryable failure, and a chat still starting on the machine (its conflict result
carries the detail `starting`, sent only to a controller that advertised `chat-reattach-detail-v1`, so either side can be
updated first; an older controller shows its old "open in another session" wording for that rare moment) answers
`chat_reattach_starting` ("Still reconnecting. Try again in a moment.") instead of "open in another session". A non-OpenCode row answers "This chat cannot be reconnected in place."; the generic failure
now reads "The machine did not bring this chat back. Try again, or start a new chat.". Web: Try again only for retryable
codes, the failure card is an alert, one live region while pending, "Reconnecting" for an exited row in the sidebar, and
(ruling R10) a folder-trust refusal shows the plugin files and re-offers Continue with history (new row, `trust: true`) in
the pane and the header menu, without Try again.

Checks for the fix wave (raw logs `.superpowers/sdd/2026-11-03-chat-reattach/final-fix-go.log`, `final-fix-go-full.log`
and `final-fix-web.log`, not committed): `go vet ./...` exit 0; `go test -race -count=1 ./internal/store
./internal/terminalbroker ./internal/terminalagent ./internal/terminalproto ./internal/api -run
'Reattach|Observe|End|Yolo|Resume'` all `ok`; full `go test -race -count=1 ./cmd/... ./internal/terminalproto/
./internal/terminalagent/ ./internal/terminalbroker/ ./internal/terminalhost/` all `ok` (terminalbroker 63.564s); full
`go test -race -count=1 -timeout 40m ./internal/api/ ./internal/store/` printed `ok .../internal/api 731.812s` and
`ok .../internal/store 308.621s`. Other Go packages were not re-run in this wave. In `web/`, `npm run check` (53 `# fail 0`
summaries, none non-zero; svelte-check `COMPLETED 360 FILES 0 ERRORS 0 WARNINGS 0 FILES_WITH_PROBLEMS`), `npm run build`
(`built in 12.38s`), and against a preview build `chat-recovery.e2e.mjs` and `sessions.e2e.mjs` passed. `chat.e2e.mjs`
failed twice in its Skills-install section while the full Go race run loaded the machine, then passed on a third run
after it finished; the same section also failed once on the unchanged baseline build in this session.

Follow-up F1: refuse an archived chat (local branch `followups/reattach`, commit `dd9a731`). `Broker.ReattachChat` refuses a
row with an entry in `terminal_session_archives` (new store helper `TerminalArchivedFor`, scoped by account) with
`ErrReattachArchived` before any machine call; the route answers 409 `chat_reattach_archived` "This chat was archived, so
its history is gone.", and the web treats it as final (no Try again, no Continue with history). In the final-review wave the
archived check moved above the ended checks, so a chat that was both ended and archived answers archived too. Checks
(`.superpowers/sdd/reattach-followups/f1-report.md`, not committed; all focused, none full): `go vet ./...` clean; `go test
-race -count=1` for `internal/store -run 'Archiv|Reattach|Observe'`, `internal/terminalbroker -run Reattach` and
`internal/api -run Reattach` all `ok` (timings not recorded); `reattachState.test.mjs` `pass 10 fail 0`; `npm run check` 0
errors 0 warnings.

Follow-up F2: negotiate the still-starting marker (commits `28f3e1c` code, `5e2555b` docs, nit `d3f0156`). New feature
`chat-reattach-detail-v1`: the controller advertises it, the connector echoes it, and the machine sends `conflict` with the
detail `starting` only to a controller that advertised it (any other gets a plain `conflict`), so the controller and the
machine can be updated in either order. Checks (`f2-report.md`, `f2-verify.log`, `f2-api-full.log`, not committed):
`go vet ./...` clean; full `internal/terminalproto` `ok 1.132s`, `internal/agenthealth` `ok 6.852s`, `internal/terminalagent`
`ok 7.673s`; focused `internal/terminalbroker -run 'Reattach|Conflict|Feature'` `ok 10.679s` and `internal/api -run
'PrivateControl|Reattach'` `ok 26.007s`; full `internal/api` `ok .../internal/api 714.878s` (an earlier run hit the default
10-minute test timeout and was re-run with `-timeout 40m`).

Full `internal/store` once on the final code before the final-review wave (`final-store-full.log`): `ok
.../internal/store 255.988s`; it was not re-run after that wave, which did not touch `internal/store`.

Follow-up F3: trusted in-place reconnect (local branch `followups/reattach`, commits `24c602b` Go, `b0b6fd9` web, and this
docs commit). `POST /api/v2/terminal/sessions/{id}/reattach` takes an optional body: none, `null` or `{}` mean no trust,
`{"trust": true}` passes trust for that request (the controller never stores it; each click is one request), anything
else is 400 before the machine is asked. The machine keeps the trust in its restart record exactly as for any chat started
with trust (including Continue with history), so its own restart reopens the chat with it (record kept up to 7 days or 3
failed reopens); a later in-place reconnect from the controller asks again.
`Broker.ReattachChat(ctx, uid, session, trust)` sends `Trust` on the wire, records `trust` in the `chat-reattach` audit
event, and keys its single-flight on session and trust, so a trusting call never joins a non-trusting one. The
`chat_reattach_trust` sentence is now "This folder has OpenCode plugins, which run as code on this machine, so reet did not
reconnect automatically. Reconnect only if you trust it.", and the card shows only the plugin-file list from the machine's
detail (not its "Start a chat only…" sentence). Web: after that refusal the card offers **Trust this folder and reconnect** (primary), **Continue
with history** and **Start a new chat**, still no Try again; only that click sends `{"trust":true}` (the automatic attempt
and every other request send the earlier `{}` body). Checks (raw logs `.superpowers/sdd/reattach-followups/f3-*.log`, not
committed): `go vet ./...` exit 0; full `go test -race -count=1 ./internal/terminalbroker/` printed `ok
.../internal/terminalbroker 72.247s` and full `./internal/api/` printed `ok .../internal/api 733.013s`; `internal/agenthealth`
and `internal/terminalproto` full `ok`; other Go packages were not re-run. In `web/`, `reattachState.test.mjs` `# pass 14`
`# fail 0`, `npm run check` (svelte-check `COMPLETED 360 FILES 0 ERRORS 0 WARNINGS 0 FILES_WITH_PROBLEMS`), `npm run build`
(`built in 9.64s`), and against a preview build `chat-recovery.e2e.mjs` and `sessions.e2e.mjs` passed.
F3 fix round (review): the copy says plugins run as code on the machine, the card shows only the plugin-file list, the docs
say what the machine keeps, and a terminalagent test pins the machine's trust gate on a reattach (refused without trust,
nothing left running; starts with trust). Checks: `go vet ./...` exit 0; full `go test -race -count=1
./internal/terminalagent/` `ok 7.914s`; `./internal/api/ -run 'Reattach|RefusalSentences'` `ok 35.018s`; `reattachState.test.mjs`
`# pass 15` `# fail 0`; `npm run check` 0 errors; `npm run build` `built in 10.56s`; `chat-recovery.e2e.mjs` and
`sessions.e2e.mjs` passed against a fresh preview of that build.

## CHAT-RECOVERY-1: continue a closed chat with saved history (3 November 2026)

PR [#445](https://github.com/eyupio/reet.sh/pull/445), branch `fix/chat-history-recovery`, implementation commit `e511a4a`,
based on main `e91afad`, adds **Continue with history** beside the ended-chat reason
when the OpenCode conversation id is saved and the machine advertises history support. It uses the existing exact-id
continuation operation, creates a new row with the same folder/model, and sends no prompt automatically. A missing process
no longer claims that native history could not be restored. The banner keeps the fresh-chat choice.

Failed connector-start restores retain the native id in their lifecycle reports. A successful manual continuation replaces
the old restart record. Manual and automatic loads share a supervisor gate that refuses a conversation already starting,
running, or held by a process whose termination is unconfirmed. No protocol or database migration and no dependency added.

Checks: web unit suites, svelte-check (0 errors/warnings), production build, the dedicated recovery browser contract,
existing OpenCode chat browser contracts (legacy and descriptor capabilities), Claude/Codex browser contracts and Go vet pass. The recovery contract covers
1360/390/320 px, both themes, lost/exited/ended states, unsupported history, offline machines, reload refusal and the same
operation id after an uncertain reply. New Go recovery tests pass under race; the retained-id and restart-record tests and
the web wording tests fail on unchanged main. Captures and scope are in [UI verification](docs/ui-verification.md).

The repository race run is not green: `TestCloseKillsOrphanedGroupMembers` and
`TestListModelsTimeoutKillsTheProcessGroup` also fail on unchanged main in this environment. The API suite exhausted its
ten-minute package budget while an unrelated authentication test was running; that test and
`TestChatResumeNeedsAClosedChatOwnedHere` pass together under race when targeted. The full run passes terminalagent,
terminalbroker and store. A separate affected-package run hit the timing-sensitive
`TestSwapToChatTerminatesALingeringTerminal`; it passes in isolation on both main and this branch.

CI on `c80bfce` passes the full browser matrix and native service jobs, including macOS. Its full race run exposed a
timing race in `TestSupervisorResumesPersistedChat`: the provisional Running row preceded process installation and the
registry write. The test now attaches before startup and waits for the real lifecycle receipt before ending the chat.
Before the fix it failed three times in 100 repetitions; the fixed recovery/supervisor group passes 100 repetitions under
race. Automated code review could not run because its usage limit was reached; the patch was reviewed directly.

Production chats, live provider reloads and preservation of a running reply across connector death were not exercised.
The owner authorised watching the PR, fixing failures and merging once CI is green. Nothing is deployed or merged. Next: watch
[#445](https://github.com/eyupio/reet.sh/pull/445) and CI, starting with `internal/terminalagent/chat_registry.go` and
`web/tests/chat-recovery.e2e.mjs`; restoring a lost conversation still depends on its native store being available.

## Gaffer block mascot and executive summaries (2 November 2026)

Draft [#418](https://github.com/eyupio/reet.sh/pull/418), branch `feat/gaffer-block-mascot`, based on main `1e626ff`, carries the owner-approved block portrait,
stepped facial animation and compact status card. Yorkshire quips rotate every eight seconds while
the agent works. Pause, hidden-tab handling and reduced motion cover the decoration, including idle
expressions. The final working and summary mocks are in `docs/images/ui/gaffer-*-desktop.png`.

The collapsible executive summary uses retained chat events: the latest turn while a session is
open, all retained activity when it ends. It shows the agent’s final handover, reported file and
command operations, failures, unfinished calls and a next step. Truncated history and missing
handovers are explicit. No provider request or stored report is introduced.

Web unit tests, svelte-check, the production build and the Gaffer browser matrix pass. The browser
matrix covers replay, phase changes, animation pause, summaries, failures, session end, keyboard
controls, both themes and 320/390/768 px widths. See [UI verification](docs/ui-verification.md) for
the approved mocks, phone captures and animation recording. The full chat browser fixture, Go vet,
race-enabled web/CLI packages and the API suite without race pass. The full race attempt exposed
existing process, model-listing and missing-tmux fixture failures, reproduced on unchanged `1e626ff`.
API uses the documented local non-race run; the completed UI build passes the web/CLI race rerun.
Main `4959472` (#416) was merged without rewriting history to bring in the startup-hook race fix;
both supervisor-resume regression tests now pass under race. CI results belong in the PR.
Next: review the draft PR and its checks; merge only on the owner’s instruction.

## UI refresh: directory, inbox, admin and machines (2 November 2026)

The owner asked for a page-by-page UI refresh with a UX focus. Each family was built test-first,
photographed at desktop and phone widths in light and dark, reviewed, and merged only after CI was
green. Contracts and rulings are in `docs/superpowers/specs/2026-11-02-directory-pages-refresh.md`,
`...-skills-machines-findability.md`, `...-inbox-pages.md`, `...-admin-users.md`,
`...-admin-team-usage.md`, `...-admin-tickets.md`, `...-admin-email.md` and
`...-admin-notifications.md`, `...-admin-backups.md`, `...-admin-settings.md`,
`...-admin-activity.md`, `...-account-sessions.md`, `...-account-tokens.md`,
`...-account-profile.md`, `...-account-security.md`, `...-account-mcp.md`,
`...-skill-card-machine-actions.md`, `...-machine-add-skill.md`,
`...-skill-remove-from-machine.md`, `...-skills-sync-vocabulary.md`,
`...-written-skill-record.md`, `...-written-skill-lifecycle.md` and
`...-session-skill-drafts.md`; checks are in [docs/ui-verification.md](docs/ui-verification.md).
The refresh merged through #457. Merge main into the next branch as later work lands.

**Merged, all with every required check green:**

| PR | Merge | Scope |
| --- | --- | --- |
| #395 | `5c7463b` | Lists, Community and Teams into one collection grammar: panel, count, search, honest scope chips, one row anatomy, folded Teams management, phone toolbar. |
| #400 | `01bb06f` | Jump finds shelf skills with saved sync intent; “Skills you create” rows say where each skill is; Machines search by host, alias and skill. |
| #403 | `26797e8` | Setups list in the same panel with a count, search, relative recency and honest revision wording. |
| #405 | `1235328` | Per-scope machine cleanup moved from the card disclosure into **Settings → Access** (audit H3). |
| #407 | `3211b32` | Notifications and Support in one panel grammar; locked security preference is an **Always on** fact; Support status chips and search. A Codex review found that the unread list must stay server-side; fixed and regression-tested. |
| #408 | `f051948` | `/sync` marks Machines current (desktop and phone); simple Sync source renamed **Everything on my shelf**. |
| #410 | `d61c6c0` | Admin → Users as one people panel with count, search, role chips, per-person rows and the existing Manage actions; invite is its own card. |
| #415 | `1e626ff` | Admin → Team usage as one report panel: count, search, List/Tag/Sort, one row per skill with metrics and Team disclosure; wide table and duplicate mobile cards removed. |
| #419 | `d374e00` | Admin → Support as one ticket queue panel: count, search, status chips whose figures follow the search, one row anatomy; detail and composer unchanged. |
| #420 | `4f18c10` | Admin → Email as one outbox panel: count, search, state chips whose figures follow the search, one row anatomy, retry; settings unchanged. |
| #421 | `90fa3a6` | Admin → Notifications as one grouped defaults panel: count, Save, locked facts, retry; no search. |
| #422 | `a08acb1` | Admin → Backups copies and runs in the collection grammar: counts, search, overlapping location chips, verify damage, retry; settings unchanged. |
| #423 | `6fc3197` | Admin → Settings as one instance panel: six grouped settings, Save, read-only view; immediate cards now only on Settings. |
| #424 | `ba3ce32` | Admin → Activity as one audit panel: newest 100, count, search, area chips, one row anatomy; no API change. |
| #425 | `85bde14` | Account → Sessions as one browser panel: count, search, this/other chips, sign-out actions; search reads friendly devices. |
| #426 | `415d2dd` | Account → API tokens as a create card plus one token panel: count, search, used/never chips, one-time secret, revoke. |
| #427 | `00840c7` | Account → Profile as one grouped preferences panel: count, Save, blank-date fallback; viewer name read-only. |
| #428 | `f920dd2` | Account → Password as one panel: three fields, head action, mismatch check, successful body and notice. |
| #429 | `f7e668c` | Account → MCP access clients in one panel: count, search, OAuth/bearer chips, connection actions; page sections unchanged. |
| #432 | `106c2fc` | Skill cards: a large fleet collapses to one installed/missing/unconfirmed summary; unconfirmed machines offer Add now that queues a scan. |
| #434 | `6d75e72` | Machines: Add a skill… picker on the card menu, shelf candidates with Installed tags, existing install intent; disabled without an agent. |
| #438 | `2a334ca` | Skill cards: Remove… opens Sync with an exact-set removal prepared; no write until save, preview and approval. |
| #441 | `e91afad` | Machines audit minors: profile-format timestamps, a single Commands close, shared icons, attention order and a large-fleet status filter; paused cards show an amber dot. |
| #446 | `d759055` | Skills and Sync vocabulary: one glossary, Sync only under Machines, **Write a skill** with a next-step link, and install locations and sync plans on cards and warnings. |
| #450 | `5ee31b8` | Written skills: **Your drafts** holds unpublished skills, a published one appears once as its shelf card with **Edit written skill**, and the authoring list names the machines. |
| #455 | `790a489` | Written-skill lifecycle: the shelf row is a hidden projection, so unpublish/delete hide it while tags, notes and list membership survive; rename carries the hidden state and all list readers exclude hidden rows. |
| #457 | `b0b087a` | Session picker: **Your skills** installs a private draft or a published skill through the same private-revision preview/approve path; a missing or unavailable skill has its own refusal. |

**What the refresh established.** The directory pages (Lists, Community, Teams, Setups) share one
bounded surface, a count, search, scope chips whose figures follow the search, one row anatomy and
panel-owned loading, empty and error states. Lists keeps Rows/Cards. Teams folds management per
group. Setups keeps its master-detail layout.

**Findability and safety.** Jump searches sessions, folder projects, shelf skills and machines; a
skill result carries `Skill · <saved intent>` and opens `/skills?skill=<id>`. A created skill shows
saved intent first, then observed state, then **Not on a machine yet**; published skills link to
their shelf card. Machines search reads host, saved alias and every observed skill. Per-scope Forget,
whole-machine Forget and Revoke now share **Settings → Access** with the usual confirmations.

**Inbox and admin.** Notifications loads its unread tab with the server-side predicate, so an old
unread beyond the newest page remains reachable; its chip and **Mark all read** use the server's
authoritative count. Support has status chips whose counts follow the search. Admin Users is the
proving page for Account & Admin: invite card, people panel, role chips, inline role change and the
existing Manage disclosure, with no wide table or horizontal scroller. Team usage is the second:
one report panel with its existing filters, one row per skill, and the drift and people views still
folded below. The Support queue is the third: one ticket panel with a live count, search, status
chips whose figures follow the search, and the conversation detail unchanged. The Email outbox is
the fourth: one message panel with a live count, search, a chip for every state, and rows that stack
on a phone; the mail settings stay above it. Notification defaults is the fifth: one grouped panel
with a count, Save, the existing locked security facts and rows that stack on a phone. Backups is
the sixth: copies and runs each have a count, search, chips, one row anatomy and a phone layout;
the schedule, destinations and import stay as they were. Settings is the seventh: six grouped
instance settings in one panel with Save, plus the immediate Approvals, Repository maintenance and
Software artifact mirror cards that now appear only there. Activity is the eighth: the newest 100
events with a count, search, area chips and one row anatomy. All seven Admin sections are now in the
collection grammar. Account Sessions is the first Account page in it: count, search, this/other
browsers and the existing sign-out actions. API tokens is the second: a create card, then a token
panel with count, search, used/never chips and the existing revoke confirmation. Profile is the
third: eight preferences in four groups with Save in the panel head and a blank date format shown as
the device's order. Password is the fourth: three fields in one panel with the mismatch check and
the existing successful-change notice. MCP access is the fifth: one Connected clients panel with a
count, search and OAuth/bearer chips; the connect card and other sections stay.

**Rulings recorded in the specs.** Scope chips keep no URL state; Rows and Cards both survive; the
directory pages use rows; ownership and role are each printed once; recency is relative with the
exact time in the title; Teams management stays in-page and folded; Jump reads the loaded shelf;
machine search matches observed reports, not saved intent; the notification unread query stays
server-side; the invite card stays above the people list; the role badge is hidden when the role
select is shown. No API, store, permission or dependency was added for the refresh.

**Not claimed.** No physical phone or tablet, no screen reader, and no Safari or Firefox for any of
the refreshed pages. Touch sizes were measured in an emulated coarse-pointer viewport. The Machines
search matches observed reports, not saved intent. All seven Admin sections are refreshed. All
Account list and form pages are refreshed: Sessions, API tokens, Profile, Password and MCP access;
Account Backups inherits the refreshed Backups panels, and Account Activity already had its own
count, filters and day groups. Skill cards now summarise a large fleet in one press, let an
unconfirmed machine be scanned before adding, and prepare a reviewed removal in Sync. A published
written skill appears once as its shelf card; **Your drafts** holds the unpublished ones, and the
shelf card's **Edit written skill** action returns to the editor. Unpublishing or deleting hides
that card while keeping its tags, notes and list membership for a later republish; the hosted
content and shelf row now share one lifecycle. The authoring list names the machines for every
skill. Machine timestamps follow the profile's zone and date order; a large
fleet sorts by attention and can be filtered by status. Skills and Sync use one glossary, Sync has
one place under Machines, and the authoring page is **Write a skill** with **Next: Put it on a
machine** after every save. The session picker also installs a private draft
through the same private-revision path, with no publish step.

**Next UI work:**
1. **Physical-device checks** for the refreshed pages: 320/390 touch, screen reader, Safari/Firefox.
   Do not claim these from the Chromium fixtures.

## Import into a running controller (4 November 2026)

Branch `backup-import-running`, on top of `backup-restore-checks`. `POST /api/v2/admin/backups/import` is now `multipart/form-data`
(options first, then `file`; header `X-Reet-Import: 1`), owner only, with the password in the body. The upload is streamed to a temp file and
bounded. `Service.StageImport` refuses a controller that holds data (`ErrNotEmpty`, `409`) unless `replace`, runs `prepare`, copies
the live database to `reet.db.before-import-<ts>` with `VacuumInto`, then stages it; `ApplyPendingImport` still swaps it in at the next
start. `reet backup import --server URL --token-file F FILE` is the matching command. Empty means the first owner and nothing else
(`holds` in `restore.go`; the topic tags and the server's own `reet-server` environment a new controller makes for itself are not counted). The old octet-stream plus `X-Copy-Password` form is gone.

## Careful whole-controller restore (4 November 2026)

Branch `backup-restore-checks`. `reet backup restore` checks everything before the swap, in order: integrity, schema version (the
refusal names both data formats), `--owner-email`, then prints the inventory (accounts by role, shelves, skills, lists, tags,
environments, connected machines, destinations). Without `--yes` it reports and changes nothing. Sessions and personal tokens now come
across by default; `--drop-sessions` and `--drop-tokens` end them, and `--keep-tokens` is an accepted no-op. Sealed secrets the
controller's key cannot open are cleared (a destination is also switched off) and listed with what to do. The choices are written to the
restored database's activity log as `backup-restored`. The staged import over the API shares `Service.prepare`, so it takes the same
checks and the query options `dropSessions`, `dropTokens` and `ownerEmail`. Next: an import command and a replace guard for a
controller that already holds data.

## Whole-controller restore and import (2 November 2026)

`reet backup restore` now adapts the copy before it replaces anything (`internal/backup/restore.go`, `internal/store/store_restore.go`):
browser sessions and reset links end, personal tokens and MCP connections are revoked unless `--keep-tokens`, the private-connection
identity is dropped, stored secrets the controller's key cannot open are named, and a copy stamped with a newer `PRAGMA user_version`
(`store.SchemaVersion`, stamped by `store.Open`) is refused. The owner can stage a copy on a running controller with
`POST /api/v2/admin/backups/import` (also `GET`, `DELETE`); `backup.ApplyPendingImport` swaps it in at the start of `serve`.
The import has an Admin → Backups control (`Backups.svelte`). Left for later: bumping `store.SchemaVersion`
when a release changes the schema incompatibly. See [docs/backups.md](docs/backups.md).

## Structured chats (SC1–SC5): resume here (2 November 2026)

**Read first:** [docs/structured-chat-plan.md](docs/structured-chat-plan.md) (the plan and every progress note),
[docs/device-checks.md](docs/device-checks.md) (what needs a real device or login) and
[docs/research/structured-adapters/probe-report.md](docs/research/structured-adapters/probe-report.md) (what each adapter was
measured to do). This section is the position and the order of what is left.

**Where it stands.** OpenCode is the only structured chat anyone is offered. Claude Code and Codex are built end to end but
dark: a machine reports one as available only when its connector runs with `REET_EXPERIMENTAL_CLAUDE_CHAT=1` or
`REET_EXPERIMENTAL_CODEX_CHAT=1` (`internal/acpagent/spec.go`: `ClaudeEnabled`, `CodexEnabled`). Descriptors are per agent
(`internal/terminalproto/acp_agents.go`); the controller binds each to its reviewed adapter; the browser offers a chat only from
a descriptor that says `available`.

**Main state (2 November 2026):** #409 merged as `c0b0c49`; #414 (connector-restart recovery) as `f64b407`; the UI refresh through
#415 as `1e626ff`. The earlier documentation-only #401 merged as `d5a4e80`. The active bounded SC5 slice is now the measured descriptor
option contract on branch `feat/descriptor-chat-options`; it adds no control, login gate or process behaviour. Older notes below describe
the slices' publication state at the time, not current `main`.

**On main (merged):**
- SC1–SC2: per-agent descriptors; probes of the Claude and Codex adapters (report above).
- SC3: typed agent questions, machine core to chat card; the form capability (`elicitation.form`) and Claude chats stay off until
  the touch and screen-reader checks (device-checks section 1) are recorded.
- SC4 Claude: `ClaudeCandidate` profile (`internal/acpagent/claude.go`), descriptor reports approvals and the model switch only, browser
  offers it from an available descriptor, credential-check script `scripts/check-claude-credentials.py`.
- SC5 audit fixes: force-stop of an agent that ignores cancel (`ForceStopGrace`), late permission and question callbacks refused
  (`turnAcceptsRequests`), neutral "this chat has ended" wording, tools left running when a turn stops.
- SC4 Codex: profile `CodexCandidate` (`codex.go`, floor 1.13.1, requires and starts in `read-only` via `Spec.StartMode`, only
  `read-only` and `agent` choosable via `Spec.ChoosableModes`, Stop ends the chat when a command is running via
  `EndChatOnStopWhileToolRuns`); `Session.SetMode` confirms a switch with the agent and never reports the old mode after an
  unanswered one (it clears the mode and tries once to restore); feature `acp-agent-mode-v1` (descriptor `modes`,
  `Capabilities.ACPAgentMode`, message `acp_agent_mode`, broker `SetAgentMode`, `POST /api/v2/terminal/sessions/{id}/chat/agent-mode`);
  the confirmed mode is stored on the session (`terminal_sessions.agent_mode`, JSON `agentMode`; empty means unknown), set to the
  first listed mode at create, to the new mode on a confirmed switch, and cleared on an unconfirmed or lost one.
- Probe scripts: `scripts/probe-codex-turn.py`, `scripts/probe-codex-start.py` (no model call; `--auth none|api-key|chat-gpt`,
  `--codex-home`) and `scripts/probe-agent-options.py` (initialize and `session/new` only; measured option values, no prompt).

**Merged on 2 November:** #375, "Offer Codex chats in the browser with an approvals-mode control that says what each mode means", and
#384, "Show the agent's reason on an approval card and say a Codex reject ends the turn". #375 makes the controller refuse a message
with a 409 (`ErrACPModeUnknown`, `PromptChat`) while an agent that offers modes is in an unknown mode, makes `codex` a reviewed chat
tool in the browser, and adds the Approvals control (`web/src/lib/terminal/AgentModeControl.svelte`, wired in `ChatPane.svelte`: the
wording for each mode, Send held while the mode is unknown, a YOLO-aware sentence), the Start and Home dialog copy, and
`web/tests/chat-codex.e2e.mjs`. #384 puts the model's justification (`_meta.permission.description`, carried in the permission event's
existing `text` field, so no wire change) on the approval card under a "Why {agent} asks" label, makes Codex's reject button read
"Deny and end the turn", and adds `TestChatHostSwitchesAgentMode` for the connector's mode switch. Both merged with every check green,
`phone-zoom` included.

**`phone-zoom` on main:** green again. #380 ("Let the skill card's sync line wrap long host names") merged first, then #375 took `main`
in and dropped its own copy of that rule, so `SkillCard.svelte` matches `main`. The failure had been the Skills page laying out 355 px
wide at 320 px since #370.

**Decisions already made (do not re-ask):**
- Codex starts in `read-only` (the only mode that can ask before a command leaves the sandbox); `agent` is chosen per chat, never the
  default; `agent-full-access` is never offered. The wording is in the plan and `AgentModeControl.svelte`.
- No "allow always" for Codex (its `accept_execpolicy_amendment` writes a persistent rule); allow once or reject only.
- Stop while a Codex command runs ends the whole chat (cancel leaves the command alive).
- A silent sandbox denial or an `agent`-mode refusal is not visible in the adapter's events; accepted, hence the labelled mode.
- Codex sign-in: reet never calls `authenticate` (`chat-gpt` opens a browser on the machine). The person signs in to Codex
  themselves; the Codex profile will set `CODEX_HOME` to their own Codex folder and pass nothing else from their real home; a failed
  start says "Sign in to Codex on that machine first"; the adapter's `_auth/status_update` label is shown in the chat, never the email.
- The current mode is stored on the session, not sent only as an event.

**Codex login access merged (2 November, [#391](https://github.com/eyupio/reet.sh/pull/391)):** the owner approved building Codex login access from source evidence and keeping section 3's signed-in Run B as
final verification. The profile now routes `CODEX_HOME` to the connector's own setting or original `HOME/.codex`, keeps replacement `HOME`, drops XDG
home directories for Codex, never calls `authenticate`, maps the authentication-required start error and forwards only the bounded status label as a notice.
Neither feature switch is retired. See the new progress note in the plan and source checkpoint in the probe report; test and publication results belong
in that PR. The earlier requirement to wait for section 3 before coding is superseded for this slice. Run B remains the next owner check; if it still
says "Authentication required", stop and ask the owner which way to go. No hardware or genuine-login check has been completed here.

**Owner checks, still unverified:** the owner deferred section 3 signed-in Run B ("skip this, assume it works for now"). This is permission to
continue independent SC5 work, not evidence of a real login. Do not re-ask while it remains deferred. When resumed, signed-in Run B runs first,
then Run A's follow-up; if Run B still reports "Authentication required", stop and ask which direction to take. Next are the signed-in runs
(section 2 Run B for Claude and section 3 Run B for Codex), then section 1's physical touch and screen-reader checks. Neither experimental switch nor
`elicitation.form` is retired before its required evidence is recorded. Record each result in the ledger at the top of
[docs/device-checks.md](docs/device-checks.md#recorded-results) with its date and the PR that carries it. Reet never calls `authenticate`.

**Recorded 4 November 2026 (logged-out runs, pinned adapters):** Claude `claude-agent-acp@0.99.1` advertises no login methods, creates a session
without a login and refuses the first prompt with `-32000 Authentication required` (`authenticatedHint: false`); `TestRealClaudeStart` passes against
that adapter with no model call; Codex `codex-acp@1.13.1` refuses `session/new` before any authenticate and reports
`{kind: none, label: "Not logged in"}`. Because Claude answers at the prompt rather than at session/new, the prompt path now maps that refusal to
`acpagent.ErrClaudeLogin` ("Sign in to Claude on that machine first.") the way session/new already did for Codex, so a signed-out machine says what
to do instead of looking broken; the chat shows it as a sign-in step, not an error to diagnose (merged as `c600e114`, and the
status now carries `code: "login"` so the panel can say so). Signed-in runs for both agents and every phone/reader item are still open.

**Delivered SC5 slices:**
- #392: live model controls and lookups follow each available agent descriptor. Merged; bot review was blocked by account quota and its terminal CI
  failure was reproduced on main and left outside scope.
- #396 (`35225eb`): pending approvals/current status survive full/stalled reconnect replay to the surviving connector. All implementation CI passed
  on `ef2a418`; Linux native services used its single allowed retry. Codex review quota was exhausted: no submitted review or finding threads.
- #402 (`a78ffa6`): delivered typed-question cards recover with original tokens, schema and deadline; detached resolutions retire state, older peers
  receive no question frames and pending questions are cancelled. New questions without a negotiated recipient remain cancelled. All CI passed on
  `7dec2cc`; Codex review completed without findings. The native fixture now waits for foreground startup readiness before status polling, which
  otherwise can steal startup's enrollment lock; private failure logs are retained.
- #406 (`6973f08`): live model replies must confirm applied state. OpenCode alone permits omitted-list live acknowledgements; malformed/ambiguous or
  lost replies clear process/stored model and retained status detail. Explicit protocol refusals keep the prior model; later confirmation recovers.
  All five CI workflows passed on final head `59f896f`, without retries; Codex review completed without findings
  ([receipt](https://github.com/eyupio/reet.sh/pull/406#issuecomment-6585858018)). No genuine-login/device gate was closed.
- #409 (`c0b0c49`): explicit startup model confirmation before admission or the opening prompt. Its full validation is below.
- #414 (`f64b407`): structured chats recover after a connector restart or reboot from the persisted live-chat registry. Its startup resume race is fixed in #416.

**Merged SC5 slice — startup model confirmation ([#409](https://github.com/eyupio/reet.sh/pull/409), `c0b0c49`):** an explicitly chosen model must have exactly one valid matching read-back from `session/new`
or `session/load` before admission or the opening prompt. Startup has no omitted-list exception, even for OpenCode. Agent-default launches preserve
existing behaviour. Definite failed chat creates clear unconfirmed model metadata but retain the operation fingerprint/refusal for replay;
ordinary terminals and transport uncertainty are unchanged. A close callback before admission cannot announce a premature exited session; the admitted
process's Done watcher still observes an exit racing startup. No extra model-setting RPC, wire field, database column or adapter capability is added.

New fake-child/controller regressions failed before these fixes and pass under race. Real pinned OpenCode 1.19.5 and Claude adapter 0.99.1
(SDK 0.3.312) were probed on Linux x86-64 with isolated homes, without prompts or `authenticate`. OpenCode new/load reported `opencode/big-pickle`;
Claude new/load reported `default`. Requesting `anthropic/claude-sonnet-4-5` through OpenCode's private config still read back `opencode/big-pickle`:
config alone did not establish that selection. Enhanced opt-in Go tests passed for explicit matching OpenCode new/load and Claude new. These are
startup/profile checks, not real-login or model-turn evidence. Exact versions, reproduction and limits are in the probe report.

**#409 publication and validation:** merged as `c0b0c49`. The checks below describe implementation head `bd10520`; later heads have their own CI. All five CI workflows passed on that head, without retries:
[full tests/image](https://github.com/eyupio/reet.sh/actions/runs/41513691712) (full Go vet/race, web and MCP),
[terminal browser and both phone jobs](https://github.com/eyupio/reet.sh/actions/runs/41513691713),
[macOS/Linux native services and minimum tmux](https://github.com/eyupio/reet.sh/actions/runs/41513691764),
[private connection](https://github.com/eyupio/reet.sh/actions/runs/41513691722) and
[binary release](https://github.com/eyupio/reet.sh/actions/runs/41513691723). Local affected-package race checks, Go vet, API tests, web check/build
and all 16 MCP tests passed. The five known model-discovery/orphan container failures were reproduced on main `6973f08` before local exclusions;
CI passed the full suite without those exclusions.

Codex bot review did **not** complete before the merge: the account's code-review quota was exhausted
([bot response](https://github.com/eyupio/reet.sh/pull/409#issuecomment-6586917713)); there are no submitted reviews or inline finding threads.
The bot summary's finding flags do not establish findings when the review failed. This handoff-only follow-up records the completed implementation
checks; any new-head CI results belong in the PR. Do not represent the earlier head's green checks as checks of a later documentation commit.
The slice is merged; no deployment is claimed. Login/device checks remain deferred and unverified; both experimental switches and form capability stay off.

**Resume startup race fixed in #416 (this PR):** `newChatSupervisor` no longer starts persisted-chat adoption before a caller can install its
process hooks. `NewChatSupervisor` and the connection-scoped fallback call `startResume()` once; the two resume tests call it after their fakes.
The race in #415's `test` job reproduced on unchanged main and is fixed here. See the PR for the package race and vet results.

**Delivered SC5 slice — connector-restart recovery ([#414](https://github.com/eyupio/reet.sh/pull/414), merged `f64b407`):** a recorded chat
is reopened with `session/load` under the same reet session id after the connector restarts, the replay is dropped because the controller
already has the transcript, and the controller stores the same bounded transcript in SQLite. Archiving drops the record and transcript;
records older than seven days are dropped, and a failed reopen is attempted at most three times. Detached per-session workers and turn
generations across a native process restart remain future work; a turn in flight when the connector stopped is reported as stopped.

**Active SC5 slice — measured descriptor option contract (`feat/descriptor-chat-options`):** a new negotiated descriptor field `options`
describes the non-permission selectable options a pinned adapter was measured to offer. It is bounded, ordered, distinct and descriptive
only: no switch, startup option, UI control, database column or adapter capability is added beyond `acp-agent-options-v1`. The existing
`modes` list remains the permission-sensitive switch list and `model` the dynamic catalogue, so mode and model are not duplicated. Measured
values are Claude ACP 0.99.1 effort `default`/`low`/`medium`/`high`/`xhigh`/`max`, `fast` and `ultracode` (`on`/`off`); Codex ACP 1.13.1
`reasoning_effort` `low` through `ultra`, `fast-mode` (`off`/`on`) and `collaboration_mode` (`default`/`plan`). OpenCode 1.19.5 reported no
other option. Values can depend on the current model, so the contract says what may be offered, not what is valid now. The probe sent no
prompt, made no model call and used no real credential; Codex used a synthetic api-key placeholder for `session/new`.

**Next independent work after this slice:** a bounded effort control can use this contract, reading the live option list from the adapter and
confirming a switch before reporting it. Detached workers and turn generations across a native process restart remain future work.

**Working rules the owner set (keep following them):**
- One item at a time: build it test-first, open a **draft** PR, watch it, and **merge only when the owner says so** (they often mark
  PRs ready or merge them themselves). Then take the next item.
- Decisions for the owner: one question at a time, recommendation first. Do not re-ask decisions recorded here.
- Keep the requested commit trailer `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`. Use truthful Codex generation attribution;
  do not invent or request a Claude session URL. PR bodies use What and why / How I checked it / Checklist and end with
  `🤖 Generated with [Codex](https://openai.com/codex/)`. GitHub comments end with `---` and `_Generated by [Codex](https://openai.com/codex/)_`.
  Never name or link an external reference project in a commit or PR.
- Every ready PR gets a Codex bot review. Treat each finding as a claim to verify against the code: fix what is real with a test that
  fails without the fix, reply on the thread, resolve it. Merge `main` into a branch for conflicts or a fix that landed after the branch
  was cut; never rewrite history. A failing CI job is re-run at most once; a second failure is real, or is shown to be red on `main` too.
- The Codex review sometimes finds a second-order effect of an earlier fix (an unconfirmed mode, a lost reply, YOLO, a held message).
  The pattern that held: when something may have been applied but was not confirmed, record "unknown" and refuse to proceed, never
  claim the old state.

**Running things here:**
- In this persistent workspace activate the cached toolchain first: `. /workspace/.reet-tools/activate.sh` (Go 1.27.1).
  The system `go` is unrelated. Avoid reinstalling available tools; web/MCP dependencies are cached.
- `go test ./internal/api` takes about two minutes without `-race`; with `-race` it can exceed a ten-minute timeout in a small
  container (CI allows sixty). Run the other Go packages with `-race`.
- Browser tests: `npm --prefix web run build`, then `npm --prefix web run preview -- --host 127.0.0.1 --port 4173`, then
  `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium REET_UI_URL=http://127.0.0.1:4173 node web/tests/<name>.e2e.mjs`.
  The agent chooser in the Start dialog is a dropdown (`getByRole('combobox', { name: 'Agent' }).selectOption`), not radios.
- The probe scripts are in `scripts/`, but the adapters they run are not in the repository: install the pinned versions with
  `npm install --ignore-scripts` as the probe report says.
- A fresh container may have no Go toolchain: `curl -sSL https://go.dev/dl/go1.27.1.linux-amd64.tar.gz | tar -xz -C ~/gotool` and put
  `~/gotool/go/bin` on `PATH`. The first `go build ./...` downloads modules and takes minutes. `TestCloseKillsOrphanedGroupMembers`
  (internal/acpagent) and the `TestModels*` family (internal/terminalagent) fail in this container on `main` too (no OpenCode binary,
  no process-group control), so do not read them as a regression; check the same test on `main` before chasing one.
- Creating a branch by pushing history containing workflow changes can fail with the token's missing `workflows` permission.
  Create the remote branch from current main through GitHub's branch tool first, then push updates. Use command-scoped
  `git -c 'credential.helper=!gh auth git-credential' fetch/push` if cached Git HTTPS auth fails. Never print credentials.
- Never use `pkill -f '<pattern>'` or `grep` for a pattern that also appears in your own command line: it ends the shell.

## SR1 — Self-healing automatic sync (2 November 2026)

**Scope:** the controller↔enrolled-agent sync job protocol; no new remote shell or arbitrary
command surface. Branch `fix/sync-self-heal`, based on main `f051948`.

**Problem:** a preview approved for apply could find that local files, the target revision or the
saved selection had changed. The machine refused correctly, but the controller failed the whole
automatic run, so later scopes waited behind a card the person had to press. The retry only
re-queued the one scope. That is the failure shown in Sync activity as **The plan changed before it
ran**.

**Fix:** the machine now answers the recovery direction, not just receives jobs. `internal/scan`
classifies the stale-plan refusals as recoverable and reports the new wire status `replan`; no
files were written. `internal/store` accepts `replan` only from preview/apply/automatic jobs,
records the attempt honestly as failed, and — when the job belongs to a running automatic run —
queues a fresh automatic job at the same `sync_run_jobs.position`, leaving the cursor untouched so
the run continues into later scopes. The controller-side "desired revision changed during apply"
check takes the same path. A scope gets at most three attempts per run (`syncRunReplanLimit`); after
that the run fails and the existing **Fix and try again** starts a fresh plan. Manual previews and
applies never retry themselves. Results stay idempotent through `sync_receipts`, and the agent's
durable `.result` file still resends rather than replaying filesystem work.

**Checks:** `gofmt`, `go vet ./internal/store ./internal/scan`, `go build ./...`, and
`go test ./internal/store ./internal/scan` (including
`TestSyncRunReplansStalePlanAndKeepsGoing`, `TestSyncRunStopsAfterReplanLimit`,
`TestSyncRefusesAChangedApprovedPlan`, `TestOrdinarySyncFailuresAreNotReplanned`).

**Limits:** this does not add a general controller-to-machine command executor; the existing
allowlisted typed jobs and local opt-ins still gate every write. A run interrupted by a disconnect
or timeout is still failed by the reaper rather than automatically re-planned, because partial
writes cannot be proven absent.

**Next bounded task:** decide whether a reaped automatic run should requeue a fresh preview (safe,
read-only) and then continue, with the same attempt budget; and add an end-to-end agent fixture that
drifts local files between preview and apply to prove the whole loop over HTTP.

## Structured chat recovery across a connector restart or reboot (2 November 2026)

**Scope:** the machine's structured-chat supervisor and the controller's chat transcript; no change
to ACP, permissions or the launch gates. Uncommitted working-tree changes on `ux/boot-shell`.

**Problem:** a machine agent upgrade, a crash or a reboot ended every Gaffer chat, and the browser was
told its live history was gone. OpenCode keeps the conversation on disk, but nothing reopened it.

**Fix:** the connector now records each live Gaffer chat's `ses_…` id, folder, model, approval mode
and folder trust in a private `structured-chats.json` beside its terminal state. On the next start it
reopens each one with `session/load` under the same reet session id and drops the replay, because the
controller already holds the transcript; the controller then adopts the row as running instead of
marking it lost. Ending, archiving or switching a chat to the OpenCode TUI removes the record, folder
trust and YOLO are re-checked at reopen, records older than seven days are dropped, and a failed
reopen is tried at most three times. The controller now also stores the same bounded transcript (1,000
events / 256 KiB per session) in SQLite, restored on subscribe, so its own restart or a reboot keeps
the conversation rather than only the row. A lost row may return to running only for a structured
chat; a plain shell is never resurrected. The stale "held in memory and is gone" copy is corrected.

**Checks:** `gofmt`, `go build ./...`, `go test ./internal/terminalagent ./internal/terminalbroker
./internal/store ./internal/terminalproto ./internal/api`, and the chat/agent-launch browser unit
tests. No real machine was rebooted; the reopen path is covered by a fake ACP child and a registry
fixture.

**Limits:** a turn that was in flight when the connector stopped is reported as stopped, not resumed.
Detached per-session workers, turn generations across a native restart, and a live-machine reboot
check remain future work. Records and transcripts are stored in the controller database and the
machine's private state directory; the agent-side record holds metadata only.

## Gaffer interface and Eli compatibility probe (1 November 2026)

Branch `feat/gaffer-interface`, based on main `9b8bc17`. The native browser
interface is branded Gaffer, with shared accessible HTML explanations; stored
`reet`/`chat` choices, ACP sessions and OpenCode TUI remain compatible. A bounded
Eli provider/conformance probe and licence/dependency review contribute to KZ5A;
no coordinator or new adapter is enabled. See [the companion update](docs/work-orchestration-handoff.md#gaffer-branding-and-eli-reuse-1-november-2026)
and [reuse decisions](docs/research/eli-reuse.md). KZ0 and KZ5A remain incomplete;
the exact remaining decisions and future stage gates stay in those documents.
Publication and validation are recorded in the PR; no merge or deployment is implied.

## Durable work, Eli and Kanzo follow-on handoff (1 November 2026)

The staged [work orchestration handoff](docs/work-orchestration-handoff.md) is a
companion plan for the user's durable-work direction: steps assigned across
agents, models and machines, with revision-bound independent reviews and an
Eli-style coordinator informed by Zoomies. It owns stages KZ0–KZ12, including
independent Kanzo-inspired documentation/resource/refresh improvements. This file
remains the entry point for standing rules and the active task. The core stages
fit richer run orchestration; the current structured-chat sequence retains its
recorded priority and capability gates until the active task is explicitly changed.
No KZ runtime implementation stage is complete; the branding follow-up records
a bounded KZ5A probe. The 5 November session-companion MVP now owns the active
first-release direction, with GM0 as its next design slice. The broader KZ
workstream retains its KZ0 reconciliation and later dependencies. Record the one
selected slice here and keep the companion's active section authoritative.

**Docs tidy-up (27 October 2026).** Finished plans, specs, stage documents and audits were removed
on 27 October 2026; they live in git history before commit `bd9e56c` (for example
`docs/superpowers/plans|specs/2026-10-2[145]*`, `docs/agent-setup-plan.md`, `remote-machine-*` and
`docs/remote-machine-stage*.md`, and the root files `FEATURES_AUDIT.md`, `GEO-ANALYSIS.md` and `UI_AUDIT.md`). Older per-feature delivery notes (single sign-on PR logs, terminal
sudo repair, roadmap checkpoints, terminal polish, terminal-first workspace requirements, the
implemented baseline, S/A/W/L task lists, start-agent-session protocol notes, Projects PR 1 and
delivered, phone terminal/zoom/dialog notes, the ready-to-use continuation brief) are in
[handoff-history.md](handoff-history.md), under "Moved from handoff.md on 27 October 2026".

## Loading: compressed assets, one progress bar, skeletons everywhere (2 November 2026)

**Scope:** how every page arrives and how it waits; no API, data or copy change. Branch
`claude/optimistic-volta-ehhex6`.

**Problem:** filmed on a slow connection (1.6 Mbit/s, 150 ms), a first load painted nothing for about five
seconds: the built bundle is 1.3 MB of JavaScript and 440 KB of CSS and the server sent both uncompressed,
with no cache header, so the boot shell could not appear until the render-blocking stylesheet had arrived
and every visit paid for it again. Once the app was up, each page showed a bare `Loading…` sentence (or
nothing at all) while its sections popped in one by one, and a request made from a page gave no sign it was
in flight.

**Fix:** `web/compress.go` gzips each built file once, on first request, and serves the compressed body to a
browser that takes it (q-values honoured, `Vary` said either way, fonts and images left alone); the hashed
files under `/assets/` are `immutable` for a year and `index.html` stays `no-cache` but goes compressed too.
The main chunk is now 383 KB on the wire and the page 6 KB, so the boot shell paints in the first half
second on the same link. In the app, `lib/inflight.ts` counts every request `api.ts` makes and
`shell/ProgressBar.svelte` draws one thin bar along the top edge while anything is outstanding, after a
200 ms grace and for at least 400 ms, so a fast answer shows nothing and a burst reads as one wait.
`lib/Skeleton.svelte` is the one loading affordance for content (lines, rows or cards, the shape of what is
coming, with a visually hidden label for a screen reader) and `lib/LoadingBoundary.svelte` renders the
waiting, failed, empty and content states in one place; every bare `Loading…` line (twenty-two of them,
including the machine list the UI audit's H9 named) now shows a skeleton, and the two ad hoc skeletons
(the shelf, the machine grid) use the same component. A button that is doing what it was asked carries
`aria-busy`, which `app.css` draws as a small turning ring beside its label; spinners live there and nowhere
else.

**Checks:** `go test ./web/` gained tests for the compressed and cached assets, the plain body for a client
that refuses gzip, the uncompressed fonts and the compressed page; `lib/inflight.test.mjs` checks the two
delays against a fake clock and that `api.ts` reports to it; `npm run check` (unit tests and svelte-check,
0 errors) and `npm run build` pass; `first-run`, `first-skills`, `navigation`, `home`, `empty-states` and
`first-machine` browser suites pass against the built preview. Filmed again on the same throttled
connection, every route paints within 400 ms and the machines page shows card skeletons inside its own
frame until the inventory lands.

## Boot shell: no crawler page, no bare loading screen (1 November 2026)

**Scope:** the first paint of every route, signed in and signed out; no API, data or copy change beyond the
crawler page. Uncommitted working-tree changes.

**Problem:** on a slow connection the browser painted the static crawler copy of the landing page (the older
"One control room. For your AI fleet." layout, no header, system fonts) for as long as the bundle took, then
snapped to the real Welcome page. App routes showed a bare `Loading…` line while the first auth answer was in
flight, and opening a session left an empty page with one sentence while the terminal chunk downloaded.

**Fix:** `index.html` now carries three small theme-aware boot shells (landing hero, entry card, app page).
The head script marks `data-js` before the body is parsed, so a browser hides the crawler copy at once and
shows the shell in the same frame; a crawler that runs no script still gets the readable copy, now updated to
the current Welcome words. `embed.go` serves only the shell for the route, names it on `<html data-boot>`, and
therefore handles a cookie-signed-in visitor on `/` without the browser having to know the session. A static
preview picks the same variant from the path. `App.svelte` sets `data-ready` after the first auth answer, so
the shell fades once the real page is behind it and `main.ts` removes it; public routes that need nothing from
the auth answer (community, shared lists, join, reset) dismiss it as soon as they draw, even if that answer
stalls. A late bundle shows a "taking longer" line with a reload link after ten seconds; the line is a live
region outside the aria-hidden shell, so a screen reader receives it. The pre-auth branch and the terminal
chunk's gap now use skeletons (`BootSkeleton.svelte`, and a workspace-shaped card in `Machines.svelte`) instead
of sentences. The two faces the first screen uses are preloaded; reduced motion gets no fade.

**Also fixed (found by CI on the merge with main):** the Gaffer help tooltip is a read-only explanation but its
popover was in the top layer and intercepted pointer events, so on a narrow layout it could cover the launch
bar's own listbox and block an option. It is now `pointer-events: none`; `launch.e2e.mjs` asserts that and that
the slot, list and options stay clickable.

**Checks:** `npm run build`, `npm run check` (unit tests and svelte-check, 0 errors/warnings), `go vet` and
`go test ./web/` (including the new route-to-shell test), and `first-run.e2e.mjs` gained a throttled-bundle
case: the crawler copy stays hidden, only the landing shell is visible, it is removed after auth, a
no-JavaScript context still reads the landing copy, the recovery line is a live region outside the hidden
shell, and `/reset` draws before a stalled auth answer. `public-workspace.e2e.mjs` passes at light/dark ×
1440/390/320. Screenshots at 1280 and 390 in both themes, and a delayed terminal chunk, confirmed the shells
match the pages they hand over to.

## Welcome page: one command on any machine (1 November 2026)

**Scope:** the signed-out Welcome page and the entry card beside sign-in, set-up and sign-up; no API or
signed-in change. Branch `welcome-one-command-any-machine`.

The hero now leads with the moment a newcomer can picture, one command on any machine, and the lede
says what reet.sh keeps on every machine (agents, skills and setups: synced, backed up, restorable).
The illustration is the product's own enrolment: a Machines list with three machines in sync over a
terminal running `enrollCommands()` from `machineSetup.ts` line for line, so it cannot drift from what
Add a machine gives. The three steps are enrol a machine, agents, skills and setups stay in sync, put
your agents to work. The entry card's headline is "Your whole AI setup. On every machine." The voice,
the rose and the palette are unchanged, and the page still says self-hosted, never a service.
`public-workspace.e2e.mjs` and `first-run.e2e.mjs` hold the words and the illustration's commands.
The public header also carries the colour choice (`ThemePicker.svelte`: three icons, light, dark, then
match this device, the chosen one ringed) between the links and Sign in, in the links row at every width
so a phone's set-up form still fits its first screen; it applies through `applyTheme`, is kept in this
browser, and a signed-in profile's saved choice still wins. The signed-in account menu is unchanged. The
sign-in, sign-up and set-up leads say machines are enrolled and agents, skills and setups stay in sync.

## SC2 — Claude lifecycle probes and ID/diff hardening (31 October 2026)

**Scope:** follow-up to PR #301; no adapter is enabled. Branch `ccr-4cc5980a-dt7hki`.

**Added:** `scripts/probe-claude-lifecycle.py` (trust, cancel, load, model, subagent);
adapter-aware native ID validation (`nativeIDMatches`: `ses_` for OpenCode, canonical
lower-case UUID for Claude/Codex, still gated by `ReviewedChatTool`); and `typedDiff` in
`internal/acpagent/session.go`, which turns Claude's `{type:"diff"}` content into the
existing bounded 10 KiB diff and reads `rawInput.file_path`. A missing or unusable diff
still leaves an edit card unapprovable. Results are in the
[probe report](docs/research/structured-adapters/probe-report.md).

**Findings that block enabling Claude:** a repository's own hooks run without an approval
card; leader exit did not retire the whole process group; `session/load` ignores `cwd`;
an unknown model value is silently reset; subagent calls arrive as kind `think`.

**Checks:** `go vet` and `go test` for `internal/acpagent` and `internal/terminalproto`,
gofmt, `py_compile`, and a real run of every scenario against the pinned adapter with
the loopback stub. The full race suite, web checks and browser fixtures were not re-run:
no web or protocol-shape change. No real provider, phone or other OS.

**Hooks gate (local, not pushed):** `Spec.SessionMeta` is sent as `_meta` on session/new and
session/load; `ClaudeSessionMeta` pins `settingSources` to `["user"]`, verified to stop a
repository hook while approvals still appear. No Claude profile is registered yet.

**Also local:** `retireGroup`/`GroupRetired` (group SIGKILL, confirmed via /proc, zombie-aware) and
the `think` subagent card (`subagentSummary`). Tests: group_test.go, session_test.go.

**Also local:** `Session.SetModel` reads the model option back from `session/set_config_option`
and returns `ErrModel` (recording the real value) when the agent reports a different one; a
response without an option list keeps the old behaviour. Test: `TestSetModelReportsAppliedState`.

**Subagents and Codex (local):** nested subagent calls raise their own approval with a
`parentToolUseId`; reet labels them "Subagent:". Codex probe (`scripts/probe-codex-turn.py`)
showed no per-command approval request in any mode and a provider error disguised as
`end_turn`; Codex stays disabled. See the probe report.

**Environment (local):** `acpagent.ClaudeCandidate` (unregistered) holds the measured launch profile:
adapter command, name/version floor, `default` mode required, credential-only allowlist, no
provider redirects, background traffic off, `ClaudeSessionMeta`. Not registered.

**Next unchecked task:** Claude missing/real-credential behaviour and registering the profile behind the
gates (descriptor + launch + history); for Codex, re-run with a working
sandbox to see whether any approval request exists, and cancel on a running command; then SC3.

## SC3 — Typed agent questions, machine core (31 October 2026)

**Scope:** `internal/acpagent` only; no wire, API or UI change and no agent enabled. See the SC3 progress
note in [the plan](docs/structured-chat-plan.md).

**Added:** a normalized `Question` model (text, single select, multi select), a strict `elicitation/create`
parser that refuses unsupported forms whole, an authoritative answer validator (`Question.Validate`,
`DecodeAnswers`), and session handling with single-use IDs, deadlines, a pending cap and cancellation on turn
cancel, close and exit. `Options.Questions` is off in every launch, `initialize` does not advertise
`elicitation.form`, and the connector drops `question*` events.

**Checks:** `go vet`, `gofmt`, `go build ./...` and `go test -race` for `acpagent`, `terminalagent` and
`terminalproto`; two deliberate mutations (any choice accepted, answered question not consumed) failed the
tests. No browser, phone, real provider or live adapter was used.

**Next unchecked task:** private-protocol messages and negotiation, the controller answer route with
account checks, then the chat card and Needs-you summary; request-scoped (pre-session) questions and
old-peer/concurrent-tab/phone fixtures. Do not advertise the form capability before those.

## SC3 — Typed agent questions, machine core (31 October 2026)

**Scope:** `internal/acpagent` only; no wire, API or UI change and no agent enabled. See the SC3 progress
note in [the plan](docs/structured-chat-plan.md).

**Added:** a normalized `Question` model (text, single select, multi select), a strict `elicitation/create`
parser that refuses unsupported forms whole, an authoritative answer validator (`Question.Validate`,
`DecodeAnswers`), and session handling with single-use IDs, deadlines, a pending cap and cancellation on turn
cancel, close and exit. `Options.Questions` is off in every launch, `initialize` does not advertise
`elicitation.form`, and the connector drops `question*` events.

**Checks:** `go vet`, `gofmt`, `go build ./...` and `go test -race` for `acpagent`, `terminalagent` and
`terminalproto`; two deliberate mutations (any choice accepted, answered question not consumed) failed the
tests. No browser, phone, real provider or live adapter was used.

**Wire stage:** `acp-question-v1`, the
question events and `acp_question_answer`, `question_invalid`, connector routing, `AnswerQuestionChat` and
the `/chat/questions/{id}` route. Still off by default; see the plan.

**Chat card:** the browser card for typed questions, with unit and browser tests; see `docs/ui-verification.md`.

**Next unchecked task:** SC3 code and fixtures are done (startup questions are cancelled with their own notice; holding them is
deferred). Left: the touch-device and screen-reader checks on a real device. Do not advertise the form capability before those.

## SC4 — Claude wired dark (1 November 2026)

Claude is registered but dark: `REET_EXPERIMENTAL_CLAUDE_CHAT=1` on a machine's connector makes it report Claude as available
(approvals and the model switch only; no history, images or questions), the controller binds each descriptor to its reviewed
adapter, and a descriptor-less peer is still OpenCode-only.

**Superseded:** the browser now offers Claude and Codex from an available descriptor, and the next tasks are listed in
"Structured chats (SC1–SC5): resume here" at the top of this file.

## SC2 — Structured adapter probe checkpoint (31 October 2026)

**Scope:** separate probe PR targeting main; no new runtime adapter is enabled.
Branch `feat/structured-adapter-probes`, following the merged
[SC1 PR #300](https://github.com/eyupio/reet.sh/pull/300). See
[the probe report](docs/research/structured-adapters/probe-report.md) and its
narrowed protocol fixtures. This is partial SC2 evidence, not completion of the
first additional-agent integration.

**Observed:** Claude adapter 0.99.1 / SDK 0.3.312 / native Claude Code 2.1.312,
and Codex adapter 1.13.1 / CLI 0.156.1, on Linux x86-64 with Node 22.23.4.
Adapter tarball SHA-512 values match recorded registry metadata. Both real
adapters initialized and created UUID-style sessions with fs/terminal delegation
disabled; Codex needed explicit synthetic-key ACP authentication. Claude Bash
and Write allow/deny controlled actual marker-file effects against a local stub;
form negotiation enabled AskUserQuestion and a typed answer completed the turn.
Write's typed content diff needs normalization before reet can review/allow it.

**Checks:** runnable standard-library Python probes, syntax/formatting and
recorded wire fixtures; actual candidate process versions and stub-backed
permission/question assertions. SC1's complete Go race suite, vet, web
check/build, MCP and browser checks passed separately. No live provider, real
credentials, physical phone or cross-OS verification was used here.

**Next unchecked task:** finish Claude's trust/override, cancellation/process
retirement, history/load, model and subagent probes; then add adapter-specific
UUID validation and bounded edit normalization. Keep unverified candidates and
form capability disabled. Codex tool/approval/provider/error paths need their own
stub probes. Read `scripts/probe-structured-adapters.py`,
`scripts/probe-claude-turn.py`, the report, `internal/acpagent/session.go` and
`internal/terminalproto/acp_agents.go`.

**Publication:** [PR #301](https://github.com/eyupio/reet.sh/pull/301) records this
checkpoint against main, following SC1's verified merge `e902b869`.
Its canonical PR record carries the current merge status; this checkpoint does
not enable an adapter or perform a release/deployment. Current main was merged
into the probe branch to resolve the handoff conflict after SC1 was squash-merged.
The resulting diff contains only the five SC2 report/fixture/helper/handoff files.
Syntax, fixture, link and whitespace checks passed after resolution; CI on the
resolved head must pass before merging. Preserve all remaining integration gates.

## SC1 — Per-agent structured chat capabilities (31 October 2026)

**Task ID and scope:** SC1, negotiated per-agent chat availability and feature
reporting, descriptor-aware launch/continuation, and tool-bound history ownership.
OpenCode remains the only reviewed structured integration; no new adapter runs.

**Branch / base / PR:** `feat/structured-agent-capabilities`, from main `c77f389`,
[PR #300](https://github.com/eyupio/reet.sh/pull/300), implementation commit `c9f5b52`.
Earlier uncommitted installation/research work remains in its original checkout;
this branch includes only the chat contract, its tests and related documentation.

**Behaviour added:** `acp-agents-v1` negotiates a bounded `acpAgents` list. Entries
report tool, adapter, minimum/observed installation version and
available/missing/unverified/incompatible states. Available features are masked
by negotiated history/images/model/approval support. The connector reads
installation metadata and resolves binaries without executing them. An available
entry still checks executable version and ACP identity/version at launch.

Explicit descriptors are authoritative, including an empty list. Older peers
omit them and retain OpenCode-only fallback. Controller and browser launch,
continuation and feature controls use the selected agent's descriptor. A
connector cannot enable an unreviewed tool by advertising it. Native session ID
validation dispatches by tool, preserving OpenCode's existing namespace, and the
resume query now binds the tool alongside account, machine, folder and closed
session state. There is no schema migration or controller transcript storage.

**Checks and limitations:** protocol/collector/profile, ownership, broker and API
regressions were added, plus launch/continuation unit cases and a descriptor mode
in `chat.e2e.mjs`. Web check/build and both legacy/descriptor chat browser modes
passed, including the launch dialog's desktop/tablet/phone light/dark matrix.
Full `go vet ./...`, all 16 MCP tests, and focused race regressions for launch
refusal, API feature gating and tool/account/machine/folder history binding
passed. The complete `go test -race -timeout=60m ./...` suite also passed;
API took 1,160.6 seconds and store 1,078.2 seconds on this host. The container's
non-reaping PID 1 needs a test-only subreaper for orphan-exit checks, and socket fixtures need
network permission. No real provider, physical phone or new adapter was run.

**Exact next unchecked task:** SC2's Claude Code and Codex ACP adapter probes;
read [the plan](docs/structured-chat-plan.md), `internal/acpagent/{spec,describe,session}.go`,
`internal/terminalproto/acp_agents.go`, `internal/scan/agent_terminal_control.go`
and `web/src/lib/StartAgentDialog.svelte`. Do not enable a candidate before its
callback, permission, trust, provider and history contracts are observed.

**Publication state:** [PR #300](https://github.com/eyupio/reet.sh/pull/300)
merged as `e902b869cecc4b2194a8bc778c8271ec2172c654`.
SC2 remains the separate probe checkpoint in PR #301 with its own handoff.
No release or deployment is performed by these continuation tasks.

## Terminal interface (30 October 2026)

The binary now has a terminal interface: `reet` in a terminal opens a menu over Doctor,
Healthcheck, Scan, Sync, Backups, Setups, Upgrade, Install, Connect machine, Version, Terminal
status, and `--tui` opens a screen directly. Setups also previews and applies a restore and rolls back the
newest completed restore for a profile, all through the same `setup` commands. Scan is a native `huh` form with dry-run, prune and reconcile; Sync
previews a target and applies a hash-checked plan; Backups lists, takes and restores copies through
the shared `backup.RestoreInstance`; Setups creates, pushes and pulls profiles through the shared
`setup.HTTPRemote`; Upgrade runs the same verified, backed-up, rollback-capable upgrade as the text
command behind an explicit confirmation. Install runs the same managed installer behind an explicit
confirmation, with a dry run. Connect machine writes the same enrollment as `reet agent enroll` behind a
confirmation. It is
built with Bubble Tea v2 under `internal/tui`, reads the same report values as the text commands,
and does not touch the store or the web app. v2 removes the v1 startup background query, so a
terminal that does not answer OSC 11 no longer delays every command; the palette comes from
`REET_TUI_BACKGROUND`, `COLORFGBG`, or a dark default. This deliberately moves the "full terminal TUI" note below
ahead of its earlier lower priority; it is scoped to the binary's existing commands, not a second
control room. The planned terminal-interface slices are complete; install, agent enrollment and the longer setup restore/rollback flows remain text commands.

## Live approval modes (31 October 2026)

- **A live structured chat can switch Ask me ↔ YOLO.** New `acp-approval-v1` (`ACPApproval` capability,
  `acp_mode` body) lets the controller change a running chat without restarting it. The connector sets its
  own auto-answer, answers cards already waiting, and keeps the local YOLO switch gate. The controller keeps
  the launch gates (account Allow YOLO, machine `yolo`, agent option), updates `terminal_sessions.yolo`, and
  audits `yolo-mode`; the session header and actions menu carry the switch, with the existing confirmation
  dialog in switching form. Docs: [structured sessions](docs/structured-sessions.md).
- **Ask me can allow a command or file for this session.** A card with a command or path adds
  **Allow this command/file for this session**. The controller remembers the exact tool kind plus command or
  path for that chat, answers repeats itself, audits each as `session-allow`, and forgets on chat end,
  archive or restart. It never sends the agent's `allow_always`. `PermissionChat` takes `forSession`; the
  route body adds `forSession`; the card shows the third button.
- **Checks:** `go test` on terminalproto, acpagent, terminalagent, terminalbroker, store, agenthealth and api;
  `npm --prefix web run check`; `npm --prefix web run build`.

## Agent software inventory and plan (31 October 2026)

**Task ID and scope:** L1, first slice — observe OpenCode on Linux and show a reviewed install, update or remove
plan; nothing is downloaded or executed.

**Branch / commit / PR:** `feat/agent-software-inventory`, commits `ec2a5d3`, `c4be69c`, `42bbded` and `7db0bec`,
[#282](https://github.com/eyupio/reet.sh/pull/282).

**Behaviour added:** a new `software-v1` private-control feature. A negotiating connector reports one
`terminalproto.SoftwareEntry` for OpenCode in its capabilities: state (`installed`, `missing`, `unknown`,
`unsupported`), exact version, package, manager/source evidence, executable path, platform/arch, the plan operations
the evidence supports and `checkedAt`. The connector reads a bounded `package.json` beside the resolved executable
through `setup.InspectInstallations`; it never runs OpenCode, npm or an installer. `GET
/api/v2/machines/{agentId}/software/plan?tool=opencode&action=install|update|remove` builds a plan from stored
evidence (current/target version, exact command, steps, running-session effects, always `executes: false`); the only
reviewed target is the pinned 1.19.5. Machines → Settings → **Software** shows the evidence and the plan inline, with
no Apply action. Read [docs/agent-software.md](docs/agent-software.md).

**Compatibility / migration behaviour:** an older controller does not advertise `software-v1`, so a new connector
sends no inventory; an older connector reports none. No schema migration — the inventory rides in the capabilities
JSON already stored per connection. The plan route reads stored evidence and session rows only and never contacts the
machine.

**Checks actually run and their results:** `npm --prefix web run check` (includes `software.test.mjs`) and
`npm --prefix web run build` passed; `go vet ./...` passed; `go test` passed for `terminalproto`, `terminalhost`,
`agenthealth`, `agentsoftware`, `scan` and the whole `api` package (176 s); `go test -race` passed for the first five
packages and for the software/private-control tests in `api`; `git diff --check` passed. The full `api` race package
timed out at 20 minutes on this shared host (other long Go suites were running), so CI is the gate for it. The
terminal-browser and phone-zoom jobs surfaced two pre-existing timing checks (`skill-content.e2e.mjs`'s records
section and `home.e2e.mjs`'s Start/Opening word), stabilised in `7db0bec` without changing product behaviour.

**Platform or live-environment checks not performed:** no physical machine, real npm run or live OpenCode install
was changed; macOS, Windows, other package managers, other CLIs and any live registry version lookup are out of scope.

**Known limitations or unresolved decisions:** inventory refreshes on reconnect or a local switch change, not on a
timer; 1.19.5 is the only reviewed version and a newer release is not assumed ACP-compatible; execution, rollback
and interruption recovery are not implemented.

**Exact next unchecked task and first files to read:** L2 — verified single-machine maintenance with rollback, still
plan-first. Read `internal/agentsoftware/plan.go`, `internal/terminalhost/software.go`, `internal/api/software.go`,
`internal/scan/agent.go`, `web/src/lib/MachineSettings.svelte` and [docs/agent-software.md](docs/agent-software.md).

**Publication / deployment state (verified, not inferred):** the branch is pushed and PR #282 is open; nothing is
merged, released or deployed.

## Agent software execution (L2, 31 October 2026)

**Owner decisions, 31 October 2026:** (1) use a **verified staged artifact** — download the pinned
`opencode-linux-x64@1.19.5` tarball, verify its recorded sha512 and binary sha256/size, and install from it, never from
an installer script; (2) a pre-existing OpenCode that reet did not install is **inspect-only** — reet installs its own
managed copy alongside and launches prefer it; (3) **refuse** install/update/remove while any live OpenCode session
runs on that machine; (4) the **connector downloads** the artifact from the fixed registry URL (no controller mirror
yet); (5) **any signed-in writer** may apply (the plan binding and audit are the safety boundary); (6) only **install
and update** are exposed in this slice, with automatic restore on a failed verification; (7) **one confirmation
dialog** with the machine, version, source and effects.

**L2a and L2b, on `feat/agent-software-apply` ([#289](https://github.com/eyupio/reet.sh/pull/289), commits `552ed51` and `2742911`, rebased onto `main` after #282 merged):** `internal/agentsoftware/artifact.go` holds the
one reviewed archive (URL, sha512, binary sha256 and size, the single `package/bin/opencode` member) and
`StageTarball`, which downloads it with a bounded response, verifies the archive checksum, extracts only the member
and verifies the binary's hash and size; it never executes anything. `managed.go` owns
`~/.reet/software/<tool>/versions/<version>/opencode` with a per-version receipt and a current receipt, an atomic
install from a staged file, a hash-verified rollback to the kept previous version, and a remove that deletes only the
managed tree (configuration and history live outside it). A system install has no receipt and is never updated or
removed. Tests use a fake tarball and a local HTTP server: checksum failure, wrong member, symlink member, traversal
entries, install/current/rollback/remove, broken binary, no previous version, and a user config file surviving removal.
Checks: `go test -race ./internal/agentsoftware`, `go vet ./internal/agentsoftware`, `go build ./...`.

**L2b, implemented on the same branch:** `software-apply-v1` is a separate negotiated capability, reported only while
terminal access is on. `terminalproto` adds `SoftwareApply`/`SoftwareApplyResult`, `SoftwareEntry.Managed`, the
`managed_broken` reason and `software-apply-v1`. The connector re-inspects the evidence, refuses when it changed,
downloads and verifies the artifact, installs it, verifies the current copy and restores the previous managed version
on failure (`terminalhost.ApplySoftware`); a process-wide slot serialises applies. The broker asks a ready connection
and checks the capability. `POST /api/v2/machines/{agentId}/software/apply` rebuilds the plan, refuses while a live
OpenCode session exists, sends only state/version/managed, merges the fresh entry into stored capabilities, audits
`software-apply`, and maps refusals/failures to codes. The UI shows **Apply** only when the machine reports the
capability, uses one confirmation dialog, and records the fresh entry. Launches, model listing and inventory now
prefer the managed binary (`launchArgv`, `ListModels`, `InspectSoftware`). A system install has no managed receipt and
is never updated or removed. Docs: [docs/agent-software.md](docs/agent-software.md).

**Checks for L2b:** `go test` passed for `terminalproto`, `agentsoftware`, `terminalhost`, `terminalagent`,
`terminalbroker`, `agenthealth`, `scan` and `api`; `go test -race` passed for the first seven and for the
software/private-control tests in `api`; `npm --prefix web run check` and `npm run build` passed; `go vet ./...` and
`git diff --check` passed. The full `api` race package was not run on this loaded host; CI is the gate.

**L2c, on `feat/agent-software-remove-rollback` ([#291](https://github.com/eyupio/reet.sh/pull/291), commit `e9657dd`, stacked on #289):** explicit **rollback** and **remove**, both only
for a reet-managed copy. `SoftwareEntry.Previous` carries the kept version; `SoftwareActionRollback` and
`SoftwareActionRemove` extend the apply request/result; `ApplySoftware` verifies the evidence, calls
`RollbackManaged`/`RemoveManaged`, verifies the result and reports the fresh entry. The controller's plan and apply
routes accept both; remove/rollback are refused while an OpenCode session runs; the UI lists the supported plans
(install/update/rollback/remove) and remove needs the machine's name typed in the confirmation dialog. A system copy is
never touched, and removing takes reet's kept previous versions with its own copy. Tests cover rollback plans,
remove effects, applied/refused results, the API actions and typed confirmation; docs and README are updated.

**L2d, on `feat/agent-software-remove-rollback` (with L2c):** `agentsoftware.RecoverManaged` runs once when the
connector starts. It removes staging files and unfinished version directories, verifies the current binary, restores
the kept previous version when the current is broken (clearing the broken rollback target), clears a previous target
that no longer verifies, and reports a broken current with no valid previous as `managed_broken` rather than hiding it
or falling back to a system copy. Tests cover leftovers, a broken current with a good previous, a broken current with
no previous, an invalid previous, and orphan versions without a current receipt. Docs: [docs/agent-software.md](docs/agent-software.md).

**L2e, on `feat/fleet-software-review` ([#292](https://github.com/eyupio/reet.sh/pull/292), commit `b06585d`, stacked on #291):** a **plan-only fleet review**. `GET
/api/v2/software/plans` builds one row per enrolled machine from stored evidence (version, managed/system/missing,
action, target, source, running sessions, whether the machine can apply, sentence) without contacting a machine.
Machines → **Software** opens the review dialog; rows can be selected locally, the footer says cross-machine apply is
not available yet, and each machine's Settings remains the place to apply. Tests cover the aggregate rows and the
label/warning helpers.

**L2f, on the same branch:** the fleet review can now **apply one machine at a time**. Each actionable row has
**Apply plan** when the machine reports `software-apply-v1` and no OpenCode session runs; it opens one confirmation
dialog and calls the existing per-machine apply endpoint. Other rows are untouched; there is no batch selection. The
row shows the outcome and the list refreshes from stored evidence; rollback/remove stay in Settings. Tests cover the
review label/warning and the new can-apply rule.

**Still not in this slice:** no batch apply or controller-side artifact mirror; the binary is verified at install and at
startup, not on every launch. Next: reviewed batch apply with a pilot order, and the mirror. Read
`internal/agentsoftware/{artifact,managed,plan}.go`, `internal/terminalhost/{software,software_apply}.go`,
`internal/terminalbroker/software.go`, `internal/api/software.go`, `web/src/lib/MachineSettings.svelte` and
`web/src/lib/FleetSoftware.svelte`.

**L3, reviewed batch apply (PR [#314](https://github.com/eyupio/reet.sh/pull/314)):** the fleet review now selects machines into an **ordered batch**. The
controller rebuilds each plan from stored evidence and freezes it; a machine that cannot apply (no plan, a live
OpenCode session, no `software-apply-v1`, already at the target, or held by another active batch) is excluded with its
reason shown. A batch applies one machine at a time in position order and stops at the first failure; a stopped batch
offers **Retry**, **Skip and continue** or **Abandon batch**. A refused plan whose machine already shows the target
version, and a member left running by a controller restart, are reconciled as applied. At most one batch is active per
signed-in person; a machine is in at most one. New durable tables `software_batches` / `software_batch_members`
(`internal/store/store_software.go`), a `softwarerun` orchestrator modelled on `projectrun`
(`internal/softwarerun/run.go`), and routes `POST/GET /api/v2/software/batches…` with retry/skip/abandon
(`internal/api/software.go`). Progress is polled; `FleetSoftware.svelte` carries selection, accessible up/down ordering,
one confirmation sheet and the live batch panel. Audit: `software-batch` at start and each terminal state, plus
`software-batch-retry`, `-skip` and `-abandon`. Tests cover the store guards and transitions, the orchestrator's order,
stop, retry, skip, offline code and reconciliation, and the API routes and audit. Checks run: `go vet ./...`; `go test`
for `store`, `softwarerun` and the whole `api` package (210 s); `npm --prefix web run check` (includes
`software.test.mjs`) and `npm --prefix web run build`. Not run: a live machine apply and the browser e2e suites.
**Still not in this slice:** batching rollback/remove. Read `internal/softwarerun/run.go`,
`internal/store/store_software.go`, `internal/api/software.go` and `web/src/lib/FleetSoftware.svelte`.

**L4, controller-side artifact mirror (same PR):** an opt-in admin setting (`software_mirror_enabled`) and an
Admin → **Software artifact mirror** section (owner/admin) with the toggle, cached size and **Cache the reviewed
artifact now**. `internal/softwaremirror/mirror.go` downloads the pinned archive once on demand (single-flight),
verifies its size, sha256 and sha512 and the extracted binary's sha256 and size with `agentsoftware.CheckTarball`,
then fsyncs and atomically renames it to a content-addressed file; a partial file is never served, and GC at
controller start keeps only the reviewed digest. The archive sha256 and size joined the pinned table
(`internal/agentsoftware/artifact.go`). Machines fetch it over the existing private connection at
`/private/v1/software/artifact/<archive sha256>` (exact-path allowlist, machine credentials, any enrolled machine in
the account, four concurrent serves, exact `Content-Length`); the route 404s when the mirror is off or the digest is
unknown. The connector derives the path from its own pinned table, tries the registry first and the mirror only when
that fails, verifies the same pinned artifact either way, and reports `registry`, `mirror` or `registry+mirror` in the
apply result. Timeouts: registry leg 5 minutes, mirror leg 15, connector apply 20, broker 21, orchestrator and
per-machine API 22. `FleetSoftware.svelte` carries the mirror status line. Tests cover the mirror's cache, single-flight,
corrupt/truncated files and GC; the connector's fallback and both-failed reporting; and the settings, cache action and
private route (enabled, disabled, unknown digest, wrong credential). Checks run: `go vet ./...`; `go test` for
`agentsoftware`, `softwaremirror`, `terminalhost`, `terminalagent`, `store`, `softwarerun` and the whole `api` package;
`npm --prefix web run check` and `npm --prefix web run build`. Not run: a live registry download into a real mirror, a live
machine fallback, or the browser e2e suites. **Publication state (verified, not inferred):** branch
`feat/software-batches-mirror` is pushed and PR #314 is open; nothing is merged, released or deployed. **Next:** the
roadmap's next directions (structured chats across agents, richer run orchestration); batching rollback/remove and
mirror resilience are later follow-ons. Read
`internal/softwaremirror/mirror.go`, `internal/api/software_mirror.go`, `internal/api/private.go`,
`internal/terminalhost/software_apply.go` and `internal/terminalagent/agent.go`.

## Agents home redesign (27–30 October 2026)

### Resume here (after Needs you and the approvals slice, 31 October 2026)

- **Chat images (in progress, branch `cline/6t9b2yrw`):** the reet TUI now passes a pasted, dropped or chosen image to
  OpenCode as embedded image content: the browser uploads it to the machine, `POST /chat/prompt` carries the upload path, and the
  connector embeds the bytes as an ACP image content block (`acp-image-v1`, `acpImages` capability). The agent sees the
  image itself, not a path it must open with a tool. At most eight images per prompt; older connectors keep the path-as-text
  fallback. Unit tests cover the protocol, ACP child content blocks, connector upload-root validation and the API; the mocked
  chat e2e at 1360x900 and 390x844 passes. Not checked: a live OpenCode model turn with an image, or Safari.

**#272 (inline approvals) merged as `362ffd9`; #269 (Needs you) as `52afc9d`;
#270 (reet TUI model choice and provider error recovery) as `1717b7a`;
#266 (YOLO audit follow-up) as `99b3fd0`; #264 (YOLO sessions) as `9cf82f4`;
#261 (reet TUI naming and terminal views) as `329f978`; #260 (Home reet TUI) as `2b35802`;
#258 (reet TUI history, continue, images) as `2ee12f6`; #256 (OpenCode in the reet TUI) as `7555da2`;
#255 (models UI) as `32866ad`; #252 as `3be0eaf`; #247 as `16da0bb`; #245 as `1371cb5`;
#242 as `961d530`; #231 as `7886511`.**

- **Merged PR:** [#269](https://github.com/eyupio/reet.sh/pull/269) implements PR G (`attention-v1`
  terminal activity and bell reporting, per-person `terminal_session_seen` store and `POST /api/v2/terminal/sessions/{id}/seen`,
  attention derivation for both `attention-v1` terminal sessions and structured reet TUI sessions, recovering completed-turn
  attention across controller restarts, gating chat snapshot attention fields behind `attention-v1` feature negotiation,
  coordinating workspace document titles with live attention counts via `titleWithAttention` in `App.svelte`, Home's **Needs you** queue,
  the bell popover, `(N)` document title prefix, polite live-region announcements, and Codex's YOLO preset updated to `--ask-for-approval never`).
  Its final head was green; merged under the owner's exception because the Codex quota was exhausted (recorded on the PR).
- **Merged PR:** [#272](https://github.com/eyupio/reet.sh/pull/272) implements Program 2, Slice 3
  (inline **Allow once** and **Deny** approvals on Home's **Needs you** cards and the header bell popover, backed by
  `TerminalSession.PendingPermission` queueing the earliest pending permission in `ApplyChatAttention`, shortening command
  previews to 240 bytes with `Truncated: true` to require opening chat for full commands, answer plumbing, unit tests, and Playwright e2e tests).
  It was retargeted from `ccr-attention` to `main` after #269 merged and merged under the same exception.
- **Merged PR:** [#270](https://github.com/eyupio/reet.sh/pull/270) added reet TUI model choice and provider-error
  recovery. OpenCode's `stream error` log is surfaced when a provider rejects a model, and the shared model picker lets a
  reet TUI session use a selected model while preserving OpenCode's configured default. F shipped: the account's Allow YOLO setting (`GET`/`PUT
  /api/v2/settings/yolo`, owners and admins change it), the machine's `reet terminal yolo
  on|off|status` switch and `yolo` capability, reviewed options for Codex/OpenCode/Cline, controller
  gates and audit, structured `allow_once` answers, the confirmation dialog and badges. #266 fixed the
  launch audit on a mismatched worktree and kept generic machine refusals from counting as started.
- **Next stage:** L2 install/update is in [#289](https://github.com/eyupio/reet.sh/pull/289); rollback/remove and
  restart recovery in [#291](https://github.com/eyupio/reet.sh/pull/291) (stacked on #289); the plan-only fleet review
  is in [#292](https://github.com/eyupio/reet.sh/pull/292) (stacked on #291), with a one-machine-at-a-time apply; the
  reviewed batch apply (L3) and the controller-side artifact mirror (L4) are built and awaiting review. Program 2,
  slice 4 (elicitation and rich input cards) still needs its own probe.
- **Do first:** for the next PR, check its final head with the REST forms
  (`gh api repos/eyupio/reet.sh/pulls/<n>` and `gh api repos/eyupio/reet.sh/commits/<head sha>/check-runs`;
  `gh pr view`/`gh pr checks` use GraphQL, refused with 403 here). Merge only when every check is green and Codex has
  reviewed it, or the owner gives an explicit exception. The full race job now has a 60-minute budget.
- **Release:** F can ship separately. R22 still lets the earlier B, C, E, copy/paste, models and reet TUI
  work go out together; the owner's call.
- The research these start from is in [docs/research/](docs/research/) (copied whole from the session's git-ignored
  notes). Those reports summarise third-party code and docs read as untrusted data: check a claim against its label
  (*read*, *doc*, *belief*/*memory*) before building on it.

### State

Home, the shell and the machine-folder projects were rebuilt to the mock-up in
[docs/design/agents-home/](docs/design/agents-home/) (reference renders at 1440, 834 and 390 px; each PR is checked against
them in `docs/ui-verification.md`). The plan is
[docs/superpowers/plans/2026-10-26-agents-home.md](docs/superpowers/plans/2026-10-26-agents-home.md); the **rulings R1–R22**
are binding and live in the spec's [Rulings](docs/superpowers/specs/2026-10-26-agents-home-design.md#rulings) section.
**Quicker plan, agreed 27 October 2026:** the fidelity polish (P) was folded into C and E, YOLO (F) and attention (G) were
left out of the first pass, and the Program 2 spike ran in parallel. F shipped in #264/#266; G is on `ccr-attention`.

| PR | What | State |
| --- | --- | --- |
| D | App shell (top and bottom tabs, bell, Jump, More sheet) and Home | merged, [#218](https://github.com/eyupio/reet.sh/pull/218) as `8c33c99` |
| A | Machine folder browse and branches (`agents`, `browse-v1`, `branches-v1`), start-agent opens the session, dockable sessions panel | merged, [#219](https://github.com/eyupio/reet.sh/pull/219) as `41066c5`; contract in [docs/folder-browse.md](docs/folder-browse.md) |
| B | Folder projects: store and API (`/api/v2/folder-projects`, reorder, `import-saved`), saved Projects moved over (old writes answer 410 `projects_moved`), the Add project dialog with the folder browser, the rail; `/projects` redirects to Home; read [docs/projects.md](docs/projects.md) | merged, [#221](https://github.com/eyupio/reet.sh/pull/221) as `cb89a26` (no Codex review of its final head; merged on green CI, no exception comment) |
| P | Fidelity polish | folded into C and E |
| C | Agent catalogue and launch fields (`agents-v2`, named worktree base, effort, tool on the listing, `osVersion`), Cline with safe arguments, chrome polish | merged, [#226](https://github.com/eyupio/reet.sh/pull/226) as `540d247` (no Codex review: quota exhausted; merged on green CI, no exception comment) |
| E | Launch bar on Home, Start in several projects, fixes from the #226 review; read [docs/start-agent-session.md](docs/start-agent-session.md#on-home-the-launch-bar) | merged, [#227](https://github.com/eyupio/reet.sh/pull/227) as `fe3e9b7` (owner exception, recorded on the PR) |
| F | YOLO (off by default, per session, audited, machines can refuse), with the account Allow YOLO setting, machine switch and capability, reviewed preset options, confirmation dialog and badges | merged, [#264](https://github.com/eyupio/reet.sh/pull/264) as `9cf82f4`; follow-up [#266](https://github.com/eyupio/reet.sh/pull/266) as `99b3fd0`; plan `docs/superpowers/plans/2026-10-26-agents-home.md` (Task F) |
| G | Attention (`attention-v1`), per-person seen state (`POST /api/v2/terminal/sessions/{id}/seen`), Home's Needs you queue, bell popover, title count and live-region announcements | merged, [#269](https://github.com/eyupio/reet.sh/pull/269) as `52afc9d` (owner exception: Codex quota, recorded on the PR); plan `docs/superpowers/plans/2026-10-26-agents-home.md` (Task G) |
| Program 2, slice 3 | Inline approvals on Home: answer pending reet TUI command and edit approvals (**Allow once** / **Deny**) directly from Home's **Needs you** column and bell popover | merged, [#272](https://github.com/eyupio/reet.sh/pull/272) as `362ffd9` (retargeted to `main`; owner exception); plan `docs/superpowers/plans/2026-10-26-agents-home.md` |
| H | `/sessions` (the Sessions page), the session screen, tablet and phone layouts, the last redirects, the phone rail on Sessions | merged, [#231](https://github.com/eyupio/reet.sh/pull/231) as `7886511` (owner's decision; no Codex review possible: quota exhausted). H1 `f0bffe5` Sessions page and Control's address retired (`/control` → `/`, `/terminals` → `/sessions`), fixes `048e431` (session colours, **Saved commands** button: owner ruling), `b85338a`; H2 `f74001f` session screen (tabs, header with the state pill, Chat \| Terminal with Chat disabled, context panel), fixes `f026cfb`, `c02fec6`; H3 `7581ad0` tablet and phone layouts (no rail on narrow Home, the rail on Sessions, slim phone header, `keyboardChrome('session')`), fixes `a984b5e` (44 px tablet targets, cases moved to `sessions-phone.e2e.mjs`), `3f1e89b` (separator grip); `521c406` CI fix (first-terminal checklist off `/sync`); merges of main `9c88a46` (#232, #233), `ae763c8` (#234) and `57d8f53` (#235, #236); H4 `1a5018d` (research copied into `docs/research/`), `c349a2b` (the matrix, phone target fixes, docs, this handoff), `1ff4230` (the research notes name the reference tool by role); final fix wave after the broad review `b53c7dd` (`/sessions/`, viewers at a session address), `adec499` (steady Sessions rows, menu focus, the connection in the phone header, tablet swatches and grip, context panel, tab tooltip, New → `/#launch`), `dd31068` (dead code), `8e6ce3e` (checks that went with Control, steadier phone tests), `6778211` (start-agent expects the Machines title) and the docs commit after them |
| Copy/paste | Terminal paste sanitiser and bracket-aware review, copy screen/last lines/all, Select text sheet, copy-on-select, gated OSC 52 (`clipboard-v1`), image attach/paste/drop with `upload-v1` and private per-session storage | merged, [#242](https://github.com/eyupio/reet.sh/pull/242) as `961d530` (owner's instruction; Codex quota exhausted). C1 `81b0e74`, C2 `dc93c39`, C3 `93e7e37`, C4 `7a75e8a`, C5 `f90e1e3`, U1 `64b2240`, U2 `497eda7`, U3 `cb606bc`, fence `1614b44`, U4 `4e9ac2f`, U5 `b209152`, review fixes `81b9e34`, `aa532b6`; the final CodeQL response/headers work and this follow-up are in #245 |
| Copy/paste follow-up | `Exited` cleanup, running 24 h sweep, two concurrent uploads and third refusal at the connector, lost-reply timeout, Scrollback-mid-upload wording, this handoff | merged, [#245](https://github.com/eyupio/reet.sh/pull/245) as `1371cb5` (owner approval; Codex re-review clean) |
| Models backend | `models-v1`, OpenCode listing, local switch, per-machine cache, read/refresh/list API, favourites, account defaults, project effort, Cline project default | merged, [#247](https://github.com/eyupio/reet.sh/pull/247) as `16da0bb` (owner exception; Codex quota). M0 `c1152ff`, M1 `f4bf81c`, M2 `11afb42`, M3 `9d7f234`, M4 `5490a6c`, docs `3cd34f0` (plan `2a54133`); review fixes `56f5f5b`, `39a61a8`, `a8c9310` |
| Models UI (M5–M7) | `models.ts`, model/favourite clients, picker star/status/Refresh/Use, launch bar defaults-first, Home account defaults, automatic machine read | **[#252](https://github.com/eyupio/reet.sh/pull/252), open**. Plan `d802ff0`, M5 `a7e0d7d`, picker `9202695`, bar `ca0f972`, review fixes `67feb99`, evidence `24df6cc` |
| Models UI (M8–M11) | Start an agent picker with project/account defaults and Cline thinking; Add project/Project settings defaults; Settings › Agents; Machines “Agents” cache state and guarded Refresh; Activity/hover-card model lines; docs and phone evidence | merged, [#255](https://github.com/eyupio/reet.sh/pull/255) as `32866ad` (owner's instruction; Codex reviewed an earlier head, the runner timed out on the final head and a rerun passed). Plan `docs/superpowers/plans/2026-10-29-agent-models-ui-2.md` |
| Program 2, slice 1 | OpenCode in the reet TUI: `acp-v1`, machine-side ACP supervisor with the seven probe conditions, controller relay and SSE, reet TUI, Allow once / Deny, docs | merged, [#256](https://github.com/eyupio/reet.sh/pull/256) as `7555da2` (owner's instruction; Codex findings fixed, all checks green) |
| Program 2, slice 2 | Reopen a closed reet session with `session/load` history, continue it, and attach images as machine paths | merged, [#258](https://github.com/eyupio/reet.sh/pull/258) as `2ee12f6` (Codex mixed-version finding fixed with `acp-history-v1`; all checks green) |
| Home OpenCode reet TUI | OpenCode on an `acp` machine starts in the reet TUI from Home's launch bar; the TUI word switches to the agent TUI | merged, [#261](https://github.com/eyupio/reet.sh/pull/261) as `329f978` (renamed reet TUI/OpenCode TUI, trust, Creating reconciliation, terminal views) |

Merged into main by the owner while this work was open: #222, #223 (`4791d89`), #224, #225, #228, #229, #230, #232, #233,
#234, #235 and #236 (each merged into the open branch before its next push; #235 and #236 in `57d8f53`), then #237–#244
and the Sync follow-ups while #242 was open (merged into it before each push). #242 then merged as `961d530`; #245 as
`1371cb5`; #246 and later main work landed while the models backend was open.

**Merge rule:** a PR in this work merges only when all checks are green on its final head **and** Codex has reviewed it
cleanly, or the owner gives an explicit exception. So far: #227's exception is recorded on the PR (a comment); #221 and
#226 merged on green CI without a Codex review and without an exception comment; #231 merged on the owner's decision
after green checks; #242 merged on the owner's 28 October instruction to create it and merge when green; #245 merged
on the owner's approval of the follow-up recommendation with a clean Codex re-review; #247 merged as `16da0bb` under the
owner's merge instruction; #252 and #255 merged on the owner's instruction to merge when green; #255's final head passed
after a terminal-browser rerun. CodeQL alert #20 on #242 was dismissed as a false positive (the
machine path is validated plain text, URL-escaped into response headers, and never in the JSON body; JSON plus nosniff).

**Rulings beyond R1–R22** (all recorded in the session notes and kept here so they survive):

- Owner, 27 Oct: the quicker plan above; Pi stays disabled; Cursor stays blocked until its installer host is allowed.
- Owner, 28 Oct: a **Saved commands** button on the Sessions page; copy and paste is its own PR after H, with pasted
  images in a private per-session folder (0700/0600, random names); Program 2 slice 1 is trusted workspaces only (OpenCode,
  Allow once / Deny only) behind the seven probe conditions; the order after H is copy/paste, models, Program 2; pause after
  PR H. Later that day: create the copy/paste PR and merge it when green. Copy/paste is #242 (merged as `961d530`);
  its image path is text only (per-agent attachment behaviour unverified). Then approve the follow-up #245 and start
  the models backend. 29 Oct: the models backend is #247; the web picker follows it as UI-1 (#252, M5–M7) and UI-2
  (M8–M11). The owner instructed: keep going, create PRs and merge when green.
- Controller defaults, 28 Oct (owner may revisit): models D2 a per-machine default is deferred, D3 a favourite can be the
  "default for this agent", D4 the model-name rule stays as it is.

### Limits and known gaps

After the copy/paste PR:

- Pasted images reach an agent as a **path**, not as an attachment reet creates. The UI and docs say so. Per-agent
  image-path behaviour at the pinned CLI versions has not been checked; do not claim one before checking it.
- Uploads are a new `upload-v1` feature and are on by default; `reet terminal uploads off` refuses them and the browser
  shows the sentence. An older controller never negotiates the token and never sends one. The machine stores at most 20
  images / 50 MiB per session, sweeps files older than 24 h (at start and about hourly), and removes the folder when the
  session ends. The controller audits machine, host, tool, MIME and a size bucket only.
- OSC 52 forwarding needs `clipboard-v1` and `reet terminal clipboard on`; `allow-passthrough` stays off. The browser
  still writes only after a gesture or **Copy it**, write-only, at most 1 MiB. Old browsers ignore the sequence.
- Image uploads write to the machine's private terminal directory, outside the project. They are derived state, not in
  the registry. A machine that ends a session mid-upload may make that one upload fail; nothing is retried.

After the models UI:

- Only OpenCode reports a machine-owned model list. Favourites, reet's suggestions and Other… are offered for every
  agent; they are suggestions, not proof that a model exists. A per-machine default stays deferred (D2); the account
  and project defaults are per account/project.
- **Refresh** reads the machine only when it is online and models-capable. Offline shows the last list with its age and
  disables Refresh; a failed refresh keeps the last good list and age. A cache read never contacts a machine.
- No real provider-backed OpenCode list was used; the tests use fake machines and synthetic API answers. A 500-model
  list was not load-tested, and no physical phone, Safari or live agent CLI run was part of this PR.
- The new terminal controls have no mock-up element. They join the existing toolbar, menu and banner styles; at 1440
  the toolbar scrolls as it did before. The Select text sheet, upload chip, copy-on-select checkbox and the uploads-off
  note are the additions listed in [docs/ui-verification.md](docs/ui-verification.md).
- `terminal-clipboard.e2e.mjs` and `terminal-upload.e2e.mjs` are in the `phone-dialogs` CI job. Nothing new was added to
  `terminal-browser`. The local matrix's session screen passes 40 states.
- Still true from PR H: the session screen keeps the sessions panel and workspace toolbar (the mock-up has neither); the
  tabs share the toolbar row and the pane keeps its own header. Old `/sessions` links on a rolled-back server still land
  on `/` or an unknown page. Needs you now shows real cards (#269); before that PR it was an honest empty state. The site footer and phone terminal chrome keep their
  documented 40 px targets. Nothing has been checked on a real phone or in Safari.

After the models backend:

- Only OpenCode has a machine-read list. Claude Code, Codex, Gemini CLI and Cline keep reet's static suggestions,
  favourites and Other…; the web picker is not in #247, so the browser does not show a machine list yet. The
  favourites/defaults API is likewise not wired to a page.
- A listed model is a suggestion, never proof it works, and it never blocks a launch. Env-only OpenCode providers
  are missing because the listing environment is an allowlist; say so when the picker lands.
- The cache keeps the last good list through a failed refresh and is deleted with the machine. A first read is
  synchronous only for an online machine that reports `models`; refresh is 6/min per account and machine and is the
  only audited path. A failure is visible as its own state, not as an empty successful list.
- The account defaults are for sessions outside a project; the browser resolves explicit > project > account >
  agent default. A per-machine default remains deferred (D2). A project default model still needs a project agent;
  an existing profile model without an agent stays readable and does not block unrelated edits.
- No real provider or paid model was used in the backend tests; the listing runner uses fake binaries, the API uses
  synthetic machines, and a physical phone/Safari was not checked.

After the models picker/launch bar:

- The launch bar now opens on the resolved defaults, not the last launch; the last models are a Recent group. The
  model slot has a star, filter, Use ‘x’ and a status/Refresh line; tablet/phone keep the model in the Change sheet.
- Only OpenCode has a machine list; other agents use favourites/suggestions/Other. The account defaults and
  favourite manager are under Account → Agents; the Start dialog, Add project/Project settings, Machines → Settings
  → Agents, Activity and the hover card are in #255. A per-machine default remains deferred.
- Home does not load favourites itself; the bar does (a recorded plan deviation). A model read failure never
  blocks Start. The real cached-offline shape is overlaid in the bar, not by the API.
- No physical phone/Safari; a 500-model list under a slow reader is not load-tested. The picker/star additions are
  not in the drawings; `docs/ui-verification.md` lists them.

After the reet TUI slices:

- **Home starts OpenCode in the reet TUI by default.** The launch sentence shows **in reet TUI** for OpenCode on an
  `acp` machine; the **TUI** word switches to **in OpenCode TUI**. The reet TUI runs in the project folder, not a
  worktree, and uses OpenCode's configured model. Home requires **I trust this folder** before Start, because `.opencode`
  plugins run when the reet TUI starts. A multi-project selection must switch to the agent TUI; it starts in one folder.
  A session's actions menu offers **Open a terminal here…**, a plain shell in the same folder as a separate session; it
  needs `shell-dir-v1` on the machine. **Open a terminal beside this session…** shows that plain shell in the right-hand
  panel, replacing the context panel while it is open; Close ends it. The agent TUI/reet TUI stays in the main column.
- **reet TUI history is bounded and kept.** The controller keeps at most 1,000 events / 256 KiB per reet session in SQLite,
  and a controller restart restores it; archiving drops it. A closed reet session can be reopened with Continue in reet TUI
  with history… as a new row that runs `session/load`; the old row stays. A control connection loss detaches chats without
  killing them; the next connection adopts the running process, reconciles it as running, and drains the bounded queue of
  events produced while detached. A connector restart or reboot reopens the recorded chats from OpenCode's store. Pending
  approvals are not restored; replayed tools are shown interrupted.
- **OpenCode 1.19.5 or newer.** The connector refuses an older version before starting a process; the ACP handshake must
  also name `OpenCode` and speak protocol 1. The seven conditions in
  [structured-sessions.md](docs/structured-sessions.md) are mandatory; a repository with `.opencode` plugins needs the
  Start dialog's trust confirmation. Always allow is never offered, and a diff or command too large for one control frame
  cannot be allowed.
- **No live model turn** was sent in the checks. The real 1.19.5 binary opened and then loaded a session in a throwaway
  HOME; the protocol tests use a fake ACP child. Modes, MCP servers and other agents are not in this slice. No physical
  phone, Safari or Firefox was checked.
- **Images are paths.** reet TUI attachments use the launch-upload folder and the prompt names the file; no image bytes are
  embedded in ACP. The 5 MiB / 20 files / 50 MiB limits and 24-hour sweep apply. The reet TUI's model is OpenCode's configured
  model; the picker is hidden. The process can still install `@opencode-ai/plugin` from npm in the background.

Before this PR:

- The session screen keeps the sessions panel and the workspace toolbar (the mock-up has neither); the tabs share the
  toolbar row, and the pane keeps its own header (some information is shown twice). A later PR may drop the panel there.
- Old `/sessions` links and sign-in resumes that reach an older server (a rollback) land on `/` or on an unknown page:
  PR H changed no Go protocol and nothing redirects `/sessions` there.
- `/sessions` with `session`, `agent` or `view=terminal` in the address is the workspace; bare `/sessions` is the list.
  The app has no client-side router: links are full page loads.
- Sessions outside a project are grouped "Other on <machine>" only (the listing has no folder to match a project by).
  The **Needs you** filter on `/sessions` matches sessions currently in `needsYou`. Check and remove worktree is offered only when the worktree path and
  branch name the run.
- `railState` puts "N running" ahead of "machine offline", so a project on a machine that is not reporting shows its
  last known count without saying so.
- Two sentences the spec does not word: **Start in these on Home** and "Projects now live on the Sessions screen."
- The site footer (account name, Admin, the developer credit) has targets under 44 px on touch; it predates the agents
  home and was left as it is.
- On a touch phone the terminal's chrome (workspace toolbar, pane header and footer, terminal keys) is 40 × 40 px, not
  44, so the terminal keeps its rows; documented in [docs/terminal-workspace.md](docs/terminal-workspace.md#phones). H4's
  40 px footer button costs one row with the keyboard down at 390 × 844 (39 → 38); keyboard-open and sideways rows are
  unchanged.
- `terminal-phone.e2e.mjs`'s 20 × 8 floor case used to read 13 rows in about one run in five: the workspace height was
  measured while the shell held the top bar aside and not measured again when it came back. Fixed and hardened in
  `5abe001` (the workspace measures again whenever the shell toggles its chrome; the test waits for the layout to
  settle instead of a fixed 450 ms); 10 runs in a row and one under CPU load pass, with the same figures.

Still true from earlier PRs:

- YOLO is live behind the account setting, the machine's acceptance, a reviewed agent option and a per-session
  confirmation. Claude Code and Gemini have no captured YOLO option, so they refuse it. The first line of a prompt
  becomes the stored session title. The Start an agent dialog now offers Cline on an agents-v2 machine; Pi and Cursor
  stay disabled.
- A base a member only inherits from the account's default is not sent to a machine without `agents-v2`; a project's
  own base is sent, and such a machine refuses it with "Update reet on that machine…". Rolling back to an older server
  while a start in several projects has members pending may fail those members or start them without base or effort.
- A replayed start is checked again against today's rules; a start in several projects checks "one start per folder" by
  machine and folder.
- The old Projects tables and their start/retry routes stay one release before they are dropped; a downgrade then
  re-upgrade leaves Projects saved while downgraded as "waiting folders"; a session's `projectId` is checked for account
  and machine, not folder; the import's answer still says "saved repositories".
- Worktree git calls share the lazy-fetch and fsmonitor hazard the folder browser already guards against.
- Nothing has been checked on a real phone or in Safari (browser emulation only), including the touch long press.
- The tablet shows its own **Change** button for model and worktree (approved deviation). Per-screen differences from
  the drawings are listed per PR in [docs/ui-verification.md](docs/ui-verification.md).

### Queued work (decided; next up)

**1. Verified agent maintenance (L2).** L1 inventory and plan-only maintenance for OpenCode on Linux is in
[#282](https://github.com/eyupio/reet.sh/pull/282) (see the section above). Next: execute a reviewed install, update
or remove with a staged/verified artifact or a specifically supported package-manager adapter, an explicit decision
about running sessions, a journal/rollback route and verification of the resulting version. No generic curl-to-shell
installer or arbitrary controller shell command.

**2. Program 2, slice 3: Inline approval cards in Home's Needs you column.** Merged in
[PR #272](https://github.com/eyupio/reet.sh/pull/272) as `362ffd9` (retargeted to `main`; owner exception because the
Codex quota was exhausted). It surfaces pending reet TUI permission
requests (`permission` events with `permissionId`, `title`, `command`, `diff`, `truncated`) on `GET /api/v2/terminal/sessions`
so Home's **Needs you** column and the bell popover render the command/edit approval cards with inline **Allow once** and
**Deny** actions (`POST /api/v2/terminal/sessions/{id}/chat/permissions/{permissionId}`) alongside **Open**. Slices 1 and
2 are merged in #256 and #258; their seven conditions and limits remain binding:
[docs/research/probe-report.md](docs/research/probe-report.md),
[reference-tool-frontend-research.md](docs/research/reference-tool-frontend-research.md) and
[reference-tool-agents-research.md](docs/research/reference-tool-agents-research.md). Owner decision: trusted workspaces only, OpenCode, Allow
once / Deny only, and only with all seven conditions:

1. A reet-owned ask-everywhere config in `OPENCODE_CONFIG_CONTENT` (edit, bash and external_directory ask, for every
   built-in agent; `default_agent: "build"`; `task: deny`; `webfetch: deny`), `OPENCODE_DISABLE_PROJECT_CONFIG=1`, and the environment
   scrubbed of `OPENCODE_PERMISSION`, `OPENCODE_CONFIG`, `OPENCODE_CONFIG_DIR` (and any `OPENCODE_*` reet does not set);
   check the mode is `build` after `session/new`.
2. A random `OPENCODE_SERVER_PASSWORD` per process: every `opencode acp` starts an unauthenticated local HTTP API on
   127.0.0.1:4096 that can answer permissions behind the client's back.
3. `task: deny` (subagent permissions never reach the protocol).
4. A permission timeout and a turn watchdog, with reet closing its own pending cards on cancel.
5. An environment allowlist and the opt-outs (models fetch, autoupdate, default plugins, LSP download and the rest).
6. A warning when the repository has `.opencode` plugins (repository plugins run at session start).
7. OpenCode pinned at exactly `opencode-ai@1.19.5` (integrity in the probe report). There is no `--standalone` or
   `--auto` on `acp` in 1.19.5.

**The probe's blocking findings** (full text: [probe-report.md](docs/research/probe-report.md), sections 4, 6 and 7):

- By default OpenCode asks nothing: writes, edits and shell commands ran without a permission request.
- A repository's config, the user's config or `OPENCODE_PERMISSION` can switch asking off.
- Every ACP process serves an unauthenticated control API on a predictable local port; an approval over it leaves the
  ACP card pending.
- Repository plugin code runs when a session opens, whatever the flags: no-go for untrusted repositories.
- No permission timeout; subagent permissions never reach the client, so a turn can hang; provider errors take ~74 s.
- "Always allow" stores a wildcard broader than the command shown; writes land in the user's `.git`.
- It contacts npmjs for `@opencode-ai/plugin` even with the opt-outs set.

**What not to copy from the other front end studied** (full lists: section 5 of each `reference-tool-*` report):

- Answering terminal prompts by sending keystrokes; regex status rules; writing hooks or trust into users' agent configs.
- Reading or writing agents' private stores (session databases, transcripts); use the protocol's load and list or nothing.
- YOLO that disables the agent's own sandbox, or set through an environment variable that `ps` cannot show.
- Forwarding `SSH_AUTH_SOCK`, `DISPLAY` or DBus to agents; `npm install -g` from a web button; `@latest` hints.
- Pasted images written into the worktree and cleaned by an in-memory timer.
- A `--help` probe on every launch as the main evidence (keep reet's captured help with integrity; add a `--version`
  check only).
- Breadth before depth (seventeen agents); unauthenticated modes and tokens in URLs.

### Pitfalls for the next session

- **CI** runs on the Zoomies runners (`zoomies-linux-x64`); tmux and Chromium are installed per job.
  - `terminal-browser` takes 17–25 of its 30 minutes: **add no fixtures or cases to that job**.
  - `phone-zoom` and `phone-dialogs` (10 minutes each) have headroom; new browser cases go there.
  - CI's Chromium (1248) has no DejaVu Sans Mono, so terminal rows differ from a local run: never assert an absolute row
    count; compare with a layout measured in the same browser.
  - The real-browser Go harness needs `LANG=C.UTF-8 LC_ALL=C.UTF-8` locally, or terminal.e2e's UTF-8 split check fails.
  - CI does not run workflows while a PR conflicts with main.
  - `home.e2e.mjs` opens the `details.home-detail` sections after each navigation; the old In flight tests were
    removed when #288 replaced the component with the SessionBoard, which `session-board.e2e.mjs` covers.
  - `mcp/test/oauth.test.mjs` needs its fixture built first in CI (the warm-up step in `image.yml`).
- **Merges:** the owner merges other PRs while one is open (#228–#241 during this work). Always `git fetch origin main`
  and merge it before pushing.
- **Git and processes:** never `git checkout -p`; commit with pathspecs; never `pkill -f` (it can match the session's own
  processes); never a shared tmux `kill-server`. The stop hook nags while subagents are still working; that is expected.
- **Commit trailers:** new commits follow the session's attribution reminder (`Co-Authored-By` and `Claude-Session`);
  every commit in this PR carries Claude Sonnet 5.5.
- **Upload wire**: the machine reads exactly the declared size and then returns one `UploadResult` frame; the controller
  waits for that frame before closing, because a websocket peer may not be able to write after a close. The host reserves
  quota under `uploadMu` and releases it while streaming; cleanup never takes `uploadMu`, or an ended session stalls the
  connector. The 24 h sweep runs at start and hourly while `Run` is alive.
- **New UI fixtures** go in `phone-dialogs`; `terminal-clipboard.e2e.mjs` and `terminal-upload.e2e.mjs` were added there.
  Run them plus `phone-dialogs`, `phone-zoom`, `sessions`, `sessions-phone` and the `terminal-browser` files in workflow
  order before the final push. The local matrix session screen is `REET_MATRIX_ONLY=session`.
- **Models listing**: only connector-owned argv, a fresh temp cwd, the launch allowlist minus `TERM`/`SHELL`, the
  fetch/autoupdate opt-outs, a 10 s process-group kill and 64 KiB stdout. Never run `--refresh` (it ignores the
  fetch opt-out) and never forward stderr. The controller caches the last good list; a failed refresh must not
  erase it. The first read is synchronous only for a machine that reports the capability.
- **Models UI**: the launch/e2e fixture must answer `/api/v2/profile/model-favourites` and
  `/api/v2/machines/<id>/models` (the bar reads both without blocking Start); `phone-zoom`/`phone-dialogs` use
  `helpers/longtext.mjs` and need the same routes. The picker's filter is pinned and the sheet body scrolls; keep it
  that way for 500 models. Star focus must be restored after the row moves groups.
- **Mixed versions:** `acpSessionId`, `resumeAcp` and the `acpHistory` capability are sent only after
  `acp-history-v1`. A slice-1 controller negotiated `acp-v1` but its strict decoder rejects those fields; never send them
  without the separate feature. `Continue in reet TUI` is offered only when the machine reports `acpHistory`.
- **reet TUI history:** a closed reet session is reopened as a **new** row with `resumeAcp`; the machine runs `session/load` and
  replays before its result. Store the `acpSessionId`, never a transcript. `CanResumeACPSessionFor` restricts the id to
  this account's closed structured sessions on that machine. Replayed pending tools are marked interrupted; pending
  approvals are not restored. The old row stays in history.
- **reet TUI:** the machine-side `internal/acpagent` starts `opencode acp` at or above `1.19.5` with
  `OPENCODE_CONFIG_CONTENT`, `OPENCODE_DISABLE_PROJECT_CONFIG=1`, a random `OPENCODE_SERVER_PASSWORD`, `task: deny` and
  the environment allowlist. It answers `fs/write_text_file` with success without writing (OpenCode already wrote the
  file), checks mode `build`, offers `once`/`reject` only, closes its own cards on cancel/timeout, and kills the process
  group on end. Do not forward raw ACP JSON to the browser; map it to `ACPEvent` and keep it inside the control frame.
- **reet TUI transport:** events are a bounded ring in `internal/terminalbroker/chat.go` and SSE at
  `/api/v2/terminal/sessions/{id}/chat`; prompts, cancel and approvals are POSTs. A browser reconnect resets and replays
  the ring. A structured session is an ordinary row with `structured=1`; it has no terminal, so do not attach to it.
- **Process used** (subagent-driven development): an implementer per task with a brief, a task review, fix rounds
  (at most five) with a scoped re-review, then a final broad review split by area and a single fix wave; mutation checks
  restore files byte-identical (`cp` before, `cmp` after); fidelity checks at 1440, 834 and 390 px recorded in
  `docs/ui-verification.md`. The full local matrix is `web/tests/matrix/agents-home.matrix.mjs` (not in CI).

**Rule for this work: every PR updates this block (state table, limits, queued work) before it merges.**

### Standing notes carried over from the moved sections

- Terminal security: a later, user-agreed security pass for privilege-capable browser terminals (a PIN
  or similar preference) is deferred; nothing of it is implemented. Read "Later: user-controlled
  terminal security preferences" in [handoff-history.md](handoff-history.md) before starting it.
- A reported terminal-stop incident (Stop/End with removal interrupting other tmux work) is unresolved;
  do not disturb the user's work or ask them to reproduce it. Never use a shared or default tmux
  `kill-server`. Details and what to collect if it recurs: "Reported terminal-stop incident" in
  [handoff-history.md](handoff-history.md).
- Shared dialog shell for phones: `web/src/lib/dialogShell.ts` and `web/src/dialog-shell.css`; how to
  adopt it is in "Phones: dialogs" in [handoff-history.md](handoff-history.md).
- Product acceptance and evidence boundaries: discovery is not install capability, shell persistence
  is not conversation recovery, configuration restore is not runtime health, and local checks are not
  a deployment; describe outcomes without competitor names (full text in the history).

## Code map: read before proposing new types or endpoints

| Area | Existing entry points |
| --- | --- |
| App/routes and the shell | `web/src/App.svelte`, `web/src/lib/shell/*` (AppShell, TopTabs, BottomTabs, MoreSheet, JumpSearch, Bell) |
| Home and projects | `web/src/lib/home/*` (Home, FleetStrip, InFlight, NeedsYou, ProjectRail, ProjectRailHost, AddProjectDialog, FolderBrowser), `web/src/lib/home.ts` |
| Launch bar | `web/src/lib/launch/LaunchBar.svelte`, `SlotPicker.svelte`, `web/src/lib/launchBar.ts`, `web/src/lib/catalogue.ts` |
| Sessions page and session screen | `web/src/lib/sessions/*` (SessionsPage, SessionScreen, SessionTabs, ContextPanel), `web/src/lib/sessions.ts` |
| Machines/onboarding | `Machines.svelte`, `MachineFleet.svelte`, `MachineSetup.svelte`, `machineSetup.ts`, `machines.ts` under `web/src/lib/` |
| Sync/fleet UI | `EnvironmentSync.svelte`, `SyncControl.svelte`, `FleetActions.svelte`, `fleet.ts`, `api.ts` under `web/src/lib/` |
| Live sync domain/execution | `internal/scan/sync.go`, `agent.go`, `restore.go`; adjacent tests |
| Desired sets/jobs/receipts | `internal/store/store_sync.go`, `store_sync_control.go`; `internal/api/sync.go`, `sync_control.go` |
| Store/migrations and route registration | `internal/store/store.go`, `internal/api/api.go`; inspect feature-local initialisation too |
| Hosted bundle validation/editor | `internal/store/store_hosted.go`, `internal/api/hosted.go`, `web/src/lib/Hosting.svelte` |
| Read-only content manifests (S1) | `internal/skillbundle`, [contract](docs/skill-bundle-manifests.md); independent of live sync/snapshot formats |
| Snapshot whole-skill collection/verification | `internal/setup/skills.go`, `skills_verify.go`, `snapshot.go`, `restore_apply.go`, `rollback.go` |
| Agent adapters/discovery/evidence | `internal/setup/adapters.go`, `discovery.go`, `installation.go`, `health.go`, config/policy files and tests |
| Setup profiles | `internal/setup/profile.go`, `internal/profilehistory`, `internal/store/store_setup_profiles.go`, `internal/api/setup_profiles.go` |
| Host shell registry and services | `internal/terminalhost/manager.go`, `manager_tmux.go`, `registry.go`, `service.go`, `readiness.go` |
| Typed private transport and broker | `internal/terminalproto`, `internal/terminalagent`, `internal/terminalbroker`, `internal/privateconn` |
| Terminal API and records | `internal/api/terminal.go`, `internal/store/store_terminal.go` |
| Terminal UI and connection state | `web/src/lib/terminal/TerminalWorkspace.svelte`, `TerminalPane.svelte`, `api.ts`, `connection.ts` |
| OpenCode in the reet TUI | `internal/acpagent` (agent spec, process, config, env, permissions), `internal/terminalagent/chat.go` (dispatch), `internal/terminalbroker/chat.go` (ring/commands), `internal/api/terminal_chat.go` (SSE and POSTs), `web/src/lib/chat.ts` and `terminal/ChatPane.svelte` |
| Controller enablement | `cmd/reet/terminal.go`, `internal/terminalcontrol`, `docs/terminal-controller-commands.md` |
| Machine health | `internal/agenthealth`, `internal/store/store_remote_health.go`, `internal/api/health.go` |
| Notifications | `internal/notify`, `internal/store/store_notify.go`, `internal/api/notifications.go`, `web/src/lib/Notifications.svelte` |
| Agents home fixtures | `web/tests/home.e2e.mjs`, `launch.e2e.mjs`, `add-project.e2e.mjs`, `sessions.e2e.mjs`, `sessions-phone.e2e.mjs`; the local matrix `web/tests/matrix/agents-home.matrix.mjs` |
| Existing regression fixtures | `internal/store/store_sync_control_test.go`, `web/src/lib/fleet.test.mjs`, `web/tests/control.e2e.mjs`, `web/tests/terminal.test.mjs`, `web/tests/terminal.e2e.mjs`, `internal/api/terminal_integration_test.go` |

Names such as `Skill`, `SyncSkill`, `SyncTarget`, `SyncPlan`, `SetupSkill`,
`SnapshotSkill`, `HostedSkill`, `TerminalSession` and `SyncAgent` already exist.
Read their actual contracts before adding a similarly named concept. Especially,
`SyncPlan.Hash()` hashes the planned actions; it is not an installed-content hash.

## Safety and conflict handling

- Restore verification uses format-specific limits: configurations 1 MiB, whole
  skill folders 8 MiB, supported memory databases 64 MiB. Capture/restore agree.
- An action is marked as written before verification, so a verification or journal
  persistence failure includes the current action in rollback. Write errors propagate.
- Codex memory restore/verification uses the destination's effective CODEX_HOME.
  SQLite sidecars are rechecked immediately before writes. Destination agents must
  be stopped; sidecar absence does not prove an application is inactive.
- Memory verification continues after a blocked database. Each skill produces one
  final result; an unsafe file cannot also make the skill count as verified.
- The restore wizard prints mapped paths and warnings, asks per-destination conflict
  choices and confirms the final plan. Incomplete file verification returns an error, including the standalone verify command; JSON reports are still emitted.
- Server profile writes require an identical history or an exact append, with an
  atomic payload comparison protecting concurrent writers. Stale/divergent pushes
  return 409. Web edits carry the viewed base revision; stale edits are rejected.
- Local pulls preserve divergent/newer local histories. Preserve a local copy with
  `reet setup profile rename NAME NAME-local`, then pull the server copy and review.
  Local saves/pulls/rename/remove share an exclusive `profiles/.write.lock`.
  Remove a stale lock only after confirming its owning process has stopped.
- A failed profile deletion can be retried without reloading the application.

## Installation and release changes

The shell installer verifies checksums, runs the staged candidate's version before
replacement, preserves a regular existing binary as `reet.previous`, and reports
PATH shadowing. It refuses an existing symlink rather than silently replacing it.
Existing data and managed deployment files remain separate.

Dev publication keeps legacy rolling names for existing clients, and also uploads
commit-addressed archives `reet_dev_SHA_OS_ARCH.tar.gz` with `checksums-SHA.txt`.
Those assets are checked on a rerun and never overwritten. Only after they are
available does the publisher update `dev-build.txt`. New shell installers and Go
upgrades resolve that pointer once and download the corresponding immutable pair.
Only a missing (404) pointer falls back to legacy releases; other errors fail closed.
No assets, tags, deployments or production hosts are published/upgraded by local tests.


## Accepted product direction and priority

The user wants reet.sh to be the central control unit for multiple machines:
maintain skills on reet, connect machines, import local skills, distribute to one,
selected, or all machines, review changes in both directions, and use installed
AI agents through the web terminal. Agent software install/update/remove and
cross-machine configuration management are future requirements. The user accepted
the central-control roadmap on 24 October 2026 and requested this detailed handoff.
This records the accepted direction; this turn authorises the handoff update,
not implementation of every milestone or production changes. Future sessions
should implement the task requested within this direction, and verify its gate.
Later steering prioritises the terminal workspace as the main everyday use case.

Priorities, in order (terminal UX moved first by the user on 24 October):

1. **T — Make terminals the primary usable workspace:** viewport layout, clear
   session navigation/controls and safe history maintenance (T1–T3). T1–T9 are
   merged (see handoff-history.md).
2. **S — Complete the skill foundation:** versioned bundles, content-level drift,
   reviewed local edits, safe one/many-machine transfer and verification.
3. **A — Make remote work convenient:** registered agent capabilities, project
   selection, explicit launch, semantic activity and an attention queue.
4. **W — Make parallel work reviewable:** managed worktrees, files/diffs and
   structured agent interaction where supported.
5. **L — Fleet lifecycle:** observed agent versions, reviewed install/update/remove,
   desired MCP configuration and optional isolated agent sessions.

Conversation resume/fork, granular machine/project permissions, read-only terminal
observation and OIDC login are valuable later extensions. A full terminal TUI,
themes, sound effects and a broad plugin marketplace are lower priority than the
outcomes above. Keep the warm Reet branding, Yorkshire rose and familiar
navigation; read [voice and copy](docs/voice-and-copy.md) for user-facing changes.

The older setup plan records an earlier delivery order (backup/restore, profiles,
maintenance). Preserve its data-safety and recovery requirements. This roadmap is
the newer ordering for the central-control product, with terminal UX now first;
do not attempt complete setup recovery before the bounded workspace/content tasks. Do not label partial file
recovery as a fully working recovered agent environment.

## Execution rules for the next session

1. Read this file, CONTRIBUTING.md, applicable AGENTS.md files, and the entry-point
   files for the single task selected below. Check the actual checkout; paths and
   branch state here are a dated baseline, not permission to reset local work.
2. Run `git status --short`, inspect the current branch/log and fetch the current
   base when network/authentication are available. Preserve existing edits.
   Confirm the previous slice's state through its live PR/CI/merge record, then start
   from current main and preserve local edits.
3. Pick **one bounded next slice**: the open agents-home work above, or from the
   continuation lists in the S4 records (now in handoff-history.md) and
   docs/skill-content-receipts.md; the user chooses the order. T1–T9, S1–S3 and the
   S4 planner, pins, preview, ownership receipts and review UI are done; do not restart them. Do not build an execution
   control before its prerequisite is present or turn the whole roadmap into one PR.
4. Before editing, state the task ID, files affected, expected behaviour and checks.
   Resolve routine implementation choices from existing conventions. If a domain
   decision genuinely remains open, record the question and continue independent
   work; never invent an unsafe default just to make a control appear functional.
5. Keep domain code separate from HTTP handlers, SQLite records and Svelte views.
   Reuse existing parsing, bounds, ownership, locks and recovery code where its
   contract fits. Do not copy a whole subsystem or introduce a dependency casually.
6. Add meaningful regression tests for new runtime behaviour and failures. Use
   synthetic homes, databases and fixtures; do not enroll or alter real machines
   or invoke a real paid AI provider as a test side effect.
7. Show observed/unknown/stale/incomplete/unsupported states distinctly. A request
   accepted by the controller is not an observed successful change on the host.
8. Update this file with completed task IDs, commit/PR, checks, limitations and the
   exact next task. Keep the stage-per-PR workflow and required CI. Do not merge,
   publish a release or change production merely because local tests pass.

### Terms that must not be conflated

| Term | Meaning |
| --- | --- |
| Reet connector / `reet agent` | Background machine enrollment, reporting and command transport; not Claude Code or Codex |
| AI agent application | A CLI such as Claude Code, Codex or OpenCode |
| Terminal session | A persistent shell; `running` does not mean an AI agent is working |
| Agent run | Proposed record binding a terminal/runtime to a registered AI agent, machine and project |
| Skill scope | A particular agent/global/project destination; several scopes may share physical files |
| Library membership | Personal shelf/list organisation; not install ownership or deletion approval |
| Desired set | Saved installation intent for a target; not a verified filesystem observation |
| Bundle revision | Proposed immutable skill contents and provenance; not a shelf row timestamp |
| Setup profile | Existing revisioned snapshot/exclusion workflow; not automatically a live agent launch profile |
| MCP endpoint | Reet's own API for clients; distinct from configuring MCP servers used by other agents |

## Validation commands and interpretation

For documentation-only changes, check Markdown links, whitespace and consistency
with the inspected code; do not rerun the entire application suite just to validate
prose. For implementation, run targeted regression tests while developing, then
required repository checks before PR handoff. Use tools from the current environment;
never assume a previous session's binaries or caches are present.

Prerequisites: versions from `go.mod` and `web/package.json` (at this baseline Go
1.27.1 and Node 22), dependency installation when missing, tmux 3.2+ for terminal
service/integration tests, and a supported Chromium for browser tests. Read the
managed environment runtime instructions before network/container work. Never
print environment secrets, commit credentials or change global system directories
just to reconstruct this previous session's environment.

From the repository root, run sequentially:

```sh
(cd web && npm run check && npm run build)
go vet ./...
go test -race ./...
(cd mcp && npm test)
git diff --check
```

**Finish the web build before starting Go builds/tests.** Go embeds `web/dist`;
concurrent Vite cleaning/rebuilding can produce missing embedded asset failures
that are test-environment races, not application regressions. Keep inputs stable
while checking. The full API race package can take several minutes on cold caches;
do not remove timeouts, skip checks or assume a silent process is hung.

Examples of narrow checks (add the relevant new package/tests for the chosen task):

```sh
go test ./internal/setup ./internal/scan ./internal/store
go test -race ./internal/setup ./internal/scan ./internal/store
```

For the existing synthetic Control browser contract, start the dev server in a
separate terminal and keep it running:

```sh
cd web
npm run dev -- --host 127.0.0.1 --port 4173
```

Then, from the repository root:

```sh
node web/tests/control.e2e.mjs
node web/tests/lists.e2e.mjs
node web/tests/community.e2e.mjs
node web/tests/spaces.e2e.mjs
node web/tests/setups.e2e.mjs
node web/tests/inbox.e2e.mjs
node web/tests/admin-users.e2e.mjs
node web/tests/admin-team.e2e.mjs
node web/tests/admin-tickets.e2e.mjs
node web/tests/admin-email.e2e.mjs
node web/tests/admin-notifications.e2e.mjs
node web/tests/admin-backups.e2e.mjs
node web/tests/admin-settings.e2e.mjs
node web/tests/admin-activity.e2e.mjs
node web/tests/account-sessions.e2e.mjs
node web/tests/account-tokens.e2e.mjs
node web/tests/account-profile.e2e.mjs
node web/tests/account-security.e2e.mjs
node web/tests/account-mcp.e2e.mjs
node web/tests/skill-card.e2e.mjs
node web/tests/machine-add-skill.e2e.mjs
node web/tests/sync-remove.e2e.mjs
node web/tests/machines-search.e2e.mjs
node web/tests/terminal-workspace.e2e.mjs
node web/tests/terminal-lifecycle.e2e.mjs
```

`REET_UI_URL` can select another dev URL. `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`
selects a browser (the current fixture otherwise uses `/usr/bin/chromium`).
`REET_UI_SCREENSHOTS` is an optional output directory for synthetic screenshots.
This fixture mocks APIs; pair it with real domain/API/host tests for new effects.

For real terminal browser regression, follow `.github/workflows/terminal-browser.yml`
and the integration fixture, after building web and making tmux/browser available:

```sh
REET_BROWSER_TESTS=1 go test -race -count=1 -timeout=3m ./internal/api -run '^TestTerminalPrivateLifecyclePreservesActualShell$' -v
```

This exercises a real local shell/broker/PTY with test transports. It does not
establish a production relay deployment, cross-OS recovery, or a live fleet rollout.
Physical macOS service-manager and supported cross-OS checks remain separate gates.

### Record after every implementation task

Replace the relevant unchecked task with its completed state only after recording:

```text
Task ID and scope:
Branch / commit / PR:
Behaviour added:
Compatibility / migration behaviour:
Checks actually run and their results:
Platform or live-environment checks not performed:
Known limitations or unresolved decisions:
Exact next unchecked task and first files to read:
Publication / deployment state (verified, not inferred):
```

If a task cannot meet its acceptance gate, leave it incomplete and describe the
failing case and recoverable state. "Code compiles", "API returned 200", a mocked
success response or a green screenshot does not establish successful remote writes,
working agent authentication or complete environment recovery.
