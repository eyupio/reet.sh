# Explicit machine skill import — CLI transfer slice

An enrolled machine can explicitly copy one local skill into its account's private
revision store. Run this from an ordinary host shell or a reet browser terminal.
No background scan uploads files, and this command never installs, removes,
publishes or executes skill contents. It does not add a bookmark or change the
existing metadata-only shelf/adoption actions.

This first transfer slice is CLI-first. Browser preview/dispatch, private-content
library controls and remote import jobs are still future work. S3 remains open
until that central-control workflow is implemented. S4 file apply comes afterwards.

## Use

The controller and CLI both need this feature. The enrollment must belong to the
current local user/machine and use an active write-role account's personal token.
The controller must have a reported inventory scope for that enrollment; run the
usual agent scan if it has never reported the selected scope. Anonymous and
shared-token installations cannot inherit the default owner's private store.

```sh
# Preview only: no network request or upload.
reet agent skill import --agent agents --name demo

# Preview, check controller support, then type IMPORT to send the selected files.
reet agent skill import --agent agents --name demo --upload

# Exclude exact relative files; repeat --exclude as needed.
reet agent skill import --agent claude --name demo --exclude .env --upload

# Project roots must already be enrolled locally.
reet agent skill import --agent agents --project /absolute/enrolled/project --name demo --upload

# Append to an existing owned bundle at an explicitly reviewed revision.
reet agent skill import --agent agents --name demo --bundle BUNDLE_ID --expected-base REVISION_ID --upload
```

`--config PATH` selects an existing private enrollment file. `--yes` confirms the
printed preview for automation and requires `--upload`. No upload flag means local
preview only. Blank/other confirmation cancels before payload dispatch. Updating a
CLI binary alone does not change an already running connector/service executable.

Preview shows machine/scope, controller, destination, selected relative files,
byte counts, executable flags, explicit exclusions and the selected digest. It
prints no file contents, token or enrollment secret. The skill folder and valid
SKILL.md frontmatter must have the same name. SKILL.md cannot be excluded.
Private revisions currently have API access under `/api/v2/skill-bundles`; they
are separate from the existing published-skill editor and shelf UI.

## Files, limits and checks

`skillbundle.ReadContents` captures bytes through the same bounded, no-follow
reads and metadata/membership checks as S1 Scan. It verifies a supplied preview
digest and recomputes the captured contents' canonical digest. Incomplete, unsafe
or changed observations return no file payload. Full source capture must succeed
before exclusions; symlinks/special files cannot be hidden with exclusion flags.
Limits remain 256 files / 8 MiB, S1 path/depth/entry bounds and 2 MiB SKILL.md.

The CLI captures a complete source, selects exact existing portable relative file
paths, checks credential hazards and displays the selection. It re-captures the
whole source against that preview after confirmation, reapplies the exact same
selection and verifies the selected digest before sending bytes. A change to any
source file, including an excluded one, during review cancels this attempt. The
sent payload is a bounded in-memory copy; later filesystem edits do not change it.
CRLF, binary bytes and executable-bit presence are retained exactly.

The shared selection policy blocks common credential/configuration paths
(including .env variants, .ssh/.aws/.kube, credentials/auth/settings, key files),
private-key markers and known token patterns. The API repeats the same checks.
Errors show a relative path/reason, never matched secret values. There is no
force-upload bypass. Remove/redact locally or explicitly exclude a blocked
supporting file, then preview again. These conservative checks can flag examples
and are not a guarantee that every secret format is detected; review the files.
Missing/duplicate exclusion paths and exclusion of SKILL.md are rejected.

## Transport and ownership

GET `/api/v2/sync/skill-import` negotiates version 1 before any file bytes are
sent. HTTP 404/405, an unknown schema, failed account/enrollment authentication or
pause state prevents payload dispatch. HTTPS controller origins are required
except local loopback HTTP. Redirects are never followed with account/enrollment
headers. Neither new route expands the MCP endpoint allowlist.

POST the same path with strict JSON (16 MiB maximum): `schemaVersion`,
`requestId`, optional `bundleId`, explicit `expectedBase`, `agent`, `project`,
`name`, selected `digest`, and selected `files` using S2 base64 bytes/mode fields.
There are no caller-selected owner, origin, host, filesystem-root or publication
fields. Unknown fields/query parameters are rejected. Account and owned enrollment
headers are required. Browser writes retain the S2 same-origin requirement; this
endpoint does not provide browser clients with enrollment secrets.

The server recomputes the received files' manifest/digest and checks selection
policy and SKILL.md before writing. The private-store transaction rechecks active
write role, owned enrollment, pause state and the scope's membership on that
machine, then uses S2's expected-base CAS, capacity limits and immutable receipt.
An enrollment revocation/role change cannot race past that transaction. Failed
validation or SQL writes leave no partial bundle, revision, blob or head changes.
No controller filesystem paths are opened.

Origin kind is `import`, with a stable reference containing enrollment ID, agent,
project and folder name. This is connector-supplied source information recorded
by the endpoint; it is not cryptographic proof of machine contents or upstream
authorship. The general S2 API also permits user-supplied import provenance.
The verified claim is that stored bytes/modes match the reviewed selected digest.

## Retry and recovery

The CLI prints a random request ID **before** sending bytes. If a response is
lost, repeat the same arguments and unchanged selected files with
`--request-id PRINTED_ID`. A matching retry returns its original receipt, even if
the head has since advanced; it never rewinds the head or creates another copy.
A changed payload under the same ID conflicts. Keep the original base when
retrying an append. A fresh default invocation creates a new private copy, so do
not start one to resolve an uncertain earlier result. Read the private bundle or
revision API to inspect stored results.

Full controller DB backups include the imported bytes through existing S2 tables.
Personal shelf JSON/backups do not include them. There is no new deletion/GC or
private-content export UI in this slice.

## Continuation: central browser import

1. Preserve the CLI/capture/policy/store contracts and read S3 in `handoff.md`.
2. Negotiate a separate remote-import capability before dispatching new job types;
   never send new import fields/kinds to legacy strict terminal messages or old
   connectors. Keep periodic observation reports metadata-only.
3. Add an account-owned request for one enrolled machine, reported scope and skill.
   Bind request ID, generation, cancellation, expiry and selected file metadata;
   never accept arbitrary controller-provided filesystem roots on the connector.
4. Return a bounded preview of origin, paths, sizes, modes, exclusions, hazard
   reasons and completeness. Display a clear blocked state until hazards resolve.
   The browser must never receive enrollment secrets or unapproved file bytes.
5. Bind approval to the exact reviewed selection/digest/destination/base. Revalidate
   on the host before transfer. Enforce the approved request in the same transaction
   as `SaveMachineSkillImportFor` rather than adding a check-then-write race.
6. Show stable private bundle/revision receipts, lost-response retry, cancellation
   and cross-machine/account failures. Add central private-content library access
   without conflating it with public publication or metadata-only shelf adoption.
7. Test two machine fixtures end-to-end through browser review, exact bytes/modes,
   excluded secrets, stale/changed previews, old connectors, revocation/role change,
   interruption/retry and absence of public/shelf changes. Only then close S3 and
   start the reviewed S4 file planner; do not jump to bulk apply.

## Verification evidence

Local frontend check/build passed with zero Svelte diagnostics, along with the
full Go vet/race suite and all 16 MCP tests. Targeted race checks after integrating
main through `ec1b6b5` cover the CLI-to-real-HTTP-to-private-SQLite flow for two
enrolled synthetic machines, distinct same-name revisions, binary bytes/modes,
exclusions, unchanged shelf contents, private account isolation and identical retry
receipts. Other tests cover unsafe/changed captures returning no payload, common
credential hazards, cancellation, source scope restrictions, older controllers,
strict request bounds/fields/origins, bad digests, paused/revoked enrollments, stale
expected bases and transaction rollback. No live user machine or service was changed.
Published CI must pass for the final PR head before merge.
