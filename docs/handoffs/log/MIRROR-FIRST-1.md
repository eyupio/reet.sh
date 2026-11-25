## MIRROR-FIRST-1: an owner or admin can have machines try the controller's artifact mirror before the registry (25 November 2026)

The second half of the roadmap's "Mirror resilience" item (the first, continuing a half-finished download, is
SOFTWARE-RESUME-1). The roadmap item is removed and the shipped list says so.

**Setting.** `software_mirror_first` beside `software_mirror_enabled`, written by the same `PUT
/api/v2/settings/software-mirror` (owner or admin only), which now takes `enabled`, `preferred` or both and records who and
when in `software_mirror_by`/`_at` as before. `preferred` is refused with 409 `software_mirror_off` while the mirror is off,
and turning the mirror off sets it off, so turning the mirror back on does not quietly bring it back. The view reports
`preferred` as mirror on and preference on. Activity records `software-mirror-first-on`/`-off`.

**Protocol.** New feature `software-mirror-first-v1`, advertised by the controller, claimed by a connector beside
`software-apply-v1` and reported as `softwareMirrorFirst` in its capabilities (validated: it needs `softwareApply` and the
negotiated feature). `SoftwareApply` gains `preferMirror` (omitempty). Both builders set it from the setting: the single apply
in `internal/api/software.go` and the batch through `softwarerun.Config.PreferMirror`. The broker clears it for a machine that
did not report `softwareMirrorFirst`, so an older machine's strict decoder never sees it; the connector's gate refuses it on a
generation that did not negotiate it.

**Found and fixed on the way.** The connector's terminal feature header (`terminalFeatures` in `internal/agenthealth`) never
listed `software-apply-v1`, while the terminal dispatcher refuses a `SoftwareApply` unless that header has it. Unless I misread
the wiring, a real machine answered every apply with an invalid-message error; the existing tests set the header by hand. It now
lists both software features, and `TestSoftwareMirrorFirstIsNegotiatedBesideApply` checks the header.

**Machine.** `stageSoftware` tries the mirror first when `preferMirror` is set and a mirror source exists, then the registry.
Same pinned artifact, same checksums, same `.part` file, same per-source limits (5 minutes registry, 15 mirror). The source
says which supplied it; both failing reports `mirror+registry`, and the error kept for the reason is the registry's, as before.
Without the flag the order and strings are as they were.

**UI.** Admin's mirror card has a second checkbox, "Download from this controller first", disabled while the mirror is off,
with its one-line explanation as the description. Fleet software's mirror line adds ", tried first". Labels in the mirror card
are 44 px on a coarse pointer.

**Checked.** Go: new tests in `internal/api` (setting, both builders, stripping for an older machine), `terminalproto`,
`agenthealth`, `terminalagent`, `softwarerun` and `terminalhost` (mirror first, fallback, registry first otherwise, both
source strings, the mirror's half continued by the registry with a Range request); each failed before its change. Web:
`npm run check`, `admin-settings.e2e.mjs` (new "mirror first" case at 390 and 1280) and `fleet-software.e2e.mjs` (new
"mirror first" case), which failed against the old build; screenshots of the card off, on and preferred at 390 and 1280 in
both themes were looked at.

**Not checked.** A real machine against a real controller, so the feature-header fix is untested end to end; the full
`internal/api` suite; a real registry or mirror over a slow link; screen readers and other browsers.
