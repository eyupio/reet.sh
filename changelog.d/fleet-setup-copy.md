- **Copy one machine's agent setup to others**: the controller can read a machine's global instruction files
  (`CLAUDE.md`, `AGENTS.md`) and portable settings, show a plan for each chosen machine, and apply it one machine at a
  time, with Retry, Skip, Abandon and an Undo per machine. Model defaults, skills, MCP servers, memory and anything that
  runs a command are never copied. A copy that would let agents do more without asking needs the typed word `copy`. A
  machine accepts a copy only with terminal access and `reet agent allow-apply` on. The copied text is held only while
  the copy runs (24 hours at most) and never in a backup. `reet setup rollback --journal FILE` now works from the
  journal alone. On the Setups page, **Copy to other machines**
  reads a machine, shows what each other machine would create, merge or leave alone (with the changed instruction lines and
  a plain reason for any machine left out), and lets you choose and order the machines. It asks once before it starts,
  and asks for the word `copy` only when a machine would let agents do more without asking. It follows the copy, offers
  Retry, Skip and continue and Abandon on a failure, and puts an Undo beside every finished machine.
