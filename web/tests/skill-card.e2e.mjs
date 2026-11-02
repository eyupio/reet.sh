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
  await silent.context.close()
 }
 console.log('Skill card: Copy install leads when installed nowhere, the shelf step leads once installed, the prompt is available while collapsed, and the card says where the skill is set to sync.')
} finally {await browser.close()}
