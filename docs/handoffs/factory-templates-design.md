# Private Factory templates: design and API contract

22 November 2026 · Stage 6 of [the delivery review plan](factory-delivery-review.md). This is the contract the
backend is built to and the UI builds from.

## What was traced on main

- **Plan.** `gaffer.OutcomeInput` → `outcomePieces` → `store.GafferOutcome`, one JSON document per outcome in
  `gaffer_outcomes`, owned by one account. A stage that names no agent is given its tier's agent from the account's
  saved crew when the plan is approved, else the outcome's own agent.
- **Criteria.** `criteria[] {id, text, revision}` and `criteriaRevision` on each pipeline (stage 1).
- **Skills.** A pending stage's skill guidance is chosen on the page (`POST /api/v2/skill-buddy/choose`) and saved on
  the stage as `skillGuidance[]`, each with its `id` (`builtin:…` or `library:<item>`) and the `revision` it was
  approved at. A skill is approved for an account when it is a built-in that is not disabled, or a library skill that
  is Active with a saved revision.

## Model

A **template** is a stable id and its **versions** 1…n. Each version is one immutable row of a new table,
`gaffer_templates(user_id, id, version, request_id, outcome_id, created_at, data)`, primary key `(id, version)`,
unique `(user_id, request_id)`. A version is never updated; a change is a new row.

Why a table of version rows rather than one document per template, as outcomes have: versions must stay readable and
must never be rewritten, and two saves on the same version must not both land. Both are what a row per version with a
primary key on `(id, version)` gives directly: the second insert of the same version fails inside its transaction.

A version's `data` (`store.GafferTemplateContent`):

| Field | Meaning |
| --- | --- |
| `name`, `description` | Up to 80 characters on one line; up to 1,000 bytes of plain text. |
| `objective` | The outcome's objective, up to 4,000 bytes. |
| `maxReviews`, `hours` | The outcome's review limits. |
| `pipelines[]` | 1 to 5: `{title, prompt, autoMerge, stages[], criteria[]}`. |
| `stages[]` | `{kind, part?, brief?, tier, tierWhy?, skills[]}`. No agent, model, effort, billing source or backup. |
| `skills[]` | Up to 2 per stage: `{id, name, source, revision}`. The revision the skill was approved at; no instructions. |
| `criteria[]` | Up to 10: `{id, text}`. `id` is stable across versions of the template. |

Each version also records `requestId`, `outcomeId` (the outcome it was saved from, or empty for an edit) and
`createdAt`.

An outcome started from a template records `templateId` and `templateVersion` on itself. Outcomes without them behave
exactly as before.

## Rules

- **Private.** Every route takes the signed-in browser account (`featureAccess`): another account's template is not
  found (404), read-only accounts, API tokens and MCP connections are refused (403). There is no MCP tool.
- **Saved from a successful plan.** The outcome must be the account's own, not `starting`, have at least one pipeline,
  and every pipeline must be successful: it has stages and is `done`, or its delivery in hand was accepted
  (`deliveryAccepted`). Otherwise **409 `template_not_successful`**. A plan of more than five pipelines cannot be
  started again and is refused (422).
- **What is copied.** The objective, review limits, and per pipeline the title, prompt, merge setting (`autoMerge`), the
  stage layout (kind, part, brief, tier and its reason), the current criteria text and each stage's skill ids and
  approved revisions. Never copied: the machine, project or folder, any agent, model, effort, billing source or backup,
  session, conversation or request ids, YOLO, folder trust, pull-latest, images, notes, summaries, evidence, decisions,
  Gaffer's decisions or reports, and usage.
- **Versioned.** Saving from an outcome without `templateId` makes a new template at version 1. Saving with
  `templateId` and `baseVersion`, or editing, makes version `baseVersion + 1`, and only when `baseVersion` is the latest:
  otherwise **409 `template_changed`**. Two saves on the same version: one wins, the other gets `template_changed`.
- **Bounded.** 20 templates per account (**409 `template_limit`** for a 21st), 20 versions kept per template (saving
  the 21st drops the oldest; the latest always stays), 64 KiB of JSON per version (422).
- **Duplicate requests.** Every write carries `requestId` (32 lowercase hex). The same request again answers the
  version it made; the same id with a different request is **409 `template_request_reused`**. A launch passes its
  `requestId` to the outcome, which already answers a repeated create with the outcome it started.
- **Resolved at launch.** Launching builds an ordinary `OutcomeInput` from the version for the project the person
  chooses now, and the machine is that project's. Stages name no agent, so each takes its tier's from the crew saved at
  that moment, else the outcome agent chosen in the launch. YOLO, trust, pull-latest, automation, auto-report and
  balancing are chosen in the launch, as they are when approving a plan. The plan input otherwise equals the version.
- **Skills at launch.** Each named skill is loaded for the account now. It is used only when it is still approved
  **at the same revision**; it is never replaced by another revision. A skill that is not is a gap, `changed` (a
  newer revision is the approved one) or `unavailable` (archived, removed, disabled or unreadable). A launch must list
  the gaps it was shown in `skippedSkills` (`"<pipeline index>:<stage index>:<skill id>"`); if the gaps now differ,
  **409 `template_skills_changed`** with the current gaps. Skipped skills are left out of the stage, nothing else.
- **Show changes before use.** `lastUsedVersion` is the version of the account's newest outcome started from the
  template, read from the outcomes kept. The difference route compares any two kept versions.
- **Existing runs keep their inputs.** An outcome copies everything it runs on when it is created. Editing or deleting
  the template later changes nothing in it; it still names the version it came from.
- **Delete** removes the template and every version; outcomes are untouched. It names the latest version it saw:
  otherwise `template_changed`.

## API

All under `/api/v2/gaffer/templates`, JSON, strict bodies, `Cache-Control: no-store`. 404 for an id the account does
not have; 422 with `field` for invalid input.

| Route | Body | Answer |
| --- | --- | --- |
| `GET /templates` | — | `{templates: [Summary]}`, newest change first |
| `POST /templates` | `{requestId, outcomeId, name, description?, templateId?, baseVersion?}` | 201 `Template` |
| `GET /templates/{id}` | — | `Template` (the latest version) |
| `GET /templates/{id}/versions/{version}` | — | `Template` for that version |
| `PUT /templates/{id}` | `{requestId, baseVersion, content}` | 200 `Template` (the new version) |
| `GET /templates/{id}/diff?from=&to=` | — | `Diff`. `to` defaults to the latest, `from` to `lastUsedVersion`, else `to − 1` |
| `POST /templates/{id}/launch` | `{requestId, version, projectId, tool, model?, provider?, trust, yolo, pullLatest?, autoReport, automation, balanceBelow?, skippedSkills?, confirmed}` | 201 outcome, as `POST /outcomes` |
| `DELETE /templates/{id}?version=N` | — | 204 |

`content` in a `PUT` is the version's `data` above. A criterion with a known `id` keeps it; one without gets a new one.

`Summary`: `{id, version, name, description, pipelines, createdAt, updatedAt, lastUsedVersion}`.

`Template`:

```json
{
  "id": "…", "version": 3, "latest": 3, "versions": [1, 2, 3],
  "outcomeId": "…", "createdAt": 1795000000000, "lastUsedVersion": 2,
  "content": {"name": "CSV export", "description": "", "objective": "…", "maxReviews": 8, "hours": 24,
    "pipelines": [{"title": "Export API", "prompt": "…", "autoMerge": false,
      "criteria": [{"id": "…", "text": "The export downloads as CSV."}],
      "stages": [{"kind": "code", "tier": "standard",
        "skills": [{"id": "library:…", "name": "Go tests", "source": "personal", "revision": "…"}]}]}]},
  "skillGaps": [{"pipeline": 0, "stage": 0, "id": "library:…", "name": "Go tests", "revision": "…",
    "status": "changed", "key": "0:0:library:…"}]
}
```

`Diff`:

```json
{
  "from": 2, "to": 3, "changed": true,
  "nameChanged": false, "descriptionChanged": false, "objectiveChanged": false, "limitsChanged": false,
  "pipelines": [{"title": "Export API", "change": "changed",
    "promptChanged": true, "autoMergeChanged": false, "stagesChanged": true,
    "stagesBefore": ["code", "review"], "stagesAfter": ["code", "review", "test"],
    "criteria": {"added": ["…"], "removed": ["…"], "reworded": [{"id": "…", "from": "…", "to": "…"}]},
    "skills": [{"stage": "code", "id": "library:…", "name": "Go tests", "from": "rev1", "to": "rev2"}]}]
}
```

`change` is `added`, `removed`, `changed` or `same`. Pipelines are matched by title, stages by kind and part. A skill
`from` or `to` is empty where the stage did not have it. `stagesChanged` covers the kind, part, brief and tier.

Conflict codes (409, with `code`): `template_not_successful`, `template_changed`, `template_limit`,
`template_request_reused`, `template_skills_changed`.

## Not built here

The UI (a separate job), an MCP tool (templates are browser-only on purpose), sharing a template with another
account, and starting a template with a different objective or pipelines without saving a new version first.
