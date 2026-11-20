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

// What each session's command history answers (by session id), when the machine reads it; how many earlier-output
// snapshots were read.
let histories = new Map()
let historyReads = 0
// The fixture's state: reset() before each part.
let profileTheme = 'light'
let desks, sessions, patches, patched, ends, creates, staleNext, hidden, sockets, failing, ticketDelay, attachingNow, attachingMost
// Reopens: what each sent, the answers some panes get instead, how many desks moved on first, and how many were in flight.
let reopens, reopenFail, staleReopens, reopenDelay, reopenNow, reopenMost, conflicted
// Session renames, the account's folder projects, creates answered with a refusal (in order), desks whose writes fail and
// whether a desk read fails.
let renames, folderProjects, createFail, patchFail, deskReadFail
// Whether the folder projects read and a session rename fail, and how many times the projects were read.
let projectsFail, renameFail, projectReads
// The person's End setting (Account › Profile); desk deletes as sent, and the sessions a delete cannot end; ends that
// fail (by session); a refusal for the next desk create and for each desk's document saves; and what the controller
// does to a saved document (it keeps a folder a machine reported, say) before it answers.
let profileRemove, deletes, deleteLeft, endFail, createDeskFail, docRefuse, adjustDoc
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
  posts = []; bracketed = new Set(); histories = new Map(); historyReads = 0; delete machines[0].capabilities.commandHistory; delete machines[0].capabilities.resume
  renames = []; folderProjects = []; createFail = []; patchFail = new Set(); deskReadFail = false
  projectsFail = false; renameFail = false; projectReads = 0
  profileRemove = false; deletes = []; deleteLeft = new Set(); endFail = new Set(); createDeskFail = null; docRefuse = new Map(); adjustDoc = null
  machines.splice(1)
  for (const k of ['launch', 'tools', 'agentsV2']) delete machines[0].capabilities[k]
}
let posts = []
// The sessions whose shell turns bracketed paste on.
let bracketed = new Set()
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
  else if (path === '/api/v2/profile') data = { profile: { theme: profileTheme, shelfView: 'list', cardsExpanded: false, terminalRemoveOnClose: profileRemove } }
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
  else if (path === '/api/v2/folder-projects') {
    projectReads++
    if (projectsFail) return route.fulfill({ status: 500, json: { error: "reet couldn't do that just now. Try again." } })
    data = { projects: folderProjects, savedWaiting: 0 }
  }
  else if (path.startsWith('/api/v2/folder-projects/') && method === 'GET') {
    const found = folderProjects.find(p => p.id === decodeURIComponent(path.split('/').at(-1)))
    if (!found) return route.fulfill({ status: 404, json: { error: 'not found', code: 'project_not_found' } })
    data = found
  }
  else if (path === '/api/v2/project-groups') data = { groups: [] }
  else if (/^\/api\/v2\/terminal\/sessions\/[^/]+$/.test(path) && method === 'PATCH') {
    const id = decodeURIComponent(path.split('/').at(-1)), body = request.postDataJSON()
    renames.push({ id, ...body })
    if (renameFail) return route.fulfill({ status: 503, json: { error: 'terminal machine is reconnecting; retry after it reconnects' } })
    sessions = sessions.map(x => x.id === id ? { ...x, title: body.title } : x)
    data = sessions.find(x => x.id === id) ?? {}
  }
  else if (path === '/api/v2/terminal/sessions') {
    if (method === 'GET') data = { sessions }
    else {
      const body = request.postDataJSON()
      creates.push(body)
      if (createFail.length) return route.fulfill(createFail.shift())
      const created = { ...running(`sess-new-${creates.length}`, body.title), dir: body.dir || '/home/dev', ...(body.tool ? { tool: body.tool } : {}) }
      sessions = [created, ...sessions]
      return route.fulfill({ status: 201, json: created })
    }
  }
  else if (/^\/api\/v2\/terminal\/sessions\/[^/]+\/end$/.test(path)) {
    const id = path.split('/').at(-2)
    ends.push({ id, ...request.postDataJSON() })
    if (endFail.has(id)) return route.fulfill({ status: 503, json: { error: 'terminal machine is reconnecting; retry after it reconnects' } })
    // An end of a session that has already ended changes nothing in Sessions (the machine forgets the pane's history).
    sessions = sessions.map(s => s.id === id && !['ended', 'exited', 'lost'].includes(s.state) ? { ...s, state: 'ended' } : s)
  }
  else if (path.endsWith('/history')) {
    historyReads++
    return route.fulfill({ body: Array.from({ length: 400 }, (_, i) => `earlier output ${i}`).join('\n') })
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
  else if (path === '/api/v2/skills/new' && method === 'GET') data = { newOnMachines: [] }
  else if (path === '/api/v2/desks' && method === 'POST') {
    const body = request.postDataJSON()
    posts.push(body)
    if (createDeskFail) { const refusal = createDeskFail; createDeskFail = null; return route.fulfill(refusal) }
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
    if (method === 'GET') {
      if (deskReadFail) return route.fulfill({ status: 500, json: { error: 'reet couldn\'t do that just now. Try again.' } })
      data = d
    } else if (method === 'PATCH') {
      const body = request.postDataJSON()
      patches.push(body); patched.push(id)
      if (patchFail.has(id)) return route.fulfill({ status: 500, json: { error: "reet couldn't do that just now. Try again." } })
      // As the controller: a session is in one pane across the account's desks.
      const elsewhere = sid => [...desks.values()].some(o => o.id !== id && o.doc.tabs.some(t => Object.values(t.panes).some(p => p.session === sid)))
      if (body.doc?.tabs.some(t => Object.values(t.panes).some(p => p.session && elsewhere(p.session)))) return route.fulfill({ status: 409, json: { error: 'A session can be in only one pane.', code: 'desk_session_taken' } })
      if (staleNext) { const newer = staleNext; staleNext = null; desks.set(id, newer); return route.fulfill({ status: 409, json: { error: 'desk_stale', code: 'desk_stale', desk: newer } }) }
      if (body.expect !== d.version) return route.fulfill({ status: 409, json: { error: 'desk_stale', code: 'desk_stale', desk: d } })
      // As the controller: names are unique on an account, ignoring case; a document that breaks a rule is refused.
      if (body.name !== undefined && [...desks.values()].some(o => o.id !== id && o.name.toLowerCase() === body.name.toLowerCase())) return route.fulfill({ status: 409, json: { error: 'Another desk already has that name.', code: 'desk_name_taken', field: 'name' } })
      if (body.doc && docRefuse.has(id)) return route.fulfill(docRefuse.get(id))
      const doc = body.doc && adjustDoc ? adjustDoc(structuredClone(body.doc)) : body.doc
      const next = { ...d, ...(body.name !== undefined ? { name: body.name } : {}), ...(body.colour !== undefined ? { colour: body.colour } : {}), ...(doc ? { doc } : {}), version: d.version + 1 }
      desks.set(id, next)
      data = next
    } else if (method === 'DELETE') {
      // As the controller: the version is checked, the desk goes first, then (when asked) its running shells are ended;
      // any that will not end are listed.
      const body = request.postDataJSON()
      deletes.push({ id, ...body })
      if (body.expect !== d.version) return route.fulfill({ status: 409, json: { error: 'desk_stale', code: 'desk_stale', desk: d } })
      desks.delete(id)
      const left = []
      if (body.endSessions) {
        for (const sid of d.doc.tabs.flatMap(t => Object.values(t.panes).map(p => p.session)).filter(Boolean)) {
          const s = sessions.find(x => x.id === sid)
          if (!s || !['creating', 'running'].includes(s.state)) continue
          if (deleteLeft.has(sid)) { left.push(sid); continue }
          sessions = sessions.filter(x => x.id !== sid)
        }
      }
      data = { ok: true, left }
    }
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
      // A shell that turned bracketed paste on takes a pasted block as one.
      if (bracketed.has(id)) socket.send(Buffer.from('\x1b[?2004h'))
    } else if (data.type === 'resize') record?.resizes.push([data.cols, data.rows])
  })
}
await page.route('**/api/**', api)
await page.routeWebSocket('**/api/v2/terminal/socket', socketStub)

const nav = page.getByRole('navigation', { name: 'Main navigation' })
// The tab list claims its tabs with aria-owns; they sit in the bar beside it, with each tab's ⋮ button.
const tablist = page.locator('.bar .tabs')
const tab = name => tablist.getByRole('tab', { name, exact: true })
const tabNames = () => tablist.getByRole('tab').evaluateAll(els => els.map(el => el.querySelector('.name')?.textContent))
const quiet = () => page.waitForTimeout(700)
const newWindow = () => page.evaluate(() => { try { sessionStorage.clear() } catch { /* no page yet */ } })
async function openDesk() {
  // A window keeps its own tab for a desk; this opens the desk as a new window would.
  await newWindow()
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
  await newWindow()
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
  // After Factory and before Machines (the owner's order: Home, Sessions, Factory, Desks, Machines).
  expect(links[at - 1].trim()).toBe('Factory')
  expect(links[at + 1].trim()).toBe('Machines')
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

  // ── the tab shown is this window's own ──
  reset()
  await openDesk()
  await quiet(); patches = []
  await tab('logs').click()
  await expect(tab('logs')).toHaveAttribute('aria-selected', 'true')
  await quiet()
  // Switching tab saves nothing, so no other window is moved.
  expect(patches).toEqual([])
  expect(desks.get('desk-prod').doc.active).toBe('tab-deploy')
  // The window keeps its tab across a reload; a new window opens on the tab the desk was last worked in.
  await page.reload()
  await expect(tab('logs')).toHaveAttribute('aria-selected', 'true')
  // Working in the tab (its one pane was already its focused pane) records it as the tab the desk was last worked in.
  await pane('pane-logs').locator('.xterm').click()
  await expect.poll(() => desks.get('desk-prod').doc.active).toBe('tab-logs')
  // A new window now opens on it.
  await newWindow()
  await page.reload()
  await expect(tab('logs')).toHaveAttribute('aria-selected', 'true')
  await quiet()
  reset()
  await openDesk()
  // Work in another tab in a second window does not move this one, which opened on deploy.
  {
    const other = await context.newPage()
    other.on('pageerror', error => errors.push(error.message))
    await other.route('**/api/**', api)
    await other.routeWebSocket('**/api/v2/terminal/socket', socketStub)
    await other.goto(`${base}/desks/desk-prod`)
    await other.locator('.bar .tabs').getByRole('tab', { name: 'logs', exact: true }).click()
    await other.locator('.desk-pane[data-pane="pane-logs"] .xterm').click()
    await expect.poll(() => desks.get('desk-prod').doc.active).toBe('tab-logs')
    await expect.poll(() => page.evaluate(() => sessionStorage.getItem('reet-desk-tab:desk-prod'))).toBe('tab-deploy')
    await page.reload()
    await expect(tab('deploy')).toHaveAttribute('aria-selected', 'true')
    await other.close()
    await quiet()
    reset()
    await openDesk()
  }
  // The tab list holds the tabs and nothing else: each tab's ⋮ button sits outside it.
  const owned = await page.getByRole('tablist', { name: 'Tabs' }).getAttribute('aria-owns')
  expect(owned.split(' ')).toEqual(['desk-tab-tab-deploy', 'desk-tab-tab-logs', 'desk-tab-tab-db'])
  expect(await page.getByRole('tablist', { name: 'Tabs' }).locator('button').count()).toBe(0)
  const tree = await page.locator('.bar').ariaSnapshot()
  expect(tree).toMatch(/- tablist "Tabs":\n\s+- tab "deploy" \[selected\]\n\s+- tab "logs"\n\s+- tab "db"\n/)
  // Nothing floats over a terminal: the Gaffer button is not on an open desk.
  await expect(page.getByRole('button', { name: 'Ask Gaffer' })).toHaveCount(0)
  console.log("ok - the tab shown is the window's own, the tab list owns only tabs")

  // ── Layout on a tab of one pane combines tabs: all of them, or the ones chosen ──
  reset()
  await openDesk()
  await quiet(); patches = []
  await page.getByRole('button', { name: 'Layout' }).click()
  const layMenu = page.getByRole('menu', { name: 'Layout' })
  await expect(layMenu.getByRole('menuitem')).toHaveText(['Show all tabs side by side', 'Show all tabs stacked', 'Show all tabs as a grid', 'Combine tabs…'])
  await layMenu.getByRole('menuitem', { name: 'Combine tabs…' }).click()
  const combine = page.getByRole('dialog', { name: 'Combine tabs into “deploy”' })
  await expect(combine.getByRole('checkbox', { name: 'All tabs' })).toBeChecked()
  await expect(combine.getByRole('checkbox', { name: /^logs/ })).toBeChecked()
  await expect(combine.getByText('3 of 9 panes in this tab.')).toBeVisible()
  await combine.getByRole('checkbox', { name: /^db/ }).uncheck()
  if (shots) await page.screenshot({ path: `${shots}/desk-combine-tabs.png` })
  await combine.getByRole('radio', { name: 'Stacked' }).check()
  await combine.getByRole('button', { name: 'Combine 1 tab' }).click()
  await expect(combine).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Layout' })).toBeFocused()
  expect(await tabNames()).toEqual(['deploy', 'db'])
  await expect(page.locator('.desk-pane')).toHaveCount(2)
  await quiet()
  expect(lastDoc().tabs.map(t => t.id)).toEqual(['tab-deploy', 'tab-db'])
  expect(lastDoc().tabs[0].root).toEqual({ split: 'down', ratio: 0.5, a: { pane: 'pane-deploy' }, b: { pane: 'pane-logs' } })
  await attached(['sess-deploy', 'sess-logs'])
  // With two panes the menu is the layouts again, and still offers the tab that is left.
  await page.getByRole('button', { name: 'Layout' }).click()
  await expect(layMenu.getByRole('menuitem')).toHaveText(['Side by side', 'Stacked', 'Grid', 'Main and stack', 'Even out', 'Combine tabs…'])
  await page.keyboard.press('Escape')
  // All at once from the menu.
  reset()
  await openDesk()
  await quiet(); patches = []
  await page.getByRole('button', { name: 'Layout' }).click()
  await layMenu.getByRole('menuitem', { name: 'Show all tabs side by side' }).click()
  expect(await tabNames()).toEqual(['deploy'])
  await expect(page.locator('.desk-pane')).toHaveCount(3)
  await quiet()
  expect(Object.keys(lastDoc().tabs[0].panes).sort()).toEqual(['pane-db', 'pane-deploy', 'pane-logs'])
  console.log('ok - Layout combines tabs, all or chosen')

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
  // The person keeps ended sessions in Sessions (their Profile setting, as the Sessions End dialog follows it).
  await expect(dialog.getByText("Stops what is running in it. The session stays in Sessions as ended, and this pane's command history is removed.")).toBeVisible()
  await expect(dialog.getByText('It keeps running, and stays in Sessions.')).toBeVisible()
  if (shots) await page.screenshot({ path: `${shots}/desks-close-light.png` })
  await page.keyboard.press('Enter')
  await expect(dialog).toHaveCount(0)
  await expect.poll(() => ends.length).toBe(1)
  expect(ends[0]).toMatchObject({ id: 'sess-deploy', removeHistory: false, forgetDesk: true })
  await expect.poll(tabNames).toEqual(['logs', 'db'])
  await quiet()
  expect(lastDoc().tabs.map(t => t.id)).toEqual(['tab-logs', 'tab-db'])
  // With the setting that removes an ended session, the dialog says the session leaves Sessions, and so it does.
  reset()
  profileRemove = true
  await openDesk()
  await page.getByRole('button', { name: 'Actions for reet' }).click()
  await page.getByRole('menuitem', { name: 'Close' }).click()
  await expect(page.getByRole('dialog', { name: 'Close “reet”?' }).getByText("Stops what is running in it. The session leaves Sessions, and this pane's command history is removed.")).toBeVisible()
  await page.keyboard.press('Enter')
  await expect.poll(() => ends.length).toBe(1)
  expect(ends[0]).toMatchObject({ id: 'sess-deploy', removeHistory: true, forgetDesk: true })
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
  await page.getByRole('menu', { name: 'New tab' }).getByRole('menuitem', { name: 'New shell…' }).click()
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
    const ttabs = tp.locator('.bar .tabs')
    await expect(ttabs.getByRole('tab', { name: 'deploy', exact: true })).toBeVisible()
    const heights = async locator => (await locator.boundingBox()).height
    for (const [what, locator] of [
      ['tab', ttabs.getByRole('tab', { name: 'deploy', exact: true })],
      ['tab menu', tp.getByRole('button', { name: 'Actions for tab deploy' })],
      ['new tab', tp.getByRole('button', { name: 'New tab' })],
      ['desk name', tp.getByRole('button', { name: 'prod deploy, switch desk' })],
      ['desk menu', tp.getByRole('button', { name: 'Desk menu' })],
    ]) expect(await heights(locator), what).toBeGreaterThanOrEqual(44)
    // A short tab name still makes a tab at least 44 px wide.
    expect((await ttabs.getByRole('tab', { name: 'db', exact: true }).boundingBox()).width, 'short tab').toBeGreaterThanOrEqual(44)
    await tp.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
    await tp.goto(`${base}/desks`)
    const title = tp.getByRole('link', { name: 'prod deploy' })
    await expect(title).toBeVisible()
    expect(await heights(title), 'card title').toBeGreaterThanOrEqual(44)
    await touch.close()
  }
  console.log('ok - touch targets are 44 px on a touch screen')

  // ── an on-screen keyboard that overlays the page ──
  {
    reset()
    const touch = await browser.newContext({ viewport: { width: 834, height: 1000 }, hasTouch: true, isMobile: true })
    const tp = await touch.newPage()
    tp.on('pageerror', error => errors.push(error.message))
    // The keyboard is stood in for by a visual viewport the test can shrink: the window keeps its height.
    await tp.addInitScript(() => {
      const fake = new EventTarget()
      Object.assign(fake, { height: innerHeight, width: innerWidth, offsetTop: 0, offsetLeft: 0, pageTop: 0, pageLeft: 0, scale: 1 })
      Object.defineProperty(window, 'visualViewport', { value: fake, configurable: true })
      window.__keyboard = height => { fake.height = height; fake.dispatchEvent(new Event('resize')) }
    })
    await tp.route('**/api/**', api)
    await tp.routeWebSocket('**/api/v2/terminal/socket', socketStub)
    await tp.goto(`${base}/desks/desk-prod`)
    await expect(tp.locator('.bar .tabs').getByRole('tab', { name: 'deploy', exact: true })).toBeVisible()
    const bottom = () => tp.evaluate(() => Math.round(document.querySelector('.desk-screen').getBoundingClientRect().bottom))
    expect(await bottom(), 'the desk fills the window').toBe(1000)
    await tp.evaluate(() => window.__keyboard(560))
    await expect.poll(bottom, 'the desk ends above the keyboard').toBeLessThanOrEqual(560)
    expect(await tp.evaluate(() => document.querySelector('.area').getBoundingClientRect().height), 'the panes keep some room').toBeGreaterThan(120)
    // The focused pane has the keyboard, so only the tab bar stays above it.
    const areaHeight = () => tp.evaluate(() => Math.round(document.querySelector('.area').getBoundingClientRect().height))
    await expect(tp.locator('.site-header')).toBeHidden()
    await expect(tp.locator('.dhead')).toBeHidden()
    await expect(tp.locator('.desk-toolbar')).toHaveCount(0)
    await expect(tp.locator('.bar .tabs').getByRole('tab', { name: 'deploy', exact: true })).toBeVisible()
    expect(await areaHeight(), 'the panes get most of the room above the keyboard').toBeGreaterThan(440)
    expect(await bottom(), 'still above the keyboard').toBeLessThanOrEqual(560)
    if (shots) await tp.screenshot({ path: `${shots}/desks-keyboard-834.png` })
    await tp.evaluate(() => window.__keyboard(innerHeight))
    await expect.poll(bottom, 'the desk fills the window again').toBe(1000)
    await expect(tp.locator('.site-header')).toBeVisible()
    await expect(tp.locator('.dhead')).toBeVisible()
    // A keyboard opened by a field outside the panes moves nothing aside: the desk's own name stays where it is typed.
    await tp.getByRole('button', { name: 'prod deploy, switch desk' }).focus()
    await tp.evaluate(() => window.__keyboard(560))
    await expect.poll(bottom).toBeLessThanOrEqual(560)
    await expect(tp.locator('.dhead')).toBeVisible()
    await expect(tp.locator('.site-header')).toBeVisible()
    await touch.close()
  }
  console.log('ok - the desk is fitted above an on-screen keyboard')

  // ── loading another page reads only the desk list, never a desk ──
  // Home (and the bell on every other page) reads the list to keep desk sessions off Home and to mark one that waits;
  // with nothing waiting on a desk, no desk itself is read.
  {
    reset()
    const reads = []
    const note = request => { if (new URL(request.url()).pathname.startsWith('/api/v2/desks')) reads.push(request.url()) }
    page.on('request', note)
    await page.goto(`${base}/`)
    await expect(nav.getByRole('link', { name: 'Home' })).toHaveAttribute('aria-current', 'page')
    await page.waitForLoadState('networkidle')
    expect(reads.length).toBeGreaterThan(0)
    expect(reads.map(url => new URL(url).pathname).filter(path => path !== '/api/v2/desks')).toEqual([])
    reads.length = 0
    // Jump reads them once it is opened, to offer each desk.
    await page.getByRole('combobox', { name: 'Jump to a session, project, skill or machine' }).fill('prod')
    await expect(page.getByRole('listbox', { name: 'Jump results' }).getByRole('option', { name: /prod deploy/ })).toHaveAttribute('href', '/desks/desk-prod')
    expect(reads.length).toBeGreaterThan(0)
    page.off('request', note)
  }
  console.log('ok - loading another page reads only the desk list, never a desk')

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

  // ── the toolbar stays where it is whichever pane is focused ──
  // A long pane name does not push the tools along or bring a scroll bar: the name has a column of its own, and the
  // labels follow the toolbar's width.
  reset()
  desks.get('desk-split').doc.tabs[0].panes['pane-b'].name = 'a pane with a long name of forty characters'
  for (const width of [1360, 1600, 1745, 1920]) {
    await page.setViewportSize({ width, height: 860 })
    await openSplit()
    await attached(['sess-a', 'sess-b', 'sess-c'])
    const tools = page.locator('.desk-toolbar .tools')
    const place = () => page.evaluate(() => {
      const box = sel => { const r = document.querySelector(sel).getBoundingClientRect(); return [Math.round(r.left), Math.round(r.width)] }
      const t = document.querySelector('.desk-toolbar .tools')
      return { who: box('.desk-toolbar .who'), first: box('.desk-toolbar .tools button'), tools: box('.desk-toolbar .tools'), fits: t.scrollWidth <= t.clientWidth, height: Math.round(t.getBoundingClientRect().height) }
    })
    await pane('pane-a').locator('.xterm').click()
    await expect(page.getByRole('toolbar', { name: 'Terminal tools for alpha' })).toBeVisible()
    const short = await place()
    await pane('pane-b').locator('.xterm').click()
    await expect(page.getByRole('toolbar', { name: 'Terminal tools for a pane with a long name of forty characters' })).toBeVisible()
    const long = await place()
    expect(long, `toolbar at ${width}`).toEqual(short)
    expect(long.fits, `no scroll bar at ${width}`).toBe(true)
    expect(await tools.evaluate(el => getComputedStyle(el).overflowX)).toBe('auto')
  }
  await page.setViewportSize({ width: 1360, height: 860 })
  console.log('ok - the toolbar does not move with the pane name')

  // ── Earlier output toggles on the focused pane, and only on it ──
  reset()
  await openSplit()
  await attached(['sess-a', 'sess-b', 'sess-c'])
  await pane('pane-a').locator('.xterm').click()
  await expect(page.getByRole('toolbar', { name: 'Terminal tools for alpha' })).toBeVisible()
  {
    const earlier = page.getByRole('toolbar', { name: 'Terminal tools for alpha' }).getByRole('button', { name: 'Earlier output' })
    const snapshot = pane('pane-a').getByRole('region', { name: 'Earlier terminal output', exact: true })
    const before = ['a', 'b', 'c'].map(x => sockets.get(`sess-${x}`).input.length)
    await expect(earlier).toHaveAttribute('aria-pressed', 'false')
    await earlier.click()
    await expect(earlier).toHaveAttribute('aria-pressed', 'true')
    await expect(pane('pane-a').getByRole('region', { name: 'Scrollable terminal output' })).toContainText('earlier output 399')
    // The pane beside it keeps its live screen: the toggle is the focused pane's own.
    await expect(pane('pane-b').getByRole('region', { name: 'Earlier terminal output' })).toHaveCount(0)
    await earlier.click()
    await expect(earlier).toHaveAttribute('aria-pressed', 'false')
    await expect(snapshot).toHaveCount(0)
    await expect(pane('pane-a').locator('.xterm')).toBeVisible()
    // A third press opens it again cleanly: one read per opening, and one copy of the buffer.
    const reads = historyReads
    await earlier.click()
    await expect(earlier).toHaveAttribute('aria-pressed', 'true')
    await expect(snapshot).toContainText('earlier output 0')
    await expect(snapshot.locator('pre')).toHaveCount(1)
    expect(historyReads).toBe(reads + 1)
    await earlier.click()
    await expect(snapshot).toHaveCount(0)
    // No pane's shell was typed into while the snapshot was open or closed.
    expect(['a', 'b', 'c'].map(x => sockets.get(`sess-${x}`).input.length)).toEqual(before)
  }
  console.log('ok - Earlier output toggles on the focused pane only')

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
    await page.getByRole('menuitem', { name: 'New shell…' }).click()
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
  expect(ends[0]).toMatchObject({ id: 'sess-deploy', removeHistory: false, forgetDesk: true })
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
    // A template is not a desk card: it is under Templates, and offered under From a template.
    await expect(page.locator('.card:not(.tpl)').getByRole('link', { name: 'prod deploy template' })).toHaveCount(0)
    await expect(page.getByRole('region', { name: 'Templates' }).getByRole('link', { name: 'prod deploy template' })).toBeVisible()
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

  // ── a multi-line paste is not broadcast when one pane lacks bracketed paste ──
  reset()
  bracketed.add('sess-a'); bracketed.add('sess-b')
  await openThree()
  await typeInAll()
  await pane('pane-a').locator('textarea').evaluate(el => {
    const data = new DataTransfer(); data.setData('text/plain', 'one\ntwo\nthree')
    el.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }))
  })
  // alpha takes the block as one (its own rule); gamma would run it line by line, so neither other pane gets it.
  await expect.poll(() => sent('sess-a')).toBe('\x1b[200~one\rtwo\rthree\x1b[201~')
  await expect(page.getByRole('alert')).toContainText('Stopped typing in all panes: gamma would run this paste line by line.')
  await expect(stripe()).toHaveCount(0)
  await quiet()
  expect(sent('sess-b')).toBe('')
  expect(sent('sess-c')).toBe('')
  console.log('ok - a multi-line paste is not broadcast when one pane lacks bracketed paste')

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

  // ── a start-up command with a line break is not run ──
  {
    resetStates()
    desks.get('desk-states').doc.tabs[0].panes['pane-lost'].start = 'echo safe\nrm -rf ~/work'
    await openStates()
    await pane('pane-lost').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
    await expect(pane('pane-lost')).toHaveAttribute('data-state', 'live')
    const bar = pane('pane-lost').getByRole('group', { name: 'Start-up command' })
    const hiddenSentence = "This command has hidden characters, so reet won't run it. Edit it first."
    await expect(bar.getByRole('alert')).toHaveText(hiddenSentence)
    // The whole command is shown, wrapped, so what would run is what is on screen.
    await expect(bar.locator('code')).toContainText('rm -rf ~/work')
    expect(await bar.locator('code').evaluate(el => getComputedStyle(el).whiteSpace)).toBe('pre-wrap')
    await attached(['sess-re-1'])
    await expect(bar.getByRole('button', { name: 'Run' })).toBeDisabled()
    await bar.getByRole('button', { name: 'Run' }).click({ force: true })
    await quiet()
    expect(sockets.get('sess-re-1').input.join('')).not.toContain('echo safe')
    // The dialog refuses the same characters with the same sentence, and saves nothing.
    patches = []
    await pane('pane-lost').getByRole('button', { name: 'Actions for lost one' }).click()
    await page.getByRole('menuitem', { name: 'Start-up command…' }).click()
    const dialog = page.getByRole('dialog', { name: 'Start-up command' })
    const field = dialog.getByRole('textbox', { name: 'Command to offer when this pane is reopened' })
    for (const bad of ['ls ‮-la', 'ls​ -la', 'ls\u0007', 'ls ⁦x']) {
      await field.fill(bad)
      await dialog.getByRole('button', { name: 'Save' }).click()
      await expect(dialog.getByRole('alert')).toHaveText(hiddenSentence)
    }
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await quiet()
    expect(patches.filter(p => p.doc?.tabs[0].panes['pane-lost'].start !== 'echo safe\nrm -rf ~/work')).toEqual([])
    console.log('ok - a start-up command with a line break is not run')
  }

  // ── a template opened by its address starts nothing ──
  {
    reset()
    const tp = (id, name) => paneOf(id, name, '', '/home/dev/PROD_reet')
    desks.set('desk-tpl', deskOf('desk-tpl', 'deploy template', [
      { id: 'tab-t1', name: 'deploy', colour: '', root: { split: 'right', ratio: 0.5, a: leaf('pane-t1'), b: leaf('pane-t2') }, zoomed: '', active: 'pane-t1', panes: { 'pane-t1': tp('pane-t1', 'web'), 'pane-t2': tp('pane-t2', 'api') } },
    ], { template: true, position: 4 }))
    await page.goto(`${base}/desks/desk-tpl`)
    await expect(page.getByRole('heading', { name: 'deploy template', level: 1 })).toBeVisible()
    await expect(page.getByText('This is a template. Make a desk from it to start shells.')).toBeVisible()
    await expect(pane('pane-t1')).toHaveAttribute('data-state', 'empty')
    for (const name of ['Start', 'New tab', 'Split right', 'Split down', 'Type in all panes']) await expect(page.getByRole('button', { name, exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: /^Reopen all/ })).toHaveCount(0)
    await pane('pane-t1').getByRole('button', { name: 'Actions for web' }).click()
    await expect(page.getByRole('menu', { name: 'Actions for web' }).getByRole('menuitem', { name: 'Rename' })).toBeVisible()
    await expect(page.getByRole('menuitem', { name: 'Split right' })).toHaveCount(0)
    await page.keyboard.press('Escape')
    await chord('D')
    await chord('T')
    await quiet()
    expect(creates).toEqual([])
    expect(reopens).toEqual([])
    await expect(page.locator('dialog[open]')).toHaveCount(0)
    await page.getByRole('button', { name: 'New desk from this template' }).click()
    await expect(page.getByRole('heading', { name: 'deploy', level: 1 })).toBeVisible()
    expect(posts.at(-1)).toMatchObject({ name: 'deploy', from: 'desk-tpl' })
    expect(posts.at(-1).template).toBeUndefined()
    console.log('ok - a template opened by its address starts nothing')
  }

  // ── Continue keeps the project and one request id, and its limit card's Try again continues ──
  {
    resetStates()
    machines[0].capabilities.resume = true
    folderProjects = [{ id: 'fp-app', host: agent, machine: 'studio', dir: '/home/dev/app', name: 'app', isRepo: false, baseBranch: '', worktreeDefault: 'inherit', defaultTool: '', defaultModel: '', hasStarterPrompt: false, note: '', position: 0, createdAt: 1, updatedAt: 1, version: 1 }]
    sessions = sessions.map(x => x.id === 'sess-agent' ? { ...x, projectId: 'fp-app' } : x)
    // The first create gets no answer it can trust; the second is refused at the machine's limit.
    createFail = [{ status: 503, json: { error: 'terminal machine is reconnecting; retry after it reconnects' } }, { status: 409, json: { error: 'terminal session limit reached' } }]
    await openStates()
    await pane('pane-agent').getByRole('button', { name: 'Continue' }).click()
    await expect(pane('pane-agent').getByRole('alert')).toHaveText('terminal machine is reconnecting; retry after it reconnects')
    await pane('pane-agent').getByRole('button', { name: 'Continue' }).click()
    await expectCard('pane-agent', 'limit', 'At the limit', 'studio already has 24 sessions running.', 'Try again')
    await pane('pane-agent').getByRole('button', { name: 'Try again' }).click()
    await expect(pane('pane-agent')).toHaveAttribute('data-state', 'live')
    expect(creates).toHaveLength(3)
    for (const c of creates) expect(c).toMatchObject({ agentId: agent, tool: 'claude', resume: true, dir: '/home/dev/app', projectId: 'fp-app' })
    expect(new Set(creates.map(c => c.requestId)).size).toBe(1)
    expect(reopens).toEqual([])
    await expect.poll(() => patches.at(-1)?.doc.tabs[0].panes['pane-agent'].session).toBe('sess-new-3')
    console.log('ok - Continue keeps the project and one request id, and its limit card’s Try again continues')
  }

  // ── a pane already live is not counted as reopened ──
  {
    resetStates()
    reopenFail.set('pane-ended', { status: 409, json: { error: 'That pane already has a running shell.', code: 'pane_live' } })
    await openStates()
    await page.getByRole('button', { name: 'Reopen all (4)' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Reopened' })).toContainText('Reopened 1. 2 could not be reopened: old build (folder gone), locked (folder unavailable).', { timeout: 20_000 })
    console.log('ok - a pane already live is not counted as reopened')
  }

  // ── the pane stays busy while the desk cannot be read after a reopen ──
  {
    resetStates()
    await openStates()
    await quiet()
    deskReadFail = true
    await pane('pane-lost').getByRole('button', { name: 'Reopen in ~/PROD_reet' }).click()
    await expect.poll(() => reopens.length).toBe(1)
    const waitingButton = pane('pane-lost').getByRole('button', { name: 'Reopening…' })
    await expect(waitingButton).toBeDisabled()
    await page.waitForTimeout(2000)
    await expect(waitingButton).toBeDisabled()
    await expect(page.getByRole('button', { name: /^Reopen all/ })).toHaveText('Reopen all (3)')
    expect(reopens.length).toBe(1)
    deskReadFail = false
    await expect(pane('pane-lost')).toHaveAttribute('data-state', 'live', { timeout: 15_000 })
    expect(reopens.length).toBe(1)
    console.log('ok - the pane stays busy while the desk cannot be read after a reopen')
  }

  // ── Choose a folder on a machine that cannot open a terminal says so ──
  {
    resetStates()
    machines.push({ agentId: 'connector-oldbox0000000001', host: 'old-box', ready: true, serverEnabled: true, capabilities: { osUser: 'dev', terminalEnabled: false, readiness: 'ready' } })
    desks.get('desk-states').doc.tabs[0].panes['pane-folder'].host = 'connector-oldbox0000000001'
    await openStates()
    await pane('pane-folder').getByRole('button', { name: 'Reopen in ~/old' }).click()
    await pane('pane-folder').getByRole('button', { name: 'Choose a folder' }).click()
    const dialog = page.getByRole('dialog', { name: 'Choose a folder' })
    await expect(dialog.getByRole('alert')).toHaveText('This pane’s machine can’t open a terminal just now.')
    await expect(dialog.getByRole('combobox', { name: 'Machine' })).toHaveCount(0)
    await expect(dialog.getByRole('button', { name: 'Reopen' })).toBeDisabled()
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await expect(dialog).toHaveCount(0)
    expect(reopens.length).toBe(1)
    console.log('ok - Choose a folder on a machine that cannot open a terminal says so')
  }

  // ── a stored failure goes once the pane holds another session ──
  {
    resetStates()
    await openStates()
    await pane('pane-folder').getByRole('button', { name: 'Reopen in ~/old' }).click()
    await expect(pane('pane-folder')).toHaveAttribute('data-state', 'folderGone')
    // Another window put a session in the pane, which has since ended.
    sessions = [{ ...running('sess-other', 'old build', '/home/dev/old'), state: 'exited', exitCode: 0 }, ...sessions]
    const d = desks.get('desk-states'), t = d.doc.tabs[0]
    desks.set(d.id, { ...d, version: d.version + 1, doc: { ...d.doc, tabs: [{ ...t, panes: { ...t.panes, 'pane-folder': { ...t.panes['pane-folder'], session: 'sess-other' } } }] } })
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
    await expect(pane('pane-folder')).toHaveAttribute('data-state', 'ended')
    console.log('ok - a stored failure goes once the pane holds another session')
  }

  // ── Continue does not drop the project when the projects cannot be read ──
  {
    resetStates()
    machines[0].capabilities.resume = true
    const app = { id: 'fp-app', host: agent, machine: 'studio', dir: '/home/dev/app', name: 'app', isRepo: false, baseBranch: '', worktreeDefault: 'inherit', defaultTool: '', defaultModel: '', hasStarterPrompt: false, note: '', position: 0, createdAt: 1, updatedAt: 1, version: 1 }
    sessions = sessions.map(x => x.id === 'sess-agent' ? { ...x, projectId: 'fp-app' } : x)
    projectsFail = true
    await openStates()
    await pane('pane-agent').getByRole('button', { name: 'Continue' }).click()
    await expectCard('pane-agent', 'projectsUnread', 'Not continued', "reet couldn't read your projects just now, so this conversation was not continued.", 'Try again')
    await quiet()
    expect(creates).toEqual([])
    // Try again reads the projects afresh and continues, with the project.
    projectsFail = false
    folderProjects = [app]
    const readsBefore = projectReads
    await pane('pane-agent').getByRole('button', { name: 'Try again' }).click()
    await expect(pane('pane-agent')).toHaveAttribute('data-state', 'live')
    expect(projectReads).toBe(readsBefore + 1)
    expect(creates).toHaveLength(1)
    expect(creates[0]).toMatchObject({ tool: 'claude', resume: true, dir: '/home/dev/app', projectId: 'fp-app' })
    expect(reopens).toEqual([])
    console.log('ok - Continue does not drop the project when the projects cannot be read')
  }

  // ── a limit refusal says which limit ──
  {
    resetStates()
    machines[0].capabilities.resume = true
    // The account is nearly full on another machine; studio has room. The server refuses at the account's limit.
    sessions = [...sessions, ...Array.from({ length: 48 }, (_, i) => ({ ...running(`sess-elsewhere-${i}`, `other ${i}`), agentId: 'connector-other000000001' }))]
    createFail = [{ status: 409, json: { error: 'terminal session limit reached' } }]
    await openStates()
    await pane('pane-agent').getByRole('button', { name: 'Continue' }).click()
    await expectCard('pane-agent', 'limit', 'At the limit', 'You already have 64 sessions running.', 'Try again')
    // The browser's own count was under both limits: the refusal came from the server.
    expect(creates).toHaveLength(1)
    console.log('ok - a limit refusal says which limit')
  }

  // ── a session rename that fails puts the pane's name back ──
  {
    reset()
    await openDesk()
    await quiet(); patches = []
    renameFail = true
    await pane('pane-deploy').getByRole('button', { name: 'Actions for reet' }).click()
    await page.getByRole('menuitem', { name: 'Rename' }).click()
    await page.getByRole('textbox', { name: 'Pane name' }).fill('web box')
    await page.keyboard.press('Enter')
    await expect(page.getByRole('alert').filter({ hasText: 'The pane was renamed' })).toContainText('The pane was renamed, but its session could not be. Try again.')
    await expect(pane('pane-deploy').locator('.nm')).toHaveText('deploy')
    await expect.poll(() => desks.get('desk-prod').doc.tabs[0].panes['pane-deploy'].name).toBe('deploy')
    expect(renames).toEqual([{ id: 'sess-deploy', title: 'web box' }])
    console.log('ok - a session rename that fails puts the pane’s name back')
  }

  // ── renaming a pane renames the session; hidden characters never reach a name ──
  {
    reset()
    await openDesk()
    await quiet(); patches = []
    await pane('pane-deploy').getByRole('button', { name: 'Actions for reet' }).click()
    await page.getByRole('menuitem', { name: 'Rename' }).click()
    const field = page.getByRole('textbox', { name: 'Pane name' })
    await field.fill('web​ box‮')
    await expect(field).toHaveValue('web box')
    await field.press('Enter')
    await expect.poll(() => renames).toEqual([{ id: 'sess-deploy', title: 'web box' }])
    await expect.poll(() => lastDoc()?.tabs[0].panes['pane-deploy'].name).toBe('web box')
    // A tab's name too.
    await tab('db').focus()
    await page.keyboard.press('F2')
    const tabField = tablist.getByRole('textbox', { name: 'Tab name' })
    await tabField.fill('data⁦base\u0007')
    await expect(tabField).toHaveValue('database')
    await tabField.press('Enter')
    await expect.poll(() => lastDoc()?.tabs.find(t => t.id === 'tab-db')?.name).toBe('database')
    // A pane with no session keeps the name on the desk only.
    await openSplit('tab-nine')
    await quiet(); renames = []
    await pane('pane-n1').getByRole('button', { name: 'Actions for n1' }).click()
    await page.getByRole('menuitem', { name: 'Rename' }).click()
    await page.getByRole('textbox', { name: 'Pane name' }).fill('spare')
    await page.keyboard.press('Enter')
    await expect.poll(() => lastDoc()?.tabs.find(t => t.id === 'tab-nine')?.panes['pane-n1'].name).toBe('spare')
    await quiet()
    expect(renames).toEqual([])
    console.log('ok - renaming a pane renames the session')
  }

  // ── Add to a desk puts a running session in a new pane ──
  {
    reset()
    sessions = [running('sess-free', 'free'), ...sessions]
    // Sessions lists a desk session only when it needs you, so the two moved from here are waiting.
    sessions = sessions.map(s => s.id === 'sess-db' || s.id === 'sess-deploy' ? { ...s, attention: 'waiting', attentionSource: 'terminal', changedSinceSeen: true, bellAt: now } : s)
    await page.goto(`${base}/sessions?view=list`)
    const row = title => page.locator('.session-row').filter({ has: page.locator('.title', { hasText: new RegExp(`^${title}$`) }) })
    await row('free').getByRole('button', { name: 'Actions for free' }).click()
    await page.getByRole('menuitem', { name: 'Add to a desk…' }).click()
    const dialog = page.getByRole('dialog', { name: 'Add to a desk' })
    await expect(dialog.getByRole('combobox', { name: 'Desk' })).toHaveValue('desk-prod')
    await expect(dialog.getByRole('radio', { name: 'deploy' })).toBeChecked()
    await dialog.getByRole('radio', { name: 'logs' }).check()
    patches = []; patched = []
    if (shots) {
      for (const theme of ['light', 'dark']) {
        await page.setViewportSize({ width: 1440, height: 900 })
        await page.emulateMedia({ colorScheme: theme })
        await page.evaluate(t => document.documentElement.setAttribute('data-theme', t), theme)
        await page.waitForTimeout(300)
        await page.screenshot({ path: `${shots}/add-to-desk-${theme}.png` })
      }
      await page.emulateMedia({ colorScheme: 'light' })
      await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'light'))
      await page.setViewportSize({ width: 1360, height: 860 })
    }
    await dialog.getByRole('button', { name: 'Add', exact: true }).click()
    await expect(dialog).toHaveCount(0)
    await expect(page.getByText('Sorted. “free” is on “prod deploy”.')).toBeVisible()
    expect(patched).toEqual(['desk-prod'])
    const logs = desks.get('desk-prod').doc.tabs.find(t => t.id === 'tab-logs')
    const added = Object.values(logs.panes).find(p => p.session === 'sess-free')
    expect(added).toMatchObject({ host: agent, kind: 'shell', name: 'free', dir: '/home/dev/PROD_reet', start: '' })
    expect(logs.root).toEqual({ split: 'right', ratio: 0.5, a: { pane: 'pane-logs' }, b: { pane: added.id } })
    expect(logs.active).toBe(added.id)
    // It is on a desk and does not need anyone, so Sessions no longer lists it.
    await expect(row('free')).toHaveCount(0)

    // One already on a desk is moved: taken off its old pane first, then added.
    patched = []
    await row('db').getByRole('button', { name: 'Actions for db' }).click()
    await page.getByRole('menuitem', { name: 'Add to a desk…' }).click()
    await expect(dialog).toContainText('It is on “prod deploy” now.')
    await expect(dialog.getByRole('combobox', { name: 'Desk' })).toHaveValue('desk-dev')
    await expect(dialog.getByRole('radio', { name: 'A new tab' })).toBeChecked()
    await dialog.getByRole('button', { name: 'Move here' }).click()
    await expect(page.getByText('Sorted. “db” moved to “zoomies dev”.')).toBeVisible()
    expect(patched).toEqual(['desk-prod', 'desk-dev'])
    expect(desks.get('desk-prod').doc.tabs.find(t => t.id === 'tab-db').panes['pane-db'].session).toBe('')
    expect(Object.values(desks.get('desk-dev').doc.tabs[0].panes)[0]).toMatchObject({ session: 'sess-db', name: 'db' })
    await expect(row('db').getByRole('link', { name: 'On the desk zoomies dev' })).toBeVisible()

    // When the add fails after the old pane let go, it says so; nothing is guessed.
    patched = []
    patchFail.add('desk-split')
    await row('deploy').getByRole('button', { name: 'Actions for deploy' }).click()
    await page.getByRole('menuitem', { name: 'Add to a desk…' }).click()
    await dialog.getByRole('combobox', { name: 'Desk' }).selectOption('desk-split')
    await expect(dialog.getByRole('radio', { name: 'deploy' })).toBeChecked()
    await expect(dialog.getByRole('radio', { name: /^nine/ })).toBeDisabled()
    await dialog.getByRole('button', { name: 'Move here' }).click()
    await expect(dialog.getByRole('alert')).toContainText('It was taken off “prod deploy” but could not be added to “split desk”, so it is on neither desk.')
    expect(patched).toEqual(['desk-prod', 'desk-split'])
    expect(desks.get('desk-prod').doc.tabs.find(t => t.id === 'tab-deploy').panes['pane-deploy'].session).toBe('')
    // The page reads the desks again, so the row no longer names the desk it was taken off; the dialog now offers Add.
    await expect(row('deploy').getByRole('link', { name: /^On the desk/ })).toHaveCount(0)
    await expect(dialog.getByRole('button', { name: 'Add', exact: true })).toBeVisible()
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    console.log('ok - Add to a desk puts a running session in a new pane')
  }

  // ── a session on a desk that needs you shows the desk's name in Sessions ──
  {
    reset()
    sessions = [running('sess-free', 'free'), ...sessions]
    // Only the desk session that needs you is listed here; the quiet ones belong to the desk alone.
    sessions = sessions.map(s => s.id === 'sess-logs' ? { ...s, attention: 'waiting', attentionSource: 'terminal', changedSinceSeen: true, bellAt: now } : s)
    // A desk whose only session is not listed here is not read.
    desks.set('desk-ghost', deskOf('desk-ghost', 'ghost', [tabOf('tab-g', 'g', '', paneOf('pane-g', 'g', 'sess-ghost', '/home/dev'))], { position: 5 }))
    const reads = []
    const note = request => { const p = new URL(request.url()).pathname; if (p.startsWith('/api/v2/desks')) reads.push(p) }
    page.on('request', note)
    await page.goto(`${base}/sessions?view=list`)
    const row = title => page.locator('.session-row').filter({ has: page.locator('.title', { hasText: new RegExp(`^${title}$`) }) })
    const chip = row('logs').getByRole('link', { name: 'On the desk prod deploy' })
    await expect(chip).toHaveText('prod deploy')
    await expect(chip).toHaveAttribute('href', '/desks/desk-prod#pane-logs')
    // The quiet desk sessions are not on this page.
    await expect(row('a')).toHaveCount(0)
    await expect(row('b')).toHaveCount(0)
    await expect(row('free').getByRole('link')).toHaveCount(1)
    // The desk pane's colour (else its tab's) colours the row when the browser has no choice of its own.
    await expect(row('logs')).toHaveClass(/coloured/)
    await expect(row('logs').locator('.colour-name')).toHaveText('Blue')
    await page.waitForLoadState('networkidle')
    // The list for the page's sessions, again for the pane and colour of what it lists, and once for the bell, which
    // marks the waiting desk session with its desk. Only that desk is read, and none of the empty desk, the quiet
    // desk's or the ghost desk's.
    expect(reads.filter(p => p === '/api/v2/desks')).toHaveLength(3)
    expect([...new Set(reads.filter(p => p !== '/api/v2/desks'))]).toEqual(['/api/v2/desks/desk-prod'])
    page.off('request', note)
    if (shots) {
      for (const theme of ['light', 'dark']) {
        await page.setViewportSize({ width: 1440, height: 900 })
        await page.emulateMedia({ colorScheme: theme })
        await page.evaluate(t => document.documentElement.setAttribute('data-theme', t), theme)
        await page.waitForTimeout(300)
        await page.screenshot({ path: `${shots}/sessions-desk-chip-${theme}.png` })
      }
      await page.emulateMedia({ colorScheme: 'light' })
      await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'light'))
      await page.setViewportSize({ width: 1360, height: 860 })
    }
    // The chip opens the desk at that pane.
    await chip.click()
    await expect(page.getByRole('heading', { name: 'prod deploy', level: 1 })).toBeVisible()
    await expect(tab('logs')).toHaveAttribute('aria-selected', 'true')
    await expect(pane('pane-logs')).toHaveAttribute('aria-current', 'true')
    await expect(page).toHaveURL(`${base}/desks/desk-prod`)
    console.log("ok - a session on a desk shows the desk's name in Sessions")
  }

  // ── Add a running session… puts one in a new tab, and moves one from another desk ──
  {
    reset()
    sessions = [running('sess-free', 'free'), ...sessions]
    await openDesk()
    await quiet()
    await page.getByRole('button', { name: 'New tab' }).click()
    await expect(page.getByRole('menu', { name: 'New tab' }).getByRole('menuitem')).toHaveText(['New shell…', 'Start an agent…', 'Add a running session…'])
    await page.getByRole('menuitem', { name: 'Add a running session…' }).click()
    const dialog = page.getByRole('dialog', { name: 'Add a running session' })
    // Those already on this desk are not offered.
    await expect(dialog.getByRole('radio')).toHaveCount(8)
    await expect(dialog.getByRole('radio', { name: /^deploy/ })).toHaveCount(0)
    await dialog.getByRole('radio', { name: /^free studio/ }).check()
    patched = []
    await dialog.getByRole('button', { name: 'Add', exact: true }).click()
    await expect(dialog).toHaveCount(0)
    await expect(tab('free')).toHaveAttribute('aria-selected', 'true')
    expect(patched).toEqual(['desk-prod'])
    await attached(['sess-free'])
    await page.getByRole('button', { name: 'New tab' }).click()
    await page.getByRole('menuitem', { name: 'Add a running session…' }).click()
    await dialog.getByRole('radio', { name: /^a studio · on “split desk”/ }).check()
    await expect(dialog).toContainText('It is on “split desk” now.')
    patched = []
    await dialog.getByRole('button', { name: 'Move here' }).click()
    await expect(dialog).toHaveCount(0)
    await expect(tab('a')).toHaveAttribute('aria-selected', 'true')
    expect(patched).toEqual(['desk-split', 'desk-prod'])
    // The pane it left is closed and its neighbour takes the space: a move leaves no hole.
    expect(desks.get('desk-split').doc.tabs[0].panes['pane-a']).toBeUndefined()
    expect(desks.get('desk-split').doc.tabs[0].root).toEqual({ split: 'down', ratio: 0.5, a: leaf('pane-b'), b: leaf('pane-c') })
    expect(creates).toEqual([])
    console.log('ok - Add a running session puts one in a new tab, and moves one from another desk')
  }

  // ── Start an agent from a desk fills the new pane ──
  {
    reset()
    Object.assign(machines[0].capabilities, { launch: true, tools: ['claude'], agentsV2: true })
    folderProjects = [{ id: 'fp-reet', host: agent, machine: 'studio', dir: '/home/dev/PROD_reet', name: 'reet', isRepo: false, baseBranch: '', worktreeDefault: 'inherit', defaultTool: '', defaultModel: '', hasStarterPrompt: false, note: '', position: 0, createdAt: 1, updatedAt: 1, version: 1 }]
    await openDesk()
    await quiet()
    await page.getByRole('button', { name: 'New tab' }).click()
    await page.getByRole('menuitem', { name: 'Start an agent…' }).click()
    const dialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
    await expect(dialog.getByLabel('Project')).toHaveValue('fp-reet')
    await dialog.getByRole('button', { name: 'Start', exact: true }).click()
    await expect(dialog).toHaveCount(0)
    expect(creates).toHaveLength(1)
    expect(creates[0]).toMatchObject({ agentId: agent, tool: 'claude', projectId: 'fp-reet', dir: '/home/dev/PROD_reet' })
    expect(creates[0].deskPane).toBeUndefined()
    await expect(page.locator('.desk-pane')).toHaveAttribute('data-state', 'live')
    await expect.poll(() => lastDoc()?.tabs.length).toBe(4)
    const added = lastDoc().tabs.at(-1)
    expect(Object.values(added.panes)[0]).toMatchObject({ session: 'sess-new-1', kind: 'agent', host: agent })
    expect(lastDoc().active).toBe(added.id)
    console.log('ok - Start an agent from a desk fills the new pane')
  }

  // ── Templates are listed under their own heading, with Open, Rename and Delete ──
  {
    reset()
    const tp = (id, name) => paneOf(id, name, '', '/home/dev/PROD_reet')
    desks.set('desk-tpl', deskOf('desk-tpl', 'deploy template', [
      { id: 'tab-t1', name: 'deploy', colour: '', root: { split: 'right', ratio: 0.5, a: leaf('pane-t1'), b: leaf('pane-t2') }, zoomed: '', active: 'pane-t1', panes: { 'pane-t1': tp('pane-t1', 'web'), 'pane-t2': tp('pane-t2', 'api') } },
    ], { template: true, colour: 'blue', position: 4 }))
    await page.goto(`${base}/desks`)
    await page.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
    await page.goto(`${base}/desks`)
    const group = page.getByRole('region', { name: 'Templates' })
    await expect(group.getByRole('heading', { name: 'Templates', level: 2 })).toBeVisible()
    await expect(group.getByRole('link')).toHaveText(['deploy template'])
    await expect(group.locator('.card .meta')).toContainText('1 tab · 2 panes')
    // Not among the desks.
    await expect(page.locator('.card:not(.tpl)').getByRole('link', { name: 'deploy template' })).toHaveCount(0)
    await group.getByRole('button', { name: 'Actions for deploy template' }).click()
    const menu = page.getByRole('menu', { name: 'Actions for deploy template' })
    await expect(menu.getByRole('menuitem')).toHaveText(['Open', 'New desk from this template', 'Rename', 'Delete'])
    // Rename.
    await menu.getByRole('menuitem', { name: 'Rename' }).click()
    const rename = page.getByRole('dialog', { name: 'Rename “deploy template”' })
    await rename.getByLabel('Name').fill('web template')
    await rename.getByRole('button', { name: 'Rename' }).click()
    await expect(group.getByRole('link')).toHaveText(['web template'])
    expect(patches.at(-1)).toEqual({ expect: 1, name: 'web template' })
    // Open: the template's own page, which starts nothing.
    await group.getByRole('button', { name: 'Actions for web template' }).click()
    await page.getByRole('menuitem', { name: 'Open' }).click()
    await expect(page).toHaveURL(/\/desks\/desk-tpl$/)
    await expect(page.getByText('This is a template. Make a desk from it to start shells.')).toBeVisible()
    await page.getByRole('button', { name: 'web template, switch desk' }).click()
    await page.getByRole('menuitem', { name: 'All desks' }).click()
    // New desk from this template.
    await group.getByRole('button', { name: 'Actions for web template' }).click()
    await page.getByRole('menuitem', { name: 'New desk from this template' }).click()
    await expect(page.getByRole('heading', { name: 'web', level: 1 })).toBeVisible()
    expect(posts.at(-1)).toMatchObject({ name: 'web', colour: 'blue', from: 'desk-tpl' })
    // Delete: nothing runs in a template.
    await page.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
    await page.goto(`${base}/desks`)
    await group.getByRole('button', { name: 'Actions for web template' }).click()
    await page.getByRole('menuitem', { name: 'Delete' }).click()
    const del = page.getByRole('dialog', { name: 'Delete “web template”?' })
    await expect(del).toContainText('Its tabs and panes are removed. Nothing is running in it.')
    await del.getByRole('button', { name: 'Delete' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Sorted.' })).toHaveText('Sorted. “web template” is deleted.')
    await expect(group).toHaveCount(0)
    expect(deletes).toEqual([{ id: 'desk-tpl', expect: 2, endSessions: false }])
    console.log('ok - templates are listed under their own heading, with Open, Rename and Delete')
  }

  // ── a pane whose shell already ended closes at once, and its machine forgets the pane's history ──
  {
    reset()
    sessions = sessions.map(x => x.id === 'sess-deploy' ? { ...x, state: 'exited', exitCode: 0 } : x.id === 'sess-c' ? { ...x, state: 'lost' } : x)
    await openDesk()
    await expect(pane('pane-deploy')).toHaveAttribute('data-state', 'ended')
    await page.getByRole('button', { name: 'Actions for reet' }).click()
    await page.getByRole('menuitem', { name: 'Close' }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect.poll(tabNames).toEqual(['logs', 'db'])
    await expect.poll(() => ends.length).toBe(1)
    expect(ends[0]).toMatchObject({ id: 'sess-deploy', removeHistory: false, forgetDesk: true })
    // Nothing leaves Sessions.
    expect(sessions.find(x => x.id === 'sess-deploy')).toMatchObject({ state: 'exited' })
    // A tab with a live pane and a lost one: End the shell ends the live one as the setting says and forgets the lost one.
    await openSplit()
    await attached(['sess-a', 'sess-b'])
    ends = []
    await page.getByRole('button', { name: 'Actions for tab deploy' }).click()
    await page.getByRole('menuitem', { name: 'Close tab' }).click()
    const both = page.getByRole('dialog', { name: 'Close “deploy”?' })
    await expect(both).toContainText('2 of its shells are still running on studio.')
    await expect(both).toContainText("Stops what is running in them. The sessions stay in Sessions as ended, and these panes' command history is removed.")
    await both.getByRole('radio', { name: /Take off this desk/ }).click()
    await both.getByRole('button', { name: 'Take off this desk' }).click()
    // Take off leaves the running shells as they are; the lost one's pane is gone for good all the same.
    await expect.poll(() => ends.length).toBe(1)
    expect(ends[0]).toMatchObject({ id: 'sess-c', removeHistory: false, forgetDesk: true })
    await quiet()
    expect(ends.length).toBe(1)
    // An end that fails still closes the pane, and says nothing.
    reset()
    sessions = sessions.map(x => x.id === 'sess-logs' ? { ...x, state: 'lost' } : x)
    endFail.add('sess-logs')
    await openDesk()
    await page.getByRole('button', { name: 'Actions for tab logs' }).click()
    await page.getByRole('menuitem', { name: 'Close tab' }).click()
    await expect.poll(tabNames).toEqual(['deploy', 'db'])
    await expect.poll(() => ends.length).toBe(1)
    await quiet()
    await expect(page.getByRole('alert')).toHaveCount(0)
    console.log("ok - a pane whose shell already ended closes at once, and its machine forgets the pane's history")
  }

  // ── deleting a desk says what happens to its shells, and which could not be ended ──
  {
    reset()
    await page.goto(`${base}/desks`)
    await page.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
    await page.goto(`${base}/desks`)
    await expect(page.getByRole('link', { name: 'prod deploy' })).toBeVisible()
    // Another window moved the desk on since the list was read; logs' machine will not end it.
    { const d = desks.get('desk-prod'); desks.set(d.id, { ...d, version: 2 }) }
    deleteLeft.add('sess-logs')
    await page.getByRole('button', { name: 'Actions for prod deploy' }).click()
    await page.getByRole('menuitem', { name: 'Delete' }).click()
    const del = page.getByRole('dialog', { name: 'Delete “prod deploy”?' })
    await expect(del).toContainText('3 of its shells are still running. End them, or leave them running in Sessions.')
    await expect(del).toContainText("End its shells: Its shells are ended and leave Sessions, and their panes' command history is removed.")
    await del.getByRole('button', { name: 'End its shells' }).click()
    await expect(page.getByRole('alert')).toHaveText('Sorted. “prod deploy” is deleted. One of its shells could not be ended. It is still in Sessions.')
    expect(deletes).toEqual([{ id: 'desk-prod', expect: 1, endSessions: true }, { id: 'desk-prod', expect: 2, endSessions: true }])
    expect(sessions.map(x => x.id)).not.toContain('sess-deploy')
    expect(sessions.find(x => x.id === 'sess-logs')).toMatchObject({ state: 'running' })
    // The controller forgets the ended panes' history itself when it ends the shells: the browser sends no end.
    await quiet()
    expect(ends).toEqual([])
    // Leave them running: nothing running is ended; a pane whose shell had ended has its history forgotten.
    reset()
    sessions = sessions.map(x => x.id === 'sess-db' ? { ...x, state: 'exited', exitCode: 0 } : x)
    await page.goto(`${base}/desks`)
    await page.getByRole('button', { name: 'Actions for prod deploy' }).click()
    await page.getByRole('menuitem', { name: 'Delete' }).click()
    await expect(del).toContainText('2 of its shells are still running.')
    await del.getByRole('button', { name: 'Leave them running in Sessions' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Sorted.' })).toHaveText('Sorted. “prod deploy” is deleted.')
    expect(deletes).toEqual([{ id: 'desk-prod', expect: 1, endSessions: false }])
    await expect.poll(() => ends.length).toBe(1)
    expect(ends[0]).toMatchObject({ id: 'sess-db', removeHistory: false, forgetDesk: true })
    expect(sessions.find(x => x.id === 'sess-deploy')).toMatchObject({ state: 'running' })
    console.log('ok - deleting a desk says what happens to its shells, and which could not be ended')
  }

  // ── a session removed from history elsewhere leaves its pane Not started ──
  {
    reset()
    sessions = sessions.map(x => x.id === 'sess-deploy' ? { ...x, state: 'exited', exitCode: 0 } : x)
    await openDesk()
    await expect(pane('pane-deploy')).toHaveAttribute('data-state', 'ended')
    // Removing the session from history (or revoking its machine) clears it from its pane, and the desk moves on.
    sessions = sessions.filter(x => x.id !== 'sess-deploy')
    const d = desks.get('desk-prod')
    desks.set(d.id, { ...d, version: d.version + 1, doc: { ...d.doc, tabs: d.doc.tabs.map(t => t.id === 'tab-deploy' ? { ...t, panes: { 'pane-deploy': { ...t.panes['pane-deploy'], session: '' } } } : t) } })
    // The desk list, read every 15 seconds, brings it in.
    await expect(pane('pane-deploy')).toHaveAttribute('data-state', 'empty', { timeout: 20_000 })
    await expectCard('pane-deploy', 'empty', 'Not started', 'studio · ~/PROD_reet', 'Start')
    expect(reopens).toEqual([])
    console.log('ok - a session removed from history elsewhere leaves its pane Not started')
  }

  // ── a desk card counts only shells that ended or were lost as to reopen ──
  {
    reset()
    sessions = sessions.map(x => x.id === 'sess-logs' ? { ...x, state: 'lost' } : x)
    await page.goto(`${base}/desks`)
    await page.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
    await page.goto(`${base}/desks`)
    const card = name => page.locator('.card').filter({ has: page.getByRole('link', { name, exact: true }) })
    await expect(card('prod deploy').locator('.pill')).toHaveText(['2 live', '1 to reopen'])
    // The split desk's nine-pane tab was never started: nothing there is to reopen.
    await expect(card('split desk').locator('.pill')).toHaveText(['7 live'])
    console.log('ok - a desk card counts only shells that ended or were lost as to reopen')
  }

  // ── a change made in another browser appears without a reload or a save ──
  {
    reset()
    await openDesk()
    await quiet(); patches = []
    await page.evaluate(() => { window.__sameDocument = true })
    const d = desks.get('desk-prod')
    desks.set(d.id, { ...d, version: d.version + 1, doc: { ...d.doc, tabs: d.doc.tabs.map(t => t.id === 'tab-logs' ? { ...t, name: 'logs (moved)' } : t) } })
    // The desk list is read every 15 seconds; it lists the newer version, and the desk is read again.
    await expect(tab('logs (moved)')).toBeVisible({ timeout: 20_000 })
    expect(await page.evaluate(() => window.__sameDocument)).toBe(true)
    expect(patches).toEqual([])
    await expect(page.getByText('This desk was changed in another window.')).toHaveCount(0)
    console.log('ok - a change made in another browser appears without a reload or a save')
  }

  // ── a save the controller refuses is not retried, and says why ──
  {
    reset()
    await openDesk()
    await quiet(); patches = []
    docRefuse.set('desk-prod', { status: 400, json: { error: "That desk can't be saved: a pane names a session twice.", code: 'desk_invalid' } })
    await tab('db').dblclick()
    await tablist.getByRole('textbox', { name: 'Tab name' }).fill('database')
    await tablist.getByRole('textbox', { name: 'Tab name' }).press('Enter')
    await expect(page.getByRole('alert')).toContainText("reet couldn't save your last change: That desk can't be saved: a pane names a session twice. Showing the desk as it is saved.")
    await expect.poll(tabNames).toEqual(['deploy', 'logs', 'db'])
    await expect(page.locator('.save')).toHaveText('last change not saved')
    await page.waitForTimeout(2500)
    expect(patches.length).toBe(1)
    // The next change saves as usual.
    docRefuse.delete('desk-prod')
    await tab('logs').dblclick()
    await tablist.getByRole('textbox', { name: 'Tab name' }).fill('platform')
    await tablist.getByRole('textbox', { name: 'Tab name' }).press('Enter')
    await expect(page.locator('.save')).toHaveText('saved')
    expect(desks.get('desk-prod').doc.tabs.map(t => t.name)).toEqual(['deploy', 'platform', 'db'])
    console.log('ok - a save the controller refuses is not retried, and says why')
  }

  // ── the controller's other refusals are shown in its words ──
  {
    reset()
    await page.goto(`${base}/desks`)
    await page.evaluate(() => localStorage.removeItem('reet.desks.last.1'))
    await page.goto(`${base}/desks`)
    createDeskFail = { status: 409, json: { error: 'An account holds at most 50 desks. Remove one first.', code: 'desk_full' } }
    await page.locator('.head').getByRole('button', { name: 'New desk' }).click()
    await expect(page.getByRole('alert')).toHaveText('An account holds at most 50 desks. Remove one first.')
    await expect(page).toHaveURL(/\/desks$/)
    await page.getByRole('button', { name: 'Actions for zoomies dev' }).click()
    await page.getByRole('menuitem', { name: 'Rename' }).click()
    const rename = page.getByRole('dialog', { name: 'Rename “zoomies dev”' })
    await rename.getByLabel('Name').fill('Prod Deploy')
    await rename.getByRole('button', { name: 'Rename' }).click()
    await expect(rename.getByRole('alert')).toHaveText('Another desk already has that name.')
    await expect(rename).toBeVisible()
    expect(desks.get('desk-dev').name).toBe('zoomies dev')
    console.log("ok - the controller's other refusals are shown in its words")
  }

  // ── a save the controller adjusted is merged, not overwritten ──
  {
    reset()
    // The controller keeps the folder deploy's machine reported, newer than the one the browser sent.
    adjustDoc = doc => {
      const p = doc.tabs.find(t => t.id === 'tab-deploy')?.panes['pane-deploy']
      if (p && p.dirAt < 9e12) { p.dir = '/home/dev/PROD_reet/web'; p.dirAt = 9e12 }
      return doc
    }
    await openDesk()
    await quiet(); patches = []
    await tab('logs').dblclick()
    await tablist.getByRole('textbox', { name: 'Tab name' }).fill('platform')
    await tablist.getByRole('textbox', { name: 'Tab name' }).press('Enter')
    await expect.poll(() => patches.length).toBe(1)
    await quiet()
    await tab('db').dblclick()
    await tablist.getByRole('textbox', { name: 'Tab name' }).fill('database')
    await tablist.getByRole('textbox', { name: 'Tab name' }).press('Enter')
    await expect.poll(() => patches.length).toBe(2)
    expect(patches[1].expect).toBe(2)
    expect(patches[1].doc.tabs[0].panes['pane-deploy']).toMatchObject({ dir: '/home/dev/PROD_reet/web', dirAt: 9e12 })
    expect(patches[1].doc.tabs.map(t => t.name)).toEqual(['deploy', 'platform', 'database'])
    await quiet()
    await expect(page.getByText('This desk was changed in another window.')).toHaveCount(0)
    console.log('ok - a save the controller adjusted is merged, not overwritten')
  }

  // ── a change made just before the page goes is still saved ──
  {
    reset()
    // A page of its own that notes each request sent with keepalive (one that may finish after the page has gone).
    const kp = await context.newPage()
    kp.on('pageerror', error => errors.push(error.message))
    await kp.addInitScript(() => {
      const send = window.fetch
      window.__keepalive = []
      window.fetch = (input, init) => { if (init?.keepalive) window.__keepalive.push(`${init.method} ${String(input)}`); return send(input, init) }
    })
    await kp.route('**/api/**', api)
    await kp.routeWebSocket('**/api/v2/terminal/socket', socketStub)
    await kp.goto(`${base}/desks/desk-prod`)
    await expect(kp.getByRole('heading', { name: 'prod deploy', level: 1 })).toBeVisible()
    await kp.waitForTimeout(700); patches = []
    await kp.locator('.bar .tabs').getByRole('tab', { name: 'db', exact: true }).focus()
    await kp.keyboard.press('F2')
    await kp.getByRole('textbox', { name: 'Tab name' }).fill('database')
    // Enter and the page going, within autosave's pause: the save is sent at once, with keepalive.
    await kp.evaluate(() => {
      document.querySelector('input[aria-label="Tab name"]').dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
      window.dispatchEvent(new PageTransitionEvent('pagehide'))
    })
    await expect.poll(() => kp.evaluate(() => window.__keepalive)).toEqual(['PATCH /api/v2/desks/desk-prod'])
    await expect.poll(() => patches.length).toBe(1)
    expect(desks.get('desk-prod').doc.tabs.find(t => t.id === 'tab-db').name).toBe('database')
    await kp.close()
    console.log('ok - a change made just before the page goes is still saved')
  }

  // ── New tab and Split: one start per press, the limits first, and plain sentences ──
  {
    reset()
    sessions = sessions.map(x => x.id === 'sess-deploy' ? { ...x, liveDir: '/home/dev/PROD_reet/web' } : x)
    await openDesk()
    await quiet(); patches = []
    await page.getByRole('button', { name: 'New tab' }).click()
    await page.getByRole('menuitem', { name: 'New shell…' }).click()
    const dialog = page.getByRole('dialog', { name: 'New tab' })
    // The folder the focused pane's shell is in now.
    await expect(dialog.getByLabel('Folder')).toHaveValue('/home/dev/PROD_reet/web')
    // The first create gets no answer it can trust: the second press asks about the same start.
    createFail.push({ status: 503, json: { error: 'terminal machine is reconnecting; retry after it reconnects' } })
    await dialog.getByRole('button', { name: 'Open tab' }).click()
    await expect(dialog.getByRole('alert')).toHaveText('studio is not reporting. Try again once it is back.')
    await dialog.getByRole('button', { name: 'Open tab' }).click()
    await expect(dialog).toHaveCount(0)
    expect(creates).toHaveLength(2)
    expect(creates[1].requestId).toBe(creates[0].requestId)
    expect(creates[1].deskPane).toBe(creates[0].deskPane)
    await quiet()
    expect(Object.keys(lastDoc().tabs.at(-1).panes)).toEqual([creates[0].deskPane])
    // The next New tab is a new start.
    await page.getByRole('button', { name: 'New tab' }).click()
    await page.getByRole('menuitem', { name: 'New shell…' }).click()
    await dialog.getByRole('button', { name: 'Open tab' }).click()
    await expect(dialog).toHaveCount(0)
    expect(creates[2].requestId).not.toBe(creates[0].requestId)
    // A New tab that failed and was then cancelled is over: the next one is a new start too.
    createFail.push({ status: 503, json: { error: 'terminal machine is reconnecting; retry after it reconnects' } })
    await page.getByRole('button', { name: 'New tab' }).click()
    await page.getByRole('menuitem', { name: 'New shell…' }).click()
    await dialog.getByRole('button', { name: 'Open tab' }).click()
    await expect(dialog.getByRole('alert')).toBeVisible()
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await page.getByRole('button', { name: 'New tab' }).click()
    await page.getByRole('menuitem', { name: 'New shell…' }).click()
    await dialog.getByRole('button', { name: 'Open tab' }).click()
    await expect(dialog).toHaveCount(0)
    expect(creates).toHaveLength(5)
    expect(creates[4].requestId).not.toBe(creates[3].requestId)
    // At the machine's limit nothing is sent, and the dialog says so as the pane card does.
    reset()
    sessions.push(...Array.from({ length: 14 }, (_, i) => running(`sess-fill-${i}`, `fill ${i}`)))
    await openDesk()
    await page.getByRole('button', { name: 'New tab' }).click()
    await page.getByRole('menuitem', { name: 'New shell…' }).click()
    await dialog.getByRole('button', { name: 'Open tab' }).click()
    await expect(dialog.getByRole('alert')).toHaveText('studio already has 24 sessions running.')
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await openSplit()
    await attached(['sess-a', 'sess-b', 'sess-c'])
    await pane('pane-a').locator('.xterm').click()
    await page.getByRole('toolbar', { name: 'Terminal tools for alpha' }).getByRole('button', { name: 'Split right' }).click()
    await expect(page.getByRole('alert')).toContainText('studio already has 24 sessions running.')
    expect(creates).toEqual([])
    // A refusal the browser could not see coming (another window filled the machine) is said the same way.
    sessions = sessions.filter(x => !x.id.startsWith('sess-fill-'))
    await page.reload()
    await attached(['sess-a', 'sess-b', 'sess-c'])
    createFail.push({ status: 409, json: { error: 'terminal session limit reached' } })
    await pane('pane-a').locator('.xterm').click()
    await page.getByRole('toolbar', { name: 'Terminal tools for alpha' }).getByRole('button', { name: 'Split right' }).click()
    await expect(page.getByRole('alert')).toContainText('studio already has 24 sessions running.')
    await expect(page.getByRole('alert')).not.toContainText('terminal session limit reached')
    console.log('ok - New tab and Split: one start per press, the limits first, and plain sentences')
  }

  // ── nothing of a desk carries over to the next one ──
  {
    reset()
    await page.goto(`${base}/desks/desk-dev`)
    await expect(page.getByRole('heading', { name: 'zoomies dev', level: 1 })).toBeVisible()
    await page.getByRole('button', { name: 'zoomies dev, switch desk' }).click()
    await page.getByRole('menuitem', { name: 'prod deploy' }).click()
    await expect(page.getByRole('heading', { name: 'prod deploy', level: 1 })).toBeVisible()
    await page.getByRole('button', { name: 'Actions for reet' }).click()
    await page.getByRole('menuitem', { name: 'Close' }).click()
    await expect(page.getByRole('dialog', { name: 'Close “reet”?' })).toBeVisible()
    // Back while the dialog is open: the dialog goes with its desk, and nothing is ended.
    await page.goBack()
    await expect(page.getByRole('heading', { name: 'zoomies dev', level: 1 })).toBeVisible()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await quiet()
    expect(ends).toEqual([])
    console.log('ok - nothing of a desk carries over to the next one')
  }

  // ── a desk that could not be read offers Try again, and comes in once it can be ──
  {
    reset()
    deskReadFail = true
    await page.goto(`${base}/desks/desk-prod`)
    await expect(page.getByRole('alert')).toHaveText('This desk could not be read just now. Try again in a moment.')
    deskReadFail = false
    await page.getByRole('button', { name: 'Try again' }).click()
    await expect(page.getByRole('heading', { name: 'prod deploy', level: 1 })).toBeVisible()
    // A later read that works (the page coming back into view) clears the failure too.
    deskReadFail = true
    await page.reload()
    await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible()
    deskReadFail = false
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
    await expect(page.getByRole('heading', { name: 'prod deploy', level: 1 })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Try again' })).toHaveCount(0)
    console.log('ok - a desk that could not be read offers Try again, and comes in once it can be')
  }

  // ── a change still waiting when the desk is left is retried until it is saved ──
  {
    reset()
    await openDesk()
    await quiet(); patched = []
    patchFail.add('desk-prod')
    await tab('db').focus()
    await page.keyboard.press('F2')
    await tablist.getByRole('textbox', { name: 'Tab name' }).fill('database')
    await tablist.getByRole('textbox', { name: 'Tab name' }).press('Enter')
    await page.getByRole('button', { name: 'prod deploy, switch desk' }).click()
    await page.getByRole('menu', { name: 'Switch desk' }).getByRole('menuitem', { name: 'zoomies dev' }).click()
    await expect(page.getByRole('heading', { name: 'zoomies dev', level: 1 })).toBeVisible()
    await expect.poll(() => patched.length).toBe(1)
    patchFail.delete('desk-prod')
    await expect.poll(() => desks.get('desk-prod').doc.tabs.find(t => t.id === 'tab-db').name, { timeout: 10_000 }).toBe('database')
    expect(patched).toEqual(['desk-prod', 'desk-prod'])
    console.log('ok - a change still waiting when the desk is left is retried until it is saved')
  }

  // ── small things: one instruction on a pane open elsewhere, Zoom focuses its pane, Type in all says why it is off ──
  {
    resetStates()
    await openStates()
    await expect(pane('pane-elsewhere')).toHaveAttribute('data-state', 'elsewhere')
    await expect(pane('pane-elsewhere')).toContainText('This terminal is open in another window.')
    await expect(pane('pane-elsewhere')).not.toContainText('Choose Continue here')
    reset()
    await openSplit()
    await attached(['sess-a', 'sess-b', 'sess-c'])
    await page.getByRole('button', { name: 'Actions for beta' }).click()
    await page.getByRole('menuitem', { name: 'Zoom' }).click()
    await expect(pane('pane-a')).toBeHidden()
    await expect(page.getByRole('toolbar', { name: 'Terminal tools for beta' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Type in all panes' })).toBeDisabled()
    await expect(page.getByRole('button', { name: 'Type in all panes' })).toHaveAccessibleDescription('Needs two or more live panes on this tab.')
    console.log('ok - one instruction on a pane open elsewhere, Zoom focuses its pane, Type in all says why it is off')
  }

  // ── the terminal's own answers to a program's queries reach only its own shell ──
  {
    reset()
    await openThree()
    await typeInAll()
    await expect(pane('pane-a').locator('textarea')).toBeFocused()
    // A program in the focused pane asks for the device attributes and the cursor position; xterm answers each itself.
    sockets.get('sess-a').socket.send(Buffer.from('\x1b[c'))
    sockets.get('sess-a').socket.send(Buffer.from('\x1b[6n'))
    await expect.poll(() => sent('sess-a')).toMatch(/\x1b\[\?[\d;]*c/)
    await expect.poll(() => sent('sess-a')).toMatch(/\x1b\[\d+;\d+R/)
    await quiet()
    expect(sent('sess-b')).toBe('')
    expect(sent('sess-c')).toBe('')
    // The mode is still on, and typing still reaches every pane.
    await expect(stripe()).toBeVisible()
    await page.keyboard.type('k')
    for (const x of ['b', 'c']) await expect.poll(() => sent(`sess-${x}`)).toBe('k')
    console.log("ok - the terminal's own answers to a program's queries reach only its own shell")
  }

  // ── a paste held for review before the mode was on stays in its own pane ──
  {
    reset()
    bracketed.add('sess-b'); bracketed.add('sess-c')
    await openThree()
    await typeInAll()
    await allButton().click()
    await expect(stripe()).toHaveCount(0)
    // alpha takes a paste line by line, so it asks first; beta and gamma would take the block as one.
    await pane('pane-a').locator('textarea').evaluate(el => {
      const data = new DataTransfer(); data.setData('text/plain', 'one\ntwo')
      el.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }))
    })
    const review = page.getByRole('dialog', { name: 'Paste into the remote shell?' })
    await expect(review).toBeVisible()
    // While it asks, the mode is turned on (asked once already in this page, so it turns on at once).
    await page.evaluate(() => document.querySelector('.btn.all').click())
    await expect(stripe()).toBeVisible()
    await review.getByRole('button', { name: 'Paste text' }).click()
    await expect.poll(() => sent('sess-a')).toBe('one\rtwo')
    await quiet()
    expect(sent('sess-b')).toBe('')
    expect(sent('sess-c')).toBe('')
    await expect(stripe()).toBeVisible()
    console.log('ok - a paste held for review before the mode was on stays in its own pane')
  }

  // ── input from a pane that is not the focused one is not sent on ──
  {
    reset()
    await openThree()
    await typeInAll()
    await expect(pane('pane-a').locator('textarea')).toBeFocused()
    // A paste lands in beta without beta taking the focus: it goes into beta's own shell only.
    await pane('pane-b').locator('textarea').evaluate(el => {
      const data = new DataTransfer(); data.setData('text/plain', 'echo beta')
      el.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }))
    })
    await expect.poll(() => sent('sess-b')).toBe('echo beta')
    await quiet()
    expect(sent('sess-a')).toBe('')
    expect(sent('sess-c')).toBe('')
    await expect(stripe()).toBeVisible()
    console.log('ok - input from a pane that is not the focused one is not sent on')
  }

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
        // A window keeps its own tab for a desk; each shot opens the desk as a new window would.
        await wp.evaluate(() => { try { sessionStorage.clear() } catch { /* no page yet */ } })
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
  console.log('Desks passed: top bar, empty list, last desk, tab rename, drag reorder, tab colour, close and take off, stale save, newer schema, new tab, switch saves, menu move, touch targets, no desk read on other pages, splits, the pane limit, separators by pointer and keys, presets, zoom, the shared toolbar, focus keys, chords out of text fields, a failing pane, theatre, the live folder, swap and move, the phone, pane states, no reopen on open, Reopen, Reopen in home, a stale reopen, Reopen all, the limit in the browser, Choose a folder, the pane id on create, forgetting a pane’s history, History by pane, Continue, the start-up command, Duplicate, templates, the limits in Reopen all, the account limit, a double press, a rename before Reopen, Type in all panes (asks once, typing, paste, a block one pane would run line by line, Compose, Run stays put, zoom, tab change, a drop, a takeover, another window, reload), a start-up command with hidden characters, a template by address, Continue (project, one request id, the limit), pane_live in Reopen all, busy while the desk cannot be read, a machine that cannot open a terminal, a failure cleared, a project read that failed, which limit, a failed session rename, renames, Add to a desk, the desk chip in Sessions, Add a running session, Start an agent, templates listed, an ended pane forgotten, a session cleared from its pane, delete and its shells, another browser’s change, a refused save, the controller’s refusals, a server-adjusted save, the save on pagehide, one start per press and the limits, a short tab’s width, the reopen count, nothing carried over, Try again, a retried save after leaving, one instruction elsewhere, Zoom focuses, the query answers, a held paste, the source pane.')
} catch (error) {
  console.error('Browser errors:', errors)
  if (shots) { await mkdir(shots, { recursive: true }); await page.screenshot({ path: `${shots}/desks-failure.png`, fullPage: true }) }
  throw error
} finally { await browser.close() }
