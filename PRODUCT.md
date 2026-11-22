# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users
Primary: a solo developer or homelabber who runs AI agents (OpenCode, Claude Code, Codex, Cline, Antigravity, Copilot) across several of their own machines and self-hosts one controller. They want calm oversight of what needs them and a quick way back into ongoing work.
Secondary: small teams sharing one installation (invites, accounts, published lists and skills). Design for the solo user first.

## Product Purpose
reet.sh is a self-hosted control room for a machine fleet, AI agents, skills and reusable setups. It provides browser terminals and desks, a central skill library synced to every connected machine, backups, structured agent sessions (Gaffer), and the Factory (goal-to-merge pipelines with approvals). Success: the user sees what is waiting for them, returns to ongoing work, and can bring an empty machine up to speed with one command.

## Positioning
Self-hosted, no telemetry, you own your fleet: one Go binary and one SQLite file, working with the agents already installed on your machines, with nothing leaving your control.

## Operating Context
Signed-out landing and entry pages (sign in, request invite, create account, set up instance), then a signed-in workspace: Home (Waiting for you, Continue working, Planning board), Sessions, Machines, Skills, Sync, Factory, Desks, and More. Public docs and showcase live at reet.sh; private entry and workspace pages are not indexed. Light, dark, or match-device appearance.

## Capabilities and Constraints
- Single Go binary, SQLite storage, AGPL-3.0, container image on ghcr.io.
- Public pages never load private inventories.
- Agent chat availability is reported per agent; some agents are gated behind a machine switch.
- Layout preferences are stored per browser and per account.
- Desktop-first, responsive down to smaller screens (Details moves under a toggle).

## Brand Commitments
Yorkshire voice ("Ey up. Your AI fleet, in good order."), the Gaffer mascot, and the logo and brand assets in `brand/` are binding identity commitments.

## Evidence on Hand
Real product screenshots in `docs/images/`, brand assets in `brand/`, existing docs in `docs/`, `UI_AUDIT.md` and `SEO_AUDIT.md`. Do not fabricate testimonials, customers, benchmarks or pricing.

## Product Principles
1. The user's own machines and data stay theirs: self-hosted, no telemetry, honest about what leaves.
2. Surface what needs the human first; everything else stays calm and out of the way.
3. Work with the agents and tools already on each machine rather than replacing them.
4. Show real state and last-known freshness; never imply certainty the system lacks.
5. Warm, dry personality in voice and mascot, never at the expense of clarity.
