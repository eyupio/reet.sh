// The retired Projects page: /projects and /projects/<id> go to Home with a one-time note, an old Project id opens the
// folder project made from it (/?project=<id>), and the old runs are kept to read in Activity, where their worktrees can
// still be checked and removed. Synthetic API answers; nothing is started on a machine.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, owner } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const NOTE = 'Projects now live in the rail on Home.'
const caps = {protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,worktree:true,tools:['claude']}
const tmachines = [{agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:caps}]
const fproject = (id, name, dir) => ({id,host:'a1',machine:'studio',dir,name,isRepo:true,baseBranch:'',worktreeDefault:'inherit',defaultTool:'',defaultModel:'',hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1})
const member = (position, dir, over={}) => ({position,host:'a1',dir,tool:'claude',model:'',state:'started',code:'',sentence:'',sessionId:`old-${position}`,worktreePath:`/home/dev/.reet/worktrees/site/${position}`,worktreeRepo:'site',worktreeBranch:`reet/site-r1-${position}`,worktreeRun:'r1',worktreeProject:'site',sharesCheckout:false,attempt:1,...over})
const oldRun = () => ({id:'r1',projectId:'p1',projectName:'Site',state:'partial',startedAt:now-3*24*3600_000,useWorktrees:true,worktreeProject:'site',
 members:[member(0,'/home/dev/site'), member(1,'/home/dev/api',{state:'failed',code:'folder_unavailable',sentence:"That folder isn't on studio.",sessionId:'',worktreePath:'',worktreeBranch:'',worktreeRun:''})]})

// A writer's workspace with folder projects, the old Project p1 (made into fp1 and fp2) and its run.
async function workspace(options={}) {
 const box = {requests:[], removed:false}
 const routes = {
  '/api/v2/terminal/machines': {machines:tmachines},
  '/api/v2/terminal/sessions': {sessions:[]},
  '/api/v2/folder-projects': request => {
   const legacy = new URL(request.url()).searchParams.get('legacy')
   if (legacy === null) return {projects:[fproject('fp1','site','/home/dev/site'), fproject('fp2','api','/home/dev/api')], savedWaiting:0}
   return legacy === 'p1' ? {id:'fp1'} : {status:404, json:{error:"That project isn't on this account.", code:'project_not_found'}}
  },
  '/api/v2/projects': {projects:[{id:'p2',name:'Broken',defaultTool:'claude',defaultModel:'',useWorktrees:true,hasSharedPrompt:false,members:[],createdAt:1,updatedAt:1,version:1}, {id:'p1',name:'Site',defaultTool:'claude',defaultModel:'',useWorktrees:true,hasSharedPrompt:false,members:[{host:'a1',dir:'/home/dev/site',tool:'',model:'',position:0}],createdAt:1,updatedAt:1,version:1}]},
  '/api/v2/projects/p2/runs': {status:500, json:{error:'down'}},
  '/api/v2/projects/p1/runs': () => ({runs:[box.removed ? {...oldRun(), members:oldRun().members.map(m => m.position === 0 ? {...m, state:'removed'} : m)} : oldRun()]}),
  '/api/v2/project-runs/r1': () => box.removed ? {...oldRun(), members:oldRun().members.map(m => m.position === 0 ? {...m, state:'removed'} : m)} : oldRun(),
  '/api/v2/terminal/worktrees/status': {path:'/home/dev/.reet/worktrees/site/0',branch:'reet/site-r1-0',changed:0,unpushed:0,paths:[],unreferenced:0,hidden:0,nested:false,incomplete:false},
  '/api/v2/terminal/worktrees/remove': () => { box.removed = true; return {ok:true,path:'/home/dev/.reet/worktrees/site/0',branch:'reet/site-r1-0'} },
  '/api/events': {events:[{id:1,at:now,source:'migration',action:'projects-migrated',detail:JSON.stringify({project:'Site',folderProjects:['fp1','fp2']})}]},
  ...(options.routes ?? {}),
 }
 const opened = await openWorkspace(browser, {width:1360, height:900, ...options, routes})
 opened.page.on('request', r => { const u = new URL(r.url()); if (u.pathname.startsWith('/api/')) box.requests.push(`${r.method()} ${u.pathname}${u.search}`) })
 return {...opened, box}
}

try {
 // The old list address goes to Home and says where projects live now, once: a reload of Home does not say it again.
 if (run('old list redirects with the note')) {
  const {context,page,errors} = await workspace()
  await page.goto(`${base}/projects`)
  await expect(page).toHaveURL(`${base}/`)
  expect(await page.title()).toBe('Home · reet.sh')
  await expect(page.getByText(NOTE,{exact:true})).toBeVisible()
  await expect(page.getByRole('complementary',{name:'Projects'}).getByRole('button',{name:/^site, studio/})).toBeVisible()
  await page.reload()
  await expect(page.getByRole('complementary',{name:'Projects'}).getByRole('button',{name:/^site, studio/})).toBeVisible()
  await expect(page.getByText(NOTE,{exact:true})).toHaveCount(0)
  // Nothing of it was kept to say it again.
  expect(await page.evaluate(() => JSON.stringify([Object.entries(localStorage), Object.entries(sessionStorage)]))).not.toContain('rail')
  expect(errors).toEqual([])
  await context.close()
 }

 // An old Project's address opens Home with the folder project made from its first folder chosen; an id that made none
 // stays on Home with the note.
 if (run('old project id redirects to its folder project')) {
  const {context,page,errors,box} = await workspace()
  await page.goto(`${base}/projects/p1`)
  await expect(page).toHaveURL(`${base}/?project=fp1`)
  await expect(page.getByText(NOTE,{exact:true})).toBeVisible()
  const rail = page.getByRole('complementary',{name:'Projects'})
  await expect(rail.getByRole('button',{name:/^site, studio/})).toHaveAttribute('aria-current','true')
  await expect(rail.getByRole('button',{name:/^api, studio/})).not.toHaveAttribute('aria-current','true')
  expect(box.requests).toContain('GET /api/v2/folder-projects?legacy=p1')
  await page.goto(`${base}/projects/nope/`)
  await expect(page.getByText(NOTE,{exact:true})).toBeVisible()
  await expect.poll(() => box.requests.includes('GET /api/v2/folder-projects?legacy=nope')).toBe(true)
  await expect(page).toHaveURL(`${base}/`)
  await expect(rail.locator('[aria-current="true"]')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // A malformed old address (a broken escape) still goes to Home with the note; the app does not fail to start.
 if (run('a malformed old address goes to Home')) {
  const {context,page,errors,box} = await workspace()
  // The preview server refuses the address itself; reet's server serves the app for it, so the page is served here.
  await page.route(url => new URL(url).pathname === '/projects/%E0', async route => route.fulfill({response:await route.fetch({url:`${base}/`})}))
  await page.goto(`${base}/projects/%E0`)
  await expect(page).toHaveURL(`${base}/`)
  await expect(page.getByText(NOTE,{exact:true})).toBeVisible()
  await expect(page.getByRole('complementary',{name:'Projects'}).getByRole('button',{name:/^site, studio/})).toBeVisible()
  expect(box.requests.filter(r => r.includes('legacy='))).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 // Activity keeps the old runs to read: each opens its run, with no Retry; a worktree of a run whose session has ended can
 // still be checked and removed through the existing sheet.
 if (run('old run is read-only in Activity and its worktree can be checked and removed')) {
  const {context,page,errors,box} = await workspace()
  await page.goto(`${base}/activity`)
  await expect(page.getByText('Moved saved Project Site into the rail as 2 projects',{exact:true})).toBeVisible()
  const runs = page.getByRole('region',{name:'Runs from Projects'})
  // One saved Project's runs could not be read: the others are still listed, and it says which.
  await expect(runs).toContainText("Couldn't read the runs of Broken just now.")
  await expect(runs).toContainText('the newest 20 runs of each saved Project')
  const head = runs.getByRole('button',{name:/^Site/})
  await expect(head).toHaveAttribute('aria-expanded','false')
  await head.click()
  await expect(head).toHaveAttribute('aria-expanded','true')
  await expect(runs).toContainText("That folder isn't on studio.")
  await expect(runs.getByRole('button',{name:/^Retry/})).toHaveCount(0)
  await runs.getByRole('button',{name:'Remove worktree for studio /home/dev/site'}).click()
  const sheet = page.getByRole('dialog',{name:'Remove worktree',exact:true})
  await expect(sheet).toContainText('No uncommitted changes')
  await sheet.getByRole('button',{name:/^Remove with no unpushed commits/}).click()
  await expect.poll(() => box.removed).toBe(true)
  await expect(sheet.getByRole('button',{name:'Done'})).toBeVisible()
  await sheet.getByRole('button',{name:'Done'}).click()
  await expect(runs.getByRole('button',{name:'Remove worktree for studio /home/dev/site'})).toHaveCount(0)
  // Nothing here starts or retries a run.
  expect(box.requests.filter(r => /^POST \/api\/v2\/(projects\/[^/]+\/runs|project-runs\/)/.test(r))).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 // A viewer has no Projects runs to look at: Activity shows their own activity only, and no run is read.
 if (run('viewer sees no Runs from Projects')) {
  const {context,page,errors,box} = await workspace({status:{user:{...owner,role:'viewer'}}})
  await page.goto(`${base}/activity`)
  await expect(page.getByText('Moved saved Project Site into the rail as 2 projects',{exact:true})).toBeVisible()
  await expect(page.getByRole('region',{name:'Runs from Projects'})).toHaveCount(0)
  expect(box.requests.filter(r => r.includes('/api/v2/projects'))).toEqual([])
  // The old address takes a viewer to Home too, with the note and no rail.
  await page.goto(`${base}/projects/p1`)
  await expect(page).toHaveURL(`${base}/`)
  await expect(page.getByText(NOTE,{exact:true})).toBeVisible()
  await expect(page.getByRole('complementary',{name:'Projects'})).toHaveCount(0)
  expect(box.requests.filter(r => r.includes('legacy='))).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }
 console.log('Projects retired: /projects and /projects/<id> go to Home with the note once, an old id opens its folder project, and old runs are read-only in Activity with Check and remove worktree; viewers get no runs.')
} finally { await browser.close() }
