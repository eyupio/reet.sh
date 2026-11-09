import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const { yoloChatSentence, yoloSwitchTitle, yoloModeMeaning } = await import(asURL(compile('./yoloWording.ts')))

test('a chat on most agents says YOLO answers every permission request, as before', () => {
  for (const tool of ['claude', 'opencode', 'cline']) {
    assert.equal(yoloChatSentence('studio', tool), 'The agent will not wait for you. reet says yes, this once, to every permission request it makes on studio, including running commands and changing files.')
    assert.equal(yoloSwitchTitle(tool), 'Answer every approval for this session')
  }
})

// Codex asks only in "Ask before leaving the sandbox"; in "Let Codex decide" its own reviewer decides and the cards it still raises are left to the person.
test('a Codex chat says where YOLO answers and where it does not', () => {
  const sentence = yoloChatSentence('studio', 'codex')
  assert.match(sentence, /on studio/)
  assert.match(sentence, /Ask before leaving the sandbox/)
  assert.match(sentence, /Let Codex decide/)
  assert.match(sentence, /left to you/)
  assert.doesNotMatch(sentence, /every permission request/)
  assert.doesNotMatch(yoloSwitchTitle('codex'), /every approval/)
  assert.match(yoloSwitchTitle('codex'), /sandbox/i)
})

test('the mode text with YOLO on says what happens in each Codex mode', () => {
  assert.match(yoloModeMeaning('read-only', 'Codex'), /YOLO is on.*not asked/)
  const agent = yoloModeMeaning('agent', 'Codex')
  assert.match(agent, /YOLO is on/)
  assert.match(agent, /left to you/)
  assert.equal(yoloModeMeaning('plan', 'Codex'), null)
  assert.equal(yoloModeMeaning('act', 'OpenCode'), null)
})
