import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// CSV the browser writes follows the controller's export (internal/api/api.go csvCell): nothing a spreadsheet would run.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { csvCell, csvText } = await import(`data:text/javascript;base64,${Buffer.from(compile('./csv.ts')).toString('base64')}`)

test('a cell a spreadsheet would read as a formula starts with a quote', () => {
  for (const lead of ['=', '+', '-', '@', '\t']) assert.equal(csvCell(`${lead}cmd`), `'${lead}cmd`, JSON.stringify(lead))
  assert.equal(csvCell('\rcmd'), `"'\rcmd"`)
  assert.equal(csvCell('=HYPERLINK("https://example.test","x")'), `"'=HYPERLINK(""https://example.test"",""x"")"`)
})

test('other cells are written as they are, quoted only when they must be', () => {
  assert.equal(csvCell('code-review'), 'code-review')
  assert.equal(csvCell('1+1=2'), '1+1=2')
  assert.equal(csvCell(''), '')
  assert.equal(csvCell('a, b'), '"a, b"')
  assert.equal(csvCell('say "hi"'), '"say ""hi"""')
  assert.equal(csvCell('two\nlines'), '"two\nlines"')
  // A number is a number, never a formula: a count is written plainly, even below zero.
  assert.equal(csvCell(4), '4')
  assert.equal(csvCell(-1), '-1')
})

test('csvText joins cells with commas and rows with newlines', () => {
  assert.equal(csvText([['skill', 'used_by'], ['=x', 2], ['pdf', 0]]), "skill,used_by\n'=x,2\npdf,0")
})
