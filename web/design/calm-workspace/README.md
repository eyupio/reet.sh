# Calm workspace reference

Proposed design for review, using fictional data. No production components or APIs are imported. No external requests, analytics or agent calls are made; fonts are loaded from the repository's existing local font assets.

Open `index.html`, or serve the repository root with `python3 -m http.server 8765` and open `/web/design/calm-workspace/`.

Try the session rows, session search, Needs you filter, Details tabs, light/dark control, Home, Planning board, example permission response, local composer, New session and phone navigation. Reload resets the fictional data. Some destination controls intentionally explain the preserved production workflow instead of implementing it.

The prototype does not implement production routing, browser Back handling, persistence, reconnect, real terminal rendering, launch options or permissions. Its mode-switch explanation deliberately preserves the distinction between changing a view and restarting an agent process.

[Read the handoff](../../../docs/handoffs/calm-workspace-redesign.md) for scope, preservation contracts, staged implementation and review decisions. Approval of these materials comes before production implementation.

## Review verification

Browser checks cover 320, 390, 768 and 1440 px in light and dark themes, with no horizontal page overflow. Interaction checks exercise isolated session drafts, Details tabs and keyboard navigation, explicit mode-switch explanation, simulated allow/deny and shared attention counts, Home/Planning board, disconnected composer, literal message text, session search, fictional task capture and phone navigation focus return. No browser runtime errors or external requests were observed.

The captures show desktop at 1440 × 1000 and phone at 390 × 980. These checks validate this standalone fixture only. Production accessibility, contrast, terminal behaviour, browser history and backend contracts remain acceptance work for the implementation PRs.
