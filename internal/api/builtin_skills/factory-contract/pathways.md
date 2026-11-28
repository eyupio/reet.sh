# Pathways

A pathway is the person's own saved line of stages, used instead of the standard stages. Name one only if the person
told you to, or `reet_list_pathways` showed it (connected agents can call that).

Write the pathway's exact name in the header:

```
pathway: Release train
```

Every pipeline in the order then follows that pathway. Because the pathway brings its own stages, agents and
prompts:

- No pipeline may give `stages:`, `tiers:` or `### Part:` sections. Each is refused with
  `A pipeline on a pathway takes no stages. Remove the line, or remove pathway: from the header.` (or
  `no tiers`, or `is not split into parts`).
- `crew:` is refused: `Remove crew: a pathway names its own agents.`
- Briefs and criteria work as usual.

A work order that has `continues` may also name `pathway:`. Only the pipelines it adds follow the pathway.

A pathway must be one of the person's own that is switched on: `No pathway called “<name>” is switched on.` Over MCP, a pathway that does not exist or is switched off refuses the start, and the answer lists the pathways that
are on. On paste, the person chooses a pathway or the standard stages.
