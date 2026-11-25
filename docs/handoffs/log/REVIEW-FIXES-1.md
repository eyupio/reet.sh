## REVIEW-FIXES-1: twenty-one findings from a review of the machines and skills features are fixed (25 November 2026)

A review of the machines and skills code (store, API, fetch and install, web client) found twenty-one things to fix. Each
was re-read against the source before it was accepted, and each fix has a test that failed before it and passes after.
Findings are numbered as in the review.

What changed.

Skills reaching machines.

- 1. A skill added from someone else's published page never got files, so no machine received it. `SkillSourceFor` now
  returns `hosted` sources for a skill that is not the account's own (`store_skill_content.go`), the new
  `PublishedHostedFiles` (`store_hosted_source.go`) reads the page's files, and `copyHostedSkill` (`skill_copies.go`)
  saves them. It is a copy as the page is when added; "Update from original" still answers 404 for it. An unpublished
  skill ends as "no longer published".
- 2. Deleting or renaming a written skill now puts the old name on `skill_removals` (`store_hosted.go`), as a delete from
  the Skills page does, so machines remove the folder.
- 4. A `__pycache__` folder in an installed skill no longer makes the copy read as edited (`osFile` in
  `reconcile_copies.go`). `node_modules` is not skipped: `Scan` applies its file limit first. Left as a `shortcut:` note.
- 10. Names are folded in Go (`strings.ToLower`) where reports are read back (`store_skill_reports.go`,
  `store_sync_machines.go`, `skillNameTakenTx`), because SQL `lower()` only folds ASCII. Comparisons made entirely in SQL
  were left: they agree with each other.
- 12, 13, 14. A link inside a fetched skill folder now fails that skill with a clear message instead of saving a partial
  copy. A transport error is shown as "could not reach the source" with no address in it. One YAML-based
  `skillfetch.SkillName` replaces two parsers, and `pickFolder` parses a SKILL.md only when the cheap matches fail.
- 7. "Update from original" takes a global download slot as well as its per-account one, and answers busy when none is free.

Machines and sync.

- 3. A storage error is no longer reported as "enrolment revoked". `AuthenticateSyncAgentBuild` and
  `AuthenticateRemoteAgent` return `ErrUnknownMachine` only for no row, and a failed last-seen stamp no longer refuses a
  machine. `agentIdentity` answers 403 only for a refusal and 500, with a fixed message, otherwise, so an agent keeps its
  saved update result.
- 5. A scan upload is read before `importMu` is taken.
- 6. A machine adds to a private skill bundle only if one of its revisions was made by that enrolment
  (`ErrMachineBundleNotOwned`, 409). `docs/machine-skill-import.md` said a personal token was needed; it now says what is true.
- 8. `EnrolWithCode` is one transaction, so a failure after the claim rolls it back.
- 9. Merging two machines that each have an agent answers 409. The merge no longer waits for a running job or cancels queued
  ones: update jobs hold the agent id, not the host.
- 11. `QueueAgentUpdate` keeps the newest twenty finished updates of that machine.
- 15. `DeleteUser` also clears `skill_activity`, and two more tables with a user id and no foreign key
  (`terminal_chat_events`, `worktree_cleanups`), which a scan of the schema found.

Web client.

- 16. The Machines page clears an old error on a good read, and the setup panel polls with the background read. The
  review also asked to keep the last skill health when only the skills read fails. That was tried and reverted:
  `library-machine-flow.e2e.mjs` states the rule that a failed current read must show "Skill sync not confirmed" and
  never fall back to older health as proof of sync, and the old health would have been shown as current.
- 17. Skill Buddy checks the draft has room (`skillPromptFits`) before it saves a choice. A caller with no `fits` still
  gets the old check afterwards.
- 18, 19, 20. The setup panel reads `updateJobs` first, forgetting a machine goes through `updateState`, and a second
  library row cannot be changed while one saves.
- 21. Dead code removed: `captureMachineLibraryContent`, `installPrivateSkill` (its two tests now check
  `privateSkillFiles` and `stageSkillFiles`), the never-set `direct` import mode, six unused helpers in `machines.ts`
  and the `sessionId` prop of `SkillPicker`.

Decisions made along the way.

- Finding 6 was a choice, not a plain bug: the documented `--bundle` command worked with the machine's secret alone. The
  owner chose to keep it for bundles the machine started and refuse the rest.
- A link anywhere in a repository whose skill is the repository root now fails that skill, because every file is part of it.
- Finding 15 went past the one table named, to the two others the scan found.

Checked.

- `go vet ./...` clean. `go test ./...`: every package passed except `internal/gaffer`, where
  `TestAStopIsNotAskedAgainWhileClaimedOrOnceAnswered` failed once while the whole suite ran in parallel. It passes 15 of 15
  alone and the whole package passes alone (87 s). Nothing in `internal/gaffer` refers to code changed here, and the
  directory is unchanged. It is not in `flaky-tests.md`; it needs an owner to look at it.
- The web unit tests (127 files) and `svelte-check` (537 files, no errors or warnings) pass.
- Browser suites against a built UI, all passing: `machine-import`, `add-machine`, `machines-fleet`, `library-machine-flow`,
  `machines-search`, `first-machine`, `skill-buddy`, `library`, `chat` and `session-workspace`. `library-machine-flow` failed
  first, which is how the part of finding 16 above was found to be wrong.
- Under `-race`, `internal/store`, `internal/scan` and `internal/skillfetch` pass with no race reported. `internal/api`
  under `-race` ran into Go's default ten minute timeout with no race reported; it needs the two shards and 40 minute
  timeout of `scripts/ci-go-shard.sh api-1` and `api-2`, which CI runs. Those shards had not been run to the end locally
  when this was written.

Not checked.

- No test drives the Library double change, the Skill Buddy over-limit message or the Machines page polling directly. The
  suites above cover the behaviour around them, and these three were checked by reading only.
- The leads the review listed but did not confirm are untouched: no limit on `POST /api/import` items or names, the Skills
  page and copy planner repeating work, Skill Buddy posting on every typing pause, pages that start a poll while one runs,
  touch targets and copy in the machine dialogs, the controller proof that can be relayed by a rogue `set-server` target,
  the unauthenticated `agent-proof` and code routes using the single write connection, and a branch never pushed reporting
  `fetch_failed`.
- The other callers of `AuthenticateRemoteAgent` (`private_control.go`, `software_mirror.go`, `terminal.go`, `private.go`,
  `machineUser`) still answer a refusal for a storage error. The agent only drops its result on `/api/v2/sync/result`.
