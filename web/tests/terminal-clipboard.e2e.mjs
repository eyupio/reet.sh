// Terminal paste on the wire: bracketed paste skips the review modal for ordinary
// multi-line text, a payload cannot close its own bracket, and the review preview
// never shows an escape sequence. This fixture grows with the copy tasks.
import { chromium, expect } from '@playwright/test'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now(), agentA = 'connector-laptop000000001', sessionA = 'session-build00000000001'
const sessions = [{ id: sessionA, agentId: agentA, title: 'Build workspace', state: 'running', createdAt: now, updatedAt: now, exitCode: null }]
const machines = [{ agentId: agentA, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { osUser: 'developer', terminalEnabled: true, terminalControl: true, readiness: 'ready' } }]

async function open(bracketed, { rows = 0 } = {}) {
  const context = await browser.newContext({ viewport: { width: 1360, height: 900 } })
  const page = await context.newPage(), errors = [], inputs = []
  let send = () => {}
  page.on('pageerror', error => errors.push(error.message))
  await page.addInitScript(() => {
    let clipboard = ''
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { readText: async () => clipboard, writeText: async text => { clipboard = text; window.__copied = text } } })
    window.__setClipboard = text => clipboard = text
  })
  await page.route('**/api/**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname
    if (path === `/api/v2/terminal/sessions/${sessionA}/history`) return route.fulfill({ status: 200, contentType: 'text/plain', body: 'earlier row one\nearder row two\n' })
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
    send = data => socket.send(Buffer.from(data))
    socket.onMessage(message => {
      if (typeof message === 'string') {
        const data = JSON.parse(message)
        if (data.ticket) {
          socket.send('{"type":"ready"}')
          // The mode line mirrors what tmux forwards from the active pane.
          const mode = bracketed ? '\x1b[?2004h' : '\x1b[?2004l'
          const body = rows ? Array.from({ length: rows }, (_, i) => `row-${String(i + 1).padStart(2, '0')}`).join('\r\n') : 'clipboard fixture'
          socket.send(Buffer.from(`\x1b[?2026h\x1b[2J\x1b[H${mode}${body}\x1b[?2026l`))
        }
      } else inputs.push(message.toString())
    })
  })
  await page.goto(`${base}/sessions?agent=${agentA}&session=${sessionA}`)
  const pane = page.locator('.pane')
  await expect(pane).toHaveAttribute('data-connection-state', 'live')
  const marker = rows ? `row-${String(rows).padStart(2, '0')}` : 'clipboard fixture'
  await page.waitForFunction(text => document.querySelector('.xterm-rows')?.textContent.includes(text), marker)
  return { context, page, errors, inputs, pane, send }
}

async function pasteFromToolbar(page) {
  await page.getByRole('button', { name: 'Paste', exact: true }).click()
}

// The marker after the OSC sequence proves xterm parsed it before the assertion.
async function sendOsc52(send, page, payload, marker) {
  await send(`\x1b]52;${payload}\x07`)
  await send(`${marker}\r\n`)
  await expect.poll(() => page.locator('.xterm-rows').textContent()).toContain(marker)
}

// A payload over the pane's 32 KiB frame limit has to arrive in paced frames,
// exactly as the controller splits a large stream, or the connection resynchronises.
async function sendLargeOsc52(send, page, payload, marker) {
  const sequence = `\x1b]52;${payload}\x07`
  for (let offset = 0; offset < sequence.length; offset += 16 * 1024) {
    await send(sequence.slice(offset, offset + 16 * 1024))
    await page.waitForTimeout(5)
  }
  await send(`${marker}\r\n`)
  await expect.poll(() => page.locator('.xterm-rows').textContent()).toContain(marker)
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

{
  // Copy screen reads the visible viewport, not the whole scrollback.
  const { context, page, errors } = await open(true, { rows: 80 })
  await page.locator('.xterm-screen').click({ button: 'right', position: { x: 30, y: 30 } })
  await page.getByRole('menuitem', { name: 'Copy screen', exact: true }).click()
  await expect(page.getByText('Copied the screen.', { exact: true })).toBeVisible()
  await expect.poll(() => page.evaluate(() => window.__copied || '')).toContain('row-80')
  const copied = await page.evaluate(() => window.__copied || '')
  const visible = await page.evaluate(() => [...document.querySelectorAll('.xterm-rows > div')].map(row => row.textContent.trim()).filter(Boolean))
  expect(visible.length).toBeGreaterThan(1)
  for (const row of [visible[0], visible[visible.length - 1]]) expect(copied).toContain(row)
  expect(copied).not.toContain('row-01')
  await expect(page.locator('dialog[open]')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('Copy screen: the visible rows reached the clipboard and the first scrollback row did not.')
}

{
  // Last 50 reads exactly 50 buffer lines, clamped to the scrollback base.
  const { context, page, errors } = await open(true, { rows: 80 })
  await page.locator('.xterm-screen').click({ button: 'right', position: { x: 30, y: 30 } })
  await page.getByRole('menuitem', { name: 'Copy last 50 lines', exact: true }).click()
  await expect(page.getByText('Copied the last 50 lines.', { exact: true })).toBeVisible()
  await expect.poll(() => page.evaluate(() => window.__copied || '')).toMatch(/(?:^|\n)row-31(?:\n|$)/)
  const copied = await page.evaluate(() => window.__copied || '')
  expect(copied).not.toMatch(/(?:^|\n)row-30(?:\n|$)/)
  expect(copied).toMatch(/(?:^|\n)row-80(?:\n|$)/)
  expect(errors).toEqual([])
  await context.close()
  console.log('Copy last 50 lines: line 31 was the first copied line and line 30 was absent.')
}

{
  // Copy all keeps the whole scrollback, including the rows the screen no longer shows.
  const { context, page, errors } = await open(true, { rows: 80 })
  await page.locator('.xterm-screen').click({ button: 'right', position: { x: 30, y: 30 } })
  await page.getByRole('menuitem', { name: 'Copy all', exact: true }).click()
  await expect(page.getByText('Copied all terminal output.', { exact: true })).toBeVisible()
  await expect.poll(() => page.evaluate(() => window.__copied || '')).toMatch(/(?:^|\n)row-01(?:\n|$)/)
  const copied = await page.evaluate(() => window.__copied || '')
  expect(copied).toMatch(/(?:^|\n)row-80(?:\n|$)/)
  expect(errors).toEqual([])
  await context.close()
  console.log('Copy all: both the first and last rows of the terminal buffer were copied.')
}

{
  // The Select text sheet fills a read-only pre from the buffer, so a phone gets
  // native long-press selection; Copy all writes exactly that transient text.
  const { context, page, errors } = await open(true, { rows: 80 })
  await page.getByRole('button', { name: 'Select text', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  const pre = dialog.locator('pre')
  await expect(pre).toContainText('row-01')
  await expect(pre).toContainText('row-80')
  const shown = (await pre.textContent()) ?? ''
  await dialog.getByRole('button', { name: 'Copy all', exact: true }).click()
  await expect(page.getByText('Copied all terminal text.', { exact: true })).toBeVisible()
  await expect.poll(() => page.evaluate(() => window.__copied || '')).toBe(shown)
  const copied = await page.evaluate(() => window.__copied || '')
  expect(copied).toContain('row-01')
  expect(copied).toContain('row-80')
  await dialog.getByRole('button', { name: 'Close select text', exact: true }).click()
  await expect(page.locator('dialog[open]')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('Select text: the sheet showed the buffer, Copy all wrote it, and closing discarded the sheet.')
}

{
  // The Scrollback snapshot has its own Copy all; the text stays transient.
  const { context, page, errors } = await open(true)
  await page.getByRole('button', { name: 'Earlier output', exact: true }).click()
  const scrollback = page.locator('.scrollback')
  await expect(scrollback).toBeVisible()
  await expect(scrollback.locator('pre')).toContainText('earlier row one')
  await scrollback.getByRole('button', { name: 'Copy all', exact: true }).click()
  await expect.poll(() => page.evaluate(() => window.__copied || '')).toContain('earlier row one')
  expect(await page.evaluate(() => window.__copied || '')).toBe('earlier row one\nearder row two\n')
  await expect(scrollback.getByText('Copied all earlier output.', { exact: true })).toBeVisible()
  await scrollback.getByRole('button', { name: 'Back to live ↓', exact: true }).click()
  await expect(scrollback).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('Scrollback Copy all: the snapshot reached the clipboard, the status appeared, and the view closed.')
}

{
  // Copy-on-select is off by default, survives a reload, and copies a settled selection.
  const { context, page, errors } = await open(true)
  await page.getByRole('button', { name: 'Appearance', exact: true }).click()
  const drawer = page.getByRole('dialog')
  const toggle = drawer.getByRole('checkbox', { name: 'Copy when I select text' })
  await expect(toggle).not.toBeChecked()
  await toggle.check()
  await page.getByRole('button', { name: 'Close Terminal appearance' }).click()
  await expect(page.locator('dialog[open]')).toHaveCount(0)
  expect(await page.evaluate(() => localStorage.getItem('reet-terminal-copy-on-select'))).toBe('1')
  await page.evaluate(() => { window.__copied = '' })
  const screen = page.locator('.xterm-screen'), box = await screen.boundingBox()
  if (!box) throw new Error('The xterm screen has no box to drag across.')
  await page.mouse.move(box.x + 16, box.y + 16)
  await page.mouse.down()
  await page.mouse.move(box.x + Math.min(320, box.width - 16), box.y + 16, { steps: 10 })
  await page.mouse.up()
  await expect.poll(() => page.evaluate(() => window.__copied || '')).not.toBe('')
  await expect(page.getByText('Selection copied.', { exact: true })).toBeVisible()
  await page.reload()
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state', 'live')
  expect(await page.evaluate(() => localStorage.getItem('reet-terminal-copy-on-select'))).toBe('1')
  expect(errors).toEqual([])
  await context.close()
  console.log('Copy on select: off by default, copied a drag, and the saved flag survived a reload.')
}

{
  // Without a gesture the agent's OSC 52 write waits in memory behind a banner;
  // only the explicit Copy it press reaches the clipboard.
  const { context, page, errors, send } = await open(true)
  const text = 'agent clipboard text', payload = Buffer.from(text).toString('base64')
  await sendOsc52(send, page, `c;${payload}`, 'osc52-no-gesture')
  await expect(page.locator('.agent-copy')).toContainText('The agent copied')
  expect(await page.evaluate(() => window.__copied || '')).toBe('')
  await page.getByRole('button', { name: 'Copy it', exact: true }).click()
  await expect.poll(() => page.evaluate(() => window.__copied || '')).toBe(text)
  await expect(page.getByText('Agent copied to your clipboard.', { exact: true })).toBeVisible()
  await expect(page.locator('.agent-copy')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('OSC 52 without a gesture: banner held the text, Copy it wrote it, and the banner cleared.')
}

{
  // A read query and an empty write are consumed but never touch the clipboard
  // or show the banner.
  const { context, page, errors, send } = await open(true)
  await page.evaluate(() => { window.__copied = 'sentinel' })
  await sendOsc52(send, page, 'c;?', 'osc52-query')
  await sendOsc52(send, page, 'c;', 'osc52-empty')
  expect(await page.evaluate(() => window.__copied || '')).toBe('sentinel')
  await expect(page.locator('.agent-copy')).toHaveCount(0)
  await expect(page.getByText('The agent tried to copy more than 1 MiB.', { exact: true })).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('OSC 52 query and empty write: neither the clipboard nor the banner changed.')
}

{
  // A payload without the c; selector is malformed and is ignored.
  const { context, page, errors, send } = await open(true)
  await page.evaluate(() => { window.__copied = 'sentinel' })
  await sendOsc52(send, page, Buffer.from('no selector').toString('base64'), 'osc52-bare')
  expect(await page.evaluate(() => window.__copied || '')).toBe('sentinel')
  await expect(page.locator('.agent-copy')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('OSC 52 without a selector: ignored.')
}

{
  // A pointer gesture inside the pane in the last 2 s lets the write through.
  const { context, page, errors, send } = await open(true)
  const text = 'copied after a click', payload = Buffer.from(text).toString('base64')
  await page.locator('.xterm-screen').click()
  await sendOsc52(send, page, `c;${payload}`, 'osc52-gesture')
  await expect.poll(() => page.evaluate(() => window.__copied || '')).toBe(text)
  await expect(page.getByText('Agent copied to your clipboard.', { exact: true })).toBeVisible()
  await expect(page.locator('.agent-copy')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('OSC 52 after a click: the text reached the clipboard and the success line showed.')
}

{
  // Over the 1 MiB cap the write is dropped before it reaches any clipboard path.
  const { context, page, errors, send } = await open(true)
  const payload = Buffer.from('x'.repeat((1 << 20) + 1)).toString('base64')
  await sendLargeOsc52(send, page, `c;${payload}`, 'osc52-large')
  await expect(page.getByText('The agent tried to copy more than 1 MiB.', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => window.__copied || '')).toBe('')
  await expect(page.locator('.agent-copy')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('OSC 52 over 1 MiB: nothing was written and the limit sentence showed.')
}

await browser.close()
