- **Download agent software from the controller first**: Admin → **Software artifact mirror** has a second choice,
  **Download from this controller first**, for an owner or admin. While the mirror is on, an updated machine then tries
  the controller's copy before the registry, and the registry only if that fails; the same checksums are checked either
  way. It is off by default, turns off with the mirror, and an older machine keeps trying the registry first. Fleet
  software says "tried first" beside the mirror when it is on.
