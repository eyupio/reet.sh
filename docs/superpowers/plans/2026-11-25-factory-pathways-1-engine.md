# Factory pathways, part 1: stages, storage and starting work — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A saved pathway (an ordered line of built-in and custom stages, each with who does it, skills and a prompt) can be created through the API and an outcome can be started on it.

**Architecture:** Pathways are one per-account table. Starting work copies a pathway's stages into the outcome's pipelines; the existing supervisor runs them. One new stage kind, `custom`, carries a name and prompt. A stage is named everywhere by its *label*: the kind for a built-in stage, the name for a custom one.

**Tech Stack:** Go (`internal/store`, `internal/gaffer`, `internal/api`), SQLite, Svelte (two small display changes only).

**Spec:** `docs/superpowers/specs/2026-11-25-factory-pathways-design.md`. Parts 2 (list, canvas, dialogs) and 3 (Gaffer's suggestion, MCP tools) get plans of their own.

## Global Constraints

- British spelling, plain words, no em dashes in UI copy. Commit subjects are an imperative sentence with no prefix.
- No new dependency.
- Limits, verbatim from the spec: name 1 to 60 plain characters, unique in the account without case; description up to 300 bytes; 1 to 12 stages; custom stage name 1 to 40 plain characters, unique in the pathway, never a built-in kind's name (any case); prompt up to 4,000 bytes, required for custom; up to 2 skills and 2,000 bytes of guidance together a stage; at most 50 pathways an account; a custom stage's goal carries its prompt clipped to 1,200 bytes.
- Order rules: a built-in kind at most once; merge, if present, last; babysit and merge each need pr before them.
- Standard pipelines (no pathway) must behave exactly as before: every existing test passes unchanged.
- Do not skip, retry or lengthen a timeout to get green.
- The pull request adds `docs/handoffs/log/PATHWAYS-1.md` and `changelog.d/pathways-engine.md`; it does not edit `CHANGELOG.md` or `handoff.md`.

## Review Focus

1. A custom stage named `Merge`, ` merge ` or `PR`: refused at save, so nothing can pass for a gate by name. (Task 2)
2. An outcome saved before this change (stages with no `name`): its stages still label, send back and move on by kind. (Task 3)
3. A pathway edited or deleted while an outcome started from it is running: the outcome's stages are unchanged. (Task 5)
4. A 4,000-byte custom prompt of multi-byte characters with a long pipeline prompt: the stage prompt stays within `MaxOutcomePromptBytes`, is valid UTF-8 and still ends with the boundary ("Do not merge."). (Task 3)
5. Two saves of one pathway from two tabs: the second, carrying the old `updatedAt`, is refused and changes nothing. (Task 1, Task 6)

## File structure

- Create `internal/store/store_gaffer_pathways.go` (+ `_test.go`): types, table, reads and writes.
- Create `internal/gaffer/pathways.go` (+ `_test.go`): validation, the service's pathway methods, `UsePathway`.
- Modify `internal/store/store_gaffer_outcomes.go`: stage `Name`, `Prompt`, `Label()`; pipeline `PathwayID`, `PathwayName`.
- Modify `internal/gaffer/outcome_runs.go`, `outcome_manage.go`, `outcome_delivery.go`, `outcomes.go`: custom stages and labels.
- Create `internal/api/gaffer_pathways.go` (+ `_test.go`): routes. Modify `internal/api/api.go`, `gaffer_outcomes.go`: register, accept `pathway`.
- Modify `web/src/lib/gafferOutcome.ts`, `web/src/lib/factory/*.svelte`: show a custom stage by name; hide "Save as template" for a pathway outcome.

---

### Task 1: Store pathways

**Files:** Create `internal/store/store_gaffer_pathways.go`, `internal/store/store_gaffer_pathways_test.go`. Modify `internal/store/store.go` (call `migrateGafferPathways` after `migrateGafferTemplates`, line 451).

**Interfaces — Produces:**

```go
const MaxGafferPathways = 50
var ErrGafferPathwayChanged, ErrGafferPathwayLimit, ErrGafferPathwayName error // changed since read; 50 reached; name taken

type GafferPathwayStage struct {
	Kind   string            `json:"kind"`             // a built-in kind or "custom"
	Name   string            `json:"name,omitempty"`   // custom only
	Prompt string            `json:"prompt,omitempty"`
	Tier   string            `json:"tier,omitempty"`   // set, or Agent is
	Agent  *GafferStageAgent `json:"agent,omitempty"`  // existing type: tool, model, effort, provider
	Skills []string          `json:"skills,omitempty"` // skill ids
}
type GafferPathway struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Enabled     bool                 `json:"enabled"`
	Stages      []GafferPathwayStage `json:"stages"`
	CreatedAt   int64                `json:"createdAt"`
	UpdatedAt   int64                `json:"updatedAt"`
}
type GafferPathwayRef struct{ ID, Name string }

func (s *Store) GafferPathwaysFor(uid int64) ([]GafferPathway, error)            // by name, case-insensitive
func (s *Store) GafferPathwayFor(uid int64, id string) (GafferPathway, error)    // ErrNotFound
func (s *Store) SaveGafferPathwayFor(uid int64, p GafferPathway, readAt int64) (GafferPathway, error)
func (s *Store) DeleteGafferPathwayFor(uid int64, id string) error               // ErrNotFound
```

Table: `gaffer_pathways(id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, name TEXT NOT NULL COLLATE NOCASE, enabled INTEGER NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, data TEXT NOT NULL CHECK(json_valid(data)), UNIQUE(user_id,name))`. `data` holds description and stages. `SaveGafferPathwayFor` with `readAt == 0` inserts (limit and name checked); otherwise updates the row whose `updated_at == readAt`, else `ErrGafferPathwayChanged`. `UpdatedAt` is always later than the value it replaces (`max(now(), old+1)`), so two saves in one millisecond still differ.

- [ ] **Step 1: Write failing tests** in `store_gaffer_pathways_test.go`, using the test store helper the template store tests use:
  - `TestPathwayRoundTrip`: save, read back equal (stages, skills, agent), listed.
  - `TestPathwayNameIsUniqueWithoutCase`: "Ship" then "ship" gives `ErrGafferPathwayName`; another account may use it.
  - `TestPathwayStaleSaveIsRefused`: save, update with its `UpdatedAt` succeeds, a second update with the first `UpdatedAt` gives `ErrGafferPathwayChanged` and the stored name is the first update's.
  - `TestPathwayLimit`: the 51st gives `ErrGafferPathwayLimit`.
  - `TestPathwayOfAnotherAccountIsNotFound`: read, save and delete by a second uid give `ErrNotFound`.
- [ ] **Step 2:** `go test ./internal/store -run Pathway` fails to compile.
- [ ] **Step 3:** Implement as above, following `store_gaffer_templates.go` for scanning and transactions.
- [ ] **Step 4:** `go test ./internal/store -run Pathway` passes.
- [ ] **Step 5:** Commit "Store Factory pathways".

### Task 2: Validate and manage pathways

**Files:** Create `internal/gaffer/pathways.go`, `internal/gaffer/pathways_test.go`.

**Interfaces — Consumes:** Task 1. **Produces:**

```go
const (
	CustomStage            = "custom"
	MaxPathwayStages       = 12
	MaxPathwayNameRunes    = 60
	MaxStageNameRunes      = 40
	MaxPathwayDescBytes    = 300
	MaxStagePromptBytes    = 4000
	MaxStageSkills         = 2
	MaxStageGuidanceBytes  = 2000
)
// validPathwayOrder reports the first broken order rule, for labels in order; "" when sound.
func validPathwayOrder(kinds []string) string
// cleanPathway trims and checks p, returning it ready to save, or *Invalid naming field "name", "description" or "stages".
func cleanPathway(p store.GafferPathway) (store.GafferPathway, error)
func (s *Service) Pathways(uid int64) ([]store.GafferPathway, error)
func (s *Service) Pathway(uid int64, id string) (store.GafferPathway, error)
func (s *Service) SavePathway(uid int64, p store.GafferPathway, readAt int64) (store.GafferPathway, error) // ID "" makes one (newChatID)
func (s *Service) DeletePathway(uid int64, id string) error
```

Each service method starts with `s.chatPrincipal(uid)` as `Templates` does. `cleanPathway` checks: name with `terminalproto.ValidTitle` and the rune limit; description bytes and `store.ValidWorkerText`; 1 to 12 stages; kind is in `StageKinds` or `custom`; a custom stage has a name (valid title, 40 runes, not equal to a kind or `custom` under `strings.EqualFold`, unique under `EqualFold`) and a prompt; a built-in stage has no name; prompt at most 4,000 bytes and `terminalproto.ValidPrompt`; exactly one of tier (`validStageTier`) or agent (`terminalproto.ReviewedChatTool`, `ValidModel` for model and effort, provider only where `ToolTakesProvider`), an empty pair meaning tier `DefaultStageTier(kind)` (`standard` for custom, so add that case to `DefaultStageTier`); at most 2 skill ids, each non-empty, no repeats; then `validPathwayOrder`. Skill existence and guidance size are checked where the loader is (Task 5), not here.

- [ ] **Step 1: Write failing tests** (table-driven, `TestCleanPathway`), each asserting the `Invalid` field or success:
  - sound: `plan, code, custom"Security audit", pr, merge`; `review, code` (review before code); a single custom stage.
  - refused (`stages`): no stages; 13 stages; `code, code`; `merge, code`; `babysit` with no pr before; `merge` with no pr; custom with no prompt; custom named `Merge`, ` merge `, `PR`, `custom`; two customs `Audit` and `audit`; a built-in stage with a name; 4,001-byte prompt; three skills; a repeated skill; both tier and agent; tier `huge`.
  - refused (`name`): empty; 61 runes. Refused (`description`): 301 bytes.
  - `TestCleanPathwayDefaultsTier`: a custom stage with neither comes back with tier `standard`, a plan stage with `heavy`.
- [ ] **Step 2:** `go test ./internal/gaffer -run Pathway` fails to compile.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Passes.
- [ ] **Step 5:** Commit "Check a pathway's stages and order".

### Task 3: Custom stages run

**Files:** Modify `internal/store/store_gaffer_outcomes.go` (stage and pipeline fields), `internal/gaffer/outcome_runs.go` (`OutcomeStageInput`, `outcomePieces` :585-676, `stagePrompt` :309, `StageGoal` :282, `partTitle` caller), `internal/gaffer/outcomes.go` (`OutcomeSession`). Test: `internal/gaffer/pathway_stages_test.go`.

**Interfaces — Produces:**

```go
// store.GafferOutcomeStage gains:
Name   string `json:"name,omitempty"`   // a custom stage's name
Prompt string `json:"prompt,omitempty"` // custom: its work; built-in: extra instructions
func (g GafferOutcomeStage) Label() string // Name when Kind == "custom" and Name != "", else Kind

// store.GafferOutcomeSession gains:
PathwayID   string `json:"pathwayId,omitempty"`
PathwayName string `json:"pathwayName,omitempty"`

// gaffer.OutcomeStageInput gains, set only by a pathway (no request body can):
Name   string `json:"-"`
Prompt string `json:"-"`
// gaffer.OutcomeSession gains:
Pathway *store.GafferPathwayRef `json:"-"`

func StageGoal(objective, title string, stage store.GafferOutcomeStage) string // was (objective, title, kind string); update its callers
```

Behaviour:
- `outcomePieces`: for an item with `Pathway != nil`, check `validPathwayOrder` over the stages' kinds in place of the fixed-order check, refuse any part or brief, and set `piece.PathwayID/PathwayName`. For an item without, a `custom` kind stays refused as an unknown kind does now. Copy `Name` and `Prompt` on to the stored stage.
- `stagePrompt`: for custom, the opening is `You are doing ONE stage of a staged piece of work: the “<name>” stage. <prompt>`; the finish line uses the deliverable `what this stage’s instructions ask for`. For a built-in stage with a prompt, add `\n\nExtra instructions for this stage:\n<prompt>` after the built-in work. Handover and "later stages" text use `Label()`. The boundary (`end`) is unchanged and still last; when over the limit the pipeline brief gives way first as now, and the lead text is clipped with `clipTo` (rune-safe) as now.
- `StageGoal`: `Stage: <label>`; for custom the work is the prompt through `clipBytes(prompt, 1200)`.

- [ ] **Step 1: Write failing tests:**
  - `TestLabelFallsBackToKind`: `GafferOutcomeStage{Kind:"code"}.Label()=="code"`; `{Kind:"custom",Name:"Audit"}.Label()=="Audit"`; `{Kind:"custom"}.Label()=="custom"`.
  - `TestPathwayPipelineKeepsItsOrder`: `outcomePieces` with a `Pathway` ref and stages `review, code, custom"Audit", pr, merge` returns stages in that order with `Name`/`Prompt` kept and `PathwayID` set.
  - `TestStandardPipelineStillRefusesReorderAndCustom`: the same stages with no ref give the existing "Keep the stages…" error; a lone `custom` with no ref is refused.
  - `TestCustomStagePrompt`: contains `the “Audit” stage`, the prompt, `what this stage’s instructions ask for`, names later stages `pr, merge`, ends with ` Do not merge.` before any image suffix.
  - `TestBuiltInStageExtraInstructions`: a code stage with a prompt contains the built-in code wording, then `Extra instructions for this stage:` and the prompt.
  - `TestLongCustomPromptKeepsTheBoundary` (Review Focus 4): prompt of 4,000 bytes of `é`, pipeline prompt of 8,000 bytes: `len(out) <= MaxOutcomePromptBytes`, `utf8.ValidString(out)`, contains `Do not merge.`.
  - `TestCustomStageGoal`: contains `Stage: Audit` and the first 1,200 bytes of a 2,000-byte prompt, not the rest.
- [ ] **Step 2:** `go test ./internal/gaffer -run 'Pathway|Label|CustomStage|BuiltInStage|LongCustom|StandardPipeline'` fails.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Those pass, and `go test ./internal/gaffer ./internal/store` passes whole.
- [ ] **Step 5:** Commit "Run a custom stage in a pathway pipeline".

### Task 4: Name a stage by its label everywhere

**Files:** Modify `internal/gaffer/outcome_manage.go` (move-on :530, send-back :702 and its rebuild :760, notes), `internal/gaffer/outcome_delivery.go` (:433-447, :912), and any other place `rg -n '\.Kind\b' internal/gaffer internal/api/gaffer_*.go` shows a stage's kind being *shown to a person or agent, or compared with a `stage` argument*. Leave alone every comparison with the literal `"merge"` or `"code"` and every `stageWork`/tier lookup: those are behaviour, and stay on kind. Test: add to `internal/gaffer/pathway_stages_test.go`.

- [ ] **Step 1: Write failing tests**, built on the existing send-back and move-on test helpers in `outcome_manage_test.go`:
  - `TestSendBackToACustomStage`: a pathway pipeline at `pr` sent back to `Audit` lands on the Audit stage, state `pending`, with its `Name` and `Prompt` still set (the rebuild copies them), and the note reads `Sent back to the Audit stage. Starting it again.`
  - `TestMoveOnNamesACustomStage`: `MovePipelineOn(..., "Audit")` moves on from the Audit stage; with `"code"` it is refused with the message naming `Audit` as where the pipeline is.
  - `TestDeliveryListsCustomStagesApart`: stages `code, custom"A", custom"B", merge` list `["code","A","B"]`, default `code`.
  - `TestOldOutcomeStillSendsBackByKind` (Review Focus 2): a stored pipeline with no `Name` anywhere sends back to `code` as before.
  - `TestMergeAfterACustomStageStillWaits`: a pathway pipeline `code, custom"A", pr, merge`, at merge with `MergeApproved` false and auto-merge off, is `ready_to_merge` and starts no session.
- [ ] **Step 2:** They fail. **Step 3:** Replace `.Kind` with `.Label()` at the places found; add `Name`, `Prompt` to the send-back rebuild; in the delivery list drop the neighbour merge only when labels are equal. **Step 4:** `go test ./internal/gaffer` passes whole. **Step 5:** Commit "Name a stage by its label when sending back and moving on".

### Task 5: Start work on a pathway

**Files:** Modify `internal/gaffer/pathways.go`, `internal/api/gaffer_outcomes.go` (the start handler and the add-pipelines handler at :483). Tests: `internal/gaffer/pathways_test.go`, `internal/api/gaffer_pathways_test.go`.

**Interfaces — Produces:**

```go
// UsePathway gives every session the pathway's stages as it is saved now, with each stage's skills loaded.
// Refused: a session that already has stages; a missing or disabled pathway; a skill that load cannot give,
// or a stage whose guidance together passes MaxStageGuidanceBytes (the message names pathway, stage and skill).
func (s *Service) UsePathway(uid int64, id string, sessions []OutcomeSession, load SkillLoader) error
// PathwayProblems lists, for the pathway list's "!" mark, what would refuse a start: one sentence each, empty when sound.
func (s *Service) PathwayProblems(p store.GafferPathway, load SkillLoader) []string
```

A stage on a tier becomes `OutcomeStageInput{Kind, Name, Prompt, Tier, Skills}` (no tool, so the crew resolves it with its backups); a stage on an agent sets `Tool, Model, Effort, Provider` and no backup. Both request bodies gain `pathway string` beside their existing fields (decode into a wrapper struct; do not add it to `OutcomeInput`). With it set the handler calls `UsePathway(uid, pathway, in.Sessions, s.templateSkills(uid))` before the existing call. Errors go through the existing `gafferDeliveryError`.

- [ ] **Step 1: Write failing tests:**
  - `TestUsePathwayFillsStages`: two sessions each get the pathway's five stages, `Pathway` ref set, tier stage has no `Tool`, agent stage has its model and no backup, skills loaded in order.
  - `TestUsePathwayRefusals`: a session with stages; a disabled pathway (message says it is switched off); an unknown id; a loader error for skill `lint-rules` (message contains the pathway name, the stage label and `lint-rules`); two skills of 1,200 bytes each.
  - `TestRunningWorkKeepsItsStages` (Review Focus 3): start an outcome on a pathway with the fake starter the outcome tests use, then save the pathway with a stage removed, then delete it; the stored outcome's stages and `PathwayName` are unchanged.
  - API: `TestStartOutcomeOnAPathway` posts `{"pathway": id, ...}` and reads back stages by label; `TestStartRefusesStagesWithAPathway` posts both and gets the refusal status the handler gives an `Invalid`.
- [ ] **Step 2:** fail. **Step 3:** implement. **Step 4:** `go test ./internal/gaffer ./internal/api -run 'Pathway|RunningWork'` passes. **Step 5:** Commit "Start an outcome on a pathway".

### Task 6: Pathway routes

**Files:** Create `internal/api/gaffer_pathways.go`; extend `internal/api/gaffer_pathways_test.go`. Modify `internal/api/api.go:431` (register after the template routes).

Routes, behind `s.featureAccess` with words "Factory pathways need a browser account." / "Sign in to use Factory pathways." / "This account is read-only, so it cannot use Factory pathways.":

- `GET /api/v2/gaffer/pathways` → `{"pathways":[{...pathway, "problems":[...]}]}` (problems from `PathwayProblems`).
- `POST /api/v2/gaffer/pathways` → 201, the pathway. `GET /api/v2/gaffer/pathways/{id}`.
- `PUT /api/v2/gaffer/pathways/{id}` body: the pathway with the `updatedAt` it was read at. `DELETE /api/v2/gaffer/pathways/{id}` → 204.
- Errors: `ErrGafferPathwayChanged` → 409 code `pathway_changed`; `ErrGafferPathwayLimit` → 409 `pathway_limit`; `ErrGafferPathwayName` → 409 `pathway_name`; `ErrNotFound` → 404 "That pathway is not available."; else `gafferDeliveryError`.

- [ ] **Step 1: Write failing tests:** create then list (enabled by default when the body omits it is **not** assumed: the body says); `TestPathwayStalePutIs409` (Review Focus 5: second PUT with the first `updatedAt` gets 409 `pathway_changed` and a GET shows the first PUT's name); invalid stages get the status an `Invalid` gets with field `stages`; another account gets 404; a read-only account is refused; a list entry whose skill the loader refuses carries one problem sentence.
- [ ] **Step 2:** fail. **Step 3:** implement, following `gaffer_templates.go`. **Step 4:** `go test ./internal/api -run Pathway` passes. **Step 5:** Commit "Serve pathways over the API".

### Task 7: Show custom stages, and the notes

**Files:** Modify `web/src/lib/gafferOutcome.ts` (stage type gains `name?: string`; add `export const stageName = (stage: { kind: string; name?: string }): string => stage.kind === 'custom' && stage.name ? stage.name : stageLabel(stage.kind)`), then use it wherever `web/src/lib/factory/*.svelte` and `web/src/lib/home/*.svelte` render `stageLabel(stage.kind)` for a stored stage (`rg -n 'stageLabel\(' web/src`). Where a `stage` value is *sent* (send-back, move-on), send `stage.name ?? stage.kind` for a custom stage. Hide "Save as template" when any pipeline of the outcome has `pathwayId`. Show "Pathway: <name>" in the outcome's settings. Test: `web/src/lib/gafferOutcome.test.mjs` (or the test file that already covers `stageLabel`). Create `docs/handoffs/log/PATHWAYS-1.md`, `changelog.d/pathways-engine.md`.

- [ ] **Step 1:** Failing unit test: `stageName({kind:'custom',name:'Audit'}) === 'Audit'`, `stageName({kind:'pr'}) === 'PR'`, `stageName({kind:'custom'}) === 'custom'`.
- [ ] **Step 2:** `cd web && npm ci` (if `node_modules` is missing) and run the web unit tests: fails.
- [ ] **Step 3:** Implement. **Step 4:** `cd web && npm run check` passes; `go vet ./... && go test ./...` passes.
- [ ] **Step 5:** Write the handoff entry (what, why, checked, not done: no UI to build a pathway yet, no Gaffer or MCP use yet) and the changelog fragment per `changelog.d/README.md`. Commit "Show a custom stage by its name", push, and update pull request 940's description.
