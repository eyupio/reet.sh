import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const source = readFileSync(new URL('./draftOptions.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const ideaApi = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const { draftOptionRules, cleanDraftOptions } = ideaApi
const base = { tool: 'claude', isRepo: true, canChat: false, chat: false }

test('worktree shows only for a repository project', () => {
  assert.equal(draftOptionRules(base).showWorktree, true)
  assert.equal(draftOptionRules({ ...base, isRepo: false }).showWorktree, false)
})
test('chat is only for OpenCode on a machine that supports it, and hides the worktree', () => {
  assert.equal(draftOptionRules({ ...base, canChat: true }).showChat, false)
  const r = draftOptionRules({ ...base, tool: 'opencode', canChat: true, chat: true })
  assert.deepEqual(r, { showChat: true, chat: true, showWorktree: false, showTrust: true })
  assert.equal(draftOptionRules({ ...base, tool: 'opencode', canChat: false, chat: true }).chat, false)
})
test('hidden choices are cleared before saving', () => {
  const out = cleanDraftOptions({ ...base, isRepo: false, worktree: true, base: ' main ', trust: true, yolo: true })
  assert.deepEqual(out, { yolo: true, chat: false, trust: false, worktree: false, base: '' })
  assert.equal(cleanDraftOptions({ ...base, worktree: true, base: ' main ', trust: false, yolo: false }).base, 'main')
})

test('an idea needs a title or a note, nothing more', () => {
  const { ideaProblem, IDEA_START_NOTE } = ideaApi
  assert.equal(ideaProblem({ idea: true, title: '', prompt: '' }), 'Give the idea a title or a note.')
  assert.equal(ideaProblem({ idea: true, title: '  ', prompt: ' ' }), 'Give the idea a title or a note.')
  assert.equal(ideaProblem({ idea: true, title: 'Faster filters', prompt: '' }), '')
  assert.equal(ideaProblem({ idea: true, title: '', prompt: 'rough note' }), '')
  assert.equal(ideaProblem({ idea: false, title: '', prompt: '' }), '')
  assert.match(IDEA_START_NOTE, /switch off Idea/)
})
