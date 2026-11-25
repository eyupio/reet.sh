## SKILLS-ON-DOCS-ORIGIN-1: the docs site publishes the built-in skills at /eyupio/<slug> (25 November 2026)

skills.sh lists the 14 built-in skills with the source `https://reet.sh/eyupio/<slug>`, and every one of those links
was a 404. The earlier fix (c459cbf) embedded the skills in the binary and served them at `/eyupio/<slug>`, which works
on a controller (`dev.reet.sh/eyupio/reet-repo-maintenance` answers 200), but `reet.sh` itself is the MkDocs site on
GitHub Pages and never runs the binary.

What changed.

- `hooks/docs.py` reads `internal/api/builtin_skills/*/SKILL.md` (the binary's own copies, so there is one source) and
  publishes each skill on the docs origin at the paths the binary serves: a themed page at `eyupio/<slug>/`, the file at
  `eyupio/<slug>/SKILL.md`, and `eyupio/<slug>/.well-known/agent-skills/index.json` with
  `.well-known/agent-skills/<slug>/SKILL.md` for the skills CLI. The page shows the skill in a fenced block instead of
  rendering it, because a skill has several h1 headings and the docs check allows one.
- `.github/workflows/docs.yml` uploads the Pages artifact with `include-hidden-files: true`; without it the
  `.well-known` directories are dropped from the artifact and the CLI finds nothing. The workflow also runs when
  `internal/api/builtin_skills/**` changes.
- The pinned CLI version in the page's install command is a copy of `ref.SkillsCLI` (`SKILLS_CLI` in the hook). Change
  both together.

Checked. `python -m unittest discover -s hooks -p '*_test.py'` (31 pass, one new), `mkdocs build --strict`,
`python scripts/check-docs.py` (74 pages). The built site served locally with `python -m http.server`:
`npx skills@1.7.4 add http://127.0.0.1:8765/eyupio/reet-repo-maintenance -y` installed the skill, and the page was
looked at in light at 1280 px and dark at 390 px.

Not checked: GitHub Pages itself (it only changes after this merges and the docs workflow deploys; then
`curl -I https://reet.sh/eyupio/reet-repo-maintenance/.well-known/agent-skills/index.json` should be 200),
`scripts/check-docs-browser.py` locally (CI runs it), and whether skills.sh re-reads the source on its own or keeps a
cached copy. The skill pages are not in the nav, so MkDocs lists them in an INFO line on every build; they are in the
sitemap, search and `llms.txt`.
