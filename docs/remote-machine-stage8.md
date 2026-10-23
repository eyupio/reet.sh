# Stage 8: terminal workspace and health UX

Updated product decision: root shells are supported when the agent/enrollment
belongs to root and terminal access is explicitly enabled locally. This replaces
the non-root restriction in stages 4–7. Root never becomes available through an
API privilege switch or browser-selected username; filesystem ownership checks,
local consent and account isolation remain mandatory. Services currently use
the owning user's service manager, including root's user manager on Linux.

The existing session selector resumes immutable logical sessions. Rename changes
controller metadata only. End requires confirmation, uses a stable per-session
request key on uncertain responses, and displays pending offline requests.
Creating/running/end-requested sessions count towards host (8) and account (32)
limits. Detach and switching sessions leave the shell running.

The terminal adds plain-text search, explicit selection copy and paste, font size
(10–22, the only persisted terminal preference), maximise/restore, and mobile
Esc/Tab/arrows/Enter/Ctrl+C plus one-shot Ctrl/Alt keys. Normal shell shortcuts
remain available; terminal search uses Ctrl/Command+Shift+F. Multiline, control
character and large pastes require an escaped preview confirmation. A connection
generation change invalidates that confirmation; clipboard reads are fenced too.
Pastes over 240 KiB are refused. xterm handles bracketed paste.

Recent history is a separate escaped, read-only snapshot (at most 2,000 lines /
256 KiB), never spliced into the live stream or persisted in browser storage.
History requests are aborted on teardown. Clipboard and OSC hyperlink controls
remain disabled for terminal-generated content.

Machine inventory includes compact health readings. Sync agent online/paused
labels explicitly describe sync, independent from private terminal connectivity.
Expanded 1h/6h/24h charts display minute CPU means and minute RTT medians;
missing observations are gaps rather than zeros. Each chart has an accessible
data table. Retained history never implies the host is currently connected.

Validation: web tests and Svelte diagnostics; production build; real Chromium,
API, broker, PTY and tmux test under Go race detection. The browser test checks
the original shell PID and long job across refresh/agent reconnect, renaming,
cancelled end, search, cancelled dangerous paste, bounded history, font sizing,
maximise, dark mobile layout at 390px, and no document overflow/page errors.
Screenshots are CI artifacts. Encryption and native supervision retain their
separate Linux/macOS gates; the browser fixture uses a private transport seam.

Stage 9 adds the gated rollout, failure/privacy coverage and operator runbook.
The requested final review follows stage 9 and will report its fixes separately.
