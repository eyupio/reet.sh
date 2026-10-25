import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// Compile the pure modules without a browser or an extra dependency, wiring each relative import to its data URL.
const transpile = (file) => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const url = (source) => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const machinesURL = url(transpile('./machines.ts'))
const setupURL = url(transpile('./machineSetup.ts').replace("from './machines'", `from '${machinesURL}'`))
const reviewURL = url(transpile('./contentReview.ts'))
const joinSource = transpile('./machineJoin.ts')
  .replace("from './machineSetup'", `from '${setupURL}'`)
  .replace("from './contentReview'", `from '${reviewURL}'`)
  .replace("from './machines'", `from '${machinesURL}'`)
const join = await import(url(joinSource))
const setup = await import(setupURL)
const review = await import(reviewURL)
const { MAX_MACHINE_NAME_BYTES, JOIN_WATCH_CAP_MS, validateMachineName, canonicalHost, joinCommands, joinBaseline, joinState, landingFor, joinDelay, JOIN_AUTO_MS, fromPrivateSkills, landingNote } = join

const now = 1_800_000_000_000
const DAY = 86_400_000
const group = (agent, changes = {}) => ({ id: `${agent}-id`, host: 'studio', agent, skills: [], complete: true, scannedAt: now - 1000, os: '', arch: '', version: '', ...changes })
const machine = (groups, host = 'studio') => ({ host, groups, skillCount: 0 })
const agent = (changes = {}) => ({ id: 'a1', host: 'studio', version: '1', lastSeen: now, paused: false, autoUpdate: false, ...changes })
const readyMachine = () => machine([group('claude')])
const opts = (changes = {}) => ({ accounts: true, canWrite: true, now, ...changes })
const view = (changes = {}) => joinState({ name: 'studio', machines: [readyMachine()], agents: [agent()], baseline: null, now, ...changes })

test('validateMachineName trims and refuses empty, too long and control characters, each with its own reason', () => {
  assert.deepEqual(validateMachineName('  studio  '), { ok: true, name: 'studio' })
  assert.equal(MAX_MACHINE_NAME_BYTES, 200)
  const reasons = new Set()
  for (const bad of ['', '   ', 'a'.repeat(201), 'a\nb', 'a\tb', 'a\0b', 'a\x7fb', 'a\x85b', 'a\u2028b']) {
    const r = validateMachineName(bad)
    assert.equal(r.ok, false, JSON.stringify(bad))
    assert.match(r.reason, /\S/)
    reasons.add(r.reason)
  }
  assert.equal(reasons.size, 3)
  assert.equal(validateMachineName('').reason, validateMachineName('   ').reason)
  assert.equal(validateMachineName('a'.repeat(200)).ok, true)
  assert.equal(validateMachineName('\n').ok, false)
  assert.deepEqual(validateMachineName(' Zoë Ünï 研究 '), { ok: true, name: 'Zoë Ünï 研究' })
  assert.deepEqual(validateMachineName("Paul's Mac"), { ok: true, name: "Paul's Mac" })
})

test('the name limit counts UTF-8 bytes, as the server does', () => {
  const tooLong = 'That name is too long. Use at most 200 bytes; accented letters and emoji take more than one.'
  assert.equal(validateMachineName('😀'.repeat(50)).ok, true) // 200 bytes
  assert.deepEqual(validateMachineName('😀'.repeat(51)), { ok: false, reason: tooLong }) // 204 bytes
  assert.equal(validateMachineName('a'.repeat(200)).ok, true)
  assert.deepEqual(validateMachineName('a'.repeat(201)), { ok: false, reason: tooLong })
  assert.equal(validateMachineName('é'.repeat(100)).ok, true) // 200 bytes
  assert.deepEqual(validateMachineName('é'.repeat(101)), { ok: false, reason: tooLong })
  assert.equal(validateMachineName('研'.repeat(66)).ok, true) // 198 bytes
  assert.deepEqual(validateMachineName('研'.repeat(67)), { ok: false, reason: tooLong }) // 201 bytes
  // Trimming happens before counting.
  assert.equal(validateMachineName(`  ${'a'.repeat(200)}  `).ok, true)
})

test('joinCommands is install, configure and enrol in order with a blank line between, and never holds a token', () => {
  const site = 'https://reet.example'
  for (const channel of ['stable', 'dev']) {
    assert.equal(joinCommands(site, 'studio', channel), [setup.installCommands(channel), setup.configureCommand(site), setup.enrollCommands(site, 'studio')].join('\n\n'))
  }
  const text = joinCommands(site, 'studio', 'stable')
  assert.doesNotMatch(text, /token|Bearer|--allow-apply/i)
  assert.equal(joinCommands(site, '  studio ', 'stable'), text)
})

test('joinCommands quotes hostile names exactly as shellQuote does', () => {
  const site = 'https://reet.example'
  const install = 'curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh\nexport PATH="$HOME/.local/bin:$PATH"\nreet --version'
  const prefix = `${install}\n\nreet configure --url 'https://reet.example' --version latest\n\nreet agent enroll --server 'https://reet.example' --env `
  assert.equal(joinCommands(site, "it's $(x) `y`", 'stable'), `${prefix}'it'\\''s $(x) \`y\`'\nreet agent run`)
  assert.equal(joinCommands(site, 'a b', 'stable'), `${prefix}'a b'\nreet agent run`)
  assert.equal(joinCommands(site, "o'brien", 'stable'), `${prefix}'o'\\''brien'\nreet agent run`)
  assert.equal(joinCommands(site, '$(touch /tmp/x)', 'stable'), `${prefix}'$(touch /tmp/x)'\nreet agent run`)
})

test('joinState: no connector for the name is waiting', () => {
  const v = view({ agents: [agent({ host: 'other' })] })
  assert.equal(v.stage, 'waiting')
  assert.equal(v.label, 'Waiting for it')
  assert.equal(v.canUse, false)
  assert.equal(view({ agents: [], machines: [] }).stage, 'waiting')
})

test('joinState: a paused connector is connected, with the pause reason, and cannot be used', () => {
  const v = view({ agents: [agent({ paused: true })] })
  assert.equal(v.stage, 'connected')
  assert.equal(v.label, 'Connected')
  assert.equal(v.detail, 'Its connector is paused. Resume it on the Machines page.')
  assert.equal(v.canUse, false)
})

test('joinState: a connector without a complete recent supported scan is waiting for its first scan', () => {
  const cases = {
    'no machine entry': { machines: [] },
    'no groups': { machines: [machine([])] },
    'scan-only agent': { machines: [machine([group('codex')])] },
    'server group only': { machines: [machine([group('reet-server')])] },
    'incomplete': { machines: [machine([group('claude', { complete: false })])] },
    'no scan time': { machines: [machine([group('claude', { scannedAt: 0 })])] },
    'non-numeric time': { machines: [machine([group('claude', { scannedAt: 'yesterday' })])] },
  }
  for (const [label, changes] of Object.entries(cases)) {
    const v = view(changes)
    assert.equal(v.stage, 'first_scan', label)
    assert.equal(v.label, 'Waiting for its first scan', label)
    assert.equal(v.canUse, false, label)
  }
})

test('joinState: a stale-only scan does not count as ready, but exactly 14 days does', () => {
  assert.equal(view({ machines: [machine([group('claude', { scannedAt: now - 14 * DAY - 1 })])] }).stage, 'first_scan')
  assert.equal(view({ machines: [machine([group('claude', { scannedAt: now - 14 * DAY })])] }).stage, 'ready')
})

test('joinState: a ready machine is ready and usable, with plain words', () => {
  const v = view()
  assert.deepEqual([v.stage, v.label, v.canUse], ['ready', 'Ready', true])
  assert.match(v.detail, /\S/)
  for (const stage of [v, view({ agents: [agent({ paused: true })] }), view({ agents: [] }), view({ machines: [] })]) {
    assert.doesNotMatch(stage.detail, /verified|healthy|succe/i)
  }
  // One supported group with a good scan is enough, even beside a stale one and a scan-only one.
  assert.equal(view({ machines: [machine([group('codex'), group('cursor', { scannedAt: now - 30 * DAY }), group('opencode')])] }).stage, 'ready')
})

test('joinState: several connectors for one name are ready if any running one qualifies, connected only if all are paused', () => {
  assert.equal(view({ agents: [agent({ id: 'a1', paused: true }), agent({ id: 'a2' })] }).stage, 'ready')
  assert.equal(view({ agents: [agent({ id: 'a1', paused: true }), agent({ id: 'a2', paused: true })] }).stage, 'connected')
  assert.equal(view({ agents: [agent({ id: 'a1', paused: true }), agent({ id: 'a2' })], machines: [] }).stage, 'first_scan')
})

test('joinState: host matching folds ASCII only, for the connector and for the machine entry', () => {
  assert.equal(view({ name: 'STUDIO' }).stage, 'ready')
  assert.equal(view({ name: 'studio', agents: [agent({ host: 'STUDIO' })], machines: [machine([group('claude')], 'Studio')] }).stage, 'ready')
  const zoe = (host, mHost) => view({ name: host, agents: [agent({ host })], machines: [machine([group('claude')], mHost)] })
  assert.equal(zoe('Zoë', 'ZOë').stage, 'ready')
  assert.equal(zoe('Zoë', 'ZoË').stage, 'first_scan')
  assert.equal(view({ name: 'Zoë', agents: [agent({ host: 'ZoË' })] }).stage, 'waiting')
  assert.equal(view({ name: '\u212Aelvin', agents: [agent({ host: 'kelvin' })] }).stage, 'waiting')
})

test('the host fold is the one contentReview uses', () => {
  assert.equal(review.sameHost('HOST', 'host'), true)
  assert.equal(review.sameHost('Zoë', 'ZoË'), false)
  assert.equal(review.sameHost('ZOë', 'zoë'), true)
})

test('joinBaseline lists the connector ids under the name, or null', () => {
  assert.equal(joinBaseline('studio', [], []), null)
  assert.equal(joinBaseline('studio', [agent({ host: 'other' })], [machine([group('claude')], 'other')]), null)
  assert.deepEqual(joinBaseline(' STUDIO ', [agent({ id: 'a1' }), agent({ id: 'a2', host: 'other' }), agent({ id: 'a3', host: 'studio', paused: true })], []), { agentIds: ['a1', 'a3'], scannedAt: 0 })
  // Scans alone make a baseline; only complete scans of supported folders count, and the newest wins.
  const scanned = machine([group('claude', { scannedAt: now - 5000 }), group('cursor', { scannedAt: now - 2000 }), group('codex', { scannedAt: now }), group('opencode', { complete: false, scannedAt: now })])
  assert.deepEqual(joinBaseline('studio', [], [scanned]), { agentIds: [], scannedAt: now - 2000 })
  assert.equal(joinBaseline('studio', [], [machine([group('codex')])]), null)
})

test('joinState: a baseline that matches a healthy machine is already_here and usable', () => {
  const agents = [agent()]
  const v = view({ agents, baseline: joinBaseline('studio', agents, [readyMachine()]) })
  assert.equal(v.stage, 'already_here')
  assert.equal(v.label, 'A machine called studio is already here.')
  assert.equal(v.canUse, true)
  assert.equal(view({ name: '  studio ', agents, baseline: { agentIds: ['a1'], scannedAt: now - 1000 } }).label, 'A machine called studio is already here.')
})

test('joinState: already_here cannot be used when the existing machine is paused, stale or unscanned, and says why', () => {
  const base = { agentIds: ['a1'], scannedAt: now - 1000 }
  const paused = view({ agents: [agent({ paused: true })], baseline: base })
  assert.deepEqual([paused.stage, paused.canUse], ['already_here', false])
  assert.match(paused.detail, /paused/)
  const stale = view({ machines: [machine([group('claude', { scannedAt: now - 15 * DAY })])], baseline: base })
  assert.deepEqual([stale.stage, stale.canUse], ['already_here', false])
  assert.match(stale.detail, /14 days/)
  const gone = view({ agents: [], baseline: base })
  assert.deepEqual([gone.stage, gone.canUse], ['already_here', false])
})

test('joinState: already_here is sticky while lastSeen moves, and ends only when a connector outside the baseline appears', () => {
  const baseline = { agentIds: ['a1'], scannedAt: now - 1000 }
  for (const lastSeen of [now - 5 * DAY, now - 1, now, now + 60_000, now + 10 * DAY]) {
    assert.equal(view({ agents: [agent({ lastSeen })], baseline }).stage, 'already_here', String(lastSeen))
  }
  // The same id, resumed or paused, is still the baseline connector.
  assert.equal(view({ agents: [agent({ paused: true, lastSeen: now + 5 })], baseline }).stage, 'already_here')
  // A connector for a different name does not end it.
  assert.equal(view({ agents: [agent(), agent({ id: 'zz', host: 'other' })], baseline }).stage, 'already_here')
  // A fresh enrolment under the name does, and the normal states then apply to it.
  assert.equal(view({ agents: [agent(), agent({ id: 'a2' })], baseline }).stage, 'first_scan') // the old scan is not evidence about the newcomer
  assert.equal(view({ agents: [agent(), agent({ id: 'a2' })], machines: [machine([group('claude', { scannedAt: now })])], baseline }).stage, 'ready')
  assert.equal(view({ agents: [agent(), agent({ id: 'a2', paused: true })], baseline }).stage, 'connected')
  assert.equal(view({ agents: [agent(), agent({ id: 'a2' })], machines: [], baseline }).stage, 'first_scan')
  // With no baseline the normal states apply from the start.
  assert.equal(view({ baseline: null }).stage, 'ready')
})

test('joinState: anything it does not recognise is unknown, with the fallback sentence', () => {
  for (const changes of [{ agents: null }, { machines: undefined }, { name: '   ' }, { agents: [null] }, { agents: [{ id: 'a1' }] }, { machines: [{ host: 'studio' }] }]) {
    const v = view(changes)
    assert.equal(v.stage, 'unknown', JSON.stringify(changes))
    assert.equal(v.label, 'Something changed. Check again.')
    assert.equal(v.canUse, false)
  }
})

test('landingFor prefers the same agent, otherwise the first qualifying folder, and counts the others', () => {
  const m = machine([group('claude'), group('cursor'), group('opencode')])
  const a = [agent()]
  assert.deepEqual(landingFor({ skill: 'tidy', agent: 'cursor' }, m, a, opts()), { kind: 'open', environmentId: 'cursor-id', name: 'tidy', chosen: 'Cursor', others: 2 })
  assert.deepEqual(landingFor({ agent: 'codex' }, m, a, opts()), { kind: 'open', environmentId: 'claude-id', chosen: 'Claude Code', others: 2 })
  assert.deepEqual(landingFor({}, machine([group('claude')]), a, opts()), { kind: 'open', environmentId: 'claude-id', chosen: 'Claude Code', others: 0 })
  // A preferred folder that does not qualify is skipped; the others are counted without it.
  const mixed = machine([group('cursor', { complete: false }), group('opencode'), group('claude')])
  assert.deepEqual(landingFor({ agent: 'cursor' }, mixed, a, opts()), { kind: 'open', environmentId: 'opencode-id', chosen: 'OpenCode', others: 1 })
  // Project folders keep their label.
  const project = machine([group('claude', { id: 'p', project: '/work/app' })])
  assert.equal(landingFor({}, project, a, opts()).chosen, 'Claude Code · /work/app')
})

test('landingFor says plainly why nothing qualifies', () => {
  const a = [agent()]
  const none = (m, agents = a, o = opts(), ctx = {}) => {
    const r = landingFor(ctx, m, agents, o)
    assert.equal(r.kind, 'none')
    assert.match(r.reason, /\S/)
    return r.reason
  }
  assert.equal(none(undefined), "That machine isn't listed yet.")
  assert.equal(none(machine([])), "That machine hasn't reported an agent folder yet.")
  assert.equal(none(machine([group('codex')])), 'Its agent folders are scan-only, so private skills cannot be pinned to them.')
  assert.equal(none(machine([group('claude', { scannedAt: now - 15 * DAY })])), "This machine hasn't reported a complete scan of this folder in the last 14 days.")
  assert.equal(none(machine([group('claude', { complete: false })])), "This machine hasn't reported a complete scan of this folder in the last 14 days.")
  assert.equal(none(machine([group('claude')]), [agent({ paused: true })]), 'The connector on this machine is paused. Resume it to check this machine.')
  assert.equal(none(machine([group('claude')]), []), "No connector is enrolled on this machine, so it can't be checked.")
  assert.equal(none(machine([group('claude')]), [agent()], opts({ canWrite: false })), "Your account can only view, so it can't pin or check private skills.")
  // The reason follows the folder that got furthest: a stale preferred folder does not hide a paused good one.
  assert.equal(none(machine([group('cursor', { scannedAt: 0 }), group('claude')]), [agent({ paused: true })], opts(), { agent: 'cursor' }), 'The connector on this machine is paused. Resume it to check this machine.')
})

test('joinDelay starts at 3000, grows and is capped at 20000; the watch cap is fifteen minutes', () => {
  assert.equal(joinDelay(0), 3000)
  assert.equal(joinDelay(-1), 3000)
  assert.equal(joinDelay(1), 6000)
  assert.equal(joinDelay(2), 12000)
  assert.equal(joinDelay(3), 20000)
  assert.equal(joinDelay(50), 20000)
  assert.equal(joinDelay(2), setup.retryDelay(2, 3000, 20000))
  assert.equal(JOIN_WATCH_CAP_MS, 900000)
})

test('joinState: an old scan never makes a new enrolment ready; only a strictly newer one does', () => {
  const old = machine([group('claude', { scannedAt: now - 1000 })])
  // A scan-only name (no connector yet) is a baseline with scans: waiting, then first_scan, then ready on a newer scan.
  const baseline = joinBaseline('studio', [], [old])
  assert.deepEqual(baseline, { agentIds: [], scannedAt: now - 1000 })
  assert.equal(view({ agents: [], machines: [old], baseline }).stage, 'waiting')
  const newcomer = view({ agents: [agent()], machines: [old], baseline })
  assert.deepEqual([newcomer.stage, newcomer.label, newcomer.canUse], ['first_scan', 'Waiting for its first scan', false])
  assert.equal(view({ agents: [agent()], machines: [machine([group('claude', { scannedAt: now - 1000 })])], baseline }).stage, 'first_scan') // equal is not newer
  assert.equal(view({ agents: [agent()], machines: [machine([group('claude', { scannedAt: now - 999 })])], baseline }).stage, 'ready')
  assert.equal(view({ agents: [agent()], machines: [machine([group('claude', { scannedAt: now - 1000 }), group('cursor', { scannedAt: now })])], baseline }).stage, 'ready')
  // A paused newcomer is connected whatever the scan.
  assert.equal(view({ agents: [agent({ paused: true })], machines: [old], baseline }).stage, 'connected')
  // An old healthy machine plus a new connector id: first_scan until a newer scan arrives.
  const healthy = [agent()]
  const base = joinBaseline('studio', healthy, [old])
  assert.equal(view({ agents: [...healthy, agent({ id: 'a2' })], machines: [old], baseline: base }).stage, 'first_scan')
  assert.equal(view({ agents: [...healthy, agent({ id: 'a2' })], machines: [machine([group('claude', { scannedAt: now })])], baseline: base }).stage, 'ready')
  // A fresh name (null baseline) behaves as before.
  assert.equal(view({ baseline: null }).stage, 'ready')
})

test('malformed machines and groups are unknown, never a throw', () => {
  for (const bad of [{ host: 'studio', groups: [null] }, { host: 'studio', groups: [{ id: 'x', scannedAt: now, complete: true }] }, { host: 'studio', groups: [{ agent: 3 }] }]) {
    const v = view({ machines: [bad] })
    assert.equal(v.stage, 'unknown', JSON.stringify(bad))
    const l = landingFor({}, bad, [agent()], opts())
    assert.equal(l.kind, 'none')
    assert.match(l.reason, /\S/)
  }
  assert.equal(joinBaseline('studio', [agent()], [{ host: 'studio', groups: [null] }]).scannedAt, 0)
  assert.equal(landingFor({}, readyMachine(), [null], opts()).kind, 'none')
})

test('names with text-direction overrides are refused, but zero-width joiners are allowed', () => {
  const reasons = new Set()
  for (const c of ['\u202a', '\u202b', '\u202c', '\u202d', '\u202e', '\u2066', '\u2067', '\u2068', '\u2069']) {
    const r = validateMachineName(`a${c}b`)
    assert.equal(r.ok, false, c)
    reasons.add(r.reason)
  }
  assert.equal(reasons.size, 1)
  assert.notEqual([...reasons][0], validateMachineName('a\nb').reason)
  assert.equal(validateMachineName('a\u200db\u200c').ok, true)
})

test('joinCommands only quotes: refusing control characters is validateMachineName\'s job, not the builder\'s', () => {
  assert.match(joinCommands('https://reet.example', 'a\nb', 'stable'), /--env 'a\nb'\n/)
})

test('landingFor passes on the sign-in reason when accounts are off', () => {
  assert.deepEqual(landingFor({}, readyMachine(), [agent()], opts({ accounts: false })), { kind: 'none', reason: 'Private skills need a signed-in account.' })
})

const aliased = (host, aliases, groups = [group('claude', { host })]) => ({ host, aliases, groups, skillCount: 0 })

test('canonicalHost maps a saved alias to its machine and leaves other names alone', () => {
  const ms = [aliased('studio', ['old-studio', 'mac-1']), machine([group('claude')], 'other')]
  assert.equal(canonicalHost('old-studio', ms), 'studio')
  assert.equal(canonicalHost('  Old-Studio ', ms), 'studio') // aliases are stored lower case
  assert.equal(canonicalHost('MAC-1', ms), 'studio')
  assert.equal(canonicalHost('studio', ms), 'studio')
  assert.equal(canonicalHost('  brand-new ', ms), 'brand-new')
  assert.equal(canonicalHost('old-studio', []), 'old-studio')
  // Aliases are compared exactly once lower-cased: no partial match, no stored upper case.
  assert.equal(canonicalHost('old', ms), 'old')
  assert.equal(canonicalHost('x', [aliased('studio', ['X'])]), 'x')
})

test('canonicalHost never throws on odd input', () => {
  for (const ms of [null, undefined, 'x', 3, {}, [null], [undefined, 4, 'x'], [{ host: 'h', aliases: 'a' }], [{ host: 'h', aliases: [1, null, {}] }], [{ aliases: ['a'] }], [{ host: 5, aliases: ['a'] }]]) {
    assert.equal(canonicalHost('a', ms), 'a', JSON.stringify(ms))
  }
  assert.equal(canonicalHost(undefined, []), '')
  assert.equal(canonicalHost(null, []), '')
})

test('joinState: an alias of a machine that is already reporting is already_here, and says what was typed', () => {
  const machines = [aliased('studio', ['old-studio'])]
  const agents = [agent()]
  const baseline = joinBaseline('old-studio', agents, machines)
  assert.deepEqual(baseline, { agentIds: ['a1'], scannedAt: baseline.scannedAt })
  const v = view({ name: 'Old-Studio', machines, agents, baseline })
  assert.equal(v.stage, 'already_here')
  assert.equal(v.label, 'A machine called studio is already here.')
  assert.equal(v.canUse, true)
  assert.match(v.detail, /You typed Old-Studio; reet\.sh saved it under studio\./)
  // The same machine typed by its own name has no such sentence.
  const plain = view({ name: 'studio', machines, agents, baseline: joinBaseline('studio', agents, machines) })
  assert.equal(plain.label, 'A machine called studio is already here.')
  assert.doesNotMatch(plain.detail, /You typed/)
  // The sentence also shows when the machine cannot be used.
  const paused = view({ name: 'old-studio', machines, agents: [agent({ paused: true })], baseline })
  assert.match(paused.detail, /paused/)
  assert.match(paused.detail, /You typed old-studio; reet\.sh saved it under studio\./)
})

test('joinState: an alias of a machine with no connector yet waits, then walks to ready under the canonical host', () => {
  const old = aliased('studio', ['old-studio'], [group('claude', { scannedAt: now - 1000 })])
  const baseline = joinBaseline('old-studio', [], [old])
  assert.deepEqual(baseline, { agentIds: [], scannedAt: now - 1000 })
  const at = (changes) => view({ name: 'old-studio', baseline, machines: [old], agents: [], ...changes })
  const waiting = at({})
  assert.equal(waiting.stage, 'waiting')
  assert.match(waiting.detail, /The machine will appear as studio\./)
  assert.equal(at({ agents: [agent({ host: 'other' })] }).stage, 'waiting')
  assert.equal(at({ agents: [agent({ paused: true })] }).stage, 'connected')
  assert.equal(at({ agents: [agent()] }).stage, 'first_scan')
  const fresh = aliased('studio', ['old-studio'], [group('claude', { scannedAt: now })])
  const ready = at({ agents: [agent()], machines: [fresh] })
  assert.deepEqual([ready.stage, ready.canUse], ['ready', true])
  // A machine with no groups at all behaves the same.
  const bare = aliased('studio', ['old-studio'], [])
  assert.equal(view({ name: 'old-studio', machines: [bare], agents: [], baseline: joinBaseline('old-studio', [], [bare]) }).stage, 'waiting')
  assert.equal(view({ name: 'old-studio', machines: [aliased('studio', ['old-studio'], [group('claude')])], agents: [agent()], baseline: null }).stage, 'ready')
})

test('joinState: a name that is not an alias is unchanged, with no alias wording', () => {
  const machines = [aliased('studio', ['old-studio'])]
  const v = view({ name: 'annexe', machines, agents: [], baseline: null })
  assert.equal(v.stage, 'waiting')
  assert.doesNotMatch(v.detail, /will appear as/)
  assert.equal(view({ name: 'studio', machines, agents: [agent()], baseline: null }).stage, 'ready')
  assert.deepEqual(joinBaseline('annexe', [agent()], machines), null)
})

test('alias handling never throws on odd machines', () => {
  for (const ms of [[{ host: 'studio', groups: [], aliases: 'old' }], [{ host: 'studio', groups: [], aliases: [null, 3] }]]) {
    assert.doesNotThrow(() => joinBaseline('old', [agent()], ms))
    assert.doesNotThrow(() => view({ name: 'old', machines: ms }))
  }
})

test('the Private skills context is told apart from the Machines page, and the landing note says only what was chosen', () => {
  assert.equal(JOIN_AUTO_MS, 3000)
  assert.equal(fromPrivateSkills({}), false)
  assert.equal(fromPrivateSkills({ skill: 'demo' }), true)
  assert.equal(fromPrivateSkills({ agent: 'claude' }), true)
  assert.equal(fromPrivateSkills({ bundleId: 'b' }), false)   // a bundle alone says nothing about where the person came from
  assert.equal(fromPrivateSkills({ agent: 'claude', bundleId: 'b' }), true)   // an agent alone is enough
  const open = (others) => ({ kind: 'open', environmentId: 'e', chosen: 'Claude Code', others })
  assert.equal(landingNote('rack', open(0)), 'Opened on Claude Code on rack.')
  assert.equal(landingNote('rack', open(1)), 'Opened on Claude Code on rack. 1 other folder also qualifies; open it from the Machines page.')
  assert.equal(landingNote('rack', open(3)), 'Opened on Claude Code on rack. 3 other folders also qualify; open them from the Machines page.')
  assert.doesNotMatch(landingNote('rack', open(2)), /verified|healthy/i)
})
