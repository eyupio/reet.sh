// Terminal image upload sources: the file picker, drag-drop and the Paste button
// all reach the raw upload route and insert one fenced reference with no Enter.
import { chromium, expect } from '@playwright/test'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now(), agentA = 'connector-laptop000000001', sessionA = 'session-build00000000001'
const sessions = [{ id: sessionA, agentId: agentA, title: 'Build workspace', state: 'running', createdAt: now, updatedAt: now, exitCode: null }]
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zl9sAAAAASUVORK5CYII=', 'base64')
const uploadDir = '/home/developer/.local/state/reet/uploads/session-build00000000001'
const uploadPath = index => `${uploadDir}/img-fixture${index}.png`
const MAX = 5 << 20

async function open({ uploads = true, uploadsOff = false, hold = false, delay = 300 } = {}) {
  const context = await browser.newContext({ viewport: { width: 1360, height: 900 } })
  const page = await context.newPage(), errors = [], inputs = [], received = [], sockets = []
  const capabilities = { osUser: 'developer', terminalEnabled: true, terminalControl: true, readiness: 'ready' }
  if (uploads) capabilities.uploads = true
  if (uploadsOff) capabilities.uploadsOff = true
  const machines = [{ agentId: agentA, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities }]
  let send = () => {}, release = () => {}, fulfilSucceeded = 0, fulfilFailed = 0, index = 0
  const gate = hold ? new Promise(resolve => release = resolve) : null
  page.on('pageerror', error => errors.push(error.message))
  await page.addInitScript(() => {
    let clipboardText = '', clipboardImage = null
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: {
      readText: async () => clipboardText,
      writeText: async text => { window.__copied = text },
      read: async () => clipboardImage
        ? [{ types: ['image/png'], getType: async () => new Blob([new Uint8Array(clipboardImage)], { type: 'image/png' }) }]
        : [{ types: ['text/plain'], getType: async () => new Blob([clipboardText], { type: 'text/plain' }) }],
    } })
    window.__setClipboardText = text => { clipboardText = text; clipboardImage = null }
    window.__setClipboardImage = bytes => { clipboardImage = bytes }
  })
  await page.route('**/api/**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname
    if (request.method() === 'POST' && path === `/api/v2/terminal/sessions/${sessionA}/uploads`) {
      const at = index++
      received.push({ contentType: request.headers()['content-type'], body: Buffer.from(request.postDataBuffer() ?? []) })
      if (gate) await gate
      else if (delay) await new Promise(resolve => setTimeout(resolve, delay))
      try { await route.fulfill({ headers: { 'X-Reet-Upload-Path': encodeURIComponent(uploadPath(at)), 'X-Reet-Upload-Reference': encodeURIComponent(uploadPath(at)) }, json: { uploaded: true } }); fulfilSucceeded++ }
      catch { fulfilFailed++ }
      return
    }
    if (path === `/api/v2/terminal/sessions/${sessionA}/history`) return route.fulfill({ status: 200, contentType: 'text/plain', body: 'earlier output\n' })
    let data = {}
    if (path === '/api/v2/auth/status') data = { mode: 'accounts', user: { id: 1, name: 'Upload tester', email: 'upload@example.test', role: 'owner' } }
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
    sockets.push(socket)
    send = data => socket.send(Buffer.from(data))
    socket.onMessage(message => {
      if (typeof message === 'string') {
        const data = JSON.parse(message)
        if (data.ticket) {
          socket.send('{"type":"ready"}')
          socket.send(Buffer.from('\x1b[?2026h\x1b[2J\x1b[H\x1b[?2004hupload fixture\x1b[?2026l'))
        }
      } else inputs.push(message.toString())
    })
  })
  await page.goto(`${base}/sessions?agent=${agentA}&session=${sessionA}`)
  const pane = page.locator('.pane')
  await expect(pane).toHaveAttribute('data-connection-state', 'live')
  await page.waitForFunction(() => document.querySelector('.xterm-rows')?.textContent.includes('upload fixture'))
  const closeSocket = async () => { await sockets.at(-1).close() }
  return { context, page, errors, inputs, received, pane, send, release: () => release(), closeSocket, stats: () => ({ fulfilSucceeded, fulfilFailed }) }
}

async function pickFiles(page, files) {
  await page.locator('input[type=file]').setInputFiles(files)
}

async function dropFile(page, file) {
  await page.locator('.screen-frame').evaluate((element, file) => {
    const data = new DataTransfer()
    data.items.add(new File([new Uint8Array(file.bytes)], file.name, { type: file.type }))
    for (const type of ['dragenter', 'dragover', 'drop']) element.dispatchEvent(new DragEvent(type, { bubbles: true, cancelable: true, dataTransfer: data }))
  }, file)
}

async function pasteFile(page, file) {
  await page.locator('.xterm-screen').evaluate((element, file) => {
    const data = new DataTransfer()
    data.items.add(new File([new Uint8Array(file.bytes)], file.name, { type: file.type }))
    data.setData('text/plain', 'this text must not be pasted')
    const event = new ClipboardEvent('paste', { bubbles: true, cancelable: true })
    Object.defineProperty(event, 'clipboardData', { value: data })
    element.dispatchEvent(event)
  }, file)
}

const wireText = inputs => inputs.join('').replace(/\x1b\[200~/g, '').replace(/\x1b\[201~/g, '')

{
  // The file picker uploads the exact PNG once; the chip is visible while the
  // request is in flight and disappears afterwards. Insertion is one paced paste.
  const { context, page, errors, inputs, received } = await open()
  await pickFiles(page, { name: 'shot.png', mimeType: 'image/png', buffer: png })
  await expect(page.locator('.upload-chip')).toBeVisible()
  await expect(page.getByRole('status').filter({ hasText: 'shot.png' })).toBeVisible()
  await expect(page.getByText('Uploaded and inserted the path.', { exact: true })).toBeVisible()
  expect(received.length).toBe(1)
  expect(received[0].contentType).toBe('image/png')
  expect(received[0].body.equals(png)).toBe(true)
  const wire = inputs.join('')
  const clean = wireText(inputs)
  expect(clean).toBe(` ${uploadPath(0)} `)
  expect((wire.match(new RegExp(uploadPath(0).replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'g')) || []).length).toBe(1)
  expect(wire).not.toContain('\r')
  await expect(page.locator('.upload-chip')).toHaveCount(0)
  const stored = await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))
  expect(stored).not.toContain('img-fixture0')
  expect(stored).not.toContain('picker.png')
  expect(errors).toEqual([])
  await context.close()
  console.log('File picker: one PNG body, one chip, and exactly one inserted reference with no Enter.')
}

{
  // An image item in the pane's capture-phase paste event wins over its text
  // twin, so an image is never pasted as text.
  const { context, page, errors, inputs, received } = await open({ delay: 100 })
  await pasteFile(page, { name: 'pasted.png', type: 'image/png', bytes: [...png] })
  await expect(page.getByText('Uploaded and inserted the path.', { exact: true })).toBeVisible()
  expect(received.length).toBe(1)
  expect(received[0].contentType).toBe('image/png')
  expect(received[0].body.equals(png)).toBe(true)
  expect(wireText(inputs)).toBe(` ${uploadPath(0)} `)
  expect(inputs.join('')).not.toContain('this text must not be pasted')
  expect(errors).toEqual([])
  await context.close()
  console.log('Image paste: the image item uploaded and its text twin never reached the terminal.')
}

{
  // Two files upload in parallel and one insertion carries both references in order.
  const { context, page, errors, inputs, received, release } = await open({ hold: true })
  await pickFiles(page, [
    { name: 'one.png', mimeType: 'image/png', buffer: png },
    { name: 'two.png', mimeType: 'image/png', buffer: png },
  ])
  await expect.poll(() => received.length).toBe(2)
  await expect(page.locator('.upload-chip')).toHaveCount(2)
  release()
  await expect(page.getByText('Uploaded 2 images and inserted their paths.', { exact: true })).toBeVisible()
  expect(wireText(inputs)).toBe(` ${uploadPath(0)} ${uploadPath(1)} `)
  await expect(page.locator('.upload-chip')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('Two files: both requests were in flight together and one insertion held both paths in order.')
}

{
  // Cancel removes the chip and sends no insertion even after the held response is released.
  const { context, page, errors, inputs, received, release } = await open({ hold: true })
  await pickFiles(page, { name: 'cancelled.png', mimeType: 'image/png', buffer: png })
  const chip = page.locator('.upload-chip')
  await expect(chip).toBeVisible()
  await expect.poll(() => received.length).toBe(1)
  await page.getByRole('button', { name: 'Cancel upload of cancelled.png', exact: true }).click()
  await expect(chip).toHaveCount(0)
  release()
  await page.waitForTimeout(300)
  expect(inputs.join('')).toBe('')
  await expect(page.getByText('Uploaded and inserted the path.', { exact: true })).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('Cancel: the chip went, nothing reached the terminal, and the held response was discarded.')
}

{
  // A reconnect while the upload is in flight changes the pane generation, so the
  // path is offered for copying instead of being inserted into the new connection.
  const { context, page, errors, inputs, received, release, closeSocket } = await open({ hold: true })
  await pickFiles(page, { name: 'late.png', mimeType: 'image/png', buffer: png })
  await expect.poll(() => received.length).toBe(1)
  await closeSocket()
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state', 'live')
  release()
  await expect(page.getByText(/Uploaded, but the connection changed; nothing was inserted\./)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Copy path', exact: true })).toBeVisible()
  expect(inputs.join('')).toBe('')
  await page.getByRole('button', { name: 'Copy path', exact: true }).click()
  await expect.poll(() => page.evaluate(() => window.__copied || '')).toBe(uploadPath(0))
  expect(errors).toEqual([])
  await context.close()
  console.log('Reconnect mid-upload: nothing was inserted into the new connection and Copy path held the reference.')
}

{
  // Opening Earlier output while the upload is in flight keeps the path out of
  // the live shell and names the real reason, rather than blaming the connection.
  const { context, page, errors, inputs, received, release } = await open({ hold: true })
  await pickFiles(page, { name: 'scroll.png', mimeType: 'image/png', buffer: png })
  await expect.poll(() => received.length).toBe(1)
  await page.getByRole('button', { name: 'Earlier output', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Earlier terminal output' })).toBeVisible()
  release()
  await expect(page.getByText(/Uploaded\. Return to live output before inserting\. Paste the path:/)).toBeVisible()
  expect(inputs.join('')).toBe('')
  await page.getByRole('button', { name: 'Back to live ↓', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Copy path', exact: true })).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
  console.log('Scrollback mid-upload: nothing was inserted and the path was offered for copying.')
}

{
  // Uploads off: the control says so and an image drop sends nothing.
  const { context, page, errors, inputs, received } = await open({ uploadsOff: true })
  const attach = page.getByRole('button', { name: 'Attach image', exact: true })
  await expect(attach).toBeDisabled()
  await expect(attach).toHaveAttribute('title', 'Image uploads are off on this machine.')
  await expect(page.getByText('Image uploads are off on this machine.', { exact: true })).toBeVisible()
  await dropFile(page, { name: 'off.png', type: 'image/png', bytes: [...png] })
  await expect(page.getByText('Image uploads are off on this machine.', { exact: true }).first()).toBeVisible()
  expect(received.length).toBe(0)
  expect(inputs.join('')).toBe('')
  expect(errors).toEqual([])
  await context.close()
  console.log('Uploads off: the Attach button was disabled with the off sentence and the drop sent nothing.')
}

{
  // No upload capability: the control is absent and the update sentence explains why.
  const { context, page, errors, inputs, received } = await open({ uploads: false })
  await expect(page.getByRole('button', { name: 'Attach image', exact: true })).toHaveCount(0)
  await dropFile(page, { name: 'old.png', type: 'image/png', bytes: [...png] })
  await expect(page.getByText('Update reet on that machine to attach images.', { exact: true })).toBeVisible()
  expect(received.length).toBe(0)
  expect(inputs.join('')).toBe('')
  expect(errors).toEqual([])
  await context.close()
  console.log('No capability: no Attach button, one update sentence, and no upload request.')
}

{
  // Drag-drop of a PNG takes the same path and inserts once without Enter.
  const { context, page, errors, inputs, received } = await open({ delay: 100 })
  await dropFile(page, { name: 'dropped.png', type: 'image/png', bytes: [...png] })
  await expect(page.getByText('Uploaded and inserted the path.', { exact: true })).toBeVisible()
  expect(received.length).toBe(1)
  expect(received[0].contentType).toBe('image/png')
  expect(received[0].body.equals(png)).toBe(true)
  expect(wireText(inputs)).toBe(` ${uploadPath(0)} `)
  expect(inputs.join('')).not.toContain('\r')
  expect(errors).toEqual([])
  await context.close()
  console.log('Drag-drop: a dropped PNG uploaded and was inserted once with no Enter.')
}

{
  // The 5 MiB boundary is checked in the browser before any request.
  const { context, page, errors, inputs, received } = await open({ delay: 0 })
  await pickFiles(page, { name: 'huge.png', mimeType: 'image/png', buffer: Buffer.alloc(MAX + 1) })
  await expect(page.getByText('That image is larger than 5 MiB.', { exact: true })).toBeVisible()
  expect(received.length).toBe(0)
  expect(inputs.join('')).toBe('')
  await expect(page.locator('.upload-chip')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  console.log('5 MiB + 1: refused in the browser with no request.')
}

{
  // navigator.clipboard.read() with only an image makes Paste upload it; text
  // still wins and pastes as text.
  const { context, page, errors, inputs, received } = await open({ delay: 100 })
  await page.evaluate(bytes => window.__setClipboardImage(bytes), [...png])
  await page.getByRole('button', { name: 'Paste', exact: true }).click()
  await expect(page.getByText('Uploaded and inserted the path.', { exact: true })).toBeVisible()
  expect(received.length).toBe(1)
  expect(received[0].contentType).toBe('image/png')
  expect(wireText(inputs)).toBe(` ${uploadPath(0)} `)
  await page.evaluate(() => window.__setClipboardText('typed clipboard text'))
  await page.getByRole('button', { name: 'Paste', exact: true }).click()
  await expect.poll(() => inputs.join('')).toContain('typed clipboard text')
  expect(received.length).toBe(1)
  expect(errors).toEqual([])
  await context.close()
  console.log('Clipboard read: an image-only clipboard uploaded, and later text pasted as text.')
}

await browser.close()
