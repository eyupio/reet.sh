# Agent models UI, second half: dialog, project, settings, machines

Plan for M8–M11 of [agent models](../../research/models-design.md), after UI-1 (#252). The backend (#247) and the
picker/launch bar (#252) are merged. This PR finishes the places a model or default is chosen and records the docs.

## Global constraints

- Reuse `models.ts`, `SlotPicker`, and the backend clients; do not fork a second picker or model rule. The model
  list is a suggestion and a read failure never blocks a start.
- Precedence: explicit > project > account > favourite default > agent default; a model/effort only applies with its
  agent. The server stores/validates; the browser resolves and sends. Free text/Other… always works.
- D1/D5/D3/D2/D4 as before (defaults first, auto-read, favourite default, no per-machine default, model rule).
- Only OpenCode has a machine list; other agents show favourites/suggestions/Other and say so. A favourite not on
  the machine is offered with the “not in …” detail.
- Phones: shared bottom sheet, 44 px targets, 16 px fields, no sideways scroll. Keep the launch/phone evidence intact.
- CI: no new `terminal-browser` fixtures. New/extended specs go in `phone-dialogs`/`phone-zoom`; add them to the
  workflow. Unit tests join `web/package.json`.
- TDD, failing-first tests; mutation checks restored byte-identically; task reviews; broad split review before merge;
  the last task updates `handoff.md` and `docs/ui-verification.md`.

## M8 — Start an agent dialog

**Files:** `web/src/lib/StartAgentDialog.svelte`, `web/src/lib/agentLaunch.ts`, `agentLaunch.test.mjs`,
`web/tests/start-agent.e2e.mjs`; entry points only if a prop is needed
(`terminal/TerminalWorkspace.svelte`, `ProjectsBar.svelte`, `MachineFleet.svelte`).

**Behaviour**
- Unify the agent list on `CATALOGUE` + `agentChoices` so Cline is offered when the machine has `agents-v2` and the
  binary; remove the four-agent `TOOLS` limit (keep `toolChoices` returning disabled reasons).
- Replace the native model `<select>` with the shared `SlotPicker`: agent default, favourites, the machine list by
  provider, reet’s suggestions, Current, Other…; star toggles use the favourites API; the picker opens as a sheet on
  touch. A model read/star failure only changes the status line and never disables Start.
- Resolve the agent/model/effort when the dialog opens and when the machine/agent changes: explicit `initialAgent`
  first, then project (when `projectId` is known), then account, then a favourite default, then the first available
  agent. Fetch the project (`getFolderProject`) and profile/favourites when the dialog has a project; a read failure
  falls back to the static suggestions. A project/account model that matches the chosen agent is applied even if it
  is not in the static hints.
- Add a **Thinking** control for an effort-taking agent (Cline) and send `effort` with the launch; clear it when the
  agent does not take effort. The model/effort only go out when non-empty.
- Keep the dialog's focus, folder, prompt, refusal and start-lock behaviour. The picker is not a second source of
  truth for the prompt.

**Tests (failing first)**
- `agentLaunch.test.mjs`: Cline offered under agents-v2; old machine stays four; `rekey` re-resolves to the default;
  no model read failure blocks.
- `start-agent.e2e.mjs`: the picker replaces the select; project default model/effort applied; account default
  applied from Machines/Workspace; Cline + thinking; Other; favourite star; failed list still starts; machine/agent
  change re-resolves; phone sheet; the three entry points still open the dialog.

## M9 — Add project / Project settings

**Files:** `web/src/lib/home/AddProjectDialog.svelte`, `web/tests/add-project.e2e.mjs`,
`add-project-edit.e2e.mjs`, `home.e2e.mjs`, `web/tests/helpers/folders.mjs`.

**Behaviour**
- Default agent list includes Cline (from `CATALOGUE`/`agentChoices` for the project’s machine).
- Add **Default thinking** (`low|medium|high`) only for an agent that takes effort; send/read `defaultEffort`.
- Default model is the shared picker for the chosen machine (favourites + machine list + suggestions + Other); when
  the machine/folder changes, load its list without blocking Save. The free-text field goes away; a model needs an
  agent and an effort needs an effort-taking agent, refused locally with the field sentence before the server.
- Keep the 409 conflict recovery fields (`Mine`) and the More fold; include `defaultEffort` in the snapshot and the
  “Your changes” summary.

**Tests:** create/edit with Cline, thinking and a machine-listed model; a model not in the list via Other; invalid
model/effort combinations local; stale 409 keeps the fields; phone sheet/fit; folder-list failures do not block.

## M10 — Settings › Agents

**Files:** create `web/src/lib/AgentsSettings.svelte`; modify `web/src/lib/Account.svelte`,
`web/tests/phone-zoom.e2e.mjs` (section list), add `web/tests/account.e2e.mjs` and register it in
`phone-dialogs` (or extend an existing account/settings spec).

**Behaviour**
- New Account section **Agents** (hash `#agents`): “New sessions without a project” agent/model/effort using the
  shared picker (no machine list; favourites/suggestions/Other), the existing base-branch/worktree defaults finally
  given a UI, and **Favourite models** grouped by agent with Move up/Move down (keyboard), Remove, Add a model
  (picker over the cached lists of all machines plus Other) and “Use as default when I pick this agent” (D3).
- Saving calls `saveProfile` for the defaults and the favourites APIs for stars/order/default; the order sends the
  version and a 409 reloads the list without losing the person’s place. Read-only viewers can read but not mutate.
- Keep the Account nav/section pattern; 44 px touch, phone sheet, 16 px fields; no page zoom.

**Tests:** account section opens and saves defaults; favourites add/remove/order/default; stale order; viewer read
only; phone-zoom section list includes `agents`; no storage beyond the existing preferences.

## M11 — Machines, Activity, hover card and docs

**Files:** `web/src/lib/MachineSettings.svelte`, `MachineFleet.svelte`, `events.ts`, `events.test.mjs`,
`ActivitySection.svelte`, `terminal/SessionHoverCard.svelte`; tests `add-machine.e2e.mjs`,
`phone-dialogs.e2e.mjs`, `phone-zoom.e2e.mjs`; docs `agent-models.md`, `terminal-launch.md`,
`start-agent-session.md`, `projects.md`, `terminal-workspace.md`, `handoff.md`, `docs/ui-verification.md`,
`docs/roadmap.md`.

**Behaviour**
- Machine settings gains an **Agents** section: installed launchable agents (from terminal capabilities/tools),
  model state (“42 read 3 h ago”, “not supported”, “couldn’t read: timed out”), and Refresh per agent when the
  machine is capable/online; offline disables Refresh. The section reads the model cache and refreshes through the
  API; no model bytes or errors with paths.
- Activity labels for `machine-models-read`, `model-favourite-added/removed/default`, and profile default changes;
  the hover card shows the model with the agent line.
- Docs: update agent models to say the UI exists everywhere it does; terminal launch/start/projects/workspace;
  roadmap/README feature line if appropriate; `handoff.md` state/next stage (Program 2); `ui-verification.md` with
  the three-size comparison and the additions (settings, dialog, project, machines).
- Phone/zoom matrix rows for the new sections/pickers; no new `terminal-browser` fixture.

**Tests:** machine Agents section read/Refresh/offline/old; Activity labels; hover card; phone sections; docs links;
full local matrix of affected specs.

## Test and CI matrix

| Check | Where |
| --- | --- |
| unit `agentLaunch`, `models`, `launchBar` | `npm run check` |
| `start-agent.e2e.mjs` | `terminal-browser` (existing, no new file) |
| `add-project.e2e.mjs`, `add-project-edit.e2e.mjs`, `home.e2e.mjs` | `phone-zoom` |
| `account.e2e.mjs` (new), `phone-dialogs.e2e.mjs` | `phone-dialogs`; add the new spec to the workflow |
| `phone-zoom`/`sessions-phone` | phone jobs |
| matrix | local only; add the Agents section and pickers to the session/home matrix rows |

## Limits

Only OpenCode has a machine list; other agents use suggestions/favourites/Other. A per-machine default stays
deferred; the account and project defaults are per account/project. No real provider list, physical phone or Safari;
a 500-model scroll is not load-tested. Favourites/order version conflicts show a reload sentence rather than merging.
