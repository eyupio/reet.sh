# REAL-AGENT-CHECKS-2: real-agent plan covers batch rollback and remove, and fleet setup copy

**What:** `docs/handoffs/real-agent-checks.md` gains part 8 (software batches: default update, roll back, remove, refusals,
connection loss, restart) and part 9 (copy a setup: read, merge plan, credential refusals, widening and the typed word, order,
undo, expiry, comments lost in merged JSONC and TOML, size limit, backups). The plan now takes about three hours.

**Why:** both features were built and tested only against fixtures and fake connectors.

**Checked:** the steps match `docs/agent-software.md` and `docs/fleet-setup.md` as merged (#934, #937).

**Not checked:** nothing was run; this is the list of what to run.
