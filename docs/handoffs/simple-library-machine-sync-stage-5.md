# Stage 5: Library and Machines UX

Parent: [product contract and acceptance matrix](simple-library-machine-sync.md).

**Status: authorised first UI slice, incomplete and draft.** On 9 November 2026 the owner asked to start Stage 5, agreed to work against existing APIs while Stage 4 remains unfinished, and requested a PR-linked handoff for a new session. This overrides earlier checkpoints saying Stage 5 was unstarted; it does not waive the Stage 4 dependency for final Stage 5 acceptance. Do not mark either stage complete or start Stage 6 from this checkpoint.

## Branch and review boundary

- Branch: `feat/library-machines-stage-5`, owned worktree `/workspace/work/stage5`.
- Starting main: `37376deb0ed31604e47eae281dfb376fdb01a185`.
- This branch is independent of the unfinished [Stage 4 draft #680](https://github.com/eyupio/reet.sh/pull/680); it does not import or enable that branch's gated removals.
- PR link and final validation are recorded in the publication section below before handoff.
- Keep this PR draft until its intended review scope and dependency boundary are agreed. Passing UI checks does not establish completed automatic removal, restoration, factual notifications or live-machine sync.

## Implemented first slice

An enrolled account writer can choose a machine's library skills directly from its **Choose skills** button on the Machines card. This works for a disconnected machine with retained reported folders. The basic action opens a dialog instead of navigating through Sync, comparisons, previews or approvals. On these cards the redundant fallback **Set up sync** link is suppressed; existing progress, manual sync and diagnostic actions remain available.

`MachineLibrary.svelte` supplies the shared dialog shell, machine heading, Escape/close behavior and focus return to the card button. It reuses `SelectedLibrary.svelte`, whose optional `host` and `snapshot` props restrict the picker to that machine's enrolled, supported reported folders. A single folder is selected automatically; multiple folders require an explicit choice. Older global `/sync` usage remains supported by the same component, without the host restriction. No second selection API, executor, sync policy or optimistic completion flag is introduced.

The editor loads the existing canonical library and versioned selection, preserves explicit item IDs, offers search and Select all In use, includes pending-content In use items in intent, and retains existing inactive selections. An unselected inactive item cannot be newly chosen. Failed initial reads remain visible with retry even when no folders were loaded; failed saves keep the choices available and show no success message. A successful save says only that selection was saved and available updates run when connected. It never labels an offline selection as verified Up to date. Closing/reopening reads the persisted server selection.

Machine-card accessible IDs now encode the host, fixing a reproduced failure for names containing spaces: raw whitespace split `aria-labelledby` into nonexistent references and made the card unnamed. Native checkbox keyboard behavior, dialog focus and machine-scoped options are exercised in the browser checks.

No skill authoring, status, likes, lists or existing library routes are removed. No new Library page, install wizard, agent provisioning or restored notification is claimed by this slice.

## Validation and screenshots

Validation is recorded after the final source checks complete. The browser harness uses real Chromium and the production build with mocked HTTP responses. It proves UI behavior and exact selection requests, not a signed-in machine applying files.

The extended `web/tests/selected-library.e2e.mjs` remains in the existing terminal-browser workflow. It retains global `/sync` selection coverage and adds Machines-only offline selection at 320/390/1280px in light/dark themes, a second-machine isolation check, failed save/retry, reopen persistence, search, Escape/focus return, keyboard operation and horizontal bounds. Screenshots are captured through `REET_UI_SCREENSHOTS`; curated evidence is linked below after verification.

## Resume in a new session

1. Read this file, the parent contract, current PR metadata and repository instructions. Refresh remote head/base/reviews/checks before editing; preserve any concurrent commits. The owner has not requested subagents.
2. Reconcile this branch with current main and the eventual Stage 4 branch without losing either handoff's current status. Stage 4's controller approval/acknowledgement fences, recovery, ownership migration, debounced restoration and durable notifications are still prerequisites for final Stage 5 behavior. Never fabricate these states in UI fixtures and present them as product capabilities.
3. Complete the minimal Library view using canonical identities while preserving authored/imported items, statuses, lists, likes and supported legacy URLs. Avoid exposing a duplicate shelf/authoring/selection journey as a new permanent flow. The current `/skills` and `/hosting` pages remain in place.
4. Complete the single connect/import/selection journey. The current Add a machine dialog, Import a machine skill action and advanced setup remain separate existing entry points; this slice only joins destination selection to the machine card. Registered custom roots and per-app compatibility must follow actual backend support. The picker retains the existing supported adapter set, including shared skills for Codex; it does not invent support for scan-only legacy Codex folders.
5. Replace remaining normal-sync navigation into `/sync` with the minimal flow once equivalent behavior and verified progress exist. Preserve advanced diagnostics behind details. Inventory connection state, saved selection, pending content, verified deployment and durable failure events are distinct; don't infer successful deployment from a saved selection or a recent heartbeat.
6. Finish Stage 5 acceptance: empty Library/Machines, online/offline, partial per-item failure, status removal, restored notifications, refresh/browser close, keyboard and phone/desktop states. Record real backend and browser evidence separately. Capture final screenshots and update the parent ledger only when the full stage is accepted.

## Workspace and ongoing PR supervision

Use `/workspace/.reet-tools/activate.sh` for Node/Go; system `go` is not the compiler. Dependencies are an untracked symlink from `web/node_modules` to `/workspace/reet.sh/web/node_modules`; never commit it. Disks are constrained: use short `TMPDIR=/workspace/t`, preserve user files, and reclaim only inspected owned regenerable outputs if needed. The preview used for this branch is port 4174 because another session owns 4173; `REET_UI_URL` selects the test origin. Do not stop unrelated preview processes.

The standing PR-watch authorization remains: fix genuine failures/reviews/conflicts within scope and merge completed reviewed work only after all applicable current-head checks pass, with a fresh expected-head guard and `merge_method=merge`; verify the merge. No force pushes, branch deletion, protection/test/benchmark weakening, blind auto-merge, deployment or release. Keep #680 and this incomplete Stage 5 work draft. The signed-in owner acceptance gate for final default-on Claude activation remains unchanged. Inspect live PR state rather than treating historical checkpoints as current.

## Publication

Pending draft PR creation and final evidence update in this session.
