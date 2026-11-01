# Skills in a running session

Status: draft for review. Nothing here is built.

## Goal

From inside an active session, a person can find a skill and put a short invocation of it into that session, by pressing
**+ Skills** or, in the reet TUI only, by typing `/` at the start of the composer. It works in both session kinds:

- the **reet TUI** (the structured chat, `ChatPane.svelte`), and
- the **agent TUI** (the terminal, `TerminalPane.svelte`).

Skills come from three places, shown together and labelled by where they live:

1. **On this machine, for this session's agent** (machine-hosted and agent-aware).
2. **On this machine, for other agents** (shown dimmed: they exist but are not native to this agent).
3. **reet-hosted** (the person's hosting, `HostedSkill`) and their reet library.

## Decisions already made with the user

- **Skills reach a session by being inserted as a prompt.** reet does not attach or install skills for the person to use a
  skill that is already on the machine.
- **The inserted text is a short invocation**, never the skill's full content. The chat composer is capped at 2,000
  characters (`maxlength` in `ChatPane.svelte`), and a whole skill would flood the agent's context.

## What exists today (checked in the code)

- **Machines report skill names.** The scan (`internal/scan/scan.go`) records each skill with the environment it was found in:
  the host and the agent folder (`.claude/skills`, `.agents/skills`, `.cursor/skills`, `.codex/skills`,
  `.config/opencode/skills`; project folders `.claude` and `.agents`). `GET /api/skills` already returns each skill with its
  `environments`, and `GET /api/environments` lists them. So "on this machine, for this agent" is a filter over existing data.
- **Reet-hosted skills** are `HostedSkill` (`slug`, `description`, `content`, `published`), read through `GET /api/v2/hosting`.
  A hosted skill has an install URL (`syncSource` in `internal/scan/sync.go`).
- **Sync is a mirror, not an install.** `GET/PUT /api/v2/sync` and the preview/auto/apply jobs
  (`internal/store/store_sync_control.go`) make a target environment match a source environment, with an approval step.
  There is **no call that installs one skill on demand**. Installing a reet-hosted skill into a session's machine is
  therefore a new write path, and is Phase 2 below.
- **Session shape.** A `TerminalSession` carries `agentId` (the machine's connector id) and `tool` (the agent). Scan
  environments carry `host` (a hostname), so the catalogue must map a session's machine to the scan host.
- **Text input.** The reet TUI composer is a `textarea` bound to `draft`. The agent TUI is a terminal; text reaches it as
  typed input over the terminal socket.

## Design

### Catalogue

One read-only endpoint returns the merged list for a session:

`GET /api/v2/terminal/sessions/{id}/skills`

Each entry:

| field | meaning |
| --- | --- |
| `id`, `name`, `description` | what to show |
| `source` | `machine` or `reet` |
| `agent` | the agent folder it lives in on the machine (`claude`, `codex`, `opencode`, ...), empty for reet-hosted |
| `native` | true when `agent` matches the session's agent |
| `onMachine` | true when it is installed on the session's machine at all |
| `invocation` | the exact text to insert, or empty when it cannot be invoked from here |

The server builds it from the existing skills/environments tables and the hosting records, scoped to the session's machine.
Groups in the picker follow the fields: native on this machine, other agents on this machine, reet-hosted.
A skill installed in a project folder (a project-scoped environment) is listed only when the session's directory is that project or below it; otherwise it is not listed at all, and a session with no known directory sees none. A session whose machine is not among the account's agents still gets the reet-hosted group, with an empty `host` and no machine skills.

### Invocation text

`invocation` is produced on the server by one pure function (`skillInvocation(agent, name)`), so both TUIs insert exactly what
the server said and the rule is tested in one place. An invocation is always a single line of printable characters: a skill
whose name is empty, has control characters or is longer than 80 characters gets an empty `invocation` and is listed but not
insertable, because text with a newline typed into a terminal would run.

- Default for every agent: `Use the {name} skill.`
- A per-agent override table for agents that have a slash or mention syntax for skills (for example `/{name}`). **The table
  starts with only the entries confirmed against each agent's own documentation during planning**; any agent not confirmed
  uses the default. This repo cannot confirm those syntaxes, so none are assumed here.

### The picker

One shared Svelte component, `SkillPicker.svelte`, used by both TUIs.

- Opened by **+ Skills** (a button beside the composer's Attach button, and in the terminal toolbar) or, in the reet TUI only,
  by typing `/` as the first character of the composer. The agent TUI has no `/` trigger: the agent owns that key and its own
  slash commands must keep working.
- A filter box, the three groups above, and a source label on each row. A skill not on the machine shows "reet-hosted ·
  not on this machine" and is not insertable in Phase 1.
- Keyboard: arrows move, Enter inserts, Escape closes and returns focus to where it came from. On a phone it opens as the
  shared bottom sheet (`dialogShell`).
- Accessible: a listbox with option roles, the group names as labels, and the source said in words, not colour alone.

### Inserting

- **reet TUI:** the invocation replaces the typed `/query` (or is appended after **+ Skills**) in `draft`, with the caret
  after it. The person edits and sends. Nothing is sent automatically.
- **Agent TUI:** the invocation is typed into the terminal **without a trailing newline**, so the person can review it and
  press Enter. A terminal that is not connected disables **+ Skills** with the reason.

### Phases

**Phase 1 (this spec's build):** catalogue endpoint, `skillInvocation`, `SkillPicker`, both insert paths. Skills that are not
on the machine are listed but not insertable. No new write path.

**Phase 2 (separate spec and plan):** "Install on this machine" for a reet-hosted skill. It needs a one-skill install job
through the existing sync machinery, with its approval step, and its own design review. It is deliberately not part of
Phase 1.

## Not in scope

- Attaching or linking skills to a session so the agent loads them natively.
- Editing skills from the picker.
- Inserting a skill's full content.

## Testing

- Go: the catalogue handler, covering a session on a machine with skills for several agents, a machine with none, a machine
  that has not been scanned, and a session on a machine that does not match any scan host.
- Go: `skillInvocation`, including a name with a newline or control character getting no invocation.
- Unit (node): grouping and filtering, the `/` trigger rules (only as the first character, not inside a path or a URL), and
  applying an invocation to a draft without passing the 2,000-byte cap.
- Playwright: the picker in both TUIs, with keyboard and pointer, at desktop, tablet and phone width, in light and dark; the
  inserted text; and that nothing is sent without Enter.

## Open questions for the plan

1. The slash or mention syntax each supported agent uses for skills, to fill the override table.
   The shared `.agents/skills` folder counts as native for every agent until a check shows an agent that does not read it.
2. How a terminal session's `agentId` maps to a scan `host` when a machine has been renamed or merged
   (`/api/v2/environments/merge` exists), and what the picker says when it cannot.
3. How fresh the machine's skill list is: the scan runs periodically, so a skill added minutes ago may be missing. The picker
   should say when the list was last read, and offer the existing refresh.
