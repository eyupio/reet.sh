# Written-skill lifecycle

Date: 3 November 2026. Status: done. Completes
`docs/superpowers/specs/2026-11-03-written-skill-record.md`.

## One lifecycle

A written skill's content stays in `hosted_skills`; its shelf card is a row in
`skills`. That row is now a **projection** with one lifecycle, not a second
independent record. `skills.hidden` marks the projection while the skill is not
public.

| Action | Hosted content | Shelf projection | Lists and counts |
| --- | --- | --- | --- |
| Save draft | updated | hidden | hidden |
| Publish | updated | upserted, unhidden, metadata refreshed | visible |
| Unpublish | updated | hidden | hidden |
| Republish | updated | unhidden, metadata refreshed | visible |
| Rename | moved to the new slug | moved with tags, notes, lists and hidden state | follows the new id |
| Delete | removed, sync binding dropped | hidden tombstone | hidden |

A hidden projection keeps its tags, notes and list membership. Republishing the
same slug unhides it with that data intact.

## Readers

The open shelf, the per-person shelf and its counts, a shared list's rows and
count, the admin rollup, and the export path all read `hidden=0`. A hidden row is
therefore not reachable through a list, a list count or an export. The internal
`Get` by id still loads it for the owning workflow; no public list exposes it.

## Rulings

- **Hide the shelf projection instead of deleting it on unpublish.** *Cost if
  wrong:* a tombstone row remains in `skills` after every publish/unpublish.
- **Delete the hosted content but hide its projection.** *Cost if wrong:* other
  people's shelf cards for the deleted public skill disappear, but their tags,
  notes and list membership remain and return if the author publishes the same
  slug again.
- **Omit hidden rows from exports and backups.** *Cost if wrong:* a draft's
  shelf metadata is not in a controller backup; the hosted content is not part of
  this export either.
- **Keep `source_type` and the public URL on the hidden row.** *Cost if wrong:* a
  direct store read by id still sees the old public reference; list readers hide
  it, and republishing restores the same identity.

## Testing

- `store_hosted_test.go` checks publish, draft, rename while hidden, republish
  with notes intact, delete, the open shelf, the per-person shelf, a shared
  list's rows and its count.
- `hosted_test.go` checks that a draft disappears from the author's and another
  person's shelves, returns on republish, and disappears after delete, without
  a name collision with another author's same-named skill.
- `go test ./internal/store ./internal/api`, `go test ./internal/backup` and
  `go vet ./...` pass.
