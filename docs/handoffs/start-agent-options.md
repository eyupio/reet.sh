# Startup and backlog model/effort handoff

Updated 4 November 2026. PR: [#493](https://github.com/eyupio/reet.sh/pull/493). Branch: `feat/start-agent-options`. Follow-up to merged live-controls PR #488.

## Behaviour and compatibility

Both the workspace Start an agent modal and the Home Start a session modal expose Model and Effort. The backlog composer
supports those choices at creation and edit, and retains them across save, reload and Start. Existing model catalogues,
favourites and configured IDs remain available. Reviewed Claude/Codex Gaffer plans are now supported alongside OpenCode;
folder-plugin trust remains specific to OpenCode.

Native effort suggestions come only from the selected machine's advertised thought-level values. Agent default remains
available. Missing options and older connectors show an explanation. Unsupported saved choices remain visible and can be
cleared to default; they cannot be silently dropped on launch. Terminal launches retain their fixed preset mappings.

Negotiate `acp-start-options-v1` separately from `acp-live-controls-v1`: previously released live-controls connectors still
reject structured create effort. Both controller and connector refuse unnegotiated startup effort. The selected model and
effort are confirmed by the actual session before readiness or its opening prompt. A definite refused admission clears
unconfirmed session metadata while retaining the operation receipt and saved backlog plan. No database migration is needed.

## Verification

Web unit checks, Svelte check and build pass on the initial implementation. The new desktop/phone browser test covers both
start dialogs and backlog save/reload/edit; legacy Codex and Start an agent suites pass. Focused race-enabled protocol,
broker, admission, saved-effort acknowledgement and backlog tests cover requested values, refusals and persistence.
Independent capability negotiation and the refusal regression pass; web check/build report zero Svelte errors or warnings,
and the final three-path browser suite passes on desktop and phone. Current main `ac5938d` integrated without conflicts. The terminal-browser workflow runs the new
`web/tests/launch-options.e2e.mjs` regression. Review and final-head CI must pass before merge.

## Recovery and remaining limits

If the selected model rejects an effort, choose an advertised value or Agent default and start again; the opening prompt
has not been delivered. Installation choices are suggestions; models may offer different runtime values. Agents that do
not advertise thought-level options use Agent default until their live session offers controls. These settings do not add
automatic model routing. No live paid-provider turn, physical phone, Safari or cross-platform release rollout was exercised.
Offline edit preserves an existing Gaffer plan and its unchanged model/effort even when capabilities are unavailable;
editing its prompt cannot silently convert it to a terminal plan. The browser regression covers this saved-plan recovery.
Rollback can hide `acpStartOptions` or revert this follow-up; retained backlog choices remain editable.

Screenshots and device checks are recorded in [UI verification](../ui-verification.md#startup-and-backlog-options-4-november-2026).

PR #490 is integrated as a dependency so final-head checks exercise the complete session experience. Both UI verification
appendices are retained. The complete chat/recovery and legacy Start an agent suites pass after switching test helpers to
the visible header controls. All seven affected backend packages pass their complete race suites, including the full API package (472 seconds). No inline review findings were open at the latest check.

Publication state and final-head checks are recorded on PR #493. Merge #490 first, then this follow-up when its checks and
review are green. Resolve any new main changes without dropping either handoff or UI verification section.

## Current head

The branch now merges the current session action branch, which itself carries main through the public-page crawler test and
the resizable session workspace panel (#495). That brings the Sessions phone expectation for the visible Running/project/
machine line and the landscape controls-row budget into this branch, so the shared phone suite no longer asserts the old
hidden header. The only conflict was `docs/handoffs/session-actions.md`; the session action branch's corrected text and its
new integration section are kept in full. The startup option code is unchanged by the merge: both start dialogs, the backlog
composer and the negotiated `acp-start-options-v1` contract are exactly as verified above.

Final-head CI was the merge gate and the head recorded in the pull request was authoritative. [PR #493](https://github.com/eyupio/reet.sh/pull/493)
merged as `918f452`; final head `906cb2d` was green across all twenty checks, including native service on ubuntu and macos,
local relay on ubuntu and macos, tmux minimum, private connection, image, binary release, terminal-browser, phone-dialogs,
web, test, source, build, CodeQL and the language analyses, with no review findings open.

On the merged head, web unit tests, Svelte check with zero errors and warnings and the production build pass, as do the
Start-agent/start-session/backlog selection regression, the Sessions phone suite and the Sessions suite. No startup option
code changed in the merge, so the earlier backend race suites and negotiation tests remain the evidence for the protocol,
broker, admission and backlog paths.
