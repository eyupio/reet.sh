# Reet as the central control unit

The main user task is to keep skills and remote work consistent across machines.
The signed-in home is Control (`/` and `/control`). Skills moved to `/skills`;
Machines stays at `/machines`; `/sync` and `/terminals` are first-class destinations.
Existing machine comparison, terminal and account machine links continue to work.
Lists and portable setups are under More.

## Everyday workflow

1. Connect a machine from Control or Machines. Install/authenticate the CLI,
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

Scan all queues one scan per recently reporting, unpaused enrollment. Sync all
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

Control reserves a clearly labelled Planned area for agent software management.
Implementation should add reported installed versions, supported platform/package
adapters, per-machine capability evidence, reviewed install/update/remove plans,
and verified results to the existing machine model. Agent software lifecycle,
agent configuration snapshots and skill distribution need distinct operations
and results. Copying binaries or credentials between machines is not implied by
skill sync. No lifecycle buttons claim functionality before those adapters exist.

Reet's product focus is a maintained central skill library, controlled distribution
and reconciliation, and explicit execution across connected machines. Capability
claims must distinguish current behaviour from planned work and locally tested
results from verified fleet deployment.

## Validation

Run `cd web && npm run check && npm run build`, then the Go vet/race checks
and MCP suite in CONTRIBUTING.md. For the browser workflow test, start the web
dev server at 127.0.0.1:4173 and run `node web/tests/control.e2e.mjs` from the
repository root. It uses synthetic API data to exercise navigation, one-to-many
saves, incoming skill adoption from different machines, partial failures,
read-only controls and responsive light/dark layouts. It does not enroll real
machines or prove cross-machine transport. Real queue serialisation and exact
approval semantics are covered by the store tests.
