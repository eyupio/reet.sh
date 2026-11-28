# Tiers and crew

## Tiers

A tier says how demanding a stage is: `light`, `standard` or `heavy`. reet uses it to choose an agent and a backup from
the person's saved crew. Without one, a stage has its kind's default:

| Stage | Default tier |
| --- | --- |
| `plan`, `design`, `review` | `heavy` |
| `code`, `test` | `standard` |
| `pr`, `babysit`, `merge` | `light` |

Set a tier in the pipeline's `tiers:` line, as `<stage>=<tier>` pairs separated by commas:

```
tiers: code=heavy, review=standard
```

A part may carry its own `tier:` line as the first line under its heading. It overrides the stage's tier for that part.
Ask for `heavy` only where the work is hard: it costs the person more.

Refusals:

- `tiers names “<stage>”, which is not one of this pipeline's stages.`
- `The tier of <stage> is light, standard or heavy, not “<tier>”.`
- `A part's tier is light, standard or heavy, not “<tier>”.`

## Crew

`crew:` in the header suggests who does the work, per tier, not per stage:

```
crew: heavy=claude/opus/high, standard=codex
```

Each entry is `<tier>=<agent>[/<model>[/<effort>]]`, separated by commas, each tier at most once. The agent is one of
`claude`, `codex`, `opencode`, `cline` or `antigravity`. Model and effort are optional and are the agent's own names
for them.

A crew line is a suggestion, never an instruction:

- **Paste.** It pre-fills the person's crew choice for this outcome only, shown as **Suggested by the work order**.
  They can keep their saved crew instead. Their saved crew is never changed. reet also says what their machine cannot
  run.
- **MCP.** It is not applied. The answer says so in its `note`. The tiers and the person's saved crew decide.

Leave `crew:` out unless the person asked for particular agents.

Refusals:

- `Write crew as <tier>=<agent>, with light, standard or heavy for the tier; “<entry>” is not that.`
- `crew names the <tier> tier twice. Keep one.`
- `crew cannot use “<value>”. Write an agent reet can run as <agent>/<model>/<effort>, with model and effort optional.`
- `Remove crew: a pathway names its own agents.`
- `Remove crew: an outcome's crew was chosen when it started.` (with `continues`)
