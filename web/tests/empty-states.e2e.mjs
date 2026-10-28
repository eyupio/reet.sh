// Pages that depend on a connected machine say so, rather than showing a full form that cannot work yet.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, freshInstanceSync, terminalMachine, active } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // Sync with nothing connected: one message and one way forward, not four dropdowns and greyed-out fleet buttons.
 // (A fresh instance reports its own server scan, which is not a machine the person connected.)
 if (run('sync')) {
  for (const width of [1440,375]) {
   const {context,page,errors} = await open({width,sync:freshInstanceSync()})
   await page.goto(`${base}/sync`)
   await expect(page.getByRole('heading',{name:'Connect a machine to start syncing'})).toBeVisible()
   await expect(page.getByRole('link',{name:'Connect a machine'})).toHaveAttribute('href','/machines#add-machine')
   await expect(page.getByText('Fleet controls')).toHaveCount(0)
   await expect(page.getByText('Choose what to sync')).toHaveCount(0)
   await expect(page.locator('main .btn.primary'), `filled buttons at ${width}px`).toHaveCount(1)
   expect(errors).toEqual([])
   await context.close()
  }
  // The only next step on the empty page must be tappable on a phone, not hidden under the fixed tab bar.
  for (const width of [320, 375, 390]) {
   const {context,page,errors} = await open({width,height:700,touch:true,sync:freshInstanceSync()})
   await page.goto(`${base}/sync`)
   const cta = page.getByRole('link',{name:'Connect a machine'})
   await expect(cta).toBeVisible()
   const box = await cta.boundingBox()
   expect(box, `CTA box at ${width}px`).toBeTruthy()
   expect(box.y + box.height, `CTA bottom at ${width}px`).toBeLessThanOrEqual(700)
   const hit = await page.evaluate(({x,y}) => {
    const element = document.elementFromPoint(x, y)
    return !!element?.closest('a[href="/machines#add-machine"]')
   }, {x: box.x + box.width / 2, y: box.y + box.height / 2})
   expect(hit, `CTA hit-test at ${width}px`).toBe(true)
   expect(errors).toEqual([])
   await context.close()
  }
  // A viewer cannot connect machines: the message stays, the button does not.
  const viewer = await open({width:1440,sync:freshInstanceSync(),status:{user:{id:2,name:'Pat',email:'pat@example.test',role:'viewer'}}})
  await viewer.page.goto(`${base}/sync`)
  await expect(viewer.page.getByRole('heading',{name:'Connect a machine to start syncing'})).toBeVisible()
  await expect(viewer.page.getByRole('link',{name:'Connect a machine'})).toHaveCount(0)
  await viewer.context.close()
 }

 // Sync with a machine that has agents: the working form, as before.
 if (run('sync-connected')) {
  const scope = {id:'laptop-agents',host:'dev-laptop',agent:'agents',name:'dev-laptop',skills:[],skillCount:0,complete:true,scannedAt:Date.now(),lastSeen:Date.now(),os:'linux',arch:'amd64',version:'test'}
  const {context,page} = await open({width:1440,sync:{environments:[scope],machines:[{host:'dev-laptop',groups:[scope],skillCount:0}],agents:[],targets:[],jobs:[],runs:[],updateJobs:[]}})
  await page.goto(`${base}/sync`)
  await expect(page.getByText('Choose what to sync')).toBeVisible()
  await expect(page.getByRole('heading',{name:'Connect a machine to start syncing'})).toHaveCount(0)
  await context.close()
 }

 // Setups with none saved: the steps lead; how they are stored is there when wanted.
 if (run('setups')) {
  const {context,page} = await open({width:1440})
  await page.goto(`${base}/profiles`)
  await expect(page.getByRole('heading',{name:'No setups yet.'})).toBeVisible()
  await expect(page.getByText('reet setup profile save work')).toBeVisible()
  await expect(page.getByText(/The synced profile is metadata/)).toBeHidden()
  await page.getByText('How setups are stored',{exact:true}).click()
  await expect(page.getByText(/The synced profile is metadata/)).toBeVisible()
  await context.close()
 }
 console.log('Empty states: Sync asks for a machine first, the Sync form is unchanged once one exists, Setups keeps the storage detail one click away.')
} finally {await browser.close()}
