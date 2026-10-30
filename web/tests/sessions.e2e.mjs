// The Sessions page (/sessions): every session across machines, grouped by Project, Machine or State as on Home, with search,
// filters, the actions menu, and the projects below. Synthetic API answers; nothing is started or ended on a machine.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const shots = process.env.REET_UI_SCREENSHOTS

const MIN = 60_000, HOUR = 60 * MIN
const report = cpu => ({cpuPercent:{value:cpu},memory:{totalBytes:{value:4},availableBytes:{value:2}},disk:{totalBytes:{value:4},availableBytes:{value:2}},hostUptimeSeconds:{value:1},scope:'host'})
const caps = os => ({protocolVersion:1,os,osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,tools:['claude','codex'],worktree:true,browse:true,branches:true,agentsV2:true})
// studio and server report now; desk does not.
const health = () => ({serverNow:Date.now(),machines:[
 {agentId:'a1',host:'studio',state:'connected',capabilities:caps('linux'),connection:{connected:true,reportFresh:true,missedProbes:0,reconnects:0,path:'direct',report:report(38)}},
 {agentId:'a2',host:'server',state:'connected',capabilities:caps('linux'),connection:{connected:true,reportFresh:true,missedProbes:0,reconnects:0,path:'direct',report:report(12)}},
 {agentId:'a3',host:'desk',state:'unreachable',capabilities:caps('darwin'),connection:{connected:false,reportFresh:false,missedProbes:4,reconnects:2,path:'direct'}},
]})
const tmachines = [
 {agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:caps('linux')},
 {agentId:'a2',host:'server',ready:true,serverEnabled:true,capabilities:caps('linux')},
 {agentId:'a3',host:'desk',ready:false,serverEnabled:true,capabilities:caps('darwin')},
]
const project = (id, name, host, machine, colour, position) => ({id,host,machine,dir:`/home/dev/${name}`,name,isRepo:true,baseBranch:'main',worktreeDefault:'inherit',colour,defaultTool:'',defaultModel:'',hasStarterPrompt:false,note:'',position,createdAt:1,updatedAt:1,version:1})
const projects = () => [project('p1','reet','a1','studio','green',0), project('p2','site','a2','server','blue',1), project('p3','notes','a3','desk','',2)]
const session = (id, agentId, title, ago, over={}) => ({id,agentId,title,state:'running',createdAt:Date.now()-2*HOUR,updatedAt:Date.now()-ago,exitCode:null,...over})
// Shaped like the mock-up's Sessions screen: reet on studio with three, site on server with one, and a session on desk.
const sessions = () => [
 session('s1','a1','Refactor the store layer',12*MIN,{tool:'claude',model:'sonnet',projectId:'p1',worktreePath:'/home/dev/.reet/worktrees/reet/reet/a1b2c3d4e5f60718',worktreeRepo:'reet',worktreeBranch:'reet/reet-a1b2c3d4e5f60718'}),
 session('s2','a1','Add browse-v1 protocol',2*MIN,{tool:'claude',model:'sonnet',projectId:'p1',worktreePath:'/home/dev/.reet/worktrees/reet/reet/0f1e2d3c4b5a6978',worktreeRepo:'reet',worktreeBranch:'reet/reet-0f1e2d3c4b5a6978'}),
 session('s3','a1','Docs pass',HOUR,{tool:'codex',projectId:'p1',state:'exited',exitCode:0}),
 session('s4','a2','Speed up the runner pool',34*MIN,{tool:'claude',projectId:'p2',yolo:true}),
 session('s5','a3','Review code',9*MIN,{tool:'codex',projectId:'p3'}),
 session('s6','a1','Shell',3*HOUR,{state:'ended',worktreePath:'/home/dev/.reet/worktrees/reet/reet/1122334455667788',worktreeRepo:'reet',worktreeBranch:'reet/reet-1122334455667788'}),
]

// The skills line's record: studio's one target was last checked three minutes ago and found nothing to change.
const sync = () => ({environments:[],machines:[],agents:[{id:'a1',host:'studio',version:'test',lastSeen:Date.now(),paused:false,autoUpdate:false}],
 targets:[{target:{id:'t1',host:'studio'},revision:'r1',sourceId:'',sourceName:'',desired:[],missing:[],extra:[],updatedAt:1}],
 jobs:[{id:'j1',agentId:'a1',targetId:'t1',kind:'apply',status:'succeeded',revision:'r1',hash:'',plan:{actions:[]},message:'',createdAt:1,updatedAt:Date.now()-3*MIN}],runs:[],updateJobs:[]})
// What the worktree status read answers: 23 uncommitted changes, the paths the machine listed.
const changedPaths = Array.from({length:23}, (_, i) => i < 3 ? ['internal/terminalproto/browse.go','internal/terminalhost/browse.go','docs/terminal-launch.md'][i] : `web/src/file${i}.ts`)

async function open(options={}) {
 const box = {sessions:options.sessions ?? sessions(), projects:options.projects ?? projects(), fail:false, writes:[], commands:[], hold:null, status:[], sockets:[]}
 const routes = {
  '/api/v2/machines/health': () => box.fail ? {status:500,json:{error:'down'}} : health(),
  '/api/v2/terminal/machines': () => box.fail ? {status:500,json:{error:'down'}} : {machines:options.machines ?? tmachines},
  '/api/v2/terminal/sessions': async () => { if (box.hold) await box.hold; return box.fail ? {status:500,json:{error:'down'}} : {sessions:box.sessions} },
  '/api/v2/terminal/commands': request => {
   if (request.method() === 'GET') return {commands:box.commands}
   const item = {...request.postDataJSON(), id:`c${box.commands.length + 1}`, revision:1}
   box.commands.push(item)
   return item
  },
  '/api/v2/folder-projects': () => box.fail ? {status:500,json:{error:'down'}} : {projects:box.projects,savedWaiting:0},
  '/api/v2/projects': {projects:[]},
  '/api/v2/terminal/status': {configured:true,available:true,reason:''},
  '/api/v2/terminal/worktrees/status': request => { box.status.push(request.postDataJSON()); return {path:'/home/dev/.reet/worktrees/reet/reet/0f1e2d3c4b5a6978',branch:'reet/reet-0f1e2d3c4b5a6978',changed:options.changed ?? changedPaths.length,unpushed:0,paths:changedPaths.slice(0, options.changed ?? changedPaths.length),unreferenced:0,hidden:0,nested:false,incomplete:false} },
 }
 const opened = await openWorkspace(browser, {width:options.width ?? 1440, height:options.height ?? 900, touch:options.touch, theme:options.theme ?? 'light', routes, sync:options.sync ?? sync()})
 // An open session's terminal: a ticket, then a socket that says it is ready.
 await opened.context.routeWebSocket('**/api/v2/terminal/socket', socket => { box.sockets.push(socket); socket.onMessage(message => { if (typeof message === 'string' && JSON.parse(message).ticket) { socket.send('{"type":"ready"}'); socket.send(Buffer.from('ready> ')) } }) })
 // The sessions panel's saved colours, as the workspace keeps them in this browser.
 if (options.colours) await opened.page.addInitScript(colors => localStorage.setItem('reet-terminal-layout:1:sessions', JSON.stringify({colors})), options.colours)
 // A session's own writes: rename (PATCH), end and remove from history (POST).
 await opened.page.route('**/api/v2/terminal/sessions/*', async route => {
  const request = route.request(), id = decodeURIComponent(new URL(request.url()).pathname.split('/').pop())
  box.writes.push({kind:'rename',id,body:request.postDataJSON()})
  const s = box.sessions.find(x => x.id === id)
  if (s) s.title = request.postDataJSON().title
  return route.fulfill({json:s ?? {}})
 })
 await opened.page.route('**/api/v2/terminal/sessions/*/*', async route => {
  const request = route.request(), parts = new URL(request.url()).pathname.split('/'), id = decodeURIComponent(parts.at(-2)), kind = parts.at(-1)
  if (kind === 'attachments') return route.fulfill({json:{ticket:'sessions-fixture-ticket01'}})
  box.writes.push({kind,id,body:request.postDataJSON()})
  const s = box.sessions.find(x => x.id === id)
  if (kind === 'end' && s) s.state = 'end_requested'
  if (kind === 'archive') box.sessions = box.sessions.filter(x => x.id !== id)
  return route.fulfill({json:{}})
 })
 return {...opened, box}
}
const rows = page => page.locator('main .session-row')
const filters = page => page.locator('main details.filters')
const titles = async page => (await rows(page).locator('.title').allInnerTexts()).map(t => t.trim())
const noSideScroll = page => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)

try {
 // The page itself: the heading, Group by shared with Home, the groups as the mock-up draws them, and the projects below.
 if (run('page')) {
  const {context,page,errors,state} = await open()
  await page.goto(`${base}/sessions`)
  await expect(page.getByRole('heading',{name:'Sessions',level:1})).toBeVisible()
  expect(await page.title()).toBe('Sessions · reet.sh')
  await expect(page.getByRole('navigation',{name:'Main navigation'}).getByRole('link',{name:'Sessions',exact:true})).toHaveAttribute('aria-current','page')
  // New goes to Home with the launch prompt focused, as the session screen's +.
  await expect(page.locator('main').getByRole('link',{name:'New session',exact:true})).toHaveAttribute('href','/#launch')
  const group = page.getByRole('radiogroup',{name:'Group by'})
  await expect(group.getByRole('radio',{name:'Project'})).toBeChecked()
  // Home's grouping: projects by name, then the rest under their machine.
  await expect(page.locator('main .group h2 .name')).toHaveText(['notes','reet','site','Other on studio'])
  await expect(page.locator('main .group').nth(1).locator('.machine-pill')).toHaveText('studio')
  expect(await titles(page)).toEqual(['Review code','Add browse-v1 protocol','Refactor the store layer','Docs pass','Speed up the runner pool','Shell'])
  // A row: state in words as well as colour, agent · model · branch, badges, the age, and its link.
  const first = rows(page).nth(1)
  await expect(first.locator('.word')).toHaveText('Running')
  await expect(first.locator('.meta')).toHaveText('Claude Code · sonnet · reet/reet-0f1e2d3c4b5a6978')
  await expect(first.locator('.badge:not(.colour-name)')).toHaveText(['worktree'])
  await expect(first.locator('a.open')).toHaveAttribute('href','/sessions?agent=a1&session=s2')
  await expect(rows(page).filter({hasText:'Speed up the runner pool'}).locator('.badge:not(.colour-name)')).toHaveText(['YOLO'])
  // Group by Machine, then State; the choice is Home's.
  await group.getByText('Machine',{exact:true}).click()
  await expect(page.locator('main .group h2 .name')).toHaveText(['studio','server','desk'])
  expect(await page.evaluate(() => localStorage.getItem('reet.home.groupBy.1'))).toBe('machine')
  await group.getByText('State',{exact:true}).click()
  await expect(page.locator('main .group h2 .name')).toHaveText(['Running','Exited','Ended'])
  // Projects · N, with Add a project and a card per project that opens it on Home.
  await expect(page.getByText('Projects · 3',{exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'Add a project'})).toBeVisible()
  await expect(page.getByRole('link',{name:/^reet,/})).toHaveAttribute('href','/?project=p1')
  await page.getByRole('button',{name:'Add a project'}).click()
  await expect(page.getByRole('dialog',{name:/Add a project/})).toBeVisible()
  await page.keyboard.press('Escape')
  // Nothing about the sessions or projects goes into browser storage: only the Group by choice.
  const stored = await page.evaluate(() => Object.keys(localStorage))
  expect(stored.filter(k => !k.startsWith('reet.home.groupBy.') && k !== 'reet-theme')).toEqual([])
  expect(errors).toEqual([])
  // Every read the page makes is answered (the Add project dialog's folder listing is its own test's).
  expect(state.unexpected.filter(p => !p.endsWith('/browse'))).toEqual([])
  await context.close()
 }

 if (run('search matches branch and machine')) {
  const {context,page,errors} = await open()
  await page.goto(`${base}/sessions`)
  const search = page.getByRole('searchbox',{name:'Search sessions'})
  await search.fill('reet/reet-a1b2')
  await expect.poll(() => titles(page)).toEqual(['Refactor the store layer'])
  await search.fill('desk')
  await expect.poll(() => titles(page)).toEqual(['Review code'])
  await search.fill('codex studio')
  await expect.poll(() => titles(page)).toEqual(['Docs pass'])
  await search.fill('nothing like it')
  await expect(page.getByText('No sessions match.',{exact:true})).toBeVisible()
  // The query stays on the page: never in the address.
  expect(new URL(page.url()).search).toBe('')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('filters combine')) {
  const {context,page,errors} = await open()
  await page.goto(`${base}/sessions`)
  await page.getByText('Filters',{exact:true}).click()
  await filters(page).getByLabel('State',{exact:true}).selectOption('running')
  await expect.poll(() => titles(page)).toEqual(['Review code','Add browse-v1 protocol','Refactor the store layer','Speed up the runner pool'])
  await filters(page).getByLabel('Agent',{exact:true}).selectOption('claude')
  await expect.poll(() => titles(page)).toEqual(['Add browse-v1 protocol','Refactor the store layer','Speed up the runner pool'])
  await filters(page).getByLabel('Machine',{exact:true}).selectOption('a1')
  await expect.poll(() => titles(page)).toEqual(['Add browse-v1 protocol','Refactor the store layer'])
  await filters(page).getByLabel('YOLO only',{exact:true}).check()
  await expect(rows(page)).toHaveCount(0)
  await expect(page.getByText('No sessions match these filters.',{exact:true})).toBeVisible()
  await filters(page).getByLabel('YOLO only',{exact:true}).uncheck()
  await filters(page).getByLabel('Worktree only',{exact:true}).check()
  await expect.poll(() => titles(page)).toEqual(['Add browse-v1 protocol','Refactor the store layer'])
  await expect(page.locator('summary').filter({hasText:'Filters'})).toContainText('4 on')
  await page.getByRole('button',{name:'Clear filters'}).click()
  await expect(rows(page)).toHaveCount(6)
  // Sort: newest change, name, or the custom order the sessions panel saved in this browser.
  await filters(page).getByLabel('Sort',{exact:true}).selectOption('name')
  expect((await titles(page)).slice(1,4)).toEqual(['Add browse-v1 protocol','Docs pass','Refactor the store layer'])
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('actions menu by right-click and Shift+F10')) {
  const {context,page,errors,box} = await open()
  await page.goto(`${base}/sessions`)
  const row = rows(page).filter({hasText:'Refactor the store layer'})
  await row.locator('a.open').click({button:'right'})
  const menu = page.getByRole('menu',{name:'Actions for Refactor the store layer'})
  await expect(menu).toBeVisible()
  await expect(menu.getByRole('menuitem')).toHaveText(['Rename…','Open in theatre','Copy branch','End session…','Check and remove worktree…'])
  await expect(menu.getByRole('menuitem',{name:'Open in theatre'})).toHaveAttribute('href','/sessions?agent=a1&session=s1&theatre=1')
  // A live session's worktree is in use: removing it waits for the session to end.
  await expect(menu.getByRole('menuitem',{name:'Check and remove worktree…'})).toBeDisabled()
  await expect(menu.getByRole('menuitem',{name:'Rename…'})).toBeFocused()
  await page.keyboard.press('ArrowDown')
  await expect(menu.getByRole('menuitem',{name:'Open in theatre'})).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(menu).toBeHidden()
  await expect(row.locator('a.open')).toBeFocused()
  // Shift+F10 on the focused row opens the same menu: the row's own key handler (a synthetic key press, which the browser
  // does not turn into a contextmenu event), then the real key.
  await row.locator('a.open').dispatchEvent('keydown',{key:'F10',shiftKey:true})
  await expect(menu).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(menu).toBeHidden()
  await page.keyboard.press('Shift+F10')
  await expect(menu).toBeVisible()
  await expect(menu.getByRole('menuitem',{name:'Rename…'})).toBeFocused()
  await page.keyboard.press('Enter')
  const rename = page.getByRole('dialog',{name:'Rename session'})
  await expect(rename).toBeVisible()
  await rename.getByLabel('Session name').fill('Split the store layer')
  await rename.getByRole('button',{name:'Save name'}).click()
  await expect(rows(page).filter({hasText:'Split the store layer'})).toHaveCount(1)
  expect(box.writes.at(-1)).toEqual({kind:'rename',id:'s1',body:{title:'Split the store layer'}})
  // The ⋮ button opens it too; End session asks first.
  await rows(page).filter({hasText:'Add browse-v1 protocol'}).getByRole('button',{name:'Actions for Add browse-v1 protocol'}).click()
  await page.getByRole('menuitem',{name:'End session…'}).click()
  const end = page.getByRole('dialog',{name:'End Add browse-v1 protocol?'})
  await expect(end).toBeVisible()
  await end.getByRole('button',{name:'End session'}).click()
  await expect(rows(page).filter({hasText:'Add browse-v1 protocol'}).locator('.word')).toHaveText('Ending')
  expect(box.writes.at(-1)).toMatchObject({kind:'end',id:'s2',body:{removeHistory:false}})
  // An ended session: Remove from history (with a confirmation) instead of End, and its worktree can be checked.
  await rows(page).filter({hasText:'Shell'}).locator('a.open').click({button:'right'})
  const ended = page.getByRole('menu',{name:'Actions for Shell'})
  await expect(ended.getByRole('menuitem')).toHaveText(['Rename…','Open in theatre','Copy branch','Remove from history…','Check and remove worktree…'])
  await expect(ended.getByRole('menuitem',{name:'Check and remove worktree…'})).toBeEnabled()
  await ended.getByRole('menuitem',{name:'Remove from history…'}).click()
  await page.getByRole('dialog',{name:'Remove Shell from history?'}).getByRole('button',{name:'Remove from history'}).click()
  await expect(rows(page).filter({hasText:'Shell'})).toHaveCount(0)
  expect(box.writes.at(-1)).toMatchObject({kind:'archive',id:'s6'})
  // A plain session without a branch has no Copy branch.
  await rows(page).filter({hasText:'Docs pass'}).locator('a.open').click({button:'right'})
  await expect(page.getByRole('menu',{name:'Actions for Docs pass'}).getByRole('menuitem',{name:'Copy branch'})).toHaveCount(0)
  await page.keyboard.press('Escape')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('project cards have a right-click menu')) {
  const {context,page,errors} = await open()
  await page.goto(`${base}/sessions`)
  const card = page.getByRole('link',{name:/^reet,/})
  await card.click({button:'right'})
  const menu = page.getByRole('menu',{name:'Actions for reet'})
  await expect(menu).toBeVisible()
  await expect(menu.getByRole('menuitem')).toHaveText(['Open on Home','New session here','Worktrees…'])
  await expect(menu.getByRole('menuitem',{name:'Open on Home'})).toHaveAttribute('href','/?project=p1')
  await expect(menu.getByRole('menuitem',{name:'New session here'})).toHaveAttribute('href','/?project=p1#launch')
  await expect(menu.getByRole('menuitem',{name:'Open on Home'})).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(menu).toBeHidden()
  await expect(card).toBeFocused()
  // Shift+F10 opens the same menu, and Worktrees opens the existing sheet for the folder.
  await card.dispatchEvent('keydown',{key:'F10',shiftKey:true})
  await page.getByRole('menuitem',{name:'Worktrees…'}).click()
  const worktrees = page.getByRole('dialog',{name:'Worktrees for reet'})
  await expect(worktrees).toBeVisible()
  await expect(worktrees).toContainText('In use by Refactor the store layer')
  await worktrees.getByRole('button',{name:'Close',exact:true}).click()
  await expect(worktrees).toBeHidden()
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('a long session title wraps in the remove confirmation')) {
  // A shell command can be one unbroken link: the confirmation must wrap it instead of scrolling sideways.
  const title = 'npx skills use "https://github.com/emilkowalski/skills/blob/main/very-long-unbroken-path-0123456789abcdef" --skill find-animation-opportunities'
  const {context,page,errors} = await open({sessions:sessions().map(s => s.id === 's6' ? {...s, title} : s)})
  await page.goto(`${base}/sessions`)
  await rows(page).filter({hasText:'find-animation-opportunities'}).locator('a.open').click({button:'right'})
  await page.getByRole('menuitem',{name:'Remove from history…'}).click()
  const dialog = page.getByRole('dialog',{name:`Remove ${title} from history?`})
  await expect(dialog).toBeVisible()
  expect(await dialog.evaluate(d => {
   const heading = d.querySelector('h2')
   return d.scrollWidth <= d.clientWidth + 1 && !!heading && heading.scrollWidth <= heading.clientWidth + 1
  })).toBe(true)
  await page.keyboard.press('Escape')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('offline machine group sentence')) {
  const {context,page,errors} = await open()
  await page.goto(`${base}/sessions`)
  const notes = page.locator('main .group[aria-label="notes"]')
  await expect(notes.locator('.offline')).toHaveText('desk is offline')
  await expect(page.locator('main .group .offline')).toHaveCount(1)
  // Its sessions stay listed; what needs the machine is off.
  await expect(notes.locator('.session-row')).toHaveCount(1)
  await notes.locator('a.open').click({button:'right'})
  const menu = page.getByRole('menu',{name:'Actions for Review code'})
  await expect(menu.getByRole('menuitem',{name:'Open in theatre'})).toHaveAttribute('aria-disabled','true')
  await page.keyboard.press('Escape')
  await page.getByRole('radiogroup',{name:'Group by'}).getByText('Machine',{exact:true}).click()
  await expect(page.locator('main .group[aria-label="desk"] .offline')).toHaveText('desk is offline')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('rows at least 44 px on touch')) {
  for (const [width,height] of [[390,844],[834,1112]]) {
   const {context,page,errors} = await open({width,height,touch:true})
   await page.goto(`${base}/sessions`)
   await expect(rows(page)).toHaveCount(6)
   const sizes = await page.locator('main .session-row a.open, main .session-row button, main .group-by label, main label.search, main button, main summary, main .project-card').evaluateAll(els => els.filter(e => e.getClientRects().length).map(e => [e.textContent.trim() || e.getAttribute('aria-label') || e.tagName, e.getBoundingClientRect().height]))
   for (const [name,h] of sizes) expect(h, `${name} at ${width}px`).toBeGreaterThanOrEqual(44)
   // Each row's ⋮ button is a 44 × 44 px target too.
   for (const w of await page.locator('main .session-row button.more').evaluateAll(els => els.map(e => e.getBoundingClientRect().width))) expect(w, `⋮ width at ${width}px`).toBeGreaterThanOrEqual(44)
   expect(await page.locator('main input[type=search]').evaluate(el => getComputedStyle(el).fontSize)).toBe('16px')
   expect(await noSideScroll(page), `no sideways scroll at ${width}px`).toBe(true)
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // A refresh that fails keeps the last list and says so, with Try again; the list comes back when the read does.
 if (run('a failed refresh keeps the list')) {
  const {context,page,errors,box} = await open()
  await page.goto(`${base}/sessions`)
  await expect(rows(page)).toHaveCount(6)
  box.fail = true
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByText("Couldn't refresh. Showing what we last saw.",{exact:true})).toBeVisible()
  await expect(rows(page)).toHaveCount(6)
  box.fail = false
  await page.getByRole('button',{name:'Try again'}).click()
  await expect(page.getByText("Couldn't refresh. Showing what we last saw.",{exact:true})).toBeHidden()
  expect(errors).toEqual([])
  await context.close()
 }

 // Empty states: no projects, no sessions, no machines.
 if (run('empty states')) {
  let {context,page} = await open({projects:[],sessions:[]})
  await page.goto(`${base}/sessions`)
  await expect(page.getByText('Add a project to start an agent in one of your folders.',{exact:true})).toBeVisible()
  await expect(page.getByText('No sessions yet. Start one from Home.',{exact:true})).toBeVisible()
  await context.close()
  ;({context,page} = await open({projects:[],sessions:[],machines:[]}))
  await page.goto(`${base}/sessions`)
  await expect(page.getByRole('link',{name:'Connect a machine'})).toHaveAttribute('href','/machines#add-machine')
  await context.close()
 }

 // The sessions panel's colours show on the rows (with the colour's name, not colour alone) and can be filtered on.
 if (run('session colours')) {
  const {context,page,errors} = await open({colours:{s3:'red',s5:'violet'}})
  await page.goto(`${base}/sessions`)
  const row = name => rows(page).filter({hasText:name})
  // The panel's own choice, else the project's colour (reet is green); none draws nothing.
  await expect(row('Docs pass').locator('.colour-name')).toHaveText('Red')
  await expect(row('Refactor the store layer').locator('.colour-name')).toHaveText('Green')
  await expect(row('Review code').locator('.colour-name')).toHaveText('Violet')
  await expect(row('Shell').locator('.colour-name')).toHaveCount(0)
  // The strip is the colour itself: red's swatch on a red row, nothing on a row with no colour.
  const strip = name => row(name).evaluate(el => { const c = getComputedStyle(el); return c.borderLeftWidth !== '0px' && c.borderLeftStyle !== 'none' ? c.borderLeftColor : 'none' })
  expect(await strip('Docs pass')).toBe('rgb(217, 87, 87)')
  expect(await strip('Review code')).toBe('rgb(161, 126, 209)')
  expect(await strip('Shell')).toBe('rgba(0, 0, 0, 0)')
  await page.getByText('Filters',{exact:true}).click()
  await filters(page).getByLabel('Colour',{exact:true}).selectOption('red')
  await expect.poll(() => titles(page)).toEqual(['Docs pass'])
  await filters(page).getByLabel('Colour',{exact:true}).selectOption('none')
  await expect.poll(() => titles(page)).toEqual(['Shell'])
  expect(errors).toEqual([])
  await context.close()
 }

 // Saved commands from the header, with no session needed (owner ruling): open, add one, close; focus goes back.
 if (run('saved commands without a session')) {
  const {context,page,errors,box} = await open({sessions:[]})
  await page.goto(`${base}/sessions`)
  await expect(page.getByText('No sessions yet. Start one from Home.',{exact:true})).toBeVisible()
  const button = page.getByRole('button',{name:'Saved commands',exact:true})
  await button.click()
  const library = page.getByRole('dialog',{name:'Saved commands'})
  await expect(library).toBeVisible()
  await library.getByRole('button',{name:'Save your first command'}).click()
  await page.getByLabel('Command name').fill('Disk check')
  await page.getByLabel('Saved command text').fill('df -h')
  await page.getByRole('button',{name:'Save command',exact:true}).click()
  await expect(page.getByRole('dialog')).toContainText('Saved to your account.')
  expect(box.commands.map(c => c.command)).toEqual(['df -h'])
  await page.getByRole('button',{name:/^Close /}).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(button).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
 }

 // "· last known" on an offline machine's row: drawn on a tablet and desktop, visually hidden (clipped, still read out) on a
 // phone, as the mock-up's phone row has the word alone.
 if (run('last known on a phone')) {
  for (const [width,height,touch,hidden] of [[390,844,true,true],[600,900,true,true],[834,1112,true,false],[1440,900,false,false]]) {
   const {context,page,errors} = await open({width,height,touch})
   await page.goto(`${base}/sessions`)
   const word = rows(page).filter({hasText:'Review code'}).locator('.word')
   await expect(word).toHaveText('Running · last known')
   const box = await word.locator('.lk').evaluate(el => { const c = getComputedStyle(el), r = el.getBoundingClientRect(); return {display:c.display, position:c.position, clip:c.clipPath, w:r.width, h:r.height} })
   expect(box.display, `display at ${width}px`).not.toBe('none')
   if (hidden) { expect(box.position, `${width}px`).toBe('absolute'); expect(box.clip, `${width}px`).toBe('inset(50%)'); expect(box.w).toBeLessThanOrEqual(1) }
   else { expect(box.clip, `${width}px`).toBe('none'); expect(box.w, `${width}px`).toBeGreaterThan(20) }
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // The fix round's smaller points.
 if (run('menu focus, checking and words')) {
  const {context,page,errors,box} = await open()
  await page.goto(`${base}/sessions`)
  // Escape after the ⋮ button gives focus back to that button; Shift+F10 on it opens reet's menu.
  const more = page.getByRole('button',{name:'Actions for Docs pass'})
  await more.click()
  await expect(page.getByRole('menu',{name:'Actions for Docs pass'})).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(more).toBeFocused()
  await more.dispatchEvent('keydown',{key:'F10',shiftKey:true})
  await expect(page.getByRole('menu',{name:'Actions for Docs pass'})).toBeVisible()
  await page.keyboard.press('Escape')
  await page.keyboard.press('Shift+F10')
  await expect(page.getByRole('menu',{name:'Actions for Docs pass'})).toBeVisible()
  await page.keyboard.press('Escape')
  // An offline row reads "Running · last known", with its space.
  expect(await rows(page).filter({hasText:'Review code'}).locator('.word').textContent()).toBe('Running · last known')
  // A later read shows "Checking…" over the last known list.
  // The rows never move while it shows (a tap aimed at a row must land on it): its line is reserved.
  const rowTop = () => rows(page).first().evaluate(el => el.getBoundingClientRect().top)
  const before = await rowTop()
  let release
  box.hold = new Promise(r => { release = r })
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByText('Checking…',{exact:true})).toBeVisible()
  await expect(rows(page)).toHaveCount(6)
  expect(Math.abs(await rowTop() - before), 'rows still while checking').toBeLessThanOrEqual(1)
  box.hold = null; release()
  await expect(page.getByText('Checking…',{exact:true})).toBeHidden()
  expect(Math.abs(await rowTop() - before), 'rows still after checking').toBeLessThanOrEqual(1)
  // Needs you: nothing raises it yet, and the page says so.
  await page.getByText('Filters',{exact:true}).click()
  await filters(page).getByLabel('State',{exact:true}).selectOption('needs')
  await expect(page.getByText('Nothing marks a session as needing you yet, so none is listed here.',{exact:true})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // A viewer at /sessions gets the Machines page under its own name.
 if (run('viewer at /sessions')) {
  const opened = await openWorkspace(browser, {width:1440, status:{user:{id:2,name:'Pat',email:'pat@example.test',role:'viewer'}}})
  await opened.page.goto(`${base}/sessions`)
  await expect(opened.page.getByRole('heading',{level:1})).toHaveText('Your machines, connected.')
  expect(await opened.page.title()).toBe('Machines · reet.sh')
  await expect(opened.page.getByRole('navigation',{name:'Main navigation'}).getByRole('link',{name:'Machines',exact:true})).toHaveAttribute('aria-current','page')
  // A session's own address too: the Machines page with its heading, never a headerless page called Sessions.
  await opened.page.goto(`${base}/sessions?agent=a1&session=s1`)
  await expect(opened.page.getByRole('heading',{level:1})).toHaveText('Your machines, connected.')
  expect(await opened.page.title()).toBe('Machines · reet.sh')
  await expect(opened.page.getByRole('navigation',{name:'Main navigation'}).getByRole('link',{name:'Machines',exact:true})).toHaveAttribute('aria-current','page')
  await expect(opened.page.getByRole('region',{name:'Remote terminal workspace'})).toHaveCount(0)
  await opened.context.close()
 }

 // ---- The session screen (/sessions?agent=…&session=…): tabs, header, the terminal, and the context panel. ----
 const screen = page => page.locator('.session-screen')
 const tabs = page => page.getByRole('navigation',{name:'Open sessions'})
 const tabTitles = async page => (await tabs(page).locator('a.tab .name').allInnerTexts()).map(t => t.trim())
 const sidebarRow = (page, title) => page.getByRole('navigation',{name:'Terminal sessions'}).getByRole('button',{name:new RegExp(`^${title}`)}).first()
 const panel = page => page.getByRole('complementary',{name:'Session context'})
 const fact = (page, name) => panel(page).locator('.where div').filter({has:page.locator('dt',{hasText:new RegExp(`^${name}$`)})}).locator('dd')

 if (run('opening a row adds a tab; closing a tab never ends the session')) {
  const {context,page,errors,box} = await open()
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  await expect(screen(page).getByRole('heading',{name:'Add browse-v1 protocol',level:2})).toBeVisible()
  expect(await tabTitles(page)).toEqual(['Add browse-v1 protocol'])
  await expect(tabs(page).getByRole('link',{name:/Add browse-v1 protocol/})).toHaveAttribute('aria-current','page')
  // Opening another session from the panel adds its tab and raises it; the page is not reloaded.
  await page.evaluate(() => { window.__same = true })
  await sidebarRow(page, 'Refactor the store layer').click()
  await expect(screen(page).getByRole('heading',{name:'Refactor the store layer',level:2})).toBeVisible()
  expect(await tabTitles(page)).toEqual(['Add browse-v1 protocol','Refactor the store layer'])
  await expect(tabs(page).getByRole('link',{name:/Refactor the store layer/})).toHaveAttribute('aria-current','page')
  // A tab is a link to its session; choosing it switches in place.
  await expect(tabs(page).getByRole('link',{name:/Add browse-v1 protocol/})).toHaveAttribute('href','/sessions?agent=a1&session=s2')
  await tabs(page).getByRole('link',{name:/Add browse-v1 protocol/}).click()
  await expect(screen(page).getByRole('heading',{name:'Add browse-v1 protocol',level:2})).toBeVisible()
  expect(await page.evaluate(() => window.__same)).toBe(true)
  // Closing a tab closes the view only: the session keeps running and nothing is sent to the machine.
  await tabs(page).getByRole('button',{name:'Close Refactor the store layer'}).click()
  expect(await tabTitles(page)).toEqual(['Add browse-v1 protocol'])
  await expect(sidebarRow(page, 'Refactor the store layer')).toBeVisible()
  // Middle-click closes the current tab too; the next tab is shown in its place.
  await sidebarRow(page, 'Speed up the runner pool').click()
  await expect(screen(page).getByRole('heading',{name:'Speed up the runner pool',level:2})).toBeVisible()
  await tabs(page).getByRole('link',{name:/Speed up the runner pool/}).click({button:'middle'})
  expect(await tabTitles(page)).toEqual(['Add browse-v1 protocol'])
  await expect(screen(page).getByRole('heading',{name:'Add browse-v1 protocol',level:2})).toBeVisible()
  expect(box.writes.filter(w => w.kind === 'end' || w.kind === 'archive')).toEqual([])
  expect(box.sessions.every(s => s.state !== 'end_requested')).toBe(true)
  // + goes to Home's launch bar.
  await expect(tabs(page).getByRole('link',{name:'New session'})).toHaveAttribute('href','/#launch')
  // Tabs are a view held in memory: nothing in browser storage, only ids in the address.
  expect(await page.evaluate(() => sessionStorage.length)).toBe(0)
  const stored = await page.evaluate(() => Object.entries(localStorage).map(([k,v]) => k + '=' + v).join('\n'))
  for (const word of ['s1','s2','s4','Refactor','browse','tabs']) expect(stored).not.toContain(word)
  expect(new URL(page.url()).searchParams.get('session')).toBe('s2')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('ninth tab drops the oldest')) {
  const nine = Array.from({length:9}, (_, i) => session(`n${i+1}`,'a1',`Task ${i+1}`,(i+1)*MIN))
  const {context,page,errors} = await open({sessions:nine})
  await page.goto(`${base}/sessions?agent=a1&session=n1`)
  await expect(screen(page).getByRole('heading',{name:'Task 1',level:2})).toBeVisible()
  for (let i = 2; i <= 9; i++) {
   await sidebarRow(page, `Task ${i}`).click()
   await expect(screen(page).getByRole('heading',{name:`Task ${i}`,level:2})).toBeVisible()
  }
  expect(await tabTitles(page)).toEqual(['Task 2','Task 3','Task 4','Task 5','Task 6','Task 7','Task 8','Task 9'])
  // Opening one that already has a tab raises it; it is not added twice.
  await sidebarRow(page, 'Task 4').click()
  expect(await tabTitles(page)).toHaveLength(8)
  await expect(tabs(page).getByRole('link',{name:/Task 4/})).toHaveAttribute('aria-current','page')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('context panel shows machine, project, branch, worktree')) {
  const {context,page,errors} = await open()
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  await expect(panel(page)).toBeVisible()
  await expect(panel(page).getByRole('heading',{name:"Where it's working"})).toBeVisible()
  await expect(fact(page,'Machine')).toHaveText(/^studio\s*●\s*online$/)
  await expect(fact(page,'Project')).toHaveText('reet')
  await expect(fact(page,'Branch')).toHaveText('reet/reet-0f1e2d3c4b5a6978')
  await expect(fact(page,'Worktree')).toHaveText('own copy')
  // Skills on this machine: the fleet card's line, and Manage opens that machine's skills.
  await expect(panel(page).locator('.skills-line')).toHaveText(/skills in sync · checked 3 min ago/)
  await expect(panel(page).getByRole('link',{name:'Manage skills on studio'})).toHaveAttribute('href','/machines?view=compare&machine=studio')
  // A session in the project's own checkout, on another machine, with no branch recorded.
  await sidebarRow(page, 'Speed up the runner pool').click()
  await expect(fact(page,'Machine')).toHaveText(/^server\s*●\s*online$/)
  await expect(fact(page,'Project')).toHaveText('site')
  await expect(fact(page,'Branch')).toHaveText('Not recorded')
  await expect(fact(page,'Worktree')).toHaveText('the checkout')
  await expect(panel(page).locator('.skills-line')).toHaveText('Not checked')
  // An offline machine is named so, never as reporting.
  await sidebarRow(page, 'Review code').click()
  await expect(fact(page,'Machine')).toHaveText(/^desk\s*●\s*offline$/)
  await expect(panel(page)).not.toContainText(/verified|healthy/i)
  // Under 1100 px the panel waits behind Details (a 44 px target).
  await page.setViewportSize({width:834,height:1112})
  await expect(panel(page)).toBeHidden()
  const details = screen(page).getByRole('button',{name:'Details',exact:true})
  await expect(details).toHaveAttribute('aria-expanded','false')
  expect((await details.boundingBox()).height).toBeGreaterThanOrEqual(44)
  await details.click()
  await expect(panel(page)).toBeVisible()
  await expect(details).toHaveAttribute('aria-expanded','true')
  await page.keyboard.press('Escape')
  await expect(panel(page)).toBeHidden()
  await expect(details).toBeFocused()
  // Over the terminal the panel has its own Close details (it covers the header's Details).
  await details.click()
  await panel(page).getByRole('button',{name:'Close details'}).click()
  await expect(panel(page)).toBeHidden()
  await expect(details).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('changed files fetched once and on refresh')) {
  const {context,page,errors,box} = await open()
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  const files = panel(page).getByRole('region',{name:'Changed files'})
  await expect(files.locator('li')).toHaveCount(20)
  await expect(files.locator('li').first()).toHaveText('internal/terminalproto/browse.go')
  await expect(files.locator('.total')).toHaveText('23')
  await expect(files).toContainText('and 3 more not listed.')
  expect(box.status).toEqual([{agentId:'a1',project:'reet',repo:'reet',run:'0f1e2d3c4b5a6978'}])
  // The workspace reads the sessions again (as it does every 15 seconds); the changed files are not read again.
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await page.waitForTimeout(400)
  expect(box.status).toHaveLength(1)
  await files.getByRole('button',{name:'Refresh changed files'}).click()
  await expect.poll(() => box.status.length).toBe(2)
  // A session outside a worktree lists nothing and reads nothing.
  await sidebarRow(page, 'Speed up the runner pool').click()
  await expect(panel(page).getByRole('region',{name:'Changed files'})).toContainText('Only sessions in their own worktree list changed files.')
  await page.waitForTimeout(300)
  expect(box.status).toHaveLength(2)
  expect(errors).toEqual([])
  await context.close()
 }

 // The listing names a session's worktree only later (an older listing, or the connector catching up): the panel follows it,
 // reading the changed files once when the worktree can first be named, and not again on later listings.
 if (run('changed files follow a worktree named later')) {
  const early = sessions().map(x => x.id === 's2' ? {...x, worktreeRepo:'', worktreeBranch:''} : x)
  const {context,page,errors,box} = await open({sessions:early})
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  const files = panel(page).getByRole('region',{name:'Changed files'})
  await expect(files).toContainText("reet can't name this worktree")
  expect(box.status).toHaveLength(0)
  box.sessions = sessions()
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await expect(files.locator('li')).toHaveCount(20)
  expect(box.status).toEqual([{agentId:'a1',project:'reet',repo:'reet',run:'0f1e2d3c4b5a6978'}])
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await page.waitForTimeout(400)
  expect(box.status).toHaveLength(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // A tab's close button says what closing it does: a live session keeps running, a finished one stays in its history.
 if (run('close tooltips follow the state')) {
  const {context,page,errors} = await open()
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  await sidebarRow(page, 'Docs pass').click()
  await expect(tabs(page).getByRole('button',{name:'Close Add browse-v1 protocol'})).toHaveAttribute('title','Close the tab (the session keeps running)')
  await expect(tabs(page).getByRole('button',{name:'Close Docs pass'})).toHaveAttribute('title','Close the tab (the session stays in its history)')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('Chat is disabled with its reason')) {
  const {context,page,errors} = await open()
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  const view = screen(page).getByRole('group',{name:'View'})
  const chat = view.getByRole('button',{name:'reet TUI'})
  await expect(chat).toHaveAttribute('aria-disabled','true')
  await expect(chat).toHaveAccessibleDescription('The reet TUI is for a structured OpenCode session.')
  await expect(view.getByRole('button',{name:'Claude Code TUI'})).toHaveAttribute('aria-pressed','true')
  // Choosing it changes nothing (it is aria-disabled, so it stays in the tab order with its reason).
  await chat.click({force:true})
  await expect(view.getByRole('button',{name:'Claude Code TUI'})).toHaveAttribute('aria-pressed','true')
  await expect(page.locator('.pane')).toBeVisible()
  // The header: agent · model · started, the state in words with its source, and no step count or progress bar (R10).
  await expect(screen(page).locator('.sub')).toHaveText('Claude Code · sonnet · started 2 hours ago')
  const pill = screen(page).locator('.pill')
  await expect(pill).toContainText('Running')
  await expect(pill).toHaveAttribute('title',"From the session's state")
  await expect(screen(page).locator('progress, [role=progressbar]')).toHaveCount(0)
  await expect(screen(page)).not.toContainText(/Step \d/)
  await expect(screen(page).locator('.approvals')).toHaveText('Approvals: Ask me')
  await expect(panel(page).locator('.decisions')).toHaveText('Asked in the terminal.')
  // The actions menu: Rename, Theatre view, Copy branch, End session, Check and remove worktree (waits for the end).
  await screen(page).getByRole('button',{name:'Session actions'}).click()
  const menu = page.getByRole('menu',{name:'Actions for Add browse-v1 protocol'})
  await expect(menu.getByRole('menuitem')).toHaveText(['Open a terminal here…','Open a terminal beside this session…','Rename…','Theatre view','Copy branch','End session…','Check and remove worktree…'])
  await expect(menu.getByRole('menuitem',{name:'Open a terminal here…'})).toBeFocused()
  await expect(menu.getByRole('menuitem',{name:'Check and remove worktree…'})).toBeDisabled()
  await page.keyboard.press('Escape')
  await expect(menu).toBeHidden()
  await expect(screen(page).getByRole('button',{name:'Session actions'})).toBeFocused()
  await screen(page).getByRole('button',{name:'Session actions'}).click()
  await menu.getByRole('menuitem',{name:'Rename…'}).click()
  await expect(page.getByRole('dialog',{name:'Rename session'})).toBeVisible()
  await page.keyboard.press('Escape')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('YOLO session shows its decisions line')) {
  const {context,page,errors} = await open()
  await page.goto(`${base}/sessions?agent=a2&session=s4`)
  await expect(screen(page).getByRole('heading',{name:'Speed up the runner pool',level:2})).toBeVisible()
  await expect(screen(page).locator('.head .badge.yolo')).toHaveText('YOLO')
  await expect(panel(page).locator('.decisions')).toContainText('YOLO: the agent asked not to be prompted; the launch is audited.')
  await expect(panel(page).locator('.decisions').getByRole('link')).toHaveAttribute('href','/activity')
  await expect(screen(page).locator('.approvals')).toHaveText('Approvals: YOLO')
  // A session that is not YOLO says the terminal asks.
  await sidebarRow(page, 'Add browse-v1 protocol').click()
  await expect(screen(page).locator('.head .badge.yolo')).toHaveCount(0)
  await expect(panel(page).locator('.decisions')).toHaveText('Asked in the terminal.')
  expect(errors).toEqual([])
  await context.close()
 }

 // Fix round 1: the context panel by keyboard alone, docked and as a sheet; the focused control is never covered.
 const focusedUncovered = page => page.evaluate(() => {
  const el = document.activeElement, r = el.getBoundingClientRect()
  const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)
  return !!hit && (hit === el || el.contains(hit))
 })
 const focusedName = page => page.evaluate(() => { const el = document.activeElement; return (el.getAttribute('aria-label') || el.textContent || '').trim() })
 // Tab until the focused control is named `name` (at most `limit` presses), checking it is never covered.
 async function tabTo(page, name, limit = 12) {
  for (let i = 0; i < limit; i++) {
   await page.keyboard.press('Tab')
   expect(await focusedUncovered(page), `focus on ${await focusedName(page)} is covered`).toBe(true)
   if (process.env.REET_TRACE) console.log("tab:", await focusedName(page))
   if (await focusedName(page) === name) return
  }
  throw new Error(`Tab never reached ${name}; ended on ${await focusedName(page)}`)
 }

 if (run('context panel by keyboard')) {
  // Docked at 1440: header, then the panel, then the terminal.
  let {context,page,errors} = await open()
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
  await screen(page).getByRole('button',{name:'Session actions'}).focus()
  await tabTo(page, 'Refresh changed files')
  await tabTo(page, 'Manage skills on studio')
  await sidebarRow(page, 'Speed up the runner pool').click()
  // The new terminal takes focus once it is live; start from the header after that.
  await expect(page.locator('.pane')).toHaveAttribute('aria-label',/^Speed up the runner pool/)
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
  await screen(page).getByRole('button',{name:'Session actions'}).focus()
  await tabTo(page, 'Activity')
  expect(errors).toEqual([])
  await context.close()
  // As a sheet at 834 and 390: Details moves focus into it, Tab stays inside, Escape and Close details give it back.
  for (const [width,height] of [[834,1112],[390,844]]) {
   ;({context,page,errors} = await open({width,height,touch:true}))
   await page.goto(`${base}/sessions?agent=a1&session=s2`)
   await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
   const details = screen(page).getByRole('button',{name:'Details',exact:true})
   await details.focus()
   await page.keyboard.press('Enter')
   await expect(panel(page)).toBeVisible()
   await expect(panel(page).getByRole('button',{name:'Close details'})).toBeFocused()
   expect(await focusedUncovered(page), `${width}px`).toBe(true)
   // The sheet never lies over the header's Details and ⋮.
   for (const name of ['Details','Session actions']) {
    const b = await screen(page).getByRole('button',{name,exact:true}).boundingBox()
    expect(await page.evaluate(([x,y]) => !document.elementFromPoint(x,y)?.closest('.context-panel'), [b.x+b.width/2, b.y+b.height/2]), `${name} covered at ${width}px`).toBe(true)
   }
   await tabTo(page, 'Refresh changed files')
   await tabTo(page, 'Manage skills on studio')
   // Tab keeps going round the sheet: it never reaches the terminal behind it.
   for (let i = 0; i < 12; i++) { await page.keyboard.press('Tab'); expect(await page.evaluate(() => !!document.activeElement.closest('.context-panel')), `Tab left the sheet at ${width}px`).toBe(true) }
   await page.keyboard.press('Shift+Tab')
   expect(await page.evaluate(() => !!document.activeElement.closest('.context-panel'))).toBe(true)
   await page.keyboard.press('Escape')
   await expect(panel(page)).toBeHidden()
   await expect(details).toBeFocused()
   await page.keyboard.press('Enter')
   await expect(panel(page).getByRole('button',{name:'Close details'})).toBeFocused()
   await page.keyboard.press('Enter')
   await expect(panel(page)).toBeHidden()
   await expect(details).toBeFocused()
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // Fix round 2: a reconnect while the sheet is open does not pull focus into the terminal behind it.
 if (run('a reconnect keeps focus in the sheet')) {
  const {context,page,errors,box} = await open({width:834,height:1112,touch:true})
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  const pane = page.locator('.pane')
  await expect(pane).toHaveAttribute('data-connection-state','live')
  const details = screen(page).getByRole('button',{name:'Details',exact:true})
  await details.focus()
  await page.keyboard.press('Enter')
  const close = panel(page).getByRole('button',{name:'Close details'})
  await expect(close).toBeFocused()
  // The machine drops the connection; the pane reconnects and is live again.
  const before = box.sockets.length
  await box.sockets.at(-1).close()
  await expect(pane).not.toHaveAttribute('data-connection-state','live')
  await expect(pane).toHaveAttribute('data-connection-state','live',{timeout:10000})
  expect(box.sockets.length).toBeGreaterThan(before)
  await page.waitForTimeout(300)
  await expect(close).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(panel(page)).toBeHidden()
  await expect(details).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
 }

 // Tab out of an actions menu goes on from what opened it (the menus are drawn after the page, so the browser's own Tab
 // would drop focus at the page's end or start); Shift+Tab goes back to the opener.
 if (run('Tab from a menu keeps its place')) {
  const {context,page,errors} = await open()
  const follows = (opener) => opener.evaluate(el => { const a = document.activeElement; return a !== document.body && !!(el.compareDocumentPosition(a) & Node.DOCUMENT_POSITION_FOLLOWING) && !a.closest('footer') && !a.closest('[role=menu]') })
  await page.goto(`${base}/sessions`)
  const more = page.getByRole('button',{name:'Actions for Docs pass'})
  await more.click()
  await expect(page.getByRole('menu',{name:'Actions for Docs pass'})).toBeVisible()
  await page.keyboard.press('Tab')
  await expect(page.getByRole('menu')).toHaveCount(0)
  expect(await follows(more), 'Sessions page: Tab goes on after the ⋮ button').toBe(true)
  await more.click()
  await page.keyboard.press('Shift+Tab')
  await expect(more).toBeFocused()
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
  const actions = screen(page).getByRole('button',{name:'Session actions'})
  await actions.click()
  await expect(page.getByRole('menu',{name:/^Actions for /})).toBeVisible()
  await page.keyboard.press('Tab')
  await expect(page.getByRole('menu',{name:/^Actions for /})).toHaveCount(0)
  expect(await follows(actions), 'session screen: Tab goes on after ⋮').toBe(true)
  await actions.click()
  await page.keyboard.press('Shift+Tab')
  await expect(actions).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
 }

 // A reconnect while the session's ⋮ menu is open leaves focus in the menu.
 if (run('a reconnect keeps focus in the menu')) {
  const {context,page,errors,box} = await open()
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  const pane = page.locator('.pane')
  await expect(pane).toHaveAttribute('data-connection-state','live')
  await screen(page).getByRole('button',{name:'Session actions'}).click()
  const first = page.getByRole('menu',{name:/^Actions for /}).getByRole('menuitem').first()
  await expect(first).toBeFocused()
  await box.sockets.at(-1).close()
  await expect(pane).not.toHaveAttribute('data-connection-state','live')
  await expect(pane).toHaveAttribute('data-connection-state','live',{timeout:10000})
  await page.waitForTimeout(300)
  await expect(first).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('an empty listing for a moment keeps the tabs')) {
  const {context,page,errors,box} = await open()
  await page.goto(`${base}/sessions?agent=a1&session=s1`)
  await sidebarRow(page, 'Add browse-v1 protocol').click()
  await sidebarRow(page, 'Refactor the store layer').click()
  expect(await tabTitles(page)).toEqual(['Refactor the store layer','Add browse-v1 protocol'])
  const all = box.sessions
  box.sessions = []
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await expect(screen(page)).toHaveCount(0)
  box.sessions = all
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await expect.poll(() => tabTitles(page)).toEqual(['Refactor the store layer','Add browse-v1 protocol'])
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('a tab whose session left the listing')) {
  const {context,page,errors,box} = await open()
  await page.goto(`${base}/sessions?agent=a1&session=s1`)
  await sidebarRow(page, 'Add browse-v1 protocol').click()
  await sidebarRow(page, 'Speed up the runner pool').click()
  expect(await tabTitles(page)).toEqual(['Refactor the store layer','Add browse-v1 protocol','Speed up the runner pool'])
  // s2 is removed elsewhere; the next read drops its tab, and it no longer counts.
  box.sessions = box.sessions.filter(s => s.id !== 's2')
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await expect.poll(() => tabTitles(page)).toEqual(['Refactor the store layer','Speed up the runner pool'])
  await tabs(page).getByRole('button',{name:'Close Speed up the runner pool'}).click()
  await expect(screen(page).getByRole('heading',{name:'Refactor the store layer',level:2})).toBeVisible()
  expect(new URL(page.url()).searchParams.get('session')).toBe('s1')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('tab strip on touch')) {
  const many = Array.from({length:8}, (_, i) => session(`n${i+1}`,'a1',`Long task number ${i+1}`,(i+1)*MIN))
  const {context,page,errors} = await open({sessions:many, width:834, height:1112, touch:true})
  await page.goto(`${base}/sessions?agent=a1&session=n1`)
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
  for (let i = 2; i <= 8; i++) await page.evaluate(t => [...document.querySelectorAll('nav[aria-label="Terminal sessions"] button')].find(b => b.textContent.trim().startsWith(t))?.click(), `Long task number ${i}`)
  await expect(tabs(page).locator('a.tab')).toHaveCount(8)
  // Every tab link and close button is a 44 px target.
  for (const h of await tabs(page).locator('a.tab, button.close, a.new').evaluateAll(els => els.map(e => e.getBoundingClientRect().height))) expect(h).toBeGreaterThanOrEqual(44)
  // Tabs that do not fit fade at the edge that hides them, and the current tab is scrolled into view.
  const strip = tabs(page).locator('ul')
  await expect(strip).toHaveClass(/more-start|more-end/)
  const current = await tabs(page).locator('a.tab[aria-current=page]').boundingBox(), area = await strip.boundingBox()
  expect(current.x).toBeGreaterThanOrEqual(area.x - 1); expect(current.x + current.width).toBeLessThanOrEqual(area.x + area.width + 1)
  // A tab out of view comes into view when it takes focus.
  await tabs(page).locator('a.tab').first().focus()
  const first = await tabs(page).locator('a.tab').first().boundingBox()
  expect(first.x).toBeGreaterThanOrEqual(area.x - 1)
  expect(await noSideScroll(page)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
 }

 // A tablet (834 px, touch): Details opens the context panel as a side sheet along the terminal's right edge, from under the
 // header to the foot of the screen, and every target on the screen and in the sheet is at least 44 px.
 if (run('tablet details side sheet')) {
  const {context,page,errors} = await open({width:834, height:1112, touch:true, changed:3})
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
  const screen = page.locator('.session-screen')
  const targets = async scope => (await scope.evaluateAll(els => els.filter(e => e.getClientRects().length && getComputedStyle(e).visibility !== 'hidden' && !e.closest('.vh, .xterm'))
   .map(e => { const r = e.getBoundingClientRect(); return [(e.getAttribute('aria-label') || e.textContent).trim().replace(/\s+/g,' ').slice(0,30), Math.round(r.width), Math.round(r.height)] })))
  // The workspace toolbar, the tab strip and the session's header (the pane keeps its own 40 px terminal controls, as the spec allows).
  for (const [name,w,h] of await targets(page.locator('.workspace-toolbar a, .workspace-toolbar button, .session-screen .head a, .session-screen .head button')))
   expect(Math.min(w,h), `"${name}" is ${w}×${h}`).toBeGreaterThanOrEqual(44)
  await screen.getByRole('button',{name:'Details',exact:true}).click()
  const sheet = screen.locator('.context-panel')
  await expect(sheet).toBeVisible()
  const s = await sheet.boundingBox(), head = await screen.locator('.head').boundingBox(), stage = await screen.locator('.stage').boundingBox(), whole = await screen.boundingBox()
  expect(Math.round(s.width)).toBe(340)
  expect(Math.abs(s.x + s.width - (whole.x + whole.width)), 'along the right edge').toBeLessThanOrEqual(1)
  expect(s.y, 'below the header').toBeGreaterThanOrEqual(head.y + head.height - 1)
  expect(Math.abs(s.y + s.height - (stage.y + stage.height)), 'down to the foot of the screen').toBeLessThanOrEqual(1)
  for (const [name,w,h] of await targets(sheet.locator('a, button')))
   expect(Math.min(w,h), `sheet "${name}" is ${w}×${h}`).toBeGreaterThanOrEqual(44)
  expect(await noSideScroll(page)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
 }

 // A touch tablet (834 px): every control in the workspace, the docked sessions panel included, is at least 44 px; the
 // terminal's own keys (the pane's toolbar and extra keys) are the one exception, at 40 px (spec: Accessibility).
 if (run('tablet targets are 44 px')) {
  const {context,page,errors} = await open({width:834, height:1112, touch:true})
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
  await expect(page.locator('aside#terminal-sessions')).toBeVisible()
  const sizes = await page.locator('.workspace :is(a[href], button, select, input, summary, [role=button], [tabindex="0"])').evaluateAll(els => els
   // Shown controls only: not inside a closed <details> (its summary aside), not visually hidden, not the xterm's own.
   // The separator's track is 24 px; its 44 × 88 px grip is checked by hit-testing in "tablet separator never covers a control".
   .filter(e => e.getClientRects().length && getComputedStyle(e).visibility !== 'hidden' && !e.closest('.vh, .xterm, dialog') && !(e.closest('details:not([open])') && !e.matches('summary')) && !e.matches('.panel-separator'))
   .map(e => { const r = e.getBoundingClientRect(); return {name:(e.getAttribute('aria-label') || e.textContent || e.tagName).trim().replace(/\s+/g,' ').slice(0,30), w:Math.round(r.width), h:Math.round(r.height), keys:!!e.closest('.terminal-toolbar, .mobile-keys')} }))
  expect(sizes.filter(t => !t.keys).length).toBeGreaterThan(20)
  const small = sizes.filter(t => Math.min(t.w, t.h) < (t.keys ? 40 : 44)).map(t => `${t.name} ${t.w}×${t.h}`)
  expect(small, 'targets under 44 px (40 px for the terminal keys)').toEqual([])
  // A row's menu in the panel, and a project row's menu: their items and colour swatches too.
  for (const opener of ['Actions for Docs pass', 'Actions for machine studio', 'Actions for reet']) {
   await page.locator('aside#terminal-sessions').getByRole('button',{name:opener,exact:true}).click()
   const items = await page.locator('.session-context[role=menu] :is(button, a[href])').evaluateAll(els => els.filter(e => e.getClientRects().length).map(e => { const r = e.getBoundingClientRect(); return `${(e.getAttribute('aria-label') || e.textContent).trim().slice(0,24)} ${Math.round(r.width)}×${Math.round(r.height)}` }))
   expect(items.length, opener).toBeGreaterThan(1)
   expect(items.filter(t => Math.min(...t.split(' ').pop().split('×').map(Number)) < 44), `${opener} menu`).toEqual([])
   // Every colour swatch lies wholly inside its menu (they wrap rather than run past its edge).
   const outside = await page.locator('.session-context[role=menu]').evaluate(menu => { const m = menu.getBoundingClientRect(); return [...menu.querySelectorAll('.colours button')].map(b => [b.getAttribute('aria-label'), b.getBoundingClientRect()]).filter(([, r]) => r.left < m.left || r.right > m.right || r.top < m.top || r.bottom > m.bottom || Math.min(r.width, r.height) < 44).map(([n, r]) => `${n} ${Math.round(r.left)}–${Math.round(r.right)} in ${Math.round(m.left)}–${Math.round(m.right)}`) })
   expect(outside, `${opener} swatches`).toEqual([])
   await page.keyboard.press('Escape')
  }
  expect(errors).toEqual([])
  await context.close()
 }

 // The panel's separator on a touch tablet: a 44 px grab area that never covers a control beside it. Every control within
 // 24 px of it (the panel's right-hand controls, the pane's left edge, the panel's scroll bar) answers a press at its centre
 // and at each edge, not the separator.
 if (run('tablet separator never covers a control')) {
  const {context,page,errors} = await open({width:834, height:1112, touch:true})
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
  const sep = page.getByRole('separator',{name:'Resize session navigation'})
  const track = await sep.boundingBox()
  // The grip: a 44 px wide area at the middle of the track that reaches into the panel's padding, from 34 px left of the
  // track's centre to 3 px right of it, and no further than the track itself over the terminal: a press 10 px inside the
  // terminal's left edge at mid-height reaches the terminal, not the separator.
  const mid = [track.x + track.width / 2, track.y + track.height / 2]
  const role = (x, y) => page.evaluate(([x, y]) => document.elementFromPoint(x, y)?.getAttribute('role'), [x, y])
  for (const dx of [-34, -21, -11, 0, 3]) expect(await role(mid[0] + dx, mid[1]), `grip at ${dx} px`).toBe('separator')
  const paneLeft = await page.locator('.terminal-stage').evaluate(el => el.getBoundingClientRect().left)
  for (const dy of [-40, 0, 40]) expect(await role(paneLeft + 10, mid[1] + dy), `10 px into the terminal at ${dy} px`).not.toBe('separator')
  const covered = await page.evaluate(() => {
   const sep = document.querySelector('.panel-separator'), g = sep.getBoundingClientRect(), out = []
   const near = r => r.right >= g.left - 24 - 22 && r.left <= g.right + 24 + 22
   for (const el of document.querySelectorAll('.workspace :is(a[href], button, select, input, summary, [role=button], [tabindex="0"]):not(.panel-separator)')) {
    if (!el.getClientRects().length || el.closest('.xterm, .vh') || (el.closest('details:not([open])') && !el.matches('summary'))) continue
    const r = el.getBoundingClientRect()
    if (!near(r) || r.width === 0 || r.bottom < 0 || r.top > innerHeight) continue
    const pts = [[(r.left + r.right) / 2, (r.top + r.bottom) / 2], [r.left + 1, (r.top + r.bottom) / 2], [r.right - 1, (r.top + r.bottom) / 2], [(r.left + r.right) / 2, r.top + 1], [(r.left + r.right) / 2, r.bottom - 1]]
    for (const [x, y] of pts) { const hit = document.elementFromPoint(x, y); if (hit && hit !== el && !el.contains(hit)) out.push(`${(el.getAttribute('aria-label') || el.textContent).trim().slice(0, 30)} at ${Math.round(x)},${Math.round(y)} → ${hit.className || hit.tagName}`) }
   }
   return out
  })
  expect(covered, 'controls the separator covers').toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }
 // The same beside the mini rail (the panel shrunk to its 64 px rail of 44 px buttons): the grip does not reach into the
 // rail, so every rail button answers a press across its width, and the 24 px track is still there to drag.
 if (run('tablet separator by the mini rail')) {
  const {context,page,errors} = await open({width:834, height:1112, touch:true})
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
  await page.locator('aside#terminal-sessions').getByRole('button',{name:'Shrink sessions to a mini rail',exact:true}).click()
  const rail = page.getByRole('complementary',{name:'Session rail',exact:true})
  await expect(rail).toBeVisible()
  const sep = page.getByRole('separator',{name:'Resize session navigation'})
  await expect(sep).toHaveAttribute('aria-valuetext','Mini rail')
  const track = await sep.boundingBox(), mid = [track.x + track.width / 2, track.y + track.height / 2]
  const role = (x, y) => page.evaluate(([x, y]) => document.elementFromPoint(x, y)?.getAttribute('role'), [x, y])
  for (const dx of [-11, 0, 11]) expect(await role(mid[0] + dx, mid[1]), `track at ${dx} px`).toBe('separator')
  const covered = await rail.evaluate(r => {
   const out = []
   for (const el of r.querySelectorAll('button, a[href]')) {
    if (!el.getClientRects().length) continue
    const b = el.getBoundingClientRect()
    if (b.bottom < 0 || b.top > innerHeight) continue
    for (const [x, y] of [[(b.left + b.right) / 2, (b.top + b.bottom) / 2], [b.left + 1, (b.top + b.bottom) / 2], [b.right - 1, (b.top + b.bottom) / 2], [(b.left + b.right) / 2, b.top + 1], [(b.left + b.right) / 2, b.bottom - 1]]) {
     const hit = document.elementFromPoint(x, y)
     if (hit && hit !== el && !el.contains(hit)) out.push(`${(el.getAttribute('aria-label') || el.textContent).trim().slice(0, 30)} at ${Math.round(x)},${Math.round(y)} → ${hit.className || hit.tagName}`)
    }
   }
   return out
  })
  expect(covered, 'rail buttons the separator covers').toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('copy branch is announced')) {
  const {context,page,errors} = await open()
  await context.grantPermissions(['clipboard-read','clipboard-write'])
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  // The live region is there before anything is said, so the first message is announced.
  const live = screen(page).getByRole('status').first()
  await expect(live).toBeAttached()
  expect(await live.evaluate(el => getComputedStyle(el).display)).not.toBe('none')
  await screen(page).getByRole('button',{name:'Session actions'}).click()
  await page.getByRole('menuitem',{name:'Copy branch'}).click()
  await expect(live).toHaveText('Copied reet/reet-0f1e2d3c4b5a6978.')
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('a paused machine is not shown as online')) {
  const paused = {...sync(), agents:[{id:'a1',host:'studio',version:'test',lastSeen:Date.now(),paused:true,autoUpdate:false}]}
  const {context,page,errors} = await open({sync:paused})
  await page.goto(`${base}/sessions?agent=a1&session=s2`)
  await expect(fact(page,'Machine')).toHaveText(/^studio\s*●\s*paused$/)
  const state = fact(page,'Machine').locator('.state')
  await expect(state).toHaveClass(/warn/)
  await expect(state).not.toHaveClass(/good/)
  expect(errors).toEqual([])
  await context.close()
 }

 // The mock-up's sizes, light and dark, for the fidelity check (REET_UI_SCREENSHOTS=<folder>).
 if (shots && run('screens')) {
  await mkdir(shots, {recursive:true})
  for (const [width,height,touch] of [[1440,900,false],[834,1112,true],[390,844,true],[320,700,true]]) for (const theme of ['light','dark']) {
   // Three changed files, as the mock-up lists.
   const {context,page} = await open({width,height,touch,theme,changed:3})
   await page.goto(`${base}/sessions`)
   await expect(rows(page)).toHaveCount(6)
   await page.screenshot({path:`${shots}/sessions-${width}-${theme}.png`})
   // The phone rail, scrolled into view under the groups.
   if (width < 1100) { await page.evaluate(() => scrollTo(0, document.documentElement.scrollHeight)); await page.waitForTimeout(150); await page.screenshot({path:`${shots}/sessions-${width}-${theme}-rail.png`}); await page.evaluate(() => scrollTo(0, 0)) }
   // The session screen, shaped like the mock-up: four open tabs, the second raised.
   // A phone has no strip (H3 lays it out), so it opens the one session.
   await page.goto(`${base}/sessions?agent=a1&session=${width > 650 ? 's1' : 's2'}`)
   await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
   if (width > 650) {
    for (const title of ['Add browse-v1 protocol','Review code','Speed up the runner pool']) await page.evaluate(t => [...document.querySelectorAll('nav[aria-label="Terminal sessions"] button')].find(b => b.textContent.trim().startsWith(t))?.click(), title)
    await page.locator('nav[aria-label="Open sessions"] a.tab').nth(1).click()
   }
   await expect(page.locator('.session-screen h2')).toHaveText('Add browse-v1 protocol')
   await page.waitForTimeout(300)
   await page.screenshot({path:`${shots}/session-${width}-${theme}.png`})
   // REET_AXE=<path to axe.min.js> (kept outside the repository) checks the screen, with Details open where it is collapsed
   // and with the actions menu open, and prints the counts for docs/ui-verification.md.
   if (process.env.REET_AXE) {
    const audit = async state => {
     await page.addScriptTag({path:process.env.REET_AXE})
     const r = await page.evaluate(() => window.axe.run(document, {runOnly:{type:'tag',values:['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa']}}))
     console.log(`axe ${width} ${theme} ${state}: ${r.violations.length} violations ${JSON.stringify(r.violations.map(v => [v.id, v.nodes.map(n => n.target.join(' '))]))}, ${r.incomplete.length} incomplete ${JSON.stringify(r.incomplete.map(v => [v.id, v.nodes.length, process.env.REET_AXE_NODES ? v.nodes.map(n => n.target.join(' ') + ' ' + (n.any[0]?.message ?? '')) : []]))}`)
    }
    await audit('screen')
    const details = page.locator('.session-screen').getByRole('button',{name:'Details',exact:true})
    if (await details.isVisible()) { await details.click(); await page.screenshot({path:`${shots}/session-${width}-${theme}-details.png`}); await audit('details open'); await page.keyboard.press('Escape') }
    await page.locator('.session-screen').getByRole('button',{name:'Session actions'}).click()
    await audit('menu open')
    await page.keyboard.press('Escape')
   }
   await context.close()
  }
 }
 console.log('Sessions: groups shared with Home, search by branch and machine, filters together, the actions menu by right-click, Shift+F10 and its button, the offline sentence, 44 px rows on touch, a failed refresh keeping the list, the empty states, session colours, saved commands without a session, menu focus, Checking… on later reads, the Needs you sentence, a viewer at /sessions, and the session screen: tabs held in memory (eight at most, closing one never ends its session), the context panel, changed files read once and on refresh, the reet TUI disabled with its reason and the YOLO decisions line.')
} finally { await browser.close() }
