# Interactive host setup and verified native handoff

The reported failure showed a connected agent with a start time from the previous
day, still labelled `background/foreground process` after native service install.
That standalone process held the enrollment lock while the new service competed
for it. Controller access being enabled did not prove host terminal readiness.

Interactive `reet terminal start`/`restart` now continues into local-host setup
once controller access is available. It inspects the enrollment, asks before
enrollment or local shell consent, and offers missing tmux installation and native
service repair. Root access is explicitly labelled in the consent question.
Existing answers/configuration are reused. Status/stop do not prompt; unattended
controller commands do not silently grant host access. `--agent-config PATH`
selects a non-default enrollment without replacing other identities.

The generic questions use a real free-form prompt reader, with domain validation
in the wizard. The previous keyed URL/release helper rejects generic labels and
was unsuitable for these questions. Its existing validated behavior remains;
the new helper reads one answer without consuming the next one. Enrollment
credentials use hidden input and private storage; installers strip REET variables
from their child environment.

Service install preflights tmux ownership, takes the existing lifecycle start
lock, and gracefully stops this enrollment's standalone agent through its private
socket before installing/restarting the native unit. It preserves tmux and refuses
foreign/unmanaged services. It verifies native supervision and both service states
before reporting success. Systemd failed-unit state is reset to recover after the
previous lock contention. Prefix-bound services can use `--service-prefix`.

Host setup rechecks consent/enrollment, local manager validity and private
connectivity before reporting completion. Deferred steps and failures are reported
explicitly; a controller-only success never claims that the host is ready.

Validation includes a pseudo-terminal test of the actual start-command prompt,
declined consent/enrollment safety, existing prompt compatibility and local Go
race tests. Native Linux/macOS gates reproduce the running-standalone/install
handoff with the real binary and verify that the original tmux job survives.
