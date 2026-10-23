# Skills, lists, setups and machines

The Library visual identity now supports four main tasks. Skills are individual
finds and personal evaluation stages. Lists are curated collections with an audience.
Setups are named records of encrypted local snapshots and restore exclusions.
Machines are observed inventories, comparison targets and reviewed sync controls.

## Routes and tasks

| Destination | Main action | Secondary tasks |
| --- | --- | --- |
| Skills `/` | Add a skill | Evaluate, tag and organise |
| Lists `/shared` | Create a list | Search, share and prepare a machine |
| Setups `/profiles` | Create a setup locally | Inspect coverage, compare revisions, prepare recovery |
| Machines `/machines` | Connect a machine | Inventory, compare, sync and diagnose |

Community, Teams, Host skills, Account and Admin are secondary navigation under More.
Legacy account machine/sync routes still resolve to the machine workspace.

## Recovery presentation

A synced profile contains metadata; it does not prove a snapshot is stored remotely.
Coverage shows captured counts and counts after the selected scopes' exclusions.
The browser cannot check encrypted values or destination files. Capture metadata,
paths and software evidence remain available behind disclosures. Revision comparison
shows metadata additions, removals and changes in the selected scope.

Prepare this machine starts with CLI installation and authentication, then profile
pull and snapshot transfer/fetch. The local preview shows destination paths, warnings,
unsupported items and conflicts. Users stop destination agents, choose each conflict
and approve the final plan. File verification and runtime health are separate outcomes.

List preparation opens comparison with the source collection selected. Target
selection, desired-set saving and execution approval remain explicit separate actions.
Rows suit larger collections; cards remain available and the choice is saved locally.
Admin access does not imply a person was invited to a list.

## Installation

Connection onboarding presents Install, Authenticate and Scan one step at a time and creates no local server. Hosting onboarding previews a native
user service or Compose deployment. The page shows the remote server build separately
from `command -v reet`, `reet --version` and `reet setup support` on the local machine.
The initial channel follows the server when known; capabilities must still be checked.
Tokens are copied into hidden prompts, rather than included in generated shell commands.

## Verification

Use `npm run check`, `npm run build`, Go vet/race tests and the MCP suite. UI helpers
have regression tests for global/scoped exclusions, revision comparison and sharing
relationships. Browser checks use synthetic data and include failed deletion/retry,
mobile overflow, keyboard menu dismissal and dark mode. These are not a WCAG
certification or verification on physical Safari/Firefox/touch devices.
