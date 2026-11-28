---
name: factory-contract
description: Hand a piece of work to a reet Factory as a precise work order, and carry on from the work report it gives back. Use when asked to send work to reet, start a Factory outcome, or continue one.
---

# Handing work to a reet Factory

reet's Factory turns an objective into pipelines. Each pipeline is a line of stages (plan, code, review, test, pull
request, and so on) run by real coding agents on the person's own machines, in their project. You do not run any of
that. You write a **work order**: Markdown with a short header, in the format below.

A person confirms the plan, and may refine it, before anything starts. Nothing in a work order starts work by itself.
So do not ask for YOLO, folder trust, merges that happen by themselves, who takes decisions, approvals or review
limits. A work order has no fields for them, and a header key it does not know is refused.

## Two routes, one format

1. **Paste (the usual route).** Print the work order in one fenced code block and nothing else of substance. Make that
   fence longer than any fence inside it or after it: four backticks if a brief holds a fence of three (see
   [work-order.md](work-order.md)). Tell the person to paste it into **New Factory outcome** in reet. reet reads it,
   shows the pipelines and stages, and the person confirms and starts.
2. **MCP (only if reet's tools are connected to you).** Call `reet_check_work_order` with `work_order`, fix every
   problem it lists, then call `reet_start_outcome` with the same `work_order`. See [mcp.md](mcp.md).

If you use `crew`, it is a suggestion per tier that the person can decline. It is applied only when a person pastes
the work order, never over MCP.

Send one work order at a time. Everything in it is read as data and shown to the person.

## The smallest valid work order

```markdown
---
reet: work-order/1
objective: Add CSV export to the reports page
---

## Pipeline: Export endpoint

Add GET /reports/{id}/export.csv. Reuse the report's read permission. Stream the rows.
```

With no `stages:` line a pipeline runs code, review, test, pr, babysit and merge. The brief must stand on its own:
the agent doing the work sees only the objective and that brief.

## If you need more

| Topic | Read it when |
| --- | --- |
| [work-order.md](work-order.md) | You write anything beyond the smallest order: stages, parts, criteria. It has every rule, the limits and the refusals. |
| [tiers-and-crew.md](tiers-and-crew.md) | You want to say how demanding a stage is, or suggest which agent should do it. |
| [pathways.md](pathways.md) | The person has a saved pathway of their own stages and wants the work to follow it. |
| [work-report.md](work-report.md) | The person pastes a work report to you, or you want to know what one says. |
| [continuing.md](continuing.md) | You are answering a work report with more work. |
| [mcp.md](mcp.md) | reet's tools are connected to you. |

## Continuing from a work report

The person may paste a **work report** back to you. Read what is done, what failed and what waits for them. If more
work is needed, answer with a new work order whose header has `continues: <outcome id>` from the report. Its
pipelines are added to that outcome. Do not repeat finished work. See [continuing.md](continuing.md).

## Other reet skills

A brief may ask the agent doing the work to use one of these skills by name. Each is fetched with
`npx skills use "<this controller>/eyupio/<name>" --skill "<name>"`.

- `reet-api-design`
- `reet-architect`
- `reet-babysit`
- `reet-cost-optimisation`
- `reet-mcp-design`
- `reet-observability-and-reliability`
- `reet-performance`
- `reet-product-design`
- `reet-repo-maintenance`
- `reet-security`
- `reet-test-coverage`
- `reet-ui-refresh`
- `reet-ux-ui`
- `ruthless-ux-ui-designer`
