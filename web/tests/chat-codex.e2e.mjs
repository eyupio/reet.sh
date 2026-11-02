// A Codex chat, offered only where a machine reports the reviewed adapter as available: the dialog's copy and create body (no
// OpenCode plugin trust), the session screen's wording, and the approvals-mode control that says what each mode means for the person.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-laptop000000001', S = 'session-codex00000000000001', SITE = '/home/dev/work/site'
const now = Date.now()
const session = { id: S, agentId: A, title: 'Codex · site', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'codex', model: '', effort: '', yolo: false, projectId: null, dir: SITE, structured: true, agentMode: 'read-only' }
const codexChat = { tool: 'codex', adapter: 'codex-acp', state: 'available', minimumVersion: '1.13.1', approval: true, modes: ['read-only', 'agent'] }
const machine = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, agentsV2: true, models: true, acp: true, acpApproval: true, acpAgentMode: true, tools: ['codex'], acpAgents: [codexChat] } }

const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { context, page, state, errors } = await openWorkspace(browser, {
  width: 1360, height: 900, touch: false, terminalStatus: active, terminalMachines: [machine],
  routes: {
    '/api/v2/terminal/sessions': req => {
      if (req.method() === 'POST') { state.create = req.postDataJSON(); return { ...session, title: state.create.title, state: 'running' } }
      return { sessions: state.created ? [session] : [] }
    },
    '/api/v2/folder-projects': { projects: [], savedWaiting: 0 },
    [`/api/v2/machines/${A}/models`]: { tool: 'codex', state: 'unavailable', models: [], more: false, source: '', version: '', fetchedAt: now, checkedAt: now, reason: 'no list', sentence: 'Model list unavailable.' },
    '/api/v2/profile/model-favourites': { version: 1, favourites: [] },
    [`/api/v2/terminal/sessions/${S}/chat/agent-mode`]: req => {
      state.modeBodies = [...(state.modeBodies ?? []), req.postDataJSON()]
      if (state.modeRefuse) return { status: 409, json: { error: 'The agent did not confirm the new mode. Check which mode the chat is in before you send anything else.' } }
      return { mode: req.postDataJSON().mode }
    },
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
  // Where the machine has not opted in (Codex only unverified), the dialog offers no Gaffer for Codex.
  machine.capabilities.acpAgents = [{ ...codexChat, state: 'unverified', modes: undefined }]
  await page.goto(`${base}/sessions?agent=${A}`)
  await page.getByRole('button', { name: 'New agent session', exact: true }).first().click()
  const dark = page.getByRole('dialog', { name: 'Start an agent', exact: true })
  await dark.getByRole('combobox', { name: 'Agent', exact: true }).selectOption({ label: 'Codex' })
  await expect(dark.getByRole('combobox', { name: 'Session', exact: true })).toHaveCount(0)
  await dark.getByRole('button', { name: 'Close', exact: true }).click()

  // Opted in: Gaffer is offered with Codex's own copy, which says it asks before leaving the sandbox, and no plugin warning.
  machine.capabilities.acpAgents = [codexChat]
  await page.goto(`${base}/sessions?agent=${A}`)
  await page.getByRole('button', { name: 'New agent session', exact: true }).first().click()
  const dialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
  await dialog.getByRole('combobox', { name: 'Agent', exact: true }).selectOption({ label: 'Codex' })
  await expect(dialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveValue('reet')
  await expect(dialog.getByText('Codex starts in the mode where it runs commands inside its sandbox and asks before one leaves it.', { exact: false })).toBeVisible()
  await expect(dialog.getByText('you may not be asked', { exact: false })).toBeVisible()
  await expect(dialog.getByText('.opencode', { exact: false })).toHaveCount(0)
  await dialog.getByRole('textbox', { name: /Opening prompt/ }).fill('fix the failing build')
  state.created = true
  await dialog.getByRole('button', { name: 'Start', exact: true }).click()
  await page.waitForURL(new RegExp(`session=${S}`))
  expect(state.create).toMatchObject({ structured: true, tool: 'codex', prompt: 'fix the failing build' })
  expect(state.create.trust).toBeUndefined()

  // The session screen names Codex and shows the approvals control, starting in the mode that can ask.
  await page.waitForFunction(() => window.__chatStream?.url.endsWith('/chat'))
  await page.evaluate(() => window.__chatStream.emit('reset', { truncated: false }))
  await expect(page.getByRole('textbox', { name: 'Message Codex' })).toBeVisible()
  // The connector forwards only the account label as an existing notice; it survives replay.
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 1, kind: 'notice', text: 'Codex sign-in: ChatGPT Pro' }))
  await expect(page.getByText('Codex sign-in: ChatGPT Pro', { exact: true })).toBeVisible()
  await page.evaluate(() => {
    window.__chatStream.emit('reset', { truncated: false })
    window.__chatStream.emit('chat', { seq: 1, kind: 'notice', text: 'Codex sign-in: ChatGPT Pro' })
  })
  await expect(page.getByText('Codex sign-in: ChatGPT Pro', { exact: true })).toHaveCount(1)

  await expect(page.getByText('Ask OpenCode', { exact: false })).toHaveCount(0)
  const group = page.getByRole('radiogroup', { name: 'Approvals' })
  await expect(group.getByRole('radio', { name: 'Ask before leaving the sandbox' })).toBeChecked()
  await expect(group.getByRole('radio', { name: 'Let Codex decide' })).not.toBeChecked()
  await expect(group.getByText('Codex runs commands inside its sandbox and asks before one leaves it.')).toBeVisible()

  // Choosing the other mode is sent, and the wording changes to say the person may not be asked.
  await group.getByRole('radio', { name: 'Let Codex decide' }).click()
  await expect.poll(() => state.modeBodies?.at(-1)).toEqual({ mode: 'agent' })
  await expect(group.getByRole('radio', { name: 'Let Codex decide' })).toBeChecked()
  await expect(group.getByText('Codex decides for itself what needs your approval. You may not be asked.')).toBeVisible()

  // A switch the machine did not confirm shows no active mode and says so, instead of keeping the old one.
  state.modeRefuse = true
  await group.getByRole('radio', { name: 'Ask before leaving the sandbox' }).click()
  await expect(page.getByRole('alert').getByText('did not confirm the new mode')).toBeVisible()
  await expect(group.getByRole('radio', { name: 'Ask before leaving the sandbox' })).not.toBeChecked()
  await expect(group.getByRole('radio', { name: 'Let Codex decide' })).not.toBeChecked()
  await expect(group.getByText('Codex’s mode isn’t confirmed. Choose one before you send anything else.')).toBeVisible()
  // Nothing is sent while the mode is unknown: it may be the one that never asks.
  await page.getByRole('textbox', { name: 'Message Codex' }).fill('run the tests')
  await expect(page.getByRole('button', { name: 'Send', exact: true })).toBeDisabled()

  // Choosing again after the machine answers puts a mode back.
  state.modeRefuse = false
  await group.getByRole('radio', { name: 'Ask before leaving the sandbox' }).click()
  await expect(group.getByRole('radio', { name: 'Ask before leaving the sandbox' })).toBeChecked()
  await expect(page.getByRole('button', { name: 'Send', exact: true })).toBeEnabled()

  // With YOLO on, the sandbox mode does not promise that the person is asked.
  session.yolo = true
  session.agentMode = 'read-only'
  await page.reload()
  await page.waitForFunction(() => window.__chatStream?.url.endsWith('/chat'))
  await page.evaluate(() => window.__chatStream.emit('reset', { truncated: false }))
  const yoloGroup = page.getByRole('radiogroup', { name: 'Approvals' })
  await expect(yoloGroup.getByText('YOLO is on for this chat, so when it asks to leave the sandbox you are not asked.', { exact: false })).toBeVisible()
  await expect(yoloGroup.getByText('and asks before one leaves it.', { exact: false })).toHaveCount(0)
  expect(errors).toEqual([])
  console.log('Codex chat UI passed: no Gaffer for Codex where the adapter is only unverified, Codex copy and no plugin warning where it is available, the create body is structured without trust, and the approvals control states each mode, sends a switch, and shows an unconfirmed switch as unknown.')
} finally {
  await context.close()
  await browser.close()
}
