import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
// Compile the pure review module and the API client without a browser or an extra test dependency.
const compile = (file) => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const load = (file) => import(`data:text/javascript;base64,${Buffer.from(compile(file)).toString('base64')}`)
const review = await load('./contentReview.ts')
const api = await load('./api.ts')
const {
  OWNERSHIP_SENTENCE, APPLY_NOTICE, FALLBACK,
  presenceLabel, ownershipNote, checkStateLabel, reasonText, failureText, planCodeText, preservedText,
  scopeEligibility, classifyCheck, adoptable, summarisePlan, candidatesFor, preselect, groupBreakdown, claimSummary, pollDelay, describeError, CLOSED_CODES,
  asSeenFromPin, planVerdict, CONNECTOR_PROBLEM,
  connectorFor, groupBundles, preselectBundle, newestFirst, modeLabel, MODES, pinChangeNote, shortId, samePin, bundleLabel, revisionLabels, revisionLabel, missingNote,
} = review

// The closed tables are read from the Go side, so a code the server starts to send fails loudly here instead of
// reading as "Something changed". Paths are relative to this file; every extractor is asserted non-empty in the
// "Go sources" test below, so a regex that silently stops matching cannot shrink a list unnoticed.
const go = (path) => readFileSync(new URL(`../../../internal/${path}`, import.meta.url), 'utf8')
const planGo = go('skillplan/plan.go')
const previewGo = go('store/store_content_previews.go')
const literals = (text, re) => [...text.matchAll(re)].map(m => m[1]) // the first capture group of every match
const unique = (xs) => [...new Set(xs)].sort()
// Planner block and conflict codes: block("x"), conflict("x") and the one Conflict{... Code: "x"} literal.
// "root_"+ownership is built at run time, so root_shared and root_unknown are named here, once the source is seen to build them.
const buildsRootCodes = /block\("root_"\s*\+\s*s\.RootOwnership\)/.test(planGo)
const PLAN_CODES = unique([
  ...literals(planGo, /\b(?:block|conflict)\("([a-z_]+)"\)/g),
  ...literals(planGo, /Conflict\{[^}]*Code:\s*"([a-z_]+)"/g),
  ...(buildsRootCodes ? ['root_shared', 'root_unknown'] : []),
])
// Preserved{... Reason: "x"}
const PRESERVED_REASONS = unique(literals(planGo, /Preserved\{[^}]*Reason:\s*"([a-z_]+)"/g))
// Preview states: `State: "x"` in a literal, and the first string after the `=` of any statement that sets or compares a State
// (this reaches `p.State, p.Error = "failed", ...` and `p.InventoryRevision, p.State = revision, "capturing"` too).
const STATES = unique([...literals(previewGo, /\bState:\s*"([a-z_0-9]+)"/g), ...literals(previewGo, /\bState\b[^=\n"]*=[^\n]*?"([a-z_0-9]+)"/g)])
// Failure codes: the connector may report the cases of the switch in FailSkillContentPreviewFor; the controller itself records plan_unavailable.
const FAILURE_CODES = unique([
  ...literals(previewGo, /switch code \{([\s\S]*?)default:/g).flatMap(list => literals(list, /"([a-z_]+)"/g)),
  ...literals(previewGo, /p\.State, p\.Error = "failed", "([a-z_]+)"/g),
])
// Fence reasons: every reasons = append(reasons, "x") inside previewFence.
const fenceStart = previewGo.indexOf('func previewFence(')
const fenceBody = previewGo.slice(fenceStart, previewGo.indexOf('\nfunc ', fenceStart + 1))
const GO_FENCE_REASONS = unique(literals(fenceBody, /reasons = append\(reasons, "([a-z_]+)"\)/g))
// The reasons table also words two reasons that come from a state rather than the fence: a check can be "expired" or "stale".
const STATE_REASONS = ['expired', 'stale']
const FENCE_REASONS = unique([...GO_FENCE_REASONS, ...STATE_REASONS])
const TABLES = [
  ['checkStateLabel', checkStateLabel, STATES],
  ['reasonText', reasonText, FENCE_REASONS],
  ['failureText', failureText, FAILURE_CODES],
  ['planCodeText', planCodeText, PLAN_CODES],
  ['preservedText', preservedText, PRESERVED_REASONS],
]

const NOW = 1_700_000_000_000
const DAY = 86_400_000
const digest = (c) => c.repeat(64)
const file = (path, o = {}) => ({ path, bytes: 10, digest: digest('a'), executable: false, ...o })
const manifest = (files = [file('SKILL.md')], o = {}) => ({ schemaVersion: 1, algorithm: 'sha256', modeSemantics: 'posix-executable-v1', complete: true, files, totalBytes: files.reduce((n, f) => n + f.bytes, 0), digest: digest('m'), ...o })
const capture = (o = {}) => ({ schemaVersion: 1, adapter: 'posix-executable-v1', rootId: digest('r'), rootOwnership: 'exclusive', presence: 'present', complete: true, manifest: manifest(), issues: [], ...o })
const plan = (o = {}) => ({ version: 1, preconditions: {}, ready: false, actions: [], conflicts: [{ path: '.', code: 'adoption_required' }], preserved: [], hash: digest('h'), ...o })
const preview = (o = {}) => ({
  id: 'p'.repeat(48), requestKey: 'k'.repeat(20), targetId: 't'.repeat(48), targetVersion: 'v'.repeat(48), environmentId: '1/workstation/agents', agentId: 'a'.repeat(48), agent: 'agents', project: '', name: 'demo',
  bundleId: 'b'.repeat(32), revisionId: 'c'.repeat(32), manifestDigest: digest('m'), mode: 'additive', instanceId: 'i'.repeat(16), inventoryRevision: '4611686018427387904', state: 'captured', reasons: [],
  createdAt: NOW, updatedAt: NOW, expiresAt: NOW + 600_000, capturedAt: NOW, capture: capture(), captureHash: digest('c'), plan: plan(), reviewHash: digest('r'), authority: 'preview_only', ...o,
})
const env = (o = {}) => ({ id: '1/workstation/agents', name: 'agents', kind: 'agent', agent: 'agents', host: 'Workstation', lastSeen: NOW, scannedAt: NOW - 1000, complete: true, os: 'linux', arch: 'amd64', version: '1', skills: [], ...o })
const agent = (o = {}) => ({ id: 'a'.repeat(48), host: 'workstation', version: '1', lastSeen: NOW, paused: false, autoUpdate: false, ...o })
const opts = (o = {}) => ({ accounts: true, canWrite: true, now: NOW, ...o })
const reasonOf = (r) => { assert.equal(r.ok, false, 'expected a refusal'); return r.reason }

test('the three exact constants', () => {
  assert.equal(OWNERSHIP_SENTENCE, 'Recorded by reet from a recent check, not verified on the machine.')
  assert.equal(APPLY_NOTICE, "Applying changes isn't available yet.")
  assert.equal(FALLBACK, 'Something changed. Check again.')
})

test('the Go sources: every extractor found something, and the closed tables hold exactly the codes the server can send', () => {
  assert.ok(buildsRootCodes, 'plan.go no longer builds root_shared and root_unknown from RootOwnership; update this test')
  const found = { states: STATES, reasons: GO_FENCE_REASONS, failures: FAILURE_CODES, planCodes: PLAN_CODES, preserved: PRESERVED_REASONS }
  for (const [name, codes] of Object.entries(found)) assert.ok(codes.length > 0, `nothing was extracted for ${name}: a regex has stopped matching`)
  // The two reasons the table words beyond the fence are real states.
  for (const r of STATE_REASONS) assert.ok(STATES.includes(r), `${r} is not a preview state`)
  assert.deepEqual([...CLOSED_CODES.states].sort(), STATES)
  assert.deepEqual([...CLOSED_CODES.reasons].sort(), FENCE_REASONS)
  assert.deepEqual([...CLOSED_CODES.failures].sort(), FAILURE_CODES)
  assert.deepEqual([...CLOSED_CODES.planCodes].sort(), PLAN_CODES)
  assert.deepEqual([...CLOSED_CODES.preserved].sort(), PRESERVED_REASONS)
})

// One distinguishing word per sentence, written out by hand from the sentences themselves, so that swapping two rows of a
// table (or rewording one into another's meaning) fails. Each pattern must match its own sentence and no other in its table.
const KEYWORDS = {
  checkStateLabel: {
    queued: /waiting/i, capturing: /reading/i, captured: /^Checked$/, failed: /^Couldn't check$/,
    cancelled: /stopped/i, expired: /ran out of time/i, superseded: /out of date/i, stale: /too old/i,
  },
  reasonText: {
    pin_changed: /pin changed/i, pin_removed: /pin was removed/i, connector_paused: /paused/i, connector_removed: /connector.*was removed/i,
    process_changed: /restarted or stopped reporting/i, inventory_changed: /newer scan/i, scope_removed: /no longer reports/i,
    receipt_changed: /ownership record.*changed/i, expired: /ran out of time/i, stale: /five minutes/i,
  },
  failureText: { capture_failed: /couldn't read/i, scope_unavailable: /can't use this folder/i, plan_unavailable: /couldn't be worked out/i },
  planCodeText: {
    adapter_unsupported: /layout isn't supported/i, root_shared: /or a link, reaches this same folder/i, root_unknown: /couldn't tell whether/i,
    generation_changed: /connector restarted/i, inventory_changed: /newer scan/i, observation_unknown: /couldn't be read completely/i,
    observation_stale: /too long ago/i, observation_invalid: /files found/i, adoption_required: /not recorded as owned/i,
    unmanaged_path: /doesn't own it/i, managed_target_missing: /folder isn't there now/i, local_modified: /has changed since ownership was recorded/i,
    both_modified: /on the machine and in the pinned revision/i, local_deleted: /missing from the machine/i, result_invalid: /planned result/i,
    baseline_target_mismatch: /different folder or layout/i, baseline_bundle_mismatch: /different library skill/i,
  },
  preservedText: { local_only: /stays yours/i, additive_managed: /no longer has this file/i },
}

for (const [name, fn, codes] of TABLES) {
  test(`${name}: each sentence says what its code means, and no other sentence in the table does`, () => {
    const keywords = KEYWORDS[name]
    assert.deepEqual(Object.keys(keywords).sort(), codes, 'the keyword table must name every code the server can send')
    for (const code of codes) for (const other of codes) {
      assert.equal(keywords[code].test(fn(other)), code === other, `${name}: /${keywords[code].source}/ against the sentence for ${other}`)
    }
  })
}

for (const [name, fn, codes] of TABLES) {
  test(`${name}: every known code maps to its own plain, non-fallback sentence`, () => {
    const seen = new Set()
    for (const code of codes) {
      const text = fn(code)
      assert.equal(typeof text, 'string', `${name}(${code})`)
      assert.ok(text.trim().length > 0, `${name}(${code}) is empty`)
      assert.notEqual(text, FALLBACK, `${name}(${code}) fell back`)
      assert.ok(!seen.has(text), `${name}(${code}) repeats another sentence`)
      seen.add(text)
    }
  })
  test(`${name}: an unknown string returns the fallback, even one that is a built-in object key`, () => {
    for (const code of ['', 'nonsense', 'QUEUED', ' queued', 'constructor', 'toString', '__proto__', 'hasOwnProperty']) assert.equal(fn(code), FALLBACK, `${name}(${JSON.stringify(code)})`)
  })
}

test('presence reads Found, Not there yet or Couldn\'t tell, and never anything reassuring when unsure', () => {
  assert.equal(presenceLabel(preview({ capture: capture({ presence: 'present' }) })), 'Found')
  assert.equal(presenceLabel(preview({ capture: capture({ presence: 'absent', manifest: undefined }) })), 'Not there yet')
  assert.equal(presenceLabel(preview({ capture: capture({ presence: 'unknown', manifest: undefined }) })), "Couldn't tell")
  assert.equal(presenceLabel(preview({ capture: capture({ presence: 'something-new' }) })), "Couldn't tell")
  assert.equal(presenceLabel(preview({ capture: undefined })), '')
  assert.equal(presenceLabel(null), '')
})

test('folder ownership: only exclusive says nothing; shared says so; anything missing or unrecognised reads as unknown', () => {
  assert.equal(ownershipNote('exclusive'), '')
  const shared = ownershipNote('shared'), unknown = ownershipNote('unknown')
  assert.match(shared, /another agent scope, or a link, reaches this same folder/i)
  assert.match(shared, /will not touch it/)
  assert.match(unknown, /couldn't tell/i)
  assert.notEqual(shared, unknown)
  // Silence means "this scope has the folder to itself", so it is never the answer to missing or odd input.
  for (const odd of [undefined, '', 'exclusivee', 'Exclusive', ' exclusive', 'constructor']) assert.equal(ownershipNote(odd), unknown, JSON.stringify(odd))
  assert.equal(shared, planCodeText('root_shared'))
  assert.equal(unknown, planCodeText('root_unknown'))
})

test('scopeEligibility passes a supported, recently scanned scope with an enrolled, running connector', () => {
  for (const a of ['agents', 'claude', 'cursor', 'opencode']) assert.deepEqual(scopeEligibility(env({ agent: a }), [agent()], opts()), { ok: true }, a)
  assert.deepEqual(scopeEligibility(env({ agent: 'claude', project: '/home/me/app' }), [agent()], opts()), { ok: true })
  assert.deepEqual(scopeEligibility(env({ agent: 'agents', project: '/srv/app' }), [agent()], opts()), { ok: true })
})

test('scopeEligibility gives each failure its own plain reason, in order', () => {
  const reasons = {
    accounts: reasonOf(scopeEligibility(env(), [agent()], opts({ accounts: false }))),
    writer: reasonOf(scopeEligibility(env(), [agent()], opts({ canWrite: false }))),
    codex: reasonOf(scopeEligibility(env({ agent: 'codex' }), [agent()], opts())),
    unsupported: reasonOf(scopeEligibility(env({ agent: 'reet' }), [agent()], opts())),
    projectAgent: reasonOf(scopeEligibility(env({ agent: 'cursor', project: '/home/me/app' }), [agent()], opts())),
    projectPath: reasonOf(scopeEligibility(env({ agent: 'claude', project: 'app' }), [agent()], opts())),
    scan: reasonOf(scopeEligibility(env({ complete: false }), [agent()], opts())),
    loading: reasonOf(scopeEligibility(env(), undefined, opts())),
    unenrolled: reasonOf(scopeEligibility(env(), [], opts())),
    paused: reasonOf(scopeEligibility(env(), [agent({ paused: true })], opts())),
  }
  const texts = Object.values(reasons)
  for (const t of texts) { assert.ok(t.trim().length > 0); assert.notEqual(t, FALLBACK) }
  assert.equal(new Set(texts).size, texts.length, 'two failures share one reason')
  assert.match(reasons.accounts, /account/i)
  assert.match(reasons.writer, /view/i)
  assert.match(reasons.codex, /scan-only/i)
  assert.match(reasons.scan, /14 days/)
  assert.match(reasons.unenrolled, /enrolled/i)
  assert.equal(reasons.loading, 'Machines are still loading.')
  assert.doesNotMatch(reasons.loading, /enrolled/i)
  assert.match(reasons.paused, /paused/i)
  // The first failing rule wins.
  assert.equal(reasonOf(scopeEligibility(env({ agent: 'codex', complete: false }), [], opts({ accounts: false, canWrite: false }))), reasons.accounts)
  assert.equal(reasonOf(scopeEligibility(env({ agent: 'codex', complete: false }), [], opts({ canWrite: false }))), reasons.writer)
  assert.equal(reasonOf(scopeEligibility(env({ agent: 'codex', complete: false }), [], opts())), reasons.codex)
  assert.equal(reasonOf(scopeEligibility(env({ agent: 'cursor', project: 'rel', complete: false }), [], opts())), reasons.projectAgent)
  assert.equal(reasonOf(scopeEligibility(env({ agent: 'claude', project: 'rel', complete: false }), [], opts())), reasons.projectPath)
  assert.equal(reasonOf(scopeEligibility(env({ complete: false }), [agent({ paused: true })], opts())), reasons.scan)
  assert.equal(reasonOf(scopeEligibility(env(), [agent({ host: 'elsewhere', paused: true })], opts())), reasons.unenrolled)
})

test('scopeEligibility: a project scope must belong to claude or agents and have an absolute path', () => {
  for (const a of ['cursor', 'opencode']) assert.equal(scopeEligibility(env({ agent: a, project: '/abs/path' }), [agent()], opts()).ok, false, a)
  for (const p of ['app', './app', '~/app', 'C:\\app', '\\\\server\\share', ' /app']) assert.equal(scopeEligibility(env({ agent: 'claude', project: p }), [agent()], opts()).ok, false, p)
  assert.equal(scopeEligibility(env({ agent: 'claude', project: '/' }), [agent()], opts()).ok, true)
})

test('scopeEligibility: scan-only Codex and unknown agents are refused whatever else is true', () => {
  for (const a of ['codex', 'reet', 'reet-server', '', 'Claude', 'constructor']) assert.equal(scopeEligibility(env({ agent: a }), [agent()], opts()).ok, false, JSON.stringify(a))
})

test('scopeEligibility: a complete scan must be within 14 days, counted inclusively', () => {
  assert.equal(scopeEligibility(env({ scannedAt: NOW - 14 * DAY }), [agent()], opts()).ok, true)
  assert.equal(scopeEligibility(env({ scannedAt: NOW - 14 * DAY - 1 }), [agent()], opts()).ok, false)
  assert.equal(scopeEligibility(env({ scannedAt: NOW - 14 * DAY + 1 }), [agent()], opts()).ok, true)
  assert.equal(scopeEligibility(env({ scannedAt: NOW + 5000 }), [agent()], opts()).ok, true)
  assert.equal(scopeEligibility(env({ scannedAt: 0 }), [agent()], opts()).ok, false)
  assert.equal(scopeEligibility(env({ scannedAt: NOW, complete: false }), [agent()], opts()).ok, false)
  assert.equal(scopeEligibility(env({ scannedAt: undefined }), [agent()], opts()).ok, false)
  // A question asked with no usable clock is refused, not waved through.
  assert.equal(scopeEligibility(env(), [agent()], opts({ now: Number.NaN })).ok, false)
})

test('scopeEligibility: the connector is matched by host without regard to case', () => {
  assert.equal(scopeEligibility(env({ host: 'Workstation' }), [agent({ host: 'workstation' })], opts()).ok, true)
  assert.equal(scopeEligibility(env({ host: 'workstation' }), [agent({ host: 'WORKSTATION' })], opts()).ok, true)
  assert.equal(scopeEligibility(env({ host: 'Work-Station.local' }), [agent({ host: 'work-station.LOCAL' })], opts()).ok, true)
  assert.equal(scopeEligibility(env({ host: 'workstation' }), [agent({ host: 'workstation2' })], opts()).ok, false)
  assert.equal(scopeEligibility(env({ host: 'workstation' }), undefined, opts()).ok, false)
})

test('scopeEligibility: agents that have not loaded yet are not the same as no enrolled connector', () => {
  for (const notLoaded of [undefined, null]) assert.equal(reasonOf(scopeEligibility(env(), notLoaded, opts())), 'Machines are still loading.')
  assert.notEqual(reasonOf(scopeEligibility(env(), [], opts())), 'Machines are still loading.')
  // Earlier rules still win while the machines load.
  assert.notEqual(reasonOf(scopeEligibility(env({ complete: false }), undefined, opts())), 'Machines are still loading.')
})

test('scopeEligibility: host names are compared the way the database does, folding ASCII letters only', () => {
  assert.equal(scopeEligibility(env({ host: 'HOST' }), [agent({ host: 'host' })], opts()).ok, true)
  assert.equal(scopeEligibility(env({ host: 'host' }), [agent({ host: 'HOST' })], opts()).ok, true)
  assert.equal(scopeEligibility(env({ host: 'ZOë' }), [agent({ host: 'zoë' })], opts()).ok, true)
  // A non-ASCII letter in a different case is a different host to the database, so it is to us.
  assert.equal(scopeEligibility(env({ host: 'Zoë' }), [agent({ host: 'ZoË' })], opts()).ok, false)
  assert.equal(scopeEligibility(env({ host: '\u212Aelvin' }), [agent({ host: 'kelvin' })], opts()).ok, false) // U+212A KELVIN SIGN lower-cases to k in Unicode, not in ASCII
})

test('scopeEligibility: a paused connector blocks the scope, but a running one on the same host does not', () => {
  assert.equal(scopeEligibility(env(), [agent({ paused: true })], opts()).ok, false)
  assert.equal(scopeEligibility(env(), [agent({ host: 'WORKSTATION', paused: true })], opts()).ok, false)
  assert.equal(scopeEligibility(env(), [agent({ paused: true }), agent({ id: 'b'.repeat(48), paused: false })], opts()).ok, true)
  assert.equal(scopeEligibility(env(), [agent({ host: 'other', paused: false }), agent({ paused: true })], opts()).ok, false)
})

test('adoptable needs a present folder, an exclusive root, a complete manifest and exactly one adoption_required', () => {
  assert.equal(adoptable(preview()), true)
  assert.equal(adoptable(preview({ capture: capture({ presence: 'absent' }) })), false)
  assert.equal(adoptable(preview({ capture: capture({ presence: 'unknown' }) })), false)
  assert.equal(adoptable(preview({ capture: capture({ rootOwnership: 'shared' }) })), false)
  assert.equal(adoptable(preview({ capture: capture({ rootOwnership: 'unknown' }) })), false)
  assert.equal(adoptable(preview({ capture: capture({ manifest: undefined }) })), false)
  assert.equal(adoptable(preview({ capture: capture({ manifest: manifest([file('SKILL.md')], { complete: false }) }) })), false)
  assert.equal(adoptable(preview({ capture: undefined })), false)
  // Adoption is bound to the review hash, so a check without one cannot be adopted.
  assert.equal(adoptable(preview({ reviewHash: '' })), false)
  assert.equal(adoptable(preview({ reviewHash: undefined })), false)
  assert.equal(adoptable(preview({ plan: undefined })), false)
  assert.equal(adoptable(preview({ plan: plan({ conflicts: [] , ready: true }) })), false)
  assert.equal(adoptable(preview({ plan: plan({ conflicts: [{ path: '.', code: 'root_shared' }] }) })), false)
  assert.equal(adoptable(preview({ plan: plan({ conflicts: [{ path: '.', code: 'adoption_required' }, { path: 'a', code: 'unmanaged_path' }] }) })), false)
  assert.equal(adoptable(preview({ plan: plan({ conflicts: [{ path: '.', code: 'adoption_required' }, { path: '.', code: 'adoption_required' }] }) })), false)
})

test('classifyCheck: no check yet', () => {
  const c = classifyCheck(null, NOW, NOW)
  assert.equal(c.kind, 'none')
  assert.equal(c.canAdopt, false)
  assert.equal(c.canCheckAgain, true)
  assert.deepEqual(c.reasons, [])
  assert.ok(c.headline.length > 0 && c.headline !== FALLBACK)
})

test('classifyCheck: queued and capturing are working and cannot be adopted or restarted', () => {
  for (const state of ['queued', 'capturing']) {
    const c = classifyCheck(preview({ state, capture: undefined, plan: undefined, capturedAt: undefined, reviewHash: undefined }), NOW, NOW)
    assert.equal(c.kind, 'working', state)
    assert.equal(c.canAdopt, false, state)
    assert.equal(c.canCheckAgain, false, state)
    assert.ok(c.headline.length > 0 && c.headline !== FALLBACK, state)
  }
  assert.notEqual(classifyCheck(preview({ state: 'queued' }), NOW, NOW).headline, classifyCheck(preview({ state: 'capturing' }), NOW, NOW).headline)
})

test('classifyCheck: a captured, adoptable check that was just read is current and adoptable', () => {
  const c = classifyCheck(preview(), NOW, NOW)
  assert.equal(c.kind, 'current')
  assert.equal(c.canAdopt, true)
  assert.equal(c.canCheckAgain, true)
  assert.deepEqual(c.reasons, [])
  assert.equal(classifyCheck(preview(), NOW, NOW + 1).canAdopt, true)
})

test('classifyCheck: the browser read, not the server clock, decides; 30 seconds is the limit', () => {
  const p = preview({ capturedAt: NOW - 4 * 60_000, createdAt: NOW - 5 * 60_000, updatedAt: NOW - 4 * 60_000 })
  // Server timestamps in the payload are never compared with the browser's now.
  assert.equal(classifyCheck(p, NOW, NOW).canAdopt, true)
  assert.equal(classifyCheck(preview({ capturedAt: NOW + 3_600_000 }), NOW, NOW).canAdopt, true)
  assert.equal(classifyCheck(p, NOW, NOW + 30_000).kind, 'current')
  assert.equal(classifyCheck(p, NOW, NOW + 30_000).canAdopt, true)
  const old = classifyCheck(p, NOW, NOW + 30_001)
  assert.equal(old.kind, 'refreshing')
  assert.equal(old.canAdopt, false)
  assert.equal(old.canCheckAgain, true)
  assert.deepEqual(old.reasons, [])
  assert.ok(old.headline.length > 0 && old.headline !== FALLBACK)
  assert.equal(classifyCheck(p, NOW, NOW + 3_600_000).kind, 'refreshing')
  // A read time that is not a number is never treated as fresh.
  assert.equal(classifyCheck(p, Number.NaN, NOW).kind, 'refreshing')
  assert.equal(classifyCheck(p, NOW, Number.NaN).canAdopt, false)
  // Reading it again makes it current again.
  assert.equal(classifyCheck(p, NOW + 31_000, NOW + 31_000).canAdopt, true)
})

test('classifyCheck: a read from the future, or at an unusable time, is never fresh and never adoptable', () => {
  const p = preview()
  // The read happened "after" the question was asked: the clocks cannot be trusted, so wait for a fresh read.
  for (const readAt of [NOW + 3_600_000, NOW + 1, Infinity, -Infinity, Number.NaN]) {
    const c = classifyCheck(p, readAt, NOW)
    assert.equal(c.kind, 'refreshing', String(readAt))
    assert.equal(c.canAdopt, false, String(readAt))
    assert.equal(c.canCheckAgain, true, String(readAt))
  }
  for (const now of [Infinity, -Infinity, Number.NaN]) {
    const c = classifyCheck(p, NOW, now)
    assert.equal(c.kind, 'refreshing', String(now))
    assert.equal(c.canAdopt, false, String(now))
  }
  assert.equal(classifyCheck(p, NOW, NOW).canAdopt, true)
  assert.equal(classifyCheck(p, NOW - 1, NOW).canAdopt, true)
})

test('classifyCheck: a captured check that cannot be adopted is still current but offers no adoption', () => {
  const cases = {
    absent: preview({ capture: capture({ presence: 'absent', manifest: undefined }), plan: plan({ ready: true, conflicts: [], actions: [{ path: 'SKILL.md', kind: 'create', after: file('SKILL.md') }] }) }),
    unknown: preview({ capture: capture({ presence: 'unknown', manifest: undefined }), plan: plan({ conflicts: [{ path: '.', code: 'observation_unknown' }] }) }),
    shared: preview({ capture: capture({ rootOwnership: 'shared' }), plan: plan({ conflicts: [{ path: '.', code: 'root_shared' }] }) }),
    alreadyOwned: preview({ plan: plan({ ready: true, conflicts: [] }) }),
    noPlan: preview({ plan: undefined }),
  }
  for (const [name, p] of Object.entries(cases)) {
    const c = classifyCheck(p, NOW, NOW)
    assert.equal(c.kind, 'current', name)
    assert.equal(c.canAdopt, false, name)
    assert.equal(c.canCheckAgain, true, name)
  }
  // Even a long-old read of such a check is simply refreshing.
  assert.equal(classifyCheck(cases.shared, NOW, NOW + 31_000).kind, 'refreshing')
})

test('classifyCheck: the headline names what was found, and never calls an unsure read a find', () => {
  assert.match(classifyCheck(preview(), NOW, NOW).headline, /found/i)
  assert.match(classifyCheck(preview({ capture: capture({ presence: 'absent', manifest: undefined }), plan: undefined }), NOW, NOW).headline, /not there yet/i)
  assert.match(classifyCheck(preview({ capture: capture({ presence: 'unknown', manifest: undefined }), plan: undefined }), NOW, NOW).headline, /couldn't tell/i)
  assert.doesNotMatch(classifyCheck(preview({ capture: capture({ presence: 'unknown', manifest: undefined }), plan: undefined }), NOW, NOW).headline, /found/i)
})

test("classifyCheck: the server's stale state is stale, with its own reason when it gives none", () => {
  const c = classifyCheck(preview({ state: 'stale' }), NOW, NOW)
  assert.equal(c.kind, 'stale')
  assert.equal(c.canAdopt, false)
  assert.equal(c.canCheckAgain, true)
  assert.deepEqual(c.reasons, [reasonText('stale')])
  assert.deepEqual(classifyCheck(preview({ state: 'stale', reasons: ['stale'] }), NOW, NOW).reasons, [reasonText('stale')])
})

test('classifyCheck: superseded is stale with every reason it was given, unknown reasons falling back once', () => {
  const c = classifyCheck(preview({ state: 'superseded', reasons: ['pin_changed', 'receipt_changed'] }), NOW, NOW)
  assert.equal(c.kind, 'stale')
  assert.equal(c.canAdopt, false)
  assert.equal(c.canCheckAgain, true)
  assert.deepEqual(c.reasons, [reasonText('pin_changed'), reasonText('receipt_changed')])
  const odd = classifyCheck(preview({ state: 'superseded', reasons: ['pin_changed', 'brand_new', 'another_new'] }), NOW, NOW)
  assert.deepEqual(odd.reasons, [reasonText('pin_changed'), FALLBACK])
  assert.deepEqual(classifyCheck(preview({ state: 'superseded', reasons: [] }), NOW, NOW).reasons, [FALLBACK])
  for (const r of FENCE_REASONS) assert.deepEqual(classifyCheck(preview({ state: 'superseded', reasons: [r] }), NOW, NOW).reasons, [reasonText(r)], r)
})

test('classifyCheck: a captured check that carries reasons is not trusted', () => {
  const c = classifyCheck(preview({ state: 'captured', reasons: ['inventory_changed'] }), NOW, NOW)
  assert.equal(c.kind, 'stale')
  assert.equal(c.canAdopt, false)
  assert.deepEqual(c.reasons, [reasonText('inventory_changed')])
})

test('classifyCheck: expired and cancelled have ended, with Check again on offer', () => {
  const expired = classifyCheck(preview({ state: 'expired', capture: undefined, plan: undefined }), NOW, NOW)
  assert.equal(expired.kind, 'ended')
  assert.equal(expired.canAdopt, false)
  assert.equal(expired.canCheckAgain, true)
  assert.deepEqual(expired.reasons, [reasonText('expired')])
  const cancelled = classifyCheck(preview({ state: 'cancelled', capture: undefined, plan: undefined }), NOW, NOW)
  assert.equal(cancelled.kind, 'ended')
  assert.equal(cancelled.canAdopt, false)
  assert.equal(cancelled.canCheckAgain, true)
  assert.notEqual(expired.headline, cancelled.headline)
})

test("classifyCheck: a failed check is blocked and says why in the failure code's own words", () => {
  for (const code of FAILURE_CODES) {
    const c = classifyCheck(preview({ state: 'failed', error: code, capture: undefined, plan: undefined }), NOW, NOW)
    assert.equal(c.kind, 'blocked', code)
    assert.equal(c.canAdopt, false, code)
    assert.equal(c.canCheckAgain, true, code)
    assert.deepEqual(c.reasons, [failureText(code)], code)
  }
  // The headline is true whichever code follows: plan_unavailable means the machine was read, so it must not blame the machine.
  for (const code of FAILURE_CODES) {
    const headline = classifyCheck(preview({ state: 'failed', error: code }), NOW, NOW).headline
    assert.equal(headline, "That check didn't finish.", code)
    assert.doesNotMatch(headline, /machine|couldn't be checked/i, code)
  }
  assert.deepEqual(classifyCheck(preview({ state: 'failed', error: 'a_new_code' }), NOW, NOW).reasons, [FALLBACK])
  assert.deepEqual(classifyCheck(preview({ state: 'failed' }), NOW, NOW).reasons, [FALLBACK])
})

test('classifyCheck: an unknown state is blocked with the fallback and cannot be adopted', () => {
  for (const state of ['', 'applied', 'verified', 'CAPTURED', 'constructor']) {
    const c = classifyCheck(preview({ state }), NOW, NOW)
    assert.equal(c.kind, 'blocked', state)
    assert.equal(c.headline, FALLBACK, state)
    assert.equal(c.canAdopt, false, state)
    assert.equal(c.canCheckAgain, true, state)
  }
})

test('classifyCheck: only a current, adoptable, just-read captured check can ever be adopted', () => {
  for (const state of STATES) for (const age of [0, 30_000, 30_001]) for (const reasons of [[], ['pin_changed']]) {
    const c = classifyCheck(preview({ state, reasons }), NOW, NOW + age)
    const expected = state === 'captured' && age <= 30_000 && reasons.length === 0
    assert.equal(c.canAdopt, expected, `${state} ${age} ${reasons}`)
    if (c.canAdopt) assert.equal(c.kind, 'current')
  }
})

test('summarisePlan', () => {
  assert.deepEqual(summarisePlan(undefined), { kind: 'unknown', actions: [], conflicts: [], preserved: [] })
  const ready = summarisePlan(plan({ ready: true, conflicts: [] }))
  assert.equal(ready.kind, 'up_to_date')
  assert.deepEqual(ready.actions, [])
  const actions = [{ path: 'SKILL.md', kind: 'update', before: file('SKILL.md'), after: file('SKILL.md', { digest: digest('b') }) }, { path: 'new.md', kind: 'create', after: file('new.md') }, { path: 'old.md', kind: 'remove', before: file('old.md') }]
  const preserved = [{ path: 'mine.txt', reason: 'local_only' }]
  const changes = summarisePlan(plan({ ready: true, conflicts: [], actions, preserved }))
  assert.equal(changes.kind, 'changes')
  assert.deepEqual(changes.actions, actions)
  assert.deepEqual(changes.preserved, preserved)
  assert.deepEqual(changes.conflicts, [])
  const conflicts = [{ path: 'a.md', code: 'local_modified' }, { path: 'b.md', code: 'both_modified' }]
  const withConflicts = summarisePlan(plan({ ready: false, conflicts }))
  assert.equal(withConflicts.kind, 'conflicts')
  assert.deepEqual(withConflicts.conflicts, conflicts)
  assert.deepEqual(withConflicts.actions, [])
  assert.equal(summarisePlan(plan({ conflicts: [{ path: '.', code: 'root_shared' }] })).kind, 'conflicts')
  const adoption = summarisePlan(plan())
  assert.equal(adoption.kind, 'adoption')
  assert.deepEqual(adoption.conflicts, [{ path: '.', code: 'adoption_required' }])
  // adoption_required alongside any other conflict is a set of conflicts, not an offer.
  assert.equal(summarisePlan(plan({ conflicts: [{ path: '.', code: 'adoption_required' }, { path: 'x', code: 'unmanaged_path' }] })).kind, 'conflicts')
  // A plan that is neither ready nor explained is never reassuring.
  assert.equal(summarisePlan(plan({ ready: false, conflicts: [] })).kind, 'unknown')
  // A ready plan that still lists a conflict is never up to date.
  assert.equal(summarisePlan(plan({ ready: true, conflicts: [{ path: 'x', code: 'local_modified' }] })).kind, 'conflicts')
})

test('candidatesFor keeps the server order, and an absent offer has none', () => {
  const candidates = [{ revisionId: 'b'.repeat(32), matched: 2, observedOnly: 0, revisionOnly: 1 }, { revisionId: 'a'.repeat(32), matched: 5, observedOnly: 1, revisionOnly: 0 }]
  assert.deepEqual(candidatesFor({ candidates }), candidates)
  assert.deepEqual(candidatesFor({ candidates }).map(c => c.revisionId), ['b'.repeat(32), 'a'.repeat(32)])
  assert.deepEqual(candidatesFor(undefined), [])
  assert.deepEqual(candidatesFor({ candidates: [] }), [])
  assert.deepEqual(candidatesFor({}), [])
  assert.deepEqual(candidatesFor({ candidates: null }), [])
})

test('preselect chooses a revision only when the offer holds exactly one candidate', () => {
  const c = (id) => ({ revisionId: id.repeat(32), matched: 1, observedOnly: 0, revisionOnly: 0 })
  assert.equal(preselect(undefined), '')
  assert.equal(preselect({ candidates: [] }), '')
  assert.equal(preselect({ candidates: [c('a')] }), 'a'.repeat(32))
  assert.equal(preselect({ candidates: [c('a'), c('b')] }), '')
  assert.equal(preselect({ candidates: [c('b'), c('a')] }), '')
  assert.equal(preselect({ candidates: [c('a'), c('b'), c('c')] }), '')
})

const row = (path, status, o = {}) => ({ path, bytes: 10, digest: digest('a'), executable: false, status, ...o })

test('groupBreakdown merges a stays-yours and library-only pair into one differs row, keeping the order', () => {
  const rows = groupBreakdown({
    revisionId: 'c'.repeat(32), matched: 1, observedOnly: 2, revisionOnly: 2,
    files: [
      row('SKILL.md', 'claimed'),
      row('notes/mine.txt', 'stays_yours', { bytes: 12, digest: digest('n') }),
      row('scripts/run.sh', 'stays_yours', { bytes: 80, digest: digest('m'), executable: false }),
      row('scripts/run.sh', 'library_only', { bytes: 90, digest: digest('l'), executable: true }),
      row('z-new.md', 'library_only', { bytes: 5, digest: digest('z') }),
    ],
  })
  assert.deepEqual(rows.map(r => [r.path, r.state]), [['SKILL.md', 'claimed'], ['notes/mine.txt', 'stays_yours'], ['scripts/run.sh', 'differs'], ['z-new.md', 'library_only']])
  // The merged row describes the machine's copy: that is the file that differs from the revision and stays yours.
  assert.deepEqual(rows[2], { path: 'scripts/run.sh', state: 'differs', bytes: 80, executable: false, digest: digest('m') })
  assert.deepEqual(rows[0], { path: 'SKILL.md', state: 'claimed', bytes: 10, executable: false, digest: digest('a') })
  assert.deepEqual(rows[1], { path: 'notes/mine.txt', state: 'stays_yours', bytes: 12, executable: false, digest: digest('n') })
})

test('groupBreakdown merges a pair whichever row comes first, and does not mutate its input', () => {
  const files = [row('a.txt', 'library_only', { bytes: 90, digest: digest('l') }), row('a.txt', 'stays_yours', { bytes: 80, digest: digest('m') }), row('b.txt', 'claimed')]
  const before = JSON.parse(JSON.stringify(files))
  const rows = groupBreakdown({ revisionId: 'c'.repeat(32), files, matched: 1, observedOnly: 1, revisionOnly: 1 })
  assert.deepEqual(rows.map(r => [r.path, r.state, r.bytes, r.digest]), [['a.txt', 'differs', 80, digest('m')], ['b.txt', 'claimed', 10, digest('a')]])
  assert.deepEqual(files, before)
})

test('groupBreakdown never merges different paths, and compares paths exactly', () => {
  const nfc = 'caf\u00e9.md', nfd = 'cafe\u0301.md'
  const rows = groupBreakdown({ revisionId: 'c'.repeat(32), matched: 0, observedOnly: 1, revisionOnly: 1, files: [row(nfc, 'stays_yours'), row(nfd, 'library_only'), row('Readme.md', 'stays_yours'), row('readme.md', 'library_only')] })
  assert.deepEqual(rows.map(r => [r.path, r.state]), [[nfc, 'stays_yours'], [nfd, 'library_only'], ['Readme.md', 'stays_yours'], ['readme.md', 'library_only']])
})

test('groupBreakdown never reads an unrecognised status as claimed', () => {
  const rows = groupBreakdown({ revisionId: 'c'.repeat(32), matched: 0, observedOnly: 0, revisionOnly: 0, files: [row('a', 'adopted'), row('b', ''), row('c', 'constructor')] })
  assert.deepEqual(rows.map(r => r.state), ['stays_yours', 'stays_yours', 'stays_yours'])
  assert.deepEqual(groupBreakdown({ revisionId: 'c'.repeat(32), matched: 0, observedOnly: 0, revisionOnly: 0, files: [] }), [])
  assert.deepEqual(groupBreakdown({ revisionId: 'c'.repeat(32), matched: 0, observedOnly: 0, revisionOnly: 0 }), [])
})

test('groupBreakdown handles the 256-file limit with unicode and very long paths', () => {
  const long = `${'segment-'.repeat(40)}/${'ディレクトリ/'.repeat(20)}file.md`
  const files = []
  for (let i = 0; i < 256; i++) {
    const path = i === 0 ? 'SKILL.md' : i === 1 ? long : i === 2 ? 'docs/résumé/日本語 ✓.md' : `refs/file-${String(i).padStart(3, '0')}.md`
    files.push(row(path, i === 0 ? 'claimed' : 'stays_yours', { bytes: i, executable: i % 2 === 0 }))
  }
  // Turn 100 of them into stays-yours/library-only pairs.
  const pairs = []
  for (let i = 0; i < 100; i++) pairs.push(row(files[100 + i].path, 'library_only', { bytes: 9_999 }))
  const input = [...files, ...pairs]
  const rows = groupBreakdown({ revisionId: 'c'.repeat(32), files: input, matched: 1, observedOnly: 255, revisionOnly: 100 })
  assert.equal(input.length, 356)
  assert.equal(rows.length, 256)
  assert.deepEqual(rows.map(r => r.path), files.map(f => f.path))
  assert.equal(rows.filter(r => r.state === 'differs').length, 100)
  assert.equal(rows.filter(r => r.state === 'stays_yours').length, 155)
  assert.equal(rows.filter(r => r.state === 'claimed').length, 1)
  assert.equal(rows[1].path, long)
  assert.equal(rows[2].path, 'docs/résumé/日本語 ✓.md')
  for (let i = 0; i < 256; i++) assert.equal(rows[i].bytes, i, `row ${i} keeps the machine's size`)
})

test('claimSummary counts what is claimed, what stays yours and what is only in the library', () => {
  const b = {
    revisionId: 'c'.repeat(32), matched: 2, observedOnly: 2, revisionOnly: 2,
    files: [
      row('SKILL.md', 'claimed', { bytes: 100 }), row('a.md', 'claimed', { bytes: 50 }),
      row('mine.txt', 'stays_yours'), row('edited.sh', 'stays_yours'), row('edited.sh', 'library_only'), row('new.md', 'library_only'),
    ],
  }
  const s = claimSummary(b)
  assert.equal(s.claimed, 2)
  assert.equal(s.claimedBytes, 150)
  assert.equal(s.staysYours, 1)
  assert.equal(s.differs, 1)
  assert.equal(s.libraryOnly, 1)
  assert.equal(s.text, '2 files would be recorded as owned by reet. 1 file differs from this revision and stays yours. 1 file is only on this machine and stays yours. 1 file is only in the library.')
  assert.equal(claimSummary({ revisionId: 'c'.repeat(32), matched: 1, observedOnly: 0, revisionOnly: 0, files: [row('SKILL.md', 'claimed')] }).text, '1 file would be recorded as owned by reet.')
  assert.equal(claimSummary({ revisionId: 'c'.repeat(32), matched: 0, observedOnly: 3, revisionOnly: 2, files: [row('a', 'stays_yours'), row('b', 'stays_yours'), row('c', 'stays_yours'), row('d', 'library_only'), row('e', 'library_only')] }).text, 'No files would be recorded as owned by reet. 3 files are only on this machine and stay yours. 2 files are only in the library.')
  assert.equal(claimSummary({ revisionId: 'c'.repeat(32), matched: 0, observedOnly: 0, revisionOnly: 0, files: [row('a', 'stays_yours'), row('a', 'library_only'), row('b', 'stays_yours'), row('b', 'library_only')] }).text, 'No files would be recorded as owned by reet. 2 files differ from this revision and stay yours.')
  const empty = claimSummary({ revisionId: 'c'.repeat(32), matched: 0, observedOnly: 0, revisionOnly: 0, files: [] })
  assert.deepEqual([empty.claimed, empty.staysYours, empty.differs, empty.libraryOnly, empty.claimedBytes], [0, 0, 0, 0, 0])
  assert.equal(empty.text, 'No files would be recorded as owned by reet.')
})

test('pollDelay: three seconds while queued or reading, ten while captured, otherwise none, and none when hidden', () => {
  assert.equal(pollDelay('queued', false), 3000)
  assert.equal(pollDelay('capturing', false), 3000)
  assert.equal(pollDelay('captured', false), 10000)
  for (const state of ['failed', 'cancelled', 'expired', 'superseded', 'stale', '', 'unheard-of', 'constructor']) assert.equal(pollDelay(state, false), null, state)
  for (const state of STATES) assert.equal(pollDelay(state, true), null, `${state} while hidden`)
})

test('describeError gives the server text when there is some, and a plain sentence otherwise', () => {
  assert.equal(describeError(new Error('Private skill unavailable.')), 'Private skill unavailable.')
  assert.equal(describeError(new api.UnauthorizedError('missing or wrong token')), 'missing or wrong token')
  assert.equal(describeError(new api.ApiError('That pin changed.', 409)), 'That pin changed.')
  assert.equal(describeError({ error: 'That pin changed.' }), 'That pin changed.')
  assert.equal(describeError('Content preview history is full (64 requests). Dismiss finished previews, then try again.'), 'Content preview history is full (64 requests). Dismiss finished previews, then try again.')
  const generic = 'Something went wrong. Try again.'
  for (const e of [null, undefined, 42, {}, [], new Error(''), new Error('   '), { error: '' }, { error: 7 }]) assert.equal(describeError(e), generic, String(JSON.stringify(e)))
  // The client's own fallback when a refusal carried no text.
  for (const m of ['409 Conflict', '500 Internal Server Error', '404 ', '403']) assert.equal(describeError(new Error(m)), generic, m)
})

test('describeError passes on the message of an Error, ApiError or UnauthorizedError, and of no other kind of error', () => {
  const generic = 'Something went wrong. Try again.'
  // What req throws: the server's own text, or its status line (which is not shown).
  assert.equal(describeError(new Error('Private skill unavailable.')), 'Private skill unavailable.')
  assert.equal(describeError(new api.ApiError('Private skill unavailable.', 404)), 'Private skill unavailable.')
  assert.equal(describeError(new api.ApiError('404 Not Found', 404)), generic)
  assert.equal(describeError(new api.UnauthorizedError()), 'missing or wrong token')
  // Everything else is a fault in the browser, not something the server said. A page that answers 200 with HTML ends here.
  const others = [
    new SyntaxError("Unexpected token '<', \"<!doctype \"... is not valid JSON"),
    new ReferenceError('machine is not defined'), new RangeError('Invalid array length'), new EvalError('eval'), new URIError('URI malformed'),
    new DOMException('The string did not match the expected pattern.', 'SyntaxError'), new DOMException('The operation was aborted.', 'AbortError'),
    new DOMException('Network down', 'NetworkError'), Object.assign(new Error('Plain text from a library'), { name: 'LibraryError' }),
  ]
  for (const e of others) assert.equal(describeError(e), generic, `${e.name}: ${e.message}`)
})

test('describeError never shows stack text, raw browser errors or very long messages', () => {
  const withStack = new Error('Could not save the pin.')
  withStack.stack = 'Error: Could not save the pin.\n    at saveContentPin (http://localhost/assets/index.js:1:2)\n    at async onclick (http://localhost/assets/index.js:3:4)'
  assert.equal(describeError(withStack), 'Could not save the pin.')
  const multi = new Error('Could not save the pin.\n    at foo (file:///x.js:1:1)')
  assert.equal(describeError(multi), 'Could not save the pin.')
  assert.doesNotMatch(describeError(new Error('    at foo (file:///x.js:1:1)')), /at foo|file:/)
  assert.equal(describeError(new TypeError('Failed to fetch')), "Couldn't reach reet. Check your connection and try again.")
  assert.equal(describeError(new TypeError('NetworkError when attempting to fetch resource.')), "Couldn't reach reet. Check your connection and try again.")
  assert.equal(describeError(new TypeError('Load failed')), "Couldn't reach reet. Check your connection and try again.")
  // A TypeError from our own code is not a connection problem and must not be reported as one.
  assert.equal(describeError(new TypeError("Cannot read properties of undefined (reading 'files')")), 'Something went wrong. Try again.')
  assert.equal(describeError(Object.assign(new Error('The operation was aborted.'), { name: 'AbortError' })), 'Something went wrong. Try again.')
  assert.equal(describeError('TypeError: x is not a function'), 'Something went wrong. Try again.')
  assert.equal(describeError(new Error('x'.repeat(301))), 'Something went wrong. Try again.')
  assert.equal(describeError(new Error('x'.repeat(300))), 'x'.repeat(300))
})

test('no sentence says an ownership record was verified, names a dialect word or a raw code, or claims an install', () => {
  const sentences = [OWNERSHIP_SENTENCE, APPLY_NOTICE, FALLBACK, presenceLabel(preview()), ownershipNote('shared'), ownershipNote('unknown'), describeError(null), describeError(new TypeError('x'))]
  for (const [, fn, codes] of TABLES) for (const code of codes) sentences.push(fn(code))
  for (const p of [null, preview(), preview({ state: 'queued' }), preview({ state: 'capturing' }), preview({ state: 'stale' }), preview({ state: 'superseded', reasons: FENCE_REASONS }), preview({ state: 'expired' }), preview({ state: 'cancelled' }), preview({ state: 'failed', error: 'capture_failed' }), preview({ state: 'what' }), preview({ capture: capture({ presence: 'absent' }) }), preview({ capture: capture({ presence: 'unknown' }) })]) {
    const c = classifyCheck(p, NOW, NOW)
    sentences.push(c.headline, ...c.reasons)
  }
  sentences.push(classifyCheck(preview(), NOW, NOW + 60_000).headline)
  const e = env(), a = [agent()]
  sentences.push(
    reasonOf(scopeEligibility(e, a, opts({ accounts: false }))), reasonOf(scopeEligibility(e, a, opts({ canWrite: false }))), reasonOf(scopeEligibility(env({ agent: 'codex' }), a, opts())),
    reasonOf(scopeEligibility(env({ agent: 'reet' }), a, opts())), reasonOf(scopeEligibility(env({ agent: 'cursor', project: '/x' }), a, opts())), reasonOf(scopeEligibility(env({ project: 'x' }), a, opts())),
    reasonOf(scopeEligibility(env({ complete: false }), a, opts())), reasonOf(scopeEligibility(e, undefined, opts())), reasonOf(scopeEligibility(e, [], opts())), reasonOf(scopeEligibility(e, [agent({ paused: true })], opts())),
  )
  const claim = claimSummary({ revisionId: 'c'.repeat(32), matched: 1, observedOnly: 2, revisionOnly: 2, files: [row('SKILL.md', 'claimed'), row('a', 'stays_yours'), row('b', 'stays_yours'), row('b', 'library_only'), row('c', 'library_only')] })
  sentences.push(claim.text)
  assert.ok(sentences.length > 60)
  for (const s of sentences) {
    assert.equal(typeof s, 'string')
    assert.ok(s.length > 0, 'an empty sentence')
    assert.doesNotMatch(s, /installed|installs|\bapplied\b|up to date/i, `"${s}" implies a change or an install`)
    assert.doesNotMatch(s, /_/, `"${s}" shows a raw code`)
    assert.doesNotMatch(s, /\b(nowt|owt|summat|reight|ey up|lass|lad|thee|tha)\b/i, `"${s}" uses dialect`)
    assert.doesNotMatch(s, /\b(color|authoriz\w*|organiz\w*|recogniz\w*|behavior|center|gray)\b/i, `"${s}" is not British spelling`)
    assert.doesNotMatch(s, /\b(dropbox|syncthing|rsync|git|npm|chezmoi|stow)\b/i, `"${s}" names another product`)
    assert.ok(/[.!?]$/.test(s) || s.split(' ').length <= 5, `"${s}" is neither a sentence nor a short label`)
  }
  // The only mention of verification denies it.
  for (const s of sentences.filter(s => /verified/i.test(s))) assert.match(s, /\bnot verified\b/)
})

// ----------------------------------------------------------------------------------------------
// Pins: the connector, the library choice, the newest revision and the words for a refused write.
const bundle = (id, name, o = {}) => ({ id: id.repeat(32), name, head: 'h'.repeat(32), createdAt: NOW, ...o })
const revision = (id, createdAt, o = {}) => ({ id: id.repeat(32), manifestDigest: digest(id), createdAt, origin: { kind: 'user' }, ...o })
const pin = (o = {}) => ({ id: 'p'.repeat(48), environmentId: '1/workstation/agents', agentId: 'a'.repeat(48), name: 'demo', bundleId: 'b'.repeat(32), revisionId: 'c'.repeat(32), manifestDigest: digest('m'), mode: 'additive', version: 'v'.repeat(48), updatedAt: NOW, verification: 'not_verified', ...o })

test('connectorFor: a running connector on the same host, case-folded the way the database does', () => {
  const running = agent({ id: 'r'.repeat(48) }), paused = agent({ id: 'p'.repeat(48), paused: true })
  assert.equal(connectorFor(env(), [running]), running)
  assert.equal(connectorFor(env({ host: 'WORKSTATION' }), [agent({ host: 'workstation' })])?.host, 'workstation')
  assert.equal(connectorFor(env(), [paused]), undefined)
  assert.equal(connectorFor(env(), [agent({ host: 'elsewhere' })]), undefined)
  // Only ASCII letters fold: a Kelvin sign is not a k.
  assert.equal(connectorFor(env({ host: '\u212Aelvin' }), [agent({ host: 'kelvin' })]), undefined)
  for (const none of [undefined, null, []]) assert.equal(connectorFor(env(), none), undefined)
})

test('connectorFor skips a paused connector even when it is listed first or seen last, and prefers the most recently seen', () => {
  const paused = agent({ id: 'p'.repeat(48), paused: true, lastSeen: NOW + 5000 }), old = agent({ id: 'o'.repeat(48), lastSeen: NOW - 5000 }), live = agent({ id: 'l'.repeat(48), lastSeen: NOW })
  assert.equal(connectorFor(env(), [paused, old, live]), live)
  assert.equal(connectorFor(env(), [live, old]), live)
  assert.equal(connectorFor(env(), [old, live]), live)
  // A tie goes to the first listed.
  const twin = agent({ id: 't'.repeat(48), lastSeen: NOW })
  assert.equal(connectorFor(env(), [live, twin]), live)
  assert.equal(connectorFor(env(), [twin, live]), twin)
})

test('connectorFor and scopeEligibility agree: a scope can be offered exactly when a connector is found', () => {
  const cases = [[], [agent()], [agent({ paused: true })], [agent({ paused: true }), agent({ id: 'b'.repeat(48) })], [agent({ host: 'other' })], [agent({ host: 'WORKSTATION', paused: true })], [agent({ host: 'WORKSTATION' })]]
  for (const agents of cases) assert.equal(scopeEligibility(env(), agents, opts()).ok, connectorFor(env(), agents) !== undefined, JSON.stringify(agents))
})

test('groupBundles puts the reported skill first and keeps the library order within each group', () => {
  const list = [bundle('1', 'alpha'), bundle('2', 'demo'), bundle('3', 'demo'), bundle('4', 'zeta')]
  const g = groupBundles(list, 'demo')
  assert.deepEqual(g.matching.map(b => b.id[0]), ['2', '3'])
  assert.deepEqual(g.others.map(b => b.id[0]), ['1', '4'])
  for (const none of [undefined, '', 'nothing', 'DEMO', 'dem']) assert.deepEqual(groupBundles(list, none), { matching: [], others: list }, JSON.stringify(none))
  assert.deepEqual(groupBundles([], 'demo'), { matching: [], others: [] })
  assert.equal(list.length, 4)
})

test('preselectBundle chooses only when exactly one library skill has the reported name', () => {
  const list = [bundle('1', 'alpha'), bundle('2', 'demo'), bundle('3', 'zeta')]
  assert.equal(preselectBundle(list, 'demo'), '2'.repeat(32))
  assert.equal(preselectBundle([...list, bundle('4', 'demo')], 'demo'), '')
  for (const none of [undefined, '', 'other', 'Demo']) assert.equal(preselectBundle(list, none), '', JSON.stringify(none))
  assert.equal(preselectBundle([], 'demo'), '')
})

test('newestFirst puts the bundle head first and the oldest last, whatever order the library listed them in', () => {
  const r1 = revision('1', NOW - 3000), r2 = revision('2', NOW - 2000), r3 = revision('3', NOW - 1000)
  assert.deepEqual(newestFirst([r1, r2, r3], r3.id).map(r => r.id[0]), ['3', '2', '1'])
  assert.deepEqual(newestFirst([r3, r1, r2], r3.id).map(r => r.id[0]), ['3', '2', '1'])
  // The head leads even when its creation time disagrees, and with no known head the creation time decides.
  assert.deepEqual(newestFirst([r1, r2, r3], r1.id).map(r => r.id[0]), ['1', '3', '2'])
  assert.deepEqual(newestFirst([r1, r2, r3]).map(r => r.id[0]), ['3', '2', '1'])
  assert.deepEqual(newestFirst([r1, r2, r3], 'z'.repeat(32)).map(r => r.id[0]), ['3', '2', '1'])
  assert.deepEqual(newestFirst([], 'x'), [])
})

test('newestFirst orders revisions made in the same millisecond by id, and does not change its input', () => {
  const a = revision('a', NOW), b = revision('b', NOW), c = revision('c', NOW)
  assert.deepEqual(newestFirst([b, a, c]).map(r => r.id[0]), ['c', 'b', 'a'])
  assert.deepEqual(newestFirst([a, b, c]).map(r => r.id[0]), ['c', 'b', 'a'])
  const input = [a, b, c]
  newestFirst(input, a.id)
  assert.deepEqual(input.map(r => r.id[0]), ['a', 'b', 'c'])
  // A missing or odd creation time never makes the order unstable.
  assert.deepEqual(newestFirst([revision('1', Number.NaN), revision('2', Number.NaN)]).map(r => r.id[0]), ['2', '1'])
})

test('mode words: Add and update, and the managed mode in the spec\'s own words; anything else is not a raw code', () => {
  assert.equal(modeLabel('additive'), 'Add and update')
  assert.equal(modeLabel('managed'), 'Also remove files reet owns that a newer revision drops')
  assert.deepEqual(MODES.map(m => m.value), ['additive', 'managed'])
  assert.deepEqual(MODES.map(m => m.label), ['Add and update', 'Also remove files reet owns that a newer revision drops'])
  for (const odd of ['', 'mirror', 'bidirectional', 'Additive', 'constructor', 'toString', '__proto__']) {
    const text = modeLabel(odd)
    assert.equal(text, "A mode this page doesn't recognise", JSON.stringify(odd))
    assert.doesNotMatch(text, /_/)
  }
  assert.equal(shortId('abcdef0123456789'), 'abcdef01')
  assert.equal(shortId('abc'), 'abc')
})

test('pinChangeNote says what changed in plain words, and nothing when nothing did', () => {
  const was = pin({ revisionId: '2'.repeat(32), mode: 'managed', version: 'v1' })
  const changed = pin({ revisionId: '1'.repeat(32), mode: 'additive', version: 'v2' })
  const note = pinChangeNote(was, changed, 'demo')
  assert.match(note, /changed elsewhere/)
  assert.match(note, /11111111/)
  assert.match(note, /Add and update/)
  assert.match(note, /22222222/)
  assert.match(note, /Also remove files reet owns/)
  assert.match(note, /Nothing was saved/)
  assert.match(pinChangeNote(was, undefined, 'demo'), /removed elsewhere/)
  assert.match(pinChangeNote(was, undefined, 'demo'), /Nothing was saved/)
  const created = pinChangeNote(undefined, changed, 'demo')
  assert.match(created, /was pinned elsewhere/)
  assert.match(created, /11111111/)
  assert.match(created, /Nothing was saved/)
  assert.match(pinChangeNote(was, pin({ ...was, version: 'v3' }), 'demo'), /saved again elsewhere, with the same revision and mode/)
  assert.match(pinChangeNote(was, pin({ ...was, bundleId: 'z'.repeat(32), version: 'v3' }), 'demo'), /changed elsewhere/)
  // A removal says nothing was removed, never that anything was saved.
  const removing = pinChangeNote(was, changed, 'demo', 'remove')
  assert.match(removing, /changed elsewhere/)
  assert.match(removing, /Nothing was removed/)
  assert.match(removing, /remove it again/)
  assert.doesNotMatch(removing, /saved\b(?! again)/)
  assert.match(pinChangeNote(was, undefined, 'demo', 'remove'), /already removed elsewhere, so there was nothing to remove/)
  assert.match(pinChangeNote(was, pin({ ...was, version: 'v3' }), 'demo', 'remove'), /saved again elsewhere.*Nothing was removed/)
  assert.equal(pinChangeNote(was, was, 'demo', 'remove'), '')
  // Nothing differs, or there was never a pin: the server's own refusal is the better words.
  assert.equal(pinChangeNote(was, was, 'demo'), '')
  assert.equal(pinChangeNote(undefined, undefined, 'demo'), '')
})

test('samePin: a pin is the same only when connector, library skill, revision and mode all match', () => {
  const p = pin()
  const want = { agentId: p.agentId, bundleId: p.bundleId, revisionId: p.revisionId, mode: p.mode }
  assert.equal(samePin(p, want), true)
  assert.equal(samePin(p, { ...want, agentId: 'z'.repeat(48) }), false, 'a pin made through another connector is not the current pin')
  assert.equal(samePin(p, { ...want, agentId: '' }), false)
  assert.equal(samePin(p, { ...want, bundleId: 'z'.repeat(32) }), false)
  assert.equal(samePin(p, { ...want, revisionId: 'z'.repeat(32) }), false)
  assert.equal(samePin(p, { ...want, mode: 'managed' }), false)
  assert.equal(samePin(undefined, want), false)
})

test('MODES is derived from the mode words, in order', () => {
  assert.deepEqual(MODES.map(m => [m.value, m.label]), [['additive', modeLabel('additive')], ['managed', modeLabel('managed')]])
})

test('bundleLabel is the name, with the start of the id only when two library skills share a name', () => {
  const list = [bundle('1', 'alpha'), bundle('2', 'twin'), bundle('3', 'twin'), bundle('4', 'zeta')]
  assert.equal(bundleLabel(list[0], list), 'alpha')
  assert.equal(bundleLabel(list[1], list), `twin · ${'2'.repeat(8)}`)
  assert.equal(bundleLabel(list[2], list), `twin · ${'3'.repeat(8)}`)
  assert.equal(bundleLabel(list[3], list), 'zeta')
  assert.equal(bundleLabel(list[0], [list[0]]), 'alpha')
})

test('revisionLabels names each revision by when it was made and marks the newest', () => {
  const at = (ms) => `t${ms}`
  const r1 = revision('1', 1000), r2 = revision('2', 2000), r3 = revision('3', 3000)
  const labels = revisionLabels([r3, r2, r1], r3.id, at)
  assert.equal(labels.get(r3.id), 't3000 · Newest')
  assert.equal(labels.get(r2.id), 't2000')
  assert.equal(labels.get(r1.id), 't1000')
  assert.equal(revisionLabels([r1, r2], undefined, at).get(r2.id), 't2000', 'no head, no Newest')
  // Two that share a time carry the start of their id, and the newest is still marked.
  const a = revision('a', 5000), b = revision('b', 5000), c = revision('c', 6000)
  const shared = revisionLabels([a, b, c], b.id, at)
  assert.equal(shared.get(a.id), `t5000 · ${'a'.repeat(8)}`)
  assert.equal(shared.get(b.id), `t5000 · ${'b'.repeat(8)} · Newest`)
  assert.equal(shared.get(c.id), 't6000')
  // No raw id appears when the times differ, and the real clock format never shows one either.
  for (const label of revisionLabels([r1, r2, r3], r3.id).values()) assert.doesNotMatch(label, /[0-9a-f]{8}/)
  assert.equal(revisionLabels([], 'x').size, 0)
})

test('revisionLabel falls back to the start of the id only when the revision is not in a list that was read', () => {
  const r1 = revision('1', NOW)
  assert.match(revisionLabel(r1.id, [r1], r1.id), /Newest$/)
  assert.equal(revisionLabel('9'.repeat(32), [r1], r1.id), '9'.repeat(8))
  assert.equal(revisionLabel('9'.repeat(32), undefined, undefined), '9'.repeat(8))
})

test('pinChangeNote uses the revision words it is given, and the start of the id without them', () => {
  const was = pin({ revisionId: '2'.repeat(32), mode: 'managed', version: 'v1' }), changed = pin({ revisionId: '1'.repeat(32), mode: 'additive', version: 'v2' })
  const note = pinChangeNote(was, changed, 'demo', 'save', p => `from ${p.revisionId[0]} o'clock`)
  assert.match(note, /revision from 1 o'clock \(Add and update\)/)
  assert.match(note, /revision from 2 o'clock \(Also remove files reet owns/)
  assert.doesNotMatch(note, /11111111|22222222/)
  assert.match(pinChangeNote(was, changed, 'demo'), /11111111/)
})

test('missingNote says which thing is gone and that nothing was done', () => {
  assert.match(missingNote('save'), /isn't there any more.*nothing was saved/)
  assert.match(missingNote('remove'), /isn't there any more.*nothing was removed/)
  for (const s of [missingNote('save'), missingNote('remove')]) assert.doesNotMatch(s, /installed|installs|\bapplied\b|up to date|verified|_/i)
})

test('the pin sentences name no raw code, claim no install, never call anything verified, and use plain British English', () => {
  const was = pin({ revisionId: '2'.repeat(32), mode: 'managed', version: 'v1' }), changed = pin({ revisionId: '1'.repeat(32), mode: 'additive', version: 'v2' })
  const sentences = [
    ...MODES.map(m => m.label), modeLabel('odd'),
    pinChangeNote(was, changed, 'demo'), pinChangeNote(was, undefined, 'demo'), pinChangeNote(undefined, changed, 'demo'), pinChangeNote(was, pin({ ...was, version: 'v3' }), 'demo'),
    pinChangeNote(was, changed, 'demo', 'remove'), pinChangeNote(was, undefined, 'demo', 'remove'),
    pinChangeNote(pin({ mode: 'what', version: 'a' }), pin({ mode: 'else', version: 'b' }), 'demo'),
  ]
  for (const s of sentences) {
    assert.ok(s.length > 0)
    assert.doesNotMatch(s, /installed|installs|\bapplied\b|up to date|verified/i, `"${s}" implies a change or a check that did not happen`)
    assert.doesNotMatch(s, /_/, `"${s}" shows a raw code`)
    assert.doesNotMatch(s, /\b(nowt|owt|summat|reight|ey up|lass|lad|thee|tha)\b/i, `"${s}" uses dialect`)
    assert.doesNotMatch(s, /\b(color|authoriz\w*|organiz\w*|recogniz\w*|behavior|center|gray)\b/i, `"${s}" is not British spelling`)
  }
})

// ----------------------------------------------------------------------------------------------
// The API client: thin wrappers over `req`. Run them against a recording fetch.
const calls = []
let reply = () => new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
globalThis.fetch = async (url, init = {}) => { calls.push({ url: String(url), method: init.method ?? 'GET', body: init.body, signal: init.signal, contentType: new Headers(init.headers).get('Content-Type') }); return reply() }
const last = () => calls[calls.length - 1]

test('each content wrapper makes exactly one request to the documented route, with the body untouched', async () => {
  const signal = new AbortController().signal
  const pinWrite = { environmentId: '1/workstation/agents', agentId: 'a'.repeat(48), bundleId: 'b'.repeat(32), revisionId: 'c'.repeat(32), mode: 'additive', expectedVersion: '' }
  const previewCreate = { requestKey: 'k'.repeat(20), targetId: 't'.repeat(48), expectedTargetVersion: 'v'.repeat(48) }
  const adopt = { requestKey: 'k'.repeat(20), previewId: 'p'.repeat(48), reviewHash: digest('r'), adoptAsRevisionId: 'c'.repeat(32) }
  const table = [
    ['getContentPins', () => api.getContentPins(signal), 'GET', '/api/v2/skill-content-targets', undefined, signal],
    ['saveContentPin', () => api.saveContentPin(pinWrite), 'POST', '/api/v2/skill-content-targets', pinWrite],
    ['removeContentPin', () => api.removeContentPin('x'.repeat(48), 'v'.repeat(48)), 'DELETE', `/api/v2/skill-content-targets/${'x'.repeat(48)}`, { expectedVersion: 'v'.repeat(48) }],
    ['getContentPreviews', () => api.getContentPreviews(signal), 'GET', '/api/v2/skill-content-previews', undefined, signal],
    ['getContentPreview', () => api.getContentPreview('p'.repeat(48), signal), 'GET', `/api/v2/skill-content-previews/${'p'.repeat(48)}`, undefined, signal],
    ['requestContentPreview', () => api.requestContentPreview(previewCreate), 'POST', '/api/v2/skill-content-previews', previewCreate],
    ['stopContentPreview', () => api.stopContentPreview('p'.repeat(48)), 'POST', `/api/v2/skill-content-previews/${'p'.repeat(48)}/cancel`, {}],
    ['dismissContentPreview', () => api.dismissContentPreview('p'.repeat(48)), 'DELETE', `/api/v2/skill-content-previews/${'p'.repeat(48)}`, undefined],
    ['getContentReceipts', () => api.getContentReceipts(signal), 'GET', '/api/v2/skill-content-receipts', undefined, signal],
    ['adoptContent', () => api.adoptContent(adopt), 'POST', '/api/v2/skill-content-receipts', adopt],
    ['releaseContentReceipt', () => api.releaseContentReceipt('r'.repeat(48), 'v'.repeat(48)), 'DELETE', `/api/v2/skill-content-receipts/${'r'.repeat(48)}`, { expectedVersion: 'v'.repeat(48) }],
    ['getAdoptionBreakdown', () => api.getAdoptionBreakdown('p'.repeat(48), 'c'.repeat(32), signal), 'GET', `/api/v2/skill-content-previews/${'p'.repeat(48)}/adoption/${'c'.repeat(32)}`, undefined, signal],
  ]
  for (const [name, run, method, url, body, expectSignal] of table) {
    calls.length = 0
    assert.deepEqual(await run(), { ok: true }, `${name} returns the parsed body`)
    assert.equal(calls.length, 1, `${name} makes one request`)
    assert.equal(last().method, method, name)
    assert.equal(last().url, url, name)
    if (body === undefined) assert.equal(last().body, undefined, `${name} sends no body`)
    else assert.deepEqual(JSON.parse(last().body), body, `${name} sends its input unchanged`)
    if (expectSignal) assert.equal(last().signal, expectSignal, `${name} passes its abort signal`)
  }
})

test('the wrappers encode ids into the path and never call the revision read that returns file bytes', async () => {
  calls.length = 0
  await api.removeContentPin('a/b c', 'v')
  await api.getContentPreview('../x?y', undefined)
  await api.dismissContentPreview('p#1')
  await api.stopContentPreview('p/2')
  await api.releaseContentReceipt('r&1', 'v')
  await api.getAdoptionBreakdown('p/1', 'r?2')
  assert.deepEqual(calls.map(c => c.url), [
    '/api/v2/skill-content-targets/a%2Fb%20c', '/api/v2/skill-content-previews/..%2Fx%3Fy', '/api/v2/skill-content-previews/p%231',
    '/api/v2/skill-content-previews/p%2F2/cancel', '/api/v2/skill-content-receipts/r%261', '/api/v2/skill-content-previews/p%2F1/adoption/r%3F2',
  ])
  // Source guard: the new wrappers only ever name the content routes.
  const source = readFileSync(new URL('./api.ts', import.meta.url), 'utf8')
  const wrappers = source.split('\n').filter(l => /^export (async )?function (getContent|saveContent|removeContent|requestContent|stopContent|dismissContent|adoptContent|releaseContent|getAdoptionBreakdown)/.test(l))
  assert.equal(wrappers.length, 12)
  for (const l of wrappers) { assert.doesNotMatch(l, /skill-bundles|revisions/, l); assert.match(l, /\/api\/v2\/skill-content-(targets|previews|receipts)/, l) }
})

const jsonReply = () => new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
const previewInput = { requestKey: 'k'.repeat(20), targetId: 't'.repeat(48), expectedTargetVersion: 'v'.repeat(48) }

test('a refusal is an ApiError carrying the server text and the HTTP status', async () => {
  try {
    reply = () => new Response(JSON.stringify({ error: 'x' }), { status: 409 })
    await assert.rejects(api.requestContentPreview(previewInput), (e) => {
      assert.ok(e instanceof api.ApiError, 'an ApiError')
      assert.ok(e instanceof Error, 'still an Error, so callers that read e.message are unchanged')
      assert.equal(e.status, 409)
      assert.equal(e.message, 'x')
      assert.equal(e.name, 'ApiError')
      return true
    })
    // No JSON body: the message is the status line, exactly as before, and the status is still kept.
    reply = () => new Response('', { status: 500, statusText: 'Internal Server Error' })
    await assert.rejects(api.requestContentPreview(previewInput), (e) => { assert.ok(e instanceof api.ApiError); assert.equal(e.status, 500); assert.equal(e.message, '500 Internal Server Error'); return true })
    reply = () => new Response('<html>bad gateway</html>', { status: 502, statusText: 'Bad Gateway' })
    await assert.rejects(api.getContentPins(), (e) => { assert.ok(e instanceof api.ApiError); assert.equal(e.status, 502); assert.equal(e.message, '502 Bad Gateway'); return true })
    // Any non-ok, non-401 status, through any wrapper.
    for (const status of [400, 403, 404, 409, 500, 503]) {
      reply = () => new Response(JSON.stringify({ error: `refused ${status}` }), { status })
      await assert.rejects(api.removeContentPin('x'.repeat(48), 'v'.repeat(48)), (e) => { assert.ok(e instanceof api.ApiError, String(status)); assert.equal(e.status, status); assert.equal(e.message, `refused ${status}`); return true })
    }
  } finally { reply = jsonReply }
})

test('a 401 is still an UnauthorizedError and is not an ApiError', async () => {
  try {
    reply = () => new Response(JSON.stringify({ error: 'sign in again' }), { status: 401 })
    await assert.rejects(api.getContentPins(), (e) => { assert.ok(e instanceof api.UnauthorizedError); assert.ok(!(e instanceof api.ApiError)); assert.equal(e.message, 'sign in again'); return true })
    reply = () => new Response('', { status: 401, statusText: 'Unauthorized' })
    await assert.rejects(api.getContentPins(), (e) => { assert.ok(e instanceof api.UnauthorizedError); assert.ok(!(e instanceof api.ApiError)); assert.equal(e.message, 'missing or wrong token'); return true })
  } finally { reply = jsonReply }
})

test('a refusal carrying the server text is passed on by describeError, and a bare status line is not', async () => {
  try {
    reply = () => new Response(JSON.stringify({ error: 'This ownership record is unavailable, blocked, changed or already exists.' }), { status: 409 })
    await assert.rejects(api.adoptContent({ requestKey: 'k'.repeat(20), previewId: 'p'.repeat(48), reviewHash: digest('r'), adoptAsRevisionId: 'c'.repeat(32) }), (e) => {
      assert.equal(describeError(e), 'This ownership record is unavailable, blocked, changed or already exists.')
      return true
    })
    reply = () => new Response('', { status: 500, statusText: 'Internal Server Error' })
    await assert.rejects(api.getContentPins(), (e) => { assert.equal(describeError(e), 'Something went wrong. Try again.'); return true })
    // A 200 that is not JSON (a proxy's page, say) is a SyntaxError from the wrapper's own .json(), never shown as text.
    reply = () => new Response('<!doctype html><title>x</title>', { status: 200 })
    await assert.rejects(api.getContentPins(), (e) => { assert.equal(e.name, 'SyntaxError'); assert.equal(describeError(e), 'Something went wrong. Try again.'); return true })
  } finally { reply = jsonReply }
})

test('asSeenFromPin: a check made for an older pin version is superseded by the pin, whatever the server last said', () => {
  const captured = { state: 'captured', reasons: [], targetVersion: 'v1', capture: { presence: 'present' } }
  const same = asSeenFromPin(captured, { version: 'v1' })
  assert.equal(same, captured)
  const moved = asSeenFromPin(captured, { version: 'v2' })
  assert.equal(moved.state, 'superseded')
  assert.deepEqual(moved.reasons, ['pin_changed'])
  assert.equal(classifyCheck(moved, 0, 0).kind, 'stale')
  assert.equal(classifyCheck(moved, 0, 0).canAdopt, false)
  assert.equal(captured.state, 'captured', 'the input is not changed')
  for (const state of ['queued', 'capturing', 'stale', 'superseded']) assert.equal(asSeenFromPin({ state, reasons: ['pin_changed'], targetVersion: 'v1' }, { version: 'v2' }).reasons.length, 1)
  for (const state of ['failed', 'cancelled', 'expired']) assert.equal(asSeenFromPin({ state, targetVersion: 'v1' }, { version: 'v2' }).state, state)
  assert.equal(asSeenFromPin(null, { version: 'v2' }), null)
  assert.equal(asSeenFromPin(captured, undefined), captured)
})

test('planVerdict: up to date is only said of a folder that was found', () => {
  const ready = { ready: true, actions: [], conflicts: [], preserved: [] }
  assert.equal(planVerdict({ capture: { presence: 'present' }, plan: ready }).kind, 'up_to_date')
  assert.equal(planVerdict({ capture: { presence: 'unknown' }, plan: ready }).kind, 'unknown')
  assert.equal(planVerdict({ capture: { presence: 'absent' }, plan: ready }).kind, 'unknown')
  assert.equal(planVerdict({ capture: undefined, plan: ready }).kind, 'unknown')
  assert.equal(planVerdict({ capture: { presence: 'present' }, plan: undefined }).kind, 'unknown')
  const create = { ready: true, actions: [{ path: 'a', kind: 'create' }], conflicts: [], preserved: [] }
  assert.equal(planVerdict({ capture: { presence: 'absent' }, plan: create }).kind, 'changes')
})

test('the connector problem sentence is plain and carries no code', () => {
  assert.match(CONNECTOR_PROBLEM, /^[A-Z][^_]*\.$/)
})
