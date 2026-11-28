# The work order, version 1

A work order is Markdown: a header of `key: value` lines between two `---` lines, then one section per pipeline. The
header is not YAML: no nesting, no quotes, one value per line. A complete example, which reet's own tests parse:

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

## Sending it

- Put the work order on its own, or inside one code fence. Text before it outside a fence is refused. The fence closes
  at the first line of only its own characters, at least as long, that is not closing a code block inside a brief;
  anything after that line is ignored, unless a section of the work order follows it, which is refused. If a brief
  holds a bare code fence of the same characters, wrap the work order in a longer fence (````).
- One work order per paste. A second `reet: work-order/` header is refused.
- At most 64 KB in all.
- Write plain text. Everything is shown to the person as written.

## The header

The first line after the opening `---` is `reet: work-order/1`. Each key is given at most once.

| Key | Meaning |
| --- | --- |
| `reet` | Required. `work-order/1`. Any other version is refused. |
| `objective` | Required, unless `continues` is given. The outcome in plain text. |
| `project` | Optional. A project's name, matched exactly in any letter case. No match is not an error: the person chooses the project. Never an id or a path. |
| `crew` | Optional. Who should do the work, per tier. See [tiers-and-crew.md](tiers-and-crew.md). |
| `pathway` | Optional. A saved pathway's exact name. See [pathways.md](pathways.md). |
| `continues` | Optional. An outcome's id from a work report. See [continuing.md](continuing.md). |

Any other key is refused. There is no key for YOLO, folder trust, merging, approvals, decisions or limits.

## A pipeline

Start each with `## Pipeline: <title>`. Pipeline titles are unique within the order. Part titles are unique within their stage. Then, in this order:

1. Optional setting lines directly under the heading: `stages:` and `tiers:`, each at most once, written exactly so
   (lower case, with a colon). A line there that starts with `stage`, `stages`, `tier` or `tiers` and `:` or `=` in
   any other way is refused, never read as the brief. Further down, the brief may start a line with these words.
2. The brief: self-contained prose. Say what to change, where, and how it will be checked.
3. Optional `### Part: <stage> / <title>` sections.
4. Optional `### Criteria`, once.

Only these headings are sections. A heading indented by up to three spaces is still a heading. Inside a brief use headings of level 4 (`####`) or deeper; a heading of level 1, 2
or 3 that is not one of the sections above is refused. A line of only `---` is refused unless it sits inside a fenced
code block. Fenced code (``` or ~~~) is left alone, so a brief may quote a work order or a header.

### Stages

`stages:` lists stage names separated by commas, in this order and each once:
`plan, design, code, review, test, pr, babysit, merge`.

With no `stages:` line the pipeline runs `code, review, test, pr, babysit, merge`. Leave a stage out to skip it; you
cannot reorder them. A pipeline on a pathway takes no `stages:`.

### Parts

Split a stage into parts when its pieces are separate work. Only `plan`, `design`, `code`, `review` and `test` split.
Write `### Part: code / Streaming writer`: the stage, a slash, the part's title. The stage must be one the pipeline
has. Then an optional `tier:` line, then the part's brief, which every part needs. Part titles are unique within a stage. A stage has no parts, or two to
four. Parts of a stage run in the order written.

### Tiers

`tiers:` gives a stage's tier as `<stage>=<tier>` pairs, each stage once, for example
`tiers: code=heavy, review=standard`. A part's own `tier:` line, directly under its heading, overrides it. See [tiers-and-crew.md](tiers-and-crew.md).

### Criteria

A list of `- ` items (or `* `): what the person will check the delivery against. Criteria can only add a stop for the
person's review of the delivery. They never remove one. Each is one line. A `### Criteria` heading with no items is
refused.

## Limits

These are the figures this controller enforces, read from the code.

| Limit | Value |
| --- | --- |
| Pipelines in one work order | 5 |
| Objective (bytes) | 4000 |
| Pipeline brief (bytes) | 8000 |
| Parts in one split stage (at most) | 4 |
| Part brief (bytes) | 1200 |
| Criteria in one pipeline | 10 |
| One criterion (bytes) | 240 |
| Pipeline or part title (characters) | 80 |

A figure over its limit is refused; nothing is clipped.

## Refusals you can expect

A refusal names a line number (0 for the whole text) and says what to change. Nothing starts until there are none.
The ones you are most likely to meet:

- `This is not a work order. It starts with a --- line, then reet: work-order/1.`
- `This controller reads work-order/1 only. Write reet: work-order/1.`
- `The header has no “<key>” key. Use reet, objective, project, crew, pathway or continues.`
- `Add an objective: line to the header.`
- `Write the objective as plain text of up to 4000 bytes.`
- `Write each header line as key: value.`
- `The header gives “<key>” twice. Keep one.`
- `Give “<key>” a value, or remove the line.` (for `project`, `pathway` or `continues`)
- `“<key>” is a name of 1 to 80 plain characters. Shorten it.` (for `project` or `pathway`)
- `The header needs a closing --- line.`
- `Add at least one ## Pipeline: <title> section.`
- `Give the pipeline a title of 1 to 80 plain characters.`
- `Send the work order on its own, or inside one code fence. Remove the text before it.`
- `The code fence around the work order closes here, but the work order goes on after it. Wrap the work order in a longer fence (````) than any fence inside it.`
- `Send one work order per paste. Remove everything from this line on.`
- `Start each pipeline with ## Pipeline: <title>.`
- `“<heading>” is not a section of a work order. Use ## Pipeline: <title>, ### Part: <stage> / <title> or ### Criteria; inside a brief use headings of level 4 or deeper.`
- `A brief cannot hold a --- line. Remove it.`
- `This code block is never closed. Close it with a line of the same fence characters, so the headings after it are read as headings.`
- `“<name>” is not a stage. The stages are plan, design, code, review, test, pr, babysit, merge.`
- `Put the stages in pipeline order, each once: plan, design, code, review, test, pr, babysit, merge.`
- `tiers names “<stage>”, which is not one of this pipeline's stages.`
- `tiers gives <stage> a tier twice. Keep one.`
- `Write this setting as stages: <stage>, <stage>, in lower case with a colon.` (and the same for `tiers: <stage>=<tier>, <stage>=<tier>` under a pipeline, or `tier: light, standard or heavy` under a part)
- `“<key>:” is given twice for this pipeline. Keep one.`
- `Split a stage into 2 to 4 parts, or give “<stage>” no parts.`
- `The <stage> stage is not split into parts. Split plan, design, code, review or test.`
- `Write a part as ### Part: <stage> / <title>.`
- `A pipeline has one ### Criteria list. Merge them.`
- `This ### Criteria list is empty. Add “- ” items, or remove the heading.`
- `Write each criterion as a list item starting with “- ”.`
- `A work order has at most 5 pipelines. Remove this one, or send it in another work order.`
- `Give each pipeline its own title; “<title>” is used twice.`
- `Write a prompt of at most 8,000 bytes without control characters for “<title>”.` (a pipeline brief that is empty, too long or holds control characters; this refusal is for the whole text, line 0)
- `A split stage of “<title>” has up to 4 parts, each with its own short name and a brief saying what it does. The pull request, babysitting and merge are not split.` (a part with a duplicate title, an empty brief or one over 1200 bytes)
- `The work order is over 64 KB. Shorten the briefs.`

Fix every problem and send the whole work order again.
