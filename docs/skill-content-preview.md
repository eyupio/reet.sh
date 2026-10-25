# Negotiated metadata-only content previews (S4 preview slice)

A proper look at a machine before anything gets touched. An account can ask one
enrolled connector what is on disk for one pinned skill, and gets back per-file
metadata plus the [pure planner's](skill-content-planner.md) verdict. Nothing is
installed, changed or removed, no file bytes travel, and the result is not permission
to write. A preview never adopts anything: adoption is a separate, explicit decision
bound to a preview's review hash, set out in the
[ownership receipts contract](skill-content-receipts.md). This slice added no browser
control; the API below is the contract the [Private skills dialog](skill-content-review-ui.md)
now uses (it asks for a check, reads it, and says what it means) and the later executor will
build on.

## What a preview is and is not

| It is | It is not |
| --- | --- |
| A bounded, no-follow, read-only observation of one skill folder | An atomic filesystem snapshot or proof of equality |
| Per-file path, size, executable bit and digest, with explicit present / absent / unknown | File contents, local paths or a way to choose a directory |
| The planner run on trusted storage, that observation and the slot's receipt if it has one | An ownership receipt, adoption, approval or apply authority |
| A review identity that binds the pin **version** | Reuse of an import approval, or a durable execution receipt |

A matching name, source URL or even byte-identical files never establish ownership.
A slot with no [ownership receipt](skill-content-receipts.md) has no baseline, so any
existing tree is reported as `adoption_required`. A receipt, which only an explicit
adoption creates, is the baseline otherwise.

## Flow

1. The connector asks for work (`/work`). That request **is** the negotiation: it
   reads no files, records the connector's process generation and says whether it can
   serve previews. Older controllers (404/405), controllers that forbid the channel
   (403), a paused or not-ready connector (409, treated as a quiet "not now") and newer
   schemas leave every file unread.
2. The browser requests a preview naming an **owned pin ID and the exact pin version
   it reviewed**. The server re-reads the pin; the revision, bundle, skill name, mode
   and scope all come from storage. A stale version conflicts. The request is accepted
   only when that connector negotiated within 90 seconds and is not paused, and only
   one request per machine is active at a time.
3. On its next poll the connector claims the request. The **inventory revision is
   bound at this moment**, when capture begins (see below). Work is retired rather
   than handed out if any fence has failed.
4. The connector maps the reported scope onto its **own configured** home or enrolled
   project root, captures that one skill folder and reports (`/report`). Failures use a
   short allow-listed code (`/failure`), never raw local errors.
5. The server rechecks every fence inside the writing transaction, builds the planner
   snapshot from storage, the validated capture and the slot's ownership receipt (the
   baseline, when one exists), and stores the plan, the receipt version it saw and a
   review hash.

The whole claim-capture-report sequence runs inside one connector cycle, ahead of the
legacy inventory scan, so a scan cannot slip in between. Import work goes first; a
running legacy sync job also holds preview work back.

## What a request is bound to, and what invalidates it

| Binding | Set | A change makes the request |
| --- | --- | --- |
| Account, active writer role | every read and write | hidden on read, refused on write |
| Pin ID **and independent pin version** | creation | `superseded`: `pin_changed` / `pin_removed` |
| Enrolment (connector) and its pause state | creation, every write | `superseded`: `connector_paused` / `connector_removed` |
| Connector process generation (random per-process nonce) | creation | `superseded`: `process_changed` |
| Version of the slot's ownership receipt (empty when none) | report | `superseded`: `receipt_changed` |
| Inventory revision of the reported scope | claim | `superseded`: `inventory_changed` / `scope_removed` |
| Request lifetime (ten minutes before capture) | creation | `expired` |
| Capture age (five minutes, the planner's window) | report | `stale` (still readable, never shown as current) |

Pin versions are fresh random values on every meaningful change. A pin that moves to
another revision and **back to identical content** has a different version, so a review
made before the round trip is `superseded`. `reviewHash` binds the owner, request,
pin ID/version, bundle, revision, manifest digest, mode, connector, process, inventory
revision, capture hash, plan hash and receipt version. A future approval must bind this
value, not the plan hash alone. Machine renames, enrolment revocation and forgetting a
scope already bump or remove the pin, so they invalidate previews through the same fence.

Receipt versions are random too: a receipt that is released and adopted again with
identical content supersedes older reviews, as a pin round trip does. A captured preview
is `superseded` with `receipt_changed` when the slot's receipt is created, released or
replaced, and adopting consumes the preview it used. The fences are live reads, with
consequences the [receipts contract](skill-content-receipts.md#preview-binding) sets
out.

Why inventory binds at claim, not creation: every legacy inventory report draws a new
random revision, and one can complete between a browser click and the connector's next
five-second poll. Binding at claim fences exactly the window in which files are read.

Reads report fences as they stand now; stored state is only advanced when the
connector next claims, so a read never writes.

## States

Stored: `queued`, `capturing`, `captured`, `failed`, `cancelled`, `expired`,
`superseded`. Reads may also report `stale`. A superseded or stale preview keeps its
capture and plan for inspection but carries `reasons` and must not be acted on.
Every response says `"authority":"preview_only"`.

### Presence

| Presence | Meaning | Never means |
| --- | --- | --- |
| `present` | A complete traversal produced a manifest listing **every** file, managed or not | That reet owns any of them |
| `absent` | The scope root is a verified real directory that holds no entry with this name | Anything about a missing scope root |
| `unknown` | Any failed, partial, refused or unsupported capture; `issues` say why | Absence, emptiness or health |

An unsafe entry anywhere in the tree (a symlink, special file, collision, limit,
changing file) makes the whole skill `unknown`: a partial manifest is never offered.
A skill folder that is itself a symlink is `unknown` (`skill_symlink`), even when its
target is a valid skill. A scope root that does not exist yet is `unknown`
(`scope_root_missing`); creating roots needs its own reviewed step.

Issue codes seen from the connector include `invalid_skill_name`, `scope_root_missing`,
`scope_root_unreadable`, `scope_root_not_directory`, `scope_root_symlink`,
`scope_root_changed`, `root_aliased`, `alias_check_incomplete`, `skill_symlink`,
`skill_not_directory`, `skill_unavailable`, `capture_failed`, `platform_unsupported`
and S1's own traversal codes. Paths in issues are relative to the skill folder.

## Physical root identity and ownership

The connector derives `rootId` from the scope root's device and inode (hashed, so it
exposes no path and changes if the directory is replaced or moved). It then compares
that directory with **every other configured scope root**: the other home scopes
(including scan-only Codex) and each enrolled project's `.claude` / `.agents` roots.

| Ownership | When | Effect |
| --- | --- | --- |
| `exclusive` | A real directory, and no other configured scope resolves to it | Planner may continue |
| `shared` | The root is a symlink, or another configured scope is the same directory | Whole plan blocks (`root_shared`); nothing inside is read |
| `unknown` | Identity or the alias set could not be established | Whole plan blocks (`root_unknown`); nothing inside is read |

"Exclusive" means exclusive **among the connector's configured scopes**. It cannot see
other users, other tools or bind mounts outside that list. Treat it as necessary, not
sufficient, for a future executor, which must still lock and revalidate.
Re-enrolment gives a new connector ID, so an old root identity or baseline cannot be
inherited silently. More than 256 enrolled projects cannot be alias-checked and report
`unknown`.

## Connector channel

All connector requests carry the existing personal account token and enrolment
headers; HTTPS is required except loopback HTTP; redirects are never followed with
credentials; responses are bounded to 1 MiB. The process nonce is a fence, not a
credential. The channel version is 1 and is independent of observation and import.

| Route (POST) | Body | Result |
| --- | --- | --- |
| `/api/v2/sync/skill-content-preview/work` | `{schemaVersion:1, instanceId}` | `{schemaVersion:1, work:null\|{id,instanceId,agent,project,name,expiresAt}}` |
| `/api/v2/sync/skill-content-preview/report` | `{id, instanceId, capture}` (≤ 1 MiB) | `{recorded:true}`; identical retries are acknowledged |
| `/api/v2/sync/skill-content-preview/failure` | `{id, instanceId, code}` | `{recorded:true}`; codes `capture_failed`, `scope_unavailable` |

The work item names a scope and a skill folder, nothing else: no bundle, revision, pin
or mode. The connector refuses scan-only Codex, names that are not a lowercase slug, and
projects it has not enrolled locally (`scope_unavailable`) without reading anything. A
retried report with a **different** capture conflicts; the reviewed capture is never
replaced.

## Browser API

Same boundary as the pin API: accounts mode only, active writer, no query parameters,
no caching, same-origin browser writes or an owning personal token, strict JSON (unknown
fields and trailing data rejected), no MCP allowlist change.

| Method | Path | Result |
| --- | --- | --- |
| GET | `/api/v2/skill-content-previews` | `{previews, capabilities}`: bounded summaries (no capture or plan) |
| POST | `/api/v2/skill-content-previews` | Create or retry `{requestKey, targetId, expectedTargetVersion}` (16 KiB) |
| GET | `/api/v2/skill-content-previews/{id}` | One preview with capture, plan and review hash, plus an `adoption` offer when adoptable |
| POST | `/api/v2/skill-content-previews/{id}/cancel` | `{}`; revokes outstanding work, leaves a recorded capture |
| DELETE | `/api/v2/skill-content-previews/{id}` | Cancels live work, or dismisses finished history |

`requestKey` is 16–80 letters, digits, `_` or `-`. A repeated key returns the original
request (even if its pin has since moved, in which case it reads `superseded`); a
changed payload under that key conflicts. `inventoryRevision` is a 63-bit random value
and travels as a **string**. Errors: 400 invalid input, 404 foreign or missing, 409
stale pin version, unavailable scope, no recent negotiation, paused connector, busy
machine or full history.

A single-preview read of a `captured`, present, exclusive preview whose only conflict is
`adoption_required` also carries an informational `adoption` block: the revisions of the
pin's bundle that could prove ownership, best match first. It is computed on read, never
stored, never listed, and absent from a `POST` replay. `adoption_required` therefore now
has a resolution, the explicit adoption in the
[receipts contract](skill-content-receipts.md#the-adoption-offer).

Dismissing history never touches the pin, the private revision, an ownership receipt or
any machine file.

## Storage and limits

New tables `skill_content_preview_capabilities` and `skill_content_previews` are
account- and enrolment-scoped and cascade when an enrolment is revoked. The pin
reference is by ID and version rather than a foreign key, so a removed pin leaves a
readable `superseded` record. Full SQLite backups include both; personal shelf exports
do not. At most 64 retained previews per account (dismiss finished ones) and one active
request per machine. Scopes must be ones the pin API accepts: complete, recently
reported, on the connector's own host, home `claude`/`agents`/`cursor`/`opencode` or an
absolute `claude`/`agents` project. Per skill: S1's 256 files, 8 MiB, depth and entry
limits, plus at most 32 issues.

## Compatibility

Old connectors never negotiate and never read content; new connectors facing old
controllers skip the channel, as above. No field or hash meaning in `SyncSkill`,
`SyncTarget`, legacy desired sets, observations, imports or terminal messages changed.
The legacy poll never receives preview work, and a preview creates no job. The ownership
receipts slice added the optional `receiptVersion` and the item-read `adoption` block;
previews captured before it carry no receipt version and read as before.

## What this does not establish

No filesystem write, staging, locking, journal, rollback or verified (`applied`) receipt
exists, and none was exercised. The only baselines are controller-attested adoption
receipts, so a folder with none still needs adoption, and a plan against an adopted
folder is a proposal, never permission to write. Ownership beyond configured scopes,
non-POSIX platforms, ACLs, empty directories and parent-directory safety are not
modelled (the planner already says an executor must inspect every leaf and parent).
Capture is a point-in-time read of a changing disk. Tests use synthetic homes, SQLite
and HTTP; no live machine, production service or terminal session was touched, and no
physical cross-platform or fleet check was made.

## Granular continuation

1. **Done in a following slice:** the review UI. The [Private skills
   dialog](skill-content-review-ui.md) has pin selection and the check together, with
   truthful present / absent / unknown labels, the reasons list, `stale` and `superseded`
   handling, and keyboard and phone layouts. It surfaces `adoption_required` and the
   `adoption` offer as a decision to be made, never as a one-click default.
2. **Done in a following slice:** ownership receipts and explicit adoption, stored in
   their own table bound to account, connector, root identity, slot, adapter and
   revision. Adopted receipts are controller-attested. Initial installs still record
   nothing until post-apply verification passes. See
   [the receipts contract](skill-content-receipts.md).
3. **Reviewed executor.** Full payload staging, local lock, immediate precondition
   revalidation against this review hash, journal and backup before replacement,
   installed verification and durable receipts, with temporary-filesystem
   failure/crash/recovery gates. Do not close S4 or start S5 fleet apply before they pass.
   It must also re-read the slot's baseline through the store's validating lookup in its
   own transaction, since a review's fences compare receipt versions only.

Preserve terminal sessions and controls, the approved sudo template behaviour and the
deferred user-controlled security/PIN preference noted in `handoff.md`.

## Validation recorded for this implementation

Local checks on the final code passed in the required order: frontend check (228 files,
zero errors or warnings) and build, `go vet ./...`, `go test -race -count=1 ./...` (31
packages, no failures, tmux 3.4 on PATH), all 16 MCP tests and `git diff --check`.
Tests were written first and each fence was then broken on purpose to confirm they fail:
capture aliasing and truncation, missing roots, pin version, process, inventory, removal,
pause, staleness, root ownership, the store's revision identity check, strict JSON,
connector identity and the agent-cycle wiring. They use real SQLite, a real HTTP
controller and synthetic temporary homes, and show the machine's files unchanged byte
for byte. A macOS build and vet pass; the whole-binary Windows build already fails on
`main` for unrelated reasons. The published workflows must pass for the final pull
request head before merge; the pull request is the record of that.

The ownership receipts slice that followed records its own checks in
[its contract](skill-content-receipts.md#validation-recorded-for-this-implementation).
