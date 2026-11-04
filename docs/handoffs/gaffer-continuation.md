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

## Published work and current state

| Work | Branch / PR | Published head / status |
| --- | --- | --- |
| Runtime model/effort, steering, durable queue | `feat/live-chat-controls`, [#488](https://github.com/eyupio/reet.sh/pull/488) | Merged as `ac5938d`; do not reopen or push new scope to this closed PR. |
| Active-session action UX | `feat/session-actions`, [#490](https://github.com/eyupio/reet.sh/pull/490) | Open. Head `d94f1d2` merges main `b6b6ea4` and records the integration; its own final-head checks were running at this handoff. |
| Start dialogs and backlog model/effort | `feat/start-agent-options`, [#493](https://github.com/eyupio/reet.sh/pull/493) | Open. Head `0a66ce7` merges the `d94f1d2` session action head and records the merged-head verification. |
| Agent image previews | `feat/chat-image-previews`, [#498](https://github.com/eyupio/reet.sh/pull/498) | Open. Contains #490 and #493 and current main; this handoff update is its newest commit. Inspect GitHub for the current head. |

Last fetched main was `b6b6ea4` (#497, public and self-canonical sitemap test), which follows #496 (remote-agent, sync and MCP
guides) and #495 (collapsible/resizable session workspace panel). Fetch again before acting. Main also includes #494
(public documentation links), #493-independent Gaffer artwork/motion and published documentation (#489, #491, #492), and
#488. All three feature branches now carry that main; the toolbar/panel overlap is resolved identically in each.

**GitHub publication and CI-trigger problem (reproduced and partly recovered):** a normal push to `feat/session-actions` or
`feat/start-agent-options` updated the branch ref but left the pull request snapshot and its synthetic
`refs/pull/<n>/head` on an older commit, so no `pull_request` workflow runs were created and old checks could be mistaken
for the new head. Closing and reopening PR #490 resynchronised `refs/pull/490/head` to the pushed head and triggered the
image, binary release and terminal-browser workflows on it. Closing and reopening PR #493, and pushing a further
handoff commit to it, did not make GitHub dispatch its `pull_request` workflows; only the lightweight pull-request summary
run appeared. Do not merge from a stale `refs/pull/<n>/head`: check `gh api repos/<owner>/<repo>/git/refs/pull/<n>/head`
against the branch ref first. One `git push` also returned `Internal Server Error` and succeeded on retry.

PR #498 ran its complete PR-triggered matrix on head `f6be70c`: image (`41814713292`), terminal browser (`41814713184`),
native services (`41814713241`), private connection (`41814713386`) and binary release (`41814713285`) all succeeded, as did
CodeQL and the language analysis jobs. No inline review findings were open on any of the three PRs. The preview branch then
merged main `b6b6ea4` and the `0a66ce7` startup option head; that newer head needs its own green run before merge.
All three PRs remained open and unmerged when this handoff was published.

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
action controls and the full session-workspace resize/collapse browser suite pass. Complete preview package race suites and final full browser CI remain gates, as do
real-provider/device/platform limitations.

## Portable restart procedure

1. Clone/fetch the repository, read `AGENTS.md` if present, `handoff.md`, this file and each feature handoff.
2. Inspect the current PRs and remote branch heads, reviews, check runs and main. Reconcile changes made since this snapshot.
3. Check out PR #498 (`gh pr checkout 498`, or fetch/check out `feat/chat-image-previews`) for the full combined implementation. It should include #490/#493; preserve all handoffs,
   screenshot assets and UI-verification appendices during merges. Use separate branches/worktrees if fixing an older PR.
4. Restore final-head CI for #490/#493; inspect whether the GitHub mergeability/trigger problem has recovered. Never force
   through pending, missing or failed checks. If a job fails, read its logs and fix the cause; retry only demonstrated transient failures.
5. Finish any preview verification/review findings. Resolve comments and merge conflicts, then merge #490, #493 and previews
   in dependency order when green. The preview branch contains both published older heads; if GitHub still cannot validate
   the older PRs, the combined preview PR can be the reviewed integration path once all its own final-head checks pass and
   all review findings are addressed. Verify and record the older PRs’ final state after that merge; do not silently discard them.
   Re-fetch after each merge and verify actual merge state.
6. Update these handoffs in the relevant PR whenever the implementation, evidence or remaining work changes. Keep all new
   public messaging free of the external comparison project's name. Do not copy old machine-specific paths or process IDs.

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
