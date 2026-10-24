# Reviewed browser imports from enrolled machines

Machines → **Private skill imports** opens account-owned import requests and private
revision history. A reported skill's **Import privately** button opens the same
flow with that scope/folder selected. This imports a private copy or appends to an
owned bundle; it does not publish, install, execute, remove machine files or change
metadata-only shelf adoption. CLI import remains available independently.

## User flow

1. Choose a reported machine/agent scope and skill folder. Choose a new private
   copy or an existing matching-name bundle; the current head is the expected base.
2. Request a file preview. An updated, online, unpaused connector must have recently
   negotiated browser import. Older or unsupported connectors remain usable for
   their existing features but cannot receive this work.
3. Review relative file names, byte counts, executable flags, file digests and the
   full source fingerprint. No file contents are returned to the browser. Known
   credential/configuration hazards are marked. Exclude supporting files explicitly;
   SKILL.md stays required. Invalid skill metadata or a selected hazard blocks import.
4. **Import N files privately** approves that exact preview/selection/destination/base.
   The connector captures the whole source again, checks the selected contents and
   checks current authorization before sending bytes. An edited file, expired
   preview, cancelled request or changed connector generation prevents a later commit.
5. A successful receipt identifies the immutable private bundle/revision and digest.
   Private copies expose their head and revision history. Shelf/public editor actions
   stay separate; full private editing/deletion/export UI remains later work.

The panel has a fixed header/action area with a scrolling body/file list, including
phone layouts. Polling is read-only and fenced by component lifetime/selection;
closing the panel leaves its request intact. Use **Cancel import** to revoke pending
approval. If completion wins the cancellation race, the saved receipt remains
visible. **Dismiss import history** removes only a finished request record, never
its saved private bundle or machine files. One active request is allowed per machine.

## Compatibility and transport

Import work uses its own version-1 REST channel, not the existing sync-job queue or
strict terminal messages. Old connectors never claim new job kinds. A connector
negotiates by POSTing `/api/v2/sync/skill-import/work` with `schemaVersion: 1` and a
random 48-hex per-process `instanceId`. No content is read for this negotiation.
A capability is recent for 90 seconds. Linux/macOS connectors advertise it; unsupported
platforms do not. Accountless/shared-token deployments do not inherit private storage.

A successful work response is `{schemaVersion:1, work:null|request}`. The server
returns only `previewing` or `uploading` work owned by that enrollment. Preview and
transfer retry the same request; completed work is never claimed again. A new
process instance invalidates outstanding requests bound to the old instance. The
connector skips an unavailable optional channel and continues legacy inventory/sync
work; handling a claimed import occupies that cycle. A running legacy sync job
prevents claiming import work. No arbitrary server-provided filesystem root is used:
`skillImportRoot` selects only the locally enrolled home/project agent roots.

All connector requests carry the existing personal account token and enrollment
headers. HTTPS is required except loopback HTTP; redirects are not followed with
credentials. Responses are bounded to 1 MiB. The browser never receives an enrollment
secret. The instance nonce fences generations; it is not an authentication credential.
No MCP endpoint allowlist is expanded.

## Browser API

These routes require an active accounts-mode write-role user. Browser writes retain
same-origin Origin validation; personal tokens use their own account. Query parameters
and unknown JSON fields are rejected. Responses are private/no-store/nosniff.

| Route | Purpose |
| --- | --- |
| GET `/api/v2/skill-imports` | Bounded request summaries and recent connector capabilities; no full file previews |
| GET `/api/v2/skill-imports/{id}` | One owned request, preview metadata and optional receipt; never file bytes |
| POST `/api/v2/skill-imports` | Create/retry `{requestKey,agentId,environmentId,name,bundleId,expectedBase}` |
| POST `/api/v2/skill-imports/{id}/approve` | `{previewHash,excluded}` binds approval to the immutable preview and exact excluded file paths |
| POST `/api/v2/skill-imports/{id}/cancel` | Revoke pending work; preserve a receipt if it already completed |
| DELETE `/api/v2/skill-imports/{id}` | Cancel active work or dismiss finished/expired history; never delete a private revision |

Creation binds the owned environment to the enrollment's current host, copies its
agent/project metadata, snapshots the recent process generation and fixes destination
and expected base. No caller-selected owner, source root, origin, raw data or publication
flags are accepted. A matching creation key returns the original request; a changed
payload under that key conflicts. Retained requests have a ten-minute deadline.
Expired/cancelled work cannot be approved or committed. The browser reuses its original
request key after uncertain creation and reconciles uncertain approval through GET.

## Connector reporting and atomic commit

- `/api/v2/sync/skill-import/preview`: strict metadata-only `{id,instanceId,preview}`.
  The preview includes schema, full source digest, SKILL.md validity and file metadata
  plus bounded hazard codes. `FromMetadata` validates portable paths, collisions,
  counts, sizes and digests using the S1 identity format. Server hashes canonical
  preview metadata; identical retries are acknowledged without replacing the review.
- Approval rejects selected hazards and derives the selected manifest digest from the
  reviewed file metadata. The destination/base were already fixed at creation.
- `/api/v2/sync/skill-import/transfer`: `{id,instanceId,sourceDigest,files}` with selected
  S2 base64 bytes/mode fields, at most 16 MiB JSON / 8 MiB decoded / 256 files. The
  connector uses `ReadContents` against the full source fingerprint, reapplies exact
  exclusions/policy, then rechecks work authorization before dispatch.
- The server recomputes actual selected byte/mode identity, repeats hazard/SKILL.md
  validation and enters the S2 private write transaction. It rechecks active write
  role, owned unpaused enrollment, reported scope, process generation, deadline,
  request state and approved digest/preview before creating any private rows.
  Expected-base CAS and account storage bounds remain in force.
- Request completion/receipt and immutable private bundle/revision/blob/head changes
  commit in the **same transaction**, including original-request retries. An injected
  receipt-write failure rolls back all private writes. Ambiguous upload responses
  are retried with the same controller-owned S2 key, not a new copy. An identical
  completed transfer returns its original receipt even after later head movement.
- `/api/v2/sync/skill-import/failure` accepts bounded codes only, never raw local errors
  or contents. Cancellation/expiry/revocation wins if recorded before the commit
  transaction's guard; a completed copy is not undone by a later cancellation.
  Bytes already in flight cannot be unsent, but an unauthorized later commit fails.

Limits: 128 retained requests per account; dismiss finished history when full. File
capture/path limits and conservative credential blocking are unchanged from CLI import.
Those checks are not a guarantee of detecting every secret format. Only excluded file
metadata, never their bytes, reaches the controller. Unsupported filesystem entries
cannot be hidden with exclusions. Existing private storage remains append-only, with
128 bundles/256 revisions/64 MiB unique bytes per account; broader capacity/deletion/GC
and content editing/export controls need their own reviewed work.

New tables are `machine_import_capabilities` and `machine_import_requests`, both
account/enrollment scoped with revoke/account-delete cascades. Full controller DB
backups include them and S2 payloads. Personal shelf backups do not include private
contents. Request-key and upload receipt recovery is available while the request is
retained; explicit history removal/revocation ends that request's retry window, while
saved private revisions remain account-owned and accessible through their own API.

## Implementation and next stage

- `skillbundle/metadata.go`: pure metadata identity/validation, not proof of host bytes.
- `skillimport/remote.go` and shared selection policy: immutable review hashes,
  explicit exclusions and hazard codes without matched values.
- `store_remote_import.go`: separate queue, capability/generation fences, owned state
  transitions and completion receipts; S2 helpers supply before/after transaction hooks.
- `api/remote_import.go`: bounded browser/connector routes, separate credentials/roles.
- `scan/remote_import.go`: negotiated polling, read-only capture, revalidation and safe
  transfer/retry. It never runs terminal input or skill instructions.
- `MachineSkillImports.svelte`: source/destination selection, review, exclusions,
  cancellation, private history and responsive fixed actions. `ContentObservation`
  and `MachineFleet` link into it. No raw file data is fetched for review/history.

After final checks and merge, take **S4 content-aware preview/planner** from handoff.md.
Use owned private revisions and fresh observations as inputs; do not treat matching
names/sources as file equality or reuse import approval to authorize remote apply.
Implement a pure bounded planner first, with explicit conflicts and no filesystem
writes, then a separate reviewed single-target apply/receipt stage. Preserve all
terminal controls, the approved sudo template repair and the deferred user-controlled
PIN/security preference note. Do not implement the whole roadmap in one PR.

## Validation recorded for this implementation

Local checks passed after integrating main through `144900b`: frontend check/build
with zero Svelte diagnostics, full `go vet ./...` and `go test -race ./...`, all
16 MCP tests, and six production browser fixtures (new import workflow plus
Control/public/terminal workspace/lifecycle/organisation). The final fixed-action
mobile review layout also passed its focused browser fixture and visual inspection.

Protocol/store tests use real HTTP/SQLite and two synthetic machine directories:
no private writes before approval, metadata-only preview, exact CRLF/binary/mode
preservation, explicit credential exclusions, changed source rejection, unsupported
controllers, cross-owner/machine/role/generation checks, expiry/cancellation, atomic
receipt rollback and original-result retries. The browser fixture uses synthetic
API responses and tests explicit review, missing capabilities, lost approval reply
reconciliation, completion winning cancellation, history retention and responsive
light/dark/viewer behaviour. These layers validate their shared contract; physical
fleet rollout and cross-platform import on live machines were not performed.
Required published workflows must pass for the final PR head before merge.
