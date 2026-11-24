// The terminal's compose box: opened from the terminal toolbar, written in an ordinary field, sent as one
// paste with Enter (Send) or without (Insert); the draft kept until it is sent; the warning for several lines in a
// terminal that runs each as it arrives; dictation through a stand-in for the browser's speech recognition.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY, run = name => !only || only === name
const now = Date.now()
const machines = [{ agentId: 'a1', host: 'studio', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, os: 'linux', osUser: 'sam', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready', launch: true, tools: ['claude'] } }]
const sessions = [
  { id: 't1', agentId: 'a1', title: 'Refactor the store layer', state: 'running', tool: 'claude', model: 'sonnet', createdAt: now, updatedAt: now, exitCode: null },
  { id: 't2', agentId: 'a1', title: 'Plain shell', state: 'running', tool: '', createdAt: now, updatedAt: now, exitCode: null },
]
// A speech recognition that hears what the test tells it to.
const speech = () => {
  window.SpeechRecognition = class {
    constructor() { window.__speech = this; this.started = 0 }
    start() { this.started++; window.__listening = true }
    stop() { window.__listening = false; this.onend?.() }
    abort() { window.__listening = false; window.__aborted = true; this.onend?.() }
    hear(text, final = true) { const result = Object.assign([{ transcript: text }], { isFinal: final }); this.onresult?.({ resultIndex: 0, results: [result] }) }
    fail(error) { this.onerror?.({ error }); this.stop() }
  }
}
async function open({ width, height, touch, session = 't1', bracketed = false, dictation = true }) {
  const inputs = [], sizes = []
  const opened = await openWorkspace(browser, { width, height, touch, theme: 'dark', sessionFocus: touch ? undefined : false, terminalStatus: active, terminalMachines: machines, routes: {
    '/api/v2/machines/health': () => ({ serverNow: Date.now(), machines: machines.map(m => ({ ...m, state: 'connected', connection: { connected: true, reportFresh: true, missedProbes: 0, reconnects: 0, path: 'direct' } })) }),
    '/api/v2/terminal/sessions': { sessions }, '/api/v2/folder-projects': { projects: [], savedWaiting: 0 }, '/api/v2/terminal/commands': { commands: [] },
    '/api/v2/terminal/worktrees/status': { path: '', branch: '', changed: 0, unpushed: 0, paths: [], unreferenced: 0, hidden: 0, nested: false, incomplete: false },
    '/api/v2/settings/yolo': { allowed: false },
  } })
  const { page } = opened
  if (dictation) await page.addInitScript(speech)
  else await page.addInitScript(() => { delete window.SpeechRecognition; delete window.webkitSpeechRecognition })
  await page.route('**/attachments', async route => { const body = route.request().postDataJSON(); sizes.push(body); await route.fulfill({ json: { ticket: 'fixture-ticket00000000000' } }) })
  await page.route('**/history', route => route.fulfill({ body: 'saved output\n' }))
  let socket
  await page.routeWebSocket('**/api/v2/terminal/socket', ws => { socket = ws; ws.onMessage(message => {
    if (typeof message !== 'string') { inputs.push(message.toString()); return }
    const data = JSON.parse(message)
    if (data.ticket) { ws.send('{"type":"ready"}'); ws.send(Buffer.from((bracketed ? '\x1b[?2004h' : '') + 'ready> ')) }
  }) })
  await page.goto(`${base}/sessions?agent=a1&session=${session}`)
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state', 'live')
  return { ...opened, inputs, drop: () => socket.close() }
}
const sheet = page => page.getByRole('dialog', { name: 'Compose' })
const field = page => sheet(page).getByLabel(/^Message for/)

try {
  if (run('phone')) {
    const { page, inputs, errors, context } = await open({ width: 390, height: 844, touch: true })
    // The toolbar's Compose is in view without scrolling, and opens it without raising the keyboard over the terminal first.
    const key = page.locator('.terminal-toolbar').getByRole('button', { name: /^Compose/ })
    await expect(key).toBeVisible()
    const keyBox = await key.boundingBox()
    expect(keyBox.x + keyBox.width).toBeLessThanOrEqual(390)
    expect(keyBox.height).toBeGreaterThanOrEqual(40); expect(keyBox.width).toBeGreaterThanOrEqual(40)
    // The key row keeps Esc, Tab and Ctrl in view, as before.
    expect(await page.locator('.mobile-keys .keycap').first().textContent()).toBe('Esc')
    await key.click()
    await expect(sheet(page)).toBeVisible()
    await expect(field(page)).toBeFocused()
    await expect(sheet(page).getByText('Message for Claude Code')).toBeVisible()
    // An ordinary message field: the phone's autocorrect, capitals and Enter-for-a-new-line all apply.
    for (const [name, value] of [['autocapitalize', 'sentences'], ['spellcheck', 'true'], ['enterkeyhint', 'enter']]) await expect(field(page)).toHaveAttribute(name, value)
    expect(await field(page).evaluate(e => parseFloat(getComputedStyle(e).fontSize))).toBeGreaterThanOrEqual(16)
    const send = sheet(page).getByRole('button', { name: 'Send', exact: true }), insert = sheet(page).getByRole('button', { name: 'Insert', exact: true })
    await expect(send).toBeDisabled(); await expect(insert).toBeDisabled()
    for (const button of await sheet(page).getByRole('button').all()) expect((await button.boundingBox()).height).toBeGreaterThanOrEqual(44)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)

    // Enter is a new line and sends nothing.
    await field(page).fill('Refactor the store layer.')
    await page.keyboard.press('Enter')
    await page.keyboard.type('Then run the tests   ')
    expect(inputs).toEqual([])
    // Several lines in a terminal that runs each as it arrives: said before it is sent.
    await expect(sheet(page).getByText('This terminal takes each line as it arrives, so the first line runs as soon as it is pasted.')).toBeVisible()

    // Closed and opened again, the draft is still there.
    await sheet(page).getByRole('button', { name: 'Close Compose' }).click()
    await expect(sheet(page)).toHaveCount(0)
    await key.click()
    await expect(field(page)).toHaveValue('Refactor the store layer.\nThen run the tests   ')

    // Send: one paste, trailing space trimmed, then Enter. The sheet goes and the pane says so.
    await send.click()
    await expect(sheet(page)).toHaveCount(0)
    await expect.poll(() => inputs).toEqual(['Refactor the store layer.\rThen run the tests', '\r'])
    await expect(page.locator('.pane').getByRole('status').filter({ hasText: 'Sent.' })).toBeVisible()
    // The keyboard is not raised behind the person's back.
    expect(await page.evaluate(() => document.activeElement?.classList.contains('xterm-helper-textarea'))).toBe(false)

    // Insert: the text, and no Enter. A sent draft does not come back.
    await key.click()
    await expect(field(page)).toHaveValue('')
    await field(page).fill('git status')
    await expect(sheet(page).getByText('pastes this and presses Enter')).toBeVisible()
    await insert.click()
    await expect.poll(() => inputs.slice(2)).toEqual(['git status'])
    await expect(page.locator('.pane').getByRole('status').filter({ hasText: 'Inserted. Press Enter in the terminal to send it.' })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('dictation')) {
    const { page, inputs, context } = await open({ width: 390, height: 844, touch: true, bracketed: true })
    await page.locator('.terminal-toolbar').getByRole('button', { name: /^Compose/ }).click()
    const dictate = sheet(page).getByRole('button', { name: 'Dictate', exact: true })
    await expect(dictate).toHaveAttribute('aria-pressed', 'false')
    await field(page).fill('Refactor the store')
    await dictate.click()
    await expect(sheet(page).getByRole('button', { name: 'Stop dictating' })).toHaveAttribute('aria-pressed', 'true')
    // Where the audio goes is said while it is going.
    await expect(sheet(page).getByRole('status')).toHaveText('Listening. Your browser sends the audio to its own speech service, not to reet.')
    // Words still being heard are shown apart from the draft; a finished phrase joins it with one space.
    await page.evaluate(() => window.__speech.hear('and run', false))
    await expect(sheet(page).locator('.interim')).toHaveText('and run…')
    await expect(field(page)).toHaveValue('Refactor the store')
    await page.evaluate(() => window.__speech.hear(' and run the tests. '))
    await expect(field(page)).toHaveValue('Refactor the store and run the tests.')
    // Nothing is sent by speaking.
    expect(inputs).toEqual([])
    await sheet(page).getByRole('button', { name: 'Stop dictating' }).click()
    await expect(dictate).toHaveAttribute('aria-pressed', 'false')
    // A refusal says what to do.
    await dictate.click()
    await page.evaluate(() => window.__speech.fail('not-allowed'))
    await expect(sheet(page).getByRole('status')).toHaveText('The browser did not allow the microphone. Allow it for this site, then try again.')
    // This program takes a paste as one block, so several lines carry no warning and go wrapped as a paste.
    await field(page).fill('one\ntwo')
    await expect(sheet(page).getByText('This terminal takes each line')).toHaveCount(0)
    await sheet(page).getByRole('button', { name: 'Send', exact: true }).click()
    await expect.poll(() => inputs).toEqual(['\x1b[200~one\rtwo\x1b[201~', '\r'])
    await context.close()
  }

  if (run('no speech')) {
    // A browser with no speech recognition offers no Dictate button; the keyboard's own microphone still works in the field.
    const { page, context } = await open({ width: 390, height: 844, touch: true, dictation: false })
    await page.locator('.terminal-toolbar').getByRole('button', { name: /^Compose/ }).click()
    await expect(sheet(page).getByRole('button', { name: 'Send', exact: true })).toBeVisible()
    await expect(sheet(page).getByRole('button', { name: 'Dictate' })).toHaveCount(0)
    await context.close()
  }

  if (run('desktop')) {
    // On a mouse screen Ctrl+Enter sends, and focus goes back to the terminal.
    const { page, inputs, context, drop } = await open({ width: 1360, height: 900, touch: false, session: 't2' })
    await page.locator('.terminal-toolbar').getByRole('button', { name: /^Compose/ }).click()
    await expect(sheet(page).getByText('Message for the shell')).toBeVisible()
    await field(page).fill('ls -la')
    await page.keyboard.press('Control+Enter')
    await expect(sheet(page)).toHaveCount(0)
    await expect.poll(() => inputs).toEqual(['ls -la', '\r'])
    await expect.poll(() => page.evaluate(() => document.activeElement?.classList.contains('xterm-helper-textarea'))).toBe(true)

    // The connection goes while the box is open: nothing can be sent, and the text is kept.
    await page.locator('.terminal-toolbar').getByRole('button', { name: /^Compose/ }).click()
    await field(page).fill('still here')
    drop()
    await expect(sheet(page).getByText('Reconnect before sending. Your text is kept.')).toBeVisible()
    await expect(sheet(page).getByRole('button', { name: 'Send', exact: true })).toBeDisabled()
    await expect(field(page)).toHaveValue('still here')
    expect(inputs.length).toBe(2)
    await context.close()
  }
  console.log('Terminal compose box: the toolbar entry in view on a phone, an ordinary message field, Send and Insert as one paste, the kept draft, the several-lines warning, bracketed paste, dictation and its refusal, no speech recognition, Ctrl+Enter and a dropped connection.')
} finally {
  await browser.close()
}
