## SKILL-BUDDY-FIT-1: chat skill suggestions follow the draft, and a removed stage skill can be chosen again (27 November 2026)

A review of **Skills for this task** found four faults. The owner chose to fix all four.

- **Every chat got the same suggestions.** With no Factory stage, `Recommend` (`internal/skillbuddy/recommend.go`)
  defaulted to `code`, which gave `builtin:code` 12 points and added "implementation coding fix" to the query. So
  "Focused implementation" led every chat, and any skill with "code" in its name scored. A defaulted stage now decides
  which built-ins may be offered but adds no score or words. Stage words are read from `stageForms`, so "tests",
  "testing" and "reviewing" name their stage. `TestAChatDraftIsMatchedOnItsOwnWords` fails on the old matcher.
- **Remove hid a stage skill for good.** Remove saves `dismissed`, a later suggestion never overwrites it, and the
  panel filtered `dismissed` out. The filter is gone; nothing else dismisses. `skill-buddy.e2e.mjs` starts Unslop as
  dismissed and selects it.
- **Long library skills could not be added in chat.** The server refuses guidance over 1,500 bytes for a draft
  (`chatGuidanceBytes`). Reviewing one in a session now returns `tooLong`, and the panel says so instead of offering
  **Add to message** (which answered "Shorten it" even with an empty message).
- **Storage errors reached the person.** `buddyLoadProblem` maps a load failure: a refusal keeps its text (400), a
  missing library skill is 404 with plain words, anything else is a generic 500. The MCP `reet_load_skill` uses it too.

Checked: `go test ./internal/skillbuddy ./internal/api ./internal/store`; svelte-check; browser suites skill-buddy, chat,
factory-delivery, gaffer-outcome, gaffer-factory, session-workspace, terminal-workspace.

Not changed: library skills are still suggested in chat before their size is known (reading each revision per
keystroke would cost more than the honest message at review). Each pause in typing still reads outcomes, library and
community skills and writes receipts; fine for now, the first place to look if it slows.
