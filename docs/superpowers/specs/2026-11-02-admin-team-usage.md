# Admin → Team usage

Date: 2 November 2026. Status: done. Scope: the **Team usage** section of
Instance administration (`/admin/team`).

## Why

The adoption report was a wide table with a sticky first column and a second,
separate card rendering of the same rows for smaller screens. The two versions
could drift, the table scrolled sideways, and the count and search were missing.
The report is read to answer one question — which skills are earning their keep,
and where does a list need attention — so it belongs in the same collection grammar
as the other directory pages.

## Contract

1. **One report panel.** **Team usage** has a live skill count, **Download CSV** and
   **Refresh** opposite it, the team figures beneath, and a search over skill name,
   source, list and tag.
2. **The existing filters stay as selects.** **List**, **Tag** and **Sort** are
   selects because the values can be long and numerous; the search is the free-text
   control. The count follows all four.
3. **One row per skill.** Name, source, list chips and tag chips; **In use** with its
   bar, **Saved by** and **Agent reports**; and the Team disclosure with the people
   and agent reports. The same row stacks on a phone instead of a second card
   rendering.
4. **Honest figures and empty states.** `N skills` or `N of M skills`, a summary
   line for the team, a privacy note when per-person rows are off, and **Clear
   filters** when nothing matches.
5. **Download CSV follows the filters.** The file contains exactly the rows on
   screen.
6. **The secondary “List gaps & people” disclosure stays** below the panel, with
   the same drift and people content.
7. **No API or permission change.**

## Rulings

- **List and Tag remain selects, not chips.** A team can have many long list and
  tag names. *Cost if wrong:* the primary filter is less immediate than the chips
  used on Lists and Teams.
- **Search reads name, source, list and tag.** It does not search member names;
  the Team disclosure keeps those. *Cost if wrong:* a person cannot find a skill
  by typing a colleague's name.
- **The Team disclosure sits in the right-hand column on desktop** and under the
  row on a phone. *Cost if wrong:* on a wide screen it sits below the metrics
  rather than under the skill name.
- **The drift and people views stay folded.** They are secondary to the adoption
  rows. *Cost if wrong:* one more press to reach them.
- **No API change.**

## Testing

- Unit: `adminTeam.test.mjs` covers search by name, source, list and tag, and the
  count label.
- Browser: `admin-team.e2e.mjs` covers the count and summary, search, List and Tag
  filters, empty recovery, the row metrics and Team disclosure, and phone layouts
  at 390 and 320 px with no overflow.
- Regression: `admin-users.e2e.mjs`, `phone-zoom.e2e.mjs`, `navigation.e2e.mjs`,
  `npm run check` and the production build.
