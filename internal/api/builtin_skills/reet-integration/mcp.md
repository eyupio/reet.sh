# Working over MCP

When reet's MCP tools are connected, a work order can be submitted directly, and a work report read directly,
without any paste step. This page only names the tools; read each tool's own description, as your MCP client
shows it, for its exact arguments and answer shape rather than a schema repeated here.

## Reading

- `reet_check_work_order`: the dry run. Parses a work order and reports its plan, hints and problems; starts
  nothing. Use it before `reet_start_outcome` or `reet_add_pipelines` to catch problems early.
- `reet_get_work_report`: the work report for one outcome (see [work-report.md](work-report.md)).
- `reet_list_factory`: the person's current Factory outcomes.
- `reet_get_factory_outcome`: one outcome's full detail.
- `reet_list_factory_projects`: the projects a work order's `project` key can name.
- `reet_list_pathways`: the pathways switched on, for the `pathway` key (see [pathways.md](pathways.md)).

These reads are open to the same connections the existing Factory reads already are.

## Writing

- `reet_start_outcome`: starts a new outcome. Takes `work_order` as an alternative to naming pipelines directly;
  with `work_order` given, `pipelines`, `objective` and `pathway` must be left out, and a header `project` must
  either match the `project_id` argument or be left out itself.
- `reet_add_pipelines`: adds pipelines to an outcome already running. Takes `work_order` the same way, for a work
  order carrying `continues` (see [continuing.md](continuing.md)); its `continues` value must equal the outcome
  being called on.

reet's other Factory write tools are unchanged by work orders and still do only what they always did:
`reet_stop_outcome`, `reet_retry_pipeline`, `reet_send_pipeline_back`, `reet_move_pipeline_on`,
`reet_tell_pipeline_agent`, `reet_set_pipeline_merge`, `reet_approve_pipeline_merge` and `reet_report_outcome`.

## What stays the same

Starting an outcome from a work order over MCP keeps every rule an MCP-started outcome already has: `confirm`
must still be sent, ordinary approvals still apply, YOLO is never used, and every pipeline still stops at ready
to merge. A work order changes what is proposed, never how reet runs it.
