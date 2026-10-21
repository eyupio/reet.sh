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

A destination's secret access key is sealed in the database with the same
key, so a copy of `reet.db` does not expose it.

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

A password-protected copy downloaded from the page is a `.reetbackup` file.
Open it on any machine with:

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

## Verifying

**Download** hands over the sealed `.reetbackup` file exactly as it is kept and shipped. **Verify** opens a copy as a restore would:

- an instance copy is written to a temporary file and checked with SQLite's
  `PRAGMA quick_check`;
- a shelf copy is parsed and checked for a skills array.

Every copy also carries the SHA-256 of its payload inside its unsealed header,
so a damaged file fails before a restore begins.

## Restoring the instance

The server cannot swap the database it is serving from. Stop reet, then on its
machine:

```sh
reet backup restore --yes <id>
reet backup restore --yes --password-file ./password <id>   # a password copy
```

A copy protected with a password uses the password it was given; with none
it uses the one stored for that owner when the database is still there.

The command opens the copy, checks it, moves the live `reet.db` aside to
`reet.db.before-restore-<timestamp>`, removes stale `-wal`/`-shm` files, and
writes the restored database in place. Start reet again afterwards. Nothing is
deleted, so the database you replaced can be put back by hand.

A copy that exists only at a destination is downloaded by the same command.

### A machine rebuilt from nothing

1. Install reet and point `REET_DATA_DIR` at a fresh folder (or start the
   container with an empty data folder).
2. Put the backup key in `REET_BACKUP_KEY`, or copy `backup.key` into the data
   folder.
3. Describe the destination with `REET_BACKUP_S3_*`, or start reet once and add
   it under **Admin → Backups**.
4. `reet backup list` shows the copies on the machine; a copy only at the
   destination is fetched by `reet backup restore`.
5. `reet backup restore --yes <id>` and start reet.

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
| Off-site object | `<prefix>/instance/<id>.reetbackup` or `<prefix>/user-<id>/<id>.reetbackup` |
