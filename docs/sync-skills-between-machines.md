# Sync skills between machines

A skill that only exists on your laptop is not much use on the VPS. reet.sh keeps a central skill library and sends a reviewed selection to one machine or many.

This guide covers the safe path: scan what is there, choose a source and destinations, preview the exact plan, approve it, and check the result.

## What sync does and does not do

- It installs, replaces or removes skills in the folders your tools read.
- It shows a concrete plan before anything changes.
- It keeps a backup of a replaced local copy under `.reet-sync-backups`.
- It matches skills by identity and source, not by file contents or upstream commit.
- It does not publish private skills, upload arbitrary local files, or change a machine that has not opted in to remote apply.

## 1. Enrol the machines

Install the `reet` binary on each machine and enrol it from **Machines → Add a machine** in the controller. The dialog gives you the exact command:

```sh
export REET_URL='https://reet.example.com'
# Set REET_TOKEN to this machine's personal API token privately.
reet agent enroll --env work-laptop --allow-apply
reet agent run
```

Use `--allow-apply` only on machines where you want plans approved in the web UI to run. Without it, the controller can still scan and preview, but applying is refused locally.

For a machine with several project folders, add repeatable absolute paths:

```sh
reet agent enroll --env work-laptop --project /work/app --project /work/docs --allow-apply
```

## 2. Scan what is there

The connector reports an inventory once a minute. To refresh it on demand, use **Scan all** on the Machines page, or run the scanner on the machine:

```sh
reet scan --server https://reet.example.com --token "$REET_TOKEN" --env work-laptop --prune
```

`--prune` removes the environment link for skills that are no longer in a scanned folder; the skill and its notes stay. Add `--dry-run` to preview the scan.

A complete, recent scan is required before a target can be saved for sync. Stale, incomplete and heartbeat-only reports cannot confirm what a machine has.

## 3. Choose a source and destinations

Open **Sync**. Start from one of these sources:

- **Reet.sh shelf & lists** — your personal shelf, or a list you can read.
- **My reet.sh skills** — published skills by their public URL, and unpublished drafts served privately from this controller.
- **A reported machine or agent scope** — use a machine you already trust as the reference.

Select the skills you want, then choose one destination or **All machines**. Adjust individual scopes if a machine needs something different.

Saving a desired set is separate from previewing and applying it. Nothing installs because you bookmarked a skill, added it to a list, or saved a selection.

## 4. Review the exact plan

Choose **Preview on machine**. The agent reads the current files, validates the sources and returns a plan: what will be added, replaced, removed or left alone.

Read the plan. It is the moment to catch a wrong folder, a source conflict or an extra you did not mean to remove.

Then choose **Apply changes**. The agent checks the enrolment, target, desired revision, local plan hash and filesystem state again before writing. If anything changed, the plan fails and you need a fresh preview.

## 5. Check the result

**Sync activity** shows queued, running, awaiting approval, succeeded, failed and cancelled jobs. A successful apply rescans and records the desired revision.

When a run has a failure or something waiting for review, the page opens with a **Needs you** strip. Read the exact command or reason on the card before retrying; a retry asks for a fresh preview rather than replaying an old approval.

## Modes

- **Add selected skills; keep extras** is the default. It installs missing selected skills and leaves other local skills alone.
- **Exact set** also removes known-source extras after plan approval. Unknown local-only files and shared-folder conflicts stay blocked.
- **Bidirectional; review local and web changes** keeps a common set for three-way comparison. Use **Review local & web changes** to combine drift, resolve conflicts and save a new reviewed selection.

Start with the default mode. Move to exact or bidirectional only when you want the destination to match the selection precisely.

## Troubleshooting

- **A machine is skipped.** It is offline, paused, stale or incomplete, or a preview is already pending. The reason is shown in the UI.
- **Apply fails without changing anything.** Remote apply is off on that machine. Run `reet agent allow-apply on` there, or re-enrol with `--allow-apply`.
- **The plan changed before it ran.** The files, source revision or target changed between preview and apply. Preview again and review the new plan.
- **A skill is unknown or unlinked.** Link it to its source, or publish a supported bundle, before selecting it for installation.
- **Two known sources have the same name.** Resolve the source conflict explicitly. reet does not guess which one you meant.

## Limits

- Sync matches identity and source, not bytes. Two skills with the same name from different sources are not the same skill.
- A replacement backs up the installed local copy, but you should still keep your own backups.
- Applying content changes to private machine skill files is not available yet; the current reviewed path covers installation and replacement through the supported sync flow.

For the full set of controls, read [Skills and enrolled machines](controller-sync.md).
