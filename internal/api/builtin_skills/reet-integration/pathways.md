# Pathways

A pathway is a stage recipe the person has already saved and switched on in reet, under its own name. Asking for
a pathway is an alternative to naming `stages:` and `tiers:` yourself.

## The `pathway` header key

```
---
reet: work-order/1
objective: Add CSV export to the reports page
pathway: Small fix
---

## Pipeline: Export endpoint

Add GET /reports/{id}/export.csv …
```

`pathway` names a saved pathway's exact name, matched against the pathways the person has switched on. The
pipeline's brief is still written as usual; its stages, their order and their tiers come entirely from the
pathway instead.

## It is exclusive with `stages`, `tiers` and `crew`

A pipeline that gives `pathway` must not also give `stages:` or `tiers:`; reet refuses the one that duplicates what
the pathway already decides. The header's `crew` key is refused outright whenever any pipeline in the work order
asks for a pathway, because a pathway already decides who does the work, tier by tier, as the person set it up.

## Discovering pathways

A connected agent can list the pathways switched on for a project with `reet_list_pathways` (see
[mcp.md](mcp.md)) before writing `pathway`. An agent working only by paste has no such list; it either knows the
pathway's name from earlier context, or leaves `pathway` out and lets the person choose one in the plan review
after pasting.

## Refusals you can expect

- No pathway switched on by that exact name (the refusal names the ones that are).
- `pathway` together with `stages:`, `tiers:` or the header's `crew`.
