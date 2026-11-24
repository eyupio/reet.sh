// Unsent text on a phone: the chat draft, the chat's browser-held queue and the terminal's compose box survive a reload; the chat
// composer stays typeable while the stream reconnects (Send waits, with its reason, and nothing is sent twice or by itself);
// another account sees none of it and signing out clears it. Screenshots go to REET_SHOTS when it is set.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const shots = process.env.REET_SHOTS
const A = 'connector-drafts00000000001', S = 'session-drafts0000000000001', T = 'session-drafts0000000000002'
const now = Date.now()
const chatSession = { id: S, agentId: A, title: 'OpenCode · site', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'opencode', model: '', effort: '', yolo: false, projectId: null, dir: '/home/dev/work/site', structured: true }
const shellSession = { id: T, agentId: A, title: 'Refactor the store layer', state: 'running', tool: 'claude', model: 'sonnet', createdAt: now, updatedAt: now, exitCode: null }
const machine = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, tools: ['opencode', 'claude'] } }
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })

async function open(theme = 'light') {
  const prompts = []
  const opened = await openWorkspace(browser, { width: 390, height: 844, touch: true, theme, scale: 1, sessionFocus: false, terminalStatus: active, terminalMachines: [machine], routes: {
    '/api/v2/machines/health': () => ({ serverNow: Date.now(), machines: [machine].map(m => ({ ...m, state: 'connected', connection: { connected: true, reportFresh: true, missedProbes: 0, reconnects: 0, path: 'direct' } })) }),
    '/api/v2/terminal/sessions': { sessions: [chatSession, shellSession] }, '/api/v2/folder-projects': { projects: [], savedWaiting: 0 },
    '/api/v2/terminal/commands': { commands: [] }, '/api/v2/settings/yolo': { allowed: false },
    '/api/v2/terminal/worktrees/status': { path: '', branch: '', changed: 0, unpushed: 0, paths: [], unreferenced: 0, hidden: 0, nested: false, incomplete: false },
    [`/api/v2/terminal/sessions/${S}/chat/prompt`]: req => { prompts.push(req.postDataJSON().text); return { sent: true } },
  } })
  const { page } = opened
  await page.addInitScript(() => {
    class FakeEventSource {
      constructor() { this.listeners = {}; window.__chatStream = this; setTimeout(() => this.onopen?.({}), 0) }
      addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn) }
      close() {}
      emit(type, data) { for (const fn of this.listeners[type] ?? []) fn({ data: JSON.stringify(data) }) }
    }
    // Only the chat session's page reads a stream; the terminal page keeps the browser's own.
    if (location.search.includes('0000001')) window.EventSource = FakeEventSource
  })
  await page.routeWebSocket('**/api/v2/terminal/socket', ws => ws.onMessage(message => {
    if (typeof message === 'string' && JSON.parse(message).ticket) { ws.send('{"type":"ready"}'); ws.send(Buffer.from('ready> ')) }
  }))
  await page.route('**/attachments', route => route.fulfill({ json: { ticket: 'fixture-ticket00000000000' } }))
  await page.route('**/history', route => route.fulfill({ body: '' }))
  return { ...opened, prompts }
}
const chatUrl = `${base}/sessions?agent=${A}&session=${S}`
const stored = page => page.evaluate(() => localStorage.getItem('reet.drafts.v1'))
const composerOf = page => page.getByRole('textbox', { name: 'Message OpenCode' })
const sendOf = page => page.getByRole('button', { name: /^(Send|Queue)$/ })
const settle = page => page.evaluate(() => window.__chatStream.emit('reset', { truncated: false }))
const reload = async page => { await page.reload(); await expect(composerOf(page)).toBeVisible(); await settle(page) }

try {
  for (const theme of ['light', 'dark']) {
    const { page, prompts, context, errors } = await open(theme)
    await page.goto(chatUrl)
    const composer = composerOf(page)
    await expect(composer).toBeVisible(); await settle(page)

    // A draft survives a reload; it comes back without taking focus, and nothing is sent.
    await composer.fill('check the failing build')
    await expect.poll(() => stored(page)).toContain('check the failing build')
    await reload(page)
    await expect(composer).toHaveValue('check the failing build')
    await expect(composer).not.toBeFocused()
    // A restored draft says so, quietly, until the first keystroke.
    const earlier = page.getByText('Draft from earlier. Edit or send.')
    await expect(earlier).toBeVisible()
    if (shots) await page.screenshot({ path: `${shots}/restored-draft-${theme}.png` })
    await composer.focus(); await page.keyboard.type('!'); await page.keyboard.press('Backspace')
    await expect(earlier).toHaveCount(0)
    await expect(composer).toHaveValue('check the failing build')

    // The stream drops: the field stays typeable and keeps the text; Send waits and says why; nothing goes out by itself.
    await page.evaluate(() => window.__chatStream.onerror?.({}))
    await expect(composer).toBeEnabled()
    await expect(page.getByText('Reconnecting. Your message is kept. Send turns on when you are back.')).toBeVisible()
    await expect(sendOf(page)).toBeDisabled()
    await composer.focus(); await page.keyboard.type(' and then the tests')
    await page.keyboard.press('Enter')
    await expect(composer).toHaveValue('check the failing build and then the tests')
    await expect(composer).toHaveAttribute('aria-describedby', /-kept$/)
    if (shots) await page.screenshot({ path: `${shots}/reconnecting-${theme}.png` })
    expect(prompts).toEqual([])
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect((await sendOf(page).boundingBox()).height).toBeGreaterThanOrEqual(44)
    // Back again: the text is still there, still unsent, and Send works once.
    await page.evaluate(() => window.__chatStream.onopen?.({})); await settle(page)
    await expect(page.getByText('Reconnecting. Your message is kept. Send turns on when you are back.')).toHaveCount(0)
    await expect(composer).toHaveValue('check the failing build and then the tests')
    await page.waitForTimeout(300); expect(prompts).toEqual([])
    await expect(sendOf(page)).toBeEnabled(); await sendOf(page).click()
    await expect.poll(() => prompts).toEqual(['check the failing build and then the tests'])
    // Sent: nothing is kept, so a reload starts empty.
    await expect(composer).toHaveValue('')
    await expect.poll(() => stored(page)).toBeNull()
    await reload(page)
    await expect(composer).toHaveValue('')
    expect(prompts.length).toBe(1)

    if (theme === 'light') {
      // Queued messages (this agent's queue lives in the browser) come back after a reload and wait for Retry.
      await page.evaluate(() => window.__chatStream.emit('chat', { seq: 1, kind: 'status', state: 'working' }))
      await composer.fill('then update the changelog'); await sendOf(page).click()
      await composer.fill('and tidy the imports'); await sendOf(page).click()
      await expect(page.getByRole('region', { name: 'Queued messages' })).toContainText('then update the changelog')
      await expect.poll(() => stored(page)).toContain('and tidy the imports')
      await composer.fill('half typed')
      await expect.poll(() => stored(page)).toContain('half typed')
      await reload(page)
      const queue = page.getByRole('region', { name: 'Queued messages' })
      await expect(queue).toContainText('then update the changelog'); await expect(queue).toContainText('and tidy the imports')
      await expect(composer).toHaveValue('half typed')
      await expect(page.getByText('Paused after a reload. Retry sends it.')).toBeVisible()
      const retry = queue.getByRole('button', { name: 'Retry' })
      expect((await retry.boundingBox()).height).toBeGreaterThanOrEqual(44)
      // The restored draft says so here too, and the restored queue is paused, not sent.
      await expect(page.getByText('Draft from earlier. Edit or send.')).toBeVisible()
      await page.waitForTimeout(500); expect(prompts.length).toBe(1)
      await queue.getByRole('button', { name: 'Retry' }).click()
      await expect.poll(() => prompts).toEqual(['check the failing build and then the tests', 'then update the changelog'])
      await composer.fill('')

      // Another account's draft is never shown, one past its fourteen days is dropped, and rubbish in storage is ignored.
      await page.evaluate(() => localStorage.setItem('reet.drafts.v1', JSON.stringify([
        { u: 2, s: 'session-drafts0000000000001', at: Date.now(), chat: 'someone else' },
        { u: 1, s: 'session-drafts0000000000001', at: Date.now() - 15 * 864e5, chat: 'too old' }])))
      await reload(page)
      await expect(composer).toHaveValue('')
      await page.evaluate(() => localStorage.setItem('reet.drafts.v1', '{nope'))
      await reload(page)
      await expect(composer).toBeEnabled(); await expect(composer).toHaveValue('')

      // Storage that throws: the composer works as usual.
      await page.evaluate(() => { Storage.prototype.setItem = () => { throw new Error('full') }; Storage.prototype.getItem = () => { throw new Error('off') } })
      await composer.fill('no storage here'); await expect(composer).toHaveValue('no storage here')
      await page.waitForTimeout(600)

      // Signing out clears every draft.
      await page.reload(); await expect(composer).toBeVisible()
      await composer.fill('before I sign out')
      await expect.poll(() => stored(page)).toContain('before I sign out')
      await page.goto(`${base}/account/sessions`)
      await page.locator('.signout').getByRole('button', { name: 'Sign out', exact: true }).click()
      await expect.poll(() => stored(page)).toBeNull()
    }
    expect(errors).toEqual([])
    await context.close()
  }

  // The terminal's compose box: a draft survives a reload and is marked on the Compose key, without opening or taking focus.
  {
    const { page, context } = await open('light')
    await page.goto(`${base}/sessions?agent=${A}&session=${T}`)
    await expect(page.locator('.pane')).toHaveAttribute('data-connection-state', 'live')
    const key = page.locator('.terminal-toolbar').getByRole('button', { name: /^Compose/ })
    await expect(key).toHaveAccessibleName('Compose')
    await key.click()
    const sheet = page.getByRole('dialog', { name: 'Compose' }), field = sheet.getByLabel(/^Message for/)
    await field.fill('git log --oneline | head')
    await expect.poll(() => stored(page)).toContain('git log --oneline')
    await page.reload()
    await expect(page.locator('.pane')).toHaveAttribute('data-connection-state', 'live')
    await expect(sheet).toHaveCount(0)
    await expect(key).toHaveAccessibleName('Compose, unsent message kept')
    await expect(key).toHaveAttribute('data-kept', '')
    await key.click()
    await expect(field).toHaveValue('git log --oneline | head')
    await field.fill('')
    await expect.poll(() => stored(page)).toBeNull()
    await context.close()
  }
  console.log('Phone drafts: chat draft, queue and compose box kept across a reload; the composer typeable while reconnecting; other accounts, old drafts, broken storage and sign out')
} finally {
  await browser.close()
}
