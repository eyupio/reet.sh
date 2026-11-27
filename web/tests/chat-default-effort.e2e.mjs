import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

// A live Claude Code chat saves its model and the effort it is running beside it as the agent's default, saves a changed
// effort again, and clears both. The effort is only offered for saving where the agent's launches take that level.
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-effort0000000001', S = 'session-effort00000000001', root = `/api/v2/terminal/sessions/${S}/chat`
const now = Date.now()
const session = { id: S, agentId: A, title: 'Claude Code · site', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'claude', model: 'opus', effort: '', yolo: false, projectId: null, dir: '/tmp', structured: true }
const claudeChat = { tool: 'claude', adapter: 'claude-agent-acp', state: 'available', minimumVersion: '0.99.1', model: true, approval: true }
const machine = { agentId: A, host: 'effort-machine', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, agentsV2: true, models: true, acp: true, acpModel: true, acpApproval: true, acpLiveControls: true, tools: ['claude'], acpAgents: [claudeChat] } }
const controls = { working: false, waiting: false, compacting: false, configuring: false, steering: true, model: 'opus', options: [{ id: 'model', category: 'model', current: 'opus', values: ['opus', 'sonnet'] }, { id: 'effort', category: 'thought_level', current: 'medium', values: ['default', 'low', 'medium', 'high', 'max', 'ultra'] }] }
const state = { favourites: [], calls: [] }
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { page, context, errors } = await openWorkspace(browser, {
 width: 1360, height: 900, terminalStatus: active, terminalMachines: [machine],
 routes: {
  '/api/v2/terminal/sessions': { sessions: [session] },
  [`${root}/queue`]: { queue: [] },
  [`${root}/commands`]: { commands: [] },
  [`/api/v2/terminal/sessions/${S}/skills`]: { skills: [], machineSeen: now - 60_000, host: 'effort-machine' },
  [`/api/v2/terminal/sessions/${S}/native-view`]: { available: false },
  [`${root}/controls`]: req => {
   if (req.method() === 'POST') { const body = req.postDataJSON(); controls.options.find(o => o.id === body.configId).current = body.value }
   return { controls }
  },
  '/api/v2/profile/model-favourites': req => {
   if (req.method() === 'POST') { state.calls.push(['star', req.postDataJSON()]); state.favourites = [{ ...req.postDataJSON(), agentDefault: false }]; return { version: 2, favourites: state.favourites } }
   return { version: 1, favourites: state.favourites }
  },
  '/api/v2/profile/model-favourites/default': req => {
   const body = req.postDataJSON(); state.calls.push(['default', body])
   state.favourites = state.favourites.map(f => body.model && f.model === body.model ? { ...f, agentDefault: true } : { ...f, agentDefault: false, effort: undefined })
   return { version: 3, favourites: state.favourites }
  },
  '/api/v2/profile/model-favourites/default-effort': req => {
   const body = req.postDataJSON(); state.calls.push(['effort', body])
   state.favourites = state.favourites.map(f => f.agentDefault ? { ...f, effort: body.effort || undefined } : f)
   return { version: 4, favourites: state.favourites }
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
try {
 await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
 await page.waitForFunction(() => window.__chatStream)
 await page.evaluate(() => { window.__chatStream.emit('reset', { truncated: false }); window.__chatStream.emit('chat', { seq: 1, kind: 'status', state: 'idle' }) })
 const effort = page.getByRole('combobox', { name: 'Effort', exact: true })
 await expect(effort).toHaveValue('medium')

 // Making the model the default saves the effort this chat is running beside it.
 await page.getByRole('button', { name: 'Make opus the default for Claude Code', exact: true }).click()
 await expect(page.getByText('New claude sessions use opus at medium effort.')).toBeVisible()
 expect(state.calls).toEqual([['star', { tool: 'claude', model: 'opus' }], ['default', { tool: 'claude', model: 'opus' }], ['effort', { tool: 'claude', effort: 'medium' }]])
 const pressed = page.getByRole('button', { name: 'Clear opus as the default for Claude Code', exact: true })
 await expect(pressed).toHaveAttribute('aria-pressed', 'true')

 // A changed effort is a different default again: the star lets go, and pressing it saves just the new level.
 await effort.selectOption('high')
 const again = page.getByRole('button', { name: 'Make opus the default for Claude Code', exact: true })
 await expect(again).toHaveAttribute('aria-pressed', 'false')
 state.calls.length = 0
 await again.click()
 await expect(page.getByText('New claude sessions use opus at high effort.')).toBeVisible()
 expect(state.calls.at(-1)).toEqual(['effort', { tool: 'claude', effort: 'high' }])
 await expect(pressed).toHaveAttribute('aria-pressed', 'true')

 // A level Claude Code's launches do not take ("ultra") is not saved: the model alone is the default.
 await effort.selectOption('ultra')
 await expect(pressed).toHaveAttribute('aria-pressed', 'true')

 // Clearing the default clears its effort with it.
 state.calls.length = 0
 await pressed.click()
 await expect(page.getByText('Cleared the default for claude.')).toBeVisible()
 expect(state.calls).toEqual([['default', { tool: 'claude', model: '' }]])
 expect(state.favourites.every(f => !f.agentDefault && !f.effort)).toBe(true)
 expect(errors).toEqual([])
 console.log('Chat default effort passed.')
} finally { await context.close(); await browser.close() }
