# Skill card: large fleets and unconfirmed machines

Date: 2 November 2026. Status: done. Scope: the installation badges on a shelf skill
card (`web/src/lib/SkillCard.svelte`) and the Add now action behind them
(`web/src/lib/syncActions.ts`). The Machines page and the per-machine picker are not
part of this pass.

## Why

A card with more than three machines showed three badges and a **+N more** link, so
the collapsed card hid half the fleet behind a small control. A machine whose scan was
stale or incomplete showed **Not confirmed** and no action at all, even though the
person could ask it to rescan. The two changes answer both without pretending an
unconfirmed machine is missing.

## Contract

1. **Large fleets collapse to one answer.** With more than three machine reports and
   the card collapsed, one summary button says `On N of M`, then `· X missing` and
   `· Y not confirmed` when either is non-zero. Pressing it expands the card and shows
   every machine badge.
2. **Small fleets are unchanged.** Three or fewer machines show the existing badges
   without a summary.
3. **Add now is offered on Missing and Not confirmed.** The unconfirmed button keeps
   the honest caveat: no recent complete scan, so the action asks the machine to
   rescan first.
4. **An unconfirmed Add now queues a scan, not an install.** It posts the existing
   scan job (`kind: scan`, no target) and says **Scan requested for {host}. Add
   {skill} once the scan is complete.** It never claims the skill was installed.
5. **An offline unconfirmed machine is refused** with **{host} is offline. Reconnect
   it, then scan and add.**
6. **The ⋯ menu follows the badge.** Missing reports say **Install on {host}**;
   unconfirmed reports say **Scan and add on {host}**.
7. **No new API or permission.** The scan is the existing sync job.

## Rulings

- **The summary replaces +N more.** *Cost if wrong:* a person must press once to see
  which machines are missing; the summary already says how many.
- **The summary includes unconfirmed machines rather than hiding them.** *Cost if
  wrong:* the line is longer on a mixed fleet.
- **Add now on Not confirmed is honest but not automatic.** *Cost if wrong:* the
  person presses Add now twice: once to scan, once after the scan reports.
- **A scan is not queued for an offline machine.** *Cost if wrong:* the person must
  reconnect first, which is already what Sync does.

## Testing

- Unit: `machines.test.mjs` covers the summary counts for installed, missing and
  unconfirmed machines.
- Browser: `skill-card.e2e.mjs` covers the four-machine summary and expansion, the
  unconfirmed Add now queueing a scan, and the 44 px summary on a phone.
- Regression: `phone-zoom.e2e.mjs`, `npm run check` and the production build.
