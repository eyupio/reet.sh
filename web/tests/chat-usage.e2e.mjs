import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

// A chat's status line says what its agent reports using: how much of its context window is left, or the chat's tokens
// when the agent never said how large the window is. Nothing is shown for an agent that has reported nothing.
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-usage00000000001', S = 'session-usage00000000001', root = `/api/v2/terminal/sessions/${S}/chat`
const now = Date.now()
const session = { id: S, agentId: A, title: 'Usage', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'codex', model: 'gpt-5-codex', effort: 'low', yolo: false, projectId: null, dir: '/tmp', structured: true }
const machine = { agentId: A, host: 'usage-machine', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', acp: true, tools: ['codex'] } }
const none = { reported: false, inputTokens: 0, outputTokens: 0, cachedTokens: 0, contextUsed: 0, contextSize: 0, updatedAt: 0 }
let usage = none, reads = 0, fail = false
// What the machine last read of its agents' plans: the session window has most of its allowance, the week less.
let planReads = 0
const plans = { state: 'read', sentence: '', planLimits: true, checkedAt: now, accounts: [{ tool: 'codex', state: 'signed_in', method: 'subscription', account: 'dev@example.test', plan: 'plus', limitsState: 'read', limitsAt: now, limits: [{ kind: 'session', usedPercent: 19, resetsAt: now + 130 * 60_000 }, { kind: 'week', usedPercent: 60, resetsAt: now + 3 * 86_400_000 + 3_600_000 }] }] }
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { page, context, errors } = await openWorkspace(browser, {
 width: 1360, height: 900, terminalStatus: active, terminalMachines: [machine],
 routes: {
  '/api/v2/terminal/sessions': { sessions: [session] },
  [`/api/v2/machines/${A}/accounts`]: () => { planReads++; return plans },
  [`${root}/usage`]: () => { reads++; return fail ? { status: 500, json: { error: 'the chat\'s usage could not be read' } } : usage },
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
let seq = 0
const status = state => page.evaluate(([seq, state]) => window.__chatStream.emit('chat', { seq, kind: 'status', state }), [++seq, state])
async function open() {
 seq = 0
 await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
 await page.waitForFunction(() => window.__chatStream)
 await page.evaluate(() => window.__chatStream.emit('reset', { truncated: false }))
}
try {
 // An agent that has said nothing: no figure, and no zero in its place.
 await open()
 const line = page.locator('[data-chat-usage]')
 await expect(page.getByLabel('Conversation status')).toBeVisible()
 await expect.poll(() => reads).toBeGreaterThan(0)
 await expect(line).toHaveCount(0)

 // The turn ends and the agent has reported: the line appears without a reload.
 await status('working')
 const before = reads
 usage = { reported: true, inputTokens: 1300, outputTokens: 250, cachedTokens: 5000, contextUsed: 90000, contextSize: 200000, updatedAt: now }
 await status('idle')
 await expect.poll(() => reads).toBeGreaterThan(before)
 await expect(line).toContainText('Context 55% left')
 await expect(line).toHaveAttribute('title', 'Codex reports 110k of its 200k-token context window free. This chat has used 1,550 tokens, and 5,000 cached.')
 await expect(line).not.toHaveClass(/low/)
 await expect(line.locator('.usage-meter > span')).toHaveAttribute('style', /width:\s*55%/)

 // A failed read keeps the agent's last word.
 await status('working')
 fail = true
 const failedFrom = reads
 await status('idle')
 await expect.poll(() => reads).toBeGreaterThan(failedFrom)
 await expect(line).toContainText('Context 55% left')
 fail = false

 // A nearly full window is marked.
 await status('working')
 usage = { ...usage, contextUsed: 180000 }
 await status('idle')
 await expect(line).toContainText('Context 10% left')
 await expect(line).toHaveClass(/low/)

 // An agent that reports tokens but never its window shows the tokens, with no meter.
 usage = { reported: true, inputTokens: 84000, outputTokens: 600, cachedTokens: 0, contextUsed: 0, contextSize: 0, updatedAt: now }
 await open()
 await expect(line).toContainText('85k tokens')
 await expect(line.locator('.usage-meter')).toHaveCount(0)

 usage = { reported: true, inputTokens: 1300, outputTokens: 250, cachedTokens: 5000, contextUsed: 90000, contextSize: 200000, updatedAt: now }
 await open()
 await expect(line).toContainText('Context 55% left')
 for (const theme of ['light', 'dark']) {
  await page.evaluate(theme => { document.documentElement.dataset.theme = theme }, theme)
  for (const width of [1360, 390, 320]) {
   await page.setViewportSize({ width, height: 900 })
   await expect(line).toBeVisible()
   // The page changes between its wide and its phone arrangement a moment after the window does, so each
   // measurement is of the arrangement it settles in, not of the moment between.
   await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
   // The figure stays inside the status line at every width.
   await expect.poll(() => page.evaluate(() => {
    const strip = document.querySelector('.conversation-status').getBoundingClientRect(), chip = document.querySelector('[data-chat-usage]').getBoundingClientRect()
    return chip.left >= strip.left && chip.right <= strip.right + 0.5 && chip.top >= strip.top && chip.bottom <= strip.bottom + 0.5
   }), { message: `the usage figure inside the status line at ${width}px, ${theme}` }).toBe(true)
   // Nothing floats over it: from one end to the other, the element on top is the figure itself.
   await expect.poll(() => page.evaluate(() => {
    const chip = document.querySelector('[data-chat-usage]'), box = chip.getBoundingClientRect(), y = box.top + box.height / 2
    return [box.left + 3, box.left + box.width / 2, box.right - 3].every(x => chip.contains(document.elementFromPoint(x, y)))
   }), { message: `nothing over the usage figure at ${width}px, ${theme}` }).toBe(true)
   if (process.env.REET_UI_SCREENSHOTS) await page.screenshot({ path: `${process.env.REET_UI_SCREENSHOTS}/chat-usage-${theme}-${width}.png` })
  }
 }
 // A machine that does not read sign-ins is never asked for plan limits, and the line shows none.
 await expect(page.locator('[data-plan-left]')).toHaveCount(0)
 expect(planReads).toBe(0)
 // One that does: what is left of the agent's plan sits beside the context left, naming the window with the least
 // left. Every window and when it starts again is in the title, and the figure stays inside the line on a phone.
 machine.capabilities.agentAccounts = true
 await page.setViewportSize({ width: 1360, height: 900 })
 await open()
 const plan = page.locator('.conversation-status [data-plan-left]'), chips = plan.locator('button.strip')
 // The chip is the agent's mark and the share left of its tightest window; its name says the rest.
 await expect(chips.locator('.value')).toHaveText('40%')
 await expect(chips).toHaveAccessibleName(/^Plan limits\. Codex: Session 81% left · Week 40% left\. Open for every limit/)
 await expect(plan.locator('.chip')).not.toHaveClass(/low/)
 await expect(line).toContainText('Context 55% left')
 expect(planReads).toBeGreaterThan(0)
 // A mouse over the chip says every window without a click, and the tip never takes a click from what is under it.
 const tip = plan.getByRole('tooltip')
 await expect(tip).toBeHidden()
 await chips.hover()
 await expect(tip).toBeVisible()
 await expect(tip).toContainText('Codex');await expect(tip).toContainText('Session 81% left · Week 40% left')
 expect(await tip.evaluate(node => getComputedStyle(node).pointerEvents)).toBe('none')
 await page.mouse.move(5, 5)
 await expect(tip).toBeHidden()
 // A click opens every limit with its reset, the account and where it was read; Escape closes and returns focus.
 await chips.click()
 const panel = page.getByRole('dialog', { name: 'Plan limits', exact: true })
 await expect(panel).toBeVisible()
 await expect(tip).toBeHidden()
 await expect(chips).toHaveAttribute('aria-expanded', 'true')
 const account = panel.getByRole('region', { name: 'Codex, dev@example.test', exact: true })
 await expect(account).toContainText('Plus');await expect(account).toContainText('dev@example.test')
 await expect(account.getByRole('meter', { name: 'Session left', exact: true })).toHaveAttribute('aria-valuenow', '81')
 await expect(account.getByRole('meter', { name: 'Week left', exact: true })).toHaveAttribute('aria-valuenow', '40')
 await expect(account).toContainText(/Resets in 2 h \d+ min/);await expect(account).toContainText('Resets in 3 days')
 await expect(account).toContainText('Read on usage-machine just now.')
 // The panel is inside the window whatever is around the chip.
 expect(await panel.evaluate(node => { const box = node.getBoundingClientRect(); return box.left >= 0 && box.top >= 0 && box.right <= innerWidth && box.bottom <= innerHeight })).toBe(true)
 await page.keyboard.press('Escape')
 await expect(panel).toBeHidden()
 await expect(chips).toBeFocused()
 await expect(chips).toHaveAttribute('aria-expanded', 'false')
 for (const width of [390, 320]) {
  await page.setViewportSize({ width, height: 900 })
  await expect(chips).toBeVisible()
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await expect.poll(() => page.evaluate(() => {
   const strip = document.querySelector('.conversation-status').getBoundingClientRect()
   return [...document.querySelectorAll('[data-chat-usage],.conversation-status [data-plan-left]')].every(node => { const chip = node.getBoundingClientRect(); return chip.left >= strip.left && chip.right <= strip.right + 0.5 && chip.top >= strip.top && chip.bottom <= strip.bottom + 0.5 })
  }), { message: `both figures inside the status line at ${width}px` }).toBe(true)
  if (process.env.REET_UI_SCREENSHOTS) await page.screenshot({ path: `${process.env.REET_UI_SCREENSHOTS}/chat-plan-${width}.png` })
 }
 expect(errors).toEqual([])
 console.log('Chat usage line passed.')
} finally { await context.close(); await browser.close() }
