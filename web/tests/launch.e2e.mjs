// The launch bar on Home: the sentence, its slots, the prompt and Start, against synthetic API answers. Every create the page
// sends is recorded, so what was sent (and that nothing was sent) is checked against what the bar says. No agent is started.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const MIN = 60_000
const caps = (extra = {}) => ({protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,
 worktree:true,agentsV2:true,branches:true,browse:true,tools:['claude','codex','gemini','cline'],...extra})
// studio is ready; desk is reconnecting (not ready), so its projects and its home folder are offered disabled with the sentence.
const tmachines = () => [
 {agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:caps()},
 {agentId:'a2',host:'desk',ready:false,serverEnabled:true,capabilities:caps()},
]
const health = () => ({serverNow:Date.now(),machines:[
 {agentId:'a1',host:'studio',state:'connected',capabilities:caps()},
 {agentId:'a2',host:'desk',state:'unreachable',capabilities:caps()},
]})
const fproject = (id, name, host, dir, over={}) => ({id,host,machine:host === 'a1' ? 'studio' : 'desk',dir,name,isRepo:true,baseBranch:'',worktreeDefault:'inherit',defaultTool:'',defaultModel:'',
 hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1,...over})
const fprojects = () => [
 fproject('fp1','Store rewrite','a1','/home/dev/store',{baseBranch:'main'}),
 fproject('fp2','Notes','a2','/home/dev/notes',{isRepo:false,position:1}),
 fproject('fp3','Site','a1','/home/dev/site',{position:2}),
]
const session = (id, agentId, title, over={}) => ({id,agentId,title,state:'running',createdAt:now-2*60*MIN,updatedAt:now-12*MIN,exitCode:null,...over})
const sync = () => ({environments:[],machines:[],runs:[],updateJobs:[],targets:[],jobs:[],
 agents:[{id:'a1',host:'studio',version:'1',lastSeen:Date.now(),paused:false,autoUpdate:false},{id:'a2',host:'desk',version:'1',lastSeen:Date.now()-2*60*MIN,paused:false,autoUpdate:false}]})
const SECRETS = ['Store rewrite','Notes','Site','fp1','fp2','fp3','/home/dev','studio','desk','a1','Fix the failing tests','gpt-5-codex','codex','develop']

// A writer's Home. `answer(body, n)` decides each create's reply (default: a running session); `sessions` is the live list.
async function launch(options={}) {
 const box = {posts:[], sessions:options.sessions ?? [], projects:options.projects ?? fprojects(), reads:[], storage:[]}
 const answer = options.answer ?? (body => session(`new-${body.requestId.slice(0,8)}`, body.agentId, body.title, {createdAt:Date.now(),updatedAt:Date.now()}))
 const routes = {
  '/api/v2/machines/health': health(),
  '/api/v2/terminal/machines': () => ({machines:tmachines()}),
  '/api/v2/terminal/sessions': async request => {
   if (request.method() !== 'POST') return {sessions:box.sessions}
   const body = request.postDataJSON()
   box.posts.push(body)
   const reply = await answer(body, box.posts.length)
   if (reply?.status) return reply
   box.sessions = [reply, ...box.sessions]
   return reply
  },
  '/api/v2/folder-projects': () => ({projects:box.projects, savedWaiting:0}),
  '/api/v2/folder-projects/fp1': () => ({...box.projects.find(p => p.id === 'fp1'), starterPrompt:'Fix the failing tests in the store package'}),
  '/api/v2/machines/a1/branches': request => {
   const u = new URL(request.url())
   box.reads.push(`branches ${u.searchParams.get('dir')}`)
   return {current:'main', default:'main', branches:['main','develop','release/1.2'], more:false}
  },
  // Answered at once: a request sent after the page's others, so a test knows they have all reached these routes.
  '/api/__e2e/sync': () => ({}),
  ...(options.routes ?? {}),
 }
 const opened = await openWorkspace(browser, {width:1440, height:900, sync:sync(), ...options, routes})
 await opened.page.exposeFunction('__recordStorage', (op, key, value) => { box.storage.push({op, key, value}) })
 await opened.page.addInitScript(() => {
  for (const area of ['localStorage','sessionStorage']) {
   const store = window[area], set = store.setItem.bind(store)
   store.setItem = (k, v) => { window.__recordStorage(`${area}.set`, k, String(v)); return set(k, v) }
  }
 })
 // The session page is not under test: it answers with an empty page.
 await opened.page.route(url => new URL(url).pathname === '/sessions', route => route.fulfill({status:200, contentType:'text/html', body:'<title>session</title>'}))
 return {...opened, box}
}
// Deterministic in place of a fixed wait: the page's pending work runs (a macrotask after Svelte's effects), then a request
// sent after any the page has made is answered, so whatever the page sent before it has been recorded by the routes.
const settled = page => page.evaluate(async () => { await new Promise(r => setTimeout(r, 0)); await fetch('/api/__e2e/sync') })
const bar = page => page.getByRole('form',{name:'Start a session'})
const prompt = page => bar(page).getByRole('textbox',{name:'Describe the job'})
const startButton = page => bar(page).getByRole('button',{name:/^(Start the session|Try the same request again)$/})
const slot = (page, name) => bar(page).getByRole('button',{name:new RegExp(`^${name}: `)})
const sentenceText = page => bar(page).locator('.sentence').evaluate(el => [...el.querySelectorAll('.part')].map(p => p.textContent.trim()).join(' '))
const pick = async (page, name, option) => {
 await slot(page, name).click()
 const list = page.getByRole('listbox',{name})
 await expect(list).toBeVisible()
 await list.getByRole('option',{name:option}).click()
 await expect(list).toBeHidden()
}

// A start in several projects: the run as the server keeps it, with one member per project.
const runMember = (position, projectId, name, state, over={}) => ({position,host:'a1',dir:`/home/dev/${name.toLowerCase()}`,tool:'claude',model:'',state,code:'',sentence:'',sessionId:'',
 worktreePath:'',worktreeRepo:'',worktreeBranch:'',worktreeRun:'',worktreeProject:'',sharesCheckout:false,attempt:1,projectId,name,base:'',...over})
const LIMIT = 'The terminal session limit has been reached. End a session and try again.'
const selectionRun = members => ({id:'run-1',origin:'selection',projectId:'',projectName:members.map(m => m.name).join(', '),state:members.some(m => m.state === 'pending') ? 'running' : 'partial',startedAt:now,useWorktrees:true,worktreeProject:'',members})
const blog = () => fproject('fp4','Blog','a1','/home/dev/blog',{position:3})

try {
 // Select (from the rail heading's menu) turns the cards into checkboxes; with three chosen, Start sends one run request and
 // never a session create. A lost reply is sent again exactly; the status sheet counts only what the controller says started,
 // and a member that failed is retried on its own.
 if (run('select three projects and start; one fails and retries')) {
  const runs = [], retries = []
  let reads = 0, retried = false, lostOnce = true
  const routes = {
   '/api/v2/folder-projects/runs': async request => {
    runs.push(request.postDataJSON())
    if (lostOnce) { lostOnce = false; return {status:502, json:{error:'bad gateway'}} }
    return {status:201, json:selectionRun([runMember(0,'fp1','Store rewrite','pending'), runMember(1,'fp3','Site','pending'), runMember(2,'fp4','Blog','pending')])}
   },
   '/api/v2/folder-projects/runs/run-1': () => {
    reads++
    const site = retried ? runMember(1,'fp3','Site','started',{sessionId:'s-site',attempt:2}) : runMember(1,'fp3','Site','failed',{code:'session_limit',sentence:LIMIT})
    const blogMember = reads < 2 ? runMember(2,'fp4','Blog','pending') : runMember(2,'fp4','Blog','started',{sessionId:'s-blog'})
    return selectionRun([runMember(0,'fp1','Store rewrite','started',{sessionId:'s-store'}), site, blogMember])
   },
   '/api/v2/folder-projects/runs/run-1/members/1/retry': request => {
    retries.push(request.postDataJSON())
    retried = true
    return selectionRun([runMember(0,'fp1','Store rewrite','started',{sessionId:'s-store'}), runMember(1,'fp3','Site','pending',{attempt:2}), runMember(2,'fp4','Blog','started',{sessionId:'s-blog'})])
   },
  }
  const {context,page,errors,box} = await launch({projects:[...fprojects(), blog()], routes})
  await page.goto(base)
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Store rewrite at studio from main in a fresh worktree')
  const rail = page.locator('aside.rail')
  await rail.getByRole('button',{name:'Projects menu'}).click()
  await page.getByRole('menuitem',{name:'Select projects'}).click()
  for (const name of [/^Store rewrite/, /^Site/, /^Blog/]) {
   const box = rail.getByRole('checkbox',{name})
   await box.click()
   await expect(box).toHaveAttribute('aria-checked','true')
  }
  await expect(rail.getByText('3 chosen.',{exact:true})).toBeVisible()
  // The cards keep their finger-sized targets while they are checkboxes.
  for (const card of await rail.getByRole('checkbox').all()) expect((await card.boundingBox()).height).toBeGreaterThanOrEqual(44)
  await expect.poll(() => sentenceText(page)).toBe("Run Claude Code with default on 3 projects at studio from each project's default in a fresh worktree")
  await expect(slot(page,'Project')).toHaveCount(0)
  await prompt(page).fill('Fix the failing tests')
  await startButton(page).click()
  // The reply was lost: the same request, every field the same, goes again.
  await expect(startButton(page)).toHaveAccessibleName('Try the same request again')
  await expect(bar(page).locator('.status')).toContainText('the agents may have started')
  await startButton(page).click()
  await expect.poll(() => runs.length).toBe(2)
  expect(runs[1]).toEqual(runs[0])
  expect(runs[0]).toEqual({requestId:runs[0].requestId, projectIds:['fp1','fp3','fp4'], tool:'claude', model:'', effort:'', prompt:'Fix the failing tests', worktree:true})
  expect(runs[0].requestId).toMatch(/^[0-9a-f-]{36}$/)
  expect(box.posts).toEqual([])
  const sheet = page.getByRole('dialog',{name:'Start in several projects'})
  await expect(sheet).toBeVisible()
  // Every member has settled (Blog started on the second read) before the count is read, so a count that took the failed
  // member for started would read 3.
  const siteRow = sheet.getByRole('listitem').filter({hasText:'Site'})
  await expect(sheet.getByRole('listitem').filter({hasText:'Blog'})).toContainText('Started.')
  await expect(siteRow).toContainText(LIMIT)
  await expect(sheet.getByRole('listitem').filter({hasText:'Store rewrite'})).toContainText('Started.')
  await expect(sheet.getByRole('status')).toHaveText('2 of 3 started')
  // Closed with a member that failed, the run stays one press away.
  await sheet.getByRole('button',{name:'Close'}).click()
  await expect(sheet).toBeHidden()
  await bar(page).getByRole('button',{name:'Show run status'}).click()
  await expect(sheet).toBeVisible()
  await expect(sheet.getByRole('status')).toHaveText('2 of 3 started')
  await expect(sheet.getByRole('link',{name:'Open Store rewrite'})).toHaveAttribute('href','/sessions?agent=a1&session=s-store')
  await expect(sheet).not.toContainText(/verified|healthy|\d+%/i)
  await expect(sheet.locator('progress, [role=progressbar]')).toHaveCount(0)
  await siteRow.getByRole('button',{name:'Retry Site'}).click()
  await expect(sheet.getByRole('status')).toHaveText('3 of 3 started')
  expect(retries).toEqual([{}])
  await expect(siteRow.getByRole('button',{name:'Retry Site'})).toHaveCount(0)
  expect(box.posts).toEqual([])
  // Done closes the sheet and the rail leaves Select; the prompt is cleared for the next start.
  await sheet.getByRole('button',{name:'Done'}).click()
  await expect(sheet).toBeHidden()
  await expect(rail.getByRole('checkbox')).toHaveCount(0)
  await expect(prompt(page)).toHaveValue('')
  // Every member started: nothing is left to show.
  await expect(bar(page).getByRole('button',{name:'Show run status'})).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // The rail and the bar never fall out of step: a run request kept after a lost reply and then discarded once the rail
 // has left Select goes back to one project; projects chosen before the bar is ready are the bar's projects once it is.
 if (run('the bar follows the rail after a discarded run and before it is ready')) {
  const runs = []
  const routes = {'/api/v2/folder-projects/runs': request => { runs.push(request.postDataJSON()); return {status:502, json:{error:'bad gateway'}} }}
  const {context,page,errors,box} = await launch({projects:[...fprojects(), blog()], routes})
  await page.goto(base)
  const rail = page.locator('aside.rail')
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Store rewrite at studio from main in a fresh worktree')
  await rail.getByRole('button',{name:'Projects menu'}).click()
  await page.getByRole('menuitem',{name:'Select projects'}).click()
  for (const name of [/^Site/, /^Blog/, /^Store rewrite/]) await rail.getByRole('checkbox',{name}).click()
  await startButton(page).click()
  await expect(startButton(page)).toHaveAccessibleName('Try the same request again')
  await rail.getByRole('button',{name:'Done selecting'}).click()
  await expect(rail.getByRole('checkbox')).toHaveCount(0)
  // While the request is kept the bar still names it; once discarded it names one project, as the rail does.
  await expect.poll(() => sentenceText(page)).toContain('on 3 projects')
  await bar(page).getByRole('button',{name:'Discard this request'}).click()
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Store rewrite at studio from main in a fresh worktree')
  await expect(slot(page,'Project')).toHaveAccessibleName('Project: Store rewrite')
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(runs).toHaveLength(1)
  expect(box.posts).toHaveLength(1)
  expect(box.posts[0]).toMatchObject({projectId:'fp1'})
  expect(errors).toEqual([])
  await context.close()
  // Home's profile read is held, so the bar has no choice yet while three projects are chosen.
  let release = () => {}, reads = 0
  const profile = {user:{id:1,name:'Sam',email:'sam@example.test',role:'owner'},sessionCount:1,accounts:true,profile:{theme:'light',timezone:'Europe/London',dateFormat:'',shelfView:'list',cardsExpanded:false,hostedPrefix:'reet-',updatedAt:1}}
  const held = await launch({projects:[...fprojects(), blog()], routes:{'/api/v2/profile': () => reads++ === 0 ? profile : new Promise(r => { release = () => r(profile) })}})
  await held.page.goto(base)
  const heldRail = held.page.locator('aside.rail')
  await heldRail.getByRole('button',{name:'Projects menu'}).click()
  await held.page.getByRole('menuitem',{name:'Select projects'}).click()
  for (const name of [/^Site/, /^Blog/, /^Store rewrite/]) await heldRail.getByRole('checkbox',{name}).click()
  await expect(heldRail.getByText('3 chosen.',{exact:true})).toBeVisible()
  release()
  await expect.poll(() => sentenceText(held.page)).toBe("Run Claude Code with default on 3 projects at studio from each project's default in a fresh worktree")
  expect(held.errors).toEqual([])
  await held.context.close()
 }

 // A start in several projects refused for speed keeps its request (nothing started) and says so; one refused because a
 // project is still starting says it in this start's words.
 if (run('a run start refused for speed keeps its request; refusals in its own words')) {
  const runs = []
  let n = 0
  const routes = {'/api/v2/folder-projects/runs': request => {
   runs.push(request.postDataJSON())
   n++
   if (n === 1) return {status:429, json:{code:'project_rate_limited', error:'Slow down a moment, then try again.'}}
   return {status:409, json:{code:'run_in_progress', error:'One of those projects is still starting. Wait for it to finish, then start again.'}}
  }}
  const {context,page,errors,box} = await launch({projects:[...fprojects(), blog()], routes})
  await page.goto(base)
  const rail = page.locator('aside.rail')
  await rail.getByRole('button',{name:'Projects menu'}).click()
  await page.getByRole('menuitem',{name:'Select projects'}).click()
  for (const name of [/^Site/, /^Blog/]) await rail.getByRole('checkbox',{name}).click()
  await startButton(page).click()
  await expect(bar(page).locator('.status')).toHaveText('Slow down a moment, then try again.')
  await expect(startButton(page)).toHaveAccessibleName('Try the same request again')
  await startButton(page).click()
  await expect(bar(page).locator('.status')).toHaveText('One of those projects is still starting. Wait for it to finish, then start again.')
  expect(runs).toHaveLength(2)
  expect(runs[1]).toEqual(runs[0])
  await expect(startButton(page)).toHaveAccessibleName('Start the session')
  expect(box.posts).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 // On touch a long press on a card starts Select; at 320 px long names wrap and Start stays on screen; from 1100 to 1239 px
 // the 56 px strip shows how many are chosen.
 if (run('select on touch, at 320 px and on the strip')) {
  const long = [...fprojects(), blog()].map(p => p.id === 'fp1' ? {...p, name:'the-store-rewrite-with-a-very-long-project-name-indeed'} : p)
  const phone = await launch({width:320, height:700, touch:true, projects:long})
  await phone.page.clock.install()
  await phone.page.goto(base)
  const card = phone.page.locator('aside.rail .card').filter({hasText:'the-store-rewrite'})
  await card.scrollIntoViewIfNeeded()
  const at = await card.boundingBox()
  const point = {pointerType:'touch', isPrimary:true, clientX:at.x + 20, clientY:at.y + 10, bubbles:true}
  await card.dispatchEvent('pointerdown', point)
  // The long press (500 ms) on the page's own clock, not a real wait.
  await phone.page.clock.runFor(600)
  await card.dispatchEvent('pointerup', point)
  const rail = phone.page.locator('aside.rail')
  await expect(rail.getByRole('checkbox',{name:/^the-store-rewrite/})).toHaveAttribute('aria-checked','true')
  // One chosen: the summary names it, wraps, and keeps Start on the screen.
  await expect(phone.page.locator('.summary-text')).toContainText('the-store-rewrite-with-a-very-long-project-name-indeed')
  const one = await startButton(phone.page).boundingBox()
  expect(one.x + one.width).toBeLessThanOrEqual(320)
  expect(await phone.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await rail.getByRole('checkbox',{name:/^Site/}).click()
  await rail.getByRole('checkbox',{name:/^Blog/}).click()
  await expect(rail.getByText('3 chosen.',{exact:true})).toBeVisible()
  await expect(phone.page.locator('.summary-text')).toContainText('3 projects')
  const s = await startButton(phone.page).boundingBox()
  expect(s.x + s.width).toBeLessThanOrEqual(320)
  expect(await phone.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  for (const c of await rail.getByRole('checkbox').all()) {
   const b = await c.boundingBox()
   expect(b.height).toBeGreaterThanOrEqual(44)
   expect(b.x + b.width).toBeLessThanOrEqual(320)
  }
  // Done leaves Select and gives focus back to the heading's menu button.
  await rail.getByRole('button',{name:'Done selecting'}).click()
  await expect(rail.getByRole('button',{name:'Projects menu'})).toBeFocused()
  expect(phone.errors).toEqual([])
  await phone.context.close()
  const strip = await launch({width:1180, height:800, projects:[...fprojects(), blog()]})
  await strip.page.goto(base)
  const panel = strip.page.getByRole('button',{name:/^Open the project rail/})
  await panel.click()
  await strip.page.locator('aside.rail').getByRole('button',{name:'Projects menu'}).click()
  await strip.page.getByRole('menuitem',{name:'Select projects'}).click()
  await strip.page.locator('aside.rail .full').getByRole('checkbox',{name:/^Site/}).click()
  await strip.page.locator('aside.rail .full').getByRole('checkbox',{name:/^Blog/}).click()
  await strip.page.locator('aside.rail').getByRole('button',{name:'Close the project rail'}).first().click()
  await expect(strip.page.locator('aside.rail .strip .picked-count')).toHaveText('Chosen: 2')
  await expect(strip.page.locator('aside.rail .strip').getByRole('checkbox')).toHaveCount(4)
  await expect.poll(() => sentenceText(strip.page)).toContain('on 2 projects at studio')
  expect(strip.errors).toEqual([])
  await strip.context.close()
 }

 // A rail card (or its New session here) puts that project in the sentence with its own defaults: agent, model and worktree
 // choice; its starter prompt is offered with the chip and goes out only once the person uses it.
 if (run('a rail card applies the project defaults and offers its starter prompt')) {
  const projects = fprojects().map(p => p.id === 'fp3' ? {...p, defaultTool:'codex', defaultModel:'gpt-5-codex', worktreeDefault:'no', hasStarterPrompt:true} : p)
  const routes = {'/api/v2/folder-projects/fp3': () => ({...projects.find(p => p.id === 'fp3'), starterPrompt:'Tidy the site build and say what changed'})}
  const {context,page,errors,box} = await launch({projects, routes})
  await page.goto(base)
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Store rewrite at studio from main in a fresh worktree')
  const rail = page.locator('aside.rail')
  await rail.locator('.card').filter({hasText:'Site'}).click()
  await expect.poll(() => sentenceText(page)).toBe('Run Codex with gpt-5-codex on Site at studio in the checkout')
  await expect(prompt(page)).toBeFocused()
  await expect(prompt(page)).toHaveValue('')
  // Back to a project that names no agent (the choice stays, with its own base and worktree), then New session here from
  // Site's menu: Site's defaults again.
  await pick(page,'Agent','Claude Code')
  await rail.locator('.card').filter({hasText:'Store rewrite'}).click()
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Store rewrite at studio from main in a fresh worktree')
  await rail.getByRole('button',{name:'More for Site'}).click()
  await page.getByRole('menuitem',{name:'New session here'}).click()
  await expect.poll(() => sentenceText(page)).toBe('Run Codex with gpt-5-codex on Site at studio in the checkout')
  await bar(page).getByRole('button',{name:'Use the starter prompt'}).click()
  await expect(prompt(page)).toHaveValue('Tidy the site build and say what changed')
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts[0]).toMatchObject({agentId:'a1', tool:'codex', model:'gpt-5-codex', dir:'/home/dev/site', projectId:'fp3', prompt:'Tidy the site build and say what changed'})
  for (const key of ['worktree','project','base']) expect(box.posts[0], key).not.toHaveProperty(key)
  expect(errors).toEqual([])
  await context.close()
 }

 // A refusal shown after a kept request stays when the bar is free again and its projects are the same: the rail's selection,
 // made while the request was kept, is applied without wiping the sentence that says why nothing started.
 if (run('a refusal after a kept request survives the same selection')) {
  const {context,page,errors,box} = await launch({answer:(body, n) => n === 1 ? {status:502, json:{error:'bad gateway'}} : {status:422, json:{error:'tool unavailable', code:'tool_unavailable'}}})
  await page.goto(base)
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Store rewrite at studio from main in a fresh worktree')
  await startButton(page).click()
  await expect(startButton(page)).toHaveAccessibleName('Try the same request again')
  const rail = page.locator('aside.rail')
  await rail.getByRole('button',{name:'Projects menu'}).click()
  await page.getByRole('menuitem',{name:'Select projects'}).click()
  await rail.getByRole('checkbox',{name:/^Store rewrite/}).click()
  await expect(rail.getByText('1 chosen. Choose at least one more.',{exact:true})).toBeVisible()
  await startButton(page).click()
  await expect(startButton(page)).toHaveAccessibleName('Start the session')
  await settled(page)
  await expect(bar(page).locator('.status')).toHaveText("reet doesn't start this agent yet.")
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Store rewrite at studio from main in a fresh worktree')
  // A different project set is a new start: the old sentence goes.
  await rail.getByRole('checkbox',{name:/^Site/}).click()
  await expect.poll(() => sentenceText(page)).toContain('on 2 projects')
  await expect(bar(page).locator('.status')).toHaveText('')
  expect(box.posts).toHaveLength(2)
  expect(errors).toEqual([])
  await context.close()
 }

 // Show run status is never stale: it goes once every member has started (read after the sheet closed), and a later start in
 // one project drops it.
 if (run('Show run status goes once all started or after a later start')) {
  let reads = 0
  const routes = {
   '/api/v2/folder-projects/runs': () => ({status:201, json:selectionRun([runMember(0,'fp1','Store rewrite','started',{sessionId:'s-store'}), runMember(1,'fp3','Site','pending')])}),
   '/api/v2/folder-projects/runs/run-1': () => { reads++; return selectionRun([runMember(0,'fp1','Store rewrite','started',{sessionId:'s-store'}), reads < 3 ? runMember(1,'fp3','Site','pending') : runMember(1,'fp3','Site','started',{sessionId:'s-site'})]) },
  }
  const {context,page,errors} = await launch({routes})
  await page.goto(base)
  const rail = page.locator('aside.rail')
  await rail.getByRole('button',{name:'Projects menu'}).click()
  await page.getByRole('menuitem',{name:'Select projects'}).click()
  for (const name of [/^Store rewrite/, /^Site/]) await rail.getByRole('checkbox',{name}).click()
  await startButton(page).click()
  const sheet = page.getByRole('dialog',{name:'Start in several projects'})
  await expect(sheet.getByRole('status')).toHaveText('1 of 2 started')
  await sheet.getByRole('button',{name:'Close'}).click()
  const chip = bar(page).getByRole('button',{name:'Show run status'})
  await expect(chip).toBeVisible()
  // Site starts while the sheet is closed: the chip goes.
  await expect(chip).toHaveCount(0, {timeout:10_000})
  expect(errors).toEqual([])
  await context.close()
  // A member that failed keeps the chip until a later start in one project, which drops it.
  const failedRuns = {
   '/api/v2/folder-projects/runs': () => ({status:201, json:selectionRun([runMember(0,'fp1','Store rewrite','started',{sessionId:'s-store'}), runMember(1,'fp3','Site','failed',{code:'session_limit',sentence:LIMIT})])}),
  }
  const later = await launch({routes:failedRuns})
  await later.page.route(url => new URL(url).pathname === '/sessions', route => route.fulfill({status:204, body:''}))
  await later.page.goto(base)
  const lrail = later.page.locator('aside.rail')
  await lrail.getByRole('button',{name:'Projects menu'}).click()
  await later.page.getByRole('menuitem',{name:'Select projects'}).click()
  for (const name of [/^Store rewrite/, /^Site/]) await lrail.getByRole('checkbox',{name}).click()
  await startButton(later.page).click()
  const lsheet = later.page.getByRole('dialog',{name:'Start in several projects'})
  await expect(lsheet.getByRole('status')).toHaveText('1 of 2 started')
  await lsheet.getByRole('button',{name:'Done'}).click()
  await expect(lrail.getByRole('checkbox')).toHaveCount(0)
  const lchip = bar(later.page).getByRole('button',{name:'Show run status'})
  await expect(lchip).toBeVisible()
  // Pressed and read from inside: Playwright's own click and locators wait for a navigation that a 204 never finishes.
  await later.page.evaluate(() => document.querySelector('form[aria-label="Start a session"] button[type=submit]').click())
  await expect.poll(() => later.box.posts.length).toBe(1)
  // The next page never arrives: the bar lets go after a few seconds, with no chip for the older run.
  const look = () => later.page.evaluate(() => { const f = document.querySelector('form[aria-label="Start a session"]'), b = f.querySelector('button[type=submit]')
   return {disabled:b.disabled, chip:[...f.querySelectorAll('button')].some(x => x.textContent.trim() === 'Show run status')} })
  await expect.poll(look, {timeout:9000}).toEqual({disabled:false, chip:false})
  expect(later.errors).toEqual([])
  await later.context.close()
 }

 // A rail card or New session here while the bar is busy with a start says why nothing changed, in the bar's status line,
 // and keeps what the bar already said.
 if (run('choosing a project while the bar is locked says why')) {
  const {context,page,errors} = await launch({answer:() => ({status:502, json:{error:'bad gateway'}})})
  await page.goto(base)
  await startButton(page).click()
  await expect(startButton(page)).toHaveAccessibleName('Try the same request again')
  const rail = page.locator('aside.rail')
  await rail.locator('.card').filter({hasText:'Site'}).click()
  const status = bar(page).locator('.status')
  await expect(status).toContainText("We didn't hear back, so the agent may have started.")
  await expect(status).toContainText('Finish or discard the current start first.')
  await rail.getByRole('button',{name:'More for Site'}).click()
  await page.getByRole('menuitem',{name:'New session here'}).click()
  await expect(status).toContainText('Finish or discard the current start first.')
  await expect.poll(() => sentenceText(page)).toContain('on Store rewrite')
  await bar(page).getByRole('button',{name:'Discard this request'}).click()
  await expect(status).toHaveText('')
  await rail.locator('.card').filter({hasText:'Site'}).click()
  await expect.poll(() => sentenceText(page)).toContain('on Site')
  await expect(status).toHaveText('')
  expect(errors).toEqual([])
  await context.close()
 }

 // The rail marks no card while the bar names the home folder, even when the address named a project.
 if (run('the rail marks no card for the home folder')) {
  const {context,page,errors} = await launch()
  await page.goto(`${base}/?project=fp3`)
  const rail = page.locator('aside.rail')
  await expect.poll(() => sentenceText(page)).toContain('on Site at studio')
  await expect(rail.locator('.card[aria-current="true"]')).toHaveCount(1)
  await expect(rail.locator('.card[aria-current="true"]')).toContainText('Site')
  await pick(page,'Project','Home folder on studio')
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on the home folder at studio')
  await expect(rail.locator('[aria-current="true"]')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // The Branch list opened while the machine is still answering says it is reading, not that it failed; after a read that
 // failed, opening the list again asks the machine again.
 if (run('branch list while reading and after a failed read')) {
  let release = () => {}, reads = 0
  const routes = {'/api/v2/machines/a1/branches': () => {
   reads++
   if (reads === 1) return new Promise(r => { release = () => r({status:503, json:{error:'reconnecting', code:'machine_reconnecting'}}) })
   return {current:'main', default:'main', branches:['main','develop'], more:false}
  }}
  const {context,page,errors} = await launch({routes})
  await page.goto(base)
  await expect.poll(() => reads).toBe(1)
  await slot(page,'Branch').click()
  const list = page.getByRole('listbox',{name:'Branch'})
  await expect(list).toBeVisible()
  await expect(page.getByText('Reading branches…',{exact:true})).toBeVisible()
  await expect(page.getByText("Couldn't read the branches just now.")).toHaveCount(0)
  release()
  await expect(page.getByText('Reading branches…',{exact:true})).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(list).toBeHidden()
  await slot(page,'Branch').click()
  await expect.poll(() => reads).toBe(2)
  await expect(list.getByRole('option',{name:'develop',exact:true})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // YOLO's reason can be read without a screen reader: a tooltip in the bar and a line in the Change sheet.
 if (run('YOLO reason is visible')) {
  const desk = await launch()
  await desk.page.goto(base)
  await expect(bar(desk.page).locator('label', {hasText:'YOLO'})).toHaveAttribute('title', "reet doesn't start YOLO sessions yet.")
  expect(desk.errors).toEqual([])
  await desk.context.close()
  const {context,page,errors} = await launch({width:390, height:844, touch:true})
  await page.goto(base)
  await bar(page).getByRole('button',{name:'Change',exact:true}).click()
  const sheet = page.getByRole('dialog',{name:'Change the launch'})
  await expect(sheet.getByText("reet doesn't start YOLO sessions yet.",{exact:true})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // The session screen's + arrives at /#launch: the prompt takes focus and the address forgets the fragment.
 if (run('arriving from a session tab focuses the prompt')) {
  const {context,page,errors} = await launch()
  await page.goto(`${base}/#launch`)
  await expect(bar(page).getByLabel('Describe the job')).toBeFocused()
  expect(new URL(page.url()).hash).toBe('')
  expect(errors).toEqual([])
  await context.close()
 }

 // Select started on the 56 px strip (1100 to 1239 px) can be stopped there too.
 if (run('Stop selecting on the strip')) {
  const {context,page,errors} = await launch({width:1180, height:800, projects:[...fprojects(), blog()]})
  await page.goto(base)
  const strip = page.locator('aside.rail .strip')
  await page.getByRole('button',{name:/^Open the project rail/}).click()
  await page.locator('aside.rail').getByRole('button',{name:'Projects menu'}).click()
  await page.getByRole('menuitem',{name:'Select projects'}).click()
  await page.locator('aside.rail').getByRole('button',{name:'Close the project rail'}).first().click()
  await expect(strip.getByRole('checkbox')).toHaveCount(4)
  const stop = strip.getByRole('button',{name:'Stop selecting'})
  expect(Math.min(...Object.values(await stop.boundingBox()).slice(2))).toBeGreaterThanOrEqual(40)
  await stop.click()
  await expect(strip.getByRole('checkbox')).toHaveCount(0)
  await expect(stop).toHaveCount(0)
  await expect(strip.getByRole('button',{name:/^Open the project rail/})).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
 }

 // Projects that can't be read leave the bar with a reason, not silently disabled.
 if (run('projects never read: the bar says why')) {
  const {context,page,errors} = await launch({routes:{'/api/v2/folder-projects': () => ({status:500, json:{error:'down'}})}})
  await page.goto(base)
  await expect(bar(page).locator('.status')).toHaveText("Couldn't read your projects just now. Start waits until they have been read; Home tries again every 15 seconds.")
  await expect(startButton(page)).toBeDisabled()
  expect(errors).toEqual([])
  await context.close()
 }

 // A project removed while its request is kept does not change what the bar says that request starts.
 if (run('a removed project leaves a kept request as it was')) {
  const {context,page,errors,box} = await launch({answer:() => ({status:502, json:{error:'bad gateway'}})})
  await page.clock.install()
  await page.goto(base)
  const said = 'Run Claude Code with default on Store rewrite at studio from main in a fresh worktree'
  await expect.poll(() => sentenceText(page)).toBe(said)
  await startButton(page).click()
  await expect(startButton(page)).toHaveAccessibleName('Try the same request again')
  box.projects = box.projects.filter(p => p.id !== 'fp1')
  // Home reads its lists again every 15 seconds.
  await page.clock.runFor(16_000)
  await expect(page.locator('aside.rail .card').filter({hasText:'Store rewrite'})).toHaveCount(0)
  await settled(page)
  expect(await sentenceText(page)).toBe(said)
  // Discarded, the bar falls back as a fresh open would.
  await bar(page).getByRole('button',{name:'Discard this request'}).click()
  await expect.poll(() => sentenceText(page)).not.toContain('Store rewrite')
  expect(errors).toEqual([])
  await context.close()
 }

 // A start the server no longer has stops being read and says so; a retry refused for any reason but "not failed" is said
 // beside the member rather than read past.
 if (run('a run that is gone, and a retry refused')) {
  let reads = 0
  const routes = {
   '/api/v2/folder-projects/runs': () => ({status:201, json:selectionRun([runMember(0,'fp1','Store rewrite','started',{sessionId:'s-store'}), runMember(1,'fp3','Site','failed',{code:'session_limit',sentence:LIMIT}), runMember(2,'fp4','Blog','pending')])}),
   '/api/v2/folder-projects/runs/run-1': () => { reads++; return {status:404, json:{error:'not found', code:'run_not_found'}} },
   '/api/v2/folder-projects/runs/run-1/members/1/retry': () => ({status:409, json:{error:'One of those projects is still starting. Wait for it to finish, then start again.', code:'run_in_progress'}}),
  }
  const {context,page,errors} = await launch({projects:[...fprojects(), blog()], routes})
  await page.clock.install()
  await page.goto(base)
  const rail = page.locator('aside.rail')
  await rail.getByRole('button',{name:'Projects menu'}).click()
  await page.getByRole('menuitem',{name:'Select projects'}).click()
  for (const name of [/^Store rewrite/, /^Site/, /^Blog/]) await rail.getByRole('checkbox',{name}).click()
  await startButton(page).click()
  const sheet = page.getByRole('dialog',{name:'Start in several projects'})
  await expect(sheet).toBeVisible()
  await page.clock.runFor(1600)
  await expect(sheet).toContainText("reet no longer has this start, so it can't say more. Your sessions list shows what started.")
  await page.clock.runFor(10_000)
  expect(reads).toBe(1)
  const site = sheet.getByRole('listitem').filter({hasText:'Site'})
  await site.getByRole('button',{name:'Retry Site'}).click()
  await expect(site).toContainText('One of those projects is still starting. Wait for it to finish, then start again.')
  expect(reads).toBe(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // Home has no Start an agent button: the bar is the way to start.
 if (run('one press starts with the last choices')) {
  // The newest session that names an agent is the last launch: Codex with gpt-5-codex in Store rewrite, in a worktree.
  const sessions = [
   session('s1','a1','Tidy',{tool:'claude',projectId:'fp3',createdAt:now-50*MIN}),
   session('s2','a1','Refactor',{tool:'codex',model:'gpt-5-codex',projectId:'fp1',worktreePath:'/home/dev/.reet/worktrees/store-rewrite/store/x',worktreeRepo:'store',createdAt:now-5*MIN}),
   session('s3','a1','Plain shell',{createdAt:now-MIN}),
  ]
  const {context,page,errors,box} = await launch({sessions})
  await page.goto(base)
  await expect(bar(page)).toBeVisible()
  await expect(page.getByRole('button',{name:'Start an agent',exact:true})).toHaveCount(0)
  await expect.poll(() => sentenceText(page)).toBe('Run Codex with gpt-5-codex on Store rewrite at studio from main in a fresh worktree')
  await expect(slot(page,'Agent')).toHaveAccessibleName('Agent: Codex')
  await expect(prompt(page)).toHaveValue('')
  await expect(prompt(page)).toHaveAttribute('placeholder','Describe the job. Say what done looks like.')
  await expect(bar(page).getByRole('radiogroup',{name:'Approvals'}).getByRole('radio',{name:'Ask me'})).toBeChecked()
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?agent=a1&session=new-/)
  expect(box.posts).toHaveLength(1)
  const body = box.posts[0]
  expect(body).toMatchObject({agentId:'a1', tool:'codex', model:'gpt-5-codex', dir:'/home/dev/store', projectId:'fp1', project:'store-rewrite', base:'main', title:'Codex · store'})
  expect(body.worktree).toMatch(/^[0-9a-f]{32}$/)
  expect(body.requestId).toMatch(/^[0-9a-f-]{36}$/)
  for (const key of ['prompt','effort','yolo']) expect(body, key).not.toHaveProperty(key)
  await expect(page).toHaveURL(`${base}/sessions?agent=a1&session=${encodeURIComponent(`new-${body.requestId.slice(0,8)}`)}`)
  expect(errors).toEqual([])
  await context.close()
 }

 // Each slot is a button named for its slot and value; it opens a listbox, and the choice shows in the sentence and goes out.
 // With a keyboard on a desktop, Tab or Escape from an open list (or from the project list's filter) closes it and leaves focus
 // in the bar: Escape on its slot, Tab on the next slot, never on the page's first control.
 if (run('Tab and Escape from a slot list keep focus in the bar')) {
  const {context,page,errors,box} = await launch()
  await page.goto(base)
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Store rewrite at studio from main in a fresh worktree')
  await slot(page,'Agent').click()
  const agents = page.getByRole('listbox',{name:'Agent'})
  await expect(agents.getByRole('option',{name:/^Claude Code/})).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(agents).toBeHidden()
  await expect(slot(page,'Model')).toBeFocused()
  await slot(page,'Project').click()
  const find = page.getByRole('searchbox',{name:'Find a project'})
  const projects = page.getByRole('listbox',{name:'Project'})
  await expect(find).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(projects).toBeHidden()
  await expect(slot(page,'Project')).toBeFocused()
  await slot(page,'Project').click()
  await expect(find).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(projects).toBeHidden()
  await expect(slot(page,'Branch')).toBeFocused()
  expect(box.posts).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 if (run('each slot opens and sets its value')) {
  const {context,page,errors,box} = await launch()
  await page.goto(base)
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Store rewrite at studio from main in a fresh worktree')
  // Escape closes a list and gives focus back to its slot; arrow keys move through the options.
  await slot(page,'Agent').click()
  const agents = page.getByRole('listbox',{name:'Agent'})
  await expect(agents.getByRole('option',{name:/^Claude Code/})).toHaveAttribute('aria-selected','true')
  await expect(agents.getByRole('option',{name:/^Pi\b/})).toHaveAttribute('aria-disabled','true')
  await expect(agents.getByRole('option',{name:/^Pi\b/})).toContainText("reet doesn't start this agent yet.")
  await page.keyboard.press('Escape')
  await expect(agents).toBeHidden()
  await expect(slot(page,'Agent')).toBeFocused()
  await pick(page,'Agent','Gemini CLI')
  await expect(slot(page,'Agent')).toHaveAccessibleName('Agent: Gemini CLI')
  await pick(page,'Model','gemini-2.5-flash')
  await pick(page,'Project',/^Site/)
  await expect(slot(page,'Branch')).toHaveAccessibleName('Branch: main')
  await slot(page,'Branch').click()
  const branches = page.getByRole('listbox',{name:'Branch'})
  await expect(branches.getByRole('option')).toHaveText([/current branch/, 'main', 'develop', 'release/1.2'])
  await branches.getByRole('option',{name:'develop',exact:true}).click()
  // The machine follows the project (R18): it is named, not chosen.
  await expect(bar(page).getByRole('button',{name:/^Machine: /})).toHaveCount(0)
  await expect.poll(() => sentenceText(page)).toBe('Run Gemini CLI with gemini-2.5-flash on Site at studio from develop in a fresh worktree')
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts[0]).toMatchObject({agentId:'a1', tool:'gemini', model:'gemini-2.5-flash', dir:'/home/dev/site', projectId:'fp3', project:'site', base:'develop'})
  // Cline takes effort; the checkout has no branch slot and says where it works.
  await page.goto(base)
  await pick(page,'Agent','Cline')
  await expect(slot(page,'Thinking')).toHaveAccessibleName('Thinking: default')
  await pick(page,'Thinking','high')
  await pick(page,'Worktree','the checkout')
  await expect(slot(page,'Branch')).toHaveCount(0)
  await expect(bar(page).getByText('Works in the checkout on main.',{exact:true})).toBeVisible()
  await expect.poll(() => sentenceText(page)).toBe('Run Cline with default on Store rewrite at studio in the checkout · thinking high')
  // The Home folder on a ready machine: no project, no branch or worktree.
  await pick(page,'Project','Home folder on studio')
  await expect.poll(() => sentenceText(page)).toBe('Run Cline with default on the home folder at studio · thinking high')
  await pick(page,'Project',/^Store rewrite/)
  await pick(page,'Worktree','the checkout')
  // YOLO waits for a later release: its half is disabled, with the reason in its description.
  const yolo = bar(page).getByRole('radiogroup',{name:'Approvals'}).getByRole('radio',{name:'YOLO'})
  await expect(yolo).toBeDisabled()
  await expect(yolo).toHaveAccessibleDescription("reet doesn't start YOLO sessions yet.")
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts[1]).toMatchObject({agentId:'a1', tool:'cline', effort:'high', dir:'/home/dev/store', projectId:'fp1'})
  for (const key of ['worktree','project','base','model','yolo']) expect(box.posts[1], key).not.toHaveProperty(key)
  expect(errors).toEqual([])
  await context.close()
 }

 // A project on a machine that is not ready is listed, disabled, with that machine's own sentence; so is its home folder.
 if (run('unready machine disabled with its sentence')) {
  const sessions = [session('s1','a2','Notes work',{tool:'claude',projectId:'fp2'})]
  const {context,page,errors,box} = await launch({sessions})
  await page.goto(base)
  const reason = 'The machine is reconnecting. Existing shells will be kept.'
  // The last launch was on desk: Start waits, and says why.
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Notes at desk')
  await expect(startButton(page)).toBeDisabled()
  await expect(bar(page).locator('.status')).toHaveText(reason)
  await slot(page,'Project').click()
  const list = page.getByRole('listbox',{name:'Project'})
  const notes = list.getByRole('option',{name:/^Notes/})
  await expect(notes).toHaveAttribute('aria-disabled','true')
  await expect(notes).toContainText(reason)
  await expect(list.getByRole('option',{name:/^Home folder on desk/})).toHaveAttribute('aria-disabled','true')
  await notes.click({force:true})
  await expect(list).toBeVisible()
  await list.getByRole('option',{name:/^Store rewrite/}).click()
  await expect(startButton(page)).toBeEnabled()
  await expect(bar(page).locator('.status')).toHaveText('')
  expect(box.posts).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 // A project's starter prompt is offered with a chip while the box is empty; it is never put in the box, or sent, by itself.
 if (run('starter prompt is offered, never filled')) {
  const projects = fprojects().map(p => p.id === 'fp1' ? {...p, hasStarterPrompt:true} : p)
  const {context,page,errors,box} = await launch({projects})
  await page.goto(base)
  const chip = bar(page).getByRole('button',{name:'Use the starter prompt',exact:true})
  await expect(chip).toBeVisible()
  await expect(prompt(page)).toHaveValue('')
  await expect(bar(page).getByText("Secrets don't belong here.",{exact:false})).toHaveCount(0)
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts[0]).not.toHaveProperty('prompt')
  await page.goto(base)
  await chip.click()
  await expect(prompt(page)).toHaveValue('Fix the failing tests in the store package')
  await expect(prompt(page)).toBeFocused()
  await expect(chip).toHaveCount(0)
  await expect(bar(page).getByText("Secrets don't belong here. The prompt is visible in the machine's process list.",{exact:true})).toBeVisible()
  // Another project without one: no chip.
  await prompt(page).fill('')
  await pick(page,'Project',/^Site/)
  await expect(chip).toHaveCount(0)
  await pick(page,'Project',/^Store rewrite/)
  await expect(chip).toBeVisible()
  await chip.click()
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts[1]).toMatchObject({prompt:'Fix the failing tests in the store package', title:'Fix the failing tests in the store package'})
  expect(errors).toEqual([])
  await context.close()
 }

 // A reply that never came: the bar says so, keeps the same request (id, run token and every field) and Retry sends exactly it.
 if (run('lost reply retries the same request')) {
  const {context,page,errors,box} = await launch({answer:(body, n) => n === 1 ? {status:502, json:{error:'bad gateway'}} : session('new-1', body.agentId, body.title)})
  await page.goto(base)
  await prompt(page).fill('Fix the failing tests')
  await startButton(page).click()
  await expect(bar(page).locator('.status')).toContainText("We didn't hear back, so the agent may have started.")
  await expect(startButton(page)).toHaveAccessibleName('Try the same request again')
  // Nothing that would change the request can be changed while it is held.
  await expect(prompt(page)).toBeDisabled()
  await expect(slot(page,'Agent')).toBeDisabled()
  expect(page.url()).toBe(`${base}/`)
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?agent=a1&session=new-1/)
  expect(box.posts).toHaveLength(2)
  expect(box.posts[1]).toEqual(box.posts[0])
  expect(box.posts[0]).toMatchObject({prompt:'Fix the failing tests', tool:'claude'})
  expect(errors).toEqual([])
  await context.close()
 }

 // A refusal: nothing started, so the bar unlocks with the sentence and the next press is a new request. Discard drops a held one.
 if (run('a refusal unlocks the bar with its sentence')) {
  const {context,page,errors,box} = await launch({answer:(body, n) => n === 1 ? {status:422, json:{error:'tool unavailable', code:'tool_unavailable'}} : n === 2 ? {status:502, json:{error:'down'}} : session('new-3', body.agentId, body.title)})
  await page.goto(base)
  await startButton(page).click()
  await expect(bar(page).locator('.status')).toHaveText("reet doesn't start this agent yet.")
  await expect(startButton(page)).toHaveAccessibleName('Start the session')
  await expect(prompt(page)).toBeEnabled()
  await startButton(page).click()
  await expect(startButton(page)).toHaveAccessibleName('Try the same request again')
  await bar(page).getByRole('button',{name:'Discard this request',exact:true}).click()
  await expect(startButton(page)).toHaveAccessibleName('Start the session')
  await expect(prompt(page)).toBeEnabled()
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts[2].requestId).not.toBe(box.posts[1].requestId)
  expect(box.posts[1].requestId).not.toBe(box.posts[0].requestId)
  expect(errors).toEqual([])
  await context.close()
 }

 // The project's folder goes out exactly as it is, even when its name starts with a dash.
 if (run('a project folder named -x launches with dir unchanged')) {
  const projects = [fproject('fpx','-x','a1','/home/dev/-x'), ...fprojects()]
  const {context,page,errors,box} = await launch({projects})
  await page.goto(base)
  await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on -x at studio in a fresh worktree'.replace('in a fresh', 'from main in a fresh'))
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts[0]).toMatchObject({dir:'/home/dev/-x', projectId:'fpx', project:'x', title:'Claude Code · -x'})
  expect(errors).toEqual([])
  await context.close()
 }

 // While the request is out the bar is locked: the status line says what is starting, and no second press or key sends again.
 if (run('bar locked while starting')) {
  let release
  const held = new Promise(r => { release = r })
  const {context,page,errors,box} = await launch({answer:async body => { await held; return session('new-held', body.agentId, body.title) }})
  await page.goto(base)
  await prompt(page).fill('Fix the failing tests')
  await prompt(page).press('Control+Enter')
  await expect(bar(page).locator('.status')).toHaveText('Starting Claude Code on studio…')
  await expect(startButton(page)).toBeDisabled()
  await expect(startButton(page)).toHaveAttribute('aria-busy','true')
  await expect(prompt(page)).toBeDisabled()
  for (const name of ['Agent','Model','Project','Branch','Worktree']) await expect(slot(page,name), name).toBeDisabled()
  await page.evaluate(() => { const f = document.querySelector('form[aria-label="Start a session"]'); f.requestSubmit(); f.querySelector('textarea').dispatchEvent(new KeyboardEvent('keydown', {key:'Enter', ctrlKey:true, bubbles:true})) })
  await settled(page)
  expect(box.posts).toHaveLength(1)
  release()
  await expect(page).toHaveURL(/\/sessions\?agent=a1&session=new-held/)
  expect(box.posts).toHaveLength(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // Plain Enter adds a line (R8); Ctrl+Enter sends from a keyboard.
 if (run('Enter adds a line and Ctrl+Enter sends')) {
  const {context,page,errors,box} = await launch()
  await page.goto(base)
  await prompt(page).fill('Fix the failing tests')
  await prompt(page).press('Enter')
  await expect(prompt(page)).toHaveValue('Fix the failing tests\n')
  expect(box.posts).toEqual([])
  await prompt(page).press('Control+Enter')
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts).toHaveLength(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // On touch no key sends (R8): Enter and Ctrl+Enter add a line, and only Start sends.
 if (run('Enter never sends on touch')) {
  const {context,page,errors,box} = await launch({width:390, height:844, touch:true})
  await page.goto(base)
  await prompt(page).fill('Fix the failing tests')
  await prompt(page).press('Enter')
  await prompt(page).press('Control+Enter')
  await prompt(page).press('Meta+Enter')
  await settled(page)
  expect(box.posts).toEqual([])
  await expect(prompt(page)).toHaveValue('Fix the failing tests\n')
  expect(page.url()).toBe(`${base}/`)
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts).toHaveLength(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // A phone held sideways (844 × 390, touch) takes the phone's layout: one summary line with one Change, never the tablet's too.
 if (run('landscape phone shows one Change')) {
  const {context,page,errors} = await launch({width:844, height:390, touch:true})
  await page.goto(base)
  await expect(bar(page).locator('.summary-text')).toHaveText('Claude Code on Store rewrite at studio · main · fresh worktree')
  await expect(bar(page).getByRole('button',{name:'Change',exact:true})).toHaveCount(1)
  expect(await bar(page).locator('button.change').evaluateAll(list => list.filter(b => b.getClientRects().length > 0).map(b => b.parentElement.className.split(' ')[0]))).toEqual(['summary'])
  await expect(bar(page).locator('.sentence')).toBeHidden()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
 }

 // A phone: one-line prompt, a round 48 px Start and one summary line; Change opens a bottom sheet with the slots as large rows
 // and Approvals.
 if (run('phone Change sheet')) {
  const {context,page,errors,box} = await launch({width:390, height:844, touch:true})
  await page.goto(base)
  const start = startButton(page)
  const size = await start.boundingBox()
  expect([Math.round(size.width), Math.round(size.height)]).toEqual([48, 48])
  await expect(bar(page).locator('.sentence')).toBeHidden()
  const line = bar(page).locator('.summary-text')
  await expect(line).toHaveText('Claude Code on Store rewrite at studio · main · fresh worktree')
  await bar(page).getByRole('button',{name:'Change',exact:true}).click()
  const sheet = page.getByRole('dialog',{name:'Change the launch'})
  await expect(sheet).toBeVisible()
  const row = name => sheet.getByRole('button',{name:new RegExp(`^${name}: `)})
  for (const name of ['Agent','Model','Project','Branch','Worktree']) {
   await expect(row(name), name).toBeVisible()
   expect((await row(name).boundingBox()).height, name).toBeGreaterThanOrEqual(48)
  }
  await expect(sheet.getByRole('radiogroup',{name:'Approvals'})).toBeVisible()
  await row('Agent').click()
  const agents = page.getByRole('listbox',{name:'Agent'})
  await expect(agents).toBeVisible()
  expect((await agents.getByRole('option',{name:/^Codex/}).boundingBox()).height).toBeGreaterThanOrEqual(48)
  await agents.getByRole('option',{name:/^Codex/}).click()
  await expect(agents).toBeHidden()
  await expect(row('Agent')).toHaveAccessibleName('Agent: Codex')
  await expect(row('Agent')).toBeFocused()
  await sheet.getByRole('button',{name:'Done',exact:true}).click()
  await expect(sheet).toBeHidden()
  await expect(bar(page).getByRole('button',{name:'Change',exact:true})).toBeFocused()
  await expect(line).toHaveText('Codex on Store rewrite at studio · main · fresh worktree')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await start.click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts[0]).toMatchObject({tool:'codex', dir:'/home/dev/store'})
  expect(errors).toEqual([])
  await context.close()
  // At 320 px a long project name wraps the summary; Start stays on the screen.
  const long = fprojects().map(p => p.id === 'fp1' ? {...p, name:'the-store-rewrite-with-a-very-long-project-name-indeed'} : p)
  const narrow = await launch({width:320, height:700, touch:true, projects:long})
  await narrow.page.goto(base)
  await expect(narrow.page.locator('.summary-text')).toContainText('the-store-rewrite-with-a-very-long-project-name-indeed')
  const s = await startButton(narrow.page).boundingBox()
  expect(s.x + s.width).toBeLessThanOrEqual(320)
  const change = await bar(narrow.page).getByRole('button',{name:'Change',exact:true}).boundingBox()
  expect(change.x + change.width).toBeLessThanOrEqual(320)
  expect(await narrow.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(narrow.errors).toEqual([])
  await narrow.context.close()
 }

 // With the keyboard up while typing the prompt the tab bar steps aside and the prompt stays above the keyboard.
 if (run('launch prompt with keyboard open at 320 and 390 px')) {
  for (const width of [320, 390]) {
   const {context,page,errors} = await launch({width, height:844, touch:true})
   await page.addInitScript(() => {
    const events = new EventTarget(), real = window.visualViewport
    let height = null
    const visual = {get width() { return innerWidth }, get height() { return height ?? innerHeight }, offsetTop:0, offsetLeft:0, pageTop:0, pageLeft:0, scale:1,
     addEventListener:(...args) => { events.addEventListener(...args); real?.addEventListener(...args) }, removeEventListener:(...args) => { events.removeEventListener(...args); real?.removeEventListener(...args) }}
    Object.defineProperty(window, 'visualViewport', {configurable:true, get:() => visual})
    window.__keyboard = value => { height = value; events.dispatchEvent(new Event('resize')) }
   })
   await page.goto(base)
   const tabs = page.getByRole('navigation',{name:'Main navigation'})
   await expect(tabs).toBeVisible()
   const field = prompt(page)
   expect(await field.evaluate(el => parseFloat(getComputedStyle(el).fontSize)), 'prompt is 16px so focusing never zooms').toBeGreaterThanOrEqual(16)
   // The dock sits above the tab bar.
   const dock = await bar(page).boundingBox(), tabBox = await tabs.boundingBox()
   expect(dock.y + dock.height).toBeLessThanOrEqual(tabBox.y + 1)
   const top = page.locator('.shell-bar')
   await expect(top).toBeVisible()
   await field.focus()
   await page.evaluate(() => window.__keyboard(420))
   await expect(tabs).toBeHidden()
   // The top bar steps aside too (the spec's keyboard-open rule), so the prompt keeps the room above the keyboard.
   await expect(top).toBeHidden()
   await expect.poll(async () => { const b = await field.boundingBox(); return b.y + b.height }).toBeLessThanOrEqual(420)
   expect((await field.boundingBox()).y).toBeGreaterThanOrEqual(0)
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
   await page.evaluate(() => window.__keyboard(null))
   await expect(tabs).toBeVisible()
   await page.evaluate(() => window.__keyboard(420))
   await expect(tabs).toBeHidden()
   await field.blur()
   await expect(tabs).toBeVisible()
   await expect(top).toBeVisible()
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // Nothing of the prompt, the projects, the machines or the choices reaches browser storage; the last launch is read from the
 // sessions list.
 if (run('nothing in storage')) {
  const {context,page,errors,box} = await launch()
  await page.goto(base)
  await pick(page,'Agent','Codex')
  await pick(page,'Model','gpt-5-codex')
  await pick(page,'Branch','develop')
  await prompt(page).fill('Fix the failing tests')
  const dump = () => page.evaluate(() => JSON.stringify([Object.entries(localStorage), Object.entries(sessionStorage)]))
  for (const secret of SECRETS) expect(await dump(), secret).not.toContain(secret)
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  // The theme cache (#228) holds only light, dark or system, for the first paint; nothing else may be written.
  expect(box.storage.filter(w => w.key === 'reet-theme').every(w => ['light','dark','system'].includes(w.value))).toBe(true)
  expect(box.storage.filter(w => !/^reet\.home\.groupBy\./.test(w.key) && w.key !== 'reet-theme')).toEqual([])
  for (const secret of SECRETS) expect(JSON.stringify(box.storage), secret).not.toContain(secret)
  expect(page.url()).not.toContain('Fix')
  expect(errors).toEqual([])
  await context.close()
 }

 // With a mouse at phone and tablet widths the Change sheet's slot lists open as sheets above it, and Escape and Done always close.
 if (run('Change sheet works with a mouse')) {
  for (const width of [390, 600, 834]) {
   const {context,page,errors,box} = await launch({width, height:width === 834 ? 1112 : 844})
   await page.goto(base)
   const change = bar(page).getByRole('button',{name:'Change',exact:true})
   await change.click()
   const sheet = page.getByRole('dialog',{name:'Change the launch'})
   await expect(sheet).toBeVisible()
   const row = name => sheet.getByRole('button',{name:new RegExp(`^${name}: `)})
   await row('Agent').click()
   const agents = page.getByRole('dialog',{name:'Agent'}).getByRole('listbox',{name:'Agent'})
   await expect(agents, `${width}`).toBeVisible()
   await agents.getByRole('option',{name:/^Codex/}).click()
   await expect(row('Agent')).toHaveAccessibleName('Agent: Codex')
   await row('Model').click()
   await page.getByRole('listbox',{name:'Model'}).getByRole('option',{name:'gpt-5-codex'}).click()
   await row('Worktree').click()
   await page.getByRole('listbox',{name:'Worktree'}).getByRole('option',{name:'the checkout'}).click()
   // Typed fields in a list are 16 px at phone width whatever the pointer, so focusing never zooms.
   await row('Project').click()
   const find = page.getByRole('searchbox',{name:'Find a project'})
   if (width < 651) expect(await find.evaluate(el => parseFloat(getComputedStyle(el).fontSize)), `${width}`).toBeGreaterThanOrEqual(16)
   await page.keyboard.press('Escape')
   // Escape on a list closes only the list; Escape on the sheet closes it; Done closes it.
   await row('Agent').click()
   await expect(page.getByRole('listbox',{name:'Agent'})).toBeVisible()
   await page.keyboard.press('Escape')
   await expect(page.getByRole('listbox',{name:'Agent'})).toBeHidden()
   await expect(sheet).toBeVisible()
   await page.keyboard.press('Escape')
   await expect(sheet, `${width} Escape`).toBeHidden()
   await expect(change).toBeFocused()
   await change.click()
   await sheet.getByRole('button',{name:'Done',exact:true}).click()
   await expect(sheet, `${width} Done`).toBeHidden()
   // Done closes the sheet even with one of its lists still open (pressed from the page: the list is modal over it).
   await change.click()
   await row('Agent').click()
   await expect(page.getByRole('listbox',{name:'Agent'})).toBeVisible()
   await page.evaluate(() => [...document.querySelectorAll('dialog[open] button')].find(b => b.textContent.trim() === 'Done').click())
   await expect(sheet, `${width} Done over a list`).toBeHidden()
   await expect(page.getByRole('listbox',{name:'Agent'})).toBeHidden()
   await startButton(page).click()
   await expect(page).toHaveURL(/\/sessions\?/)
   expect(box.posts[0]).toMatchObject({tool:'codex', model:'gpt-5-codex', dir:'/home/dev/store'})
   expect(box.posts[0]).not.toHaveProperty('worktree')
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // A tablet's sentence is the drawing's: no model or worktree inline; both are in the Change sheet.
 if (run('tablet sentence matches the drawing')) {
  const {context,page,errors} = await launch({width:834, height:1112, touch:true})
  await page.goto(base)
  const visible = () => bar(page).locator('.sentence').evaluate(el => [...el.querySelectorAll('.part')].filter(p => p.getClientRects().length).map(p => p.textContent.trim()).join(' '))
  await expect.poll(visible).toBe('Run Claude Code on Store rewrite at studio from main')
  await expect(slot(page,'Model')).toBeHidden()
  await expect(slot(page,'Worktree')).toBeHidden()
  await bar(page).getByRole('button',{name:'Change',exact:true}).click()
  const sheet = page.getByRole('dialog',{name:'Change the launch'})
  await expect(sheet.getByRole('button',{name:'Model: default'})).toBeVisible()
  await expect(sheet.getByRole('button',{name:'Worktree: a fresh worktree'})).toBeVisible()
  // Words keep their spacing: a 44 px target does not push the sentence apart.
  await sheet.getByRole('button',{name:'Done'}).click()
  const gaps = await bar(page).locator('.sentence').evaluate(el => { const parts = [...el.querySelectorAll('.part')].filter(p => p.getClientRects().length); const text = p => { const r = document.createRange(); r.selectNodeContents(p); return r.getBoundingClientRect() }; const out = []; for (let i = 1; i < parts.length; i++) { const a = text(parts[i-1]), b = text(parts[i]); if (Math.abs(a.top - b.top) < 4) out.push(Math.round(b.left - a.right)) } return out })
  for (const g of gaps) expect(g, `gap ${gaps}`).toBeLessThanOrEqual(12)
  for (const name of ['Agent','Project','Branch']) { const b = await slot(page,name).boundingBox(); expect(Math.min(b.width, b.height), name).toBeGreaterThanOrEqual(44) }
  expect(errors).toEqual([])
  await context.close()
 }

 // Until the machines have been read, Start waits and says so: nothing is sent unchecked. A later refresh lets it go.
 if (run('Start waits for the machines')) {
  let reads = 0
  const {context,page,errors,box} = await launch({routes:{'/api/v2/terminal/machines': () => reads++ === 0 ? {status:500, json:{error:'down'}} : {machines:tmachines()}}})
  await page.clock.install()
  await page.goto(base)
  await expect.poll(() => sentenceText(page)).toContain('Store rewrite')
  await expect(startButton(page)).toBeDisabled()
  await expect(bar(page).locator('.status')).toHaveText('Start waits until your machines have been read.')
  await prompt(page).fill('Fix the failing tests')
  await prompt(page).press('Control+Enter')
  await settled(page)
  expect(box.posts).toEqual([])
  await page.clock.runFor(15_000)
  await expect(startButton(page)).toBeEnabled()
  await startButton(page).click()
  await expect.poll(() => box.posts.length).toBe(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // The docked bar never hides the control that has keyboard focus (WCAG 2.4.11): the page scrolls it clear.
 if (run('focus is never hidden under the bar')) {
  const many = [...fprojects(), ...['wiki','api','docs','blog'].map((n, i) => fproject(`fq${i}`, n, 'a1', `/home/dev/${n}`, {position:3 + i}))]
  const sessions = Array.from({length:12}, (_, i) => session(`r${i}`, 'a1', `Session number ${i}`, {tool:'claude', projectId:'fp1'}))
  for (const [width, height, touch, target] of [[834, 1112, true, 'aside.rail .item .more'], [1360, 800, false, '.flight a.row']]) {
   const {context,page,errors} = await launch({width, height, touch, projects:many, sessions})
   await page.goto(base)
   await expect(bar(page)).toBeVisible()
   await page.locator(target).last().waitFor()
   const last = await page.locator(target).count()
   // Tab forward from the page top until the last of them has focus.
   await page.locator('body').click({position:{x:1, y:1}})
   for (let i = 0; i < 200; i++) {
    await page.keyboard.press('Tab')
    if (await page.locator(target).nth(last - 1).evaluate(el => el === document.activeElement)) break
   }
   const hidden = await page.locator(target).nth(last - 1).evaluate(el => { const r = el.getBoundingClientRect(), f = document.querySelector('form[aria-label="Start a session"]').getBoundingClientRect(); return el === document.activeElement ? r.bottom > f.top + 1 : 'not focused' })
   expect(hidden, `${width}px`).toBe(false)
   // In flight's last row and Needs you can be scrolled clear of the docked bar.
   for (const [what, el] of [['In flight', page.locator('.flight a.row').last()], ['Needs you', page.getByRole('region',{name:'Needs you'})]]) {
    await expect(el, `${width}px ${what}`).toBeAttached()
    await el.evaluate(e => e.scrollIntoView({block:'end'}))
    const clear = await el.evaluate(e => { const r = e.getBoundingClientRect(), f = document.querySelector('form[aria-label="Start a session"]').getBoundingClientRect(); return r.bottom <= f.top + 1 })
    expect(clear, `${width}px ${what}`).toBe(true)
   }
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // The title is shown before sending, made from the prompt's first line.
 if (run('title preview')) {
  const {context,page,errors,box} = await launch()
  await page.goto(base)
  await expect(bar(page).getByText(/^Titled /)).toHaveCount(0)
  await prompt(page).fill('Fix the failing tests\nThen tidy up')
  await expect(bar(page).getByText('Titled “Fix the failing tests”',{exact:true})).toBeVisible()
  await startButton(page).click()
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.posts[0].title).toBe('Fix the failing tests')
  expect(errors).toEqual([])
  await context.close()
 }

 // Home's own profile read failing, or never answering, leaves the bar usable (each project's own base, worktrees by default).
 if (run('profile read failing leaves the bar usable')) {
  for (const how of ['fails', 'hangs']) {
   let reads = 0, release = () => {}
   const profile = {user:{id:1,name:'Sam',email:'sam@example.test',role:'owner'},sessionCount:1,accounts:true,profile:{theme:'light',timezone:'Europe/London',dateFormat:'',shelfView:'list',cardsExpanded:false,hostedPrefix:'reet-',updatedAt:1}}
   // The first read is the app's own; Home's is the second.
   const {context,page,errors,box} = await launch({routes:{'/api/v2/profile': () => reads++ === 0 ? profile : how === 'fails' ? {status:500, json:{error:'down'}} : new Promise(r => { release = () => r({status:500, json:{error:'late'}}) })}})
   await page.clock.install()
   await page.goto(base)
   if (how === 'hangs') { await expect.poll(() => reads).toBe(2); await page.clock.runFor(11_000) }
   await expect(startButton(page), how).toBeEnabled()
   await expect.poll(() => sentenceText(page)).toBe('Run Claude Code with default on Store rewrite at studio from main in a fresh worktree')
   await startButton(page).click()
   await expect.poll(() => box.posts.length).toBe(1)
   expect(errors).toEqual([])
   // The held answer is let go, so closing never waits on a route handler that never finishes.
   release()
   await context.close()
  }
 }

 console.log('Launch bar: Select and start in several projects (lost reply, status sheet, retry, touch, 320 px, strip count), one press with the last choices, each slot, unready machines, the starter prompt chip, lost replies and refusals, a -x folder, the locked bar and a card chosen while it is locked, a refusal kept across the same selection, Show run status going once all started or after a later start, Enter and Ctrl+Enter (and no key on touch), the phone Change sheet and the landscape phone with one Change, the keyboard-open prompt, Tab and Escape from a list, no card marked for the home folder, branch reads, the YOLO reason, Stop selecting on the strip, unread projects, a removed project under a kept request, a run that is gone and nothing in storage checked.')
} finally {
 await browser.close()
}
