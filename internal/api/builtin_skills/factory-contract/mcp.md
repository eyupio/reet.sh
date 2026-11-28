# For an agent connected over MCP

Use this route only if reet's tools are available to you. If they are not, print the work order for the person to
paste (see [SKILL.md](SKILL.md)). The tool descriptions your client shows are the authority on arguments; this page
only says which tool to use when.

| Tool | Use it to |
| --- | --- |
| `reet_check_work_order` | Check a work order without starting anything. Pass `work_order`. Any connection on a writer account may call it. |
| `reet_start_outcome` | Start the outcome from a checked work order. Pass `work_order`, `project_id`, `agent` (the agent that runs the stages, such as `opencode`) and `confirm: true`. |
| `reet_add_pipelines` | Add the pipelines of a `continues` work order to that outcome. Pass `work_order`, `outcome_id` and `confirm: true`. |
| `reet_get_work_report` | Read an outcome as a work report. Pass `outcome_id`. |
| `reet_list_factory_projects` | Find the project a work order should run in. |
| `reet_list_pathways` | See which saved pathways are on, before naming one. |
| `reet_list_factory`, `reet_get_factory_outcome` | Read an outcome's state in full. |

The other Factory tools (stop, retry, send back, tell an agent, merge approval) are the person's controls. Use one only
when the person asked for exactly that.

## What the check answers

`reet_check_work_order` answers `ok` (true or false), `problems`, `plan`, and sometimes `project_id`, `pathway_id` and
`note`.

- `problems` is a list of `{line, text}`; line 0 is about the whole text. Fix every one and check again. When there
  are any, `ok` is false and `plan` is empty.
- `plan` has one entry per pipeline: its `title`, its `stages` (each with `kind`, `name`, a `part` for a split stage
  and a `tier` only when the order gave one) and the number of `criteria`. A pipeline on a pathway lists the
  pathway's stage names.
- `note` says what reet could not match, such as an unknown project, and, if you gave `crew`, that it applies only
  when a person pastes the work order.
- Besides the format, the check refuses a pathway that is not switched on, and a `continues` that is not a running
  outcome of the person's: `No running outcome of yours has that id. Copy a fresh report from the outcome and use its id.`

## Reading a report

`reet_get_work_report` answers `{report}`, the work report as Markdown (see [work-report.md](work-report.md)).
Handovers are included only for a management connection.

## Rules that differ from paste

Each refusal below starts nothing.

- With `work_order`, do not also pass `pipelines`, `objective` or `pathway`: `Give a work order or these arguments, not both. Nothing was started.`
- `project_id`, `agent`, `model`, `confirm` and `request_id` stay arguments. The header's `project`, if present, must
  name the project of `project_id` (for `reet_add_pipelines`, the outcome's project), in any letter case.
- `continues` given to `reet_start_outcome` is refused: use `reet_add_pipelines`. Given to `reet_add_pipelines`, it must
  equal `outcome_id`.
- Problems in the work order come back as `The work order has problems: line N: text ...`. Fix them and send it again.
- `crew:` is never applied over MCP. The tiers and the person's saved crew decide.
- Acceptance criteria in the order are kept on their pipelines.
- Pass `confirm: true` only when the person asked you to start exactly this work. A stable `request_id` makes a retry
  safe.
- An outcome started over MCP has ordinary approvals, never YOLO, no folder trust and the person's usual limits, and
  every pipeline stops at Ready to merge. You cannot change that, and no work order can ask for it.
