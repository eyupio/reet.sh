---
name: reet-ui-refresh
description: Refresh an existing web application's UI to one shared look, page by page, as stacked pull requests driven to green. Use when asked to restyle, modernise or unify any part of a product's UI (an overview or dashboard, list or table pages, detail pages, settings and forms, setup flows, a band, a sidebar) or to apply a design mock pack to an existing UI, in any repository and stack. Reads the repository's own guidance first, groups the pages into families that share one grammar, designs with the user's decisions, builds test-first, reviews each branch fresh, photographs every page, and babysits CI and merges.
---

# UI refresh

Bring an existing application's UI to one shared look without lowering the
repository's own standards. The skill is a way of working, not a design: the
design comes from the repository's mock pack or spec and from the decisions
the user makes along the way.

## Before anything else

1. Read the repository's agent guidance (`CLAUDE.md`, `AGENTS.md`, scoped
   copies beside the code), its contributing notes, its UI or design
   guidelines, its component inventory and its dependency policy. They
   outrank this skill: follow their voice, spelling, token rules, test
   conventions, commit style and layout.
2. Learn how the project is built, run locally and tested: unit tests, browser
   tests, lint, type check, documentation tests, screenshot and gallery
   tooling, and how to run one test alone. Find the fixture or demo data the
   browser tests use and how many test runs may share a port at once.
3. Find the design authority. A mock pack or spec is binding; say where it and
   the shipped product disagree and ask the user which wins, because the
   product has usually made decisions since the mock was drawn. With none,
   propose a short spec in chat before building.
4. Survey the UI and group its pages into families that share one anatomy:
   the overview or dashboard (cards and figures), the list pages (a table of
   records with filters), the detail pages (one record and its panels), the
   settings pages (forms), the setup flows (wizards), the shell (band,
   header, sidebar, navigation). Announce the families, the order you
   propose to take them in, and the kit each would share.

## Decisions

Use the brainstorming skill for the design and ask the user each decision one
at a time with a clickable question, your recommendation first. Do not stall
on the rest: rule on them yourself, and record every ruling as a dated note in
the spec and in your final message, with what it costs if wrong. Typical
decisions: which families and in which order; where the mock and the product
disagree, which wins; whether context blocks fold, move or go; whether default
columns or fields are trimmed to the mock; how an existing control whose shape
the mock does not have (a composite filter, a view switch, a wizard step) is
carried across.

## One contract per family

For each family, write once in the spec what every page in it does, and hold
every page to it. The contract names the shared card or frame, where the
page's own controls go, how the primary filter or navigation is drawn, what
folds and what stays, what is trimmed by default and the rule for exceptions
(a column or field that is the only place the page shows a feature is not
trimmed), what the eyebrow or count is made of (figures the page already
has), and what never changes (the URL's parameters, the saved layouts, the
pages that link in). Two rules hold in every family:

- Nothing is reachable from the UI that the API does not expose.
- A figure shown beside rows or fields is withheld whenever another filter
  or state would make it wrong for what is beneath it.

## Building a page

Use the writing-plans skill for the plan and the executing-plans skill with
test-driven development for the build. The kit a family shares rides the
first page that needs it, and that page is the proving page; the rest follow
one short pull request each.

1. Pure model first (what the chips or tiles are, what a count or sentence
   says, how a press is read) with unit tests watched to fail.
2. Browser tests for the behaviours on the built binary, watched to fail on
   the old build: every figure against the API, every control against the
   address bar, a pasted address with a value the product lacks, figures
   withheld under a narrowing state, trimmed columns or fields and their
   return, no horizontal overflow on a phone, keyboard and screen-reader
   names unchanged or re-pointed.
3. The page itself, then type check, lint, unit tests, the page's own spec on
   every browser project, then every spec that shares the kit, the layout, the
   tooltips or the page. Grep the test tree for every header, label and value
   the page used to show by default; a test reading something now hidden is
   re-pointed to bring it back first, never deleted.
4. Photograph the page at desktop and phone widths in every style and colour
   mode and look at the pictures before calling it done.
5. Update the user documentation, the guidelines, the component gallery and
   the captures with the repository's own tooling, in the same change. Alt
   text and prose must say what the page now does. Run the documentation
   tests.
6. Dispatch a fresh-context review of the branch (the requesting-code-review
   skill's reviewer brief, most capable model) with a Review Focus list of the
   inputs the spec is silent on. Fix Critical and Important findings with tests
   seen to fail first; defer Minor ones and list them in the pull request body.
   When a review doubts a behaviour, measure it in the browser before fixing
   or declining it.

## Pull requests and CI

- One branch per page, stacked on the previous one. Push, open a draft pull
  request with the repository's template (the problem, the approach, what you
  ran with numbers, the impact, the rulings and deferred minors), and
  subscribe to it.
- Drive each pull request yourself: read failing CI logs, reproduce locally
  when the file is yours, fix or re-run once, and comment only on a real
  failure with what it is and why it is or is not this change's.
- Keep one scheduled check-in so merges, retargets and conflicts are handled
  while the user is away. When a pull request in the stack merges, retarget the
  next one and merge the base in. When the base moves under a branch, merge it
  in with a merge commit, resolve, retake any captures both sides changed on
  the merged build, and push.
- Treat review-bot findings as claims: trace them in the code, measure them in
  the browser when they are about layout or pointer behaviour, then fix what is
  real or answer the thread with what you found, and resolve it either way.

## Hard constraints

- No new dependency without the justification the repository asks for; no raw
  colours or values that already have a token; no library the repository has
  chosen to do without.
- Branch names, commit messages and pull request text are public: nothing in
  them may name a private repository, customer, plan, price or anything the
  repository's guards forbid. Run any audit script the repository has before
  every push.
- Commit messages in the repository's style, imperative and plain; no model
  names in commits or pull requests.
- Never rewrite history on a branch someone else may have checked out; never
  push an empty commit to kick CI; never skip, disable or quarantine a test;
  never approve or merge.

## Finish

The final message carries, in full: every ruling made on the user's behalf
with its cost if wrong, every deferred minor, the pull requests open with their
state, and what remains in each family.