## PATHWAYS-3: Gaffer and AI clients can read, suggest and start pathways (26 November 2026)

**What:** pathways are now part of Gaffer and of the MCP tools. A new read tool, `reet_list_pathways`, lists the pathways
that are on, by id, name, description, stage names and any problems that would refuse a start. `reet_start_outcome` and
`reet_add_pipelines` take an optional `pathway` (an id or an exact name in any letter case); it is refused with one
sentence, and nothing started, beside stages, tiers or parts, and an unknown or switched-off name is refused with the
names that are on. `stage` on `reet_move_pipeline_on` and `reet_send_pipeline_back` is the stage's name as the outcome
lists it, and one the pipeline does not have is refused with the names it does. The outcome view gives each stage a
`name` and each pipeline its `pathway`. Gaffer's context names a pathway stage and its pathway, and skill
recommendations for one of a person's own stages use its name and prompt. Gaffer's planning is given each enabled
pathway's name, description and stage names (at most 20, never prompts, models or skills) and may suggest one; the
suggestion can only select among the person's own enabled pathways, and anything else is dropped. The New Factory
outcome dialog shows the suggestion already selected with its reason and never overrides a choice the person made. The
two start dialogs work out a pathway's agents as the server does: a saved crew agent that is not on the machine holds the
start with a sentence. With a pathway, both dialogs count each stage's agent and its tier's backups for folder trust;
under standard stages the new-outcome dialog counts every agent the start will send, backups included, and the
outcome's own agent, for folder trust and YOLO. Small fixes: the chosen pathway row is always in view,
"Edit pathway" is no longer part of an option's name, and the drag hint has three fixed places inside the window, clear of
the pointer and the drop target, with the palette leaving room for the bottom tab bar. The tool count is now 50. User
docs: "With Gaffer and AI clients" in `docs/factory.md` and the Factory tools in `docs/mcp.md`.

**Why:** the owner asked for each pathway to be part of the Factory and of Gaffer. PATHWAYS-1 gave the engine and
PATHWAYS-2 the pages and the choice in the dialogs. Gaffer still never starts work on a pathway except through the start
tools, under the person's existing off, ask and allow settings.

**Checked:** each task was built test first and reviewed by a second reader. Go tests for `internal/api`,
`internal/gaffer` and `internal/skillbuddy`, the MCP wrapper's tests (`npm test` in `mcp/`, which also fails on a stale
`mcp_tools.json`), the two pathways browser suites and the six existing dialog suites were run. The planning request for
an account with no pathways was compared byte for byte with one captured before the change. A marker prompt planted in
tests shows that no pathway prompt reaches a tool result, Gaffer's context or the planning request.
A whole-branch review was made and its findings fixed: the sharing page now says stage and pathway names are part of
status; the new-outcome dialog counts the outcome's own agent and the crew's backups for folder trust, for pathways and
for standard stages; a retried start no longer re-selects a dismissed skill or is refused because its pathway was
switched off since; an MCP pathway id is matched before any name; and planning goes on without pathways when they
cannot be read.

**Not done:** skill recommendations for one of a person's own stages still treat it as coding unless its words say
otherwise. `reet_recommend_skills` does not accept a pathway's own stage. Gaffer cannot create or edit pathways. Touch
dragging is still tested with simulated touch in Chromium only, not on a real phone or tablet. If the saved crew
cannot be read, the dialogs fall back to the outcome's agent and the server has the last word. A report's proposed
pipelines from a pathway outcome use built-in stages and no pathway. Under standard stages the Add pipelines dialog still does not
count a crew's backups for an outcome approved without folder trust, so the server may refuse; a follow-up.
