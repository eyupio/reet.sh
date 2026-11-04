# Agent image previews handoff

Updated 4 November 2026. Branch: `feat/chat-image-previews`. Read [the complete continuation handoff](gaffer-continuation.md)
for all related PRs, current heads, merge gates and portable commands. The PR attached to this branch records live review/CI state.

## Implemented behaviour

- Preserve embedded image and image resource blocks in agent/tool output. Keep surrounding text and tool status.
- Capture files explicitly named by completed read/edit tools, including sparse completions that omit the original path/kind.
  Also recognise local Markdown image references across text chunks. No directory scanning or remote URL fetching.
- Capture current file contents at the time the update is processed; replayed path-only tools do not prove the original bytes.
  Return immutable snapshot IDs, so a later file edit does not change a displayed preview.
- Negotiate `acp-previews-v1` independently. Older controllers receive no preview fields/events; ordinary conversation remains compatible.
- Persist private per-session content-hash snapshots under the connector layout. Atomic replacement repairs interrupted captures.
  Bound the cache to 64 MB/1,024 files; refuse reads after 24 hours and prune expired files/empty directories on capture.
- Accept PNG/JPEG/GIF/WebP only, up to 5 MB, 16,384 pixels per edge and 32 million pixels. File fallbacks require a regular
  file owned by the connector user under its home or temporary root; scoped opens prevent symlink escapes and special files do not block.
- Fetch 32 KB private chunks through account/session checks; validate offsets, size, MIME and the final SHA-256 before serving.
  Four controller reads may assemble concurrently. No browser-supplied file path is accepted. Images use no-store/nosniff headers.
- Show lazy inline previews and the existing full-size keyboard/touch viewer. Offline failures can be retried; missing/expired,
  oversized and unsupported images retain labelled placeholders. Retire object URLs and abort stale requests when cards leave view.
  Keep the scroll position when releasing previews and follow new image layout only while the user is already at the conversation end.

## Validation performed

- Web unit suite, Svelte check (zero errors/warnings) and production build pass.
- `go vet ./...` passes.
- Focused race suites pass for preview extraction and streamed paths, protocol bounds, cache persistence/isolation/expiry/quota,
  sparse tool targets, negotiated delivery, chunk/hash/ownership checks and HTTP reply filtering/authentication.
- Desktop/phone light/dark browser regression passes, including offline retry, decoding, lightbox focus/Escape, replay,
  limits, no overflow and object URL cleanup. It is included in terminal-browser CI.
- Screenshots are tracked in `docs/images/ui/agent-preview-*.png`, linked from `docs/ui-verification.md`.

## Remaining work and limitations

Main #495 and the latest #490 phone regression are integrated. The toolbar/panel conflicts are resolved by retaining both
features, with terminal size held during context-panel resizing. Integrated web check/build, the four preview browser cases and the full workspace panel browser suite pass.
Inspect final-head CI and review, fix any findings and merge only when green. The complete six backend package race suites
and full browser workflow have not been rerun for the preview feature; the focused suites above are the completed evidence.
No live paid-provider turn, physical phone, Safari or cross-platform release rollout was tested. Large images beyond the
bounded ACP input/preview limits cannot be rendered. Expired/evicted snapshots require a fresh agent read or generation.

Rollback: hide `acpPreviews` or revert this feature. Existing text/history contracts and uploaded user images are unchanged.
Cache files can be removed after the connector stops; do not edit the live cache manually. Browser retries only read snapshots.
