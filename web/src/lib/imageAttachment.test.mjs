import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./imageAttachment.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const a = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const MB = 1 << 20
const LIMIT = 5 << 20

test('the modal accepts PNG, JPEG and WebP at the exact size limit', () => {
  assert.equal(a.MAX_ATTACHMENT_BYTES, LIMIT)
  assert.deepEqual([...a.ACCEPTED_ATTACHMENT_TYPES], ['image/png', 'image/jpeg', 'image/webp'])
  for (const type of a.ACCEPTED_ATTACHMENT_TYPES) assert.deepEqual(a.validateAttachment({ type, size: LIMIT }), { ok: true }, type)
})

test('one byte over the limit, an empty file and a wrong type are refused', () => {
  assert.deepEqual(a.validateAttachment({ type: 'image/png', size: LIMIT + 1 }), { ok: false, reason: 'size' })
  assert.deepEqual(a.validateAttachment({ type: 'image/png', size: 0 }), { ok: false, reason: 'empty' })
  assert.deepEqual(a.validateAttachment(null), { ok: false, reason: 'empty' })
  for (const type of ['image/gif', 'image/svg+xml', 'text/html', 'IMAGE/PNG', '', 'application/octet-stream']) {
    assert.deepEqual(a.validateAttachment({ type, size: 512 }), { ok: false, reason: 'type' }, type)
  }
})

test('every refusal has a clear sentence', () => {
  assert.equal(a.attachmentMessage('type'), 'Attach a PNG, JPEG or WebP image.')
  assert.equal(a.attachmentMessage('size'), 'That image is larger than 5 MiB.')
  assert.equal(a.attachmentMessage('empty'), 'That image is empty.')
  assert.equal(a.attachmentMessage('read'), 'That image could not be read.')
})
