import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-live000000000001', S = 'session-live0000000000001', root = `/api/v2/terminal/sessions/${S}/chat`
const now = Date.now()
const session = { id: S, agentId: A, title: 'Live controls', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'opencode', model: 'provider/one', effort: 'low', yolo: false, projectId: null, dir: '/tmp', structured: true }
const machine = { agentId: A, host: 'live-machine', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', acp: true, acpModel: true, acpLiveControls: true, tools: ['opencode'] } }
let queue = [], changes = [], serial = 0, commandsAnswer = { commands: [] }, skillsAnswer = { skills: [], machineSeen: now - 60_000, host: 'live-machine' }
let controls = { working: true, waiting: false, compacting: false, configuring: false, steering: true, model: 'provider/one', options: [{ id: 'model', category: 'model', current: 'provider/one', values: ['provider/one', 'provider/two'] }, { id: 'effort', category: 'thought_level', current: 'low', values: ['low', 'high'] }] }
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { page, context, errors } = await openWorkspace(browser, {
 width: 1360, height: 900, terminalStatus: active, terminalMachines: [machine],
 routes: {
  '/api/v2/terminal/sessions': { sessions: [session] },
  [`${root}/commands`]: () => commandsAnswer,
  [`/api/v2/terminal/sessions/${S}/skills`]: () => skillsAnswer,
  [`/api/v2/terminal/sessions/${S}/native-view`]: { available: true },
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
   // On a desktop the single floating entry stays in its reserved corner, clear of growing composer docks, the durable
   // queue and live controls. A phone chat has no such corner, so its status row carries the entry instead.
   await composer.fill('Keep the scope agreed.\n'.repeat(8))
   const entry = page.getByRole('button', { name: 'Ask Gaffer', exact: true })
   await expect(entry).toBeVisible()
   if (width <= 767) {
    await expect(page.locator('.gaffer-launcher')).toBeHidden()
    expect((await entry.boundingBox()).height).toBeGreaterThanOrEqual(36)
   } else {
    await expect.poll(() => page.evaluate(() => {
     const button = document.querySelector('.gaffer-launcher')?.getBoundingClientRect()
     const docks = [...document.querySelectorAll('[data-floating-clearance]')].map(node => node.getBoundingClientRect()).filter(r => r.width && r.height)
     return !!button && docks.length > 0 && button.top >= 0 && button.bottom<=innerHeight && docks.every(dock => button.bottom <= dock.top || button.top >= dock.bottom || button.right <= dock.left || button.left >= dock.right)
    })).toBe(true)
    await expect(page.locator('.gaffer-launcher')).toHaveCount(1)
   }
   await composer.fill('')
   await page.screenshot({ path: `${process.env.REET_SCREENSHOT_DIR || "/tmp"}/live-chat-${theme}-${width}.png`, fullPage: true })
  }
 }

 // The agent's own commands in the Skills sheet, and the line for a command it did not list. Idle chat, so Skills and Send are on.
 const shots = process.env.REET_UI_SCREENSHOTS
 const shot = async name => { if (shots) await page.screenshot({ path: `${shots}/${name}.png` }) }
 const announced = [{ name: 'review', description: 'Review changes', hint: 'branch' }, { name: 'init', description: 'Create an AGENTS.md file', hint: '' }]
 const skillRows = [
  { id: 'm1', name: 'review', description: 'Review a branch as a skill', source: 'machine', agent: 'opencode', native: true, onMachine: true, invocation: '/review' },
  { id: 'm2', name: 'pdf', description: 'Read and fill forms', source: 'machine', agent: 'opencode', native: true, onMachine: true, invocation: '/pdf' },
 ]
 machine.capabilities.nativeView = true
 const sentTexts = []
 page.on('request', request => { if (request.method() === 'POST' && request.url().endsWith(`${root}/queue`)) sentTexts.push(request.postDataJSON().text) })
 const skillsButton = page.locator('.composer').getByRole('button', { name: 'Skills', exact: true })
 const picker = page.getByRole('dialog', { name: 'Skills', exact: true })
 const unlisted = page.locator('.notice.unlisted')
 // A lone `/word` opens the Skills sheet as the person types; Escape puts it away before Send, as a person would.
 async function sendDraft(composer, text) {
  await composer.fill(text)
  if (await picker.isVisible()) await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
 }
 async function openIdle(width, theme) {
  await page.setViewportSize({ width, height: 900 })
  await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
  await page.waitForFunction(() => window.__chatStream)
  await page.evaluate(theme => { document.documentElement.dataset.theme = theme }, theme)
  await page.evaluate(() => { window.__chatStream.emit('reset', { truncated: false }); window.__chatStream.emit('chat', { seq: 1, kind: 'status', state: 'idle' }) })
  // A phone opens a chat in focus, where Skills waits behind Exit focus.
  const exit = page.getByRole('button', { name: 'Exit focus mode', exact: true })
  if (width < 768 && await exit.isVisible({ timeout: 3000 }).catch(() => false)) await exit.click()
  await expect(skillsButton).toBeEnabled()
 }
 for (const [width, theme] of [[1360, 'light'], [390, 'dark']]) {
  const label = `${width}-${theme}`
  commandsAnswer = { commands: announced }; skillsAnswer = { ...skillsAnswer, skills: skillRows }
  await openIdle(width, theme)
  const composer = page.getByRole('textbox', { name: 'Message OpenCode', exact: true })
  await skillsButton.click()
  await expect(picker.getByText('Commands from OpenCode', { exact: true })).toBeVisible()
  const options = picker.getByRole('option')
  // The commands group comes first; a skill invoked the same way is shown once, in the commands group.
  await expect(options.first()).toContainText('/review')
  await expect(options.first()).toContainText('Review changes')
  await expect(options.first()).toContainText('branch')
  await expect(options).toHaveCount(3)
  await expect(picker.getByText('Review a branch as a skill')).toHaveCount(0)
  await expect(picker.getByRole('option', { name: /pdf/ })).toBeVisible()
  const box = await options.first().boundingBox()
  expect(box.height).toBeGreaterThanOrEqual(width < 500 ? 40 : 36)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await shot(`commands-sheet-${label}`)
  const filter = picker.getByRole('searchbox', { name: 'Filter skills' })
  await filter.fill('agents.md')
  await expect(options).toHaveCount(1)
  await expect(options.first()).toContainText('/init')
  await filter.fill('rev')
  const before = sentTexts.length
  await filter.press('ArrowDown')
  await page.keyboard.press('Enter')
  await expect(picker).toHaveCount(0)
  await expect(composer).toHaveValue('/review ')
  expect(sentTexts.length).toBe(before)
  // A command the agent did not list is still sent, with one line beside the composer that goes on the next edit.
  await sendDraft(composer, '/compact the thread')
  await expect.poll(() => sentTexts.at(-1)).toBe('/compact the thread')
  await expect(unlisted).toHaveText(/OpenCode did not list \/compact for Chat\. It may only work in its terminal\./)
  await expect(unlisted.getByRole('button', { name: 'Open terminal', exact: true })).toBeVisible()
  await shot(`unlisted-${label}`)
  await composer.fill('x')
  await expect(unlisted).toHaveCount(0)
  await sendDraft(composer, '/compact')
  await expect(unlisted).toBeVisible()
  await sendDraft(composer, '/review now')
  await expect.poll(() => sentTexts.at(-1)).toBe('/review now')
  await expect(unlisted).toHaveCount(0)
  // A path is not a command: it is sent and nothing is said.
  await sendDraft(composer, '/etc/hosts')
  await expect.poll(() => sentTexts.at(-1)).toBe('/etc/hosts')
  await expect(unlisted).toHaveCount(0)
  // Open terminal uses the session's own terminal view.
  await sendDraft(composer, '/compact')
  await unlisted.getByRole('button', { name: 'Open terminal', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Terminal', exact: true })).toHaveAttribute('aria-pressed', 'true')
  // The agent announced nothing: no group, and no line for any command.
  commandsAnswer = { commands: [] }
  await openIdle(width, theme)
  await skillsButton.click()
  await expect(picker.getByText(/^Commands from/)).toHaveCount(0)
  await expect(picker.getByRole('option')).toHaveCount(2)
  await page.keyboard.press('Escape')
  await sendDraft(composer, '/compact')
  await expect.poll(() => sentTexts.at(-1)).toBe('/compact')
  await expect(unlisted).toHaveCount(0)
  // A machine too old to report them: one muted line with the server's sentence, no group.
  const old = 'Update reet on this machine to see the agent’s commands.'
  commandsAnswer = { commands: [], code: 'chat_commands_unsupported', error: old }
  await openIdle(width, theme)
  await skillsButton.click()
  await expect(picker.getByText(old, { exact: true })).toBeVisible()
  await expect(picker.getByText(/^Commands from/)).toHaveCount(0)
  await shot(`commands-old-machine-${label}`)
  await page.keyboard.press('Escape')
 }
 expect(errors).toEqual([])
 console.log('Live controls and durable queue UI passed.')
} finally { await context.close(); await browser.close() }
