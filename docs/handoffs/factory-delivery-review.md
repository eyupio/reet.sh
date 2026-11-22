# Factory delivery review and follow-up implementation plan

22 November 2026 · Product reference [SoduWork’s requirement and acceptance flow](https://github.com/mdiri-ai/SoduWork/blob/d28cd7eb94454a7ea35dd073ddaacd069229582b/README.md#how-it-works).

**Ship Factory delivery review first.** Start with an editable checklist and a human decision tied to the reviewed Git revision. Follow with launch readiness and recovery UX. Use the operational data from those changes to choose the smallest worthwhile performance improvement.

## Delivery stages

| Stage | Representative agent and model | Depends on | Deliverable and exit criteria |
| --- | --- | --- | --- |
| 0. Baseline and design | **Codex · GPT-6.2 Astra · High** | — | Trace plan, worktree, session, review and approval records on current main. Define the review API, state transitions and permission checks. Capture a synthetic Factory and long-chat baseline. Agree how unverified criteria, stale reviews, rework, and pre-existing outcomes behave. |
| 1. Delivery review backend | **Codex · GPT-6.2 Sol · High** | 0 | Add versioned acceptance criteria, evidence and recorded decisions using the existing Factory store and API patterns. An authorised reviewer can accept or send back the version they saw. An older action receives a conflict and duplicate submissions yield one decision. A restored outcome without criteria still opens and runs. |
| 2. Delivery review UI | **Claude Code · Sonnet 5.5 · High** | 1 API contract. UI fixtures can begin during 1. | Edit criteria in plan approval. Present each criterion, its evidence and the decision together in Factory. Home shows a concise status and opens the relevant review. Send-back carries the failed criteria and notes into rework. Desktop, phone and keyboard journeys pass. |
| 3. Launch readiness and recovery | **Codex · GPT-6.2 Sol · Medium** | 0. Can run alongside 1–2 on a separate branch. | Show resolved machine, agent, model, effort, selection source and eligible backup before approval. Reuse connector readiness and the existing Applied line. Home and Factory explain exhaustion and offer the existing recovery actions in place. Missing, stale, unavailable and explicitly default configurations have distinct outcomes. |
| 4. Performance and delivery reliability | **Codex · GPT-6.2 Astra · High** | 0 baseline, 1 review model, 3 recovery states | Re-measure Factory refresh payloads, request rate and time to a visible decision. Add compact summary reads and lazy detail where the baseline warrants them. Exercise disconnects at launch, fallback, approval and handover. Persist or replay only transitions shown to lose state. Long-chat tests capture p95 update time, payload bytes and scroll stability. |
| 5. First useful run | **Claude Code · Sonnet 5.5 · Medium** | 3, so onboarding uses the same readiness messages | Add a guided first session after machine enrolment. Surface installed agent, sign-in and model readiness, then offer a small reversible prompt with an observable result. Preserve direct access to Factory, agent sessions and terminals. Validate both a successful first reply and recovery from a signed-out agent. Update the quick start. |
| 6. Private Factory templates | **Codex · GPT-6.2 Sol · High** | 1–3, after the first release is in use | Save a successful plan as a private versioned template. Retain stage layout, criteria and approved skill revisions. Resolve machine and credentials at launch. Show template changes before use and record the version on each run. Existing runs keep their recorded inputs. |
| 7. Release verification | **OpenCode · Kimi K3** | Runs on each PR and release slice | Run targeted Go tests, web checks and Chromium journeys. Record the tested commit, commands and results. Recheck changed-head and permission boundaries, which receive a separate Claude Code review. Track CI and report failures for the owning agent to resolve. A human makes the merge decision. |

Stage 0 should also triage the reproduced temporary-project ordering test. Give it its own small PR if it interferes with the baseline. Confirm the required tie-break and make the test deterministic. [Ordering test](https://github.com/eyupio/reet.sh/blob/89c6ba1a5ed8ec108ebe78e60efc3ca67fe0063e/internal/store/store_folder_projects_temporary_test.go#L32-L44)

## First release slice

- **Plan and data.** Give checklist items stable IDs and an approved criteria revision. Store evidence against the pipeline, criterion and reviewed commit. Record the actor and time for acceptance or rework. Use trusted GitHub check references and approved artefact links. Label manual reviewer notes distinctly.
- **Server rules.** Check authorisation and pipeline scope on every review read and write. Validate the observed revision and current Git head before applying a decision, and use an atomic conditional write for the local review state. A later revision presents earlier evidence as historical. Keep delivery acceptance and GitHub merge approval as distinct actions. If a check or commit cannot be verified, display **Unverified**.
- **Decision card.** Show **Passed**, **Needs work** and **Unverified** with supporting evidence. Put **Accept delivery** and **Send back** alongside the findings. Make the selected commit and changed-head state legible on a phone. Link the Home summary to the exact review.
- **Tests.** Cover an old outcome, an interrupted request, duplicate submissions, two reviewers acting concurrently, changed criteria, a new commit, missing evidence and access from another project or account. Exercise the Go store/API and the existing Chromium Factory and Home fixtures. [Factory handover](https://github.com/eyupio/reet.sh/blob/89c6ba1a5ed8ec108ebe78e60efc3ca67fe0063e/internal/gaffer/outcome_runs.go#L1078-L1138) · [SoduWork review reference](https://github.com/mdiri-ai/SoduWork/blob/d28cd7eb94454a7ea35dd073ddaacd069229582b/hub-backend/app/services/task_acceptance_review.py#L124-L188)

## Factory crew settings

Save these tier defaults for the implementation pipelines. The stage table above names the intended workers for each workstream. Use the available tier selection in a pipeline or a separately configured implementation session when a workstream needs a different pairing. Confirm the exact model IDs listed by the selected machine and account before approving a run.

| Factory tier | Primary | Backup | Default Factory stages |
| --- | --- | --- | --- |
| Heavy | **Claude Code · Opus 5.5 · High** (`claude-opus-5-5`) | **Codex · GPT-6.2 Astra · High** (`gpt-6.2-astra`) | Plan, Design, Review |
| Standard | **Codex · GPT-6.2 Sol · Medium** (`gpt-6.2-sol`) | **OpenCode · Kimi K3** (`opencode-go/kimi-k3`) | Code, Test |
| Light | **OpenCode · DeepSeek V4.1 Flash** (`opencode-go/deepseek-v4.1-flash`) | **Claude Code · Haiku 5.5** (`claude-haiku-5-5`) | PR preparation and CI monitoring |

Use Claude Opus 5.5 for independent review of Codex implementation, and Codex GPT-6.2 Astra for independent review of Claude implementation. If fallback puts the author and reviewer on the same agent, choose another eligible agent or ask a person to review. Have OpenCode prepare the PR and summarise CI. Escalate substantive failures to the Standard or Heavy worker. The Factory crew has a Light default for the Merge stage. For this rollout configure merge to wait for human approval and use the worker to prepare the GitHub checks and review status. [Factory crew](https://github.com/eyupio/reet.sh/blob/main/docs/factory.md#your-factory-crew) · [Codex models](https://developers.openai.com/codex/models) · [Claude Code model configuration](https://code.claude.com/docs/en/model-config) · [OpenCode Go](https://opencode.ai/docs/console/go/) · [OpenCode Go model IDs](https://models.opencode.ai/providers/opencode-go/)

## Rollout and measurements

1. **Release A** includes the delivery review backend and UI. Start with explicitly configured acceptance criteria. Check the existing outcome format and the migration or backup-restore path on representative old data. Exercise approve, send-back and stale-head handling in a disposable Git repository.
2. **Release B** includes launch readiness and recovery. Test all three agent integrations on a connector where they are already configured. Simulate exhaustion and lost connections. Verify that retry records the selected backup and retains the pipeline history.
3. **Release C** includes the measured efficiency work and guided first run. Compare the same fixture and settings before and after. Track requests per minute, bytes per refresh, p95 time to a visible decision and time to first session response. Treat reduced transfer volume with no regression in decision latency as the initial target. Use the baseline to set a numerical budget before changing transport.
4. **Release D** introduces private templates after the review contract and reuse needs are established.

Use one PR for the review backend and one for its UI, with the UI dependent on the backend. Launch readiness and recovery can proceed in parallel after the design agrees the state names and API boundaries. Check current main and open PRs at the start so the implementation builds on the latest Factory and connector work. Keep any performance transport change and template work in separate PRs.
