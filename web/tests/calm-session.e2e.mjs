// The header stays small while existing actions and saved Details remain reachable.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now(), sessionId = 'calm-session-001'
const session = { id: sessionId, agentId: 'a1', title: 'Make payment retries safe', state: 'running', createdAt: now - 60000, updatedAt: now, exitCode: null, tool: 'opencode', model: 'provider/model', dir: '/work/checkout-api', structured: true, acpSessionId: 'ses_example000000000000001', attention: 'idle', attentionSource: 'chat' }
const machine = { agentId: 'a1', host: 'studio-mac', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, os: 'linux', osUser: 'dev', terminalEnabled: true, terminalControl: true, launch: true, tools: ['opencode'], acp: true, acpHistory: true, acpApproval: true, sessionMode: true } }
async function open(width, theme, saved, native = false) {
 const writes = []
 const w = await openWorkspace(browser, { width, height: 960, touch: width < 700, theme, routes: {
  '/api/v2/terminal/status': { configured: true, available: true, reason: '' },
  '/api/v2/terminal/sessions': { sessions: [session] },
  '/api/v2/terminal/machines': { machines: [{...machine,capabilities:{...machine.capabilities,nativeView:native}}] },
  [`/api/v2/terminal/sessions/${sessionId}/native-view`]: {available:native},
  [`/api/v2/terminal/sessions/${sessionId}/seen`]: {},
  [`/api/v2/terminal/sessions/${sessionId}/skills`]: { skills: [], host: machine.host },
  [`/api/v2/terminal/sessions/${sessionId}/prompt`]: { prompt: 'Make payment retries safe.' },
  [`/api/v2/terminal/sessions/${sessionId}/mode`]: request => { writes.push(request.postDataJSON()); return { status: 409, json: { error: 'Fixture does not restart an agent.' } } },
  '/api/v2/yolo': { allowed: false },
 } })
 await w.page.addInitScript(saved => {
  if (saved !== undefined) localStorage.setItem('reet-terminal-layout:1:context', JSON.stringify(saved))
  class Stream {
   constructor() { this.listeners = {}; window.__calmStream = this; setTimeout(() => { this.onopen?.({}); this.emit('reset', { truncated: false }); this.emit('chat', { seq: 1, kind: 'agent', messageId: 'a1', text: 'The payment handler is ready to review.' }); this.emit('chat', { seq: 2, kind: 'status', state: 'idle' }) }, 0) }
   addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn) }
   emit(type, data) { for (const fn of this.listeners[type] ?? []) fn({ data: JSON.stringify(data) }) }
   close() {}
  }
  window.EventSource = Stream
 }, saved)
 await w.page.goto(`${base}/sessions?agent=a1&session=${sessionId}`)
 await expect(w.page.locator('.session-screen .title')).toHaveText(session.title)
 return { ...w, writes }
}
try {
 for (const theme of ['dark', 'light']) for (const width of [1440, 768, 390, 320]) {
  const { page, context, errors, writes } = await open(width, theme)
  const screen = page.locator('.session-screen'), head = screen.locator('.head')
  const exit = page.getByRole('button', { name: 'Exit focus mode', exact: true })
  if (await exit.count()) await exit.click()
  const details = page.getByRole('button', { name: 'Session details panel', exact: true })
  await expect(details).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByRole('complementary', { name: 'Session context', exact: true })).toBeHidden()
  await page.locator('.composer-input').fill('Keep this draft while inspecting the session')
  await page.evaluate(() => window.__originalCalmStream = window.__calmStream)
  const actions = head.getByRole('button', { name: 'Session actions', exact: true })
  await actions.click()
  const sheet = page.getByRole('dialog', { name: 'Session actions', exact: true })
  await expect(sheet.getByRole('button', { name: 'Rename…', exact: true })).toBeVisible()
  await expect(sheet.getByRole('button', { name: 'End session…', exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(sheet).toHaveCount(0)
  await expect(actions).toBeFocused()
  await expect(page.locator('.composer-input')).toHaveValue('Keep this draft while inspecting the session')
  expect(await page.evaluate(() => window.__originalCalmStream === window.__calmStream)).toBe(true)
  expect(writes).toEqual([])
  await details.click()
  await expect(details).toHaveAttribute('aria-expanded', 'true')
  const panel = page.getByRole('complementary', { name: 'Session context', exact: true })
  await expect(panel).toBeVisible()
  await panel.getByRole('button', { name: /Close details|Collapse session workspace/ }).click()
  await expect(details).toBeFocused()
  await expect(details).toHaveAttribute('aria-expanded', 'false')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  if (process.env.REET_UI_SCREENSHOTS && [1440, 390].includes(width)) {
   await mkdir(process.env.REET_UI_SCREENSHOTS, { recursive: true })
   await page.screenshot({ path: `${process.env.REET_UI_SCREENSHOTS}/session-${theme}-${width}.png` })
  }
  expect(errors).toEqual([])
  await context.close()
 }
 // A view switch keeps the active worker, chat stream, draft and terminal attachment alive.
 for (const width of [1440,390]) {
  const w=await open(width,'dark',undefined,true),{page}=w
  const requests=[],inputs=[];let sockets=0
  await page.route(`**/api/v2/terminal/sessions/${sessionId}/attachments`,async route=>{requests.push(route.request().postDataJSON());await route.fulfill({json:{ticket:'native-fixture-ticket'}})})
  await page.routeWebSocket('**/api/v2/terminal/socket',socket=>{sockets++;socket.onMessage(message=>{if(typeof message==='string'){const body=JSON.parse(message);if(body.ticket){socket.send(JSON.stringify({type:'ready'}));socket.send(Buffer.from('\x1b[2J\x1b[HOpenCode native TUI\r\nSame running agent> '))}}else inputs.push(message.toString())})})
  await page.waitForFunction(()=>window.__calmStream)
  await page.locator('.composer-input').fill('A draft kept across native views')
  await page.evaluate(()=>{window.__beforeViewStream=window.__calmStream;window.__beforeViewField=document.querySelector('.composer-input');window.__calmStream.emit('chat',{seq:3,kind:'status',state:'working'})})
  const toggle=page.getByRole('group',{name:'Session view',exact:true})
  const terminal=toggle.getByRole('button',{name:'Terminal',exact:true}),chat=toggle.getByRole('button',{name:'Chat',exact:true})
  await expect(terminal).not.toHaveAttribute('aria-disabled','true')
  await terminal.click()
  const nativePane=page.locator('[data-session-presentation=terminal] .pane')
  await expect(nativePane).toHaveAttribute('data-connection-state','live')
  await expect(page.locator('[data-session-presentation=chat]')).toBeHidden()
  await chat.click()
  await expect(page.locator('.composer-input')).toHaveValue('A draft kept across native views')
  expect(await page.evaluate(()=>window.__beforeViewStream===window.__calmStream&&window.__beforeViewField===document.querySelector('.composer-input'))).toBe(true)
  await terminal.click();await expect(nativePane).toHaveAttribute('data-connection-state','live')
  expect(requests).toHaveLength(1);expect(requests[0].nativeView).toBe(true);expect(sockets).toBe(1);expect(w.writes).toEqual([])
  expect(w.errors).toEqual([]);await w.context.close()
 }
 // A saved desktop choice overrides the quieter default.
 const { page, context, errors } = await open(1600, 'dark', { open: true, width: 400 })
 await expect(page.getByRole('button', { name: 'Session details panel', exact: true })).toHaveAttribute('aria-expanded', 'true')
 await expect(page.getByRole('complementary', { name: 'Session context', exact: true })).toBeVisible()
 expect(errors).toEqual([])
 await context.close()
 console.log('Calm session: saved Details, action focus, stable conversation/draft and 8 theme/width combinations passed.')
} finally { await browser.close() }
