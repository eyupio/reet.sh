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
const ctx = (over = {}) => ({ host: 'studio', dir: '~/PROD_reet', agent: 'Claude Code', perMachine: 24, perAccount: 64, accountLimit: false, canContinue: false, ...over })

test('the state of each kind of pane', () => {
  const cases = [
    ['a running shell on a machine that reports', [pane(), session(), online, fine], 'live'],
    ['the machine is offline', [pane(), session(), { online: false, deskShell: true }, fine], 'reconnecting'],
    ['the attachment dropped', [pane(), session(), online, { conflict: false, down: true }], 'reconnecting'],
    ['open in another window', [pane(), session(), online, { conflict: true, down: false }], 'elsewhere'],
    ['an ended shell', [pane(), session({ state: 'exited', exitCode: 0 }), online, fine], 'ended'],
    ['a session no longer listed', [pane(), undefined, online, fine], 'ended'],
    ['a lost shell', [pane(), session({ state: 'lost' }), online, fine], 'lost'],
    ['an ended agent', [pane({ kind: 'agent' }), session({ state: 'exited', tool: 'claude' }), online, fine], 'agentEnded'],
    ['an agent no longer listed', [pane({ kind: 'agent' }), undefined, online, fine], 'agentEnded'],
    ['a reopen found the folder gone', [pane(), session({ state: 'exited' }), online, fine, 'folderGone'], 'folderGone'],
    ['a reopen could not open a shell in the folder', [pane(), session({ state: 'lost' }), online, fine, 'folderUnavailable'], 'folderUnavailable'],
    ['the machine is at its limit', [pane(), session({ state: 'exited' }), online, fine, 'machineLimit'], 'limit'],
    ['the account is at its limit', [pane(), session({ state: 'exited' }), online, fine, 'accountLimit'], 'limit'],
    ['a reopen found the machine offline', [pane(), session({ state: 'lost' }), online, fine, 'offline'], 'reconnecting'],
    ['the machine was removed', [pane(), session(), undefined, fine], 'machineGone'],
    ['a reopen found the machine removed', [pane(), session({ state: 'lost' }), online, fine, 'machineGone'], 'machineGone'],
    ['a pane never started', [pane({ session: '' }), undefined, online, fine], 'empty'],
    ['an ended shell on a machine without desk shells is still ended', [pane(), session({ state: 'exited' }), { online: true, deskShell: false }, fine], 'ended'],
    ['a live pane on an older machine is still live', [pane(), session(), { online: true, deskShell: false }, fine], 'live'],
  ]
  for (const [name, args, want] of cases) assert.equal(p.paneState(...args), want, name)
})

// One row per card: the tag, the heading and the main action, read from the table the component draws from.
test('each card has its tag, heading and main action', () => {
  const rows = [
    ['lost', ctx(), 'Lost', 'studio restarted, so this shell was lost.', "It was in ~/PROD_reet. A new shell opens there with this pane's command history.", ['Reopen in ~/PROD_reet', 'Close pane']],
    ['ended', ctx(), 'Ended', 'This shell ended.', "It was in ~/PROD_reet. A new shell opens there with this pane's command history.", ['Reopen in ~/PROD_reet', 'Close pane']],
    ['folderGone', ctx({ dir: '~/old' }), 'Folder gone', '~/old no longer exists on studio.', "reet won't quietly open somewhere else.", ['Choose a folder', 'Reopen in home']],
    ['folderUnavailable', ctx({ dir: '~/old' }), 'Folder unavailable', "reet couldn't open a shell in ~/old on studio.", "reet won't quietly open somewhere else.", ['Choose a folder', 'Reopen in home']],
    ['limit', ctx(), 'At the limit', 'studio already has 24 sessions running.', '', ['Try again']],
    ['limit', ctx({ accountLimit: true }), 'At the limit', 'You already have 64 sessions running.', '', ['Try again']],
    ['machineGone', ctx(), 'Machine removed', 'This machine was removed from reet.', '', ['Choose a machine', 'Close pane']],
    ['reconnecting', ctx(), 'Machine offline', 'studio is not reporting.', 'The shell may still be running there. reet keeps trying; nothing you typed is resent.', ['Retry now']],
    ['elsewhere', ctx(), 'Open elsewhere', 'This terminal is open in another window.', 'A terminal has one keyboard at a time.', ['Use it here']],
    ['agentEnded', ctx({ canContinue: true, dir: '~/app' }), 'Agent ended', 'Claude Code finished in this pane.', 'Continue the same conversation, or start a plain shell in ~/app.', ['Continue', 'Start a shell here']],
    ['agentEnded', ctx({ dir: '~/app' }), 'Agent ended', 'Claude Code finished in this pane.', 'Continue the same conversation, or start a plain shell in ~/app.', ['Start a shell here']],
    ['empty', ctx(), 'Not started', 'studio · ~/PROD_reet', '', ['Start']],
  ]
  for (const [state, c, tag, heading, body, actions] of rows) {
    const card = p.paneCard(state, c)
    assert.equal(card.tag, tag, state)
    assert.equal(card.tag, p.PANE_CARDS[state].tag, state)
    assert.equal(card.heading, heading, state)
    assert.equal(card.body, body, state)
    assert.deepEqual(card.actions.map(a => a.label), actions, state)
  }
  // Every state but live has a card, and the rows above cover them all.
  assert.deepEqual(Object.keys(p.PANE_CARDS).sort(), [...new Set(rows.map(r => r[0]))].sort())
  assert.equal(p.paneCard('live', ctx()), null)
})

test('the actions say what they do', () => {
  assert.deepEqual(p.paneCard('folderGone', ctx()).actions.map(a => a.kind), ['chooseFolder', 'reopenHome'])
  assert.deepEqual(p.paneCard('lost', ctx()).actions.map(a => a.kind), ['reopen', 'close'])
  assert.deepEqual(p.paneCard('limit', ctx()).actions.map(a => a.kind), ['again'])
  assert.deepEqual(p.paneCard('empty', ctx()).actions.map(a => a.kind), ['start'])
})

test('a machine with no name is that machine', () => {
  assert.equal(p.paneCard('lost', ctx({ host: '' })).heading, 'That machine restarted, so this shell was lost.')
})

test('an older machine gets a note under a live shell, not a card', () => {
  const note = "Update reet on that machine to keep this pane's folder and history."
  assert.equal(p.paneNote(pane(), 'live', { deskShell: false }), note)
  assert.equal(p.paneNote(pane(), 'live', { deskShell: true }), '')
  assert.equal(p.paneNote(pane(), 'live', {}), '')
  assert.equal(p.paneNote(pane({ kind: 'agent' }), 'live', { deskShell: false }), '')
  assert.equal(p.paneNote(pane(), 'ended', { deskShell: false }), '')
})

test('only lost and ended shells are reopened together', () => {
  assert.deepEqual(['live', 'reconnecting', 'elsewhere', 'ended', 'lost', 'agentEnded', 'folderGone', 'folderUnavailable', 'limit', 'machineGone', 'empty'].filter(p.reopenable), ['ended', 'lost'])
})

test('a folder is shown from home', () => {
  assert.equal(p.dirLabel('/home/dev/PROD_reet'), '~/PROD_reet')
  assert.equal(p.dirLabel(''), '~')
  assert.equal(p.dirLabel('/root'), '~')
  assert.equal(p.dirLabel('/srv/app'), '/srv/app')
})

test('agent names', () => {
  assert.equal(p.agentName('claude'), 'Claude Code')
  assert.equal(p.agentName(undefined), 'The agent')
  assert.equal(p.agentName('other'), 'other')
})
