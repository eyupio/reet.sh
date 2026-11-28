# Continuation prompt: build Factory work orders

Paste everything below the line into a brand new session opened on `eyupio/reet.sh`.

---

Build **Factory work orders** in this repository. The design is approved and the plan is written; your job is to
implement the plan, not to redesign.

**Read these first, in this order, in full:**

1. `AGENTS.md` and `CONTRIBUTING.md` (house rules, traps, how to test and open a pull request).
2. `docs/superpowers/specs/2026-11-28-factory-work-orders-design.md` (what and why).
3. `docs/superpowers/plans/2026-11-28-factory-work-orders.md` (the eight tasks, with exact names and tests).

**What it is, in four sentences.** An agent outside reet reads a public built-in skill, `factory-contract`, served at
`/eyupio/factory-contract` with topic files it can follow for more detail. It writes a Markdown work order with a
flat header (`reet: work-order/1`). A person pastes that into **New Factory outcome**; reet recognises it, shows the
pipelines, stages, parts, criteria and a per-tier crew suggestion in the existing plan review, and nothing starts
until the person confirms. A work report (`reet: work-report/1`) carries the state back by **Copy for your agent**,
and the same check, start, add and report are MCP tools for a connected agent.

**Decisions already made by the owner. Do not reopen them:**

- The contract is a public skill used with `npx skills use`; paste is the primary route; MCP is the second.
- The syntax is Markdown with a flat `key: value` header. It is not YAML and not JSON.
- Two-way is copy-back plus MCP. reet never calls an address an agent supplied.
- Agent choice is a suggestion per tier (`crew:`), applied on paste only, never over MCP, and never saved.
- The skill is an entry page plus topic files, so capabilities are added later by adding a file and a table row.
- A work order can never switch on YOLO, folder trust, auto-merge, Gaffer's decisions or the review limits.

**How to work:**

- Use the superpowers:subagent-driven-development skill (or superpowers:executing-plans if you have no subagents)
  and go task by task, in order. Tasks 1 to 3 are Go and must land before 5 to 7. Each task is test first: write the
  failing test the plan names, see it fail, implement, see it pass, commit with an imperative subject and no prefix.
- Where the plan says to follow an existing function (`outcomePieces`, `mcpOutcomeFull`, `hostedUsePrompt`,
  `stagePlan`), read that function before writing. Reuse it; do not copy its rules.
- Run `npm ci` in `web/` before Go tests. Browser suites need a built UI and a preview server stopped by its PID.
- If the code contradicts the plan, trust the code, make the smallest change that keeps the spec's intent, and say so
  in the handoff entry. If the spec itself cannot be met, stop and ask the owner one question with your
  recommendation first.
- Do not skip, retry or lengthen a timeout to get a test green.

**Done means:** all eight tasks committed; `go test ./...`, the web tests, the `mcp` tests, the docs check and
`web/tests/work-order.e2e.mjs` pass and their output is quoted in `docs/handoffs/log/WORK-ORDERS-1.md`;
`changelog.d/factory-work-orders.md` exists; the example in the skill parses with no problems; you have pasted the
example into a running controller and seen the plan review in light and dark at 390 px and desktop; a draft pull
request is open from a fresh branch off the latest `main`, with the repository's template filled in.

**Report at the end:** what you built, what you verified and how, anything the plan got wrong, and anything you did
not check.
