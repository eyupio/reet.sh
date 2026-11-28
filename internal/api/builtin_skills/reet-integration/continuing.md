# Continuing an outcome

A work report (see [work-report.md](work-report.md)) ends with an invitation to send more work to the same
outcome. Answering it is the only way a work order steers something already running: there is no paste or MCP way
to tell an existing pipeline to change course mid-stage. The person has buttons for that, and a connected agent
has reet's other Factory tools.

## The `continues` header key

```
---
reet: work-order/1
continues: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4
---

## Pipeline: Export performance follow-up

Profile the export under 100,000 rows and fix whatever is slow.
```

`continues` names an outcome's id, copied from a work report's own `outcome:` line. With `continues` given,
`objective` is not required, and is ignored if given anyway: the outcome already has one. The work order's
pipelines are added to that outcome; nothing about its existing pipelines changes.

## Paste

Pasting a work order with `continues` opens the plan review in an adding mode on that outcome, with the new
pipelines filled in. The person confirms exactly as they would for a fresh outcome.

## MCP

A connected agent sends `continues` to `reet_add_pipelines`, with the value matching the outcome id it is calling
on. `reet_start_outcome` refuses a work order carrying `continues`, naming `reet_add_pipelines` as the tool to use
instead.

## Refusals you can expect

- `continues` together with the header's `crew`: the outcome's crew was already chosen when it started, so a new
  suggestion for it is refused rather than silently ignored.
- `continues` sent to `reet_start_outcome` over MCP.
- `continues` sent to `reet_add_pipelines` that does not match the outcome id being called on.
- An id that names no outcome the caller can see.
