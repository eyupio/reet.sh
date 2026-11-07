# Home entry for session follow-ups

## Problem and behavior

Gaffer's existing Followed sessions overview was hidden inside Chat options. A person could have a goal waiting for a decision without seeing that on Home.

The compact Home summary now shows a touch-sized entry when a follow-up needs review or a goal is active. It opens the existing Followed sessions dialog directly, without opening general chat, reading provider configuration, creating a conversation, changing grants or starting a model turn. Goal review and explicit controls remain in the existing dialog. The entry is absent when there is no active/reviewable work. A failed first read offers an unavailable/retry entry; later failures retain known counts with a visible last-read label.

The summary uses the existing read-only goals endpoint, refreshed with Home's 15-second loader. Its answers share Home's generation and abort fencing. Successful explicit goal changes request an immediate refresh. Only account writers read these records or receive the entry; viewer and open-mode Home remain unchanged. Closing the standalone overview returns focus to the Home entry, or to the Gaffer launcher if stopping the last goal removed that entry. Existing general-chat entry and drafts remain available.

## Boundaries

- Worker approvals and Gaffer review counts have separate labels. Worker Needs you, the bell and document-title counts retain their existing semantics; no duplicate session identities are introduced into that list.
- No new panel or projects column is added. Compact projects, saved expansion, Home view preference and Planning board remain intact.
- No backend, schema, protocol or connector change is required. This slice needs a controller/web deployment. A merge alone is not deployment verification.
- Broader Home/bell/session attention unification needs an explicit identity and deduplication contract. Navigation and supporting-page refinements remain separate work.

## Acceptance and continuation

`web/tests/home-followups.e2e.mjs` passed six desktop/phone/theme views, a 44 px entry, no sideways scrolling, direct overview without general/provider reads, no implicit writes, explicit goal-stop confirmation and immediate count refresh, stale-read retention/retry, focus return, empty records, viewer/open-mode boundaries and standalone history-to-chat navigation. The last-goal focus probe failed before the launcher fallback and passed with it. It runs in Gaffer browser CI. Full existing Home, planning-board, followed-session, companion-chat and twelve-case widget layout suites passed. Web checks report zero Svelte diagnostics and the production build passed. Final exact-head CI/review remain release gates.

Preview captures: [desktop](previews/home-followups-desktop.png), [phone](previews/home-followups-phone.png). These use synthetic records and contain no production session data.

The preceding repair [#642](https://github.com/eyupio/reet.sh/pull/642) merged as `3952c332` after seven green workflows. This branch is `feat/home-gaffer-followups` in `/workspace/work/reet-home-followups`, based on that merge. Use `/workspace/.reet-tools/activate.sh` if the environment survives; never commit node_modules symlinks. Logs and preview captures are under `/workspace/work/redesign-review/home-followups-*`. Follow the user's standing authorization to merge ready, green work and continue scoped refinements.
