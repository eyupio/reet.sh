## SKILLS-FILTERS-1: on a phone the Skills filters sit behind a Filters button (28 November 2026)

Follow-up to SKILLS-CLARIFY-1, which left the phone toolbar taking three rows (search, Liked and Tag, Sort and Select)
above the list.

What changed.

- At 760 px and under, `SkillsPage.svelte` shows Search, a Filters button and Select on one row. Liked, Tag, List and
  Sort are in a group (`#skills-filters`) that opens under them, two to a row. The button is `aria-expanded` and
  `aria-controls` that group, is 44 px tall, and carries a count of the filters that are not at their default (Liked on,
  a tag, a list, or a Sort other than Name), so a filtered list is not mistaken for the whole one.
- Wider than 760 px nothing moves: the group is `display: contents` and the button is hidden.
- Select stays on the first row because it starts a bulk action, not a filter.
- `first-skills.e2e.mjs` has a `filters` section: at 390 px Liked is hidden until Filters opens, the count says 1 after
  Liked is on, and there is no sideways scroll; at 1440 px Filters is hidden and everything is visible.

Checked: the build, the skills unit tests, the design detector (no findings), light and dark screenshots of the phone
toolbar shut and open, and the `first-skills`, `skills-new`, `skills-prune`, `skill-open`, `empty-states` and
`supporting-nav` browser suites. Not checked: a real touch device, and the filters group is not closed again when a
filter is cleared from the empty state's Clear filters button.
