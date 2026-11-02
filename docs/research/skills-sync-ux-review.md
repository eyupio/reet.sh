# Skills and sync: UX review

**Date:** 2 November 2026. **Scope:** the signed-in journey from "I have a skill" to "it is on my machines", and back: creating or adding a skill (Skills page, Hosting page), finding out where a skill is, and controlling what each machine gets (Sync page, Machines page). **Method:** read the shipped docs and code, then rendered the built app against fixture data at 1440 px and 390 px (two enrolled machines, two shelf skills, one published and one draft "skill you create").

The prompt for this review was two complaints: a newly created skill cannot be synced to a machine, and the Skills page gives no simple way to look a skill up and see where it is synced. Both are confirmed. Both are symptoms of the same structural problem, so the findings start there.

## Overall impression

The individual surfaces are well made. Shelf cards carry live per-machine badges with an **Add now** action, the Sync page has a genuinely simple two-step setup, and machine cards lead with one sync state and one action. The trouble is between the surfaces. A person is asked to hold five different models of "my skills" in their head, nothing on the Skills page says what a machine *should* have (only what a scan *saw*), and the skill the person wrote themself is the one kind of skill the sync path cannot reach without first making it public.

## 1. The structural problem: three skill stores, two sync systems, no bridge

What the product calls a skill is one of three different records, each with its own page, API and vocabulary.

| Record | Where it lives | What the UI calls it | Can sync reach it? |
| --- | --- | --- | --- |
| Shelf entry (`skills`, `POST /api/skills`) | Skills page cards | "shelf", "On the shelf / To test / In use" | Yes, if it has a linked source (GitHub or a reet.sh public URL). Local or unlinked entries are refused by `SaveDesiredFor`. |
| Hosted skill (`/api/v2/hosting/skills`) | Hosting page editor, "Skills you create" block on the Skills page | "your skills", "hosted skills", "draft", "published" | **Only once published.** `SyncHostedFor` (`internal/store/store_sync_control.go:814`) skips every unpublished skill. A draft is not on the shelf and not a sync candidate. |
| Private bundle (`/api/v2/skill-bundles`) | "Private skills" dialog on the Machines page | "library", "private skills", "revision", "pin" | **Not yet.** Pins, checks and ownership receipts exist; the reviewed executor that writes files does not (`docs/skill-content-review-ui.md`, `docs/private-skill-revisions.md`). |

On top of that sit two sync mechanisms that do not know about each other: the desired-set sync (`sync_targets`, driven from the Sync page and **Add now**) and the private-content pipeline (pins and receipts, driven from the Machines page). The Sync page never mentions pins. The Private skills dialog never mentions desired sets.

### Consequences the user sees

1. **Create a skill, save it, and it is a dead end.** The save notice reads "Draft saved. Its page and install files are private." There is no next step, no machine, no sync. The "Skills you create" row offers Edit, a Public switch, Publish & share, Copy share link and Copy prompt. Nothing about machines.
2. **The only route from a new skill to a machine is to publish it publicly.** Publishing puts it on the shelf under To test (`internal/api/hosted.go:125-132`), and only then do machine badges and **Add now** appear. The Public switch's help text is "Anyone with the link can use this skill", so a person who wants a private skill on their own laptop is being asked to make it public to get it there. Nothing explains that this is the trade-off, and nothing says "private skills reach machines through the Private skills dialog instead", which would in any case be only half true today.
3. **Two things are called your private skills and they are not the same thing.** A draft in "Skills you create" is not a bundle in the library the Private skills dialog pins. A person who writes a skill on the Hosting page and then opens Private skills on a machine finds it is not there.
4. **The Hosting page does not know about machines, and the Machines page does not know about hosting.** The Hosting sidebar shows Published / Draft; machine cards show counts; neither shows the other.

This is the root of both complaints. Every later finding is easier once a hosted skill (draft or published) and a private bundle are one record with one lifecycle, and once the Sync page treats "a skill you wrote" as a first-class source without a public URL as the price of admission.

## 2. Findability: "where is this skill?"

| Finding | Severity | Recommendation |
| --- | --- | --- |
| Per-skill badges show what a scan **observed** (Installed, Missing, Not confirmed), never what is **intended**. A skill that is in a saved desired set but not yet applied looks the same as one nobody asked for. No code path joins `sync_targets.desired` back to a skill. | 🔴 Critical | **Done in this PR:** a per-skill sync line computed from targets ("Set to sync to dev-laptop", "Pending on build-box · synced to dev-laptop", "Not set to sync" with a Sync to machines… link) under the badges (`web/src/lib/skillSync.ts`). |
| No way to filter or search the shelf by sync state. The filters are tags, machine, agent group, "Only on one machine" and "Copy missing installs"; all read observed inventory. | 🔴 Critical | **Done in this PR:** filter chips **Not on any machine**, **Missing somewhere**, **Not set to sync** and **Sync pending** in the Environments row. |
| "Skills you create" rows have no machine information at all, draft or published. The published one (`reet-standup`) only gains badges further down the page, in its separate shelf card, so the same skill appears twice with different controls and no link between them. | 🔴 Critical | Give the created-skill row the same badge strip, or merge created skills into the shelf as a stage/filter ("Mine") instead of a separate block. One card per skill. |
| Machine cards show counts only ("1 unique skill · 1 agent scope"). The names are not listed; to find whether `code-review` is on `build-box` the person must go back to Skills and scan badges. | 🟡 Moderate | On expanding "Agents on this machine", list the skills per scope with Installed / Pending / Extra state and a per-row Remove or Add. Add a search box on the Machines page that filters machines by skill name. |
| Jump search covers sessions, projects and machines, not skills. | 🟡 Moderate | Add skills to Jump search with their sync summary as the detail line. |
| Only three badges show while collapsed; a fleet of six machines hides half the answer behind Expand. | 🟢 Minor | Collapse badges into one summary pill ("On 4 of 6 · 2 missing") that expands on press. |

## 3. Controls: putting a skill on a machine

| Finding | Severity | Recommendation |
| --- | --- | --- |
| **Add now** is only offered where a machine reports the skill as *missing*. A machine that has never scanned, or whose scan is stale, shows Not confirmed and no button. The newest machine, the one people most want to fill, is the one with no control. | 🟡 Moderate | Offer **Add now** on Not confirmed as well, with the honest caveat already in `installSkillOnMachine` ("needs a recent scan first; rescan and add"), and let it queue the scan. |
| From the Machines page a single skill cannot be added or removed. The ⋯ menu only links to `/sync?machine=`. | 🟡 Moderate | Add "Add a skill…" to the machine card menu that opens a picker (same data as Choose individual skills) and reuses `installSkillOnMachine`. |
| The Sync page summary reads "3 skills · 0 machines" with **Sync now** disabled, while the dev-laptop card shows Keep synced on and "1 skill to install". Two different selections (the checkbox and the switch) sit in the same card and are not explained as different. | 🟡 Moderate | Separate "this run" from "the standing rule" visually: checkbox row for the run, Keep synced as a row in the card footer with "applies to whatever you save here". Disable **Sync now** with a reason ("Choose at least one machine"). |
| "All my skills" means the shelf. Drafts, unpublished skills and private bundles are not "my skills" here, and the copy does not say so. | 🟡 Moderate | Rename to "Everything on my shelf" and add a one-line note under it: "Skills you create join the shelf when published; private drafts are not synced yet." Until the bridge exists, honesty beats a promise. |
| The published-skill source is hidden in Advanced sync as a Source dropdown option, "My published reet.sh skills", next to "Reet.sh shelf & lists" and every machine scope. A person following the simple path never sees it. | 🟢 Minor | Fold it into the simple path: when the person has created skills, show them as a group in Choose individual skills. |
| Removal is a different mode ("Exact set") in a different place (Advanced sync). Nothing on a skill card or machine card offers "take this off that machine". | 🟡 Moderate | Add "Remove from {machine}" beside Installed badges, routed through the existing review-before-removal flow. |
| The Private skills dialog is the only control for private content and it can only pin, check, adopt and release. It cannot apply. The button does not say so until the dialog is open. | 🟡 Moderate | Until the executor ships, label it "Pin private skills (preview only)" and put "reet can't write private skills to machines yet" at the top of the dialog, not the bottom. |

## 4. Understanding: vocabulary and navigation

**The same idea has too many names.** This was flagged in `docs/skills-machines-ux.md` ("Still worth doing next") and is still open. Collected from the rendered pages and copy:

- *The thing being synced:* skill, shelf skill, hosted skill, published skill, private skill, draft, bundle, library revision, pin.
- *The destination:* machine, environment, agent scope, agent group, install location, target, scope, folder, host.
- *The selection:* desired set, saved set, sync set, saved skills, selection, reference snapshot, list.
- *The action:* Sync now, Set up sync, Choose skills & scopes, Add now, Install on {host}, Copy install, Preview, Apply, Pin.

Recommendation: adopt one glossary and apply it everywhere (copy, docs, API field names can lag). Suggested: **skill** (one record, with a *visibility* of private or public), **machine**, **location** (an agent's folder on a machine), **sync plan** (what a machine should have), **Sync** (the verb). Retire "desired set", "scope", "environment" and "target" from user-facing copy.

**Navigation.** `/sync` highlights **More** in the top navigation while the page eyebrow says MACHINES and the subtab strip (Inventory & connection · Sync · Software) belongs to Machines. The Skills page header button "Sync skills" and the skill menu's "Sync to machines…" both land there. Make Sync a subtab under the Machines tab consistently, or give it its own tab; one or the other.

**Create a skill leaves the Skills page.** "Create a skill" navigates to `/hosting`, a page headed "Host your own skills" with a sidebar called "Your skills", while the Skills page calls the same list "Skills you create". The Hosting page's framing (hosting, publishing, a username, a name prefix) is about sharing, not about using the skill on your own machines, which is what most people create a skill for. Recommendation: keep authoring on `/hosting` if the editor needs the room, but head it "Write a skill", open it in a dialog or side panel from Skills when possible, and make the first save say what comes next: "Saved. Put it on a machine →" with the same picker as **Add now**.

**Post-create feedback.** Three notices need a next step:

| Today | Suggested |
| --- | --- |
| Draft saved. Its page and install files are private. | Draft saved. Only you can see it. Next: put it on a machine, or publish to share. |
| Skill published. It is also on your shelf so you can test it. | Published. It's on your shelf under To test. Next: sync it to a machine. |
| Reet, added {name} to the shelf. | Reet, added {name} to the shelf. Not on any machine yet — sync it? |

## 5. What works well

- **Add now on a Missing badge** is the right shape: intent in one press, reviewed apply behind it, honest refusal when the machine cannot take it.
- **The simplified Sync page** (two choices, then machines) is clear, and the Keep synced copy ("Saved skills stay synced, even when this page is closed") is exactly the reassurance people need.
- **Machine cards lead with one state and one action.** "1 skill to install · Ready to apply · never synced" says more than most dashboards.
- **Honesty about unknowns.** Not confirmed, Rescan needed and "Nowt on screen says more than the machine has told us" are the product's best habit. Keep it when adding intent state: "Set to sync" must never read as "installed".
- **Accessibility basics are in place**: native switches with labels, menu roles, Escape handling, live regions, phone layouts without horizontal scroll.

## 6. Accessibility notes from the rendered pages

- Colour contrast on the amber Missing / Add now badges is adequate; the green Installed badge text is small and light on a tinted background and should be checked at AA for 12 px.
- The ⋯ menu trigger is a 32 px chip; on the phone layout the badge **Add now** buttons are roughly 28 px tall, under the 44 px touch guidance.
- The Public/Private switch on created-skill rows has a correct `aria-label`, but its state text ("Anyone with the link…") is not associated with it; use `aria-describedby`.

## 7. Priority recommendations

1. **One skill record, one lifecycle, one card.** Merge hosted skills and private bundles into a single "skill you create" with a visibility flag; show it once on the Skills page with the same machine badges as any shelf skill. This removes the two-"private skills" confusion and is the precondition for everything else.
2. **A sync source for your own skills that does not require publishing.** Serve a created skill (draft or published) to the agent from the controller under an account-scoped URL so `installSkillOnMachine` and the Sync page can treat it like any linked source. Until the private-content executor ships this is the pragmatic bridge, and it is the fix for "I made a skill and can't sync it".
3. **Intent on the skill card.** Done in this PR: "set to sync to…" from targets next to the observed badges, with four sync-state filters. This is the fix for "I can't look a skill up and see where it's synced". Still to do: the same line on "Skills you create" rows and in Jump search.
4. **Close the loop after create.** Every save and add notice ends with the next action, and the Hosting page gets a "Machines" column (Installed / Pending / Not set) per skill.
5. **One vocabulary and one place for Sync in the navigation.** Cheap, and it makes the other four land.

## Appendix: journey as it stands

```
Create a skill  ──►  /hosting  ──►  Save draft
                                      │  "Draft saved. Its page and install files are private."
                                      ▼
                                  (dead end: no machine action, not on shelf, not a sync candidate)
                                      │
                       Publish publicly ◄── the only way forward
                                      ▼
                           Shelf card appears under To test
                                      │
                   badges (observed only) ──► Add now (only where a scan says Missing)
                                      │
                      /sync  "All my skills" or "Choose individual skills" ──► machines ──► Sync now
                                      │
                         Keep synced (per machine, independent of the run selection)
```

Private bundles and the Private skills dialog run beside this and do not join it.

## Verification

Rendered with the production build served by `vite preview` and the UI contract fixtures in `web/tests/helpers/workspace.mjs`; no server, machine or account was changed. Screenshots at 1440 and 390 px of `/skills`, `/skills` expanded, `/sync`, `/machines` and `/hosting?new=1` are in the review session's scratchpad and were used for the visual findings above. Code references are to the working tree at the review date.
