import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-laptop000000001', S = 'session-chat000000000000001', S2 = 'session-chat000000000000002', S3 = 'session-chat000000000000003', SITE = '/home/dev/work/site'
const now = Date.now()
const session = { id: S, agentId: A, title: 'OpenCode · site', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'opencode', model: '', effort: '', yolo: false, projectId: null, dir: SITE, structured: true }
const closedSession = { ...session, id: S2, state: 'exited', exitCode: 0, model: 'anthropic/claude-sonnet-4-5', acpSessionId: 'ses_fake000000000000000000000001' }
const machine = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, agentsV2: true, models: true, acp: true, acpHistory: true, uploads: true, tools: ['opencode'] } }

const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { context, page, state, errors } = await openWorkspace(browser, {
  width: 1360, height: 900, touch: false, terminalStatus: active, terminalMachines: [machine],
  routes: {
    '/api/v2/terminal/sessions': req => {
      if (req.method() === 'POST') {
        const body = req.postDataJSON()
        if (body.resumeAcp) {
          state.resume = body
          state.newSession = { ...closedSession, id: S3, title: body.title, state: 'running' }
          state.resumed = true
          return state.newSession
        }
        state.create = body
        return { ...session, title: body.title, state: 'running' }
      }
      if (state.closed) return { sessions: state.resumed ? [closedSession, state.newSession] : [closedSession] }
      return { sessions: [session] }
    },
    '/api/v2/folder-projects': { projects: [], savedWaiting: 0 },
    [`/api/v2/machines/${A}/models`]: { tool: 'opencode', state: 'listed', models: [{ id: 'anthropic/claude-sonnet-4-5', provider: 'anthropic' }], more: false, source: 'opencode models', version: '', fetchedAt: now, checkedAt: now, reason: '', sentence: '' },
    [`/api/v2/terminal/sessions/${S}/chat/prompt`]: req => { state.prompt = req.postDataJSON(); return { sent: true } },
    [`/api/v2/terminal/sessions/${S}/chat/cancel`]: () => { state.cancelled = true; return { cancelled: true } },
    [`/api/v2/terminal/sessions/${S}/chat/permissions/p1`]: req => { state.answer = req.postDataJSON(); return { answered: true } },
  },
})
await page.route(`**/api/v2/machines/${A}/uploads`, async route => {
  state.upload = route.request().headers()['content-type']
  await route.fulfill({ status: 200, headers: { 'X-Reet-Upload-Path': encodeURIComponent('/home/dev/launch-uploads/shot.png'), 'X-Reet-Upload-Reference': encodeURIComponent('/home/dev/launch-uploads/shot.png') }, json: { uploaded: 'true' } })
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
  await page.goto(`${base}/sessions?agent=${A}`)
  await page.getByRole('button', { name: 'New agent session', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('radio', { name: 'OpenCode', exact: true }).check()
  await expect(dialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveValue('reet')
  await expect(dialog.getByRole('button', { name: /^Model:/ })).toHaveAccessibleName('Model: OpenCode’s configured model')
  await dialog.getByRole('button', { name: /^Model:/ }).click()
  const models = page.getByRole('listbox', { name: 'Model', exact: true })
  await expect(models).toBeVisible()
  await models.getByRole('option', { name: 'anthropic/claude-sonnet-4-5', exact: true }).click()
  await expect(dialog.getByRole('button', { name: /^Model:/ })).toHaveAccessibleName('Model: anthropic/claude-sonnet-4-5')
  await dialog.getByRole('textbox', { name: /Opening prompt/ }).fill('fix the failing build')
  await dialog.getByRole('button', { name: 'Start', exact: true }).click()
  await page.waitForURL(new RegExp(`session=${S}`))
  await expect(page.getByRole('button', { name: 'reet TUI', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('button', { name: 'OpenCode TUI', exact: true })).toHaveAttribute('aria-disabled', 'true')
  await page.waitForFunction(() => window.__chatStream?.url.endsWith('/chat'))
  await page.evaluate(() => window.__chatStream.emit('reset', { truncated: false }))
  await page.evaluate(() => {
    window.__chatStream.emit('chat', { seq: 1, kind: 'user', text: 'fix the failing build' })
    window.__chatStream.emit('chat', { seq: 2, kind: 'agent', messageId: 'm1', text: 'Looking ' })
    window.__chatStream.emit('chat', { seq: 3, kind: 'agent', messageId: 'm1', text: 'at the build.' })
    window.__chatStream.emit('chat', { seq: 4, kind: 'permission', permissionId: 'p1', toolCallId: 'c1', title: 'Run the tests', toolKind: 'execute', command: 'go test ./...' })
  })
  await expect(page.getByText('Looking at the build.')).toBeVisible()
  await expect(page.getByText('go test ./...')).toBeVisible()
  await page.getByRole('button', { name: 'Allow once', exact: true }).click()
  await expect.poll(() => state.answer).toEqual({ allow: true })
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 5, kind: 'permission_resolved', permissionId: 'p1', allow: true }))
  await expect(page.getByText('Allowed once.')).toBeVisible()
  await page.getByRole('textbox', { name: 'Message OpenCode' }).fill('now run the linter')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  await expect.poll(() => state.prompt).toEqual({ text: 'now run the linter' })
  await page.locator('input[type=file]').setInputFiles({ name: 'shot.png', mimeType: 'image/png', buffer: Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]) })
  await expect(page.getByText('shot.png')).toBeVisible()
  await page.getByRole('textbox', { name: 'Message OpenCode' }).fill('look at this shot')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  await expect.poll(() => state.upload).toBe('image/png')
  await expect.poll(() => state.prompt).toEqual({ text: 'look at this shot\n\nImage: /home/dev/launch-uploads/shot.png' })
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 6, kind: 'status', state: 'working' }))
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Stop', exact: true }).click()
  await expect.poll(() => state.cancelled).toBe(true)
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 7, kind: 'status', state: 'cancelled' }))
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toHaveCount(0)
  expect(state.create).toMatchObject({ structured: true, trust: true, tool: 'opencode', model: 'anthropic/claude-sonnet-4-5', prompt: 'fix the failing build' })
  expect(state.create.effort).toBeUndefined()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('textbox', { name: 'Message OpenCode' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.evaluate(() => window.__chatStream.emit('reset', { truncated: true }))
  await expect(page.getByText('Earlier messages in this reet session are no longer available.')).toBeVisible()

  // A closed chat keeps OpenCode's session id and reopens as a new row that loads its history.
  state.closed = true
  await page.goto(`${base}/sessions?agent=${A}&session=${S2}`)
  await expect(page.getByRole('button', { name: 'Session actions' })).toBeVisible()
  await page.getByRole('button', { name: 'Session actions' }).click()
  await page.getByRole('menuitem', { name: /Continue in reet TUI with history/ }).click()
  await expect.poll(() => state.resume).toMatchObject({ structured: true, trust: true, resumeAcp: closedSession.acpSessionId, dir: SITE })
  await page.waitForURL(new RegExp(`session=${S3}`))
  expect(state.resume.prompt).toBeUndefined()
  expect(state.resume.model).toBe('anthropic/claude-sonnet-4-5')
  expect(errors).toEqual([])
  console.log('Structured OpenCode chat UI passed: the dialog offers the reet TUI for OpenCode, the create body carries structured, trust and the chosen model, the session screen opens reet TUI with the OpenCode TUI disabled, agent chunks stream, a command approval is Allow once or Deny, a message sends, an attached image uploads and its path joins the prompt, Stop cancels, the phone fits, a truncated reset says so, and a closed chat reopens through the continue action with its OpenCode session id.')
} finally {
  await context.close()
  await browser.close()
}
