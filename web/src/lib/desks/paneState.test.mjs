import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./paneState.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const pane = (over = {}) => ({ id: 'p1', name: 'reet', colour: '', host: 'm1', session: 's1', kind: 'shell', dir: '~/PROD_reet', dirAt: 0, start: '', ...over })
const session = (over = {}) => ({ id: 's1', agentId: 'm1', title: 'reet', state: 'running', createdAt: 0, updatedAt: 0, exitCode: null, ...over })
const online = { online: true, deskShell: true }
const fine = { conflict: false, down: false }
/** The state and the label of its first action. */
const check = (args, state, first) => {
  const got = p.paneState(...args)
  assert.equal(got, state)
  const [, s] = args
  assert.equal(p.paneAction(got, args[0], s)[0]?.label, first)
}

test('live: a running shell on a machine that reports', () => check([pane(), session(), online, fine], 'live', undefined))
test('reconnecting: the machine is offline', () => check([pane(), session(), { online: false, deskShell: true }, fine], 'reconnecting', 'Retry now'))
test('reconnecting: the attachment dropped', () => check([pane(), session(), online, { conflict: false, down: true }], 'reconnecting', 'Retry now'))
test('elsewhere: open in another window', () => check([pane(), session(), online, { conflict: true, down: false }], 'elsewhere', 'Use it here'))
test('ended shell offers to reopen in its folder', () => check([pane(), session({ state: 'exited', exitCode: 0 }), online, fine], 'ended', 'Reopen in ~/PROD_reet'))
test('ended: a session no longer listed is ended', () => check([pane(), undefined, online, fine], 'ended', 'Reopen in ~/PROD_reet'))
test('lost shell offers to reopen in its folder', () => check([pane(), session({ state: 'lost' }), online, fine], 'lost', 'Reopen in ~/PROD_reet'))
test('ended agent offers Continue', () => check([pane({ kind: 'agent' }), session({ state: 'exited', tool: 'claude' }), online, fine], 'ended', 'Continue'))
test('ended agent without a session to resume starts a shell', () => {
  const state = p.paneState(pane({ kind: 'agent' }), undefined, online, fine)
  assert.equal(state, 'ended')
  assert.equal(p.paneAction(state, pane({ kind: 'agent' }))[0].label, 'Start a shell here')
})
test('folder gone: choose a folder', () => check([pane(), session({ state: 'exited' }), online, fine, 'folderGone'], 'folderGone', 'Choose a folder'))
test('at the limit: try again', () => check([pane(), session({ state: 'exited' }), online, fine, 'limit'], 'limit', 'Try again'))
test('machine removed: choose a machine', () => check([pane(), session(), undefined, fine], 'machineGone', 'Choose a machine'))
test('empty pane: start', () => check([pane({ session: '' }), undefined, online, fine], 'empty', 'Start'))
test('update: a machine that says it has no desk shell', () => check([pane(), session({ state: 'exited' }), { online: true, deskShell: false }, fine], 'update', 'Reopen in ~/PROD_reet'))
test('a machine that does not say (before desk-shell-v1 is reported) is not asked to update', () => {
  assert.equal(p.paneState(pane(), session({ state: 'exited' }), { online: true }, fine), 'ended')
})
test('a live pane on an older machine is still live', () => {
  assert.equal(p.paneState(pane(), session(), { online: true, deskShell: false }, fine), 'live')
})
test('a folder is shown from home', () => {
  assert.equal(p.paneAction('lost', pane({ dir: '/home/dev/PROD_reet' }))[0].label, 'Reopen in ~/PROD_reet')
  assert.equal(p.paneAction('lost', pane({ dir: '' }))[0].label, 'Reopen in ~')
  assert.equal(p.paneAction('lost', pane({ dir: '/srv/app' }))[0].label, 'Reopen in /srv/app')
})
test('each state has a sentence', () => {
  for (const s of ['reconnecting', 'elsewhere', 'ended', 'lost', 'folderGone', 'limit', 'machineGone', 'empty', 'update']) {
    assert.ok(p.paneCopy(s, pane(), undefined, 'studio').title, s)
  }
  assert.equal(p.paneCopy('lost', pane(), undefined, 'studio').title, 'studio restarted, so this shell was lost.')
  assert.equal(p.paneCopy('limit', pane(), undefined, 'studio').title, 'studio already has 24 sessions running.')
})
