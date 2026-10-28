# Reet as the central control unit

The main user task is to reach machines, open remote terminals and return to
ongoing work. **Home** sits at `/`: the fleet strip, In flight, Needs you and the
Project rail (see [Home and the tabs](terminal-workspace.md#home-and-the-tabs)).

**Control is folded into Home and Machines** (R4 in the agents home design). Its
active-session band is replaced by In flight on Home and the Sessions page; its
machine connection cards by Home's fleet strip and the Machines page. The
first-terminal checklist ("Get your first terminal running") now opens the
Machines page for a writer until a machine has done its part on a server that
allows terminals. `/control` redirects to `/`, and Control is no longer under
More. Home, Sessions and Machines are the primary tabs; Skills joins them on a
desktop. Activity lives in the account page at `/account/activity`, and the old
`/activity` address opens that section (an open instance without accounts keeps
its own Activity page and tab). Sync, lists and portable setups are under More
with their existing routes. Legacy machine comparison and account links continue
to work.

## Sessions

The Sessions tab is `/sessions` (R17). Without a session in the address it lists
every session across machines: search, Group by Project, Machine or State (the
same choice as Home's), filters (state, agent, machine, YOLO only, worktree only)
and each row's actions menu (right-click, Shift+F10 or the ⋮ button), with the
folder projects and **Add a project** below. `/sessions?agent=…&session=…` opens
that session. `/terminals` redirects to `/sessions` keeping its query, so every
old link to a session still opens it. Ready, setup-needed, reconnecting and
last-known states remain distinct; a machine that is not reporting says so on
its group. Viewers see machine information without requesting private terminal
data. The command library opens from **Saved commands** in the Sessions header, or
from a session's toolbar. See
[terminal workspace](terminal-workspace.md) for the session page, sidebar
actions, appearance, account commands and close defaults.

## Skill distribution workflow

1. Connect a machine from Home or Machines. Install/authenticate the CLI,
   scan its agent scopes, and enroll its background agent for remote jobs.
2. Maintain skills in the Skills library, or create owned skills in Host skills.
   Bring selected machine skills into reet from Sync. Existing notes survive.
3. Choose a source in Sync: library/list, published skills, or another machine's
   reported scope. Choose a machine or All machines, then select agent scopes.
4. Select skills and additive, bidirectional review or exact mode. Inspect each
   destination's differences, then save desired sets. Source conflicts block save.
   Incomplete or stale destination reports must be refreshed first.
5. Preview a scope, a machine, or the fleet. Review exact plans and approve them
   individually. Bidirectional review combines local/web changes through the
   existing three-way reconciliation, with explicit conflict choices. Comparisons
   establish identity/source equality, not bundle contents or installed versions.
6. Open a machine terminal and launch an installed agent as usual. Terminals
   require account authorization, server enablement and local machine opt-in.
   There is no automatic agent launch or software installation.

## Bulk operations

In More → Sync, Scan all queues one scan per recently reporting, unpaused enrollment. Sync all
queues previews for saved targets with recent complete inventories and available
agents. Pending work, offline/paused agents and stale/incomplete reports are
listed as skipped. A preview on another scope does not block a distinct scope's
preview, but scans and existing applies block new previews. The controller still
claims only one running job per machine. Exact plan hashes and revisions remain
required for approval and execution.

A confirmation lists the scopes before queueing. Queue failures identify the
failed destinations and preserve successful responses. A network failure can be
ambiguous: refresh before retrying. Bulk saving desired sets also reports partial
progress if a later scope fails. These operations are not cross-machine atomic.

All machines expands the currently reported inventory scopes, including offline
machines. Users can deselect scopes. Each scope receives the same chosen set;
unsupported scan-only scopes cannot receive jobs. A new machine does not inherit
a saved selection automatically. To propagate later source changes, review and
save the desired selection again. Sync all previews saved desired state; it does
not silently replace it with the latest library.

## Agent maintenance next

Agent software management remains planned; Machines does not show functioning lifecycle buttons.
Implementation should add reported installed versions, supported platform/package
adapters, per-machine capability evidence, reviewed install/update/remove plans,
and verified results to the existing machine model. Agent software lifecycle,
agent configuration snapshots and skill distribution need distinct operations
and results. Copying binaries or credentials between machines is not implied by
skill sync. No lifecycle buttons claim functionality before those adapters exist.

Reet's product focus is a machine and terminal workspace, with controlled skill
distribution and reconciliation as supporting workflows. Capability
claims must distinguish current behaviour from planned work and locally tested
results from verified fleet deployment.

## Validation

Run `cd web && npm run check && npm run build`, then the Go vet/race checks
and MCP suite in CONTRIBUTING.md. For the browser workflow test, start the web
dev server at 127.0.0.1:4173 and run `node web/tests/control.e2e.mjs` from the
repository root. It uses synthetic API data to exercise `/control` going to Home, navigation, one-to-many
saves, incoming skill adoption from different machines, partial failures,
read-only controls and responsive light/dark layouts. It does not enroll real
machines or prove cross-machine transport. Real queue serialisation and exact
approval semantics are covered by the store tests.
