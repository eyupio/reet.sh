// AGY's native ACP integration through the existing Gaffer UI. All API answers
// and streamed events are synthetic; no coding agent or provider is started.
import { chromium, expect } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-agy000000000001', S = 'session-agy00000000000001'
const name = 'Google Antigravity (AGY)', now = Date.now()
const login = 'Sign in to Google Antigravity on that machine first (run agy).'
const descriptor = { tool: 'antigravity', adapter: 'agy', state: 'available', minimumVersion: '1.5.0', history: true, images: true, model: true, approval: true, options: [{ id: 'thought_level', category: 'thought_level', values: ['low', 'medium', 'high'] }] }
mkdirSync('work', { recursive: true })
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
try {
 for (const width of [1360, 320]) for (const theme of ['light', 'dark']) {
  const machine = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, agentsV2: true, tools: ['antigravity'], acp: true, acpHistory: true, acpImages: true, acpModel: true, acpApproval: true, acpLiveControls: true, acpStartOptions: true, acpAgents: [] } }
  const session = { id: S, agentId: A, title: name, state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'antigravity', model: 'gemini-4-argon', effort: 'medium', yolo: false, projectId: null, dir: '/home/dev', structured: true }
  const { context, page, state, errors } = await openWorkspace(browser, {
   width, height: 900, theme, touch: width < 600, terminalStatus: active, terminalMachines: [machine],
   routes: {
    '/api/v2/terminal/sessions': req => {
     if (req.method() === 'POST') {
      state.create = req.postDataJSON()
      if (state.failLogin) return { status: 422, json: { code: 'acp_unavailable', error: login, detail: login } }
      state.created = true
      return { ...session, title: state.create.title }
     }
     return { sessions: state.created ? [session] : [] }
    },
    '/api/v2/settings/yolo': { allowed: false },
    [`/api/v2/terminal/sessions/${S}/chat/controls`]: { controls: { configuring: false, working: false, waiting: false, compacting: false, steering: false, model: 'gemini-4-argon', options: [{ id: 'model', category: 'model', current: 'gemini-4-argon', values: ['gemini-4-argon'] }, { id: 'thought_level', category: 'thought_level', current: 'medium', values: ['low', 'medium', 'high'] }] } },
    [`/api/v2/terminal/sessions/${S}/chat/queue`]: { queue: [] },
    [`/api/v2/terminal/sessions/${S}/chat/permissions/p1`]: req => { state.allow = req.postDataJSON(); return { answered: true } },
    [`/api/v2/terminal/sessions/${S}/chat/permissions/p2`]: req => { state.deny = req.postDataJSON(); return { answered: true } },
   },
  })
  try {
   await page.addInitScript(() => {
    class FakeEventSource {
     constructor(url) { this.url = String(url); this.listeners = {}; this.closed = false; if (this.url.includes('/chat')) window.__chatStream = this; setTimeout(() => this.onopen?.({}), 0) }
     addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn) }
     close() { this.closed = true }
     emit(type, data) { for (const fn of this.listeners[type] ?? []) fn({ data: JSON.stringify(data) }) }
    }
    window.EventSource = FakeEventSource
   })
   // An old/terminal-only connector never gains Gaffer through legacy flags.
   await page.goto(`${base}/sessions?agent=${A}`)
   await page.getByRole('button', { name: width < 600 ? /Start a new agent session/ : 'New agent session', exact: width >= 600 }).first().click()
   const dialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
   await dialog.getByRole('combobox', { name: 'Agent', exact: true }).selectOption('antigravity')
   await expect(dialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveCount(0)
   await dialog.getByRole('button', { name: 'Close', exact: true }).click()

   machine.capabilities.acpAgents = [descriptor]
   await page.goto(`${base}/sessions?agent=${A}`)
   await page.getByRole('button', { name: width < 600 ? /Start a new agent session/ : 'New agent session', exact: width >= 600 }).first().click()
   await expect(dialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveValue('reet')
   await expect(dialog.getByText('AGY workspace hooks and customizations may run.', { exact: false })).toBeVisible()
   await expect(dialog.getByText('.opencode', { exact: false })).toHaveCount(0)
   await dialog.getByRole('button', { name: /^Model:/ }).click()
   await page.getByRole('listbox', { name: 'Model', exact: true }).getByRole('option', { name: 'Other…', exact: true }).click()
   await page.getByRole('textbox', { name: 'Model name', exact: true }).fill('gemini-4-argon')
   await page.getByRole('button', { name: 'Use', exact: true }).click()
   await dialog.getByRole('combobox', { name: 'Effort', exact: true }).selectOption('medium')
   await dialog.getByRole('textbox', { name: /Opening prompt/ }).fill('hello')
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
   state.failLogin = true
   await dialog.getByRole('button', { name: 'Start', exact: true }).click()
   await expect(dialog.getByRole('status')).toContainText(login)
   const refused = state.create.requestId
   state.failLogin = false
   await dialog.getByRole('button', { name: 'Start', exact: true }).click()
   await page.waitForURL(new RegExp(`session=${S}`))
   expect(state.create.requestId).not.toBe(refused)
   expect(state.create).toMatchObject({ tool: 'antigravity', structured: true, trust: true, model: 'gemini-4-argon', effort: 'medium', prompt: 'hello' })
   expect(state.create).not.toHaveProperty('yolo')
   await expect(page.getByRole('textbox', { name: `Message ${name}`, exact: true })).toBeVisible()
   await page.waitForFunction(() => window.__chatStream?.url.includes('/chat') && !window.__chatStream.closed && window.__chatStream.listeners.chat?.length)
   await page.evaluate(() => {
    window.__chatStream.emit('reset', { truncated: false })
    window.__chatStream.emit('chat', { seq: 1, kind: 'user', text: 'hello' })
    window.__chatStream.emit('chat', { seq: 2, kind: 'agent', messageId: 'm1', text: 'AGY is connected.' })
   })
   await expect(page.locator('.prose').filter({ hasText: 'AGY is connected.' })).toBeVisible()
   await page.evaluate(() => window.__chatStream.emit('chat', { seq: 3, kind: 'permission', permissionId: 'p1', toolCallId: 'c1', title: 'Run the tests', toolKind: 'execute', command: 'go test ./...' }))
   await expect(page.locator('.approval .command')).toContainText('go test ./...')
   await page.getByRole('button', { name: 'Allow once', exact: true }).click()
   await expect.poll(() => state.allow).toMatchObject({ allow: true })
   await page.evaluate(() => {
    window.__chatStream.emit('chat', { seq: 4, kind: 'permission_resolved', permissionId: 'p1', allow: true })
    window.__chatStream.emit('chat', { seq: 5, kind: 'permission', permissionId: 'p2', toolCallId: 'c2', title: 'Edit notes', toolKind: 'edit', path: '/home/dev/notes.txt', diff: '--- notes.txt\n+++ notes.txt\n+hello\n' })
   })
   await expect(page.locator('.approval .approval-path')).toContainText('notes.txt')
   await page.screenshot({ path: `work/agy-gaffer-${width}-${theme}.png`, fullPage: true })
   await page.getByRole('button', { name: 'Deny', exact: true }).click()
   await expect.poll(() => state.deny).toMatchObject({ allow: false })
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
   expect(errors).toEqual([])
   console.log(`AGY ${width}px ${theme} passed`)
  } catch (error) {
   await page.screenshot({ path: `work/agy-gaffer-${width}-${theme}-failure.png`, fullPage: true })
   console.error((await page.locator('body').innerText()).slice(0, 8000))
   throw error
  } finally { await context.close() }
 }
 console.log('AGY Gaffer UI passed on desktop and phone in both themes: descriptor gating, folder trust, model/effort, missing login and retry, streaming, Allow once and Deny.')
} finally { await browser.close() }
