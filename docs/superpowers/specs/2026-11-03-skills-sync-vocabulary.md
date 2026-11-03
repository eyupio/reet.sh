# Skills and sync: one vocabulary, one Sync

Date: 3 November 2026. Status: done. Builds on
`docs/research/skills-sync-ux-review.md` and `docs/skills-machines-ux.md`.

## The glossary

| Term | Means | Do not use |
| --- | --- | --- |
| **Skill** | one skill record, public or private | shelf skill, hosted skill, bundle |
| **Shelf** | the skills saved in reet.sh | my skills, library |
| **Machine** | the computer running an agent | environment, host, target, destination |
| **Install location** | one agent's skills folder on a machine | scope, agent group, target |
| **Sync plan** | the skills one machine's install location should have | desired set, saved set, selection |
| **Sync** | the verb: compare, save a plan, preview, approve | apply as the name of the page |
| **Private draft** / **Published** | a skill's visibility | hosted, public/private skill |
| **Write** | authoring a skill in reet.sh | host (except the publishing username) |

The API keeps its own field names (`target`, `environment`, `desired`,
`scope`). Only user-facing copy changes.

## One Sync place

**Sync is a subtab of Machines, not a destination in More.** The Machines page
already heads the Sync page and marks its own tab current. The duplicate More
entry is removed. `/sync`, `/machines?view=compare` and the Skills
**Sync skills** button keep working.

## Writing flow

**The authoring page is called “Write a skill”.** Its header says what the page
is for, not only where the result is hosted. Choosing a publishing username and
the Publish controls stay on the page. Every save notice ends with the next
action: **Put it on a machine** links to Sync with the skill pre-selected.
**Skills you write** on the Skills page says that published skills also appear
on the shelf, so the same skill appearing twice is explained rather than
surprising.

## Rulings

- **Sync lives only under Machines.** *Cost if wrong:* reaching it takes one
  more press from More, and a pasted `/sync` link still lands correctly.
- **“Install location” replaces scope/environment/agent group.** *Cost if
  wrong:* people who know an agent's configuration folder may look for
  “scope”.
- **“Sync plan” replaces desired/saved set.** *Cost if wrong:* the phrase is
  longer, and API users will still see `desired` in the API.
- **“Write a skill” is the authoring page, not “Host skills”.** *Cost if
  wrong:* publishing is less prominent in the page title, although its
  controls remain.
- **A published skill stays in Skills you write with an “On your shelf” link.**
  *Cost if wrong:* the skill still appears in two places; merging the records
  is a larger lifecycle change and is deferred.
- **Keep “Source”, “List” and “Sync mode” as control labels.** *Cost if
  wrong:* these technical controls stay less plain-language than the rest.

## Deferred

- Merging a written skill and its shelf card into one record with one
  lifecycle.
- A Machines column on the authoring page itself; the Skills page already
  shows the saved intent and observed state for each written skill.

## Testing

- `sync-vocabulary.e2e.mjs` opens Advanced sync and fails if user-facing copy
  contains desired/saved set, scope, environment, target or destination; it
  also checks the new Sync plan and install-location labels, the single Sync
  entry under Machines, and the authoring title and next-step link.
- Updated `control.e2e.mjs`, `sync-remove.e2e.mjs`, `skills-sync-simple.e2e.mjs`
  and `navigation.e2e.mjs`.
