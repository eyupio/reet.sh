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
 {agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:{...caps('linux'),browse:true,branches:true,worktree:true}},
 {agentId:'a2',host:'desk',ready:false,serverEnabled:true,capabilities:caps('darwin')},
]
const session = (id, agentId, title, state='running', over={}) => ({id,agentId,title,state,createdAt:now-2*HOUR,updatedAt:now-12*MIN,exitCode:null,...over})
const baseSessions = () => [
 session('s1','a1','Refactor the store layer','running',{projectId:'fp1',worktreePath:'/home/dev/.reet/worktrees/store/store/split',worktreeRepo:'store',worktreeBranch:'reet/store-split'}),
 session('s2','a1','Tidy the release notes'),
 session('s3','a2','Speed up the runner pool'),
 session('s4','a1','Docs pass','exited',{exitCode:1,updatedAt:now-HOUR}),
]
// Folder projects as GET /api/v2/folder-projects lists them: store is a repository on studio, notes a plain folder on desk.
const fproject = (id, name, host, dir, over={}) => ({id,host,machine:host === 'a1' ? 'studio' : 'desk',dir,name,isRepo:true,baseBranch:'',worktreeDefault:'inherit',defaultTool:'',defaultModel:'',hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1,...over})
const fprojects = () => [fproject('fp1','Store rewrite','a1','/home/dev/store',{baseBranch:'main'}), fproject('fp2','Notes','a2','/home/dev/notes',{isRepo:false,position:1})]
// An old saved Project run whose worktree on studio is still recorded, for the Worktrees menu.
const oldProject = {id:'p1',name:'Store rewrite',defaultTool:'claude',defaultModel:'',useWorktrees:true,hasSharedPrompt:false,members:[{host:'a1',dir:'/home/dev//./store/',tool:'',model:'',position:0}],createdAt:1,updatedAt:1,version:1}
const oldRun = {id:'r1',projectId:'p1',projectName:'Store rewrite',state:'complete',startedAt:now-3*24*HOUR,useWorktrees:true,worktreeProject:'store-rewrite',members:[{position:0,host:'a1',dir:'/home/dev//./store/',tool:'claude',model:'',state:'started',code:'',sentence:'',sessionId:'old1',worktreePath:'/home/dev/.reet/worktrees/store-rewrite/store/r1',worktreeRepo:'store',worktreeBranch:'reet/store-rewrite-r1',worktreeRun:'r1',worktreeProject:'store-rewrite',sharesCheckout:false,attempt:1}]}
const job = (id, agentId, targetId, actions, ago) => ({id,agentId,targetId,kind:'preview',status:'awaiting-approval',revision:'r',hash:'h',plan:{actions:Array.from({length:actions},(_,i)=>({add:true,skill:{id:`k${i}`,name:`k${i}`}}))},message:'',createdAt:Date.now()-ago-MIN,updatedAt:Date.now()-ago})
const target = (id, host) => ({target:{id,host,agent:'claude'},sourceId:'s',sourceName:'src',desired:[],missing:[],extra:[],revision:'r',updatedAt:Date.now()})
// Built per test: the ages it shows are read against the page's clock, so a copy made when the file started would drift a
// minute for every minute the earlier tests took.
const makeSync = () => ({environments:[],machines:[],runs:[],updateJobs:[],
 agents:[{id:'a1',host:'studio',version:'1',lastSeen:Date.now(),paused:false,autoUpdate:false},{id:'a2',host:'desk',version:'1',lastSeen:Date.now()-2*HOUR,paused:false,autoUpdate:false}],
 targets:[target('t1','studio'),target('t2','desk')],
 jobs:[job('j1','a1','t1',0,5*MIN),job('j2','a2','t2',2,HOUR)]})

// A writer's Home with the fixtures above. `fail` names paths that answer 500 from now on; `sessions` is the live list.
async function home(options={}) {
 const reads = [], storage = []
 // `hold` delays the next sessions read, answering with the list as it was when the read arrived.
 const box = {sessions:baseSessions(), fail:new Set(), created:null, hold:null, projects:options.projects ?? fprojects(), savedWaiting:options.savedWaiting ?? 0, writes:[], holdProjects:null}
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
    box.sessions = [s, ...box.sessions]; box.created = s; box.createdBody = body
    return s
   }
   if (box.fail.has('sessions')) return {status:500,json:{error:'down'}}
   const then = {sessions:box.sessions}
   if (box.hold) { const wait = box.hold; box.hold = null; return wait.then(() => then) }
   return then
  },
  '/api/v2/folder-projects': request => {
   if (request.method() === 'POST') {
    const body = request.postDataJSON()
    box.writes.push({method:'POST', path:'/api/v2/folder-projects', body})
    const made = fproject(`fp${box.projects.length + 10}`, body.name || body.dir.split('/').pop(), body.host, body.dir, {isRepo:!!body.isRepo, position:box.projects.length})
    box.projects = [...box.projects, made]
    return {status:201, json:made}
   }
   if (box.fail.has('projects')) return {status:500,json:{error:'down'}}
   const then = {projects:box.projects, savedWaiting:box.savedWaiting}
   if (box.holdProjects) { const wait = box.holdProjects; box.holdProjects = null; return wait.then(() => then) }
   return then
  },
  '/api/v2/folder-projects/reorder': request => {
   const body = request.postDataJSON()
   box.writes.push({method:'POST', path:'/api/v2/folder-projects/reorder', body})
   box.projects = body.ids.map((id, i) => ({...box.projects.find(p => p.id === id), position:i, version:box.projects.find(p => p.id === id).version + 1}))
   return {projects:box.projects, savedWaiting:box.savedWaiting}
  },
  '/api/v2/folder-projects/import-saved': request => {
   box.writes.push({method:'POST', path:'/api/v2/folder-projects/import-saved', body:request.postData()})
   const created = box.savedWaiting
   for (let i = 0; i < created; i++) box.projects = [...box.projects, fproject(`fp-saved-${i}`, `saved-${i}`, 'a1', `/home/dev/saved-${i}`, {position:box.projects.length})]
   box.savedWaiting = 0
   return {projects:box.projects, savedWaiting:0, created, skipped:0, message:created ? `Added ${created} saved repositories as projects.` : "Nothing is waiting to be added: every saved folder is already in reet, or can't be used as a project."}
  },
  '/api/v2/folder-projects/fp1': request => {
   const current = box.projects.find(p => p.id === 'fp1')
   if (request.method() === 'GET') return {...current, starterPrompt:''}
   const match = request.headers()['if-match']
   box.writes.push({method:request.method(), path:'/api/v2/folder-projects/fp1', body:request.postData() ? request.postDataJSON() : null, match})
   if (match !== `"${current.version}"`) return {status:409, json:{error:'Someone changed this project. Reload it, then try again.', code:'project_changed'}}
   if (request.method() === 'DELETE') { box.projects = box.projects.filter(p => p.id !== 'fp1'); return {ok:true, message:'Project removed. Its sessions and any worktrees stay on studio.'} }
   const body = request.postDataJSON()
   const next = {...current, ...body, version:current.version + 1}
   box.projects = box.projects.map(p => p.id === 'fp1' ? next : p)
   return next
  },
  '/api/v2/folder-projects/fp2': request => ({...box.projects.find(p => p.id === 'fp2'), starterPrompt:''}),
  '/api/v2/machines/a1/browse': request => {
   const path = new URL(request.url()).searchParams.get('path') || '/home/dev'
   return {path, home:'/home/dev', parent:'', repo:path === '/home/dev/site' ? 'root' : 'none', repoTop:'', entries:path === '/home/dev' ? [{name:'site',dir:true,repoRoot:true,hidden:false}] : [], more:false, partial:false, scanned:1, omitted:0}
  },
  '/api/v2/projects': {projects:[oldProject, {...oldProject, id:'p9', name:'Broken shop'}]},
  '/api/v2/projects/p9/runs': {status:500, json:{error:'down'}},
  '/api/v2/projects/p1/runs': {runs:[oldRun]},
  ...(options.routes ?? {}),
 }
 const opened = await openWorkspace(browser, {width:1440, sync:makeSync(), routes, ...options, routes})
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
  await expect(studio).toHaveAccessibleName(/^studio, online, 2 sessions running, 1 worktree in use, CPU 38%, skills in sync · checked 5 min ago$/)
  await expect(studio.locator('.figure')).toHaveText('2')
  await expect(studio).toContainText('sessions running')
  await expect(studio.locator('.os')).toHaveText('Linux')
  await expect(studio.locator('.cpu')).toHaveText(/CPU\s*38%/)
  await expect(studio.locator('.skills')).toHaveText('skills in sync · checked 5 min ago')
  await expect(studio.locator('.skills')).toHaveClass(/good/)
  const desk = card(page,'desk')
  await expect(desk.locator('.os')).toHaveText('macOS')
  await expect(desk.locator('.skills')).toHaveText('2 skills out of date')
  await expect(desk.locator('.skills')).toHaveClass(/warn/)
  // An offline machine's count is what was last known: the card says so.
  await expect(desk).toHaveAccessibleName(/^desk, offline, 1 session running \(last known, not reporting\), no worktrees in use, CPU No reading, 2 skills out of date$/)
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
  await expect(desk).toHaveAccessibleName(/^desk, offline, 0 sessions running · 2 ending \(last known, not reporting\), no worktrees in use, CPU No reading, 2 skills out of date$/)
  await expect(desk.locator('.figure')).toHaveText('0')
  await expect(desk.locator('.sessions')).toHaveText(/^0\s+sessions running\s*· 2 ending$/)
  await expect(desk.locator('.caption')).toHaveText('Last known count. desk is not reporting.')
  const studio = card(page,'studio')
  await expect(studio).toHaveAccessibleName(/^studio, online, 2 sessions running, 1 worktree in use, CPU 38%/)
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

 // The session listing has no record of removed worktrees, so the only figure is the worktrees live sessions are in now,
 // labelled "in use": a finished session's worktree is never counted.
 if (run('worktree figure counts only worktrees in use')) {
  const {context,page,errors,box} = await home()
  box.sessions = [...baseSessions(), session('s9','a1','Old pass','exited',{exitCode:0,worktreePath:'/home/dev/.reet/worktrees/x/y/z',worktreeBranch:'reet/x-z'})]
  await page.goto(base)
  await expect(card(page,'studio').locator('.worktrees')).toHaveText('1 worktree in use')
  await expect(card(page,'desk').locator('.worktrees')).toHaveText('no worktrees in use')
  await expect(rail(page).getByRole('button',{name:/^Store rewrite, /})).toContainText('1 worktree in use')
  await expect(rail(page).getByRole('button',{name:/^Notes, /})).not.toContainText('worktree')
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
  // Grouping comes from each session's own project id: no old Projects or runs are read for it.
  expect(reads.filter(r => /\/api\/v2\/(projects|project-runs)/.test(r))).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 // A session started outside every project is listed under its machine, not left out and not given a project.
 if (run('a session outside every project sits under its machine')) {
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

 // The rail lists the folder projects: repository diamond or folder icon, name, machine tag, base branch, state in words and
 // worktrees in use. Choosing a card opens Start an agent in that folder (the launch bar takes over in a later release); the
 // card menu has New session here, Edit, Worktrees, Move up and down, and Remove from reet (confirmed).
 if (run('rail lists folder projects with their menu')) {
  const {context,page,errors,box} = await home()
  await page.goto(base)
  const r = rail(page)
  await expect(r.getByRole('heading',{name:'Projects'})).toBeVisible()
  const store = r.getByRole('button',{name:'Store rewrite, studio, main, 1 running, 1 worktree in use',exact:true})
  await expect(store).toBeVisible()
  await expect(store.locator('.icon.repo')).toHaveCount(1)
  await expect(store.locator('.tag')).toHaveText('studio')
  await expect(store.locator('.branch')).toHaveText('main')
  await expect(store.locator('.branch')).toHaveCSS('font-family', /Plex Mono|mono/i)
  await expect(store.locator('.state')).toHaveText('1 running')
  const notes = r.getByRole('button',{name:'Notes, desk, machine offline',exact:true})
  await expect(notes.locator('.icon.folder')).toHaveCount(1)
  await expect(notes.locator('.branch')).toHaveCount(0)
  await expect(r.getByText('Folders are browsed live on the machine',{exact:true})).toBeVisible()
  expect(Math.round(await r.evaluate(el => el.getBoundingClientRect().width))).toBe(301)
  // Choosing the card starts an agent in that folder on its machine, tied to the project.
  await store.click()
  const start = page.getByRole('dialog',{name:'Start an agent',exact:true})
  await expect(start.getByRole('combobox',{name:'Machine',exact:true})).toHaveValue('a1')
  await expect(start.getByRole('textbox',{name:'Folder path'})).toHaveValue('/home/dev/store')
  await page.route(url => new URL(url).pathname === '/terminals', route => route.fulfill({status:200, contentType:'text/html', body:'<title>t</title>'}))
  await start.getByRole('button',{name:'Start',exact:true}).click()
  await expect.poll(() => box.created?.id ?? null).not.toBeNull()
  expect(box.createdBody).toMatchObject({agentId:'a1', dir:'/home/dev/store', projectId:'fp1'})
  await expect(page).toHaveURL(/\/terminals\?/)
  await page.goto(base)
  // The menu: New session here does the same; Move down sends the whole order with the versions read.
  const menuButton = rail(page).getByRole('button',{name:'More for Store rewrite',exact:true})
  await menuButton.click()
  const menu = page.getByRole('menu',{name:'Store rewrite'})
  await expect(menu.getByRole('menuitem')).toHaveText(['New session here','Edit','Worktrees','Move up','Move down','Remove from reet'])
  await expect(menu.getByRole('menuitem',{name:'Move up'})).toBeDisabled()
  await expect(menu.getByRole('menuitem',{name:'New session here'})).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(menu).toBeHidden()
  await expect(menuButton).toBeFocused()
  await menuButton.click()
  await menu.getByRole('menuitem',{name:'Move down'}).click()
  await expect.poll(() => box.writes.find(w => w.path === '/api/v2/folder-projects/reorder')?.body ?? null).toEqual({ids:['fp2','fp1'], versions:[1,1]})
  await expect.poll(() => rail(page).locator('.full .card').evaluateAll(els => els.map(e => e.querySelector('.name').textContent))).toEqual(['Notes','Store rewrite'])
  // Edit reads the project with its starter prompt and saves only what changed, with the version read.
  await rail(page).getByRole('button',{name:'More for Store rewrite',exact:true}).click()
  await page.getByRole('menu',{name:'Store rewrite'}).getByRole('menuitem',{name:'Edit'}).click()
  const edit = page.getByRole('dialog',{name:'Edit Store rewrite',exact:true})
  await edit.getByRole('textbox',{name:/^Name/}).fill('Store two')
  await edit.getByRole('button',{name:'Save changes'}).click()
  await expect(edit).toBeHidden()
  await expect(page.getByText('Sorted. Store two saved.',{exact:true})).toBeVisible()
  const patch = box.writes.find(w => w.method === 'PATCH')
  expect(patch.body).toEqual({name:'Store two'})
  expect(patch.match).toBe('"2"')
  // Remove from reet asks first, sends the version read, and says what stays on the machine.
  await rail(page).getByRole('button',{name:'More for Store two',exact:true}).click()
  await page.getByRole('menu',{name:'Store two'}).getByRole('menuitem',{name:'Remove from reet'}).click()
  const confirm = page.getByRole('dialog',{name:'Remove Store two from reet?',exact:true})
  await expect(confirm).toContainText('Its sessions and any worktrees stay on studio')
  await confirm.getByRole('button',{name:'Cancel'}).click()
  expect(box.writes.filter(w => w.method === 'DELETE')).toEqual([])
  await rail(page).getByRole('button',{name:'More for Store two',exact:true}).click()
  await page.getByRole('menu',{name:'Store two'}).getByRole('menuitem',{name:'Remove from reet'}).click()
  await page.getByRole('dialog',{name:'Remove Store two from reet?',exact:true}).getByRole('button',{name:'Remove from reet'}).click()
  await expect(page.getByText('Project removed. Its sessions and any worktrees stay on studio.',{exact:true})).toBeVisible()
  await expect(rail(page).getByRole('button',{name:/^Store two, /})).toHaveCount(0)
  expect(box.writes.find(w => w.method === 'DELETE').match).toBe('"3"')
  await expect(page.locator('main')).not.toContainText(/healthy|verified/i)
  expect(errors).toEqual([])
  await context.close()
  // Under 1240 px the rail is a 56 px strip that still reaches each project and Add.
  const narrow = await home({width:1200})
  await narrow.page.goto(base)
  const n = rail(narrow.page)
  await expect(n.getByRole('button',{name:'Store rewrite, studio, main, 1 running, 1 worktree in use',exact:true})).toBeVisible()
  await expect(n.getByRole('button',{name:'Add a project'})).toBeVisible()
  expect(Math.round(await n.evaluate(el => el.getBoundingClientRect().width))).toBe(56)
  expect(await noSideways(narrow.page)).toBe(true)
  await narrow.context.close()
 }

 // The Worktrees menu lists the worktrees recorded for that folder: the ones live sessions are in, and those old Project
 // runs made there, with the existing Check and remove sheet.
 if (run('worktrees menu lists recorded worktrees with Check and remove')) {
  const {context,page,errors} = await home()
  await page.goto(base)
  await rail(page).getByRole('button',{name:'More for Store rewrite',exact:true}).click()
  await page.getByRole('menu',{name:'Store rewrite'}).getByRole('menuitem',{name:'Worktrees'}).click()
  const d = page.getByRole('dialog',{name:'Worktrees for Store rewrite',exact:true})
  await expect(d).toContainText('reet/store-split')
  await expect(d).toContainText('In use by Refactor the store layer')
  // The old Project saved the folder as /home/dev//./store/: cleaned as the server cleans it, it is this project's.
  await expect(d).toContainText('reet/store-rewrite-r1')
  await expect(d).toContainText('the newest 20 runs of each saved Project')
  // One saved Project's runs could not be read: it is named, and the others are still listed.
  await expect(d).toContainText("Couldn't read the runs of Broken shop just now.")
  await d.getByRole('button',{name:'Check and remove reet/store-rewrite-r1'}).click()
  await expect(page.getByRole('dialog',{name:/worktree/i}).last()).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // Add opens the folder browser on a machine that can browse; a typed or chosen folder is added with one press, and the
 // page says so in its status line.
 if (run('Add opens the folder browser')) {
  const {context,page,errors,box} = await home()
  await page.goto(base)
  const add = rail(page).getByRole('button',{name:'Add a project'})
  await add.click()
  const d = page.getByRole('dialog',{name:'Add a project',exact:true})
  await expect(d.getByRole('list',{name:/^Folders in /})).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(d).toBeHidden()
  await expect(add).toBeFocused()
  await add.click()
  await d.getByRole('list',{name:/^Folders in /}).getByRole('button',{name:'site, git repository'}).click()
  await d.getByRole('button',{name:'Use this folder'}).click()
  await d.getByRole('button',{name:'Add project'}).click()
  await expect(d).toBeHidden()
  await expect(page.getByText('Sorted. site added.',{exact:true})).toBeVisible()
  expect(box.writes.find(w => w.method === 'POST').body).toEqual({host:'a1', dir:'/home/dev/site', isRepo:true, worktreeDefault:'inherit'})
  await expect(rail(page).getByRole('button',{name:/^site, studio/})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // The note from moving saved Projects shows on its card until the person dismisses it; dismissing clears it for good.
 if (run('migration note shown once')) {
  const gates = {}
  const {context,page,errors,box} = await home({gates, projects:[fproject('fp1','Store rewrite','a1','/home/dev/store',{note:'Old shop, Site'}), fproject('fp2','Notes','a2','/home/dev/notes',{isRepo:false,position:1})]})
  await page.goto(base)
  const note = rail(page).getByRole('note')
  await expect(note).toHaveCount(1)
  await expect(note).toContainText('Moved from your saved Projects.')
  await expect(note).toContainText('Old shop, Site')
  // Pressed twice while the first is on its way: one change is sent, and nobody else is said to have changed it.
  // Both presses run in one task, so a second request would be issued right after the first, before any answer.
  let release
  const issued = []
  page.on('request', r => { if (r.method() === 'PATCH') issued.push(new URL(r.url()).pathname) })
  gates['/api/v2/folder-projects/fp1'] = new Promise(r => { release = r })
  await note.getByRole('button',{name:'Dismiss the note on Store rewrite',exact:true}).evaluate(b => { b.click(); b.click() })
  await expect.poll(() => issued.length).toBe(1)
  // While it is on its way no card can be moved: the order would name the version the dismiss is replacing.
  await rail(page).getByRole('button',{name:'More for Notes',exact:true}).click()
  await expect(page.getByRole('menu',{name:'Notes'}).getByRole('menuitem',{name:'Move up'})).toBeDisabled()
  await page.keyboard.press('Escape')
  delete gates['/api/v2/folder-projects/fp1']
  release()
  await expect(rail(page).getByRole('note')).toHaveCount(0)
  expect(issued).toEqual(['/api/v2/folder-projects/fp1'])
  expect(box.writes.filter(w => w.method === 'PATCH')).toHaveLength(1)
  await expect(page.getByText(/Someone changed/)).toHaveCount(0)
  const patch = box.writes.find(w => w.method === 'PATCH')
  expect(patch.body).toEqual({note:''})
  expect(patch.match).toBe('"1"')
  await page.reload()
  await expect(rail(page).getByRole('button',{name:/^Store rewrite, /})).toBeVisible()
  await expect(rail(page).getByRole('note')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // Saved repositories that waited for room are brought in only when the person asks; the answer is shown as sent.
 if (run('remaining saved Projects come in when asked')) {
  const {context,page,errors,box} = await home({savedWaiting:2})
  await page.goto(base)
  const prompt = rail(page).getByRole('button',{name:'Add the waiting folders (2)',exact:true})
  await expect(prompt).toBeVisible()
  await prompt.click()
  await expect(page.getByText('Added 2 saved repositories as projects.',{exact:true})).toBeVisible()
  await expect(prompt).toHaveCount(0)
  await expect(rail(page).getByRole('button',{name:/^saved-1, studio/})).toBeVisible()
  expect(box.writes.filter(w => w.path === '/api/v2/folder-projects/import-saved')).toHaveLength(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // One change at a time per press: a second Move while the first is on its way sends nothing; after Remove from reet the
 // focus goes to the next card (or Add), and when the remove is refused it goes back to the card's menu button.
 if (run('moves and removes are sent once and keep the focus')) {
  const gates = {}
  const {context,page,errors,box} = await home({gates})
  await page.goto(base)
  let release
  gates['/api/v2/folder-projects/reorder'] = new Promise(r => { release = r })
  const more = name => rail(page).getByRole('button',{name:`More for ${name}`,exact:true})
  await more('Store rewrite').click()
  await page.getByRole('menu',{name:'Store rewrite'}).getByRole('menuitem',{name:'Move down'}).click()
  await more('Store rewrite').click()
  await expect(page.getByRole('menu',{name:'Store rewrite'}).getByRole('menuitem',{name:'Move down'})).toBeDisabled()
  await page.keyboard.press('Escape')
  delete gates['/api/v2/folder-projects/reorder']
  release()
  await expect.poll(() => rail(page).locator('.full .card .name').allTextContents()).toEqual(['Notes','Store rewrite'])
  expect(box.writes.filter(w => w.path === '/api/v2/folder-projects/reorder')).toHaveLength(1)
  // A refused remove (someone else changed the project): the sentence, and focus back on the menu button.
  box.projects = box.projects.map(p => p.id === 'fp1' ? {...p, version:p.version + 5} : p)
  await more('Store rewrite').click()
  await page.getByRole('menu',{name:'Store rewrite'}).getByRole('menuitem',{name:'Remove from reet'}).click()
  const reread = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v2/folder-projects' && r.request().method() === 'GET')
  await page.getByRole('dialog',{name:'Remove Store rewrite from reet?',exact:true}).getByRole('button',{name:'Remove from reet'}).click()
  await expect(page.getByText('Someone changed this project. Reload it, then try again.',{exact:true})).toBeVisible()
  await expect(more('Store rewrite')).toBeFocused()
  // Read again, then removed: focus goes to a card that is still there.
  await reread
  await expect.poll(async () => (await rail(page).getByRole('button',{name:/^Store rewrite, /}).count())).toBe(1)
  await more('Notes').click()
  await page.getByRole('menu',{name:'Notes'}).getByRole('menuitem',{name:'Remove from reet'}).click()
  await page.getByRole('dialog',{name:'Remove Notes from reet?',exact:true}).getByRole('button',{name:'Remove from reet'}).click()
  await expect(rail(page).getByRole('button',{name:/^Notes, /})).toHaveCount(0)
  await expect(rail(page).getByRole('button',{name:/^Store rewrite, /})).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
 }

 // From 1100 to 1239 px the rail is a 56 px strip: each project shows its repository badge, branch and worktrees in use in
 // place, and every action of the full rail is reachable by keyboard through Open the project rail, which marks a note or
 // saved folders waiting.
 if (run('collapsed strip reaches every rail action by keyboard')) {
  for (const width of [1239, 1100]) {
   const {context,page,errors,box} = await home({width, savedWaiting:2, projects:[fproject('fp1','Store rewrite','a1','/home/dev/store',{baseBranch:'main',note:'Old shop'}), fproject('fp2','Notes','a2','/home/dev/notes',{isRepo:false,position:1})]})
   await page.goto(`${base}/?project=fp1`)
   const strip = rail(page).locator('.strip')
   const store = strip.getByRole('button',{name:'Store rewrite, studio, main, 1 running, 1 worktree in use',exact:true})
   await expect(store).toHaveAttribute('aria-current','true')
   await expect(strip.getByRole('button',{name:/^Notes, /})).not.toHaveAttribute('aria-current','true')
   await expect(store, `${width}`).toBeVisible()
   await expect(store.locator('.badge')).toBeVisible()
   await expect(store.locator('.mini-branch')).toHaveText('main')
   await expect(store.locator('.mini-wt')).toHaveText('1')
   expect(Math.round(await rail(page).evaluate(el => el.getBoundingClientRect().width))).toBe(56)
   const toggle = strip.locator('button.manage')
   await expect(toggle).toHaveAccessibleName('Open the project rail: 1 note, 2 folders waiting')
   // Opened from a scrolled page, the panel starts where the strip is on screen (the header has scrolled away).
   await page.evaluate(() => scrollTo(0, 120))
   await toggle.focus()
   await page.keyboard.press('Enter')
   await expect(toggle).toHaveAttribute('aria-expanded','true')
   await expect(toggle).toHaveAccessibleName('Close the project rail')
   const panel = rail(page).locator('.full')
   await expect(panel.getByRole('button',{name:'Add a project'})).toBeFocused()
   const [panelTop, stripTop] = await page.evaluate(() => [document.querySelector('aside.rail .full').getBoundingClientRect().top, document.querySelector('aside.rail').getBoundingClientRect().top])
   expect(Math.round(panelTop)).toBe(Math.max(0, Math.round(stripTop)))
   // Tab and Shift+Tab stay inside the panel: the page drawn under it never takes focus.
   for (const key of [...Array(25).fill('Tab'), ...Array(5).fill('Shift+Tab')]) {
    await page.keyboard.press(key)
    expect(await page.evaluate(() => !!document.activeElement?.closest('aside.rail .full')), `${width}px ${key}`).toBe(true)
   }
   await panel.getByRole('button',{name:'Add a project'}).focus()
   // The card menu.
   await panel.getByRole('button',{name:'More for Store rewrite',exact:true}).focus()
   await page.keyboard.press('Enter')
   const menu = page.getByRole('menu',{name:'Store rewrite'})
   await expect(menu.getByRole('menuitem')).toHaveText(['New session here','Edit','Worktrees','Move up','Move down','Remove from reet'])
   await expect(menu.getByRole('menuitem',{name:'New session here'})).toBeFocused()
   for (const item of ['Edit','Worktrees','Move down','Remove from reet']) {
    await page.keyboard.press('ArrowDown')
    if (item !== 'Edit') continue
    await expect(menu.getByRole('menuitem',{name:'Edit'})).toBeFocused()
   }
   await expect(menu.getByRole('menuitem',{name:'Remove from reet'})).toBeFocused()
   await page.keyboard.press('Escape')
   // The note's Dismiss and the saved folders prompt.
   const dismiss = panel.getByRole('button',{name:'Dismiss the note on Store rewrite',exact:true})
   await dismiss.focus()
   await page.keyboard.press('Enter')
   await expect.poll(() => box.writes.filter(w => w.method === 'PATCH').length).toBe(1)
   const bring = panel.getByRole('button',{name:'Add the waiting folders (2)',exact:true})
   await bring.focus()
   await page.keyboard.press('Enter')
   await expect.poll(() => box.writes.filter(w => w.path === '/api/v2/folder-projects/import-saved').length).toBe(1)
   // Escape closes the panel and gives focus back.
   await panel.getByRole('button',{name:'Add a project'}).focus()
   await page.keyboard.press('Escape')
   await expect(toggle).toHaveAttribute('aria-expanded','false')
   await expect(toggle).toHaveAccessibleName(/^Open the project rail/)
   await expect(toggle).toBeFocused()
   await expect(panel).toBeHidden()
   // Its own Close button does the same.
   await toggle.click()
   await panel.getByRole('button',{name:'Close the project rail',exact:true}).click()
   await expect(panel).toBeHidden()
   await expect(toggle).toBeFocused()
   expect(await noSideways(page)).toBe(true)
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // On a tablet or phone the rail is below In flight: a project chosen by the address is scrolled into view and named.
 if (run('a chosen project is shown on a phone')) {
  const {context,page,errors} = await home({width:390, touch:true})
  await page.goto(`${base}/?project=fp2`)
  const card = rail(page).getByRole('button',{name:/^Notes, /})
  await expect(card).toHaveAttribute('aria-current','true')
  await expect(page.getByText('Showing Notes in Projects below.',{exact:true})).toBeVisible()
  await expect(card).toBeInViewport()
  expect(errors).toEqual([])
  await context.close()
 }

 // A sign-in lost while adding a project goes back to sign in, as the other dialogs do.
 if (run('a lost sign-in while adding returns to sign in')) {
  const {context,page,errors} = await home({routes:{'/api/v2/folder-projects': request => request.method() === 'POST' ? {status:401, json:{error:'Sign in to use projects.', code:'signed_out'}} : {projects:fprojects(), savedWaiting:0}}})
  await page.goto(base)
  await rail(page).getByRole('button',{name:'Add a project'}).click()
  const d = page.getByRole('dialog',{name:'Add a project',exact:true})
  await d.getByRole('button',{name:'Hide browser'}).click()
  await d.getByRole('textbox',{name:'Folder',exact:true}).fill('/home/dev/site')
  await d.getByRole('button',{name:'Add project'}).click()
  // Signed out on / is the welcome and sign-in page.
  await expect(page).toHaveTitle('Your AI fleet control room · reet.sh')
  await expect(rail(page)).toHaveCount(0)
  await expect(page.getByRole('dialog',{name:'Add a project',exact:true})).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
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
  await expect.poll(() => box.created?.title, {message:'the create request reached the mock'}).toBeTruthy()
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
  for (const secret of ['Store rewrite','Notes','Refactor the store layer','Speed up the runner pool','s1','fp1','/home/dev','studio','desk','a1']) expect(dump, secret).not.toContain(secret)
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
    // The rail sits under In flight until the Sessions tab takes it.
    await expect(rail(page)).toBeVisible()
    expect(await top(strip(page))).toBeLessThan(await top(needs(page)))
    expect(await top(needs(page))).toBeLessThan(await top(inFlight(page)))
    expect(await top(inFlight(page))).toBeLessThan(await top(rail(page)))
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
  const synced = makeSync()
   const paused = {...synced, agents:[{...synced.agents[0], paused:true}, synced.agents[1], {id:'a3',host:'quiet-box',version:'1',lastSeen:Date.now(),paused:true,autoUpdate:false}]}
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
  await expect(rail(page).getByText("Couldn't read your projects just now.",{exact:true})).toBeVisible()
  // Projects read but sessions not: no card says "idle" or how many run; the offline machine is still named.
  box.fail.delete('projects')
  await page.reload()
  await expect(rail(page).getByRole('button',{name:'Store rewrite, studio, main',exact:true})).toBeVisible()
  await expect(rail(page).getByRole('button',{name:'Notes, desk, machine offline',exact:true})).toBeVisible()
  await expect(rail(page)).not.toContainText('idle')
  await expect(rail(page)).not.toContainText('Reading your projects')
  expect(errors).toEqual([])
  await context.close()
 }

 // A refresh already under way when a project is added must not drop it when its older answer lands. (A start from Home no
 // longer refreshes: Home is leaving, so a poll in flight may still replace the list without the new row. That is harmless:
 // the next page shows the session, and a Home restored from the browser cache reads everything again.)
 if (run('a slow refresh does not drop a just-added project')) {
  const {context,page,errors,box} = await home()
  await page.clock.install()
  await page.goto(base)
  await expect(inFlight(page).getByRole('link',{name:/Tidy the release notes/})).toBeVisible()
  let release
  box.holdProjects = new Promise(r => { release = r })
  await page.clock.runFor(15_000)
  await rail(page).getByRole('button',{name:'Add a project'}).click()
  const d = page.getByRole('dialog',{name:'Add a project',exact:true})
  await d.getByRole('textbox',{name:'Folder',exact:true}).fill('/home/dev/fresh')
  await d.getByRole('button',{name:'Add project'}).click()
  await expect(d).toBeHidden()
  await expect(rail(page).getByRole('button',{name:/^fresh, studio/})).toBeVisible()
  // The older list's answer arrives and is handled (two frames later); the new project is still there.
  const older = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v2/folder-projects' && r.request().method() === 'GET')
  release()
  await older
  await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))))
  await expect(rail(page).getByRole('button',{name:/^fresh, studio/})).toBeVisible()
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
  await expect(rail(narrow.page).locator('.strip').getByRole('button',{name:'Store rewrite, studio, main, 1 running, 1 worktree in use',exact:true})).toBeVisible()
  await expect(rail(narrow.page).locator('.strip').getByRole('button',{name:'Notes, desk, machine offline',exact:true})).toBeVisible()
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
 console.log('Home: fleet strip with sessions running, worktrees in use, CPU or No reading and the skills line, Add a machine, In flight grouped by folder project, Machine and State with offline sessions kept, Needs you empty with a quiet bell, the folder-project rail with its menu, Worktrees, Add with the folder browser, the migration note and the saved Projects waiting, Start an agent going straight to the new session (Starting… until Home leaves, refusals kept on Home, Back to the new row), the read-only Home, refresh errors keeping the last lists, storage limited to the Group by key, the desktop, tablet and phone layouts, paused machines in words, and the width, theme, target, focus and motion matrix.')
} finally { await browser.close() }
