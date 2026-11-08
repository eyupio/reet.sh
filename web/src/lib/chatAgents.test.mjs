import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const { GATED_CHATS, chatToggles, chatTogglePatch, CHAT_GATE_HELP, isGatedChat, SWITCH_LIVE_CHAT_HINT } = await import(asURL(compile('./chatAgents.ts')))

test('the gated chats are Claude Code and Codex, labelled for Gaffer', () => {
  assert.deepEqual(GATED_CHATS.map(c => c.id), ['claude', 'codex'])
  assert.deepEqual(GATED_CHATS.map(c => c.label), ['Claude Code in Gaffer', 'Codex in Gaffer'])
})

test('a toggle reads the server\'s effective state and is off when the server said nothing', () => {
  assert.deepEqual(chatToggles({ claude: true, codex: false }).map(t => t.on), [true, false])
  assert.deepEqual(chatToggles(undefined).map(t => t.on), [false, false])
  assert.deepEqual(chatToggles({}).map(t => t.on), [false, false])
})

test('one toggle sends only its own agent', () => {
  assert.deepEqual(chatTogglePatch('claude', true), { chatAgents: { claude: true } })
  assert.deepEqual(chatTogglePatch('codex', false), { chatAgents: { codex: false } })
})

test('the help line says the chat has not been checked with a signed-in account', () => {
  assert.match(CHAT_GATE_HELP, /Not yet checked with a signed-in account/)
})

test('a gated chat that lost its approval controls points at Settings, not at updating the machine', () => {
  assert.equal(isGatedChat('claude'), true)
  assert.equal(isGatedChat('codex'), true)
  assert.equal(isGatedChat('opencode'), false)
  assert.equal(SWITCH_LIVE_CHAT_HINT('opencode'), 'Update reet on that machine to switch a live chat.')
  assert.match(SWITCH_LIVE_CHAT_HINT('claude'), /Settings > Agents/)
  assert.doesNotMatch(SWITCH_LIVE_CHAT_HINT('claude'), /Update reet/)
})
