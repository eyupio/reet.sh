// A Cline chat, offered only where a machine reports the reviewed native ACP adapter as available: the dialog's copy and
// create body (no OpenCode plugin trust), the session screen's wording, the plan/act mode control and a read approval card
// that carries the file it would read.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-laptop000000001', S = 'session-cline00000000000001', SITE = '/home/dev/work/site'
const now = Date.now()
const session = { id: S, agentId: A, title: 'Cline · site', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'cline', model: '', effort: '', yolo: false, projectId: null, dir: SITE, structured: true, agentMode: 'act' }
const clineChat = { tool: 'cline', adapter: 'cline', state: 'available', minimumVersion: '3.0.74', history: true, images: true, model: true, approval: true, modes: ['act', 'plan'] }
const machine = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, agentsV2: true, models: true, acp: true, acpHistory: true, acpImages: true, acpModel: true, acpApproval: true, acpAgentMode: true, tools: ['cline'], acpAgents: [clineChat] } }

const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { context, page, state, errors } = await openWorkspace(browser, {
  width: 1360, height: 900, touch: false, terminalStatus: active, terminalMachines: [machine],
  routes: {
    '/api/v2/terminal/sessions': req => {
      if (req.method() === 'POST') { state.create = req.postDataJSON(); return { ...session, title: state.create.title, state: 'running' } }
      return { sessions: state.created ? [session] : [] }
    },
    '/api/v2/folder-projects': { projects: [], savedWaiting: 0 },
    [`/api/v2/machines/${A}/models`]: { tool: 'cline', state: 'unavailable', models: [], more: false, source: '', version: '', fetchedAt: now, checkedAt: now, reason: 'no list', sentence: 'Model list unavailable.' },
    '/api/v2/profile/model-favourites': { version: 1, favourites: [] },
    [`/api/v2/terminal/sessions/${S}/chat/agent-mode`]: req => {
      state.modeBodies = [...(state.modeBodies ?? []), req.postDataJSON()]
      return { mode: req.postDataJSON().mode }
    },
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
  // Where the machine reports Cline's ACP server only unverified, the dialog offers no Gaffer for Cline.
  machine.capabilities.acpAgents = [{ ...clineChat, state: 'unverified', modes: undefined, history: false, images: false, model: false, approval: false }]
  await page.goto(`${base}/sessions?agent=${A}`)
  await page.getByRole('button', { name: 'New agent session', exact: true }).first().click()
  const dark = page.getByRole('dialog', { name: 'Start an agent', exact: true })
  await dark.getByRole('combobox', { name: 'Agent', exact: true }).selectOption({ label: 'Cline' })
  await expect(dark.getByRole('combobox', { name: 'Session', exact: true })).toHaveCount(0)
  await dark.getByRole('button', { name: 'Close', exact: true }).click()

  // Available: Gaffer is offered with Cline's own copy, and nothing claims OpenCode's plugin trust.
  machine.capabilities.acpAgents = [clineChat]
  await page.goto(`${base}/sessions?agent=${A}`)
  await page.getByRole('button', { name: 'New agent session', exact: true }).first().click()
  const dialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
  await dialog.getByRole('combobox', { name: 'Agent', exact: true }).selectOption({ label: 'Cline' })
  await expect(dialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveValue('reet')
  await expect(dialog.getByText('Cline asks before it reads a file, edits one or runs a command.', { exact: false })).toBeVisible()
  await expect(dialog.getByText('.opencode', { exact: false })).toHaveCount(0)
  await dialog.getByRole('textbox', { name: /Opening prompt/ }).fill('fix the failing build')
  await dialog.getByRole('combobox', { name: 'Project', exact: true }).selectOption({ label: 'No project — start in a folder' })
  state.created = true
  await dialog.getByRole('button', { name: 'Start', exact: true }).click()
  await page.waitForURL(new RegExp(`session=${S}`))
  expect(state.create).toMatchObject({ structured: true, tool: 'cline', prompt: 'fix the failing build' })
  expect(state.create.trust).toBeUndefined()

  // The session screen names Cline and offers its own modes, with words for each.
  await page.waitForFunction(() => window.__chatStream?.url.endsWith('/chat'))
  await page.evaluate(() => window.__chatStream.emit('reset', { truncated: false }))
  await expect(page.getByRole('textbox', { name: 'Message Cline' })).toBeVisible()
  await expect(page.getByText('Ask OpenCode', { exact: false })).toHaveCount(0)
  const modes = page.getByRole('radiogroup', { name: 'Approvals' })
  await expect(modes.getByRole('radio', { name: 'Make changes', exact: true })).toHaveAttribute('aria-checked', 'true')
  await expect(modes.getByText('Cline explores and changes the project, and asks before each command or edit.', { exact: true })).toBeVisible()
  await modes.getByRole('radio', { name: 'Plan only', exact: true }).click()
  await expect.poll(() => state.modeBodies).toEqual([{ mode: 'plan' }])
  await expect(modes.getByRole('radio', { name: 'Plan only', exact: true })).toHaveAttribute('aria-checked', 'true')

  // A read card carries its file, so it can be reviewed and allowed like any other.
  await page.evaluate(() => {
    window.__chatStream.emit('chat', { seq: 1, kind: 'permission', permissionId: 'p1', toolCallId: 'c1', title: 'read_files: notes.txt', toolKind: 'read', path: '/home/dev/work/site/notes.txt' })
  })
  await expect(page.locator('.approval .approval-path')).toContainText('notes.txt')
  await page.getByRole('button', { name: 'Allow once' }).click()
  await expect.poll(() => state.answer).toMatchObject({ allow: true })
  expect(errors).toEqual([])
  console.log('Cline chat UI passed: no Gaffer where the adapter is unverified, Cline copy and no plugin warning where it is available, the create body is structured without trust, the plan/act control says what each mode means, and a read approval shows its file.')
} finally {
  await context.close()
  await browser.close()
}
