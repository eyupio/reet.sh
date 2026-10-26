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
 const box = {sessions:baseSessions(), fail:new Set(), created:null}
 const routes = {
  '/api/v2/machines/health': () => box.fail.has('health') ? {status:500,json:{error:'down'}} : health(),
  '/api/v2/terminal/machines': {machines:tmachines},
  '/api/v2/terminal/sessions': request => {
   if (request.method() === 'POST') {
    const body = request.postDataJSON()
    const s = session(`new-${body.requestId.slice(0,8)}`, body.agentId, body.title, 'running', {createdAt:Date.now(),updatedAt:Date.now()})
    box.sessions = [s, ...box.sessions]; box.created = s
    return s
   }
   return box.fail.has('sessions') ? {status:500,json:{error:'down'}} : {sessions:box.sessions}
  },
  '/api/v2/projects': () => box.fail.has('projects') ? {status:500,json:{error:'down'}} : {projects},
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

 // Start an agent sits docked at the foot of the page and opens the existing dialog; the started session joins In flight.
 if (run('Start an agent opens the existing dialog and the new session appears')) {
  const {context,page,errors,box} = await home()
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
  await dialog.getByRole('button',{name:'Start',exact:true}).click()
  await expect(dialog).toBeHidden()
  expect(box.created).not.toBeNull()
  await expect(inFlight(page).getByRole('link',{name:new RegExp(box.created.title.replace(/[.*+?^${}()|[\]\\]/g,'\\$&'))})).toBeVisible()
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
 console.log('Home: fleet strip with sessions running, CPU or No reading and the skills line, no worktree figure, Add a machine, In flight grouped by Project, Machine and State with offline sessions kept, Needs you empty with a quiet bell, the read-only rail with Add, Start an agent, the read-only Home, refresh errors keeping the last lists, storage limited to the Group by key, and the desktop, tablet and phone layouts.')
} finally { await browser.close() }
