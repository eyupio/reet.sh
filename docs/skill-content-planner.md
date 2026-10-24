# Pure skill-content planner (S4 first slice)

`internal/skillplan.Build(snapshot, nowMillis)` computes a bounded, deterministic
single-skill plan from metadata. It does not read files, access SQLite, call APIs,
transfer bytes, run commands or write to machines. The
[pinned desired-state API](skill-content-targets.md) stores its desired input, and the
[negotiated metadata preview](skill-content-preview.md) is the only product caller:
it runs Build on storage plus a connector capture, with the slot's
[ownership receipt](skill-content-receipts.md) as the baseline when it has one, and
stores the resulting plan as a read-only review. S4 remains incomplete until reviewed
execution and verified (`applied`) receipts are implemented and verified. S3 import
approval cannot authorise a machine write.

## Inputs and authority

Version 1 accepts the `posix-executable-v1` adapter and S1 manifests. Each desired
revision pins an owning account, private bundle ID, immutable revision ID and name.
The target binds account, connector, enrollment generation, opaque environment ID,
physical-root identity, skill slot and adapter/schema. Environment IDs can contain
slashes; they are not treated as paths or hex IDs. The future connector adapter
must derive RootID from its actual local root and detect aliases across scopes.
RootID must never become a controller-supplied destination path.

This library validates identity consistency, not the authenticity of those
identities. Future callers must authenticate account/enrollment, check active
writer permission, pause/revocation, selected scope and owned private revision,
and establish physical-root ownership before constructing a snapshot. No public
request handler should accept an arbitrary snapshot as proof of authority.

Observation presence is explicitly `present`, `absent` or `unknown`. A complete,
recent adapter observation must establish absence; a missing digest, failed scan,
old connector or omitted skill is unknown. Existing periodic S3 observations have
aggregate fingerprints, not a full per-file manifest or trustworthy absence proof;
review needs a separately negotiated, bounded metadata capture. Server receipt
time controls freshness (five minutes), never the host clock. Generation must
match the target. The current controller inventory revision must also match the
observation revision; it is separate from the stable baseline target binding.
Future-dated, stale and incomplete observations block planning.

An existing root requires a verified managed baseline bound to the same account,
connector/generation/environment/root/slot/adapter and private bundle. Baseline
revision identity is pinned too. A baseline is the last verified **managed subset**;
it is not inferred from a matching name, source URL or matching file bytes.
The only baselines stored today are controller-attested receipts created by explicit
adoption ([contract](skill-content-receipts.md)); the planner itself still knows nothing
about provenance and has no adoption path. With no baseline, an existing root is blocked
even if its contents match; a complete absent root permits a new installation. Missing
previously managed roots require review rather than automatic recreation.

A baseline that does not fit the snapshot is ordinary state and blocks the plan, with the
plan version unchanged:

| Condition | Result |
| --- | --- |
| The baseline's target differs (root moved or replaced, adapter or schema changed, another connector or generation) | Blocked `baseline_target_mismatch` |
| The baseline's bundle differs from the desired bundle | Blocked `baseline_bundle_mismatch` |
| The baseline belongs to another owner | Error: an integrity violation, not a mismatch |
| The baseline revision ID is invalid or its manifest is not canonical | Error |

These are checked after the adapter, root ownership, generation, inventory, unknown
observation and staleness blocks, and before the presence logic that yields
`managed_target_missing` and `adoption_required`.

The package also holds `ClaimSubset`, the pure computation adoption uses. It returns the
files of an observed manifest that equal the same path of a revision manifest in size,
digest and executable bit, with counts of what was not claimed, and refuses unless
`SKILL.md` is among them. `IsSubset` checks a stored managed manifest against its
revision. Neither reads anything, and neither decides ownership on its own.

Desired, baseline and observed present manifests must pass S1 schema, path,
case/directory collision, count, byte and canonical digest validation. The planner
also bounds metadata before cloning inputs. It includes executable-bit changes
in file identity. No fuzzy text comparison or normalisation is performed.

## File decisions

Both `additive` and `managed` modes preserve unmanaged files. `managed` means
replace/remove only known managed files; it is not a directory mirror operation.

| Desired/current/baseline situation | Result |
| --- | --- |
| Desired path absent on disk and absent from baseline | Create |
| Managed path unchanged locally, changed in desired | Update, including executable-bit changes |
| Managed path already equals desired | No write, retain ownership |
| Desired path exists locally but is not baseline-owned | Block as unmanaged, even for equal bytes |
| Managed path edited locally, or edited differently on both sides | Block for review |
| Managed desired path deleted locally | Block for review |
| Obsolete baseline-owned path unchanged locally, managed mode | Remove that exact file |
| Obsolete baseline-owned path unchanged locally, additive mode | Preserve and retain ownership |
| Obsolete baseline-owned path edited locally | Block in either mode |
| Path exists only locally | Preserve, do not claim ownership |
| Obsolete managed path already absent on both sides | No write, drop ownership |

Shared/unknown physical roots block the entire plan. Cross-tree collisions and
combined result limits block even when each input manifest is individually valid.
For example, installing a file `tools` cannot overwrite a preserved local file
`tools/notes`. A blocked plan exposes diagnostics and no executable action list or
predicted result manifests. No subset of its changes is implicitly approved.

A ready plan includes exact before/after metadata for each action, the predicted
complete installed manifest, and the predicted managed subset. The installed
manifest includes preserved local-only files and additive extras: it can differ
from the desired private revision. Do not label those roots byte-for-byte equal
to the private revision. Paths/actions are sorted for identity and review; this
is not an execution order. File-to-directory transitions need staging and explicit
execution dependencies in the future executor, never a naive action loop.

S1 manifests do not list empty directories, ACLs or ownership bits. A ready plan
cannot prove a destination is safe to create on disk. The executor must inspect
every prospective leaf and parent without following links, preserve unmanaged
directories, and reject changed types or permissions rather than forcibly clear
a path. The existing aggregate observations cannot supply these checks.

## Plan identity and limitations

`Hash` is SHA-256 of compact JSON for the complete versioned plan with an empty
hash field. It binds canonical manifests, owner/target/root/generation, desired
and baseline revisions, current/observed inventory revision, receipt time, mode, ownership, actions,
conflicts, preserved paths and predicted results. Reordered valid input manifests
produce the same hash. The same fresh snapshot at a different planning time has
the same hash; the timestamp in its preconditions still expires. Inputs and
outputs do not share mutable slices/pointers. Semantic/encoding contract changes
require a new planner version; never reinterpret an old approved hash with new rules.

The hash is neither a signature nor an approval token. A ready plan is a proposal,
not proof of actual bytes or permission to write. A future approval must bind this
hash and recheck time/identity/ownership; a future host executor must lock, capture
actual contents and revalidate all preconditions immediately before mutation.
A repeated hash never permits replaying a completed or ambiguously executed write.

## Granular continuation tasks

Keep subsequent work in reviewable slices. Do not wire this library directly to
legacy name-based sync jobs or terminal command messages.

1. Done: optional owned private revision pins ([desired-state slice](skill-content-targets.md)).
   Preserve reference-only targets and `SyncSkill` hash semantics. The pin version is
   carried in the preview's review identity; identical content selected after an
   intervening change does not revive an old review.
2. Done: the [negotiated per-file preview](skill-content-preview.md) authenticates
   account plus enrolment, binds the server-side request to the pin, process and
   inventory, and captures metadata from the connector's own configured root with S1
   no-follow reads. It negotiates before any content read, distinguishes unsupported,
   failed, absent and unknown, and carries no file bytes.
3. Partly done: physical-root identity and alias checks exist (shared and unknown
   roots block), and [ownership receipts with explicit adoption](skill-content-receipts.md)
   are stored separately from pins and observations. Root moves block with
   `baseline_target_mismatch` and re-enrolment needs a revoke that removes the receipts, so
   neither inherits an old baseline. Still to do: verified (`applied`) receipts. Initial
   installations record no ownership until post-apply verification succeeds. Do not
   bypass `adoption_required` because fingerprints match.
4. Done: owned private revisions, trusted observations and the slot's receipt feed Build,
   and the exact plan and hash are persisted as the review. Still to do: an approval that
   binds the review hash and rechecks time, identity and ownership rather than rebuilding
   from mutable desired state. A fence compares receipt versions only, so the executor
   must re-read the baseline through the store's validating lookup inside its own
   transaction.
5. In a separate executor slice, stage and hash the entire private payload, check
   selection policy, acquire the correct local lock and re-read preconditions.
   Journal/back up before replacements, including the current partially written
   action in recovery accounting. Never delete a root wholesale. Reject local
   edits or changed generations after review; require a new review.
6. Verify the actual installed tree and managed subset. Commit a durable receipt
   before acknowledgement. Persist per-action results, verified identities and
   incomplete/rollback failures. Reconcile a lost reply or crash from journal and
   files; never blindly rerun writes. Only verified receipts advance the baseline.
7. Use temporary filesystem fixtures for disk/write/verification failures, crashes
   around each journal/write/receipt boundary, safe retry, file/directory changes,
   preserved local files, executable bits and rollback failures. Only then close
   S4 and add S5 single-skill conflict review followed by fleet composition.

Preserve all terminal controls and live jobs. The deferred account security/PIN
preference design remains in handoff.md; it is not part of this planner.

## Verification

The package tests exercise explicit absence/new installs, content/mode updates,
managed/additive removals, local-only preservation, three-way conflicts, equal-byte
unmanaged rejection, partial-plan suppression, shared roots, stale/unknown/legacy
observations, identity isolation, invalid metadata, combined byte/file/path
collisions, deterministic hashes and caller mutation isolation. These are pure
metadata tests; they do not establish remote execution, on-disk recovery, physical
fleet behaviour or production rollout.

Local validation passed: frontend check/build (zero Svelte diagnostics), full Go
vet and race suites, all 16 MCP tests, and focused planner race tests with 96.6%
statement coverage. The full suite was rerun with installed tmux 3.5a on PATH
after an initial environment-only failure. No new browser fixture is needed for
this unconnected pure package; existing UI and protocol behaviour are unchanged.
Published PR workflows must pass for the final head before merge.

The receipt-baseline cases (the two block codes and the errors that stay errors) and the
claim computation are covered by the package's own tests; the ownership receipts contract
records how the rest was validated.
