## PHONE-DRAFTS-1: unsent drafts survive a reload, and the chat box stays typeable while it reconnects (24 November 2026)

Two items of the roadmap's "Phone and web": a phone browser may discard a tab or a reload may happen at any time, and
a half-written message was lost each time; and the chat message box went dead for the length of every short reconnect.

What "queued messages" turned out to be. There are two queues. An agent whose machine has live controls
(`acpLiveControls`) queues on the controller (`chatEnqueue`, shown by `ChatQueue.svelte`), so a reload already keeps
it. Every other agent queues in the browser only (`queued` in `ChatPane.svelte`, up to 10, with their pictures, and a
comment said "Nothing is stored"). Only the second needed work.

- `web/src/lib/terminal/drafts.ts`: one localStorage key, `reet.drafts.v1`, an array of entries, one per account and
  session, each with up to three text fields (`chat`, `compose`, `queue`). At most 20 entries (oldest dropped), at most
  32 KiB each (an entry over it is dropped whole, with any older copy, rather than half restored), expiring after 14
  days. Every read and write is in try/catch; with storage off nothing is kept and nothing else changes. Typing is
  written after 400 ms and when the page is hidden or closed; emptying a field is written at once, so a message just
  sent is never restored. `App.svelte` tells the module who is signed in (before the panes mount) and `signOut` clears
  every draft. The rules take the store as an argument and are tested in `drafts.test.mjs` (limits, expiry, other
  account, broken or rubbish storage).
- `ChatPane.svelte` and `TerminalPane.svelte` restore once per session, only into an empty box, without focusing it and
  without opening the Compose sheet (the Compose key gets a small dot when a draft is kept). A browser-held queue comes
  back as text, halted: the first message shows Retry and nothing is sent until it is pressed, so a reload can never
  send a message twice. Pictures are never kept, so a queued message that had one comes back without it.
- The chat message box is no longer disabled while the stream reconnects (nor is its dictation button). Send stays
  disabled with the visible line "Reconnecting. Your message is kept." (the box's description), `send()` still refuses
  while reconnecting, and nothing sends on reconnect. The terminal's Compose box already behaved this way and is
  unchanged. The chat Send and Stop buttons are now 44 px on a coarse pointer (they were 40).
- `web/tests/phone-drafts.e2e.mjs` (phone-chat group in `terminal-browser.yml`), 390 px, light and dark: restore,
  reconnect, no auto-send, sent clears, queue restore, other account, 15-day-old entry, rubbish and throwing storage,
  sign out, and the terminal Compose draft.

Not checked: a real phone browser discarding a tab (the tests reload the page); iOS Safari private mode; two tabs
open on the same session, where the last write wins. Known limits: an expired session (a 401) does not clear drafts,
because that would lose what was typed just before it expired; they stay under that account id until they expire in
14 days. The skills and prompts buttons still wait for a connected chat, as they read from the server. The in-memory
draft of a prefill from the session overview is still refused while reconnecting.
