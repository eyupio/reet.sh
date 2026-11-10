import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const transpile = (file) => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const dataURL = (source) => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const machinesURL = dataURL(transpile('./machines.ts'))
const overviewURL = dataURL(transpile('./syncOverview.ts'))
const { fleetSyncStatus, fleetSyncSentence, machineSyncRow } = await import(dataURL(transpile('./fleetSync.ts')
  .replace("from './machines'", `from '${machinesURL}'`)
  .replace("from './syncOverview'", `from '${overviewURL}'`)))

const now = Date.now()
const OFFLINE = now - 10 * 60_000
// A machine that has been the way it is for a while.
const STALE = now - 10 * 60_000

// One machine's slice of the sync state: an agent, its scanned folder, and its plan.
function host(name, options = {}) {
  const { online = true, paused = false, keepSynced = true, scanned = true, planned = true, missing = 0, problem, running = false, finishedAgo, firstSeen = STALE } = options
  const id = `${name}-scope`
  const skills = Array.from({ length: missing }, (_, i) => ({ id: `s${i}`, name: `s${i}`, slug: `s${i}`, source: 'org/repo', sourceType: 'github' }))
  const group = { id, name: id, agent: 'claude', host: name, firstSeen, lastSeen: now, scannedAt: now, complete: true, skills: [] }
  return {
    agent: { id: `agent-${name}`, host: name, lastSeen: online ? now : OFFLINE, paused, keepSynced },
    machine: scanned ? { host: name, groups: [group], skillCount: 1 } : undefined,
    environment: scanned ? group : undefined,
    target: scanned && planned ? { target: { id, host: name, agent: 'claude' }, sourceId: 'library', sourceName: 'Library', desired: [], revision: 'r1', appliedRevision: 'r1', updatedAt: STALE, mode: 'additive', missing: skills, extra: [], ...(problem ? { problem } : {}) } : undefined,
    runs: running ? [{ id: `run-${name}`, agentId: `agent-${name}`, targets: [id], cursor: 0, status: 'running' }] : [],
    // An attempt that finished this long ago: the machine has just completed one of its folders.
    jobs: finishedAgo === undefined ? [] : [{ id: `job-${name}`, agentId: `agent-${name}`, targetId: id, kind: 'auto', status: 'succeeded', revision: 'r0', hash: '', plan: {}, message: '', createdAt: now - finishedAgo, updatedAt: now - finishedAgo }],
  }
}
const fleet = (...hosts) => ({
  agents: hosts.map(h => h.agent),
  machines: hosts.map(h => h.machine).filter(Boolean),
  environments: hosts.map(h => h.environment).filter(Boolean),
  targets: hosts.map(h => h.target).filter(Boolean),
  runs: hosts.flatMap(h => h.runs),
  jobs: hosts.flatMap(h => h.jobs),
})

test('no machines yet: the one thing to do is connect one', () => {
  const status = fleetSyncStatus({ agents: [], machines: [], environments: [], targets: [], runs: [], jobs: [] }, now)
  assert.equal(status.state, 'none')
  assert.equal(status.headline, 'Connect your first machine.')
  assert.equal(status.canSync, false)
  assert.equal(status.machines, 0)
})

test('every connected machine is up to date', () => {
  const status = fleetSyncStatus(fleet(host('laptop'), host('desktop')), now)
  assert.equal(status.state, 'in-sync')
  assert.equal(status.headline, 'Everything is in sync.')
  assert.equal(status.detail, '')
  assert.equal(status.canSync, true, 'pressing Sync again is always allowed, it just checks')
  assert.equal(status.inSync, 2)
})

test('a machine that is missing skills means the button is the next step, and says how many machines', () => {
  assert.equal(fleetSyncStatus(fleet(host('laptop', { missing: 3 }), host('desktop')), now).headline, '1 machine needs syncing.')
  const two = fleetSyncStatus(fleet(host('laptop', { missing: 3 }), host('desktop', { missing: 1 })), now)
  assert.equal(two.state, 'behind')
  assert.equal(two.headline, '2 machines need syncing.')
  assert.equal(two.behind, 2)
  assert.equal(two.canSync, true)
})

test('automatic sync being off is something the button fixes, even on a machine that looks current', () => {
  const status = fleetSyncStatus(fleet(host('laptop'), host('desktop', { keepSynced: false })), now)
  assert.equal(status.state, 'behind')
  assert.equal(status.headline, '1 machine needs syncing.')
})

test('an offline machine is not behind and not an error: it catches up by itself', () => {
  const one = fleetSyncStatus(fleet(host('laptop'), host('thinkpad', { online: false })), now)
  assert.equal(one.state, 'in-sync')
  assert.equal(one.headline, 'Everything is in sync.')
  assert.equal(one.detail, "thinkpad will catch up when it's back online.")
  const two = fleetSyncStatus(fleet(host('laptop'), host('thinkpad', { online: false }), host('elitebook', { online: false })), now)
  assert.equal(two.detail, "thinkpad and elitebook will catch up when they're back online.")
  const many = fleetSyncStatus(fleet(host('a'), host('b', { online: false }), host('c', { online: false }), host('d', { online: false })), now)
  assert.equal(many.detail, "3 machines will catch up when they're back online.")
})

test('with every machine offline there is nothing to say is in sync, only that they will sync later', () => {
  const status = fleetSyncStatus(fleet(host('thinkpad', { online: false }), host('elitebook', { online: false })), now)
  assert.equal(status.state, 'waiting')
  assert.equal(status.headline, "Your machines will sync when they're back online.")
  assert.equal(status.detail, 'Offline right now: thinkpad and elitebook.')
  assert.equal(status.canSync, true)
})

test('a paused machine is left alone and mentioned, never counted as behind', () => {
  const status = fleetSyncStatus(fleet(host('laptop'), host('build-box', { paused: true, missing: 5 })), now)
  assert.equal(status.state, 'in-sync')
  assert.equal(status.detail, 'build-box is paused.')
  assert.equal(status.paused, 1)
  const two = fleetSyncStatus(fleet(host('laptop'), host('a', { paused: true }), host('b', { paused: true })), now)
  assert.equal(two.detail, '2 machines are paused.')
})

test('while anything is syncing the headline says so, the button waits, and the rest are summarised', () => {
  const status = fleetSyncStatus(fleet(host('laptop', { running: true, missing: 2 }), host('desktop', { missing: 1 }), host('thinkpad', { online: false })), now)
  assert.equal(status.state, 'syncing')
  assert.equal(status.headline, 'Syncing your machines…')
  assert.equal(status.canSync, false)
  assert.equal(status.syncing, 1)
})

test('a machine that has not reported its first scan is being set up, not behind', () => {
  const status = fleetSyncStatus(fleet(host('laptop'), host('new-box', { scanned: false })), now)
  assert.equal(status.state, 'syncing')
  assert.equal(status.headline, 'Syncing your machines…')
})

test('a real problem is the only thing that reads as needing a look, in the machine\'s own words', () => {
  const status = fleetSyncStatus(fleet(host('laptop'), host('elitebook', { problem: 'Sync source is unavailable: source needs a recent complete scan' })), now)
  assert.equal(status.state, 'attention')
  assert.equal(status.headline, '1 machine needs a look.')
  assert.equal(status.detail, 'elitebook: Sync source is unavailable: source needs a recent complete scan')
  assert.equal(status.canSync, true)
  // Something behind outranks it: pressing the button is still the first move.
  const both = fleetSyncStatus(fleet(host('laptop', { missing: 1 }), host('elitebook', { problem: 'Sync source is unavailable: x' })), now)
  assert.equal(both.state, 'behind')
  assert.match(both.detail, /elitebook needs a look/)
})

test('the copy stays plain: no internal words reach the sentence', () => {
  const states = [
    fleetSyncStatus(fleet(host('a')), now), fleetSyncStatus(fleet(host('a', { missing: 1 })), now),
    fleetSyncStatus(fleet(host('a', { online: false })), now), fleetSyncStatus(fleet(host('a', { running: true })), now),
    fleetSyncStatus(fleet(host('a', { problem: 'Sync source is unavailable: x' })), now), fleetSyncStatus(fleet(host('a', { paused: true }), host('b')), now),
  ]
  for (const status of states) {
    assert.ok(!/target|revision|scope|desired|baseline|plan|job|run\b/i.test(`${status.headline} ${status.detail.replace(/^[^:]+: /, '')}`), `${status.headline} | ${status.detail}`)
    assert.ok(!/error|failed|fail\b|problem/i.test(status.headline), status.headline)
  }
})

test('with every machine paused there is nothing the button can do, and it says so', () => {
  const status = fleetSyncStatus(fleet(host('a', { paused: true }), host('b', { paused: true })), now)
  assert.equal(status.state, 'paused')
  assert.equal(status.headline, 'Sync is paused on all your machines.')
  assert.equal(status.canSync, false)
})

test('the order of machines in the answer does not change the sentence', () => {
  const a = host('laptop'), b = host('thinkpad', { online: false }), c = host('desktop', { missing: 2 })
  const forwards = fleetSyncStatus(fleet(a, b, c), now), backwards = fleetSyncStatus(fleet(c, b, a), now)
  assert.equal(forwards.headline, backwards.headline)
  assert.equal(forwards.state, backwards.state)
})

test('automatic sync off outranks offline: pressing the button turns it on wherever the machine is', () => {
  const status = fleetSyncStatus(fleet(host('laptop'), host('thinkpad', { online: false, keepSynced: false })), now)
  assert.equal(status.state, 'behind')
  assert.equal(status.headline, '1 machine needs syncing.')
})

test('a machine that has scanned but has no plan yet is being set up, not behind', () => {
  // The shared library prepares each machine's folders on its own after the first complete scan.
  const status = fleetSyncStatus(fleet(host('laptop'), host('fresh', { planned: false, firstSeen: now - 5_000 })), now)
  assert.equal(status.state, 'syncing')
  assert.equal(status.headline, 'Syncing your machines…')
})

test('between two folders a machine is still syncing: it picks up the next one a few seconds later', () => {
  // A machine with several folders finishes one and starts the next at its next check-in. That gap is part of syncing.
  const between = fleetSyncStatus(fleet(host('laptop', { missing: 2, finishedAgo: 4_000 })), now)
  assert.equal(between.state, 'syncing')
  assert.equal(between.headline, 'Syncing your machines…')
  // The same machine, left behind for a while, is one the button is for.
  assert.equal(fleetSyncStatus(fleet(host('laptop', { missing: 2, finishedAgo: 5 * 60_000 })), now).state, 'behind')
  assert.equal(fleetSyncStatus(fleet(host('laptop', { missing: 2 })), now).state, 'behind')
})

test('automatic sync being off is never "between folders": nothing will start by itself', () => {
  const status = fleetSyncStatus(fleet(host('laptop', { missing: 2, finishedAgo: 4_000, keepSynced: false })), now)
  assert.equal(status.state, 'behind')
})

test('the sentence for a message is what is true now followed by what will follow', () => {
  assert.equal(fleetSyncSentence(fleetSyncStatus(fleet(host('a')), now)), 'Everything is in sync.')
  assert.equal(fleetSyncSentence(fleetSyncStatus(fleet(host('a'), host('b', { online: false })), now)), "Everything is in sync. b will catch up when it's back online.")
  assert.equal(fleetSyncSentence(fleetSyncStatus(fleet(host('a', { running: true })), now)), 'Syncing your machines…')
  assert.equal(fleetSyncSentence(fleetSyncStatus(fleet(host('a', { online: false })), now)), "Your machines will sync when they're back online. Offline right now: a.")
})

test('what the library adds is part of the one sentence, after the machines', () => {
  const notes = { copying: 'Copying seo privately from your machines.', leftOut: '2 skills are left out for now.' }
  const status = fleetSyncStatus(fleet(host('a'), host('b', { online: false })), now, notes)
  assert.equal(status.headline, 'Everything is in sync.')
  assert.equal(status.detail, "b will catch up when it's back online. Copying seo privately from your machines. 2 skills are left out for now.")
  // Nothing to add leaves the sentence as it was.
  assert.equal(fleetSyncStatus(fleet(host('a')), now, { copying: '', leftOut: '' }).detail, '')
})

test('a machine row says what the sentence says about that machine', () => {
  const row = (h) => machineSyncRow(h.agent, fleet(h), now)
  assert.deepEqual(row(host('a')), { host: 'a', online: true, label: 'Everything in sync', detail: 'Last checked 10 min ago', tone: 'synced' })
  assert.equal(row(host('a', { paused: true })).label, 'Paused')
  assert.equal(row(host('a', { keepSynced: false })).label, 'Sync off')
  assert.equal(row(host('a', { keepSynced: false })).tone, 'attention')
  const offline = row(host('a', { online: false }))
  assert.equal(offline.label, 'Waiting for machine')
  assert.equal(offline.online, false)
  assert.equal(row(host('a', { scanned: false })).detail, 'Connected · waiting for the first scan')
  assert.equal(row(host('a', { missing: 2 })).label, '2 skills to install')
  assert.equal(row(host('a', { problem: 'Two skills share a name.' })).tone, 'attention')
  assert.equal(row(host('a', { problem: 'Two skills share a name.' })).detail, 'Two skills share a name.')
})

test('a machine first seen a moment ago with nothing planned is being set up, in the sentence and on its row', () => {
  const fresh = host('a', { planned: false, firstSeen: now - 5_000 })
  assert.equal(fleetSyncStatus(fleet(fresh), now).headline, 'Syncing your machines…')
  const row = machineSyncRow(fresh.agent, fleet(fresh), now)
  assert.equal(row.label, 'Setting up')
  assert.equal(row.tone, 'working')
})

test('a settled machine with nothing planned has no skills chosen: nothing is syncing and the sentence says what to do', () => {
  const unplanned = host('a', { planned: false })
  const status = fleetSyncStatus(fleet(unplanned), now)
  assert.equal(status.state, 'unchosen')
  assert.equal(status.headline, 'No skills chosen for a yet.')
  assert.equal(status.detail, 'Open Skills on this machine to choose them.')
  assert.doesNotMatch(fleetSyncSentence(status), /Syncing/)
  const row = machineSyncRow(unplanned.agent, fleet(unplanned), now)
  assert.equal(row.label, 'No skills chosen')
  assert.equal(row.tone, 'attention')
  // Another machine that is really syncing still leads.
  assert.equal(fleetSyncStatus(fleet(unplanned, host('b', { running: true })), now).headline, 'Syncing your machines…')
  assert.equal(fleetSyncStatus(fleet(host('a', { planned: false }), host('b', { planned: false })), now).headline, '2 machines have no skills chosen yet.')
})

test('a review waiting or a finished update is said on the machine row', () => {
  const a = host('a')
  const job = (kind, status, ago) => ({ id: `${kind}-${status}-${ago}`, agentId: a.agent.id, targetId: kind === 'update' ? '' : 'a-scope', kind, status, revision: 'r1', hash: '', plan: {}, message: '', createdAt: now - ago, updatedAt: now - ago })
  const withJobs = (...jobs) => machineSyncRow(a.agent, { ...fleet(a), jobs }, now)
  assert.equal(withJobs().note, undefined)
  assert.deepEqual(withJobs(job('preview', 'awaiting-approval', 60_000)).note, { text: '1 plan is waiting for your review.', action: 'Review changes', href: '#sync-jobs' })
  assert.equal(withJobs(job('preview', 'awaiting-approval', 60_000)).label, 'Review needed')
  assert.deepEqual(withJobs(job('update', 'succeeded', 3 * 60_000)).note, { text: 'Update complete · 3 min ago' })
  // Once something newer happened on the machine, the update is history and is not repeated on the row.
  assert.equal(withJobs(job('update', 'succeeded', 3 * 60_000), job('auto', 'succeeded', 60_000)).note, undefined)
  // Another machine's update is not this machine's news.
  assert.equal(machineSyncRow(a.agent, { ...fleet(a), jobs: [{ ...job('update', 'succeeded', 1000), agentId: 'someone-else' }] }, now).note, undefined)
})
