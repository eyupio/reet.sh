// A Claude chat, offered only where a machine reports the reviewed adapter as available: the dialog's copy and create body
// (no OpenCode plugin trust), the session screen's wording, an approval card, and a machine that has not opted in.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-laptop000000001', S = 'session-claude0000000000001', SITE = '/home/dev/work/site'
const now = Date.now()
const session = { id: S, agentId: A, title: 'Claude Code · site', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'claude', model: '', effort: '', yolo: false, projectId: null, dir: SITE, structured: true }
const claudeChat = { tool: 'claude', adapter: 'claude-agent-acp', state: 'available', minimumVersion: '0.99.1', model: true, approval: true }
const machine = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, agentsV2: true, models: true, acp: true, acpModel: true, acpApproval: true, tools: ['claude'], acpAgents: [claudeChat] } }

const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { context, page, state, errors } = await openWorkspace(browser, {
  width: 1360, height: 900, touch: false, terminalStatus: active, terminalMachines: [machine],
  routes: {
    '/api/v2/terminal/sessions': req => {
      if (req.method() === 'POST') { state.create = req.postDataJSON(); return { ...session, title: state.create.title, state: 'running' } }
      return { sessions: state.created ? [session] : [] }
    },
    '/api/v2/folder-projects': { projects: [], savedWaiting: 0 },
    [`/api/v2/machines/${A}/models`]: { tool: 'claude', state: 'unavailable', models: [], more: false, source: '', version: '', fetchedAt: now, checkedAt: now, reason: 'no list', sentence: 'Model list unavailable.' },
    '/api/v2/profile/model-favourites': { version: 1, favourites: [] },
    [`/api/v2/terminal/sessions/${S}/chat/permissions/p1`]: req => { state.answer = req.postDataJSON(); return { answered: true } },
  },
})
await page.addInitScript(() => {
  class FakeEventSource {
    constructor(url) { this.url = url; this.listeners = {}; this.closed = false; window.__chatStream = this; setTimeout(() => this.onopen?.({}), 0) }
    addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn) }
    close() { this.closed = true }
    emit(type, data) { for (const fn of this.listeners[type] ?? []) fn({ data: JSON.stringify(data) }) }
  }
  window.EventSource = FakeEventSource
})
try {
  // Where the machine has not opted in (Claude only unverified), the dialog offers no Gaffer for Claude.
  machine.capabilities.acpAgents = [{ ...claudeChat, state: 'unverified' }]
  await page.goto(`${base}/sessions?agent=${A}`)
  await page.getByRole('button', { name: 'New agent session', exact: true }).first().click()
  const dark = page.getByRole('dialog', { name: 'Start an agent', exact: true })
  await dark.getByRole('radio', { name: 'Claude Code', exact: true }).check()
  await expect(dark.getByRole('combobox', { name: 'Session', exact: true })).toHaveCount(0)
  await dark.getByRole('button', { name: 'Close', exact: true }).click()

  // Opted in: Gaffer is offered, with Claude's own copy and none of OpenCode's plugin warning.
  machine.capabilities.acpAgents = [claudeChat]
  await page.goto(`${base}/sessions?agent=${A}`)
  await page.getByRole('button', { name: 'New agent session', exact: true }).first().click()
  const dialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
  await dialog.getByRole('radio', { name: 'Claude Code', exact: true }).check()
  await expect(dialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveValue('reet')
  await expect(dialog.getByText('Claude Code asks before it edits a file or runs a command.', { exact: false })).toBeVisible()
  await expect(dialog.getByText('Only your own Claude settings apply', { exact: false })).toBeVisible()
  await expect(dialog.getByText('.opencode', { exact: false })).toHaveCount(0)
  await dialog.getByRole('textbox', { name: /Opening prompt/ }).fill('fix the failing build')
  state.created = true
  await dialog.getByRole('button', { name: 'Start', exact: true }).click()
  await page.waitForURL(new RegExp(`session=${S}`))
  expect(state.create).toMatchObject({ structured: true, tool: 'claude', prompt: 'fix the failing build' })
  expect(state.create.trust).toBeUndefined()

  // The session screen names Claude Code, not OpenCode, and shows an approval with the usual choices.
  await page.waitForFunction(() => window.__chatStream?.url.endsWith('/chat'))
  await page.evaluate(() => {
    window.__chatStream.emit('reset', { truncated: false })
    window.__chatStream.emit('chat', { seq: 1, kind: 'user', text: 'fix the failing build' })
    window.__chatStream.emit('chat', { seq: 2, kind: 'permission', permissionId: 'p1', toolCallId: 'c1', title: 'Run the tests', toolKind: 'execute', command: 'go test ./...' })
  })
  await expect(page.getByRole('textbox', { name: 'Message Claude Code' })).toBeVisible()
  await expect(page.getByText('Ask OpenCode', { exact: false })).toHaveCount(0)
  await expect(page.getByText('go test ./...')).toBeVisible()
  await page.getByRole('button', { name: 'Allow once' }).click()
  await expect.poll(() => state.answer).toMatchObject({ allow: true })
  await page.evaluate(() => {
    window.__chatStream.emit('chat', { seq: 3, kind: 'permission_resolved', permissionId: 'p1', allow: true })
    window.__chatStream.emit('chat', { seq: 4, kind: 'status', state: 'working' })
  })
  await expect(page.getByRole('status').getByText('Claude Code is working…')).toBeVisible()
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 5, kind: 'status', state: 'error', detail: 'stopped' }))
  await expect(page.getByRole('alert').getByText('Claude Code could not continue')).toBeVisible()
  expect(errors).toEqual([])
  console.log('Claude chat UI passed: no Gaffer for Claude where the adapter is only unverified, Claude Code copy and no plugin warning where it is available, the create body is structured without trust, and the session screen says Claude Code with an approval card.')
} finally {
  await context.close()
  await browser.close()
}
