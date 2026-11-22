import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const { yoloChatSentence, yoloTerminalSentence, yoloSandboxOffNotice, yoloSwitchTitle, yoloModeMeaning } = await import(asURL(compile('./yoloWording.ts')))

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

// A Codex terminal on YOLO also runs without its sandbox, and the confirmation says so; other agents keep the plain sentence.
test('a terminal launch says Codex also loses its sandbox', () => {
  const plain = yoloTerminalSentence('studio', 'Claude Code', 'claude')
  assert.equal(plain, "The agent will not ask before running commands and changing files on studio. reet passes Claude Code's own skip-the-prompts option and records the launch.")
  const codex = yoloTerminalSentence('studio', 'Codex', 'codex')
  assert.ok(codex.startsWith("The agent will not ask before running commands and changing files on studio. reet passes Codex's own skip-the-prompts option and records the launch."))
  assert.match(codex, /turning its sandbox off/)
  assert.match(codex, /any file and the network on studio/)
})

// Switching a Codex chat on YOLO to its terminal carries YOLO across, so the confirmation has to say the sandbox goes.
test('the switch-to-terminal warning says the sandbox goes and where', () => {
  const text = yoloSandboxOffNotice('studio')
  assert.match(text, /turns Codex’s sandbox off/)
  assert.match(text, /any file and the network on studio/)
  assert.match(text, /Let Codex decide/)
})
