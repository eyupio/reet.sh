import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./prompts.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const prompt = (over = {}) => ({ title: 'Review', description: 'Review the change', body: 'Review it.', category: 'Quality', tool: '', model: '', effort: '', ...over })

test('the backslash trigger is a token at the start or after whitespace', () => {
  assert.deepEqual(p.promptTrigger('\\rev'), { start: 0, query: 'rev' })
  assert.deepEqual(p.promptTrigger('Please \\rev'), { start: 7, query: 'rev' })
  assert.deepEqual(p.promptTrigger('Please\n\\'), { start: 7, query: '' })
  assert.equal(p.promptTrigger('C:\\Users\\me'), null)
  assert.equal(p.promptTrigger('Please \\review this'), null)
  assert.equal(p.promptTrigger('/skill'), null)
})

test('the trigger follows the caret, not just the end of the draft', () => {
  assert.deepEqual(p.promptTrigger('hello \\rev there', 10), { start: 6, query: 'rev' })
  assert.equal(p.promptTrigger('hello \\rev there', 15), null)
})

test('a pick replaces only the trigger token', () => {
  const out = p.replacePromptTrigger('Please \\rev this', { start: 7, query: 'rev' }, 'review the diff')
  assert.deepEqual(out, { draft: 'Please review the diff this', caret: 22 })
})

test('a pick keeps the rest of a multiline message', () => {
  const out = p.replacePromptTrigger('First line\n\\re\nLast line', { start: 11, query: 're' }, 'Do the thing')
  assert.deepEqual(out, { draft: 'First line\nDo the thing\nLast line', caret: 23 })
})

test('a pick past the box limit changes nothing', () => {
  const out = p.replacePromptTrigger('\\p', { start: 0, query: 'p' }, 'x'.repeat(30), 20)
  assert.match(out.error, /too long/)
  assert.equal(out.draft, undefined)
})

test('variables are filled, including the local date', () => {
  const now = new Date(2026, 10, 9).getTime()
  const got = p.resolvePrompt('On {{project}} ({{branch}}) in {{folder}} for {{sessionTitle}} on {{date}}.', {
    project: 'Checkout', branch: 'main', folder: '/home/me/checkout', sessionTitle: 'Fix login', now,
  })
  assert.equal(got.text, 'On Checkout (main) in /home/me/checkout for Fix login on 2026-11-09.')
  assert.deepEqual(got.unresolved, [])
})

test('a missing variable stays as written and is reported once', () => {
  const got = p.resolvePrompt('{{project}} {{project}} {{branch}}')
  assert.equal(got.text, '{{project}} {{project}} {{branch}}')
  assert.deepEqual(got.unresolved, ['{{project}}', '{{branch}}'])
})

test('skill tokens expand in one pass and never rescan the inserted text', () => {
  const got = p.resolvePrompt('Use {{skill:review}} then {{skill:unknown}}.', {}, {
    review: 'Run the review skill. It mentions {{date}} literally.',
  })
  assert.equal(got.text, 'Use Run the review skill. It mentions {{date}} literally. then {{skill:unknown}}.')
  assert.deepEqual(got.unresolved, ['{{skill:unknown}}'])
})

test('namespaced skill IDs expand and missing namespaced skills are reported', () => {
  const got = p.resolvePrompt('{{skill:team/toolkit/review.v2}} then {{skill:team/toolkit/missing}}', {}, {
    'team/toolkit/review.v2': 'Run the review.',
  })
  assert.equal(got.text, 'Run the review. then {{skill:team/toolkit/missing}}')
  assert.deepEqual(got.unresolved, ['{{skill:team/toolkit/missing}}'])
})

test('unknown skill names never resolve through inherited object properties', () => {
  const body = '{{skill:constructor}} {{skill:toString}} {{skill:__proto__}}'
  const got = p.resolvePrompt(body)
  assert.equal(got.text, body)
  assert.deepEqual(got.unresolved, ['{{skill:constructor}}', '{{skill:toString}}', '{{skill:__proto__}}'])
  assert.equal(p.resolvePrompt('{{skill:constructor}}', {}, { constructor: 'Use this named skill.' }).text, 'Use this named skill.')
})

test('search matches title, description, category and body', () => {
  const items = [prompt(), prompt({ title: 'Ship', body: 'Publish the release', category: 'Release' })]
  assert.deepEqual(p.searchPrompts(items, 'quality').map(item => item.title), ['Review'])
  assert.deepEqual(p.searchPrompts(items, 'RELEASE').map(item => item.title), ['Ship'])
  assert.equal(p.searchPrompts(items, '').length, 2)
  assert.deepEqual(p.searchPrompts(items, 'nothing'), [])
})

test('validation names each field and accepts a complete prompt', () => {
  assert.deepEqual(p.validatePrompt(prompt()), {})
  assert.equal(p.validatePrompt(prompt({ title: ' ' })).title, 'Give the prompt a title.')
  assert.equal(p.validatePrompt(prompt({ title: 'x'.repeat(81) })).title, 'Keep the title to 80 characters.')
  assert.equal(p.validatePrompt(prompt({ body: ' ' })).body, 'Write what the agent should do.')
  assert.equal(p.validatePrompt(prompt({ body: 'x'.repeat(8001) })).body, 'That prompt is too long. Keep it under 8,000 bytes.')
  assert.equal(p.validatePrompt(prompt({ body: 'bad\u001b[2J' })).body, 'Use text without terminal control characters.')
  assert.equal(p.validatePrompt(prompt({ description: 'x'.repeat(501) })).description, 'Keep the description to 500 characters.')
  assert.equal(p.validatePrompt(prompt({ category: 'bad\u0000' })).category, 'Use a category without control characters.')
})

test('multi-byte text is measured in bytes, not characters', () => {
  assert.equal(p.validatePrompt(prompt({ body: 'é'.repeat(4001) })).body, 'That prompt is too long. Keep it under 8,000 bytes.')
})

test('suggestionLine names only what the prompt suggests', () => {
  const names = id => id === 'claude' ? 'Claude Code' : id
  assert.equal(p.suggestionLine(prompt()), '')
  assert.equal(p.suggestionLine(prompt({ tool: 'claude', model: 'opus', effort: 'high' }), names), 'Suggests Claude Code · opus · high')
  assert.equal(p.suggestionLine(prompt({ model: 'opus' }), names), 'Suggests opus')
})
