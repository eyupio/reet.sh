# Fleet setup copy

Copy one machine's agent setup onto other machines from the controller. You read the
setup from one machine, review a plan for each machine you want to copy it to, and
then reet applies it one machine at a time. Each machine keeps a journal, so a copy
can be undone.

## What is copied, and what never is

Copied, for Claude Code, Codex, OpenCode and Pi, from the machine's own global
(home-folder) locations only:

- **Global instruction files**: `~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`,
  `~/.config/opencode/AGENTS.md` and `~/.pi/agent/AGENTS.md` (or where the agent's own
  override variable points), as whole text.
- **Portable settings**, field by field:

| Agent | File | Fields |
| --- | --- | --- |
| Claude Code | `settings.json` | `permissions`, `outputStyle`, `includeCoAuthoredBy`, `cleanupPeriodDays`, `alwaysThinkingEnabled`, `forceLoginMethod` |
| Codex | `config.toml` | `approval_policy`, `sandbox_mode`, `hide_agent_reasoning`, `file_opener`, `tools` |
| OpenCode | `opencode.json` / `opencode.jsonc` | `permission`, `default_agent`, `theme`, `instructions`, `disabled_providers`, `enabled_providers`, `share`, `autoupdate`, `compaction`, `watcher` |
| Pi | `settings.json` | `theme` |

Never copied: skills (your library puts every Active skill on every machine), MCP
servers and the fields that turn them on, memory, history, project files, model
defaults (`model`, `defaultModel`, `small_model`, `model_provider`, `model_reasoning_*`,
`model_verbosity`, `model_context_window`, `model_max_output_tokens`, `defaultProvider`),
anything that runs a command (hooks, OpenCode's `formatter`, Pi's `packages`, status
lines, credential helpers), environment variables, providers, and every field the setup
export policy keeps back without explicit credential selection. Claude Code's
`~/.claude.json` and `settings.local.json` are not copied: the first holds account state
the agent rewrites all the time, the second is machine-specific.

A file that looks like it holds a credential (a private key block or a well-known token
shape) refuses the whole read. The machine checks before it sends anything, and the
controller checks again; the refusal names the agent and file, never the text.

## Who may copy, and to which machines

Anyone signed in to the browser except a viewer (owners, admins and members) may copy
setups, as for software batches. A machine accepts a copy only when both are true:

1. it runs a reet that negotiated `setup-apply-v1` and `setup-rollback-v1` with this
   controller, with terminal access on (`reet terminal on`), and is not locked; and
2. its local remote-apply opt-in is on: `reet agent allow-apply on`, the same switch that
   lets the library write skills there. The machine reads it again for every plan, apply
   and undo.

The source machine needs `setup-read-v1` and terminal access; reading does not need
allow-apply. Every other machine is left out with a plain reason (see below).

## How a copy runs

1. **Read and review.** reet reads the source machine and asks every chosen machine for a
   dry-run plan. Each machine maps every file by agent, kind and file name to its own
   location; the source's path is only a label and is never written to. Symlinks,
   unreadable files and anything that is not a regular file are `blocked` and left alone.
   Per file the plan says `create`, `merge`, `unchanged` or `blocked`. For settings,
   `merge` keeps the machine's own file and overwrites only the listed fields the source
   has; fields the source does not have are left as they are. For an instruction file,
   `merge` means its text is replaced, and the plan shows the lines added and removed.
2. **Permissions.** Permission settings are copied. The plan flags any machine where the
   copy would let agents do more without asking: a new Claude Code allow rule or extra
   directory, a dropped deny or ask rule, a more permissive `defaultMode`; a Codex
   `approval_policy` further along `untrusted`, `on-request`, `on-failure`, `never`, or a
   `sandbox_mode` further along `read-only`, `workspace-write`, `danger-full-access`; an
   OpenCode permission that becomes more permissive (`deny`, `ask`, `allow`), a new
   `allow`, or a dropped `deny` or `ask`. Anything reet cannot show to be the same or
   narrower counts as wider. A batch with such a machine needs the typed word `copy`.
3. **Apply.** Machines are applied in the order you chose, one at a time. Each machine
   rebuilds its plan and refuses with `changed` if it no longer matches the one you
   reviewed. It writes through the same journal as `reet setup restore`: a rollback copy
   (mode 0600) of every file it changes, a re-check of each file just before it is
   written, and its own writes undone if anything fails. A failed machine stops the batch;
   you then retry it, skip it and continue, or abandon the batch. A machine that was not
   connected when its turn came is skipped (nothing reached it); one whose connection
   dropped after the request went out is a failure, because it may have applied. A retry
   finds the machine's own completed journal and does not write twice. A controller restart
   resumes the batch.
4. **Undo.** Each applied machine can be undone from its journal, once, even after the
   batch has finished. The machine refuses if a file changed after the copy. On the
   machine, `reet setup journals` lists the journal and `reet setup rollback --journal FILE`
   reverses it too.

Settings files are written back as plain JSON or TOML, so comments in a merged
`opencode.jsonc` or `config.toml` are not kept. The rollback copy keeps the original.

## Where the copied text is held

This is the one place the controller ever holds setup text. Between the read and the
batch it is in the controller's memory only, for at most 15 minutes and one review per
person. A batch then keeps it in its own table (`setup_batch_content`), and it is deleted
when the batch is done or abandoned, and in any case 24 hours after the batch was made (a
batch still waiting then is abandoned with code `expired`). It is never in a controller
backup: every backup copy is taken with SQLite's `VACUUM INTO`, and reet empties that
table in the copy (with `secure_delete`) before it is used. Restoring a backup therefore
abandons any unfinished setup copy, and says so. The batch record itself keeps machine
names, file names, field names and plan fingerprints, never text.

A machine's read must fit in one control message: about 60 KB of instruction and settings
text in all. A larger setup is refused as `too_large`.

## HTTP API

All routes need a signed-in browser session (any role but viewer) and, for `POST`, the
controller's own origin. Request bodies are JSON and refuse unknown fields. Errors are
`{"error": "...", "code": "...", "message": "...", "field": "..."}` with a plain sentence.

### `POST /api/v2/setup/fleet/source`

Reads the source machine and reviews every target. Nothing is written.

Request: `{"agentId": "SOURCE", "targets": ["ID", ...]}`. `targets` is optional; without
it every enrolled machine except the source is reviewed (at most 100).

Response `200`:

```json
{
  "readId": "...",
  "expiresAt": 1764000000000,
  "source": {"agentId": "...", "host": "studio", "readAt": 1764000000000},
  "items": [{"agent": "claude", "kind": "instructions", "name": "CLAUDE.md", "format": "text", "label": "/home/me/.claude/CLAUDE.md", "bytes": 812}],
  "machines": [{
    "agentId": "...", "host": "laptop",
    "state": "ready",
    "widens": true,
    "fingerprint": "64 hex",
    "files": [{
      "agent": "claude", "kind": "settings", "name": "settings.json",
      "destination": "/home/me/.claude/settings.json",
      "state": "merge",
      "fields": ["permissions"], "widens": ["permissions"]
    }, {
      "agent": "claude", "kind": "instructions", "name": "CLAUDE.md", "state": "merge",
      "added": ["New line."], "removed": ["Old line."], "truncated": false
    }]
  }]
}
```

`machines[].state` is `ready`, `unchanged` (nothing to change: not an error) or
`excluded`, with `code` and `reason`: `same_machine`, `not_enrolled`, `unsupported`
(older reet), `terminal_off`, `locked`, `not_allowed` (allow-apply off), `offline`, `busy`,
`plan_failed`, `unchanged`. A file's `reason` for `blocked` is `no_location`,
`unreadable` or `destination_symlink` / `destination_error` / `destination_unsupported`.
`added` and `removed` hold at most 40 lines each of up to 200 characters; `truncated`
says some were left out. The review returns no setup text other than those diff lines.

Errors: `404 machine_not_found`; `409 setup_unsupported`, `setup_offline`,
`setup_terminal_off`, `setup_locked`, `setup_busy`, `setup_nothing`;
`413 setup_too_large`; `422 setup_credential` (names the file and agent);
`502 setup_read_invalid`, `setup_failed`; `504 setup_timed_out`.

### `POST /api/v2/setup/fleet/batches`

Request: `{"requestId": "16-128 letters, digits, - or _", "readId": "...", "agents": ["ID", ...], "confirm": ""}`.
`agents` is the order to apply in. `confirm` must be exactly `copy` when any included
machine has `widens`; otherwise it is not checked.

Response `201` (new) or `200` (a repeated `requestId` answers the batch it made, even
after the review expired): a batch.

```json
{
  "id": "...", "sourceAgentId": "...", "sourceHost": "studio", "readAt": 1764000000000,
  "state": "pending", "widens": false, "contentHeld": true,
  "createdAt": 0, "updatedAt": 0,
  "members": [{
    "position": 0, "agentId": "...", "host": "laptop", "fingerprint": "...",
    "widens": false, "files": [{"agent": "claude", "kind": "settings", "name": "settings.json", "state": "merge", "fields": ["permissions"]}],
    "state": "pending", "code": "", "reason": "", "journal": "", "undoneAt": 0
  }]
}
```

Batch `state`: `pending`, `running`, `stopped` (a machine failed; retry, skip or
abandon), `done`, `abandoned`. Member `state`: `pending`, `running`, `applied` (with
`journal`), `failed` (with `code`), `skipped` (`offline`, `abandoned`, `expired`,
`paused_by_restore`, or the failure it was skipped over), `excluded` (with `code` and
`reason`, as in the review, or `not_reviewed`). Failure codes: the machine's own `changed`,
`not_allowed`, `locked`, `off`, `busy`, `nothing`, `invalid`, `write_failed`,
`timed_out`, or `connection_lost`, `apply_unsupported`, `bad_answer`, `apply_failed`,
`content_expired`.

Errors: `400 setup_request_invalid`, `setup_batch_empty`, `setup_batch_duplicate`;
`409 setup_batch_source_only`, `setup_batch_in_progress`, `setup_batch_empty`;
`410 setup_read_expired`; `422 request_id_required`, `setup_confirm_required`.

### Following and steering a batch

- `GET /api/v2/setup/fleet/batches/active`: the person's pending, running or stopped batch,
  or `null`.
- `GET /api/v2/setup/fleet/batches/{batchId}`: one batch.
- `POST .../batches/{batchId}/members/{position}/retry` and `.../skip`: for a failed
  machine. `POST .../batches/{batchId}/abandon`: between machines.
  Errors: `404 setup_batch_not_found`, `409 setup_batch_member_not_failed`,
  `setup_batch_finished`, `setup_batch_running`, `400 software_batch_position_invalid`.
- `POST .../batches/{batchId}/members/{position}/undo`: undoes one applied machine and
  answers the batch, with that member's `undoneAt` set. Errors: `409 setup_undo_done`,
  `setup_undo_unavailable`, `setup_changed` (a file changed after the copy; nothing was
  changed), `setup_no_journal`, `setup_not_allowed`, `setup_locked`, `setup_offline`,
  `setup_unsupported`, `502 setup_failed`.

Each step is audited: `setup-read`, `setup-batch` (start and finish), `setup-batch-retry`,
`setup-batch-skip`, `setup-batch-abandon` and `setup-batch-undo`. The audit names machines,
files, fields and journals, never text.

## Protocol

Three negotiated features, each its own permission: `setup-read-v1` (read),
`setup-apply-v1` (plan, the dry run, and apply) and `setup-rollback-v1` (undo). A machine
claims each only when the controller advertised it and terminal access is on; a controller
never sends an operation the machine did not negotiate, and a machine treats one as a
protocol error. The message is `setup_fleet` with `op` `read`, `plan`, `apply` or
`rollback`, answered by `setup_fleet_result` with closed reasons only; paths, error text
and file contents other than the read's items stay on the machine.
