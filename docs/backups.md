# Backups

reet takes copies of two things:

- **The instance**, for the owner: the whole SQLite database, every account,
  shelf, list, tag and environment.
- **A person's shelf**, for that person: their own skills, notes, tags and
  environments, exported as the same JSON snapshot the UI calls Download JSON.

Both are sealed with one key, written where the owner chooses, and copied to
any S3-compatible destination you configure. Off-site is where a copy survives
losing the machine, so "both" is the default.

## The key

Copies are encrypted with AES-256-GCM; the key is derived from a passphrase
with Argon2id. Set `REET_BACKUP_KEY` to a value from:

```sh
reet backup keygen
```

If it is empty, reet generates `backup.key` beside the database on first start
and says so on the Backups page. That is convenient and not sufficient on its
own: keep a copy of the file somewhere other than this machine, or the
off-site copies become unreadable when the machine is lost. The fingerprint
shown on the page identifies the key without revealing it.

A destination's secret access key, a stored copy password, the outgoing mail
password and the private-connection identity are sealed in the database with
the same key, so a copy of `reet.db` does not expose them. They only open under
that key, which matters when a copy is restored onto a controller with another
one (see below).

## Compression and a copy password

Each owner chooses, with the schedule, how a copy is stored:

- **Compression**: `gzip` (the default) or `none`. Compressed copies are
  smaller on this machine and in every destination; the manifest records
  whether a copy was compressed so any build opens it.
- **Copy password** (optional): when set, each copy is sealed with a key
  derived from that password with Argon2id instead of the instance key, so
  the copy opens with the password alone. The password is stored sealed
  under the instance key, which is what lets the schedule take such copies
  without anybody typing it; it is never returned to a page. At least 12
  characters. Removing it affects later copies only: existing
  password-protected copies still need that password.

### A password for one copy

The copy password above is a setting: it seals every copy, including every
scheduled one. That is the wrong tool for someone who operates a controller for
another team. They hold the only owner credential, the team's people are not
owners, and a portable copy for the team has to be sealed with a password the
team chose. Setting the team's password, taking the copy and clearing it again
would seal every copy taken in between (the schedule's included) with a password
the operator does not know, leave the password in the settings for the whole
window, and leave later copies sealed with it if clearing it failed.

So the password can be a parameter of one copy instead:

```sh
reet backup run --password-file ./team-copy.password    # or - for standard input
```

```sh
curl -X POST https://reet.example/api/v2/admin/backups/run \
  -H "Authorization: Bearer $OWNER_TOKEN" -H "Content-Type: application/json" \
  -d '{"password": "a password the team chose"}'
```

- **It seals that copy alone.** Same format and manifest as a copy sealed with
  the stored copy password (`protection` is `password`), so it opens with
  `reet backup open --password-file` on any machine. It does not open with the
  controller's key.
- **The stored setting is never read, changed or cleared.** Scheduled copies and
  every other run carry on as they were, and a stored password that could not be
  opened does not stop this run.
- **The rule is the setting's:** at least 12 characters. A blank password, or a
  shorter one, is refused before anything is taken: `400` with "a copy password
  needs at least 12 characters", and the copy list and the run history do not
  change. A body is optional; with none, an empty object or `"password": null`
  the route behaves as it always did. An unknown key, or a password that is not a
  string, is refused rather than quietly sealing the copy with the stored password.
- **Owner only,** as the route has always been.
- **The password is never stored, logged, returned or put in an error.** The
  activity log records `backup-run` with `passwordOnRequest` true and
  `protection` `password`, against the owner who made the request and the time,
  and nothing else about the password.
- **A copy sealed this way is the team's to open.** The controller does not know
  the password, so `POST .../backups/items/{id}/verify` cannot open it without
  the password in its body (`{"password": "..."}`), and a restore from the
  controller's own list needs `--password-file`. The copy is opened with the
  password and checked as it is taken, as every copy is.

`GET /api/version` lists `"copy-password-request"` under `features` when the
route accepts the body.

A copy downloaded from the page is a `.zip` holding the sealed payload
(`<id>.sealed`) and its clear manifest (`<id>.json`). The `.sealed` file opens
exactly like a `.reetbackup` copy. Open it on any machine with:

```sh
reet backup open --password-file ./password copy.reetbackup > copy.db
```

`reet backup open` also opens a key-protected copy when `REET_BACKUP_KEY`
holds the key it was sealed under. The command writes only the plaintext
payload, so redirect it to a file and treat that file as the secret it is.

Every copy is opened as a restore would immediately after it is sealed —
digest checked, decompressed, and for the instance run through SQLite's
integrity check — before it is written over an older copy, shipped, or
counted by retention.

## Off-site destinations

Any service that implements the S3 API works: AWS S3, Cloudflare R2, Backblaze
B2, MinIO, Garage, Ceph. Add one under **Admin → Backups** (instance copies) or
**Account → Backups** (your own shelf), or describe one in the environment
with `REET_BACKUP_S3_*`.

The credentials need, on the bucket and the prefix:

- `s3:PutObject`
- `s3:GetObject`
- `s3:DeleteObject`
- `s3:ListBucket`
- `s3:AbortMultipartUpload`

A destination may also set `REET_BACKUP_S3_PASSPHRASE`: the already-encrypted
payload is then sealed once more before it is stored, and the manifest records
only `passphrase_sealed`, `outer_size` and `outer_sha256` — never the
passphrase. A wrong or changed passphrase is named when the copy is fetched.

Use `https://` for a destination on another machine. An `http://` endpoint on
localhost is accepted; an `http://` endpoint elsewhere is refused unless
`REET_BACKUP_ALLOW_LOCAL=1`, because the access key id travels in the clear
even though the copies themselves are sealed.

The **Test** button writes a small object under the prefix, reads it back byte
for byte, and deletes it again. It never leaves the test object behind. The
form has its own **Test** beside **Save**, against exactly what is typed
there, so a destination and its secret can be proven before they are stored.
When an existing destination is edited and the secret is left blank, the test
uses the stored secret only while the endpoint is unchanged.

Saving a destination reports what happened in the form itself: a refused
name, endpoint, bucket or credential is shown where it was typed rather than
only at the top of the page.

## Where copies go

Each owner chooses one of three answers, saved with the schedule:

- **This machine and off-site** (the default): every copy is written beside
  the database and sent to each enabled destination.
- **This machine only**: nothing is sent. A copy survives a mistake, not the
  disk.
- **Off-site only**: a copy is sent to an enabled destination and removed from
  this machine once at least one destination has it. If no destination takes
  it, the copy is kept here rather than lost, and the run says so. With no
  usable destination at all, the run fails before taking a copy and says what
  to fix.

Reading, verifying, downloading and restoring treat a copy the same way
wherever it is; one held only at a destination is fetched from there. Set the
default for a new owner with `REET_BACKUP_DESTINATION=local|remote|both`.

## Escrow

A machine rebuilt from nothing has no database to read the key or the
destinations from. Seal them together under a passphrase, kept somewhere
other than this machine and the backup bucket:

```sh
reet backup escrow --out reet-escrow.sealed --passphrase-file ./passphrase
```

The file holds the backup key and every destination, secrets included,
compressed and sealed with the passphrase alone. It is only as useful as the
passphrase, so keep the two apart. To get one destination back:

```sh
reet backup unescrow --in reet-escrow.sealed --passphrase-file ./passphrase \
  --remote offsite > restored.env
```

It prints `REET_BACKUP_KEY` and one destination as `REET_BACKUP_S3_*` lines,
and names the others. Run it again after changing the key or a destination.

## Schedule and retention

The instance defaults to a copy every 24 hours, keeping the newest 48 here.
A person's own backups are manual until they choose an interval, keeping the
newest 14. Change both on the Backups page. `0` turns the schedule off; a count
of `0` keeps every copy. Each destination keeps its own count, so an off-site
bucket can have a longer tail than this machine.

Retention runs after every successful take. **Apply retention now** runs it
without taking a copy, and reports what it removed here and per destination.
**Copy offsite now** sends each enabled destination whatever this machine
holds and it does not, then applies that destination's own count; it is what
to run after fixing a credential. The page lists what each destination
holds, live from the bucket, and one of those copies can be removed from
there without touching this machine.

A copy sent to a destination that may pass (a 5xx, 429 or 408, or a
connection that failed) is tried three times before the run records the
refusal; a refusal that will not pass is tried once.

A copy is sealed and opened as one payload in memory, so this build caps a
single copy at 512 MiB. That is far above a normal shelf; an instance that has
kept years of event history may need its history pruned first.

## What an off-site object is

Each successful copy is exactly **one object**: a small clear manifest header,
then the sealed payload. The key ends `.gz.enc` when the plaintext was gzipped
before it was encrypted, and `.enc` when it was not. The manifest records the
target, time, plaintext and payload sizes and SHA-256 digests, compression, key
fingerprint and version/instance facts; it never holds a passphrase, access
key, token, password or private key. Compression always happens before
encryption, never after.

Listing reads only the bucket keys, so it never GETs every copy. Fetching reads
the header only where a manifest is needed, verifies the header digest and the
payload digests, and reconstructs the local sealed payload. A destination with
its own passphrase seals the already-encrypted payload once more and records
`passphrase_sealed`, `outer_size` and `outer_sha256`; a wrong or changed
passphrase is named when the copy is opened.

A copy larger than the store's single-PUT limit is sent as a multipart upload
with parts sized for the store. The whole object is hashed before completion; a
completion whose answer was lost is settled by checking that the object is
there, and a failed upload is aborted so no copy is advertised that may not
exist.

The earlier one-object layout (`<id>.reetbackup`) and the older two-object
layout (`<id>.json` plus `<id>.sealed`) are still listed, fetched, restored and
deleted. When both layouts exist for one name the new one-object key wins.
Nothing is rewritten during an upgrade: new copies use the new layout and old
ones age out by retention.

## Verifying

**Download** hands over a `.zip` with the sealed payload and its manifest, never an uncompressed archive. **Verify** opens a copy as a restore would:

- an instance copy is written to a temporary file and checked with SQLite's
  `PRAGMA quick_check`;
- a shelf copy is parsed and checked for a skills array.

Every copy also carries the SHA-256 of its payload inside its unsealed header,
so a damaged file fails before a restore begins.

## From the terminal interface

Run `reet` and choose **Backups** for the existing list, take, restore and open
commands. Press `e` to select a backup ID or add options. The real command keeps
its confirmations and password handling; output stays visible until you return
home. See [CLI home](cli-home.md).

## Restoring the instance

The server cannot swap the database it is serving from. Stop reet, then on its
machine:

```sh
reet backup restore --yes <id>
reet backup restore --yes --password-file ./password <id>   # a password copy
```

A copy protected with a password uses the password it was given; with none
it uses the one stored for that owner when the database is still there.

Without `--yes` the command opens the copy, runs every check below, prints what
the copy holds and what would be done to it, and changes nothing. With `--yes`
it prints the same inventory, then moves the live `reet.db` aside to
`reet.db.before-restore-<timestamp>`, removes stale `-wal`/`-shm` files and
writes the restored database in place. Start reet again afterwards. Nothing is
deleted, so the database you replaced can be put back by hand. The copy is
opened into a temporary file beside the database, read and adapted there, and
the file is removed afterwards; the password is read from a file or standard
input, never from the command line.

### What a restore checks, in order

Every check runs before anything live changes, so a refusal leaves `reet.db`
exactly as it was.

1. **The copy must not be newer than this reet.** The database records a schema
   version (`PRAGMA user_version`) that each build stamps when it opens it. A
   copy stamped higher than the restoring build understands is refused with a
   message naming both data formats and the copy's release, and a request to
   upgrade first. The check reads the version from the opened copy, not from
   the manifest's release string: development builds have no ordered release,
   and the database is what the binary actually has to read. A copy from
   before versions were recorded reads as 0 and is accepted.
2. **The copy must pass SQLite's integrity check.**
3. **The copy's source must have been stopped.** A copy that says its source
   was still running, or says nothing, is refused unless `--allow-live-source`
   is given (see below).
4. **`--owner-email ADDR` must be an owner in the copy.** The address must
   belong to an owner account that is not disabled, or the restore is refused
   and names the owners the copy does have. It exists so nobody swaps in a
   database they cannot sign in to.
5. **The inventory is printed**: accounts by role, shelves, skills, lists, tags,
   environments, connected machines and backup destinations.

### When nobody here can sign in as the copy's owner: `--make-owner`

A copy can arrive on a machine where its owner is no use: the controller signed
people in through an identity provider the new machine does not have, or its
owner account belonged to whoever operated it before and the controller has been
handed to a new operator. Name an account that is already in the copy and it
becomes the owner, with a password you choose:

```sh
reet backup restore --yes --make-owner sam@example.com --owner-password-file ./owner-password <id>
```

- The account must be in the copy and not disabled; otherwise the restore is
  refused, naming the address, before anything changes. No account is created.
- The password is read from a file, or standard input for `-`, never from the
  command line, and must be at least 8 characters. It is not printed or logged.
- There is one owner, so the owners the copy had become admins. Nothing else
  about them changes; remove or disable them afterwards if they should not stay.
- The new owner's earlier browser sign-ins and reset links are ended. Its
  two-step sign-in, if it had one, is kept.
- The report says who was made owner and who is now an admin, and the activity
  log records `ownerMade`. Without `--yes` the same is shown and nothing changes.

It cannot be combined with `--owner-email`, which only checks. It is for the
restore on the machine itself; an import into a running controller does not take
it, because that controller already has an owner who is signed in.

### What a copy records about where it came from

A whole-controller copy now carries two fields in its clear header, readable
before it is opened (`reet backup open` and the listing show them):

- `source`: the controller's own identifier (`ctl_` and 32 hex digits), made the
  first time a copy is taken and kept in the database's settings. It names the
  controller, not its address, so it stays the same when the address changes. A
  copy restored on another machine brings the identifier with it, because it says
  which controller the data came from.
- `fenced`: true when the controller had been told to stop acting for the whole
  time the copy was being taken. The state is read before and after the snapshot
  and the copy is marked fenced only if both reads agree, so a fence raised or
  lifted mid-copy reads as not fenced, the safe side.

A copy taken by an older reet has neither field and reads as **unknown**.

### Why a copy of a running controller is refused

`restore` and `import` refuse a copy whose `fenced` is false or unknown. Two live
controllers working from one set of data split it between them, and a copy of a
running controller may already be out of date by the time it lands. Stop the old
controller, take a final copy once it is stopped, and use that. If you know the
old controller is gone (a failed machine, say) and the copy is the best you have,
say so with `--allow-live-source`; the API form field is `allowLiveSource`.

The check runs with the others, before anything changes, so a refusal leaves
`reet.db` as it was, and an import stages nothing. When the flag is used the
choice is written to the activity log (`backup-restored`, with `allowLiveSource`,
`sourceState` of `live` or `unknown`, and `source`); no secret is recorded. A
fence recorded in a copy is never carried across: the controller a copy lands on
is the one meant to be live, so a restore clears it.

### Moving a controller, the order of it

1. On the old controller, fence it (`reet backup fence --moved-to NEW`) so it stops acting.
2. Take the final copy (`reet backup run`); it is marked fenced.
3. On the new controller, restore or import it with `--public-url NEW`, and set what the report's checklist lists by hand.
4. Machines whose agents are running follow on their own once the new controller answers, because the fence named its address; on any other machine run `reet agent set-server NEW` (see [following a moved controller](controller-sync.md#follow-a-controller-that-has-moved)). Either way the machine has the new controller prove it holds its record before it sends any credential.
5. Resume the work the restore held back.
6. When everything is working, the old controller can stay fenced or be retired.

### Moving a controller from the web page

The same move needs no terminal on either controller. Everything below is in the
**Move** section of **Admin → Backups**, for the owner only, and each piece is offered only when the
controller lists it in `GET /api/version` (see
[asking a controller what it can do](#asking-a-controller-what-it-can-do)).

**On the old controller, Move this controller away:**

1. **Fence this controller.** Type the new address if you know it (optional, a
   plain `https://` origin) and confirm. The dialog says what stops: every change
   is refused, background work stands still, nothing is deleted, and it can be
   undone. From then on the page carries a banner saying since when the
   controller has been fenced and where it moved to, and every other page shows
   one line under the header saying the same, so a refused save is not the first
   anybody hears of it. An admin's pages read the state when they open; anyone
   else sees the line after the first change the controller refuses.
2. **Take the final copy.** This step opens only once the fence is up. Type a
   copy password of at least 12 characters: it seals this one copy
   ([a password for one copy](#a-password-for-one-copy)), is not stored, and is
   what the new controller will ask for. The page then shows the copy with
   **taken fenced** beside it, read from the copy's own header, and **Download
   the final copy**. If the fence changed while the copy was being taken the
   page says so and asks for another, rather than handing over a copy the new
   controller would refuse.
3. The card then lists what is left: the import on the new controller, and on
   each machine the command `reet agent set-server <new address>`, with a button
   to copy it.
4. **Lift the fence** is there the whole time. It asks first, because a
   controller that is already running from the copy would then share its data
   with this one.

**On the new controller, Move a controller in:** choose the downloaded copy and
type its password. **This controller's address** is filled in with the address
the page was opened at; it is the `publicUrl` of an import, so the addresses
saved in the copy are rewritten to it, and clearing the field leaves them alone.
A copy that was not taken fenced is refused, the controller's own words are
shown beside the form with everything you typed still there, and only then does
the page ask for the tick **The old controller is stopped or gone**
(`allowLiveSource`). Once the copy is staged the page shows the report: what
came across, what was done about the address, what was held back, what is not in
the copy and has to be set by hand, and the `set-server` command for the
machines. Restart the controller to put the copy in place.

The first answer of the import, and `GET /api/v2/admin/backups/import` while a
copy is staged, carry `notInCopy`: the same checklist the command line prints,
as `[{"names": [...], "say": "..."}]`. A copy in `GET /api/v2/admin/backups` and
the answer of a run carry `fenced` (true, false, or absent when the copy does
not say) and `protection` (`key` or `password`) from the copy's header. An import
refused because its source was live answers `409` with
`"code": "copy_source_live"`; one refused because the controller already holds
data answers `"code": "controller_not_empty"`.

### Fencing a controller that is moving

Moving a controller safely means the old one stops acting before the final copy
is taken. The **fence** is how. It is an owner-set state, saved in the database,
that says "this controller has moved". It deletes and changes no data, and
lifting it returns the controller to exactly what it was.

```sh
reet backup fence --moved-to https://reet.new.example   # raise it (the address is optional)
reet backup run                                         # the final copy; it is marked fenced
reet backup fence --lift                                # undo it
```

The command works on the data in `REET_DATA_DIR`, so it can be used with the
server stopped or running: the server reads the state from the database each
time, never from memory. The same is available to the owner over the API:

| Request | Answer |
| --- | --- |
| `GET /api/v2/admin/fence` | the state |
| `POST /api/v2/admin/fence` with `{"movedTo": "https://..."}` | raises it; the address is optional and must be an http or https address |
| `DELETE /api/v2/admin/fence` | lifts it |

Every answer is `{"fenced": bool, "since": int64, "movedTo": string}`, with
`since` in milliseconds since the Unix epoch (0 when not fenced). A `POST` when
already fenced answers with the existing state and changes nothing, so a second
call cannot move the date or the address. A `DELETE` when not fenced answers the
same way and succeeds. Like every `POST`, it needs a JSON body; send `{}` for no
address. Raising and lifting are for the owner only. An admin may read the state
with `GET`, which is what lets every admin page say that the controller is fenced
before a change is refused; it tells them nothing the 423 does not.

**While fenced:**

- **Every write is refused with 423 (Locked)**, with a message that says the
  controller has moved, when, and to where when the owner said. Writes include
  `/quick`, which saves through a `GET`, and every MCP tool that changes
  something; MCP reads still work, and so do all other reads.
- **Exempt:** the owner's backup settings (`PATCH /api/v2/admin/backups`), run
  (`POST .../backups/run`), download and verify, so a final copy can be taken and
  checked; the fence's own `POST` and `DELETE`; and signing in and out. Sign-in is
  not in the list of exemptions the contract names, and is a write, but without it
  an owner whose session had lapsed could not get back in to take the final copy
  or lift the fence.
- **Background work stands still.** Each place that acts on its own checks the
  fence on its next pass, and carries on after the lift:

  | Work | Where it checks | While fenced |
  | --- | --- | --- |
  | Scheduled backups | `backup.Service.runDue` | none are taken; the owner's own run still works |
  | Mail queue | `mail.Queue.RunOnce` | nothing is sent; queued mail stays queued |
  | Notifier | `notify.Service.Loop` | old read notifications are not pruned |
  | Project runs | `projectrun.Orchestrator` (`Paused`) | no member is started; members stay pending |
  | Software batches | `softwarerun.Orchestrator` (`Paused`) | no machine is updated; members stay pending |
  | Gaffer goals | `gaffer.Service.reviewDueGoals` | no goal is reviewed |
  | Gaffer outcomes | `gaffer.Service.advanceOutcomes` | no approved session is started; the outcome waits |
  | Queued chat messages | `terminalbroker.Broker.stepChatQueue` | none is delivered to a worker |
  | Worktree sweeps | `runScheduledMaintenance` | no sweep runs on any machine |
  | Topic sync | `syncTopics` | not fetched |
  | Catalogue refresh | `refreshBatch` | not run |

- **A fenced start comes up fenced and quiet.** The state is in the database, so
  a restart keeps it. Pending project runs and software batches are not resumed
  at start.
- **After the lift**, the API continues the paused project runs and software
  batches at once, and a watcher continues them within five seconds when the lift
  was made with the command line. The loops above need no nudge.

What the fence does not stop: a member or an update already in flight when the
fence goes up finishes, because it cannot be recalled; and mail already sent is
sent. That is why the fence goes up before the final copy is taken, and why a
copy taken while fenced is the one `restore` and `import` accept without
`--allow-live-source`. A copy carries no fence: a restore clears it, because the
controller a copy lands on is the one meant to be live.

### When the controller answers at a new address: `--public-url`

`reet backup restore` and `reet backup import` take `--public-url ADDRESS` (the
API form field is `publicUrl`). It must be a plain http or https origin, such as
`https://reet.example.com`; a path, query, fragment or credentials are refused,
and the refusal comes before anything changes. With it:

- the saved **mail base URL** and the **sync library origin** are rewritten to the
  new address, and nothing else is changed;
- **MCP grants** issued for the old address are removed, because each is bound to
  the address it was issued for, so every client authorises again here. The MCP
  connections themselves are kept (they are revoked only by `--drop-tokens`);
- the report says what was changed, and the activity log records `publicUrl`.

Without it, no address is touched. `REET_PUBLIC_URL`, which the server reads at
start, is not in the database: set it on the new machine as well.

### What is held back, and how to resume it

A copy can hold work that would start again by itself the moment the restored
controller starts, doing something the old one may already have done. A restore
or import holds it back, using the states the product already has, so each item
resumes with the retry that already exists, and nothing is deleted:

| Work | Held as | Resume with |
| --- | --- | --- |
| Gaffer goals | paused (needs the user) | resume the goal |
| Gaffer outcomes still starting | stopped; sessions not yet started stay unstarted | give Gaffer the outcome again |
| Project runs | each pending member becomes failed with the code `paused_by_restore`; started members are untouched | retry the member |
| Software batches | the first unfinished machine fails with `paused_by_restore` and the batch stops; machines that were running go back to waiting | retry that machine |
| Queued mail (queued or sending) | held | send it from the outbox; a held reset link points at the old address |

The report lists each run, batch and the mail count under "Held back". Held work
is not resumed at start, because the orchestrators resume only pending members.

### What is not in the copy

The report ends with a checklist of what lives outside the database and has to
be set by hand on the new machine: the `REET_*` variables the server reads, the
single sign-on settings (register the new `REET_OIDC_REDIRECT_URL` with the
identity provider), the backup key and any backup schedule or destination set in
the environment, the mail outbox folder, the terminal state in
`terminal-control/state.json`, and the service definition. It is derived from the
code: a test reads what `serve`, the OIDC and backup readers, the mail outbox
setting and `reet install` use, and fails when a setting is added that the
checklist does not name.

### What a restore changes in the copy

The copy is then adapted, and only the temporary file is changed:

- **Sign-ins and personal tokens come across.** By default every browser
  session, password-reset link, personal API token and MCP connection in the
  copy keeps working on the controller it lands on. `--drop-sessions` ends every
  session and reset link. `--drop-tokens` revokes every personal API token and
  MCP connection. Use them when the old controller stays in service, or when
  you do not trust who held a credential. `--keep-tokens` is accepted and does
  nothing: keeping is now the default.
- **The private-connection identity is always dropped.** The restored
  controller makes its own on first use; two controllers never present one
  identity.
- **Stored secrets that cannot be opened are cleared.** A destination's access
  key, a stored copy password or the mail password sealed under a different
  `REET_BACKUP_KEY` cannot be opened here. Rather than keep a value nothing can
  read, the restore clears it, switches a destination off and records why, and
  lists each with what to do: enter the access key again and switch the
  destination on, set the copy password again, or enter the mail password
  again. The cleared values are still in the database that was moved aside,
  which opens with the old key. A copy protected with a password opens on any
  machine, and these are the secrets it does not carry.
- **The choices are written to the activity log** of the restored database
  (`backup-restored`): the copy's id and release, whether sessions and tokens
  were dropped, whether an owner was checked and how many secrets were cleared.
  No secret is in it.

Machine connectors keep the credentials they were enrolled with, so they work
against the restored controller once it answers at the address they use.

### Import into a running controller

For a team that cannot stop a controller to run a command on its machine. The
owner sends a copy and its password to the running controller, which runs the
same checks and prints the same report as a restore (`Service.prepare` is one
function, used by both), and stages the result.

**Why it is staged, not applied.** The server holds `reet.db` open, with its
write-ahead log and connections, and swapping the file under it would corrupt
what it is writing. The smallest safe way is to do everything that can fail
while the server keeps serving, then put the file in place before the database
is opened: the checked, adapted copy waits in `pending-import/`, and `reet serve`
swaps it in at its next start, before anything opens the database. Restart the
controller to finish. If the swap fails, the staged folder is moved aside and
the controller starts on the database it had. A second import replaces the
first; `DELETE` discards it.

**From the terminal**, from any machine that can reach the controller:

```sh
reet backup import --server https://reet.example --token-file ./owner.token \
  --password-file ./copy.password --owner-email you@example.com copy.reetbackup
```

The token is an owner's personal API token. A secret on the command line is a
file or `-` for standard input, never an argument; the token and the password
cannot both be `-`. `--drop-sessions`, `--drop-tokens` and `--owner-email` mean
what they do for a restore, and so does `--allow-live-source`: the example above
needs it unless the copy was taken from a fenced controller. `--replace` is
described below. The command streams
the file, so it is not read into memory on this side.

**From the page**, under **Admin → Backups → Move a controller in**: choose the
copy, enter its password if it has one, check the address and pick the options.
The page shows the report and a **Discard** button while it waits. See
[moving a controller from the web page](#moving-a-controller-from-the-web-page).

**Over the API**, `POST /api/v2/admin/backups/import` takes `multipart/form-data`
and the owner only. Send the options first and the copy last, as the field
`file`, and add the header `X-Reet-Import: 1` (a website cannot add a header to a
form, which keeps the upload from being forged cross-site):

```sh
curl -X POST "$REET_URL/api/v2/admin/backups/import" \
  -H "Authorization: Bearer $TOKEN" -H 'X-Reet-Import: 1' \
  -F 'password=<copy.password' -F ownerEmail=you@example.com \
  -F dropSessions=1 -F dropTokens=1 -F replace=0 \
  -F file=@copy.reetbackup
```

(`-F 'password=<file'` sends the file's contents as the field, so the password is
not on the command line; the file must hold the password and nothing else, with no
trailing newline, because the server uses it exactly as sent.) The password is a form field in the body, never in the
address or a header; nothing logs it, the response does not repeat it and the
activity log does not hold it. The upload is capped at the largest copy this
build opens (512 MiB) and written to disk as it arrives, then removed. A copy is
sealed and opened as one payload, so this build still holds the opened payload in
memory once while it checks it; the cap is what bounds that. `GET` on the same
path shows what is staged and `DELETE` discards it.

**A controller that already holds data is refused** with `409`, naming what it
holds, unless you send `replace` (`--replace`). A new controller has its first
owner and nothing else: no other account and no skills, shelves, lists, tags,
environments or connected machines. Backup destinations and settings do not
count, because they are set up before an import on purpose. Nor does what a
controller makes for itself on first start: the topic tags the daily sync adds
and the report the server makes of its own machine. A tag you created, or a
machine you connected, does count.

**A safety copy comes first.** After every check has passed and before anything
is staged, the live database is copied to `reet.db.before-import-<timestamp>`
(mode 0600) beside it, and the response, the report and the activity log say
where. Together with `reet.db.before-restore-<timestamp>`, which the swap at the
next start leaves, the database you replaced is never lost. A refusal, a wrong
password or a damaged copy changes nothing: no safety copy, nothing staged, no
upload left on disk.

The activity log gets one `backup-import-staged` line with the copy's id and the
choices (`dropSessions`, `dropTokens`, `ownerChecked`, `replace`, the safety copy
and how many secrets were cleared). On the next start the staged database
replaces the live one, which is kept as `reet.db.before-restore-<timestamp>`, and
the report is written to the log.

### A staged import is checked again when it is applied

A staged import can wait a long time for the restart. When `reet serve` goes to
apply it, it asks again what was asked at staging: the staged database is still
the file that was checked (its SHA-256 is recorded in `pending-import/report.json`
when it is staged), it still opens and passes SQLite's integrity check, and its
data format is no newer than this binary reads. The copy's password is not asked
for a second time: it was spent opening the copy, and what waits in
`pending-import/` is the opened database, so the recorded digest stands in for
it.

If any of that fails, the live database is not touched. The stage is moved to
`pending-import.refused-<timestamp>/`, its `report.json` gains a `refusal` field
saying why, the same reason is written to the log, and the controller starts on
the database it already had. Stage the copy again once the cause is dealt with.

### Asking a controller what it can do

`GET /api/version` answers `{"version", "commit"}` and, when this build holds
any part of controller moving in full, `"features": [...]`, drawn from `"fence"`,
`"copy-origin"` and `"restore-public-url"`. A name is listed only when every piece
of that part is built. A controller built before the list existed has no
`features` field, and so does one that holds none of the parts; treat a missing
field as "none". A script that moves a controller should read it first rather
than assume. The Backups page does the same: the move card needs `"fence"`, the
copy's own password `"copy-password-request"`, the fenced mark and the
live-source tick `"copy-origin"`, and the address field `"restore-public-url"`.

### A machine rebuilt from nothing

1. Install reet and point `REET_DATA_DIR` at a fresh folder (or start the
   container with an empty data folder).
2. Put the backup key in `REET_BACKUP_KEY`, or copy `backup.key` into the data
   folder.
3. Describe the destination with `REET_BACKUP_S3_*`, or start reet once and add
   it under **Admin → Backups**.
4. `reet backup list` shows the copies on the machine; a copy only at the
   destination is fetched by `reet backup restore`.
5. `reet backup restore --yes <id>` and start reet. If the key is not the one the
   copy was sealed under, use a copy with a password; the report names the
   stored secrets it cleared, to enter again.

## Restoring a person's shelf

**Account → Backups → Restore** puts that person's shelf back. It only touches
their own account: the server checks the copy's owner before restoring. Other
people's shelves and the instance's catalogue are untouched.

## Export instead

For a one-off, **Download JSON** (or `GET /api/export`) is the unsealed
snapshot, and **Restore from JSON** (or `POST /api/restore`) brings it back.
It is useful for moving between instances and for reading the data with other
tools. It is not encrypted, so treat it accordingly.

## Where things live

| What | Default |
| --- | --- |
| Database | `<data>/reet.db` |
| Copies on this machine | `<data>/backups/*.reetbackup` |
| Generated key | `<data>/backup.key` |
| Off-site object | `<prefix>/instance/<id>.gz.enc` or `<prefix>/user-<id>/<id>.gz.enc` (`.enc` when uncompressed) |
