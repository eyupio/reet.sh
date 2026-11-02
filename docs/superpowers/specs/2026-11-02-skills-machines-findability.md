# Skills and Machines findability

Date: 2 November 2026. Status: done in the follow-up to the directory pages
refresh. Scope: the shelf's Jump search, the "Skills you create" rows on the
Skills page, and the fleet list on Machines.

## Why

The directory refresh gave Lists, Community and Teams one collection grammar. The
next questions are where a skill is and which machine has it:

- Jump searched sessions, folder projects and machines, but not the skills the
  page is built around.
- A "Skills you create" row said nothing about machines. The same published skill
  could appear again further down as a shelf card with different controls and no
  link between them.
- Machines showed counts only: "2 skills", "1 agent scope". To find which machine
  had `code-review`, a person had to go back to Skills and read badges.

## Contract

1. **Jump finds skills.** A skill result is labelled with its name and carries
   `Skill · <saved intent>` (or its source when the sync state has not loaded).
   Selecting it opens `/skills?skill=<id>` and brings that card into view once.
   Results stay in the order sessions, Projects, skills, machines.
2. **A created skill says where it is.** Each hosted-skill row shows the saved
   intent first ("Pending on dev-laptop", "Set to sync to …"), otherwise the
   observed state ("On 2 machines · 1 missing", "Missing on 2 machines"), otherwise
   "Not on a machine yet". A published skill links to its shelf card. A draft keeps
   the existing **Sync to machines…** link and is never described as installed.
3. **Machines can be searched.** The fleet has one labelled search over the host,
   saved aliases and every skill any agent scope reported. The count is honest
   (`1 of 2 machines`), a search that matches nothing offers **Clear search**, and a
   machine just added stays visible even while a search that does not name it is
   active.
4. **No new surface.** The work reads the shelf, the saved sync selections and the
   existing machine reports. It adds no API, write or permission.

## Rulings

- **Jump reads the loaded shelf.** It does not issue a second full read. *Cost if
  wrong:* while a shelf filter is active on the Skills page, Jump sees that filtered
  set; a full read can be added if that becomes confusing.
- **A machine is matched by observed skills only.** Saved sync intent is not a
  report that the files are there. *Cost if wrong:* a skill set to sync but not yet
  applied does not make a machine match until a scan reports it; the skill card's
  intent line still says it is pending.
- **Skill results come before machines.** A skill is the thing being looked for;
  the machine is usually the follow-up. *Cost if wrong:* a host named like a skill
  appears one row lower.
- **The row line is one sentence, not badges.** The HostedShelf row is compact and
  already links to Sync; a second badge strip would repeat the shelf card above it.
  *Cost if wrong:* a person who wants each machine's badge must open the linked
  shelf card.
- **No new API.** *Cost if wrong:* a per-host breakdown in the hosted row waits
  for a dedicated read.

## Testing

- Unit: `tabs.test.mjs` covers Jump order and skill matching;
  `machines.test.mjs` covers host, alias and skill matching and excludes server
  heartbeats.
- Browser: `machines-search.e2e.mjs` covers the fleet search, honest count, empty
  recovery and phone targets. `navigation.e2e.mjs` checks a skill result and its
  `/skills?skill=` address. `first-skills.e2e.mjs` checks the draft's saved-intent
  line and the shelf link, and `phone-zoom.e2e.mjs` covers the renamed Jump field.
- Regression: the directory specs, `control.e2e.mjs`, `skills-sync-simple.e2e.mjs`,
  `npm run check` and the production build.
