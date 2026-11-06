// Real workspace layout with scripted worker events; no provider or live session is contacted.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const shots = process.env.REET_UI_SCREENSHOTS
if (shots) await mkdir(shots, { recursive: true })
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now()
const session = { id: 'session-gaffer-layout', agentId: 'a1', title: 'Polish the session workspace', structured: true, state: 'running', tool: 'opencode', model: 'worker-model', dir: '/work/reet.sh', createdAt: now - 90000, updatedAt: now, activityAt: now, exitCode: null }
const machine = { agentId: 'a1', host: 'Workshop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, os: 'linux', osUser: 'dev', terminalEnabled: true, terminalControl: true, readiness: 'ready', acp: true, acpHistory: true, inspect: true } }

try {
  for (const theme of ['dark', 'light']) for (const width of [1600, 1280, 768, 375]) {
    let providerReads = 0
    const routes = {
      '/api/v2/terminal/status': { configured: true, available: true },
      '/api/v2/terminal/machines': { machines: [machine] },
      '/api/v2/terminal/sessions': { sessions: [session] },
      '/api/v2/machines/health': { serverNow: now, machines: [{ ...machine, state: 'connected', connection: { connected: true, reportFresh: true, contactAt: now, reportAt: now } }] },
      '/api/v2/folder-projects': { projects: [], savedWaiting: 0 },
      '/api/v2/projects': { projects: [] },
      '/api/v2/yolo': { allowed: false },
      [`/api/v2/terminal/sessions/${session.id}/seen`]: {},
      [`/api/v2/terminal/sessions/${session.id}/skills`]: { skills: [], host: 'Workshop' },
      [`/api/v2/terminal/sessions/${session.id}/inspect`]: { checkedAt: now, repo: true, branch: 'feature/gaffer-layout', changed: 2, ahead: 0, behind: 0, conflicts: 0, files: [], reason: 'no_pr' },
      '/api/v2/gaffer/providers': () => { providerReads++; return { providers: [], presets: [], available: true } },
      '/api/v2/gaffer/conversations': { status: 404, json: { error: 'No companion conversation' } },
    }
    const { page, context, errors } = await openWorkspace(browser, { width, height: 1000, theme, touch: width < 800, routes })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.addInitScript(() => {
      window.__workerStreams = 0
      class Stream {
        constructor() {
          this.listeners = {}; window.__layoutStream = this; window.__workerStreams++
          setTimeout(() => {
            this.onopen?.(); this.emit('reset', { truncated: false })
            this.emit('chat', { seq: 1, kind: 'user', text: 'Move Gaffer into Overview and keep useful live status beside the conversation.' })
            this.emit('chat', { seq: 2, kind: 'agent', messageId: 'plan', text: 'I’m refining the workspace layout. The conversation will keep a clear live status, with Gaffer’s detailed updates in Overview.' })
            this.emit('chat', { seq: 3, kind: 'tool', toolCallId: 'read', toolKind: 'read', title: 'Review the chat layout', status: 'completed', path: '/work/reet.sh/web/src/lib/terminal/ChatPane.svelte' })
            this.emit('chat', { seq: 4, kind: 'tool', toolCallId: 'edit', toolKind: 'edit', title: 'Refine the conversation layout', status: 'in_progress', path: '/work/reet.sh/web/src/lib/terminal/ChatStatus.svelte' })
            this.emit('chat', { seq: 5, kind: 'status', state: 'working' })
          }, 0)
        }
        addEventListener(type, listener) { (this.listeners[type] ??= []).push(listener) }
        emit(type, data) { for (const listener of this.listeners[type] ?? []) listener({ data: JSON.stringify(data) }) }
        close() {}
      }
      window.EventSource = Stream
    })
    await page.goto(`${base}/sessions?agent=a1&session=${session.id}`)
    const narrative = page.locator('.chat-pane')
    const strip = narrative.locator('[aria-label="Conversation status"]')
    const entry = narrative.getByRole('button', { name: 'Ask Gaffer', exact: true })
    const overview = page.getByRole('complementary', { name: 'Session context' })
    const card = overview.getByRole('region', { name: 'Gaffer’s work update' })
    await expect(strip).toHaveAttribute('data-phase', 'editing')
    expect((await strip.boundingBox()).height).toBeGreaterThanOrEqual(38)
    expect((await strip.boundingBox()).width).toBeGreaterThan(200)
    expect((await entry.boundingBox()).height).toBeGreaterThanOrEqual(width < 800 ? 44 : 36)
    expect(await entry.evaluate(el => getComputedStyle(el.parentElement).clip)).toBe('auto')
    await expect(strip).toContainText('1 done · 1 active')
    await expect(strip.locator('.spinning svg')).toBeVisible()
    expect(await strip.locator('.spinning svg').evaluate(el => getComputedStyle(el).animationName)).toBe('none')
    await expect(narrative.locator('.gaffer-avatar')).toHaveCount(0)
    await expect(page.locator('.gaffer-launcher')).toHaveCount(0)
    expect(providerReads).toBe(0)
    await entry.focus(); await page.keyboard.press('Enter')
    const companion = page.getByRole('dialog', { name: 'Gaffer', exact: true })
    await expect(companion).toBeVisible()
    await expect(companion.getByRole('link', { name: 'Set up Gaffer' })).toBeVisible()
    if (width >= 1100) {
      const edge = (await narrative.boundingBox()).x + (await narrative.boundingBox()).width
      const panel = await companion.boundingBox()
      expect(Math.abs(panel.x + panel.width - (edge - 12))).toBeLessThan(3)
    }
    await page.keyboard.press('Escape'); await expect(companion).toHaveCount(0); await expect(entry).toBeFocused()
    if (shots && width === 375) await page.screenshot({ path: `${shots}/gaffer-narrative-${width}-${theme}.png` })
    if (!await overview.isVisible()) {
      const exit = page.getByRole('button', { name: 'Exit focus mode', exact: true })
      if (await exit.count()) await exit.click()
      await page.getByRole('button', { name: 'Session details panel', exact: true }).click()
    }
    await expect(card).toBeVisible()
    await expect(card).toHaveAttribute('data-phase', 'editing')
    expect(await card.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
    const expand = card.getByRole('button', { name: 'Expand Gaffer’s card' })
    if (width < 800) { const hit = await expand.boundingBox(); expect(hit.width).toBeGreaterThanOrEqual(44); expect(hit.height).toBeGreaterThanOrEqual(44) }
    await expand.click()
    expect(await card.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
    if (shots && (width === 1600 || width === 375)) await page.screenshot({ path: `${shots}/gaffer-layout-${width}-${theme}.png` })
    let seq = 10
    const emit = event => page.evaluate(e => window.__layoutStream.emit('chat', e), { seq: seq++, ...event })
    await emit({ kind: 'tool', toolCallId: 'edit', toolKind: 'edit', status: 'failed' })
    await expect(strip).toContainText('1 failed')
    await emit({ kind: 'permission', permissionId: 'allow', toolKind: 'execute', title: 'Run the browser checks', command: 'npm run check' })
    await expect(strip).toHaveAttribute('data-phase', 'approval')
    await expect(strip.locator('.spinning')).toHaveCount(0)
    await expect(card).toHaveAttribute('data-phase', 'approval')
    await emit({ kind: 'permission_resolved', permissionId: 'allow', allow: true })
    await emit({ kind: 'question', questionId: 'tone', question: { message: 'Which layout would you prefer?', fields: [{ key: 'layout', kind: 'text', title: 'Layout', required: true }] } })
    await expect(strip).toHaveAttribute('data-phase', 'question')
    await emit({ kind: 'question_resolved', questionId: 'tone', reason: 'decline' })
    await page.evaluate(() => window.__layoutStream.onerror?.())
    await expect(strip).toHaveAttribute('data-phase', 'reconnecting')
    await expect(card).toHaveAttribute('data-phase', 'reconnecting')
    await expect(strip.locator('.spinning')).toHaveCount(0)
    await page.evaluate(() => {
      const stream = window.__layoutStream
      stream.onopen?.(); stream.emit('reset', { truncated: false })
      stream.emit('chat', { seq: 1, kind: 'user', text: 'The same turn replayed after reconnecting.' })
      stream.emit('chat', { seq: 2, kind: 'tool', toolCallId: 'read', toolKind: 'read', status: 'completed' })
      stream.emit('chat', { seq: 3, kind: 'tool', toolCallId: 'edit', toolKind: 'edit', status: 'failed' })
    })
    seq = 4
    await emit({ kind: 'agent', messageId: 'handover', text: 'The new layout is ready. Review the tool failure before continuing.' })
    await emit({ kind: 'status', state: 'idle' })
    await expect(strip).toHaveAttribute('data-phase', 'idle')
    await expect(card).toHaveAttribute('data-phase', 'idle')
    await card.getByRole('button', { name: 'Job sheet', exact: true }).click()
    await expect(card.getByRole('region', { name: 'Gaffer’s job sheet' })).toContainText('1 tool call reported failure')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(await page.evaluate(() => window.__workerStreams)).toBe(1)
    expect(errors).toEqual([])
    await context.close()
  }
  console.log('Gaffer layout: Overview placement, mascot-free status, nearby launcher, focus return, eight responsive/theme checks, approvals, questions, reconnect and handover passed.')
} finally { await browser.close() }
