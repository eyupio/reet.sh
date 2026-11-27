## GAFFER-EFFORT-1: an Anthropic provider takes a reasoning effort (27 November 2026)

Why. Gaffer's provider settings refused reasoning effort for Anthropic ("does not accept the OpenAI reasoning-effort
setting"), so Gaffer's own AI on Anthropic always ran at the model's default effort. The Messages API takes
`output_config.effort` (low, medium, high, xhigh, max; GA, no beta header). Follows the effort work in EFFORT-LEVELS-1.

What changed.

- `internal/gaffer/settings.go`: the allowed levels follow the provider kind: Anthropic low to max with no none;
  OpenAI-compatible none to high, as before.
- `internal/elicore/provider/anthropic.go`: a set effort is sent as `output_config: {effort}` on every request, the
  check included; nothing is sent when it is unset. `thinking` is left as it was: current models already think
  adaptively by default, and turning thinking on explicitly for older ones would need thinking blocks carried through
  tool turns, which the adapter does not do.
- `web/src/lib/GafferSettings.svelte`: the Reasoning effort select shows for Anthropic too, with that kind's levels; a
  level the new kind lacks is cleared when the preset changes.
- `docs/gaffer-ai.md` says so.

Tests. The adapter sends the effort on a chat and the check and sends no `output_config` when unset; the validation
per kind. Web unit tests and svelte-check pass. No browser suite was run, and no request reached the real API.

Not checked. Which levels each model takes is left to the API: an older model (Opus 4.6 has no xhigh; Sonnet 4.5 and
Haiku 4.5 take no effort) refuses under Test connection, as the OpenAI-compatible setting already does.
