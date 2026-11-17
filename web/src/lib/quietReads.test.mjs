import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// The bell's background reads of the Factory and Gaffer's goals do not drive the shell's progress bar (as the terminal
// reads do not); a page's own read of them still does.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const dataUrl = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
globalThis.__begun = 0
const stub = dataUrl('export const inflight = { begin() { globalThis.__begun++; return () => {} } }')
const api = await import(dataUrl(compile('./api.ts').replace(/(['"])\.\/inflight\1/, JSON.stringify(stub))))
globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} }
globalThis.fetch = async path => new Response(JSON.stringify(String(path).includes('outcomes') ? { outcomes: [] } : { goals: [], readAt: 1 }), { status: 200, headers: { 'Content-Type': 'application/json' } })

test('a quiet read of the outcomes and the goals does not count for the progress bar', async () => {
  globalThis.__begun = 0
  await api.listGafferOutcomes(undefined, true)
  await api.listGafferGoals(undefined, true)
  assert.equal(globalThis.__begun, 0)
})

test('a page’s own read of them still does', async () => {
  globalThis.__begun = 0
  await api.listGafferOutcomes()
  await api.listGafferGoals()
  assert.equal(globalThis.__begun, 2)
})
