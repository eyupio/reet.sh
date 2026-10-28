import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const transpile = (file) => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const dataURL = (source) => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const machinesURL = dataURL(transpile('./machines.ts'))
const setupURL = dataURL(transpile('./machineSetup.ts').replace("from './machines'", `from '${machinesURL}'`))
const { explainSyncFailure } = await import(dataURL(transpile('./syncProblems.ts').replace("from './machineSetup'", `from '${setupURL}'`)))

const context = { host: 'studio', site: 'https://reet.example' }

test('a missing skills folder names the exact folder and the one command that makes it', () => {
  const problem = explainSyncFailure('cannot read target skills directory: open /home/jnnngs/.config/opencode/skills: no such file or directory', context)
  assert.equal(problem.kind, 'skills-folder')
  assert.deepEqual(problem.commands, ["mkdir -p '/home/jnnngs/.config/opencode/skills'"])
  assert.equal(problem.local, true)
  assert.equal(problem.retry, true)
  assert.match(problem.detail, /studio/)
})

test('a path with a quote is quoted for a shell, not pasted raw', () => {
  const problem = explainSyncFailure("cannot read target skills directory: open /home/o'brien/.claude/skills: no such file or directory", context)
  assert.deepEqual(problem.commands, ["mkdir -p '/home/o'\\''brien/.claude/skills'"])
})

test('remote apply off gives the local opt-in command and says why the web cannot flip it', () => {
  const problem = explainSyncFailure('remote apply disabled locally; run reet agent allow-apply on in a terminal on this machine to opt in', context)
  assert.equal(problem.kind, 'remote-apply')
  assert.deepEqual(problem.commands, ['reet agent allow-apply on'])
  assert.equal(problem.local, true)
  assert.match(problem.detail, /local|web/)
})

test('stale plans retry without a command, but local changes need a review', () => {
  const stale = explainSyncFailure('approved plan no longer matches local files or desired revision; preview again', context)
  assert.equal(stale.retry, true)
  assert.equal(stale.local, false)
  assert.equal(stale.commands.length, 0)
  const review = explainSyncFailure('local changes need review in Compare & sync; save the reconciled selection before applying', context)
  assert.equal(review.retry, false)
  assert.equal(review.href, '/sync#sync-selection')
})

test('a paused machine points at its settings, and a missing agent names the site it belongs to', () => {
  const paused = explainSyncFailure('Sync is paused for studio. Resume it under Machines → Settings, then try again.', context)
  assert.equal(paused.kind, 'paused')
  assert.equal(paused.href, '/machines?machine=studio')
  const missing = explainSyncFailure('studio has no reet agent yet. Connect it once, then install from here in one click.', context)
  assert.equal(missing.kind, 'unconnected')
  assert.match(missing.commands[0], /reet agent enroll --server 'https:\/\/reet\.example' --env 'studio'/)
})

test('an unknown failure keeps its own words and still allows a safe retry', () => {
  const problem = explainSyncFailure('something odd happened', context)
  assert.equal(problem.kind, 'other')
  assert.match(problem.detail, /something odd happened/)
  assert.equal(problem.retry, true)
})
