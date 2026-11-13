// Where a session will start, said and confirmed before anything starts. A session in a saved repository gets a worktree
// cut from the project's base branch, which the machine fetches from GitHub first; when the local branch and GitHub's
// differ, Home's launch bar reads the base check, says what will happen and asks. Synthetic API answers throughout:
// every create the page sends is recorded, so a start that was not made is checked as well as one that was.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, modelList, modelFavourites, owner } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const START = 'abc1234def5678901234567890abcdef12345678'
const AHEAD = "main on studio has commits GitHub doesn't have. This session starts from the local main (abc1234)."
const caps = (extra = {}) => ({protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,
 worktree:true,agentsV2:true,yolo:true,branches:true,browse:true,models:true,tools:['claude','codex'],...extra})
// `ready` is read on every machine read, so a test can take a machine out of service while a dialog is open.
const tmachines = (ready={}) => [
 {agentId:'a1',host:'studio',ready:ready.a1 !== false,serverEnabled:true,capabilities:caps()},
 {agentId:'a2',host:'desk',ready:ready.a2 !== false,serverEnabled:true,capabilities:caps()},
]
const health = (ready={}) => ({serverNow:Date.now(),machines:[
 {agentId:'a1',host:'studio',state:ready.a1 === false ? 'unreachable' : 'connected',capabilities:caps()},
 {agentId:'a2',host:'desk',state:ready.a2 === false ? 'unreachable' : 'connected',capabilities:caps()},
]})
const fproject = (id, name, dir, over={}) => ({id,host:'a1',machine:'studio',dir,name,isRepo:true,baseBranch:'',worktreeDefault:'inherit',colour:'',defaultTool:'',defaultModel:'',
 defaultEffort:'',defaultYolo:false,hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1,...over})
const fprojects = () => [fproject('fp1','Store rewrite','/home/dev/store',{baseBranch:'main'}), fproject('fp2','Notes','/home/dev/notes',{isRepo:false,position:1})]
// What the machine's last scan reported, so the Machines page has a card for studio to start from.
const scope = () => ({id:'s1',host:'studio',name:'studio',kind:'agent',agent:'claude',project:'/home/dev/store',complete:true,scannedAt:now,lastSeen:now,
 skills:[],skillCount:0,os:'linux',arch:'amd64',version:'dev'})
const fleet = () => [{host:'studio',groups:[scope()],skillCount:0}]
const sync = () => ({environments:[scope()],machines:fleet(),runs:[],updateJobs:[],targets:[],jobs:[],
 agents:[{id:'a1',host:'studio',version:'1',lastSeen:Date.now(),paused:false,autoUpdate:false},
  {id:'a2',host:'desk',version:'1',lastSeen:Date.now(),paused:false,autoUpdate:false}]})
// A second saved repository, on the other machine, with its own base: a start there is a different question.
const pair = () => [fproject('fp1','Store rewrite','/home/dev/store',{baseBranch:'main'}),
 fproject('fp4','Site','/home/dev/site',{host:'a2',machine:'desk',baseBranch:'develop',position:1})]
// The answer for whichever folder was asked about.
const aheadFor = asked => asked.dir === '/home/dev/site'
 ? {base:'develop',relation:'ahead',local:START,start:START}
 : {base:'main',relation:'ahead',local:START,start:START}
const SITE_AHEAD = "develop on desk has commits GitHub doesn't have. This session starts from the local develop (abc1234)."
// The page reads its machines again when the browser tab becomes visible, so a machine taken out of service is noticed
// without waiting for the poll.
const reread = page => page.evaluate(() => { document.dispatchEvent(new Event('visibilitychange')) })

// A writer's Home with one saved repository on a ready machine. `check` answers the base check; every create and every
// check is recorded in `box`.
async function scene(options={}) {
 const box = {posts:[], checks:[], confirms:0}
 // The machines a test can take out of service: `ready.a1 = false` and a visibility change makes the page read it again.
 const ready = options.ready ?? {}
 const check = options.check ?? (() => ({base:'main',relation:'ahead',local:START,start:START}))
 const routes = {
  '/api/v2/machines/health': () => health(ready),
  '/api/v2/terminal/machines': () => ({machines:tmachines(ready)}),
  '/api/v2/settings/yolo': () => ({allowed:true, changedBy:'Owner', changedAt:Date.now()}),
  '/api/v2/terminal/sessions': async request => {
   if (request.method() !== 'POST') return {sessions:[]}
   const body = request.postDataJSON()
   box.posts.push(body)
   return {id:`new-${box.posts.length}`,agentId:body.agentId,title:body.title,state:'running',createdAt:now,updatedAt:now,exitCode:null}
  },
  '/api/v2/folder-projects': () => ({projects:options.projects ?? fprojects(), savedWaiting:0}),
  '/api/v2/profile/model-favourites': () => modelFavourites([]),
  '/api/v2/machines/a1/models': request => modelList({tool:new URL(request.url()).searchParams.get('tool') ?? 'claude'}),
  '/api/v2/machines/a2/models': request => modelList({tool:new URL(request.url()).searchParams.get('tool') ?? 'claude'}),
  '/api/v2/machines/a1/branches': () => ({current:'main', default:'main', branches:['main','develop'], more:false}),
  '/api/v2/machines/a2/branches': () => ({current:'develop', default:'develop', branches:['main','develop'], more:false}),
  '/api/v2/machines/a2/base-check': request => {
   const u = new URL(request.url())
   const asked = {method:request.method(), dir:u.searchParams.get('dir'), base:u.searchParams.get('base')}
   box.checks.push(asked)
   return check(asked, box.checks.length)
  },
  '/api/v2/machines/a1/base-check': request => {
   const u = new URL(request.url())
   const asked = {method:request.method(), dir:u.searchParams.get('dir'), base:u.searchParams.get('base')}
   box.checks.push(asked)
   return check(asked, box.checks.length)
  },
  // Answered at once: a request sent after the page's others, so a test knows they have all reached these routes.
  '/api/__e2e/sync': () => ({}),
  ...(options.routes ?? {}),
 }
 const opened = await openWorkspace(browser, {width:options.width ?? 1280, height:options.height ?? 900, theme:options.theme ?? 'light', touch:!!options.touch, sync:sync(), inventory:{environments:[scope()],machines:fleet()}, routes})
 await opened.page.addInitScript(()=>{if(localStorage.getItem('reet.rail.collapsed')===null)localStorage.setItem('reet.rail.collapsed','0')})
 // Every appearance of the confirmation is recorded here, so a start that was made without being asked about is caught
 // even once the page has moved on to the session.
 await opened.page.exposeFunction('__sawBaseConfirm', () => { box.confirms++ })
 await opened.page.addInitScript(() => {
  const show = HTMLDialogElement.prototype.showModal
  HTMLDialogElement.prototype.showModal = function (...args) {
   try { if ((this.textContent || '').includes('Check where this starts')) window.__sawBaseConfirm() } catch { /* the page is not under test here */ }
   return show.apply(this, args)
  }
 })
 // The session page is not under test: it answers with an empty page.
 await opened.page.route(url => new URL(url).pathname === '/sessions', route => route.fulfill({status:200, contentType:'text/html', body:'<title>session</title>'}))
 return {...opened, box}
}
const settled = page => page.evaluate(async () => { await new Promise(r => setTimeout(r, 0)); await fetch('/api/__e2e/sync') })
const bar = page => page.getByRole('form',{name:'Start a session'})
const prompt = page => bar(page).getByRole('textbox',{name:'Describe the job'})
const startButton = page => bar(page).getByRole('button',{name:/^(Start the session|Try the same request again)$/})
const confirm = page => page.getByRole('alertdialog',{name:'Check where this starts'})
// The other browser start path: the Start an agent dialog, opened from a machine card on Machines (the same dialog the
// project rail's + and the Sessions screen open), with the saved repository chosen in it.
const agentDialog = page => page.getByRole('dialog',{name:'Start an agent',exact:true})
const openStartAgent = async page => {
 await page.goto(`${base}/machines`)
 const card = page.locator('article').filter({has:page.getByRole('heading',{name:'studio',exact:true})})
 await card.getByRole('button',{name:'Start an agent',exact:true}).click()
 const dialog = agentDialog(page)
 await expect(dialog).toBeVisible()
 await dialog.getByRole('combobox',{name:'Project',exact:true}).selectOption('fp1')
 return dialog
}
const sentenceText = page => bar(page).locator('.sentence').evaluate(el => [...el.querySelectorAll('.part')].map(p => p.textContent.trim()).join(' '))
const slot = (page, name) => bar(page).getByRole('button',{name:new RegExp(`^${name}: `)})
const pick = async (page, name, option) => {
 await slot(page, name).click()
 const list = page.getByRole('listbox',{name})
 await expect(list).toBeVisible()
 await list.getByRole('option',{name:option}).click()
 await expect(list).toBeHidden()
}
// The bar opens in a modal; #launch opens it.
const openBar = async page => {
 await page.evaluate(() => { location.hash = 'launch' })
 await page.getByRole('dialog',{name:'Start a session'}).waitFor({timeout:10000})
 await expect.poll(() => sentenceText(page), {timeout:20000}).toContain('on Store rewrite at studio')
}

try {
 // The machine says the repository's main has commits GitHub doesn't: Start asks first, in the person's words, and
 // neither Escape nor Cancel starts anything. The same page, at both widths and in both themes.
 for (const screen of [{width:1280, theme:'light'}, {width:1280, theme:'dark'}, {width:390, height:780, theme:'light', touch:true}, {width:390, height:780, theme:'dark', touch:true}]) {
  if (!run(`asks before starting from a local base (${screen.width} px, ${screen.theme})`)) continue
  const {context,page,errors,box} = await scene(screen)
  await page.goto(base)
  await openBar(page)
  expect(await page.evaluate(() => document.documentElement.dataset.theme ?? 'light')).toBe(screen.theme)
  await prompt(page).fill('Fix the failing tests')
  await startButton(page).click()

  await expect(confirm(page)).toBeVisible()
  await expect(confirm(page)).toContainText(AHEAD)
  // What is happening is in words, not in a colour alone, and nothing is under way yet.
  await expect(confirm(page)).toContainText('Nothing has started yet.')
  const cancel = confirm(page).getByRole('button',{name:'Cancel'})
  const go = confirm(page).getByRole('button',{name:'Start session'})
  await expect(cancel).toBeVisible()
  await expect(go).toBeVisible()
  // Finger-sized on touch.
  // (Its own laid-out height: the dialog's opening animation scales the box for a moment.)
  if (screen.touch) for (const button of [cancel, go]) expect(await button.evaluate(el => el.offsetHeight)).toBeGreaterThanOrEqual(44)
  // The question is asked before anything is sent, and the check named the folder and the project's base.
  await settled(page)
  expect(box.posts).toEqual([])
  expect(box.checks).toEqual([{method:'GET', dir:'/home/dev/store', base:'main'}])
  // Focus never reaches the bar behind: the keyboard goes round the dialog (or out of the page) and back, never onto
  // Start, the prompt or a slot while the question is open.
  for (let i = 0; i < 6; i++) {
   await page.keyboard.press('Tab')
   const where = await page.evaluate(() => { const a = document.activeElement; return !a || a === document.body ? 'page' : a.closest('dialog[role=alertdialog]') ? 'dialog' : (a.getAttribute('aria-label') || a.textContent || a.tagName).slice(0, 40) })
   expect(where, `Tab ${i + 1} moved focus behind the dialog`).toMatch(/^(dialog|page)$/)
  }
  // Escape closes it, starts nothing and gives Start the keyboard back.
  await page.keyboard.press('Escape')
  await expect(confirm(page)).toBeHidden()
  await expect(startButton(page)).toBeFocused()
  await settled(page)
  expect(box.posts).toEqual([])

  // Asked again, Cancel does the same.
  await startButton(page).click()
  await expect(confirm(page)).toBeVisible()
  await confirm(page).getByRole('button',{name:'Cancel'}).click()
  await expect(confirm(page)).toBeHidden()
  await expect(startButton(page)).toBeFocused()
  await settled(page)
  expect(box.posts).toEqual([])
  // The prompt is still there to start with.
  await expect(prompt(page)).toHaveValue('Fix the failing tests')

  // Start session makes exactly one create, with the base the check asked about.
  await startButton(page).click()
  await expect(confirm(page)).toBeVisible()
  await confirm(page).getByRole('button',{name:'Start session'}).click()
  await expect.poll(() => box.posts.length).toBe(1)
  expect(box.posts[0]).toMatchObject({agentId:'a1', dir:'/home/dev/store', base:'main', tool:'claude', prompt:'Fix the failing tests', projectId:'fp1'})
  expect(box.posts[0].worktree).toMatch(/^[0-9a-f]{32}$/)
  // Three presses, three reads: the answer is never assumed from the last time.
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.checks).toHaveLength(3)
  expect(box.confirms).toBe(3)
  expect(errors).toEqual([])
  await context.close()
 }

 // The two sides agree: there is nothing to say, so the start is one press as before.
 if (run('a base that matches GitHub starts without a dialog')) {
  const {context,page,errors,box} = await scene({check:() => ({base:'main',relation:'same',local:START,remote:START,start:START})})
  await page.goto(base)
  await openBar(page)
  await prompt(page).fill('Fix the failing tests')
  await startButton(page).click()
  await expect.poll(() => box.posts.length).toBe(1)
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.confirms).toBe(0)
  expect(box.checks).toHaveLength(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // A check that cannot be answered never holds up a start: an older machine (409), a refused read (500) and a reply that
 // never comes (the browser's own ten-second deadline) each start the session with no dialog and no message about it.
 for (const [name, answer] of [
  ['an older machine', () => ({status:409, json:{error:'Update reet on that machine to check where a session starts.', code:'base_check_unsupported'}})],
  ['a machine that failed the read', () => ({status:500, json:{error:'The terminal connection is unavailable.'}})],
  // Nothing is ever sent back, so the browser gives up after ten seconds. This case really waits for them.
  ['a reply that never comes', () => new Promise(() => {})],
 ]) {
  if (!run(`a check ${name} never holds up the start`)) continue
  const {context,page,errors,box} = await scene({check:answer})
  await page.goto(base)
  await openBar(page)
  await prompt(page).fill('Fix the failing tests')
  await startButton(page).click()
  // The deadline case really waits out the ten seconds the browser gives the check.
  await expect.poll(() => box.posts.length, {timeout:25000}).toBe(1)
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.confirms).toBe(0)
  expect(box.checks).toHaveLength(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // The Start an agent dialog asks the same question, in the same words, with the same two answers. Both widths, both themes.
 for (const screen of [{width:1280, theme:'light'}, {width:1280, theme:'dark'}, {width:390, height:780, theme:'light', touch:true}, {width:390, height:780, theme:'dark', touch:true}]) {
  if (!run(`Start an agent asks before starting from a local base (${screen.width} px, ${screen.theme})`)) continue
  const {context,page,errors,box} = await scene(screen)
  const dialog = await openStartAgent(page)
  expect(await page.evaluate(() => document.documentElement.dataset.theme ?? 'light')).toBe(screen.theme)
  const start = dialog.getByRole('button',{name:'Start',exact:true})
  await start.click()

  await expect(confirm(page)).toBeVisible()
  await expect(confirm(page)).toContainText(AHEAD)
  await expect(confirm(page)).toContainText('Nothing has started yet.')
  await settled(page)
  expect(box.posts).toEqual([])
  expect(box.checks).toEqual([{method:'GET', dir:'/home/dev/store', base:'main'}])
  // Cancel starts nothing and gives the keyboard back to the Start it came from.
  await confirm(page).getByRole('button',{name:'Cancel'}).click()
  await expect(confirm(page)).toBeHidden()
  await expect(start).toBeFocused()
  await settled(page)
  expect(box.posts).toEqual([])
  // Escape does the same.
  await start.click()
  await expect(confirm(page)).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(confirm(page)).toBeHidden()
  await expect(start).toBeFocused()
  await settled(page)
  expect(box.posts).toEqual([])

  await start.click()
  await expect(confirm(page)).toBeVisible()
  await confirm(page).getByRole('button',{name:'Start session'}).click()
  await expect.poll(() => box.posts.length).toBe(1)
  expect(box.posts[0]).toMatchObject({agentId:'a1', dir:'/home/dev/store', base:'main', projectId:'fp1'})
  expect(box.posts[0].worktree).toMatch(/^[0-9a-f]{32}$/)
  expect(box.checks).toHaveLength(3)
  expect(box.confirms).toBe(3)
  expect(errors).toEqual([])
  await context.close()
 }

 // The same two quiet cases for that dialog: a base that matches GitHub, and a machine that cannot answer.
 for (const [name, answer] of [
  ['a base that matches GitHub', () => ({base:'main',relation:'same',local:START,remote:START,start:START})],
  ['an older machine', () => ({status:409, json:{error:'Update reet on that machine to check where a session starts.', code:'base_check_unsupported'}})],
 ]) {
  if (!run(`Start an agent starts without a dialog for ${name}`)) continue
  const {context,page,errors,box} = await scene({check:answer})
  const dialog = await openStartAgent(page)
  await dialog.getByRole('button',{name:'Start',exact:true}).click()
  await expect.poll(() => box.posts.length).toBe(1)
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.confirms).toBe(0)
  expect(box.checks).toHaveLength(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // A yes belongs to the start it was given for. The machine goes out of service while the question is open, so the start
 // it was answering is refused before anything is sent; choosing another repository and pressing Start must ask again.
 if (run('a confirmation never carries over to another start from the bar')) {
  const ready = {a1:true}
  const {context,page,errors,box} = await scene({projects:pair(), ready, check:aheadFor})
  await page.goto(base)
  await openBar(page)
  await prompt(page).fill('Fix the failing tests')
  await startButton(page).click()
  await expect(confirm(page)).toBeVisible()
  await expect(confirm(page)).toContainText(AHEAD)
  // studio stops taking sessions while the question is on screen.
  ready.a1 = false
  await reread(page)
  await expect(startButton(page)).toBeDisabled({timeout:15000})
  // The yes is for a start that can no longer happen: nothing is sent.
  await confirm(page).getByRole('button',{name:'Start session'}).click()
  await expect(confirm(page)).toBeHidden()
  await settled(page)
  expect(box.posts).toEqual([])
  // Another repository, on the machine that is still there: the question is asked again, in its own words.
  await pick(page, 'Project', 'Site')
  await expect.poll(() => sentenceText(page), {timeout:15000}).toContain('on Site at desk')
  await startButton(page).click()
  await expect(confirm(page)).toBeVisible()
  await expect(confirm(page)).toContainText(SITE_AHEAD)
  await settled(page)
  expect(box.posts).toEqual([])
  await confirm(page).getByRole('button',{name:'Start session'}).click()
  await expect.poll(() => box.posts.length).toBe(1)
  expect(box.posts[0]).toMatchObject({agentId:'a2', dir:'/home/dev/site', base:'develop', projectId:'fp4'})
  expect(errors).toEqual([])
  await context.close()
 }

 // The same for the Start an agent dialog: a yes for one repository is never spent on another.
 if (run('a confirmation never carries over to another start from the dialog')) {
  const ready = {a1:true}
  const {context,page,errors,box} = await scene({projects:pair(), ready, check:aheadFor})
  const dialog = await openStartAgent(page)
  await dialog.getByRole('button',{name:'Start',exact:true}).click()
  await expect(confirm(page)).toBeVisible()
  await expect(confirm(page)).toContainText(AHEAD)
  ready.a1 = false
  await reread(page)
  await expect(dialog.getByRole('button',{name:'Start',exact:true})).toBeDisabled({timeout:15000})
  await confirm(page).getByRole('button',{name:'Start session'}).click()
  await expect(confirm(page)).toBeHidden()
  await settled(page)
  expect(box.posts).toEqual([])
  // The other repository, on the machine that is still there.
  await dialog.getByRole('combobox',{name:'Project',exact:true}).selectOption('fp4')
  await expect(dialog.getByRole('combobox',{name:'Machine',exact:true})).toHaveValue('a2')
  await dialog.getByRole('button',{name:'Start',exact:true}).click()
  await expect(confirm(page)).toBeVisible()
  await expect(confirm(page)).toContainText(SITE_AHEAD)
  await settled(page)
  expect(box.posts).toEqual([])
  await confirm(page).getByRole('button',{name:'Start session'}).click()
  await expect.poll(() => box.posts.length).toBe(1)
  expect(box.posts[0]).toMatchObject({agentId:'a2', dir:'/home/dev/site', base:'develop', projectId:'fp4'})
  expect(errors).toEqual([])
  await context.close()
 }

 // A saved backlog plan is a person's start too: its Start reads the same check and asks the same question, and the
 // controller is only asked to start it once the person has said yes.
 if (run('a backlog plan asks before it starts')) {
  const draft = {id:'d1',title:'Fix the flaky test',prompt:'Fix the flaky login test',agentId:'a1',projectId:'fp1',dir:'/home/dev/store',tool:'claude',model:'',effort:'',
   images:[],imageCount:0,idea:false,yolo:false,worktree:true,base:'main',chat:false,trust:false,starting:false,version:1,createdAt:now,updatedAt:now}
  const starts = []
  const {context,page,errors,box} = await scene({routes:{
   '/api/v2/session-backlog': () => ({drafts:[draft]}),
   '/api/v2/session-backlog/d1': () => draft,
   '/api/v2/session-backlog/d1/start': request => {
    starts.push(request.postDataJSON())
    return {id:'new-d1',agentId:'a1',title:draft.title,state:'running',createdAt:now,updatedAt:now,exitCode:null}
   },
  }})
  // The full card, as the board's own fixture does: a compact card keeps its actions behind the fold.
  await page.addInitScript(() => { if (!localStorage.getItem('reet.sessionBoard.1')) localStorage.setItem('reet.sessionBoard.1', JSON.stringify({compact:false})) })
  await page.goto(base)
  const card = page.locator('article.session-card').filter({hasText:'Fix the flaky test'})
  const start = card.getByRole('button',{name:'Start',exact:true})
  await expect(start).toBeEnabled()
  await start.click()
  await expect(confirm(page)).toBeVisible()
  await expect(confirm(page)).toContainText(AHEAD)
  await settled(page)
  expect(starts).toEqual([])
  expect(box.checks).toEqual([{method:'GET', dir:'/home/dev/store', base:'main'}])
  await confirm(page).getByRole('button',{name:'Cancel'}).click()
  await expect(confirm(page)).toBeHidden()
  await expect(start).toBeFocused()
  await settled(page)
  expect(starts).toEqual([])
  await start.click()
  await expect(confirm(page)).toBeVisible()
  await confirm(page).getByRole('button',{name:'Start session'}).click()
  await expect.poll(() => starts.length).toBe(1)
  expect(starts[0]).toMatchObject({version:1, prompt:'Fix the flaky login test'})
  // The plan goes to the controller, which makes the session: the browser sends no create of its own.
  expect(box.posts).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 // The question is about exactly the branch the start will use. This start carries no base of its own (the project asks
 // for no worktree) and the project saved none, so no base is named: the controller names none either, and the machine
 // answers for the branch its own remote calls default. The account's default base is not put in its place.
 if (run('a start that carries no base asks about no base')) {
  const profile = () => ({user:owner, sessionCount:1, accounts:true, profile:{theme:'light', timezone:'Europe/London', dateFormat:'', shelfView:'list',
   cardsExpanded:false, hostedPrefix:'reet-', defaultBaseBranch:'develop', worktreesByDefault:true, updatedAt:now}})
  const {context,page,errors,box} = await scene({
   projects:[fproject('fp1','Store rewrite','/home/dev/store',{baseBranch:'', worktreeDefault:'no'})],
   routes:{'/api/v2/profile':profile},
   check:() => ({base:'main',relation:'same',local:START,remote:START,start:START}),
  })
  await page.goto(base)
  await openBar(page)
  await prompt(page).fill('Fix the failing tests')
  await startButton(page).click()
  await expect.poll(() => box.posts.length).toBe(1)
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.checks).toEqual([{method:'GET', dir:'/home/dev/store', base:null}])
  expect(box.posts[0].base).toBeUndefined()
  expect(box.posts[0].worktree).toBeUndefined()
  expect(box.confirms).toBe(0)
  expect(errors).toEqual([])
  await context.close()

  // The same start in a project that did save a base asks about that one: the create carries none, so the controller
  // fills the project's base in, and the question must be about the branch the session will really start from.
  const saved = await scene({
   projects:[fproject('fp1','Store rewrite','/home/dev/store',{baseBranch:'main', worktreeDefault:'no'})],
   routes:{'/api/v2/profile':profile},
   check:() => ({base:'main',relation:'same',local:START,remote:START,start:START}),
  })
  await saved.page.goto(base)
  await openBar(saved.page)
  await prompt(saved.page).fill('Fix the failing tests')
  await startButton(saved.page).click()
  await expect.poll(() => saved.box.posts.length).toBe(1)
  expect(saved.box.checks).toEqual([{method:'GET', dir:'/home/dev/store', base:'main'}])
  expect(saved.box.posts[0].base).toBeUndefined()
  expect(saved.errors).toEqual([])
  await saved.context.close()
 }

 // A folder that is not a repository is never isolated into a worktree, so there is nothing to check and nothing to ask.
 if (run('a folder that is no repository is never checked')) {
  const {context,page,errors,box} = await scene({projects:[fproject('fp2','Notes','/home/dev/notes',{isRepo:false})]})
  await page.goto(base)
  await page.evaluate(() => { location.hash = 'launch' })
  await page.getByRole('dialog',{name:'Start a session'}).waitFor({timeout:10000})
  await expect.poll(() => sentenceText(page), {timeout:20000}).toContain('on Notes at studio')
  await prompt(page).fill('Fix the failing tests')
  await startButton(page).click()
  await expect.poll(() => box.posts.length).toBe(1)
  await expect(page).toHaveURL(/\/sessions\?/)
  expect(box.confirms).toBe(0)
  expect(box.checks).toEqual([])
  expect(box.posts[0].worktree).toBeUndefined()
  expect(errors).toEqual([])
  await context.close()
 }

 console.log('Launch base: every browser start path says where a session starts and asks first — Home\'s launch bar and the Start an agent dialog, each at 1280 and 390 px in light and dark, and Start on a backlog plan. Escape and Cancel start nothing and return focus to the Start they came from, Start session sends exactly one create (or one plan start) with the project\'s base, a matching base and a failed or unsupported check start with no dialog, a start that carries no base is asked about the project\'s saved base or about no base at all (never the account\'s default), and a folder that is no repository is never checked.')
} finally {
 await browser.close()
}
