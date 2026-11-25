## SOFTWARE-APPLY-FEATURE-1: a machine lists software-apply-v1 for its own dispatcher (25 November 2026)

A bug found while building the "download from the controller first" setting, and fixed on its own so it does not wait for that review.

**What was wrong.** A machine decides it supports a software apply when the controller offers `software-apply-v1`
(`cfg.softwareApply`), reports the capability `softwareApply`, and the controller then sends `software_apply`. The machine's
dispatcher (`bodyFeatureError` in `internal/terminalagent/agent.go`) serves that message only when the features it checks against
include `software-apply-v1`, and those come from `terminalFeatures(cfg)` in `internal/agenthealth/agent.go`, which never added it. So the
dispatcher answered every apply with an invalid-message error. Reproduced on a clean checkout of main: with the feature
negotiated, `capabilitiesFor` says `SoftwareApply=true` and `terminalFeatures` is empty. Tests did not catch it because they set the
feature header by hand rather than through the machine's own list.

**Effect.** Installing, updating, rolling back or removing OpenCode from reet's Fleet software page most likely never worked on a
real machine (it worked in tests). Once machines run this change, those requests will reach the apply code for the first time, so
expect the first real runs to find anything else that was hiding behind this.

**Fix.** `terminalFeatures` lists `software-apply-v1` when `cfg.softwareApply` is set. `TestSoftwareApplyIsListedForTheDispatcher`
fails without it (negotiated but not listed: "").

Not checked: an apply on a real machine against a real controller. I confirmed the list is wrong and the dispatcher's check, not a
live refusal.
