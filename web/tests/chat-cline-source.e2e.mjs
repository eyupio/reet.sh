import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

// A live Cline chat offers the billing source Cline reports, switches it through the controls endpoint, saves a default
// source on the profile, and points a credit failure at the other sources.
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-cline000000000001', S = 'session-cline0000000000001', root = `/api/v2/terminal/sessions/${S}/chat`
const now = Date.now()
const session = { id: S, agentId: A, title: 'Cline source', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'cline', model: 'stub/model', effort: '', yolo: false, projectId: null, dir: '/tmp', structured: true }
const machine = { agentId: A, host: 'cline-machine', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', acp: true, acpModel: true, acpLiveControls: true, acpProvider: true, tools: ['cline'] } }
const catalogue = { cline: ['stub/model', 'provider/other'], 'cline-pass': ['cline-pass/deepseek-v4', 'cline-pass/other'] }
let changes = [], patches = [], savedSource = ''
let controls
const reset = (values = ['cline', 'cline-pass'], working = false) => {
 changes = []; patches = []; savedSource = ''
 controls = { working, waiting: false, compacting: false, configuring: false, steering: false, model: 'stub/model', options: [
  { id: 'provider', category: 'provider', current: 'cline', values },
  { id: 'model', category: 'model', current: 'stub/model', values: catalogue.cline },
 ] }
}
const profile = () => ({ user: { id: 1, name: 'Sam', email: 'sam@example.test', role: 'owner' }, sessionCount: 1, accounts: true,
 profile: { theme: 'light', timezone: 'Europe/London', dateFormat: '', shelfView: 'list', cardsExpanded: false, hostedPrefix: 'reet-', updatedAt: 1, defaultClineSource: savedSource } })
reset()
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { page, context, errors } = await openWorkspace(browser, {
 width: 1360, height: 900, terminalStatus: active, terminalMachines: [machine],
 routes: {
  '/api/v2/terminal/sessions': { sessions: [session] },
  '/api/v2/profile': req => {
   if (req.method() === 'PATCH') { const body = req.postDataJSON(); patches.push(body); if ('defaultClineSource' in body) savedSource = body.defaultClineSource }
   return profile()
  },
  [`${root}/controls`]: req => {
   if (req.method() === 'POST') {
    const body = req.postDataJSON(); changes.push(body)
    const option = controls.options.find(o => o.id === body.configId)
    option.current = body.value
    // A source switch replaces the model catalogue, as Cline's own reply does.
    if (body.configId === 'provider') {
     const model = controls.options.find(o => o.id === 'model')
     model.values = catalogue[body.value] ?? model.values; model.current = model.values[0]; controls.model = model.current
    }
   }
   return { controls }
  },
 },
})
await page.addInitScript(() => {
 class Stream {
  constructor() { this.listeners = {}; window.__chatStream = this; setTimeout(() => this.onopen?.({}), 0) }
  addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn) }
  close() {}
  emit(type, data) { for (const fn of this.listeners[type] ?? []) fn({ data: JSON.stringify(data) }) }
 }
 window.EventSource = Stream
})
async function open(state = 'idle') {
 await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
 await page.waitForFunction(() => window.__chatStream)
 await page.evaluate(state => { window.__chatStream.emit('reset', { truncated: false }); window.__chatStream.emit('chat', { seq: 1, kind: 'status', state }) }, state)
}
const source = () => page.getByRole('combobox', { name: 'Source', exact: true })
try {
 // Two sources: the control shows the current one, and choosing another is a confirmed set on the controls endpoint.
 await open()
 await expect(source()).toBeVisible()
 await expect(source()).toHaveValue('cline')
 await expect(source().locator('option')).toHaveText(['Usage-Billing', 'ClinePass'])
 await source().selectOption('cline-pass')
 await expect.poll(() => changes.some(c => c.action === 'set' && c.configId === 'provider' && c.value === 'cline-pass')).toBe(true)
 await expect(source()).toHaveValue('cline-pass')
 // The model picker now offers the new source's catalogue.
 await expect(page.getByRole('button', { name: 'Model: cline-pass/deepseek-v4', exact: true })).toBeVisible()

 // Make default saves the source on the profile; pressing it again clears it.
 await page.getByRole('button', { name: 'Make ClinePass the default source', exact: true }).click()
 await expect.poll(() => patches.some(p => p.defaultClineSource === 'cline-pass')).toBe(true)
 const clear = page.getByRole('button', { name: 'ClinePass is the default source. Clear it', exact: true })
 await expect(clear).toBeVisible()
 await clear.click()
 await expect.poll(() => patches.some(p => p.defaultClineSource === '')).toBe(true)
 await expect(page.getByRole('button', { name: 'Make ClinePass the default source', exact: true })).toBeVisible()

 // A credit failure points at the other sources the session offers.
 await page.evaluate(() => window.__chatStream.emit('chat', { seq: 2, kind: 'status', state: 'error', detail: 'Cline: Internal error: Insufficient balance. Your Cline Credits balance is $-0.00' }))
 await expect(page.getByRole('alert').getByText('This source has no credit. Switch to Usage-Billing above, or add credits with Cline, then send a message to continue.', { exact: true })).toBeVisible()

 // One value is not a choice: no Source control.
 reset(['cline'])
 await open()
 await expect(page.getByRole('button', { name: /^Model:/ })).toBeVisible()
 await expect(source()).toHaveCount(0)

 // A source switch is not offered while a turn runs.
 reset(['cline', 'cline-pass'], true)
 await open('working')
 await expect(source()).toBeDisabled()

 expect(errors).toEqual([])
 console.log('Cline source control passed.')
} finally { await context.close(); await browser.close() }
