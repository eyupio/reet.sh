import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const { chatAdapterHint, chatAdapterInstall, HINTED_TOOLS } = await import(asURL(compile('./chatAdapter.ts')))
const CLAUDE_ADAPTER_INSTALL = chatAdapterInstall('claude')

const machine = (tools, acpAgents) => ({ agentId: 'm', host: 'studio', ready: true, capabilities: { tools, ...(acpAgents === undefined ? {} : { acpAgents }) } })
const entry = state => ({ tool: 'claude', adapter: 'claude-agent-acp', state })
const codexEntry = state => ({ tool: 'codex', adapter: 'codex-acp', state })

test('the install commands name the adapters reet was checked against', () => {
  assert.equal(CLAUDE_ADAPTER_INSTALL, 'npm install --global @agentclientprotocol/claude-agent-acp@0.99.1')
  assert.equal(chatAdapterInstall('codex'), 'npm install --global @agentclientprotocol/codex-acp@1.13.1')
  assert.deepEqual(HINTED_TOOLS, ['claude', 'codex'])
  assert.equal(chatAdapterInstall('opencode'), '')
})

// Codex's adapter uses the `codex` that is already on the machine's PATH, so the command installs the adapter alone and the hint says whose it is.
test('a machine with Codex but no adapter is told what to install', () => {
  const hint = chatAdapterHint('codex', machine(['codex'], [codexEntry('missing')]))
  assert.ok(hint?.includes(chatAdapterInstall('codex')))
  assert.match(hint, /Codex is installed on studio/)
  assert.match(hint, /codex-acp/)
  assert.match(hint, /PATH/)
  // Another agent's missing adapter is not Codex's, and a tool the table does not know never speaks.
  assert.equal(chatAdapterHint('codex', machine(['codex'], [entry('missing')])), null)
  assert.equal(chatAdapterHint('codex', machine(['claude'], [codexEntry('missing')])), null)
  assert.equal(chatAdapterHint('codex', machine(['codex'], [codexEntry('available')])), null)
  assert.equal(chatAdapterHint('codex', machine(['codex'], [codexEntry('unverified')])), null)
  assert.equal(chatAdapterHint('opencode', machine(['opencode'], [{ tool: 'opencode', adapter: 'opencode', state: 'missing' }])), null)
})

test('a machine with Claude Code but no adapter is told what to install', () => {
  const hint = chatAdapterHint('claude', machine(['claude'], [entry('missing')]))
  assert.ok(hint?.includes(CLAUDE_ADAPTER_INSTALL))
  assert.match(hint, /studio/)
  // `missing` means reet on that machine cannot find the adapter on its PATH, which is not the same as it not being installed.
  assert.match(hint, /PATH/)
  assert.doesNotMatch(hint, /is not\.|adapter Gaffer needs for it is not/)
})

test('no hint when the adapter is there, Claude Code is not, or the machine cannot say', () => {
  assert.equal(chatAdapterHint('claude', machine(['claude'], [entry('available')])), null)
  assert.equal(chatAdapterHint('claude', machine(['opencode'], [entry('missing')])), null)
  assert.equal(chatAdapterHint('claude', machine(['claude'])), null)
  assert.equal(chatAdapterHint('claude', undefined), null)
  assert.equal(chatAdapterHint('claude', { agentId: 'm', host: 'studio', ready: true }), null)
})

test('no hint while the chat is not offered to this account, or the adapter is the wrong version', () => {
  // An account with the chat off (or a controller without the gate) sees Claude as unverified, whatever is installed.
  assert.equal(chatAdapterHint('claude', machine(['claude'], [entry('unverified')])), null)
  assert.equal(chatAdapterHint('claude', machine(['claude'], [entry('incompatible')])), null)
  // Another agent's descriptor is not Claude's.
  assert.equal(chatAdapterHint('claude', machine(['claude'], [{ tool: 'codex', adapter: 'codex-acp', state: 'missing' }])), null)
})
