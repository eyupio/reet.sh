## PATHWAYS-AUDIT-1: The Factory pathways and templates audit is written (27 November 2026)

**What:** `docs/research/factory-pathways-and-templates-audit.md`. It traces pathway CRUD from the six browser routes
through the service and store, how a Factory outcome and an Agent session choose a pathway, what Templates contain
and where they are read, and what would help a person know which pathway fits. It ends with a keep/fix verdict on
Templates and seven recommendations for guidance.

**Why:** the owner asked for an evidence-based audit of whether pathways and Templates earn their keep, with a verdict
on Templates and design-only recommendations for pathway guidance.

**Checked:** every route, handler, service, store function and browser component cited was read against `main` at
834121f9. The citations were checked by script: all 44 referenced files exist and every cited line is within the file.
No application, Go test, web test or browser suite was run for this audit, so the findings are code-reading only; that
limit is stated in section 7 of the document.

**Not done:** no code, API, UI, schema or behaviour changed, and no throwaway probe was left behind. The recommendation
to bridge Templates and pathway outcomes, and the suggestion that Gaffer could propose a pathway, are design notes
only. The later stages (review, pull request, babysit, merge) have not been started.

**Review (27 November 2026):** all cited line ranges were re-checked against the source and are within their files;
every substantive claim in sections 1 to 6 was re-read against the code. Three fixes: the `updatedAt`-on-save citation
now points at `web/src/lib/api.ts` (the save and body helpers) rather than `pathways.ts`, the per-account limit row now
says "50 pathways" rather than "Stage 50", and Finding T2 no longer says a plan change must start from the template,
since any finished outcome can be saved as a new version. No finding's severity changed; the verdict, recommendations
and open questions stand, including that no runtime test was run.
