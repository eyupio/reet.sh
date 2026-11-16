// A desk on a phone: one pane at a time with the pane names as chips under a tab bar that scrolls, no splits, presets or
// Type in all panes, the saved tree left as it was, and every target at least 44 px. Synthetic API answers and a fake
// terminal socket; nothing is started or ended on a machine.
// Run against `npm --prefix web run preview -- --host 127.0.0.1 --port 4173` after `npm --prefix web run build`.
// REET_UI_SCREENSHOTS=<dir> also saves a split tab on a 390 px phone in the dark theme.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const shots = process.env.REET_UI_SCREENSHOTS
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const errors = []

const now = Date.now(), agent = 'connector-studio00000001'
const machines = [{ agentId: agent, host: 'studio', ready: true, serverEnabled: true, capabilities: { osUser: 'dev', terminalEnabled: true, terminalControl: true, readiness: 'ready', browse: true } }]
const running = (id, title) => ({ id, agentId: agent, title, state: 'running', createdAt: now, updatedAt: now, exitCode: null, dir: '/home/dev/PROD_reet' })
const leaf = id => ({ pane: id })
const paneOf = (id, name, session, colour = '') => ({ id, name, colour, host: agent, session, kind: 'shell', dir: '/home/dev/PROD_reet', dirAt: 0, start: '' })
// A 2 + 1 tab (alpha and beta side by side over gamma), a lost shell, and enough other tabs that the tab bar scrolls.
const twoPlusOne = { split: 'down', ratio: 0.62, a: { split: 'right', ratio: 0.4, a: leaf('pane-a'), b: leaf('pane-b') }, b: leaf('pane-c') }
function deskDoc() {
  const tabs = [
    { id: 'tab-three', name: 'deploy', colour: 'red', root: twoPlusOne, zoomed: '', active: 'pane-a', panes: { 'pane-a': paneOf('pane-a', 'alpha', 'sess-a', 'red'), 'pane-b': paneOf('pane-b', 'beta', 'sess-b', 'blue'), 'pane-c': paneOf('pane-c', 'gamma', 'sess-c', 'green') } },
    { id: 'tab-lost', name: 'after reboot', colour: 'amber', root: { split: 'right', ratio: 0.5, a: leaf('pane-lost'), b: leaf('pane-l2') }, zoomed: 'pane-lost', active: 'pane-lost', panes: { 'pane-lost': paneOf('pane-lost', 'lost one', 'sess-lost'), 'pane-l2': paneOf('pane-l2', 'other', 'sess-l2') } },
    ...['logs', 'db migrations', 'workers', 'cache', 'search index', 'mail'].map((name, i) => ({ id: `tab-x${i}`, name, colour: '', root: leaf(`pane-x${i}`), zoomed: '', active: `pane-x${i}`, panes: { [`pane-x${i}`]: paneOf(`pane-x${i}`, name, '') } })),
  ]
  return { tabs, active: 'tab-three' }
}
let desk, patches, sessions, reopens
function reset() {
  desk = { id: 'desk-split', name: 'split desk', colour: 'violet', template: false, position: 0, version: 1, schema: 1, doc: deskDoc() }
  patches = []; reopens = []
  sessions = [running('sess-a', 'alpha'), running('sess-b', 'beta'), running('sess-c', 'gamma'), { ...running('sess-lost', 'lost one'), state: 'lost' }, running('sess-l2', 'other')]
}
reset()
const summary = d => ({ id: d.id, name: d.name, colour: d.colour, template: d.template, position: d.position, version: d.version, tabs: d.doc.tabs.length,
  panes: d.doc.tabs.reduce((n, t) => n + Object.keys(t.panes).length, 0), sessions: d.doc.tabs.flatMap(t => Object.values(t.panes).map(p => p.session)).filter(Boolean), hosts: [agent] })

const api = async route => {
  const request = route.request(), path = new URL(request.url()).pathname, method = request.method()
  let data = {}
  if (path === '/api/v2/auth/status') data = { mode: 'accounts', user: { id: 1, name: 'Desk tester', email: 'test@example.test', role: 'owner' } }
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
  else if (path === '/api/v2/terminal/sessions' && method === 'GET') data = { sessions }
  else if (path.endsWith('/command-history')) data = { entries: [], sources: [], truncated: false, skipped: 0 }
  else if (path.endsWith('/attachments')) data = { ticket: `ticket-${path.split('/').at(-2)}` }
  else if (path === '/api/v2/desks' && method === 'GET') data = { desks: [summary(desk)] }
  else if (/^\/api\/v2\/desks\/[^/]+\/panes\/[^/]+\/reopen$/.test(path) && method === 'POST') {
    const pid = decodeURIComponent(path.split('/')[6]), body = request.postDataJSON()
    reopens.push({ pane: pid, ...body })
    if (body.expect !== desk.version) return route.fulfill({ status: 409, json: { error: 'desk_stale', code: 'desk_stale', desk } })
    const created = running(`sess-re-${reopens.length}`, 'lost one')
    sessions = [created, ...sessions]
    desk = { ...desk, version: desk.version + 1, doc: { ...desk.doc, tabs: desk.doc.tabs.map(t => !t.panes[pid] ? t : { ...t, panes: { ...t.panes, [pid]: { ...t.panes[pid], session: created.id } } }) } }
    return route.fulfill({ json: { desk, session: created } })
  }
  else if (path === `/api/v2/desks/${desk.id}`) {
    if (method === 'GET') data = desk
    else if (method === 'PATCH') {
      const body = request.postDataJSON()
      patches.push(body)
      if (body.expect !== desk.version) return route.fulfill({ status: 409, json: { error: 'desk_stale', code: 'desk_stale', desk } })
      desk = { ...desk, ...(body.doc ? { doc: body.doc } : {}), version: desk.version + 1 }
      data = desk
    }
  }
  else return route.fulfill({ status: 404, json: { error: `Unmocked ${path}` } })
  await route.fulfill({ json: data })
}
const socketStub = socket => {
  socket.onMessage(message => {
    if (typeof message !== 'string') return
    const data = JSON.parse(message)
    if (data.ticket) { socket.send('{"type":"ready"}'); socket.send(Buffer.from('\x1b[2J\x1b[Hdev@studio:~/PROD_reet$ ')) }
  })
}
async function phonePage(colorScheme = 'light') {
  const context = await browser.newContext({ viewport: { width: 390, height: 780 }, hasTouch: true, isMobile: true, colorScheme })
  const page = await context.newPage()
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/**', api)
  await page.routeWebSocket('**/api/v2/terminal/socket', socketStub)
  return { context, page }
}
const quiet = page => page.waitForTimeout(700)
/** The tree of each tab as saved: the splits, ratios and zoom, without which pane is focused. */
const trees = doc => doc.tabs.map(t => ({ id: t.id, root: t.root, zoomed: t.zoomed }))

let page, context
try {
  // ── a phone shows one pane and chips ──
  {
    reset()
    ;({ context, page } = await phonePage())
    await page.goto(`${base}/desks/desk-split`)
    await expect(page.getByRole('heading', { name: 'split desk', level: 1 })).toBeVisible()
    const chips = page.getByRole('group', { name: 'Panes on this tab' })
    await expect(chips.getByRole('button')).toHaveText(['alpha', 'beta', 'gamma'])
    await expect(chips.getByRole('button', { name: 'alpha' })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.locator('.desk-pane:visible')).toHaveCount(1)
    // The pane fills the width, edge to edge.
    const box = await page.locator('.desk-pane[data-pane="pane-a"]').boundingBox()
    expect(box.width).toBeGreaterThanOrEqual(388)
    await chips.getByRole('button', { name: 'gamma' }).tap()
    await expect(page.locator('.desk-pane[data-pane="pane-c"]')).toBeVisible()
    await expect(page.locator('.desk-pane[data-pane="pane-a"]')).toBeHidden()
    await expect(chips.getByRole('button', { name: 'gamma' })).toHaveAttribute('aria-pressed', 'true')
    // Nothing that splits, lays out or types into every pane.
    for (const name of ['Layout', 'Type in all panes', 'Split right', 'Split down', 'Zoom']) await expect(page.getByRole('button', { name, exact: true })).toHaveCount(0)
    await expect(page.getByRole('separator')).toHaveCount(0)
    await page.locator('.desk-pane[data-pane="pane-c"] .more').tap()
    const menu = page.getByRole('menu', { name: 'Actions for gamma' })
    await expect(menu.getByRole('menuitem', { name: 'Rename' })).toBeVisible()
    for (const name of ['Split right', 'Split down', 'Zoom', 'Swap with…', 'Move to tab…']) await expect(menu.getByRole('menuitem', { name })).toHaveCount(0)
    await page.keyboard.press('Escape')
    // The tab bar scrolls sideways; the page itself never does.
    const bar = page.getByRole('tablist', { name: 'Tabs' })
    expect(await bar.evaluate(el => el.scrollWidth > el.clientWidth && getComputedStyle(el).overflowX === 'auto')).toBe(true)
    await bar.getByRole('tab', { name: 'mail', exact: true }).tap()
    await expect(bar.getByRole('tab', { name: 'mail', exact: true })).toHaveAttribute('aria-selected', 'true')
    // A tab of one pane has no chips.
    await expect(chips).toHaveCount(0)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await context.close()
    console.log('ok - a phone shows one pane and chips')
  }

  // ── the saved tree is unchanged after a phone visit ──
  {
    reset()
    const before = trees(desk.doc)
    ;({ context, page } = await phonePage())
    await page.goto(`${base}/desks/desk-split`)
    const chips = page.getByRole('group', { name: 'Panes on this tab' })
    for (const name of ['beta', 'gamma', 'alpha']) await chips.getByRole('button', { name }).tap()
    // A zoomed tab shows its panes one at a time too, and keeps its zoom.
    await page.getByRole('tab', { name: 'after reboot', exact: true }).tap()
    await expect(chips.getByRole('button')).toHaveText(['lost one', 'other'])
    await chips.getByRole('button', { name: 'other' }).tap()
    await expect(page.locator('.desk-pane[data-pane="pane-l2"]')).toBeVisible()
    // A lost shell is reopened from its card, on the phone as anywhere.
    await chips.getByRole('button', { name: 'lost one' }).tap()
    await page.locator('.desk-pane[data-pane="pane-lost"]').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).tap()
    await expect(page.locator('.desk-pane[data-pane="pane-lost"]')).toHaveAttribute('data-state', 'live')
    await quiet(page)
    await page.reload()
    await expect(page.getByRole('heading', { name: 'split desk', level: 1 })).toBeVisible()
    await quiet(page)
    expect(patches.length).toBeGreaterThan(0)
    for (const p of patches) expect(trees(p.doc)).toEqual(before)
    expect(trees(desk.doc)).toEqual(before)
    expect(reopens).toHaveLength(1)
    await context.close()
    console.log('ok - the saved tree is unchanged after a phone visit')
  }

  // ── targets are at least 44 px ──
  {
    reset()
    ;({ context, page } = await phonePage())
    await page.goto(`${base}/desks/desk-split`)
    const chips = page.getByRole('group', { name: 'Panes on this tab' })
    await expect(chips.getByRole('button', { name: 'alpha' })).toBeVisible()
    const height = async locator => (await locator.boundingBox()).height
    const targets = [
      ['desk name', page.getByRole('button', { name: 'split desk, switch desk' })],
      ['desk menu', page.getByRole('button', { name: 'Desk menu' })],
      ['tab', page.getByRole('tab', { name: 'deploy', exact: true })],
      ['tab menu', page.getByRole('button', { name: 'Actions for tab deploy' })],
      ['new tab', page.getByRole('button', { name: 'New tab' })],
      ['chip', chips.getByRole('button', { name: 'beta' })],
      ['pane menu', page.locator('.desk-pane[data-pane="pane-a"] .more')],
      ['theatre', page.getByRole('button', { name: 'Theatre view' })],
      ['compose', page.getByRole('button', { name: 'Compose' })],
    ]
    for (const [what, locator] of targets) expect(await height(locator), what).toBeGreaterThanOrEqual(44)
    // The + menu, and a card's actions.
    await page.getByRole('button', { name: 'New tab' }).tap()
    for (const item of await page.getByRole('menu', { name: 'New tab' }).getByRole('menuitem').all()) expect(await height(item), 'new tab menu').toBeGreaterThanOrEqual(44)
    await page.keyboard.press('Escape')
    await page.getByRole('tab', { name: 'after reboot', exact: true }).tap()
    const card = page.locator('.desk-pane[data-pane="pane-lost"]')
    expect(await height(card.getByRole('button', { name: 'Reopen in ~/PROD_reet' })), 'reopen').toBeGreaterThanOrEqual(44)
    expect(await height(card.getByRole('button', { name: 'Close pane' })), 'close pane').toBeGreaterThanOrEqual(44)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await context.close()
    console.log('ok - targets are at least 44 px')
  }

  // ── a tablet keeps every control reachable ──
  {
    reset()
    context = await browser.newContext({ viewport: { width: 820, height: 1180 }, hasTouch: true })
    page = await context.newPage()
    page.on('pageerror', error => errors.push(error.message))
    await page.route('**/api/**', api)
    await page.routeWebSocket('**/api/v2/terminal/socket', socketStub)
    await page.goto(`${base}/desks/desk-split`)
    await expect(page.locator('.desk-pane .xterm-screen')).toHaveCount(3)
    // The toolbar wraps rather than hide its last controls: Theatre view is on screen without scrolling, and pressed.
    const tools = page.locator('.desk-toolbar .tools')
    const theatre = page.getByRole('button', { name: 'Theatre view' })
    for (const width of [820, 1100]) {
      await page.setViewportSize({ width, height: 1180 })
      await expect(theatre).toBeInViewport({ ratio: 1 })
      expect(await tools.evaluate(el => el.scrollWidth <= el.clientWidth), `toolbar at ${width}`).toBe(true)
    }
    await page.setViewportSize({ width: 820, height: 1180 })
    await theatre.tap()
    await expect(page.getByRole('button', { name: 'Exit theatre view' })).toHaveAttribute('aria-pressed', 'true')
    await page.getByRole('button', { name: 'Exit theatre view' }).tap()
    // A half-width pane's key bar scrolls sideways within the pane, so every key can be reached.
    for (const id of ['pane-a', 'pane-b']) {
      const p = page.locator(`.desk-pane[data-pane="${id}"]`), keys = p.locator('.mobile-keys')
      expect(await keys.evaluate(el => getComputedStyle(el).overflowX), id).toBe('auto')
      expect(await keys.evaluate(el => el.scrollWidth > el.clientWidth), id).toBe(true)
      const pb = await p.boundingBox(), kb = await keys.boundingBox()
      expect(kb.x + kb.width, id).toBeLessThanOrEqual(pb.x + pb.width + 1)
      const last = keys.getByRole('button').last()
      await last.scrollIntoViewIfNeeded()
      const lb = await last.boundingBox()
      expect(lb.x + lb.width, id).toBeLessThanOrEqual(kb.x + kb.width + 1)
    }
    if (shots) { await mkdir(shots, { recursive: true }); await page.screenshot({ path: `${shots}/desk-tablet-split.png` }) }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await context.close()
    console.log('ok - a tablet keeps every control reachable')
  }

  if (shots) {
    reset()
    await mkdir(shots, { recursive: true })
    ;({ context, page } = await phonePage('dark'))
    await page.goto(`${base}/desks/desk-split`)
    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'dark'))
    await expect(page.locator('.desk-pane[data-pane="pane-a"] .xterm-screen')).toBeVisible()
    await page.waitForTimeout(800)
    await page.screenshot({ path: `${shots}/desk-phone-split-dark.png` })
    await context.close()
  }
  expect(errors).toEqual([])
  console.log('Desks on a phone passed: one pane and chips, the tree kept, 44 px targets, a tablet\'s toolbar and key bars.')
} catch (error) {
  console.error('Browser errors:', errors)
  if (shots && page && !page.isClosed()) { await mkdir(shots, { recursive: true }); await page.screenshot({ path: `${shots}/desks-phone-failure.png`, fullPage: true }) }
  throw error
} finally { await browser.close() }
