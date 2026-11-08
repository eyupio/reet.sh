# Compact work items and meaningful names

## Approved direction and implementation

The user requested slimmer items across Continue working, Backlog, Active and Finished, with core data visible and optional expansion, plus meaningful automatic titles. They approved the desktop/phone mock on 2026-11-07. [Desktop and phone mock](previews/compact-work-items-mock.png) is an illustrative design with synthetic records; it is not an implemented product screenshot.

- Collapsed rows show the task title, project, actual state and age. Keep action and disclosure targets accessible on touch screens.
- Expand one item to reveal prompt context, agent/model, machine, branch, and its existing actions. Details must remain available in compact mode; the current compact CSS hides disclosure entirely.
- Bound initial lists, with accurate shown/total counts, Show more and View all. Counts in the illustration are placeholders; actual controls must calculate the remaining number. Preserve search over all matching records, not only visible rows. Bound the total Continue preview as well as board columns so many project groups cannot create an unbounded page.
- Keep lifecycle/attention evidence honest. Preserve explicit Start/Retry, approvals, archive/end rules, image access, keyboard actions and saved view preferences.

## Naming findings and next implementation

The server already derives a name from an opening prompt when a create request leaves title blank (`internal/api/terminal.go`, `terminalproto.AutoTitle`). Backlog and launch-bar paths already have prompt-based title helpers. The standard `StartAgentDialog` instead sends `titleFor(tool, dir)`, bypassing the server's prompt-based fallback. Correct that path using the actual task text before attachment paths are appended, preserving retry identity and user-authored names.

Do not silently rename existing custom titles. For sessions started without a substantive prompt, naming from a later first task needs an explicit automatic-versus-manual ownership contract before any persisted rename. Avoid provider calls merely to label a row. The source of a title must remain distinct from the session lifecycle and running process.

## Acceptance and status

Implementation is complete locally. Compact rows are the unset default; explicit saved density remains respected. Board stages initially show five records each, Continue is capped at five records globally across groups, the compact project strip scrolls within the viewport rather than stretching the page, and Home Needs you shows five with full counts and access to every approval. Show more, View all and Show fewer preserve keyboard focus. Filtering and sorting operate over all records before the visible slice. Details stay available for all items, including those without a prompt or branch. Session actions remain in expanded rows and the session workspace; backlog Start/Edit/Delete expand with Details; a pending-start Retry remains visible. Launch titles use the existing prompt title helper on original task text before uploads, with agent/folder fallback and exact retry identity. Checkout is `feat/compact-work-items` in `/workspace/work/reet-compact-items`, based on merged #643 (`7f948d39`). #642 and #643 merged after seven and six green workflows respectively; deployment remains independently unverified.

The new browser acceptance seeds 200 records across sixty projects in six theme/width cases; it checks bounded rendering, full-dataset search, expansion, direct touch hit tests, more/all/fewer, focus, saved density, zero implicit writes and attention counts/approval access. Existing full-card board coverage explicitly saves that preference. The full launch suite verifies task titles and exact retries, including image uploads whose generated paths must not enter the title. Local validation passed as recorded below; inspect final exact-head CI/review before merge. The user has standing authorization to merge ready changes after exact-head CI and review pass. Never commit node_modules symlinks.


## Reproduce locally

If this workspace survives, activate `source /workspace/.reet-tools/activate.sh`. Dependencies are reused through an untracked `web/node_modules` symlink to the earlier implementation checkout. Build with `npm --prefix web run check && npm --prefix web run build`. Serve with `npm --prefix web run preview -- --host 127.0.0.1 --port 4188` (an existing preview may already occupy that port). Run browser scripts with `REET_UI_URL=http://127.0.0.1:4188`; Chromium is `/usr/bin/chromium`.

Relevant acceptance scripts are `web/tests/compact-work-items.e2e.mjs`, `home.e2e.mjs`, `session-board.e2e.mjs`, `start-agent.e2e.mjs`, `phone-zoom.e2e.mjs` and `shell-fidelity.e2e.mjs`. The new compact acceptance is included in terminal-browser CI. No backend or protocol migration is introduced, and this slice needs no connector upgrade. Controller/web rollout still needs separate verification.

## Review notes

- Initial visual review found overlapping Details/Start hit areas; controls now occupy a separate grid column. Browser acceptance checks the actual hit target at three points for each stage on desktop/phone.
- The first full phone sweep found long anchor rectangles clipped by their heading. Links/buttons now constrain their own width and ellipsis. Keep the original overflow checks enabled.
- Sixty project icons previously stretched the page despite bounded work lists. The Home project strip now scrolls within the viewport, and the large-list probe checks page height and reachability of the last project.
- Full-card regression coverage explicitly saves `compact:false` only when no preference exists; it must not overwrite view/density choices during reload. Compact-default and saved-density behavior are covered separately.
- First-message retitling of an already-created session is outside this implementation. Preserve manual names and do not infer automatic ownership just because a title looks generic. Broader attention identity/deduplication and supporting-page navigation remain separate work.


## Local validation results

- Web unit checks passed; Svelte check reported zero errors/warnings; production build passed.
- New compact-work acceptance passed: 200 records, sixty project groups, six width/theme cases, independent project-rail scrolling, full-set search, more/all/fewer and focus, disclosure hit targets, expanded Start/Edit, visible pending Retry, saved density and full approval access/counts. No implicit writes were made.
- Full Home, saved full-card board and Start an agent browser suites passed. Launch coverage checks opening-task names, image-path exclusion, blank-prompt fallback and byte-identical retry bodies.
- The repaired full phone-zoom sweep passed all 46 screens at 320/360/390/412 px. Original overflow assertions were preserved. Shell geometry passed seven widths in both themes.
- Actual implementation captures: [desktop](previews/compact-work-items-desktop.png), [phone Continue](previews/compact-work-items-phone.png). These contain synthetic records. The earlier approved mock remains separately labelled.

Local validation is not PR CI or deployed acceptance. Inspect the final pushed head's workflows and review threads before merge. No production deployment has been verified.


## PR and session handoff checkpoint

Implementation PR: [#644 — Keep Home work lists compact and name sessions from tasks](https://github.com/eyupio/reet.sh/pull/644). Branch `feat/compact-work-items`, base main after #643. Runtime changes and local acceptance were committed as `6f6040e0e13684c7bde6c16819c5d6f84f93ec8c`. The following handoff update changes documentation only; use the current GitHub PR head, not the earlier source hash, when reading CI or merging.

At this handoff the PR was open and had not been merged (it has since merged; see Merge outcome below). Local checks listed above passed. Fresh CI and review remain pending, and deployment remains unverified. The user explicitly requested a fully current handoff and a continuation prompt for a new session; do not restart the design or ask again whether the approved mock should be implemented.

Next session:

1. Read this file and `calm-workspace-continuation.md`; inspect PR #644's current head, all workflows and review threads.
2. Investigate any real check failures or valid review findings, fix them, update this handoff and repeat affected checks. Do not weaken overflow/hit-target assertions or merge a pending/failing head.
3. Confirm the merge tree matches the tested change; if main changed materially, validate the combined result. Merge only when ready and green under the user's existing authorization; no new merge confirmation is needed.
4. Record the actual merge and any verified deployment outcome. Keep later-message retitling and broader attention/navigation refinements separate unless their ownership/scope is explicitly established.

The original checkout is untouched. This worktree should have no tracked uncommitted changes after the final handoff push; `web/node_modules` is an intentional untracked symlink and must never be committed. Logs are under `/workspace/work/redesign-review/compact-*`. The continuation prompt uses repository paths so it can be used even if this local environment is replaced.


## CI follow-up on PR head `cf92445d`

- `phone-dialogs` failed in `web/tests/launch-options.e2e.mjs`: it clicked backlog **Edit** directly, but compact rows keep Edit/Delete/Start behind the **Details** disclosure by design. This suite was not in the local validation list. The test now opens "Show details for <title>" before Edit (both places; the card is collapsed again after a reload). The product behaviour is unchanged; the approved layout is not weakened.
- Codex review never completed on this PR (usage limit); there were no review threads or findings, so no review-driven change was made.
- The fix was pushed as `d10efd84`. Exact-head CI then passed: all 20 check runs were green or intentionally skipped (publish, verify and deploy are skipped on PRs), including `test`, `terminal-browser`, `phone-dialogs`, `phone-zoom`, both offline suites, `gaffer-settings`, `web`, `build` and CodeQL. There were no review threads or reviews, and the PR was mergeable/clean. `main` had not moved past the PR base (`7f948d39`), so the merged tree is the tested tree.

## Merge outcome

PR #644 was merged on 2026-11-07 as merge commit `9e9372bf1713eece6af049a14041fee49b0a045e` (head `d10efd84`, merge method: merge commit, pinned to the verified head) under the standing authorization. Post-merge `main` workflows (release, image, terminal browser, docs, Gaffer) had only just started when this was recorded and are not verified here. **Deployment is still unverified**: controller/web rollout needs its own check, and no connector upgrade is required for this slice. Later-message retitling (see Later-message retitling below) and broader attention identity/navigation remain separate, unscoped work.

## Later-message retitling

Spec: [Name Chat sessions from their first real task](../superpowers/specs/2026-11-07-chat-first-task-retitle-design.md). Plan: [implementation plan](../superpowers/plans/2026-11-07-chat-first-task-retitle.md). The change is merged (#647; the image-only follow-up #650, merge commit `568d31ca`; extra shell-quoted upload-reference test rows and the Docker CI runner fix #652, merge commit `8ca28167`). Nothing here is verified as deployed.

Contract:

- Every session has a title origin: `manual`, `prompt` or `fallback`. Existing sessions are `manual`.
- Only Chat sessions are ever renamed; Terminal sessions never are.
- The first of a session's first three accepted Chat prompts that yields a title (`terminalproto.PromptTitle`) names a `fallback` session once.
- A manual rename always wins; a rename racing the first message keeps the manual title.

Where it lives: `title_origin` and `retitle_tries` on `terminal_sessions`; `terminalproto.ResolveTitle` at create (the web client sends `titleOrigin`); `store.RetitleChatFromPrompt`, called from `PromptChat` only after the agent acknowledges a prompt (which also covers the queue drain) and from the queue drain's steered `injected` delivery in `internal/terminalbroker/chat_queue.go`. Open views show a new title on their next session-list refresh (Home polls every 15 seconds); there is no title push channel.

Known limits (deliberate):

- A trailing `Image:`/`Images:` path line (appended for uploads) is ignored when deriving a title, so image-only launches are stored `fallback` and stay renameable (issue #648). An `Image:` line anywhere but last is still ordinary text.
- `PromptTitle` strips only a fixed list of greetings and fillers, so a first message that is just "thanks" yields the title "Thanks" and ends eligibility. This is existing `PromptTitle` behaviour, not changed here.
- A message relayed into a worker session by a Gaffer conversation (queue origin `companion`) can name a fallback-titled Chat session, since it is still the first real task that session receives.
- "Continue" of an ended fallback Chat starts the new session with the old title and no claim, so it is stored `manual` and keeps the generic name.

Deployment of this change is a separate verification (controller and web rollout; no connector upgrade needed) and is not verified by merging. Attention identity/deduplication and supporting-page navigation remain separate, unscoped work.
