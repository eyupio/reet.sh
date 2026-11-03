# Machines audit minors

Date: 2 November 2026. Status: done. Scope: the Machines inventory and Commands
dialog. The audit references are in `UI_AUDIT.md`.

## N1 — one time language

1. **Per-card recency stays relative**: last report, group scan and sync activity
   use `agoLabel`.
2. **Absolute times use the person's profile**: `formatWhen` reads the account's
   time zone and date order, drops seconds and is used for every title/tooltip.
   A profile with no date order reads as the device's order, as elsewhere.
3. **One helper.** `web/src/lib/time.ts` owns the absolute format; MachineFleet and
   EnvironmentSync receive `timeZone` and `dateFormat` from the profile through
   Machines.

## N7 — one way out of Commands

1. The Commands dialog keeps the header close control and the primary
   **Open compare & sync** link.
2. The duplicate footer **Done** button is gone. The dead **Set up from scratch**
   action remains gone.

## H10 — one icon language

1. `web/src/lib/Icon.svelte` is one 16 px SVG set: chevron, plus, refresh,
   arrow-right, x and more.
2. The machine setup step/action arrows, Commands and Settings close controls and
   the machine card overflow control use it. Icons are decorative; the control
   keeps its text or label.

## N6 — attention order and a compact status filter

1. **Order.** The fleet sorts by attention: action, review, working, attention,
   setup, synced; host name breaks ties. The just-added card still bypasses the
   filters.
2. **Filter.** When the fleet has more than six machines, a **Status** select offers
   **All machines / Needs attention / Working / Up to date**, each with its count.
   Setup and paused/offline machines count as needing attention.
3. **Together.** Search and status combine; the count becomes `N of M machines`; an
   empty result offers **Clear filters**, which resets both.
4. **The card dot follows the agent.** Green means online, amber paused, hollow
   offline. A paused card no longer shows a green connection dot.

## No API or permission change

The work reads the existing sync snapshot and profile preferences. It adds no
request, write or permission.

## Rulings

- **The status filter appears only past six machines.** *Cost if wrong:* three
  machines do not get a filter that would save nothing.
- **Setup counts as attention.** A machine with no agent needs a person before it
  can produce inventory. *Cost if wrong:* “Needs attention” includes machines that
  are not broken, only unconfigured.
- **The profile format is for absolute tooltips, not the visible recency.** *Cost if
  wrong:* a long date on every card would dominate the card.
- **Commands has one close affordance.** *Cost if wrong:* a person who expects Done
  at the bottom uses the header close or Escape.

## Testing

- Unit: `time.test.mjs` covers the profile date order, time zone, no-seconds and
  fallbacks.
- Browser: `machines-fleet.e2e.mjs` covers attention order, the status filter and
  counts, empty recovery and the 44 px filter on phones. `phone-dialogs.e2e.mjs`
  covers the Commands dialog's single primary link; `control.e2e.mjs` and
  `machines-search.e2e.mjs` cover the renamed setup arrows and Clear filters.
- Regression: `npm run check`, `npm run build`, `phone-zoom.e2e.mjs` and
  `navigation.e2e.mjs`.
