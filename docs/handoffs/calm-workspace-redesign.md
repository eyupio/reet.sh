# Calm workspace redesign handoff

Status: proposed, awaiting design review. Date: 7 November 2026.

This handoff and its standalone prototype are review materials. They do not authorise implementation of the production UI. Agree the direction in the design PR before starting feature work. Prototype interactions use fictional data and never connect to an agent, machine or API.

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

For the first slice, keep the current capability checks and explicit switch confirmation. Name it **Switch to agent terminal…** or **Switch to conversation…** in More when supported. Do not introduce a cosmetic Conversation / Terminal tab that calls the mode-change API. Unsupported agents retain their working terminal and an honest capability explanation. A future non-disruptive dual view requires a separate technical proposal and backend evidence before its UI is promised.

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

Consolidate session header/rows and apply quiet surfaces using existing data and APIs. Preserve runtime mode-change semantics. Reuse attention and context models, retain focus mode and controls, and avoid unrelated Home or machine-management changes. Keep changes reversible at component/token boundaries; do not migrate stored data or invalidate saved preferences.

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
| First scope | Session presentation only | Pending |
| Details default | Closed for new/unset preference; honour existing saved choice | Pending |
| Home | Continue-first optional view; Planning board stays prominent | Pending |
| Companion | Compact, static by default in this reference; retain user animation setting in production | Pending |
| Terminal access | Explicit capability-aware runtime switch; dual-view design deferred | Pending |
| Navigation | Home, Sessions, Machines, Skills and grouped More remain reachable | Pending |

No production implementation should begin until this direction is reviewed. The design PR contains this document, the isolated prototype and preview captures only.
