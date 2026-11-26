## AGENT-REVIEW-FIXES-1: the findings of a review of agents, their models and Gaffer are fixed (26 November 2026)

A review of how reet starts and talks to agents, relays their chats and approvals, talks to Gaffer's own provider and
runs the Factory left 40 numbered findings. This entry is the fix for them, one commit per finding, each with a test
that fails without it unless it only deletes code or changes words. Five areas were fixed side by side in separate
worktrees and joined here.

What changed, by area.

- **Approvals and YOLO.** A truncated card has no session key (`permissionKey` in `internal/terminalbroker/chat.go`), a
  path cut to 1 KiB is marked truncated, and `PermissionChat` refuses an allow for one (`ErrPermissionTruncated`, 409
  `permission_truncated`). A session approval is stored only after the machine answers OK, and is dropped when the chat
  closes or is reconnected in place. A lost reply to a switch to YOLO stores YOLO and writes a `yolo-mode` event with
  `reply: lost`. `LaunchBar.svelte` and `StartAgentDialog.svelte` drop the YOLO confirmation when the agent, the chat or
  terminal choice, or the worktree choice changes. The approval timer in `internal/acpagent/session.go` lets an answer
  that beat it stand, and Stop on a card that still waits no longer counts as a running tool.
- **ACP sessions.** A write to an agent's stdin has a 15 second deadline (`writeTimeout`); an agent that does not take
  a frame in that time is ended, which frees Stop, the turn deadlines and End. A model or mode switch marks the chat
  configuring and releases the turn lock while it waits. An exited agent's process group is settled at exit, so a later
  Close never signals a reused id. Antigravity's unreachable `StartMode` and the dead fields and duplicate descriptor
  branches are gone.
- **Chat relay.** `restoreChat` keys on "no live event in this process", so a tab that reconnects first after a
  controller restart no longer wipes a finished state. The connector keeps the confirmed agent mode in its restart
  record and puts a reopened chat back in it. A closed chat's ring is unloaded once stored and unread (`unload`,
  `loadLocked`), triggered by the session list read. A queue delivery shows its bubble after the machine answers, and
  a prompt the process refuses before the turn begins is refused up front or said in the chat.
- **Gaffer chat and providers.** The context snapshot gives up events, then earlier handovers, then the reply's start
  before failing. `ContextLatestReplyFor` joins the chunks of one message and reads back by size, not 60 events. A tool
  that ran during a cancel is recorded and audited. Of several reet tools asked for at once the first runs. Tool
  arguments count towards a conversation's size. A turn that offers reet's tools asks for 16,384 output tokens (the
  owner chose this over 4,096 and 8,192, knowing a model that allows less may refuse the turn). The OpenAI adapter
  sends `max_completion_tokens` to `api.openai.com` only. The Anthropic adapter counts cached input, sends earlier tool
  calls as plain text when a request defines no tools, and also marks the last finished answer as a cache point. A
  write tool on Allow asks first once a tool in the turn returned text other people wrote (the owner chose this over
  "after any tool result", which the existing tests and docs rule out).
- **Factory.** `StopGafferGoal` closes a pipeline's goal from any state. Approving a plan allows the provider session
  text and code (`CreateGafferOutcomeFor`); a stage start no longer does, and stops following with a note when they are
  off. For two minutes after a controller start (`outcomeResumeWait`) a launch whose machine is offline is asked for
  again. Archive and Delete judge a stopped outcome as the page shows it. `gaffer_outcomes` has a `live` column with a
  partial index, set on every write and refreshed at each start, and the supervisor's scans read `live=1` on the read
  pool. An added pipeline never merges by itself. `planLeft` skips a window that has reset.
- **Agent sign-ins.** Copilot tries only the `gh` token of the login its files name and never a token from the
  environment. One comparison and one date reader serve the five agents.

What was checked: `go vet ./...`, `go test -race ./...` and `npm run check` on the joined branch, and the `launch`,
`start-agent`, `gaffer-chat`, `launch-base`, `stage-follow` and `gaffer-goal` browser suites on the web changes.

What was not checked, and what is left.

- Nothing was run against a live provider. The OpenAI and Anthropic changes rest on those APIs' published rules and
  body-level tests. Test connection sends a ceiling of one token; how an OpenAI reasoning model answers that is not
  known.
- The 15 second write deadline was not tried with a real agent given several large images on a slow machine.
- Gaffer's history list (`ContextEventsFor`, 20 events from the last 40 records) is still read per chunk, so with an
  agent that streams small chunks it holds little. Only the latest reply was fixed.
- A finished outcome whose done pipeline once left a session behind stays `live` until archived.
- `AppendTerminalChatEventFor` and `SetTerminalChatAttentionFor` are still used only by tests.
- When an in-place reconnect adopts a process the machine still runs, the controller resets its stored mode to the
  start mode while the process keeps the chosen one. Seen, not changed.
- `TestARunKeepsItsInputsWhenItsTemplateChangesOrGoes` (`internal/gaffer`) failed once on a loaded machine and passed
  alone and in full runs. Cause not found; not yet in the unstable list.
