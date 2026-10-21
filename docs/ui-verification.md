# UI verification — 21 October 2026

## Scope and method

The requested Web Design Builder skill was run and its complete output read. Playwright MCP is unavailable, so its MCP-specific phase was skipped. Separate local Playwright tests drove the installed Chromium browser, and axe-core checked rendered pages against its WCAG 2 A/AA and WCAG 2.1 A/AA rules.

The real app was tested against Go servers using isolated SQLite databases under the temporary work directory. Test accounts, invitations, API tokens, skills, lists, and settings were created only in those databases. No existing instance or data was changed. Browser tooling was installed outside the app; the app package manifest and lockfile are unchanged.

## Results

| Check | Result |
| --- | --- |
| `npm run check` | Pass; zero errors and zero warnings |
| `npm run build` | Pass |
| `go vet ./...` | Pass |
| `go test -race ./...` | Pass; all Go packages |
| `git diff --check` | Pass |
| Concept layout matrix | 342 passes: 19 views × 3 directions × 2 themes × 3 widths |
| Concept axe scans | 114 scans; no reported violations: 19 views × 3 directions × 2 themes at desktop width |
| Real app layout and axe matrix | 114 passes: 19 rendered states × 2 themes × 3 widths; no document overflow or reported violations |
| Browser runtime errors | None in completed concept and app runs |

Widths were 375, 768, and 1440 CSS pixels. Desktop captures used a 1000-pixel viewport height; phone captures used 812 pixels. Reduced motion was enabled for the main runs. Wide tables scroll within their labelled regions.

The 19 app states were account sign-in, token sign-in, owner setup with bootstrap, first-run token shelf, populated shelf, tools expanded, tag management dialog, account, shared list, list people, list settings, confirmation dialog, admin with users/rollup/settings, public list, unavailable public link, invitation acceptance, expired invitation, bookmarklet success, and bookmarklet error. Owner setup without bootstrap and a first-run account shelf also passed in the initial integration run.

## Interaction evidence

The explorer passed password mismatch handling, password visibility, sample sign-in navigation, search, likes, notes, removal confirmation, Escape to close the dialog, undo, state switching, and keyboard tab navigation. Phones use a labelled screen selector.

Real server-backed checks passed:

- Owner setup rejects mismatched passwords and creates a session after valid input.
- Show/hide password preserves the bound value.
- Shelf search changes server-backed results.
- Restore from JSON opens the file chooser from a focused button using Enter.
- A personal token is shown once; reloading removes its secret from the page.
- A public list excludes member identity.
- An invitation creates the intended account; a member cannot open admin.
- Failed account sign-in is announced; valid credentials open the shelf.
- Legacy token sign-in rejects a wrong token and accepts the temporary valid token.
- A configured bootstrap credential remains required for owner setup.

## Issues corrected

- Orange selection text measured 4.36:1. It now uses ink with the brand tint.
- Adoption badges measured 4.49:1. The success colour was darkened.
- Public-list primary button text was overridden by a generic action rule. Primary buttons now retain their foreground colour.
- A hidden username input inherited full width and caused token sign-in overflow. The width rule now applies only to visible inputs.
- Admin role selects now have accessible names identifying their user.
- Restore now uses a keyboard-operable button to activate its file input.

Screenshots in `docs/images/ui/` show the final concepts and implemented form. The explorer is `web/concepts/controller.html`.

## Limits

Automated axe results are not a WCAG certification. Screen-reader behaviour, physical touch interaction, high zoom, long translated content, Safari, and Firefox still need manual validation. Checks exercised representative data and roles, not every permission combination or every control on every page. Explorer failure states demonstrate designs rather than real network recovery.

The explorer has no external asset dependencies. The production build emits roughly 62.4 kB of CSS and 177.1 kB of JavaScript (10.5 kB and 54.5 kB gzip). These are build sizes, not measured load times. The explorer is outside the production entry graph.

No deployment was performed. The three directions remain concepts; the app contains the shared Library foundation and improvements described in `ui-redesign.md`.
