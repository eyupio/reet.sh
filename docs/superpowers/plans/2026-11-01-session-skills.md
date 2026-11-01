# Skills in a Running Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From a running session, `+ Skills` (and `/` in the reet TUI composer) opens one picker of machine-hosted, agent-aware and reet-hosted skills and inserts a short invocation into the session.

**Architecture:** A read-only endpoint merges the account's scanned skills and hosted skills for one session's machine and agent, and decides each entry's invocation text on the server. One shared Svelte picker is used by the reet TUI (`ChatPane.svelte`, text goes into the draft) and the agent TUI (`TerminalPane.svelte`, text is typed into the terminal without Enter). Phase 1 only: reet-hosted skills are listed but not insertable.

**Tech Stack:** Go (`internal/api`, `internal/store`), Svelte 5 and TypeScript (`web/src/lib/terminal`), node:test, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-01-session-skills-design.md`

## Global Constraints

- The endpoint is `GET /api/v2/terminal/sessions/{id}/skills`, registered with `s.terminal(...)` (accounts-only browser session, like its siblings). It is read-only: no new write path, no sync job.
- An invocation is one line of printable characters, at most 80 runes of skill name, and never contains a newline, tab or other control character. Text with a newline typed into a terminal runs.
- Inserting never sends: the reet TUI puts text in the draft; the agent TUI types it with no trailing newline.
- The composer's `maxlength` is 2000: an insertion that would pass it is refused with a sentence, never truncated.
- Phase 1 invocation text is `Use the {name} skill.` for every agent. No slash form is added until it is confirmed against that agent's own documentation.
- The shared `.agents/skills` folder is native for every agent.
- No new dependency. No new `localStorage` or `sessionStorage` key. Touch targets are at least 44 px on coarse pointers.
- Skill names and descriptions are untrusted text: render them as text, never as HTML.

## Review Focus

Failure modes the spec implies that a person would hit, each pinned by a test in the task that owns the code:

1. A machine that has never been scanned, or has no skills: the picker says so and when the list was last read, not a blank list. (Tasks 1, 3, 4)
2. A skill named with a newline, control character or 200 characters: listed, not insertable, never typed into a terminal. (Task 1)
3. A renamed or merged machine: the session still finds its skills through the machine alias. (Task 2)
4. The session ends or the connection changes while the picker is open: nothing is inserted and the person is told. (Tasks 5, 6)
5. A draft already near 2,000 characters: the insertion is refused with a sentence and the draft is untouched. (Tasks 3, 5)
6. `/` typed inside a message, a path or a URL does not open the picker. (Tasks 3, 5)

## File Structure

- Create `internal/api/terminal_skills.go`: the pure catalogue builder, `skillInvocation`, the handler and its route.
- Create `internal/api/terminal_skills_test.go`: builder table tests and handler tests.
- Modify `internal/api/terminal.go` (route list near line 41).
- Create `web/src/lib/terminal/skills.ts`: types and pure functions shared by both TUIs.
- Create `web/src/lib/terminal/skills.test.mjs`; add it to the `test` script in `web/package.json`.
- Modify `web/src/lib/terminal/api.ts`: `getSessionSkills`.
- Create `web/src/lib/terminal/SkillPicker.svelte`: the shared picker.
- Modify `web/src/lib/terminal/ChatPane.svelte` and `TerminalPane.svelte`.
- Modify `web/tests/chat.e2e.mjs` and `web/tests/terminal-organisation.e2e.mjs`.
- Modify `README.md` and `docs/terminal-workspace.md`.

---

### Task 1: Catalogue builder and invocation (Go, pure)

**Files:**
- Create: `internal/api/terminal_skills.go`
- Test: `internal/api/terminal_skills_test.go`

**Interfaces:**
- Consumes: `store.Skill` (`ID`, `Name`, `Slug`, `Description`, `Environments []store.Environment` with `Host`, `Agent`, `LastSeen` in milliseconds) and `store.HostedSkill` (`Slug`, `Description`).
- Produces:
  - `type SessionSkill struct { ID, Name, Description, Source, Agent string; Native, OnMachine bool; Invocation string }` with JSON tags `id,name,description,source,agent,native,onMachine,invocation`.
  - `type SessionSkills struct { Skills []SessionSkill; MachineSeen int64; Host string }` with JSON tags `skills,machineSeen,host`.
  - `func skillInvocation(agent, name string) string`: `"Use the " + name + " skill."`, or `""` when `name` is empty, longer than 80 runes, or contains a rune that is not `unicode.IsPrint`.
  - `func buildSessionSkills(tool, host string, skills []store.Skill, hosted []store.HostedSkill) SessionSkills`.

Rules for `buildSessionSkills`: an environment is on this machine when `strings.EqualFold(env.Host, host)`. A skill with at least one such environment is a machine skill (`Source "machine"`, `OnMachine` true); its `Agent` is that of its first native environment, else of its first environment. An environment is native when `Agent == tool || Agent == "agents"`. `Native` is true when the skill has a native environment on this machine; only native skills get `Invocation = skillInvocation(tool, name)`. A hosted skill whose `Slug` equals the `Slug` of a machine skill is not repeated; any other becomes `Source "reet"`, `ID "reet:"+slug`, `Name` the slug, `OnMachine` false, `Invocation ""`. `MachineSeen` is the greatest `LastSeen` over every environment on this machine, 0 when none. Order: native machine skills, other-agent machine skills, reet skills; each by lower-cased name then `ID`. `Skills` is never nil.

- [ ] **Step 1: Write the failing tests** in `terminal_skills_test.go`, table-driven, with these names and assertions:
  - `TestSkillInvocationIsASingleSafeLine`: `skillInvocation("claude","pdf")` equals `"Use the pdf skill."`; `""` for `""`, for `"a\nb"`, for `"a\x1b[2Jb"`, and for an 81-rune name; a name of exactly 80 runes is accepted.
  - `TestBuildSessionSkillsSplitsNativeOtherAndReet`: a machine with a `claude` skill, a `.agents` skill, a `codex` skill and a hosted skill, for `tool "claude"`. Expect order native (claude, agents), other (codex, `Native` false, `Invocation ""`, `Agent "codex"`), then reet (`OnMachine` false, `Invocation ""`).
  - `TestBuildSessionSkillsIgnoresOtherMachinesAndMatchesHostCaseInsensitively`.
  - `TestBuildSessionSkillsDoesNotRepeatAHostedSkillThatIsInstalled`.
  - `TestBuildSessionSkillsMachineSeenAndEmpty`: greatest `LastSeen`; an unscanned machine returns `MachineSeen 0` and `Skills` of length 0 that marshals to `[]`.
  - `TestBuildSessionSkillsListsAnUnsafeNameWithoutAnInvocation`: a native skill named with a newline appears with `Invocation ""`.

- [ ] **Step 2: Run to verify they fail.** Run: `go test ./internal/api -run 'SkillInvocation|BuildSessionSkills' -count=1`. Expected: build failure, `undefined: skillInvocation`.

- [ ] **Step 3: Implement** the types, `skillInvocation` and `buildSessionSkills` in `terminal_skills.go` to the rules above (one pass to collect machine skills, then hosted, then a stable sort).

- [ ] **Step 4: Run to verify they pass.** Run: `go test ./internal/api -run 'SkillInvocation|BuildSessionSkills' -count=1 -race`. Expected: `ok`.

- [ ] **Step 5: Commit.**

```bash
git add internal/api/terminal_skills.go internal/api/terminal_skills_test.go
git commit -m "Build the per-session skill catalogue and its invocation text"
```

---

### Task 2: The endpoint (Go)

**Files:**
- Modify: `internal/api/terminal_skills.go`, `internal/api/terminal.go` (add after the `command-history` route)
- Test: `internal/api/terminal_skills_test.go`

**Interfaces:**
- Consumes: `buildSessionSkills`, `SessionSkills` (Task 1); `s.st.TerminalSessionFor(uid, id) (store.TerminalSession, error)` (`AgentID`, `Tool`); `s.st.SyncAgentsFor(uid)` (each has `ID`, `Host`); `s.st.ResolveMachineHostFor(uid, host) (string, error)`; `s.st.ListFor(uid, store.Filter{})`; `s.st.HostedSkillsFor(uid)`.
- Produces: `func (s *server) terminalSessionSkills(w http.ResponseWriter, r *http.Request)`; route `GET /api/v2/terminal/sessions/{id}/skills`; response body is `SessionSkills` JSON with `Host` set to the resolved machine name.

Behaviour: unknown or another account's session answers 404 with the same body the sibling handlers use (follow `terminalHistory`); a session whose machine is not among the account's agents answers 200 with an empty list and `host ""`; a store error answers through `terminalError`.

- [ ] **Step 1: Write the failing tests**, using `privateFixture`, `terminalLogin` and `ReserveTerminalCreateFor` exactly as `terminal_archive_test.go` does:
  - `TestTerminalSessionSkillsListsTheSessionsMachine`: create a session with `Tool "claude"`, save scanned skills for that agent's host (use the store's scan-ingest helper the sync tests use), request the path with the login cookie and an `Origin` header; expect 200, a native entry with `invocation` `"Use the <name> skill."`, and `host` equal to the agent's host.
  - `TestTerminalSessionSkillsFollowsAMachineAlias`: a skill recorded under the old host name of a machine aliased to the agent's host is listed.
  - `TestTerminalSessionSkillsRefusesAnotherAccountAndAnonymous`: another user's cookie gets 404; no cookie gets the sibling routes' status.
- [ ] **Step 2: Run to verify they fail.** Run: `go test ./internal/api -run TerminalSessionSkills -count=1`. Expected: 404 from the missing route.
- [ ] **Step 3: Implement** the handler (resolve session, find the agent's host, resolve alias, `ListFor`, `HostedSkillsFor`, `writeJSON(w, 200, buildSessionSkills(...))`) and register the route with `s.terminal(s.terminalSessionSkills)`.
- [ ] **Step 4: Run the package.** Run: `go vet ./internal/api && go test ./internal/api -count=1 -race`. Expected: `ok`.
- [ ] **Step 5: Commit.**

```bash
git add internal/api/terminal_skills.go internal/api/terminal_skills_test.go internal/api/terminal.go
git commit -m "Serve the skills a running session can use"
```

---

### Task 3: Shared pure logic and the API call (web)

**Files:**
- Create: `web/src/lib/terminal/skills.ts`, `web/src/lib/terminal/skills.test.mjs`
- Modify: `web/src/lib/terminal/api.ts`, `web/package.json` (append `&& node src/lib/terminal/skills.test.mjs` to `test`)

**Interfaces:**
- Consumes: the Task 2 response shape.
- Produces, all exported from `skills.ts`:
  - `type SessionSkill = { id: string; name: string; description: string; source: 'machine' | 'reet'; agent: string; native: boolean; onMachine: boolean; invocation: string }`
  - `type SessionSkills = { skills: SessionSkill[]; machineSeen: number; host: string }`
  - `type SkillGroup = { key: 'native' | 'other' | 'reet'; label: string; skills: SessionSkill[] }`
  - `groupSkills(skills: SessionSkill[], query: string, tool: string): SkillGroup[]`: groups in the order native, other, reet, labels `On this machine for {tool}`, `On this machine for other agents`, `reet-hosted`; the query matches name or description case-insensitively; empty groups are omitted.
  - `slashQuery(draft: string): string | null`: the text after a leading `/` when the draft starts with `/` and has no whitespace after it; `null` otherwise (so `/pdf` gives `pdf`, `/` gives `''`, `fix /etc/hosts`, `/etc/hosts now` and `http://x` give `null`; `/etc/hosts` gives `etc/hosts`, which simply matches nothing).
  - `applyInvocation(draft: string, invocation: string, limit: number): { draft: string } | { error: string }`: replaces a leading `slashQuery` token, else appends after one space when the draft is non-empty and does not end in whitespace; an empty `invocation` or a result longer than `limit` gives `{ error }` with a sentence (`This skill can't be inserted from here.` or `That would make the message longer than 2,000 bytes. Nothing was inserted.`) and no change.
  - `seenLine(machineSeen: number, host: string, now: number): string`: `Skills on {host} have not been read yet.` when `machineSeen` is 0, else `Skills on {host} were read {n} {unit} ago.` using minutes, hours or days.
  - In `api.ts`: `getSessionSkills(sessionId: string, signal?: AbortSignal): Promise<SessionSkills>` using the file's `terminalRequest` helper.

- [ ] **Step 1: Write the failing test** `skills.test.mjs` (same transpile-and-import pattern as `panelState.test.mjs`) with: `groupSkills` ordering, filtering and omitting empty groups; every `slashQuery` case listed above; `applyInvocation` replacing `/pd`, appending to `fix the bug`, refusing past a limit of 20 and leaving the draft untouched, refusing an empty invocation; `seenLine` for 0, 3 minutes, 5 hours, 2 days.
- [ ] **Step 2: Run it.** Run: `node web/src/lib/terminal/skills.test.mjs`. Expected: fails to import `skills.ts`.
- [ ] **Step 3: Implement** `skills.ts` and `getSessionSkills`; add the test to `package.json`.
- [ ] **Step 4: Verify.** Run: `cd web && npm run check`. Expected: all tests pass and `svelte-check` reports 0 errors.
- [ ] **Step 5: Commit.**

```bash
git add web/src/lib/terminal/skills.ts web/src/lib/terminal/skills.test.mjs web/src/lib/terminal/api.ts web/package.json
git commit -m "Add the shared skill picker logic and API call"
```

---

### Task 4: The picker component (web)

**Files:**
- Create: `web/src/lib/terminal/SkillPicker.svelte`

**Interfaces:**
- Consumes: `SessionSkills`, `SessionSkill`, `groupSkills`, `seenLine` (Task 3); `shell` from `../dialogShell`.
- Produces: `SkillPicker` with props `{ data: SessionSkills | null; loading: boolean; error: string; tool: string; initialQuery?: string; onpick: (skill: SessionSkill) => void; onclose: () => void; onretry: () => void }`.

Behaviour: a modal `<dialog use:shell={{ sheet: 'bottom' }}>` (the bottom sheet on phones) titled "Skills". A filter input, focused on open and seeded with `initialQuery`. A `role="listbox"` per group with the group label as its accessible name and `role="option"` rows: name, description, a text source label (`On this machine · {agent}` or `reet-hosted`), and `not on this machine` or `not native to {tool}` when `invocation` is empty. Rows with an empty `invocation` are `aria-disabled` and dimmed and do nothing on click or Enter. ArrowUp/ArrowDown/Home/End move the active option, Enter picks it, Escape closes (the caller restores focus). Below the list, the `seenLine`. Empty states: loading says `Reading skills…`; an error shows the message and a Retry button calling `onretry`; no skills shows `No skills found on {host} yet.`; a filter with no match shows `No skills match.`. Names and descriptions render as text. Rows are at least 44 px high under `@media (pointer: coarse)`. No storage.

- [ ] **Step 1: Write the component** to the behaviour above, following `YoloDialog.svelte` for the dialog and focus handling.
- [ ] **Step 2: Verify it type-checks.** Run: `cd web && npx svelte-check --tsconfig ./tsconfig.json`. Expected: 0 errors. It is exercised by the e2e tests in Tasks 5 and 6.
- [ ] **Step 3: Commit.**

```bash
git add web/src/lib/terminal/SkillPicker.svelte
git commit -m "Add the shared skill picker"
```

---

### Task 5: The reet TUI (ChatPane)

**Files:**
- Modify: `web/src/lib/terminal/ChatPane.svelte` (the composer, near the Attach button and the `draft` textarea)
- Test: `web/tests/chat.e2e.mjs`

**Interfaces:**
- Consumes: `SkillPicker` (Task 4), `getSessionSkills`, `slashQuery`, `applyInvocation` (Task 3), the pane's `draft` state and `session.id`.
- Produces: a `+ Skills` button (`aria-label="Skills"`, in `composer-tools` beside Attach) and a `/` trigger.

Behaviour: the button and the `/` trigger open the picker and load the catalogue once per opening (abort on close). The `/` trigger fires from the textarea's input handler when `slashQuery(draft) !== null` and the picker was not just dismissed for this same draft (Escape sets a flag cleared when the draft stops being a leading-slash token). Picking calls `applyInvocation(draft, skill.invocation, 2000)`; on `{ draft }` it assigns the draft, closes, and returns focus to the textarea with the caret at the end; on `{ error }` it shows the sentence in the pane's `feedback` line and leaves the draft alone. The button and trigger are disabled, with a reason as the title, while `chat.connection !== 'open'` or the session is closed, and a pick made after the connection changed inserts nothing and says `Connection changed. Nothing was inserted.` Nothing is sent.

- [ ] **Step 1: Write the failing e2e cases** in `chat.e2e.mjs`, with the route `**/api/v2/terminal/sessions/*/skills` mocked to return one native skill `pdf` (`invocation "Use the pdf skill."`), one other-agent skill (empty invocation), one reet skill (empty invocation) and `machineSeen` a few minutes old:
  - the `Skills` button opens a dialog with three groups; filtering to `pd` leaves one row;
  - choosing `pdf` with the keyboard closes the dialog, puts `Use the pdf skill.` in the composer, focuses it, and the mocked send route has not been called;
  - typing `/pd` opens the picker with `pd` in the filter; choosing replaces `/pd`; typing `fix /etc/hosts` and `see http://x` opens nothing; Escape on `/` then typing `a` after it does not reopen it;
  - the other-agent and reet rows are `aria-disabled` and picking them changes nothing;
  - a draft of 1,995 characters refuses the insertion with the limit sentence and keeps the draft;
  - an empty machine (`skills: []`, `machineSeen: 0`) shows `No skills found on` and `have not been read yet`;
  - a failed request shows the error and Retry works;
  - the dialog is a bottom sheet at 390 px wide with no sideways scroll.
- [ ] **Step 2: Run to see them fail.** With the preview server up (`cd web && npm run build && npx vite preview --port 4173`) run `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium node tests/chat.e2e.mjs`. Expected: the new cases fail, the old ones pass.
- [ ] **Step 3: Implement** the integration to the behaviour above.
- [ ] **Step 4: Run to see them pass.** Rebuild, then the same command. Expected: the script's `passed` line.
- [ ] **Step 5: Commit.**

```bash
git add web/src/lib/terminal/ChatPane.svelte web/tests/chat.e2e.mjs
git commit -m "Insert a skill from the reet TUI composer"
```

---

### Task 6: The agent TUI (TerminalPane)

**Files:**
- Modify: `web/src/lib/terminal/TerminalPane.svelte` (toolbar at the `Commands` button, `insertSaved`, `paste`)
- Test: `web/tests/terminal-organisation.e2e.mjs`

**Interfaces:**
- Consumes: `SkillPicker`, `getSessionSkills` (Tasks 3, 4); the pane's `paste(text: string, confirm: boolean)`, `live`, `generation`, `alive`, `restoreFocus()`, `feedback`, `session` (with `id` and `tool`).
- Produces: a `Skills` toolbar button beside `Commands`.

Behaviour: the button (`aria-label="Skills"`) opens the picker; it is disabled with a reason while the terminal is not `live`. There is no `/` trigger here. Picking follows `insertSaved` exactly: capture `generation`, close the picker, `await tick()`, and if `!alive || intended !== generation || !live` set `feedback` to `Connection changed. Open Skills again after reconnecting; nothing was inserted.` and stop; otherwise `paste(skill.invocation, false)` and `restoreFocus()`. The invocation has no newline, so the terminal receives the text and no Enter.

- [ ] **Step 1: Write the failing e2e cases** in `terminal-organisation.e2e.mjs` (its harness already mocks the socket and records `inputs`), with the skills route mocked as in Task 5:
  - the toolbar `Skills` button opens the picker; choosing `pdf` sends exactly `Use the pdf skill.` to the socket and no `\r` or `\n` (assert on the recorded `inputs`);
  - typing `/` in the terminal does not open the picker and reaches the socket as `/`;
  - after the mocked socket drops, the button is disabled, and choosing a skill from an already-open picker sends nothing and shows the connection sentence;
  - the other-agent and reet rows send nothing.
- [ ] **Step 2: Run to see them fail.** Run: `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/opt/pw-browsers/chromium node web/tests/terminal-organisation.e2e.mjs`. Expected: the new cases fail.
- [ ] **Step 3: Implement** the button and the guarded pick.
- [ ] **Step 4: Run to see them pass,** after `cd web && npm run build`. Expected: the script's `passed` line.
- [ ] **Step 5: Commit.**

```bash
git add web/src/lib/terminal/TerminalPane.svelte web/tests/terminal-organisation.e2e.mjs
git commit -m "Insert a skill into the agent TUI without pressing Enter"
```

---

### Task 7: Docs and the full check

**Files:**
- Modify: `README.md` (the Sessions feature list), `docs/terminal-workspace.md` (a short "Skills in a session" section)

**Interfaces:**
- Consumes: the behaviour from Tasks 1 to 6.
- Produces: user-facing description.

The docs say what the picker lists, that `/` works only in the reet TUI, that nothing is sent without Enter, that reet-hosted skills are listed but cannot be installed from the picker yet, and that the list is as fresh as the last machine scan.

- [ ] **Step 1: Write the docs** to the points above, matching the surrounding style.
- [ ] **Step 2: Run everything.** Run: `cd web && npm run check` and `cd .. && go vet ./... && go test -race ./...`. Then rebuild and run `chat`, `terminal-organisation`, `sessions`, `terminal-workspace`, `terminal-phone`, `sessions-phone`, `launch` and `start-agent` Playwright scripts. Expected: all pass.
- [ ] **Step 3: Commit.**

```bash
git add README.md docs/terminal-workspace.md
git commit -m "Document skills in a session"
```

---

## Self-review

- **Spec coverage:** catalogue and three groups (Tasks 1, 2); invocation rule and single-line safety (Task 1); picker with filter, groups, keyboard, phone sheet, source words (Task 4); reet TUI insert with `/` trigger (Tasks 3, 5); agent TUI insert without Enter and no `/` trigger (Task 6); Phase 2 install and attaching are out of scope; open questions 2 and 3 are handled by the alias test (Task 2) and `seenLine` (Tasks 3, 4), question 1 by the default phrase.
- **Types:** `SessionSkill`, `SessionSkills`, `groupSkills`, `slashQuery`, `applyInvocation`, `seenLine`, `getSessionSkills`, `skillInvocation` and `buildSessionSkills` are named identically wherever used.
- **Proportion:** the plan is about twice the spec's length because it lists test names and assertions; there are no function bodies.
