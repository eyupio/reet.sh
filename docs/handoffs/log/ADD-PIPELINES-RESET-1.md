## ADD-PIPELINES-RESET-1: a crew tier in the Add pipelines dialog can be put back (25 November 2026)

STUCK-STAGE-1 gave a new outcome's crew a link that undoes a change to one tier and left the Add pipelines dialog
without one. The owner asked for it there too.

What changed. In `AddPipelines.svelte`, a tier under **Choose who does what** shows **Back to your saved crew** when the
saved crew names an agent for it that this machine can run, else **Back to the pipeline's agent**, and pressing it sets
that tier to `savedRole(title, tier)`, which is what the dialog starts it on. The link is shown when the tier's agent,
model or effort differs from that start (`moved`), not from a flag set on edit: setting the pipeline's own agent gives
it to every tier, so a tier the saved crew names is then away from its start and can be put back, while a tier with
nothing saved is where it started and shows nothing. An effort this machine does not report is dropped by the dialog
itself and is not counted as the person's change (`kept`). Backups are left as they are, as in the outcome dialog.
`docs/gaffer-ai.md` says so.

Checked. `gaffer-factory.e2e.mjs` (light and dark, 1440 and 390): the link appears on Light after the pipeline's agent
is changed, not on Heavy or Standard; pressing it puts Light on the saved crew's agent and removes the link; a changed
Standard effort goes back to the pipeline's own; what is sent is unchanged. The check failed before the change because
the button did not exist. Web unit tests and `svelte-check` pass.

Not checked. A saved crew whose effort the machine does not report was not driven through the link in a browser; the
suite's existing case for unreported efforts passes unchanged.
