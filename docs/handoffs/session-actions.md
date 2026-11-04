# Active session actions handoff

Updated 4 November 2026. PR: [#490](https://github.com/eyupio/reet.sh/pull/490). Branch: `feat/session-actions`.

## Behavior

- Separate session identity from frequent controls. Theatre view is a persistent header action with a pressed state;
  phones retain a visible caption, and exiting returns the original panels without replacing the session or connection.
- Keep interface, approval and terminal-creation actions in a wrapping toolbar at every screen size. Phones use 44 px
  targets; landscape shares a compact row where there is room. Disabled interface actions still explain why.
- Keep Rename, Copy branch, End session, Remove from history and worktree management in More. Remove duplicate interface,
  approval, theatre and terminal-creation entries. Existing confirmations and approval gates are preserved.
- Remove the duplicate Theatre action from the embedded terminal toolbar; standalone terminal panes retain their control.
- Continue with history is directly available for a recoverable closed session. Details and menu focus handling are retained.

## Validation

Web checks and build pass. The new responsive browser case covers desktop, tablet, 390 px and 320 px phones, and landscape
in light/dark themes; it checks visible actions, 44 px touch targets, bounds, menu contents, focus return and stream continuity.
The Sessions suite passes with updated expectations for controls that are now directly visible (the initial run and the corrected folder-trust-to-end sections).
Terminal workspace checks pass for renderer fit, theatre toggling and connection continuity. The phone suite passes with the deliberate landscape control-row budget and keyboard fit; Maximise remains available for more room. Web embedding tests pass.

Screenshots and detailed checks are in `docs/ui-verification.md`. Final PR review and final-head CI must pass before merge.
Record any review fixes and merge conflicts here; the public PR records the final merge state.

## Recovery and scope

This changes layout and discoverability, not launch, approval, mode-switch or session-end semantics. End session keeps
its confirmation. Creating a terminal still uses the existing folder and machine checks. Layout changes keep the current
chat/terminal mounted. No new dependency or persistent preference is introduced.


## Integration and review

Current main includes the merged live-controls PR #488, updated Gaffer artwork/motion and published documentation. The
UI verification append conflict is resolved by retaining both feature sections, and both sets of TerminalWorkspace props
are preserved. The prior CI failures came from a phone-dialog helper using a removed menu item and a duplicated New
terminal accessible name while the separate create form was open. The helper now uses the visible interface control;
the contextual direct action is labelled Terminal here so its purpose is distinct from the create-form submit button.

Desktop/tablet/phone light/dark screenshots are included under `docs/images/ui/session-actions-*.png`. Review and final-head
CI remain authoritative; merge only when green. No inline findings were open at the latest check.
The integrated head passes the full 23-dialog phone suite across six viewports, refreshed responsive controls checks
and web check/build. The legacy Start an agent suite also passes after the accessible-name fix. Full CI found two more continuation helpers
in chat and chat-recovery that used the removed menu item; they now exercise the visible header action and check that
the menu does not duplicate it. The complete chat/recovery browser suites are being rerun.

The full phone CI pass exposed an older Sessions-phone expectation for the previous hidden landscape header. It now checks
the visible Running/project/machine line, compact controls and their measured terminal-row budget. A first CI attempt
stalled in the unchanged launch suite; the full launch suite passes locally. Final-head CI remains the merge gate.

## Main integration on the current head

This branch now merges current main, which brought the delivered documentation pages, the public-page crawler test and the
resizable/collapsible session workspace panel (#495). Only `web/src/lib/sessions/SessionScreen.svelte` and
`web/src/lib/terminal/TerminalWorkspace.svelte` conflicted. Both keep every feature: the persistent Theatre action, the
wrapping interface/approval/Terminal here toolbar, the `Session actions` menu without duplicates, the docked panel's
separator, its saved width and open state and its sheet mode below 1100 px, and `holdSize` staying true while the panel
is being dragged. The combined preview branch resolved the same overlap the same way, so the three branches agree.

Web unit tests, Svelte check with zero errors and warnings, and the production build pass on the merged tree, as do the
Sessions phone, session workspace panel and terminal workspace browser suites. Final-head CI on this branch is the merge
gate; the branch head recorded in the pull request is authoritative.
