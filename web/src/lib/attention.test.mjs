import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./attention.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const a = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const { ATTENTION_WINDOW_MS, hasPending, isBlocked, attentionRuleOf, isApproval } = a

const NOW = 1_800_000_000_000
const HOUR = 60 * 60_000
const s = (over = {}) => ({ attention: '', changedSinceSeen: false, updatedAt: NOW - HOUR, ...over })
const perm = { permissionId: 'p1' }
const question = { questionId: 'q1' }

test('the window is 24 hours', () => {
  assert.equal(ATTENTION_WINDOW_MS, 24 * HOUR)
})

test('hasPending needs a non-empty permission or question id', () => {
  assert.equal(hasPending(s()), false)
  assert.equal(hasPending(s({ pendingPermission: { permissionId: '' } })), false)
  assert.equal(hasPending(s({ pendingPermission: perm })), true)
  assert.equal(hasPending(s({ pendingQuestion: question })), true)
})

test('a terminal dialog holds a waiting session blocked after it has been seen', () => {
  const dialog = { question: 'Run it?', options: [{ index: 1, label: 'Yes' }], fingerprint: 'f1' }
  assert.equal(hasPending(s({ terminalQuestion: dialog })), true)
  assert.equal(hasPending(s({ terminalQuestion: { ...dialog, fingerprint: '' } })), false)
  assert.equal(isBlocked(s({ attention: 'waiting', terminalQuestion: dialog })), true)
  assert.equal(isBlocked(s({ attention: 'working', terminalQuestion: dialog })), false)
})

test('isBlocked: waiting and unseen is blocked', () => {
  assert.equal(isBlocked(s({ attention: 'waiting', changedSinceSeen: true })), true)
})

test('isBlocked: waiting, seen, with a pending permission or question is blocked', () => {
  assert.equal(isBlocked(s({ attention: 'waiting', pendingPermission: perm })), true)
  assert.equal(isBlocked(s({ attention: 'waiting', pendingQuestion: question })), true)
})

test('isBlocked: waiting, seen, nothing pending is not blocked', () => {
  assert.equal(isBlocked(s({ attention: 'waiting' })), false)
})

test('isBlocked: working with a pending item is not blocked', () => {
  assert.equal(isBlocked(s({ attention: 'working', pendingPermission: perm })), false)
})

test('attentionRuleOf: a blocked session is waiting', () => {
  assert.equal(attentionRuleOf(s({ attention: 'waiting', changedSinceSeen: true }), NOW), 'waiting')
})

test('attentionRuleOf: an approval-only pending, seen, is still waiting', () => {
  assert.equal(attentionRuleOf(s({ attention: 'waiting', pendingPermission: perm }), NOW), 'waiting')
})

test('attentionRuleOf: failed and unseen inside the window is failed', () => {
  assert.equal(attentionRuleOf(s({ attention: 'failed', changedSinceSeen: true, updatedAt: NOW - HOUR }), NOW), 'failed')
})

test('attentionRuleOf: finished and unseen inside the window is finished', () => {
  assert.equal(attentionRuleOf(s({ attention: 'finished', changedSinceSeen: true, updatedAt: NOW - HOUR }), NOW), 'finished')
})

test('attentionRuleOf: finished and unseen past the window is nothing', () => {
  assert.equal(attentionRuleOf(s({ attention: 'finished', changedSinceSeen: true, updatedAt: NOW - 25 * HOUR }), NOW), null)
})

test('attentionRuleOf: failed but seen is nothing', () => {
  assert.equal(attentionRuleOf(s({ attention: 'failed', changedSinceSeen: false }), NOW), null)
})

test('attentionRuleOf: no attention is nothing', () => {
  assert.equal(attentionRuleOf(s({ attention: '', changedSinceSeen: true }), NOW), null)
  assert.equal(attentionRuleOf(s({ attention: undefined }), NOW), null)
})

test('attentionRuleOf: exactly the window old and unseen failed is still failed', () => {
  assert.equal(attentionRuleOf(s({ attention: 'failed', changedSinceSeen: true, updatedAt: NOW - ATTENTION_WINDOW_MS }), NOW), 'failed')
})

test('attentionRuleOf: one millisecond past the window and unseen failed is nothing', () => {
  assert.equal(attentionRuleOf(s({ attention: 'failed', changedSinceSeen: true, updatedAt: NOW - ATTENTION_WINDOW_MS - 1 }), NOW), null)
})

test('attentionRuleOf: unseen and in the window but working or quiet is nothing', () => {
  assert.equal(attentionRuleOf(s({ attention: 'working', changedSinceSeen: true }), NOW), null)
  assert.equal(attentionRuleOf(s({ attention: 'quiet', changedSinceSeen: true }), NOW), null)
})

test('attentionRuleOf: a failed session that also holds a pending item is still failed', () => {
  assert.equal(attentionRuleOf(s({ attention: 'failed', changedSinceSeen: true, pendingPermission: perm }), NOW), 'failed')
})

test('a dialog with a yes and a no style option is an approval, others are questions', () => {
  const d = labels => ({ question: 'x', options: labels.map((label, i) => ({ index: i + 1, label })), fingerprint: 'f' })
  assert.equal(isApproval(d(['Yes', "Yes, and don't ask again", 'No, and tell Claude what to do differently (esc)'])), true)
  assert.equal(isApproval(d(['Yes, proceed (y)', 'No, and tell Codex what to do differently (esc)'])), true)
  assert.equal(isApproval(d(['Allow once', 'Deny'])), true)
  assert.equal(isApproval(d(['Postgres', 'SQLite', 'Other'])), false)
  assert.equal(isApproval(d(['Yes'])), false)
})

