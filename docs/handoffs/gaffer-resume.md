# Gaffer continuation checkpoint — paused 6 November 2026

This is the current pause point. It supersedes older “next task”, unmerged-PR and
GM0/GM3/GM4 sequencing statements in historical handoffs. The user asked to pause;
no further implementation slice has been selected.

## Delivered and verified

- Owner-merged #625: https://github.com/eyupio/reet.sh/pull/625. Final head
  `60fc00f942474988a5ddadc7a8060e5ce9ad2a2d`; merge `23a62767d95bdc3c51e8cd12a0b03245875f058c`.
  All 20 applicable CI checks passed, with four intentional skips.
- Owner-merged #626: https://github.com/eyupio/reet.sh/pull/626. Final head
  `db4f4ff7c4d642b3b8789bb72a14f6491092bcc3`; merge `aa64c24f235928a1cc5a116905932d3dcd9ad394`.
  All 22 applicable CI checks passed, with four intentional skips. No unresolved
  inline review threads. The GitHub automated code reviewer could not run because
  the account review quota was exhausted; this is not a completed automated review.
- #620 offline runtime and #623 followed-session overview were merged earlier.
- #625 supplies the reference mascot treatment, Compact/Default/Expanded presets,
  modal full screen/phone layouts, saved left/right corner, temporary desktop
  dragging/resizing and keyboard movement. History is in the header; model choice
  is at the composer; narrative keeps the main area. The launcher reserves space
  around valuable actions, with a side strip in wider/landscape workspaces and a
  compact bottom strip on portrait phones. It steps aside for terminal and
  structured-worker keyboards, including visual-viewport-only keyboard changes.
- One reviewed “Look after this session” setup atomically saves needed provider
  session/text maxima, only this conversation/session's read and worker grants,
  and its bounded goal. Advanced controls are optional. Failed/stale setup rolls
  back every access change. Resume preserves budget/deadline and the ten-active-
  goal cap. General chat and MCP subjects retain separate permissions.
- #626 adds fake-backed real-controller HTTP/scheduler/store/sealed-key/private-
  WebSocket/broker/queue acceptance, without browser API interception. Native/race
  and non-root Docker runtime flows verify browser-free queued recovery, approval
  waits, scope isolation, pause/resume, limits, user completion, queued revocation
  and uncertain handoff without replay. Working model retains implementation.
- A benchmark gate falsely flagged identical store code in two runs. The workflow
  now compiles both revisions before sampling and interleaves eight one-second
  samples, counterbalancing starting order. Benchmark set, significance rule and
  15% gate are unchanged. Identical-code control: 234.6 vs 234.1 microseconds,
  p=0.959, n=8 per side; a synthetic 25% slowdown is rejected. Final CI passes.
- README, provider/workspace guides, concept handoff, acceptance contract and
  inspected synthetic UI captures are updated. Strict docs, 51-page static checks
  and 40-view docs browser smoke passed. Full Go race/vet and broad UI suites passed.

## Boundaries and unverified areas

Gaffer is the user's quirky critical buddy. The existing session model does
coding, commands, PR creation, check fixes and conflict resolution. Gaffer reads
permitted evidence and sends bounded next-turn instructions. It cannot answer
worker approvals, enable YOLO, interrupt automatically, merge, run shell commands
or acquire a direct GitHub-write path. An explicit browser interrupt is separate.

Each explicitly selected session has its own companion, goal and progress. Gaffer
uses an independent default provider/model with per-session override. Default
limits are 12 reviews / 24 hours, editable; reviews require fresh worker progress.
Quiet/busy/unavailable states spend no model calls. The worker must monitor and
report external PR/check progress; Gaffer does not independently poll GitHub.
Uncertain delivery is never automatically replayed. Only the user marks complete.

All model and worker evidence is synthetic. No real credentials, paid calls,
physical-device validation or deployment was performed by this agent. The restart
fixture reconstructs the controller and closes/reopens SQLite in process. Docker
uses the production runtime base/user, not the complete production CLI/image
packaging or OS service restart. Real-provider/backend parity, production packaging,
live PR supervision and physical/browser-device acceptance remain unverified;
consult the provider capability table before promising support. Older KZ durable
work orchestration, cross-machine routing and independent review remain separate.

## Resume procedure and standing preferences

1. Read `handoff.md`, this checkpoint, `docs/work-orchestration-handoff.md`,
   `docs/gaffer-ai.md`, and the widget/setup and offline-acceptance contracts.
2. Fetch current main and inspect current PR state/reviews/checks. Both #625 and
   #626 were merged by the owner; do not reopen their old implementation branches
   or repeat completed slices. Preserve unrelated upstream work.
3. Summarise the actual remaining capability/acceptance gaps and recommend the
   smallest next slice. No next slice is pre-authorised by this pause checkpoint.
   Ask needed decisions one at a time with clickable options, recommended first
   and labelled “(Recommended)”. Avoid routine confirmation requests.
4. Work only on reet.sh. Reference repositories are read-only. Use fakes until the
   user explicitly authorises real-provider setup/calls. Do not deploy, purchase
   credits or merge future PRs without explicit authorisation. Past owner merges
   are not merge authority for the agent. Do not spawn agents without permission.
5. Keep handoffs current and monitor the active PR's CI, review comments and
   conflicts during implementation. Preserve the worker/approval boundary and
   Gaffer's restrained Yorkshire personality.

At pause, main was `aa64c24f`. A documentation-only checkpoint branch
`docs/gaffer-pause-handoff` records this state; if its PR is unmerged, read that
branch before continuing. No runtime change belongs to this checkpoint.
