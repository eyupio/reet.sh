import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-live000000000001', S = 'session-live0000000000001', root = `/api/v2/terminal/sessions/${S}/chat`
const now = Date.now()
const session = { id: S, agentId: A, title: 'Live controls', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'opencode', model: 'provider/one', effort: 'low', yolo: false, projectId: null, dir: '/tmp', structured: true }
const machine = { agentId: A, host: 'live-machine', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', acp: true, acpModel: true, acpLiveControls: true, tools: ['opencode'] } }
let queue = [], changes = [], serial = 0
let controls = { working: true, waiting: false, compacting: false, configuring: false, steering: true, model: 'provider/one', options: [{ id: 'model', category: 'model', current: 'provider/one', values: ['provider/one', 'provider/two'] }, { id: 'effort', category: 'thought_level', current: 'low', values: ['low', 'high'] }] }
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { page, context, errors } = await openWorkspace(browser, {
 width: 1360, height: 900, terminalStatus: active, terminalMachines: [machine],
 routes: {
  '/api/v2/terminal/sessions': { sessions: [session] },
  [`${root}/controls`]: req => {
   if (req.method() === 'POST') { const in_ = req.postDataJSON(); changes.push(in_); controls.options.find(o => o.id === in_.configId).current = in_.value; if (in_.configId === 'model') controls.model = in_.value }
   return { controls }
  },
  [`${root}/queue`]: req => {
   if (req.method() === 'POST') { const body = req.postDataJSON(); if (!queue.some(q => q.id === body.id)) queue.push({ ...body, state: 'queued', intent: 'wait', revision: ++serial, note: '' }) }
   if (req.method() === 'PATCH') {
    const body = req.postDataJSON(); changes.push(body)
    const item = queue.find(q => q.id === body.id)
    if (body.action === 'remove') queue = queue.filter(q => q !== item)
    else { item.revision++; item.intent = body.action; if (body.action === 'edit') item.text = body.text; if (body.action === 'retry') { item.state = 'queued'; item.note = '' } }
   }
   return { queue }
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
async function open() {
 await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
 await page.waitForFunction(() => window.__chatStream)
 await page.evaluate(() => { window.__chatStream.emit('reset', { truncated: false }); window.__chatStream.emit('chat', { seq: 1, kind: 'status', state: 'working' }) })
}
try {
 await open()
 const model = page.getByRole('button', { name: /^Model:/ })
 await expect(model).toBeEnabled()
 await model.click()
 await page.getByRole('listbox', { name: 'Model', exact: true }).getByRole('option', { name: 'provider/two', exact: true }).click()
 await expect.poll(() => controls.model).toBe('provider/two')
 await expect(page.getByRole('button', { name: 'Model: provider/two', exact: true })).toBeVisible()
 await page.getByRole('combobox', { name: 'Effort', exact: true }).selectOption('high')
 await expect.poll(() => changes.some(c => c.configId === 'effort' && c.value === 'high')).toBe(true)
 const composer = page.getByRole('textbox', { name: 'Message OpenCode', exact: true })
 await composer.fill('Use the existing implementation')
 await page.getByRole('button', { name: 'Queue', exact: true }).click()
 const region = page.getByRole('region', { name: 'Queued messages' })
 await expect(region).toContainText('Use the existing implementation')
 await page.reload()
 await page.waitForFunction(() => window.__chatStream)
 await page.evaluate(() => { window.__chatStream.emit('reset', { truncated: false }); window.__chatStream.emit('chat', { seq: 1, kind: 'status', state: 'working' }) })
 await expect(region).toContainText('Use the existing implementation')
 await region.getByRole('button', { name: 'Send now', exact: true }).click()
 await expect.poll(() => changes.some(c => c.action === 'send')).toBe(true)
 await region.getByRole('button', { name: 'Stop and send', exact: true }).click()
 await expect.poll(() => changes.some(c => c.action === 'stop')).toBe(true)
 queue[0].state = 'uncertain'; queue[0].note = 'Check the conversation before retrying.'; queue[0].revision++
 const retry = region.getByRole('button', { name: 'Retry', exact: true })
 await expect(retry).toBeDisabled()
 await region.getByRole('checkbox').check()
 await expect(retry).toBeEnabled()
 await retry.click()
 expect(changes.at(-1).acknowledge).toBe(true)
 for (const theme of ['light', 'dark']) {
  await page.evaluate(theme => { document.documentElement.dataset.theme = theme }, theme)
  for (const width of [1360, 390]) {
   await page.setViewportSize({ width, height: 900 })
   await expect(region).toBeVisible()
   const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)
   expect(overflow).toBe(false)
   // The single floating entry stays in its reserved corner, clear of growing
   // composer docks, the durable queue and live controls.
   await composer.fill('Keep the scope agreed.\n'.repeat(8))
   const entry = page.getByRole('button', { name: 'Ask Gaffer', exact: true })
   await expect(entry).toBeVisible()
   await expect.poll(() => page.evaluate(() => {
    const button = document.querySelector('.gaffer-launcher')?.getBoundingClientRect()
    const docks = [...document.querySelectorAll('[data-floating-clearance]')].map(node => node.getBoundingClientRect()).filter(r => r.width && r.height)
    return !!button && docks.length > 0 && button.top >= 0 && button.bottom<=innerHeight && docks.every(dock => button.bottom <= dock.top || button.top >= dock.bottom || button.right <= dock.left || button.left >= dock.right)
   })).toBe(true)
   await expect(page.locator('.gaffer-launcher')).toHaveCount(1)
   await composer.fill('')
   await page.screenshot({ path: `${process.env.REET_SCREENSHOT_DIR || "/tmp"}/live-chat-${theme}-${width}.png`, fullPage: true })
  }
 }
 expect(errors).toEqual([])
 console.log('Live controls and durable queue UI passed.')
} finally { await context.close(); await browser.close() }
