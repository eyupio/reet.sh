# The work report format

A work report is reet's mirror of a work order: the same plain Markdown-with-a-header style, giving back what an
outcome is doing. The person copies it to you with **Copy for your agent** on the outcome; a connected agent reads
the same thing from `reet_get_work_report` (see [mcp.md](mcp.md)).

## A worked example

```markdown
---
reet: work-report/1
outcome: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4
objective: Add CSV export to the reports page
state: running
---

## Pipeline: Export endpoint
state: delivery review
stage in hand: test

- plan: done
- code: done (heavy)
- review: done
- test: in hand

### Handover: code
(the stage's handover note, clipped to 600 characters as it is over MCP)

### Criteria
- [passed] An export of 10,000 rows finishes in under 5 seconds
- [unverified] A reader of another team's report is refused

### Waiting for the person
Delivery review.

To add more work to this outcome, answer with a work order whose header carries continues: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4.
```

## The header

`outcome` is the outcome's id, the value to carry back in a follow-up work order's `continues` key (see
[continuing.md](continuing.md)). `objective` and `state` are the outcome's own, read straight from the Factory.

## State words

Every state word a work report uses is one the Factory page already shows for that outcome or pipeline: for
example `running`, `delivery review`, `ready to merge`, `stopped`. A word is always written with a space where
reet stores an underscore, nothing else changed.

## Per-pipeline detail

Each `## Pipeline:` section gives that pipeline's own state, the stage currently in hand (if any), then a list of
every stage with its status: `pending`, `in hand`, or `done`, with its tier in brackets once it is done and had
one. A finished pipeline lists only the stages it actually ran.

### Handovers

A `### Handover: <stage>` section carries that stage's handover note, clipped to 600 characters. The person's own
copy always includes handovers; a work report read over MCP leaves them out unless the connection is one that may
see session text, the same rule reet already applies to handovers everywhere else.

### Criteria

A `### Criteria` section lists every acceptance criterion with its current status, one of:

- `passed`: evidence says it is met.
- `needs_work`: evidence says it is not met.
- `unverified`: no evidence has been recorded against it yet.

### Waiting for the person

A pipeline stopped at a delivery review or ready to merge carries a `### Waiting for the person` line saying so.
No other pipeline state needs one: it is either still running or already finished.

## Continuing

The report's last paragraph is fixed and always present: it names the outcome's id and invites a follow-up work
order carrying `continues: <id>`. See [continuing.md](continuing.md) for what that does and what it refuses.
