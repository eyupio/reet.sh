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
test('reviewed agents can use Gaffer when the machine supports it, without a worktree', () => {
  assert.equal(draftOptionRules({ ...base, tool: 'gemini', canChat: true }).showChat, false)
  for (const tool of ['claude','codex','cline']) assert.deepEqual(draftOptionRules({...base, tool, canChat:true, chat:true}), {showChat:true, chat:true, showWorktree:false, showTrust:false})
  const r = draftOptionRules({ ...base, tool: 'opencode', canChat: true, chat: true })
  assert.deepEqual(r, { showChat: true, chat: true, showWorktree: false, showTrust: true })
  assert.deepEqual(draftOptionRules({ ...base, tool: 'antigravity', canChat: true, chat: true }), r)
  assert.equal(draftOptionRules({ ...base, tool: 'opencode', canChat: false, chat: true }).chat, false)
})
test('hidden choices are cleared before saving', () => {
  const out = cleanDraftOptions({ ...base, isRepo: false, worktree: true, base: ' main ', trust: true, yolo: true })
  assert.deepEqual(out, { yolo: true, chat: false, trust: false, worktree: false, base: '' })
  assert.equal(cleanDraftOptions({ ...base, worktree: true, base: ' main ', trust: false, yolo: false }).base, 'main')
})

test('an idea needs a title or a note, nothing more', () => {
  const { ideaProblem } = ideaApi
  assert.equal(ideaProblem({ idea: true, title: '', prompt: '' }), 'Give the idea a title or a note.')
  assert.equal(ideaProblem({ idea: true, title: '  ', prompt: ' ' }), 'Give the idea a title or a note.')
  assert.equal(ideaProblem({ idea: true, title: 'Faster filters', prompt: '' }), '')
  assert.equal(ideaProblem({ idea: true, title: '', prompt: 'rough note' }), '')
  assert.equal(ideaProblem({ idea: false, title: '', prompt: '' }), '')
})

// What the card, and the idea's start dialog, check before a session can start. The board resolves the machine facts.
const item = { projectId: 'p1', imageCount: 0, effort: '' }
const ready = { readiness: '', agent: { available: true }, uploads: true, effort: true, projectCurrent: true }
test('a ready item has no start problem', () => {
  const { startProblem } = ideaApi
  assert.equal(startProblem(item, ready), null)
})
test('start problems come back in the order a person would fix them', () => {
  const { startProblem } = ideaApi
  assert.equal(startProblem(item, { ...ready, readiness: 'Choose an enrolled machine.', agent: undefined }), 'Choose an enrolled machine.')
  assert.equal(startProblem(item, { ...ready, agent: { available: false, reason: 'Not installed on studio' } }), 'Not installed on studio')
  assert.equal(startProblem(item, { ...ready, agent: undefined }), 'This agent is unavailable on that machine.')
  assert.equal(startProblem({ ...item, imageCount: 2 }, { ...ready, uploads: false }), 'Image uploads are unavailable on this machine.')
  assert.equal(startProblem({ ...item, imageCount: 0 }, { ...ready, uploads: false }), null)
  assert.equal(startProblem({ ...item, effort: 'high' }, { ...ready, effort: false }), 'Update reet on this machine to choose effort.')
  assert.equal(startProblem(item, { ...ready, projectCurrent: false }), 'The project changed or was removed. Edit this backlog item first.')
  // An item in a plain folder has no project to go stale.
  assert.equal(startProblem({ ...item, projectId: '' }, { ...ready, projectCurrent: false }), null)
})
