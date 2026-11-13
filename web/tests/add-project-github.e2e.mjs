// Add a project From GitHub: the Add project dialog's second choice. The person names a repository and a machine that can
// clone it; the machine clones it into ~/reet/repos/<owner>/<repo> and the controller saves the project. The page reads
// the clone every two seconds until it ends, whether or not the dialog is still open. Synthetic API answers throughout:
// the start and every read of the clone are recorded, and nothing runs on a machine.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, modelList, modelFavourites, owner } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const START = 'abc1234def5678901234567890abcdef12345678'
const caps = (extra = {}) => ({protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,
 worktree:true,agentsV2:true,yolo:true,branches:true,browse:true,models:true,tools:['claude'],...extra})
// studio can clone from GitHub; desk is an older reet that cannot.
const tmachines = (clone = {a1:true}) => [
 {agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:caps({repoClone:!!clone.a1})},
 {agentId:'a2',host:'desk',ready:true,serverEnabled:true,capabilities:caps({repoClone:!!clone.a2})},
]
const health = () => ({serverNow:Date.now(),machines:[
 {agentId:'a1',host:'studio',state:'connected',capabilities:caps()},
 {agentId:'a2',host:'desk',state:'connected',capabilities:caps()},
]})
const fproject = (id, name, dir, over={}) => ({id,host:'a1',machine:'studio',dir,name,isRepo:true,baseBranch:'',worktreeDefault:'inherit',colour:'',defaultTool:'',defaultModel:'',
 defaultEffort:'',defaultYolo:false,hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1,...over})
const notes = () => fproject('fp2','Notes','/home/dev/notes',{isRepo:false})
// The project the controller saves when the clone is done: a repository that records where it came from.
const cloned = (over={}) => fproject('fp9','reet.sh','/home/dev/reet/repos/eyupio/reet.sh',{baseBranch:'main',origin:'eyupio/reet.sh',cloned:true,position:1,...over})
const scope = () => ({id:'s1',host:'studio',name:'studio',kind:'agent',agent:'claude',project:'/home/dev/notes',complete:true,scannedAt:now,lastSeen:now,
 skills:[],skillCount:0,os:'linux',arch:'amd64',version:'dev'})
const sync = () => ({environments:[scope()],machines:[{host:'studio',groups:[scope()],skillCount:0}],runs:[],updateJobs:[],targets:[],jobs:[],
 agents:[{id:'a1',host:'studio',version:'1',lastSeen:Date.now(),paused:false,autoUpdate:false},{id:'a2',host:'desk',version:'1',lastSeen:Date.now(),paused:false,autoUpdate:false}]})

const SENT = {host:'a1', repo:'eyupio/reet.sh', name:''}
const CLONING = "Cloning eyupio/reet.sh on studio… You can close this; the project appears when it's done."
const INVALID = 'Write the repository as owner/name, or paste its github.com link.'

// A writer's Home. `box.outcome` is what the clone becomes once `box.finish` is set: the reads before that answer
// `cloning`. Every start and every read of the clone is recorded with its time.
async function scene(options={}) {
 const box = {starts:[], reads:[], finish:false, outcome:{state:'done', project:cloned()}, projects:options.projects ?? [notes()], start:options.start, posts:[], checks:[], confirms:0}
 const routes = {
  '/api/v2/machines/health': health,
  '/api/v2/terminal/machines': () => ({machines:tmachines(options.clone)}),
  '/api/v2/folder-projects': () => ({projects:box.projects, savedWaiting:0}),
  '/api/v2/folder-projects/from-github': request => {
   box.starts.push(request.postDataJSON())
   if (box.start) return box.start(request.postDataJSON())
   return {status:202, json:{cloneId:'c0ffee', dir:'~/reet/repos/eyupio/reet.sh'}}
  },
  '/api/v2/folder-projects/clones/c0ffee': () => {
   box.reads.push(Date.now())
   if (!box.finish) return {state:'cloning'}
   if (box.outcome.state === 'done' && !box.projects.some(p => p.id === box.outcome.project.id)) box.projects = [...box.projects, box.outcome.project]
   return typeof box.outcome === 'function' ? box.outcome() : box.outcome
  },
  '/api/v2/settings/yolo': () => ({allowed:true, changedBy:'Owner', changedAt:Date.now()}),
  '/api/v2/terminal/sessions': request => {
   if (request.method() !== 'POST') return {sessions:[]}
   const body = request.postDataJSON()
   box.posts.push(body)
   return {id:`new-${box.posts.length}`,agentId:body.agentId,title:body.title,state:'running',createdAt:now,updatedAt:now,exitCode:null}
  },
  '/api/v2/profile/model-favourites': () => modelFavourites([]),
  '/api/v2/machines/a1/models': () => modelList({tool:'claude'}),
  // Folder on a machine opens its browser on studio's home first.
  '/api/v2/machines/a1/browse': () => ({path:'/home/dev', home:'/home/dev', parent:'', repo:'none', repoTop:'', entries:[], more:false, partial:false, scanned:0, omitted:0}),
  '/api/v2/machines/a1/branches': () => ({current:'main', default:'main', branches:['main'], more:false}),
  '/api/v2/machines/a1/base-check': request => {
   const u = new URL(request.url())
   box.checks.push({dir:u.searchParams.get('dir'), base:u.searchParams.get('base')})
   return {base:'main',relation:'behind',local:'1111111111111111111111111111111111111111',remote:START,start:START}
  },
  ...(options.routes ?? {}),
 }
 const opened = await openWorkspace(browser, {width:options.width ?? 1280, height:options.height ?? 900, theme:options.theme ?? 'light', touch:!!options.touch,
  status:options.status, sync:sync(), inventory:{environments:[scope()],machines:sync().machines}, routes})
 await opened.page.addInitScript(() => { if (localStorage.getItem('reet.rail.collapsed') === null) localStorage.setItem('reet.rail.collapsed', '0') })
 // Home keeps Your machines in a folded section: open it as a person would.
 await opened.page.addInitScript(() => { setInterval(() => { for (const d of document.querySelectorAll('details.home-detail')) d.open = true }, 100) })
 await opened.page.exposeFunction('__sawBaseConfirm', () => { box.confirms++ })
 await opened.page.addInitScript(() => {
  const show = HTMLDialogElement.prototype.showModal
  HTMLDialogElement.prototype.showModal = function (...args) {
   try { if ((this.textContent || '').includes('Check where this starts')) window.__sawBaseConfirm() } catch { /* not under test */ }
   return show.apply(this, args)
  }
 })
 await opened.page.route(url => new URL(url).pathname === '/sessions', route => route.fulfill({status:200, contentType:'text/html', body:'<title>session</title>'}))
 return {...opened, box}
}

const rail = page => page.getByRole('complementary',{name:'Projects'})
const dialog = page => page.getByRole('dialog',{name:'Add a project',exact:true})
const tab = (page, name) => dialog(page).getByRole('tab',{name,exact:true})
const repoField = page => dialog(page).getByRole('textbox',{name:'Repository',exact:true})
const machineField = page => dialog(page).getByRole('combobox',{name:'Machine',exact:true})
const cloneButton = page => dialog(page).getByRole('button',{name:'Clone and add',exact:true})
const footClose = page => dialog(page).locator('footer').getByRole('button',{name:'Close',exact:true})
const errorSlot = page => dialog(page).locator('[role=tabpanel]:not([hidden]) footer .status')
const noSideways = page => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
// Add a project from the rail on a desktop, from studio's New project on a phone (where Home has no rail).
async function openAdd(page, touch) {
 if (!touch) { const add = rail(page).getByRole('button',{name:'Add a project'}); await add.click(); await expect(dialog(page)).toBeVisible(); return add }
 const plus = page.getByRole('region',{name:'Your machines',exact:true}).getByRole('button',{name:'New on studio'})
 await plus.click()
 await page.getByRole('menu',{name:'studio'}).getByRole('menuitem',{name:'New project…'}).click()
 await expect(dialog(page)).toBeVisible()
 return plus
}
async function toGitHub(page) {
 await tab(page, 'From GitHub').click()
 await expect(tab(page, 'From GitHub')).toHaveAttribute('aria-selected', 'true')
}

try {
 // The whole path by keyboard, at both widths and in both themes: the choice, the field, the machine already chosen, the
 // line saying where the clone goes, the status while it runs, and the project in the rail when it is done.
 for (const screen of [{width:1280, theme:'light'}, {width:1280, theme:'dark'}, {width:390, height:780, theme:'light', touch:true}, {width:390, height:780, theme:'dark', touch:true}]) {
  if (!run(`clone and add by keyboard (${screen.width} px, ${screen.theme})`)) continue
  const {context,page,errors,box,state} = await scene(screen)
  await page.goto(base)
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.theme ?? 'light')).toBe(screen.theme)
  const opener = await openAdd(page, screen.touch)
  const d = dialog(page)
  // Folder on a machine is the default, and it is today's form.
  await expect(tab(page, 'Folder on a machine')).toHaveAttribute('aria-selected', 'true')
  await expect(d.getByRole('textbox',{name:'Folder',exact:true})).toBeVisible()
  await expect(repoField(page)).toHaveCount(0)
  // The keyboard reaches the choice and moves between the two with the arrows.
  await tab(page, 'Folder on a machine').focus()
  await page.keyboard.press('ArrowRight')
  await expect(tab(page, 'From GitHub')).toBeFocused()
  await expect(tab(page, 'From GitHub')).toHaveAttribute('aria-selected', 'true')
  await expect(d.getByRole('textbox',{name:'Folder',exact:true})).toBeHidden()
  await page.keyboard.press('ArrowLeft')
  await expect(tab(page, 'Folder on a machine')).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('ArrowRight')
  await page.keyboard.press('Tab')
  await expect(repoField(page)).toBeFocused()
  await expect(d.getByText('owner/name, or a github.com link',{exact:true})).toBeVisible()
  // Only machines that can clone are offered, by name; with one, it is chosen already.
  await expect(machineField(page).locator('option')).toHaveText(['studio'])
  await expect(machineField(page)).toHaveValue('a1')
  await expect(cloneButton(page)).toBeDisabled()
  // What does not parse says nothing while it is typed, and the controller's sentence once the field is left.
  await page.keyboard.type('eyupio')
  await expect(d).not.toContainText(INVALID)
  await page.keyboard.press('Tab')
  await expect(d.getByText(INVALID,{exact:true})).toBeVisible()
  await expect(repoField(page)).toHaveAttribute('aria-invalid', 'true')
  await expect(cloneButton(page)).toBeDisabled()
  await repoField(page).focus()
  await page.keyboard.type('/reet.sh')
  await expect(d).not.toContainText(INVALID)
  await expect(d.getByText('reet will clone it to ~/reet/repos/eyupio/reet.sh on studio.',{exact:true})).toBeVisible()
  await expect(cloneButton(page)).toBeEnabled()
  if (screen.touch) {
   const small = await d.locator('button:visible, select:visible, input:visible').evaluateAll(els => els.map(el => [el.getAttribute('aria-label') || el.textContent.trim() || el.id, el.offsetHeight]).filter(([, h]) => h < 44))
   expect(small, 'touch targets under 44 px').toEqual([])
  }
  expect(await noSideways(page)).toBe(true)
  await page.keyboard.press('Enter')

  // Started: one request, and the form gives way to the status line, announced, with Close.
  await expect(d.getByText(CLONING,{exact:true})).toBeVisible()
  await expect(d.locator('[role=status]').filter({hasText:CLONING})).toHaveCount(1)
  expect(box.starts).toEqual([SENT])
  await expect(repoField(page)).toHaveCount(0)
  await expect(footClose(page)).toBeFocused()
  // Read every two seconds.
  await expect.poll(() => box.reads.length, {timeout:10000}).toBeGreaterThanOrEqual(2)
  expect(box.reads[1] - box.reads[0]).toBeGreaterThanOrEqual(1800)
  box.finish = true
  // Done: the dialog closes, focus is back where Add was pressed, and the project is chosen with its diamond and base.
  await expect(d).toBeHidden({timeout:10000})
  await expect(opener).toBeFocused()
  await expect(page.getByText('Sorted. reet.sh added.',{exact:true})).toBeVisible()
  if (!screen.touch) {
   const card = rail(page).getByRole('button',{name:/^reet\.sh, studio, main/})
   await expect(card).toBeVisible()
   await expect(card.locator('svg.icon.repo')).toHaveCount(1)
   await expect(card).toHaveAttribute('aria-current', 'true')
  } else await expect(page.locator('.scope')).toContainText('reet.sh')
  // Nothing more is read once it has ended.
  const reads = box.reads.length
  await page.waitForTimeout(2500)
  expect(box.reads.length).toBe(reads)
  expect(await noSideways(page)).toBe(true)
  // Nothing about the clone was kept in the browser.
  expect(await page.evaluate(() => JSON.stringify([Object.entries(localStorage), Object.entries(sessionStorage)]))).not.toContain('c0ffee')
  expect(errors).toEqual([])
  expect(state.unexpected).toEqual([])
  await context.close()
 }

 // Close while it clones: the clone goes on, Home keeps reading it, and the project is chosen in the rail when it is done.
 // Opening Add again meanwhile shows the clone on From GitHub instead of a second form.
 if (run('closing mid-clone keeps reading on Home')) {
  const {context,page,errors,box} = await scene()
  await page.goto(base)
  const add = await openAdd(page)
  await toGitHub(page)
  await repoField(page).fill('https://github.com/eyupio/reet.sh')
  await cloneButton(page).click()
  await expect(dialog(page).getByText(CLONING,{exact:true})).toBeVisible()
  await footClose(page).click()
  await expect(dialog(page)).toBeHidden()
  await expect(add).toBeFocused()
  const reads = box.reads.length
  await expect.poll(() => box.reads.length, {timeout:10000}).toBeGreaterThan(reads)
  await add.click()
  await toGitHub(page)
  await expect(dialog(page).getByText(CLONING,{exact:true})).toBeVisible()
  await expect(repoField(page)).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(dialog(page)).toBeHidden()
  expect(box.starts).toHaveLength(1)
  box.finish = true
  const card = rail(page).getByRole('button',{name:/^reet\.sh, studio, main/})
  await expect(card).toBeVisible({timeout:10000})
  await expect(card).toHaveAttribute('aria-current', 'true')
  await expect(page.getByText('Sorted. reet.sh added.',{exact:true})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // Home goes away mid-clone (a new page load forgets the clone): the project is simply in the list the next time Home reads it.
 if (run('reopening Home later shows the project')) {
  const {context,page,errors,box} = await scene()
  await page.goto(base)
  await openAdd(page)
  await toGitHub(page)
  await repoField(page).fill('eyupio/reet.sh')
  await cloneButton(page).click()
  await expect(dialog(page).getByText(CLONING,{exact:true})).toBeVisible()
  await footClose(page).click()
  await page.goto(`${base}/machines`)
  box.finish = true
  box.projects = [notes(), cloned()]
  const reads = box.reads.length
  await page.goto(base)
  const card = rail(page).getByRole('button',{name:/^reet\.sh, studio, main/})
  await expect(card).toBeVisible()
  await expect(card.locator('svg.icon.repo')).toHaveCount(1)
  // The new page load has no clone to read.
  await page.waitForTimeout(2500)
  expect(box.reads.length).toBe(reads)
  expect(errors).toEqual([])
  await context.close()
 }

 // A clone that fails with the dialog closed says so where Home says its other news.
 if (run('a failure after closing is said on Home')) {
  const {context,page,errors,box} = await scene()
  box.outcome = {state:'failed', code:'not_signed_in', sentence:"studio isn't signed in to GitHub. Run gh auth login there, then try again."}
  await page.goto(base)
  await openAdd(page)
  await toGitHub(page)
  await repoField(page).fill('eyupio/reet.sh')
  await cloneButton(page).click()
  await footClose(page).click()
  box.finish = true
  const notice = page.locator('.notice.bad')
  await expect(notice).toHaveText("studio isn't signed in to GitHub. Run gh auth login there, then try again.", {timeout:10000})
  await expect(notice.locator('xpath=..')).toHaveAttribute('role', 'status')
  expect(errors).toEqual([])
  await context.close()
 }

 // A clone that fails with the dialog open: the sentence in its error slot, and the form back as it was typed.
 for (const [code, sentence] of [
  ['not_signed_in', "studio isn't signed in to GitHub. Run gh auth login there, then try again."],
  ['no_access', "GitHub has no eyupio/reet.sh that studio's sign-in can read."],
  ['exists', '~/reet/repos/eyupio/reet.sh on studio already holds something else. Move it, or add that folder as a project.'],
  ['timed_out', 'Cloning eyupio/reet.sh took longer than 10 minutes and was stopped. Nothing was kept.'],
 ]) {
  if (!run(`a failed clone (${code}) returns to the form`)) continue
  const {context,page,errors,box} = await scene()
  box.outcome = {state:'failed', code, sentence}
  await page.goto(base)
  await openAdd(page)
  await toGitHub(page)
  await repoField(page).fill('eyupio/reet.sh')
  await dialog(page).getByRole('textbox',{name:/^Name/}).fill('Reet')
  await cloneButton(page).click()
  await expect(dialog(page).getByText(CLONING,{exact:true})).toBeVisible()
  expect(box.starts).toEqual([{...SENT, name:'Reet'}])
  box.finish = true
  await expect(errorSlot(page)).toHaveText(sentence, {timeout:10000})
  await expect(errorSlot(page)).toHaveAttribute('role', 'status')
  await expect(repoField(page)).toHaveValue('eyupio/reet.sh')
  await expect(dialog(page).getByRole('textbox',{name:/^Name/})).toHaveValue('Reet')
  await expect(cloneButton(page)).toBeFocused()
  await expect(page.locator('.notice.bad')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // Each refusal of the start is the controller's own sentence, in the error slot; the form stays as typed.
 for (const [status, code, sentence] of [
  [422, 'invalid_repo', INVALID],
  [409, 'repo_clone_unsupported', 'Update reet on studio to add projects from GitHub. It also needs git and the GitHub CLI.'],
  [409, 'machine_offline', "studio is offline. Try again when it's back."],
  [429, 'clone_limit', 'Five repositories are already being cloned. Wait for one to finish.'],
  [409, 'project_limit', 'You have 100 projects, the most reet keeps. Remove one first.'],
  [409, 'name_taken', 'Another project on this account already has that name.'],
  [403, 'read_only', "This account is read-only, so it can't use projects."],
 ]) {
  if (!run(`a refused start (${code}) shows its sentence`)) continue
  const field = {invalid_repo:'repo', name_taken:'name'}[code]
  const {context,page,errors,box} = await scene({start:() => ({status, json:{error:sentence, code, ...(field ? {field} : {})}})})
  await page.goto(base)
  await openAdd(page)
  await toGitHub(page)
  await repoField(page).fill('eyupio/reet.sh')
  await cloneButton(page).click()
  await expect(errorSlot(page)).toHaveText(sentence)
  await expect(repoField(page)).toHaveValue('eyupio/reet.sh')
  await expect(dialog(page).getByText(CLONING,{exact:true})).toHaveCount(0)
  expect(box.starts).toHaveLength(1)
  // Nothing was started, so nothing is read.
  await page.waitForTimeout(2500)
  expect(box.reads).toEqual([])
  // Changing a field clears the sentence.
  await repoField(page).fill('eyupio/reet')
  await expect(errorSlot(page)).toHaveText('')
  expect(errors).toEqual([])
  await context.close()
 }

 // The button waits for both a repository it can read and a machine; with two machines that can clone, none is chosen.
 if (run('Clone and add waits for a repository and a machine')) {
  const {context,page,errors,box} = await scene({clone:{a1:true, a2:true}})
  await page.goto(base)
  await openAdd(page)
  await toGitHub(page)
  await expect(machineField(page)).toHaveValue('')
  await expect(machineField(page).locator('option:not([disabled])')).toHaveText(['studio','desk'])
  await repoField(page).fill('eyupio/reet.sh')
  await expect(cloneButton(page)).toBeDisabled()
  await expect(dialog(page)).not.toContainText('reet will clone it')
  await machineField(page).selectOption('a2')
  await expect(dialog(page).getByText('reet will clone it to ~/reet/repos/eyupio/reet.sh on desk.',{exact:true})).toBeVisible()
  await expect(cloneButton(page)).toBeEnabled()
  for (const bad of ['http://github.com/eyupio/reet.sh', 'https://gitlab.com/eyupio/reet.sh', 'eyupio/reet.sh/', 'https://github.com:443/eyupio/reet.sh', 'eyupio/...']) {
   await repoField(page).fill(bad)
   await expect(cloneButton(page), bad).toBeDisabled()
  }
  // Enter on a repository it cannot read says why, and sends nothing.
  await repoField(page).press('Enter')
  await expect(dialog(page).getByText(INVALID,{exact:true})).toBeVisible()
  expect(box.starts).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 // No machine can clone yet: the line says what one needs, and there is no form.
 if (run('no machine can clone yet')) {
  const {context,page,errors} = await scene({clone:{}})
  await page.goto(base)
  await openAdd(page)
  await toGitHub(page)
  await expect(dialog(page).getByText('No machine here can clone from GitHub yet. It needs a current reet, git and the GitHub CLI.',{exact:true})).toBeVisible()
  await expect(repoField(page)).toHaveCount(0)
  await expect(cloneButton(page)).toHaveCount(0)
  await footClose(page).click()
  await expect(dialog(page)).toBeHidden()
  expect(errors).toEqual([])
  await context.close()
 }

 // A sign-in lost at the start goes back to sign in, as the other dialogs do.
 if (run('a lost sign-in returns to sign in')) {
  const {context,page,errors} = await scene({start:() => ({status:401, json:{error:'Sign in to use projects.', code:'signed_out'}})})
  await page.goto(base)
  await openAdd(page)
  await toGitHub(page)
  await repoField(page).fill('eyupio/reet.sh')
  await cloneButton(page).click()
  await expect(page).toHaveTitle('Sign in · reet.sh')
  await expect(dialog(page)).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // A read-only account never sees Add, so never From GitHub.
 if (run('a viewer never sees From GitHub')) {
  const {context,page,errors} = await scene({status:{user:{...owner, role:'viewer'}}})
  await page.goto(base)
  await expect(page.getByRole('heading',{name:'Home',exact:true})).toBeVisible()
  await expect(page.getByRole('button',{name:'Add a project'})).toHaveCount(0)
  await expect(page.getByRole('tab',{name:'From GitHub'})).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // A project reet cloned is never asked where a session starts: its checkout is reet's own, so a base behind GitHub
 // starts at once (Task 6's question is for folders the person keeps).
 if (run('a cloned project starts without the base question')) {
  const {context,page,errors,box} = await scene({projects:[cloned({position:0}), notes()]})
  await page.goto(base)
  await page.evaluate(() => { location.hash = 'launch' })
  const bar = page.getByRole('form',{name:'Start a session'})
  await expect.poll(() => bar.locator('.sentence').evaluate(el => el.textContent), {timeout:20000}).toContain('reet.sh')
  await bar.getByRole('textbox',{name:'Describe the job'}).fill('Fix the failing tests')
  await bar.getByRole('button',{name:/^Start the session$/}).click()
  await expect.poll(() => box.posts.length).toBe(1)
  expect(box.posts[0]).toMatchObject({agentId:'a1', dir:'/home/dev/reet/repos/eyupio/reet.sh', base:'main', projectId:'fp9'})
  expect(box.checks).toEqual([{dir:'/home/dev/reet/repos/eyupio/reet.sh', base:'main'}])
  expect(box.confirms).toBe(0)
  expect(errors).toEqual([])
  await context.close()
 }

 console.log('Add From GitHub: the choice by keyboard, only machines that can clone, the parse and the line saying where the clone goes, the status read every two seconds with the dialog open or closed, each refusal and failure in the dialog or on Home, no form without a machine that can clone, sign-out, viewers, and a cloned project that starts without the base question; 1280 and 390 px, light and dark.')
} finally {
 await browser.close()
}
