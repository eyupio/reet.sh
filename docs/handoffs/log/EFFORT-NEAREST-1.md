## EFFORT-NEAREST-1: a stage whose effort is not offered starts on the nearest lower level (27 November 2026)

Why. A stage whose chosen effort its agent does not offer started on the agent's default effort. A heavy tier set to
max on an agent that offers up to high could end up on medium, the opposite of what the person asked for. Other coding
agents step a request down to the nearest level the model supports; this does the same. Follows EFFORT-LEVELS-1.

What changed.

- `internal/gaffer/outcome_effort.go`: `effortNotReported` also returns `instead`, the heaviest listed level lighter
  than the one asked for (`nearestEffort`, on the ladder minimal, low, medium, high, xhigh, max; never "none").
  `dropStageEffort` takes `instead` and puts it in `Effort`. `EffortNotOffered` keeps the level the person chose, even
  when the level put in its place is refused in turn: that refusal still restarts on the agent's default, as before.
- The invariant changed: `EffortNotOffered` set no longer means `Effort` is empty. `askedEffort`, balancing, the choice
  check and its note (`effortNotOfferedNote` takes `instead`) read `EffortNotOffered` alone. The audit entry carries
  `instead`.
- Web: the stage's facts read "high effort (max is not offered)"; the Applied line's fallback text names the level; the
  pre-launch notes and the crew and pathway hints say "the nearest lower level the agent offers, or its default".
- `docs/factory.md` and `docs/gaffer-ai.md` say so.

Tests. `nearestEffort` on its own; the reported case with a lower level (medium), with only heavier levels (default);
the backup case; a level put in place and then refused (low, then default, with `EffortNotOffered` still high); the
choice check on the nearest level; and the web Applied line. Existing browser tests' copy was updated, not run.

Not checked. A refusal path that tries the next level down before the default (it goes straight to the default); agents
whose level names are not on the ladder keep the old behaviour.
