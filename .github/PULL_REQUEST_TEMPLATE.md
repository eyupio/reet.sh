## What and why

<!-- What does this change, and what problem does it solve? Link the issue if there is one. -->

## How I checked it

<!-- Tests added, commands run, screenshots for UI changes (light and dark, phone width). -->

## Checklist

- [ ] `npm ci && npm run check` (in `web/`), `go vet ./...` and `go test -race ./...` pass
- [ ] `npm ci && npm test` (in `mcp/`, after `go build -o /dev/null ./mcp/test/fixture`) passes
- [ ] Added a handoff entry in `docs/handoffs/log/`
- [ ] Added a changelog fragment in `changelog.d/` if a person running reet would notice the change
- [ ] New behaviour has a test that fails without the change
- [ ] README and docs updated if anything user-visible changed
- [ ] No new dependency, or the reason is given above
