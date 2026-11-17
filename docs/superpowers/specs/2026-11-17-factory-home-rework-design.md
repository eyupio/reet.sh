# Factory and Home rework: one place for what is waiting, one place for its actions

Date: 17 November 2026. Status: awaiting the owner's approval.

Mocks the owner chose from, as rendered: `docs/images/ui/rework/` (`Main`, `ATabletList`, `ATabletDetail`,
`APhoneList`, `HomeWide`, `HomeBoard`, `HomeTablet`). They are pictures of the layout, not of final styling: the
build uses the app's own tokens, fonts and components.

## Why

The owner's words: with several items on the Factory page or Home it is "overwhelming", and "the action buttons and
selections are not in a single logical location". Today a Factory outcome card has controls in five places (the
card head, the decisions block, the facts row, each pipeline's facts, and each pipeline's button row), every
pipeline is fully expanded, and Home shows the same work in two lists ("Factory pipelines", which has no buttons,
and "Needs you").

## Goals

1. On arriving, the person sees what is waiting for them first, and can deal with most of it without opening anything.
2. Every action for a pipeline is in one place, with the recommended one first.
3. Every setting for an outcome is in one place.
4. More detail is available in steps, never all at once.
5. It works on a tablet and a phone as well as a wide screen. The owner uses an Android tablet.

## Not goals

- No change to what the Factory does: no new controller behaviour, API, MCP tool or stored field, except what the
  "Open questions for the builder" section names as needed.
- No change to the Give Gaffer an outcome dialog, Add pipelines dialog, session screen, Sessions page or Desks.
- No "classic view". Each page is replaced outright (the owner's choice); going back is reverting the PR.

## What would make it a failure

- Detail that appears below a long list, so the person scrolls to find what they just opened.
- A pipeline shown without the names of its stages.
- The same action offered in two places that can disagree, or an action only reachable by opening a session.
- Anything smaller than the 14 px type floor, a literal colour, or a touch target under 44 px.

## Approaches considered

- **A. Queue and detail (chosen).** A list of pipelines grouped by what they need, and one detail view per
  pipeline with one action bar. The owner picked it, then asked for the stage pipeline in today's style, layered
  detail, collapsible groups and the Projects panel kept.
- **B. One table.** Every pipeline a row with its next step in a fixed column. Dense and scannable, but it needs
  horizontal scrolling on a tablet and has no natural home for detail.
- **C. Board.** Columns by state with a side panel. Familiar, but four columns do not fit a tablet and the columns
  mostly repeat the state chip.

## The shared pieces

Both pages are built from the same four pieces, so they cannot drift apart.

### 1. A waiting item

One pipeline or session that needs the person. It shows, in this order: a kind chip (Decision, Approval, Ready to
merge, Agent stopped, Question), the title, what it belongs to (the outcome's objective, or the session's project),
its stage pipeline when it is a Factory pipeline, one sentence saying what is waiting (Gaffer's recommendation and
reason when there is one), and its actions.

Actions on the item: one filled button, then at most one other direct answer (Deny beside Allow), then
**Details**. Everything else is in the detail view. This design adds no new choice: the choices, their order and
which is first come from the code that decides them today (`needsYou` and `waitingDecisions` in `gafferOutcome.ts`,
`sessionEndedChoices` for an ended session). **The filled button is whichever choice that code marks primary.**
With Gaffer's advice that is the advised choice; without it, it is what the page offers first today (for example
"Have Gaffer look again").

| Kind | Filled button | Also on the item |
|---|---|---|
| Decision (Gaffer asked the person) | the primary choice | Details |
| Approval (the agent holds a permission) | Allow | Deny, Details |
| Ready to merge | Approve merge | Details |
| Agent stopped, review failed, session ended, pipeline failed | the primary choice (Try again, and so on) | Details |
| Question in a session (Home only) | Answer in chat | nothing else |
| Session failed or lost (Home only) | Open | nothing else |
| Session finished in a worktree, to review (Home only) | Open | Mark seen, as today |

Allow and Deny use the existing `Approval` component. The Factory page already polls the sessions list, whose rows
carry the pending permission, so no new read is needed. Where a pipeline's session has no pending permission on
record, the item keeps today's "Open the session to answer".

### 2. The stage pipeline

Three sizes of one component, all fed by the existing `track()`:

- **Labelled** (detail view; item cards at tablet width and wider cards): circles joined by a line across the full
  width, a tick in done stages, the stage in hand filled, later stages numbered, each stage's name beneath. This is
  the look of today's Factory pipeline. A split stage's parts are separate circles named by part.
- **Circles with a caption** (phone): the same circles without names, and one line beneath: the stage in hand, what
  is done and what is next ("Code in hand · Plan, Design done · next Review").
- **Dots with a caption** (the list beside an open detail on a wide screen; Home's rows on a wide screen): small
  dots and "Code · 3 of 8 · next Review".

A stage in hand is drawn in the accent colour when the pipeline is waiting for the person, and in the "working"
colour when it is running by itself. The two must not look the same.

### 3. Collapsible sections

Groups open and close with a header: an arrow, the name, the count, and on the right a one-line summary that still
says what is inside when closed (for Working, the pipeline names). The first time, only **Waiting for you** is
open. After that each page remembers which sections the person left open, per browser (the owner's choice).

### 4. The Projects panel

On a wide screen it is kept as it is today (`ProjectRailHost`): a left column with its own add, group and collapse
controls. Each project's line also says how many of its pipelines are waiting and working, and choosing a project
narrows everything on the page.

On a narrow screen neither page has a project list today (it lives on the Sessions screen). This design adds only
a **Projects** button that opens a short picker: All projects and each project with its counts, to narrow the
page. Adding, grouping and editing projects stay where they are.

## The Factory page

Sections, in order: **Waiting for you**, **Working**, **Finished or stopped**. A pipeline is in exactly one:

- **Waiting for you:** everything `waitingDecisions` returns, plus a pipeline that failed or whose session was
  lost, since only the person can set it going again.
- **Working:** a pipeline that is starting, being followed or moving between stages, and one waiting its turn to
  start (shown as "Waiting its turn").
- **Finished or stopped:** done, and those the person cancelled or stopped following. Nothing here needs anyone.
 The page head keeps
the title, one line of counts and **Give Gaffer an outcome**. Archived outcomes stay behind the existing control.

**Wide screen** (`Main`): Projects panel, then the list, then the detail of the selected pipeline, side by side. The
list scrolls by itself; the detail stays in view. The first waiting item is selected on arrival; with nothing
waiting, the first working one.

**Tablet and phone** (`ATabletList`, `ATabletDetail`, `APhoneList`): the list is the page. **Details** opens the
pipeline as its own screen, with a way back to the list and previous / next controls that step through the waiting
items in order. It is never rendered beneath the list.

### The detail view

Top to bottom:

1. The outcome's objective and "pipeline n of m", the pipeline's title, its state chip, and **Open session**.
2. The labelled stage pipeline. Each stage can be selected; the stage in hand is selected to begin with.
3. What is waiting, with Gaffer's recommendation and reason. Absent when nothing is waiting.
4. Facts for the selected stage: agent and effort, plan left, time in the stage, Gaffer's reviews, and Gaffer's
   latest review or the stage's handover. Selecting another stage shows that stage's facts, which replaces today's
   "Who did each stage" table. A stage not yet started says which agent will do it.
5. Rows that are closed by default and show their headline value without being opened: **Usage and budget**,
   **What Gaffer decided**, **History**, **Settings for this outcome**.
6. One action bar, fixed to the bottom of the detail: the primary choice first and filled, then the stage's other
   choices, then the buttons a pipeline has today whatever its state (Tell the agent…, Send back…, Move to next
   stage, Review goal). Nothing else on the page carries a pipeline action except the item's own buttons, which
   are the same actions.

**Settings for this outcome** holds every per-outcome control and nothing else does: who decides (You / Gaffer),
approvals (Ask me / YOLO, with today's confirmation), and merge for this pipeline (wait for me / by itself, with
today's YOLO confirmation). Reports (on request / by itself) is shown there as a fact, not a control: it is set
when the outcome is created and nothing changes it afterwards today.

Everything else an outcome has today keeps a home, so nothing the Factory does is lost:

- **Gaffer's report** is one more closed row, with its headline ("2 pipelines proposed", or when it was written).
  Open, it shows the report, the proposed pipelines to tick and **Add n pipelines**, and **Ask Gaffer for a report**.
- **Usage and budget** also holds the outcome's totals and the pipeline's **Goal and limits**.
- Outcome-level actions (Add pipelines, Stop starting sessions, Stop following, Archive, Delete) sit in one menu
  on the detail head. An archived outcome's pipelines show **Restore** there and nothing else.
- **Other sessions Gaffer follows** (today's list under the outcomes) becomes a fourth closed section of the list.

## Home

Kept: the Projects panel; the two views, **Continue working** and **Planning board**, as two tabs with their
remembered choice; **Your machines**; the plan meters; New session and Give Gaffer an outcome.

Changed (`HomeWide`, `HomeBoard`, `HomeTablet`):

- **One "Waiting for you" section** replaces "Factory pipelines" and "Needs you". It mixes Factory items and session
  items, oldest first, each as a waiting item with its actions. A Factory item's **Details** goes to that pipeline's
  detail on the Factory page.
- **A session that has only finished is not in it** (the owner's choice). It shows in Continue working and on the
  board, marked "not seen yet". A session that failed or lost its connection, and one that finished in a worktree
  with changes to review, still are: each asks something of the person.
- **A pipeline's session appears once**, as its Factory item. The session card for the same session is not shown.
- **The bell and the page title count what is in this section**, so Factory items join the count and plainly
  finished sessions leave it.
- **"Factory, working without you"** is a closed section listing the pipelines that need nothing.
- One line of counts in the head replaces the summary strip that repeated them; the plan meters stay beside it.
- The view tabs and their content follow, then Your machines as a closed section.

On a wide screen a waiting item is one row (chip, text, dots with caption in a fixed column, actions in a fixed
right-aligned column so rows line up). On a tablet and phone it is a card as on the Factory page.

## Constraints

- Project rules: TDD, the 14 px type floor, no literal colours, 44 px touch targets on touch screens, real
  buttons and links, `docs/ui-verification.md` and captures for every changed surface, CHANGELOG, handoff notes.
- Browser suites that cover these pages today (`gaffer-factory`, `factory-decision`, `gaffer-factory-session`,
  `home`, `home-followups`, the phone suites) must be updated to the new structure, not deleted. An assertion that
  no longer applies is replaced by one for the new behaviour.
- Deep links that exist today keep working: Home's links to a pipeline, `/factory#…`, `?project=`, `?outcome=1`.

## Delivery

Two pull requests (the owner's choice), each replacing its page outright: the Factory page first, with the shared
pieces; then Home. Each is merged only when the owner asks.

## Left to the builder

- Component boundaries and file names, and how much of `Factory.svelte` and `Home.svelte` is kept.
- The exact breakpoint between "wide" and "narrow", starting from the pages' existing `NARROW` query.
- How the open pipeline is carried in the address so a reload and a shared link reopen it.
- Storage key names for the remembered sections.
- Empty, loading and failure states, following the pages' existing ones.

## Open question for the builder to confirm in code, not to guess

- **"Oldest first"** needs a time each item began waiting. Use what the records already carry (a goal's or
  session's last change); if nothing suitable exists for a kind, keep today's order for that kind and say so.

## Calls made in writing this up, for the owner to check

These were not discussed; each is the smallest reading of what was.

1. The filled button is always today's primary choice, so without Gaffer's advice it may be "Have Gaffer look
   again" and not "Move to next stage".
2. Failed pipelines count as waiting for you; cancelled and stopped ones sit with the finished.
3. Reports is shown, not switchable, because nothing can change it after an outcome starts.
4. Gaffer's report, the totals, Goal and limits and the other followed sessions each get the home described above.
5. On a tablet or phone the Projects button only narrows the page; it does not manage projects.
6. On Home, failed sessions and worktrees to review stay in Waiting for you; only plainly finished ones leave.
7. The bell follows the Waiting for you section.
