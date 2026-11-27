## HANDOVER-CLIP-1: an over-long Gaffer handover is clipped, not refused (27 November 2026)

What. `goalQuestion` (`internal/gaffer/worker_tools.go`) refused a `hand_over` summary over 4,000 bytes, so the stage
review failed ("The handover must be one summary of up to 4,000 UTF-8 bytes"). A failed review is not repeated, and the
next worker progress started another review, so a Design stage flipped between issue and working. A `hand_over` summary
is now cut at a character boundary to 4,000 bytes ending in "…". `ask_user` keeps its 2,000-byte refusal.

Why. The model is told the limit and still overshoots on long designs; clipping keeps the first items of the plan.

Checked. `TestAHandoverMayCarryAWholePlan` has an over-long multi-byte case.

Not done. The tail of a clipped plan is lost; the Gaffer's report does not say it was clipped. The prompt still asks the
model to keep within 4,000 bytes. Not run: the browser suites or the whole Go suite.
