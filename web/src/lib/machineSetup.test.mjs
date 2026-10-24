import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const transpile = (file) => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const machinesURL = `data:text/javascript;base64,${Buffer.from(transpile('./machines.ts')).toString('base64')}`
const setupSource = transpile('./machineSetup.ts').replace("from './machines'", `from '${machinesURL}'`)
const {
  enrollCommands, machineStatus, profileFetchCommand, profilePullCommand,
  machineLink, scanCommand, shellQuote, supportedAgents, syncLink,
} = await import(`data:text/javascript;base64,${Buffer.from(setupSource).toString('base64')}`)

const now = Date.now()
const group = (id, agent, changes = {}) => ({ id, agent, skills: [], complete: true, scannedAt: now, ...changes })
const machine = (groups) => ({ host: 'studio', groups, skillCount: 0 })

test('machine status treats complete, recent reports as current', () => {
  assert.equal(machineStatus(machine([group('claude', 'claude'), group('agents', 'agents')]), now), 'current')
  assert.equal(machineStatus(machine([group('claude', 'claude', { scannedAt: now - 15 * 86400000 })]), now), 'stale')
  assert.equal(machineStatus(machine([group('claude', 'claude', { complete: false })]), now), 'partial')
  assert.equal(machineStatus(machine([group('server', 'reet-server')]), now), 'empty')
})

test('supported agent scopes are explicit and scan-only scopes stay separate', () => {
  const result = supportedAgents(machine([
    group('claude', 'claude'), group('codex', 'codex'), group('cursor', 'cursor'),
    group('opencode', 'opencode'), group('agents', 'agents'), group('server', 'reet-server'),
  ]))
  assert.deepEqual(result.map((item) => item.agent), ['claude', 'cursor', 'opencode', 'agents'])
})

test('commands are shell-quoted and machine-specific', () => {
  assert.equal(shellQuote("Sam's laptop"), "'Sam'\\''s laptop'")
  assert.match(scanCommand('https://reet.sh', "Sam's laptop"), /--env 'Sam'\\''s laptop' --prune --reconcile/)
  assert.match(enrollCommands('https://reet.sh', 'studio', true), /--allow-apply\nreet agent run/)
  assert.match(profilePullCommand('https://reet.sh', 'work'), /reet setup profile pull 'work' --server 'https:\/\/reet.sh'/)
  assert.match(profileFetchCommand('work', 4), /--profile 'work' --revision 4/)
})

test('machine links open the focused machine view', () => {
  assert.equal(machineLink('PROD_Snicket'), '/machines?machine=PROD_Snicket')
  assert.match(machineLink('studio / west'), /machine=studio%20%2F%20west/)
})

test('machine sync links preselect the machine without inventing an agent target', () => {
  assert.equal(syncLink('PROD_Snicket'), '/machines?view=compare&machine=PROD_Snicket')
  assert.match(syncLink('studio / west'), /machine=studio%20%2F%20west/)
})
