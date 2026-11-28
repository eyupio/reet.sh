# The work order format

A work order is plain text: a flat `key: value` header between two `---` lines, then Markdown. It is not YAML:
no nesting, no quoting rules, one value per line.

## A full example

```markdown
---
reet: work-order/1
objective: Add CSV export to the reports page
project: reports-api
crew: heavy=claude/opus/high, standard=codex
---

## Pipeline: Export endpoint

stages: plan, code, review, test, pr
tiers: code=heavy, review=standard

Add GET /reports/{id}/export.csv … (the brief: self-contained prose, any Markdown,
up to the first ### heading)

### Part: code / Streaming writer
tier: heavy

Write rows as they are read, never the whole file in memory.

### Part: code / Endpoint and auth

Wire the handler and reuse the report's read permission.

### Criteria

- An export of 10,000 rows finishes in under 5 seconds
- A reader of another team's report is refused
```

## Header keys

| Key | Meaning |
| --- | --- |
| `reet` | Required, and the header's first line: `work-order/1`. Another version is refused, naming the versions this controller reads. |
| `objective` | Required, unless `continues` is given. The outcome you want, up to 4000 UTF-8 bytes. |
| `project` | Optional hint: a project's name, matched exactly, any letter case. No match is only a note; the person chooses. Never an id, never a path. |
| `crew` | Optional suggestion of who does the work, per tier. See [tiers-and-crew.md](tiers-and-crew.md). |
| `pathway` | Optional: a saved pathway's exact name. See [pathways.md](pathways.md). |
| `continues` | Optional: an outcome's id, from a work report. See [continuing.md](continuing.md). |

A header key reet does not know is refused, never ignored: a work order has no field for YOLO, folder trust,
auto-merge or who takes decisions.

## Pipeline sections

Each `## Pipeline: <title>` section may start with `stages:` and `tiers:` lines, then the pipeline's brief (prose,
any Markdown, up to the next `###` or `##` heading), then optional `### Part: <stage> / <title>` sections and one
optional `### Criteria` list.

- `stages:` names the built-in stages this pipeline uses, comma-separated, in their fixed order: plan, design,
  code, review, test, pr, babysit, merge. Leave it out and reet plans the stages itself.
- `tiers:` sets a stage's tier, `<stage>=<tier>`, comma-separated. A tier is `light`, `standard` or `heavy`.
- `### Part: <stage> / <title>` splits a stage (plan, design, code, review or test only) into named parts, each
  with its own optional `tier:` line and a brief. A split stage needs at least two parts.
- `### Criteria` is a Markdown list of acceptance criteria: `- <text>`. They can only add a stop (a delivery
  review), never remove one.

## Limits

Every figure below is read from the same code reet's own plan review enforces, never a smaller or separate number:

- At most 5 pipelines in one work order.
- `objective`: up to 4000 UTF-8 bytes.
- A pipeline's brief: up to 8000 bytes.
- A split stage: 2 to 4 `### Part:` sections (the minimum of two is a parsing rule, not a figure to quote).
- A part's brief: up to 1200 bytes.
- `### Criteria`: up to 10 items in one pipeline.
- One criterion: up to 240 bytes.
- A pipeline or part title: 1 to 80 plain characters.

## Recognition

reet treats text as a work order when it starts with a header whose first key is `reet: work-order/`, either as
the whole paste or as the only content of one fenced code block (agents often wrap their output in a fence). Text
outside that fence is then ignored; inside it, or in an unfenced paste, everything must belong to the work order.
Send one work order per paste.

## Refusals you can expect

- An unknown header key, a key given twice, or a header whose opening `---` is never closed.
- `pathway` together with `crew`, or `continues` together with `crew`.
- Stages out of order, a stage used twice outside a split, or a stage that cannot be split.
- A title, brief or criterion over its limit, or a pipeline with no `## Pipeline:` sections at all.
- More than one work order in the same paste.

Every refusal names a line number (or says it is about the whole text) and what to change; nothing starts when
there is a refusal.
