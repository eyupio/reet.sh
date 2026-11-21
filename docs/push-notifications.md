# Notifications on a device

A phone, tablet or computer can be told when a session needs you, or has ended, while reet is closed. Turn it on per
device under **Notifications → On this device**. It needs an instance with accounts, reached over https.

## Turning it on

1. Open reet on the device and go to **Notifications** (the account menu).
2. Under **On this device**, choose **Turn on notifications**. The browser asks whether reet may show them.
3. Choose what this device hears about: **A session needs you** (an agent is waiting for an answer or an approval)
   and **A session has finished** (it finished its turn, ended, or stopped with a problem, and you have not looked
   yet). Both are on to
   begin with, and each device has its own choice.
4. **Send a test** shows one on every device you have turned on.

A tap on a notification opens that session, in a reet window that is already open or a new one. Signing out turns
notifications off on that browser, so the next person to sign in there is not told about your sessions; turn them on
again after you sign back in. **Turn off** stops this device; **Your other devices** lists the rest, each with **Remove**. One person can have 12 devices on.

**iPhone and iPad.** Safari only offers web push to a site on the Home Screen (iOS and iPadOS 16.4 or later). Tap
Share, then **Add to Home Screen**, open reet from there, and turn notifications on. In a browser tab the card says
so and offers no button.

**Other browsers.** Chrome, Edge, Firefox and Samsung Internet on Android, and Chrome, Edge, Firefox and Safari on a
computer, work in a tab. A browser with no web push says so on the card.

## What is told, and when

The controller looks at each person's sessions every five seconds, for people with at least one device on.

- **Needs you**: a running session is waiting (a terminal's bell, or a chat's pending approval or question) and you
  have not seen it.
- **Finished**: a session ended or stopped with a problem, or a chat finished its turn (or its turn stopped with a
  problem) and waits for your next message, and you have not looked since.

Each moment is told once: the same bell does not notify again, and a new one does. A moment is told when it is eight
seconds old, so somebody watching the session, who marks it seen by looking, is not told what they are looking at.
Anything older than fifteen minutes is not told, so turning a device on does not report an hour-old wait. Several
sessions changing together make one wake-up per device, which then shows up to four notifications, one per session.
A newer notification for a session replaces its older one.

The push service keeps a wake-up for an hour for a device that is off. Delivered late, it shows what is still true
then: a session you have since answered is not shown. When there is nothing left to show, or the device is signed
out, the notification reads "Something changed in your sessions." Browsers require a notification for every push.

A controller that has been [moved](backups.md) and fenced tells nobody; the copy that took over does.

## What leaves the controller

Nothing about a session. A wake-up is an empty request to the browser's push service, signed with this controller's
key. The device is woken, then asks this controller, over your own sign-in, what to show
(`GET /api/v2/push/pending`). So the push service learns that this controller pinged a device, and when; it never
sees a session's name, a machine's, or yours.

- The controller makes its signing key (VAPID, P-256) the first time **Notifications** is opened and keeps it in its
  database, in the `push_vapid_key` setting. A backup carries it, so devices stay on across a restore.
- A device's record is its push service address, a name such as "Chrome on Android", its two choices and when it was
  turned on. No subscription keys are kept, since nothing is encrypted for the device. The address is never sent
  back to a browser.
- The controller only posts to the push services browsers use: `fcm.googleapis.com` (Chrome, Edge, Samsung
  Internet), `*.push.services.mozilla.com` (Firefox), `*.push.apple.com` (Safari) and `*.notify.windows.com` (Edge
  on Windows), over https, and follows no redirect. Any other address is refused when a device registers. The
  controller needs outbound https to those hosts.
- The push services are told who runs the controller, as they require: `REET_CONTACT_EMAIL` if it is set, otherwise
  the controller's https address.
- A device the push service no longer knows (the browser's data was cleared, or notifications were turned off in the
  browser) is forgotten the next time it would be woken.

The service worker (`/sw.js`) does notifications only. It caches nothing and answers no requests, so reet loads from
the controller exactly as before, and there is no offline mode.

## API

All need a signed-in account; the two that change something need a writer.

| Request | Does |
| --- | --- |
| `GET /api/v2/push` | `available`, the `publicKey`, and `devices` (`id`, `label`, `waiting`, `ended`, `createdAt`, `lastOkAt`). With `X-Reet-Push-Endpoint`, `current` is the asking device's id. |
| `POST /api/v2/push/subscriptions` | Registers `{endpoint, label, waiting, ended}`, or updates the device with that endpoint. A browser that signs in as somebody else moves to them. |
| `DELETE /api/v2/push/subscriptions` | Removes one of your devices by `{endpoint}` or `{id}`. |
| `POST /api/v2/push/test` | Wakes every device you have on; each shows "Notifications are on" for the next two minutes. |
| `GET /api/v2/push/pending` | What a woken device shows: `notifications` of `{tag, title, body, url}`. |

## Verification

- `internal/webpush`: the signed request (an empty POST, `TTL`, `Urgency`, `Topic`, a token that verifies against
  the public key and not another controller's), a saved key read back, the push service address list, `404` and
  `410` meaning gone, and no request at all to an address that is not a push service.
- `internal/store` (`store_push_test.go`) and `internal/api` (`push_test.go`): a device registered, listed, changed
  and removed; one device belonging to one person; a waiting session told once, after it settles, and again on a new
  bell; a chat's finished turn told once while the session runs on, and a failed turn saying so; a seen session no
  longer pending; a device with a choice off not woken; an hour-old wait told to nobody; a
  gone subscription forgotten; no accounts, no push.
- `web/tests/device-push.e2e.mjs`: the card (on, choices, a test, off, a refusal, other devices, an iPhone in a tab,
  no accounts) at 390 and 820 px, and the real service worker woken by a push event through the browser's debugging
  protocol: it asks the controller, shows what it is given, never opens an address off the controller, and shows one
  line when it is given nothing.

Not verified: a real push through a push service to a real phone. This repository's tests never contact one, and
Safari and Firefox were not run.
