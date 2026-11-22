# Transcript-backed Antigravity chat

**Why.** An Antigravity (agy) Gaffer chat runs `agy acp`. When a turn ends with nothing said (for example agy stops on a plan
or artifact waiting for review), the chat reads "Ready for your next instruction" under an unanswered prompt, and nothing
explains it. OpenCode and Cline have a recovery path for this; agy has none.

**Approach.** Run agy as its own terminal program and build a read-only chat view from the conversation ledger agy writes
itself, `transcript_full.jsonl`. The terminal stays the place for native commands, approvals and anything the ledger does not
show. A turn can no longer go silent, because the person can always see what agy sees.

## Stages

Each stage is its own PR and leaves the app working.

1. **Ledger reader** (`internal/agytranscript`). Pure Go: replay a ledger with agy's rewind rule (a step whose index is not
   above the last replaces it and everything after), drop an unfinished last step, derive Working/Idle from the native turn,
   and refuse a malformed, oversized or over-long history rather than guess. No file access, no wiring. **This PR.**
2. **Finding the right ledger.** Agy holds one presence lock per conversation under `<agy data dir>/presence/<uuid>.lock`;
   the ledger is `brain/<uuid>/.system_generated/logs/transcript_full.jsonl`. A session's ledger is the one lock held by
   that session's own agy process, proved by a process identity read before and after (so a reused pid is never read).
   Linux reads `/proc/<pid>/fd`, macOS uses `lsof`. A missing or ambiguous lock keeps the terminal.
   Rewinds are also recorded in the conversation's SQLite catalogue before the ledger catches up; the catalogue's active step
   indices (`modernc.org/sqlite`, already a dependency) say which ledger steps are still real. Read a private copy, never the
   live file.
3. **Connector feed.** A connector-side tail of the ledger for an agy terminal session, negotiated as its own feature
   (`agy-transcript-v1`) so older controllers never see it. It sends the projected turns and the Working/Idle state to the
   controller the way chat events travel today, with paging and a revision id that changes when history is rewound.
4. **Chat view.** The session screen's Chat tab shows the ledger for an agy terminal session, with the terminal one click
   away and an explicit notice when history is unavailable. Skill and plan-review states stay in the terminal.

## Decisions already made

- Terminal first, ledger second, as above. The ACP chat for agy stays as it is until stage 4 is proven; nothing here removes it.
- Display feedback only: Working/Idle is not input readiness. An interrupted or errored turn with no final answer can stay
  Working, and the docs say so.
- Anything the reader cannot trust becomes "unavailable", never a partial guess.

## Not known yet

- Agy's exact record shapes beyond `step_index`, `type`, `status`, `content`, `tool_calls` and `model`. Stage 1 reads only those;
  stage 3 needs captured ledgers from a real agy to design the turn projection and the `<USER_REQUEST>` wrapper handling.
  **No agy binary was available while this was written.**
- Whether `agy acp` reports a plan-review stop as a permission request or as a plain end of turn. The terminal reports it as a
  blocked "Approve plan" state.

## Checks

Stage 1: `go test ./internal/agytranscript` (replay, rewind, partial line, oversize, step bound, completed-only, activity).
