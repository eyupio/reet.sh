# Pinned private skill targets (S4 desired-state slice)

Accounts can now record an exact private bundle revision for a reported machine
scope through `/api/v2/skill-content-targets`. These optional pins are separate
from legacy name/source desired sets. They do not install files, establish an
ownership baseline, dispatch connector work or authorise a future apply. Every
response is labelled `verification: "not_verified"`.

This slice supplies the persistent desired input for the
[pure content planner](skill-content-planner.md). Negotiated per-file observation,
root ownership, preview/approval UI and reviewed execution remain separate stages.
There is no new browser control in this slice.

## Ownership and scope

A pin belongs to the signed-in active account with a write role. The private
bundle **and** immutable revision must belong to that account. The selected
revision need not be the library head: later publications cannot silently update
machine intent, and an explicit version-checked rollback can select an older
revision without changing library history. Different owned bundles may share a
name; selecting another bundle for that same scope/name slot also needs the current
version. This changes intent only. Never retag an installed ownership receipt to
match a newly selected bundle: the planner must still reject that baseline mismatch
until a separately reviewed ownership transition exists.

The selected enrollment and environment must belong to the same account and
canonical host. A complete inventory within the previous fourteen days must
report the scope. Paused or revoked connectors cannot acquire/change pins.
An offline connector can have desired state if its last complete inventory is
recent enough; saving intent does not establish connector readiness.

Supported scopes follow existing writable root adapters: home `claude`, `agents`,
`cursor` and `opencode`, plus absolute, reported `claude`/`agents` project scopes.
Legacy `codex` stays scan-only. No request can supply a destination filesystem
path, slot name, installed baseline, digest or execution flag. The slot name comes
from the owned bundle. A reported project is a local adapter lookup key, never
permission for the controller to choose an arbitrary directory.

Pins remain inspectable/removable while a connector is paused/offline. Revoking
an enrollment or forgetting its environment cascades its pins; a replacement
connector does not inherit them. Neither operation deletes private revisions.
Machine renames within the same enrollment preserve pin IDs and selected revisions,
move their environment IDs and issue fresh versions so old reviews expire. Merging
identical duplicate pins retains the destination pin with a fresh version;
conflicting pins block and roll back the complete rename. Generic report merges
into another scope require removing the source pins first; they cannot silently
transfer private intent across roots. Ordinary process reconnects retain the same enrollment's intent; later previews
must independently fence their current connection/process and inventory identity.

## API

All endpoints use the existing private-skill account boundary: accounts mode only,
active writer, no query parameters, no caching, and same-origin browser writes or
an owning personal API token. Anonymous users, viewers and legacy auth modes cannot
access this feature. MCP credentials keep their existing endpoint allowlist.

| Method | Path | Result |
| --- | --- | --- |
| GET | `/api/v2/skill-content-targets` | Own `targets` array, sorted by scope/name/id |
| POST | `/api/v2/skill-content-targets` | Create/change one scope/name pin |
| GET | `/api/v2/skill-content-targets/{id}` | One owned pin or 404 |
| DELETE | `/api/v2/skill-content-targets/{id}` | Version-checked removal of intent |

POST requires exactly these fields (substitute real IDs):

```json
{
  "environmentId": "1/workstation/agents",
  "agentId": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "bundleId": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "revisionId": "cccccccccccccccccccccccccccccccc",
  "mode": "additive",
  "expectedVersion": ""
}
```

Use empty `expectedVersion` only to create a new scope/name pin. To change an
existing pin, send its current 48-character opaque version. Accepted modes are
`additive` and `managed`, with the planner's managed-file semantics; legacy
`mirror`/`bidirectional` strings are not aliases for content ownership.

Responses contain `id`, `environmentId`, `agentId`, `name`, `bundleId`, `revisionId`,
`manifestDigest`, `mode`, `version`, `updatedAt` and `verification`. They contain
no skill file bytes, per-file manifests or enrollment secrets. `manifestDigest`
is the private revision's desired identity, **not** observed/installed equality.
A meaningful change creates a new version. Saving the identical current intent
returns its unchanged version/timestamp, including after a lost response, and
cannot rewind a different newer selection.

DELETE requires `{"expectedVersion":"CURRENT_VERSION"}`. A stale version returns
409; a foreign/missing pin returns 404. Successful removal returns
`{"removed":true}` and removes only the pin. It never requests an uninstall,
removes a baseline, deletes private revisions or changes a legacy desired set.

POST/DELETE strictly reject unknown fields, extra JSON, missing/null versions and
bodies above 16 KiB. Invalid inputs return 400; unavailable scopes, version conflicts
and the 1024-pin/account limit return 409. Foreign private revisions return 404.

### Uncertain responses

These desired-state writes are not durable execution receipts. After an uncertain
POST, read the collection and compare environment/name, selected revision and mode.
After an uncertain DELETE, GET the pin: 404 means it is no longer present. Do not
blindly replay a create after another user action has removed its pin: an empty
expected version intentionally permits a new creation. An old nonempty update
version cannot recreate a removed pin. Future execution needs separate request IDs,
expiry, approvals, journals and durable original-result receipts; it must never
reuse pin upserts as permission to replay a host operation.

## Storage and compatibility

`skill_content_targets` is an optional table, migrated after private revisions and
enrollments. It has one row per account/environment/bundle-name slot, owner-bound
foreign keys to enrollment and exact revision, an environment cascade and an
independent random version. Private revision history remains immutable. Account
checks, scope checks, version comparison and writes share one transaction; the
initial account write lock serialises competing edits, including across Store
connections. Unchanged saves do not invalidate review state through version churn.

No fields or hash meanings in `SyncSkill`, `SyncTarget`, `sync_targets`,
`sync_options` or legacy jobs are changed. Old connectors receive no new job and
reference-only desired targets retain their existing content-verification limits.
Full SQLite backups include pins; personal shelf exports do not export private
content or pins. There is no new export/restore format in this slice.

## Granular continuation

1. The user requested a pause after this PR. Only when they explicitly resume,
   implement the negotiated metadata preview
   channel. Read the pure planner's input/authority contract before adding types.
2. Select the owned pin and exact immutable private revision server-side. Bind the
   pin ID/version, account, enrollment, current process/connection generation and
   inventory revision into a separately expiring preview request. Never accept a
   browser-supplied planner snapshot or treat this pin as an installed receipt.
3. Negotiate support before hashing files. Map only the selected reported scope to
   the connector's locally configured home/project root. Reuse bounded S1 reads;
   send per-file metadata only, with explicit present/absent/unknown states.
   A failed or omitted scan is not proof of absence. Keep old connectors unchanged.
4. Establish physical-root identity and aliases across scopes. Shared/unknown roots
   must block; an existing unowned tree must still require explicit adoption.
   Store future verified managed receipts separately from pins and observations.
5. Construct the planner input from trusted storage and observation data. Reject
   changed pin versions, scan generations, connection instances, permissions,
   pause/revocation, stale captures and expired requests. Bind the pin version in
   the surrounding preview/approval identity as well as the planner hash; a pin can
   change away and back to the same revision (ABA), which must invalidate old review.
6. Add API/protocol/store tests for those fences, old servers/connectors, cancellations,
   changed source and lost responses. Connect a review UI only to truthful metadata
   states. There are still no filesystem writes at this stage.
7. Add reviewed execution as a subsequent slice: full payload staging, local locks,
   immediate precondition revalidation, recoverable journal/backups, installed
   verification and durable receipts. Do not close S4 or begin S5 fleet apply until
   temporary-filesystem failure/crash/recovery gates in handoff.md pass.

Preserve terminal sessions and controls, the approved sudo template behaviour and
the deferred user-controlled security/PIN preference note in handoff.md.

## Verification boundaries

Real SQLite/store and HTTP tests cover pinned history and explicit rollback,
metadata-only responses, account/token/origin/role isolation, stale version edits
and deletes, unsupported/stale/incomplete scopes, pause/revocation, account/scope
cascades, replacement enrollments, transaction rollback, competing connections,
capacity, migration/reopen, rename preservation/conflict rollback, cross-scope
merge fences and unchanged legacy dispatch/state. These tests do not
establish remote file installation, ownership adoption, on-disk recovery or a live
fleet rollout. No production machine or terminal service was changed.

Full local validation passed: frontend check/build (zero Svelte diagnostics),
`go vet ./...`, `go test -race ./...` with tmux 3.5a on PATH, and all 16 MCP tests.
The published workflows must pass for the final PR head before merge.
