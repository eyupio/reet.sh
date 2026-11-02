# Admin → Email: the outbox

Date: 2 November 2026. Status: done. Scope: the **Email** section of Instance
administration (`/admin/email`), its outbox and the existing mail settings.

## Why

The outbox was the last wide table under Instance administration. Its filters used
the server's whole-outbox counts rather than the messages the current state showed,
there was no search and no count of what was visible, and a phone had to scroll a
table sideways to read a long recipient or error. It now follows the collection
grammar used by Users, Team usage and Support.

## Contract

1. **One Outbox panel.** **Outbox** has a live message count and **Refresh**; the
   search and **All / Queued / Sending / Held / Delivered / Failed / Bounced** chips
   sit beneath.
2. **Counts follow the search.** Every state figure comes from the messages the
   current search leaves. The panel count is `N messages` or `N of M messages`.
3. **One load, client-side filters.** The page reads the outbox once and filters in
   the browser, so a state chip does not re-fetch and cannot promise more than
   pressing it shows. The default is **All**.
4. **One row anatomy.** Subject, template and whether it carries a sign-in link;
   recipient; a compact state badge; when it was written; its tries, next attempt
   and last error; and **Resend** only when the server says it may be resent.
5. **The settings stay.** The mail server summary, form, connection check, test
   email, DNS result and held-mail warning are unchanged above the outbox.
6. **Empty, loading and error live in the panel.** A search matching nothing offers
   **Clear filters**.
7. **No API or permission change.**

## Rulings

- **The outbox reads every retained message once.** This is what makes the state
  counts honest under a search. *Cost if wrong:* a larger initial response; the
  outbox is bounded by the server's mail retention.
- **Every state has its own chip, including sending.** *Cost if wrong:* seven chips
  wrap onto more lines on a phone.
- **The default is All.** The outbox is a log, and the newest problem should be
  visible without choosing a state first. *Cost if wrong:* a busy outbox shows every
  delivered message too.
- **The wide table is gone.** Each message is one row that stacks on a phone.
  *Cost if wrong:* scanning many messages takes more vertical space.
- **The settings remain above the outbox.** *Cost if wrong:* the log is one scroll
  further down the page.

## Testing

- Unit: `adminMail.test.mjs` covers search by subject, recipient, template, last
  error and id, state counts under a search, filtering order and the count label.
- Browser: `admin-email.e2e.mjs` covers the count and every state chip, search by
  subject and error, empty recovery, an unreadable outbox and its retry, the row
  anatomy and resend offer, and phone layouts at 390 and 320 px with no overflow.
- Regression: `phone-zoom.e2e.mjs` (the admin email screen), `navigation.e2e.mjs`,
  `npm run check` and the production build.
