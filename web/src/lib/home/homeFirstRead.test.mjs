import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const code = ts.transpileModule(readFileSync(new URL('./homeFirstRead.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { homeFirstRead } = await import(`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`)

const cases = [
  ['before the first read settles', { settled: false, failed: false, writer: true, hasSessions: false, hasHealth: false }, 'loading'],
  ['before the first read settles, writer with sessions', { settled: false, failed: true, writer: true, hasSessions: true, hasHealth: true }, 'loading'],
  ['the first read failed with no sessions for a writer', { settled: true, failed: true, writer: true, hasSessions: false, hasHealth: true }, 'error'],
  ['the first read failed with no health for a viewer', { settled: true, failed: true, writer: false, hasSessions: false, hasHealth: false }, 'error'],
  ['a writer with sessions but another read failed', { settled: true, failed: true, writer: true, hasSessions: true, hasHealth: false }, 'ready'],
  ['a viewer with health but another read failed', { settled: true, failed: true, writer: false, hasSessions: false, hasHealth: true }, 'ready'],
  ['the first read succeeded', { settled: true, failed: false, writer: true, hasSessions: true, hasHealth: true }, 'ready'],
  ['no failure is never an error', { settled: true, failed: false, writer: true, hasSessions: false, hasHealth: false }, 'ready'],
  ['a viewer with sync but no health is ready', { settled: true, failed: true, writer: false, hasSessions: false, hasHealth: true }, 'ready'],
  ['a writer without sessions but a failed read is an error', { settled: true, failed: true, writer: true, hasSessions: false, hasHealth: false }, 'error'],
]

for (const [name, input, expected] of cases) {
  test(name, () => assert.equal(homeFirstRead(input), expected))
}
