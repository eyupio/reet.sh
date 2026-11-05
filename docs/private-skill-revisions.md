# Private skill revision storage (S2)

This is the persistence and HTTP contract for private, account-owned skill files.
It does not introduce a machine transfer/apply operation or a new editing UI.
Existing hosted-skill, inventory, setup-profile and shelf formats stay readable.
An old inventory name/source is still not evidence that file bytes are known.

## Identity and ownership

A bundle has a random 32-character hexadecimal `id`, immutable skill `name`,
mutable `head` revision ID and creation timestamp in Unix milliseconds. Multiple
owned bundles may share a name. Names are not identities. Each new revision has
its own random ID, parent revision, creation time, origin and complete S1 manifest.
A manifest digest identifies exact file bytes/paths/executable flags; it is not a
revision ID, an account identity, a credential or permission to reuse content.

All four SQLite tables are private: `private_skill_bundles`,
`private_skill_revisions`, `private_skill_revision_files`, `private_skill_blobs`.
Blob keys and joins include the owner account. Identical bytes can be reused
within an account; another account stores its own copy. There is no digest lookup
endpoint or cross-account existence/reuse response. Foreign and unknown bundle
or revision IDs return the same 404. Account deletion cascades through the data.
Viewer and disabled accounts cannot read/write the new store, including after a
role change. Admins access their own private bundles; their role grants no access
to another account's contents.

`origin` contains `kind` (`user`, the default, `import`, or `fork`), optional
`reference` (up to 2,048 UTF-8 bytes, no controls), and optional `derivedFrom`
(an existing private revision owned by this account). A reference is supplied
provenance, not verified authorship or permission, and is never fetched/executed.
Public/upstream identity remains separate from the new owned copy. Uploading or
forking never changes public hosting rows, publication state or publisher ownership.

## Writes and retry behavior

`POST /api/v2/skill-bundles` creates a bundle and first revision.
`POST /api/v2/skill-bundles/{bundle}/revisions` appends a revision and moves its head.
Both accept the same strict JSON object:

```json
{
  "requestId": "request_0123456789",
  "expectedBase": "",
  "name": "demo",
  "origin": {"kind": "user"},
  "files": [
    {"path": "SKILL.md", "data": "LS0tCm5hbWU6IGRlbW8KZGVzY3JpcHRpb246IEV4YW1wbGUKLS0tCkluc3RydWN0aW9ucy4K", "executable": false}
  ]
}
```

`data` is JSON base64. Empty data denotes an empty regular file. There are no
symlink, directory, owner, publication or host-execution fields. File paths are
portable relative paths, subject to the S1 traversal, case/file-directory collision,
path/depth and entry limits. The server computes the canonical manifest itself.
Byte content and executable-bit presence are retained exactly, including CRLF;
frontmatter validation uses an isolated copy and never rewrites stored bytes.
SKILL.md must contain valid frontmatter matching `name`, a description and instructions.

`expectedBase` must be explicitly present: empty for creation, or the current
revision ID for an append. `requestId` is 16–80 ASCII letters/digits/underscores/
hyphens, unique across this account's writes. Use the same ID and semantic payload
after an uncertain response. An identical retry returns the original receipt,
even after later edits; its `bundle.head` describes that original revision. Read
the bundle separately for its current head. A changed payload under the same ID
or a stale base returns 409. A retry never moves the head backwards.

A successful 200 receipt is `{bundle, revision}` with the canonical manifest but
without raw file data. Bundle creation, immutable revision, blob references and
head movement happen in one transaction. Failed validation, quota, stale edits
or interrupted file writes leave no partial bundle/blob/revision/head changes.
The owner write lock serializes concurrent writers, including separate SQLite
connections. Only one new edit of the same base can win.

Bounds: 256 files, 8 MiB decoded file bytes per revision, SKILL.md at most 2 MiB,
1,024 file/directory entries, depth 32, path length 1,024 bytes; HTTP request at
most 16 MiB including base64/metadata. Per account: 128 bundles, 256 revisions,
64 MiB unique stored file bytes. The revision and byte bounds apply to what is in
use: see [Freeing superseded history](#freeing-superseded-history). The bundle bound
frees only the bundles of deleted written skills, because any other bundle is a skill
someone saved. Original retries remain valid at capacity while their revision still
exists.

## Freeing superseded history

Storage is not append-only. Editing a skill appends a revision and nothing else
removes the one before it, so an account that edits often would reach 256 revisions
(or 64 MiB) with a handful of skills and then be unable to save anything. When a
write would pass the revision or byte bound, the controller first frees history
nothing depends on, in the same transaction as the write, and checks the bound again.
If the account is still over it, the write is refused with 409 and nothing changes.

Two passes run, gentle then thorough. The first keeps each bundle's three newest
superseded revisions, so recent edits stay recoverable while there is room. The
second keeps only what is in use. Both then delete every blob that no remaining
revision refers to; before this, a blob outlived its revisions for good.

A revision is in use, and is never freed, when it is:

- a bundle's head;
- bound as a written skill's or a captured machine skill's sync source
  (`hosted_sync_sources`, `sync_private_sources`);
- pinned to a machine, or recorded in an install receipt (`skill_content_targets`,
  `skill_content_receipts`). Both are foreign keys that cascade, so freeing the
  revision would silently delete the pin or receipt too;
- named by a saved sync plan, or by the plan of a job that has not finished. The
  agent downloads exactly that revision by ID.

A test reads the schema and fails if a table gains a reference to a revision that
this list does not cover. An original retry whose revision was freed is a new write:
it succeeds if its base is still the head, and returns 409 as a stale base if not.

### Bundles of deleted written skills

Deleting or renaming a written skill drops its sync binding but leaves its bundle
behind (a bundle's name is immutable, so a rename starts a fresh one), and nothing
can delete a bundle. Each one kept a slot against the 128 for good, so an account
that renamed or deleted enough skills could never sync a new one. When a new bundle
would pass the bound, the controller frees bundles that only ever held a written
skill's synced bytes and that nothing uses any more. It frees the oldest first and no
more than the slots needed, so as much of a deleted skill's last bytes as possible
survives.

A bundle is left alone when it is bound to a written skill; when any of its
revisions is pinned, received, planned or in an unfinished job; when any revision's
origin reference is not `hosted:<name>`, which is what syncing a written skill
records, so a bundle someone saved directly is never freed; or when it is less than
ten minutes old, because a new bundle is bound in a statement of its own just after
its write commits and looks unused for a moment.

## Reads and authentication

| Method and path | Response |
| --- | --- |
| GET `/api/v2/skill-bundles` | `{bundles: [...]}`; metadata, bounded by account capacity |
| GET `/api/v2/skill-bundles/{bundle}` | Bundle metadata and current head |
| GET `/api/v2/skill-bundles/{bundle}/revisions` | `{revisions: [...]}`; IDs, parents, digests, provenance and times, without manifests/file data |
| GET `/api/v2/skill-bundles/{bundle}/revisions/{revision}` | Revision, manifest and exact base64 file contents |

A full read verifies stored bytes/modes against its manifest before returning them.
There is no PUT/PATCH revision route or public raw-file URL. Writes never run code,
resolve an origin reference, inspect a machine, send terminal input or publish.

These routes require accounts mode and an active write-role account. Browser
writes require a same-origin Origin header; personal API tokens use their owning
account and do not require a browser Origin. URL/query credentials are rejected.
MCP credentials remain confined to their existing endpoint allowlist; this stage
adds no MCP tool. Anonymous/single-token deployments cannot inherit the default
owner's private storage. Responses use JSON, no-store and nosniff. Payloads are
not included in activity events or request log bodies. Invalid requests return
400, missing/foreign resources 404, stale/quota conflicts 409; storage errors are
reported without SQL/payload details.

## Migration, backup and recovery

Migration adds the new tables transactionally and idempotently. It does not backfill
file contents from names, remote inventories or hosted records. Old clients continue
to use their existing routes; only clients explicitly calling these endpoints gain
the new contract. No private content is advertised as observed on a machine yet.

Full server database backups use SQLite VACUUM INTO and include these tables and
file bytes. Restore the complete database using the existing server-backup process.
The new tests reopen a full snapshot and verify bytes/modes/ownership.

**Personal shelf JSON export/import (including personal backup scope) does not
contain private bundle payloads.** Its existing version-2 format remains unchanged
and is insufficient to recover this new content. Use a full controller database
backup for private revisions. Adding a bounded account-content export/restore
format requires a separate compatibility decision before presenting private
content backup in the UI.

## Continuation map and checks

- `internal/skillbundle/contents.go`: pure upload manifest builder; uses the same
  canonical identity/path rules as filesystem scanning, without creating files.
- `internal/store/store_private_skills.go`: owner-scoped migration, immutable writes,
  receipts, quotas, current heads, content reads and integrity verification.
- `internal/api/private_skills.go`: bounded account API; keep MCP/public hosting/
  terminal admission independent.
- Tests cover S1 parity, binary/empty/CRLF/executable files, unsafe/ambiguous paths,
  ownership and guessed digests, retries after advancement, same-base concurrency,
  rollback after injected SQL failure, all capacity bounds, role changes/deletion,
  old-schema upgrade, separate publication and complete database restore.

After this PR merges green, take **S3 metadata observations first** from handoff.md.
Do not dispatch these private files to a machine until observation ownership,
explicit import, transport, reviewed conflicts and apply preconditions exist.
