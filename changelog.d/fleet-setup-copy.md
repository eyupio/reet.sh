- **Copy one machine's agent setup to others**: the controller can read a machine's global instruction files
  (`CLAUDE.md`, `AGENTS.md`) and portable settings, show a plan for each chosen machine, and apply it one machine at a
  time, with Retry, Skip, Abandon and an Undo per machine. Model defaults, skills, MCP servers, memory and anything that
  runs a command are never copied. A copy that would let agents do more without asking needs the typed word `copy`. A
  machine accepts a copy only with terminal access and `reet agent allow-apply` on. The copied text is held only while
  the copy runs (24 hours at most) and never in a backup. `reet setup rollback --journal FILE` now works from the
  journal alone. The web page for it follows; this release adds the API.
