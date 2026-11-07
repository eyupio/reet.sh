# Name Chat sessions from their first real task

Status: design for review. No code is written. Follows [Compact work items and meaningful names](../../handoffs/compact-work-items.md), which shipped prompt-based names for sessions started *with* a task and left naming from a *later* message to this contract.

## Problem

A session started without a substantive prompt gets the fallback title `"<Agent> · <folder>"` (`terminalproto.AutoTitle`). When the person's first real task arrives later in Chat, the session keeps that generic name on Home, the Sessions list and the session header.

A title is currently one string column with no record of where it came from. The web Start dialog also sends a *derived* title, so the controller cannot tell a typed name from a suggested one. Renaming without that knowledge risks overwriting a name a person chose.

## Decisions (agreed with the user)

- **Ownership flag.** Store where each title came from; only a fallback title may be replaced. Do not infer ownership from how a title looks.
- **Chat sessions only.** Terminal (TUI) sessions take raw keystrokes, so no reliable "first task" exists and they are never renamed.
- **First message that yields a title, within three.** Greetings such as "hi" yield no title and the session waits. The first of the first three accepted prompts that yields a title names the session once. After that, or after three prompts, the title is left alone.
- **Explicit `titleOrigin` on create** from the web client, so a person who types the exact fallback text is still treated as manual.

## Non-goals

- No model or provider call. The name is `terminalproto.PromptTitle(text)`, a tidy-up of the person's own words.
- No renaming of existing sessions, TUI sessions or Gaffer chat.
- No new UI, toast or setting. The change rides the existing session-update path.
- No lifecycle, process, worktree, approval or view-switching change.

## Design

### 1. Ownership: `title_origin` and `retitle_tries`

Two columns on `terminal_sessions`, added with the idempotent `hasColumn` + `ALTER TABLE ADD COLUMN` loop already used in `internal/store/store_attention.go`:

| Column | Type | Default | Meaning |
|---|---|---|---|
| `title_origin` | `TEXT NOT NULL` | `'manual'` | `manual`, `prompt` or `fallback`. |
| `retitle_tries` | `INTEGER NOT NULL DEFAULT 0` | `0` | Accepted prompts that have been considered for a rename. |

The default `'manual'` means every existing session is protected. Add a `CHECK(title_origin IN ('manual','prompt','fallback'))`.

### 2. Setting the origin at create

The create request gains an optional `titleOrigin` (`"manual"`, `"prompt"` or `"fallback"`). The controller decides the stored value; the client's value is a claim it must be consistent with:

- **Blank title** → the controller derives the title with `AutoTitle` and stores `prompt` if `PromptTitle` returned a name, otherwise `fallback`.
- **Supplied title with `titleOrigin` of `prompt` or `fallback`** → accepted only if the title is exactly what `AutoTitle(tool, prompt, dir)` returns for this request and matches that classification. Otherwise it is stored as `manual`.
- **Any other supplied title, or no `titleOrigin`** → `manual`.

Older clients that send no `titleOrigin` therefore always produce `manual`, which is safe. The web Start dialog and launch bar send `prompt` or `fallback` for the titles they derive themselves (`web/src/lib/agentLaunch.ts`, `web/src/lib/launchBar.ts`) and `manual` once the person edits the field.

The retry rule is unchanged: the stored title and origin depend only on the request, so a retried create reproduces them exactly.

### 3. Manual rename wins

`RenameTerminalFor` sets `title_origin='manual'` in the same `UPDATE` as the new title. After that no automatic path can touch the title.

### 4. The automatic rename

`terminalbroker.PromptChat` is the single path for Chat messages: it is called by the HTTP handler (`internal/api/terminal_chat.go`) and by the queue drain (`internal/terminalbroker/chat_queue.go`). After the agent acknowledges a prompt successfully, and only then, it calls a new `store.RetitleChatFromPrompt(uid, sessionID, text)`.

That store method is one transaction:

1. Do nothing unless the session is structured Chat, owned by `uid`, `title_origin='fallback'` and `retitle_tries < 3`.
2. Increment `retitle_tries`.
3. Compute `terminalproto.PromptTitle(text)`. If it is empty or invalid, stop (the session keeps waiting).
4. Otherwise run `UPDATE terminal_sessions SET title=?, title_origin='prompt', updated_at=? WHERE id=? AND user_id=? AND title_origin='fallback'` and publish the normal session-update event.

Properties:

- A failed or lost send neither renames nor counts, because the hook runs only after a successful acknowledgement.
- A person's rename racing the first message wins: `manual` is written first, so the guarded `UPDATE` matches no row.
- Replays and retries are idempotent: once the origin is `prompt`, later calls change nothing.
- A message sent with only images has no text, yields no title and still counts as a try.
- The text passed in is the person's own message, not uploaded-image paths or system text.

### 5. What the person sees

The title changes through the existing session-update event stream. Home, the Sessions list and the session header already render `title`; no component change is needed beyond the create request. The rename happens at most once per session, so a name never changes mid-conversation after it is set.

### 6. Preserved behaviour

- Manual names, saved view and density preferences, approvals and explicit Start/Retry are untouched.
- The title's source stays separate from session state and the running process.
- Same-process Chat/Terminal switching is untouched: the rename is a database update with no worker restart, worktree allocation, mode-endpoint call or directory change, and both panes stay mounted.
- No connector or protocol change. Only the controller, and the web client's create request, change.

## Testing

- **Store:** origin set correctly for blank, matching-derived and other titles; exactly one rename; a manual rename racing a prompt keeps the manual title; existing rows stay `manual`; the three-try cap; greeting then task; invalid title; wrong owner.
- **Broker/API:** `fallback` → `prompt` on the first real Chat message, including a message from the queue drain; a failed or unacknowledged send changes nothing; a TUI session is never renamed; image-only messages count but do not rename.
- **Create:** old clients with no `titleOrigin` get `manual`; a mismatched claim is stored as `manual`; a retried create reproduces the same title and origin.
- **Web browser acceptance:** the dialog and launch bar send the expected `titleOrigin`; a Chat session started without a task is renamed on Home and in the session header after its first real message; a manual rename before the first message is kept. Any new browser script must be added to the CI workflow list in the same change (`launch-options.e2e.mjs` was missed from local validation on #644 and failed CI).

## Risks and open points

- **Migration.** `terminal_sessions` is a hot table; the change must use the existing column-add pattern and be covered by a test that creates the table without the new columns, as `store_agents_home_test.go` does for other columns.
- **Client/server drift.** `agentLaunch.ts` mirrors `autotitle.go` and both are checked against `internal/terminalproto/testdata/autotitle.json`. The origin classification must be added to that shared fixture so the two cannot disagree.
- **Three-try cap** is a product choice from the review. It is a single constant and easy to change.

## Out of scope for this spec

Broader attention identity/deduplication and supporting-page navigation remain separate work.
