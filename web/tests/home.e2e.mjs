// Home on today's data: the fleet strip, In flight, Needs you and the project rail, each figure labelled with what it
// measures and drawn only from the existing loaders. Synthetic API answers; nothing is started on a machine.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, owner } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const MIN = 60_000, HOUR = 60 * MIN
const report = cpu => ({cpuPercent:{value:cpu},memory:{totalBytes:{value:1},availableBytes:{value:1}},disk:{totalBytes:{value:1},availableBytes:{value:1}},hostUptimeSeconds:{value:1},scope:'host'})
const caps = (os, tools=['claude','codex']) => ({protocolVersion:1,os,osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,tools})
// studio reports 38% CPU; desk is offline and its last report is stale, so it has no reading.
const health = () => ({serverNow:Date.now(),machines:[
 {agentId:'a1',host:'studio',state:'connected',capabilities:caps('linux'),connection:{connected:true,reportFresh:true,missedProbes:0,reconnects:0,path:'direct',report:report(38.2)}},
 {agentId:'a2',host:'desk',state:'unreachable',capabilities:caps('darwin'),connection:{connected:false,reportFresh:false,missedProbes:4,reconnects:2,path:'direct',report:report(91)}},
]})
const tmachines = [
 {agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:caps('linux')},
 {agentId:'a2',host:'desk',ready:false,serverEnabled:true,capabilities:caps('darwin')},
]
const session = (id, agentId, title, state='running', over={}) => ({id,agentId,title,state,createdAt:now-2*HOUR,updatedAt:now-12*MIN,exitCode:null,...over})
const baseSessions = () => [
 session('s1','a1','Refactor the store layer','running',{worktreeBranch:'reet/store-split'}),
 session('s2','a1','Tidy the release notes'),
 session('s3','a2','Speed up the runner pool'),
 session('s4','a1','Docs pass','exited',{exitCode:1,updatedAt:now-HOUR}),
]
const member = (sessionId, host='a1') => ({position:0,host,dir:'~/store',tool:'claude',model:'',state:'started',code:'',sentence:'',sessionId,worktreePath:'',worktreeRepo:'',worktreeBranch:'',worktreeRun:'',worktreeProject:'',sharesCheckout:true,attempt:1})
const projects = [
 {id:'p1',name:'Store rewrite',defaultTool:'claude',defaultModel:'',useWorktrees:true,hasSharedPrompt:false,members:[{host:'a1',dir:'~/store',tool:'',model:'',position:0}],createdAt:1,updatedAt:1,version:1,lastRun:{id:'r1',state:'running',startedAt:now-HOUR}},
 {id:'p2',name:'Notes',defaultTool:'claude',defaultModel:'',useWorktrees:false,hasSharedPrompt:false,members:[{host:'a2',dir:'~/notes',tool:'',model:'',position:0}],createdAt:1,updatedAt:1,version:1,lastRun:{id:'r2',state:'complete',startedAt:now-3*24*HOUR}},
]
const runs = {r1:{id:'r1',projectId:'p1',projectName:'Store rewrite',state:'running',startedAt:now-HOUR,useWorktrees:true,worktreeProject:'store',members:[member('s1')]},
 r2:{id:'r2',projectId:'p2',projectName:'Notes',state:'complete',startedAt:now-3*24*HOUR,useWorktrees:false,worktreeProject:'',members:[member('s3','a2')]}}
const job = (id, agentId, targetId, actions, ago) => ({id,agentId,targetId,kind:'preview',status:'awaiting-approval',revision:'r',hash:'h',plan:{actions:Array.from({length:actions},(_,i)=>({add:true,skill:{id:`k${i}`,name:`k${i}`}}))},message:'',createdAt:now-ago-MIN,updatedAt:now-ago})
const target = (id, host) => ({target:{id,host,agent:'claude'},sourceId:'s',sourceName:'src',desired:[],missing:[],extra:[],revision:'r',updatedAt:now})
const sync = {environments:[],machines:[],runs:[],updateJobs:[],
 agents:[{id:'a1',host:'studio',version:'1',lastSeen:now,paused:false,autoUpdate:false},{id:'a2',host:'desk',version:'1',lastSeen:now-2*HOUR,paused:false,autoUpdate:false}],
 targets:[target('t1','studio'),target('t2','desk')],
 jobs:[job('j1','a1','t1',0,5*MIN),job('j2','a2','t2',2,HOUR)]}

// A writer's Home with the fixtures above. `fail` names paths that answer 500 from now on; `sessions` is the live list.
async function home(options={}) {
 const reads = [], storage = []
 // `hold` delays the next sessions read, answering with the list as it was when the read arrived.
 const box = {sessions:baseSessions(), fail:new Set(), created:null, hold:null}
 const routes = {
  // As the server answers it: cookie only, so a single-token sign-in gets 401 and an open instance 403.
  '/api/v2/machines/health': (request, state) => {
   if (state.status.mode === 'token' && !request.headers()['x-reet-token']) return {status:401,json:{error:'missing or wrong token'}}
   if (state.status.mode === 'none') return {status:403,json:{error:'machine health requires an authenticated instance'}}
   return box.fail.has('health') ? {status:500,json:{error:'down'}} : health()
  },
  '/api/v2/terminal/machines': {machines:tmachines},
  '/api/v2/terminal/sessions': request => {
   if (request.method() === 'POST') {
    const body = request.postDataJSON()
    const s = session(`new-${body.requestId.slice(0,8)}`, body.agentId, body.title, 'running', {createdAt:Date.now(),updatedAt:Date.now()})
    box.sessions = [s, ...box.sessions]; box.created = s
    return s
   }
   if (box.fail.has('sessions')) return {status:500,json:{error:'down'}}
   const then = {sessions:box.sessions}
   if (box.hold) { const wait = box.hold; box.hold = null; return wait.then(() => then) }
   return then
  },
  '/api/v2/projects': () => box.fail.has('projects') ? {status:500,json:{error:'down'}} : {projects:box.projects ?? projects},
  '/api/v2/project-runs/r1': runs.r1,
  '/api/v2/project-runs/r2': runs.r2,
  ...(options.routes ?? {}),
 }
 const opened = await openWorkspace(browser, {width:1440, sync, routes, ...options, routes})
 opened.page.on('request', r => { const u = new URL(r.url()); if (u.pathname.startsWith('/api/')) reads.push(`${r.method()} ${u.pathname}`) })
 await opened.page.exposeFunction('__recordStorage', (op, key, value) => { storage.push({op, key, value}) })
 await opened.page.addInitScript(() => {
  for (const area of ['localStorage','sessionStorage']) {
   const store = window[area], set = store.setItem.bind(store)
   store.setItem = (k, v) => { window.__recordStorage(`${area}.set`, k, String(v)); return set(k, v) }
  }
 })
 return {...opened, reads, storage, box}
}
const strip = page => page.getByRole('region',{name:'Your machines',exact:true})
const card = (page, host) => strip(page).getByRole('link',{name:new RegExp(`^${host}, `)})
const inFlight = page => page.getByRole('region',{name:'In flight'})
const needs = page => page.getByRole('region',{name:'Needs you'})
const rail = page => page.getByRole('complementary',{name:'Projects'})
const groups = page => inFlight(page).locator('.group > h3').evaluateAll(els => els.map(e => e.textContent.replace(/\s+/g,' ').trim()))
const noSideways = page => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)

try {
 // Each card names the machine, its connection word, the live sessions on it (never "agents"), CPU labelled CPU and the
 // skills line from the last finished check, and opens that machine on Machines.
 if (run('fleet strip shows CPU, sessions running and the skills line with their labels')) {
  const {context,page,errors} = await home()
  await page.goto(base)
  expect(await page.title()).toBe('Home · reet.sh')
  const studio = card(page,'studio')
  await expect(studio).toBeVisible()
  await expect(studio).toHaveAttribute('href','/machines?machine=studio')
  await expect(studio).toHaveAccessibleName(/^studio, online, 2 sessions running, CPU 38%, skills in sync · checked 5 min ago$/)
  await expect(studio.locator('.figure')).toHaveText('2')
  await expect(studio).toContainText('sessions running')
  await expect(studio).toContainText('linux')
  await expect(studio.locator('.cpu')).toHaveText(/CPU\s*38%/)
  await expect(studio.locator('.skills')).toHaveText('skills in sync · checked 5 min ago')
  await expect(studio.locator('.skills')).toHaveClass(/good/)
  const desk = card(page,'desk')
  await expect(desk.locator('.skills')).toHaveText('2 skills out of date')
  await expect(desk.locator('.skills')).toHaveClass(/warn/)
  // An offline machine's count is what was last known: the card says so.
  await expect(desk).toHaveAccessibleName(/^desk, offline, 1 session running \(last known, not reporting\), CPU No reading, 2 skills out of date$/)
  await expect(desk.locator('.caption')).toHaveText('Last known count. desk is not reporting.')
  await expect(studio.locator('.caption')).toHaveCount(0)
  await expect(page.locator('main')).not.toContainText(/agents? running/i)
  await expect(page.locator('main')).not.toContainText(/healthy|verified/i)
  await studio.click()
  await expect(page).toHaveURL(`${base}/machines?machine=studio`)
  expect(errors).toEqual([])
  await context.close()
 }

 // A machine without a fresh CPU report reads No reading, with an empty grey bar, never 0%.
 // A session whose end was asked for is not running: the card says how many are ending, in words, beside the running count.
 if (run('ending sessions are not counted as running')) {
  const {context,page,errors,box} = await home()
  box.sessions = [...baseSessions().filter(s => s.agentId === 'a1'),
   session('s3','a2','Speed up the runner pool','end_requested'), session('s5','a2','Bench the cache','end_requested')]
  await page.goto(base)
  const desk = card(page,'desk')
  await expect(desk).toHaveAccessibleName(/^desk, offline, 0 sessions running · 2 ending \(last known, not reporting\), CPU No reading, 2 skills out of date$/)
  await expect(desk.locator('.figure')).toHaveText('0')
  await expect(desk.locator('.sessions')).toHaveText(/^0\s+sessions running\s*· 2 ending$/)
  await expect(desk.locator('.caption')).toHaveText('Last known count. desk is not reporting.')
  const studio = card(page,'studio')
  await expect(studio).toHaveAccessibleName(/^studio, online, 2 sessions running, CPU 38%/)
  await expect(studio.locator('.sessions')).not.toContainText('ending')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('missing CPU reads No reading')) {
  const {context,page,errors} = await home()
  await page.goto(base)
  const desk = card(page,'desk')
  await expect(desk.locator('.cpu')).toHaveText(/CPU\s*No reading/)
  await expect(desk).not.toContainText('91%')
  await expect(desk).not.toContainText('0%')
  await expect(desk.locator('.bar > span')).toHaveCount(0)
  await expect(card(page,'studio').locator('.bar > span')).toHaveAttribute('style',/width: ?38(\.\d+)?%/)
  expect(errors).toEqual([])
  await context.close()
 }

 // The session listing cannot say which worktrees reet made and has not removed, so no card shows a worktree figure.
 if (run('no worktree figure without a source')) {
  const {context,page,errors} = await home()
  await page.goto(base)
  await expect(card(page,'studio')).toBeVisible()
  await expect(strip(page)).not.toContainText(/worktree/i)
  await expect(rail(page)).not.toContainText(/worktree/i)
  expect(errors).toEqual([])
  await context.close()
 }

 // Add a machine is the tile after the cards and opens the existing Add a machine dialog.
 if (run('Add a machine opens the existing dialog')) {
  const {context,page,errors} = await home()
  await page.goto(base)
  const add = strip(page).getByRole('link',{name:'Add a machine',exact:true})
  await expect(add).toBeVisible()
  await add.click()
  await expect(page).toHaveURL(`${base}/machines#add-machine`)
  await expect(page.getByRole('dialog',{name:'Add a machine',exact:true})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // Group by Project (sessions of a Project's run under its name, the rest under their machine), Machine or State; the choice
 // is remembered for this account in this browser under one key.
 if (run('In flight groups by project, machine and state')) {
  const {context,page,errors,reads} = await home()
  await page.goto(base)
  const choice = inFlight(page).getByRole('radiogroup',{name:'Group by'})
  await expect(choice.getByRole('radio',{name:'Project'})).toBeChecked()
  await expect.poll(() => groups(page)).toEqual(['Store rewrite 1','No project · studio 2','No project · desk 1'])
  const row = inFlight(page).getByRole('link',{name:/Refactor the store layer/})
  await expect(row).toHaveAttribute('href','/terminals?agent=a1&session=s1')
  await expect(row).toContainText('Store rewrite')
  await expect(row).toContainText('studio')
  await expect(row.locator('.branch')).toHaveText('reet/store-split')
  await expect(row).toContainText('Running')
  await expect(row).toContainText("from the session's state")
  await expect(row).not.toContainText(/Step \d/)
  const exited = inFlight(page).getByRole('link',{name:/Docs pass/})
  await expect(exited).toContainText('Exited 1')
  await expect(exited).toContainText('from the exit code')
  await choice.getByRole('radio',{name:'Machine'}).check()
  await expect.poll(() => groups(page)).toEqual(['studio 3','desk 1'])
  await choice.getByRole('radio',{name:'State'}).check()
  await expect.poll(() => groups(page)).toEqual(['Running 3','Exited 1'])
  await page.reload()
  await expect(inFlight(page).getByRole('radio',{name:'State'})).toBeChecked()
  await expect.poll(() => groups(page)).toEqual(['Running 3','Exited 1'])
  // Runs are read only for Projects whose last run started in the last 24 hours.
  expect(reads).toContain('GET /api/v2/project-runs/r1')
  expect(reads).not.toContain('GET /api/v2/project-runs/r2')
  expect(errors).toEqual([])
  await context.close()
 }

 // A session no Project's run started is listed under its machine, not left out and not given a Project.
 if (run('a session outside every Project sits under its machine')) {
  const {context,page,errors} = await home()
  await page.goto(base)
  const group = inFlight(page).locator('.group').filter({has:page.getByRole('heading',{name:/^No project · studio/})})
  await expect(group.getByRole('link',{name:/Tidy the release notes/})).toBeVisible()
  await expect(group.getByRole('link',{name:/Tidy the release notes/})).not.toContainText('Store rewrite')
  expect(errors).toEqual([])
  await context.close()
 }

 // An offline machine's sessions stay in In flight with a sentence saying they are its last known state.
 if (run('offline machine keeps its sessions with the sentence')) {
  const {context,page,errors} = await home()
  await page.goto(base)
  const row = inFlight(page).getByRole('link',{name:/Speed up the runner pool/})
  await expect(row).toBeVisible()
  await expect(row).toContainText('last known')
  await expect(inFlight(page).getByText('desk is not reporting, so its sessions show their last known state.',{exact:true})).toBeVisible()
  await expect(inFlight(page).getByText(/studio is not reporting/)).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // Nothing today signals that a session needs the person: the honest empty state and a bell without a badge.
 if (run('Needs you says Nothing needs you and the bell shows no badge')) {
  const {context,page,errors} = await home()
  await page.goto(base)
  await expect(needs(page).getByRole('heading',{name:'Needs you'})).toBeVisible()
  await expect(needs(page).locator('.count')).toHaveText('0')
  await expect(needs(page).getByText('Nothing needs you.',{exact:true})).toBeVisible()
  const bell = page.getByRole('button',{name:'Needs you: 0',exact:true})
  await expect(bell).toBeVisible()
  await expect(bell.locator('.badge')).toHaveCount(0)
  // The exited session is not turned into a card.
  await expect(needs(page)).not.toContainText('Docs pass')
  const width = await needs(page).evaluate(el => el.getBoundingClientRect().width)
  expect(Math.round(width)).toBe(380)
  expect(errors).toEqual([])
  await context.close()
 }

 // The rail lists the saved Projects read-only: name, the machines its folders are on, how many of its sessions run.
 if (run('rail lists Projects read-only and Add opens New project')) {
  const {context,page,errors} = await home()
  await page.goto(base)
  const r = rail(page)
  await expect(r.getByRole('heading',{name:'Projects'})).toBeVisible()
  const store = r.getByRole('link',{name:/^Store rewrite/})
  await expect(store).toHaveAttribute('href','/projects/p1')
  await expect(store).toContainText('studio')
  await expect(store).toContainText('1 running')
  const notes = r.getByRole('link',{name:/^Notes/})
  await expect(notes).toContainText('desk')
  await expect(notes).toContainText('idle')
  await expect(r.getByText("These are your saved Projects. Browsing a machine's folders comes in a later release.",{exact:true})).toBeVisible()
  // Read-only: the one button is Add.
  await expect(r.getByRole('button')).toHaveCount(1)
  expect(Math.round(await r.evaluate(el => el.getBoundingClientRect().width))).toBe(272)
  await r.getByRole('button',{name:'Add a project'}).click()
  const dialog = page.getByRole('dialog',{name:'New project',exact:true})
  await expect(dialog).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await expect(r.getByRole('button',{name:'Add a project'})).toBeFocused()
  await store.click()
  await expect(page).toHaveURL(`${base}/projects/p1`)
  expect(errors).toEqual([])
  await context.close()
  // Under 1240 px the rail is a 56 px strip that still reaches each Project and Add.
  const narrow = await home({width:1200})
  await narrow.page.goto(base)
  const n = rail(narrow.page)
  await expect(n.getByRole('link',{name:'Store rewrite'})).toBeVisible()
  await expect(n.getByRole('button',{name:'Add a project'})).toBeVisible()
  expect(Math.round(await n.evaluate(el => el.getBoundingClientRect().width))).toBe(56)
  expect(await noSideways(narrow.page)).toBe(true)
  await narrow.context.close()
 }

 // Start an agent sits docked at the foot of the page and opens the existing dialog.
 if (run('Start an agent opens the existing dialog')) {
  const {context,page,errors} = await home()
  await page.goto(base)
  const start = page.getByRole('button',{name:'Start an agent',exact:true})
  await expect(start).toBeVisible()
  const docked = await start.evaluate(el => el.getBoundingClientRect().bottom)
  expect(docked).toBeLessThanOrEqual(812)
  expect(docked).toBeGreaterThan(812 - 120)
  await start.click()
  const dialog = page.getByRole('dialog',{name:'Start an agent',exact:true})
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('combobox',{name:'Machine',exact:true})).toHaveValue('a1')
  expect(errors).toEqual([])
  await context.close()
 }

 // Starting from Home goes straight to the new session, at the address the other pages use: the terminal page shows its
 // connecting state with the session's title, keyboard focus lands on the terminal once it is live, and Back returns to Home
 // with the new row in In flight.
 if (run('Start an agent from Home opens the session')) {
  let ready
  const attached = new Promise(r => { ready = r })
  const {context,page,errors,box} = await home({terminalStatus:{configured:true,available:true,reason:''}, routes:{}})
  await page.route('**/api/v2/terminal/sessions/*/attachments', route => route.fulfill({json:{ticket:'fixture-ticket00000000000'}}))
  await page.routeWebSocket('**/api/v2/terminal/socket', socket => {
   socket.onMessage(message => { if (typeof message === 'string' && JSON.parse(message).ticket) void attached.then(() => { socket.send('{"type":"ready"}'); socket.send(Buffer.from('ready> ')) }) })
  })
  await page.goto(base)
  await page.getByRole('button',{name:'Start an agent',exact:true}).click()
  const dialog = page.getByRole('dialog',{name:'Start an agent',exact:true})
  await dialog.getByRole('button',{name:'Start',exact:true}).click()
  await expect(page).toHaveURL(/\/terminals\?/)
  const s = box.created
  expect(s).not.toBeNull()
  await expect(page).toHaveURL(`${base}/terminals?agent=${encodeURIComponent(s.agentId)}&session=${encodeURIComponent(s.id)}`)
  // No blank screen: the pane is there at once with the title and its existing connecting state, and the region a screen
  // reader enters is named with the title.
  const pane = page.locator('.pane')
  await expect(pane).toHaveAttribute('data-connection-state','connecting')
  await expect(pane.locator('header .identity')).toContainText(s.title)
  await expect(pane.locator('header .state')).toContainText('Connecting')
  await expect(page.getByRole('region',{name:`${s.title} · Terminal on studio`,exact:true})).toBeVisible()
  ready()
  await expect(pane).toHaveAttribute('data-connection-state','live')
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused()
  await page.goBack()
  await expect(page).toHaveURL(`${base}/`)
  await expect(page.getByRole('dialog',{name:'Start an agent',exact:true})).toHaveCount(0)
  await expect(inFlight(page).getByRole('link',{name:new RegExp(s.title.replace(/[.*+?^${}()|[\]\\]/g,'\\$&'))})).toBeVisible()
  await expect(page.getByRole('button',{name:'Start an agent',exact:true})).toBeEnabled()
  expect(errors).toEqual([])
  await context.close()
 }

 // A start whose next page never arrives (a 204, the browser's Stop, a page that will not load) must not leave a modal with
 // no way out: after a few seconds the dialog lets go and Home says what started, with a link to it.
 if (run('a start whose page never arrives releases the dialog')) {
  const {context,page,errors,box} = await home()
  await page.route(url => new URL(url).pathname === '/terminals', route => route.fulfill({status:204, body:''}))
  await page.goto(base)
  const start = page.getByRole('button',{name:'Start an agent',exact:true})
  await start.click()
  const dialog = page.getByRole('dialog',{name:'Start an agent',exact:true})
  await expect(dialog.getByRole('button',{name:'Start',exact:true})).toBeEnabled()
  // Pressed from inside: Playwright's own click waits for the navigation it starts, which a 204 never finishes.
  await page.evaluate(() => document.querySelector('dialog[open] footer button[type=submit]').click())
  // Playwright's locators wait for a navigation that a 204 never finishes, so Home is read from inside.
  const look = () => page.evaluate(() => {
   const d = document.querySelector('dialog[open]'), n = document.querySelector('.notice')
   return {url: location.pathname, open: !!d, text: d?.querySelector('footer button[type=submit]')?.textContent, status: d?.querySelector('[role=status]')?.textContent,
    notice: n?.textContent ?? null, href: n?.querySelector('a')?.getAttribute('href') ?? null, focus: document.activeElement?.textContent?.trim() ?? null}
  })
  await expect.poll(look).toMatchObject({open:true, text:'Starting…', status:`Opening ${box.created.title}…`})
  await expect.poll(look, {timeout:9000}).toMatchObject({open:false})
  expect(await look()).toEqual({url:'/', open:false, text:undefined, status:undefined, notice:`Started ${box.created.title} on studio. Open it`,
   href:`/terminals?agent=a1&session=${encodeURIComponent(box.created.id)}`, focus:'Start an agent'})
  // The dialog works as before: it opens again, and Escape and Close shut it.
  const press = label => page.evaluate(label => [...document.querySelectorAll('button')].find(b => b.textContent.trim() === label || b.getAttribute('aria-label') === label).click(), label)
  await press('Start an agent')
  await expect.poll(look).toMatchObject({open:true, text:'Start'})
  await page.keyboard.press('Escape')
  await expect.poll(look).toMatchObject({open:false})
  await press('Start an agent')
  await expect.poll(look).toMatchObject({open:true, text:'Start'})
  await press('Close')
  await expect.poll(look).toMatchObject({open:false})
  expect(errors).toEqual([])
  await context.close()
 }

 // A browser may bring Home back from its cache on Back, with the dialog as it was when Home left (Starting…). The dialog
 // then closes at once and Home reads everything again. (Headless Chromium reloads instead, so the restore is simulated on
 // a start whose page is held back.)
 if (run('Home restored from the browser cache closes the dialog and reads again')) {
  const {context,page,errors,reads} = await home()
  await page.route(url => new URL(url).pathname === '/terminals', route => route.fulfill({status:204, body:''}))
  await page.goto(base)
  const start = page.getByRole('button',{name:'Start an agent',exact:true})
  await start.click()
  const dialog = page.getByRole('dialog',{name:'Start an agent',exact:true})
  await expect(dialog.getByRole('button',{name:'Start',exact:true})).toBeEnabled()
  // Pressed from inside: Playwright's own click waits for the navigation it starts, which a 204 never finishes.
  await page.evaluate(() => document.querySelector('dialog[open] footer button[type=submit]').click())
  const look = () => page.evaluate(() => ({open: !!document.querySelector('dialog[open]'), text: document.querySelector('dialog[open] footer button[type=submit]')?.textContent,
   status: document.querySelector('dialog[open] [role=status]')?.textContent, notice: document.querySelector('.notice')?.textContent ?? null, focus: document.activeElement?.textContent?.trim() ?? null}))
  // Answered and leaving, not still waiting for the answer.
  await expect.poll(look).toMatchObject({open:true, text:'Starting…', status:'Opening Claude Code…'})
  const before = reads.filter(r => r === 'GET /api/v2/terminal/sessions').length
  await page.evaluate(() => dispatchEvent(new PageTransitionEvent('pageshow', {persisted:true})))
  await expect.poll(look, {timeout:1000}).toMatchObject({open:false, focus:'Start an agent'})
  await expect.poll(() => reads.filter(r => r === 'GET /api/v2/terminal/sessions').length).toBeGreaterThan(before)
  // Restored, it is not a failed start: no notice.
  await page.waitForTimeout(5500)
  expect((await look()).notice).toBeNull()
  expect(errors).toEqual([])
  await context.close()
 }

 // A refusal or a lost reply leaves the dialog open on Home with its own sentence, as before; nothing navigates.
 if (run('a refused start stays on Home with the sentence')) {
  let answer = {status:422,json:{error:'not installed',code:'tool_not_installed'}}
  const {context,page,errors,box} = await home({routes:{'/api/v2/terminal/sessions': request => request.method() === 'POST' ? answer : {sessions:box.sessions}}})
  const navigations = []
  page.on('framenavigated', f => { if (f === page.mainFrame()) navigations.push(f.url()) })
  await page.goto(base)
  await page.getByRole('button',{name:'Start an agent',exact:true}).click()
  const dialog = page.getByRole('dialog',{name:'Start an agent',exact:true})
  await dialog.getByRole('button',{name:'Start',exact:true}).click()
  await expect(dialog.getByRole('status')).toHaveText("That agent isn't installed on that machine.")
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('button',{name:'Start',exact:true})).toBeEnabled()
  // A lost reply: the dialog says so and keeps the same request for Retry.
  answer = {status:502,json:{error:'bad gateway'}}
  await dialog.getByRole('button',{name:'Start',exact:true}).click()
  await expect(dialog.getByRole('status')).toContainText("We didn't hear back, so the agent may have started.")
  await expect(dialog.getByRole('button',{name:'Try the same request again',exact:true})).toBeEnabled()
  await page.waitForTimeout(500)
  expect(page.url()).toBe(`${base}/`)
  expect(navigations).toEqual([`${base}/`])
  expect(errors).toEqual([])
  await context.close()
 }

 // Between the answer and the terminal page arriving, the dialog keeps its Starting… state: a second press (or Escape) does
 // nothing and sends nothing.
 if (run('second press while starting sends nothing')) {
  const {context,page,errors,reads,box} = await home()
  // As Home goes (the moment the next page replaces it), the page notes what the dialog shows, then presses Start again, submits, presses Enter and
  // Escape, and counts the creates it tried to send.
  await page.addInitScript(() => {
   if (location.pathname !== '/') return
   let creates = 0
   const send = window.fetch.bind(window)
   window.fetch = (input, init) => { if ((init?.method ?? 'GET') === 'POST' && String(input instanceof Request ? input.url : input).includes('/terminal/sessions')) creates++; return send(input, init) }
   addEventListener('pagehide', () => {
    const d = document.querySelector('dialog[open]'), b = d?.querySelector('footer button[type=submit]'), x = d?.querySelector('[data-shell-close]')
    const seen = {open: !!d, text: b?.textContent, disabled: b?.disabled, busy: b?.getAttribute('aria-busy'), close: x?.disabled, status: d?.querySelector('[role=status]')?.textContent}
    const before = creates
    if (d) {
     b.click(); d.querySelector('form').requestSubmit()
     for (const key of ['Enter','Escape']) d.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles:true, cancelable:true}))
     d.dispatchEvent(new Event('cancel', {cancelable:true}))
    }
    // Kept where the next page (same tab, same origin) can read it; the test's only use of browser storage.
    sessionStorage.setItem('__e2e_leaving', JSON.stringify({...seen, stillOpen: !!document.querySelector('dialog[open]'), extra: creates - before}))
   })
  })
  await page.goto(base)
  await page.getByRole('button',{name:'Start an agent',exact:true}).click()
  const dialog = page.getByRole('dialog',{name:'Start an agent',exact:true})
  await dialog.getByRole('button',{name:'Start',exact:true}).click()
  await expect(page).toHaveURL(/\/terminals\?agent=a1&session=new-/)
  const leaving = JSON.parse(await page.evaluate(() => sessionStorage.getItem('__e2e_leaving')))
  expect(leaving).toEqual({open:true, text:'Starting…', disabled:true, busy:'true', close:true, status:`Opening ${box.created.title}…`, stillOpen:true, extra:0})
  expect(reads.filter(r => r === 'POST /api/v2/terminal/sessions')).toHaveLength(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // Viewers (and single-token and open sign-ins) get Home with the fleet strip alone (R16).
 if (run('viewer sees the fleet strip only')) {
  for (const [what, status] of [['viewer',{user:{...owner,role:'viewer'}}],['open',{mode:'none',user:null}]]) {
   const {context,page,errors} = await home({status})
   await page.goto(base)
   await expect(strip(page), what).toBeVisible()
   await expect(card(page,'studio')).toBeVisible()
   await expect(card(page,'studio')).not.toContainText('sessions running')
   await expect(inFlight(page)).toHaveCount(0)
   await expect(needs(page)).toHaveCount(0)
   await expect(rail(page)).toHaveCount(0)
   await expect(page.getByRole('button',{name:'Start an agent'})).toHaveCount(0)
   await expect(page.getByRole('button',{name:/^Needs you/})).toHaveCount(0)
   if (what === 'viewer') await expect(strip(page).getByRole('link',{name:'Add a machine'})).toHaveCount(0)
   expect(errors, what).toEqual([])
   await context.close()
  }
 }

 // A refresh that fails keeps the last lists and says so in a plain sentence; the next good refresh clears it.
 if (run('refresh error keeps the last list')) {
  const {context,page,errors,box,reads} = await home()
  await page.clock.install()
  await page.goto(base)
  await expect(inFlight(page).getByRole('link',{name:/Tidy the release notes/})).toBeVisible()
  const before = reads.filter(r => r === 'GET /api/v2/terminal/sessions').length
  box.fail.add('sessions'); box.fail.add('health')
  await page.clock.runFor(15_000)
  await expect(page.getByText("Couldn't refresh just now. Showing what was last read.",{exact:true})).toBeVisible()
  expect(reads.filter(r => r === 'GET /api/v2/terminal/sessions').length).toBeGreaterThan(before)
  await expect(inFlight(page).getByRole('link',{name:/Tidy the release notes/})).toBeVisible()
  await expect(card(page,'studio')).toBeVisible()
  box.fail.clear()
  box.sessions = box.sessions.filter(s => s.id !== 's2')
  await page.clock.runFor(15_000)
  await expect(page.getByText("Couldn't refresh just now. Showing what was last read.",{exact:true})).toBeHidden()
  await expect(inFlight(page).getByRole('link',{name:/Tidy the release notes/})).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // Only the Group by choice is stored, under reet.home.groupBy.<uid>: no Project, session or machine reaches browser storage.
 if (run('nothing of projects or sessions in storage')) {
  const {context,page,errors,storage} = await home()
  await page.goto(base)
  await inFlight(page).getByRole('radio',{name:'Machine'}).check()
  await expect.poll(() => groups(page)).toEqual(['studio 3','desk 1'])
  const written = storage.filter(w => /home|group/i.test(w.key))
  expect(written).toEqual([{op:'localStorage.set',key:'reet.home.groupBy.1',value:'machine'}])
  const dump = await page.evaluate(() => JSON.stringify([Object.entries(localStorage),Object.entries(sessionStorage)]))
  for (const secret of ['Store rewrite','Notes','Refactor the store layer','Speed up the runner pool','s1','p1','r1','studio','desk','a1']) expect(dump, secret).not.toContain(secret)
  expect(errors).toEqual([])
  await context.close()
 }

 // The drawings: desktop rail, strip, In flight beside Needs you; tablet and phone stack machines, Needs you, then In flight,
 // with no sideways scroll and 44 px targets (48 px for the phone's primary action).
 if (run('layout follows the desktop, tablet and phone drawings')) {
  for (const [width,touch] of [[1440,false],[834,true],[390,true],[320,true]]) {
   const {context,page,errors} = await home({width,touch})
   await page.goto(base)
   await expect(card(page,'studio')).toBeVisible()
   const top = async l => (await l.boundingBox()).y
   if (width >= 1100) {
    expect(await top(needs(page))).toBeCloseTo(await top(inFlight(page)), -1)
   } else {
    await expect(rail(page)).toBeHidden()
    expect(await top(strip(page))).toBeLessThan(await top(needs(page)))
    expect(await top(needs(page))).toBeLessThan(await top(inFlight(page)))
    for (const box of await page.locator('main a, main button, main label:has(input[type=radio])').evaluateAll(els => els.filter(e => e.getClientRects().length).map(e => [e.textContent.trim().slice(0,30), e.getBoundingClientRect().height])))
     expect(box[1], `${box[0]} at ${width}px`).toBeGreaterThanOrEqual(44)
   }
   if (width < 651) {
    const start = page.getByRole('button',{name:'Start an agent',exact:true})
    expect((await start.boundingBox()).height).toBeGreaterThanOrEqual(48)
    // The docked button sits above the bottom tab bar.
    const bar = await page.getByRole('navigation',{name:'Main navigation'}).boundingBox()
    expect((await start.boundingBox()).y + (await start.boundingBox()).height).toBeLessThanOrEqual(bar.y + 1)
   }
   expect(await noSideways(page), `no sideways scroll at ${width}px`).toBe(true)
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // A single-token sign-in cannot use the cookie-only health read: Home draws the strip from the sync agents, keeps the stored
 // token and never signs the person out. An open instance is refused with 403 and does the same.
 if (run('token and open sign-ins draw the strip from sync and stay signed in')) {
  for (const [what, status] of [['token',{mode:'token',user:null}],['open',{mode:'none',user:null}]]) {
   const {context,page,errors,reads} = await home({status})
   await page.clock.install()
   if (what === 'token') await page.addInitScript(() => { if (!sessionStorage.getItem('seeded')) { localStorage.setItem('reet-token','fixture-token'); sessionStorage.setItem('seeded','1') } })
   await page.goto(base)
   await expect(card(page,'studio'), what).toHaveAccessibleName(/^studio, online, CPU No reading, skills in sync/)
   await expect(card(page,'desk'), what).toHaveAccessibleName(/^desk, offline, /)
   await page.clock.runFor(15_000)
   await expect(card(page,'studio')).toBeVisible()
   if (what === 'token') expect(await page.evaluate(() => localStorage.getItem('reet-token'))).toBe('fixture-token')
   await expect(page.getByText("Couldn't refresh just now. Showing what was last read.",{exact:true})).toHaveCount(0)
   await expect(page.getByText("Couldn't read everything just now. Home tries again every 15 seconds.",{exact:true})).toHaveCount(0)
   expect(reads.filter(r => r.endsWith('/api/v2/machines/health')), `${what} does not ask for health it cannot read`).toEqual([])
   expect(errors, what).toEqual([])
   await context.close()
  }
 }

 // A machine whose sync agent is paused but still reporting says "paused", the word Control uses (fleet.ts), never "online":
 // from the sync agents (a single-token sign-in) and beside a health report alike, on a phone chip too.
 if (run('a paused agent that still reports reads paused')) {
  const paused = {...sync, agents:[{...sync.agents[0], paused:true}, sync.agents[1], {id:'a3',host:'quiet-box',version:'1',lastSeen:now,paused:true,autoUpdate:false}]}
  for (const [what, options] of [['token',{status:{mode:'token',user:null}}],['writer',{}],['phone',{width:390,touch:true}]]) {
   const {context,page,errors} = await home({...options, sync:paused})
   if (what === 'token') await page.addInitScript(() => localStorage.setItem('reet-token','fixture-token'))
   await page.goto(base)
   await expect(card(page,'studio'), what).toHaveAccessibleName(/^studio, paused, /)
   await expect(card(page,'studio').locator('.status'), what).toHaveText('paused')
   await expect(card(page,'studio').locator('.status'), what).toBeVisible()
   await expect(card(page,'studio').locator('.caption'), what).toHaveCount(0)
   if (what === 'token') await expect(card(page,'quiet-box')).toHaveAccessibleName(/^quiet-box, paused, /)
   await expect(strip(page).getByRole('link',{name:/, online, /}), what).toHaveCount(0)
   expect(errors, what).toEqual([])
   await context.close()
  }
 }

 // A health read that fails on a refresh keeps the cards as last read and says so; it never swaps in a guess from the sync
 // agents (a viewer included).
 if (run('a failed health refresh keeps the last cards')) {
  for (const [what, status] of [['writer',undefined],['viewer',{user:{...owner,role:'viewer'}}]]) {
   const {context,page,errors,box} = await home(status ? {status} : {})
   await page.clock.install()
   await page.goto(base)
   await expect(card(page,'studio')).toHaveAccessibleName(/CPU 38%/)
   const before = await strip(page).getByRole('link').evaluateAll(els => els.map(e => e.getAttribute('aria-label') || e.textContent.trim()))
   box.fail.add('health')
   await page.clock.runFor(15_000)
   await expect(page.getByText("Couldn't refresh just now. Showing what was last read.",{exact:true}), what).toBeVisible()
   expect(await strip(page).getByRole('link').evaluateAll(els => els.map(e => e.getAttribute('aria-label') || e.textContent.trim())), what).toEqual(before)
   expect(errors, what).toEqual([])
   await context.close()
  }
 }

 // Until a sessions list has been read, no card claims a count: never a false "0 sessions running". The lists that could not
 // be read say so instead of "Reading…".
 if (run('a failed first sessions read shows no count')) {
  const {context,page,errors,box} = await home()
  box.fail.add('sessions'); box.fail.add('projects')
  await page.goto(base)
  await expect(page.getByText("Couldn't read everything just now. Home tries again every 15 seconds.",{exact:true})).toBeVisible()
  await expect(card(page,'studio')).toHaveAccessibleName(/^studio, online, CPU 38%, /)
  await expect(strip(page)).not.toContainText(/running/)
  await expect(strip(page).locator('.figure')).toHaveCount(0)
  await expect(inFlight(page).getByText("Couldn't read your sessions just now.",{exact:true})).toBeVisible()
  await expect(inFlight(page)).not.toContainText('Reading your sessions')
  await expect(rail(page).getByText("Couldn't read your Projects just now.",{exact:true})).toBeVisible()
  await expect(rail(page)).not.toContainText('Reading your Projects')
  expect(errors).toEqual([])
  await context.close()
 }

 // A refresh already under way when a Project is saved must not drop it when its older answer lands. (A start from Home no
 // longer refreshes: Home is leaving, so a poll in flight may still replace the list without the new row. That is harmless:
 // the next page shows the session, and a Home restored from the browser cache reads everything again.)
 if (run('a slow refresh does not drop a just-saved Project')) {
  const {context,page,errors,box} = await home()
  await page.clock.install()
  await page.goto(base)
  await expect(inFlight(page).getByRole('link',{name:/Tidy the release notes/})).toBeVisible()
  let release
  box.hold = new Promise(r => { release = r })
  await page.clock.runFor(15_000)
  const saved = {id:'p9',name:'Fresh project',defaultTool:'claude',defaultModel:'',useWorktrees:false,hasSharedPrompt:false,members:[{host:'a1',dir:'~/fresh',tool:'',model:'',position:0}],createdAt:1,updatedAt:1,version:1}
  await page.route('**/api/v2/projects', route => route.request().method() === 'POST' ? (box.projects = [saved, ...projects], route.fulfill({json:saved})) : route.fallback())
  await rail(page).getByRole('button',{name:'Add a project'}).click()
  const pd = page.getByRole('dialog',{name:'New project',exact:true})
  await pd.getByRole('textbox',{name:'Name'}).fill('Fresh project')
  await pd.getByRole('button',{name:'Add a repository'}).click()
  await pd.getByRole('combobox',{name:'Folder',exact:true}).selectOption({label:'Type a path…'})
  await pd.getByRole('textbox',{name:'Folder path'}).fill('/home/dev/fresh')
  await pd.getByRole('button',{name:'Save Project'}).click()
  await expect(pd).toBeHidden()
  await expect(rail(page).getByRole('link',{name:/^Fresh project/})).toBeVisible()
  release()
  await page.waitForTimeout(1000)
  await expect(rail(page).getByRole('link',{name:/^Fresh project/})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // Status is never colour alone: a phone chip that is not online says so in words, and the rail's narrow strip names each
 // Project's state in its link.
 if (run('status in words on phone chips and the rail strip')) {
  const phone = await home({width:390,touch:true})
  await phone.page.goto(base)
  await expect(card(phone.page,'desk').locator('.status')).toBeVisible()
  await expect(card(phone.page,'desk').locator('.status')).toHaveText('offline')
  await expect(card(phone.page,'desk').locator('.dot')).toHaveClass(/offline/)
  expect(await card(phone.page,'desk').locator('.dot').evaluate(e => getComputedStyle(e).borderStyle)).toBe('solid')
  await phone.context.close()
  const narrow = await home({width:1200})
  await narrow.page.goto(base)
  await expect(rail(narrow.page).getByRole('link',{name:'Store rewrite, 1 running',exact:true})).toBeVisible()
  await expect(rail(narrow.page).getByRole('link',{name:'Notes, idle',exact:true})).toBeVisible()
  await narrow.context.close()
 }
 // The shell and Home across screens, themes and motion: 1360 px with a mouse, 834 px on a touch tablet and 390 and 320 px on a
 // phone (isMobile and hasTouch), each in light and dark. No sideways scroll or zoom-out; targets at least 44 px on touch (48 px
 // for the phone's primary action) and 24 px with a mouse; every tab stop shows a focus ring; state is in words or shape, never
 // colour alone; reduced motion leaves nothing animating.
 if (run('matrix: widths, themes, targets, focus, state words and motion')) {
  for (const [width,touch] of [[1360,false],[834,true],[390,true],[320,true]]) {
   const {context,page,errors} = await home({width,touch,theme:'system'})
   await page.emulateMedia({colorScheme:'light', reducedMotion:'no-preference'})
   await page.goto(base)
   await expect(card(page,'studio')).toBeVisible()
   const backgrounds = new Set()
   for (const scheme of ['light','dark']) {
    const at = `${width}px ${scheme}`
    await page.emulateMedia({colorScheme:scheme})
    backgrounds.add(await page.evaluate(() => getComputedStyle(document.body).backgroundColor))
    expect(await page.evaluate(() => [document.documentElement.scrollWidth <= innerWidth, innerWidth, Math.round((visualViewport?.scale ?? 1) * 100)]), `${at}: no sideways scroll or zoom`).toEqual([true, width, 100])
    // Every visible target in the shell and on Home.
    const sizes = await page.locator('header a, header button, header summary, nav a, nav button, main a, main button, main label:has(input[type=radio])').evaluateAll(els => els
     .filter(e => e.getClientRects().length && getComputedStyle(e).visibility !== 'hidden' && !e.closest('.vh'))
     .map(e => { const r = e.getBoundingClientRect(); return [(e.getAttribute('aria-label') || e.textContent).trim().replace(/\s+/g,' ').slice(0,30), Math.round(r.width), Math.round(r.height)] }))
    expect(sizes.length, at).toBeGreaterThan(8)
    for (const [name,w,h] of sizes) expect(Math.min(w,h), `${at}: "${name}" is ${w}×${h}`).toBeGreaterThanOrEqual(touch ? 44 : 24)
    if (width < 651) expect((await page.getByRole('button',{name:'Start an agent',exact:true}).boundingBox()).height, at).toBeGreaterThanOrEqual(48)
    // State in words (or shape): each card names its status; one not online shows the word; the dots differ by shape too.
    for (const host of ['studio','desk']) await expect(card(page,host), at).toHaveAccessibleName(new RegExp(`^${host}, (online|offline), `))
    await expect(card(page,'desk').locator('.status'), at).toBeVisible()
    await expect(card(page,'desk').locator('.status'), at).toHaveText('offline')
    const dots = await strip(page).locator('.dot').evaluateAll(els => els.map(e => { const s = getComputedStyle(e); return `${s.borderTopStyle} ${s.borderTopWidth} ${s.backgroundImage}` }))
    expect(new Set(dots).size, `${at}: online and offline dots differ in shape, not only colour`).toBe(2)
    for (const word of await inFlight(page).locator('.row .word').evaluateAll(els => els.map(e => [e.textContent.trim(), e.getClientRects().length > 0])))
     expect(word[1] && /^(Running|Starting|Ending|Exited \d+|Finished|Failed|Lost)/.test(word[0]), `${at}: row state "${word[0]}" in words`).toBe(true)
    await expect(needs(page).locator('.count'), at).toHaveText('0')
   }
   expect(backgrounds.size, `${width}px: light and dark differ`).toBe(2)
   // Focus: the first 30 tab stops each draw a ring (an outline or an inset shadow on the control, its label or its field box).
   await page.locator('body').click({position:{x:1,y:1}})
   const seen = []
   for (let i = 0; i < 30; i++) {
    await page.keyboard.press('Tab')
    const f = await page.evaluate(() => {
     const el = document.activeElement
     if (!el || el === document.body) return null
     const ring = n => { if (!n) return false; const s = getComputedStyle(n); return (s.outlineStyle !== 'none' && parseFloat(s.outlineWidth) > 0) || (s.boxShadow !== 'none' && s.boxShadow.includes('inset')) }
     return {name:(el.getAttribute('aria-label') || el.textContent || el.tagName).trim().replace(/\s+/g,' ').slice(0,30), ring:ring(el) || ring(el.closest('label')) || ring(el.closest('.jump'))}
    })
    if (!f) continue
    seen.push(f.name)
    expect(f.ring, `${width}px: "${f.name}" shows focus`).toBe(true)
   }
   expect(seen.length, `${width}px tab stops`).toBeGreaterThan(5)
   // Motion: with no preference a press transition exists (so the check is not empty); with reduce nothing animates or moves.
   const moving = () => page.evaluate(() => [...document.querySelectorAll('header *, nav *, main *')].filter(e => { const s = getComputedStyle(e); return s.animationName !== 'none' || (/transform|all/.test(s.transitionProperty) && s.transitionDuration.split(',').some(d => parseFloat(d) > 0)) }).length + document.getAnimations().length)
   expect(await moving(), `${width}px: motion without the preference`).toBeGreaterThan(0)
   await page.emulateMedia({reducedMotion:'reduce'})
   // A transition already running when the preference flips (a focus ring fading) finishes on its own; wait for those, then it must stay at 0.
   await expect.poll(moving, {message:`${width}px: nothing moves with reduced motion`, timeout:3000}).toBe(0)
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // Each open of Start an agent reads the machines again: one that was not ready the first time is offered once it is,
 // without a reload.
 if (run('Start an agent reads the machines on every open')) {
  let reads = 0
  const notReady = tmachines.map(m => m.agentId === 'a1' ? {...m, ready:false, capabilities:{...m.capabilities, readiness:'terminal_disabled'}} : m)
  const {context,page,errors} = await home({routes:{'/api/v2/terminal/machines': () => ({machines: reads++ === 0 ? notReady : tmachines})}})
  await page.goto(base)
  const start = page.getByRole('button',{name:'Start an agent',exact:true})
  const dialog = page.getByRole('dialog',{name:'Start an agent',exact:true})
  const option = dialog.getByRole('combobox',{name:'Machine',exact:true}).locator('option[value="a1"]')
  await start.click()
  await expect(option).toBeDisabled()
  await expect(option).toHaveText('studio · not ready')
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await start.click()
  await expect(option).toBeEnabled()
  await expect(option).toHaveText('studio')
  expect(reads).toBe(2)
  expect(errors).toEqual([])
  await context.close()
 }
 console.log('Home: fleet strip with sessions running, CPU or No reading and the skills line, no worktree figure, Add a machine, In flight grouped by Project, Machine and State with offline sessions kept, Needs you empty with a quiet bell, the read-only rail with Add, Start an agent going straight to the new session (Starting… until Home leaves, refusals kept on Home, Back to the new row), the read-only Home, refresh errors keeping the last lists, storage limited to the Group by key, the desktop, tablet and phone layouts, paused machines in words, and the width, theme, target, focus and motion matrix.')
} finally { await browser.close() }
