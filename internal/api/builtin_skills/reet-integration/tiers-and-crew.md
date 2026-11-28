# Tiers and crew

reet's plan review chooses who does the work by tier, not by stage: `light`, `standard` or `heavy`. Every stage in
a pipeline falls into one of those three tiers, either from a `tiers:` line in the work order or from reet's own
default for that stage.

## The `crew` header key

`crew` is an optional, per-tier suggestion of who should do the work:

```
crew: heavy=claude/opus/high, standard=codex
```

Each entry is `<tier>=<agent>[/<model>[/<effort>]]`, comma-separated, each tier named at most once. The agent is
one of reet's reviewed agent names: `claude`, `codex`, `opencode`, `cline` or `antigravity`. Any other name is
refused. Model and effort are optional and free text; reet does not check them against what each agent actually
offers.

## Paste only

A pasted work order's `crew` suggestion pre-fills the New Factory outcome dialog's crew choice, for that outcome
only. The dialog marks it **Suggested by the work order**, alongside the existing **Saved crew** and **Your
choice**, with a way back to the saved crew. The person's saved crew itself is never changed by a work order, and
launch readiness still says what the machine in front of them cannot run.

## MCP: the saved crew decides instead

Over MCP, `crew` is read but never applied. `reet_start_outcome`'s and `reet_add_pipelines`' answers say so in a
`note` when a work order's `crew` is ignored this way. Tiers and the person's saved crew decide who does the work,
exactly as they do for any other MCP-started outcome.

## Refusals you can expect

- `crew` naming a tier other than `light`, `standard` or `heavy`, or naming one tier twice.
- `crew` naming an agent reet does not run.
- `crew` together with `pathway` (a pathway already decides who does the work).
- `crew` together with `continues` (the outcome's crew was chosen when it started; see
  [continuing.md](continuing.md)).
