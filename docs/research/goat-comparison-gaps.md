# What GOAT has that reet.sh lacks

17 November 2026. A prioritised list of candidate roadmap items, drawn from comparing reet.sh with
[goat-si/goat](https://github.com/goat-si/goat) (shallow clone of its default branch, its README, user
guides and architecture docs; neither product was run). This is a research note, not a commitment:
`docs/roadmap.md` stays the public statement of what is planned, and nothing here is on it until a
maintainer moves it there. The public side of the comparison is [compare-goat.md](../compare-goat.md).

Fit asks whether the item suits what reet.sh is: one Go binary, one SQLite file, no external
services, machines and skills at the centre. Size is a guess from the feature's shape, not from
reading the code that would change.

## Candidates, in order

| # | Candidate | What GOAT does | Fit | Size |
| --- | --- | --- | --- | --- |
| 1 | **Structured Claude Code and Codex chats on by default** | Native chat for Claude Code, Codex, Cursor, Grok, OpenCode, Antigravity, Pi, Muse, fx and ACP agents. | Already the first "Next" item in the roadmap. Provider breadth is GOAT's clearest lead, so finishing the device and login checks in [Device checks](../device-checks.md) matters most. | In flight |
| 2 | **Source control beyond GitHub in the session workspace** | Separate adapters for GitHub, GitLab, Bitbucket, Azure DevOps and Forgejo, with PR linking and CI status. | Strong. Self-hosters often run GitLab or Forgejo. The GitHub view uses the GitHub CLI signed in on the machine, so each forge needs its own route. | Medium, one forge at a time |
| 3 | **Scheduled and triggered agent runs** | Clock and webhook tasks with durable firing outcomes and a completion notification. reet.sh schedules backups only. | Good. The controller already holds the machine connections, Factory and the attention signal. Unattended runs need a clear approval and YOLO policy first. | Medium to large |
| 4 | **Goal-style Factory** | A fleet goal says what should be true (PRs merged, each worker reported). GOAT keeps nudging workers and can move them off a draining host. | Good. Builds on Factory, worker messages and merge approval. Host drain is the part with no equivalent today. | Large |
| 5 | **Private secret requests in Needs you** | An agent asks for a secret in a private card; the agent gets a reference, with optional 1Password lookup. | Reasonable. Fits the existing Needs you flow. Secrets must never reach the controller's logs or the session transcript. | Medium |
| 6 | **Read-only session sharing without an account** | Guest links (beta): one thread, one browser, 24 hours, messages wait for approval. | Partial. reet.sh has teams, so the audience may be covered by roles. Decide whether a no-account guest is wanted. | Medium |
| 7 | **Session memory** | Finished conversations become scoped memories, shareable between devices through a hub. | Unclear. It needs a model call, which conflicts with "no external services", and overlaps with the shelf. Needs a design decision before sizing. | Unknown |
| 8 | **A stronger installable web app** | Native desktop and mobile apps. | Weak for native, since a second client doubles the surface. A better home-screen experience for the existing web app is the proportionate answer; web push is already in. | Small to medium |

## Not recommended

- **Computer use and device emulators.** Both are about an agent operating a desktop or a phone. reet.sh is
  about the machine around the agent. Different product.
- **Cloud agents** (Claude Cloud, Cursor Cloud, Devin Cloud). reet.sh's premise is agents on machines you own.
- **A managed tunnel.** GOAT's is not shipped yet, and reet.sh's outbound connection already removes the need.
- **A Slack or iMessage assistant.** It would add an external service. Email and web push cover the same
  "tell me when it needs me" job. Revisit if people ask.

## What not to give up

Reviewed sync with a plan the machine rechecks, the central skill library and inventory, sealed off-site
backups, accounts with OIDC, outbound-only connections and no telemetry are the places reet.sh is ahead.
Any candidate above that needs an inbound port, a hosted service or a default-on data flow should be
declined or redesigned.

## Gaps in this comparison

- GOAT's history was condensed on 2026-11-01 and both clones are shallow, so release cadence and
  maturity are not compared.
- GOAT's docs were searched for SSO, teams, roles and backups and none were found; its code may do more
  than its docs say.
- `docs/start-agent-session.md` line 14 still lists `gemini`, `antigravity` and `copilot` as tool ids,
  while its Limits section says Gemini CLI is retired and the launch table has `claude`, `codex` and
  `opencode`, plus `cline`, `antigravity` and `copilot` where the connector advertises them. Worth a tidy.
