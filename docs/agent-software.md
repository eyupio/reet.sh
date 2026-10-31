# Agent software: observe OpenCode, review a plan, install or update

reet can report what it can see about an installed AI agent, show a reviewable
plan, and **install or update OpenCode** on **Linux** from one verified artifact.
Remove and explicit rollback are not exposed yet. reet never changes an OpenCode
installation it did not make.

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
| reet's managed copy | `plan_update`, `plan_remove` | update it (remove is not exposed yet) |
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

`GET /api/v2/machines/{agentId}/software/plan?tool=opencode&action=install|update`
returns a plan from the stored evidence. It names:

- the current and target version (the only reviewed version is **1.19.5**);
- the reviewed source URL (`opencode-linux-x64@1.19.5` on the npm registry);
- steps, and the effect on running OpenCode sessions;
- `executes: false`: the plan itself runs nothing.

An update from 1.19.5 answers `state: "noop"`. An install on a machine with a
system OpenCode says it adds a managed copy alongside and leaves the existing
installation alone. Update is offered only for a reet-managed copy.

## Applying it

`POST /api/v2/machines/{agentId}/software/apply` with `{"tool":"opencode",
"action":"install"|"update"}`. The browser shows one confirmation dialog with
the machine, version, source and effects.

The controller rebuilds the plan from stored evidence and refuses while **any
live OpenCode session** runs on that machine. The request carries only the tool,
action and the evidence the person approved (state, version, whether it was
managed); the URL, checksums and command stay in the connector.

The connector re-inspects the machine and refuses if the evidence changed. It
then:

1. downloads the reviewed tarball with a bounded response;
2. verifies the archive's recorded sha512;
3. extracts only `package/bin/opencode` and verifies its sha256 and size;
4. installs it under `~/.reet/software/opencode/versions/1.19.5/` with a
   per-version receipt and a current receipt;
5. verifies the current binary and reports the fresh evidence.

Nothing is executed from the archive, no npm script runs, and no shell command
comes from the controller. If verification fails, the previous managed version
is restored; a system installation is never touched. Remove is not exposed yet,
so the managed tree is kept.

Refusals and failures use `409`/`502`/`504` with a code such as
`software_changed`, `software_sessions_running`, `software_download_failed`,
`software_checksum_failed` or `software_install_failed`; the message says what
was and was not changed.

## Where it shows

Machines → a machine's settings → **Software** lists the evidence, offers
**Review install/update plan**, and shows the plan inline. When the machine
reports `software-apply-v1`, the plan has **Apply**; otherwise it says to update
reet on that machine. The apply uses one confirmation dialog. Activity records a
`software-apply` event with the machine, tool, action, version and outcome.

## Limits

- OpenCode only, on Linux only. Other tools and operating systems are inspect-only
  or unsupported.
- 1.19.5 is the only reviewed version; a newer release is not assumed
  compatible. No registry version lookup is made.
- The connector downloads the artifact, so the machine needs access to
  `registry.npmjs.org`. A controller-side mirror for offline machines is future
  work.
- Apply is synchronous with a five-minute download/install deadline; a slow link
  can time out and nothing is changed. Remove, explicit rollback, fleet-wide
  apply and interruption recovery after a connector restart are future work.
- Integrity is verified at install. A launch checks the binary's recorded size,
  not its full hash, on every run.
- The checks use fake artifacts and temporary homes; no physical machine, live
  npm download or real OpenCode install was changed.
