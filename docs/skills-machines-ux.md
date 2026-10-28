# Skills and machines UX pass

This pass looked at the two surfaces people use together: the Skills shelf (`/skills`) and
the machine workspace (`/machines`, `/sync`). The goal was a single obvious next action, live
feedback while it runs, and no dead ends when a skill is missing from a machine.

## What was getting in the way

1. **Sync lived below the fold of a machine card.** A connected machine showed its inventory,
   agent scopes, terminal and settings before the action people actually wanted. The sync
   button sat in the same row as Commands and Settings, and its label changed between
   “Choose skills to sync”, “Sync this machine” and “Sync saved skills”.
2. **The card had no sync state.** Freshness came from an inventory badge, while installs,
   approvals and active runs were inferred from a thin line in the metadata row.
3. **Starting sync gave no close feedback.** Progress lived in the Sync page's activity
   disclosure. A person who pressed Sync now on a machine card saw “Starting…” and then had
   to leave the page to find out what happened.
4. **“Missing” was a statement, not an action.** A skill badge could say `Missing` on a
   machine, but installing it meant going to Sync, choosing the machine and scopes, saving a
   desired set, previewing and approving. Every step was individually safe, but the path was
   far longer than the user's intent (“put this skill there”).
5. **The same intent had too many names.** “Choose skills to sync”, “Sync saved skills”,
   “Sync now”, “Preview all on machine” and “Sync all now” described related operations from
   different places with no shared vocabulary.

## What changed

### Machine profile band

Each machine card now opens with a sync band under its identity:

- one plain-language state: `N skills to install`, `Everything in sync`, `Syncing now`,
  `Review needed`, `Rescan needed`, `Sync paused`, `Sync not connected`, or
  `No skills set to sync`;
- one primary action: **Sync now** when it can work, **View progress** while a run is active,
  or **Set up sync** when the machine still needs a scan or a desired set;
- a `⋯` menu for the deliberate alternatives: choose skills and scopes, preview only, rescan,
  and open sync activity;
- a thin progress bar while a run is walking through saved scopes.

The former primary sync buttons were removed from the general action row, so the card has one
sync decision instead of two competing ones. Terminal, Start an agent, Commands and Settings
remain secondary actions.

The state comes from `machineSyncOverview` (`web/src/lib/syncOverview.ts`), which is unit
tested against active runs, queued jobs, approvals, stale scans, paused agents and
unconnected machines.

### Live sync activity

`SyncActivity.svelte` is a non-modal dock mounted on Machines and on Skills while an install is
running. The Sync page owns its own activity section, so the dock never covers the page that
already lists the same jobs. The dock:

- appears as a compact one-line bar when a job starts, rather than opening a panel over the page;
- expands on demand to the run position (`1 / 3 scopes`), the current scope, a live progress bar
  and the stage timeline (`Read machine → Plan changes → Apply changes`);
- stays for 15 seconds after a run finishes so the outcome is visible from the page that started
  it, and can be minimised or dismissed without losing the work;
- polls every two seconds while visible, while both pages continue their normal refresh;
- keeps motion behind `prefers-reduced-motion` and stacks above the phone bottom navigation.

The Sync page's activity section is the detailed view. It groups by machine, keeps running work
first, collapses identical failures into one card with their scopes and job count, and folds
finished jobs into **Earlier activity**. A repeated failure is retried once per scope, so a
repeated preview cannot queue the same work twice. Only the first job that needs a person opens
by default; every other card is one press away.

Failure cards say the exact fix once, name the machine, and replace “try again” with “retry”.
When a machine is offline the commands and retry are hidden and the card says the fix runs on the
machine, rather than leaving a command that cannot work yet.

### Add now from a missing badge

When a skill shows as missing on a machine, the badge now carries **Add now**. The skill's
`⋯` menu and a right-click anywhere on the card offer the same action per missing machine,
plus a link to review that machine's sync settings. The menu is keyboard reachable and closes
on Escape or an outside press.

`installSkillOnMachine` (`web/src/lib/syncActions.ts`) makes the intent work:

1. read the live sync state;
2. require an enrolled, unpaused agent and a recent complete scan;
3. reuse every fresh saved sync set on the machine whose source contains the skill, preserving
   each set's mode and revision;
4. if the machine has no saved set, create one additive set on the preferred shared `agents`
   scope, then start a sync run;
5. otherwise explain, in plain language, whether the skill needs a linked source, the machine
   needs a connector, the sync is paused, or the skill is outside the existing saved source.

Nothing is applied directly from the browser. The existing controller contract still applies:
the agent previews the exact plan, additive work may apply automatically, and removals require
review. The dock makes that journey visible instead of hiding it in another page.

## Deliberate boundaries

- **No one-click “install everything”.** Add now installs one selected skill. The shelf is not
  a licence to overwrite a machine's saved set.
- **No new apply path.** The browser queues the same jobs and runs the Sync page uses. A
  viewer still cannot install, and a paused or unconnected machine explains why before any
  write.
- **A failed or stale inventory never claims a skill is missing.** The existing `unknown`
  state is unchanged and does not get an Add now button.
- **Saved sets are reused, not redefined.** A skill that is not in an existing list- or
  hosted-backed set is not silently added to it; the person is sent to Sync to choose.

## Verification

```sh
cd web
npm run test        # includes syncOverview.test.mjs
npm run check       # tests + svelte-check
npm run build
```

The pure helpers cover the state hierarchy, saved/fallback destinations, list-backed source
normalisation, candidate eligibility, job stages and plan counts. Browser checks still need a
running Go server and an enrolled agent for a real end-to-end apply.

## Still worth doing next

- A guided first-run “put this skill on this machine” dialog for people with several agent
  scopes, so the default scope is explained rather than assumed.
- One vocabulary across the product: choose between “sync set”, “desired set” and
  “saved skills” and use it everywhere.
