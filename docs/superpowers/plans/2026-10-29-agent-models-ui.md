# Agent models UI: the picker, launch defaults and settings

Plan for the web half of [agent models](../../research/models-design.md), after the backend (#247). Two PRs:

1. **UI-1 (M5–M7):** the pure model logic and API clients, the shared picker with a star, and the launch bar/Home integration.
2. **UI-2 (M8–M11):** the Start an agent dialog, Add project/Project settings, Settings › Agents, Machines “Agents”, Activity/hover-card display, docs and the handoff.

Owner decisions: **D1** the launch bar opens with the defaults first and the last used models are a Recent group; **D5** the machine list loads automatically (the backend first read is synchronous, then cached); **D3** a favourite can be the default for its agent; **D2** per-machine default deferred; **D4** the model-name rule stays.

## Global constraints

- The model list is a suggestion. A read failure, an offline machine or an old connector never blocks a launch; free text/Other… always works. The server never substitutes a default into a launch; the browser resolves and sends the chosen values.
- Only OpenCode has a machine list. Other agents use favourites, reet’s suggestions and Other…; say so honestly.
- Precedence: explicit choice in this bar/dialog > project default (when the project’s agent matches) > account default (when its agent matches) > favourite marked for this agent > the agent’s own default (`""`, send nothing).
- D1: `initialChoice` uses the resolved defaults, not the last launch. Last launches feed the Recent group (newest three distinct models for that agent).
- Keep the existing launch/phone layout: the model slot stays hidden in the tablet/phone summary; the Change sheet and the picker sheet show it. No sideways scroll; 44 px touch targets in the sheet, 40 px for the terminal’s own keys; 16 px fields on phones.
- Do not add fixtures to `terminal-browser`; use `phone-dialogs`/`phone-zoom`. Unit tests join `web/package.json` `test`.
- Design fidelity: the launch bar and Change sheet are drawn in `Main.dc.html`, `Tablet.dc.html` and `PhoneHome.dc.html`; the model picker rows and star are not drawn, so build them from the existing slot listbox and record every addition in `docs/ui-verification.md` at 1440, 834 and 390. Settings › Agents has no drawing; use the Account/Profile section pattern and record it.
- TDD, failing-first tests for every task; mutation checks restored byte-identically; task reviews and a broad split review before merge; the last task updates `handoff.md`.

## UI-1 — M5–M7

### M5: pure model logic and clients

**Files:** create `web/src/lib/models.ts`, `models.test.mjs`; modify `web/src/lib/terminal/api.ts`,
`web/src/lib/api.ts`, `web/package.json`.

**Interfaces**

```ts
type ModelEntry = { id: string; provider: string }
type MachineModels = {
  tool: string; state: 'listed'|'unsupported'|'failed'|'off'|'offline'|'update';
  models: ModelEntry[]; more: boolean; source: string; version: string;
  fetchedAt: number|null; checkedAt: number|null; reason: string; sentence: string
}
type ModelFavourite = { tool: string; model: string; agentDefault: boolean }
type ModelOptionsInput = {
  tool: string; current?: string; machine?: MachineModels | null;
  favourites: ModelFavourite[]; recent: string[]; suggestions: string[];
  selectedModel?: string | null
}
type ModelOption = { value: string; label: string; detail?: string; group: string; disabled?: boolean; reason?: string; starred?: boolean }
```

- `providerOf(id)`, `dedupeModels`, `recentModels(sessions, tool, limit=3)`.
- `modelOptions(input)`: groups in order **Agent default**, **Favourites** (this agent, account order), **Recent** (newest distinct, not already shown), **On <machine>** grouped by provider (deduped, marked “not in <machine>’s list” for favourites/suggestions missing from the machine list), **reet’s suggestions**, **Other…**. The empty `""` option is the agent default; its detail says “<machine>’s <agent> decides” or “the agent decides”.
- `resolveLaunchDefaults({project?, account?, favourite?, agent})` → `{agent, model, effort}` for the precedence above, only applying a project/account model when its agent matches; drop effort unless the agent takes it.
- `modelStatus(machine, host)` → `{label, sentence, canRefresh, age}` for listed/unsupported/failed/off/offline/update; the server `sentence` wins; age from `fetchedAt` (“read 3 h ago”), failure from `checkedAt`; refresh is available for listed/failed and a capable online machine, not for off/unsupported/update.
- `effortChoices(tool)` → `['', 'low','medium','high']` only for `ToolTakesEffort` agents, else `['']`.
- `moveFavourite(items, index, delta)`, `favouriteKey(tool,model)`, `starLabel(tool,model)`.

Clients:
- `terminal/api.ts`: add `models?`, `modelsOff?` to `Capabilities`; add `defaultEffort` to `FolderProject`/`FolderProjectInput`; add `machineModels(agentId, tool, signal)`, `refreshMachineModels(agentId, tool)`, `allModels(tool)` using the backend routes. The refresh is POST JSON; the read is GET.
- `api.ts`: add `defaultTool/defaultModel/defaultEffort` to `Profile` and `saveProfile` input; add `listModelFavourites`, `addModelFavourite`, `removeModelFavourite`, `reorderModelFavourites`, `setModelFavouriteDefault` with the backend codes.

**Tests (failing first):** option order/dedupe/groups/agent default/Other; recent distinct/limit; precedence table (explicit, project, account, favourite default, agent default; agent mismatch; effort dropped); status sentences/ages/canRefresh; favourite move/star label; API clients with mocked fetch (paths, methods, bodies, error codes).

### M6: the shared picker

**Files:** modify `web/src/lib/launch/SlotPicker.svelte`; extend `web/tests/launch.e2e.mjs` and/or `phone-dialogs.e2e.mjs`; maybe create `web/src/lib/launch/ModelPicker.svelte` as a thin wrapper.

**Behaviour**
- `SlotOption` gains `starred?: boolean`, `onstar?: () => void`, `starLabel?: string`. A row is a wrapper; the option remains a `role="option"` button and the star is a **separate sibling button** (not nested), `aria-pressed`, 44 px on touch, label “Star <model> for <agent>” / “Unstar…”. Clicking the star does not choose the option or close the list.
- The model slot uses the existing `filter`; the filter is pinned at the top, groups have sticky headings, and the status line has `role="status" aria-live="polite"` and an inline **Refresh** action when allowed.
- Typed text that matches nothing offers **Use ‘x’** (Other… pre-filled). The keyboard model (arrows/Home/End/Enter/Space/Escape/Tab) stays; the star is reachable by Tab within the sheet and its state is announced.
- Phone: existing bottom sheet, 16 px input, 44 px targets; no new sideways scroll.

**Tests:** launch.e2e keyboard star/announcement, model filter, Other, status/Refresh disabled where honest; phone-dialogs sheet geometry with the star.

### M7: launch bar and Home

**Files:** modify `web/src/lib/launchBar.ts`, `launchBar.test.mjs`, `web/src/lib/launch/LaunchBar.svelte`, `web/src/lib/home/Home.svelte`, `web/tests/launch.e2e.mjs`, `web/tests/helpers/workspace.mjs`, `longtext.mjs`.

**Behaviour**
- `LaunchDefaults` gains `defaultTool/defaultModel/defaultEffort`; `LaunchProject` gains `defaultEffort`; `initialChoice` uses `resolveLaunchDefaults` (D1), falls back to the first available agent, and no longer restores the last launch. `lastChoice`/Recent feed `modelOptions` only.
- Home reads the three account defaults and favourites (same 10 s profile read) and passes them to the bar; the profile read failure keeps the static suggestions.
- On mount/target change the bar loads the chosen machine/agent model list (D5) and refreshes on a person’s Refresh; a failed read only changes the status line. The model slot options come from `modelOptions`; the agent slot still comes from `agentChoices`.
- Changing the agent re-resolves the model/effort from the defaults; it never leaves a model from the previous agent.
- Favourites load from the account; toggling a star updates the bar/picker immediately and calls the API; a failure restores the previous state with a plain sentence. `Recent` comes from the session list.
- Several projects on different machines: favourites + suggestions only, with “Models differ per machine.”; a single target machine uses its list.
- Keep model failure out of `startable`/`reason`; the sentence says suggestions are still available.
- `default` still sends `""` (nothing). A model not in the list is still allowed if it passes `validateModel`.

**Tests:** launchBar unit precedence/defaults/agent switch/recent/status; launch.e2e list from machine, offline cache, Other, favourite first, project/account defaults, failure never blocks, tablet Change sheet, phone sheet; update the old “last launch restores” expectation to a Recent group.

## UI-2 — M8–M11 (next PR)

- **M8:** replace StartAgentDialog’s `<select>` with the shared picker; apply project/account defaults (fetch the project when only `projectId` is known); include `effort` for effort-taking agents; update `agentLaunch.ts` `rekey`/`toolChoices` (Cline) and the start-agent e2e.
- **M9:** Add project/Project settings: default agent (Cline included), model picker for the project’s machine, default thinking; local validation of model-needs-agent/effort-needs-agent; update add-project/home fixtures and tests.
- **M10:** Settings › Agents: new `AgentsSettings.svelte` (account default agent/model/effort, favourites manager with Move up/Move down/Remove/Add and “default for this agent”, base-branch/worktree defaults finally given UI); add `agents` to the Account section whitelist and phone-zoom; account e2e/axe; use the Account/Profile section pattern (no drawing).
- **M11:** Machines “Agents” section with installed agents, read time, state and Refresh (capability/readiness aware); Activity lines for model events; hover-card model; docs (`agent-models.md`, `terminal-launch.md`, `start-agent-session.md`, `projects.md`, `terminal-workspace.md`), handoff, phone/zoom matrix rows.

## Test and CI matrix

| Check | Where |
| --- | --- |
| `models.test.mjs`, `launchBar.test.mjs`, `agentLaunch.test.mjs` | `npm run check` (terminal-browser job) |
| `launch.e2e.mjs`, picker/phone cases | `phone-dialogs` |
| `home.e2e.mjs`, Add project/Project settings | `phone-zoom` |
| `start-agent.e2e.mjs` | `terminal-browser` (existing file; no new fixtures) |
| account/settings e2e | `phone-dialogs` or `phone-zoom`, added to the workflow |
| matrix | local only; add model states to the launch/Change sheet screens |

## Compatibility and limits

- An old machine has no `models` capability; the picker shows suggestions and the update sentence. A machine with the switch off shows the off sentence. Offline shows the last cached list with its age.
- The backend cache states are `listed|unsupported|failed|off|offline|update`; conflict/rate limits only affect Refresh.
- A model-list read is never part of readiness; launch remains possible.
- Only OpenCode lists; the other agents are honest suggestions.
- The web stores no terminal text; only the existing per-browser launch view choices are saved. Favourites/defaults live on the server.
- No real phone/Safari; the matrix and Chromium emulation are the evidence.
