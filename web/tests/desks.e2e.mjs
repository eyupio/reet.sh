// The Desks tab (/desks): the list, the last desk opened, a desk's tabs (rename, colour, reorder, close) and autosave.
// Synthetic API answers and a fake terminal socket; nothing is started or ended on a machine.
// Run against `npm --prefix web run preview -- --host 127.0.0.1 --port 4173` after `npm --prefix web run build`.
// REET_UI_SCREENSHOTS=<dir> also saves the list and a desk in both themes and at phone width.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const shots = process.env.REET_UI_SCREENSHOTS
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const context = await browser.newContext({ viewport: { width: 1360, height: 860 } })
const page = await context.newPage(), errors = []
page.on('pageerror', error => errors.push(error.message))

const now = Date.now(), agent = 'connector-studio00000001'
const machines = [{ agentId: agent, host: 'studio', ready: true, serverEnabled: true, capabilities: { osUser: 'dev', terminalEnabled: true, terminalControl: true, readiness: 'ready', browse: true } }]
const running = (id, title) => ({ id, agentId: agent, title, state: 'running', createdAt: now, updatedAt: now, exitCode: null, dir: '/home/dev/PROD_reet' })
const paneOf = (id, name, session, dir) => ({ id, name, colour: '', host: agent, session, kind: 'shell', dir, dirAt: 0, start: '' })
const tabOf = (id, name, colour, p) => ({ id, name, colour, root: { pane: p.id }, zoomed: '', active: p.id, panes: { [p.id]: p } })
const deskOf = (id, name, tabs, over = {}) => ({ id, name, colour: 'red', template: false, position: 0, version: 1, schema: 1, doc: { tabs, active: tabs[0]?.id ?? '' }, ...over })
const summary = d => ({ id: d.id, name: d.name, colour: d.colour, template: d.template, position: d.position, version: d.version, tabs: d.doc.tabs.length,
  panes: d.doc.tabs.reduce((n, t) => n + Object.keys(t.panes).length, 0), sessions: d.doc.tabs.flatMap(t => Object.values(t.panes).map(p => p.session)).filter(Boolean),
  hosts: [...new Set(d.doc.tabs.flatMap(t => Object.values(t.panes).map(p => p.host)))] })

// The fixture's state: reset() before each part.
let desks, sessions, patches, ends, creates, staleNext, hidden
function reset({ empty = false } = {}) {
  sessions = [running('sess-deploy', 'deploy'), running('sess-logs', 'logs'), running('sess-db', 'db')]
  desks = new Map()
  if (!empty) {
    const d = deskOf('desk-prod', 'prod deploy', [
      tabOf('tab-deploy', 'deploy', 'red', paneOf('pane-deploy', 'reet', 'sess-deploy', '/home/dev/PROD_reet')),
      tabOf('tab-logs', 'logs', 'blue', paneOf('pane-logs', 'platform logs', 'sess-logs', '/home/dev/PROD_platform')),
      tabOf('tab-db', 'db', '', paneOf('pane-db', 'db', 'sess-db', '/home/dev')),
    ])
    desks.set(d.id, d)
    desks.set('desk-dev', deskOf('desk-dev', 'zoomies dev', [], { colour: 'green', position: 1 }))
  }
  patches = []; ends = []; creates = []; staleNext = null; hidden = false
}
reset()

await page.route('**/api/**', async route => {
  const request = route.request(), url = new URL(request.url()), path = url.pathname, method = request.method()
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
  else if (path === '/api/v2/terminal/sessions') {
    if (method === 'GET') data = { sessions }
    else {
      const body = request.postDataJSON()
      creates.push(body)
      const created = { ...running(`sess-new-${creates.length}`, body.title), dir: body.dir || '/home/dev' }
      sessions = [created, ...sessions]
      return route.fulfill({ status: 201, json: created })
    }
  }
  else if (/^\/api\/v2\/terminal\/sessions\/[^/]+\/end$/.test(path)) {
    const id = path.split('/').at(-2)
    ends.push({ id, ...request.postDataJSON() })
    sessions = sessions.map(s => s.id === id ? { ...s, state: 'ended' } : s)
  }
  else if (path.endsWith('/attachments')) data = { ticket: 'fixture-ticket00000000000' }
  else if (path === '/api/v2/desks' && method === 'GET') {
    data = { desks: [...desks.values()].map(summary) }
    // A desk from a newer reet: the controller never lists one, and the browser does not show one either.
    if (hidden) data.desks.push({ ...summary(deskOf('desk-new', 'from the future', [])), schema: 2 })
  }
  else if (path === '/api/v2/desks' && method === 'POST') {
    const body = request.postDataJSON()
    const d = deskOf(`desk-${desks.size + 1}`, body.name, [], { colour: body.colour ?? '' })
    desks.set(d.id, d)
    return route.fulfill({ status: 201, json: d })
  }
  else if (path.startsWith('/api/v2/desks/')) {
    const id = decodeURIComponent(path.split('/')[4]), d = desks.get(id)
    if (!d) return route.fulfill({ status: 404, json: { error: "That desk isn't on this account.", code: 'desk_missing' } })
    if (method === 'GET') data = d
    else if (method === 'PATCH') {
      const body = request.postDataJSON()
      patches.push(body)
      if (staleNext) { const newer = staleNext; staleNext = null; desks.set(id, newer); return route.fulfill({ status: 409, json: { error: 'desk_stale', code: 'desk_stale', desk: newer } }) }
      if (body.expect !== d.version) return route.fulfill({ status: 409, json: { error: 'desk_stale', code: 'desk_stale', desk: d } })
      const next = { ...d, ...(body.name !== undefined ? { name: body.name } : {}), ...(body.colour !== undefined ? { colour: body.colour } : {}), ...(body.doc ? { doc: body.doc } : {}), version: d.version + 1 }
      desks.set(id, next)
      data = next
    } else if (method === 'DELETE') { desks.delete(id); data = {} }
  }
  else return route.fulfill({ status: 404, json: { error: `Unmocked ${path}` } })
  await route.fulfill({ json: data })
})
await page.routeWebSocket('**/api/v2/terminal/socket', socket => {
  socket.onMessage(message => {
    if (typeof message !== 'string') return
    const data = JSON.parse(message)
    if (data.ticket) { socket.send('{"type":"ready"}'); socket.send(Buffer.from('\x1b[2J\x1b[Hdev@studio:~/PROD_reet$ ')) }
  })
})

const nav = page.getByRole('navigation', { name: 'Main navigation' })
const tablist = page.getByRole('tablist', { name: 'Tabs' })
const tab = name => tablist.getByRole('tab', { name, exact: true })
const tabNames = () => tablist.getByRole('tab').evaluateAll(els => els.map(el => el.querySelector('.name')?.textContent))
const quiet = () => page.waitForTimeout(700)
async function openDesk() {
  await page.goto(`${base}/desks/desk-prod`)
  await expect(page.getByRole('heading', { name: 'prod deploy', level: 1 })).toBeVisible()
  await expect(tab('deploy')).toHaveAttribute('aria-selected', 'true')
}
const lastDoc = () => patches.at(-1)?.doc

try {
  // ── Desks is in the top bar ──
  reset({ empty: true })
  await page.goto(`${base}/desks`)
  const links = await nav.getByRole('link').allTextContents()
  const at = links.map(t => t.trim()).indexOf('Desks')
  expect(at).toBeGreaterThan(0)
  expect(links[at - 1].trim()).toBe('Sessions')
  await expect(nav.getByRole('link', { name: 'Desks' })).toHaveAttribute('aria-current', 'page')
  await expect(page).toHaveTitle(/^Desks · reet\.sh/)
  console.log('ok - Desks is in the top bar')

  // ── empty list offers New desk ──
  await expect(page.getByRole('heading', { name: 'Desks', level: 1 })).toBeVisible()
  await expect(page.getByText('Saved sets of terminals. Each one comes back as you left it.')).toBeVisible()
  await expect(page.getByText('No desks yet.')).toBeVisible()
  await expect(page.getByText('A desk keeps a set of terminals together: their tabs, their folders and their history.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'From a template ▾' })).toBeDisabled()
  if (shots) { await mkdir(shots, { recursive: true }); await page.screenshot({ path: `${shots}/desks-empty-light.png` }) }
  await page.locator('.empty').getByRole('button', { name: 'New desk' }).click()
  await expect(page).toHaveURL(/\/desks\/desk-1$/)
  await expect(page.getByRole('heading', { name: 'Desk', level: 1 })).toBeVisible()
  await expect(page.getByText('Nowt on this desk yet.')).toBeVisible()
  expect(creates).toEqual([])
  console.log('ok - empty list offers New desk')

  // ── opens the last desk ──
  reset()
  await page.goto(`${base}/desks/desk-prod`)
  await expect(page.getByRole('heading', { name: 'prod deploy', level: 1 })).toBeVisible()
  expect(await page.evaluate(() => localStorage.getItem('reet.desks.last.1'))).toBe('desk-prod')
  await page.goto(`${base}/desks`)
  await expect(page).toHaveURL(/\/desks\/desk-prod$/)
  await expect(page.getByRole('heading', { name: 'prod deploy', level: 1 })).toBeVisible()
  // Only the active tab's pane is mounted, and it is the terminal (opening a desk starts nothing).
  await expect(page.locator('.desk-pane')).toHaveCount(1)
  await expect(page.locator('.desk-pane .xterm')).toHaveCount(1)
  await expect(page.locator('.desk-pane .ph .nm')).toHaveText('reet')
  expect(creates).toEqual([])
  // Nothing of a desk but its id is kept in this browser.
  expect(await page.evaluate(() => Object.keys(localStorage).filter(k => k.startsWith('reet.desks')))).toEqual(['reet.desks.last.1'])
  // The switcher lists the other desks and All desks, which goes back to the list.
  await page.getByRole('button', { name: 'prod deploy, switch desk' }).click()
  await expect(page.getByRole('menu', { name: 'Switch desk' }).getByRole('menuitem', { name: 'zoomies dev' })).toBeVisible()
  await page.getByRole('menuitem', { name: 'All desks' }).click()
  await expect(page).toHaveURL(/\/desks$/)
  await expect(page.getByRole('link', { name: 'prod deploy' })).toBeVisible()
  await expect(page.locator('.card').filter({ hasText: 'prod deploy' })).toContainText('3 tabs · 3 panes')
  await expect(page.locator('.card').filter({ hasText: 'prod deploy' })).toContainText('3 live')
  if (shots) await page.screenshot({ path: `${shots}/desks-list-light.png` })
  // A desk that has gone is forgotten: /desks shows the list.
  await page.evaluate(() => localStorage.setItem('reet.desks.last.1', 'desk-gone'))
  await page.goto(`${base}/desks`)
  await expect(page).toHaveURL(/\/desks$/)
  await expect(page.getByRole('link', { name: 'zoomies dev' })).toBeVisible()
  console.log('ok - opens the last desk')

  // ── renames a tab with F2 and saves once ──
  reset()
  await openDesk()
  await tab('logs').click()
  await expect(tab('logs')).toHaveAttribute('aria-selected', 'true')
  await quiet()
  patches = []
  await tab('logs').focus()
  await page.keyboard.press('F2')
  const field = tablist.getByRole('textbox', { name: 'Tab name' })
  await expect(field).toBeFocused()
  await field.fill('platform logs')
  await field.press('Enter')
  await expect(tab('platform logs')).toBeFocused()
  await expect(page.locator('.save')).toHaveText(/Not saved yet|Saving…|saved/)
  await quiet()
  expect(patches.length).toBe(1)
  expect(lastDoc().tabs.find(t => t.id === 'tab-logs').name).toBe('platform logs')
  await expect(page.locator('.save')).toHaveText('saved')
  // Double-click renames too; Escape keeps the name.
  await tab('db').dblclick()
  await tablist.getByRole('textbox', { name: 'Tab name' }).fill('nope')
  await tablist.getByRole('textbox', { name: 'Tab name' }).press('Escape')
  await expect(tab('db')).toBeVisible()
  console.log('ok - renames a tab with F2 and saves once')

  // ── reorders tabs by drag ──
  reset()
  await openDesk()
  await quiet(); patches = []
  const from = await tab('db').boundingBox(), to = await tab('deploy').boundingBox()
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2)
  await page.mouse.down()
  await page.mouse.move(from.x - 20, from.y + from.height / 2, { steps: 5 })
  await page.mouse.move(to.x + 4, to.y + to.height / 2, { steps: 8 })
  await page.mouse.up()
  await expect.poll(tabNames).toEqual(['db', 'deploy', 'logs'])
  await quiet()
  expect(patches.length).toBe(1)
  expect(lastDoc().tabs.map(t => t.id)).toEqual(['tab-db', 'tab-deploy', 'tab-logs'])
  // The drag did not switch tabs.
  await expect(tab('deploy')).toHaveAttribute('aria-selected', 'true')
  console.log('ok - reorders tabs by drag')

  // ── colours a tab ──
  await quiet(); patches = []
  await tab('db').click({ button: 'right' })
  const tabMenu = page.getByRole('menu', { name: 'Actions for tab db' })
  await expect(tabMenu.getByRole('menuitem', { name: 'Rename' })).toBeFocused()
  await tabMenu.getByRole('menuitemradio', { name: 'Violet' }).click()
  await expect(page.locator('[data-tab="tab-db"]')).toHaveAttribute('data-colour', 'violet')
  await quiet()
  expect(patches.length).toBe(1)
  expect(lastDoc().tabs.find(t => t.id === 'tab-db').colour).toBe('violet')
  // Shift+F10 opens the same menu from the keyboard.
  await tab('deploy').focus()
  await page.keyboard.press('Shift+F10')
  await expect(page.getByRole('menu', { name: 'Actions for tab deploy' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menu')).toHaveCount(0)
  await expect(tab('deploy')).toBeFocused()
  console.log('ok - colours a tab')

  // ── closing a live pane asks and Enter ends the shell ──
  reset()
  await openDesk()
  await quiet(); patches = []
  await page.getByRole('button', { name: 'Actions for tab deploy' }).click()
  await page.getByRole('menuitem', { name: 'Close tab' }).click()
  const dialog = page.getByRole('dialog', { name: 'Close “deploy”?' })
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText('Its shell is still running on studio.')
  await expect(dialog.getByRole('radio', { name: /End the shell/ })).toHaveAttribute('aria-checked', 'true')
  await expect(dialog.getByText('Stops what is running in it. Its command history for this pane is removed.')).toBeVisible()
  await expect(dialog.getByText('It keeps running, and stays in Sessions.')).toBeVisible()
  if (shots) await page.screenshot({ path: `${shots}/desks-close-light.png` })
  await page.keyboard.press('Enter')
  await expect(dialog).toHaveCount(0)
  await expect.poll(() => ends.length).toBe(1)
  expect(ends[0]).toMatchObject({ id: 'sess-deploy', removeHistory: true })
  await expect.poll(tabNames).toEqual(['logs', 'db'])
  await quiet()
  expect(lastDoc().tabs.map(t => t.id)).toEqual(['tab-logs', 'tab-db'])
  console.log('ok - closing a live pane asks and Enter ends the shell')

  // ── Take off this desk sends no end ──
  reset()
  await openDesk()
  await quiet(); patches = []
  await page.getByRole('button', { name: 'Actions for reet' }).click()
  await page.getByRole('menuitem', { name: 'Close' }).click()
  const takeOff = page.getByRole('dialog', { name: 'Close “reet”?' })
  await takeOff.getByRole('radio', { name: /Take off this desk/ }).click()
  await takeOff.getByRole('button', { name: 'Take off this desk' }).click()
  await expect.poll(tabNames).toEqual(['logs', 'db'])
  await quiet()
  expect(ends).toEqual([])
  expect(lastDoc().tabs.map(t => t.id)).toEqual(['tab-logs', 'tab-db'])
  // Cancel changes nothing.
  await page.getByRole('button', { name: 'Actions for tab logs' }).click()
  await page.getByRole('menuitem', { name: 'Close tab' }).click()
  await page.getByRole('dialog', { name: 'Close “logs”?' }).getByRole('button', { name: 'Cancel' }).click()
  await expect.poll(tabNames).toEqual(['logs', 'db'])
  expect(ends).toEqual([])
  console.log('ok - Take off this desk sends no end')

  // ── a stale save shows the notice ──
  reset()
  await openDesk()
  await quiet(); patches = []
  // Another window closed the db tab: renaming it here can no longer apply, so the newer desk is shown and said so.
  const current = desks.get('desk-prod')
  staleNext = { ...current, version: current.version + 1, doc: { ...current.doc, tabs: current.doc.tabs.filter(t => t.id !== 'tab-db') } }
  await tab('db').dblclick()
  await tablist.getByRole('textbox', { name: 'Tab name' }).fill('database')
  await tablist.getByRole('textbox', { name: 'Tab name' }).press('Enter')
  await expect(page.getByText('This desk was changed in another window. Showing the latest.')).toBeVisible()
  await expect.poll(tabNames).toEqual(['deploy', 'logs'])
  expect(patches.length).toBe(1)
  console.log('ok - a stale save shows the notice')

  // ── a higher-schema desk is not listed ──
  reset()
  hidden = true
  await page.evaluate(() => localStorage.setItem('reet.desks.last.1', 'desk-new'))
  await page.goto(`${base}/desks`)
  await expect(page).toHaveURL(/\/desks$/)
  await expect(page.getByRole('link', { name: 'prod deploy' })).toBeVisible()
  await expect(page.getByText('from the future')).toHaveCount(0)
  console.log('ok - a higher-schema desk is not listed')

  // ── New tab starts a plain shell and adds a tab for it ──
  reset()
  await openDesk()
  await quiet(); patches = []
  await page.getByRole('button', { name: 'New tab' }).click()
  const newTab = page.getByRole('dialog', { name: 'New tab' })
  await expect(newTab.getByLabel('Machine')).toHaveValue(agent)
  await expect(newTab.getByLabel('Folder')).toHaveValue('/home/dev/PROD_reet')
  await newTab.getByLabel('Folder').fill('/home/dev/site')
  await newTab.getByRole('button', { name: 'Open tab' }).click()
  await expect(newTab).toHaveCount(0)
  expect(creates).toHaveLength(1)
  expect(creates[0]).toMatchObject({ agentId: agent, title: 'site', dir: '/home/dev/site' })
  await expect(tab('site')).toHaveAttribute('aria-selected', 'true')
  await quiet()
  const added = lastDoc().tabs.at(-1)
  expect(added.name).toBe('site')
  expect(Object.values(added.panes)[0]).toMatchObject({ session: 'sess-new-1', host: agent, kind: 'shell', dir: '/home/dev/site' })
  expect(lastDoc().active).toBe(added.id)
  console.log('ok - New tab starts a plain shell and adds a tab for it')

  // ── the look, in both themes and on a phone ──
  if (shots) {
    reset()
    await openDesk()
    await page.screenshot({ path: `${shots}/desk-light.png` })
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'dark'))
    await page.screenshot({ path: `${shots}/desk-dark.png` })
    await page.goto(`${base}/desks`)
    await page.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
    await page.goto(`${base}/desks`)
    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'dark'))
    await expect(page.getByRole('link', { name: 'prod deploy' })).toBeVisible()
    await page.screenshot({ path: `${shots}/desks-list-dark.png` })
    await page.setViewportSize({ width: 390, height: 780 })
    await page.screenshot({ path: `${shots}/desks-list-phone.png` })
    await openDesk()
    await page.screenshot({ path: `${shots}/desk-phone-dark.png` })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  expect(errors).toEqual([])
  console.log('Desks passed: top bar, empty list, last desk, tab rename, drag reorder, tab colour, close and take off, stale save, newer schema, new tab.')
} catch (error) {
  console.error('Browser errors:', errors)
  if (shots) { await mkdir(shots, { recursive: true }); await page.screenshot({ path: `${shots}/desks-failure.png`, fullPage: true }) }
  throw error
} finally { await browser.close() }
