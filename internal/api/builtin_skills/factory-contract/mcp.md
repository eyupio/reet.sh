# For an agent connected over MCP

Use this route only if reet's tools are available to you. If they are not, print the work order for the person to
paste (see [SKILL.md](SKILL.md)). The tool descriptions your client shows are the authority on arguments; this page
only says which tool to use when.

| Tool | Use it to |
| --- | --- |
| `reet_check_work_order` | Check a work order without starting anything. Pass `work_order`. It answers the plan it would start, or the problems with line numbers. Always call it first. |
| `reet_start_outcome` | Start the outcome from a checked work order. Pass `work_order`, `project_id` and `confirm: true`. |
| `reet_add_pipelines` | Add the pipelines of a `continues` work order to that outcome. Pass `work_order`, the outcome id and `confirm: true`. |
| `reet_get_work_report` | Read an outcome as a work report. Pass `outcome_id`. |
| `reet_list_factory_projects` | Find the project a work order should run in. |
| `reet_list_pathways` | See which saved pathways are on, before naming one. |
| `reet_list_factory`, `reet_get_factory_outcome` | Read an outcome's state in full. |

The other Factory tools (stop, retry, send back, tell an agent, merge approval) are the person's controls. Use one only
when the person asked for exactly that.

## Rules that differ from paste

- With `work_order`, do not also pass `pipelines`, `objective` or `pathway`: that is refused and nothing starts.
- `project_id`, `agent`, `model`, `confirm` and `request_id` stay arguments. The header's `project`, if present, must
  name that project.
- `continues` in `reet_start_outcome` is refused: use `reet_add_pipelines`. In `reet_add_pipelines` it must equal the
  outcome id you pass.
- `crew:` is not applied over MCP. The tiers and the person's saved crew decide, and the answer's `note` says so.
- Pass `confirm: true` only when the person asked you to start exactly this work. A stable `request_id` makes a retry
  safe.
- An outcome started over MCP has ordinary approvals, never YOLO, no folder trust and the person's usual limits, and
  every pipeline stops at Ready to merge. You cannot change that, and no work order can ask for it.
- The person's own controller enforces the work order format. If a check says it reads a different version, follow
  the version it names.
