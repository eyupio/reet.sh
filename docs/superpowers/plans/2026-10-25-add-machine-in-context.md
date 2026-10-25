# Add a machine in context Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A guided **Add a machine** dialog, opened from the Machines page and from the Private skills dialog, that shows one block of commands to paste on a new machine, watches for it to report in, and hands the new machine back to whoever opened it.

**Architecture:** A pure, tested `machineJoin.ts` (name rules, command block, join state, landing choice) behind a new `AddMachineDialog.svelte`. The dialog reuses today's command builders, machine-token creator and snapshot refresh; `EnvironmentSync` mounts it and passes a landing to `MachineFleet`, which opens the Private skills dialog on the new machine's matching folder. Nothing is written to any machine and no server code changes.

**Tech Stack:** Svelte 5 (runes) + TypeScript in `web/`, Node 22 unit tests, Playwright fixtures against the preview server.

**Spec:** [docs/superpowers/specs/2026-10-25-add-machine-in-context-design.md](../specs/2026-10-25-add-machine-in-context-design.md)

## Global Constraints

- Go 1.27.1, Node 22, Svelte 5 runes, no new dependency (not even a dev one). No competing product named anywhere.
- Build the frontend before Go checks: `(cd web && npm run check && npm run build)` with 0 errors and 0 warnings, then `go vet ./...`, `go test -race ./...` with tmux on `PATH`, `(cd mcp && npm ci && npm test)`, `git diff --check`. No Go code is expected to change.
- The machine token is never placed in a command, a URL, a log or browser storage. Nothing about a machine name, token or join state goes in `localStorage`, `sessionStorage` or IndexedDB.
- The words "verified" and "healthy" never describe a machine. "Connected" and "Ready" say only what the controller has been told. The fallback sentence is exactly **Something changed. Check again.**
- At most one light Yorkshire phrase per heading, lede or success message ([voice guide](../../voice-and-copy.md)); state labels, token wording, errors and "nothing happens until you run this" stay plain. British spelling.
- Accounts mode only, hidden for legacy auth and for viewers. Native `<dialog>`; no horizontal page scroll at 320, 375, 768 and 1440px, light and dark.
- Never compare a server timestamp with the browser clock to judge freshness; compare server values with earlier server values. The 14-day complete-scan window is a coarse eligibility rule (as `scopeEligibility` has it), not a freshness claim.
- Rulings carried from the Review UI: pass a fresh `Date.now()` at the moment a state is asked; `ApiError.status` (never message text) tells refusals apart; a 401 stops every timer and calls `onsignedout`; polling pauses while the tab is hidden, backs off on failure, and is fenced by the dialog's lifetime and a generation counter; fixture assertions are never gated on a sleep (use the fast-timer instrumentation and hide the tab for racy pairs).
- Existing tests keep their expectations. Terminal sessions and controls and the deferred security/PIN preference are untouched.
- Browser fixtures run here with `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium` against `npm --prefix web run preview -- --host 127.0.0.1 --port 4173`.

## Amendments to the spec (decided while planning, each with its reason)

1. **A fifth state, `already_here`.** If the name typed already belongs to a machine that is reporting, a plain *Ready* would appear at once although nothing was run. The state reads **A machine called \<name\> is already here.** with **Use it** or *choose another name*. It is decided by comparing the connector's `lastSeen` and id at the moment the name is confirmed with its later values (server against server), so a machine that is re-enrolled under the same name is recognised when its `lastSeen` moves.
2. ***Connected* means an enrolled connector that is paused.** The snapshot has no negotiation flag in the join path; the Private skills dialog's own eligibility and Check button already carry the "connector not ready" reason, so the join dialog does not duplicate it.
3. **A watch cap.** The dialog stops polling after 15 minutes of waiting and shows **Still waiting. Check again.** with a *Check again* button, so a forgotten tab does not poll for ever.
4. **The command block is the three existing blocks in order** (install, save the connection, enrol and run) separated by a blank line, so it is one paste and one prompt for the token.

## Review Focus

Failure modes the spec implies but no task's tests would otherwise exercise, most likely first.

1. **A name that is already in use.** Typing an existing machine's name must not show *Ready*; it shows `already_here` (task 1, task 2).
2. **Shell injection through the name.** The name goes inside `--env '<name>'`. Apostrophes, `$(…)`, backticks, spaces, newlines and control characters must be quoted correctly or refused (task 1).
3. **A forgotten dialog.** A tab left open for hours must stop polling at the watch cap and while hidden (task 2).
4. **Landing on the wrong or an ineligible folder.** The Private skills dialog must never open on a folder that is stale, unsupported, scan-only or on a paused connector, and must say which folder was chosen when several qualify (task 1, task 4).
5. **Token problems.** A failed or revoked token creation, a 401 mid-flow and a viewer who somehow reaches the dialog must end in a plain sentence or the sign-in card, never a silent no-op or a token in the command (task 2).

---

### Task 1: The pure join logic

**Files:**
- Create: `web/src/lib/machineJoin.ts`, `web/src/lib/machineJoin.test.mjs`
- Modify: `web/package.json` (append `&& node src/lib/machineJoin.test.mjs` to the `test` script)

**Interfaces:**
- Consumes: `installCommands`, `configureCommand`, `enrollCommands`, `shellQuote`, `retryDelay`, `supportedAgents`, `ReleaseChannel` from `machineSetup.ts`; `Machine`, `SyncAgent`, `SyncEnvironment` from `api.ts`; `agentGroups` from `machines.ts`; `scopeEligibility` from `contentReview.ts`.
- Produces:
  - `MAX_MACHINE_NAME = 200`; `validateMachineName(name: string): { ok: true; name: string } | { ok: false; reason: string }` (trims; empty, longer than the limit, or any control character is refused with its own plain reason).
  - `joinCommands(site: string, name: string, channel: ReleaseChannel): string` (the three existing builders in order, blank-line separated; never contains a token).
  - `type JoinStage = 'waiting' | 'already_here' | 'connected' | 'first_scan' | 'ready' | 'unknown'`; `type JoinView = { stage: JoinStage; label: string; detail: string; canUse: boolean }`; `type JoinBaseline = { agentId: string; lastSeen: number } | null`.
  - `joinBaseline(name: string, agents: SyncAgent[]): JoinBaseline`; `joinState(i: { name: string; machines: Machine[]; agents: SyncAgent[]; baseline: JoinBaseline; now: number }): JoinView`.
  - `type JoinContext = { skill?: string; bundleId?: string; agent?: string }`; `landingFor(context: JoinContext, machine: Machine | undefined, agents: SyncAgent[], opts: { accounts: boolean; canWrite: boolean; now: number }): { kind: 'open'; environmentId: string; name?: string; chosen: string; others: number } | { kind: 'none'; reason: string }`.
  - `joinDelay(failures: number): number` (`retryDelay` with the base 3000 ms), `JOIN_WATCH_CAP_MS = 15 * 60 * 1000`.

- [ ] **Step 1: Write the failing node tests** in `machineJoin.test.mjs` with exact values: `validateMachineName` for empty, spaces only, 200 and 201 characters, a newline, a tab, a NUL, a unicode name and a name with an apostrophe; `joinCommands` equals the three builders joined by a blank line, contains neither `token` nor `Bearer`, and quotes `it's $(x) \`y\`` and `a b` exactly as `shellQuote` does (assert the full string); `joinState` for no agent (`waiting`, label **Waiting for it**), a paused agent (`connected`, with the pause reason), an agent without a complete supported scan (`first_scan`), a ready machine (`ready`, `canUse` true), a baseline that matches an existing healthy machine (`already_here`, label contains the name, `canUse` true), `already_here` turning into `ready` only when `lastSeen` moves, host matching that is case-insensitive on ASCII only (`Zoë` against `ZoË` does not match), a stale-only scan (older than 14 days) not counting as ready, and an unknown shape returning `unknown` with **Something changed. Check again.**; `landingFor` for same-agent preference, the first qualifying folder when the preferred agent is absent, `none` with a reason for a stale scan, a scan-only agent, a paused connector and no machine at all, and `others` counting the remaining qualifying folders; `joinDelay` 3000 then growing and capped at 20000.
- [ ] **Step 2: Run** `(cd web && node src/lib/machineJoin.test.mjs)`. Expected: FAIL (module missing).
- [ ] **Step 3: Implement** `machineJoin.ts` as pure functions with the signatures above; host folding is ASCII-only (as `scopeEligibility` does); no DOM, no fetch, no clock (take `now`).
- [ ] **Step 4: Run** `(cd web && npm run check && npm test)`. Expected: PASS, 0 diagnostics. Mutation checks (restore byte-identical, name the failing test): drop the control-character refusal; return `ready` for a baseline match without a `lastSeen` change; count a stale scan as ready.
- [ ] **Step 5: Commit** `Decide what the join dialog says and where it lands`.

---

### Task 2: The dialog and the Machines entry point

**Files:**
- Create: `web/src/lib/AddMachineDialog.svelte`, `web/tests/add-machine.e2e.mjs`
- Modify: `web/src/lib/EnvironmentSync.svelte` (an **Add a machine** button beside the existing add action, the dialog mount, the landing highlight), `web/src/lib/MachineFleet.svelte` (highlight the landed machine's card), `.github/workflows/terminal-browser.yml` (run `node web/tests/add-machine.e2e.mjs` after the skill-content fixture), `docs/add-machine-in-context.md` (new), `README.md` (one sentence beside the machine-setup sentences)

**Interfaces:**
- Consumes: Task 1; `CopyButton.svelte`; the `tokenSetup` snippet and `token` that `Machines.svelte` already passes to `EnvironmentSync`; `UnauthorizedError`, `describeError`.
- Produces: `AddMachineDialog` props `{ site: string; machines: Machine[]; snapshot: SyncState | undefined; accounts: boolean; canWrite: boolean; token: string; tokenSetup?: Snippet; context: JoinContext; opener: HTMLElement | null; onrefresh: () => Promise<void> | void; onready(r: { host: string; context: JoinContext }): void; onstepbystep(name: string): void; onsignedout?: (e: unknown) => void; onclose: () => void }`. One screen in three parts (name, key, run) and a polite live status line. *Use \<name\>* is the single primary action when `canUse`; in the Private skills case the dialog also continues on its own after a three-second pause that the person can cancel. Polling is `onrefresh` at `joinDelay` pace, paused while hidden, stopped at `JOIN_WATCH_CAP_MS` (then **Still waiting. Check again.**), fenced by lifetime and a generation counter, and stopped at once on a 401 (`onsignedout`).

- [ ] **Step 1: Write the failing fixture** `add-machine.e2e.mjs`, modelled on `skill-content.e2e.mjs` (synthetic `/api/**` routes, a `writes` log, `pageerror` capture, overflow check, the fast-timer instrumentation). Cases: the entry shows for an accounts-mode writer and is absent for a viewer and legacy auth; opening makes no write except the token-create request when the person presses it; the name field refuses an empty name and a name with a newline with a plain reason; the command block equals `joinCommands` exactly, contains no token, and the copy button copies only the block; the status line walks **Waiting for it** → **Connected** (paused connector, with its reason) → **Waiting for its first scan** → **Ready** as the synthetic snapshot changes, each in text as well as colour; a name that already belongs to a reporting machine shows `already_here` and not *Ready*; Ready offers **Use \<name\>**, and from Machines it closes, scrolls to and highlights the machine's card and returns focus to the opener; polling stops on close, while the tab is hidden and at the watch cap; a 401 stops polling and calls the sign-in hand-off; a failed token creation shows a plain sentence and keeps the dialog usable; *Prefer the step-by-step setup?* closes the dialog and opens the existing setup section for that name; nothing is written to browser storage; layout at 1360, 390 and 320px in light and dark.
- [ ] **Step 2: Run** the fixture against the preview server. Expected: FAIL (no dialog).
- [ ] **Step 3: Implement** the dialog, the entry point, the landing highlight and the workflow line. Write `docs/add-machine-in-context.md` (purpose and non-goals, the three parts, the state table with exact labels, the landing, polling and the watch cap, what is **not** established, continuation) and the README sentence; every behavioural claim checked against the code, plain British prose.
- [ ] **Step 4: Run** `(cd web && npm run check && npm run build && npm test)` then the fixture and the skill-content, machine-import, control and public-workspace fixtures. Expected: PASS, 0 diagnostics. Mutation checks (restore byte-identical, name the failing assertion): put the token in the command; show *Ready* for `already_here`; keep polling after close; keep polling at the watch cap; ignore a 401.
- [ ] **Step 5: Commit** `Add a machine from the Machines page with a live status`.

---

### Task 3: Review fixes and the first pull request

The controller runs this: a task review and fix loop for Tasks 1 and 2, full local validation in the order under Global Constraints, a rewritten pull request description from `.github/PULL_REQUEST_TEMPLATE.md` with the real evidence, every workflow on the final head, then merge. The pull request description states plainly what is still outstanding, and nothing is pushed until its review fixes are in.

---

### Task 4: The Private skills entry and the landing

**Files:**
- Modify: `web/src/lib/MachineSkillContent.svelte` (a **Not listed? Add a machine** link in the Pin step area and wherever the dialog explains that no eligible machine exists), `web/src/lib/MachineFleet.svelte` (a `landing` prop turned into the Private skills dialog on the chosen folder, with the skill preselected and a one-line note saying which folder was chosen and how many others qualified), `web/src/lib/EnvironmentSync.svelte` (glue: open the add dialog with the Private skills context, pass the landing down), `web/tests/add-machine.e2e.mjs`, `docs/add-machine-in-context.md`, `docs/skill-content-review-ui.md` (one paragraph)

**Interfaces:**
- Consumes: Tasks 1 and 2; `landingFor`; the existing `contentSource` in `MachineFleet` (`{ environmentId, name?, opener }`).
- Produces: `MachineSkillContent` optional prop `onaddmachine?: (context: JoinContext) => void` (absent for viewers and legacy auth); `MachineFleet` optional prop `landing?: { environmentId: string; name?: string; note: string } | null` that opens the dialog once and then calls `onlanded()`. When `landingFor` returns `none` the person stays in the add dialog with **Wait here** and **Close** and the reason; the dialog never opens the Pin step on an ineligible folder.

- [ ] **Step 1: Extend the fixture:** the link appears in the Private skills dialog for a writer and not for a viewer; pressing it opens the add dialog with the skill in its context and the Private skills dialog closed; on *Ready* the Private skills dialog opens on the new machine's folder with the skill chosen and a note naming the folder and how many others qualified; the preferred agent wins when two folders qualify; a machine that is Connected but has no complete scan leaves the person in place with **Wait here** and **Close**, then lands when the scan arrives; a paused connector, a stale scan and a scan-only agent never produce a landing; the three-second auto-continue can be cancelled and does not fire after Close; focus ends sensibly (heading of the new dialog); no request is made to a machine; layout at 390 and 320px in light and dark.
- [ ] **Step 2: Run** the fixture. Expected: FAIL.
- [ ] **Step 3: Implement** the link, the landing prop, the glue and the docs paragraph.
- [ ] **Step 4: Run** `(cd web && npm run check && npm run build && npm test)` then all five fixtures. Expected: PASS. Mutation checks: open the Pin step on a stale folder; skip the preferred-agent rule; fire the auto-continue after Close; each fails a named assertion.
- [ ] **Step 5: Commit** `Carry on from the Private skills dialog on a machine added just in time`.

---

### Task 5: Layout pass, handoff, broad review and the last pull request

**Files:**
- Modify: `web/tests/add-machine.e2e.mjs` (responsive and theme matrix: the dialog in each state at 1360, 768, 390 and 320px in light and dark with no document overflow, the primary action reachable, focus returned to the opener on close, Escape closing without writing, a reduced-motion run), `docs/add-machine-in-context.md`, `docs/ui-verification.md` (a dated section recording what was and was not checked), `handoff.md` (a record in its existing format; keep the deferred PIN preference and the terminal-stop notes untouched; record the follow-ups)

- [ ] **Step 1: Add the matrix** and run it.
- [ ] **Step 2: Run an accessibility scan** over the same states if `axe-core` can be fetched with `npx` outside the repository without adding a dependency; record the result honestly in `docs/ui-verification.md`; not a CI gate.
- [ ] **Step 3: Update the docs and the handoff**, check links in every edited document with a short script outside the repository, and run `git diff --check`.
- [ ] **Step 4: Commit** `Document adding a machine in context and record its handoff`.

The controller then runs the full validation, a broad review of the whole slice on the most capable model, the review fixes, the pull request description with the real evidence, every workflow on the final head, and the merge. The pull request description states plainly what is still outstanding until the fixes are pushed.

## Pull requests

1. Tasks 1 to 3: the join logic, the dialog, the Machines entry, its fixture and docs.
2. Tasks 4 and 5: the Private skills entry and landing, the matrix, the handoff, with the broad review before it merges.
