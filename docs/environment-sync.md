# Environments and confirmed sync

The **Account → Machines → Connect a machine** flow is the starting point: install the binary, create a named personal token in the connection step, then copy the connection command into that machine's terminal. It includes this instance's URL and the newly created token. Copy it before leaving the page; tokens are not saved to browser storage or shown again after a reload. Existing token holders can set `REET_URL` and `REET_TOKEN` directly in their terminal. Use a separate token per machine or server so access can be revoked independently. Generated scan/sync commands use the environment rather than embedding the token. After running the scan, use **Check for my scan** to confirm a complete, recent inventory for the selected machine name.

## Connect a machine

Install a stable binary, or select **Dev** for the rolling main build:

```sh
curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh -s -- --version dev
export REET_URL='https://your-reet.example'
export REET_TOKEN='your-personal-token'
reet scan --server "$REET_URL" --env my-laptop --prune
```

Choose a stable, distinct `--env` name for each machine. It defaults to the hostname; `REET_ENV` supplies a reusable alias. Run the scan from each project you want to track. Global and project lock files retain their separate source information. Project identities now include a hash of the full local project path so two projects named `app` do not collide. Rescan older project environments with the new binary, then forget obsolete reports from Account.

`--prune` updates reported inventory: it does not uninstall skills or delete shelf entries. Empty scans register the machine too. Missing skills folders under an existing agent configuration report an empty inventory; unreadable folders are not treated as empty. An import with errors does not prune old inventory or qualify as a complete reference scan.

If a machine has no skills yet, expand **Connecting an empty machine?** on **Account → Machines**. Create the selected agent's empty skills folder and run the generated scan. This does not install the agent itself.

## Choose a desired skill set

On **Account → Compare & sync**, choose a recently scanned **Reference** and **Target**, then **Save reference snapshot**. Both must belong to your account and have complete scans from the last 14 days. A target can be the shared `.agents/skills` environment used by current Codex, Claude Code, Cursor or OpenCode global environments, or a supported project environment. Legacy `.codex/skills` folders remain scan-only: the current skills CLI installs Codex into `.agents/skills`. Other/custom directories remain visible but require manual installation.

The controller persists the desired snapshot and its revision in SQLite. A later change to the reference machine does not silently change your target: save another reference snapshot when you want those changes. The page shows missing skills, extra skills, last report time and concrete commands for each target. Stale/incomplete reports say to rescan rather than declaring the machine up to date. Counts reflect reports, not live remote filesystem access.

## Preview and apply locally

Copy the target's command from **Account → Compare & sync** and run it on that machine. For a project target, run it from that exact project directory:

```sh
reet sync --server "$REET_URL" --env my-laptop --target 'account/environment/id'
reet sync --server "$REET_URL" --env my-laptop --target 'account/environment/id' --apply
```

The first command previews installs and removals. `--apply` prints the plan and requires you to type the target name. There is no unattended apply flag. The CLI rechecks the controller revision and local files after confirmation, takes a local target lock, and scopes installs through the vetted `npx --yes skills@1.7.4 add` command to the chosen agent and global/project location. Removed entries are moved into a uniquely named `.reet-sync-backups` folder beside the skills directory rather than permanently deleted. A symlink entry is backed up as a symlink, preserving its canonical contents. To restore an entry, move it from that backup to its original skills directory, then rescan. Node/npm must be installed for those commands. Known shared-agent source conflicts, canonical shared-folder removals still referenced by another detected agent, and unknown/local-only skills stop the plan instead of overwriting or deleting them. Resolve their provenance in the skills CLI lock file first, or back up and remove local-only extras yourself.

Commands run sequentially; this is not a filesystem transaction. If a command fails, sync stops and reports the current inventory so the controller can show remaining differences. Successful apply rescans with pruning and checks the local result. If a process is killed while applying, its `.reet-sync.lock` may remain in the skills folder; check that no sync is running before removing that stale lock and previewing again. A viewer account cannot apply a saved target. Custom `CLAUDE_CONFIG_DIR` or OpenCode `XDG_CONFIG_HOME` locations require manual syncing.

This sync matches installed skill sets and their sources. It does not compare file contents, pin upstream commits, copy local notes/files, SSH into machines, or install the agent applications themselves. Review third-party skills before confirming installation. The browser stores the plan on the controller and never runs commands on your machines.

## Servers

A `reet serve` process registers its own hostname, OS, architecture and binary version in its local owner's account and sends a heartbeat every minute. In accounts mode it registers once owner setup has completed. This identity is separate from agent skill inventories and is not itself a sync target.

To report a server's visible skills to another controller, configure the process explicitly:

```sh
export REET_CONTROLLER_URL='https://your-reet.example'
export REET_CONTROLLER_TOKEN='personal-token-from-that-controller'
export REET_ENV='my-server'
reet serve
```

These settings are also available in `.env`/Docker Compose. Remote reports use the configured token's account, never every user's account. A container sees only its own filesystem; mount the skill directories to scan the host's skills. It does not automatically apply changes: run the target's confirmed `reet sync --apply` command locally. Server heartbeats older than five minutes are shown as not seen recently; agent inventory freshness uses the 14-day scan window.

**Forget environment** removes its reports and target configuration, not installed files. Existing snapshots captured from that environment remain on other targets. A new scan or active server heartbeat can register the environment again.

## Persistence and backup

Inventory, scan completeness, runtime details and desired reference snapshots survive server restarts in the controller's SQLite database. Back up the data directory with a consistent SQLite backup or while the server is stopped to retain all of this state. Shelf JSON exports do not yet include sync reference configurations or standalone host heartbeat records.

## Machines and agent groups

An environment is a machine, identified within your account by its stable `--env` / `REET_ENV` name. Claude Code, shared agents/Codex, Cursor and project installations are subgroups of that machine. Use the same name for its scanner and server heartbeat. Use distinct names for different machines. Containers report only the files they can see; explicitly configure the host alias when a container's random hostname is not the identity you want.

The Library filters first by machine, then optionally by agent/project group. Machine skill counts deduplicate skills installed in several agent folders. **Only on one machine** counts machine identities, not agents. Existing scan and sync target IDs continue to identify individual agent/project groups, so saved references and older binaries keep working.

On **Account → Compare & sync**, **Compare machines** shows unique skills shared by the selected machines and skills present on only one side. Select individual agent groups to compare placement rather than combined inventories. A match compares reported skill identities/sources, not installed file contents or versions. Stale/incomplete agent reports and machines with only a server heartbeat cannot confirm a match; unknown/local sources require review. Rescan both machines with `--prune`, then refresh before deciding what to apply.

For a confirmed, source-linked comparison between individual groups, **Use these groups for a sync reference** prepares the snapshot form. Review it and explicitly save the desired reference. The target CLI still previews and confirms installs/removals locally; comparison alone never changes any machine.
