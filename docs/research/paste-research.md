# Copy, paste and image paste: the reference tool vs reet.sh, and a design for reet

RT = the reference tool (the third-party tool this note studies; named by role only).

Research only. Nothing in either repository was changed or run. RT checkout: `<reference-tool checkout>` at `a02ef83` (shallow). reet checkout: `/home/user/reet.sh` at `b45fceb`. Paths below are relative to each repository root.

Confidence labels: **[read]** means I read the code. **[doc]** means only the docs say so. **[belief]** means outside knowledge (agent CLI behaviour) that has to be checked against the pinned CLI versions before anything is built on it.

---

## 1. The reference tool (RT)

### 1.0 Architecture that matters here

- The web dashboard's agent terminal **is not xterm.js**. The server renders the tmux pane, either from a VT grid fed by `tmux pipe-pane` or from `capture-pane` snapshots, and pushes rows over a WebSocket. The page draws those rows as real DOM text, so scrolling, selection and zoom are the browser's own (`docs/guides/web/terminal.md:9`). This one choice explains most of its copy behaviour, phone copy especially: selection is the browser's native selection.
- Input goes back as binary keystrokes, or as a JSON `paste` message that the server applies through tmux's paste buffer (`src/server/live_ws.rs:58-64`).
- Front end: React (`web/src/components/LiveTerminalView.tsx`, `MobileLiveTerminal.tsx`, `live-terminal/useTerminalInput.ts`). Back end: Rust/axum (`src/server/...`).

### 1.1 (a) Copying text out of a session

| Mechanism | Where | Notes |
|---|---|---|
| Native DOM selection, then Ctrl/Cmd+C | rows are DOM text | **[read]** Works because the rows are real text. The hidden input textarea has focus, so `useTerminalInput.ts:115-120` maps **Ctrl+Shift+C** to `writeClipboard(window.getSelection().toString())`. |
| Selection "hold" | `web/src/hooks/useSelectionHold.ts`, `MobileLiveTerminal.tsx:161-174, 236-243` | **[read]** While a selection touches the pane, repainting stops, so the agent cannot rewrite text under the selection. Gestures give way to the selection so iOS handles can be dragged. Docs: `docs/guides/web/terminal.md:22`. |
| Copy-on-release | `docs/guides/web/terminal.md:20` | **[doc] only.** The docs say releasing a drag copies to the clipboard automatically. I found no DOM-selection auto-copy code in this checkout: `writeClipboard` is called only from Ctrl+Shift+C, the OSC 52 path and unrelated buttons (grep of `web/src`). Treat it as unverified. It may be done by tmux mouse mode plus OSC 52 for mouse-enabled apps. |
| OSC 52 from the agent (for example, an agent's own "copy" action) | tmux `set-clipboard on` + `allow-passthrough on` (`src/tmux/utils.rs:749-754`; config `[tmux] clipboard = auto/enabled/disabled`, `docs/guides/configuration.md:465`; `docs/guides/tmux-status-bar.md:12-14`) | **[read]** The server pulls OSC 52 writes out of the raw pane stream (`src/tmux/vt.rs`, `Osc52Channel`, `OSC52_MAX_PAYLOAD = 2 MiB` at `vt.rs:32`; empty writes are ignored so the clipboard is never cleared). It forwards them as `{"type":"clipboard","text":...}` (`live_ws.rs:54-56, 540-541`) **only when the connection is not read-only and the client is the size owner** (`live_ws.rs:552-563, 1261-1272`). |
| Gesture-bound clipboard write | `web/src/lib/clipboard.ts:72-134`, `LiveTerminalView.tsx:64-78`, `live-terminal/useForwardInput.ts:175-176, 196` | **[read]** On the mouse release that ends an agent-side selection (still inside the user gesture), the client calls `navigator.clipboard.write([new ClipboardItem({"text/plain": pendingPromise})])` and resolves that promise when the OSC 52 message arrives (1 s timeout). This gets past Safari and Chrome's user-activation rule for asynchronous clipboard writes. If nothing was armed, it falls back to an ungestured `writeClipboard`, and then to `execCommand("copy")` (`clipboard.ts:55-65, 136-153`). |
| Mobile | `docs/guides/web/terminal.md:59-66` | **[doc+read]** Long-press native selection. The toolbar has no Copy key. |
| Secure context | `docs/guides/web/terminal.md:26` | The Clipboard API needs HTTPS or localhost. Firefox is best-effort. |

tmux copy-mode: RT does not drive tmux copy-mode from the web UI. Its scrollback is the server's widened capture window (`docs/guides/web/terminal.md:13`).

### 1.2 (b) Pasting text

- Browser: the `paste` event on the hidden textarea or keyboard proxy reads `clipboardData.getData("text/plain")` synchronously and calls `sendPaste(text, false)` (`useTerminalInput.ts:147-165`). Mobile Paste key: `MobileTerminalToolbar.tsx:125-146` reads the clipboard with `readClipboardText()` (`clipboard.ts:35-52`: `navigator.clipboard.read()`, preferring `text/plain`, then `text/uri-list`, then `text/html` converted to text). If reading is refused, or the origin is plain HTTP, it opens the **compose sheet** (`TerminalComposeSheet.tsx`), a native textarea for dictation, autocorrect and long-press paste, whose text is sent as one paste (`docs/guides/web/terminal.md:65`).
- Wire: `{"type":"paste","text","submit"}` (`web/src/hooks/useLiveTerminal.ts:394-396`). Server (`live_ws.rs:1711-1724`): dropped for read-only or non-owner clients. `paste_payload` (`live_ws.rs:242-249`) calls `strip_paste_escapes` (`live_ws.rs:220-240`), which removes `ESC[200~`/`ESC[201~` **and every other ESC byte**, so pasted text cannot break out of bracketed paste or inject escape sequences. With `submit`, trailing CR/LF are trimmed and Enter is sent after an agent-specific delay (`send_keys_enter_delay_ms` = 150 for some agents, `src/agents.rs`).
- Delivery: `deliver_paste` (`live_ws.rs:254-296`) uses the VT input channel's `try_send_paste` on tmux 3.8+. Otherwise it uses `Session::paste_text` → `send_via_paste_buffer` (`src/tmux/session.rs:2313-2344`): `tmux load-buffer -b rt-send-<pid>-<n> -` then `paste-buffer -d -p`. With `-p`, tmux adds bracketed-paste markers **only if the pane enabled mode 2004**. So newline safety relies on the agent or shell using bracketed paste. In a raw shell without it, newlines execute.
- **No confirmation dialog and no large-paste guard** in the live terminal (grep found no paste size cap or confirm). The live WebSocket has no explicit `max_message_size` (only `runtime.rs`/`runtime_ws.rs` set one, for other sockets).

### 1.3 (c) Pasting images

This exists for the live terminal (PR/issue #2678 per comments) and separately for the ACP "structured view" composer.

**Live terminal flow** **[read]**:
1. `useTerminalInput.ts:147-181`: in the paste event it collects `clipboardData.items` of kind `file` with `image/*`. Text-only pastes go the normal way. Otherwise each image is uploaded in parallel, and then `sendPaste(" <text> <escaped paths> ", false)` is sent, **with no Enter**. Paths are backslash-escaped for whitespace and backslashes (`keySequences.ts:83-85`, "as terminal drag-and-drop does"). Write admission (`captureWriteAdmission`, `LiveTerminalView.tsx:176-185`) is checked before and after the upload, so a session switch or a read-only change in between drops the insertion.
2. `web/src/lib/api.ts:352-365` `pasteImage`: POSTs `/api/sessions/{id}/paste-image` with JSON `{mime_type, data: base64}`.
3. Route: `src/server/router.rs:118-122`, with the body limit raised to **8 MiB** for this route (global 1 MiB). Access table: `src/server/access.rs:406`.
4. Handler `src/server/api/sessions/send.rs:407-511`:
   - read-only server → refused. A the reference tool's hosted edition-locked client may only target structured sessions (`the reference tool's hosted edition_block_non_structured`).
   - base64 decode. Empty → 400. **> 5 MiB decoded** (`MAX_PASTE_IMAGE_BYTES`, `send.rs:328`, justified as "Claude Code caps image attachments around this size") → 413.
   - **Magic-byte sniff decides the type**: PNG, JPEG, GIF or WebP only (`src/server/api/acp/attachments.rs:35-47`). The client MIME is advisory. Anything else → 400 `not_an_image`. SVG is excluded as scriptable.
   - `write_paste_image` (`send.rs:357-382`): directory `<session worktree>/.rt-pasted-images/`, created with `create_dir_all`. Writes a self-ignoring `.gitignore` containing `*`. File name `rt-paste-<uuid v4>.<ext>`, ext taken from the sniffed type. `OpenOptions::create_new(true)`. **No explicit chmod**: the file gets the process umask (typically 0644), and the directory is not checked for symlinks.
   - Why it is in the worktree (`send.rs:330-335`): a Docker-sandboxed pane mounts only the worktree. `pane_visible_paste_path` (`send.rs:384-405`) maps the file to the container path (`/workspace/<dir>/.rt-pasted-images/...`) for sandboxed sessions, or to the host absolute path otherwise.
   - **Cleanup**: an in-process timer deletes the file after **1 hour** (`send.rs:493-503`). The timer is cancelled on shutdown, so files written before a restart are never removed. The directory itself is never removed.
   - Logging: on write failure only the error is logged (`send.rs:474, 482`), never the bytes.
5. Delivery to the agent: **one generic mechanism for every agent**: the path is pasted as text into the pane. RT has no per-agent logic (Claude, Codex, Gemini and so on all get the same thing). It relies on the CLI recognising a pasted image path. Claude Code turns a pasted image file path into an image attachment **[belief]**. For others the path is just text in the prompt, which the agent may then read with its file tool.
6. Mobile: images arrive only through the native paste event, on the hidden input or keyboard proxy. The toolbar Paste key reads only text (`readClipboardText`). The compose sheet has no image path. **No file picker and no drag-drop in the live terminal** (I found none).
7. Tests: `send.rs:740-793` (file lands in the worktree, `.gitignore` written, host versus container path), and `web/src/components/__tests__/MobileLiveTerminal.paste.test.tsx` / `.clipboard.test.tsx`.

**Structured view (ACP) composer**, for comparison (`docs/structured-view/interface.md:73`; `src/server/api/acp/attachments.rs:10-32`): paperclip, Cmd/Ctrl+V or drag-drop. At most 8 attachments, 10 MiB each and 20 MiB total. Images limited to png/jpeg/gif/webp, SVG excluded, plus audio and text/pdf resources. These go to the agent as ACP prompt content blocks, not as file paths. reet has no ACP front end yet ("Until Program 2 … not yet a structured chat front end", `handoff.md`), so only the terminal path applies.

---

## 2. reet.sh today

### 2.1 What exists

The terminal is **xterm.js 6.0.0** (`web/package.json:24`) in `web/src/lib/terminal/TerminalPane.svelte` (Svelte 5). Bytes go browser → controller → machine over authenticated WebSockets. The machine attaches a **tmux client inside a PTY** (`internal/terminalhost/attachment.go:100-104`, `tmux -S <private socket> attach-session`), and browser binary frames are written straight to that PTY (`attachment.go:181-189`). So xterm.js is tmux's outer terminal.

Copy:
- `copy()` at `TerminalPane.svelte:61` calls `navigator.clipboard.writeText(term.getSelection())`, or the DOM selection when the Scrollback view is open. Feedback "Selection copied." / "Clipboard access was denied…".
- Shortcuts in `keyboard.ts:5-12`: Cmd+C on Mac; Ctrl+C **only when there is a selection** on Windows/Linux; Ctrl+Shift+C. The toolbar has a **Copy** button (`TerminalPane.svelte:195`) and the right-click menu has "Copy selection" (`:219`). Shift+right-click keeps the browser menu.
- **OSC 52 is disabled on purpose** in two places: xterm `registerOscHandler(52, () => true)` (`TerminalPane.svelte:136-137`), and machine tmux `set -g set-clipboard off`, `allow-passthrough off` (`internal/terminalhost/layout.go:146-154`). OSC 8 links are disabled too. This is documented (`docs/terminal-workspace.md:227`).
- Earlier output (`OutputScrollback.svelte`) is a read-only DOM `<pre>` snapshot (last 2,000 lines / 256 KiB) fetched from the machine. Text there can be natively selected and copied.
- Saved commands and command history have their own Copy buttons (`CommandLibrary.svelte:54`, `CommandHistory.svelte:15`).

Paste:
- Native Ctrl/Cmd+V (and Ctrl+Shift+V) is left to the browser `paste` event (`keyboard.ts:9`, `TerminalPane.svelte:130`). A capture-phase listener (`:125-126`) cancels xterm's own handler and calls `paste(clipboardData.getData('text/plain'))`.
- Toolbar **Paste** button and context-menu Paste use `navigator.clipboard.readText()` (`:78`). Denial → "Paste with your keyboard inside the terminal."
- `paste()` at `:62-69`: refused while Scrollback is open or not live. **Hard cap 240 KiB** of UTF-8. **A review modal** (ConfirmDialog, `:225`) opens if the text has CR/LF, any C0 control except tab, DEL, or is over 2,000 chars. Approval is fenced to the connection `generation`, so a reconnect cancels it (`:70-77`).
- Sending: `terminal.paste(text)`. xterm converts `\r?\n` to `\r` and adds `ESC[200~ … ESC[201~` **when the remote side has enabled bracketed paste** (`node_modules/@xterm/xterm/src/browser/Clipboard.ts:13-26, 51-56`). tmux mirrors the active pane's mode 2004 to its client, so this follows the agent or shell.
- Transport limits: client frames up to 32 KiB (`connection.ts:6, 104`). A send is refused if `bytes + bufferedAmount > 256 KiB` (`connection.ts:103`). Server read limit `MaxBinaryBytes` = 32 KiB per message (`internal/api/terminal.go:442`, `terminalproto/protocol.go:19`).
- Phone: an extra-keys row (Esc, Tab, arrows, Ctrl+C, Enter, Ctrl/Alt latches; `TerminalPane.svelte:208-211`). Paste and Copy stay in the scrolling toolbar.

Attachments and images: **none**. An image paste reaches `paste('')` and does nothing, silently. The image item is never read, and there is no file picker, drag-drop, upload route or protocol message for files.

### 2.2 What is clunky (concrete)

1. **Image paste silently does nothing.** No feedback, and no way to give an agent a screenshot from the browser.
2. **Every multi-line paste opens a modal**, even into Claude Code or Codex, which use bracketed paste so newlines cannot execute anything. The review is justified for a raw shell. For a bracketed-paste agent prompt it is friction on the most common action (pasting a stack trace or a code block).
3. **Confirmed pastes are not sanitised.** xterm 6 does not strip an embedded `ESC[201~`, so after the user clicks "Paste text" a payload can end the bracket early and have the rest typed as keystrokes. The review dialog shows the text, but a user will not spot an escape sequence in a 2,000-character preview. RT strips all ESC bytes server-side. Fix this whatever else is decided.
4. **Agent "copy" actions do nothing.** Claude Code `/copy` and Codex's copy shortcut emit OSC 52 **[belief]**, and reet blocks OSC 52 in both tmux and xterm.
5. **Paste button permission prompts.** `navigator.clipboard.readText()` makes Chrome show a site permission prompt the first time and Safari/Firefox show a "Paste" callout every time. That is unavoidable for a button, but keyboard paste avoids it, and the button does not say so up front.
6. **Phone copy is weak.** xterm.js's canvas/DOM renderer does not support native long-press selection well on iOS or Android. The only good phone copy path is Scrollback, which is buried in the toolbar, is a snapshot needing a round trip to the machine, and has no "Copy all" button.
7. **No "copy screen", "copy last N lines" or "copy last output".** You must drag-select in xterm.
8. The context menu replaces the browser menu (Shift+right-click restores it). That is a deliberate choice and fine; listed only for completeness.

### 2.3 Constraints any change must keep (from code and docs)

- **Accounts-only, cookie-only, non-viewer, canonical origin, and never credentials in URLs** for every terminal route (`internal/api/terminal.go:45-84`). The socket must have an empty query string (`:68`). The terminal origin is always HTTPS or loopback (`terminal.go:97-103`), so the async Clipboard API is always available.
- **Terminal content is never stored by the controller or the browser.** "Terminal text is not persisted to browser storage" (`docs/terminal-workspace.md:228`). The machine registry keeps "operational metadata only. No bytes … or paths supplied by a controller are persisted" (`internal/terminalhost/registry.go:31-32`). History snapshots are streamed through and never saved (`terminalproto/stream.go:3-4`).
- **Prompts and user text never go to audit, logs, browser storage or URLs** (`docs/start-agent-session.md:183-191`, `docs/terminal-launch.md:160-176`). The audit records metadata only. Pasted images must follow the same rule.
- **Disconnected input is never queued or replayed.** Insertion binds to the live attachment generation (`connection.ts:8-9`, `docs/terminal-workspace.md:185, 226`).
- **Message sizes**: control messages ≤ 16 KiB, binary frames ≤ 32 KiB, queued output ≤ 256 KiB (`terminalproto/protocol.go:16-24`). Reverse streams carry binary frames ≤ 32 KiB and are authenticated by `StreamHello` with operation `attach`|`history` (`stream.go:12-18`).
- **Feature negotiation**: comma-separated tokens in `X-Reet-Terminal-Features` (`internal/agenthealth/agent.go:636-667`, `internal/api/private_control.go:17-28`). Each extension has `SupportsX(features)`, and the connector answers "unavailable" for a feature it did not negotiate (`agenthealth/agent.go:395-410`). Strict JSON decoding means **an old peer rejects unknown fields and kinds**, so new bodies must be sent only after negotiation (see the `BrowseOff` comment, `protocol.go:117-122`).
- **Machine-side privacy**: the private terminal dir is 0700 and owner-checked, and private files are 0600 written through a temp file (`layout.go:105-175`). `tmux.conf` is written **only if missing** (`layout.go:127-143`), so changing `tmuxConfig` does **not** reach existing installs without a migration step.
- **Rate limits**: per-user write limiters exist for folder projects (`internal/api/folder_projects.go:68-80`, 429 `project_rate_limited`). The terminal routes have no limiter today.
- **CI budget**: `terminal-browser` uses 20–25 of its 30 minutes, so add **no new fixtures there**. Put new browser fixtures in `phone-zoom`/`phone-dialogs` or a new job (`handoff.md:70`).
- **Merge rule**: green CI and a clean Codex review, or an explicit owner exception. Every PR updates the handoff block (`handoff.md:62-64, 101`).
- The deferred "terminal security pass" (`handoff.md:104-106`) touches the same area. Read "Later: user-controlled terminal security preferences" in `handoff-history.md` before changing paste or OSC 52 policy.

---

## 3. Proposed design for reet

### 3.1 Copy

1. **Sanitised selection copy stays as is**, plus an optional **copy-on-select** preference (Appearance drawer, a local preference like font size). Default **off**, to match today's docs; owner decision D4.
2. **"Copy screen" and "Copy last N lines"** (N = 50/200/all of xterm's 5,000-line scrollback) from the xterm buffer, client-side only (`term.buffer.active.getLine(i).translateToString(true)`), joined with `\n` and trailing spaces trimmed. Nothing crosses the network. Add it to the toolbar overflow and the context menu.
3. **Phone copy sheet ("Select text")**: a bottom sheet with a read-only `<pre>` filled from the xterm buffer (visible screen plus recent lines), giving native long-press selection, plus **Copy selection** and **Copy all** buttons. It is local, never saved, and discarded on close. This copies RT's native-text advantage without replacing xterm.js. Also add a **Copy all** button to `OutputScrollback`.
4. **Gated OSC 52 (agent copy)**, owner decision D3:
   - Client: replace the blanket `registerOscHandler(52, () => true)` with a handler that accepts **write only** (refuses the `?` query, so the clipboard is never read back), decodes base64, caps at 1 MiB, and writes **only if the user gave a keyboard or pointer gesture to this pane in the last ~2 s**. It arms a promise-valued `ClipboardItem` on keydown/pointerup, the way RT does in `clipboard.ts:72-134`. Otherwise it shows a toast: "The agent copied 2.1 KB. Copy it?" with a button, which gives a fresh gesture. Never clear the clipboard on an empty write.
   - Machine: tmux must pass OSC 52 to its client, which needs `set -g set-clipboard on` and new connector code to update the existing `tmux.conf` (it is written only if missing). Alternatively, apply `set-option -s set-clipboard on` at server start the way `manager_tmux.go:71` does. Gate this on a new connector feature and a local switch, so a connector that keeps it off simply never forwards. Keep `allow-passthrough off`; passthrough is not needed for OSC 52 when `set-clipboard on`.
   - Risk: any program in the shell (for example, `cat` of a malicious file) can emit OSC 52. The gesture gate plus the toast fallback stops silent clipboard poisoning. The docs and help text must change ("OSC 52 clipboard writes remain disabled").
5. "Copy last command" is already covered by Command history → Copy. "Copy last output" in the semantic sense needs shell integration (OSC 133 marks), which reet does not have. Do not promise it; offer "last N lines".

### 3.2 Text paste

1. **Always strip bracket markers** from pasted text before `terminal.paste`: remove `\x1b[200~` and `\x1b[201~`. Recommendation: strip **all ESC bytes** unless the review dialog was explicitly approved for control characters, and even then drop the two bracket markers. One small pure function in `keyboard.ts` or a new `paste.ts` with unit tests. This closes risk 2.2-3.
2. **Bracket-aware review** (owner decision D2). Skip the modal when **all** of these hold: `term.modes.bracketedPasteMode` is true (public xterm API); after normalisation the text has no C0 controls except `\n` and `\t`; and its size is ≤ 64 KiB. Show an inline status instead: "Pasted 42 lines as one block". Keep the modal when bracketed mode is off (newlines would run commands), for control characters, for > 64 KiB, and for saved-command or history "Run". Docs to update: `docs/terminal-workspace.md:225-226`.
3. **Large paste**: keep the 240 KiB cap (it fits the 256 KiB send window). Send as today, in 32 KiB frames. Optional later: pace the frames on `bufferedAmount` to lift the cap; not needed now.
4. **Newline safety**: xterm already sends CR. Never add an Enter on paste. A trailing newline in a bracketed paste stays inside the bracket, so it is safe. Without bracketed mode, a trailing newline must go through review (already the case).
5. **Paste button**: on a permission refusal, say "Use Ctrl/Cmd+V in the terminal", which is already done. Also switch it to `navigator.clipboard.read()`, so the button can paste **images** as well (see 3.3). Phones keep the button in the extra-keys row.

### 3.3 Image paste and upload

**User flow**
- Sources: (a) the image items of a paste event in the pane; (b) the Paste button via `navigator.clipboard.read()` (`image/png` is the type browsers expose); (c) drag-drop onto the screen frame; (d) an **Attach image** toolbar button with `<input type=file accept="image/png,image/jpeg,image/gif,image/webp">`. On phones that opens Photos or the camera, so it is the main phone path.
- The client checks type and size (≤ 5 MiB) first. It shows a chip "Uploading screenshot.png (1.2 MB)…" that can be cancelled. On success it **inserts the path into the live attachment** with `terminal.paste(" " + reference + " ")`, bracketed when the agent enabled it. **No Enter** is sent. The insertion is fenced to the connection `generation` captured when the upload started; if the generation changed, it says "Uploaded, but the connection changed; nothing was inserted. Paste the path: …" and the path can be copied.
- Nothing is kept in browser storage. Object URLs for thumbnails are revoked on close.

**Wire path** (controller never stores bytes):
1. Browser → controller: `POST /api/v2/terminal/sessions/{id}/uploads` behind `s.terminal(...)`, which gives accounts-only, non-viewer, cookie, origin check and no URL credentials. Raw body (not base64, which saves 33%), `Content-Type: image/png|jpeg|gif|webp`, `http.MaxBytesReader` 5 MiB + 1. The handler checks the session is Running and belongs to the user, the machine is live with `upload-v1` negotiated and an `Uploads` capability reported, and a per-user limiter allows it (for example 20 uploads/min, 429 `upload_rate_limited`). It then sniffs magic bytes and rejects SVG or anything unknown. Reply `{path, reference}`. The body is read into memory, at most 5 MiB per request, with a per-user concurrency of 2.
2. Controller → machine: a new control body `Upload{streamToken, size, sha256, mime}` (new kind `upload`), sent **only** when `SupportsUpload(features)`. A new `StreamHello.Operation == "upload"` is accepted by `DecodeStreamHello` only on connections that negotiated `upload-v1`; today's validator rejects anything except attach and history, so old controllers and connectors are unaffected. The controller writes ≤ 32 KiB binary frames on the reverse stream and then closes normally. The machine replies on that stream with one final text frame, `{code, path}`, or reports a typed failure code (`upload_too_large`, `upload_type`, `upload_quota`, `upload_disabled`).
   - Alternative considered: carry the bytes on the browser terminal socket. Rejected: it mixes file bytes with keystroke frames and the 32 KiB read limit would need framing changes.
3. Machine (`terminalhost`, new `upload.go`):
   - The connector checks: the session is Running in its registry; a local switch is on (`reet agent terminal uploads on|off`, default on? owner decision D6); size equals the declared size and is ≤ 5 MiB; the SHA-256 matches; the magic bytes are png/jpeg/gif/webp and match the declared MIME.
   - Storage, recommended (D1): `<layout.Dir>/uploads/<session-id>/` made with `ensurePrivateDir` (0700, owner-checked, no symlinks). File name `img-<random 128-bit hex>.<ext>`, created with `O_CREAT|O_EXCL|O_NOFOLLOW` at 0600 and written through `writePrivateFile`'s temp-and-rename pattern. **The client file name is never used.**
   - Quotas: ≤ 20 files and 50 MiB per session; ≤ 200 MiB per machine; refuse past them.
   - Cleanup: delete `uploads/<session>` when a session goes to Ended/Exited/Lost (reconcile path) and at connector start for any session that is not Running. Also apply a **24 h max age** sweep: images are read by the agent when attached, but a user may queue a prompt and come back, so 1 h as in RT is too short. Never touch anything outside `uploads/`.
   - Nothing goes in the registry: upload files are derived state, so registry.go's "no bytes or paths persisted" stays true.
4. **Reference per agent** (the controller knows `terminal_sessions.tool`). The reply's `reference` is what gets inserted:
   - `claude`: the absolute path, shell-escaped like RT's `escapePastePath`. Claude Code turns a pasted image path into `[Image #n]` **[belief]**.
   - `codex`: the absolute path; the Codex TUI attaches pasted image paths **[belief]**. (`codex -i` exists only at launch.)
   - `gemini`: `@<path>`. **But** Gemini CLI restricts file reads to its workspace directories **[belief]**, so a file in `~/.local/state/reet/...` would be refused. It needs either the workspace placement (D1 option B) or launching with `--include-directories <uploads dir>`, which changes launch arguments (`docs/terminal-launch.md` table).
   - `opencode`, `cline`, plain shell: insert the bare path; the user tells the agent what to do with it. Mark these "path only" in the UI ("The agent receives the file path").
   - Verify each **[belief]** with the pinned CLI versions in an authorised help-only or test install (as was done for Cline and Pi in C) before claiming support in the docs.
5. **Privacy and audit**: never log bytes, names or hashes. Record one audit event `terminal-upload` with machine, tool, type and size bucket only (D5), or none at all. Bytes pass through controller memory and are never written to the database, disk or the browser. Error messages contain no path from the request.

**Feature negotiation and compatibility**: new token `upload-v1` in `terminalproto` (`SupportsUpload`), added to `agreedControlFeatures`/`terminalFeatures`. New `Capabilities.Uploads bool` (requires `TerminalControl`), and `UploadsOff` reported only after negotiation, as with `BrowseOff`. Old connector: the controller never sends `Upload`, and the UI hides Attach image with "Update reet on that machine to attach images". Old controller with a new connector: the token is not agreed, so the connector serves nothing. The browser learns support from the machine's capabilities in `/terminal/machines`.

### 3.4 Tasks (small, independently reviewable)

| # | Task | Size | Depends |
|---|---|---|---|
| T1 | Paste sanitiser: strip bracket markers and ESC, unit tests; use it in `paste()`/`confirmPaste()` | S | none |
| T2 | Bracket-aware review rule (D2) + docs `terminal-workspace.md` keyboard section; e2e in `phone-dialogs` (not `terminal-browser`) | S | T1 |
| T3 | Copy screen / last N lines from the xterm buffer + context-menu and toolbar items | S | none |
| T4 | Phone "Select text" sheet + Copy all in Scrollback | M | T3 |
| T5 | Copy-on-select preference (if D4 = yes) | S | none |
| T6 | Gated OSC 52 client handler (gesture-armed ClipboardItem, toast fallback, 1 MiB cap, no query) | M | D3 |
| T7 | Connector: tmux `set-clipboard on` behind a feature/switch + migration of existing `tmux.conf` (or a runtime `set-option -s`); docs | M | D3, T6 |
| T8 | `terminalproto`: `upload-v1`, `Upload` body, `StreamHello` op `upload` (negotiated only), result codes, `Capabilities.Uploads/UploadsOff`; encode/decode/compat tests | M | D1 |
| T9 | `terminalhost/upload.go`: validation, private dir, O_EXCL/O_NOFOLLOW write, quotas, cleanup on end/reconcile/start, max-age sweep; tests incl. symlinked dir, wrong magic, oversize, quota, cleanup | M | T8 |
| T10 | `terminalagent`: handle `Upload` (stream read, final result frame), local switch `reet agent terminal uploads` | M | T9 |
| T11 | Broker `Upload()` (reverse stream write, timeouts, `recordTraffic`) + API route with limiter, sniff, size cap, per-tool reference; tests: viewer 403, URL token 400, wrong origin 403, 413, 415, 429, old-connector 409 | M | T8, T10 |
| T12 | Web: image sources (paste items, `clipboard.read()`, drag-drop, file picker), upload chip, generation-fenced insertion, the "path only" note for non-Claude/Codex tools; unit + e2e (fixture machine) | M | T11 |
| T13 | Per-agent verification note (Claude/Codex/Gemini/opencode/Cline image path behaviour at pinned versions) recorded in `docs/terminal-launch.md`; adjust references | S | authorised installs |
| T14 | Docs (`terminal-workspace.md` clipboard and attachments section, privacy bullets) + handoff block | S | all |

T1–T5 are client-only and can ship first. T8–T12 are one vertical slice but split into separate PRs.

### 3.5 Security risks and the tests that cover them

| Risk | Mitigation | Test |
|---|---|---|
| Bracket escape (`ESC[201~` inside pasted text) | T1 sanitiser | unit: payload with markers and ESC is stripped; e2e: wire bytes contain one `200~`…`201~` pair |
| Clipboard poisoning via OSC 52 from any program in the shell | gesture gate, toast fallback, write-only, 1 MiB cap | unit: no gesture → no write; `?` query ignored; empty ignored |
| Path traversal or name injection | server-generated names; client name ignored | unit: `../`, NUL and long names never reach the filesystem |
| Symlink or TOCTOU in the upload dir | `ensurePrivateDir`, `O_NOFOLLOW|O_EXCL`, owner check | host test with a symlinked `uploads` and a pre-created file |
| Non-image or polyglot (SVG/HTML) | magic-byte sniff on controller **and** machine; fixed extension | 415 on SVG, HTML and a mismatched MIME |
| Disk exhaustion or DoS | 5 MiB per file, per-session and per-machine quotas, per-user limiter, concurrency cap | quota and 429 tests |
| Bytes leaking to logs, audit, DB or URL | streams only; audit metadata only; no query params | grep-style test that the audit row has no name or hash; route rejects a query string |
| Insertion into the wrong session after a takeover or reconnect | generation fence; no replay | e2e: reconnect mid-upload → nothing inserted, message shown |
| Cross-user or viewer upload | `s.terminal` + `TerminalSessionFor(uid)` | viewer 403, other user's session 404/403 |
| Prompt injection inside an image (text in a screenshot) | out of scope for the transport; mention in docs: images go to the model provider like typed text | docs |
| Gemini workspace restriction quietly failing | per-tool reference + "path only" note until verified | T13 |
| Files left behind | end/reconcile/start cleanup + 24 h sweep | host tests for each trigger |

### 3.6 Decisions only the owner can make

- **D1. Where images live on the machine.** (A) Private `<terminal dir>/uploads/<session>/`, 0700/0600, outside the project: nothing lands in the repo and the privacy story is clean, but Gemini (and possibly others) cannot read outside the workspace. (B) RT-style `<session dir>/.reet-pasted-images/` with a self-ignoring `.gitignore`: works for every agent and sandbox, but writes into the user's working tree, can be committed if someone deletes the `.gitignore`, and interacts with worktree remove/busy checks. **Recommendation: A**, with Gemini shown as "path only" until either `--include-directories` is added to its launch or B is approved for Gemini alone.
- **D2. Skip the paste review for bracketed-paste agent prompts** (≤ 64 KiB, no control characters). **Recommendation: yes.** This is the biggest "clunky" fix. Keep the review for shells without bracketed paste and for anything with control characters.
- **D3. Allow agent OSC 52 copy** (gesture-gated, write-only) and turn tmux `set-clipboard on` on machines. **Recommendation: yes, client gate first (T6), machine part behind a connector feature and switch (T7).** Re-read the deferred terminal security preferences note before deciding.
- **D4. Copy-on-select default.** **Recommendation: an option, default off** (Ctrl+C-with-selection already works; auto-copy surprises people who select only to read).
- **D5. Audit an upload?** **Recommendation: yes, metadata only** (machine, tool, type, size bucket). Or no event, if Activity should not show it.
- **D6. Uploads on by default on connectors**, with a local `reet agent terminal uploads off`. **Recommendation: on**, since it only writes to a private per-session directory and only for the session owner.
- **D7. Limits**: 5 MiB per image (matches RT and the commonly cited Claude limit), 20 images/50 MiB per session, 24 h max age. **Recommendation: as stated**; raise per file only if an agent supports more.
- **D8. Phone priority**: ship the file-picker Attach image and the Select text sheet before drag-drop. **Recommendation: yes.**

### 3.7 Honest gaps

- I did not run RT. Copy-on-release (`docs/guides/web/terminal.md:20`) is unverified in code.
- Agent CLI image behaviour (Claude, Codex, Gemini, opencode, Cline) is from memory, not from reading those CLIs. It must be verified before docs claim support.
- I did not check whether `git worktree remove` in reet refuses a worktree containing an ignored `.reet-pasted-images` folder (only matters for D1-B).
- I did not test xterm.js touch selection on a real phone. The claim that it is weak comes from xterm's known limitations and the handoff's note that no real-device phone test has been done.
