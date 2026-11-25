## BATCH-ACTIONS-1: a software batch can roll back or remove reet's copy, behind the typed word (25 November 2026)

The roadmap's "Batch maintenance for every action" item.

**What changed.** `POST /api/v2/software/batches` takes an optional `action`. Empty or `"update"` is today's batch (each
machine's reviewed install or update, unchanged); `"rollback"` and `"remove"` apply that one action to every machine that
can take it; anything else is 400 `software_action_invalid`. A rollback or remove needs `confirm` equal to the action word,
or it is 422 `software_confirm_required` and nothing is created, so a script cannot skip the question. Per machine,
`batchSoftwareAction` (`internal/api/software.go`) includes only a reet-managed copy, and for a rollback only one with a
kept previous version; the rest are excluded with a plain reason ("No earlier version is kept on that machine.", "...
installed outside reet, so reet won't touch it.", "OpenCode isn't installed on that machine."). The plan is still built by
`agentsoftware.For`, so target, expected state, version, previous and managed are frozen exactly as the single-machine path
does. A running OpenCode session still excludes the machine. The batch now carries `action` (read from its members in
`loadSoftwareBatch`, no schema change), and `software-batch` and its `-retry`, `-skip` and `-abandon` events record it.

**A real bug fixed on the way.** `softwarerun.reconciled` treated a refused member as done when the machine had reet's copy
at the member's target, and a remove has no target, so it fell back to the reviewed version: a refused remove on a machine
still running reet's 1.19.5 would have been recorded as applied. A remove now reconciles only when the machine reports
OpenCode missing or installed outside reet (`TestRefusedRemoveIsAppliedOnlyWhenReetsCopyIsGone`).

**Web.** Fleet software, under Machines: a **What to do** choice (Update (reviewed), Roll back, Remove reet's copy), each
offered only when a machine can take it, default unchanged. Rows show which machines can and why the others cannot. The
batch question names each machine and what happens to it, says remove keeps settings and never touches a copy outside
reet, and the final button works only once the word is typed (the pattern from a machine's Settings remove). Progress and
stop text say rolling back / removing; an applied member reads "Removed" or "Rolled back to X". The order list's per-row
"Remove" became "Leave out" so it does not read as the remove action.

**Checked.** Go: `gofmt -l internal/ cmd/` (only the known file), `go vet ./...`, `go test ./internal/softwarerun/
./internal/store/ ./internal/agentsoftware/ ./internal/terminalproto/` and `go test ./internal/api/ -run
'Software|Batch|Mirror'`. New API tests: rollback and remove include/exclude the right machines with reasons and send the
action to the machine; missing, empty or wrong `confirm` refused with no batch; unknown action refused; default unchanged;
the audit records the action. `softwarerun`: rollback and remove batches go in order and stop at the first failure. Web:
`npm run check` (0 errors), `fleet-software.e2e.mjs` all cases at 390 and 1280, light and dark; the default request is
asserted to carry no `action` or `confirm`.

**Not checked.** The full `internal/api` suite; other browser suites; a real machine rolling back or removing in a batch
(the connector path is the single-machine one, already tested); a screen reader on the new radio group.
