# Continuing an outcome

After a work report, you may add work to the same outcome. Answer with a work order whose header carries the outcome's
id from the report:

```markdown
---
reet: work-order/1
continues: 0123456789abcdef0123456789abcdef
---

## Pipeline: Refuse other teams

The export endpoint lets a reader of another team's report download it. Refuse that with a 403 and add a test.

### Criteria

- A reader of another team's report is refused
```

Rules:

- `continues` is the outcome id exactly as the work report gives it. A value that is not an id is refused:
  `continues takes an outcome's id, as written in its work report.`
- `objective` is not needed and is ignored: the outcome keeps its own.
- `crew:` is refused. The outcome's crew was chosen when it started.
- `project:` is not needed. The outcome stays in its project. If given, it must name that project.
- reet checks that the outcome is one of the person's that is still running. Otherwise: `No running outcome of yours has that id. Copy a fresh report from the outcome and use its id.`
- `pathway:` may be given with `continues`. The new pipelines then follow that pathway under the pathway rules (no
  `stages:`, `tiers:` or parts; see [pathways.md](pathways.md)), and the pathway must be switched on. Leave it out to
  add pipelines with the standard stages.
- Each `## Pipeline:` section is added as a new pipeline, under the same rules as a first work order, and the
  person confirms it first. At most 5 pipelines in one order.
- Do not repeat work that is `done`. Say in the brief what the earlier pipelines already did, and what is left.
- Paste continuation only adds pipelines. It does not steer a running pipeline, send a stage back or approve a merge:
  the person does those in reet. A connected agent has tools for them (see [mcp.md](mcp.md)).
