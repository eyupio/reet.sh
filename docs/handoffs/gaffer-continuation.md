# Gaffer continuation handoff

Updated 4 November 2026. Repository: https://github.com/eyupio/reet.sh. All paths below are relative to this repository.
No state in a previous machine, checkout, browser server, temporary directory or tool session is required to continue.

## Standing user instructions

Continue the Gaffer wrapper review and implementation in reviewable PR batches. Fix review comments and merge conflicts,
keep the handoff in each PR current, babysit CI and merge when the final head is green. The user already authorised these
repository writes and merges; do not ask them to confirm routine work again. Do not reference the external comparison
project in PR titles, descriptions, commits or merge messages. Do not spawn agents without applicable explicit authorisation.
The requested features are active-session model/effort switching and queued-message steering, visible/responsive session
actions, the same model/effort choices in both start dialogs and backlog plans, and inline previews of agent-produced/read images.

## Starting a fresh session

1. Read `handoff.md`, this file, `docs/handoffs/session-actions.md`, `docs/handoffs/start-agent-options.md` and
   `docs/handoffs/chat-image-previews.md`. They record the delivered behaviour, the evidence and the limits.
2. `git fetch origin` and read `gh pr list --state open`. The Gaffer batches are merged; anything still open belongs to a
   different work stream and should be left to it.
3. Confirm the merged state in main: `web/src/lib/sessions/SessionScreen.svelte` keeps both the action toolbar and the
   resizable panel, and `internal/terminalagent/chat_previews.go`, `internal/terminalproto/acp_preview.go`,
   `web/src/lib/terminal/ChatPreview.svelte`, `web/tests/chat-previews.e2e.mjs` and `web/tests/launch-options.e2e.mjs` exist.
4. Re-run the checks below before changing anything. Push-triggered workflows on main need the self-hosted
   `zoomies-linux-x64` pool, so use a pull request for any follow-up; do not reopen the merged PRs or push to their branches.
5. If a follow-up is needed, keep the invariants in "Continuing after these merges" and add its own handoff section in the
   same PR.

## Delivered work

| Work | Branch / PR | Final state |
| --- | --- | --- |
| Runtime model/effort, steering, durable queue | `feat/live-chat-controls`, [#488](https://github.com/eyupio/reet.sh/pull/488) | Merged as `ac5938d`; do not reopen or push new scope to this closed PR. |
| Active-session action UX | `feat/session-actions`, [#490](https://github.com/eyupio/reet.sh/pull/490) | Merged as `99c7fd8`. Final head `d94f1d2` was green: source, test, web, phone-zoom, phone-dialogs, terminal-browser, build (linux, amd64), CodeQL and the language analyses. |
| Start dialogs and backlog model/effort | `feat/start-agent-options`, [#493](https://github.com/eyupio/reet.sh/pull/493) | Merged as `918f452`. Final head `906cb2d` was green across all twenty checks: source, test, web, phone-zoom, phone-dialogs, terminal-browser, native service on ubuntu and macos, local relay on ubuntu and macos, tmux minimum, private connection, image, binary release, build (linux, amd64), CodeQL and the four language analyses. |
| Agent image previews | `feat/chat-image-previews`, [#498](https://github.com/eyupio/reet.sh/pull/498) | Merged as `0de8f7d` from head `a88c78e`, which was green across all twenty checks, including terminal-browser with the preview, Sessions phone, session workspace and launch option suites. |

All three feature PRs are merged in dependency order (#490, #493, #498) and the handoff refresh that records them landed as
[#503](https://github.com/eyupio/reet.sh/pull/503) (`4539386`) and [#504](https://github.com/eyupio/reet.sh/pull/504)
(`80ae4cc`), both documentation only. Main has since advanced through #499 and #502 and other content work (#496, #501);
none of it touches the Gaffer code paths, and the addresses and page registration of the delivered documentation stay
intact. No Gaffer pull request remains open. Other work streams keep opening and merging their own pull requests, so read
`gh pr list --state open` rather than relying on any snapshot here.
The toolbar/panel overlap between #490 and #495 is resolved as follows:
the persistent Theatre action, the wrapping interface/approval/`Terminal here` toolbar and the duplicate-free menu stay,
and the docked panel keeps its separator, saved width/open state, sheet mode below 1100 px, and `holdSize` while it is
dragged. The only merge conflict in the final round was the handoff append in `docs/handoffs/session-actions.md`, resolved
by keeping the newest text.

**GitHub publication and CI-trigger problem (diagnosed and resolved):** a push to `feat/session-actions` or
`feat/start-agent-options` updated the branch ref, but the pull request snapshot and its synthetic
`refs/pull/<n>/head` could lag, and a pull request whose head is not mergeable receives no `pull_request` workflow
dispatch at all. Check `gh api repos/eyupio/reet.sh/git/refs/pull/<n>/head` against the branch ref before trusting a PR
snapshot, and confirm `mergeable` is `MERGEABLE`. Closing and reopening resynchronised #490 and started its matrix. #493
stayed silent until it actually merged current main: its earlier head still excluded main #495, so the PR was
`CONFLICTING` and its workflows never ran; after integrating the session action head (which carries main) it became
`MERGEABLE` and all five workflows dispatched within a minute. One `git push` also returned `Internal Server Error` and
succeeded on retry. Do not merge from a stale `refs/pull/<n>/head`, and never treat checks on an older commit as
final-head evidence.

Local evidence on the integrated tree matches CI. On main `80ae4cc` (which already carried #490, #493 and #498): web unit
tests, Svelte check with zero errors and warnings and the production build pass, `go vet ./...` passes, and the built site
passes `web/tests/chat-previews.e2e.mjs`, `web/tests/launch-options.e2e.mjs`, `web/tests/sessions-phone.e2e.mjs` and
`web/tests/session-workspace.e2e.mjs`. `go test -race` passes for `internal/acpagent`, `internal/agenthealth`,
`internal/terminalproto`, `internal/terminalbroker`, `internal/store` (158 s) and `internal/api` (330 s). `internal/terminalagent`
passes as well: its four model-listing cases (`TestModelsOneAtATime`, `TestModelsFloorReturnsTheLastResult`,
`TestModelsReplyShapeAndOperation`, `TestModelsLateRunAnswersTimedOut`) need the host manager, so they skip with a clear
message when tmux is not installed instead of failing on a listing that never starts. They run in the native service CI
jobs, which install tmux.

## Feature details and evidence

### #490: active-session actions

Read `docs/handoffs/session-actions.md`, `web/src/lib/sessions/SessionScreen.svelte`,
`web/src/lib/terminal/TerminalWorkspace.svelte` and `web/src/lib/terminal/TerminalPane.svelte`.
Theatre, Details, View/approvals and contextual terminals are visible; More contains management actions without duplicates.
The contextual button is **Terminal here**, distinct from the create form's **New terminal** submit. Phones retain 44 px
targets; landscape retains a compact controls header and a measured terminal-row budget. Standalone terminal theatre still works.

Web check/build, embedded-web checks, responsive light/dark desktop/tablet/phone/landscape tests, terminal workspace/phone,
Start an agent, the full phone-dialog suite, chat/recovery, launch and Sessions-phone checks pass locally. The full backend,
build, source, CodeQL, terminal-browser (including the real tmux shell) and zoom checks passed on earlier source head `f7020afe`.
The phone job first stalled in the unchanged launch browser suite; that full suite passes locally. Its retry reached a stale
Sessions-phone assertion for the old header. That assertion and landscape geometry check were corrected and pass locally
in the latest published `5ea4303` head. That head still needs its own final CI. No inline review findings were open at the last check.

### #493: startup/backlog settings

Read `docs/handoffs/start-agent-options.md`, `web/src/lib/terminal/launchOptions.ts`,
`web/src/lib/StartAgentDialog.svelte`, `web/src/lib/home/StartSessionDialog.svelte`,
`web/src/lib/home/SessionComposer.svelte`, `web/src/lib/home/SessionBoard.svelte`, `web/src/lib/launchBar.ts`,
`internal/terminalproto/acp_controls.go`, `internal/terminalbroker/broker.go` and `internal/api/session_backlog.go`.

Both modals and saved plans carry the model and native effort, including extended values advertised by the installed adapter.
Negotiate **`acp-start-options-v1` separately**: older live-controls connectors still reject structured create effort. The actual
session must confirm settings before readiness or the opening prompt. Definite admission failure clears unconfirmed session
model/effort while retaining the receipt/plan. Terminal effort retains its existing mappings. Reviewed Claude/Codex Gaffer
plans are supported alongside OpenCode; folder plugin trust stays OpenCode-specific. Offline editing preserves an existing
Gaffer plan and its unchanged model/effort when machine capabilities are unavailable.

Web check/build, the desktop/phone three-flow browser test (including offline edit), legacy Codex and Start an agent suites
pass. Complete race suites for `internal/acpagent`, `internal/agenthealth`, `internal/terminalproto`, `internal/terminalbroker`,
`internal/terminalagent`, `internal/store` and `internal/api` passed before the final UI-only offline edit (API: 472 seconds).
Independent negotiation/reconnect tests pass. Final published-head CI/review remain required. No inline findings were open.

### Image previews

Read `docs/handoffs/chat-image-previews.md` for completed behaviour and limits. Main code paths:

- `internal/acpagent/previews.go`: content/image/resource extraction, sparse tool path retention, streamed Markdown references.
- `internal/terminalproto/acp_preview.go`: negotiated metadata and bounded preview-part RPC, separate from user image uploads.
- `internal/terminalagent/chat_previews.go`: private immutable snapshots, root/owner/type checks, limits, expiry/quota and restart recovery.
- `internal/terminalagent/chat.go` and `agent.go`: negotiated delivery/replay and part serving.
- `internal/terminalbroker/chat_previews.go`: account/session ownership, bounded assembly, offset/MIME/hash verification.
- `internal/api/terminal_chat.go` and `private_control.go`: authenticated image route and production reply allowlist.
- `web/src/lib/chat.ts`, `web/src/lib/terminal/ChatPreview.svelte`, `ChatPane.svelte`, `api.ts`: reducer, lazy previews,
  full-size lightbox, retry/unavailable states, aborted stale requests and object URL cleanup.

Web unit/check/build and Go vet pass. Focused race tests pass for the new extraction/protocol/cache/broker/API paths;
desktop/phone light/dark browser checks pass. See `docs/ui-verification.md` and tracked `docs/images/ui/agent-preview-*.png`.
After integrating #495 and the final #490 phone tests, web check/build, the preview matrix, Sessions-phone, responsive
action controls and the full session-workspace resize/collapse browser suite pass, and the merged head `a88c78e` passed
every CI check including terminal-browser. Real-provider, device and cross-platform limits remain as described in the
feature handoff.

## Continuing after these merges

1. Start from main (`0de8f7d` or later) and read `AGENTS.md` if present, `handoff.md`, this file and each feature handoff.
2. Push-triggered runs on main use the self-hosted `zoomies-linux-x64` pool and stay queued when that pool is offline, so
   the pull request matrices (which run on GitHub-hosted runners) were the green evidence for these merges. Do not reopen
   the merged feature PRs or push new work to their branches; validate a follow-up with its own pull request checks.
3. If a follow-up touches the session screen, keep both the visible action toolbar and the resizable/collapsible workspace
   panel, and keep the terminal's size held while the panel is dragged. The regression coverage is
   `web/tests/sessions.e2e.mjs`, `web/tests/session-workspace.e2e.mjs`, `web/tests/sessions-phone.e2e.mjs`,
   `web/tests/terminal-workspace.e2e.mjs` and `web/tests/terminal-phone.e2e.mjs`.
4. If a follow-up touches startup or backlog settings, keep `acp-start-options-v1` negotiated separately from
   `acp-live-controls-v1` and keep `web/tests/launch-options.e2e.mjs` in terminal-browser CI.
5. If a follow-up touches previews, keep `acp-previews-v1` separate from user uploads, keep the private snapshot bounds and
   hash checks, and keep `web/tests/chat-previews.e2e.mjs` in terminal-browser CI.
6. When opening a new PR, confirm `refs/pull/<n>/head` matches the pushed branch head and that the PR is `MERGEABLE`
   before waiting on checks; reopen the PR or integrate main if workflows do not dispatch. Update the relevant handoff in
   the same PR and never use the external comparison project's name in public messaging.

## Reproduction commands

Use the Go version in `go.mod`, Node 22, tmux and a Playwright-supported Chromium. tmux is required by
`internal/terminalagent`'s host-manager cases, which skip without it. Set `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` to the
actual installed browser when needed. Run from the repository root:

```sh
npm --prefix web ci
npm --prefix web run check
npm --prefix web run build
go vet ./...
go test -race ./internal/acpagent ./internal/terminalproto ./internal/terminalagent ./internal/terminalbroker ./internal/agenthealth ./internal/api -run 'Preview|TestPrivateControlAdvertises' -count=1
```

For complete backend coverage, use an appropriate timeout and normal process reaping in the environment:

```sh
go test -race -p 2 -timeout=30m ./internal/acpagent ./internal/agenthealth ./internal/terminalproto ./internal/terminalbroker ./internal/terminalagent ./internal/store ./internal/api
```

Start the built fixture in a separate terminal, then run the focused browser suites. Choose another free local port if needed:

```sh
npm --prefix web run preview -- --host 127.0.0.1 --port 4173
```

```sh
REET_UI_URL=http://127.0.0.1:4173 node web/tests/chat-previews.e2e.mjs
REET_UI_URL=http://127.0.0.1:4173 node web/tests/launch-options.e2e.mjs
REET_UI_URL=http://127.0.0.1:4173 node web/tests/sessions-phone.e2e.mjs
REET_UI_URL=http://127.0.0.1:4173 node web/tests/phone-dialogs.e2e.mjs
REET_UI_URL=http://127.0.0.1:4173 node web/tests/chat.e2e.mjs
REET_UI_URL=http://127.0.0.1:4173 node web/tests/chat-recovery.e2e.mjs
REET_UI_URL=http://127.0.0.1:4173 node web/tests/start-agent.e2e.mjs
```

`REET_UI_SCREENSHOTS` optionally selects any output directory you create. Browser fixtures mock APIs; pair their results
with the backend tests above. Full production lifecycle commands are in `.github/workflows/terminal-browser.yml`.


## Foreground animation variety follow-up

Branch: `feat/gaffer-animation-variety`, 4 November 2026. The standalone workspace and screenshot refresh are merged through #541. This follow-up changes decorative Gaffer motion, not agent controls, previews, permissions or session lifecycle.

- `gafferPersonality.ts` chooses ambient performances using recent history, guarantees a visible brew by the second performance and excludes decorative poses from requests and snags. `gafferMotion.ts` owns the durations.
- `GafferProgress.svelte` keeps the ambient deadline independent of ordinary phase gestures and lets a performance finish through routine tool updates. New work wakes a resting head; decisions and problems interrupt immediately. The actual work labels, timer, quotes and handover remain independent.
- `GafferAvatar.svelte` keeps the same animation element through non-urgent phase updates. The two new WebP sheets add scratches, glances, a nose twitch and resting head, with the user's arm/sleeve corrections. Only visible motion-enabled work/completed cards warm these assets; paused entry artwork does not.
- `gaffer-variety.e2e.mjs` is included in the existing browser workflow. It drives the real component with a controlled clock and stream. The theme suite checks the four action frames of every new performance in both themes, in addition to prior blending, status-priority, pause and cleanup contracts.

Local validation: unit suite and production build; the existing Gaffer artwork, theme, return, interaction, progress and minimised browser checks; the new foreground-rotation regression. Final-head GitHub checks and review remain the merge gate. The new preview and both theme sheets are linked from `docs/ui-verification.md`.
