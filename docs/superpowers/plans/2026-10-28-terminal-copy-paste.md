# Terminal copy and paste, images included

Plan for the next PR after H (#231, merged as `7886511`). Input:
[docs/research/paste-research.md](../../research/paste-research.md) (T1–T14, decisions D1–D8). Owner decisions
(28 October 2026): its own PR after H; **D1** pasted images go to a private per-session folder on the machine,
outside the project (directory 0700, files 0600, random names). The controller defaults stand: **D2** skip the paste
review for bracketed-paste text with no control characters and at most 64 KiB; **D3** agent OSC 52 copy reaches the
clipboard only after a user gesture, write-only, at most 1 MiB; **D4** copy-on-select is an option, off by default;
**D5** uploads are audited as metadata only; **D6** uploads are on by default behind `upload-v1`, with a local
`reet terminal uploads off`; **D7** 5 MiB per image, 20 images or 50 MiB per session, 24 h lifetime,
PNG/JPEG/GIF/WebP by magic bytes, no SVG; **D8** phones get a file picker and a Select text sheet first.

## Global constraints

- Read `handoff.md` ("Resume here") and the plan's Global Constraints first. They bind this PR: no competing product
  names; never "verified"/"healthy" for a machine, session, worktree or ownership record; Yorkshire only in headings,
  ledes and success lines; prompts, pasted text and upload bytes never in audit, logs, browser storage or URLs.
- Terminal content is never stored by the controller or the browser. Upload bytes pass through controller memory only;
  audit rows carry machine, tool, MIME and a size bucket, never a name, path or hash.
- Disconnected input is never queued or replayed. Every insertion is fenced to the pane's live connection generation.
- Protocol compatibility: new controls and stream operations are sent only after feature negotiation; strict decoding
  means old peers must never see a new field or kind. `upload-v1` and `clipboard-v1` are added to the agreed controller
  features and negotiated by the connector. `terminalproto.Create` stays comparable.
- Limits: control messages ≤ 16 KiB, binary frames ≤ 32 KiB, queued output ≤ 256 KiB, paste cap 240 KiB.
- CI budget: **add no new fixtures to `terminal-browser`**. New browser fixtures go in `phone-dialogs` (10 minutes,
  headroom). Unit tests join `web/package.json` `test` so `npm run check` runs them.
- Phones: every new control is at least 40 × 40 px on touch (44 px in pane headers/tablet), inputs 16 px, no sideways
  scroll. Use the shared dialog shell (`use:shell`); do not edit `dialogShell.ts` or `dialog-shell.css`.
- Design fidelity: the session screen is the approved `docs/design/agents-home/Session.dc.html` and
  `PhoneSession.dc.html`. The new controls have no mock-up element; place them in the existing toolbar, context menu
  and drawers without moving anything else, and record every addition and deviation in `docs/ui-verification.md`
  at 1440×900, 834×1112 and 390×844.
- Process: implementer subagent per task, then a task review (spec and quality), fix rounds with a scoped re-review,
  then a broad review split by area and one fix wave. Ledger in `.superpowers/sdd/terminal-copy-paste/progress.md`
  (git-ignored). Failing-first tests for every task. Mutation checks restore files byte-identical (`cp` before,
  `cmp` after). The last task updates `handoff.md` before merge.

## Phase A — text paste and copy (client only)

### Task C1: Paste sanitiser and bracket-aware review

**Files:** create `web/src/lib/terminal/paste.ts`, `web/tests/terminal-paste.test.mjs`; modify
`web/src/lib/terminal/TerminalPane.svelte`, `web/package.json`; update the clipboard section of
`docs/terminal-workspace.md`.

**Behaviour**
- `sanitizePaste(text)` removes `ESC[200~` and `ESC[201~` wherever they appear, then every remaining ESC byte.
  Newlines and tabs stay. The function is pure and idempotent.
- `pasteNeedsReview(text, {bracketed, execute, force})` returns a reason: `execute` always reviews;
  any C0 other than `\n`/`\t` reviews; more than 64 KiB reviews; without bracketed paste, any `\r`/`\n` or more than
  2,000 characters reviews; `force` (saved commands with tabs) always reviews.
- `paste()` sanitises before review, caps at 240 KiB of the sanitised UTF-8 (refusal sentence unchanged), stores only
  sanitised text in `pendingPaste`, and skips the modal exactly when the rule says so. A skipped multi-line paste
  shows `Pasted N lines as one block.`; a single-line paste keeps the old silence.
- `confirmPaste()` sanitises again before `terminal.paste`; the dialog preview shows sanitised text.
- Nothing is sent when sanitising leaves nothing; feedback says `Nothing to paste.`

**Tests (failing first)**
- Unit: markers embedded mid-text removed; all ESC removed; tabs/newlines kept; idempotent; UTF-8 cap; every review
  branch; 64 KiB boundary; execute always reviews.
- Browser (`web/tests/terminal-clipboard.e2e.mjs`, `phone-dialogs` job): a paste containing `ESC[201~` reaches the
  wire with one xterm bracket pair; a multi-line paste with bracketed mode on sends without the dialog; with bracketed
  mode off the dialog appears; a control character always shows the dialog; the preview has no ESC.

### Task C2: Copy screen, copy last N lines, Copy all, copy-on-select

**Files:** create `web/src/lib/terminal/copy.ts`, `web/tests/terminal-copy.test.mjs`; modify
`TerminalPane.svelte`, `OutputScrollback.svelte`, `TerminalAppearance.svelte`, `TerminalIcon.svelte`,
`web/package.json`.

**Behaviour**
- `screenText(buffer, rows)` reads the visible viewport (`viewportY` … `viewportY + rows - 1`).
- `lastLines(buffer, n)` reads from `max(baseY, length - n)` to `length`; `n = 0` is empty. Lines use
  `translateToString(true)`; trailing spaces trimmed; blank lines kept; joins with `\n`; clamped to the buffer.
- Context menu adds `Copy screen`, `Copy last 50 lines`, `Copy last 200 lines`, `Copy all`; the toolbar adds
  `Copy screen` with its own icon. Every new copy path uses `copyText()` from `web/src/lib/clipboard.ts` and reports
  `Copied the screen.` / `Copied the last 50 lines.` / `Copied all terminal output.`
- `OutputScrollback` header gets `Copy all` using its transient snapshot; it never saves the text.
- Copy-on-select: an Appearance toggle (`Copy when I select text`) saved under `reet-terminal-copy-on-select`,
  default off; on selection change with a non-empty selection it copies the selection and says `Selection copied.`
- Empty copy says `Select terminal text first.` (unchanged for a selection; for screen/range say
  `There is nothing to copy yet.`).

**Tests (failing first)**
- Unit: viewport clamping, last-N clamping, blank/wrapped lines, empty buffer, `rows` beyond the buffer.
- Browser: context menu copies screen and last 50 lines from a fixture buffer; Copy all in Scrollback; the toggle is
  off by default, survives reload, and a selection copies only when on.

### Task C3: Select text sheet for phones and copy

**Files:** create `web/src/lib/terminal/TerminalSelectText.svelte`; modify `TerminalPane.svelte`,
`web/tests/phone-dialogs.e2e.mjs`, `web/tests/terminal-clipboard.e2e.mjs`.

**Behaviour**
- A bottom sheet (`use:shell={{sheet:'bottom'}}`) with a read-only `<pre>` holding the visible screen plus recent
  lines (at most 2,000 lines / 256 KiB), `Copy selection`, `Copy all` and `Close`. It is local and transient: text is
  discarded on close, never saved or logged. The pre is keyboard reachable.
- Toolbar button `Select text` (always present; first-class on phones), 40 × 40 px on touch.
- The sheet is added to the `DIALOGS` list in `phone-dialogs.e2e.mjs` and passes the shared shell contract.

**Tests (failing first)**
- Browser: the sheet opens from the toolbar at 390×844, its text matches the fixture buffer, Copy all writes the
  mock clipboard, closing discards it, and it meets the phone dialog contract at 320, 360, 390, 412, 844×390 and
  915×412.

## Phase B — agent copy (OSC 52)

### Task C4: Gated OSC 52 in the browser

**Files:** modify `TerminalPane.svelte`; extend `web/tests/terminal-clipboard.e2e.mjs`.

**Behaviour**
- Replace the blanket `registerOscHandler(52, () => true)` with a write-only handler:
  - accept only `c;<base64>`; ignore `?` queries and empty writes (never clear the clipboard);
  - cap the decoded payload at 1 MiB; ignore anything larger (feedback: `The agent tried to copy more than 1 MiB.`);
  - if a keyboard or pointer gesture reached this pane within 2 s and the page has focus, write the text with
    `navigator.clipboard.writeText` and say `Agent copied to your clipboard.`;
  - otherwise keep the text in memory only and show a banner: `The agent copied N. Copy it?` with `Copy it` and
    `Dismiss`; `Copy it` is the gesture that writes. Never read the clipboard back.
- Track the last gesture on the pane's keydown and pointerdown. Clear the pending text on close, session change and
  on `access_denied`.

**Tests (failing first)**: no gesture → no clipboard write, banner shown, `Copy it` writes;
`?` ignored; empty ignored; > 1 MiB ignored; gesture within 2 s writes; the handler consumes the sequence.

### Task C5: Machine OSC 52 forwarding behind `clipboard-v1`

**Files:** create `internal/terminalproto/clipboard.go`, `clipboard_test.go`; modify
`internal/terminalproto/protocol.go`; `internal/agenthealth/agent.go` (+ test); `internal/terminalagent/agent.go`
(if needed); `internal/scan/agent.go`, `agent_terminal.go`, `agent_terminal_control.go` (+ tests);
`internal/terminalhost/layout.go`, `manager_tmux.go`, `manager.go` (+ tests); `cmd/reet/terminal.go` (+ test);
`internal/api/private_control.go`, `internal/api/terminal_worktree_test.go` (expected feature list).

**Behaviour**
- `ClipboardFeature = "clipboard-v1"`, `SupportsClipboard`, `Capabilities.Clipboard` (requires `TerminalControl`,
  omitted when off). Add it to `agreedControlFeatures`, `negotiateFeatures`, `terminalFeatures`, and the capability
  builder (negotiated, terminal control on, terminal enabled, local switch on).
- Local switch `reet terminal clipboard on|off|status` (same as `reet agent terminal clipboard …`), default on,
  stored as `clipboardOff` in the agent config; `ManagerOptions.ClipboardConsent`.
- The manager applies `set-option -g set-clipboard on` (or `off`) when it prepares the tmux server for a session, so
  the forwarding follows the switch without a config rewrite. `allow-passthrough` stays off.
- A controller that has not negotiated `clipboard-v1` never sees the capability. The browser's gate is independent.

**Tests (failing first)**: token/capability round-trip and strictness; switch on/off/status and capability removal;
manager sets the option on and off; expected feature list updated.

## Phase C — image paste and upload

### Task U1: Protocol `upload-v1`

**Files:** create `internal/terminalproto/upload.go`, `upload_test.go`; modify `protocol.go`, `stream.go`,
`protocol_test.go`.

**Behaviour**
- `UploadFeature = "upload-v1"`, `SupportsUpload`.
- `Capabilities.Uploads` and `UploadsOff` (both require `TerminalControl`; never both).
- `Upload{StreamToken, Size, SHA256, MIME}` kind `upload`, session-bound, `valid()` enforces a valid token,
  `1..5 MiB`, 64 lowercase hex SHA-256, and one of `image/png`, `image/jpeg`, `image/gif`, `image/webp`.
- `StreamHello.Operation` accepts `upload`; other operations unchanged.
- `UploadResult{Code, Path}` is the final text frame on the reverse stream. Codes: `ok`, `disabled`, `too_large`,
  `type`, `quota`, `mismatch`, `failed`, `conflict`; `Path` is required only for `ok`, absolute, plain text,
  ≤ 4,096 bytes.
- `SniffUploadMIME(header)` returns the MIME and extension from magic bytes (PNG, JPEG, GIF87a/89a,
  RIFF…WEBP); SVG, HTML and unknown are rejected. Shared by controller and machine.
- `MaxUploadBytes = 5 << 20`, `MaxUploadsPerSession = 20`, `MaxUploadSessionBytes = 50 << 20`,
  `MaxUploadMachineBytes = 200 << 20`.

**Tests (failing first)**: strict round-trip; limits; capability rules; legacy JSON omits the fields; upload hello
accepted, unknown operation rejected; sanitised/ambiguous JSON rejected; magic bytes for each accepted type and for
SVG, HTML, short and mismatched input.

### Task U2: Private upload storage on the machine

**Files:** create `internal/terminalhost/upload.go`, `upload_test.go`; modify `manager.go` (cleanup hooks) and
`manager_test.go`.

**Behaviour**
- `Manager.WriteUpload(ctx, sessionID, req, src)` checks consent, the session is `Running`, and machine/session
  quotas, then streams to `<layout.Dir>/uploads/<sessionID>/`.
- Directory 0700 through `ensurePrivateDir` (owner-checked, no symlinks); file 0600, created `O_CREAT|O_EXCL|
  O_NOFOLLOW` with a random `img-<32 hex>` name and the extension from the sniffed type. The client supplies no name.
- While streaming: reject more than the declared size, a SHA-256 mismatch, magic bytes that are not an image or do
  not match the declared MIME. Verify before publishing the final name.
- Quotas: 20 files / 50 MiB per session; 200 MiB for the machine. One upload mutex serialises quota checks and writes.
- Cleanup: delete a session's folder when it becomes `Ended`, `Exited` or `Lost` (End and reconcile paths), on
  connector start for any session that is not running, and sweep files older than 24 h. Never touch anything outside
  `uploads/`.
- Errors are typed (`ErrUploadDisabled`, `ErrUploadTooLarge`, `ErrUploadType`, `ErrUploadQuota`,
  `ErrUploadMismatch`, `ErrUploadBusy`) and contain no path, name or hash.

**Tests (failing first)**: permissions and owner; symlinked uploads dir refused; pre-created target never
overwritten; oversize, wrong magic, MIME mismatch, SHA mismatch; session and machine quotas; cleanup on End, Exited,
Lost, start and 24 h sweep; path stays under the uploads root; random name and fixed extension.

### Task U3: Connector handler, switch and capability

**Files:** create `internal/terminalagent/upload_test.go` (or extend the agent tests); modify
`internal/terminalagent/agent.go`, `internal/terminalhost/manager.go` (options),
`internal/scan/agent.go`, `agent_terminal.go`, `agent_terminal_control.go` (+ tests),
`internal/agenthealth/agent.go` (+ tests), `cmd/reet/terminal.go` (+ test).

**Behaviour**
- `reet terminal uploads on|off|status` (also `reet agent terminal uploads …`), default on, stored as `uploadsOff`.
- The connector advertises `Uploads` when `upload-v1` was negotiated, terminal control is on, the terminal is enabled
  and the switch is on; `UploadsOff` when the switch is off. Both disappear while terminal access is off.
- The agent handles `Upload` only on a connection that negotiated `upload-v1`: it opens the `upload` reverse stream,
  streams frames into `Manager.WriteUpload`, then sends one `UploadResult` text frame and closes normally. At most two
  uploads per connection; a third is `conflict`. A switch turned off mid-connection refuses with `disabled`.

**Tests (failing first)**: switch on/off/status and capability; unnegotiated `Upload` refused; happy path writes
the file and returns the path; oversized/mismatched/typed failures map to result codes; two concurrent uploads and a
refusal for a third; no upload while the switch is off.

### Task U4: Broker transfer and controller route

**Files:** create `internal/terminalbroker/upload.go`, `upload_test.go`, `internal/api/terminal_upload.go`,
`terminal_upload_test.go`; modify `internal/terminalbroker/broker.go` (if the receive switch needs it),
`internal/api/terminal.go` (route + error mapping), `internal/api/api.go` (limiter state), and the expected
feature-list test.

**Behaviour**
- `Broker.Upload(ctx, uid, sessionID, data, mime)` rechecks ownership and `Running`, the peer generation and the
  `Uploads`/`UploadsOff` capability, expects the `upload` stream, sends the `Upload` control with the SHA-256 and
  size, writes ≤ 32 KiB frames with deadlines, closes normally, reads one `UploadResult` text frame, validates it
  and returns the path. No retry; a late reply is dropped safely.
- `POST /api/v2/terminal/sessions/{id}/uploads` behind `s.terminal`: exact image Content-Type, `MaxBytesReader`
  5 MiB + 1, magic-byte sniff matching the declared type, per-user 20/minute limiter and at most 2 in flight,
  session `Running` and owned by the user.
- On success: audit `terminal-upload` with machine host, tool, MIME and a size bucket (no name, path or hash); reply
  `{path, reference}` where `reference` is the shell-escaped absolute path and the UI says it is a path (per-agent
  image behaviour is not verified).
- Errors: 409 `upload_unsupported`, 409 `uploads_off`, 413, 415, 409 `upload_quota`, 429 `upload_rate_limited`,
  502/503/504 for machine failures. No path or hash in any error sentence.
- Expected controller feature list gains `upload-v1` and `clipboard-v1`.

**Tests (failing first)**: broker refuses old/off peers before sending; forwards exactly once; frame sizes and
traffic accounting; result mapping; timeout/disconnect fail without replay; API viewer 403, URL token 400, wrong
origin 403, 413, 415, 429, 409 unsupported/off; audit row has metadata only; happy path with a fake connector
returns the path and reference.

### Task U5: Web image sources and insertion

**Files:** create `web/src/lib/terminal/upload.ts`, `web/src/lib/terminal/UploadChip.svelte`,
`web/tests/terminal-upload.test.mjs`, `web/tests/terminal-upload.e2e.mjs`; modify `TerminalPane.svelte`,
`TerminalWorkspace.svelte` (pass the capability), `api.ts`, `web/package.json`, `docs/terminal-workspace.md`.

**Behaviour**
- Sources: image items in the pane's paste event; the Paste button via `navigator.clipboard.read()` (text still
  wins when both are present); drag-drop of files on the screen frame; `Attach image` with a file picker accepting
  PNG/JPEG/GIF/WebP. Phones get the picker first.
- `validateImage` rejects empty, > 5 MiB, SVG, HTML and unknown types before uploading.
- `terminalUpload` posts the raw blob behind the existing cookie/origin rules; it does not use the JSON helper.
- An upload chip shows `Uploading <name>…`, can be cancelled, and revokes any thumbnail object URL on close.
- On success insert ` <reference> ` into the live attachment with `terminal.paste`, no Enter, fenced by the pane
  generation captured at the start. If the generation changed, say `Uploaded, but the connection changed; nothing
  was inserted. Paste the path: …` and offer `Copy path`. Nothing is stored in the browser.
- `Attach image` is hidden when the machine reports neither `uploads` nor `uploadsOff`; when `uploadsOff` it says
  `Image uploads are off on this machine.` Older machines: `Update reet on that machine to attach images.`
- Every new control is 40 × 40 px on touch; the chip does not cover the terminal's last row.

**Tests (failing first)**
- Unit: validation boundaries, MIME set, path/reference escaping, generation fence.
- Browser: paste items, `clipboard.read()`, drag-drop and file picker upload (mocked route), chip progress and
  cancel, path inserted once with no Enter, reconnect mid-upload inserts nothing, capability-hidden button, 5 MiB
  refusal, no `localStorage` writes.

## Phase D — evidence and handoff

### Task T14: Docs, fidelity and the handoff block

**Files:** modify `docs/terminal-workspace.md`, `docs/terminal-launch.md` (path-only note; no unverified per-agent
claim), `docs/remote-machine-operations.md` (switches), `docs/ui-verification.md`, `handoff.md`,
`.github/workflows/terminal-browser.yml` (new fixtures under `phone-dialogs`).

**Behaviour**
- Document the sanitiser, the bracket-aware rule, copy screen/last N/copy-on-select, the Select text sheet, gated
  OSC 52 and its `clipboard-v1` switch, uploads (private folder, limits, 24 h cleanup, metadata audit, path-only
  note), and the compatibility story for older machines. Remove the sentence that OSC 52 is disabled.
- `docs/ui-verification.md` records the 1440×900, 834×1112 and 390×844 comparison against `Session.dc.html` and
  `PhoneSession.dc.html`: every addition, 40/44 px targets, no sideways scroll, the two new fixtures.
- `handoff.md`: state row for this PR, new limits and gaps, the queued order (models next), and the switch names.
- Run every e2e file from all three CI jobs locally in workflow order, plus the unit files, the matrix, and the Go
  suites. Record what was not checked (real phones, Safari, real agent CLIs).

## Test and CI matrix

| Check | Where |
| --- | --- |
| `terminal-paste.test.mjs`, `terminal-copy.test.mjs`, `terminal-upload.test.mjs` | `npm run check` (terminal-browser job) |
| `terminal-clipboard.e2e.mjs`, `terminal-upload.e2e.mjs` | `phone-dialogs` job (add after the existing four) |
| `phone-dialogs.e2e.mjs` Select text case | `phone-dialogs` job |
| Existing `terminal-workspace`, `terminal-lifecycle`, `terminal-organisation`, `terminal-phone`, `start-agent` | `terminal-browser` job (run locally, do not add cases) |
| Existing `phone-zoom`, `shell-fidelity`, `home`, `sessions`, `sessions-phone`, `launch`, `add-project` | run locally in workflow order |
| Go protocol/host/agent/broker/API tests | `go test -race -timeout 40m ./...` before the final push |

## Compatibility notes

- Old connector: no `upload-v1`/`clipboard-v1` token; the controller never sends `Upload`, and the browser shows the
  update sentence. Old controller: no token, so a new connector advertises neither capability.
- `Capabilities.Validate()` runs before a feature list is known in several paths; enforce the token only at the
  send/receive points, as browses does.
- `terminalproto.Message.Validate` binds `upload` to the session; `StreamHello.Validate` accepts `upload`; the broker
  receive switch must not treat an unexpected body as an observation.
- tmux.conf already disables clipboard and passthrough; the manager's server option is what flips forwarding, so an
  existing install does not need a config migration.
- Upload files are derived state outside the registry; the registry's metadata-only guarantee stays intact.
