## DESKS-COPY-1: the Desks list is rows, leads with what needs you, and says what each count means (28 November 2026)

Only `web/src/lib/desks/DeskList.svelte` changes (and the selectors in `desks.e2e.mjs`). Nothing else reads its markup.

What changed.

- Cards become rows in one bordered list (name, tabs and panes, machines, counts, actions). Desks with a shell waiting
  come first under "Waiting for you", with the product's waiting tint on the whole row and an Open button; the rest follow
  under "Other desks" (the headings show only when both lists exist). Order inside each list is the person's order.
- The counts are pills in the shared status colours (waiting amber, live green, to reopen slate, each with a dot and
  always words): "1 shell needs you", "3 shells live", "1 shell to reopen".
- Removed: the thumbnail strips (decoration, and a coloured bar down one side), and the dashed New desk card, which
  repeated the header button and said "from a template" though it only started an empty desk. From a template stays the
  header button, and each template's menu keeps New desk from this template.
- Added: a definition of a desk under the title, the next step in the empty state, a line under Templates, and machines
  counted once each ("a removed machine" no longer repeats).
- On a phone a row stacks: name and actions, then tabs and machines, then the pills. Touch targets are 44 px.
- The `.card` hook in `desks.e2e.mjs` is `.row`; `.tpl`, `.meta`, `.pill` and `.empty` are unchanged. Three assertions follow the new wording.

Checked. The whole `desks.e2e.mjs` suite passes (105 checks), `npm run check` and the theme test pass, and the list was
looked at in light and dark, at 1280 and 390 px, with one desk waiting and with none.

Not done. "Move to its own tab" (card menu and desk switcher) can be read as a desk tab rather than a browser tab; the
same label is in both places, so it wants its own change. Not looked at: a very long desk name or many machines beside
the Open button (the name truncates, the machines wrap), 200% zoom, and a list of dozens of desks.
