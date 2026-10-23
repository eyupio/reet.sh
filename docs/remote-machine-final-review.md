# Final implementation review

Reviewed the merged stages 0–9, protocol/store/host/broker authorization and
lifetime boundaries, daemon/native lifecycle, monitoring, and the browser
workspace. Stage 9 is merged in PR #132. This increment corrects the findings
below and adds regression coverage rather than changing the approved transport
or ownership model.

| Finding | Correction |
| --- | --- |
| A delayed create response could write a session into the URL after the user selected another host | Capture the original host/key; publish the session without changing a different selected host/session |
| A failed resource-health request blocked successful machine/session reads | Health failure is non-blocking, with an explicit notice and independently aged last observations |
| Invalid titles could lock the creation form behind an idempotency key | Validate the protocol's control/format/rune constraints before reservation; release keys only for definitive 400 rejection; keep uncertain keys |
| Root setup depended on root's user bus or GUI login | Root uses independent systemd system units / macOS LaunchDaemons; ordinary users retain per-user services; no automatic elevation |
| Monitoring UI was tied to terminal write access | Authenticated monitoring gets its own permission path; terminal routes remain browser-owner-only |
| Container CPU label implied cgroup/quota measurements | Clearly identify OS-visible observations from a container; they can reflect the host |
| Sync pause, revoke and inventory-forget wording obscured terminal effects | Describe sync-only pause, attachment revocation, surviving local shells, and inventory removal separately |
| A mobile modifier could affect an explicit paste; reconnect could steal search/dialog focus | Clear one-shot modifiers before paste/extra keys; fence clipboard reads/confirmations; preserve active dialogs and search focus |
| Maximised terminal lacked bounded focus/scroll behavior | Modal semantics, toolbar focus containment, body-scroll restoration, visual-viewport sizing and Alt+Shift+M restore; preserve shell Tab/Escape behavior |
| Active sessions sat below repeated headers and setup controls on mobile | Remove the duplicate page heading, compact health/creation into expandable panels, retain visible OS user and live/stale CPU/RTT summary |
| Hidden pages repeatedly polled unused history/resources | Pause background polling; fetch retained graphs only when expanded; abort/join component resources on cleanup |

The expanded health view includes recent RTT p50/p95, consecutive missed probes,
controller-observed reconnects, an honest unknown path, and observed terminal
payload totals for the selected history window. It does not equate payload bytes
with network throughput or fabricate unavailable values.

## Verification

The real browser fixture now deliberately fails health during initial load and
delays a real create while switching to another host (simulated selector metadata,
one real enrolled host). It verifies that terminal creation still works and the
late response does not rewrite the other host's session URL. It also confirms a
pending multiline paste is refused after an agent reconnect. Existing checks
retain original-shell/job durability, controller restart/rollback, Unicode,
alternate-screen and PTY resize, history/search, dark phone layout and privacy
inspection. Title boundary tests cover terminal/control/format characters.

Native CI runs the actual reet binary with independent agent/tmux services as
both the ordinary runner user and root on Linux/macOS, including lifecycle
commands and agent kill/restart while preserving the original tmux process/job.
These root gates must pass before merging this increment. Local Go host/scan
race tests, full Go vet/race, web checks/build, browser fixture, local relay and
security/build gates provide the remaining release evidence.

## Boundaries retained

No production deployment, upgrade or fleet pilot is claimed. Pilot real vim/top,
mobile IME, logout/reboot and external proxy/analytics configuration as described
in the [runbook](remote-machine-operations.md). Browser tests use a marked private
transport seam; real encryption and restored identity are separately gated via
local DERP. Shell state survives connection/process restarts while tmux lives;
OS process/scrollback restoration across host reboot is not provided. The server
flag remains off by default and host-local consent remains mandatory for root.
