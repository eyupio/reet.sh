# Folder browse on the machine (browse-v1)

How a machine answers a `browse` request (`internal/terminalhost/browse.go`). The protocol lives in
`internal/terminalproto/browse.go`. The design is in
`docs/superpowers/specs/2026-10-26-agents-home-design.md` ("Remote folder browser protocol").

## What a browse may do

- **Calls.** It may list a folder (readdir) and lstat or stat paths. No file is opened or read, no git
  command runs, and `.git/HEAD` is checked only for existence.
- **Confinement.** Every call runs through an `os.Root` opened at the real (link-resolved) home. A
  folder swapped mid-browse for a link that leads out is refused, not followed. A filesystem that
  cannot be confined this way is refused (`unreadable`).
- **Paths.** The requested path is cleaned lexically before it is resolved, so `..` never steps back
  out of a folder reached through a link. A refused path's reason is found by walking from home one
  component at a time. A path that leaves home at any point is `outside_home`, whether or not
  anything exists there.
- **Bounds.** A browse reads at most 5,000 entries (`scanned` shows the cap), returns at most 200,
  and its answer is at most 12 KiB. After 3 seconds it makes no more repository checks, and the page
  ends at the next link.

## Known limits

- **Search-only folders.** `os.Root` opens each folder on the way down for reading. A folder the
  user can enter but not read (mode `0311`, for example) therefore blocks browsing anything under
  it: those folders answer `denied`, and repository checks through them read false. A folder that
  can be read but not entered (`0644`) is `denied` as well. This is rare inside one's own home; the
  folder can still be typed as a launch folder.
- **Link chains through outside home.** A link in home is listed when its target finally resolves
  to a folder inside home, even if the chain passes through a link outside home on the way. For
  example, `~/via -> /elsewhere/back -> ~/real` is listed. This follows the spec ("target resolves
  to a directory inside the home") and can only show whether such links, which already exist,
  point back into home.
- **Reading links.** The walk that explains a refused path reads links with `os.Readlink`
  (unrooted), on paths beneath folders already inside home. A folder swapped mid-walk could make it
  read the text of a link outside home. That text only steers the walk, which stays inside home
  and answers `outside_home` otherwise.
- **Hung filesystems.** Some calls cannot be cut short by the deadline: resolving the path, opening
  the root, and reading the folder. A hung network or FUSE mount can hold one browse, so the connector
  must free its one-browse-at-a-time slot at the deadline even while that call is stuck.
- **Windows.** There is no `O_DIRECTORY`, so the guarantees rest on `os.Root` and the re-checks.
