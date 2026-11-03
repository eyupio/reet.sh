# Installing a private draft from the session picker

Date: 3 November 2026. Status: done. Supersedes the published-only ruling in
`2026-11-01-session-skills-install-design.md`.

## One install path

A session picker already listed the account's written skills. It now offers
**Install** for a private draft as well as for a published skill. The request
still goes to `POST /api/v2/terminal/sessions/{id}/skills/install`; the store
still queues one preview job, and the person still approves that exact plan.

For a draft, `SyncHostedFor` returns the controller-owned private revision as the
desired skill. The machine fetches those bytes from the controller with its
agent token. Nothing is published and no public URL is created.

## Copy

- The picker group is **Your skills**.
- A row's source line says **Your skill · published** or **Your skill · private draft**.
- A draft row has the same **Install** button as a published row.
- A skill deleted between listing and install is `missing`: the panel closes and
  reloads the picker. A draft whose private revision cannot be prepared is
  `unavailable`: the panel shows the reason and stays open.
- A newly created saved set is named **My written skills**.

## Rulings

- **One install path for drafts and published skills.** *Cost if wrong:* a draft
  depends on the controller private-revision store being available at install time.
- **No publish required.** *Cost if wrong:* an unpublished draft can be written
  into an agent's user-level skills folder, which is the person's own machine.
- **Use `missing` and `unavailable` codes.** *Cost if wrong:* old `unpublished`
  clients see an unknown code; the panel still closes on the legacy code.
- **Rename the source to “My written skills”.** *Cost if wrong:* existing saved
  sets keep the old name until they are saved again.

## Testing

- `store_skill_install_test.go` installs the draft fixture and checks a private
  desired source and the **My written skills** set name.
- `terminal_skills_install_test.go` checks the draft install job, private source
  and source name, and the `missing` refusal.
- `chat.e2e.mjs` checks both reet rows offer Install, the private-draft source
  line, and that a missing skill closes and reloads the picker.
- `skills.test.mjs` checks the **Your skills** group label.
