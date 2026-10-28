// Terminal paste on the wire: bracketed paste skips the review modal for ordinary
// multi-line text, a payload cannot close its own bracket, and the review preview
// never shows an escape sequence. This fixture grows with the copy tasks.
import { chromium, expect } from '@playwright/test'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now(), agentA = 'connector-laptop000000001', sessionA = 'session-build00000000001'
const sessions = [{ id: sessionA, agentId: agentA, title: 'Build workspace', state: 'running', createdAt: now, updatedAt: now, exitCode: null }]
const machines = [{ agentId: agentA, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { osUser: 'developer', terminalEnabled: true, terminalControl: true, readiness: 'ready' } }]

async function open(bracketed) {
  const context = await browser.newContext({ viewport: { width: 1360, height: 900 } })
  const page = await context.newPage(), errors = [], inputs = []
  page.on('pageerror', error => errors.push(error.message))
  await page.addInitScript(() => {
    let clipboard = ''
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { readText: async () => clipboard, writeText: async text => { clipboard = text; window.__copied = text } } })
    window.__setClipboard = text => clipboard = text
  })
  await page.route('**/api/**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname
    let data = {}
    if (path === '/api/v2/auth/status') data = { mode: 'accounts', user: { id: 1, name: 'Clipboard tester', email: 'clipboard@example.test', role: 'owner' } }
    else if (path === '/api/v2/profile') data = { profile: { theme: 'light', shelfView: 'list', cardsExpanded: false } }
    else if (path === '/api/skills') data = { skills: [], counts: { all: 0, used: 0, liked: 0, bookmarked: 0 } }
    else if (path === '/api/environments') data = { environments: [], machines: [] }
    else if (path === '/api/tags') data = { tags: [] }
    else if (path === '/api/v2/lists') data = { lists: [], linkEnabled: false }
    else if (path === '/api/v2/sync') data = { environments: [], machines: [], targets: [], agents: [], jobs: [] }
    else if (path === '/api/v2/notifications/count') data = { unread: 0 }
    else if (path === '/api/version') data = { version: 'fixture', commit: 'fixture' }
    else if (path === '/api/v2/machines/health') data = { serverNow: Date.now(), machines: [], history: [] }
    else if (path === '/api/v2/terminal/machines') data = { machines }
    else if (path === '/api/v2/terminal/status') data = { available: true, configured: true, reason: '' }
    else if (path === '/api/v2/terminal/sessions') {
      if (request.method() !== 'GET') return route.fulfill({ status: 400, json: { error: 'No implicit creation permitted' } })
      data = { sessions }
    } else if (path.endsWith('/attachments')) data = { ticket: 'fixture-ticket00000000000' }
    else return route.fulfill({ status: 404, json: { error: `Unmocked ${path}` } })
    await route.fulfill({ json: data })
  })
  await page.routeWebSocket('**/api/v2/terminal/socket', socket => {
    socket.onMessage(message => {
      if (typeof message === 'string') {
        const data = JSON.parse(message)
        if (data.ticket) {
          socket.send('{"type":"ready"}')
          // The mode line mirrors what tmux forwards from the active pane.
          const mode = bracketed ? '\x1b[?2004h' : '\x1b[?2004l'
          socket.send(Buffer.from(`\x1b[?2026h\x1b[2J\x1b[H${mode}clipboard fixture\x1b[?2026l`))
        }
      } else inputs.push(message.toString())
    })
  })
  await page.goto(`${base}/sessions?agent=${agentA}&session=${sessionA}`)
  const pane = page.locator('.pane')
  await expect(pane).toHaveAttribute('data-connection-state', 'live')
  await page.waitForFunction(() => document.querySelector('.xterm-rows')?.textContent.includes('clipboard fixture'))
  return { context, page, errors, inputs, pane }
}

async function pasteFromToolbar(page) {
  await page.getByRole('button', { name: 'Paste', exact: true }).click()
}

{
  // Bracketed paste on: a payload carrying a closing marker and newlines goes in
  // as one block, with xterm supplying the only bracket pair on the wire.
  const { context, page, errors, inputs } = await open(true)
  await page.evaluate(() => window.__setClipboard('line one\nline two\nline three\x1b[201~'))
  await pasteFromToolbar(page)
  await expect(page.locator('dialog[open]')).toHaveCount(0)
  await expect(page.getByText('Pasted 3 lines as one block.', { exact: true })).toBeVisible()
  await expect.poll(() => inputs.join('')).toContain('line three')
  const wire = inputs.join('')
  expect((wire.match(/\x1b\[200~/g) || []).length).toBe(1)
  expect((wire.match(/\x1b\[201~/g) || []).length).toBe(1)
  expect(wire.indexOf('\x1b[200~')).toBeLessThan(wire.indexOf('\x1b[201~'))
  expect(wire).toContain('line one\rline two\rline three')
  expect(errors).toEqual([])
  await context.close()
  console.log('Bracketed paste: three lines and a payload marker reached the wire as one bracketed block.')
}

{
  // Bracketed paste off: newlines would run commands, so the dialog appears and
  // the confirmed payload still carries no escape byte from the clipboard.
  const { context, page, errors, inputs } = await open(false)
  await page.evaluate(() => window.__setClipboard('first line\nsecond\x1b[201~line'))
  await pasteFromToolbar(page)
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText('Paste into the remote shell?')
  await dialog.getByRole('button', { name: 'Paste text', exact: true }).click()
  await expect.poll(() => inputs.join('')).toContain('secondline')
  const wire = inputs.join('')
  expect(wire).not.toContain('\x1b')
  expect(wire).not.toContain('201~')
  expect(wire).toContain('first line\rsecondline')
  expect(errors).toEqual([])
  await context.close()
  console.log('Unbracketed paste: review appeared; the confirmed wire text had no payload escape bytes.')
}

{
  // A control character always reviews, even with bracketed paste on.
  const { context, page, errors, inputs } = await open(true)
  await page.evaluate(() => window.__setClipboard('hello\x07world'))
  await pasteFromToolbar(page)
  await expect(page.getByRole('dialog')).toBeVisible()
  expect(inputs.join('')).toBe('')
  expect(errors).toEqual([])
  await context.close()
  console.log('Control character: bell paste opened the review dialog and sent nothing.')
}

{
  // The dialog preview is the sanitised text: no ESC and no bracket markers.
  const { context, page, errors } = await open(true)
  await page.evaluate(() => window.__setClipboard('alpha\x1b[31mbeta\x1b[201~gamma\x07'))
  await pasteFromToolbar(page)
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  const preview = await dialog.locator('textarea').inputValue()
  expect(preview).toContain('alpha')
  expect(preview).toContain('beta')
  expect(preview).toContain('gamma')
  expect(preview).not.toContain('\x1b')
  expect(preview).not.toContain('200~')
  expect(preview).not.toContain('201~')
  expect(errors).toEqual([])
  await context.close()
  console.log('Dialog preview: ESC bytes and bracket markers were absent from the shown text.')
}

await browser.close()
