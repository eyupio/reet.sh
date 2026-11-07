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

The new browser acceptance seeds 200 records across sixty projects in six theme/width cases; it checks bounded rendering, full-dataset search, expansion, direct touch hit tests, more/all/fewer, focus, saved density, zero implicit writes and attention counts/approval access. Existing full-card board coverage explicitly saves that preference. The full launch suite verifies task titles and exact retries, including image uploads whose generated paths must not enter the title. Finish final stable-build validation and inspect exact-head CI/review before merge. The user has standing authorization to merge ready changes after exact-head CI and review pass. Never commit node_modules symlinks.


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
