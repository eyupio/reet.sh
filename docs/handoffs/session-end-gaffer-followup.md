# Session end and Gaffer follow-up repair

## Report and cause

The session workspace kept a locally ended session selected after the machine confirmed closure, leaving the stopped conversation and multiple ended notices in the foreground. Keeping its history was incorrectly coupled to keeping its view open.

The Gaffer screenshot was a general conversation with status-only shared context. That ordinary turn intentionally has no worker tool. Its system instructions stated the limitation without explaining the separately granted bounded session follow-up feature, and the general view offered no direct way to choose a worker for that feature. Existing goal and relay APIs remain available.

## Behavior

- Record a local end intent. Keep uncertain, running, pending and lost sessions visible until a successful listing confirms ended/exited or removal.
- Once confirmed, dismiss that session's tab and stale machine-selection memory. Keep the user's newer selection if they have moved elsewhere. Otherwise choose another live open tab, or return to the Sessions list when none remains.
- Retain history unless the person's existing removal preference says otherwise. Explicitly reopening a stopped session still shows its retained output and continuation controls. Background/natural completion does not dismiss an unrelated view.
- General Gaffer chat provides Follow a session. A read-only picker lists running structured chats and opens the chosen conversation with Gaffer. The existing Look after this session dialog still owns grants, goal, budget and expiry. Selection alone enables nothing and sends no worker instruction.
- Ordinary and context-sharing model prompts distinguish the current read-only turn from the separate session goal feature. They explain the actual UI path and bounded, progress-driven reviews without claiming a goal is active or bypassing approvals.

## Verification

A desktop/phone browser regression covers general-to-session follow-up with zero implicit grant/worker writes, confirmed-end neighbour selection and last-tab dismissal, refusal/pending/lost holds, retained-history reopening and late confirmation after an uncertain response. It is included in terminal browser CI. Existing terminal lifecycle coverage also verifies that delayed stop and archive responses preserve a newer selection. Review found that replacement selection could re-save the ended outgoing session under its old machine; a cross-machine regression reproduced it on the initial PR head. Clearing the outgoing selection before choosing the replacement prevents that stale memory from returning when the original machine is selected.

The full Gaffer backend race suite passed. The real fake-provider integration checks ordinary and shared-context prompts both explain separately granted follow-up while their outgoing tool schemas remain empty. The final combined-main desktop/phone regression, full lifecycle suite, full companion-chat suite (including the original message-space requirement), and all twelve widget layout cases passed. Existing goal, worker messaging and followed-session suites also passed. Svelte check reports zero diagnostics and the production build passes. The real authenticated browser/broker/private-connector/tmux acceptance also passed: original job/PID continuity, two-host offline end, history archival and no recreated shell. Inspect exact-head CI before merge.

## Coordination and continuation

The Home projects PR #641 merged after all six workflows passed (`0f98e31d`). This repair is based on that main and keeps its compact projects behavior. It introduces no database or protocol migration and does not change worker/model selection, native view switching or worktree/archive contracts.

Continue branch `fix/session-end-gaffer-context` in `/workspace/work/reet-session-regressions`. If this environment survives, activate `/workspace/.reet-tools/activate.sh`. Never commit the untracked node_modules symlink. Read current PR/checks and this handoff, inspect final logs under `/workspace/work/redesign-review/session-regressions-*`, and finish remaining combined checks. The user has authorized merging ready, green changes; do not merge a pending/failing head. Do not confuse this ordinary-chat guidance with authorization to send worker instructions or enable goals automatically.
