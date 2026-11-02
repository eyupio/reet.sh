# Directory pages refresh

Date: 2 November 2026. Status: in progress. Scope: the signed-in collection pages that
show a person a set of records to browse or manage: **Lists**, **Community**, **Teams &
organisations**, and **Setups**.

## Why

The product's main surfaces have been brought to one editorial language: warm paper,
Fraunces headings, one card per idea, chips for filters and honest empty states. The
directory pages were left at different stages of that work:

- **Lists** has no surface at all: rows sit on the page background, the filter is a
  select, the count is a sentence below the controls, ownership is printed twice
  ("Owned by you · Owner"), and the Rows/Cards switch detaches on a phone.
- **Community** puts its only count inside two tab buttons and offers a separate card
  grid for lists and skills.
- **Teams & organisations** is a set of always-open management forms. A person cannot
  scan the groups they belong to without reading every member and every role select.
- **Setups** already has a strong empty state, but its populated list and its controls
  follow a fourth grammar.

The result is not a visual disaster; it is a navigation and scanning cost. A person has
to relearn where the count, the filter and the primary action live on each page.

## Design authority

The approved direction is the **Library** study in `web/concepts/controller.html`, whose
shelf grammar the shipped Skills and Home pages already use: warm paper, serif headings,
one bounded surface, quiet secondary actions, and a single accent for the primary
action. The shipped Home mock pack (`docs/design/agents-home/`) settles the tokens and
type scale; `docs/voice-and-copy.md` settles the words.

No binding mock exists for these four pages. Where the shipped pages and the Library
direction disagree on tokens or type, the direction wins. Where they disagree on the
page's job (browsing versus administration), the page keeps its job.

## The contract every page in this family applies

1. **One page header.** `PageHeader` keeps the eyebrow, title and lede. The page's own
   primary action sits opposite the title, with a secondary action beside it where one
   exists. The page title is not repeated inside the surface.
2. **One collection surface.** The records sit inside one bordered panel on `--card`,
   with the same radius and shadow as a shelf card. The panel carries an accessible name
   ("Your lists", "Community", "Your teams and organisations").
3. **The count is a figure, not prose.** The panel head shows `N records`, or
   `N of M records` while a filter narrows the set. It is a live region. It uses only
   counts the page already has.
4. **One toolbar.** Search is a labelled field. The primary filter is a row of chips
   that keep one value pressed at a time (`aria-pressed`). A layout choice, where the
   page has one, is a segmented group with a visible group label. The toolbar wraps
   before anything is clipped.
5. **Counts on chips are honest.** A chip's count is computed over the records that
   match the current search, so pressing the chip always shows that many. A chip whose
   figure would be wrong for what is beneath it is withheld, not guessed. A scope that
   does not exist in the data (the legacy audience, for example) is not offered.
6. **Records share one anatomy.** A record is one row: name (serif), description, a
   short metadata line (audience, size, owner/access, recency), and the whole row is the
   link. Ownership is said once. Relative time is used for recency, with the absolute
   date and time in the title. A row never becomes two competing rows of metadata.
7. **The layout choice survives.** Where a page already offers Rows/Cards, the choice
   keeps its existing storage key and default. A saved layout is never discarded.
8. **The surface owns loading, error and empty.** Loading draws skeleton rows; an error
   appears in the panel with a retry; an empty set appears in the panel with one clear
   next action (clear the filters, create the first record, or open the related page).
9. **Nothing new is reachable.** The refresh adds no route, no write and no field the
   API does not already expose. URLs, saved layouts and links from elsewhere keep
   working.
10. **Phone first.** At 320 and 390 px there is no horizontal document overflow; the
    toolbar wraps; touch targets are at least 44 px; the fixed tab bar never covers the
    last record or the primary action.

## Pages and order

| Order | Page | Route | What changes | Status |
| --- | --- | --- | --- | --- |
| 1 | Lists | `/shared` | Proving page. Collection panel, scope chips with counts, one ownership line, relative recency, skeleton/empty/error states, phone toolbar. Introduces the kit. | Done |
| 2 | Community | `/community` | Same panel and row anatomy for community lists and skills; counts in the panel head; tabs become a segmented scope with counts. | Done |
| 3 | Teams & organisations | `/spaces` | Same panel; groups become scannable rows with member and role figures; management folds inside the row; create becomes the page's primary action. | Done |
| 4 | Setups | `/profiles` | Same panel for saved profiles; the empty state keeps its wizard copy. | Not started |

## Rulings made on the person's behalf

- **R1 — The scope filter becomes chips and keeps no URL state.** It has no URL state
  today, so nothing that works stops working. *Cost if wrong:* filter links cannot be
  shared until a `?scope=` value is added; that is a small follow-up.
- **R2 — Rows and Cards both survive, and the saved choice wins.** The product already
  offers both and stores the choice. *Cost if wrong:* two layouts keep receiving fixes;
  if usage shows one is dead, it can be removed in a later, explicit change.
- **R3 — Community lists use the same row anatomy as Lists; skills use rows too.** Rows
  scan better past about eight records and match the family. *Cost if wrong:* the public
  directory feels less like a browsable gallery; cards can return as a layout choice.
- **R4 — Metadata keeps only what helps a choice.** Audience, size, owner/access and
  recency. The duplicate "Owner" word and the separate "Owned by you" are folded into
  one phrase. *Cost if wrong:* a person who used the duplicate role word to distinguish
  an admin from an owner loses that distinction on this page; the list's own page and
  the team pages still show it.
- **R5 — Recency is relative here.** "Updated 3 days ago", with the exact timestamp in
  the title. *Cost if wrong:* the profile's date-format preference is not applied on
  this page; a shared helper can apply it later across every surface.
- **R6 — Teams keeps management in the page and folds it per group.** No new dialog and
  no new route. *Cost if wrong:* managing a member takes one extra press; the page is
  much shorter and scannable in exchange.
- **R7 — No API or store changes.** The family only re-presents what the server already
  sends. *Cost if wrong:* a genuinely missing affordance (for example pinning a list)
  has to wait for its own backend change.
- **R8 — Page titles and voice stay as they are.** "Lists.", "Have a nosy.", "A proper
  toolkit. Together." and "Setups." keep their character. *Cost if wrong:* the family is
  unified in structure rather than in headline; that is deliberate.

## Testing

- **Unit:** `web/src/lib/lists.test.mjs` covers search matching, scope matching, scope
  counts under a search, the count label, and the access/ownership label.
- **Browser:** `web/tests/lists.e2e.mjs` runs against the production build and the
  workspace fixture. It checks the count against the API, every chip press, honest chip
  counts while searching, the empty-result recovery, the saved layout, no horizontal
  overflow at 320 and 390 px, and the accessible names of the search, scope and layout
  groups. It is run before and after the change; the new assertions fail on the old
  build.
- **Regression:** `npm run check`, `npm run build`, the phone-dialog, phone-zoom and
  navigation specs that share `/shared`, and the repository's Go checks.
- **Captures:** `/shared` at 1440, 834 and 390 px in light and dark, reviewed by eye
  before the change is called done.
