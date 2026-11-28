## WORK-ORDERS-0: Design and plan for Factory work orders from outside agents (2026-11-28)

No code changed. This adds three documents for a feature that is not built yet:

- `docs/superpowers/specs/2026-11-28-factory-work-orders-design.md`: the design the owner approved. An outside agent
  reads a public built-in skill (`reet-integration`, with topic files), writes a Markdown work order, and the person
  pastes it into New Factory outcome, where the existing plan review shows it for confirmation. A work report goes
  back by copy, and both directions are also MCP tools.
- `docs/superpowers/plans/2026-11-28-factory-work-orders.md`: eight tasks with file paths, signatures and tests.
- `docs/superpowers/plans/2026-11-28-factory-work-orders-prompt.md`: the prompt to start the implementing session.

**Why this shape.** The Factory already takes pipelines, tiers, parts and pathways over MCP, the plan review already
edits them, and built-in skills are already served publicly. The work is one parser, one report writer, a skill and
the wiring, so no new store tables and no new dependency.

**Checked.** A reviewer read the design against the code as a builder would. Its corrections are in the design: the
enforced limits (8,000-byte briefs, 1,200-byte part briefs, 80-rune titles), the criterion status words the code
uses, and that agent choice in the plan review is per tier, which is why the work order suggests a crew per tier.

**Not checked.** Nothing was built or run. Names in the plan for the web dialog's internals (`crew`, `sessions`,
`step` in `GafferOutcome.svelte`) were read from the file, but the wiring in Tasks 6 and 7 was not prototyped, and
whether `mcp/test` lists tool names was not opened. The skill name `reet-integration` is the assistant's choice over
the owner's example name `factory-contract`; the owner has not confirmed it.
