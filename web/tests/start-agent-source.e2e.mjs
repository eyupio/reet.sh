// Start an agent, Cline: the billing source beside the model. It appears for a Cline chat on a machine that negotiated
// acp-provider-v1, opens on the saved default source, is sent only when chosen, can be saved as the default, and a saved
// source Cline no longer offers never blocks Start. The default model follows the saved source.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-laptop000000001', S = 'session-cline00000000000001', SITE = '/home/dev/work/site'
const now = Date.now()
const session = { id: S, agentId: A, title: 'Cline · site', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'cline', model: '', effort: '', yolo: false, projectId: null, dir: SITE, structured: true, agentMode: 'act' }
const clineChat = { tool: 'cline', adapter: 'cline', state: 'available', minimumVersion: '3.0.74', history: true, images: true, model: true, approval: true, modes: ['act', 'plan'] }
const machine = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, agentsV2: true, models: true, acp: true, acpHistory: true, acpImages: true, acpModel: true, acpApproval: true, acpAgentMode: true, acpLiveControls: true, acpProvider: true, tools: ['cline'], acpAgents: [clineChat] } }
let savedSource = '', favourites = [], patches = [], refusal = null

const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { context, page, state, errors } = await openWorkspace(browser, {
  width: 1360, height: 900, touch: false, terminalStatus: active, terminalMachines: [machine],
  routes: {
    '/api/v2/terminal/sessions': req => {
      if (req.method() === 'POST') {
        state.create = req.postDataJSON()
        if (refusal) return { status: 409, json: refusal }
        return { ...session, title: state.create.title, state: 'running' }
      }
      return { sessions: state.create && !refusal ? [session] : [] }
    },
    '/api/v2/folder-projects': { projects: [], savedWaiting: 0 },
    [`/api/v2/machines/${A}/models`]: { tool: 'cline', state: 'unavailable', models: [], more: false, source: '', version: '', fetchedAt: now, checkedAt: now, reason: 'no list', sentence: 'Model list unavailable.' },
    '/api/v2/profile/model-favourites': () => ({ version: 1, favourites }),
    '/api/v2/profile': req => {
      if (req.method() === 'PATCH') { const body = req.postDataJSON(); patches.push(body); if ('defaultClineSource' in body) savedSource = body.defaultClineSource }
      return { user: { id: 1, name: 'Sam', email: 'sam@example.test', role: 'owner' }, sessionCount: 1, accounts: true,
        profile: { theme: 'light', timezone: 'Europe/London', dateFormat: '', shelfView: 'list', cardsExpanded: false, hostedPrefix: 'reet-', updatedAt: 1, defaultClineSource: savedSource } }
    },
  },
})
await page.addInitScript(() => {
  class FakeEventSource {
    constructor(url) { this.url = url; this.listeners = {}; setTimeout(() => this.onopen?.({}), 0) }
    addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn) }
    close() {}
  }
  window.EventSource = FakeEventSource
})

async function openDialog() {
  state.create = undefined
  await page.goto(`${base}/sessions?agent=${A}`)
  await page.getByRole('button', { name: 'New agent session', exact: true }).first().click()
  const dialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
  await dialog.getByRole('combobox', { name: 'Agent', exact: true }).selectOption({ label: 'Cline' })
  await expect(dialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveValue('reet')
  await dialog.getByRole('combobox', { name: 'Project', exact: true }).selectOption({ label: 'No project — start in a folder' })
  return dialog
}
const sourceOf = dialog => dialog.getByRole('combobox', { name: 'Source', exact: true })
const start = async dialog => {
  await dialog.getByRole('textbox', { name: /Opening prompt/ }).fill('fix the failing build')
  await dialog.getByRole('button', { name: 'Start', exact: true }).click()
}

try {
  // No saved default: Cline's own choice, nothing sent for it; ClinePass is sent when chosen.
  let dialog = await openDialog()
  await expect(sourceOf(dialog)).toBeVisible()
  await expect(sourceOf(dialog).locator('option')).toHaveText(['Cline’s own choice', 'Usage-Billing', 'ClinePass'])
  await expect(sourceOf(dialog)).toHaveValue('')
  await sourceOf(dialog).selectOption('cline-pass')
  await start(dialog)
  await page.waitForURL(new RegExp(`session=${S}`))
  expect(state.create).toMatchObject({ structured: true, tool: 'cline', provider: 'cline-pass' })

  // Choosing Cline's own choice sends no provider at all.
  dialog = await openDialog()
  await sourceOf(dialog).selectOption('cline-pass')
  await sourceOf(dialog).selectOption('')
  await start(dialog)
  await page.waitForURL(new RegExp(`session=${S}`))
  expect(state.create.provider).toBeUndefined()

  // The star saves the chosen source as the default, and pressing it again clears it.
  dialog = await openDialog()
  await sourceOf(dialog).selectOption('cline-pass')
  await dialog.getByRole('button', { name: 'Make ClinePass the default source', exact: true }).click()
  await expect.poll(() => patches.some(p => p.defaultClineSource === 'cline-pass')).toBe(true)
  const clear = dialog.getByRole('button', { name: 'ClinePass is the default source. Clear it', exact: true })
  await expect(clear).toBeVisible()
  await clear.click()
  await expect.poll(() => patches.some(p => p.defaultClineSource === '')).toBe(true)

  // A saved default opens the dialog on that source.
  savedSource = 'cline-pass'
  dialog = await openDialog()
  await expect(sourceOf(dialog)).toHaveValue('cline-pass')
  await start(dialog)
  await page.waitForURL(new RegExp(`session=${S}`))
  expect(state.create.provider).toBe('cline-pass')

  // A saved source reet has no label for (a chat's star saves whatever id Cline reports) is shown as given and still applies:
  // the dialog cannot know what Cline offers, so it never claims the machine lacks it.
  savedSource = 'cline-free'
  dialog = await openDialog()
  await expect(sourceOf(dialog)).toHaveValue('cline-free')
  await expect(sourceOf(dialog).locator('option')).toHaveText(['Cline’s own choice', 'Usage-Billing', 'ClinePass', 'cline-free'])
  await expect(dialog.getByText('isn’t available on this machine', { exact: false })).toHaveCount(0)
  await start(dialog)
  await page.waitForURL(new RegExp(`session=${S}`))
  expect(state.create.provider).toBe('cline-free')

  // The default model follows the saved source: it prefills on that source and not on another.
  savedSource = 'cline-pass'; favourites = [{ tool: 'cline', model: 'cline-pass/deepseek-v4', agentDefault: true }]
  dialog = await openDialog()
  await expect(dialog.getByRole('button', { name: /^Model:/ })).toHaveAccessibleName('Model: cline-pass/deepseek-v4')
  await sourceOf(dialog).selectOption('cline')
  await expect(dialog.getByRole('button', { name: /^Model:/ })).toHaveAccessibleName('Model: Cline’s configured model')
  await sourceOf(dialog).selectOption('cline-pass')
  await expect(dialog.getByRole('button', { name: /^Model:/ })).toHaveAccessibleName('Model: cline-pass/deepseek-v4')
  favourites = []

  // Cline's refusal of a source is shown in the dialog, which stays open for another choice.
  savedSource = ''
  refusal = { error: 'The agent did not use the chosen source, so the chat was not started.', code: 'acp_unavailable' }
  dialog = await openDialog()
  await sourceOf(dialog).selectOption('cline-pass')
  await start(dialog)
  await expect(dialog.getByText('The agent did not use the chosen source, so the chat was not started.', { exact: false })).toBeVisible()
  await expect(dialog).toBeVisible()
  refusal = null

  // A machine that did not negotiate acp-provider-v1 offers no Source, and sends no provider.
  machine.capabilities.acpProvider = false
  dialog = await openDialog()
  await expect(sourceOf(dialog)).toHaveCount(0)
  await start(dialog)
  await page.waitForURL(new RegExp(`session=${S}`))
  expect(state.create.provider).toBeUndefined()

  expect(errors).toEqual([])
  console.log('Start an agent Cline source passed.')
} finally { await context.close(); await browser.close() }
