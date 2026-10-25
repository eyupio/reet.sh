import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
// Compile the pure review module and the API client without a browser or an extra test dependency.
const compile = (file) => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const load = (file) => import(`data:text/javascript;base64,${Buffer.from(compile(file)).toString('base64')}`)
const review = await load('./contentReview.ts')
const {
  OWNERSHIP_SENTENCE, APPLY_NOTICE, FALLBACK,
  presenceLabel, ownershipNote, checkStateLabel, reasonText, failureText, planCodeText, preservedText,
  scopeEligibility, classifyCheck, adoptable, summarisePlan, candidatesFor, preselect, groupBreakdown, claimSummary, pollDelay, describeError,
} = review

// The closed tables, copied from the Go side. A new server code must fail loudly here.
const STATES = ['queued', 'capturing', 'captured', 'failed', 'cancelled', 'expired', 'superseded', 'stale']
const FENCE_REASONS = ['pin_changed', 'pin_removed', 'connector_paused', 'connector_removed', 'process_changed', 'inventory_changed', 'scope_removed', 'receipt_changed', 'expired', 'stale']
const FAILURE_CODES = ['capture_failed', 'scope_unavailable', 'plan_unavailable']
const PLAN_CODES = ['adapter_unsupported', 'root_shared', 'root_unknown', 'generation_changed', 'inventory_changed', 'observation_unknown', 'observation_stale', 'observation_invalid', 'adoption_required', 'unmanaged_path', 'managed_target_missing', 'local_modified', 'both_modified', 'local_deleted', 'result_invalid', 'baseline_target_mismatch', 'baseline_bundle_mismatch']
const PRESERVED_REASONS = ['local_only', 'additive_managed']
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

test('folder ownership: exclusive says nothing, shared and unknown say so, anything else falls back', () => {
  assert.equal(ownershipNote('exclusive'), '')
  assert.equal(ownershipNote(undefined), '')
  assert.equal(ownershipNote(''), '')
  const shared = ownershipNote('shared'), unknown = ownershipNote('unknown')
  assert.match(shared, /another agent scope reaches (the|this) same folder/i)
  assert.match(shared, /will not touch it/)
  assert.match(unknown, /couldn't tell/i)
  assert.notEqual(shared, unknown)
  assert.equal(ownershipNote('exclusivee'), FALLBACK)
  assert.equal(ownershipNote('constructor'), FALLBACK)
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
  const blocked = summarisePlan(plan({ ready: false, conflicts }))
  assert.equal(blocked.kind, 'blocked')
  assert.deepEqual(blocked.conflicts, conflicts)
  assert.deepEqual(blocked.actions, [])
  assert.equal(summarisePlan(plan({ conflicts: [{ path: '.', code: 'root_shared' }] })).kind, 'blocked')
  const adoption = summarisePlan(plan())
  assert.equal(adoption.kind, 'adoption')
  assert.deepEqual(adoption.conflicts, [{ path: '.', code: 'adoption_required' }])
  // adoption_required alongside any other conflict is a block, not an offer.
  assert.equal(summarisePlan(plan({ conflicts: [{ path: '.', code: 'adoption_required' }, { path: 'x', code: 'unmanaged_path' }] })).kind, 'blocked')
  // A plan that is neither ready nor explained is never reassuring.
  assert.equal(summarisePlan(plan({ ready: false, conflicts: [] })).kind, 'unknown')
  // A ready plan that still lists a conflict is never up to date.
  assert.equal(summarisePlan(plan({ ready: true, conflicts: [{ path: 'x', code: 'local_modified' }] })).kind, 'blocked')
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
  // The merged row describes the machine's copy: that is the file that was edited and stays yours.
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
  assert.equal(s.text, '2 files would be recorded as owned by reet. 1 file was edited on this machine and stays yours. 1 file is only on this machine and stays yours. 1 file is only in the library.')
  assert.equal(claimSummary({ revisionId: 'c'.repeat(32), matched: 1, observedOnly: 0, revisionOnly: 0, files: [row('SKILL.md', 'claimed')] }).text, '1 file would be recorded as owned by reet.')
  assert.equal(claimSummary({ revisionId: 'c'.repeat(32), matched: 0, observedOnly: 3, revisionOnly: 2, files: [row('a', 'stays_yours'), row('b', 'stays_yours'), row('c', 'stays_yours'), row('d', 'library_only'), row('e', 'library_only')] }).text, 'No files would be recorded as owned by reet. 3 files are only on this machine and stay yours. 2 files are only in the library.')
  assert.equal(claimSummary({ revisionId: 'c'.repeat(32), matched: 0, observedOnly: 0, revisionOnly: 0, files: [row('a', 'stays_yours'), row('a', 'library_only'), row('b', 'stays_yours'), row('b', 'library_only')] }).text, 'No files would be recorded as owned by reet. 2 files were edited on this machine and stay yours.')
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
  assert.equal(describeError(Object.assign(new Error('missing or wrong token'), { name: 'UnauthorizedError' })), 'missing or wrong token')
  assert.equal(describeError({ error: 'That pin changed.' }), 'That pin changed.')
  assert.equal(describeError('Content preview history is full (64 requests). Dismiss finished previews, then try again.'), 'Content preview history is full (64 requests). Dismiss finished previews, then try again.')
  const generic = 'Something went wrong. Try again.'
  for (const e of [null, undefined, 42, {}, [], new Error(''), new Error('   '), { error: '' }, { error: 7 }]) assert.equal(describeError(e), generic, String(JSON.stringify(e)))
  // The client's own fallback when a refusal carried no text.
  for (const m of ['409 Conflict', '500 Internal Server Error', '404 ', '403']) assert.equal(describeError(new Error(m)), generic, m)
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
    reasonOf(scopeEligibility(env({ complete: false }), a, opts())), reasonOf(scopeEligibility(e, [], opts())), reasonOf(scopeEligibility(e, [agent({ paused: true })], opts())),
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
// The API client: thin wrappers over `req`. Run them against a recording fetch.
const api = await load('./api.ts')
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

test('a refusal surfaces as an Error carrying the server text, which describeError passes on', async () => {
  reply = () => new Response(JSON.stringify({ error: 'This ownership record is unavailable, blocked, changed or already exists.' }), { status: 409 })
  await assert.rejects(api.adoptContent({ requestKey: 'k'.repeat(20), previewId: 'p'.repeat(48), reviewHash: digest('r'), adoptAsRevisionId: 'c'.repeat(32) }), (e) => {
    assert.equal(describeError(e), 'This ownership record is unavailable, blocked, changed or already exists.')
    return true
  })
  reply = () => new Response('', { status: 500, statusText: 'Internal Server Error' })
  await assert.rejects(api.getContentPins(), (e) => { assert.equal(describeError(e), 'Something went wrong. Try again.'); return true })
})
