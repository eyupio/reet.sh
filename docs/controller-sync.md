# Reet.sh skills and enrolled machines

Compare & sync now starts from **Reet.sh shelf & lists**, **My published reet.sh skills**, or a reported machine/agent scope. A reference machine is not required to prepare a new machine. Select skills, choose the target agent/project scope, and save a desired set. Nothing installs merely because a skill is bookmarked or a desired set is saved.

## Connect an installation

Use a binary built with the controller sync feature. Under Account → Machines, create a personal token, choose a unique machine name, and copy the enrollment command. The installer instructions include PATH setup and a version check.

```sh
export REET_URL='https://reet.sh'
# Set REET_TOKEN to this machine's personal API token privately.
reet agent enroll --env work-laptop --allow-apply
reet agent run
```

Enrollment stores the controller, personal token, random installation credential, actual hostname, home directory and tracked project directories in a mode-0600 configuration file (`~/.config/reet/agent.json` on Linux; the platform user config directory on macOS). The controller stores a hash of the installation credential. A personal token remains necessary: revoking it also stops access. Configuration is local to the installation; do not copy it to another machine.

`--allow-apply` is optional and off by default. It opts this installation into executing plans approved through the web UI. Without it, scans and previews work, but remote apply reports a failure without changing skills. The connection wizard has an explicit checkbox to include this option.

Enroll projects with repeatable absolute paths:

```sh
reet agent enroll --env work-laptop --project /work/app --project /work/docs --allow-apply
```

An agent operates only in its enrolled home/project scopes. Create an empty supported skills directory first when connecting a machine with no installed skills; the Machines page supplies the commands. The agent does not install the agent application itself.

Use `--config /absolute/path/agent.json` for a different configuration location, consistently for enroll/run/status/restore. The process reports inventories once per minute and polls every five seconds. `reet agent run --once` performs one poll/job, or an inventory refresh when there is no job, for use with a scheduler. Keep `reet agent run` under your usual process supervisor if it should survive logout. It is a scanner/controller client and does not start another web server.

```sh
reet agent status
reet doctor
```

## Compare, select and apply

1. Choose the source collection and target machine/agent/project. Select individual skills or Select visible after searching.
2. Review selected-only, machine-only, matching, unknown-source and source-conflict counts. A complete recent scan is required to save the target.
3. Choose a mode and **Save desired skills**. Exact and bidirectional modes warn when extras may be removed.
4. Choose **Preview on machine**. The agent reads current files, validates sources/shared folders, and returns a concrete plan.
5. **Review & approve** that plan. The agent checks enrollment, target, desired revision, local plan hash and filesystem state again before applying. Changed plans fail and require a fresh preview.
6. Read Sync activity for queued/running/awaiting approval/succeeded/failed/cancelled status. A successful apply rescans and acknowledges the desired revision.

The UI refreshes while visible, preserves existing data on refresh failures, and shows when data was last refreshed. Onboarding only confirms a complete scan received after the connection attempt began, rather than accepting an old matching hostname report.

### Modes and both directions

- **Add selected skills; keep extras:** the default. Installs selected missing skills and preserves other local skills, including local-only extras.
- **Exact set:** installs selected missing skills and backs up/removes known-source extras after plan approval. Unknown/local-only removals and shared-folder conflicts remain blocked.
- **Bidirectional; review local and web changes:** retains a common set for three-way comparison. Use **Review local & web changes** to combine changes since the baseline, resolve simultaneous source/deletion changes, and save the reviewed selection. New local drift blocks apply until reviewed. Deletions are represented by absence relative to the baseline, preventing silent resurrection.

**Add selected to reet.sh** adopts reported machine skills into the personal shelf or an editable selected list, preserving existing notes and workflow flags. Scanning itself already imports observed skills; adoption is especially useful for adding them to a list. Shared-list read permissions are required for source selection; edit permissions are required to adopt into it. Published-skill selection includes only the current author's published bundles, never drafts.

Desired sets are reviewed selections, not implicit subscriptions to every bookmark or list edit. **Edit selection** reloads the latest source/list so additions and removals can be reviewed and saved as a new revision. Publishing a hosted skill does not by itself authorize installing it anywhere.

Matching is based on skill identity and source, not file contents or upstream commit versions. Local notes/files are not uploaded as hosted bundles, and arbitrary local skill contents are not overwritten during source-conflict resolution. Link an unknown skill to a source, or publish a supported bundle, before selecting it for installation. Saving automatically deduplicates a selected local copy and a selected source-linked copy with the same name: the source-linked copy wins. Apply backs up the installed local copy under `.reet-sync-backups` and replaces it with the selected source. Two different known sources with the same name still require explicit review, and local-only selections still block saving because there is nothing to install from.

### Sync a machine automatically

Machines with saved desired sets show **Sync saved skills** (or **Sync this machine** for one set). It starts a controller-driven run across that machine's saved scopes without a CLI command. The enrolled agent previews each scope, the controller automatically approves plans that only install or replace entries, and the agent applies them and rescans. Plans that would remove skills stay **awaiting approval** and remain visible in Sync activity; later scopes in the run still continue. **Sync now** beside an individual saved set starts the same automatic flow for that scope. The agent must be enrolled, online and running, and remote apply must already be enabled locally. Use **Pause sync jobs** to stop the run claiming further work.

## Controls and recovery

Each machine card carries the actions for that machine (**Sync saved skills**, **Open terminal**, **Commands**) and a **Settings** dialog for how it is set: **Automatic updates**, the release channel with **Switch channel**, **Pause sync jobs**, **Scan now**, **Revoke live access** and **Forget machine**. Pause prevents further jobs and periodic scans; an already running operation may complete. Revocation prevents later claims/authentication. Revoke an old enrollment before re-enrolling a machine with a replacement configuration.

An update is queued through the enrolled agent, from **Review update** on the card when a newer build waits, **Try again** after an attempt that did not finish, or **Switch channel** in the machine's settings. The controller only names the release channel (`latest`, or `dev` for a dev build); a review first shows the build that runs now, the build it moves to, and what is kept. The agent resolves and downloads the published release itself, verifies its checksum and version, keeps the current executable as `reet.previous`, and re-execs into the new binary. A supervised service therefore restarts without a terminal command. If the release cannot be downloaded or verified, the installed binary is left unchanged and the job reports the failure.

The update is one operation with a visible timeline: queued, downloading, verifying, keeping the previous binary, installing, restarting, and waiting for the new agent to report. The agent narrates each step, so the machine card shows the step in progress rather than a single spinner while the update runs. Once it ends, the card keeps one line about the outcome for a day and drops the steps; an attempt that failed or rolled back keeps them behind **Show the steps**, since the step it stopped at is where diagnosis starts. Once the new process starts it must reach the controller; if the controller refuses it, or cannot be reached within five minutes, the agent puts `reet.previous` back, restarts into it, and reports **rolled back** with the reason. The previous binary is then the running build again, and no skills, enrollment or project scopes are touched.

Each machine card shows the running build (`v1.2.3`, or `dev` plus the source commit), the latest build published on that machine's channel, the last attempt and its outcome. Agents report their source commit on every request, so two `dev` builds are distinguishable. The controller caches the latest stable tag and the `dev` build pointer from the public release channel; if that check fails, the card says so and no automatic update is attempted.

**Automatic updates** are on by default for each enrollment. While an agent is online, not paused, and reporting an older build, the controller queues the same verified update job the manual button uses. After an attempt ends — succeeded, failed, rolled back or cancelled — the controller waits an hour before trying again. That wait matters when a machine still reports an old build after an attempt; a successful update normally makes it current and no retry is needed. The switch is in the machine's **Settings**; the card says which way it is set. Turning it off leaves the current binary alone, and the card offers **Review update** when a newer build waits. With it on, the card carries a quiet **Update now** for anyone who would rather not wait for the next check. On an old agent that does not report a commit yet, the card shows `dev · commit unknown`; enabling an update is how it becomes attributable again.

An update may wait behind another queued job or a plan awaiting approval; those jobs do not block it. It is refused only while an operation is still running on the machine, or while another update is already queued. A running job that has sent no result for ten minutes is treated as failed before this check, so an interrupted attempt does not keep blocking later updates.

Queued or awaiting-approval jobs can be cancelled. Running operations are not presented as cancellable filesystem transactions. Failed or cancelled jobs offer **Retry with fresh preview/scan**, so an old approved plan is never blindly replayed. Distinct agent scopes may queue read-only previews together. Duplicate previews for a scope are blocked, scans wait for active work, and an active scan or apply blocks new previews. Execution remains serial on each installation. Each preview requires its own approval. Execution has an eight-minute command context; work with no result after ten minutes becomes failed and requires inspection and a new preview. A local result is persisted before acknowledgment; reconnecting resends the result rather than reinstalling skills.

Removed entries are moved to `.reet-sync-backups/removed-*/SKILL` beside the selected skills directory. Sync activity includes **Copy restore command** for each successfully backed-up removal, including partial failures. Run it on the enrolled machine:

```sh
reet agent restore --target 'TARGET_ID' --backup '/path/to/.reet-sync-backups/removed-123/skill'
```

Restore verifies enrollment and backup scope, requires typing the target name, refuses to overwrite an existing skill, takes the target lock, moves the original entry back, and rescans. Review the desired set afterward so the next sync does not remove it again. Symlink entries are restored as symlinks.

A killed agent or sync may leave a `.lock` file. Check that the relevant process is no longer running before removing that specific stale lock. Keep any `.result` file for acknowledgment/recovery; the agent drops it only when the controller refuses it permanently (for example because that job has already closed). Do not run multiple workers using copied configuration files.

Renaming an enrolled machine preserves its identity and mode/baseline, cancels old queued approvals, and requires a new preview. Renaming during a running job is rejected. Two active enrolled installations cannot be merged; revoke one enrollment first if consolidating duplicate reports.

Local `reet sync --apply` also requires enrollment and verifies the actual local hostname/home binding and controller identity. Preview remains available without enrollment. Existing one-way reference snapshots remain supported.

## Inventory correctness

Unreadable scopes, permission errors on skill entries and broken symlinks produce scan errors. The controller preserves previous inventory, marks the report incomplete and exposes the error rather than pruning it as a confirmed absence. The CLI exits nonzero for incomplete scans. Existing shelf choices are preserved on import; first discoveries retain the original automatic In use behavior.

Controller state, enrollment hashes, jobs/receipts, desired sets and baselines live in SQLite and survive restart. Use a consistent full database backup to retain them. Personal shelf JSON exports do not include these operational records. Keep each local enrollment configuration private and backed up separately.

See [the control centre workflow](control-centre.md) for fleet controls and selecting all machines.
