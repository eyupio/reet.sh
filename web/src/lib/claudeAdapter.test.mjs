import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const { claudeAdapterHint, CLAUDE_ADAPTER_INSTALL } = await import(asURL(compile('./claudeAdapter.ts')))

const machine = (tools, acpAgents) => ({ agentId: 'm', host: 'studio', ready: true, capabilities: { tools, ...(acpAgents === undefined ? {} : { acpAgents }) } })
const entry = state => ({ tool: 'claude', adapter: 'claude-agent-acp', state })

test('the install command names the pinned adapter', () => {
  assert.equal(CLAUDE_ADAPTER_INSTALL, 'npm install --global @agentclientprotocol/claude-agent-acp@0.99.1')
})

test('a machine with Claude Code but no adapter is told what to install', () => {
  const hint = claudeAdapterHint(machine(['claude'], [entry('missing')]))
  assert.ok(hint?.includes(CLAUDE_ADAPTER_INSTALL))
  assert.match(hint, /studio/)
  // `missing` means reet on that machine cannot find the adapter on its PATH, which is not the same as it not being installed.
  assert.match(hint, /PATH/)
  assert.doesNotMatch(hint, /is not\.|adapter Gaffer needs for it is not/)
})

test('no hint when the adapter is there, Claude Code is not, or the machine cannot say', () => {
  assert.equal(claudeAdapterHint(machine(['claude'], [entry('available')])), null)
  assert.equal(claudeAdapterHint(machine(['opencode'], [entry('missing')])), null)
  assert.equal(claudeAdapterHint(machine(['claude'])), null)
  assert.equal(claudeAdapterHint(undefined), null)
  assert.equal(claudeAdapterHint({ agentId: 'm', host: 'studio', ready: true }), null)
})

test('no hint while the chat is not offered to this account, or the adapter is the wrong version', () => {
  // An account with the chat off (or a controller without the gate) sees Claude as unverified, whatever is installed.
  assert.equal(claudeAdapterHint(machine(['claude'], [entry('unverified')])), null)
  assert.equal(claudeAdapterHint(machine(['claude'], [entry('incompatible')])), null)
  // Another agent's descriptor is not Claude's.
  assert.equal(claudeAdapterHint(machine(['claude'], [{ tool: 'codex', adapter: 'codex-acp', state: 'missing' }])), null)
})
