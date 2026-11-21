# Skill buddy catalogue review

Reviewed 21 November 2026. This is a candidate assessment, not a claim that a
skill is installed, tested on every agent, or responsible for measured savings.

## Recommendation

Ship a small, portable set of **reet-authored guidance**, with explicit stage
boundaries and no runtime dependencies. Offer upstream specialisms as **curated
sources to explore**, rather than silently copying their Markdown and claiming
their scripts, hooks and dependent skills work. Personal Active library skills
and deliberately public community metadata join the same recommendation search.

The repository catalogue is `internal/skillbuddy/defaults.json`. It contains 16
original instruction sets, an attributed integrated Unslop adaptation, and eight researched upstream references. Instance
administrators can override entries, add their own, disable them, or restore the
repository defaults. Upgrades do not overwrite instance overrides. Selection
pins instructions and revision for a future Factory stage.

## Upstream candidates read

| Skill and source | Best fit | Assessment and integration requirement |
| --- | --- | --- |
| [Unslop](https://github.com/heydamir/unslop/blob/main/skills/unslop/SKILL.md) | Design/polish | Strong fit for refining existing UI. Read with its reference documents; preserve the existing product character and keep CC BY 4.0 credit to Damir Sokolovsky (@heydamir). Used for this feature’s UI polish and provided as a compact, attributed, selectable built-in adaptation. The private `reet-unslop` copy was not accessible for comparison. |
| [Ponytail](https://github.com/DietrichGebert/ponytail/blob/main/skills/ponytail/SKILL.md) | Code, refactoring | Strong option for focused changes and reuse. The core is instruction text; the full plugin's lifecycle injection, modes and UI require native integration. Do not present its benchmark claims as reet savings. |
| [Systematic debugging](https://github.com/obra/superpowers/blob/main/skills/systematic-debugging/SKILL.md) | Failing code and checks | Useful root-cause process. It references supporting techniques and other Superpowers skills. Review the complete bundle and its workflow expectations before adoption. |
| [Verification before completion](https://github.com/obra/superpowers/blob/main/skills/verification-before-completion/SKILL.md) | Test and handover | Useful evidence discipline before a completion claim. Preserve Factory stage boundaries and existing user authorisation. |
| [Frontend design](https://github.com/anthropics/skills/blob/main/skills/frontend-design/SKILL.md) | Design | Useful visual direction and intentional typography/layout. Match the existing product and accessibility requirements. A design skill is not a substitute for browser verification. |
| [Web application testing](https://github.com/anthropics/skills/blob/main/skills/webapp-testing/SKILL.md) | Test | Practical local Playwright workflow. Requires Python, Playwright/browser availability and its helper scripts. A lone SKILL.md is not the complete toolkit. |
| [Web design guidelines](https://github.com/vercel-labs/agent-skills/blob/main/skills/web-design-guidelines/SKILL.md) | Review | Useful interface checklist. Fetches a separate upstream guidelines document, so inspect and pin that dependency for reproducible runs. |
| [Differential security review](https://github.com/trailofbits/skills/blob/main/plugins/differential-review/skills/differential-review/SKILL.md) | Security review | Detailed risk and caller analysis. Depends on multiple supporting documents and optionally a specialist agent. Keep it task-specific rather than forcing a full security audit on every change. |
| [GitHub CI investigation](https://github.com/openai/skills/blob/main/skills/.curated/gh-fix-ci/SKILL.md) | Babysit | Focused GitHub Actions diagnosis. Requires authenticated `gh` and a Python helper. Its approval workflow must fit, and never replace, the user's existing authorisation. |

These references remain discovery links to upstream `main`; they are **not pinned
executable bundles**. The `reviewed-2026-11-21` label dates the assessment, not an
upstream release or content hash. Imported library versions use reet's own
immutable revision IDs. Check upstream licences and preserve attribution before
redistributing complete bundles; no complete upstream toolkit is vendored. Unslop is an attributed compact adaptation with its upstream link retained.

## dev.reet.sh candidates

The user supplied an inventory summary: 124 rows, 85 distinct skills, 76 Active
and nine Archived. The connected reet MCP returned internal errors on both full
and filtered skill reads and the list read. Browser fallback reached sign-in.
Consequently the following assessment is **name-level triage of the user's
inventory**, not a content audit or independent verification of the counts.
No dev library entry was changed or adopted.

| Candidate group | Consider for | Next assessment |
| --- | --- | --- |
| `reet-architect`, `reet-api-design`, `reet-mcp-design` | Plan/design | Read contracts and tool assumptions; separate project-specific content from portable guidance. |
| `reet-product-design`, `reet-ux-ui`, `reet-ui-refresh`, `reet-unslop`, `aesthetic-usability`, `emil-design-eng` | Design/review | Compare overlap and select a complementary pair, rather than loading all design instructions. |
| `reet-test-coverage`, `requesting-code-review` | Test/review | Inspect evidence expectations and any dependent agent/skill references. |
| `reet-security`, `reet-observability-and-reliability` | Review/code | Inspect scope, permissions and runtime/tool dependencies. |
| `reet-performance`, `reet-cost-optimisation`, Go style/concurrency/performance | Relevant code work | Check measurement requirements, Go-specific applicability and conflicts with project conventions. |
| `reet-babysit`, `github-actions-efficiency` | Babysit | Check merge boundaries, CI-provider support and existing authorisation. |
| `find-skills`, `improve`, `askme`, `grill-with-docs` | Discovery/clarification | Avoid recursive discovery and repeatedly interrupting a task with questions. |
| Animation/logo skills | Specific design briefs | Recommend when the user requests motion or branding; check asset/tool dependencies. |
| The `seo-*` family | SEO work | Keep as a specialist group, check external service requirements, and offer relevant members only. |
| Paseo family | Paseo work | Offer only when the environment and task use Paseo. |
| Archived skills | None by default | Do not recommend automatically; restore deliberately before use. |

Display names are not identities. Deduplicate only with source and library
identity evidence; do not merge two same-name skills or import an inventory's
`local/` and `library/` assumptions into the runtime resolver.

## Evaluation still needed

Use comparable tasks to assess recommendation acceptance, skill loading,
completion/check outcomes and added context cost. Record model, agent, task
class, skill revision and available telemetry. Keep selection, prompt inclusion,
agent-reported use and independently observed outcomes separate. Current
insights report choices and prepared prompts; they make no causal savings claim.
