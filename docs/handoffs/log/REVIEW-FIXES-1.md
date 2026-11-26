## REVIEW-FIXES-1: twenty-one findings, and the leads the review left, from a review of the machines and skills features are fixed (25 November 2026)

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
- 6. A machine adds to a private skill bundle, with its secret alone, only if one of its revisions was made by that enrolment
  (`ErrMachineBundleNotOwned`, 409). An import a person reviews and approves in the browser (`CommitRemoteImportFor`) may still
  add to any of their bundles: the check is a guard on `SaveMachineSkillImportFor` only. A first version put it in the shared
  helper and refused that approved flow too; the automated review caught it. `docs/machine-skill-import.md` said a personal token was needed; it now says what is true.
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

The leads, fixed afterwards. The review listed these without confirming them. Each was re-read before it was changed.

- Import limits. `POST /api/import` is refused with 413 when its items, text lines, environments and their skills come to more
  than `importMaxEntries` (16,384, twice the most a machine reports), before `importMu` is taken. A name over 255 bytes
  (`store.MaxLibrarySkillNameBytes`, the limit `ValidLibrarySkillName` had as a literal) is skipped with an entry in `errors`,
  whatever it was read as, so a pasted line over 255 bytes is skipped too.
- Shelf notes. A note is at most 20,000 characters (`shelfNoteMax`), refused with 400. The review said 4,000, the limit of the
  note box; `TestMCPWriteResponseLimits` and the MCP tools take 20,000, so 4,000 would have broken them.
- Repeated work. The Skills page checks the public page only of the rows it returns, not the other tab's. A copy run plans once
  (`SkillCopyPlanFor`, `MarkSkillsUnsourcedFor`) instead of twice. The page still reads on the single write connection.
- Skill Buddy. Asking again for a suggestion already held writes nothing, and the 90 day purge runs once per call, not once per
  suggestion. The web client still asks after each pause in typing, and each ask still reads the catalogue and library.
- Retry list. `pollSkillsAgent` takes the retries after the job is claimed, so a failed claim leaves them for the next poll. If
  taking them fails after the claim, the job is still sent and they wait.
- Polling. The Skills page numbers its reads per tab and its poll skips a tab that has a read on its way; the Sync page skips a tick
  while its read runs. Only the poll skips: a read after something the person did always runs.
- Touch targets and copy in the machine dialogs. The switches (a larger hit area, same look), icon buttons, tabs, step buttons and
  links reach 44 px on a coarse pointer. Em dashes in those screens are gone and they say "enrolment". The command text is unchanged.
- The set-server proof is bound to the controller's address (decided with the owner: bind it and require it). A machine sends the
  address it dialled; the controller signs over its own configured public URL (`REET_PUBLIC_URL` or the admin setting) and answers
  409 `wrong_address` for any other and `no_public_url` when it has none. The machine accepts only that bound proof, so
  `set-server` and following a move need a controller of this release with its public URL set. A request that names no address, from
  an older machine, still gets the unbound proof; machines older than this stay open to the relay until updated.
- The proof route and a code enrolment with an unknown code are answered from the read pool, so guesses cannot queue on the write
  connection. `refuseMachine` answers 500, not 403, when the machine check itself fails, in the private control, private hello,
  terminal and software mirror routes too. A branch origin does not have is `no_remote` (by `git ls-remote --exit-code`), not
  `fetch_failed`.

Decisions made along the way.

- Finding 6 was a choice, not a plain bug: the documented `--bundle` command worked with the machine's secret alone. The
  owner chose to keep it for bundles the machine started and refuse the rest.
- A link anywhere in a repository whose skill is the repository root now fails that skill, because every file is part of it.
- Finding 15 went past the one table named, to the two others the scan found.

Checked.

- `go vet ./...` clean. `go test ./...` passes in every package, run twice: once for the twenty-one findings, once again after
  the leads were added. In the first run `internal/gaffer`'s `TestAStopIsNotAskedAgainWhileClaimedOrOnceAnswered` failed once
  while the whole suite ran in parallel; it passed 15 of 15 alone, the whole package passed alone, and it did not fail in the
  second run. Nothing in `internal/gaffer` refers to code changed here. It is not in `flaky-tests.md`; it needs an owner to look
  at it.
- The web unit tests (128 files) and `svelte-check` (537 files, no errors or warnings) pass.
- Browser suites against a built UI (twenty-seven in all, across the web changes and again on the merged tree), all passing, among them: `machine-import`, `add-machine`, `machines-fleet`, `library-machine-flow`,
  `machines-search`, `first-machine`, `skill-buddy`, `library`, `chat` and `session-workspace`. `library-machine-flow` failed
  first, which is how the part of finding 16 above was found to be wrong.
- Under `-race`, `internal/store`, `internal/scan` and `internal/skillfetch` pass with no race reported. `internal/api`
  under `-race` ran into Go's default ten minute timeout with no race reported; it needs the two shards and 40 minute
  timeout of `scripts/ci-go-shard.sh api-1` and `api-2`, which CI runs. Those shards had not been run to the end locally
  when this was written.

Not checked.

- No test drives the Library double change, the Skill Buddy over-limit message or the Machines page polling directly. The
  suites above cover the behaviour around them, and these three were checked by reading only.
- Not changed, on purpose: `installCopy` (an open API route with no caller in the web client, MCP or docs), the write-only
  `keep_synced` field and the exported store functions only tests call. Removing them changes API fields and test seams; say if
  they should go. `SkillsPageFor` still reads on the write connection, and Skill Buddy still reads on every ask.
- `machineUser`, which returns a bool, still treats any error as "not a machine" and falls through to the normal sign-in checks.
- No test asserts the 44 px sizes; they were measured with a throwaway script in Chromium with touch on, at 390 and 1280 px.
- Still under 44 px on a coarse pointer, outside what the review named: `MachineSetup`'s `.advanced > summary`, `.modes` and
  `.setup > summary` rows, the 16 px checkboxes in the skill import file table, and `MachineSettings`' `.confirm-actions .btn`
  and typed-confirm input (read from the CSS, not measured).
- Left alone: "enrollment" in the copy of `UpdateReview.svelte`, `sessions/SessionsPage.svelte` and `terminal/TerminalWorkspace.svelte`,
  and em dashes in about two dozen other `.svelte` and `.ts` files (the count includes comments; copy and comments were not told
  apart).
