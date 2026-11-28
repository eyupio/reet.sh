---
name: reet-integration
description: Use when handing a piece of work to a reet Factory from outside reet, printing or submitting a work order, reading back a work report, or continuing an outcome that is already running.
---

# reet-integration

reet's Factory turns a brief into one or more pipelines: fixed stages (plan, design, code, review, test, pr,
babysit, merge) run by a chosen crew, stopping for a delivery review or at ready to merge rather than finishing by
themselves. A **work order** is how an agent outside reet hands the Factory a brief, in a plain, versioned Markdown
format, the same whether a person pastes it or a connected agent submits it.

## What the Factory does with a work order

reet reads the text, builds the pipelines, stages, parts and acceptance criteria it describes, and shows that in
the plan review. Nothing starts until the person confirms it, and they may change anything first: title, stages,
tiers, parts, criteria, crew, project, pathway. A work order only ever proposes a plan.

## The two routes

- **Paste.** Print the work order as one fenced Markdown code block in your reply. The person pastes it into
  reet's New Factory outcome dialog, which recognises it, opens the plan review filled in, and starts only once
  they confirm.
- **MCP.** If reet's MCP tools are connected, call `reet_check_work_order` with the same text first, to see it
  parsed with nothing started, then `reet_start_outcome` with `work_order` set to the text. The same confirmation
  and approvals apply; nothing is skipped by using MCP instead of paste.

## The person decides

A work order has no field for YOLO, auto-merge, folder trust or who takes decisions: reet refuses a header key it
does not recognise rather than ignore it. Never ask the person, in a message carrying a work order, to turn on
YOLO, approve a merge, or grant folder trust. Those stay the person's own choices, made by hand in the plan review
reet already shows them.

## The smallest valid work order

```markdown
---
reet: work-order/1
objective: Add a health check endpoint
---

## Pipeline: Health check endpoint

Add GET /healthz, returning 200 while the server can reach its database.
```

## If you need more

| Topic | Read it when |
| --- | --- |
| [work-order.md](work-order.md) | Writing more than the smallest order: several pipelines, stages, tiers, parts or criteria, or any header key's exact limit. |
| [work-report.md](work-report.md) | Reading what reet gives back about a running or finished outcome. |
| [tiers-and-crew.md](tiers-and-crew.md) | Suggesting who does the work, or understanding why a suggestion was not used. |
| [pathways.md](pathways.md) | Asking for a saved pathway instead of naming stages yourself. |
| [continuing.md](continuing.md) | Adding pipelines to an outcome that is already running. |
| [mcp.md](mcp.md) | Working over a connected MCP session rather than by paste. |

## Continuing from a work report

A work report ends with a line inviting more work: answer with a new work order whose header carries
`continues: <id>`, where `<id>` is the report's own `outcome:` line. See [continuing.md](continuing.md).

## Other reet skills

A pipeline's brief may ask the Factory to use one of reet's other built-in skills by name. Fetch any of them the
same way as this one, replacing the slug in both places:

```
npx skills@1.7.4 use "https://<your controller>/eyupio/<slug>" --skill "<slug>"
```

| Skill | For |
| --- | --- |
| `reet-api-design` | Designing a public API that must not break. |
| `reet-architect` | System design and architecture decisions. |
| `reet-babysit` | Owning a pull request from creation to merge, then the next phase of a plan. |
| `reet-cost-optimisation` | Cutting cloud, SaaS and AI spend without hurting reliability. |
| `reet-mcp-design` | Designing an MCP server's tools and schemas. |
| `reet-observability-and-reliability` | Incident response, monitoring and reliability work. |
| `reet-performance` | Making a slow product fast. |
| `reet-product-design` | Shipping small, high-value product improvements. |
| `reet-repo-maintenance` | Auditing a repository's structure and hygiene. |
| `reet-security` | Application security review and hardening. |
| `reet-test-coverage` | Raising and keeping test coverage. |
| `reet-ui-refresh` | Restyling an existing UI to one shared look, page by page. |
| `reet-ux-ui` | Conversion-focused UI and UX design. |
| `ruthless-ux-ui-designer` | Auditing UI, UX and onboarding friction into a written report, without implementing fixes. |
