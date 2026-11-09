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

## Connect flow: the import step, in the skill picker (9 November 2026)

What a machine reports that the library does not have is now offered in the machine skills dialog itself, below the selection, instead of only inside each folder's details on the machine card. The section "On this machine, not in your library yet" lists those skills, each with the existing **Import privately** action. Pressing it closes the picker and opens the existing reviewed import dialog with the source folder and skill already chosen (checked: the folder and skill selects hold the right values, and the only action offered is to request a file preview). Nothing is uploaded by that step.

Rules, in `foundNotInLibrary` (`machineItems.ts`, 5 new node tests, 16 in the file): a skill counts as already in the library when an item has the same name, ignoring case; only complete skills are offered; incomplete ones are counted and explained ("1 more skill here cannot be imported yet because its files could not be read completely"); a skill reported twice is offered once, sorted; and **nothing is offered until the machine's content state is `observed`**, because an unread scan is not evidence that a skill is missing. The section only appears for a writer in accounts mode, the same condition as the Choose skills button, and is wired through `MachineLibrary` and `MachineFleet` to the existing `onimport` hand-off.

Validation: `connect-flow.e2e.mjs` gains an import-step scenario at 390px dark and 1280px light (the right skill is offered, the one already in the library is not, the incomplete one is explained and not offered, horizontal bounds hold, and it opens by keyboard into the pre-filled reviewed import) and a case proving nothing is offered while content is `pending`. `selected-library`, `machine-item-states`, `library`, `add-machine`, `control` and `machine-skill-sync` pass; `svelte-check` reports zero errors or warnings. Curated image: [phone, import step](../images/ui/connect-flow-phone-import-step.png).

**This does not meet the acceptance line "without a separate source-capture or approval screen", and that is deliberate.** The import still goes through the existing review: request a preview, check which files to keep, approve, then upload. That review is the consent for sending a machine's files to the controller, and removing or merging it is a privacy decision for the owner, not a UI tidy-up. This slice only removes the hunting: the person no longer has to find the folder's details or the "Import a machine skill…" button, and the skill is pre-chosen. Not done: importing several skills in one review, bringing the person back to the picker when an import completes (they reopen Choose skills; the new item then appears in the library list), matching by anything but name, and any automatic capture at connect time (the connector's `library-import` capture is a command run on the machine and nothing in the web flow triggers it). The Add a machine dialog still ends with "Done — show {machine}".

## Real-stack browser acceptance (9 November 2026)

Until now Stage 5 was proved only with fixture responses. `web/tests/stage5-acceptance.e2e.mjs` drives the real built web app in real Chromium against the real controller and two separately enrolled connector processes (own homes, own configs), signed in with a real session: a session row in the controller's store and the `reet_session` cookie, which is exactly what a sign-in produces. It is started by `TestStage5BrowserAcceptance` (`internal/api`, build tag `sync_contract`, `REET_STAGE5_BROWSER=1`), which lives on the Stage 4 branch (#680) because it needs that backend: **it can only run on a checkout that has both #680 and #686**. The script refuses to run on its own and is not in CI. To reproduce, from a checkout with both branches merged: `npm --prefix web ci && npm --prefix web run build`, then `REET_STAGE5_BROWSER=1 REET_UI_SCREENSHOTS=/some/dir go test -tags sync_contract ./internal/api -run TestStage5BrowserAcceptance -count=1 -timeout 15m`. Managed removals are switched on for that test controller only; the production control stays off.

**Evidence: the run passed on three consecutive executions** (the first with screenshots, then `-count=2`, 138 s for both), each covering ten phases against real data and real files:

1. Signed in; the Library shows each item once, In use, selected on no machine.
2. Choose skills on one machine: select all, save; each row says waiting or syncing, never done, and never "Up to date".
3. The connector syncs on its own: the files appear on the selected machine only, and every row now reads Installed from the machine's real report.
4. The Library shows which machine keeps each item.
5. Disabled from the Library removes the managed copy from the machine, keeps the library entry and the selection, leaves exactly one recoverable copy, creates **no** notification, and the picker reads "Kept in the library only".
6. Back to In use restores the copy with no reselection.
7. **A real partial failure:** one item's local folder is replaced by a plain file. After the controller's retry back-off it reads "Needs attention · The managed directory cannot be verified; its local files are retained." beside that item only, while the other two stay Installed. Clearing the blocker lets it sync again with no reselection.
8. The browser is closed entirely; an item is turned off through the legacy "used" flag; sync continues with no browser open (the copy is removed); a new browser sees the durable result (status To test, the selection kept, the picker's inactive state).
9. A file deleted on the machine is restored from canonical bytes and announced exactly once in the notification centre ("Restored reet-review-helper on workstation after it was deleted locally."), and a later reload and more cycles add no second notice.

Curated images: [phone, items installed](../images/ui/stage5-real-installed-phone.png), [phone, status removal](../images/ui/stage5-real-status-removal-phone.png), [phone, partial failure](../images/ui/stage5-real-partial-failure-phone.png), [desktop, restored notification](../images/ui/stage5-real-restored-notification-desktop.png).

**What running for real taught us**

- A blocked or broken item reads "Waiting to sync" for **up to a minute** before "Needs attention" appears with its reason. The controller deliberately retries a canonical target at most once a minute (`now()-60000` in `MaintainSync`) so a standing problem cannot become hundreds of failed jobs. This is intended throttling, not a bug, but during that window nothing explains the wait. The acceptance run waits the window out (62 s). Showing a reason sooner would need the connector to report a blocked item without waiting for the controller's next job; that is not built.
- Turning an item off through the legacy "used" flag lands it on **To test**, not Disabled, which is the documented status mapping, and shows the Library, picker and machine all agreeing.
- The heal step (removing the blocking file) is itself a deletion of a managed folder, so it is restored and announced as its own episode: two restore notices appear in the screenshot, one per real episode, and only the intended one is asserted.
- Item states from the controller's real report matched what the fixture tests assumed: the per-item states needed no change.

**What this does not prove**

- Both connectors are subprocesses on one machine with a faked network link, not two real machines. Sleep, reboot, real network loss, native service recovery and a second operating system were not exercised.
- The session was created directly in the store, so the password sign-in form, SSO and account creation were not run. Viewer and second-account behaviour were not run in the real stack.
- An **offline** machine was not exercised for real: the page's offline window is minutes long, so "Offline — update waiting" remains covered only by fixture tests. The empty-library state is likewise fixture-only here.
- The Library page ran at desktop width in the real stack; the phone images are the same page resized to 390 px, not a touch device. Reduced-motion and assistive-technology behaviour were not exercised.
- It needs Stage 4's backend and the managed-removal control on. With that control off, status removal does not happen, by design.

**Stage 5 acceptance is still not complete.** Remaining: the single connect/import/selection flow (the file review still stands between a found skill and the library, by design, pending an owner decision), retiring the remaining `/sync` navigation, a real offline-machine state, a real signed-in sign-in path, an acceptance run on real hardware, and the unfinished Stage 4 (#680) dependency.

## Update: real sign-in, a real offline machine, and `/sync` retired for choosing skills (9 November 2026)

**The real-stack run now signs in through the real form.** The owner account is given a password and every browser signs in at `/signin`, so the session is the one the server issues; the earlier injected-session approach is gone. A new phase 10 covers a machine that is **really offline**: its connector stops talking, and once it has been quiet past the page's 90-second window the card says offline. A selection can still be saved for it, and the rows read "Offline — update waiting" (not Installed, not an error). Nothing reaches it while it is away; when it comes back its connector catches up by itself, the files appear, and the rows read Installed, while an item that is not In use never arrives. **Eleven phases passed on three consecutive executions** (the first with screenshots, then `-count=2`, 231.6 s for both, each run about two minutes because of the real waits). Image: [phone, offline machine with its selection waiting](../images/ui/stage5-real-offline-waiting-phone.png).

**Normal navigation into `/sync` for choosing skills is retired.** The machine card's menu item "Choose skills & locations" now opens the machine's own skill dialog directly for a writer with an enrolled machine (focus returns to the card's Choose skills button). Home's "Skills & sync" now goes to the machine's card with `?choose=skills`, which opens the dialog once and then removes the parameter from the address, so a reload does not reopen it. A viewer, or a legacy mode with no accounts, keeps the plain link and the card; an unknown machine in the link opens nothing. `web/tests/choose-entry.e2e.mjs` (registered in `terminal-browser.yml`) covers these at 390 and 1280 px, the viewer, the unknown machine and focus return; `home.e2e.mjs` asserts the new target; `control`, `add-machine`, `selected-library`, `machine-item-states`, `connect-flow`, `library`, `navigation` and `supporting-nav` pass. **Untouched on purpose:** links into `/sync#sync-jobs` (job review, activity, progress), the shelf's "Sync to machines…" flows from `/skills` and `/hosting`, and the problem-card fallbacks for legacy modes. These are diagnostics or legacy entry points whose disposition is unchanged, and `/sync` itself still works.

**Still not proven.** A real viewer or second account: a viewer's library is their own and empty, so a viewer run in the real stack would prove nothing here without shared data; the viewer behaviour is covered by fixtures only. Real hardware, two real machines, sleep, reboot and real network loss. The empty-library state in the real stack. SSO and account creation. The single connect/import/selection flow still has the file-review step between a found skill and the library, pending an owner decision. Stage 4 (#680) is unfinished, so Stage 5 acceptance is not complete.

## Workspace and ongoing PR supervision

Use `/workspace/.reet-tools/activate.sh` for Node/Go; system `go` is not the compiler. Dependencies are an untracked symlink from `web/node_modules` to `/workspace/reet.sh/web/node_modules`; never commit it. Disks are constrained: use short `TMPDIR=/workspace/t`, preserve user files, and reclaim only inspected owned regenerable outputs if needed. The preview used for this branch is port 4174 because another session owns 4173; `REET_UI_URL` selects the test origin. Do not stop unrelated preview processes.

The standing PR-watch authorization remains: fix genuine failures/reviews/conflicts within scope and merge completed reviewed work only after all applicable current-head checks pass, with a fresh expected-head guard and `merge_method=merge`; verify the merge. No force pushes, branch deletion, protection/test/benchmark weakening, blind auto-merge, deployment or release. Keep #680 and this incomplete Stage 5 work draft. The signed-in owner acceptance gate for final default-on Claude activation remains unchanged. Inspect live PR state rather than treating historical checkpoints as current.

## Publication

[Draft PR #686](https://github.com/eyupio/reet.sh/pull/686) targets `main` from `feat/library-machines-stage-5`. The runtime change is `29d32f39`; `b5815a85` integrates exact main `16e8fdcb`; `ec7c2a40` adds portable screenshots and validation evidence. The following documentation commit records this PR link. Re-read the live PR for its latest head, checks, reviews and mergeability; do not reuse checks from an earlier head or mark the stage complete from this record.

To resume, fetch and check out `feat/library-machines-stage-5`, read this file and #686, then continue the remaining steps above. Fresh current-head CI is still required. #680 remains a separate incomplete Stage 4 draft, and neither PR should be merged merely because a checkpoint's checks pass. No new permission request is needed to continue the authorised Stage 4/5 work within this scope. Stages 6–7 and deployment remain outside the current request.
