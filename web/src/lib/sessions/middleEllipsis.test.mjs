import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./middleEllipsis.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { middleEllipsis } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('a path that fits is returned untouched', () => {
  assert.equal(middleEllipsis('/home/dev/reet', 40), '/home/dev/reet')
  assert.equal(middleEllipsis('x'.repeat(40), 40), 'x'.repeat(40))
})

test('a long path loses its middle and keeps both ends within the limit', () => {
  const path = '/home/dev/.reet/worktrees/a-very-long-folder-name/another-long-folder/reet-session-001'
  const out = middleEllipsis(path, 40)
  assert.equal(Array.from(out).length, 40)
  assert.ok(out.startsWith('/home/dev'))
  assert.ok(out.endsWith('reet-session-001'))
  assert.equal(out.split('…').length, 2)
})

test('the cut never splits a character made of two code units', () => {
  const out = middleEllipsis('😀'.repeat(30), 11)
  assert.equal(Array.from(out).length, 11)
  assert.ok(!/[\ud800-\udbff](?![\udc00-\udfff])|(?<![\ud800-\udbff])[\udc00-\udfff]/.test(out))
})

test('a limit too small to cut into leaves the text alone', () => {
  assert.equal(middleEllipsis('/a/b/c/d', 2), '/a/b/c/d')
})
