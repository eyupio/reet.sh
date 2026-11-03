# Machines → Add a skill

Date: 2 November 2026. Status: done. Scope: the **Add a skill…** picker on a machine
card (`web/src/lib/MachineFleet.svelte`, `web/src/lib/MachineAddSkill.svelte`). Removal
from a machine remains a separate follow-up.

## Why

A machine card could start a whole saved sync, preview it or open the Sync page, but
it could not put one named skill on that machine. The shelf card could, through
**Add now**, but the machine card is where a person is already looking at the machine.

## Contract

1. **The machine card menu offers Add a skill…** for a writer with an enrolled
   agent. Without an agent it is disabled and says to connect one first.
2. **The picker lists shelf skills with a linked source.** Installed skills are shown
   with an **Installed** tag and no Add button; local or unlinked entries are not
   offered.
3. **One search and one list.** Search reads the name and source. An empty list says
   why nothing can be added, and an empty search says it matched nothing.
4. **Add reuses the existing intent.** It calls `installSkillOnMachine`, so it saves
   the same additive intent, reuses a fresh saved set, or asks for a rescan when the
   machine has no recent complete one.
5. **The result lands on the card.** A normal add says **Installing {skill} on
   {machine}. Follow it on the card.**; a scan-first add says **Scan requested for
   {machine}. Add {skill} once it reports.**
6. **No new API or permission.**

## Rulings

- **Shelf candidates only.** Private bundles and hosted drafts that cannot be
  installed from the shelf are not offered here. *Cost if wrong:* those skills cannot be added from this
  picker.
- **Installed is shown, not hidden.** A person can see that the skill is already
  there without it becoming an add action. *Cost if wrong:* the list can be longer.
- **The picker closes after an add or scan request.** Progress belongs on the card
  and the sync activity, not in a dialog. *Cost if wrong:* a second skill needs the
  menu opened again.
- **The action is disabled without an agent.** *Cost if wrong:* a person must connect
  the machine before using the picker, which is already the requirement.

## Testing

- Browser: `machine-add-skill.e2e.mjs` opens the picker from the card menu, checks
  installed and available rows, adds a skill and asserts the saved intent and run
  bodies, and checks the phone layout at 390 and 320 px.
- Regression: `control.e2e.mjs`, `phone-zoom.e2e.mjs`, `npm run check` and the
  production build.
