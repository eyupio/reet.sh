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

## Delivered work

| Work | Branch / PR | Final state |
| --- | --- | --- |
| Runtime model/effort, steering, durable queue | `feat/live-chat-controls`, [#488](https://github.com/eyupio/reet.sh/pull/488) | Merged as `ac5938d`; do not reopen or push new scope to this closed PR. |
| Active-session action UX | `feat/session-actions`, [#490](https://github.com/eyupio/reet.sh/pull/490) | Merged as `99c7fd8`. Final head `d94f1d2` was green: source, test, web, phone-zoom, phone-dialogs, terminal-browser, build (linux, amd64), CodeQL and the language analyses. |
| Start dialogs and backlog model/effort | `feat/start-agent-options`, [#493](https://github.com/eyupio/reet.sh/pull/493) | Merged as `918f452`. Final head `906cb2d` was green across all twenty checks: source, test, web, phone-zoom, phone-dialogs, terminal-browser, native service on ubuntu and macos, local relay on ubuntu and macos, tmux minimum, private connection, image, binary release, build (linux, amd64), CodeQL and the four language analyses. |
| Agent image previews | `feat/chat-image-previews`, [#498](https://github.com/eyupio/reet.sh/pull/498) | Merged as `0de8f7d` from head `a88c78e`, which was green across all twenty checks, including terminal-browser with the preview, Sessions phone, session workspace and launch option suites. |

All three feature PRs are merged in dependency order (#490, #493, #498). Main at merge time was `0de8f7d`; it also carried
#495 (collapsible/resizable session workspace panel), #496 (remote-agent, sync and MCP guides), #497 (public and
self-canonical sitemap test), #500 and #501 from other work, plus the earlier #488/#489/#491/#492/#494. The toolbar/panel
overlap between #490 and #495 is resolved identically on every branch: the persistent Theatre action, the wrapping
interface/approval/`Terminal here` toolbar and the duplicate-free menu stay, and the docked panel keeps its separator,
saved width/open state, sheet mode below 1100 px, and `holdSize` while it is dragged. The only merge conflict in the final
round was the handoff append in `docs/handoffs/session-actions.md`, resolved by keeping the newest text.

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

Local evidence on the integrated tree matches CI: web unit tests, Svelte check with zero errors and warnings, the
production build, `go vet ./...`, the preview, Sessions phone, session workspace and launch option browser suites, and
`go test -race` for `internal/acpagent`, `internal/agenthealth`, `internal/terminalproto`, `internal/terminalbroker`,
`internal/store` (158 s) and `internal/api` (330 s) all pass. The only local failure is `internal/terminalagent`'s
model-listing tests (`TestModelsOneAtATime`, `TestModelsFloorReturnsTheLastResult`, `TestModelsReplyShapeAndOperation`,
`TestModelsLateRunAnswersTimedOut`), which fail identically on unmodified main in this constrained shell and pass in the
native service CI jobs, so they are environmental rather than a regression.

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

Use the Go version in `go.mod`, Node 22, tmux and a Playwright-supported Chromium. Set
`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` to the actual installed browser when needed. Run from the repository root:

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
