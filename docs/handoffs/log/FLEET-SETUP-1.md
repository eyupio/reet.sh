## FLEET-SETUP-1: one machine's agent setup can be copied to other machines through the controller (25 November 2026)

The back end of the roadmap's "Fleet-wide agent configuration" item. The web page is described under "The web part" below; the roadmap entry is gone. [docs/fleet-setup.md](../../fleet-setup.md) has the API.

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

### The web part (Setups page)

**Where.** A **Copy to other machines** button in the Setups page header opens `web/src/lib/FleetSetup.svelte`, a dialog in
the shape of Fleet software (`FleetSoftware.svelte`): the same sheet, order list, progress list, Retry / Skip and continue /
Abandon, confirmation dialog and typed word. Pure helpers (plain names, sizes, file and machine lines, member notes, the
batch sentence, one plain sentence for every API error code) are in `web/src/lib/fleetSetup.ts`, with
`fleetSetup.test.mjs`. The API calls sit with the software ones in `web/src/lib/terminal/api.ts` (types and functions
only; no import added). `SetupProfiles.svelte` also asks for the person's active copy on load and shows "A setup copy is in
progress. Open it" when there is one; the dialog restores it, as Fleet software does.

**Flow.** Copy from (a machine list, from `/api/v2/terminal/machines`) then Read setup, then a review: what would be copied
(plain item names and sizes), then one row per other machine: ready (every file as Create / Merge / Unchanged / Blocked,
fields changed, the bounded added and removed lines), already the same, or left out with a plain reason by code. Ready
machines are ticked by default. Copy order, then one button, "Copy to N machines", then one question that names the
source and when it was read and every machine in order with what happens to it. The typed word `copy` is asked for only
when a chosen machine `widens`; the request carries `confirm: "copy"` then and no `confirm` key otherwise (the word is
trimmed and lowercased first). Leaving a widening machine out takes the word away.

**Decisions not in the brief.** (1) A finished copy's id is kept in this browser (`localStorage`, one key, wrapped in
try/catch), because `/batches/active` stops answering once a batch is done and the Undo would otherwise be lost on reload;
it is read back with `GET .../batches/{id}`, and "Start another copy" forgets it. (2) Undo asks its own question and says
`setup_changed`, `setup_undo_done` and `setup_undo_unavailable` in words; on the last two it reloads the batch. (3) A
`setup_read_expired` on start closes the question and returns to "Copy from" with the sentence. (4) `setup_credential`
shows the controller's own sentence (it names the file and agent) when it has the expected shape, and a generic one
otherwise; no setup text is ever shown beyond the plan's diff lines. (5) The source list shows every machine, marking
those that are not ready; the controller refuses the rest in words.

**Design passes (Impeccable, Operate mode).** Audit found: the widening note used a side stripe (the detector flagged it),
the ✓ / ✕ marks ran into their label, an undone machine looked like a success, the progress rows broke the number from the
name at 390 px, file chips had ragged widths, and the stopped reason was said twice. Distill removed: the tutorial
paragraphs once a machine has been read (the "never copied" and retention notes move beside the items), the per-file
widening line (one note per machine names the fields), the repeated stopped reason and hint, move buttons with only one
machine, and "Read again" as a second primary. Polish: full border for the widening note, fixed chip width, a gap after the
marks, Undone in the muted colour and without its file line, number beside name at 390 px, 44 px targets on coarse pointers
(asserted in the browser test), and the typed-word border in `--line-deep`.

**Checked (web).** `npm run check`, `node --test` for `fleetSetup.test.mjs` and `theme.test.mjs`, `npm run build`, and
`web/tests/fleet-setup.e2e.mjs` (mocked API, 390 and 1280 px, light and dark) plus `fleet-software.e2e.mjs` and
`setups.e2e.mjs` unchanged. **Not checked.** Against a real controller or machines; a real screen reader; Safari.

**After critique.** The widening warning is its own bordered block above the machines in the question, naming each file and the
fields the plan says widen (and any added lines it lists; the API gives lines for instruction files only today). The typed
word is still exactly `copy`, but its label names the machines that widen and the input is described by the warning. "Never
copied" is a plain line in the review and the question. Instruction files show a Replace chip (settings stay Merge). The
progress headline alone is the live region, and focus moves to it after start, Undo, Retry, Skip and Abandon. An undone
machine says "Put back as it was." and an all-undone copy reads "Copy undone on N machines." in the muted colour. The
action bar is pinned to the bottom of the dialog (`data-shell-foot`; on a phone the whole sheet scrolls, so the bar sticks
to it), and the order list shows only with more than one machine ticked. A stopped copy names the machine to retry.

