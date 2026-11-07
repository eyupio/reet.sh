# Structured Chat worktree launch hotfix

Current main includes #636. This fix is independent of the session redesign drafts #637 and #638. Check this PR's current CI and review status before merging; do not deploy the redesign drafts to obtain this repair.

## Production symptom and cause

Starting a Chat session in a saved Git project returns HTTP 400 with `invalid terminal request`. Reproduced against main `f047dd1a`: `POST /api/v2/terminal/sessions` fails in 4 ms. The controller now adds worktree/project fields for isolation, but the structured Create validator still rejects those fields. The ACP launcher also lacked worktree preparation and metadata handling.

The reported production entry point was the Sessions page. Its request payload has not yet been provided, so the matching HTTP status alone does not establish that every reported failure has this cause.

## Repair

- Permit worktree parameters for a new structured session. Continuing an existing conversation into a fresh worktree remains forbidden.
- Prepare or adopt the worktree before starting ACP. Use its exact directory and report its path/repository through the existing session-change fields. Preserve that evidence in connector restart records.
- Undo a newly created worktree on definite startup refusal. Never undo an adopted worktree or delete a worktree merely because a chat ends.
- Restore controller-derived worktree/project identity from the original create receipt on retries, so renaming a project does not alter an uncertain launch.
- Negotiate `acp-worktree-v1` separately from terminal-only worktrees. An older connector receives no unsupported create and reserves no session; the browser gets an actionable HTTP 409 asking for a machine update.

## Verification

The real pinned OpenCode integration test passes with an authenticated controller, broker, private connector, real Git worktree and actual ACP worker. No model/provider request or account credential is used. It verifies old-connector refusal, capability upgrade, successful saved-project Chat start, the same session/worktree on retry after rename, active-session removal refusal, unsaved-change evidence, stale-confirmation refusal, exact confirmed removal and branch retention.

Protocol/broker tests cover capability negotiation and refusal before dispatch/quota reservation. Host tests cover the worker directory, idempotent startup, cleanup after a new-worktree refusal and preservation after an adopted-worktree refusal. The real-agent gate is in `.github/workflows/structured-worktree.yml`; it verifies both archive and binary checksums against the reviewed OpenCode 1.19.5 artifact pins before execution.

## Deployment

Update the controller and the affected machine connectors to a build containing this fix. Either order is safe: until both sides negotiate support, isolated Chat starts are refused explicitly rather than falling back to a shared checkout. Reconnect the upgraded connector to refresh its capabilities. Existing conversations keep their directory; normal terminals retain their previous behavior.

No controller database migration is added. Connector registry fields are optional and old records remain valid. Rolling back to the #636 deployment restores the Chat-start failure; rollback does not repair it.

## Continuation prompt

Continue `fix/structured-worktree-launch` in eyupio/reet.sh. Read this handoff, inspect the hotfix PR and current CI, and prioritize this production repair over redesign drafts #637/#638. Local checkout: `/workspace/work/reet-structured-launch-hotfix`. Use `source /workspace/.reet-tools/activate.sh` if this environment survives. The user-owned tmux binary is `/workspace/work/native-host-tools/root/usr/bin/tmux`; prepend its directory to PATH. The reviewed agent binary is `/workspace/work/native-agent-probe/package/bin/opencode`; verify against `internal/agentsoftware/artifact.go` before reuse.

Run `REET_OPENCODE_NATIVE=<verified-binary> go test -race ./internal/api -run '^TestStructuredChatWorktreePrivateLifecycle$' -count=1 -v`, the affected Go package tests, and `go vet ./...`. Read final CI on the exact hotfix head before saying it is ready to merge. Do not merge or deploy without the user's instruction. After the hotfix merges, integrate it into the redesign branches, preserving the separate native-view capability and private reply handling fixes. Never restart a running agent or allocate another worktree when switching Chat / Terminal views.
