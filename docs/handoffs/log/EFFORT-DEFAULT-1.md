## EFFORT-DEFAULT-1: an agent's default carries a thinking level (27 November 2026)

Why. The effort work earlier today (EFFORT-LEVELS-1, EFFORT-NEAREST-1, GAFFER-EFFORT-1) made every launch path take an
effort, but a person could only save one effort per account or project, tied to that one default agent. A default
model could be saved per agent and from the live chat; an effort could only be chosen again each time, or in a live
chat. A person who uses Claude Code on high and Codex on medium could not preselect either. The owner chose to save the
effort per agent, beside the agent's default model.

What changed.

- `internal/store/store_models_favourites.go`: `model_favourites.effort` (added by `ALTER TABLE` on an existing database)
  holds the level beside the agent's default row. `SetModelFavouriteDefaultEffortFor` saves or clears it, refuses an
  agent with no default model (`ErrModelFavouriteNotFound`) and a level the agent's preset does not map
  (`ToolTakesEffortLevel`, as the profile does), and does not move the version when nothing changed. Changing or
  clearing the default model clears the effort, so a level never outlives the model it was saved with.
- `internal/api/model_favourites.go`: `PUT /api/v2/profile/model-favourites/default-effort`, writer-only, audited as
  `model-favourite-default-effort` (tool and effort only). `GET` returns `effort` on the default favourite.
- `web/src/lib/models.ts`: `resolveLaunchDefaults` takes the effort from the agent's default favourite ahead of the
  project's and account's, as the model already does. Every launch surface passed that favourite in already, so the
  Start dialog and launch bar needed no new plumbing. `LaunchBar.svelte`'s late favourites read now applies the effort
  as well as the model, through `chatEffort`, so a chat drops a level its agent does not offer.
- `web/src/lib/terminal/ChatPane.svelte`: the live chat's **Make default** saves the model and the effort the chat is
  running. The star lets go when the effort changes, so pressing it again saves just the new level. No new control.
- `web/src/lib/events.ts`: the Activity line, "Made high the default effort for Claude Code".
- `docs/agent-models.md`, `docs/start-agent-session.md` (which also now lists Codex among the agents with Thinking, as
  its launch has done since the Codex effort mapping) and a changelog fragment.

Tests. Store: save, clear, unchanged version, refusals, survives a reopen, cleared by a new default and by clearing, and
a table from before the column gains it. API: save, clear, no default model, invalid effort and tool, audit shape. Web
unit tests (resolver precedence, Activity label). Browser: the new `chat-default-effort` suite (added to the
terminal-browser workflow) and a launch-bar case where the saved effort beats the account's; `chat`, `chat-controls`,
`chat-claude`, `chat-codex`, `launch-options`, `start-agent` and the whole `launch` suite pass. Svelte check clean.

Not checked. Light, dark and 390 px by eye: no new element was added, only the star's title and pressed state change.
The Start dialog itself has no Make default for effort, so a level is saved from a live chat only; Settings > Agents
does not show it. A Codex chat on a level above high (xhigh, max, ultra) cannot be saved, because Codex's terminal preset
maps only low to high: its xhigh is measured as honoured (probe report), and adding it is a catalogue line plus the
level list, left for the owner. OpenCode reports no effort option and its CLI has no flag, so nothing can be preselected
there. A saved effort is not re-checked against the model later: the agent still refuses a level the model lacks.
