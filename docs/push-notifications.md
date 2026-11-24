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

A tap on a notification opens that session on its own view: a chat (where its questions and approvals are asked) on
the chat, a terminal on the terminal. A reet window already showing that session is brought to the front as it is, so
nothing typed there is lost; otherwise the reet window in front is moved to the session, and with no reet window open a
new one opens. A session removed before the tap opens the sessions list, which says "The session from that notification has ended or was removed." for about eight seconds. Signing out turns
notifications off on that browser, so the next person to sign in there is not told about your sessions; turn them on
again after you sign back in. **Turn off** stops this device; **Your other devices** lists the rest, each with **Remove**. One person can have 12 devices on.

**Play a sound.** Off to begin with, and chosen per device (the browser keeps the choice). With it on, a notification
that arrives while reet is open in front also plays a short, quiet two-note tone in that tab, once. A tab in the
background plays nothing, since the notification makes its own sound. **Try it** plays the tone; browsers let a page
play sound only after a press on it, so press it once after opening reet if the tone stays silent. The button reads
"Playing…" for about a second and the card says "Played", so a muted phone still shows the press worked.

**When notifications stop.** A browser can drop a device's subscription (its data was cleared, notifications were
turned off for the site, or the push service replaced the subscription) while reet still holds the device as on. The
card notices this when it opens and whenever reet comes back to the front: it compares the browser's own subscription
and permission with the device's record, and reads the controller's word that the push service no longer knows the
device. Then it says "Notifications stopped reaching this phone." (or "this device") with **Repair**, which subscribes
afresh and moves the record to the new subscription, keeping its name and choices (the browser may ask permission
again), and **Turn off**. Once either is pressed, focus moves to the card's heading, since the button it was on is gone.
The **Sessions** page makes the same check when it opens and whenever reet comes back to the front, never on a timer,
and shows one line under its search, "Notifications stopped reaching this phone." with a **Repair** link to the card
(`/notifications#device-push`); the line goes once the device works. A browser that never turned notifications on
asks the controller nothing. Nothing is
subscribed again until Repair is pressed, with one exception: when the browser itself replaces the subscription of a
device that is on, the service worker subscribes again with the same options and moves the record at once.

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
- A device the push service no longer knows (it answers 404 or 410) is marked dead the next time it would be woken,
  and nothing more is sent to it. Its record is kept, with `deadAt`, so the device says so and can be repaired; **Turn
  off** or **Remove** deletes it.

The service worker (`/sw.js`) does notifications only. It caches nothing and answers no requests, so reet loads from
the controller exactly as before, and there is no offline mode.

## API

All need a signed-in account; the two that change something need a writer.

| Request | Does |
| --- | --- |
| `GET /api/v2/push` | `available`, the `publicKey`, and `devices` (`id`, `label`, `waiting`, `ended`, `createdAt`, `lastOkAt`, `deadAt`, 0 while it works). With `X-Reet-Push-Endpoint`, `current` is the asking device's id. |
| `POST /api/v2/push/subscriptions` | Registers `{endpoint, label, waiting, ended}`, or updates the device with that endpoint. With `replace` (a device's id) or `previous` (its old endpoint) it moves that device to the new endpoint instead, keeping its name and choices, and answers 404 when you have no such device. A browser that signs in as somebody else moves to them. From a signed-in browser only: a personal API token is refused, so it cannot add a device that receives your notifications. |
| `DELETE /api/v2/push/subscriptions` | Removes one of your devices by `{endpoint}` or `{id}`. |
| `POST /api/v2/push/test` | Wakes every device you have on; each shows "Notifications are on" for the next two minutes. |
| `GET /api/v2/push/pending` | What a woken device shows: `notifications` of `{tag, title, body, url}`. A session's `url` is `/sessions?agent=…&session=…&from=push`. |

## Verification

- `internal/webpush`: the signed request (an empty POST, `TTL`, `Urgency`, `Topic`, a token that verifies against
  the public key and not another controller's), a saved key read back, the push service address list, `404` and
  `410` meaning gone, and no request at all to an address that is not a push service.
- `internal/store` (`store_push_test.go`) and `internal/api` (`push_test.go`): a device registered, listed, changed
  and removed; one device belonging to one person; a waiting session told once, after it settles, and again on a new
  bell; a chat's finished turn told once while the session runs on, and a failed turn saying so; a seen session no
  longer pending; a device with a choice off not woken; an hour-old wait told to nobody; a
  gone subscription kept, marked dead and sent nothing more, then repaired by id or by its old endpoint; a device
  removed meanwhile not brought back by the worker; no accounts, no push.
- `web/tests/service-worker.test.mjs`: the worker in node with stand-ins: a replaced subscription subscribed again
  with the same options and the controller told; a tap that brings forward the window already on that session, moves
  the reet window in front otherwise, or asks a window it cannot move; one tab in front told of a notification, none
  when every tab is hidden.
- `web/tests/device-push.e2e.mjs`: the card (on, choices, a test, off, a refusal, other devices, an iPhone in a tab,
  no accounts, stopped and repaired, stopped found by the remembered device, a device that dies while reet is in the
  background, the sound and Try it) at 390, 820 and 1280 px, a tapped session opening on its chat and a removed one on
  the sessions list, and the real service worker woken by a push event through the browser's debugging
  protocol: it asks the controller, shows what it is given, never opens an address off the controller, and shows one
  line when it is given nothing.

Not verified: a real push through a push service to a real phone, a real `pushsubscriptionchange`, and the tone
heard through real speakers. This repository's tests never contact a push service, and Safari and Firefox were not
run.
