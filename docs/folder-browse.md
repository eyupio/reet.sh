# Folder browse and branch listing (browse-v1, branches-v1)

How a machine answers a `browse` request (`internal/terminalhost/browse.go`), how it lists branches,
and how the controller offers both through its API (`internal/terminalbroker/browse.go`,
`internal/api/terminal_browse.go`). The protocol lives in `internal/terminalproto/browse.go` and
`branches.go`. The design is in `docs/superpowers/specs/2026-10-26-agents-home-design.md` ("Remote
folder browser protocol").

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
  reach the controller after its own deadline; the controller drops it (see below).

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

## On the controller

- **Negotiation.** The controller advertises `browse-v1` and `branches-v1` in
  `X-Reet-Terminal-Features` beside its other features. It sends a `browse` only to a connection
  whose current capabilities report `browse`, and a `branches` only with `branches`; it never sends
  either to an older connector, which would end the connection on a kind it does not know. The
  request is checked with the protocol's own rules first, so a malformed one never reaches a machine.
- **API.** `GET /api/v2/machines/{agentId}/browse?path=&after=&filter=&hidden=&session=` and
  `GET /api/v2/machines/{agentId}/branches?dir=&filter=`. Each parameter may appear once and no
  other is accepted (400 otherwise). `path` is empty for the home; `hidden` is `true` or `false`;
  `session` is required on browse: a random id the web makes once per opened browser (16 to 128
  letters, digits, `-` or `_`). The answers are the result types with every field present:
  `{path, home, parent, repo, repoTop, entries: [{name, dir, repoRoot, hidden}], more, partial,
  scanned, omitted}` and `{current, default, branches, more}`, sent with `Cache-Control: no-store`.
- **Who may ask.** The terminal rules: a signed-in browser account that is not a viewer, in accounts
  mode, with remote terminals on; API tokens, MCP tokens, the single-token and open modes and
  viewers are refused (403). Both routes are reads, so the browser sends no `Origin` header on them;
  a request the browser marks as coming from another site or origin (`Sec-Fetch-Site` other than
  `same-origin`, or an `Origin` that is not reet's own address) is refused (403 `origin_refused`).
  Another account's machine is answered exactly as an offline one.
- **Limits.** 20 browses and, separately, 10 branch listings per 10 seconds per account and machine
  (429 `browse_rate_limited` or `branches_rate_limited`). The controller waits 6 seconds for a
  browse and 10 for a listing (the machine answers by 4 and 8 seconds).
- **Late answers.** Answers are matched to requests only by their request id. A `browse_result` or
  `branches_result` that arrives after its request gave up has no request to go to: the controller
  counts it and drops it, and the connection stays up. It is never passed to another request.
- **Offline and disconnects.** A machine that is offline, reconnecting or not ready is answered at
  once, without waiting, and a machine that disconnects while a request is in flight fails only that
  request at once; both with the readiness sentence. A machine that stays connected but does not
  answer by the deadline gets `machine_timeout`.
- **Nothing kept.** The controller keeps no folder names, branch names, paths it was shown, filters
  or listings: no cache, no store write, no log line. The only record is one `folder-browse` audit
  event per opened browser (one `session` id on one machine), written when the machine answers its
  first page, with exactly `machine` (the machine's id), `path` (the folder the browser started at,
  as the person typed or chose it; `~` for the home) and `pages` (pages the machine answered,
  including a folder it refused; a busy answer is not a page). Later pages only update `pages`,
  which stops counting at 1,000. An account may have 32 browsers open at once; a browser idle for 30
  minutes is forgotten, and a new browser past the bound is answered `browse_rate_limited`. Branch
  listings are not audited.

### What a person reads

| Code | Status | Sentence |
|---|---|---|
| `browse_unsupported` | 409 | Update reet on that machine to browse its folders. |
| `branches_unsupported` | 409 | Update reet on that machine to list its branches. |
| `browse_off` | 409 | That machine has folder browsing switched off. |
| `branches_denied` | 409 | That machine isn't offering its branches right now. Check that terminal access is on there and git is installed. |
| `browse_rate_limited`, `branches_rate_limited` | 429 | Slow down a moment, then try again. |
| `not_found`, `not_dir` | 404 | That folder isn't on studio. (the machine's name) |
| `outside_home` | 422 | That folder is outside the home folder reet was set up with. |
| `denied`, `unreadable` | 422 | reet can't read that folder. |
| `not_repo_root` | 422 | That folder isn't the top of a git repository. |
| `git_failed` | 422 | Git couldn't list the branches there. |
| `browse_busy` | 409 | That machine is already listing a folder. Try again in a moment. |
| `branches_busy` | 409 | That machine is already listing branches. Try again in a moment. |
| `machine_unavailable` | 503 | That machine is reconnecting. Try again in a moment. |
| `machine_timeout` | 504 | That machine didn't answer in time. Try again. |

The answer never repeats the path or anything the machine sent.

### What is not established

- **Old reet or switched off.** Capabilities do not say why `browse` is missing. Because
  `branches-v1` shipped with `browse-v1`, a machine that reports `branches` but not `browse` is read
  as having its browse switch off (`browse_off`), and one that reports `browse` but not `branches`
  as not offering branches now (`branches_denied`). A current machine that reports neither (browse
  off and no git, say) is told to update.
- **`denied` has two meanings.** The machine answers a browse `denied` both for a folder it may not
  read and while its switch is off. The controller reads the capabilities again: if `browse` is
  gone, the answer is `browse_off`. In the few seconds before the machine reports the change (its
  consent poll), a switched-off machine reads as "reet can't read that folder."
- **The starting folder is the person's input.** `path` in the audit row is what the browser asked
  for first, so a typed path is kept as typed. Nothing the machine listed is kept.
- **Pages are counted in memory.** A controller restart forgets open browsers, so a browser that
  keeps going after a restart starts a second audit row.

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
