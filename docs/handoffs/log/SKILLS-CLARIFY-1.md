## SKILLS-CLARIFY-1: the Skills page says what Delete does, what an empty tab means, and keeps colour for what needs a look (28 November 2026)

A copy and layout pass over the signed-in Skills page (`SkillsPage.svelte`, `SkillRow.svelte`), done with the
Impeccable `clarify`, `distill`, `adapt` and `colorize` playbooks, from screenshots at 1440 px (light and dark) and
390 px. The opened skill, dialogs and bulk bar were read and left as they were: their copy already names the object and
the outcome.

What changed.

- The lede's "Delete it to be rid of it." is now "Delete it to remove it from reet." at every machine count.
- The empty Active tab tells three cases apart: nothing at all (unchanged), everything Archived (new: says so and offers
  Restore or a pasted link), and a read-only account (new: says it can view but not add).
- The "New on a machine" banner says what ends the wait: "until you keep or archive them".
- Colour: a healthy skill's line ("On all 3 machines") is muted text with a green dot, so the red "Failed on ThinkPad"
  and the blue "Installing" are the only coloured lines in the list. Delete wears the normal button colour and turns red
  on hover and keyboard focus, instead of five red buttons down the page.
- Phone (700 px and under): the name, heart, Archive and Delete share one line, the two actions as 44 px icon buttons
  with the same accessible names; the health line sits under them. A skill is about 30% shorter, so five fit on the
  first screen. The add field's placeholder was cut off ("...add comn") and is now "Paste a skills.sh link or npx
  command".

Kept on purpose. The eyebrow "EY UP. YOUR SKILLS." above the heading (brand voice, the owner chose to keep it), the
"No machines yet" chip (it links to Sync, whose empty state has the Add a machine button), the "Use this version" button
(docs and Go error messages name it), and Delete as an always-visible action with Undo (nothing hidden in a menu).

Not done, and worth a look next. The phone filter block (search, Liked, tags, sort, Select) takes three rows above the
list; a "Filters" disclosure would shorten it. The "Make it yours" eyebrow on Library tools is another eyebrow.

Checked: the build, the skills unit tests, the design detector (no findings), screenshots at 1440 px in light and dark
and at 390 px in light, and the `first-skills`, `skills-new`, `skills-prune`, `skill-open`, `empty-states` and
`supporting-nav` browser suites. Not checked: the two new empty-state sentences have no test asserting them, the phone
layout was not looked at in dark, and nothing was tried on a real touch device.
