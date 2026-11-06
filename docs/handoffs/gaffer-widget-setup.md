# Gaffer widget and one-step session setup

6 November 2026. The owner merged runtime #620 and overview #623. This slice
starts from main `1ba62a7c` and changes reet.sh only. The user asked for the same
panel sizes, mascot treatment and known floating location as the supplied cloud
reference, clear of valuable actions. They selected **one reviewed session setup,
with advanced controls optional**. Continue the fake-backed GM6 acceptance work
alongside this UX change; real provider credentials remain for later settings.

## Experience

- A single 56 px circular Gaffer launcher stays in its saved bottom corner on
  general pages and session workspaces. Page action docks and the session viewport
  reserve its space. The previous session-only inline entry is removed.
- Compact (24 × 32 rem), Default (28 × 42 rem), Expanded (46 × 52 rem), modal Full
  screen and left/right placement follow the reference, clamped to the viewport.
  Desktop header dragging, arrow-key movement and the native resize grip are
  available. Presets and side persist per browser account; manual geometry and
  full screen are temporary. Phones use a viewport-aware modal sheet.
- The flat-capped face sits on the reference warm circular tile in the launcher,
  header and replies, with the seated illustration in the welcome, with restrained greeting/thinking motion and reduced
  motion/user-animation preferences. The working-session mascot is unchanged.
- History stays in the header, model selection stays beside the composer, and the
  narrative remains the main area. A session's **Look after** control opens one
  review of the selected provider/model, data access, agreed goal and visible
  default bounds (12 reviews / 24 hours). Limits remain editable.
- Detailed Context, Worker messages and provider maxima remain optional advanced
  controls. Normal chat remains read-only unless a separate worker relay was
  explicitly selected. The saved goal has its own bounded messaging authority.

## One reviewed operation

`POST /api/v2/gaffer/conversations/{id}/goal/setup` is writer-browser-only. It
requires explicit confirmation and exact conversation, provider/model, goal and
grant revisions. It validates the owned unarchived structured session and its
advertised live worker channel. In one transaction it enables the provider's
session/text maxima when needed, binds only this conversation's context and worker
grants, then starts or resumes its goal. A conflict rolls back the entire operation.
Other conversations and MCP/browser clients gain no grant.

Resume preserves the existing goal ID, budget/deadline and waits for fresh worker
progress. Both legacy and one-step resume honour the ten-active-goal limit.
Pausing/stopping, delivery fences, independent working model, worker approvals,
source receipts and the prohibition on direct shell/GitHub/merge actions remain.

## Validation and remaining gate

The atomic HTTP test passes confirmation, late-CAS rollback, stale revisions,
provider maximum changes, separate-session isolation, resume preservation and
foreign-origin/MCP denial. Existing goal/store/API race selections pass after the
transaction helper refactor. The widget matrix passes ten theme/width views with
stable clearance, presets, resize reset, keyboard positioning, full screen, phone
sheet, drafts and focus. Chat/context/worker/goal/settings/overview browser regressions also pass. Source UI captures were visually inspected; strict docs, 51-page static checks
and the 40-view browser smoke pass. Final-head CI/review remains the publication
gate. Initial CI found two geometry-test assumptions: modal drawers were
incorrectly clipped to terminal ancestors, and the save-dock check expected the
old launcher placement. Both are corrected with the visible top-layer and stable
corner contracts retained. Targeted goal/API/store race tests and vet pass.

GM6's separate acceptance harness already proves a real private WebSocket/broker
queued instruction survives database reopen and reaches its worker once without a
browser. Its native/container browser flow must now use this simpler setup and
exercise approvals, bounds, revocation, ambiguity, scope isolation and recovery.
