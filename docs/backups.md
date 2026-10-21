# Backups

reet takes copies of two things:

- **The instance**, for the owner: the whole SQLite database, every account,
  shelf, list, tag and environment.
- **A person's shelf**, for that person: their own skills, notes, tags and
  environments, exported as the same JSON snapshot the UI calls Download JSON.

Both are sealed with one key, kept on the machine, and copied to any
S3-compatible destination you configure. Off-site is where a copy survives
losing the machine.

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
for byte, and deletes it again. It never leaves the test object behind.

## Schedule and retention

The instance defaults to a copy every 24 hours, keeping the newest 48 here.
A person's own backups are manual until they choose an interval, keeping the
newest 14. Change both on the Backups page. `0` turns the schedule off; a count
of `0` keeps every copy. Each destination keeps its own count, so an off-site
bucket can have a longer tail than this machine.

Retention runs after every successful take. The page also lists what each
destination holds, live from the bucket.

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
```

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
