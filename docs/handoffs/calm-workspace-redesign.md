# Calm workspace redesign handoff

Status: design approved; implementation in progress on `feat/calm-session-workspace`. Date: 7 November 2026. Design PR #635 is merged. The implementation PR [#637](https://github.com/eyupio/reet.sh/pull/637) is a draft and must not be treated as release-ready.

The user approved implementation and explicitly requires true Chat / Terminal view switching that preserves the running agent. Prototype interactions use fictional data and never connect to an agent, machine or API. The prototype predates this requirement; the implementation and contracts below supersede its restart-only terminal presentation.

## Outcome

Make returning to an agent session feel immediate: find the task, understand its state, read the work, and answer the next question. Preserve Reet's machine management, skill library, reviewed distribution and reusable setups while giving them the right prominence for the task at hand.

The first implementation slice should improve the session screen. Home and broader navigation follow after that slice is reviewed. Keep Svelte, Go, SQLite and existing services; a framework change is unnecessary.

## Review materials

- [Interactive standalone prototype](../../web/design/calm-workspace/index.html): session, Home, Needs you, optional details, theme and phone navigation. Open locally, or serve the repository root with `python3 -m http.server 8765` and visit `/web/design/calm-workspace/`.
- [Desktop, dark](../../web/design/calm-workspace/session-desktop-dark.png)
- [Desktop, light](../../web/design/calm-workspace/session-desktop-light.png)
- [Phone, dark](../../web/design/calm-workspace/session-phone-dark.png)
- [Phone, light](../../web/design/calm-workspace/session-phone-light.png)
- [Home, light](../../web/design/calm-workspace/home-desktop-light.png)

The preview is a visual and interaction proposal. Its terminal, conversation, counts, approvals, file changes and messages are fixtures. The illustrated session uses Conversation; its menu explains the existing runtime mode-switch consequence rather than pretending a terminal view is always available beside chat. It deliberately omits most production controls. The preservation contract below defines what must remain accessible in a future implementation.

## 1. Interface rules

1. One dominant work surface. The selected session owns the centre; session navigation stays narrow and Details is optional.
2. Predictable rows. Agent glyph, task title, subordinate machine/project context, trailing state. Use the same alignment in lists and navigation.
3. One situation, one message, one place. The session owns its actionable error or request. Other surfaces may show a count or link to it, not another competing prompt.
4. Neutral selection. Use a tonal fill and text weight rather than a bright border. Reserve burnt orange for the main action, focus and restrained identity accents. Status colour is accompanied by words or an accessible label.
5. Quiet resting state. Avoid cards around every section, repeated badges, decorative statistics and perpetual motion. Use spacing, type and subtle surface changes to separate content.
6. Progressive detail. Show the task, place, state and next action first. Put diagnostics, evidence, long paths and infrequent actions behind Details or a named menu.
7. Stable context. Updates must not move focus, replace a draft, reset scroll or switch a chosen view. Restore sensible fallbacks when storage is unavailable.
8. Keep Reet recognisable. Retain IBM Plex Sans/Mono, restrained Fraunces headings, warm neutral surfaces and the orange brand accent. Keep Gaffer as a compact, optional companion; actionable requests take precedence over personality.

## 2. Information architecture

### Session workspace: first slice

Desktop: a compact navigation/session rail, a single session header, conversation or terminal content, and a composer. The header carries task title, machine/project context, one state phrase, Details and a More menu. Details opens the existing Overview, Changes, Files and GitHub panel. It is closed in the reference's initial state; a person's saved preference takes precedence in production.

The session rail contains a clear New session action, a search field, a Needs you filter and session rows. Global Home, Machines, Skills and More destinations remain reachable. The selected row stays visible when filtering where practical; otherwise explain the active filter without silently opening a different session. Preserve the existing project grouping, ordering, resizing and collapse affordances through the session navigation controls.

Tablet: collapse the global rail before compressing the content. Phone: use a Sessions sheet and a Details sheet, each with a named close control and focus return. Keep the task and machine identity visible. Preserve session focus mode, keyboard height handling, safe areas, terminal keys and an accessible route back to Home and all secondary destinations. Only one sheet is open at a time; Escape and browser Back close the top layer before leaving the session.

### Home: second slice

Use Continue working, Needs you and Start a session as the main hierarchy. Keep the existing planning board accessible through a prominent Planning board view. Preserve Backlog, Active, Finished, stage counts, sorting, density, filters, idea capture, saved prompts, images and refine-before-start. Do not change the user's saved default view during rollout. A new default for new users is a separate product decision.

### Supporting pages: later slice

Keep Machines, Skills and grouped More destinations. Lists, Setups, Sync, Teams, Community, Support, publishing, administration and account routes retain their permissions and deep links. Page-local workflows own software evidence, inventory, setup and sync diagnostics. Shared navigation and visual tokens may become quieter; moving a capability must not make it undiscoverable.

## 3. Behaviour and capability contracts

### Conversation and terminal

`SessionScreen.svelte` currently derives chat from `session.structured`. `SwitchModeDialog.svelte` explains that switching the supported OpenCode session mode stops one process and starts another, can stop current work, loses terminal scrollback and may require folder trust. A view preference is not a runtime mode change.

The primary **Chat / Terminal** control changes presentation only. It must preserve the running worker, conversation, working directory, drafts, stream subscription and mounted terminal attachment. It never calls the runtime mode-change endpoint. Unsupported agents show an accessible explanation instead of implying support.

The draft implements the shared native service path for exactly OpenCode 1.19.5. A connector-negotiated `native-chat-view-v1` capability and a per-session readiness check gate it. The local ACP worker hosts an authenticated loopback service; a disposable native TUI client attaches to its existing conversation. Credentials stay on the connector. Existing restart-based interface changes remain explicitly labelled in Session actions with their confirmation.

### Attention and actions

Use the existing `needsYou`, `attentionCount` and `NeedCard` model across Home, the bell and session navigation. A count is navigation; the owning session presents the actionable request. If existing Home/bell controls remain directly actionable, they must share request identity and pending/resolved state, so a response on one surface cannot leave an apparently actionable duplicate elsewhere.

Retain exact approval scope and semantics: Allow once, Deny and any supported session-scoped allowance. Do not replace them with an ambiguous global Approve button. Show the target command/path before consent. Disable repeated submission while pending; confirm success only after acknowledgement. Preserve a recoverable failure in place. A stale request explains that it must be refreshed. A disconnected request cannot imply that a reply was delivered.

Destructive controls remain in named menus with existing confirmation. Disconnect leaves work running; End session stops it. Archive/remove-history must not imply file deletion. Permissions, read-only roles, trusted-folder checks and Ask me/YOLO consent remain intact.

### Context and data freshness

Preserve account-scoped layout preferences, remembered panel width/open state, session selection, drafts, attachment references and each surface's scroll position. Do not persist secrets or terminal contents as a new design preference. Restore only valid, current session identities; a replaced/removed session must not inherit another session's draft or approval.

Keep existing last-known/current distinctions in machine health, inventories, Changes and GitHub. Never use a quiet visual treatment to hide stale evidence, failure or disconnection. Do not claim tests passed from a mock status or inferred agent activity.

## 4. Component map and implementation boundaries

| Area | Existing entry points | Proposed work |
| --- | --- | --- |
| Global shell | `web/src/lib/shell/AppShell.svelte`, `WorkspaceSidebar.svelte`, `MoreSheet.svelte`, `JumpSearch.svelte` | Reduce competing chrome; preserve all destinations, search, account and bell. |
| Session composition | `web/src/lib/sessions/SessionScreen.svelte`, `SessionsPage.svelte`, `SessionTabs.svelte` | Consolidate header, session rows and optional Details; reuse existing session identity and routing. |
| Context | `web/src/lib/sessions/ContextPanel.svelte`, `contextLayout.ts` | Retain panel content, remembered width, resizing and narrow-screen behaviour. |
| Mode changes | `web/src/lib/sessions/SwitchModeDialog.svelte`, `web/src/lib/sessions.ts` | Keep explicit process-switch confirmation and capability gates; no cosmetic toggle over a runtime operation. |
| Conversation and companion | `web/src/lib/terminal/ChatPane.svelte`, `GafferProgress.svelte`, `GafferChatWidget.svelte`, `gafferProgress.ts`, `gafferMotion.ts` | Keep streaming, tools, attachments, stop/retry and settings; compact companion and progress presentation. Keep the existing chat owner boundaries. |
| Attention | `web/src/lib/home/NeedsYou.svelte`, `web/src/lib/home.ts`, `web/src/lib/shell/` bell surfaces | One source of request identity and delivery state; counts link to requests. |
| Home | `web/src/lib/home/Home.svelte`, `SessionBoard.svelte`, `ProjectRail.svelte`, `NewSessionDialog.svelte` | Continue-first hierarchy as an optional view; retain planning and launch contracts. |
| Visual system | `web/src/app.css`, `web/src/workspace.css`, `web/src/lib/theme.ts`, `theme.test.mjs` | Semantic surfaces, state/selection rules and shared scale; preserve measured theme contrast. |

File names are implementation starting points, not permission to rewrite every listed component. Resolve owners against the implementation branch before editing; the app has evolved and existing approved workspace work remains a baseline contract.

## 5. Suggested visual specification

- Desktop navigation rail: approximately 264 px, resizable where the existing layout supports it. Main transcript: comfortable measure around 720–800 px; flexes when Details opens.
- Details: reuse existing clamped widths and persisted preferences rather than imposing a new fixed width.
- Spacing: 4, 8, 12, 16, 24, 32 px. Avoid one-off values unless content requires them.
- Type: 14 px chrome, 16 px reading/input text, compact metadata at 12–13 px. Fraunces belongs on a small number of page/task headings; code and paths use IBM Plex Mono.
- Surfaces: canvas, navigation/panel, selected/raised, hover. Hairline separators where needed; shadows for overlays, not every row.
- Colour: retain the semantic tokens in `app.css`. The static prototype approximates the desired atmosphere and is not a replacement token export. Measure contrast on actual composed light/dark surfaces before implementation acceptance.
- Motion: transitions only for direct interaction and meaningful state changes. Honour reduced motion and existing Gaffer animation settings. No moving mascot or ambient animation in the prototype.
- Touch: at least 44 px actionable targets, 16 px field text, visible keyboard focus, no hover-only essential action.

## 6. Delivery plan after approval

### A. Agree the reference

Review the desktop and phone captures in both themes, Home hierarchy, Details behaviour, companion prominence and navigation reachability. Record decisions in this handoff. Approval covers design direction; it does not establish backend support for new capabilities.

### B. Session-only presentation PR

Consolidate session header/rows and apply quiet surfaces using existing data and APIs. Preserve the explicitly labelled legacy runtime mode-change semantics; implement the separate same-process view contract above. Reuse attention and context models, retain focus mode and controls, and avoid unrelated Home or machine-management changes. Keep changes reversible at component/token boundaries; do not migrate stored data or invalidate saved preferences.

Acceptance: a user can identify task, machine and next action; read/respond; inspect Details; reach all existing session actions; return to another session without losing context.

### C. Attention and interaction consistency PR

Unify request ownership and pending/resolved/failure presentation across existing surfaces. Verify concurrent response, stale request, offline/reconnect and view-only cases. If achieving shared delivery state requires API changes, split that work into a separately reviewed contract change.

### D. Home and supporting chrome PR

Add the Continue-first presentation without removing Planning board or changing saved defaults. Simplify navigation styling and bring secondary pages onto shared row/control conventions incrementally. Preserve the feature contract in `web/design/standalone-workspace/README.md`.

### E. Verification and rollout

Review 1440 px desktop and 390 px phone in both themes; check 320 px and short landscape for overflow/keyboard behaviour, plus long titles/paths, empty state, offline, running, needs-input, sending, failed, stale and finished states. Check keyboard-only navigation, focus return, screen-reader names, reduced motion and measured contrast.

Run `npm run check` in `web/`, `go vet ./...`, and `go test -race ./...`, plus the repository's focused browser suites for sessions, attention, mode switching, terminal sizing/paste and mobile layout. Select exact commands from the current test inventory in each implementation PR. Run the offline Gaffer acceptance gate if conversation wiring changes. Compare against the existing feature contract with populated fixtures. Record any baseline failures without suppressing them.

Rollback is a revert of the presentation PR; preserve backend contracts, old deep links and preference compatibility so a revert does not strand sessions. Broader navigation/Home changes should be independently revertible.

## 7. Design review decisions

| Decision | Proposal | Approval state |
| --- | --- | --- |
| First scope | Session presentation and shared native-view foundation | Implementation approved |
| Details default | Closed for new/unset preference; honour existing saved choice | Direction approved; acceptance pending |
| Home | Continue-first optional view; Planning board stays prominent | Direction approved; acceptance pending |
| Companion | Compact, static by default in this reference; retain user animation setting in production | Direction approved; acceptance pending |
| Terminal access | Primary same-process Chat / Terminal switch; legacy restart action stays explicit | Required by user |
| Navigation | Home, Sessions, Machines, Skills and grouped More remain reachable | Direction approved; acceptance pending |

The merged design PR contains the original reference. The implementation draft adds runtime and presentation changes; no deployment or merge is requested yet.


## 8. Implementation checkpoint and architecture lessons

The draft consolidates the session header, moves infrequent controls into Session actions, makes Details closed only for unset preferences, quietens the session rail and composer, and retains saved layout before first paint from #634. Both chat and native terminal stay mounted after initial attachment; hidden panes do not take focus or resize the visible terminal.

Use each agent's native conversation identity and supported transport rather than scraping rendered terminal output into chat. Keep a capability matrix per agent/version: a shared local service, ACP stream and transcript file have different authority and recovery semantics. Native user records and service events supplement the ACP stream, with bounded history and de-duplication of chat-origin messages. Approval requests retain one identity and must retire across both interfaces after either answers. Never infer success from a visual transition. Preserve manual view selection during streaming and reconnects. These rules matter more than matching surface styling.

Code map: `SessionViewSwitch.svelte`, `SessionScreen.svelte`, `TerminalWorkspace.svelte` and `TerminalPane.svelte` own presentation; `internal/acpagent/native_view.go` owns the local service and reconciliation; `terminalproto`, `terminalbroker`, `terminalagent` and `terminalhost` own capability negotiation, authorization, tickets and disposable PTY attachment. No native endpoint or password is returned to the browser.

## 9. Coordination with #636

Reviewed #636 at `f91945fb47db3f49225e23c5b691d04e3a3afbab` (`feat/session-worktree-isolation`), based on main `e1c4ed9b`. It is not merged into this branch. A `git merge-tree --write-tree HEAD origin/pr-636` simulation merged textually without conflicts; this does not establish runtime compatibility.

Both views must use the same existing worktree and exact worker directory. Switching views must never launch a session, allocate a run token, create a branch or create/remove a worktree. Preserve #636's archive confirmation callback, fingerprint/count checks, active-session refusal and branch-retention semantics. Overlaps are `internal/api/terminal.go`, `TerminalWorkspace.svelte` and terminal `api.ts`.

At that reviewed head, isolation injects worktree/project fields into structured creates, while the structured Create validation and launcher do not yet accept/prepare that combination. Resolve and test that integration with #636 before claiming structured worktree launches work. Re-check its current head and review fixes before integration; do not silently merge an outdated snapshot. The real native attachment probe uses an isolated-worktree-shaped directory and asserts the native client uses the ACP worker's directory, but does not replace combined launch/archive tests.

## 10. Verification and remaining merge gates

Passed on the rebased implementation: Svelte check (zero errors/warnings), production build, `go vet ./...`, and focused race tests for native views/capabilities/tickets across acpagent, terminalproto, terminalbroker, terminalagent and agenthealth. The real pinned OpenCode probe starts ACP and the actual native TUI, verifies worker PID stability, and verifies killing the disposable client leaves the worker alive. It makes no provider/model request. Browser fixtures exercise layout, actions, saved Details, drafts and a mocked native socket without calling the mode endpoint; the final post-rebase run passed all eight theme/width combinations and the mocked native continuity cases.

The unchanged-source baseline full `go test -race ./...` did not pass: API package timed out at the default ten-minute limit, terminalhost tests lacked tmux, and the model timeout process-group test reported a surviving child. These are not a passing full regression suite.

Before marking ready:

- Exercise actual terminal-origin prompts and bidirectional streamed output, concurrent approvals/questions, native replies retiring chat cards, duplicate and late responses, and model/mode reconciliation.
- Harden native SSE reconnect and stale busy-state recovery, bounded echo cleanup after failed prompts, and permission ledger races; current foundation needs these tests.
- Verify the complete API/broker/host attachment path, lease renewal, takeover, reconnect and process teardown with tmux installed. The direct native probe covers only the worker/client lifecycle.
- Update existing sessions/chat browser tests to open Session actions while retaining every previous contract. Run terminal sizing/paste, phone keyboard, focus, attention and offline Gaffer acceptance suites. New fixture tests do not replace those suites.
- Integrate the current #636 safely and test structured worktree creation, retry identity, archive confirmation and same-directory dual views together.
- Resolve or reproduce baseline full race failures and record exact outcomes. Do not claim every agent supports shared views; the current implementation is pinned to OpenCode 1.19.5.

Home and broader navigation redesign remain deferred. Rollback is a revert of the implementation commits; no stored data migration is introduced.

## 11. CI repair checkpoint

The first #637 CI run exposed four concrete regressions: the controller feature-list assertion omitted `native-chat-view-v1`; workspace measurement started before the loading skeleton was replaced, allowing the companion launcher to overlap; legacy interface tests did not open Session actions; and the terminal size assertion still counted the removed inline controls rather than the current header. The repair updates these contracts, installs measurement when the workspace is bound, restores focus after the theatre action, and preserves 44 px touch targets with compact labels at 320 px. The new calm-session browser suite is now included in CI.

The API feature regression test, full ACP package race tests, structured chat browser suite, twelve widget views, Svelte check and build passed locally. A full phone-dialog development-server run cleared all switch-dialog failures but reported one keyboard-layout hit-test failure; the isolated production-build reproduction passed. Broad browser verification continues; a passing focused test must not be reported as a passing complete workflow. The unchanged two-second synthetic terminal continuation performance gate has also fluctuated under concurrent local browser load. Check fresh CI before marking #637 ready. The native offline acceptance job passed in the original CI run; other complete workflow outcomes must be checked independently.
