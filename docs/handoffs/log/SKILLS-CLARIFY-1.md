## SKILLS-CLARIFY-1: the Skills page says what Delete does and what an empty Active tab means (28 November 2026)

A copy pass over the signed-in Skills page (`web/src/lib/skills/SkillsPage.svelte`), done with the Impeccable `clarify`
playbook. The rest of the skills area (rows, opened skill, dialogs, bulk bar) was read and left as it was: its copy
already names the object and the outcome.

What changed.

- The lede's "Delete it to be rid of it." is now "Delete it to remove it from reet." at every machine count.
- The empty Active tab tells three cases apart: nothing at all (unchanged), everything Archived (new: says so and offers
  Restore or a pasted link), and a read-only account (new: says it can view but not add).
- The "New on a machine" banner says what ends the wait: "until you keep or archive them", matching the Keep and Archive
  buttons in the review.

Kept on purpose. The eyebrow "EY UP. YOUR SKILLS." above the heading (brand voice, the owner chose to keep it), the
"No machines yet" chip (it links to Sync, whose empty state has the Add a machine button), and the "Use this version"
button, which docs and Go error messages name.

Checked: the build, the skills unit tests, the design detector (no findings), and the `first-skills`, `skills-new`,
`skills-prune`, `skill-open` and `empty-states` browser suites. Not checked: the two new empty-state sentences (all
Archived, read-only account) have no test asserting them, and nothing was looked at in the browser at 390 px or in dark.
