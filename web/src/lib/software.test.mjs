import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./software.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
})
const { batchCanAbandon, batchFailedMember, batchMemberAction, batchMemberLabel, batchMemberNote, batchStateLabel, fleetSoftwareCanApply, fleetSoftwareGuide, fleetSoftwareLabel, fleetSoftwarePress, fleetSoftwareSummary, fleetSoftwareWarning, softwareLine, softwarePlanActions } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const installed = { tool: 'opencode', state: 'installed', version: '1.19.5', manager: 'reet', package: 'opencode-ai', path: '/home/me/.reet/software/opencode/versions/1.19.5/opencode', platform: 'linux', arch: 'amd64', source: 'verified_tarball', managed: true, operations: ['plan_update', 'plan_remove'], checkedAt: 1 }
const missing = { tool: 'opencode', state: 'missing', package: 'opencode-ai', source: 'npm', platform: 'linux', arch: 'amd64', reason: 'not_installed', operations: ['plan_install'], checkedAt: 1 }

test('installed line names the manager, source and path', () => {
  assert.equal(softwareLine(installed), 'Managed by reet · verified 1.19.5 · /home/me/.reet/software/opencode/versions/1.19.5/opencode')
})

test('missing line names the reviewed package and source', () => {
  assert.equal(softwareLine(missing), 'Not installed. The reviewed package is opencode-ai from npm.')
})

test('unknown and unsupported installation lines are distinct', () => {
  assert.match(softwareLine({ ...installed, state: 'unknown', reason: 'unreadable' }), /not recognised/)
  assert.match(softwareLine({ ...installed, state: 'unsupported', reason: 'unsupported' }), /no reviewed maintenance plan/)
})

test('the plan actions follow the evidence and its operations', () => {
  assert.deepEqual(softwarePlanActions(installed), ['update', 'remove'])
  assert.deepEqual(softwarePlanActions({ ...installed, previous: '1.18.0' }), ['update', 'rollback', 'remove'])
  assert.deepEqual(softwarePlanActions(missing), ['install'])
  assert.deepEqual(softwarePlanActions({ ...installed, managed: false, state: 'installed', operations: ['plan_install'] }), ['install'])
  assert.deepEqual(softwarePlanActions({ ...installed, operations: [] }), [])
  assert.deepEqual(softwarePlanActions({ ...missing, operations: ['plan_update'] }), [])
  assert.deepEqual(softwarePlanActions({ ...installed, state: 'unknown', operations: ['plan_update'] }), [])
})

test('fleet review labels and warnings say what would happen', () => {
  const update = { state: 'update', version: '1.18.0', target: '1.19.5', sessionsRunning: 0, canApply: true }
  assert.equal(fleetSoftwareLabel(update), 'Update to 1.19.5')
  assert.equal(fleetSoftwareLabel({ state: 'install', version: '', target: '1.19.5' }), 'Install 1.19.5')
  assert.equal(fleetSoftwareLabel({ state: 'install', version: '1.18.0', target: '1.19.5' }), 'Install 1.19.5 alongside')
  assert.equal(fleetSoftwareLabel({ state: 'none', version: '1.19.5' }), 'Up to date')
  assert.equal(fleetSoftwareLabel({ state: 'unknown' }), 'Needs attention')
  assert.equal(fleetSoftwareWarning(update), '')
  assert.match(fleetSoftwareWarning({ ...update, sessionsRunning: 2 }), /2 OpenCode sessions are running/)
  assert.match(fleetSoftwareWarning({ ...update, sessionsRunning: 1 }), /1 OpenCode session is running/)
  assert.match(fleetSoftwareWarning({ ...update, canApply: false }), /Update reet/)
  assert.equal(fleetSoftwareWarning({ ...update, state: 'none', sessionsRunning: 2 }), '')
})

test('the fleet review only applies a ready single machine', () => {
  const update = { state: 'update', sessionsRunning: 0, canApply: true }
  assert.equal(fleetSoftwareCanApply(update), true)
  assert.equal(fleetSoftwareCanApply({ ...update, state: 'install' }), true)
  assert.equal(fleetSoftwareCanApply({ ...update, sessionsRunning: 2 }), false)
  assert.equal(fleetSoftwareCanApply({ ...update, canApply: false }), false)
  assert.equal(fleetSoftwareCanApply({ ...update, state: 'none' }), false)
  assert.equal(fleetSoftwareCanApply({ ...update, state: 'unknown' }), false)
})

test('batch members name their frozen action and state', () => {
  assert.equal(batchMemberAction({ action: 'update', target: '1.19.5' }), 'Update to 1.19.5')
  assert.equal(batchMemberAction({ action: 'install', target: '1.19.5' }), 'Install 1.19.5')
  assert.equal(batchMemberAction({ action: 'install', target: '1.19.5', version: '1.18.0' }), 'Install 1.19.5')
  assert.equal(batchMemberAction({}), '')
  assert.equal(batchMemberLabel({ state: 'applied', version: '1.19.5' }), 'Applied 1.19.5')
  assert.equal(batchMemberLabel({ state: 'applied' }), 'Applied')
  assert.equal(batchMemberLabel({ state: 'running' }), 'Applying')
  assert.equal(batchMemberLabel({ state: 'pending' }), 'Waiting')
  assert.equal(batchMemberLabel({ state: 'excluded' }), 'Not included')
})

test('batch notes explain codes and prefer an exclusion reason', () => {
  assert.match(batchMemberNote({ code: 'offline' }), /wasn't connected/)
  // A machine that was never reached says nothing was changed; one whose connection dropped mid-apply does not.
  assert.match(batchMemberNote({ code: 'offline' }), /nothing was changed/)
  assert.match(batchMemberNote({ code: 'connection_lost' }), /may or may not have finished/)
  assert.doesNotMatch(batchMemberNote({ code: 'connection_lost' }), /wasn't connected/)
  assert.match(batchMemberNote({ code: 'checksum_failed' }), /checksum/)
  assert.match(batchMemberNote({ code: 'in_another_batch' }), /another batch/)
  assert.equal(batchMemberNote({ code: 'offline', reason: 'That machine is already in another batch.' }), 'That machine is already in another batch.')
  assert.equal(batchMemberNote({}), '')
  assert.equal(batchMemberNote({ code: 'not_a_real_code' }), '')
})

test('batch state and abandon rules follow what is running', () => {
  assert.equal(batchStateLabel('running'), 'Applying')
  assert.equal(batchStateLabel('stopped'), 'Stopped on a failure')
  assert.equal(batchCanAbandon({ state: 'stopped', members: [{ state: 'failed' }, { state: 'pending' }] }), true)
  assert.equal(batchCanAbandon({ state: 'running', members: [{ state: 'running' }] }), false)
  assert.equal(batchCanAbandon({ state: 'done', members: [{ state: 'applied' }] }), false)
  assert.equal(batchCanAbandon({ state: 'abandoned', members: [] }), false)
  assert.equal(batchFailedMember({ members: [{ state: 'applied' }, { state: 'failed' }, { state: 'pending' }] })?.state, 'failed')
})

test('an attention row says why, what to run, and what to do next', () => {
  const row = reason => ({ state: 'unknown', host: 'Elitebook', reason })
  assert.equal(fleetSoftwareGuide({ state: 'none', host: 'a' }), null)
  assert.equal(fleetSoftwareGuide({ state: 'update', host: 'a' }), null)
  const old = fleetSoftwareGuide(row('agent_too_old'))
  assert.equal(old.title, 'Update reet on Elitebook')
  assert.equal(old.command, 'reet upgrade')
  assert.match(old.then, /press Refresh/)
  assert.equal(fleetSoftwareGuide(row('unreadable_install')).command, undefined)
  for (const r of ['agent_too_old', 'no_opencode_evidence', 'unreadable_install', 'no_plan', 'read_failed', undefined]) assert.doesNotMatch(JSON.stringify(fleetSoftwareGuide(row(r))), /doctor/)
  assert.equal(fleetSoftwareGuide(row('no_opencode_evidence')).command, 'reet upgrade')
  assert.equal(fleetSoftwareGuide(row('no_plan')).command, undefined)
  assert.equal(fleetSoftwareGuide(row('read_failed')).command, undefined)
  const fallback = fleetSoftwareGuide(row(undefined))
  assert.match(fallback.title, /Elitebook/)
  assert.ok(fallback.why && fallback.then)
})

test('an unreadable install says what the machine reported', () => {
  const row = (evidence, reason = 'unreadable_install') => ({ state: 'unknown', host: 'PROD_Snicket', reason, evidence })
  assert.equal(fleetSoftwareGuide(row(undefined)).found, undefined)
  assert.equal(fleetSoftwareGuide(row({ reason: 'unreadable', path: '/usr/bin/opencode' })).found, "reet found OpenCode at /usr/bin/opencode but couldn't read its version.")
  assert.match(fleetSoftwareGuide(row({ reason: 'package_unknown' })).found, /doesn't recognise the package/)
  assert.match(fleetSoftwareGuide(row({ reason: 'managed_broken' })).found, /integrity check/)
  assert.match(fleetSoftwareGuide(row({ reason: 'managed_broken' })).why, /installed OpenCode here itself/)
  assert.doesNotMatch(fleetSoftwareGuide(row({ reason: 'managed_broken' })).why, /could not tell how it was installed/)
  assert.match(fleetSoftwareGuide(row({ reason: 'unreadable' })).why, /could not tell how it was installed/)
  assert.match(fleetSoftwareGuide(row({ reason: 'managed_broken' })).then, /delete the folder ~\/\.reet\/software\/opencode/)
  assert.doesNotMatch(fleetSoftwareGuide(row({ reason: 'managed_broken' })).then, /Settings and reinstall/)
  assert.match(fleetSoftwareGuide(row({ reason: 'platform_unsupported' }, 'no_plan')).found, /no reviewed OpenCode build/)
  assert.equal(fleetSoftwareGuide(row({ reason: 'unreadable' })).command, undefined)
})

// One machine's stored plan, and one place in a batch.
const row = (host, state, extra = {}) => ({
  agentId: `agent-${host}`, host, state, sessionsRunning: 0, canApply: true, sentence: '',
  ...(state === 'update' ? { action: 'update', version: '1.18.0', target: '1.19.5' } : state === 'install' ? { action: 'install', target: '1.19.5' } : {}),
  ...extra,
})
const member = (host, state, extra = {}) => ({ agentId: `agent-${host}`, host, state, ...extra })
const batchOf = (state, ...members) => ({ id: 'b1', tool: 'opencode', state, createdAt: 1, updatedAt: 1, members: members.map((item, position) => ({ ...item, position })) })

test('machines that can update are one sentence and one button that names how many', () => {
  const summary = fleetSoftwareSummary([row('laptop', 'update'), row('build', 'update'), row('desk', 'none')], null)
  assert.equal(summary.state, 'ready')
  assert.equal(summary.headline, 'OpenCode can be updated on 2 machines.')
  assert.equal(summary.detail, 'laptop and build, to 1.19.5.')
  assert.equal(summary.button, 'Update 2 machines')
  assert.deepEqual(summary.ready, ['agent-laptop', 'agent-build'])
  const one = fleetSoftwareSummary([row('laptop', 'update')], null)
  assert.equal(one.headline, 'OpenCode can be updated on 1 machine.')
  assert.equal(one.button, 'Update 1 machine')
})

test('installing is named as installing, and a mix is not called only one of them', () => {
  const installs = fleetSoftwareSummary([row('a', 'install'), row('b', 'install')], null)
  assert.equal(installs.headline, 'OpenCode can be installed on 2 machines.')
  assert.equal(installs.button, 'Install on 2 machines')
  const mixed = fleetSoftwareSummary([row('a', 'install'), row('b', 'update'), row('c', 'update')], null)
  assert.equal(mixed.headline, 'OpenCode can be updated or installed on 3 machines.')
  assert.equal(mixed.button, 'Update 3 machines')
})

test('the version is named only when every machine is going to the same one, and long lists are counted', () => {
  const differing = fleetSoftwareSummary([row('a', 'update'), row('b', 'update', { target: '1.20.0' })], null)
  assert.equal(differing.detail, 'a and b.')
  const many = fleetSoftwareSummary(['a', 'b', 'c', 'd', 'e'].map(name => row(name, 'update')), null)
  assert.equal(many.detail, '5 machines, to 1.19.5.')
})

test('a machine that cannot be updated right now is left out of the button and named', () => {
  const summary = fleetSoftwareSummary([row('laptop', 'update'), row('busy', 'update', { sessionsRunning: 2 })], null)
  assert.equal(summary.state, 'ready')
  assert.deepEqual(summary.ready, ['agent-laptop'])
  assert.equal(summary.button, 'Update 1 machine')
  assert.match(summary.detail, /busy can't be updated right now\./)
  const old = fleetSoftwareSummary([row('laptop', 'update'), row('old', 'update', { canApply: false })], null)
  assert.deepEqual(old.ready, ['agent-laptop'])
})

test('a machine that needs a look is named without stopping the rest', () => {
  const summary = fleetSoftwareSummary([row('laptop', 'update'), row('odd', 'unknown', { reason: 'unreadable_install' })], null)
  assert.equal(summary.state, 'ready')
  assert.match(summary.detail, /odd needs a look\./)
})

test('with nothing to press, it says why in one sentence', () => {
  const blocked = fleetSoftwareSummary([row('busy', 'update', { sessionsRunning: 1 }), row('desk', 'none')], null)
  assert.equal(blocked.state, 'blocked')
  assert.equal(blocked.headline, "1 machine can't be updated right now.")
  assert.equal(blocked.button, '')
  assert.deepEqual(blocked.ready, [])
  const attention = fleetSoftwareSummary([row('odd', 'unknown'), row('other', 'unknown'), row('desk', 'none')], null)
  assert.equal(attention.state, 'attention')
  assert.equal(attention.headline, '2 machines need a look.')
  const current = fleetSoftwareSummary([row('a', 'none'), row('b', 'none')], null)
  assert.equal(current.state, 'current')
  assert.equal(current.headline, 'Everything is up to date.')
  assert.equal(current.button, '')
  assert.equal(fleetSoftwareSummary([], null).state, 'empty')
})

test('while a batch runs it says which machine and how far along', () => {
  const running = fleetSoftwareSummary([], batchOf('running', member('laptop', 'applied'), member('build', 'running'), member('desk', 'pending')))
  assert.equal(running.state, 'working')
  assert.equal(running.headline, 'Updating build…')
  assert.equal(running.detail, '1 of 3 done.')
  assert.equal(running.button, '')
  const waiting = fleetSoftwareSummary([], batchOf('pending', member('laptop', 'pending'), member('build', 'pending')))
  assert.equal(waiting.headline, 'Updating your machines…')
  assert.equal(waiting.detail, '0 of 2 done.')
  const withExcluded = fleetSoftwareSummary([], batchOf('running', member('a', 'running'), member('b', 'excluded')))
  assert.equal(withExcluded.detail, '0 of 1 done.')
})

test('a batch that stopped names the machine and what happened on it', () => {
  const stopped = fleetSoftwareSummary([], batchOf('stopped', member('laptop', 'applied'), member('build', 'failed', { code: 'download_failed' }), member('desk', 'pending')))
  assert.equal(stopped.state, 'stopped')
  assert.equal(stopped.headline, 'Stopped on build.')
  assert.equal(stopped.detail, "The machine couldn't download the reviewed artifact. Nothing was changed.")
  assert.equal(fleetSoftwareSummary([], batchOf('stopped', member('build', 'failed'))).detail, 'Retry it, skip it and carry on, or stop here.')
})

test('a finished batch says what was updated and what was skipped', () => {
  assert.equal(fleetSoftwareSummary([], batchOf('done', member('a', 'applied'), member('b', 'applied'))).headline, 'Updated 2 machines.')
  assert.equal(fleetSoftwareSummary([], batchOf('done', member('a', 'applied'))).headline, 'Updated 1 machine.')
  const skipped = fleetSoftwareSummary([], batchOf('done', member('a', 'applied'), member('desk', 'skipped', { code: 'offline' })))
  assert.equal(skipped.state, 'finished')
  assert.equal(skipped.headline, 'Updated 1 machine.')
  assert.equal(skipped.detail, 'desk was skipped.')
  const two = fleetSoftwareSummary([], batchOf('done', member('a', 'applied'), member('desk', 'skipped'), member('mini', 'skipped')))
  assert.equal(two.detail, 'desk and mini were skipped.')
  const none = fleetSoftwareSummary([], batchOf('done', member('desk', 'skipped')))
  assert.equal(none.headline, 'Nothing was updated.')
  assert.equal(none.detail, 'desk was skipped.')
  assert.equal(fleetSoftwareSummary([], batchOf('abandoned', member('a', 'pending'))).headline, 'The batch was abandoned.')
})

test('once a batch has ended, whatever can still update is one press away again', () => {
  // The machine that was skipped is still behind, so the same button is offered without starting over first.
  const again = fleetSoftwareSummary([row('desk', 'update'), row('laptop', 'none')], batchOf('done', member('laptop', 'applied'), member('desk', 'skipped', { code: 'offline' })))
  assert.equal(again.state, 'finished')
  assert.equal(again.headline, 'Updated 1 machine.')
  assert.equal(again.detail, 'desk was skipped.')
  assert.equal(again.button, 'Update 1 machine')
  assert.deepEqual(again.ready, ['agent-desk'])
  assert.equal(fleetSoftwareSummary([row('desk', 'update')], batchOf('abandoned', member('desk', 'pending'))).button, 'Update 1 machine')
  // Nothing left to do: no button.
  const finished = fleetSoftwareSummary([row('desk', 'none'), row('laptop', 'none')], batchOf('done', member('laptop', 'applied'), member('desk', 'applied')))
  assert.equal(finished.button, '')
  assert.deepEqual(finished.ready, [])
  // While a batch is still going, there is never a second button.
  assert.equal(fleetSoftwareSummary([row('desk', 'update')], batchOf('running', member('laptop', 'running'))).button, '')
  assert.equal(fleetSoftwareSummary([row('desk', 'update')], batchOf('stopped', member('laptop', 'failed'))).button, '')
})

test('the question asked before an update names the same press as the button it came from', () => {
  assert.deepEqual(fleetSoftwarePress([row('a', 'update'), row('b', 'update')]), { button: 'Update 2 machines', ready: ['agent-a', 'agent-b'] })
  assert.equal(fleetSoftwarePress([row('a', 'install')]).button, 'Install on 1 machine')
  assert.equal(fleetSoftwarePress([row('a', 'install'), row('b', 'update'), row('c', 'update')]).button, 'Update 3 machines')
  // Nothing chosen, nothing to ask.
  assert.deepEqual(fleetSoftwarePress([]), { button: '', ready: [] })
  // The summary's own button is the same one.
  const summary = fleetSoftwareSummary([row('a', 'update'), row('b', 'install')], null)
  assert.equal(summary.button, fleetSoftwarePress([row('a', 'update'), row('b', 'install')]).button)
})

test('the plain sentences stay plain: no internal words', () => {
  const all = [
    fleetSoftwareSummary([row('a', 'update'), row('busy', 'update', { sessionsRunning: 1 }), row('odd', 'unknown')], null),
    fleetSoftwareSummary([row('a', 'none')], null), fleetSoftwareSummary([row('a', 'unknown')], null),
    fleetSoftwareSummary([], batchOf('running', member('a', 'running'))), fleetSoftwareSummary([], batchOf('done', member('a', 'applied'), member('b', 'skipped'))),
  ]
  for (const summary of all) assert.ok(!/batch member|agentId|position|excluded|evidence|plan\b/i.test(`${summary.headline} ${summary.detail} ${summary.button}`), `${summary.headline} | ${summary.detail}`)
})
