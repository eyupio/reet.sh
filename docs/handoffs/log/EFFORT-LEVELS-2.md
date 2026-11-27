## EFFORT-LEVELS-2: the launch bar offers xhigh and max for several projects when every machine takes them (27 November 2026)

Why. A review of EFFORT-LEVELS-1 found that with more than one project selected the launch bar had no single target
machine, so its effort picker read no capabilities and offered only low, medium and high. A saved xhigh or max default
still launched (each machine is checked by `startable`, the controller and the connector), but could not be picked.

What changed. `effortsFor` in `web/src/lib/launchBar.ts` is exported and takes every host the launch starts on: a
terminal launch offers the levels beyond high only when each of them reports `effortLevels`; a chat reads its one
machine as before. The sentence and `LaunchBar.svelte`'s effort slot both use it.

Tests. `launchBar.test.mjs`: two machines that both take the levels offer them; one that does not drops them. Web unit
tests and svelte-check pass. No browser suite was run.
