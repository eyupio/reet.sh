import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('../src/lib/terminal/osc52.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const o = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const MB = 1 << 20
const b64 = text => Buffer.from(text, 'utf8').toString('base64')

test('a read query is ignored and never decoded', () => {
  assert.deepEqual(o.parseOsc52('c;?'), { kind: 'ignore', reason: 'query' })
  assert.deepEqual(o.parseOsc52('?'), { kind: 'ignore', reason: 'query' })
  assert.deepEqual(o.parseOsc52('c;??'), { kind: 'ignore', reason: 'query' })
})

test('an empty payload is ignored instead of clearing the clipboard', () => {
  assert.deepEqual(o.parseOsc52('c;'), { kind: 'ignore', reason: 'empty' })
  assert.deepEqual(o.parseOsc52(''), { kind: 'ignore', reason: 'empty' })
})

test('the c selector decodes a multi-line text payload', () => {
  const text = 'line one\nline two'
  assert.deepEqual(o.parseOsc52(`c;${b64(text)}`), { kind: 'write', text, bytes: Buffer.byteLength(text) })
})

test('a bare base64 payload is accepted for compatibility', () => {
  const text = 'copied by the agent'
  assert.deepEqual(o.parseOsc52(b64(text)), { kind: 'write', text, bytes: Buffer.byteLength(text) })
})

test('decoded bytes are read as UTF-8 and counted by byte length', () => {
  const text = 'héllo 😀'
  assert.deepEqual(o.parseOsc52(`c;${b64(text)}`), { kind: 'write', text, bytes: Buffer.byteLength(text) })
  const replacement = Buffer.from([0xff, 0xfe, 0x41]).toString('base64')
  assert.deepEqual(o.parseOsc52(`c;${replacement}`), { kind: 'write', text: '\ufffd\ufffdA', bytes: 3 })
})

test('strict base64 rejects bad characters, bad lengths and bad padding', () => {
  for (const payload of ['c;not base64!', 'c;abc', 'c;ab=c', 'c;====', 'c;YWJj=', 'c;YW Jj', 'c;YQ==YQ==']) {
    assert.deepEqual(o.parseOsc52(payload), { kind: 'ignore', reason: 'invalid' }, payload)
  }
  assert.deepEqual(o.parseOsc52('c;YWJj'), { kind: 'write', text: 'abc', bytes: 3 })
  assert.deepEqual(o.parseOsc52('c;YQ=='), { kind: 'write', text: 'a', bytes: 1 })
})

test('selectors other than c are invalid', () => {
  const payload = b64('not for this clipboard')
  for (const selector of ['p', 's', '0', 'C', 'primary', '']) {
    assert.deepEqual(o.parseOsc52(`${selector};${payload}`), { kind: 'ignore', reason: 'invalid' }, selector)
  }
})

test('the default cap accepts exactly 1 MiB', () => {
  const text = 'x'.repeat(MB)
  const result = o.parseOsc52(`c;${Buffer.from(text).toString('base64')}`)
  assert.equal(result.kind, 'write')
  assert.equal(result.bytes, MB)
  assert.equal(result.text.length, MB)
})

test('one byte over the cap is large and carries no text', () => {
  const text = 'x'.repeat(MB + 1)
  const result = o.parseOsc52(`c;${Buffer.from(text).toString('base64')}`)
  assert.deepEqual(result, { kind: 'ignore', reason: 'large' })
  assert.equal('text' in result, false)
})

test('an explicit cap is honoured at its boundary', () => {
  const text = 'abc'
  assert.deepEqual(o.parseOsc52(`c;${b64(text)}`, 3), { kind: 'write', text, bytes: 3 })
  assert.deepEqual(o.parseOsc52(`c;${b64(text)}`, 2), { kind: 'ignore', reason: 'large' })
})

test('sizes format as bytes, kilobytes and megabytes with one decimal', () => {
  assert.equal(o.formatCopySize(0), '0 B')
  assert.equal(o.formatCopySize(512), '512 B')
  assert.equal(o.formatCopySize(1023), '1023 B')
  assert.equal(o.formatCopySize(1024), '1.0 KB')
  assert.equal(o.formatCopySize(2150), '2.1 KB')
  assert.equal(o.formatCopySize(MB), '1.0 MB')
  assert.equal(o.formatCopySize(Math.round(1.5 * MB)), '1.5 MB')
})
