// A skill card leads with the action that changes a machine when nothing has the skill yet, and with its next shelf step once something does.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill, machineWith } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // Installed nowhere: Copy install leads, the shelf step is secondary, and the prompt is available while collapsed.
 if (run('not-installed')) {
  const {context,page,errors} = await open({width:1440,skills:[skill('code-review')]})
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('button',{name:'Save to test'})).toBeVisible()
  await expect(page.getByRole('button',{name:'Copy install command for code-review'})).toHaveClass(/primary/)
  await expect(page.getByRole('button',{name:'Save to test'})).not.toHaveClass(/move/)
  await expect(page.getByRole('button',{name:'Copy prompt for code-review'})).toBeVisible()
  await page.getByRole('button',{name:/^Expand/}).first().click()
  await expect(page.getByRole('button',{name:'Copy prompt for code-review'})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // Installed on a machine: the shelf step leads, Copy install is an ordinary button.
 if (run('installed')) {
  const mine = skill('code-review')
  const {context,page} = await open({width:1440,skills:[mine],inventory:machineWith('dev-laptop',[mine])})
  await page.goto(`${base}/skills`)
  await expect(page.getByText('dev-laptop',{exact:true}).first()).toBeVisible()
  await expect(page.getByRole('button',{name:'Save to test'})).toHaveClass(/move/)
  await expect(page.getByRole('button',{name:'Copy install command for code-review'})).not.toHaveClass(/primary/)
  await context.close()
 }
 // Saved sync selections: the card says where the skill is set to sync, apart from what scans observed, and the filters find it.
 if (run('sync-intent')) {
  const now = Date.now()
  const a = skill('code-review'), b = skill('release-notes')
  const inv = machineWith('dev-laptop',[a])
  const box = machineWith('build-box',[])
  const inventory = {environments:[...inv.environments,...box.environments],machines:[...inv.machines,...box.machines]}
  const scope = (id, host, skills) => ({id, host, agent:'agents', name:host, skills, skillCount:skills.length, complete:true, scannedAt:now, lastSeen:now, os:'linux', arch:'amd64', version:'test'})
  const sync = {environments:[scope('dev-laptop-agents','dev-laptop',[a]), scope('build-box-agents','build-box',[])], machines:inventory.machines, agents:[], jobs:[], runs:[], updateJobs:[],
   targets:[{target:scope('dev-laptop-agents','dev-laptop',[a]), sourceId:'shelf', sourceName:'Shelf', desired:[a,b], missing:[b], extra:[], revision:'r1', updatedAt:now, mode:'additive'}]}
  const {context,page,errors} = await open({width:1440,skills:[a,b],inventory,sync})
  await page.goto(`${base}/skills`)
  const card = id => page.locator(`[data-skill="${id}"]`)
  await expect(card(a.id).locator('.sync-intent')).toHaveText(/Set to sync to dev-laptop/)
  await expect(card(a.id).getByRole('link',{name:`Remove code-review from dev-laptop in Sync`})).toHaveAttribute('href',`/sync?machine=dev-laptop&remove=${encodeURIComponent(a.id)}`)
  await expect(card(b.id).locator('.sync-intent')).toHaveText(/Pending on dev-laptop/)
  // The filters narrow the shelf by intent and by observation.
  await page.locator('.shelf-filters summary').click()
  await page.getByRole('button',{name:'Sync pending'}).click()
  await expect(card(b.id)).toBeVisible(); await expect(card(a.id)).toHaveCount(0)
  await page.getByRole('button',{name:'Not on any machine'}).click()
  await expect(card(b.id)).toBeVisible(); await expect(card(a.id)).toHaveCount(0)
  await page.getByRole('button',{name:'Missing somewhere'}).click()
  await expect(card(a.id)).toBeVisible(); await expect(card(b.id)).toBeVisible()
  await page.getByRole('button',{name:'Not set to sync'}).click()
  await expect(card(a.id)).toHaveCount(0); await expect(card(b.id)).toHaveCount(0)
  await expect(page.getByRole('heading',{name:'Nowt matches just yet.'})).toBeVisible()
  await page.getByRole('button',{name:'Clear filters'}).click()
  await expect(card(a.id)).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }
 // Nothing saved: the line says so and offers the way to change it; without a sync answer there is no line at all.
 if (run('sync-intent-none')) {
  const mine = skill('code-review')
  const {context,page} = await open({width:1440,skills:[mine],inventory:machineWith('dev-laptop',[mine])})
  await page.goto(`${base}/skills`)
  const line = page.locator(`[data-skill="${mine.id}"] .sync-intent`)
  await expect(line).toHaveText(/Not set to sync/)
  await expect(line.getByRole('link',{name:'Sync to machines…'})).toHaveAttribute('href',`/sync?skill=${encodeURIComponent(mine.id)}`)
  await context.close()
  const silent = await open({width:1440,skills:[mine],inventory:machineWith('dev-laptop',[mine]),routes:{'/api/v2/sync':{status:500,json:{error:'down'}}}})
  await silent.page.goto(`${base}/skills`)
  await expect(silent.page.getByText('dev-laptop',{exact:true}).first()).toBeVisible()
  await expect(silent.page.locator('.sync-intent')).toHaveCount(0)
  // The intent filters are off rather than answering from nothing; the observed ones still work.
  await silent.page.locator('.shelf-filters summary').click()
  await expect(silent.page.getByRole('button',{name:'Not set to sync'})).toBeDisabled()
  await expect(silent.page.getByRole('button',{name:'Sync pending'})).toBeDisabled()
  await expect(silent.page.getByRole('button',{name:'Missing somewhere'})).toBeEnabled()
  await silent.context.close()
 }
 // Four machines: two installed, one missing, one unconfirmed. Collapsed, the card answers in one line and expands to the full set.
 if (run('large fleet summary')) {
  const mine = skill('code-review')
  const now = Date.now()
  const scope = (host, skills, extra = {}) => ({id:`${host}-agents`,host,agent:'agents',name:host,skills,skillCount:skills.length,complete:true,scannedAt:now,lastSeen:now,os:'linux',arch:'amd64',version:'test',...extra})
  const environments = [scope('one',[mine]), scope('two',[mine]), scope('three',[]), scope('four',[],{complete:false})]
  const machines = ['one','two','three','four'].map(host=>({host,groups:environments.filter(env=>env.host===host),skillCount:environments.filter(env=>env.host===host).reduce((n,env)=>n+env.skillCount,0)}))
  const {context,page,errors} = await open({width:1440,skills:[mine],inventory:{environments,machines}})
  await page.goto(`${base}/skills`)
  const card = page.locator(`[data-skill="${mine.id}"]`)
  const summary = card.getByRole('button',{name:/^Show all 4 machines/})
  await expect(summary).toHaveText(/On 2 of 4 · 1 missing · 1 not confirmed/)
  await expect(card.locator('.installation-badge')).toHaveCount(0)
  await summary.click()
  await expect(card.locator('.installation-badge')).toHaveCount(4)
  for (const host of ['one','two','three','four']) await expect(card.getByText(host,{exact:true})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // An unconfirmed scan offers Add now; the action queues a scan instead of pretending it can install without a fresh report.
 if (run('scan before add')) {
  const mine = skill('code-review')
  const now = Date.now()
  const old = {id:'pi-agents',host:'pi',agent:'agents',name:'pi',skills:[],skillCount:0,complete:false,scannedAt:now-15*86400000,lastSeen:now,os:'linux',arch:'amd64',version:'test'}
  const inventory = {environments:[old],machines:[{host:'pi',groups:[old],skillCount:0}]}
  const sync = {environments:[old],machines:[{host:'pi',groups:[old],skillCount:0}],agents:[{id:'pi-agent',host:'pi',version:'test',lastSeen:now,paused:false,autoUpdate:false}],targets:[],jobs:[],runs:[],updateJobs:[]}
  let scanBody = null
  const routes = {
   '/api/v2/sync': sync,
   '/api/v2/sync/candidates': {skills:[{...mine,source:'demo/toolkit',sourceType:'github'}]},
   '/api/v2/sync/jobs': request => { scanBody = request.postDataJSON(); return {...sync,jobs:[{id:'scan-1',agentId:'pi-agent',targetId:'',kind:'scan',status:'queued',revision:'',hash:'',plan:{},message:'',createdAt:now,updatedAt:now}]} },
  }
  const {context,page,errors} = await open({width:1440,skills:[mine],inventory,sync,routes})
  await page.goto(`${base}/skills`)
  const card = page.locator(`[data-skill="${mine.id}"]`)
  await card.getByRole('button',{name:'Scan pi and add code-review'}).click()
  await expect(page.getByText('Scan requested for pi. Add code-review once the scan is complete.',{exact:true})).toBeVisible()
  expect(scanBody).toEqual({agentId:'pi-agent',targetId:'',kind:'scan'})
  expect(errors).toEqual([])
  await context.close()
 }
 // The summary is a 44 px control on a phone and expanding it keeps the page at the device width.
 if (run('large fleet phone')) {
  const mine = skill('code-review')
  const now = Date.now()
  const scope = (host, skills, extra = {}) => ({id:`${host}-agents`,host,agent:'agents',name:host,skills,skillCount:skills.length,complete:true,scannedAt:now,lastSeen:now,os:'linux',arch:'amd64',version:'test',...extra})
  const environments = [scope('one',[mine]), scope('two',[mine]), scope('three',[]), scope('four',[],{complete:false})]
  const machines = ['one','two','three','four'].map(host=>({host,groups:environments.filter(env=>env.host===host),skillCount:environments.filter(env=>env.host===host).reduce((n,env)=>n+env.skillCount,0)}))
  for (const width of [390,320]) {
   const {context,page,errors} = await open({width,height:812,touch:true,skills:[mine],inventory:{environments,machines}})
   await page.goto(`${base}/skills`)
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`document width at ${width}px`).toBe(true)
   const summary = page.locator(`[data-skill="${mine.id}"]`).getByRole('button',{name:/^Show all 4 machines/})
   const box = await summary.boundingBox()
   expect(box.height,`summary height at ${width}px`).toBeGreaterThanOrEqual(44)
   await summary.click()
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`expanded document width at ${width}px`).toBe(true)
   expect(errors).toEqual([])
   await context.close()
  }
 }
 console.log('Skill card: Copy install leads when installed nowhere, the shelf step leads once installed, the prompt is available while collapsed, the card says where the skill is set to sync, a large fleet collapses to one summary, an unconfirmed machine is scanned before adding, and the summary works on a phone.')
} finally {await browser.close()}
