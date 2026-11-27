## CHAT-SWITCH-BACK-1: the header's Chat button switches a TUI session back to Gaffer (27 November 2026)

Reported: in a live session, someone could no longer switch between Gaffer and the agent's TUI.

Cause. The session header's Chat | Terminal switch (`SessionViewSwitch.svelte`) could restart a session one way only. From
Gaffer, Terminal with no shared native terminal opened the restart confirm (`terminalWay` → `restart`). From the TUI, Chat
always ended at "This terminal session has no shared conversation view", even when `switchModeState` allowed the switch
(a session that had moved from Gaffer to its TUI keeps its conversation id). The only way back was the Interface picker
under ··· (Session actions), where the calm redesign had moved it.

What changed.

- `sessions.ts`: `chatWay(switchState, chatView)`, the mirror of `terminalWay`: open, restart in Gaffer, or none with the
  switch's own reason.
- `SessionScreen.svelte`: the Chat button opens the same restart confirm when `chatWay` says restart, and its title says
  so; otherwise its title and notice give `switchModeState`'s reason. `SessionViewSwitch` takes a `chatReason` and tells
  `onunavailable` which button was pressed.
- `sessions.test.mjs`: a `chatWay` test.

Checked: `node --test src/lib/sessions.test.mjs`, `theme.test.mjs`, `svelte-check`. Not checked: a live session on a
machine (none in the cloud session) or a browser suite; no suite drives the header's restart today.
