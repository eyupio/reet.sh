import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./chatUsage.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const u = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const usage = (over = {}) => ({ reported: true, inputTokens: 0, outputTokens: 0, cachedTokens: 0, contextUsed: 0, contextSize: 0, updatedAt: 1, ...over })

test('an agent that has reported nothing shows nothing, never a zero', () => {
  assert.equal(u.chatUsageView(null, 'Codex'), null)
  assert.equal(u.chatUsageView(undefined, 'Codex'), null)
  assert.equal(u.chatUsageView(usage({ reported: false }), 'Codex'), null)
  // A record with nothing in it says nothing either.
  assert.equal(u.chatUsageView(usage(), 'Codex'), null)
})

test('context left leads, with the window and the chat’s tokens in the detail', () => {
  const view = u.chatUsageView(usage({ inputTokens: 1300, outputTokens: 250, cachedTokens: 5000, contextUsed: 90000, contextSize: 200000 }), 'Codex')
  assert.deepEqual(view, {
    label: 'Context 55% left', low: false, percentLeft: 55,
    detail: 'Codex reports 110k of its 200k-token context window free. This chat has used 1,550 tokens, and 5,000 cached.',
  })
})

test('a nearly full context is marked low at 15% and under', () => {
  assert.equal(u.chatUsageView(usage({ contextUsed: 170000, contextSize: 200000 }), 'Claude Code').low, true)
  assert.equal(u.chatUsageView(usage({ contextUsed: 168000, contextSize: 200000 }), 'Claude Code').low, false)
  const full = u.chatUsageView(usage({ contextUsed: 500000, contextSize: 200000 }), 'Claude Code')
  assert.equal(full.label, 'Context 0% left')
  assert.equal(full.percentLeft, 0)
  assert.equal(full.detail, 'Claude Code reports 0 of its 200k-token context window free.')
})

test('tokens alone are shown when the agent never said how large its window is', () => {
  const view = u.chatUsageView(usage({ inputTokens: 84000, outputTokens: 600 }), 'OpenCode')
  assert.deepEqual(view, { label: '85k tokens', low: false, percentLeft: null, detail: 'OpenCode reports this chat has used 85k tokens. It has not said how full its context window is.' })
  assert.equal(u.chatUsageView(usage({ inputTokens: 1, outputTokens: 0 }), 'OpenCode').label, '1 token')
  assert.equal(u.chatUsageView(usage({ inputTokens: 2_400_000 }), 'OpenCode').label, '2.4M tokens')
})

test('cached tokens alone still count as a report', () => {
  const view = u.chatUsageView(usage({ cachedTokens: 12000 }), 'Cline')
  assert.equal(view.label, '0 tokens')
  assert.equal(view.detail, 'Cline reports this chat has used 0 tokens, and 12k cached. It has not said how full its context window is.')
})
