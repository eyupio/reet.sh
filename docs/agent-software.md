# Agent software: observe OpenCode and review a maintenance plan

reet can report what it can see about an installed AI agent, and show a
reviewable plan to install, update or remove **OpenCode** on **Linux**. This
slice observes and plans only: it does not download, run a package manager or
change a machine.

## What is observed

The connector inspects the executable its launch catalogue would start for
OpenCode, then reads a bounded `package.json` beside it. It never runs
`opencode`, `npm` or an installer, and it sends no file contents.

Each `SoftwareEntry` carries:

- `state`: `installed`, `missing`, `unknown` or `unsupported`;
- `version`, `package`, `manager` and `source` (`node_package`, `package.json`);
- the resolved executable `path`, `platform` and `arch`;
- `evidence` (for example `package_bin_matches_executable`);
- `operations`: the plans this evidence supports (`plan_install`,
  `plan_update`, `plan_remove`);
- `checkedAt` and, when not installed, a closed `reason`.

An installation reet does not recognise stays `unknown` and offers no
operations. Another tool or a platform without a reviewed method is
`unsupported`; the connector reads nothing for it.

## Capability and transport

`software-v1` is advertised with the other private control features. A connector
reports `software` only when the token was negotiated and terminal control is
available. There is no new local switch and no new request: the inventory rides
in the connection's capabilities, so an older controller never receives the
field and an older connector reports nothing. The inventory is a snapshot from
when the connection was established; reconnect the machine to refresh it.

## The plan

`GET /api/v2/machines/{agentId}/software/plan?tool=opencode&action=update`
returns a plan from the stored evidence. `action` is `install`, `update` or
`remove`. The plan names:

- the current and target version (the only reviewed version is **1.19.5**);
- the exact command a person would run, such as
  `npm install -g opencode-ai@1.19.5`;
- steps, and the effect on running OpenCode sessions;
- `executes: false`, always. reet has not run the plan.

An update from 1.19.5 answers `state: "noop"`. Removing keeps configuration,
history and credentials on the machine. A missing installation can only be
installed; the other actions answer a refusal that says so.

Refusals use `409` with a code: `software_unsupported` (update reet, or no
reviewed plan), `software_not_installed`, `software_unreadable` and
`software_action_mismatch`; an unknown action is `400
software_action_invalid`.

## Where it shows

Machines → a machine's settings → **Software** lists the evidence and offers
**Review install plan** or **Review update plan** when the evidence supports it.
The plan is shown inline; there is no Apply button in this slice.

## Limits

- OpenCode only, on Linux only, with a recognised global Node package layout.
  Wrappers, other package managers and other operating systems stay
  inspect-only or unsupported.
- No package registry is queried. 1.19.5 is the version reet's ACP connection
  was probed against; a newer release is not assumed compatible.
- Nothing is executed, and no configuration, skills, history or credentials are
  changed. Verified execution with rollback is the next slice.
- The inventory is refreshed when the connector reconnects, not on a timer.
- No physical machine, package-manager run or live OpenCode install was changed
  while checking this slice; tests use temporary package layouts.
