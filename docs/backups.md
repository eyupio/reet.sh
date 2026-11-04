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
3. **`--owner-email ADDR` must be an owner in the copy.** The address must
   belong to an owner account that is not disabled, or the restore is refused
   and names the owners the copy does have. It exists so nobody swaps in a
   database they cannot sign in to.
4. **The inventory is printed**: accounts by role, shelves, skills, lists, tags,
   environments, connected machines and backup destinations.

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
what they do for a restore. `--replace` is described below. The command streams
the file, so it is not read into memory on this side.

**From the page**, under **Admin → Backups → Import a copy from another
controller**: choose the copy, enter its password if it has one, pick the
options. The page shows the report and a **Discard** button while it waits.

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
