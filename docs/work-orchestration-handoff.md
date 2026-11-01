# Reet work orchestration and Eli: staged implementation handoff

Prepared: 1 November 2026. Status: proposed follow-on work; implementation has not started.

## Gaffer branding and Eli reuse (1 November 2026)

**Gaffer** is reet's native agent control interface and the user-facing identity
for its structured browser experience. **Eli AI Chat** is the underlying provider/chat
technology being reused from Zoomies where compatible. **OpenCode TUI** is the
agent's original terminal interface; reet's actual CLI TUI remains separate.

Continue Eli reuse rather than creating a competing chat implementation. Preserve
reet's existing sessions, ACP integration, streaming, permissions, trust checks,
request IDs, capability negotiation and agent lifecycle. Additional agents remain
subject to the structured-chat plan's verified adapter gates. Coordinator API
providers and machine-local workers have separate credentials and authority.

The Gaffer naming slice retains existing internal `reet`/`chat` identifiers and
stored preferences. Shared HTML tooltips explain the interface on hover, focus
and touch, including in modal dialogs. OpenCode TUI labels and behaviour remain.

A bounded KZ5A contribution is recorded in [the Eli reuse review](research/eli-reuse.md):
source/licence review, dependency matrix and a test-only fake-backed probe of the
reused provider contract, cancellation and an offered read. No provider adapter,
credential route, second chat UI or orchestration dispatcher is enabled. The
coordinator will be integrated into Gaffer through the documented stages.

KZ0 reconciliation and KZ5A's remaining decisions are unfinished; neither stage
is marked complete. KZ5 and later stages retain their dependencies and acceptance
gates. The dated baseline, original delivery record and history below are preserved.

## Purpose and relationship to the existing handoff

This document develops the user's proposed durable-work direction for reet.sh:
one goal can move through different agents, models and machines, with reviews by
other agents/models and a record that survives individual sessions. Eli, the
assistant already built into Zoomies, is a candidate conversational interface
and source of reusable provider/chat patterns. Kanzo supplies additional lessons
about shared work, atomic claims and measured operating costs.

The core product direction is work orchestration. Installation, MCP payload and
refresh improvements remain independent supporting slices; completing those
slices is not a prerequisite for designing the core workflow.

**Keep this as a linked companion to `handoff.md`.** The root handoff remains the
entry point for current state, product priorities, standing rules and the active
implementation task. This file owns the KZ stages and their acceptance criteria.
The root file needs only a link, the selected KZ stage when one becomes active,
and the normal completion record. Do not duplicate the full plan in both places.

The existing structured-chat work continues under `docs/structured-chat-plan.md`.
The durable-work stages below fit the roadmap's **richer run orchestration**
direction. They use the existing session and project foundation and preserve
the structured-chat plan's adapter enablement gates. A terminal-based worker can
participate through verified existing capabilities while structured integrations
continue to mature; the first orchestration release need not enable every adapter.

The request that produced this file authorises the handoff and its index entry;
the user subsequently requested a PR to preserve the work. Publishing that
documentation PR does not mark the proposed implementation, migrations, new
permissions or a production rollout as completed or selected for immediate execution.

### How the documents work together

| Document | Owns | Update rule |
| --- | --- | --- |
| `handoff.md` | Current product state, active task, standing rules, checked publication state | Add a short KZ pointer; record the one selected stage and its completion |
| `docs/structured-chat-plan.md` | SC capabilities, adapters, questions and recovery | Preserve its current sequence and gates |
| This file | KZ stage scope, decisions, dependencies and acceptance evidence | Keep each stage's status and exact next task current |
| `docs/roadmap.md` | Public directions | Update only when an outcome is shipped or a direction changes |
| `handoff-history.md` | Older delivery narrative | Follow the repository's existing archive convention |

Read the latest relevant progress record and the actual code together. The root
handoff contains older checkpoints as well as later progress; an older “next
unchecked task” is not evidence that the task remains undone.

## Evidence and inspected baseline

- reet checkout inspected: `b10a46834c8e41f30cd0db577c467bfd8574d96f`, dated
  1 November 2026. The working tree was clean before this documentation change.
- Additional remote reet files were inspected at
  `97ef35a150c4456433bd701f8ff5f05746d487db` during the initial comparison.
  Reconcile with current main before implementation; this file describes a dated baseline.
- Kanzo inspected: `83a97df1140756bf698046e6d1b970e916ef1b2d`.
- Zoomies/Eli inspected after the user's clarification:
  `b4c67a1413c8274334d18225df4053959861fbc0`. Read `docs/eli.md`, the provider
  interface/conformance suite, chat/toolbox controller, conversation API/store,
  browser conversation state and the separate repair implementation.
- Review method: source and documentation inspection, plus local tool-definition
  size calculation. Neither application's advertised performance was reproduced.
  No live machine was enrolled and no provider-backed agent was run for this review.

### Observations that inform the plan

| Observation | Consequence for reet |
| --- | --- |
| Kanzo has `next_task`, comments and instructions for handing work back to review | Add an explicit work contract and agent participation after the existing session foundation |
| Kanzo selects and moves the next eligible task inside an immediate SQLite transaction | Claiming must be one atomic operation, with replay handling added for reet's distributed clients |
| Kanzo excludes human-only items from automatic selection | Make agent eligibility explicit and enforce it in the domain operation |
| reet's Backlog holds launch plans; Finished describes session termination and does not imply success | Keep work acceptance separate from terminal/session state |
| reet's MCP interface has 30 management tools and 11 read-only tools at this baseline | Measure both discovery profiles and representative responses before optimising |
| The checked-in 30-tool JSON is approximately 71 KB when minified | This is a source-artifact measurement, not an exact model-token or live wire measurement |
| reet has a Go-hosted MCP endpoint, an optional TypeScript stdio bridge, generated definitions and structured outputs | Preserve their compatibility and generated-definition parity |
| Kanzo compares resource use between base and proposed builds | Add reproducible PR evidence appropriate to reet's larger feature set |
| Kanzo polls a small change token before fetching board data | Pilot conditional refresh on one reet collection and measure the result |
| reet already skips hidden-page polling, protects against older replies, and retains last good Home data | Extend those behaviours rather than treating them as missing |
| reet already has binary installation, native services, verified upgrades and recovery | Improve discovery and the first-use journey; reuse the existing installer |

Kanzo's review/done boundary is partly agent guidance. Its atomic queue selection
is implemented in storage, but its MCP instructions are not an authorisation
system. In reet, enforce claim ownership, human acceptance and credential scope
on the server.

## Product architecture: durable work, routed attempts and independent review

### The intended experience

A person can ask a reet coordinator to plan a feature, choose suitable workers,
run implementation on one machine, ask different agents/models to review the
result elsewhere, reconcile findings and return a result for acceptance. The same
work record explains what happened after a browser reload or a worker session ends.

An illustrative workflow, with assignments chosen from the user's actual fleet:

```text
Work: add the export feature
  Planning step        -> chosen planner/model
  Implementation step  -> selected coding agent/model on machine A
  Verification step    -> supported test environment on machine A or B
  Review step 1        -> a different agent/model on machine B, artifact R1
  Review step 2        -> another requested perspective, artifact R1
  Revision attempt     -> implementing agent addresses findings, produces R2
  Re-review            -> required reviewers assess R2
  Human acceptance     -> criteria, test evidence and current review results
```

The roles do not hard-code a particular vendor or imply one model is always best.
The plan records both the requested assignment and the identity actually used.

### Durable records

| Record | Responsibility |
| --- | --- |
| Work item | Goal, acceptance criteria, owner and overall outcome |
| Plan revision | Versioned steps, dependencies, proposed assignments and approval state |
| Step | One bounded objective: plan, implement, test, review or reconcile |
| Execution attempt | One dispatch to a machine/agent/model, with request ID, session link and observed outcome |
| Artifact revision | Exact code or other output the next step may consume |
| Review | Reviewer identity, artifact revision, verdict, findings and evidence |
| Routing policy | User choices and constraints for eligibility, assignment and fallback |
| Coordinator conversation | Planning/explanation attached to the work; durable records remain authoritative |

An implementation attempt may fail or be replaced without losing the work or
rewriting its history. Different steps can use different agents and models.
Reviewers are independent participants; adding two review sessions alone does
not establish that the current artifact passed either review.

### Routing rules

Begin with manual assignments and explicit user preferences. Add suggestions
before autonomous selection. A model may propose a route; the controller decides
whether that route is eligible from observed capabilities and saved policy.

Eligibility includes machine availability, permitted project/repository access,
OS/runtime requirements, installed compatible agent, supported model/mode,
workspace/artifact availability, concurrency and the user's provider/budget/data
preferences. Distinguish configured credentials from a verified successful turn.
Record why candidates were excluded. An unavailable requested model produces an
explicit blocked state or an approved fallback, never a silent substitution.

Keep machine-local worker credentials on their existing machine. An Eli provider
on the controller does not prove that the corresponding worker is installed,
authenticated or authorised on another machine. Coordinator provider credentials
and worker agent sign-in are separate resource types.

### Cross-machine artifacts and reviews

Every review binds to an immutable artifact revision: for git work, repository
identity plus base/head commit and any necessary patch/workspace manifest; for
other work, a bounded content-addressed artifact manifest. A branch name or local
directory alone is insufficient. Uncommitted work needs an explicit verified
snapshot/patch route, or a clear unsupported state.

A destination explicitly prepares the matching workspace and acknowledges the
revision before running a dependent step. Never assume the same folder path or
checkout exists on each machine. Artifact preparation is a distinct reviewed
execution operation, not an extension of the current skill-sync API or a generic
remote shell command.

Reviews record their agent, model/provider where observable, machine, artifact,
criteria, verdict and findings. Changing code invalidates earlier pass decisions
for acceptance of the new revision while retaining them in history. Required
review roles and acceptable independence are policy: “different agent” and
“different model” are separate constraints. Disagreement remains visible until
resolved; a vote count does not turn model agreement into test evidence.

### How Eli fits

Reuse/adapt the useful seams, after a small compatibility spike:

- `internal/assistant/provider.go` and `contract.go`: streaming, tool calls,
  cancellation, provider checking and usage reporting with conformance tests.
- `internal/controller/assistant_chat.go` and `assistant_tools.go`: bounded model
  rounds, explicit tool allowlists, owner/provider selection and observed tool use.
- Conversation API/store and browser assistant components: account-owned saved
  conversations, streamed replies and visible provider/model/tool activity.
- Separate repair records: a useful example of durable attempts, request
  deduplication, budgets and interrupted outcomes, to be compared with reet's
  existing create-request and recovery contracts.

Zoomies chat currently offers a read-only fleet toolbox. Repair writes use a
separate opt-in workflow. Subscription-backed CLI chat has a different capability
contract from API tool-calling providers. These distinctions must survive reuse;
copying the widget does not create a remote multi-agent scheduler.

For reet, begin with a coordinator that reads permitted work/fleet state and
proposes a structured plan. The person reviews assignments, evidence sharing and
budget, then starts it through ordinary domain operations. Subsequent automation
can advance already-approved steps within a bounded policy. Plan text and model
tool output never directly acquire arbitrary execution authority.

Eli's saved coordinator conversation is a proposed new reet record. It is distinct
from copying worker ACP/native transcripts into the controller, which remains
outside this plan. Essential plan decisions and progress survive conversation
retention because they live in the work records.

## Delivery sequence and status

The original handoff started no stages. The Gaffer follow-up above contributes a bounded
KZ5A probe; KZ0 and KZ5A remain incomplete, and all other stages remain **not started**.
KZ0 is the next task when this workstream is selected.

| ID | Deliverable | Depends on | Approximate scope |
| --- | --- | --- | --- |
| KZ0 | Reconcile current work and capture an execution baseline | Selection of this workstream | Small investigation |
| KZ1 | README first-run journey | KZ0 | Small documentation PR |
| KZ2 | Reproducible resource/MCP report | KZ0 | Medium tooling PR; split harness and CI if needed |
| KZ3 | Measured MCP payload reduction | KZ2 baseline | Small or medium compatibility-focused PR |
| KZ4 | Conditional Home refresh pilot | KZ2 baseline | Medium API/browser PR for one collection |
| KZ5A | Eli reuse and orchestration compatibility spike | KZ0 | Bounded investigation/design PR |
| KZ5 | Work/plan/step/attempt/review and routing contract | KZ5A; current orchestration/recovery reconciled | Design-only PR |
| KZ6 | Durable domain, transitions and browser metadata API | KZ5 contract | Backend slices for records, then claims |
| KZ7 | Browser plan, manual assignment and session links | KZ6 | UI/integration slices |
| KZ8A | Explicit MCP work grants and read tools | KZ6; grant contract from KZ5 | Medium permission/protocol PR |
| KZ8B | Atomic agent claims, progress and review handoff | KZ8A; proven claim domain from KZ6 | Medium protocol PR |
| KZ9A | Cross-machine artifact preparation and dispatch | KZ7; explicit execution contract | Bounded workspace/dispatch slices |
| KZ9B | Independent review roles and revision-bound acceptance | KZ8B and KZ9A | Review-policy/integration slices |
| KZ10 | Eli coordinator: read state and propose a reviewable plan | KZ5A and KZ6; execute through KZ7/KZ9A contracts when available | Provider/chat, then proposal UI slices |
| KZ11 | Policy-driven routing and bounded automatic progression | KZ9B and KZ10 | Explicit opt-in automation slices |
| KZ12 | End-to-end acceptance, operational coverage and release readiness | Selected core features above | Bounded hardening/documentation PR |

For the clarified product goal, the recommended core sequence is KZ0 → KZ5A →
KZ5 → KZ6 → KZ7 → KZ8A → KZ8B → KZ9A → KZ9B → KZ10 → KZ11 → KZ12.
KZ10's read/proposal surface can be brought forward once KZ6 exists; dispatch
remains unavailable until its execution prerequisites are delivered. Start with
one implementing role and one independent reviewer, then demonstrate a second
machine and another review role before broader automatic routing.

KZ1–KZ4 are independent supporting improvements. KZ2 precedes KZ3/KZ4
optimisation; those supporting stages do not delay the core design. KZ4 must
coordinate with whoever owns Home, Needs you and session updates. Design can
continue alongside SC work; integrating launch/recovery uses the current verified
contracts. Existing active work keeps its recorded priority until explicitly changed.

These dependencies describe sequencing, not a request to run parallel agents.
Select one bounded slice per PR. A stage that grows beyond reviewable scope is
split with explicit intermediate acceptance criteria before code is expanded.

## KZ0 — Reconcile the active work and establish a baseline

**Outcome:** the next implementer knows what is already shipped, what is active,
which contracts must be preserved and which single stage comes next.

Tasks:

- Read `handoff.md`, `CONTRIBUTING.md`, applicable `AGENTS.md` files, the latest
  SC progress and `docs/roadmap.md`. Inspect branch, status and local changes.
- Check current main and the relevant PR/CI records when available. Record
  verified state separately from branch notes. Preserve unrelated work.
- Look for existing orchestration/work-item work added since this baseline.
  Reuse it or revise this plan before proposing duplicate types and endpoints.
- Record the actual browser and MCP access boundaries for projects, backlog,
  terminal sessions, OAuth and the stdio bridge.
- Map Home's refresh inputs: backlog, sessions/attention, projects, machine
  readiness/health and sync state. Include time-driven freshness transitions.
- Reconcile the clarified goal with any current richer-run work. Map existing
  run/worktree/artifact records before adding plan, step, attempt or review types.
- Recheck Zoomies/Eli at the recorded source revision or a newer explicitly
  inspected revision. Read applicable Zoomies instructions before adapting code;
  do not assume its UI/store/controller contracts fit reet unchanged.
- Recalculate discovery bytes from actual `tools/list` for read and management
  access; retain the static JSON figure only as a secondary comparison.
- Record toolchain versions, available test fixtures and resource measurement
  method. Choose the next bounded stage in the root handoff.

**Acceptance:** a dated baseline names the base SHA, active overlapping work,
verified completion states, measured MCP discovery sizes, constraints and the
selected next task. Unknown live/PR state is labelled unknown. No implementation
is claimed from reading this document.

**First files:** `handoff.md`, `docs/structured-chat-plan.md`,
`docs/session-board.md`, `docs/projects.md`, `docs/mcp.md`,
`internal/api/session_backlog.go`, `internal/api/mcp_server.go`,
`web/src/lib/home/Home.svelte` and `SessionBoard.svelte`.

## KZ1 — Make first use obvious

**Outcome:** a newcomer can choose local binary or Docker deployment, then follow
one supported route to a useful session.

Tasks:

- Put **Run locally** and **Deploy with Docker** together near the README quick
  start. Reuse `scripts/install.sh` and the documented `reet serve` behaviour.
- Explain the local start, owner creation where applicable, persistence location
  and link to native-service installation. Keep the short route short.
- Follow installation with: connect a machine → choose a folder project → save
  work or a prompt → start an installed agent → open the session and review it.
- Keep host installation, controller deployment, connector enrollment and
  provider sign-in distinct. Show platform availability from the current release
  matrix rather than inheriting Kanzo's platform claims.
- Preserve the recently improved Welcome page's wording and reet's Yorkshire
  voice. Use the existing enrollment-command builder for any UI command examples.
- Check that new text describes shipped behaviour. Persistent work acceptance
  remains future behaviour until the later KZ stages are delivered.

**Acceptance:** commands and links match the current implementation; both install
paths are discoverable; one coherent first-use journey is visible; there is no
new installer or runtime change hidden in this documentation slice.

**Validation:** local link/anchor checks, `git diff --check`, consistency with
`docs/binary-installation.md` and CLI help. Do not run an installer or change a
real deployment merely to validate prose. If UI copy is changed as a separate
slice, run its existing public-workspace/first-run fixtures and responsive checks.

**Likely files:** `README.md`, `docs/binary-installation.md`, and the specific
onboarding docs whose instructions actually change.

## KZ2 — Report resource and MCP costs on PRs

**Outcome:** reviewers see comparable costs for base and proposed builds before
optimisation targets are chosen.

### Measurement contract

Start with four measurements:

1. Release executable bytes for one fixed OS/architecture.
2. Built frontend asset bytes, with raw and compressed size labelled separately.
3. Idle controller resident memory after a defined readiness and settling period.
4. MCP discovery and representative reply bytes for both read and management access.

Then add a populated synthetic fixture for list/board API latency, response bytes
and loaded memory if it fits this PR. Keep startup-to-readiness separate from
request latency and from browser rendering; do not label an API timing as page load.

For MCP, measure hosted HTTP and the stdio bridge where their representations
differ. Capture structured content and text content separately and record
duplication in the wire result. Distinguish bytes, characters and estimated
tokens. Any token estimate must name its tokenizer/model; baseline measurement
must work without a paid model call or provider credential.

### Implementation and CI

- Use disposable homes/databases and synthetic accounts, inventories and sessions.
  External metadata fetching, real machines and provider calls are disabled.
- Keep fixtures within production bounds. Include empty, ordinary and populated
  cases; state the exact counts in the report instead of implying unlimited scale.
- Build base and proposed code in separate worktrees with their own lockfiles and
  web assets. Finish each web build before its Go build. Use the same runner and
  alternate repeated timing/memory measurements; report medians and sample count.
- Record base/head SHAs, OS/architecture, toolchains, fixture version, procedure
  and units in machine-readable JSON plus a readable job summary.
- Use a dedicated, bounded workflow/job on an appropriate runner. The existing
  terminal-browser job is near its time budget; add no cases there.
- Run proposed code with read-only repository permissions and no production
  credentials. A GitHub job summary and artifact are sufficient initially.
  A later PR-comment publisher must treat measured artifacts as untrusted data
  and bind them to the actual run and SHA before writing a comment.
- Begin with reporting. Establish repeatability before making resource thresholds
  required checks. Deterministic size budgets and noisy timing limits need
  different policies. Missing measurements are reported as unavailable.

**Acceptance:** rerunning the same build produces stable size results and an
explained timing/memory range; a deliberate larger fixture or payload produces a
visible increase; the report identifies its base/head; no live inventory or secret
is collected. A failed or skipped benchmark cannot look like an improvement.

**Likely files:** proposed measurement scripts under `scripts/`, a fixture near
`mcp/test/fixture` or a dedicated test helper, a dedicated workflow under
`.github/workflows/`, and measurement-method documentation. Final names are chosen
after KZ0; these are locations, not claims that new files already exist.

## KZ3 — Reduce MCP payloads without losing the contract

**Outcome:** a measured reduction in context cost with existing clients and
permission semantics preserved.

Tasks:

- Use KZ2's per-tool breakdown to choose the largest avoidable cost: repeated
  descriptions, unnecessary schema expansion, oversized list records or replies.
- Preserve tool names, input meanings, typed output guarantees, pagination,
  truncation markers, actionable errors and the “write succeeded” retry warning.
- Keep crucial descriptions: shelf state does not install files, stale scans do
  not prove absence, and destructive metadata actions have their existing controls.
- Keep hosted, stdio and Browser WebMCP definitions consistent where supported;
  regenerate `internal/api/mcp_tools.json` from its source and verify parity.
- Test concise mutation summaries while retaining structured output compatibility.
  Measure client-visible results before assuming a shorter text field reduces
  context when the same structured object is also present.
- Consider optional domain profiles or compact defaults only if evidence supports
  them. An existing request's fields or output shape cannot silently change.
  Any incompatible default needs an explicit version/profile transition.
- Preserve optional/nullable schema semantics. Do not copy Kanzo's schema trimming
  mechanically or remove required type information for a byte target.

**Acceptance:** record before/after discovery and reply measurements for the same
fixtures. Existing SDK/protocol/OAuth checks pass, generated tools match, old
arguments still work, and frozen shelf evaluation answers remain obtainable.
Choose and document a useful reduction target after the baseline; there is no
invented token-saving percentage in this handoff.

**First files:** `mcp/src/server.ts`, `output.ts`, `response.ts`,
`mcp/test/export-tools.mjs`, `internal/api/mcp_tools.json`,
`internal/api/mcp_server.go`, `mcp/test/` and `mcp/evaluations/README.md`.

## KZ4 — Pilot efficient Home refreshes

**Outcome:** one chosen Home collection refreshes conditionally, with measured
payload/query savings and existing freshness behaviour preserved.

Start with the account's backlog collection unless KZ2 identifies a better
bounded target. Retain independent refreshes for the other Home inputs. Extending
the mechanism to sessions, attention or machine health is a separately bounded
follow-up with its own invalidation coverage.

Tasks:

- Choose an account/view-scoped ETag or revision mechanism using the existing HTTP
  conventions. Validate authorisation before replying “unchanged”. Use private
  cache controls; do not introduce a shared cache for private data.
- Make every mutation relevant to the chosen response invalidate its revision.
  A cheap revision must not require rebuilding the entire expensive response.
- Include relevant role/access changes. Avoid exposing another account's changes
  through a global instance counter. Revisions must remain valid across controller
  restart, restore and account changes; use an epoch/identity where necessary.
- Preserve explicit refresh after a local write, cancellation on page exit,
  visibility handling, stale-response protection and last-good-data errors.
- Await a poll before scheduling the next. Back off transient failures and keep
  a bounded fallback if the conditional mechanism is unavailable.
- Keep the browser clock advancing when the payload is unchanged. Machine or
  inventory staleness can change with elapsed time alone. An unchanged database
  revision cannot freeze last-seen badges or pending-request expiry.
- Before touching session/attention refresh, map broker-memory changes and SC
  question/approval events as well as SQL writes. A database-only counter may
  miss these events. Preserve the existing SSE chat transport.

**Acceptance:** unchanged data avoids the full response; relevant writes appear
within the chosen refresh contract; account boundaries hold; older replies cannot
undo newer state; a server restart/restore does not pin the browser to old data;
time-based states still change; hidden tabs and slow requests do not pile up work.
Record transfer/query evidence and exactly which collection is covered.

**First files:** `web/src/lib/home/Home.svelte`, `SessionBoard.svelte`,
`web/src/lib/sessionBacklog.ts`, `internal/api/session_backlog.go`,
`internal/store/store_session_backlog.go` and their tests.

## KZ5A — Check the Eli reuse boundary

**Outcome:** a small evidence-backed reuse plan, before porting a chat widget or
building a second orchestration stack.

Tasks:

- Compare Zoomies' provider/stream/tool/usage contracts with reet's existing ACP
  worker path. Name the coordinator API-provider path and machine-local agent
  path separately, including cancellation, identity and credential ownership.
- Identify the smallest reusable provider core and conformance fixtures. Assess
  a narrow adaptation first; extract a shared package only if both products need
  the same stable contract and its versioning/ownership is clear.
- Inspect imports, auth, route, Svelte-state, Markdown, redaction, storage and
  secret-management dependencies. Record reuse as-is, adapt, or keep product-local.
  Read existing source/license obligations before copying code.
- Demonstrate provider streaming, cancellation and a permitted read tool against
  fake providers in a disposable local harness. No real API credential or model
  spend is required for the spike.
- Inventory Eli repair patterns that help with durable attempts, deduplication,
  budgets and interrupted publication; retain reet's own verified launch contracts.
- Decide where the coordinator UI appears and how conversations attach to work,
  how provider access is configured, and how a proposed plan becomes versioned
  domain data. Use Gaffer as the user-facing identity and Eli as the technical reuse reference.

**Acceptance:** a dependency/reuse matrix, one minimal fake-backed contract probe,
the proposed coordinator/worker boundary and a concrete KZ5 decision list.
Subscription CLI chat is not labelled a remote worker or a tool-capable provider
without measured support. No whole Zoomies subsystem is copied during the spike.

**Source map:** Zoomies `internal/assistant/{provider,contract}.go`,
`internal/controller/{assistant_chat,assistant_tools,eli_repairs}.go`,
`internal/api/handlers_assistant_{chat,conversations}.go`,
`internal/store/queries_assistant_conversations.go`, and
`web/src/lib/assistant/`. Reet entry points are its current ACP, broker, MCP,
account/credential and chat UI modules from `handoff.md`.

## KZ5 — Agree the work orchestration contract

**Outcome:** an implementation-ready design for one goal carried through steps,
attempts and independent reviews across agents, models and machines.

This stage is a design-only slice. It resolves the decisions below before schema,
UI and credential changes are implemented. Record the chosen contract and its
trade-offs; routine implementation details can be settled by the implementer.

### Proposed starting model

| Concept | Proposed role |
| --- | --- |
| Work item | Account-owned intent with stable ID, title, acceptance criteria, plan revisions and outcome |
| Plan/step | Versioned objectives, dependencies, roles, approved assignments and bounded progression |
| Folder project | Existing machine/folder execution context; each attempt names its actual context |
| Session draft | Existing versioned launch plan, retaining its stable create-request semantics |
| Session | One actual terminal or structured conversation, linked by an authoritative record |
| Attempt | One execution of one step, binding machine, agent, requested/observed model, session and result |
| Artifact/review | Exact produced revision and the reviewers' evidence/verdicts for that revision |
| Progress entry | Bounded, authored update; not a copied transcript |
| Claim | Durable assignment of one executable step/attempt to a verified principal with replay protection |
| Work acceptance | Human outcome decision after required current-revision review and evidence gates |

Proposed work states are `idea`, `ready`, `in_progress`, `review`, `done` and
`cancelled`. A blocked reason can accompany in-progress/review work without
reclassifying terminal state. Human-only eligibility is explicit. Final labels
and transition rules belong in the KZ5 decision record.

### Decisions to resolve

- **Placement:** how work appears alongside the current session board without two
  unexplained Backlogs or an unrelated new navigation system. Choose an integrated
  view or a clearly labelled mode using the existing shell and project context.
- **Identity:** use stable opaque IDs; optional readable references are display
  only. Preserve machine/folder project IDs. Decide how one logical work context
  names several explicitly registered execution locations and a canonical repository
  identity. Do not equate a repository name or matching path with access proof.
- **Lifecycle:** separate work acceptance from shell exit and agent turn completion.
  An agent requests review; a human accepts done or returns it with a reason.
  Decide reopen, cancel, archive and active-claim edit behaviour explicitly.
- **Ownership:** start with personal account ownership. Shared lists/teams confer
  no implicit work or terminal access. A removed project leaves history readable
  to its owner but requires explicit rebinding before another launch.
- **Plan progression:** define step dependencies, allowed implementation/review
  concurrency, plan-version approval, failed-attempt retry and a bounded revision
  loop. Start with a small fixed role sequence; arbitrary DAG scheduling can wait.
- **Assignment:** record requested and observed machine, agent, model/provider,
  effort/mode and workspace. Define eligibility, explicit override, fallback,
  resource contention and unavailable/unknown authentication states.
- **Review:** choose required roles and independence rules, structured verdicts and
  findings, current-artifact binding, stale-review behaviour, disagreement handling
  and final human acceptance. Reviewer completion is not an acceptance vote by itself.
- **Artifacts:** define commit/patch/manifests, bounded transfer, destination
  preparation, ownership, acknowledgement, retention and failure cleanup. Decide
  whether the initial implementation accepts only reachable committed git revisions.
- **Storage:** choose bounded fields, item/note/claim limits, pagination, retention
  and version checks. Store authored work metadata and small summaries; preserve
  the existing separation from ACP transcripts and native agent stores.
- **Claims:** define atomic selection of eligible ready steps, deterministic ordering, idempotency scope,
  claim generation, conflict handling and owner recovery. Never release a claim
  merely because the browser disconnects or a timer expires.
- **Launch links:** define the transaction boundary between work, frozen draft,
  create request, actual session and a lost reply. Resolve “request recorded, result
  unknown” explicitly. A claim alone does not start a terminal or grant shell access.
- **MCP access:** define separate work grants, for example `work:read`,
  `work:claim` and `work:update`, with explicit account/project binding. Names are
  proposed until reconciled with the existing OAuth and manual-credential model.
  Existing `mcp:read`/`mcp:write` grants keep their shelf meaning.
- **Recovery and data ownership:** specify account deletion, project/machine
  removal, session-history removal, backup/restore, export/import and downgrade.
  Restoring older data must invalidate obsolete claim authority.
- **Coordinator:** specify which state Eli can read, provider-specific data access,
  proposal schemas, conversation retention, budget accounting and cancellation.
  A conversation proposes versioned plan changes; execution consumes approved
  records through the same API/domain used by the browser.

**Acceptance:** state/transition tables for work, steps, attempts and reviews;
field/limit and access matrices; claim/replay and launch-recovery protocols;
artifact/routing/review rules; and migration/export coverage are recorded. A
worked example covers implementation on A, review on B and a new code revision
invalidating prior passes. Resolve outstanding product decisions before dependent
behaviour. Use a controller-owned durable state machine; a new service or worker
transcript store is not required.

## KZ6 — Implement the work domain and browser metadata API

**Outcome:** work, plans, steps, attempts, artifact references and review records
persist; transitions and claims are correct under concurrency; authorised browser
users can manage metadata. Split record/migration work from claim/API work if the
combined change is too large for one review.

Tasks:

- Add additive store migrations for the KZ5 model. Keep domain transition and
  claim rules separate from handlers and Svelte views, using existing transaction
  patterns and bounded parsing.
- Bind every read/write to the owning account and, where applicable, the project.
  Validate linked session ownership and context server-side; a submitted ID is
  never sufficient proof of access.
- Require version checks for edits and state changes. Implement claim selection
  and assignment in one transaction per ready step, excluding ideas, human-only
  steps, unmet dependencies and steps already claimed or outside the permitted
  context. Several review steps may be eligible concurrently when the plan allows.
- Persist the claimant, generation, request binding and result needed for replay.
  A repeated claim request returns the original result, including a prior empty
  result; the same request ID with different input is refused. Choose retention
  bounds explicitly so expired replay records cannot silently duplicate work.
- Reject stale generations after owner release/reassignment. Keep claims during
  transient disconnects; show stalled/unknown state and offer explicit owner recovery.
- Add bounded progress entries and review transitions. Derive actor identity from
  the signed-in user or authenticated credential, not an arbitrary author string.
- Enforce review-artifact and attempt/assignment bindings even before automatic
  dispatch is added. New artifact revisions preserve history and invalidate old
  acceptance gates atomically. Record requested identity separately from observation.
- Reuse browser account/origin/rate-limit conventions for new browser metadata
  routes. Existing browser-only backlog routes retain their authentication rules.
- Include the new records in the chosen instance/personal backup and export
  formats. Version import behaviour and test it. Account removal and retention
  must cover notes and claims, not just the main item row.

**Acceptance cases:** two separate connections claiming the same sole ready item;
lost-reply retry; same request ID with different arguments; stale edits; another
account/project/session ID; owner reassignment; agent attempting done; human-only
item; full/oversized notes; restore and migration. Failure must leave a recoverable
state. A restored database must not accept an old active claim token.

**Likely files:** a feature-local domain module if justified, additive files under
`internal/store/` and `internal/api/`, registration in existing initialisation,
backup/export code identified in KZ0, and focused regression tests. New names and
routes follow KZ5 rather than being fixed by this preliminary handoff.

## KZ7 — Add the browser work flow and session links

**Outcome:** a person can define a plan, manually assign implementation and review
roles, run supported steps through existing launch controls, and continue in another
session without losing the work identity. Multi-machine preparation follows in KZ9A.

Tasks:

- Implement KZ5's chosen placement using the existing Home/project/session
  components. Keep keyboard actions, focus restoration, touch targets, light/dark
  themes, reduced motion and honest offline/stale states.
- Support create/edit, ready/human-only eligibility, progress, review, accept,
  return and owner recovery using domain-authorised transitions.
- Show steps, attempts, requested/observed agent and model, actual machine, artifact
  revision and reviewer status. Begin with explicit assignment; show why a selected
  candidate is unavailable. Retrying creates a distinct attempt under the same step.
- Link work explicitly to session context. Reuse the frozen draft and stable
  terminal create-request ID. Do not auto-launch from a claim, a saved prompt or
  a status edit.
- Add or reuse an authoritative launch/link operation so a create request whose
  response is lost can be reconciled. Avoid separate uncoordinated browser writes
  that leave an orphan session or duplicate launch on retry.
- Retain existing account, machine, folder, agent/model, worktree, trust and YOLO
  checks at launch. A work item's saved intent is not consent to bypass them.
- Keep terminal Finished and work Done distinct in labels and filters. An exited,
  lost, cancelled or quiet session cannot automatically satisfy acceptance criteria.
- A second session adds another attempt/link to the same work item. Present the latest
  progress, previous session links and reviewer feedback without copying native
  conversation history into controller records.
- Show stale-version conflicts with a reload/reapply path. A late tab cannot
  reopen accepted work or overwrite a newer assignment accidentally.

**Acceptance journey:** create work → start session A → record partial progress →
end A → continue explicitly in session B → request review → return with feedback →
submit review again → accept. Work ID and progress survive browser/controller
restart; removed session history is represented honestly; private data stays
account-scoped. Include a lost launch reply and an offline machine case.

Use the existing synthetic session-board, launch, project and chat fixtures as
appropriate. Add new browser cases to a job with headroom, not terminal-browser.
Do not infer a paid-agent integration from a mocked UI journey.

## KZ8A — Add explicit MCP work access and read tools

**Outcome:** clients can discover and read only the work explicitly granted to
them, with old shelf credentials retaining their existing authority.

Tasks:

- Implement KZ5's grant model in hosted discovery, OAuth consent, manual
  credentials and per-call enforcement. New work permissions need explicit consent;
  existing credentials receive no additional work authority automatically.
- Bind access to account and permitted project(s). Recheck revocation, expiry,
  role and project access for each call. Read denial must not leak hidden IDs,
  progress entries, project names or terminal details.
- Add a small read surface, provisionally `reet_list_work` and `reet_get_work`,
  with steps, attempts, revision/review summaries, filters, bounded pages and compact
  summaries. Details are fetched separately and expose only the permitted context.
- Reconcile the stdio bridge's current REST-token path with the new work grants.
  Expose work tools there only when the same boundaries can be enforced. An
  unsupported transport reports that limitation instead of silently using broad
  REST credentials. Existing shelf tools remain compatible.
- Keep Browser WebMCP work exposure out of this slice unless its confirmation
  and grant contract is explicitly included and tested. UI opt-in is not a
  substitute for server enforcement.
- Extend generated definitions and KZ2 reporting when available; otherwise record
  a reproducible discovery/reply byte snapshot without waiting for the CI tooling.
  Discover only authorised tools; descriptions stay precise and small.

**Acceptance:** old credentials remain shelf-only; consent reduction, expiry,
revocation and account disable take effect; cross-account/project reads fail;
hosted discovery and supported bridges agree on shapes; clients can page work
without obtaining private prompts or unrelated session transcripts.

**First files:** `internal/api/mcp_access.go`, `mcp_oauth.go`, `mcp_server.go`,
`mcp_browser.go`, `internal/store/store_mcp*.go`, `mcp/src/` and MCP protocol tests.

## KZ8B — Let agents claim, report progress and request review

**Outcome:** an authorised agent participates in the same durable work records
as the browser, with atomic assignment and recoverable retries.

Proposed minimal additions are `reet_claim_work_step`, `reet_add_work_progress` and
`reet_submit_step_result`; final names and schemas follow KZ5 and the current MCP
contract, incorporating KZ3 changes if that supporting stage has shipped.
Avoid a generic update-everything tool that lets agents accept their own work,
change account/project ownership or launch sessions.

Tasks:

- Route all calls through the KZ6 domain operations and KZ8A grants. Keep claim
  selection and transition atomic, using the tested persisted replay record.
- Require a stable request ID for a claim and claim-generation proof for owned
  progress/review writes. Prevent duplicate progress entries when their result is
  lost; define request IDs for those writes too.
- An implementing step submits an artifact/result for review. A reviewing step
  submits its verdict/findings for the assigned artifact. Enforce the role and
  artifact identity; neither operation accepts the whole work item as done.
- Derive the actor from authenticated access. A client display name is a label,
  not ownership proof. Human-only tasks and ideas remain ineligible for agent claim.
- Return explicit no-work, conflict, revoked, stale-claim and retry guidance.
  Credential revocation blocks future writes without automatically reassigning work
  that may still be running on a machine.
- Give the client a short operational contract: read the item's context and
  acceptance criteria; claim eligible work; record useful progress; request review
  with evidence and limitations; leave acceptance to the person.
- Pass work/session context to an agent only through supported, explicit launch
  or tool configuration. Verify how credentials are delivered before adding an
  automatic connection. The first version can use a manually configured authorised
  MCP client; no token is inserted into a prompt, URL, repository or progress note.
- Keep claiming separate from execution. No terminal keystroke automation,
  implicit shell command, adapter enablement or skill installation is introduced.

**Acceptance:** two real protocol clients against a disposable local fixture
cannot claim the same eligible item; a lost-response retry returns the same claim;
duplicate progress/review requests are idempotent; stale/revoked principals cannot
write; agents cannot set done; the browser sees progress and can recover a stalled
claim. Measure discovery/reply costs against KZ2.

## KZ9A — Prepare artifacts and dispatch across machines

**Outcome:** a step can consume the exact preceding result on another permitted
machine, with observed preparation and a recoverable dispatch record.

Tasks:

- Implement the KZ5 artifact contract. A reachable immutable git commit is a
  useful first supported format; keep dirty-workspace transfer unavailable until
  a bounded verified patch/snapshot path exists.
- Bind preparation to the permitted account, destination connector, registered
  folder/project and artifact revision. Resolve repository identity explicitly.
  Reuse managed worktree infrastructure where it fits; never overwrite an
  occupied/dirty user workspace to satisfy a routing proposal.
- Record a durable prepare request and destination acknowledgement before the
  dependent step runs. Include prerequisites and toolchain/platform limitations;
  a matching source commit alone does not prove matching build dependencies.
- Use negotiated typed connector commands and fixed operations. Preserve existing
  mixed-version behaviour: unsupported peers report why dispatch is unavailable.
- Respect workspace exclusivity. Concurrent reviewers may use isolated prepared
  views; two writers cannot silently share a mutable worktree. Enforce read-only
  review execution only where the agent/runtime contract supports it.
- Recheck eligibility and the approved plan/assignment revision immediately before
  launch. A machine going offline or a model disappearing leaves an explicit
  blocked/unknown attempt; fallback follows recorded policy.
- Recover lost prepare/create replies by their stable request identities. Retain
  ambiguous attempts for reconciliation rather than dispatching a duplicate.

**Acceptance:** implementation produces R1 on A; B acknowledges R1 and runs its
assigned step. Wrong commit, dirty destination, unavailable dependency, old peer,
offline destination and lost reply all produce bounded recoverable states. Session
links identify the actual destination and observed artifact, not the plan's guess.

## KZ9B — Enforce independent reviews for the current revision

**Outcome:** review requirements can specify different agents/models and assess
the current output, with disagreement and stale results represented honestly.

Tasks:

- Add review-role policy: required role(s), agent/model independence constraints,
  permitted machines, scope and evidence requirements. Requested model identity
  and observed identity remain distinct; unknown observation is shown explicitly.
- Persist per-review verdict, findings, severity/location where applicable,
  unresolved questions, test evidence, reviewer attempt and artifact revision.
  Use bounded validated results rather than scraping arbitrary final-chat prose.
- Give reviewers the artifact, acceptance criteria and required evidence. Keep
  implementation discussion optional so an independent review need not inherit
  the implementer's confidence or conclusions.
- A revision R2 retains R1 reviews as history and makes them stale for R2 gates.
  Re-review may be targeted, but acceptance names which required current-revision
  checks were actually repeated and which evidence remains missing.
- Reconcile disagreement explicitly: route findings back for revision, request an
  additional perspective within policy, or ask the person. An extra passing
  reviewer cannot silently erase an unresolved blocking finding.
- Keep final human acceptance and any subsequent PR/merge action separate.
  Work Done records accepted outcome; it does not imply a merge or deployment.

**Acceptance:** a coding attempt and at least two configured review perspectives
produce attributable results; same-model review is refused when policy requires
another model; a changed revision invalidates prior passes; a late R1 review
cannot approve R2; disagreements and missing evidence remain visible. Demonstrate
different machines using KZ9A's acknowledged artifact.

## KZ10 — Bring an Eli-style coordinator into reet

**Outcome:** a person can discuss a goal, inspect fleet/work evidence and receive
a structured plan with proposed agent/model/machine assignments.

Deliver in bounded slices: provider/read-only conversation first, versioned plan
proposal second, existing dispatch integration third. KZ5A decides whether these
are small adaptations or a shared core with Zoomies.

Tasks:

- Adapt the provider/stream/conformance boundary and the account-owned provider
  configuration. Preserve per-provider data access, cancellation, request/output
  bounds, usage visibility and redaction. Do not copy Zoomies' instance identities,
  provider secrets or conversation records into reet.
- Offer an explicit allowlist of reet work/fleet read operations with the caller's
  existing permissions. Keep project source, artifact contents and other machine
  data subject to their recorded provider-access policy.
- Persist bounded coordinator conversations linked to work. Work/plan records
  remain authoritative if the conversation is deleted, shortened or rephrased.
  A page reload must not replay provider calls or dispatch work.
- Validate structured plan proposals against known step types, dependencies,
  capability/routing constraints and limits. Show assignments, reasons, evidence,
  likely unavailable choices and budget assumptions for review.
- Let the person edit the plan and explicitly start approved steps through the
  KZ7/KZ9A domain operations. Where prerequisites are absent, planning works but
  dispatch says what is missing. Provider text cannot directly issue arbitrary
  controller or connector commands.
- Let the coordinator explain blocked attempts, conflicting reviews and missing
  evidence. Display actual agent/model/machine activity rather than a generic
  “Gaffer is working” state that hides who is running.

**Acceptance:** a fake-backed streamed conversation proposes a valid work plan;
invalid/unknown routes are refused or shown unavailable; ownership/provider-data
boundaries hold; edited plan revisions remain consistent; cancellation ends a
model request; reloading restores work without repeating calls. Show one complete
manually approved implementation/review journey through delivered execution APIs.

## KZ11 — Add policy-driven routing and bounded automatic progression

**Outcome:** reet can choose among eligible assignments and advance approved work
within explicit limits, without requiring a human click between every safe step.

This is a later opt-in layer. Manual assignment and execution remain useful and
supported independently of it.

Tasks:

- Add saved role preferences and eligibility filtering before model-driven route
  suggestions. Explain selected/excluded candidates and allow an explicit override
  within capabilities and permissions. Route to compatible available resources;
  do not claim a universal “best model” from names or marketing.
- Persist approved plan/policy revisions, concurrency limits, allowed fallback,
  review independence, maximum attempts/review cycles, wall time and provider/tool
  usage limits. If spend is not measurable, display unknown and enforce the
  available request/usage/time caps rather than pretending to enforce a currency cap.
- A controller-owned durable dispatcher atomically admits ready steps, records
  intent before effects and reconciles connector outcomes. Several controllers or
  restart recovery must not run the same step twice; prove the actual deployment
  ownership/locking contract before enabling background dispatch.
- Allow opt-in progression from a successful implementation artifact to the
  required independent reviews, then back to a bounded revision attempt if policy
  permits. New scope, an unapproved route or an exhausted loop becomes Needs you.
- Pause/stop must prevent new admissions promptly. Cancel or retire existing
  machine attempts through the verified lifecycle contract; distinguish “stop
  requested” from observed termination. Preserve unresolved reviews and artifacts.
- Record route reasons and observed outcomes for later improvement. Compare
  models only on comparable tasks/evidence; a single passing review is not a
  quality ranking. Keep automatic fallback visible in the timeline.

**Acceptance:** deterministic eligibility fixtures; two simultaneous dispatchers;
restart after recorded intent and after a lost reply; unavailable requested model;
disallowed fallback; exhausted attempt/cost/time budget; stop during active review;
stale plan/policy revision; and one loop needing human input. All leave an honest,
recoverable work state with no silent model substitution or duplicate execution.

## KZ12 — Prove the complete workflow and document operations

**Outcome:** the integrated feature has evidence for its promised behaviour,
documented limits and a recoverable rollout route.

Tasks:

- Run the KZ7 acceptance journey through browser plus MCP clients using the same
  disposable controller records. Include restart, disconnect, late reply, revocation,
  two claimants, owner reassignment and a removed project/machine/session.
- Add one implementation on A, independent review on B and another review role,
  revision R2 with stale R1 reviews, explicit reconciliation and human acceptance.
  Exercise manual assignments before the optional policy-driven path.
- Exercise a local fake or stub-backed agent where needed. Separately record any
  real adapter/provider/device checks that are actually performed. Do not spend
  provider credits or enroll a user's fleet as an automatic test side effect.
- Verify instance and personal backup/restore, export/import, bounded growth and
  account cleanup with work items, progress, session links and claims present.
- Re-run KZ2's representative workload if available, or record a bounded resource
  and MCP payload comparison, to expose incremental costs. If KZ4 is delivered,
  confirm it covers newly included responses or documents its remaining scope accurately.
- Document stalled claims, explicit release/reassign, review/acceptance, unknown
  launch results, archival and restore invalidation. State which transports can
  participate and how people configure agent access.
- Use the repository's existing feature rollout convention if one exists. A
  deployment can leave new features unavailable until ready; disabling work UI or
  MCP must retain data and leave existing terminals usable. Document downgrade
  compatibility before calling rollback supported.
- Update the README and shipped-behaviour docs, then the roadmap and root handoff
  according to verified delivery. CI success alone does not publish a release or
  establish production adoption.

**Acceptance:** all promised transitions and failure paths have evidence; current
session, project, approval, question and shelf behaviour still passes its relevant
regressions; operational recovery is documented; unresolved platform/provider
checks are named. Release/merge/deployment state is recorded from actual records.

## Boundaries and deferred work

Keep one Go binary, one SQLite database and the existing embedded Svelte app.
Begin with personal work ownership, existing machine/folder contexts and explicit
assignments, then add verified cross-machine preparation and policy-driven
progression in the named stages. A new service or message broker is not required.

Deferred unless separately selected: arbitrary DAG/workflow editors, recurring
work, cross-instance federation, shared/team work permissions, automatic MCP injection
into every agent, and full worker-transcript indexing. A bounded staged dependency
model, cross-machine repository/artifact identity and opt-in progression are in
scope at their specified stages. Kanzo-style git synchronisation
of the controller database is outside this plan; reet retains its controller and
reviewed distribution model.

Reference images already have bounded draft/upload behaviour. Reuse that contract
for session launches; a second durable attachment store is a separate decision.
Preserve existing session IDs, request IDs, shelf stages, sync approval gates,
private transport negotiation and adapter capability gates throughout these stages.

## Validation and handoff discipline

For this documentation-only delivery, validate local links, source-path references,
stage IDs, internal anchors, whitespace and consistency with inspected code. No
application suite is required solely for prose.

For implementation, use targeted regressions while developing. Complete the
repository's current required checks before PR handoff; re-read the workflows
because requirements can change. At the inspected baseline the standard sequence is:

```sh
(cd web && npm run check && npm run build)
go vet ./...
go test -race ./...
(cd mcp && npm test)
git diff --check
```

Finish the web build before Go builds/tests: `web/dist` is embedded. Run relevant
browser/protocol/integration checks in addition to the standard sequence. Existing
examples include `web/tests/session-board.e2e.mjs`, MCP SDK/OAuth tests, and the
launch/session/chat fixtures. Use their documented server/port configuration and
disposable state. Validate claim races with separate database connections or
processes; a single in-memory client does not establish concurrency correctness.

After each selected stage, update this file's status and the root handoff's active
entry. Record:

```text
Task ID and scope:
Base SHA and overlapping active work reconciled:
Branch / commit / PR:
Behaviour added:
Compatibility / migration / permissions behaviour:
Measurements before and after, including method and units:
Checks actually run and their results:
Platform, device and live-provider checks not performed:
Known limitations or unresolved decisions:
Exact next unchecked task and first files to read:
Publication / deployment state, verified from actual records:
```

Leave a stage incomplete when its acceptance gate fails. Record the failing case
and recoverable state, then select the smallest remaining task. Preserve unrelated
local edits and other workstreams' progress entries.

## Resume instructions

When the KZ workstream is selected, start with KZ0. Read the current root handoff
first, inspect current main and local changes, verify existing progress and record
the one next slice. For the user's clarified orchestration goal, the next design
slice is KZ5A, then KZ5. KZ1–KZ4 can be selected separately as supporting work.
Reconcile integration timing with current structured-chat and recovery work.

Suggested continuation request:

> Read handoff.md and docs/work-orchestration-handoff.md. Reconcile KZ0 against
> current code and verified PR state, preserving active work. Implement only the
> selected next KZ slice, record its acceptance evidence and update both handoffs.

### Delivery record for this file

- Completed: source-grounded staged handoff and a short root-handoff index entry.
- User clarification incorporated: route steps across agents, models and machines,
  support independent revision-bound reviews, and evaluate Zoomies' Eli as the
  coordinator/provider/chat foundation.
- Application implementation: none.
- Runtime tests/provider/device checks: none for this documentation task.
- Documentation validation: stage definitions, Markdown fences, whitespace,
  root-to-companion link and locally available pinned source paths were checked.
- Publication: prepared for a documentation-only PR at the user's request. The
  associated GitHub PR record is authoritative for branch, commit and review state;
  creating the handoff does not establish a merge, release or deployment.
- Exact next task on selection: KZ0 baseline reconciliation.

## Source references

The fixed references below preserve the evidence behind the proposal. Use current
main and the latest shipped documentation when executing a stage.

| Source | Evidence |
| --- | --- |
| [reet root handoff](https://github.com/eyupio/reet.sh/blob/b10a46834c8e41f30cd0db577c467bfd8574d96f/handoff.md) | Existing priority, execution rules, code map and progress records |
| [Structured-chat plan](https://github.com/eyupio/reet.sh/blob/b10a46834c8e41f30cd0db577c467bfd8574d96f/docs/structured-chat-plan.md) | SC sequence, capability gates and recovery contract |
| [Roadmap](https://github.com/eyupio/reet.sh/blob/b10a46834c8e41f30cd0db577c467bfd8574d96f/docs/roadmap.md) | Richer run orchestration direction |
| [Session board](https://github.com/eyupio/reet.sh/blob/b10a46834c8e41f30cd0db577c467bfd8574d96f/docs/session-board.md) | Draft/session stages, idempotent launch, bounds and refresh behaviour |
| [Projects](https://github.com/eyupio/reet.sh/blob/b10a46834c8e41f30cd0db577c467bfd8574d96f/docs/projects.md) | Existing folder-project identity and browser-only access |
| [MCP contract](https://github.com/eyupio/reet.sh/blob/b10a46834c8e41f30cd0db577c467bfd8574d96f/docs/mcp.md) | Credential scopes, tool counts, output limits and supported transports |
| [MCP hosted implementation](https://github.com/eyupio/reet.sh/blob/b10a46834c8e41f30cd0db577c467bfd8574d96f/internal/api/mcp_server.go) | Discovery and text/structured result handling |
| [MCP tool definitions](https://github.com/eyupio/reet.sh/blob/b10a46834c8e41f30cd0db577c467bfd8574d96f/internal/api/mcp_tools.json) | Static discovery-size measurement input |
| [Binary installation](https://github.com/eyupio/reet.sh/blob/b10a46834c8e41f30cd0db577c467bfd8574d96f/docs/binary-installation.md) | Existing installer, native service, upgrade and recovery paths |
| [Home implementation](https://github.com/eyupio/reet.sh/blob/b10a46834c8e41f30cd0db577c467bfd8574d96f/web/src/lib/home/Home.svelte) | Independent refresh inputs, generation guards and last-good-data handling |
| [Kanzo MCP](https://github.com/emanuele-em/kanzo/blob/83a97df1140756bf698046e6d1b970e916ef1b2d/src/mcp.rs) | Short tools/replies, claims, progress and review guidance |
| [Kanzo storage](https://github.com/emanuele-em/kanzo/blob/83a97df1140756bf698046e6d1b970e916ef1b2d/src/store.rs#L742) | Atomic eligible-task selection and transition |
| [Kanzo browser](https://github.com/emanuele-em/kanzo/blob/83a97df1140756bf698046e6d1b970e916ef1b2d/src/app.js#L1139) | Change-token polling and request scheduling |
| [Kanzo resource measurements](https://github.com/emanuele-em/kanzo/blob/83a97df1140756bf698046e6d1b970e916ef1b2d/.github/resources/measure.py) | Repeated base/head measurements and MCP payload capture |
| [Kanzo resource workflow](https://github.com/emanuele-em/kanzo/blob/83a97df1140756bf698046e6d1b970e916ef1b2d/.github/workflows/resources.yml) | Separate unprivileged measurement job |
| [Zoomies Eli documentation](https://github.com/eyupio/zoomies/blob/b4c67a1413c8274334d18225df4053959861fbc0/docs/eli.md) | Providers, read-only chat tools, saved conversations and separately authorised repairs |
| [Eli provider contract](https://github.com/eyupio/zoomies/blob/b4c67a1413c8274334d18225df4053959861fbc0/internal/assistant/provider.go) | Provider-neutral stream, calls, usage and cancellation |
| [Eli provider conformance tests](https://github.com/eyupio/zoomies/blob/b4c67a1413c8274334d18225df4053959861fbc0/internal/assistant/contract.go) | Common observable adapter contract |
| [Eli tool boundary](https://github.com/eyupio/zoomies/blob/b4c67a1413c8274334d18225df4053959861fbc0/internal/controller/assistant_tools.go) | Explicit allowlist and bounded tool loop |
| [Eli conversation records](https://github.com/eyupio/zoomies/blob/b4c67a1413c8274334d18225df4053959861fbc0/internal/store/queries_assistant_conversations.go) | Account-owned, bounded saved coordinator conversations |
| [Eli repair workflow](https://github.com/eyupio/zoomies/blob/b4c67a1413c8274334d18225df4053959861fbc0/internal/controller/eli_repairs.go) | Separate durable repair/attempt lifecycle; a reuse reference, not a reet scheduler |
