# Stage 5: Library and Machines UX

Parent: [product contract and acceptance matrix](simple-library-machine-sync.md).

**Status: authorised first UI slice, incomplete and draft.** On 9 November 2026 the owner asked to start Stage 5, agreed to work against existing APIs while Stage 4 remains unfinished, and requested a PR-linked handoff for a new session. This overrides earlier checkpoints saying Stage 5 was unstarted; it does not waive the Stage 4 dependency for final Stage 5 acceptance. Do not mark either stage complete or start Stage 6 from this checkpoint.

## Branch and review boundary

- Branch: `feat/library-machines-stage-5`, owned worktree `/workspace/work/stage5`.
- Starting main: `37376deb0ed31604e47eae281dfb376fdb01a185`.
- Integrated main: `16e8fdcb0e55d3381a8b18376682671c0e3d885f` through merge `b5815a85`, including the #685 logo correction. Local runtime and browser checks below ran on this combined source without conflicts.
- This branch is independent of the unfinished [Stage 4 draft #680](https://github.com/eyupio/reet.sh/pull/680); it does not import or enable that branch's gated removals.
- Draft PR: [#686 — choose library skills directly from Machines](https://github.com/eyupio/reet.sh/pull/686). Validation and publication checkpoints are recorded below.
- Keep this PR draft until its intended review scope and dependency boundary are agreed. Passing UI checks does not establish completed automatic removal, restoration, factual notifications or live-machine sync.

## Implemented first slice

An enrolled account writer can choose a machine's library skills directly from its **Choose skills** button on the Machines card. This works for a disconnected machine with retained reported folders. The basic action opens a dialog instead of navigating through Sync, comparisons, previews or approvals. On these cards the redundant fallback **Set up sync** link is suppressed; existing progress, manual sync and diagnostic actions remain available.

`MachineLibrary.svelte` supplies the shared dialog shell, machine heading, Escape/close behavior and focus return to the card button. It reuses `SelectedLibrary.svelte`, whose optional `host` and `snapshot` props restrict the picker to that machine's enrolled, supported reported folders. A single folder is selected automatically; multiple folders require an explicit choice. Older global `/sync` usage remains supported by the same component, without the host restriction. No second selection API, executor, sync policy or optimistic completion flag is introduced.

The editor loads the existing canonical library and versioned selection, preserves explicit item IDs, offers search and Select all In use, includes pending-content In use items in intent, and retains existing inactive selections. An unselected inactive item cannot be newly chosen. Failed initial reads remain visible with retry even when no folders were loaded; failed saves keep the choices available and show no success message. A successful save says only that selection was saved and available updates run when connected. It never labels an offline selection as verified Up to date. Closing/reopening reads the persisted server selection.

Machine-card accessible IDs now encode the host, fixing a reproduced failure for names containing spaces: raw whitespace split `aria-labelledby` into nonexistent references and made the card unnamed. Native checkbox keyboard behavior, dialog focus and machine-scoped options are exercised in the browser checks.

No skill authoring, status, likes, lists or existing library routes are removed. No new Library page, install wizard, agent provisioning or restored notification is claimed by this slice.

## Validation and screenshots

On the integrated source, `npm --prefix web run check` passes all unit tests and svelte-check with zero errors/warnings; the production build passes. The extended selected-library browser suite passes all three legacy `/sync` widths and all six Machines width/theme cases. The full existing `control.e2e.mjs` suite also passes, including partial queue/save failures, no automatic apply, responsive light/dark layouts and viewer controls. Strict MkDocs and rendered checks pass for 51 pages; `git diff --check` passes. No new repository-wide Go race sweep or live-machine acceptance is claimed.

The browser harness uses real Chromium and the production build with mocked HTTP responses. It proves UI behavior and exact selection requests, not a signed-in machine applying files. The new tests initially failed on machine names containing spaces, reproducing the accessible-ID issue fixed above. The broader control fixture initially returned an unmocked library 404, previously hidden by the selection component; it now supplies the real endpoint's empty-library shape. No existing control assertions or timeouts were relaxed.

The extended `web/tests/selected-library.e2e.mjs` remains in the existing terminal-browser workflow. It retains global `/sync` selection coverage and adds Machines-only offline selection at 320/390/1280px in light/dark themes, second-machine isolation, initial library-load retry, failed save/retry, reopen persistence, explicit choice for multiple folders, search, Escape/focus return, keyboard checkbox operation and horizontal bounds. Screenshot output uses `REET_UI_SCREENSHOTS`; these curated images were visually inspected and committed:

- [Phone, offline selection saved](../images/ui/machine-library-phone-dark.png).
- [Desktop, offline selection saved](../images/ui/machine-library-desktop-light.png).
- [Phone, failed save retaining choices](../images/ui/machine-library-save-error.png).

Local evidence: `/workspace/work/stage5-web-check.log`, `stage5-web-build.log`, `stage5-selection-browser.log`, `stage5-control-browser.log`, plus the complete screenshot set under `/workspace/work/stage5-shots`. These local files may not survive a replacement environment; the source, test scripts and three images above travel with the PR.

## Resume in a new session

1. Read this file, the parent contract, current PR metadata and repository instructions. Refresh remote head/base/reviews/checks before editing; preserve any concurrent commits. The owner has not requested subagents.
2. Reconcile this branch with current main and the eventual Stage 4 branch without losing either handoff's current status. Stage 4's controller approval/acknowledgement fences, recovery, ownership migration, debounced restoration and durable notifications are still prerequisites for final Stage 5 behavior. Never fabricate these states in UI fixtures and present them as product capabilities.
3. Complete the minimal Library view using canonical identities while preserving authored/imported items, statuses, lists, likes and supported legacy URLs. Avoid exposing a duplicate shelf/authoring/selection journey as a new permanent flow. The current `/skills` and `/hosting` pages remain in place.
4. Complete the single connect/import/selection journey. The current Add a machine dialog, Import a machine skill action and advanced setup remain separate existing entry points; this slice only joins destination selection to the machine card. Registered custom roots and per-app compatibility must follow actual backend support. The picker retains the existing supported adapter set, including shared skills for Codex; it does not invent support for scan-only legacy Codex folders.
5. Replace remaining normal-sync navigation into `/sync` with the minimal flow once equivalent behavior and verified progress exist. Preserve advanced diagnostics behind details. Inventory connection state, saved selection, pending content, verified deployment and durable failure events are distinct; don't infer successful deployment from a saved selection or a recent heartbeat.
6. Finish Stage 5 acceptance: empty Library/Machines, online/offline, partial per-item failure, status removal, restored notifications, refresh/browser close, keyboard and phone/desktop states. Record real backend and browser evidence separately. Capture final screenshots and update the parent ledger only when the full stage is accepted.

## CI correction: reopen race in the machine skills dialog (9 November 2026)

Current-head CI on `7d4af79c` failed `terminal-browser` in `selected-library.e2e.mjs`: after Escape, an immediate reopen from the **Choose skills** button left no dialog. It reproduced locally in roughly one run in three, and in 5 of 40 scripted close/reopen cycles. The cause was a product race, not only a test timing issue. A native `<dialog>` reports `close` a task after it closes, so an Enter press in that window set the new dialog state and the old dialog's late `close` event then cleared it. `MachineLibrary.svelte` now ignores a late `close` from an already-destroyed instance, and `MachineFleet.svelte` keys the dialog so every open is a fresh instance. The existing test keeps all assertions and timeouts and gains a deterministic same-task close-then-reopen case. Locally the full selected-library suite passed five consecutive runs (9 of 9 cases each), `npm --prefix web run check` and the control browser suite pass. Fresh current-head CI is still required; this records no new acceptance of Stage 5.

## Per-item machine state beside each item (9 November 2026)

The machine skills dialog now says what is really happening to each saved selection, beside the item, instead of only a checkbox. `web/src/lib/machineItems.ts` maps one library item, its saved selection, the folder's target, its jobs and whether the machine is online to one state, using only evidence the controller already holds:

| State | When |
| --- | --- |
| Kept in the library only | Selected but not In use. Says it is not installed and any copy reet.sh installed is removed; the selection is kept. |
| Waiting for complete content | The item's content has not been captured completely. |
| Offline — update waiting | Selected, not yet seen on the machine, and the machine is offline. |
| Waiting to sync | The same while online, or before the controller has planned the selection. |
| Syncing | A queued, running or awaiting-approval job exists for that folder. |
| Needs attention | The newest finished plan has a problem for that item's slug (shown verbatim), or the newest finished apply failed and names the item. One item's problem never appears on another. |
| Installed | The item is in the controller's desired set and present in the machine's last report. |

Unselected items show nothing. The state is tied to its checkbox with `aria-describedby`, so the checkbox's own accessible name is unchanged. Rows keep the saved selection's state while the person edits checkboxes; the newest `snapshot` from the machine card wins, so a refresh updates the rows.

**Honest limit:** the label is "Installed", with "Seen in this machine's last report", never "Up to date". The controller knows an item appeared in the machine's last inventory, not that its bytes were compared with the current revision, and no API exposes that comparison yet. A byte-verified deployment state needs that evidence to be reported by the connector and read by the controller; it is not claimed here. Item states do not yet include "removal pending" for an item the controller has told the machine to drop, nor the restored-after-local-deletion event (that appears in the notification centre, from Stage 4).

Validation: 11 node tests cover every state and its ordering, the per-item problem isolation, newest-job-wins, and that the wording never claims byte verification (`npm --prefix web run check` passes with zero svelte-check errors or warnings). The new `web/tests/machine-item-states.e2e.mjs` runs in real Chromium with fixture responses at 320, 390 and 1280px in light and dark for the offline case, plus an online syncing case, checking each state, the failure reason beside its own item, no "Up to date" text, no horizontal overflow, the `aria-describedby` link and keyboard toggling with Space; it is registered in `terminal-browser.yml` next to the selected-library script. `selected-library.e2e.mjs` (9 cases) and `control.e2e.mjs` still pass. Inspecting the screenshots caught a missing separator between label and detail, which is fixed. These are fixture-driven UI checks, not a signed-in machine applying files. Curated images: [phone, offline states](../images/ui/machine-item-states-phone-offline.png), [desktop, offline states](../images/ui/machine-item-states-desktop-offline.png), [phone, syncing](../images/ui/machine-item-states-phone-syncing.png).

Still open for Stage 5 acceptance: the minimal Library view, the single connect/import/selection flow, retiring the remaining `/sync` navigation, the empty-state and status-removal screenshots, browser close/reopen against durable state, and an acceptance run against a real signed-in machine. Stage 5 is not complete, and its dependency on the unfinished Stage 4 (#680) stands.

## Minimal Library page (9 November 2026)

**Decision (owner, via the decision prompt):** the Library is a new `/library` page, listed first under **More → Skills & distribution**. `/skills` and `/hosting` are deliberately left exactly as they were for now and are recorded below as legacy entry points; nothing redirects yet, so no existing bookmark, list, like or authoring flow changed.

`web/src/lib/Library.svelte` reads the canonical library (`GET /api/v2/library`) and shows one entry per logical item:

- **Status**: a select over the four product statuses (In use, To test, On the shelf, Disabled). A change sends the item's `expectedVersion`, updates the row from the server's answer, and keeps its machine selections. A stale change is refused by the server (409): the row says the item changed somewhere else, the select returns to its real value and the list is re-read. Viewers see everything but cannot change a status.
- **Search and status filter** combine, with per-status counts over the whole library. Every inactive item is findable; nothing is hidden by All.
- **Machines**: from the saved selections of each enrolled folder, one entry per machine ("Selected on Dev laptop, Office desktop", "Not selected on any machine", or "… not installed while to test" for an inactive item), with a link to Machines. This is saved intent, not installation; installed state stays on Machines. A failure reading selections is said once with a retry and never hides the library.
- **Shelf details migration preserved**: written-here (private or public), liked, lists and tags, from the item's metadata. Likes and lists are shown, not edited, here; editing them remains on `/skills`.
- **History**: read on demand per item from `GET /api/v2/library-history/{item}`: when each save happened, and whether it became the current version or was kept as another version, with "time estimated" when the save time came from a fallback clock.
- **Waiting content** (pending capture or a content problem) is said beside the item. **Empty** library offers Connect a machine and (for writers) Write a skill. A failed read shows the reason with a retry.

`web/embed.go` now serves `/library` as a private SPA route (and disallows it in `robots.txt`); `web/embed_test.go` covers it, and the Go web tests pass.

**Legacy entry points and their disposition:** `/skills` (shelf, likes, lists, bookmarks, tags, notes, visibility), `/hosting` (authoring) and `/sync` remain fully working. They are not retired by this slice. Authoring is only linked, from the empty state; creating or editing an item is not duplicated in Library. **No restore control is offered**: the API has history but no restore endpoint, and none is invented here.

Validation: 7 node tests (`libraryView.test.mjs`: statuses, search plus filter, counts, machines per item, notes, history lines) and `npm --prefix web run check` (zero svelte-check errors or warnings). `web/tests/library.e2e.mjs` runs in real Chromium with fixture responses: the populated page at 320, 390 and 1280px in light and dark, search and filter, a status change with its exact request, a stale-change refusal, history open and close by keyboard, empty history, the empty library, a read failure and retry, a machine-selection failure, a viewer, and keyboard use of the filters. It is registered in `terminal-browser.yml`. `navigation.e2e.mjs` and `supporting-nav.e2e.mjs` were updated for the new More entry and pass, as do `machine-item-states`, `selected-library` and `sync-vocabulary`. Curated images: [phone, populated](../images/ui/library-page-phone-dark.png), [desktop, populated](../images/ui/library-page-desktop-light.png), [desktop, history open](../images/ui/library-page-history-desktop.png), [phone, empty](../images/ui/library-page-empty-phone.png). These are fixture-driven UI checks, not a signed-in controller.

Still open for Stage 5 acceptance: the single connect/import/selection flow, retiring the remaining `/sync` navigation, status-removal and partial-failure screenshots on Machines with real data, browser close/reopen against durable state, and an acceptance run against a real signed-in machine. Stage 5 is not complete, and its dependency on the unfinished Stage 4 (#680) stands. #687 adds a Prompts entry to the same More list and edits the same expectation lines in `navigation.e2e.mjs`; whichever merges second needs a one-line reconciliation.

## Connect flow: selection follows the connected machine (9 November 2026)

Add a machine already waits for the machine's first scan before it says Ready, and a machine with no reported folder is not drawn as a card at all (checked against the real page, not assumed). So a machine is only ever offered a skill picker after its first scan. This slice therefore tightens the hand-off from connecting to choosing, without changing the Add a machine dialog's labels or behaviour (its many assertions are untouched):

- **The next step is named.** A just-added card now reads "Just added — choose its skills next" (for a writer in accounts mode with an enrolled machine, the same condition as its Choose skills button), and that button is the card's primary action while the card is highlighted. Other cards keep the plain button.
- **The machine skills dialog follows live machine data.** Its folders are derived from the current snapshot instead of being read once when it opened. A single folder is chosen automatically (tried once per folder; a failed read is retried by hand), and a folder that a later scan reports while the dialog is open joins the list without closing it. The folder already chosen and its ticked skills never move. A failed library read keeps its message and retry and does not auto-continue (this was caught by the existing `selected-library` suite and fixed with an explicit library-ready guard).

Validation: `web/tests/connect-flow.e2e.mjs` (registered in `terminal-browser.yml`) runs in real Chromium at 390px dark and 1280px light: a single folder is chosen and the library offered without a manual choice, the saved request is exactly the ticked items, and a second folder arriving while the dialog is open adds an option (three, with the placeholder) while the choice and ticks stay. With the picker change reverted the scenario fails at the second folder (expected 3 options, received 2), and passes with it. `add-machine.e2e.mjs` gains assertions for the hint and the primary button, and it, `selected-library`, `machine-item-states`, `library` and `control` pass; `svelte-check` reports zero errors or warnings. Curated image: [phone, dialog after selection](../images/ui/connect-flow-phone-live-folders.png).

**Still not one flow.** Connecting, importing and selecting are still three entry points: Add a machine, the machine's Private skills / Import a machine skill actions, and Choose skills. Bringing a machine's existing skills into the library is an explicit import (discovery alone is not a capture), and it has not been folded into the connect dialog or the picker; doing that needs the import entry points reworked, which this slice deliberately does not attempt. The Add a machine dialog's final button still says "Done — show {machine}" and does not open the picker by itself. `/sync` navigation is unchanged.

## Workspace and ongoing PR supervision

Use `/workspace/.reet-tools/activate.sh` for Node/Go; system `go` is not the compiler. Dependencies are an untracked symlink from `web/node_modules` to `/workspace/reet.sh/web/node_modules`; never commit it. Disks are constrained: use short `TMPDIR=/workspace/t`, preserve user files, and reclaim only inspected owned regenerable outputs if needed. The preview used for this branch is port 4174 because another session owns 4173; `REET_UI_URL` selects the test origin. Do not stop unrelated preview processes.

The standing PR-watch authorization remains: fix genuine failures/reviews/conflicts within scope and merge completed reviewed work only after all applicable current-head checks pass, with a fresh expected-head guard and `merge_method=merge`; verify the merge. No force pushes, branch deletion, protection/test/benchmark weakening, blind auto-merge, deployment or release. Keep #680 and this incomplete Stage 5 work draft. The signed-in owner acceptance gate for final default-on Claude activation remains unchanged. Inspect live PR state rather than treating historical checkpoints as current.

## Publication

[Draft PR #686](https://github.com/eyupio/reet.sh/pull/686) targets `main` from `feat/library-machines-stage-5`. The runtime change is `29d32f39`; `b5815a85` integrates exact main `16e8fdcb`; `ec7c2a40` adds portable screenshots and validation evidence. The following documentation commit records this PR link. Re-read the live PR for its latest head, checks, reviews and mergeability; do not reuse checks from an earlier head or mark the stage complete from this record.

To resume, fetch and check out `feat/library-machines-stage-5`, read this file and #686, then continue the remaining steps above. Fresh current-head CI is still required. #680 remains a separate incomplete Stage 4 draft, and neither PR should be merged merely because a checkpoint's checks pass. No new permission request is needed to continue the authorised Stage 4/5 work within this scope. Stages 6–7 and deployment remain outside the current request.
