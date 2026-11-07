# Continuation prompt: session workspace and shared native views

Continue work in eyupio/reet.sh on branch `feat/calm-session-workspace`. Read `docs/handoffs/calm-workspace-redesign.md` first, including remaining merge gates. Design PR #635 is merged; the implementation PR [#637](https://github.com/eyupio/reet.sh/pull/637) is a draft. The user approved implementation, requires true Chat / Terminal switching that preserves the running agent, and asked us to account for #636. Do not name the external design reference in PR text or handoff documents.

Start by fetching the branch, checking git status and repository instructions, and inspecting the current implementation PR and #636 status/head. Preserve other people's work. The previous isolated checkout is `/workspace/work/reet-session-implementation`; its untracked `web/node_modules` is a symlink and must not be committed. Toolchain activation, if this environment survives: `source /workspace/.reet-tools/activate.sh`.

The existing checkpoint includes a calm session header, Session actions sheet, saved optional Details, quieter rows/composer, and a shared-native-view foundation. Read `internal/acpagent/native_view.go`, the native-view files in terminalproto/broker/agent, terminalhost attachment lifecycle, and `SessionViewSwitch.svelte`, `SessionScreen.svelte`, `TerminalWorkspace.svelte`, `TerminalPane.svelte`. Primary view switching must never call the runtime mode endpoint, restart a worker, allocate a worktree or change its directory. Both panes stay mounted after the first native attachment. The separately labelled restart operation retains its explicit legacy confirmation.

Native support is currently exactly OpenCode 1.19.5 with a connector-local authenticated loopback service hosted by the same ACP worker. A disposable TUI client attaches to that worker/conversation; credentials never enter browser payloads or process argv. Other agents and versions need honest unavailable reasons. Do not claim broad support.

Prioritize the remaining native reconciliation tests and fixes: real terminal-origin prompts, bidirectional output, shared approval/question ownership, late/duplicate replies, SSE reconnect stale busy recovery, failed-prompt echo cleanup, permission races and model/mode synchronization. Then verify full broker/host lease/takeover/reconnect/teardown behavior. The passing direct PTY probe proves worker/client continuity but not the whole product path.

Account for #636, reviewed at f91945fb47db3f49225e23c5b691d04e3a3afbab. It was not merged into this branch; a merge-tree simulation was textually clean. Preserve its worktree archive fingerprint/count confirmation and branch retention. At that head structured launches gained project/worktree fields that structured protocol validation/launch preparation did not yet support; re-check current fixes and resolve combined behavior before acceptance. Test structured launch, retries, archive and same-worker-directory views together. Never allocate another worktree when changing views.

Verification already passed after rebasing onto main e1c4ed9b:

- `cd web && npm run check && npm run build` (zero Svelte diagnostics).
- `go vet ./...`.
- `go test -race ./internal/acpagent ./internal/terminalproto ./internal/terminalbroker ./internal/terminalagent ./internal/agenthealth -run '^TestNative|^TestCapabilities|^TestTickets' -count=1`, with `REET_OPENCODE_NATIVE` pointing to the checksum-verified pinned binary.
- With Vite on 127.0.0.1:4173, `cd web && node tests/calm-session.e2e.mjs`: eight theme/width combinations, saved Details, action/focus, draft preservation and mocked native attachment continuity passed. Test requires installed Chromium and Playwright; inspect its environment settings.

The real probe binary was `/workspace/work/native-agent-probe/package/bin/opencode`. Recreate it only from the official pinned artifact and verify both checksums in `internal/agentsoftware/artifact.go`. It performs no provider request. Logs/captures from this environment are under `/workspace/work/redesign-review/` and are not portable CI artifacts.

The baseline full race suite failed on unchanged app source: API timeout, missing tmux for host tests, and a surviving child in the model timeout test. Do not report the full suite passing. Existing sessions/chat browser suites still need action-sheet-aware updates without weakening their checks; run offline Gaffer acceptance and full terminal/mobile/attention contracts. Keep the PR draft until these gates and #636 integration are resolved. Update the handoff and PR with concrete results, not inferred success. Home/navigation redesign is deferred. Do not deploy or merge without a separate request.
