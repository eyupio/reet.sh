# Install a Hosted Skill from the Session Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From the session skill picker, a person installs a published reet-hosted skill into the session agent's user-level skills folder on that machine, confirming the connector's previewed plan inside the picker.

**Architecture:** One new store operation merges the skill into the folder's saved sync set and queues a `preview` job in a single transaction, and a second undoes that merge on cancel. Two thin HTTP routes expose them under the terminal session. The picker gains an install panel that follows the job with the existing `GET /api/v2/sync` and approves it with the existing `PATCH /api/v2/sync/jobs`. The controller writes no files; the connector's existing apply does.

**Tech Stack:** Go (`internal/store`, `internal/api`), SQLite, Svelte 5 and TypeScript (`web/src/lib/terminal`), node:test, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-01-session-skills-install-design.md` (builds on `2026-11-01-session-skills-design.md`).

## Global Constraints

- Installs queue a job of kind `preview` only, never `auto` (`FinishSyncJob` turns an additive `auto` plan into a queued apply with no approval).
- The controller adds no code that writes files on the machine; applying stays the connector's job.
- The merge only adds: an existing saved set keeps its skills and its mode; an `additive` set is created only when the folder had none.
- Routes are registered with `s.terminal(...)`. That wrapper already refuses viewers, non-browser tokens and a foreign origin, and an unknown or foreign session answers `terminalError(terminalbroker.ErrDenied)` (403), exactly as `terminalSessionSkills` does.
- Only the signed-in account's own **published** hosted skills can be installed, through `SyncHostedFor`.
- The install target is the session agent's user-level environment on that machine: `environments.agent == session tool`, `project == ''`, host matched after machine-alias resolution, with a complete scan from the last 14 days, and `store.Syncable(agent)`.
- The picker shows an Approve button only when every plan action is a plain add (`add` true, `replace` false) of the requested slug.
- Cancelling an install leaves no trace in the saved set when the saved set's revision is still the one the install produced.
- No new dependency. No new `localStorage` or `sessionStorage` key. Touch targets at least 44 px on coarse pointers. Skill names, messages and plan text render as text only.

## Review Focus

Failure modes the spec implies that a person would hit, each pinned by a test in the task that owns the code:

1. The machine is offline or busy: the job waits, the panel says so and offers Cancel; nothing is cancelled for the person. (Tasks 1, 4)
2. A plan with a replace, a removal or another skill: no one-press Approve, a hand-off to the Sync page instead. (Tasks 3, 4)
3. A second install of a different skill while one is pending: refused with the saved set and pending job untouched. (Tasks 1, 2)
4. Cancel: a queued or awaiting job reverts the merge and a set the install created; a running job cannot be cancelled; a set someone else changed since is left alone. (Task 1)
5. The skill is unpublished or deleted between listing and pressing Install: refused, and the picker reloads. (Tasks 2, 4)
6. A merged or renamed machine still resolves its target; a Codex or Cline session (not syncable) gets a plain refusal. (Tasks 1, 2)

## File Structure

- Create `internal/store/store_skill_install.go` and `internal/store/store_skill_install_test.go`: the merge helpers, `InstallHostedSkillFor`, `CancelInstallFor`.
- Modify `internal/store/store.go`: the `skill_installs` table in the schema statement list (next to `sync_jobs`).
- Create `internal/api/terminal_skills_install.go` and `terminal_skills_install_test.go`: the two handlers.
- Modify `internal/api/terminal.go` (route list) and `internal/api/terminal_skills.go` (`published` on `SessionSkill`) with its test.
- Modify `web/src/lib/terminal/skills.ts`, `skills.test.mjs`, `api.ts`.
- Create `web/src/lib/terminal/SkillInstallPanel.svelte`; modify `SkillPicker.svelte`, `ChatPane.svelte`, `TerminalPane.svelte`.
- Modify `web/tests/chat.e2e.mjs`, `web/tests/terminal-organisation.e2e.mjs`, `README.md`, `docs/terminal-workspace.md`.

---

### Task 1: The store operations (Go)

**Files:**
- Create: `internal/store/store_skill_install.go`
- Modify: `internal/store/store.go` (add one `CREATE TABLE IF NOT EXISTS` statement after `sync_jobs`)
- Test: `internal/store/store_skill_install_test.go`

**Interfaces:**
- Consumes: `SyncSkill`, `SyncTarget`, `Syncable(agent string) bool`, `(*Store).SyncHostedFor(uid int64, origin string) ([]SyncSkill, error)`, `SyncAgentsFor`, `ResolveMachineHostFor`, the tables `environments`, `environment_scans`, `sync_targets`, `sync_policies`, `sync_options`, `sync_jobs`; `syncID()`, `now()`, `(*Store).withTx`.
- Produces:
  - `type InstallError struct{ Code, Message string }` with `func (e *InstallError) Error() string`. Codes: `unpublished`, `connector-offline`, `no-scan`, `unsupported-agent`, `follows-source`, `install-in-progress`, `already-installed`, `cannot-cancel`.
  - `type InstallRequest struct{ Tool, Host, AgentID, Slug, Origin string }`: `AgentID` is the session machine's sync agent id, `Host` its resolved machine name.
  - `type InstallResult struct{ JobID string; Reused bool; Directory, Host string }`.
  - `func MergeDesired(existing []SyncSkill, add SyncSkill) []SyncSkill` and `func RemoveDesired(existing []SyncSkill, slug string) []SyncSkill` (pure, keep order, never mutate the argument).
  - `func (s *Store) InstallHostedSkillFor(uid int64, in InstallRequest) (InstallResult, error)`.
  - `func (s *Store) CancelInstallFor(uid int64, jobID string) (reverted bool, err error)`.
  - Table `skill_installs(job_id TEXT PRIMARY KEY REFERENCES sync_jobs(id) ON DELETE CASCADE, user_id INTEGER NOT NULL, target_id TEXT NOT NULL, slug TEXT NOT NULL, created_target INTEGER NOT NULL DEFAULT 0, revision TEXT NOT NULL)`.

Messages (the `Message` field, `{host}` and `{tool}` filled in; the picker shows them as given):

| code | message |
| --- | --- |
| `unpublished` | `Publish this skill to install it.` |
| `connector-offline` | `{host} isn't connected to reet right now, so it can't install this skill.` |
| `no-scan` | `reet hasn't read {host}'s {tool} skills folder recently. Run a scan on {host}, then try again.` |
| `unsupported-agent` | `Skills can't be installed for {tool} from reet yet.` |
| `follows-source` | `{host}'s skills follow another machine, so this skill would be removed the next time it syncs.` |
| `install-in-progress` | `Another install is waiting on {host}. Finish or cancel it first.` for a pending install of a different skill on this folder; `{host} is busy with another sync job. Finish or cancel it, then try again.` when `QueueSyncJob`'s machine-busy rule is what refused |
| `already-installed` | `This skill is already on {host} for {tool}.` |
| `cannot-cancel` | `This install is already running and can't be cancelled.` |

Behaviour of `InstallHostedSkillFor`, in one `withTx` (the existing `QueueSyncJob` uses its own transaction, so copy its machine and target host checks and its exclusivity count into the same transaction rather than calling it): `unsupported-agent` when `!Syncable(in.Tool)`; the target is the environment above, else `no-scan`; `connector-offline` when the agent is unknown, paused, or not seen recently (use the same recency test the sync UI uses for "online"; find it before choosing a threshold); the skill must be in `SyncHostedFor` (published), else `unpublished`; `already-installed` when the target's actual skills already contain the slug; `follows-source` when `sync_policies.follow_source` is set; a pending job for the target whose `skill_installs.slug` equals the slug is returned with `Reused: true`; any other pending job for the target gives `install-in-progress`; otherwise merge into `sync_targets.desired` (new `revision` via `syncID()`; create the row with source `hosted` and name `My published reet.sh skills` when none exists, with `sync_options.mode = 'additive'` and `sync_policies.follow_source = 0`; keep an existing mode), insert a `preview` job with the new revision, and insert the `skill_installs` row (`created_target` true when this call created the set, `revision` the new revision). If the machine-busy count is non-zero, return `install-in-progress` with the busy sentence and write nothing.

Behaviour of `CancelInstallFor`, in one `withTx`: the job must belong to the account and have a `skill_installs` row; a `running` job or one already finished returns `cannot-cancel`; set `status='cancelled'` (same update as `ControlSyncJob`'s cancel); if `sync_targets.revision` equals the row's `revision`, remove the slug (new revision), or delete the whole saved set (`sync_targets`, `sync_policies`, `sync_options` rows for the target) when `created_target`, and return `reverted` true; otherwise leave the set and return false.

- [ ] **Step 1: Write the failing tests** in `store_skill_install_test.go`, using the same store setup as `store_sync_automation_test.go` (find its helper that enrols an agent, records a complete scan for an environment and saves a published hosted skill):
  - `TestMergeDesiredKeepsOrderAddsOnceAndDoesNotMutate`, `TestRemoveDesiredDropsOneSlug` (pure, table-driven).
  - `TestInstallCreatesAdditiveSetAndQueuesPreview`: no saved set before; after, `sync_options.mode` is `additive`, `follow_source` 0, desired is exactly the skill, one job with kind `preview` and status `queued`, a `skill_installs` row with `created_target` 1.
  - `TestInstallKeepsAnExistingSetAndMode`: a saved `mirror` set with two skills keeps both plus the new one, mode still `mirror`, `created_target` 0.
  - `TestInstallRefusals`: one subtest per code `unpublished`, `connector-offline`, `no-scan` (no scan, an incomplete scan, a scan older than 14 days), `unsupported-agent` (`codex`), `follows-source`, `already-installed`; each leaves the tables unchanged.
  - `TestInstallSameSkillTwiceReusesTheJob` and `TestInstallDifferentSkillWhilePendingIsRefused` (saved set and pending job untouched; the message is the folder-level sentence).
  - `TestInstallRefusedWhenTheMachineIsBusyWithAnotherJob` (an active `apply` job on the agent gives the busy sentence and writes nothing).
  - `TestInstallResolvesAMergedMachineAlias`.
  - `TestCancelRevertsTheMerge` (existing set: skill removed again, mode kept), `TestCancelRemovesASetTheInstallCreated`, `TestCancelLeavesASetSomeoneElseChanged` (returns `reverted` false, job cancelled), `TestCancelRefusesARunningJob`, `TestCancelAnAwaitingApprovalJob`.
- [ ] **Step 2: Run to verify they fail.** Run: `go test ./internal/store -run 'MergeDesired|RemoveDesired|Install|Cancel' -count=1`. Expected: build failure, `undefined: MergeDesired`.
- [ ] **Step 3: Implement** the table, the helpers and the two methods in `store_skill_install.go` to the behaviour above; add the `skill_installs` statement to the schema list in `store.go`.
- [ ] **Step 4: Run to verify they pass.** Run: `gofmt -l internal/store && go vet ./internal/store && go test ./internal/store -count=1 -race`. Expected: no gofmt output, `ok`.
- [ ] **Step 5: Commit.**

```bash
git add internal/store/store.go internal/store/store_skill_install.go internal/store/store_skill_install_test.go
git commit -m "Install a published hosted skill on a machine through one preview job"
```

---

### Task 2: The routes and the `published` field (Go)

**Files:**
- Create: `internal/api/terminal_skills_install.go`
- Modify: `internal/api/terminal.go` (two routes after the Phase 1 `.../skills` route), `internal/api/terminal_skills.go` (`published`)
- Test: `internal/api/terminal_skills_install_test.go`, `internal/api/terminal_skills_test.go`

**Interfaces:**
- Consumes: `store.InstallRequest`, `store.InstallResult`, `store.InstallError`, `(*Store).InstallHostedSkillFor`, `(*Store).CancelInstallFor` (Task 1); the Phase 1 handler's session, agent and alias lookup in `terminalSessionSkills`; `s.eventRequest(r, action, detail, fields)`; the base URL the server uses for `SyncHostedFor`'s `origin` (find how `saveDesired` obtains it).
- Produces:
  - `SessionSkill.Published bool` with JSON tag `published`: true for a machine skill and for a hosted skill with `Published` true.
  - `func (s *server) terminalSkillInstall(w http.ResponseWriter, r *http.Request)`: `POST /api/v2/terminal/sessions/{id}/skills/install`, body `{"skill":"reet:<slug>"}`, answers `{"job":"<id>","reused":false,"directory":"<folder>","host":"<machine>"}`.
  - `func (s *server) terminalSkillInstallCancel(w http.ResponseWriter, r *http.Request)`: `POST /api/v2/terminal/sessions/{id}/skills/install/cancel`, body `{"job":"<id>"}`, answers `{"reverted":true|false}`.
  - `func installErrorStatus(code string) int`: `unpublished`, `no-scan`, `unsupported-agent`, `connector-offline` → 422; `follows-source`, `install-in-progress`, `already-installed`, `cannot-cancel` → 409. An `InstallError` body is `{"error":"<Message>","code":"<Code>"}`; any other store error goes through `terminalError`.

Both handlers resolve the session and its machine like `terminalSessionSkills` (403 for an unknown or foreign session, machine alias resolved), reject a body that is not valid JSON or has a `skill` that does not start with `reet:` with 400, and write a `skill-install` or `skill-install-cancel` event with the session id, machine, slug and job id (never skill content).

- [ ] **Step 1: Write the failing tests** (same fixtures as `TestTerminalSessionSkills...` in `terminal_skills_test.go`):
  - `TestTerminalSkillInstallQueuesOnePreviewJob`: 200, `reused` false, a `preview` job exists for the user-level target, a `skill-install` event is recorded.
  - `TestTerminalSkillInstallErrorCodesAndStatuses`: one subtest per code, asserting the status from `installErrorStatus` and the stable `code` and `error` text.
  - `TestTerminalSkillInstallRefusesAForeignSessionAnAnonymousCallerAndAViewer` (403, 401, 403).
  - `TestTerminalSkillInstallRejectsABadBody` (not JSON, empty skill, a skill without the `reet:` prefix, a slug with a path separator): 400, nothing queued.
  - `TestTerminalSkillInstallCancel`: the install then cancel gives `{"reverted":true}` and the saved set is gone; a second cancel gives 409 `cannot-cancel`; another account's job gives 403.
  - In `terminal_skills_test.go`: `published` is true for a machine skill and a published hosted skill and false for an unpublished one.
- [ ] **Step 2: Run to verify they fail.** Run: `go test ./internal/api -run 'TerminalSkillInstall|TerminalSessionSkills' -count=1`. Expected: 404 for the missing routes and a compile error on `Published`.
- [ ] **Step 3: Implement** the handlers, `installErrorStatus`, the two routes with `s.terminal(...)` and the `published` field to the contracts above.
- [ ] **Step 4: Run the package.** Run: `gofmt -l internal/api && go vet ./... && go test ./internal/api -run 'TerminalSkillInstall|TerminalSessionSkills|BuildSessionSkills|SkillInvocation' -count=1 -race`. Expected: `ok`. The full `internal/api` package takes about 8 minutes; the controller runs it once at the end.
- [ ] **Step 5: Commit.**

```bash
git add internal/api/terminal.go internal/api/terminal_skills.go internal/api/terminal_skills_test.go internal/api/terminal_skills_install.go internal/api/terminal_skills_install_test.go
git commit -m "Add the install and cancel routes for a hosted skill in a session"
```

---

### Task 3: Shared web logic and API calls

**Files:**
- Modify: `web/src/lib/terminal/skills.ts`, `web/src/lib/terminal/api.ts`
- Test: `web/src/lib/terminal/skills.test.mjs`

**Interfaces:**
- Consumes: the existing `SessionSkill`, `SyncJob` and `SyncPlan` types in `web/src/lib/api.ts` (read `SyncJob`'s fields before writing the new types), `terminalRequest` in `terminal/api.ts`, `getSyncState` and `controlSyncJob` in `web/src/lib/api.ts`.
- Produces, exported from `skills.ts`:
  - `SessionSkill` gains `published: boolean`.
  - `type InstallStep = 'waiting' | 'review' | 'installing' | 'done' | 'failed' | 'cancelled'`.
  - `installStep(job: { kind: string; status: string } | undefined): InstallStep`: kind `preview` with status `queued` or `running` → `waiting`; `awaiting-approval` → `review`; kind `apply` with `queued` or `running` → `installing`; status `succeeded` → `done`; `failed` → `failed`; `cancelled` → `cancelled`; no job → `waiting`.
  - `type PlanVerdict = { ok: true; lines: string[] } | { ok: false; reason: string; lines: string[] }` and `planVerdict(plan: { directory?: string; actions?: { add: boolean; replace?: boolean; skill: { slug: string } }[] } | undefined, slug: string): PlanVerdict`: `ok` only when there is at least one action and every action is a plain add (`add` true, `replace` falsy) of `slug`; `lines` are `Add {slug}`, `Replace {slug}` or `Remove {slug}` in order; `reason` is `This install would change other skills. Review it on the Sync page.` when not ok, or `The machine has not sent its plan yet.` when there are no actions.
  - `installStatusLine(step: InstallStep, host: string, slug: string, waitedSeconds: number): string`: `waiting` gives `Waiting for {host}…` and, from 60 seconds, `Waiting for {host}… Is reet running on {host}?`; `review` gives `Review what will be added.`; `installing` gives `Installing {slug} on {host}…`; `done` gives `{slug} is installed on {host}.`; `failed` gives `The install did not finish.`; `cancelled` gives `Install cancelled.`.
  - In `api.ts`: `type InstallStarted = { job: string; reused: boolean; directory: string; host: string }`, `installSessionSkill(sessionId: string, skillId: string, signal?: AbortSignal): Promise<InstallStarted>` and `cancelSessionSkillInstall(sessionId: string, jobId: string, signal?: AbortSignal): Promise<{ reverted: boolean }>`, built like `getSessionSkills` (POST with a JSON body, the id URL-encoded) and rejecting with the server's `error` text and `code` kept on the thrown error (`err.code`), so the picker can show the sentence and react to `unpublished`.

- [ ] **Step 1: Write the failing tests** in `skills.test.mjs`: `installStep` for every row of the mapping above and for `undefined`; `planVerdict` for a plain add of the slug (ok, lines `['Add pdf']`), an add of another slug, a replace, a removal (`add: false`), a mix of a good add and a removal, an empty list, and `undefined`; `installStatusLine` for each step and the 59 and 60 second boundary; an `installSessionSkill` test that stubs `fetch` and checks the URL, method, body, and that a 409 body `{error, code}` rejects with `err.code === 'install-in-progress'` and `err.message` equal to the text.
- [ ] **Step 2: Run to verify they fail.** Run: `cd web && node src/lib/terminal/skills.test.mjs`. Expected: failures on the missing exports.
- [ ] **Step 3: Implement** the types, the three pure functions and the two API functions to the contracts above; `published` is added to the `SessionSkill` type only.
- [ ] **Step 4: Run to verify they pass.** Run: `cd web && npm run check`. Expected: every test file passes and `svelte-check` reports 0 errors and 0 warnings.
- [ ] **Step 5: Commit.**

```bash
git add web/src/lib/terminal/skills.ts web/src/lib/terminal/skills.test.mjs web/src/lib/terminal/api.ts
git commit -m "Add the install step, plan check and install API calls"
```

---

### Task 4: The install panel in the picker, with the reet TUI cases

**Files:**
- Create: `web/src/lib/terminal/SkillInstallPanel.svelte`
- Modify: `web/src/lib/terminal/SkillPicker.svelte`, `web/src/lib/terminal/ChatPane.svelte`, `web/src/lib/terminal/TerminalPane.svelte` (props only)
- Test: `web/tests/chat.e2e.mjs`

**Interfaces:**
- Consumes: `installStep`, `planVerdict`, `installStatusLine`, `installSessionSkill`, `cancelSessionSkillInstall` (Task 3); `getSyncState` and `controlSyncJob` from `web/src/lib/api.ts`; `SessionSkill.published`.
- Produces:
  - `SkillInstallPanel` props `{ sessionId: string; skill: SessionSkill; host: string; tool: string; onclose: (installed: boolean) => void }`. The slug is `skill.id` without the `reet:` prefix.
  - `SkillPicker` gains props `sessionId: string` and `onrefresh: () => void`. A `reet` row with `published` true and `onMachine` false shows an **Install** button after the row; an unpublished one shows the text `Publish this skill to install it.` as a link to the page where hosted skills are managed (find the page that uses `getHosting` and `saveHostedSkill` in `web/src/lib` and use its route). Pressing Install replaces the list with the panel inside the same dialog; when the panel closes with `installed` true the picker calls `onrefresh` and returns to the list with focus on the filter.
  - `ChatPane` and `TerminalPane` pass `sessionId={session.id}` and `onrefresh={() => void loadSkills()}`.

Panel behaviour: states and texts from the spec, using `installStatusLine`. **Confirm**: `Add {slug} to the {tool} skills folder on {host}. Nothing runs until you approve the plan.` with Install and Cancel; Install calls `installSessionSkill` and on an error shows the error text in the status line (and, for `unpublished`, closes the panel with refresh). After a start it polls `getSyncState` every 2 seconds (stop on destroy or when the job ends; abort in-flight reads), finds the job by id, and renders by `installStep`. **Review** shows `planVerdict` lines and, when `ok`, **Approve and install** which calls `controlSyncJob(jobId, 'approve', job.hash)`; when not `ok` it shows the `reason`, a link to `/machines`'s Sync page (use the route the existing Home approval card links to), and Cancel. **Cancel** always calls `cancelSessionSkillInstall`; when the result has `reverted` false it adds `The saved skill list was changed elsewhere, so it was left as it is.` The panel never cancels a job by itself, however long it waits. A running preview cannot be cancelled: when the call fails with `cannot-cancel` the panel shows that sentence and keeps polling. **Failed** shows `job.message` and Retry (re-runs the install) and Close. **Done** closes with `installed` true. Closing the picker mid-install does not cancel the job. The status line is a persistent `role="status"` region. Buttons are at least 44 px on coarse pointers. Escape closes the picker, not only the panel.

- [ ] **Step 1: Write the failing e2e cases** in `chat.e2e.mjs`. Extend the skills mock with a published reet skill `pdf` (`published: true`, `onMachine: false`, empty `invocation`) and an unpublished one. Mock `POST **/skills/install`, `POST **/skills/install/cancel`, `GET /api/v2/sync` (jobs under test control) and `PATCH /api/v2/sync/jobs`, recording each call:
  - the reet row shows Install, the unpublished one shows the Publish text and no button;
  - Install then confirm: the panel text names the folder and machine, no install call is made until the Install button is pressed, then exactly one call with `{"skill":"reet:pdf"}`;
  - waiting: shows `Waiting for …`, after the mock clock passes 60 seconds the Is-reet-running hint appears, Cancel calls the cancel route (one call, `reverted` true) and returns to the list;
  - review with a plain add: shows `Add pdf` and the Approve button, which makes exactly one `PATCH` with the job's hash and no other call; no approve call without pressing it;
  - review with a replace, a removal and another skill each: no Approve button, the Sync page hand-off and Cancel;
  - installing then done: after the mocked job succeeds the picker reloads (a second skills request) and `pdf` now appears as a native row; focus goes to the filter;
  - failed: shows the message and Retry;
  - `install-in-progress` and `connector-offline` errors show their sentences and leave the panel on Confirm;
  - an `unpublished` error closes the panel and reloads the list;
  - keyboard only: Tab reaches Install and Approve, Escape closes the picker;
  - at 390 px wide the panel fits with no sideways scroll.
- [ ] **Step 2: Run to verify they fail.** With the preview server up (`cd web && npm run build && npx vite preview --port 4173 --host 127.0.0.1`): `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium timeout -s KILL 250 node tests/chat.e2e.mjs`. Expected: the new cases fail on the missing Install button; the old ones pass.
- [ ] **Step 3: Implement** the panel, the picker changes and the two pane props to the behaviour above.
- [ ] **Step 4: Run to verify they pass.** `cd web && npm run check && npm run build`, then the same e2e command. Expected: the script's `passed` line and 0 `svelte-check` errors. Also run `tests/terminal-organisation.e2e.mjs` (the picker props changed).
- [ ] **Step 5: Commit.**

```bash
git add web/src/lib/terminal/SkillInstallPanel.svelte web/src/lib/terminal/SkillPicker.svelte web/src/lib/terminal/ChatPane.svelte web/src/lib/terminal/TerminalPane.svelte web/tests/chat.e2e.mjs
git commit -m "Install a published hosted skill from the session picker"
```

---

### Task 5: The agent TUI case, docs and the full check

**Files:**
- Modify: `web/tests/terminal-organisation.e2e.mjs`, `README.md`, `docs/terminal-workspace.md`

**Interfaces:**
- Consumes: Tasks 1 to 4.
- Produces: coverage that the same install works from the agent TUI's picker, and user-facing docs.

- [ ] **Step 1: Write the failing e2e case** in `terminal-organisation.e2e.mjs` (agent session with `tool: 'claude'`): the Skills picker shows Install on a published reet skill; Install, then Approve on a plain-add plan, makes one install call and one approve call; after success the skill is a native row; nothing is sent to the terminal socket during the whole flow (assert on the recorded `inputs`).
- [ ] **Step 2: Run to verify it fails**, then confirm it passes once Task 4's code is in (the pane props already pass through): `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium timeout -s KILL 250 node tests/terminal-organisation.e2e.mjs`.
- [ ] **Step 3: Update the docs.** In `docs/terminal-workspace.md`'s "Skills in a session" section replace the sentence that says reet-hosted skills cannot be installed with the install flow (what Install does, that you approve the plan, that it installs into the agent's own skills folder for the whole machine, that only published skills install, that Codex and Cline sessions cannot yet, that Cancel leaves nothing behind, and the plain refusal reasons). Add one line to the README feature list.
- [ ] **Step 4: Run everything.** `cd web && npm run check`; `go vet ./...`; `go test -race ./...` (run in the background: the `internal/api` package takes about 8 minutes); then, after `npm run build`, one at a time, `chat`, `terminal-organisation`, `sessions`, `terminal-workspace`, `terminal-lifecycle`, `terminal-phone`, `sessions-phone`, `launch` and `start-agent`. Expected: all pass. Record each command, exit code and the last line in the task report.
- [ ] **Step 5: Commit.**

```bash
git add web/tests/terminal-organisation.e2e.mjs README.md docs/terminal-workspace.md
git commit -m "Cover the agent TUI install and document installing a hosted skill"
```

---

## Self-review

- **Spec coverage:** the endpoint and its merge rules, `preview` only, the pending-job rules, the error codes and sentences, and Cancel's revert (Tasks 1 and 2); `published` on the catalogue (Task 2); the install panel's six states, the plain-add approval rule, the 60-second hint, and focus (Tasks 3 and 4); both TUIs (Tasks 4 and 5); docs (Task 5). Open questions: the Publish link goes to the hosted-skills page (Task 4 says how to find its route); the job kind is settled as `preview`; the other-agent de-duplication limit stays as Phase 1 behaviour and is documented in Task 5.
- **Deviation from the spec, deliberate:** the spec's `read-only` error code is not implemented, because the `s.terminal(...)` wrapper already refuses viewers with a 403 before the handler runs. The spec's `install-in-progress` also covers a machine that is busy with another sync job, since `QueueSyncJob`'s exclusivity rule refuses a new preview in that case; the sentence differs.
- **Types:** `InstallRequest`, `InstallResult`, `InstallError`, `MergeDesired`, `RemoveDesired`, `InstallHostedSkillFor`, `CancelInstallFor`, `installErrorStatus`, `installStep`, `planVerdict`, `installStatusLine`, `installSessionSkill`, `cancelSessionSkillInstall` and `SkillInstallPanel`'s props are named identically wherever they are used.
- **Proportion:** the plan lists signatures, rules and test names with no function bodies; it is roughly twice the spec's length because it names every test.
