// Where a session will start, said and confirmed before anything starts. A session in a saved repository gets a worktree
// cut from the project's base branch, which the machine fetches from GitHub first; when the local branch and GitHub's
// differ, Home's launch bar reads the base check, says what will happen and asks. Synthetic API answers throughout:
// every create the page sends is recorded, so a start that was not made is checked as well as one that was.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, modelList, modelFavourites } from './helpers/workspace.mjs'
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
const tmachines = () => [{agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:caps()}]
const health = () => ({serverNow:Date.now(),machines:[{agentId:'a1',host:'studio',state:'connected',capabilities:caps()}]})
const fproject = (id, name, dir, over={}) => ({id,host:'a1',machine:'studio',dir,name,isRepo:true,baseBranch:'',worktreeDefault:'inherit',colour:'',defaultTool:'',defaultModel:'',
 defaultEffort:'',defaultYolo:false,hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1,...over})
const fprojects = () => [fproject('fp1','Store rewrite','/home/dev/store',{baseBranch:'main'}), fproject('fp2','Notes','/home/dev/notes',{isRepo:false,position:1})]
const sync = () => ({environments:[],machines:[],runs:[],updateJobs:[],targets:[],jobs:[],
 agents:[{id:'a1',host:'studio',version:'1',lastSeen:Date.now(),paused:false,autoUpdate:false}]})

// A writer's Home with one saved repository on a ready machine. `check` answers the base check; every create and every
// check is recorded in `box`.
async function scene(options={}) {
 const box = {posts:[], checks:[], confirms:0}
 const check = options.check ?? (() => ({base:'main',relation:'ahead',local:START,start:START}))
 const routes = {
  '/api/v2/machines/health': health(),
  '/api/v2/terminal/machines': () => ({machines:tmachines()}),
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
  '/api/v2/machines/a1/branches': () => ({current:'main', default:'main', branches:['main','develop'], more:false}),
  '/api/v2/machines/a1/base-check': request => {
   const u = new URL(request.url())
   box.checks.push({method:request.method(), dir:u.searchParams.get('dir'), base:u.searchParams.get('base')})
   return check(box.checks.length)
  },
  // Answered at once: a request sent after the page's others, so a test knows they have all reached these routes.
  '/api/__e2e/sync': () => ({}),
  ...(options.routes ?? {}),
 }
 const opened = await openWorkspace(browser, {width:options.width ?? 1280, height:options.height ?? 900, theme:options.theme ?? 'light', touch:!!options.touch, sync:sync(), routes})
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
const sentenceText = page => bar(page).locator('.sentence').evaluate(el => [...el.querySelectorAll('.part')].map(p => p.textContent.trim()).join(' '))
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

 // A check that cannot be answered never holds up a start: an older machine (409), a refused read (500) and a reply
 // that never comes each start the session with no dialog and no message about it.
 for (const [name, answer] of [
  ['an older machine', () => ({status:409, json:{error:'Update reet on that machine to check where a session starts.', code:'base_check_unsupported'}})],
  ['a machine that failed the read', () => ({status:500, json:{error:'The terminal connection is unavailable.'}})],
 ]) {
  if (!run(`a check ${name} never holds up the start`)) continue
  const {context,page,errors,box} = await scene({check:answer})
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

 console.log('Launch base: the dialog says where a session starts and asks (1280 and 390 px, light and dark), Escape and Cancel start nothing and return focus to Start, Start session sends one create, a matching base and a failed or unsupported check start with no dialog, and a folder that is no repository is never checked.')
} finally {
 await browser.close()
}
