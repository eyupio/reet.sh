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

async function open(options={}) {
 const box = {sessions:options.sessions ?? sessions(), projects:options.projects ?? projects(), fail:false, writes:[], commands:[], hold:null}
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
  '/api/v2/terminal/status': {configured:true,available:true,reason:''},
 }
 const opened = await openWorkspace(browser, {width:options.width ?? 1440, height:options.height ?? 900, touch:options.touch, theme:options.theme ?? 'light', routes})
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
  let release
  box.hold = new Promise(r => { release = r })
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
  await expect(page.getByText('Checking…',{exact:true})).toBeVisible()
  await expect(rows(page)).toHaveCount(6)
  box.hold = null; release()
  await expect(page.getByText('Checking…',{exact:true})).toBeHidden()
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
  await opened.context.close()
 }

 // The mock-up's sizes, light and dark, for the fidelity check (REET_UI_SCREENSHOTS=<folder>).
 if (shots && run('screens')) {
  await mkdir(shots, {recursive:true})
  for (const [width,height,touch] of [[1440,900,false],[834,1112,true],[390,844,true]]) for (const theme of ['light','dark']) {
   const {context,page} = await open({width,height,touch,theme})
   await page.goto(`${base}/sessions`)
   await expect(rows(page)).toHaveCount(6)
   await page.screenshot({path:`${shots}/sessions-${width}-${theme}.png`})
   await context.close()
  }
 }
 console.log('Sessions: groups shared with Home, search by branch and machine, filters together, the actions menu by right-click, Shift+F10 and its button, the offline sentence, 44 px rows on touch, a failed refresh keeping the list, the empty states, session colours, saved commands without a session, menu focus, Checking… on later reads, the Needs you sentence and a viewer at /sessions.')
} finally { await browser.close() }
