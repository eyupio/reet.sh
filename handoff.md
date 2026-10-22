# AI agent setup handoff

## Goal and agreed scope

Expand reet from skills management to portable AI agent setups. The first
milestone is complete backup and restore of supported setups on a new machine;
profile sync follows, then reviewed updates/diagnostics/rollback.

The authoritative detailed plan is [docs/agent-setup-plan.md](docs/agent-setup-plan.md).
It records the owner's decisions, architecture, delivery slices, acceptance gates,
validation and compatibility constraints. Do not reopen settled choices without
new evidence.

Key choices: broad popular-agent discovery (Claude Code, Codex, OpenCode, Pi and
others); explicit verified support/gaps; standalone local CLI; local preview and
confirmation; verified cross-OS restore; exact recorded versions; per-conflict
choices and rollback copies; reusable memory included; history opt-in; portable
credentials optionally encrypted with a user-held key/passphrase; local encrypted
storage plus optional S3; named profiles with machine/project overrides.

## Branch and baseline

- Branch: `feat/agent-setup-backup-plan`.
- Base: `origin/main` at `3fd9e2c` (includes controller skill sync, PR #38).
- Existing skill scan/sync, hosted MCP, lifecycle and backup behaviour must remain
  compatible. Hosted MCP access is distinct from managing installed MCP servers.
- No implementation capability is implied by this planning document.

## Progress

- [x] Inspect existing scanner, CLI, backup primitives and repository guidance.
- [x] Write the detailed plan and this handoff.
- [ ] Open a draft PR containing the plan before beginning implementation.
- [ ] Implement S0: local agent discovery and capability reporting.
- [ ] Add fixtures and CLI tests; update README with actual commands/limitations.
- [ ] Run required checks and record results below.

## Next implementation task

Implement `internal/setup` and `reet setup scan [--json] [--project PATH]`.
Use metadata-only path discovery and executable lookup without executing agents
or reading their configuration contents. Return explicit observation states and
honest restore limitations. Start with the four named agents plus Cursor and
Gemini CLI; adding another adapter must not require changing the CLI.

No setup backup/restore command is implemented in this slice. The next task after
S0 is typed component inventory and secret classification (S1), before collecting
file contents into archives. Do not generically copy agent configuration trees:
they may mix secrets, session history, caches and executable hooks.

## Validation and continuation

Repository guidance is in `CONTRIBUTING.md`; no AGENTS.md was found in the
workspace during initial inspection. Required commands:

```sh
cd web && npm run check
cd .. && go vet ./... && go test -race ./...
```

This workspace has toolchains in `/workspace/.reet-tools/activate.sh`; source it
before local Go/Node commands. This is a workspace convenience, not a product
runtime requirement. Network commands may require sandbox network permission.

Validation results and final implementation details will be appended as work
completes. Keep this file current for the next contributor.
