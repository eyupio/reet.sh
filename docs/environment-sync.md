# Machine inventory and library sync

Every connected machine follows the account's Active skills. [Controller sync](controller-sync.md) covers enrollment, local file consent and recovery; [the library workflow](sync-skills-between-machines.md) covers Archive, Restore, Delete and new machine discoveries. Inventory reports describe what was scanned; they do not choose a separate desired skill set.

The **Account → Machines → Connect a machine** flow is the starting point: install the binary, make an enrolment code in the connection step, then copy the connection command into that machine's terminal and enter the code when it asks (or set `REET_ENROL_CODE`; `--code` also works, but a command line is visible to other users of the machine). A code enrols one machine, once, within 15 minutes, and opens nothing else. The machine then keeps only its own id and secret, never a personal token. A machine enrolled earlier with a personal token moves to its own credential with `reet agent enroll --again` on it, which keeps its local settings. Personal API tokens, which `reet scan` from a terminal still uses, are made and revoked only from a signed-in browser. Run `reet` and choose **Connect machine** to open the existing enrollment command. Each enrolled machine has its own secret, so removing one machine under Machines revokes it alone. Generated scan/sync commands use the environment rather than embedding the token. After running the scan, use **Check for my scan** to confirm a complete, recent inventory for the selected machine name.

## Connect a machine

Install a stable binary, or select **Dev** for the rolling main build:

```sh
curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh -s -- --version dev
export REET_URL='https://your-reet.example'
export REET_TOKEN='your-personal-token'
reet scan --server "$REET_URL" --env my-laptop --prune
```

If `REET_TOKEN` is unset, `reet scan` prompts for the token and saves it in
`reet/token` under the platform user configuration directory with owner-only
permissions. Later scans reuse it; `REET_TOKEN`, `--token` or `--env-file` still
take precedence. Remove the saved copy with `reet configure --forget-token`.

On a machine enrolled with `reet agent enroll` and no personal token anywhere,
`reet scan` reports with the machine's own enrolment instead (`--agent-config`
names another enrolment file): only to the controller it was enrolled with and
only under its own name, so `--server` and `--env` may be left out and a
different one is refused before anything is sent. `--reconcile` is then skipped
with one line, since merging machine names needs a personal token or **Merge** on
the Machines page; the inventory still goes. A token, when there is one, wins.
The running agent scans by itself as soon as `reet agent run` starts and every
minute after, so an enrolled machine needs no manual scan.

Choose a stable, distinct `--env` name for each machine. It defaults to the hostname; `REET_ENV` supplies a reusable alias. Run the scan from each project you want to track. Global and project lock files retain their separate source information. Project identities now include a hash of the full local project path so two projects named `app` do not collide. Rescan older project environments with the new binary, then forget obsolete reports from Account.

`--prune` updates reported inventory: it does not uninstall skills or delete shelf entries. Empty scans register the machine too. Missing skills folders under an existing agent configuration report an empty inventory; unreadable folders are not treated as empty. An import with errors does not prune old inventory or qualify as a complete reference scan.

If a machine has no skills yet, expand **Connecting an empty machine?** on **Account → Machines**. Create the selected agent's empty skills folder and run the generated scan. This does not install the agent itself.

## Choose which skills machines receive

Use **Skills → Active** and **Archived**. Every connected machine receives Active skills and removes Archived or Deleted copies, with backups. There is no per-machine reference snapshot or preview/apply command. `reet sync` prints its replacement and exits without changing files.

An unknown skill waits for Keep or Archive under **New on a machine**. Scanning alone does not add it to the library. Use **Sync** to read progress, retry a failure or see why a machine is waiting.

## Servers

A `reet serve` process registers its own hostname, OS, architecture and binary version in its local owner's account and sends a heartbeat every minute. In accounts mode it registers once owner setup has completed. This identity is separate from agent skill inventories and is not itself a sync target.

To report a server's visible skills to another controller, configure the process explicitly:

```sh
export REET_CONTROLLER_URL='https://your-reet.example'
export REET_CONTROLLER_TOKEN='personal-token-from-that-controller'
export REET_ENV='my-server'
reet serve
```

These settings are also available in `.env`/Docker Compose. Remote reports use the configured token's account, never every user's account. A container sees only its own filesystem; mount the skill directories to scan the host's skills. It reports what it sees and changes nothing on that server. Server heartbeats older than five minutes are shown as not seen recently; agent inventory freshness uses the 14-day scan window.

**Forget environment** removes its inventory reports, not installed files. A new scan or active server heartbeat can register the environment again.

## Persistence and backup

Inventory, scan completeness and runtime details survive server restarts in the controller's SQLite database. Back up the data directory with a consistent SQLite backup or while the server is stopped to retain all of this state. Shelf JSON exports are not a complete replacement for an instance backup, including skill content and machine records.

## Machines and agent groups

An environment is a machine, identified within your account by its stable `--env` / `REET_ENV` name. Claude Code, shared agents/Codex, Cursor and project installations are subgroups of that machine. Use the same name for its scanner and server heartbeat. Use distinct names for different machines. Containers report only the files they can see; explicitly configure the host alias when a container's random hostname is not the identity you want.

Machine views group reported skills by agent or project. A matching name or source is inventory evidence, not proof that file bytes match. Stale or incomplete reports and server-only heartbeats cannot confirm absence. Project inventories remain useful for session pickers even though the shared library manages home folders only.

## Repair duplicate machine names and refresh a fleet

A legacy scan under a hostname and another under `--env NAME` can create two machine reports. On **Account → Machines → Manage machine names & duplicate reports**, select the hostname report and enter the preferred name (for example `PROD_Snicket`). Review and confirm. Enter an existing machine name to merge or a new name to rename. Only combine reports you know belong to the same physical machine; equal skill counts are not proof.

Matching agent/project groups keep the newest reported inventory rather than unioning old and new skill lists. Old names become account-scoped aliases so subsequent hostname-based scans use the preferred name. Renaming an inventory report is separate from changing an enrolled installation's identity.

To repair the current hostname and refresh inventories from the binary, update reet and run on that machine:

```sh
reet scan --server https://reet.sh --env PROD_Snicket --prune --reconcile
```

Set `REET_TOKEN` to that account's personal token first. `--reconcile` requires an explicit preferred name through `--env` or `REET_ENV`; it binds the actual hostname to that name before reporting. It works on existing installations and does not install/remove skills. `--dry-run` sends neither a merge nor a report. A reference conflict stops reconciliation before the scan is imported. Ordinary older scanners still benefit from aliases already recorded on the controller.

**Full fleet inventory refresh** provides the command for each scanned machine. Choose one preferred name per physical machine, run there, and repeat from each tracked project. This refreshes detected agent groups; it cannot assert the state of offline machines, unrelated projects or unreadable/unscanned folders. Enrolled machines that are online can be rescanned and synced from **Machines** without a terminal command; other machines need the commands above. Open the refresh check before running the commands, then use **Refresh status** after each command completes. Progress counts only complete agent reports received since that check started, not cached scans. Start a new check to reset the progress window.

Expand a machine card to **Forget machine** when retiring its reports, or forget an individual agent group. Whole-machine removal clears reports and target references without deleting installed files or shelf entries. Reference snapshots used by other targets and name aliases remain; a later active scan/heartbeat can register the machine again. Viewer accounts cannot merge, rename, forget or run writes through these endpoints.
