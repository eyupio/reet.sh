## FLEET-SETUP-1: one machine's agent setup can be copied to other machines through the controller (25 November 2026)

The back end of the roadmap's "Fleet-wide agent configuration" item. The web page is a separate change; the roadmap entry
stays in "Later" until it lands, and that change removes it. [docs/fleet-setup.md](../../fleet-setup.md) has the API.

**What it does.** The controller reads one machine's global instruction files and portable settings
(`setup-read-v1`), asks each chosen machine for a dry-run plan (`setup-apply-v1`, op `plan`), and on a batch applies it
one machine at a time (op `apply`), with Retry, Skip, Abandon and a per-machine Undo (`setup-rollback-v1`). It mirrors
the software batch: `internal/setuprun` is a lean copy of `internal/softwarerun`, `store_setup_batches.go` of
`store_software.go`, with separate tables so a setup batch never holds up a software one. Software code is untouched.

**What is copied.** `fleetSettingsPolicy` (`internal/setup/fleet.go`) is the portable settings policy minus model
defaults and fields that run or enable commands (`formatter`, `packages`, the three `*McpjsonServers` fields); Claude's
`.claude.json` and `settings.local.json` are not copied at all (account state the agent rewrites constantly, and a
machine-specific file). `TestFleetAllowlistDropsModelDefaultsAndCommandFields` pins the lists. Credential-like content
(`skillimport.LooksLikeCredential`, the existing skill check, now exported) refuses the whole read on the machine and
again on the controller, naming the file only.

**Target side.** `planFleet` maps items by agent, kind and file name to the target's own global locations
(`fleetCandidates`); the source path is a label. It builds a `RestorePlan` directly rather than a synthetic snapshot
through `BuildRestorePlan`: that function marks every existing file a conflict and matches without the file's kind, so an
instruction item could land on a settings file of the same name. Settings use a real field merge: `restoreFile.base`
holds the target's own object, `restoreContent` starts from it, and the existing `merge` path (backup, `baseDigest`
re-check before write) now has its first user. The fingerprint covers each file's destination, state, base digest, item
digest and changed fields. `applyFleet` refuses `changed` on a different fingerprint and returns the id of its own
completed journal when a lost reply makes the controller ask again (label `fleet-<batchId>`).

**Widening** (`fleetWidens`): conservative, per agent, unit-tested in `TestFleetWideningPerAgent`. Anything not shown to
be equal or narrower counts as wider; a missing value before counts as the narrowest.

**Undo.** `RollbackJournal` reverses a journal from its own digests; `RollbackRestore` now checks the snapshot and then
calls the same code, so its behaviour and messages are unchanged. `reet setup rollback --journal FILE` with no snapshot
uses the journal-only path.

**Consent.** A machine reads `AllowApply` from its config for every plan, apply and undo
(`ManagerOptions.ApplyConsent`, wired in `internal/scan/agent_terminal_control.go`), as well as terminal access and the
lock. Reading needs terminal access only.

**Retention.** Review: in controller memory (`setupReads`), one per person, 15 minutes, never on disk. Batch:
`setup_batch_content`, deleted on done or abandon (`settleSetupBatchTx`, `endSetupBatchTx`) and by
`PurgeSetupBatchContent` after 24 hours (every 10 minutes and on start). Backups: `Store.VacuumInto` now empties that table
in the copy with `secure_delete` and `VACUUM` (`scrubSetupContent`), so it covers backups and the restore safety copy;
`TestBackupCopyHoldsNoSetupText` greps the copy's bytes. A restore abandons unfinished setup batches (`pauseWork`).

**Decisions not in the brief.** Settings fields the source lacks are left alone on the target (never removed). Running
agent sessions do not exclude a machine: the files are read when an agent starts. An `unchanged` machine is excluded with
code `unchanged`. `confirm` is checked only when a machine widens. The plan's instruction diff is an order-keeping
multiset of added and removed lines (40 each), not a full diff. A merged JSONC or TOML file is written back without its
comments (the rollback copy has them).

**Checked.** `go test -count=1` for setup, skillimport, terminalproto, agenthealth, terminalagent, terminalbroker, scan,
terminalhost, store, backup, setuprun (also `-race`) and the whole of `internal/api`; `go vet ./...`; `gofmt -l`;
`mkdocs build --strict` and `scripts/check-docs.py`.

**Not checked.** No real machines or agents: the API tests use fake connectors and the setup tests real files in temp
homes. Not on macOS. A setup over about 60 KB is refused as `too_large`; a long `CLAUDE.md` can reach that. No web UI.
