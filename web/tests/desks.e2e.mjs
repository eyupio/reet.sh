// The Desks tab (/desks): the list, the last desk opened, a desk's tabs (rename, colour, reorder, close) and autosave;
// a tab split into panes (splits, separators, presets, zoom, keys, the shared toolbar, swap and move, the phone's chips).
// Synthetic API answers and a fake terminal socket; nothing is started or ended on a machine.
// Run against `npm --prefix web run preview -- --host 127.0.0.1 --port 4173` after `npm --prefix web run build`.
// REET_UI_SCREENSHOTS=<dir> also saves the list and a desk in both themes and at phone width, and a 2 + 1 tab and a grid
// of four at 1440 px in both themes, and three panes receiving Type in all panes at 1440 px in both themes.
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
const running = (id, title, dir = '/home/dev/PROD_reet') => ({ id, agentId: agent, title, state: 'running', createdAt: now, updatedAt: now, exitCode: null, dir })
const paneOf = (id, name, session, dir) => ({ id, name, colour: '', host: agent, session, kind: 'shell', dir, dirAt: 0, start: '' })
const tabOf = (id, name, colour, p) => ({ id, name, colour, root: { pane: p.id }, zoomed: '', active: p.id, panes: { [p.id]: p } })
const deskOf = (id, name, tabs, over = {}) => ({ id, name, colour: 'red', template: false, position: 0, version: 1, schema: 1, doc: { tabs, active: tabs[0]?.id ?? '' }, ...over })
const summary = d => ({ id: d.id, name: d.name, colour: d.colour, template: d.template, position: d.position, version: d.version, tabs: d.doc.tabs.length,
  panes: d.doc.tabs.reduce((n, t) => n + Object.keys(t.panes).length, 0), sessions: d.doc.tabs.flatMap(t => Object.values(t.panes).map(p => p.session)).filter(Boolean),
  hosts: [...new Set(d.doc.tabs.flatMap(t => Object.values(t.panes).map(p => p.host)))] })

// A desk of split tabs: a 2 + 1 (alpha and beta side by side over gamma), four side by side, and nine.
const leaf = id => ({ pane: id })
const twoPlusOne = { split: 'down', ratio: 0.5, a: { split: 'right', ratio: 0.5, a: leaf('pane-a'), b: leaf('pane-b') }, b: leaf('pane-c') }
const sideOf = ids => ids.length === 1 ? leaf(ids[0]) : { split: 'right', ratio: 1 / ids.length, a: leaf(ids[0]), b: sideOf(ids.slice(1)) }
const nine = Array.from({ length: 9 }, (_, i) => `pane-n${i + 1}`)
function splitDesk() {
  const p = (id, name, session, colour = '') => ({ ...paneOf(id, name, session, '/home/dev/PROD_reet'), colour })
  return deskOf('desk-split', 'split desk', [
    { id: 'tab-three', name: 'deploy', colour: 'red', root: twoPlusOne, zoomed: '', active: 'pane-a', panes: { 'pane-a': p('pane-a', 'alpha', 'sess-a', 'red'), 'pane-b': p('pane-b', 'beta', 'sess-b', 'blue'), 'pane-c': p('pane-c', 'gamma', 'sess-c', 'green') } },
    { id: 'tab-four', name: 'four', colour: 'violet', root: sideOf(['pane-d1', 'pane-d2', 'pane-d3', 'pane-d4']), zoomed: '', active: 'pane-d1', panes: Object.fromEntries(['d1', 'd2', 'd3', 'd4'].map(x => [`pane-${x}`, p(`pane-${x}`, x, `sess-${x}`)])) },
    { id: 'tab-nine', name: 'nine', colour: '', root: { split: 'down', ratio: 1 / 3, a: sideOf(nine.slice(0, 3)), b: { split: 'down', ratio: 0.5, a: sideOf(nine.slice(3, 6)), b: sideOf(nine.slice(6)) } }, zoomed: '', active: 'pane-n1', panes: Object.fromEntries(nine.map(id => [id, p(id, id.slice(5), '')])) },
  ], { position: 2 })
}

// What each session's command history answers (by session id), when the machine reads it.
let histories = new Map()
// The fixture's state: reset() before each part.
let profileTheme = 'light'
let desks, sessions, patches, patched, ends, creates, staleNext, hidden, sockets, failing, ticketDelay, attachingNow, attachingMost
// Reopens: what each sent, the answers some panes get instead, how many desks moved on first, and how many were in flight.
let reopens, reopenFail, staleReopens, reopenDelay, reopenNow, reopenMost, conflicted
function reset({ empty = false } = {}) {
  sessions = [running('sess-deploy', 'deploy'), running('sess-logs', 'logs'), running('sess-db', 'db'),
    ...['a', 'b', 'c', 'd1', 'd2', 'd3', 'd4'].map(x => running(`sess-${x}`, x))]
  desks = new Map()
  if (!empty) {
    const d = deskOf('desk-prod', 'prod deploy', [
      tabOf('tab-deploy', 'deploy', 'red', paneOf('pane-deploy', 'reet', 'sess-deploy', '/home/dev/PROD_reet')),
      tabOf('tab-logs', 'logs', 'blue', paneOf('pane-logs', 'platform logs', 'sess-logs', '/home/dev/PROD_platform')),
      tabOf('tab-db', 'db', '', paneOf('pane-db', 'db', 'sess-db', '/home/dev')),
    ])
    desks.set(d.id, d)
    desks.set('desk-dev', deskOf('desk-dev', 'zoomies dev', [], { colour: 'green', position: 1 }))
    desks.set('desk-split', splitDesk())
  }
  patches = []; patched = []; ends = []; creates = []; staleNext = null; hidden = false
  sockets = new Map(); failing = new Set(); ticketDelay = 0; attachingNow = 0; attachingMost = 0
  reopens = []; reopenFail = new Map(); staleReopens = 0; reopenDelay = 0; reopenNow = 0; reopenMost = 0; conflicted = new Set()
  posts = []; histories = new Map(); delete machines[0].capabilities.commandHistory; delete machines[0].capabilities.resume
}
let posts = []
/** As the controller copies a desk (`from`): the same tabs and panes under fresh ids, and no sessions. */
let copies = 0
function copyDoc(doc) {
  const tabs = doc.tabs.map(t => {
    const ids = new Map(Object.keys(t.panes).map(id => [id, `pane-copy-${++copies}-${id}`]))
    const node = n => !n ? null : 'split' in n ? { ...n, a: node(n.a), b: node(n.b) } : { pane: ids.get(n.pane) }
    const id = `tab-copy-${++copies}`
    return { ...t, id, root: node(t.root), zoomed: '', active: ids.get(t.active) ?? '', panes: Object.fromEntries(Object.entries(t.panes).map(([pid, p]) => [ids.get(pid), { ...p, id: ids.get(pid), session: '' }])), from: t.id }
  })
  const active = tabs.find(t => t.from === doc.active)?.id ?? tabs[0]?.id ?? ''
  return { tabs: tabs.map(({ from: _from, ...t }) => t), active }
}
reset()

const api = async route => {
  const request = route.request(), url = new URL(request.url()), path = url.pathname, method = request.method()
  let data = {}
  if (path === '/api/v2/auth/status') data = { mode: 'accounts', user: { id: 1, name: 'Desk tester', email: 'test@example.test', role: 'owner' } }
  else if (path === '/api/v2/profile') data = { profile: { theme: profileTheme, shelfView: 'list', cardsExpanded: false } }
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
  else if (path.endsWith('/command-history')) {
    const id = path.split('/').at(-2)
    data = histories.get(id) ?? { entries: [], sources: [], truncated: false, skipped: 0 }
  }
  else if (path.endsWith('/attachments')) {
    // Each session's ticket names it, so the socket stub can tell the panes' sockets apart.
    const id = path.split('/').at(-2)
    // Open in another window: the attach is refused as a conflict.
    if (conflicted.has(id)) return route.fulfill({ status: 409, json: { error: 'terminal operation conflicts with current state' } })
    attachingNow++; attachingMost = Math.max(attachingMost, attachingNow)
    if (ticketDelay) await new Promise(resolve => setTimeout(resolve, ticketDelay))
    attachingNow--
    data = { ticket: `ticket-${id}` }
  }
  else if (path === '/api/v2/desks' && method === 'GET') {
    data = { desks: [...desks.values()].map(summary) }
    // A desk from a newer reet: the controller never lists one, and the browser does not show one either.
    if (hidden) data.desks.push({ ...summary(deskOf('desk-new', 'from the future', [])), schema: 2 })
  }
  else if (path === '/api/v2/desks' && method === 'POST') {
    const body = request.postDataJSON()
    posts.push(body)
    const d = deskOf(`desk-${desks.size + 1}`, body.name, [], { colour: body.colour ?? '', template: !!body.template })
    if (body.from) d.doc = copyDoc(desks.get(body.from).doc)
    desks.set(d.id, d)
    return route.fulfill({ status: 201, json: d })
  }
  else if (/^\/api\/v2\/desks\/[^/]+\/panes\/[^/]+\/reopen$/.test(path) && method === 'POST') {
    // As the controller does: the version is checked as the request arrives and again as the new session is bound.
    const parts = path.split('/'), id = decodeURIComponent(parts[4]), pid = decodeURIComponent(parts[6]), body = request.postDataJSON()
    reopens.push({ pane: pid, ...body })
    const stale = () => route.fulfill({ status: 409, json: { error: 'desk_stale', code: 'desk_stale', desk: desks.get(id) } })
    if (staleReopens) { staleReopens--; const d = desks.get(id); desks.set(id, { ...d, version: d.version + 1 }); return stale() }
    if (body.expect !== desks.get(id).version) return stale()
    reopenNow++; reopenMost = Math.max(reopenMost, reopenNow)
    if (reopenDelay) await new Promise(resolve => setTimeout(resolve, reopenDelay))
    reopenNow--
    if (!body.home && reopenFail.has(pid)) return route.fulfill(reopenFail.get(pid))
    const d = desks.get(id)
    if (body.expect !== d.version) return stale()
    const t = d.doc.tabs.find(x => x.panes[pid]), p = t.panes[pid]
    const created = running(`sess-re-${reopens.length}`, p.name, body.home ? '/home/dev' : p.dir)
    sessions = [created, ...sessions]
    const next = { ...d, version: d.version + 1, doc: { ...d.doc, tabs: d.doc.tabs.map(x => x !== t ? x : { ...x, panes: { ...x.panes, [pid]: { ...p, session: created.id, ...(body.home ? { dir: '/home/dev' } : {}) } } }) } }
    desks.set(id, next)
    return route.fulfill({ json: { desk: next, session: created } })
  }
  else if (path.startsWith('/api/v2/desks/')) {
    const id = decodeURIComponent(path.split('/')[4]), d = desks.get(id)
    if (!d) return route.fulfill({ status: 404, json: { error: "That desk isn't on this account.", code: 'desk_missing' } })
    if (method === 'GET') data = d
    else if (method === 'PATCH') {
      const body = request.postDataJSON()
      patches.push(body); patched.push(id)
      if (staleNext) { const newer = staleNext; staleNext = null; desks.set(id, newer); return route.fulfill({ status: 409, json: { error: 'desk_stale', code: 'desk_stale', desk: newer } }) }
      if (body.expect !== d.version) return route.fulfill({ status: 409, json: { error: 'desk_stale', code: 'desk_stale', desk: d } })
      const next = { ...d, ...(body.name !== undefined ? { name: body.name } : {}), ...(body.colour !== undefined ? { colour: body.colour } : {}), ...(body.doc ? { doc: body.doc } : {}), version: d.version + 1 }
      desks.set(id, next)
      data = next
    } else if (method === 'DELETE') { desks.delete(id); data = {} }
  }
  else return route.fulfill({ status: 404, json: { error: `Unmocked ${path}` } })
  await route.fulfill({ json: data })
}
// Each pane's socket, by session: what it was sent (input and resizes) and whether it has closed.
const socketStub = socket => {
  let record
  socket.onMessage(message => {
    if (typeof message !== 'string') { record?.input.push(Buffer.from(message).toString()); return }
    const data = JSON.parse(message)
    if (data.ticket) {
      const id = String(data.ticket).replace(/^ticket-/, '')
      record = { input: [], resizes: [], closed: false, socket }
      sockets.set(id, record)
      socket.onClose(() => { record.closed = true })
      // A session whose machine cannot be reached: the socket closes before it is ready.
      if (failing.has(id)) { socket.close({ code: 1011, reason: 'machine unreachable' }); return }
      socket.send('{"type":"ready"}'); socket.send(Buffer.from('\x1b[2J\x1b[Hdev@studio:~/PROD_reet$ '))
    } else if (data.type === 'resize') record?.resizes.push([data.cols, data.rows])
  })
}
await page.route('**/api/**', api)
await page.routeWebSocket('**/api/v2/terminal/socket', socketStub)

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
const pane = id => page.locator(`.desk-pane[data-pane="${id}"]`)
const chord = key => page.keyboard.press(`Alt+Shift+${key}`)
const attached = ids => expect.poll(() => ids.every(id => sockets.get(id) && !sockets.get(id).closed), { timeout: 10_000 }).toBe(true)
/** Opens the split desk on a tab (the 2 + 1 unless named). */
async function openSplit(tabId = 'tab-three') {
  desks.get('desk-split').doc.active = tabId
  await page.goto(`${base}/desks/desk-split`)
  await expect(page.getByRole('heading', { name: 'split desk', level: 1 })).toBeVisible()
  await expect(page.locator(`[data-tab="${tabId}"] [role=tab]`)).toHaveAttribute('aria-selected', 'true')
}
const tabDoc = id => lastDoc().tabs.find(t => t.id === id)

// A desk whose one tab shows a pane in each state that needs no press: lost, ended, an ended agent, never started, a
// removed machine, open elsewhere and offline; and two lost shells whose reopen finds the folder gone or unavailable.
const statePanes = ['pane-lost', 'pane-ended', 'pane-agent', 'pane-empty', 'pane-removed', 'pane-elsewhere', 'pane-offline', 'pane-folder', 'pane-unavail']
function resetStates() {
  reset()
  const gone = (id, state, over = {}) => ({ ...running(id, id.slice(5)), state, exitCode: state === 'exited' ? 0 : null, ...over })
  sessions.push(gone('sess-lost', 'lost'), gone('sess-ended', 'exited'), gone('sess-agent', 'exited', { tool: 'claude', dir: '/home/dev/app' }), gone('sess-removed', 'lost'),
    running('sess-elsewhere', 'elsewhere'), running('sess-offline', 'offline'), gone('sess-folder', 'lost'), gone('sess-unavail', 'lost'))
  const p = (id, name, session, over = {}) => ({ ...paneOf(id, name, session, '/home/dev/PROD_reet'), ...over })
  const panes = {
    'pane-lost': p('pane-lost', 'lost one', 'sess-lost'), 'pane-ended': p('pane-ended', 'ended one', 'sess-ended'),
    'pane-agent': p('pane-agent', 'agent', 'sess-agent', { kind: 'agent', dir: '/home/dev/app' }), 'pane-empty': p('pane-empty', 'empty', ''),
    'pane-removed': p('pane-removed', 'removed', 'sess-removed', { host: 'connector-removed0000001' }), 'pane-elsewhere': p('pane-elsewhere', 'elsewhere', 'sess-elsewhere'),
    'pane-offline': p('pane-offline', 'offline', 'sess-offline'), 'pane-folder': p('pane-folder', 'old build', 'sess-folder', { dir: '/home/dev/old' }),
    'pane-unavail': p('pane-unavail', 'locked', 'sess-unavail', { dir: '/home/dev/locked' }),
  }
  const root = { split: 'down', ratio: 1 / 3, a: sideOf(statePanes.slice(0, 3)), b: { split: 'down', ratio: 0.5, a: sideOf(statePanes.slice(3, 6)), b: sideOf(statePanes.slice(6)) } }
  desks.set('desk-states', deskOf('desk-states', 'after reboot', [{ id: 'tab-states', name: 'states', colour: '', root, zoomed: '', active: 'pane-lost', panes }], { position: 3 }))
  conflicted.add('sess-elsewhere'); failing.add('sess-offline')
  reopenFail.set('pane-folder', { status: 409, json: { error: 'That folder no longer exists on studio.', code: 'folder_gone' } })
  reopenFail.set('pane-unavail', { status: 409, json: { error: "reet couldn't open a shell in that folder.", code: 'folder_unavailable' } })
}
async function openStates() {
  await page.goto(`${base}/desks/desk-states`)
  await expect(page.getByRole('heading', { name: 'after reboot', level: 1 })).toBeVisible()
}
/** A pane's card: its state, its tag, its heading (a real heading) and its main action. */
async function expectCard(id, state, tag, heading, primary) {
  await expect(pane(id)).toHaveAttribute('data-state', state)
  await expect(pane(id).locator('.tag')).toHaveText(tag)
  await expect(pane(id).getByRole('heading', { name: heading, exact: true })).toBeVisible()
  await expect(pane(id).locator('.btn.primary')).toHaveText(primary)
}

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

  // ── switching desk at once still saves the change on the first desk ──
  reset()
  await openDesk()
  await quiet(); patches = []; patched = []
  await tab('db').focus()
  await page.keyboard.press('F2')
  await tablist.getByRole('textbox', { name: 'Tab name' }).fill('database')
  await tablist.getByRole('textbox', { name: 'Tab name' }).press('Enter')
  // Within autosave's pause, before any save is sent.
  await page.getByRole('button', { name: 'prod deploy, switch desk' }).click()
  await page.getByRole('menu', { name: 'Switch desk' }).getByRole('menuitem', { name: 'zoomies dev' }).click()
  await expect(page.getByRole('heading', { name: 'zoomies dev', level: 1 })).toBeVisible()
  await expect.poll(() => patched.length).toBe(1)
  await quiet()
  expect(patched).toEqual(['desk-prod'])
  expect(desks.get('desk-prod').doc.tabs.find(t => t.id === 'tab-db').name).toBe('database')
  console.log('ok - switching desk at once still saves the change on the first desk')

  // ── moves a tab with the menu ──
  reset()
  await openDesk()
  await quiet(); patches = []
  await tab('deploy').focus()
  await page.keyboard.press('Shift+F10')
  const moveMenu = page.getByRole('menu', { name: 'Actions for tab deploy' })
  await expect(moveMenu.getByRole('menuitem', { name: 'Move left' })).toBeDisabled()
  await moveMenu.getByRole('menuitem', { name: 'Move right' }).click()
  await expect.poll(tabNames).toEqual(['logs', 'deploy', 'db'])
  await expect(tab('deploy')).toBeFocused()
  await page.keyboard.press('Shift+F10')
  await expect(page.getByRole('menu', { name: 'Actions for tab deploy' }).getByRole('menuitem', { name: 'Move left' })).toBeEnabled()
  await page.keyboard.press('Escape')
  await quiet()
  expect(patches.length).toBe(1)
  expect(lastDoc().tabs.map(t => t.id)).toEqual(['tab-logs', 'tab-deploy', 'tab-db'])
  await tab('db').click({ button: 'right' })
  await expect(page.getByRole('menu', { name: 'Actions for tab db' }).getByRole('menuitem', { name: 'Move right' })).toBeDisabled()
  await page.keyboard.press('Escape')
  console.log('ok - moves a tab with the menu')

  // ── touch targets are 44 px on a touch screen ──
  {
    reset()
    const touch = await browser.newContext({ viewport: { width: 834, height: 1000 }, hasTouch: true, isMobile: true })
    const tp = await touch.newPage()
    tp.on('pageerror', error => errors.push(error.message))
    await tp.route('**/api/**', api)
    await tp.routeWebSocket('**/api/v2/terminal/socket', socketStub)
    await tp.goto(`${base}/desks/desk-prod`)
    const ttabs = tp.getByRole('tablist', { name: 'Tabs' })
    await expect(ttabs.getByRole('tab', { name: 'deploy', exact: true })).toBeVisible()
    const heights = async locator => (await locator.boundingBox()).height
    for (const [what, locator] of [
      ['tab', ttabs.getByRole('tab', { name: 'deploy', exact: true })],
      ['tab menu', tp.getByRole('button', { name: 'Actions for tab deploy' })],
      ['new tab', tp.getByRole('button', { name: 'New tab' })],
      ['desk name', tp.getByRole('button', { name: 'prod deploy, switch desk' })],
      ['desk menu', tp.getByRole('button', { name: 'Desk menu' })],
    ]) expect(await heights(locator), what).toBeGreaterThanOrEqual(44)
    await tp.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
    await tp.goto(`${base}/desks`)
    const title = tp.getByRole('link', { name: 'prod deploy' })
    await expect(title).toBeVisible()
    expect(await heights(title), 'card title').toBeGreaterThanOrEqual(44)
    await touch.close()
  }
  console.log('ok - touch targets are 44 px on a touch screen')

  // ── loading another page does not read the desks ──
  {
    reset()
    const reads = []
    const note = request => { if (new URL(request.url()).pathname.startsWith('/api/v2/desks')) reads.push(request.url()) }
    page.on('request', note)
    await page.goto(`${base}/`)
    await expect(nav.getByRole('link', { name: 'Home' })).toHaveAttribute('aria-current', 'page')
    await page.waitForLoadState('networkidle')
    expect(reads).toEqual([])
    // Jump reads them once it is opened, to offer each desk.
    await page.getByRole('combobox', { name: 'Jump to a session, project, skill or machine' }).fill('prod')
    await expect(page.getByRole('listbox', { name: 'Jump results' }).getByRole('option', { name: /prod deploy/ })).toHaveAttribute('href', '/desks/desk-prod')
    expect(reads.length).toBeGreaterThan(0)
    page.off('request', note)
  }
  console.log('ok - loading another page does not read the desks')

  // ── splits right by key and saves the tree ──
  reset()
  // The shell has moved since the pane was opened: the split opens where it is now.
  sessions = sessions.map(x => x.id === 'sess-deploy' ? { ...x, liveDir: '/home/dev/PROD_reet/web' } : x)
  await openDesk()
  await attached(['sess-deploy'])
  await quiet(); patches = []
  await pane('pane-deploy').locator('.xterm').click()
  const typed = sockets.get('sess-deploy').input.length
  await chord('KeyD')
  await expect(page.locator('.desk-pane')).toHaveCount(2)
  expect(creates).toHaveLength(1)
  expect(creates[0]).toMatchObject({ agentId: agent, title: 'web', dir: '/home/dev/PROD_reet/web' })
  await quiet()
  {
    const t = tabDoc('tab-deploy'), added = Object.keys(t.panes).find(id => id !== 'pane-deploy')
    expect(t.root).toEqual({ split: 'right', ratio: 0.5, a: leaf('pane-deploy'), b: leaf(added) })
    expect(t.panes[added]).toMatchObject({ session: 'sess-new-1', host: agent, kind: 'shell', dir: '/home/dev/PROD_reet/web' })
    expect(t.active).toBe(added)
    await expect(pane(added)).toHaveClass(/focused/)
    // The chord never reached the shell.
    expect(sockets.get('sess-deploy').input.slice(typed)).toEqual([])
    // Alt+Shift+E splits the new pane down, with another fresh id.
    await attached(['sess-new-1'])
    await chord('KeyE')
    await expect(page.locator('.desk-pane')).toHaveCount(3)
    await quiet()
    const t2 = tabDoc('tab-deploy'), third = Object.keys(t2.panes).find(id => id !== 'pane-deploy' && id !== added)
    expect(t2.root).toEqual({ split: 'right', ratio: 0.5, a: leaf('pane-deploy'), b: { split: 'down', ratio: 0.5, a: leaf(added), b: leaf(third) } })
    expect(new Set(lastDoc().tabs.flatMap(x => Object.keys(x.panes))).size).toBe(5)
  }
  console.log('ok - splits right by key and saves the tree')

  // ── the tenth pane is refused ──
  reset()
  await openSplit('tab-nine')
  await quiet(); patches = []
  await pane('pane-n5').click()
  await chord('KeyD')
  await expect(page.locator('.notice.bad')).toContainText('A tab holds at most nine panes.')
  await expect(page.locator('.desk-pane')).toHaveCount(9)
  // Four side by side: halving one would leave it under 20 columns, so that is refused too; splitting it down has room.
  await tab('four').click()
  await pane('pane-d2').locator('.ph .nm').click()
  await page.getByRole('button', { name: 'Split right' }).click()
  await expect(page.locator('.notice.bad')).toContainText("There isn't room to split this pane.")
  expect(creates).toEqual([])
  await page.getByRole('button', { name: 'Split down' }).click()
  await expect(page.locator('.desk-pane')).toHaveCount(5)
  expect(creates).toHaveLength(1)
  console.log('ok - the tenth pane is refused')

  // ── drags a separator and sends one resize ──
  reset()
  await openSplit()
  await attached(['sess-a', 'sess-b', 'sess-c'])
  await quiet(); patches = []
  for (const r of sockets.values()) r.resizes.length = 0
  {
    const sep = page.locator('[role=separator][aria-orientation=vertical]')
    await expect(sep).toHaveAttribute('aria-valuenow', '50')
    await expect(sep).toHaveAccessibleName('Resize alpha and beta')
    await expect(page.locator('[role=separator][aria-orientation=horizontal]')).toHaveAccessibleName('Resize alpha and gamma')
    const box = await sep.boundingBox(), y = box.y + box.height / 2
    await page.mouse.move(box.x + box.width / 2, y)
    await page.mouse.down()
    for (let i = 1; i <= 4; i++) { await page.mouse.move(box.x + box.width / 2 + i * 40, y, { steps: 3 }); await page.waitForTimeout(250) }
    // Nothing reaches the machines while the separator is held, however long the pauses.
    expect(sockets.get('sess-a').resizes).toEqual([])
    expect(sockets.get('sess-b').resizes).toEqual([])
    await page.mouse.up()
    await quiet()
    expect(sockets.get('sess-a').resizes).toHaveLength(1)
    expect(sockets.get('sess-b').resizes).toHaveLength(1)
    expect(sockets.get('sess-c').resizes).toEqual([])
    expect(sockets.get('sess-a').resizes[0][0]).toBeGreaterThan(sockets.get('sess-b').resizes[0][0])
    expect(patches.length).toBe(1)
    const ratio = tabDoc('tab-three').root.a.ratio
    expect(ratio).toBeGreaterThan(0.6)
    await expect(sep).toHaveAttribute('aria-valuenow', String(Math.round(ratio * 100)))
  }
  console.log('ok - drags a separator and sends one resize')

  // ── arrow keys move a separator by 2 % ──
  reset()
  await openSplit()
  await quiet(); patches = []
  {
    const sep = page.locator('[role=separator][aria-orientation=vertical]')
    await sep.focus()
    await page.keyboard.press('ArrowRight')
    await expect(sep).toHaveAttribute('aria-valuenow', '52')
    await page.keyboard.press('ArrowLeft')
    await page.keyboard.press('ArrowLeft')
    await expect(sep).toHaveAttribute('aria-valuenow', '48')
    await quiet()
    expect(tabDoc('tab-three').root.a.ratio).toBeCloseTo(0.48, 5)
    // Up and down move the one between rows; End goes as far as the panes allow, a double-click halves it again.
    const rows = page.locator('[role=separator][aria-orientation=horizontal]')
    await rows.focus()
    await page.keyboard.press('ArrowDown')
    await expect(rows).toHaveAttribute('aria-valuenow', '52')
    await page.keyboard.press('End')
    // End heads for 90 % but stops where the pane below would drop under 8 rows.
    await expect.poll(async () => Number(await rows.getAttribute('aria-valuenow'))).toBeGreaterThan(60)
    expect(Number(await rows.getAttribute('aria-valuenow'))).toBeLessThan(90)
    await rows.dblclick()
    await expect(rows).toHaveAttribute('aria-valuenow', '50')
    await quiet()
    expect(tabDoc('tab-three').root.ratio).toBe(0.5)
  }
  console.log('ok - arrow keys move a separator by 2 %')

  // ── grid preset of four ──
  reset()
  ticketDelay = 400
  await openSplit('tab-four')
  await attached(['sess-d1', 'sess-d2', 'sess-d3', 'sess-d4'])
  // Attaches start at most three at a time.
  expect(attachingMost).toBe(3)
  await quiet(); patches = []
  await page.getByRole('button', { name: 'Layout' }).click()
  await page.getByRole('menu', { name: 'Layout' }).getByRole('menuitem', { name: 'Grid' }).click()
  await quiet()
  expect(tabDoc('tab-four').root).toEqual({ split: 'right', ratio: 0.5, a: { split: 'down', ratio: 0.5, a: leaf('pane-d1'), b: leaf('pane-d2') }, b: { split: 'down', ratio: 0.5, a: leaf('pane-d3'), b: leaf('pane-d4') } })
  expect((await pane('pane-d2').boundingBox()).y).toBeGreaterThan((await pane('pane-d1').boundingBox()).y + 100)
  // The panes moved; their terminals did not reconnect.
  expect([...sockets.values()].every(r => !r.closed)).toBe(true)
  expect(sockets.size).toBe(4)
  console.log('ok - grid preset of four')

  // ── zoom hides siblings without unmounting them ──
  reset()
  await openSplit()
  await attached(['sess-a', 'sess-b', 'sess-c'])
  await quiet(); patches = []
  await pane('pane-b').locator('.ph .nm').click()
  await chord('KeyZ')
  await expect(pane('pane-a')).toBeHidden()
  await expect(pane('pane-c')).toBeHidden()
  await expect(pane('pane-b')).toBeVisible()
  await expect(page.locator('.desk-pane .xterm')).toHaveCount(3)
  expect(['a', 'b', 'c'].every(x => !sockets.get(`sess-${x}`).closed)).toBe(true)
  await expect(page.getByRole('button', { name: 'Show all panes' })).toHaveAttribute('aria-pressed', 'true')
  await quiet()
  expect(tabDoc('tab-three').zoomed).toBe('pane-b')
  await chord('KeyZ')
  await expect(pane('pane-a')).toBeVisible()
  await expect(pane('pane-c')).toBeVisible()
  expect(sockets.size).toBe(3)
  await quiet()
  expect(tabDoc('tab-three').zoomed).toBe('')
  console.log('ok - zoom hides siblings without unmounting them')

  // ── the toolbar acts on the focused pane ──
  reset()
  await openSplit()
  await attached(['sess-a', 'sess-b', 'sess-c'])
  await pane('pane-b').locator('.xterm').click()
  await expect(page.getByRole('toolbar', { name: 'Terminal tools for beta' })).toBeVisible()
  {
    const interrupt = page.getByRole('button', { name: 'Interrupt shell' })
    await expect(interrupt).toBeEnabled()
    const sentA = sockets.get('sess-a').input.length
    await interrupt.click()
    await expect.poll(() => sockets.get('sess-b').input).toContain('\x03')
    expect(sockets.get('sess-a').input.length).toBe(sentA)
    await pane('pane-a').locator('.xterm').click()
    await expect(page.getByRole('toolbar', { name: 'Terminal tools for alpha' })).toBeVisible()
    await interrupt.click()
    await expect.poll(() => sockets.get('sess-a').input).toContain('\x03')
  }
  // One toolbar for the tab: no pane carries its own toolbar, header or footer.
  await expect(page.getByRole('toolbar')).toHaveCount(1)
  await expect(page.locator('.desk-pane .terminal-toolbar, .desk-pane .pane > header, .desk-pane .pane > footer')).toHaveCount(0)
  console.log('ok - the toolbar acts on the focused pane')

  // ── Alt+Shift+arrow moves focus ──
  reset()
  await openSplit()
  await attached(['sess-a', 'sess-b', 'sess-c'])
  await quiet(); patches = []
  await pane('pane-a').locator('.xterm').click()
  {
    const sent = ['a', 'b', 'c'].map(x => sockets.get(`sess-${x}`).input.length)
    await chord('ArrowRight')
    await expect(pane('pane-b')).toHaveClass(/focused/)
    await expect(pane('pane-b').locator('textarea')).toBeFocused()
    await chord('ArrowDown')
    await expect(pane('pane-c')).toHaveClass(/focused/)
    await chord('ArrowUp')
    await expect(pane('pane-a')).toHaveClass(/focused/)
    await expect(pane('pane-a').locator('textarea')).toBeFocused()
    // No arrow reached a shell.
    expect(['a', 'b', 'c'].map(x => sockets.get(`sess-${x}`).input.length)).toEqual(sent)
    await chord('ArrowRight')
    await quiet()
    expect(tabDoc('tab-three').active).toBe('pane-b')
    // Alt+Shift+] and [ change tab; Alt+Shift+2 picks the second.
    await chord('BracketRight')
    await expect(tab('four')).toHaveAttribute('aria-selected', 'true')
    await chord('BracketLeft')
    await expect(tab('deploy')).toHaveAttribute('aria-selected', 'true')
    await chord('Digit3')
    await expect(tab('nine')).toHaveAttribute('aria-selected', 'true')
  }
  // The desk menu's Keys lists them.
  await page.getByRole('button', { name: 'Desk menu' }).click()
  await page.getByRole('menuitem', { name: 'Keys' }).click()
  {
    const keys = page.getByRole('dialog', { name: 'Keys' })
    await expect(keys).toContainText('Alt+Shift+D')
    await expect(keys).toContainText('Split right')
    await keys.getByRole('button', { name: 'Done' }).click()
    await expect(keys).toHaveCount(0)
  }
  console.log('ok - Alt+Shift+arrow moves focus')

  // ── chords stay out of text fields ──
  reset()
  await openDesk()
  await attached(['sess-deploy'])
  await quiet(); patches = []
  // One pane: nothing to zoom between.
  await expect(page.getByRole('button', { name: 'Zoom' })).toBeDisabled()
  await expect(pane('pane-deploy')).toHaveAttribute('aria-current', 'true')
  await tab('db').focus()
  await page.keyboard.press('F2')
  {
    const field = tablist.getByRole('textbox', { name: 'Tab name' })
    await expect(field).toBeFocused()
    await chord('KeyW')
    await chord('KeyD')
    await page.waitForTimeout(300)
    await expect(page.getByRole('dialog')).toHaveCount(0)
    expect(creates).toEqual([])
    await expect(field).toBeFocused()
    await field.press('Escape')
  }
  // Held down, Alt+Shift+D starts one shell, not one per repeat.
  await pane('pane-deploy').locator('.xterm').click()
  await page.keyboard.down('Alt'); await page.keyboard.down('Shift')
  await page.keyboard.down('KeyD'); await page.keyboard.down('KeyD'); await page.keyboard.down('KeyD')
  await page.keyboard.up('KeyD'); await page.keyboard.up('Shift'); await page.keyboard.up('Alt')
  await expect(page.locator('.desk-pane')).toHaveCount(2)
  await page.waitForTimeout(500)
  expect(creates).toHaveLength(1)
  console.log('ok - chords stay out of text fields')

  // ── a pane's failure leaves its neighbours live ──
  reset()
  failing.add('sess-b')
  await openSplit()
  await expect.poll(() => sockets.get('sess-b')?.closed).toBe(true)
  await expect(pane('pane-b')).toHaveAttribute('data-state', 'reconnecting')
  await expect(pane('pane-b')).toContainText('studio is not reporting.')
  await expect(pane('pane-a')).toHaveAttribute('data-state', 'live')
  await expect(pane('pane-c')).toHaveAttribute('data-state', 'live')
  await expect(pane('pane-a').locator('.word')).toHaveText('Running')
  expect(sockets.get('sess-a').closed).toBe(false)
  expect(sockets.get('sess-c').closed).toBe(false)
  await pane('pane-c').locator('.xterm').click()
  await page.getByRole('button', { name: 'Interrupt shell' }).click()
  await expect.poll(() => sockets.get('sess-c').input).toContain('\x03')
  console.log("ok - a pane's failure leaves its neighbours live")

  // ── theatre leaves the tab bar and panes ──
  reset()
  await openSplit()
  await page.getByRole('button', { name: 'Theatre view' }).click()
  await expect(page.locator('.site-header')).toBeHidden()
  await expect(page.getByRole('heading', { name: 'split desk', level: 1 })).toBeHidden()
  await expect(tablist).toBeVisible()
  await expect(page.locator('.desk-pane:visible')).toHaveCount(3)
  await page.getByRole('button', { name: 'Exit theatre view' }).click()
  await expect(page.locator('.site-header')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'split desk', level: 1 })).toBeVisible()
  console.log('ok - theatre leaves the tab bar and panes')

  // ── the pane header follows the session's folder ──
  reset()
  await openDesk()
  {
    const dir = pane('pane-deploy').locator('.ph .dir')
    await expect(dir).toHaveText('~/PROD_reet')
    const deep = '/home/dev/PROD_reet/web/src/lib/desks/fixtures/a-rather-long-folder-name'
    sessions = sessions.map(x => x.id === 'sess-deploy' ? { ...x, liveDir: deep } : x)
    // The page reads the sessions again when it comes back into view (and every 15 s).
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
    await expect(dir).toHaveAttribute('title', deep)
    await expect(dir).toHaveText(/^~\/PROD_re.*….*a-rather-long-folder-name$/)
    expect([...(await dir.textContent())].length).toBe(48)
    sessions = sessions.map(x => x.id === 'sess-deploy' ? { ...x, liveDir: '/home/dev' } : x)
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
    await expect(dir).toHaveText('~')
  }
  console.log("ok - the pane header follows the session's folder")

  // ── swap, move to tab and drag to an edge ──
  reset()
  await openSplit()
  await attached(['sess-a', 'sess-b', 'sess-c'])
  await quiet(); patches = []
  await page.getByRole('button', { name: 'Actions for alpha' }).click()
  await page.getByRole('menuitem', { name: 'Swap with…' }).click()
  await page.getByRole('menu', { name: 'Swap alpha with' }).getByRole('menuitem', { name: 'gamma' }).click()
  await quiet()
  expect(tabDoc('tab-three').root).toEqual({ split: 'down', ratio: 0.5, a: { split: 'right', ratio: 0.5, a: leaf('pane-c'), b: leaf('pane-b') }, b: leaf('pane-a') })
  expect(['a', 'b', 'c'].every(x => !sockets.get(`sess-${x}`).closed)).toBe(true)
  expect((await pane('pane-a').boundingBox()).y).toBeGreaterThan((await pane('pane-c').boundingBox()).y + 100)
  await page.getByRole('button', { name: 'Actions for beta' }).click()
  await page.getByRole('menuitem', { name: 'Move to tab…' }).click()
  await page.getByRole('menu', { name: 'Move beta to tab' }).getByRole('menuitem', { name: 'four' }).click()
  await expect(tab('four')).toHaveAttribute('aria-selected', 'true')
  await quiet()
  expect(tabDoc('tab-three').panes['pane-b']).toBeUndefined()
  expect(tabDoc('tab-four').panes['pane-b']).toMatchObject({ session: 'sess-b' })
  expect(tabDoc('tab-four').root.a).toEqual({ split: 'right', ratio: 0.5, a: leaf('pane-d1'), b: leaf('pane-b') })
  expect(lastDoc().active).toBe('tab-four')
  // Dragging d4's header onto d1's left edge puts it there, with a preview while it is over the edge.
  {
    const target = await pane('pane-d1').boundingBox()
    await pane('pane-d4').locator('.ph .nm').dragTo(pane('pane-d1'), { targetPosition: { x: 6, y: target.height / 2 } })
    await quiet()
    const root = tabDoc('tab-four').root
    expect(root.a).toEqual({ split: 'right', ratio: 0.5, a: { split: 'right', ratio: 0.5, a: leaf('pane-d4'), b: leaf('pane-d1') }, b: leaf('pane-b') })
  }
  console.log('ok - swap, move to tab and drag to an edge')

  // ── a phone shows one pane, with the names as chips ──
  {
    reset()
    const phoneContext = await browser.newContext({ viewport: { width: 390, height: 780 }, hasTouch: true, isMobile: true })
    const pp = await phoneContext.newPage()
    pp.on('pageerror', error => errors.push(error.message))
    await pp.route('**/api/**', api)
    await pp.routeWebSocket('**/api/v2/terminal/socket', socketStub)
    await pp.goto(`${base}/desks/desk-split`)
    const chips = pp.getByRole('group', { name: 'Panes on this tab' })
    await expect(chips.getByRole('button')).toHaveText(['alpha', 'beta', 'gamma'])
    await expect(chips.getByRole('button', { name: 'alpha' })).toHaveAttribute('aria-pressed', 'true')
    await expect(pp.locator('.desk-pane:visible')).toHaveCount(1)
    await expect(pp.locator('.desk-pane[data-pane="pane-a"]')).toBeVisible()
    await chips.getByRole('button', { name: 'beta' }).tap()
    await expect(pp.locator('.desk-pane[data-pane="pane-b"]')).toBeVisible()
    await expect(pp.locator('.desk-pane[data-pane="pane-a"]')).toBeHidden()
    expect((await chips.getByRole('button', { name: 'beta' }).boundingBox()).height).toBeGreaterThanOrEqual(44)
    // No splits, presets or zoom on a phone, and the saved tree is left as it was.
    await expect(pp.getByRole('button', { name: 'Layout' })).toHaveCount(0)
    await expect(pp.getByRole('button', { name: 'Split right' })).toHaveCount(0)
    await expect(pp.getByRole('button', { name: 'Zoom' })).toHaveCount(0)
    await expect(pp.getByRole('separator')).toHaveCount(0)
    await pp.waitForTimeout(700)
    expect(desks.get('desk-split').doc.tabs[0].root).toEqual(twoPlusOne)
    expect(await pp.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    if (shots) await pp.screenshot({ path: `${shots}/desk-split-phone.png` })
    await phoneContext.close()
  }
  console.log('ok - a phone shows one pane, with the names as chips')

  // ── each pane state shows its action ──
  {
    resetStates()
    await openStates()
    const cases = [
      ['pane-lost', 'lost', 'Lost', 'studio restarted, so this shell was lost.', 'Reopen in ~/PROD_reet'],
      ['pane-ended', 'ended', 'Ended', 'This shell ended.', 'Reopen in ~/PROD_reet'],
      ['pane-agent', 'agentEnded', 'Agent ended', 'Claude Code finished in this pane.', 'Start a shell here'],
      ['pane-empty', 'empty', 'Not started', 'studio · ~/PROD_reet', 'Start'],
      ['pane-removed', 'machineGone', 'Machine removed', 'This machine was removed from reet.', 'Choose a machine'],
      ['pane-elsewhere', 'elsewhere', 'Open elsewhere', 'This terminal is open in another window.', 'Use it here'],
      ['pane-offline', 'reconnecting', 'Machine offline', 'studio is not reporting.', 'Retry now'],
    ]
    for (const [id, state, tag, heading, primary] of cases) await expectCard(id, state, tag, heading, primary)
    await expect(pane('pane-lost')).toContainText("It was in ~/PROD_reet. A new shell opens there with this pane's command history.")
    await expect(pane('pane-lost').getByRole('button', { name: 'Close pane' })).toBeVisible()
    // Where the agent's own resume is not on offer the card offers only the plain shell.
    await expect(pane('pane-agent')).toContainText('Start a plain shell in ~/app.')
    await expect(pane('pane-agent')).not.toContainText('Continue the same conversation')
    await expect(pane('pane-agent').getByRole('button', { name: 'Continue' })).toHaveCount(0)
    await expect(pane('pane-elsewhere')).toContainText('A terminal has one keyboard at a time.')
    // Folder gone and folder unavailable are what a reopen finds.
    await pane('pane-folder').getByRole('button', { name: 'Reopen in ~/old' }).click()
    await expectCard('pane-folder', 'folderGone', 'Folder gone', '~/old no longer exists on studio.', 'Choose a folder')
    await expect(pane('pane-folder')).toContainText("reet won't quietly open somewhere else.")
    await expect(pane('pane-folder').getByRole('button', { name: 'Reopen in home' })).toBeVisible()
    await pane('pane-unavail').getByRole('button', { name: 'Reopen in ~/locked' }).click()
    await expectCard('pane-unavail', 'folderUnavailable', 'Folder unavailable', "reet couldn't open a shell in ~/locked on studio.", 'Choose a folder')
    // The change is said once, politely; the cards drawn on opening are not read out.
    await expect(pane('pane-unavail').locator('[role=status]')).toHaveText("Folder unavailable. reet couldn't open a shell in ~/locked on studio.")
    await expect(pane('pane-lost').locator('[role=status]')).toHaveText('')
    // Every action is reachable by keyboard.
    await pane('pane-lost').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).focus()
    await page.keyboard.press('Tab')
    await expect(pane('pane-lost').getByRole('button', { name: 'Close pane' })).toBeFocused()
    if (shots) await page.screenshot({ path: `${shots}/desk-states-light.png` })
    console.log('ok - each pane state shows its action')
  }

  // ── opening a desk with lost panes sends no reopen ──
  resetStates()
  await openStates()
  await quiet()
  expect(reopens).toEqual([])
  expect(creates).toEqual([])
  await expect(page.getByRole('button', { name: /^Reopen all/ })).toHaveText('Reopen all (4)')
  console.log('ok - opening a desk with lost panes sends no reopen')

  // ── Reopen calls the route once and mounts the new session ──
  {
    resetStates()
    await openStates()
    const before = desks.get('desk-states').version
    await pane('pane-lost').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
    await expect(pane('pane-lost')).toHaveAttribute('data-state', 'live')
    expect(reopens).toEqual([{ pane: 'pane-lost', expect: before }])
    await attached(['sess-re-1'])
    // The desk the route answered with is the one autosave now holds: the next edit saves on its version.
    await pane('pane-lost').getByRole('button', { name: 'Actions for lost one' }).click()
    await page.getByRole('menuitem', { name: 'Rename' }).click()
    await page.getByRole('textbox', { name: 'Pane name' }).fill('found one')
    await page.keyboard.press('Enter')
    await expect.poll(() => patches.length).toBe(1)
    expect(patches[0].expect).toBe(before + 1)
    expect(patches[0].doc.tabs[0].panes['pane-lost'].session).toBe('sess-re-1')
    await quiet()
    expect(reopens.length).toBe(1)
    console.log('ok - Reopen calls the route once and mounts the new session')
  }

  // ── Reopen in home sends home true ──
  resetStates()
  await openStates()
  await pane('pane-folder').getByRole('button', { name: 'Reopen in ~/old' }).click()
  await pane('pane-folder').getByRole('button', { name: 'Reopen in home' }).click()
  await expect(pane('pane-folder')).toHaveAttribute('data-state', 'live')
  expect(reopens.map(r => r.home ?? false)).toEqual([false, true])
  console.log('ok - Reopen in home sends home true')

  // ── a stale reopen re-reads the desk and retries once ──
  {
    resetStates()
    await openStates()
    staleReopens = 1
    await pane('pane-ended').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
    await expect(pane('pane-ended')).toHaveAttribute('data-state', 'live')
    // Pressing the card makes its pane the focused one, which is saved first: the version sent is the saved one.
    expect(reopens[0].expect).toBe(desks.get('desk-states').version - 2)
    expect(reopens[1].expect).toBe(reopens[0].expect + 1)
    // Twice stale: it gives up and says so in the card.
    staleReopens = 2
    await pane('pane-lost').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
    await expect(pane('pane-lost').getByRole('alert')).toHaveText('This desk changed in another window. Try again.')
    expect(reopens.length).toBe(4)
    console.log('ok - a stale reopen re-reads the desk and retries once')
  }

  // ── Reopen all reopens one at a time and reports the one that failed ──
  {
    resetStates()
    const p = (id, name) => ({ ...paneOf(id, name, `sess-${id}`, '/home/dev/PROD_reet') })
    const names = ['web', 'api', 'db', 'logs', 'jobs', 'cache']
    for (const n of names) sessions.push({ ...running(`sess-${n}`, n), state: 'lost' })
    desks.set('desk-many', deskOf('desk-many', 'after reboot', [
      { id: 'tab-m1', name: 'one', colour: '', root: sideOf(['web', 'api', 'db']), zoomed: '', active: 'web', panes: Object.fromEntries(names.slice(0, 3).map(n => [n, p(n, n)])) },
      { id: 'tab-m2', name: 'two', colour: '', root: sideOf(['logs', 'jobs', 'cache']), zoomed: '', active: 'logs', panes: Object.fromEntries(names.slice(3).map(n => [n, p(n, n)])) },
    ]))
    reopenFail.set('db', { status: 409, json: { error: 'That folder no longer exists on studio.', code: 'folder_gone' } })
    reopenDelay = 400
    await page.goto(`${base}/desks/desk-many`)
    await expect(page.getByRole('heading', { name: 'after reboot', level: 1 })).toBeVisible()
    await quiet()
    expect(reopens).toEqual([])
    await page.getByRole('button', { name: 'Reopen all (6)' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Reopened' })).toContainText('Reopened 5. 1 could not be reopened: db (folder gone).', { timeout: 20_000 })
    expect(reopenMost).toBe(1)
    expect(new Set(reopens.map(r => r.pane))).toEqual(new Set(names))
    expect(sessions.filter(s => s.id.startsWith('sess-re-')).length).toBe(5)
    await expect(pane('db')).toHaveAttribute('data-state', 'folderGone')
    await expect(pane('web')).toHaveAttribute('data-state', 'live')
    await expect(page.getByRole('button', { name: /^Reopen all/ })).toHaveCount(0)
    console.log('ok - Reopen all reopens one at a time and reports the one that failed')
  }

  // ── the limit is checked in the browser and no request is sent ──
  resetStates()
  sessions = [...sessions, ...Array.from({ length: 24 - sessions.filter(s => s.state === 'running').length }, (_, i) => running(`sess-fill-${i}`, `fill ${i}`))]
  await openStates()
  await pane('pane-lost').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
  await expectCard('pane-lost', 'limit', 'At the limit', 'studio already has 24 sessions running.', 'Try again')
  await pane('pane-lost').getByRole('button', { name: 'Try again' }).click()
  await quiet()
  expect(reopens).toEqual([])
  expect(creates).toEqual([])
  console.log('ok - the limit is checked in the browser and no request is sent')

  // ── Choose a folder writes the folder, then reopens there ──
  resetStates()
  await openStates()
  await pane('pane-folder').getByRole('button', { name: 'Reopen in ~/old' }).click()
  await pane('pane-folder').getByRole('button', { name: 'Choose a folder' }).click()
  {
    const dialog = page.getByRole('dialog', { name: 'Choose a folder' })
    await expect(dialog.getByRole('combobox', { name: 'Machine' })).toBeDisabled()
    await dialog.getByRole('textbox', { name: 'Folder', exact: true }).fill('/home/dev/new')
    reopenFail.delete('pane-folder')
    await dialog.getByRole('button', { name: 'Reopen' }).click()
    await expect(dialog).toHaveCount(0)
  }
  await expect(pane('pane-folder')).toHaveAttribute('data-state', 'live')
  expect(patches.at(-1).doc.tabs[0].panes['pane-folder'].dir).toBe('/home/dev/new')
  expect(reopens.at(-1)).toEqual({ pane: 'pane-folder', expect: desks.get('desk-states').version - 1 })
  console.log('ok - Choose a folder writes the folder, then reopens there')

  // ── New tab and Split send the pane id with the create ──
  {
    reset()
    await openDesk()
    await quiet(); patches = []
    await page.getByRole('button', { name: 'New tab' }).click()
    await page.getByRole('dialog', { name: 'New tab' }).getByRole('button', { name: 'Open tab' }).click()
    await expect.poll(() => creates.length).toBe(1)
    await quiet()
    const added = lastDoc().tabs.at(-1)
    expect(creates[0].deskPane).toMatch(/^[0-9a-f]{16}$/)
    expect(Object.keys(added.panes)).toEqual([creates[0].deskPane])
    await openSplit()
    await attached(['sess-a', 'sess-b', 'sess-c'])
    await quiet(); patches = []
    await pane('pane-a').locator('.xterm').click()
    await page.getByRole('toolbar', { name: 'Terminal tools for alpha' }).getByRole('button', { name: 'Split right' }).click()
    await expect.poll(() => creates.length).toBe(2)
    await quiet()
    const fresh = Object.keys(tabDoc('tab-three').panes).filter(id => !['pane-a', 'pane-b', 'pane-c'].includes(id))
    expect(fresh).toEqual([creates[1].deskPane])
    // Pane ids are unique across the whole desk.
    expect(lastDoc().tabs.flatMap(t => Object.keys(t.panes)).filter(id => id === creates[1].deskPane)).toHaveLength(1)
    console.log('ok - New tab and Split send the pane id with the create')
  }

  // ── End the shell asks the machine to forget the pane's history ──
  reset()
  await openDesk()
  await page.getByRole('button', { name: 'Actions for reet' }).click()
  await page.getByRole('menuitem', { name: 'Close' }).click()
  await page.getByRole('dialog', { name: 'Close “reet”?' }).getByRole('button', { name: 'End the shell' }).click()
  await expect.poll(() => ends.length).toBe(1)
  expect(ends[0]).toMatchObject({ id: 'sess-deploy', removeHistory: true, forgetDesk: true })
  // A tab's close covers its panes the same way.
  await page.getByRole('button', { name: 'Actions for tab logs' }).click()
  await page.getByRole('menuitem', { name: 'Close tab' }).click()
  await page.keyboard.press('Enter')
  await expect.poll(() => ends.length).toBe(2)
  expect(ends[1]).toMatchObject({ id: 'sess-logs', forgetDesk: true })
  console.log("ok - End the shell asks the machine to forget the pane's history")

  // ── Take off this desk does not ──
  reset()
  await openDesk()
  await page.getByRole('button', { name: 'Actions for reet' }).click()
  await page.getByRole('menuitem', { name: 'Close' }).click()
  {
    const d = page.getByRole('dialog', { name: 'Close “reet”?' })
    await d.getByRole('radio', { name: /Take off this desk/ }).click()
    await d.getByRole('button', { name: 'Take off this desk' }).click()
  }
  await expect.poll(tabNames).toEqual(['logs', 'db'])
  await quiet()
  expect(ends).toEqual([])
  console.log('ok - Take off this desk does not')

  // ── History lists this pane first ──
  {
    reset()
    machines[0].capabilities.commandHistory = true
    histories.set('sess-a', { entries: [{ command: 'make deploy', source: 'pane' }, { command: 'ls -la', source: 'bash' }], sources: [{ source: 'pane', available: true }, { source: 'bash', available: true }], truncated: false, skipped: 0 })
    histories.set('sess-b', { entries: [{ command: 'git status', source: 'bash' }], sources: [{ source: 'pane', available: false, reason: 'This shell started before reet kept a history for its pane.' }, { source: 'bash', available: true }], truncated: false, skipped: 0 })
    await openSplit()
    await attached(['sess-a', 'sess-b', 'sess-c'])
    await pane('pane-a').locator('.xterm').click()
    await page.getByRole('toolbar', { name: 'Terminal tools for alpha' }).getByRole('button', { name: 'Recent history' }).click()
    const drawer = page.getByRole('dialog', { name: 'Command history' })
    await expect(drawer.locator('.group h3')).toHaveText(['This pane', 'This machine'])
    await expect(drawer.getByRole('region', { name: 'This pane' })).toContainText('make deploy')
    await expect(drawer.getByRole('region', { name: 'This machine' })).toContainText('ls -la')
    await expect(drawer.getByRole('region', { name: 'This pane' })).not.toContainText('ls -la')
    await drawer.getByRole('button', { name: 'Close Command history' }).click()
    await expect(drawer).toHaveCount(0)
    await pane('pane-b').locator('.xterm').click()
    await page.getByRole('toolbar', { name: 'Terminal tools for beta' }).getByRole('button', { name: 'Recent history' }).click()
    await expect(drawer.locator('.group h3')).toHaveText(['This pane', 'This machine'])
    await expect(drawer.getByRole('region', { name: 'This pane' })).toContainText('This shell started before reet kept a history for its pane.')
    await expect(drawer.getByRole('region', { name: 'This machine' })).toContainText('git status')
    await drawer.getByRole('button', { name: 'Close Command history' }).click()
    console.log('ok - History lists this pane first')
  }

  // ── Continue on an ended agent pane binds the new session ──
  {
    resetStates()
    machines[0].capabilities.resume = true
    // A lost agent says it was lost, and still offers the agent's own resume.
    sessions = sessions.map(x => x.id === 'sess-agent' ? { ...x, state: 'lost' } : x)
    await openStates()
    await expectCard('pane-agent', 'agentLost', 'Lost', 'studio restarted, so this shell was lost.', 'Continue')
    await expect(pane('pane-agent')).toContainText('Continue the same conversation, or start a plain shell in ~/app.')
    await expect(pane('pane-agent').getByRole('button', { name: 'Start a shell here' })).toBeVisible()
    await quiet(); patches = []
    await pane('pane-agent').getByRole('button', { name: 'Continue' }).click()
    await expect(pane('pane-agent')).toHaveAttribute('data-state', 'live')
    expect(creates).toHaveLength(1)
    expect(creates[0]).toMatchObject({ agentId: agent, tool: 'claude', resume: true, dir: '/home/dev/app' })
    expect(reopens).toEqual([])
    await expect.poll(() => patches.length).toBeGreaterThan(0)
    const bound = patches.at(-1).doc.tabs[0].panes['pane-agent']
    expect(bound).toMatchObject({ session: 'sess-new-1', kind: 'agent' })
    console.log('ok - Continue on an ended agent pane binds the new session')
  }

  // ── Start-up command… saves the command on the pane ──
  {
    resetStates()
    await openStates()
    await quiet(); patches = []
    await pane('pane-lost').getByRole('button', { name: 'Actions for lost one' }).click()
    await page.getByRole('menuitem', { name: 'Start-up command…' }).click()
    const dialog = page.getByRole('dialog', { name: 'Start-up command' })
    const field = dialog.getByRole('textbox', { name: 'Command to offer when this pane is reopened' })
    await expect(field).toBeFocused()
    await expect(field).toHaveAttribute('maxlength', '2000')
    await expect(dialog.getByRole('button', { name: 'Remove' })).toHaveCount(0)
    await field.fill('npm run dev')
    await dialog.getByRole('button', { name: 'Save' }).click()
    await expect(dialog).toHaveCount(0)
    await expect.poll(() => patches.length).toBe(1)
    expect(lastDoc().tabs[0].panes['pane-lost'].start).toBe('npm run dev')
    // Saving it runs nothing and offers nothing: the pane has no shell.
    await expect(page.locator('.sbar')).toHaveCount(0)
    await pane('pane-lost').getByRole('button', { name: 'Actions for lost one' }).click()
    await page.getByRole('menuitem', { name: 'Start-up command…' }).click()
    await expect(field).toHaveValue('npm run dev')
    await dialog.getByRole('button', { name: 'Remove' }).click()
    await expect.poll(() => patches.length).toBe(2)
    expect(lastDoc().tabs[0].panes['pane-lost'].start).toBe('')
    console.log('ok - Start-up command… saves the command on the pane')
  }

  // ── the start-up bar appears after Reopen and Run types the command once ──
  {
    resetStates()
    desks.get('desk-states').doc.tabs[0].panes['pane-lost'].start = 'npm run dev'
    await openStates()
    await expect(page.locator('.sbar')).toHaveCount(0)
    await pane('pane-lost').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
    await expect(pane('pane-lost')).toHaveAttribute('data-state', 'live')
    const bar = pane('pane-lost').getByRole('group', { name: 'Start-up command' })
    await expect(bar).toBeVisible()
    await expect(bar.locator('.lbl')).toHaveText('Start-up command')
    await expect(bar.locator('code')).toHaveText('npm run dev')
    await attached(['sess-re-1'])
    // Nothing is typed before the press.
    await quiet()
    expect(sockets.get('sess-re-1').input.join('')).not.toContain('npm run dev')
    if (shots) {
      // The bar at 1440 px, light and dark.
      await page.setViewportSize({ width: 1440, height: 900 })
      for (const theme of ['light', 'dark']) {
        await page.emulateMedia({ colorScheme: theme })
        await page.evaluate(t => document.documentElement.setAttribute('data-theme', t), theme)
        await page.waitForTimeout(300)
        await page.screenshot({ path: `${shots}/desk-startup-${theme}.png` })
      }
      await page.emulateMedia({ colorScheme: 'light' })
      await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'light'))
      await page.setViewportSize({ width: 1360, height: 860 })
    }
    await bar.getByRole('button', { name: 'Run' }).click()
    await expect(bar).toHaveCount(0)
    await expect.poll(() => sockets.get('sess-re-1').input.join('')).toContain('npm run dev\r')
    await quiet()
    expect(sockets.get('sess-re-1').input.join('').split('npm run dev').length - 1).toBe(1)
    console.log('ok - the start-up bar appears after Reopen and Run types the command once')
  }

  // ── Dismiss sends nothing ──
  {
    resetStates()
    desks.get('desk-states').doc.tabs[0].panes['pane-ended'].start = 'npm run dev'
    await openStates()
    await pane('pane-ended').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
    const bar = pane('pane-ended').getByRole('group', { name: 'Start-up command' })
    await expect(bar).toBeVisible()
    await attached(['sess-re-1'])
    await bar.getByRole('button', { name: 'Dismiss' }).click()
    await expect(bar).toHaveCount(0)
    await quiet()
    expect(sockets.get('sess-re-1').input.join('')).not.toContain('npm run dev')
    console.log('ok - Dismiss sends nothing')
  }

  // ── a live pane shows no start-up bar on load ──
  reset()
  desks.get('desk-prod').doc.tabs[0].panes['pane-deploy'].start = 'make watch'
  await openDesk()
  await attached(['sess-deploy'])
  await quiet()
  await expect(page.locator('.sbar')).toHaveCount(0)
  expect(sockets.get('sess-deploy').input.join('')).not.toContain('make watch')
  console.log('ok - a live pane shows no start-up bar on load')

  // ── Duplicate copies tabs without sessions ──
  {
    reset()
    await page.goto(`${base}/desks`)
    await page.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
    await page.goto(`${base}/desks`)
    await page.getByRole('button', { name: 'Actions for prod deploy' }).click()
    await page.getByRole('menuitem', { name: 'Duplicate' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Sorted.' })).toHaveText('Sorted. “prod deploy copy” added.')
    expect(posts).toEqual([{ name: 'prod deploy copy', colour: 'red', from: 'desk-prod', template: false }])
    await expect(page.getByRole('link', { name: 'prod deploy copy' })).toBeVisible()
    const copy = [...desks.values()].find(d => d.name === 'prod deploy copy')
    expect(copy.template).toBe(false)
    expect(copy.doc.tabs.map(t => t.name)).toEqual(['deploy', 'logs', 'db'])
    expect(copy.doc.tabs.flatMap(t => Object.values(t.panes).map(p => p.session))).toEqual(['', '', ''])
    await page.getByRole('link', { name: 'prod deploy copy' }).click()
    await expect(page.getByRole('heading', { name: 'prod deploy copy', level: 1 })).toBeVisible()
    await expect(page.locator('.desk-pane')).toHaveAttribute('data-state', 'empty')
    await quiet()
    expect(creates).toEqual([])
    expect(reopens).toEqual([])
    console.log('ok - Duplicate copies tabs without sessions')
  }

  // ── a desk from a template starts with Start in every pane ──
  {
    reset()
    // The db pane has no folder of its own: its Start opens in the home folder.
    desks.get('desk-prod').doc.tabs[2].panes['pane-db'].dir = ''
    await page.goto(`${base}/desks`)
    await page.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
    await page.goto(`${base}/desks`)
    await page.getByRole('button', { name: 'Actions for prod deploy' }).click()
    await page.getByRole('menuitem', { name: 'Save as template' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Sorted.' })).toHaveText('Sorted. “prod deploy template” is saved as a template.')
    expect(posts).toEqual([{ name: 'prod deploy template', colour: 'red', from: 'desk-prod', template: true }])
    // A template is not a desk card; it is offered under From a template.
    await expect(page.getByRole('link', { name: 'prod deploy template' })).toHaveCount(0)
    await page.getByRole('button', { name: 'From a template ▾' }).click()
    await page.getByRole('menu', { name: 'From a template' }).getByRole('menuitem', { name: 'prod deploy template' }).click()
    await expect(page.getByRole('heading', { name: 'prod deploy', level: 1 })).toBeVisible()
    expect(posts.at(-1)).toMatchObject({ from: [...desks.values()].find(d => d.template).id })
    const made = [...desks.values()].at(-1)
    expect(made.template).toBe(false)
    if (shots) {
      for (const theme of ['light', 'dark']) {
        const wide = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: theme })
        const wp = await wide.newPage()
        await wp.route('**/api/**', api)
        await wp.routeWebSocket('**/api/v2/terminal/socket', socketStub)
        await wp.goto(`${base}/desks/${made.id}`)
        await wp.evaluate(t => document.documentElement.setAttribute('data-theme', t), theme)
        await expect(wp.locator('.desk-pane')).toHaveAttribute('data-state', 'empty')
        await wp.screenshot({ path: `${shots}/desk-from-template-${theme}.png` })
        await wp.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
        await wp.goto(`${base}/desks`)
        await wp.evaluate(t => document.documentElement.setAttribute('data-theme', t), theme)
        await wp.getByRole('button', { name: 'From a template ▾' }).click()
        await expect(wp.getByRole('menu', { name: 'From a template' })).toBeVisible()
        await wp.screenshot({ path: `${shots}/desks-templates-${theme}.png` })
        await wide.close()
      }
    }
    for (const name of ['deploy', 'logs', 'db']) {
      await tab(name).click()
      await expect(tab(name)).toHaveAttribute('aria-selected', 'true')
      await expectCard(await page.locator('.desk-pane').getAttribute('data-pane'), 'empty', 'Not started', name === 'db' ? 'studio · ~' : `studio · ${name === 'deploy' ? '~/PROD_reet' : '~/PROD_platform'}`, 'Start')
    }
    await quiet()
    expect(creates).toEqual([])
    expect(reopens).toEqual([])
    await page.locator('.desk-pane').getByRole('button', { name: 'Start' }).click()
    await expect(page.locator('.desk-pane')).toHaveAttribute('data-state', 'live')
    expect(reopens.at(-1)).toMatchObject({ home: true })
    await tab('deploy').click()
    await page.locator('.desk-pane').getByRole('button', { name: 'Start' }).click()
    await expect(page.locator('.desk-pane')).toHaveAttribute('data-state', 'live')
    expect(reopens.at(-1).home).toBeUndefined()
    console.log('ok - a desk from a template starts with Start in every pane')
  }

  // ── Reopen all counts each started shell once against the limits ──
  {
    resetStates()
    // 22 of studio's 24 are running (the ten of the fixture, the two of the states desk, ten more).
    sessions.push(...Array.from({ length: 10 }, (_, i) => running(`sess-fill-${i}`, `fill ${i}`)))
    const names = ['w1', 'w2', 'w3', 'w4']
    for (const n of names) sessions.push({ ...running(`sess-${n}`, n), state: 'lost' })
    expect(sessions.filter(x => x.state === 'running' && x.agentId === agent).length).toBe(22)
    desks.set('desk-four', deskOf('desk-four', 'four lost', [
      { id: 'tab-w', name: 'w', colour: '', root: sideOf(names), zoomed: '', active: 'w1', panes: Object.fromEntries(names.map(n => [n, paneOf(n, n, `sess-${n}`, '/home/dev/PROD_reet')])) },
    ]))
    await page.goto(`${base}/desks/desk-four`)
    await expect(page.getByRole('heading', { name: 'four lost', level: 1 })).toBeVisible()
    await page.getByRole('button', { name: 'Reopen all (4)' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Reopened' })).toContainText('Reopened 2. 2 could not be reopened: w3 (at the limit), w4 (at the limit).', { timeout: 20_000 })
    expect(reopens.map(r => r.pane)).toEqual(['w1', 'w2'])
    await expect(pane('w3')).toHaveAttribute('data-state', 'limit')
    await expect(pane('w4')).toHaveAttribute('data-state', 'limit')
    console.log('ok - Reopen all counts each started shell once against the limits')
  }

  // ── the account limit is checked in the browser too ──
  resetStates()
  // 64 running across the account, only 12 of them on studio.
  sessions.push(...Array.from({ length: 52 }, (_, i) => ({ ...running(`sess-away-${i}`, `away ${i}`), agentId: 'connector-elsewhere00001' })))
  await openStates()
  await pane('pane-lost').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
  await expectCard('pane-lost', 'limit', 'At the limit', 'You already have 64 sessions running.', 'Try again')
  await quiet()
  expect(reopens).toEqual([])
  console.log('ok - the account limit is checked in the browser too')

  // ── a double press on Reopen sends one request ──
  resetStates()
  reopenDelay = 300
  await openStates()
  await quiet()
  await pane('pane-lost').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).evaluate(button => { button.click(); button.click() })
  // Every action on the card waits while it reopens.
  await expect(pane('pane-lost').getByRole('button', { name: 'Close pane' })).toBeDisabled()
  await expect(pane('pane-lost')).toHaveAttribute('data-state', 'live')
  await quiet()
  expect(reopens.length).toBe(1)
  console.log('ok - a double press on Reopen sends one request')

  // ── a tab rename made just before Reopen survives it ──
  resetStates()
  await openStates()
  await quiet(); patches = []
  await tab('states').dblclick()
  await tablist.getByRole('textbox', { name: 'Tab name' }).fill('after')
  await tablist.getByRole('textbox', { name: 'Tab name' }).press('Enter')
  await pane('pane-lost').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
  await expect(pane('pane-lost')).toHaveAttribute('data-state', 'live')
  await quiet()
  expect(desks.get('desk-states').doc.tabs[0].name).toBe('after')
  expect(desks.get('desk-states').doc.tabs[0].panes['pane-lost'].session).toBe('sess-re-1')
  await expect(tab('after')).toBeVisible()
  console.log('ok - a tab rename made just before Reopen survives it')

  // ── Type in all panes ──
  const allButton = () => page.getByRole('button', { name: 'Type in all panes' })
  const stripe = () => page.locator('.stripe')
  const ask = () => page.getByRole('dialog', { name: 'Type in all panes?' })
  const sent = id => sockets.get(id).input.join('')
  /** Turns it on with the question answered (it is asked once a page). */
  async function typeInAll(n = 3) {
    await allButton().click()
    await ask().getByRole('button', { name: `Type in all ${n}` }).click()
    await expect(stripe()).toHaveText(`Typing in ${n} panes on this tab. Turns off when you change tab.`)
  }
  /** Opens the 2 + 1 tab with its three shells attached and the input so far forgotten. */
  async function openThree() {
    await openSplit()
    await attached(['sess-a', 'sess-b', 'sess-c'])
    await expect(allButton()).toBeEnabled()
    for (const x of ['a', 'b', 'c']) sockets.get(`sess-${x}`).input.length = 0
  }

  // ── turning it on asks once ──
  reset()
  await openThree()
  await expect(allButton()).toHaveAttribute('aria-pressed', 'false')
  await expect(stripe()).toHaveCount(0)
  await allButton().click()
  await expect(ask()).toBeVisible()
  await expect(ask()).toContainText('Everything you type, paste or send from Compose goes to all 3 live panes on this tab until you turn it off or change tab.')
  await expect(ask().getByRole('button', { name: 'Cancel' })).toBeFocused()
  await ask().getByRole('button', { name: 'Cancel' }).click()
  await expect(ask()).toHaveCount(0)
  await expect(allButton()).toHaveAttribute('aria-pressed', 'false')
  await expect(stripe()).toHaveCount(0)
  await typeInAll()
  await expect(allButton()).toHaveAttribute('aria-pressed', 'true')
  await expect(page.locator('.desk-pane .rcv')).toHaveCount(3)
  await expect(pane('pane-a').locator('.rcv')).toHaveText('Receiving')
  // Off, and on again without the question: it is asked once in a page's lifetime.
  await allButton().click()
  await expect(stripe()).toHaveCount(0)
  await expect(page.locator('.desk-pane .rcv')).toHaveCount(0)
  await allButton().click()
  await expect(ask()).toHaveCount(0)
  await expect(stripe()).toBeVisible()
  await expect(allButton()).toHaveAttribute('aria-pressed', 'true')
  console.log('ok - turning it on asks once')

  // ── typing reaches three panes ──
  reset()
  await openThree()
  await typeInAll()
  await expect(pane('pane-a').locator('textarea')).toBeFocused()
  await page.keyboard.type('uptime')
  await page.keyboard.press('Enter')
  for (const x of ['a', 'b', 'c']) await expect.poll(() => sent(`sess-${x}`)).toBe('uptime\r')
  // Focus is never sent: moving to another pane makes it the one typed in, and the others still get each key once.
  await pane('pane-c').locator('.xterm').click()
  await page.keyboard.type('w')
  for (const x of ['a', 'b', 'c']) await expect.poll(() => sent(`sess-${x}`)).toBe('uptime\rw')
  await quiet()
  for (const x of ['a', 'b', 'c']) expect(sent(`sess-${x}`)).toBe('uptime\rw')
  // Nothing of it is saved with the desk.
  expect(JSON.stringify(patches)).not.toMatch(/typeAll|receiving/i)
  console.log('ok - typing reaches three panes')

  // ── paste reaches all panes ──
  reset()
  await openThree()
  await typeInAll()
  await pane('pane-b').locator('.xterm').click()
  await pane('pane-b').locator('textarea').evaluate(el => {
    const data = new DataTransfer(); data.setData('text/plain', 'echo pasted')
    el.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }))
  })
  for (const x of ['a', 'b', 'c']) await expect.poll(() => sent(`sess-${x}`)).toBe('echo pasted')
  console.log('ok - paste reaches all panes')

  // ── Compose's send reaches all panes ──
  reset()
  await openThree()
  await typeInAll()
  await page.getByRole('button', { name: 'Compose' }).click()
  {
    const compose = page.getByRole('dialog').filter({ has: page.getByLabel(/Message for/) })
    await compose.getByLabel(/Message for/).fill('git pull')
    await compose.getByRole('button', { name: 'Send' }).click()
  }
  for (const x of ['a', 'b', 'c']) await expect.poll(() => sent(`sess-${x}`)).toBe('git pull\r')
  // Interrupt stays with the focused pane.
  await page.getByRole('button', { name: 'Interrupt shell' }).click()
  await expect.poll(() => sent('sess-a')).toBe('git pull\r\x03')
  await quiet()
  expect(sent('sess-b')).toBe('git pull\r')
  expect(sent('sess-c')).toBe('git pull\r')
  console.log("ok - Compose's send reaches all panes")

  // ── the start-up command's Run reaches only its own pane ──
  reset()
  sessions = sessions.map(x => x.id === 'sess-c' ? { ...x, state: 'exited', exitCode: 0 } : x)
  desks.get('desk-split').doc.tabs[0].panes['pane-c'].start = 'make watch'
  await openSplit()
  await attached(['sess-a', 'sess-b'])
  await pane('pane-c').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
  await attached(['sess-re-1'])
  {
    const bar = pane('pane-c').getByRole('group', { name: 'Start-up command' })
    await expect(bar).toBeVisible()
    await pane('pane-a').locator('.xterm').click()
    await expect(allButton()).toBeEnabled()
    for (const id of ['sess-a', 'sess-b', 'sess-re-1']) sockets.get(id).input.length = 0
    await typeInAll()
    await bar.getByRole('button', { name: 'Run' }).click()
    await expect.poll(() => sent('sess-re-1')).toContain('make watch\r')
    await quiet()
    expect(sent('sess-a')).not.toContain('make watch')
    expect(sent('sess-b')).not.toContain('make watch')
  }
  console.log("ok - the start-up command's Run reaches only its own pane")

  // ── a zoomed-away pane does not receive ──
  reset()
  await openThree()
  await typeInAll()
  // Zooming changes the panes typed in: it stops at once, and the hidden panes get nothing.
  await chord('KeyZ')
  await expect(pane('pane-b')).toBeHidden()
  await expect(page.getByRole('alert')).toContainText('Stopped typing in all panes: the panes on this tab changed.')
  await expect(stripe()).toHaveCount(0)
  await expect(allButton()).toBeDisabled()
  await expect(allButton()).toHaveAttribute('title', 'Needs two or more live panes on this tab.')
  await pane('pane-a').locator('textarea').focus()
  await page.keyboard.type('ls')
  await expect.poll(() => sent('sess-a')).toBe('ls')
  await quiet()
  expect(sent('sess-b')).toBe('')
  expect(sent('sess-c')).toBe('')
  console.log('ok - a zoomed-away pane does not receive')

  // ── changing tab turns it off ──
  reset()
  await openThree()
  await typeInAll()
  await tab('four').click()
  await expect(stripe()).toHaveCount(0)
  await tab('deploy').click()
  await attached(['sess-a', 'sess-b', 'sess-c'])
  await expect(allButton()).toHaveAttribute('aria-pressed', 'false')
  await expect(stripe()).toHaveCount(0)
  await expect(page.locator('.desk-pane .rcv')).toHaveCount(0)
  for (const x of ['a', 'b', 'c']) sockets.get(`sess-${x}`).input.length = 0
  await pane('pane-a').locator('.xterm').click()
  await page.keyboard.type('pwd')
  await expect.poll(() => sent('sess-a')).toBe('pwd')
  await quiet()
  expect(sent('sess-b')).toBe('')
  expect(sent('sess-c')).toBe('')
  console.log('ok - changing tab turns it off')

  // ── type in all stops when a pane drops ──
  reset()
  await openThree()
  await typeInAll()
  // beta's socket closes, and its machine does not answer the retry.
  failing.add('sess-b')
  {
    const dropped = sockets.get('sess-b')
    await dropped.socket.close({ code: 1011, reason: 'gone' })
    await expect(pane('pane-b')).toHaveAttribute('data-state', 'reconnecting')
    await page.keyboard.type('x')
    await expect.poll(() => sent('sess-a')).toBe('x')
    await expect(page.getByRole('alert')).toContainText('Stopped typing in all panes: beta is no longer connected.')
    await expect(stripe()).toHaveCount(0)
    await quiet()
    expect(sent('sess-c')).toBe('')
    expect(dropped.input.join('')).toBe('')
  }
  console.log('ok - type in all stops when a pane drops')

  // ── a pane taken over elsewhere stops the mode before the next key is sent ──
  reset()
  await openThree()
  await typeInAll()
  {
    // Another window takes gamma: its socket closes and every attach from here is refused as a conflict.
    conflicted.add('sess-c')
    const taken = sockets.get('sess-c')
    await taken.socket.close({ code: 1000, reason: 'taken over' })
    await expect.poll(() => taken.closed).toBe(true)
    // Once this window has seen the socket go, and before anything else is done, the next key is typed.
    await expect(pane('pane-c')).not.toHaveAttribute('data-state', 'live')
    await page.keyboard.type('y')
    await expect.poll(() => sent('sess-a')).toBe('y')
    await expect(page.getByRole('alert')).toContainText('Stopped typing in all panes: gamma is no longer connected.')
    await expect(pane('pane-c')).toHaveAttribute('data-state', 'elsewhere')
    await quiet()
    expect(sent('sess-b')).toBe('')
    expect(taken.input.join('')).toBe('')
  }
  console.log('ok - a pane taken over elsewhere stops the mode before the next key is sent')

  // ── a desk change from another window stops the mode ──
  reset()
  await openThree()
  await typeInAll()
  {
    // Another window closes gamma: the newer desk arrives when this one reads it again.
    const d = desks.get('desk-split'), t = d.doc.tabs[0]
    const { 'pane-c': _gone, ...panes } = t.panes
    desks.set(d.id, { ...d, version: d.version + 1, doc: { ...d.doc, tabs: [{ ...t, root: twoPlusOne.a, panes }, ...d.doc.tabs.slice(1)] } })
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
    await expect(pane('pane-c')).toHaveCount(0)
    await expect(page.getByRole('alert')).toContainText('Stopped typing in all panes: the panes on this tab changed.')
    await expect(stripe()).toHaveCount(0)
    await pane('pane-a').locator('textarea').focus()
    await page.keyboard.type('z')
    await expect.poll(() => sent('sess-a')).toBe('z')
    await quiet()
    expect(sent('sess-b')).toBe('')
  }
  console.log('ok - a desk change from another window stops the mode')

  // ── the mode is off after a reload ──
  reset()
  await openThree()
  await typeInAll()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'split desk', level: 1 })).toBeVisible()
  await attached(['sess-a', 'sess-b', 'sess-c'])
  await expect(allButton()).toBeEnabled()
  await expect(allButton()).toHaveAttribute('aria-pressed', 'false')
  await expect(stripe()).toHaveCount(0)
  expect(await page.evaluate(() => Object.keys(localStorage).filter(k => k.startsWith('reet.desks')))).toEqual(['reet.desks.last.1'])
  // A new page asks again.
  await allButton().click()
  await expect(ask()).toBeVisible()
  await ask().getByRole('button', { name: 'Cancel' }).click()
  console.log('ok - the mode is off after a reload')

  // ── the look, in both themes and on a phone ──
  if (shots) {
    // Three states side by side (lost, folder gone, an ended agent) at 1440 px in both themes, and one card on a phone.
    for (const theme of ['light', 'dark']) {
      resetStates()
      profileTheme = theme
      const d = desks.get('desk-states'), t = d.doc.tabs[0]
      t.root = sideOf(['pane-lost', 'pane-folder', 'pane-agent'])
      const wide = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2, colorScheme: theme })
      const wp = await wide.newPage()
      wp.on('pageerror', error => errors.push(error.message))
      await wp.route('**/api/**', api)
      await wp.routeWebSocket('**/api/v2/terminal/socket', socketStub)
      await wp.goto(`${base}/desks/desk-states`)
      await wp.locator('.desk-pane[data-pane="pane-folder"]').getByRole('button', { name: 'Reopen in ~/old' }).click()
      await expect(wp.locator('.desk-pane[data-pane="pane-folder"]')).toHaveAttribute('data-state', 'folderGone')
      await wp.locator('.desk-pane[data-pane="pane-lost"] .more').focus()
      await expect(wp.locator('.save')).toHaveText('saved')
      await wp.screenshot({ path: `${shots}/desk-pane-states-${theme}.png` })
      await wide.close()
    }
    profileTheme = 'light'
    {
      resetStates()
      const pc = await browser.newContext({ viewport: { width: 390, height: 780 }, hasTouch: true, isMobile: true, colorScheme: 'dark' })
      const pp = await pc.newPage()
      pp.on('pageerror', error => errors.push(error.message))
      await pp.route('**/api/**', api)
      await pp.routeWebSocket('**/api/v2/terminal/socket', socketStub)
      await pp.goto(`${base}/desks/desk-states`)
      const reopenButton = pp.locator('.desk-pane[data-pane="pane-lost"]').getByRole('button', { name: 'Reopen in ~/PROD_reet' })
      await expect(reopenButton).toBeVisible()
      expect((await reopenButton.boundingBox()).height).toBeGreaterThanOrEqual(44)
      expect(await pp.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await pp.screenshot({ path: `${shots}/desk-pane-states-phone.png` })
      await pc.close()
    }
    // Three panes receiving Type in all panes at 1440 px, light and dark at twice the pixels.
    for (const theme of ['light', 'dark']) {
      reset()
      profileTheme = theme
      desks.get('desk-split').doc.active = 'tab-three'
      const wide = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2, colorScheme: theme })
      const wp = await wide.newPage()
      wp.on('pageerror', error => errors.push(error.message))
      await wp.route('**/api/**', api)
      await wp.routeWebSocket('**/api/v2/terminal/socket', socketStub)
      await wp.goto(`${base}/desks/desk-split`)
      await expect(wp.locator('.desk-pane .xterm-screen')).toHaveCount(3)
      await expect(wp.getByRole('button', { name: 'Type in all panes' })).toBeEnabled()
      await wp.getByRole('button', { name: 'Type in all panes' }).click()
      await wp.getByRole('button', { name: 'Type in all 3' }).click()
      await expect(wp.locator('.stripe')).toBeVisible()
      await wp.keyboard.type('uptime')
      await wp.waitForTimeout(600)
      await wp.screenshot({ path: `${shots}/desk-type-in-all-${theme}.png` })
      await wide.close()
    }
    profileTheme = 'light'
    // A 2 + 1 tab and a grid of four at 1440 px, light and dark; the dark 2 + 1 again at twice the pixels.
    reset()
    const wide = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2 })
    const wp = await wide.newPage()
    await wp.route('**/api/**', api)
    await wp.routeWebSocket('**/api/v2/terminal/socket', socketStub)
    for (const theme of ['light', 'dark']) {
      await wp.emulateMedia({ colorScheme: theme })
      profileTheme = theme
      for (const [tabId, name] of [['tab-three', '2plus1'], ['tab-four', 'grid4']]) {
        desks.get('desk-split').doc.active = tabId
        if (tabId === 'tab-four') desks.get('desk-split').doc.tabs[1].root = { split: 'right', ratio: 0.5, a: { split: 'down', ratio: 0.5, a: leaf('pane-d1'), b: leaf('pane-d2') }, b: { split: 'down', ratio: 0.5, a: leaf('pane-d3'), b: leaf('pane-d4') } }
        await wp.goto(`${base}/desks/desk-split`)
        await wp.evaluate(t => document.documentElement.setAttribute('data-theme', t), theme)
        await expect(wp.locator('.desk-pane .xterm-screen')).toHaveCount(tabId === 'tab-three' ? 3 : 4)
        await wp.waitForTimeout(800)
        await wp.screenshot({ path: `${shots}/desk-${name}-${theme}.png` })
      }
    }
    await wide.close()
    profileTheme = 'light'
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
  console.log('Desks passed: top bar, empty list, last desk, tab rename, drag reorder, tab colour, close and take off, stale save, newer schema, new tab, switch saves, menu move, touch targets, no desk read on other pages, splits, the pane limit, separators by pointer and keys, presets, zoom, the shared toolbar, focus keys, chords out of text fields, a failing pane, theatre, the live folder, swap and move, the phone, pane states, no reopen on open, Reopen, Reopen in home, a stale reopen, Reopen all, the limit in the browser, Choose a folder, the pane id on create, forgetting a pane’s history, History by pane, Continue, the start-up command, Duplicate, templates, the limits in Reopen all, the account limit, a double press, a rename before Reopen, Type in all panes (asks once, typing, paste, Compose, Run stays put, zoom, tab change, a drop, a takeover, another window, reload).')
} catch (error) {
  console.error('Browser errors:', errors)
  if (shots) { await mkdir(shots, { recursive: true }); await page.screenshot({ path: `${shots}/desks-failure.png`, fullPage: true }) }
  throw error
} finally { await browser.close() }
