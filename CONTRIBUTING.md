# Contributing to reet.sh

Thanks for wanting to help. reet.sh is a small, self-hosted tool, and the aim is to keep it that way: one Go binary, one SQLite file, an embedded Svelte UI, no external services.

By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md). Your contributions are licensed under the project's [AGPL-3.0](LICENSE) licence.

## Ways to help

- **Report a bug.** Use the bug template. Include your version (`/api/version`, or the Admin page), how you run it, and the steps to reproduce.
- **Suggest a feature.** Open an issue first and describe the problem you are trying to solve. A short discussion before code saves everyone time.
- **Fix something.** Issues labelled `good first issue` are a gentle start.
- **Improve the docs.** Typos, unclear steps and missing examples all count.
- **Security problems** do not go in public issues. See [SECURITY.md](SECURITY.md).

## Development setup

You need Go 1.27.1 or newer and Node 22. The private-connection tests use a local
Tailcat relay and need loopback sockets and network-interface discovery; they do
not contact hosted relays or require root or a TUN device.

```sh
git clone https://github.com/eyupio/reet.sh && cd reet.sh
cd web && npm ci && npm run build && cd ..   # builds the UI into web/dist, which the binary embeds
go run ./cmd/reet serve                       # http://localhost:8080
```

For UI work run `npm run dev` in `web/`; it proxies the API to `localhost:8080`.

To try it without sign-in on your own machine: `REET_AUTH=none REET_DATA_DIR=/tmp/reet go run ./cmd/reet serve`.

## Before you open a pull request

Run what CI runs:

```sh
cd web && npm run check && cd ..
go vet ./...
go test -race ./...
```

Then check that:

- The change does one thing. Unrelated clean-ups belong in their own pull request.
- New behaviour comes with a test, and the test fails without your change.
- User-visible changes update the README (settings table, commands, screenshots) in the same pull request.
- No new dependency is added without a reason in the description. The binary stays small on purpose.
- The UI still works in light and dark mode and at phone width.

## Style

- **Go:** `gofmt`, small packages under `internal/`, errors returned rather than logged and ignored. Comments say why, not what.
- **Svelte/TypeScript:** follow the surrounding code; keep components small and the UI accessible (labels, focus order, reduced motion).
- **Prose:** British spelling, plain words, no marketing.
- **Commits:** imperative, plain sentence, no prefix ("Add tag merge to the admin page"). Keep each commit buildable.

## Pull request flow

1. Fork, then branch from `main`.
2. Make your change, with tests.
3. Open a pull request using the template. Say what changed and why, and how you checked it.
4. CI runs the web checks and Go tests. A maintainer reviews; expect questions, not just approval.
5. Pull requests are squashed or merged by a maintainer once green and approved.

## Brand assets

The logo and colours live in [`brand/`](brand/README.md). Please follow its usage notes if you use the marks, and do not present a fork as the official project.

## Questions

Open a discussion or an issue. There are no silly questions, only undocumented behaviour.
