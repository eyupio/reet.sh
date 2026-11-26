## PATHWAYS-2: Pathways get a list, a canvas editor and a place in the start dialogs (25 November 2026)

**What:** a person can now build and keep Factory pathways. Factory › Pathways (`/factory/pathways`) lists them, with an
on and off switch, Duplicate and Delete. The editor is a canvas where stages, models, skills and prompts are dragged on
to a line and stages are dragged to reorder; every drag has a button or keyboard way (Alt+Arrow moves a stage, Delete
removes it). The order rules (a built-in stage once, Merge last, Babysit and Merge after a PR stage, up to 12 stages,
up to 2 skills a stage) are enforced as the server does: a drop that would break one is not offered, and a button
refuses with a sentence. The editor asks before leaving with unsaved changes, says so when someone else saved first
(`pathway_changed`) and keeps the person's edits, and flags a skill gone from the shelf on a saved stage. The New
Factory outcome and Add pipelines dialogs offer a pathway choice (only enabled pathways; one that cannot start is shown
with why). Both browser suites, `pathways` and `pathways-choice`, are in `web/tests/ci-gaffer-settings.txt`, so the
gaffer-settings job runs them. User docs: a Pathways section in `docs/factory.md`.

**Why:** the owner asked for pathways to be built and kept on a visual canvas. PATHWAYS-1 gave the engine.

**Checked:** each task was built test first and reviewed by a second reader. Both browser suites were run at 1280 and
390 px, in light and dark. Touch dragging was tested with simulated touch in Chromium only, not on a real phone or
tablet. Screenshots were compared with the owner's approved mockups. `npm run check` in `web/` passes. `pathways`
takes about 76 seconds and `pathways-choice` about 17 seconds on their own in the cloud container.

**Not done:** Gaffer's own suggestion of a pathway and the MCP tools are the next pull request. Over MCP an agent still
cannot see the name of an earlier stage of the person's own. On a phone, dragging reorders stages only (adding and
applying are by tapping). iOS Safari touch is not verified.
