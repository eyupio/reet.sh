## EFFORT-LEVELS-1: terminal launches and saved defaults offer each agent's real thinking levels (27 November 2026)

Why. Chats already offered whatever levels the agent reports, but terminal launches and the account and project
default effort were fixed at low, medium and high. Claude Code's `--effort` also takes xhigh and max (captured help in
`internal/terminalhost/testdata/help/claude.txt`). This came out of a
look at how other coding agents choose effort: most keep one ladder and step it per model, and show the level a run
actually used, which reet already records.

What changed.

- `terminalproto`: `launchAgents` carries each agent's levels (`ToolEfforts`, `ToolTakesEffortLevel`).
  `ValidEffort` accepts xhigh and max; `BasicEffort` is the old three. New feature `effort-levels-v1`
  (`EffortLevelsFeature`) and capability `effortLevels`. A level beyond high is sent only where it was negotiated: the
  broker refuses it as `launch_option_unsupported` for a machine without it, and the connector refuses it for a
  controller without it, because an older peer's validator refuses the value as invalid.
- `terminalhost.Catalogue`: Claude maps xhigh and max. Codex xhigh was left out on purpose: Codex does not check
  `model_reasoning_effort`, and a model without a level silently runs at medium (`docs/terminal-launch.md`), so it waits
  for a stand-in provider probe like the three it has. The catalogue test now checks every mapped level
  against captured help and that the levels match `terminalproto`.
- Profile, folder project, selection run and launch list checks accept a level only for an agent that takes it.
- Web: `effortChoices(tool, more)`; launches offer the extra levels when the machine says `effortLevels`, saved
  defaults always offer them, and the launch bar says to update reet when the machine cannot take one.

Tests. Broker and connector gates, the per-agent level table, `launchEfforts` and `startable` with and without the
feature, and a selection run asking Codex for xhigh. `go test` for api, store, terminalproto, terminalhost,
terminalagent, terminalbroker and agenthealth, and the web unit tests and svelte-check, pass.

Not checked. Whether an older Claude
Code CLI or a model without max refuses the flag at start (the terminal shows the CLI's own error); no browser suite
was run, and light, dark and 390 px were not looked at because only option lists changed.

Not done, from the same look (ask the owner first): step a pipeline stage's effort down to the nearest level its agent
offers instead of the agent's default; Gaffer's own Anthropic provider effort; raising a stage's tier when it is retried.
