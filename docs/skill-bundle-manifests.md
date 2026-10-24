# Read-only skill bundle manifests (S1)

`internal/skillbundle` provides local filesystem observation and pure file-level
comparison. It does not change scans, live sync, snapshots, library membership or
installed files. It adds no CLI command, HTTP endpoint, database record or upload.
A manifest is not evidence of provenance, ownership, permission to publish, or a
fully valid skill: instruction frontmatter is not parsed and scripts are not run.

## API and outcomes

- `Scan(root, Limits{}) (Manifest, error)` observes one explicitly selected local
  skill folder. It requires a regular `SKILL.md` file, which may contain zero bytes.
  The caller remains responsible for choosing an authorised local directory.
- A successful manifest has `Complete=true`, sorted `Files`, `TotalBytes` and a
  content-tree `Digest`. The file count is `len(Files)`; each file includes its
  relative path, byte length, content digest and executable boolean.
- `ErrIncomplete` means some part could not be safely observed. Relative-path issue
  codes explain why. Partial file observations can remain, but the tree has no
  digest and is not comparable. Raw contents, absolute roots and OS error strings
  are not returned.
- `ErrLimits` rejects unsupported limits before accessing the directory. Zero
  fields use defaults; positive fields can tighten the defaults only.
- `Compare(previous, current)` returns `equal`, `changed` or `unknown`. Diffs are
  directional and sorted by path: added, removed or modified. Rename is remove
  plus add. Missing, incomplete, incompatible or malformed manifests return unknown
  with no executable change plan. Comparison does not perform any writes.

The scanner supports executable-mode observation on Linux and macOS; other build
platforms return an explicit incomplete `platform_unsupported` observation. macOS
is cross-compiled in this stage; native filesystem behaviour still needs macOS CI.

## Version 1 identity

Per-file digests are lowercase SHA-256 of exact file bytes. No YAML, text, Unicode
or line-ending normalization occurs. `Executable` means any POSIX execute bit was
present; owner/group, read/write permissions, timestamps and privileged mode bits
are not portable content identity. A future restore must implement its own safe
permission policy; it must not recreate privileged permissions from observations.

Tree identity hashes compact Go `encoding/json` output for this fixed field-order
structure, using the field order below for every file as well:

```json
{"schemaVersion":1,"algorithm":"sha256","modeSemantics":"posix-executable-v1","files":[{"path":"SKILL.md","bytes":1,"digest":"…lowercase SHA-256…","executable":false}]}
```

Files sort by their exact UTF-8 relative paths. Standard `encoding/json` string
escaping applies, including HTML escaping and Unicode line-separator escaping.
Totals, scan diagnostics and host identity are not part of this hash. Empty
directories are traversed/bounded but are not content entries. A golden fixture
pins the canonical digest for a bundle containing only the byte `x` in `SKILL.md`.
Changing this contract requires a schema/algorithm version change.

Existing encrypted setup snapshots use their existing BLAKE2b-based digest over
paths, modes and contents. S1 intentionally does not change those formats or
reinterpret their digests. A new small local package is used because snapshot
collection retains payload bytes for encryption, whereas S1 streams hashes through
root-confined reads and returns metadata only. Shared scanner root semantics are
unchanged; neither collector is silently replaced by the other.

## Bounds and path handling

Defaults/hard ceilings are 256 regular files; 8 MiB per file and total bytes read;
1024 directory entries including empty directories; 32 directory levels; 1024
bytes per relative path, with a 255-byte component limit. A bounded read may consume
one extra byte to detect growth beyond a limit. Failed/changed reads consume the
aggregate byte budget too. Limits can be reduced for a stricter receiver.

The hosted-skill format separately allows 100 supporting files and 2 MiB total;
these manifests do not widen that contract. Transfers will need to negotiate and
respect source/receiver limits rather than truncate data.

Root symlinks, in-tree symlinks, special files, traversal, ambiguous case-folded
paths, control/format/line-separator characters and reserved or nonportable names
are rejected. Files and directories share the case-collision checks. Exact UTF-8
path bytes are preserved; Unicode normalization and cross-platform file creation
are not claimed to be verified by a local manifest.

`os.Root` confines traversal. Child directory/file identities and metadata are
checked; regular files use no-follow/nonblocking opens so a replacement FIFO
cannot stall the regular-file check. Directory membership is rechecked and earlier
observations are checked again after traversal. These checks detect ordinary
concurrent changes, but cannot create an atomic point-in-time snapshot of an
uncooperative writer. A future apply needs its own locks, preconditions and final
verification. A JSON manifest with a consistent digest still does not authenticate
its publisher or prove that its claimed bytes actually exist on another machine.

## Checks and next stage

Run `go test -race -timeout=45s -cover ./internal/skillbundle` for local fixtures.
Coverage includes deterministic order/location-independent identity, instruction
and script edits, executable/line-ending changes, file add/remove/rename,
unreadable/missing/over-limit data, symlinks/case collisions, read-time mutations,
replacement FIFO and malformed/incompatible manifests.

The immediate product priority is the terminal workspace redesign (T1 in
`handoff.md`). The next content stage, S2, adds privately owned immutable revision
storage and ownership checks. S3 adds
explicit machine observations/import. S4 adds reviewed content apply. Until those
stages ship, live sync still compares identity/source and membership; a matching
source must not be presented as content equality.
