# Agent software: observe OpenCode, review a plan, install or update

reet can report what it can see about an installed AI agent, show a reviewable
plan, and **install, update, roll back or remove its own OpenCode copy** on
**Linux**. reet never changes an OpenCode installation it did not make.

## What is observed

The connector inspects the executable its launch catalogue would start, then
either reads the receipt of its own managed copy or a bounded `package.json`
beside a system executable. It never runs `opencode`, `npm` or an installer to
observe, and it sends no file contents.

Each `SoftwareEntry` carries:

- `state`: `installed`, `missing`, `unknown` or `unsupported`;
- `managed`: true only for a copy reet installed itself;
- `version`, `package`, `manager` and `source`;
- the resolved executable `path`, `platform` and `arch`;
- `evidence` (for example `verified_sha256` or `package_bin_matches_executable`);
- `operations`: the plans this evidence supports;
- `checkedAt` and, when not installed, a closed `reason`.

The operations follow ownership:

| What is there | Operations | What reet will do |
| --- | --- | --- |
| reet's managed copy | `plan_update`, `plan_remove`; rollback when a previous version is kept | update, roll back or remove it |
| OpenCode installed outside reet | `plan_install` | add a reet-managed copy alongside; the existing one is untouched |
| nothing | `plan_install` | install reet's copy |
| unknown wrapper, other tool or platform | none | inspect only |

A managed copy whose receipt or binary no longer matches is `unknown` with
reason `managed_broken`; reet does not silently fall back to a system binary.
Launches, model listing and inventory prefer the managed copy when one exists.

## Capability and transport

`software-v1` reports the inventory in the connection's capabilities, so an
older controller never receives the field and an older connector reports
nothing. `software-apply-v1` is a separate capability and is reported only while
terminal access is on; an older machine answers that it cannot install.

There is no new local switch: the machine's existing terminal consent is the
gate, and the browser routes require a signed-in writer (viewers are refused).

## The plan

`GET /api/v2/machines/{agentId}/software/plan?tool=opencode&action=install|update|rollback|remove`
returns a plan from the stored evidence. It names:

- the current and target version (the only reviewed version is **1.19.5**);
- the reviewed source URL (`opencode-linux-x64@1.19.5` on the npm registry);
- steps, and the effect on running OpenCode sessions;
- `executes: false`: the plan itself runs nothing.

An update from 1.19.5 answers `state: "noop"`. An install on a machine with a
system OpenCode says it adds a managed copy alongside and leaves the existing
installation alone. Update, rollback and remove are offered only for a
reet-managed copy; rollback needs a kept previous version.

## Applying it

`POST /api/v2/machines/{agentId}/software/apply` with `{"tool":"opencode",
"action":"install"|"update"|"rollback"|"remove"}`. The browser shows one
confirmation dialog with the machine, version, source and effects. Removing
requires typing the machine's name; the plan says the managed copy and every
version reet kept are removed, and that a system copy is untouched.

The controller rebuilds the plan from stored evidence and refuses while **any
live OpenCode session** runs on that machine. The request carries only the tool,
action and the evidence the person approved (state, version, whether it was
managed); the URL, checksums and command stay in the connector.

The connector re-inspects the machine and refuses if the evidence changed. For
install and update it then:

1. downloads the reviewed tarball with a bounded response;
2. verifies the archive's recorded sha512;
3. extracts only `package/bin/opencode` and verifies its sha256 and size;
4. installs it under `~/.reet/software/opencode/versions/1.19.5/` with a
   per-version receipt and a current receipt;
5. verifies the current binary and reports the fresh evidence.

Nothing is executed from the archive, no npm script runs, and no shell command
comes from the controller. If verification fails, the previous managed version
is restored.

A rollback verifies the kept binary's sha256 before making it current, and keeps
the version it came from so an update can reverse it. A remove deletes only
reet's managed tree (including the versions it kept); configuration, history and
credentials live outside it, and a system installation is never touched.

## Fleet review

Machines → **Software** opens a review across enrolled machines. Each row is
built from stored evidence and names the machine's version, the plan it would run
(install, update, none or needs attention), the target and source, and whether an
OpenCode session would make reet refuse the apply. The review reads no machine.
**Apply plan** runs the same per-machine request for that one machine after a
confirmation; other machines are untouched. Rollback and remove stay in a
machine's Settings.

## Batch apply

The rows a machine can apply now also carry a checkbox. Chosen machines form an
ordered batch; up and down move a machine in the order, and **Review batch** shows
the machines, their actions, targets and sources before anything runs. The
controller rebuilds each plan from stored evidence and freezes it, so a machine
whose evidence no longer supports its plan is left out with the reason shown.

A batch applies to one machine at a time, in that order, and stops at the first
failure. A stopped batch names the failed machine and offers **Retry**, **Skip and
continue**, or **Abandon batch**. A machine already at the target version (for
example after a lost reply) is reconciled as applied instead of failing. A member
left running by a controller restart is tried again and reconciled the same way.
At most one batch is active per signed-in person, and a machine can be in only one.

The batch is a durable record: `POST /api/v2/software/batches` creates it from an
idempotent `requestId` and an ordered `agents` list, `GET
/api/v2/software/batches/{id}` reads it, and `…/members/{position}/retry`,
`…/members/{position}/skip` and `…/abandon` move it on. Progress is read by
polling. Activity records a `software-batch` event at the start and at each
terminal state, plus `software-batch-retry`, `-skip` and `-abandon`.

## Controller artifact mirror

A machine normally downloads the reviewed archive from
`registry.npmjs.org`. When the controller's **Software artifact mirror** is on
(Admin → **Software artifact mirror**, owner or admin), a machine that cannot
reach the registry falls back to the controller over its private connection. The
controller downloads the archive once on demand, verifies its recorded sha256 and
sha512 and the extracted binary's sha256 and size against the same pinned table
the connector uses, and writes it to a content-addressed file only after the
whole verification passes. A partial or corrupt file is never served, and
concurrent requests share one download.

The machine asks for the digest in its own pinned table, never a URL the
controller chose; it verifies the archive and binary again before installing.
The registry is tried first, the mirror only if that fails. The result reports
`registry` or `mirror`; when both were tried and failed it reports both and the
refusal says so. Admins can **Cache the reviewed artifact now** to prepare the
mirror before a machine needs it. The cache keeps only the reviewed digest,
garbage-collects anything else at controller start, and is excluded from
backups because it can be fetched again.

## After a restart

On connector start, reet reconciles its managed tree: it removes staging files
and version directories an interrupted apply left behind, verifies the current
binary, restores the kept previous version if the current one is broken, and
clears a rollback target that no longer verifies. A broken current with no valid
previous stays `managed_broken` and is reported rather than hidden or replaced
by a system copy. This runs once at start, never during an apply.

Refusals and failures use `409`/`502`/`504` with a code such as
`software_changed`, `software_sessions_running`, `software_download_failed`,
`software_checksum_failed` or `software_install_failed`; the message says what
was and was not changed.

## Where it shows

Machines → a machine's settings → **Software** lists the evidence and offers the
plans the evidence supports: install, update, rollback and remove. When the
machine reports `software-apply-v1`, the plan has **Apply**; otherwise it says to
update reet on that machine. Install, update and rollback use one confirmation
dialog; remove additionally needs the machine's name typed. Activity records a
`software-apply` event with the machine, tool, action, version and outcome.

## Limits

- OpenCode only, on Linux only. Other tools and operating systems are inspect-only
  or unsupported.
- 1.19.5 is the only reviewed version; a newer release is not assumed
  compatible. No registry version lookup is made.
- The connector downloads the artifact from `registry.npmjs.org`, or from the
  controller's mirror when that is on and the registry is unreachable. The
  controller must reach the registry to cache the artifact; a machine with
  neither access still cannot apply it.
- Apply is synchronous. The registry download is capped at five minutes and the
  mirror leg at fifteen, inside a twenty-minute apply; a slow link can time out
  and nothing is changed. The fleet review applies one machine at a time; a batch
  applies install and update only, one machine at a time. The connector does not
  resume a partial download, and the mirror serves one reviewed version.
- Integrity is verified at install. A launch checks the binary's recorded size,
  not its full hash, on every run.
- The checks use fake artifacts and temporary homes; no physical machine, live
  npm download or real OpenCode install was changed.
