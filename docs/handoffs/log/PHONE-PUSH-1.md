## PHONE-PUSH-1: a dead push subscription is noticed and repaired, a tap opens the session, and an optional sound (24 November 2026)

Part of the roadmap's "Phone and web" item. Notifications on a phone could stop without anyone knowing, a tap could
land on whatever page a reet window was showing, and there was no sound while reet was open.

- **Dead subscriptions.** A push service answering 404 or 410 used to delete the device's row, so the phone still
  showed On and heard nothing. Now `push_subscriptions.dead_at` (new column, added in `migratePush`; old rows read 0,
  working) marks it, `pushWake` skips it, and `PushUsers` ignores people with only dead devices. `POST
  /api/v2/push/subscriptions` takes `replace` (id, from the page) or `previous` (old endpoint, from the worker) and
  moves that record to the new endpoint, keeping label and choices (`ReplacePushSubscription`); 404 if it is not yours,
  so a device removed elsewhere is not brought back. The worker handles `pushsubscriptionchange` only when the event
  carries the old subscription; without it nothing says the device was on. The card (`pushHealth` in
  `pushSupport.ts`) reads stopped when the controller marked the device dead, permission is no longer granted, or the
  browser no longer has the subscription of the device it remembers turning on (`reet.push.device` in localStorage,
  set on enable and repair, cleared on off and sign-out). It checks on load and on `visibilitychange`. Repair always
  unsubscribes and subscribes afresh; nothing subscribes without the press.
- **Taps.** The pending URL gains `from=push`. `notificationclick` brings forward a window already on that session
  (matched on path and `session`, since the workspace adds `view=terminal` to its address) without reloading it,
  else moves the focused or visible reet window, else opens one; a window it cannot `navigate` (not controlled) gets
  `{type: 'reet-open'}` and moves itself (`listenToWorker` in `push.ts`, started in `main.ts`). The view needed no new
  parameter: a structured session opens on its chat, where its questions and approvals are, and a terminal session
  has only the terminal. With `from=push` and the session not listed, the workspace replaces the address with
  `/sessions?view=list&gone=1` and the Sessions page says so once ("The session from that notification has ended or
  was removed."), and clears it after about eight seconds or on `popstate`.
- **Stopped, outside the card.** `stoppedDevice()` in `push.ts` is `readPush()` behind a gate (it asks the
  controller only when this browser has a remembered device or a subscription). `SessionsPage.svelte` calls it on
  mount and on `visibilitychange`, never on a timer, and shows one line in its reserved notice slot (after any other
  notice) with a Repair link to `/notifications#device-push`; `DevicePush` scrolls to that anchor once its read
  answers. After Turn on, Repair or Turn off the card moves focus to its heading, as the pressed button is gone. Try it
  uses `aria-disabled` (focus stays) for a second and says "Played" in the status line.
- **Sound.** `pushSound.ts`: off by default, `reet.push.sound` in localStorage, two sine notes (660 and 880 Hz) at a
  peak gain of 0.05, about 0.4 s. After showing notifications the worker posts `reet-notified` to one window only, the
  focused one or else a visible one, so several tabs never play it together and a hidden tab never does.
  `shouldChime` also keeps it to once a second.

Checked: `go test ./internal/webpush/ ./internal/store/ -count=1`, `go test ./internal/api/ -run TestPush -count=1`,
`gofmt`, `go vet`, `npm run check`, `tests/device-push.e2e.mjs` against a built preview, and screenshots of working,
stopped and sound on at 390 px and 1280 px in light and dark.

Not checked: any real push service, a real `pushsubscriptionchange` (the worker is tested in node with stand-ins), the
tone through real speakers or on iOS (Web Audio there may need the Try it press after every launch), Safari and
Firefox, and the full `internal/api` suite. A device enabled before this change has no remembered id until the card
is opened once while it works, so a subscription that vanished before then reads as off rather than stopped.
