import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./time.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { formatWhen } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const at = Date.UTC(2026, 10, 3, 12, 34)

test('formatWhen follows the chosen date order and time zone without seconds', () => {
  assert.match(formatWhen(at, 'UTC', 'dmy'), /3 Nov 2026/)
  assert.match(formatWhen(at, 'UTC', 'mdy'), /Nov 3, 2026/)
  assert.match(formatWhen(at, 'UTC', 'ymd'), /2026/)
  assert.match(formatWhen(at, 'UTC', 'dmy'), /12:34/)
  assert.doesNotMatch(formatWhen(at, 'UTC', 'dmy'), /:34:/)
  assert.match(formatWhen(at, 'America/New_York', 'dmy'), /7:34/)
})

test('formatWhen falls back for no timestamp and an unknown zone', () => {
  assert.equal(formatWhen(0), '')
  assert.match(formatWhen(at, 'Not/AZone', 'dmy'), /2026/)
})
