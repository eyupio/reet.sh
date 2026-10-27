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

## On the connector

- **Negotiation.** The connector serves `browse` only on a connection whose controller advertised
  `browse-v1`, and `branches` only with `branches-v1`. On any other connection such a message is a
  protocol error: nothing is sent back (a peer that did not negotiate the kind could drop the
  connection on its answer) and nothing runs.
- **Capabilities.** `browse` is reported when `browse-v1` was negotiated, the connector has terminal
  control, terminal access is on locally and folder browse is not off. `branches` is reported when
  `branches-v1` was negotiated, `worktree` is reported and terminal access is on. A change of either
  local switch is sent to the controller within the consent poll (5 s), without a reconnect. The
  connector serves a negotiated browse or branches request only while it would advertise that
  capability, worked out again for each request from cheap local reads (the enrollment file and
  git on `PATH`, never the tmux readiness probe); otherwise it answers `denied` itself.
- **Local switch.** `reet terminal browse off` (also `reet agent terminal browse off`) sets
  `browseOff` in the enrollment file; `on` removes it and `status` prints `Folder browse: on|off`
  (also shown by `reet agent terminal status`). Browse is on by default. While it is off, or while
  terminal access is off, a browse is answered `denied`. Each request re-reads the enrollment file.
  `on` while terminal access is off says that browsing stays unavailable until terminal access is
  on, and `status` then shows `on (unavailable while terminal access is off)`. Enrolling the machine
  again writes a fresh enrollment file, so browse is on again afterwards.
- **One at a time.** Each connection runs one browse and one branch listing at a time, aside from
  the dispatcher; a second is answered `conflict` at once. A browse that has not returned after 4 s
  (a listing after 8 s) is answered `unreadable` (`git_failed`) and its slot is freed, even if the
  call is still stuck; its result is dropped when it comes. At most 4 browses (and, separately, 4
  listings) stay alive in the connector process: while that many are stuck, a new one is answered
  `conflict` until one finishes. When the connector's queue of waiting requests is full, a browse or
  listing is likewise answered `conflict` by the reader, but only on a connection that negotiated its
  feature; otherwise it ends the connection as a protocol error. An answer given at the deadline can
  reach the controller after its own deadline, so the controller must drop a late `browse_result` or
  `branches_result` rather than treat it as a protocol error.

## Branch listing (branches-v1)

`ListBranches` (`internal/terminalhost/branches.go`) answers `branches`:

- **Folder.** `dir` is cleaned and resolved as a browse resolves its path. A path that leaves home
  is `outside_home`; anything else that is not a repository root is `not_repo_root`. A repository
  root has a real `.git` folder (not a link, not a `.git` file) holding a `HEAD` file, checked
  beneath an `os.Root` at home. The folder is resolved and checked again after git ran.
- **Git.** Exactly `git -C <dir> for-each-ref --count=201 --sort=-committerdate
  --format=%(refname) refs/heads refs/remotes`, then `symbolic-ref --quiet --short HEAD` and
  `symbolic-ref --quiet --short refs/remotes/origin/HEAD`, as an argument vector with the worktree
  environment (`HOME`, `PATH` and `LANG` only), within 5 seconds in all. The filter never reaches git.
  Each call also gets `GIT_DIR=<dir>/.git` and `GIT_CEILING_DIRECTORIES=<parent of dir>`, so git
  reads that `.git` or fails: a `.git` folder git does not accept (a `HEAD` alone, a garbage `HEAD`)
  is `git_failed`, never a listing of a repository around it, inside home or above it.
- **Names.** Short names (`main`, `origin/main`), newest first, at most 200, and the answer fits
  12 KiB; `more` says the list was cut. A remote's `HEAD`, a branch called `HEAD`, a repeated short
  name and any name the protocol cannot carry (for example `@`, a space, or a control or format
  character) are skipped, since one would invalidate the whole answer. `current` is empty on a
  detached HEAD; `default` is the branch `origin/HEAD` points at, without `origin/`.
- **Errors.** A git failure is `git_failed`, with nothing of git's output. `denied` (a reason
  added for `branches-v1`, matching browse's) means the machine refuses the listing locally:
  terminal access is off, or it does not offer `branches` at the moment (no git, or worktrees not
  available).

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
  the root, and reading the folder. A hung network or FUSE mount can hold one browse. The connector
  answers it at 4 s and frees its slot (see above); the stuck call stays until the mount answers, and
  four stuck calls make further browses `conflict` until one returns.
- **A repository swapped while git reads it.** `ListBranches` checks the folder, runs git in the
  resolved path, then checks the folder again and answers `outside_home` if it changed. Someone who
  can write inside home and swaps the folder for a link in between makes git read (never write) a
  repository elsewhere; its names are not sent.
- **`more` on branches is conservative.** It is set whenever git returned 201 lines, including a
  remote's `HEAD`, a name that was skipped, or a warning line git printed (its output is read with
  its error output), so it can be set when nothing more exists.
- **Repository metadata that points elsewhere.** git follows what a repository's own files say:
  `.git/commondir`, a symlinked `.git/refs` or ref, `objects/info/alternates` and `include.path` in
  its config can make a listing read refs, objects or configuration outside home. Planting them
  needs write access inside home, and worktree-v1 reads the same repositories the same way. Nothing
  in a repository's configuration is run by the three listing commands.
- **Short names can collapse.** A local branch called `origin/main` and the remote branch
  `origin/main` have the same short name; only the newer is listed. `symbolic-ref --short` can name
  the current branch `heads/x` when a tag `x` also exists; that name is sent as it is.
- **Windows.** There is no `O_DIRECTORY`, so the guarantees rest on `os.Root` and the re-checks.
