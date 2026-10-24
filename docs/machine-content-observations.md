# Machine skill-content observations (S3 metadata slice)

This extends the existing name/source inventory with read-only fingerprints. It
never uploads skill file bytes, creates private revisions, installs or removes
files, launches software, or changes the existing sync planner's identity rules.
S3 payload import is still a separate, unfinished stage.

## Connector flow and compatibility

1. An enrolled connector completes all configured legacy inventory scans first.
   The `/api/import` request shape remains unchanged. An unsuccessful scan does
   not submit a content report.
2. It POSTs `{}` to `/api/v2/sync/content-observations/prepare`, using the existing
   account token and `X-Reet-Agent`, `X-Reet-Agent-Key`, `X-Reet-Version` headers.
   HTTP 404/405 means unsupported: skip without reading skill content. A future
   schema version is also skipped before hashing. Other errors remain errors.
3. A version-1 grant contains `schemaVersion`, `scanId` (48 lowercase hex digits)
   and `expiresAt` (server milliseconds, ten-minute lifetime). A new grant replaces
   the previous one for that enrollment. It captures the machine's current
   inventory generations, rather than accepting caller-selected host identities.
4. Only then hash configured local home/project roots via S1 `skillbundle.Scan`.
   Known Claude, shared agents, Cursor, Codex and OpenCode home scopes and supported
   project scopes are reported. Controller responses cannot supply filesystem paths.
   Symlinks/special files/unsafe or changing files cannot produce comparable digests.
5. POST `/api/v2/sync/content-observations` with `schemaVersion`, `scanId`,
   `observedAt`, and `scopes`. Each scope has `agent`, `project`, `complete`,
   `skills`. Each skill has folder `name`, S1 schema/algorithm/mode semantics,
   `complete`, optional `digest`, file/byte counts and optional issue **codes**.
   No file names, content bytes or issue paths are sent. Project labels remain
   metadata, as in the existing inventory; the controller never follows them.
6. Server rechecks active write-role account, owned enrollment, pause state, grant
   and current inventory generation in one write transaction. Unknown scopes,
   expired/superseded grants and inventory changes conflict. An identical report
   retry returns success without refreshing timestamps or rewriting observations;
   altered payload with the same grant conflicts. Grant replacement ends that
   receipt's retry window. Loss of the response needs no filesystem rollback.

The connector rejects redirects for both new endpoints, avoiding forwarding
custom enrollment credentials. JSON bodies are strictly bounded to 1 MiB and
unknown fields are rejected. No MCP endpoint allowlist changes are needed.

## Bounds and uncertainty

Per cycle: 64 scopes, 512 skill records, 64 MiB of file reads. Each skill retains
S1's 256 files / 8 MiB / depth and entry limits. Failed scans consume their full
reserved byte allowance plus the bounded overflow probe; only complete scans
refund unused allowance. Exhausted budget produces incomplete records. Additional
scopes omitted at the scope bound remain unknown. Cancellation prevents dispatch.
The collector checks cancellation between bounded skill scans.

Reports are observations of a changing filesystem, not atomic snapshots or proof
of equality. Completeness means the reported observation completed, not permission
to infer a deletion or authorize later apply. A future import/apply must freshly
revalidate its own preview, bytes and ownership.

`environment_scans.inventory_revision` is an opaque random positive integer
refreshed by **every** legacy inventory report, including from older connectors.
It avoids timestamp collisions and stale observation revival when an environment
is forgotten and recreated. Migration gives existing records zero without claiming
any content observation. Observation grants/scopes are account/enrollment-owned
with cascade deletion; revoking a machine removes its observations. The latest
accepted report atomically replaces the enrollment's previous scope snapshot;
omitted scopes become unknown rather than an inferred empty bundle.

`SyncEnvironment.content` is separate from `SyncSkill`, desired sets, baselines,
and their revision hashes. It contains:

- `not_reported`: no observation for the current inventory generation; empty skills.
- `observed`: complete report and complete inventory.
- `incomplete`: incomplete scope/skill traversal or legacy inventory.
- `stale`: server receipt older than five minutes.
- `paused`: enrollment paused (takes precedence over age/completeness).

The client-provided observation time is informational. Server receipt time controls
freshness. Neither old/missing fields nor stale/incomplete states imply equality.
Machines and Sync show these labels with optional digest details. Viewing them
never requests a scan/import/apply. Public pages receive no new content calls.

## Implementation and continuation

- `internal/contentobs`: bounded shared metadata contract and validation.
- `internal/scan/content_observations.go`: capability negotiation, local collection,
  bounded metadata transport; called after successful enrolled-agent scans.
- `internal/store/store_content_observations.go`: additive migrations, owned grants,
  transactional replacement/receipts, freshness reads.
- `internal/api/content_observations.go`: strict enrolled-writer endpoints.
- `web/src/lib/ContentObservation.svelte`: honest read-only status/details, used in
  machine scopes and sync destination scopes.

After this metadata slice is merged, implement **explicit S3 import**, not S4:
read `handoff.md`, this contract, `docs/private-skill-revisions.md` and S1 scanner
constraints. Design a typed, account-authorized job for one enrolled machine and
selected skill root. Preview origin, selected files, sizes, exclusions and content
safety. Require fresh complete revalidation before transfer; bound/cancel/retry it;
recompute exact bytes/modes/digest server-side; write one immutable private revision
only after verified transfer. Do not reuse periodic observations as import approval
or add payload fields to legacy strict terminal messages. Keep the existing
metadata-only “Add selected to reet.sh” action distinct.
