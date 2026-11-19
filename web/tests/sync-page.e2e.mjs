// The Sync page: one sentence for every machine, one row per machine with its state line, a Retry for what failed
// there and a Sync on / Paused switch, and what the machines did lately, newest first. A machine below the skills
// protocol says to update reet there and offers the update; one newer than this controller says to update the
// controller, with no Pause or Retry. The plan controls stay at Advanced.
import { chromium, expect } from '@playwright/test'
import { noWork, openWorkspace, owner } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const passed = []

const now = Date.now()
const min = 60_000
const machine = (agentId, host, state, over = {}) => ({agentId, host, paused:false, online:true, protocol:2, state, failed:[], waiting:[], lastSeen:now - 2 * min, ...over})
function world() {
 return {
  machines: [
   machine('a-think', 'ThinkPad', 'failed', {failed:[{name:'seo-cluster', detail:'No write access to ~/.claude/skills'}, {name:'seo-profound', detail:'Disk full'}]}),
   machine('a-elite', 'Elitebook', 'ok'),
   machine('a-studio', 'studio', 'offline', {online:false, lastSeen:now - 3 * 3600_000, waiting:['seo-bing', 'seo-profound']}),
   machine('a-build', 'build-box', 'paused', {paused:true}),
   machine('a-old', 'old-mac', 'needs-update', {protocol:1}),
   machine('a-new', 'future', 'needs-newer-controller', {protocol:3}),
  ],
  activity: [
   {at:now - 2 * min, host:'Elitebook', name:'seo-bing', action:'removed', detail:''},
   {at:now - 2 * min - 500, host:'ThinkPad', name:'seo-bing', action:'removed', detail:''},
   {at:now - 3600_000, host:'ThinkPad', name:'seo-cluster', action:'failed', detail:'No write access to ~/.claude/skills'},
   {at:now - 30 * 3600_000, host:'Elitebook', name:'askme', action:'updated', detail:'d2'},
  ],
  work: {failed:1, installing:0, removing:1, updating:1, needsUpdate:1, needsUpdateHost:'old-mac', needsNewerController:1, needsNewerControllerHost:'future', paused:1, pausedHost:'build-box'},
  active: 118,
  patches: [], retries: [], updates: [], reads: 0,
 }
}
const sync = {environments:[], machines:[], targets:[], jobs:[], runs:[], updateJobs:[], agents:[{id:'a-old', host:'old-mac', version:'0.9.0', lastSeen:now, paused:false, autoUpdate:false}], releases:{latest:{version:'1.4.0', tag:'v1.4.0'}}}
function routes(w) {
 return {
  '/api/v2/sync/skills': () => { w.reads++; return {machines:w.machines, activity:w.activity, work:w.work, active:w.active} },
  '/api/v2/sync/agents': req => {
   const b = req.postDataJSON()
   w.patches.push(b)
   w.machines = w.machines.map(m => m.agentId === b.id ? {...m, paused:b.action === 'pause', state:b.action === 'pause' ? 'paused' : 'ok'} : m)
   return sync
  },
  '/api/v2/sync/skills/retry': req => {
   const b = req.postDataJSON()
   w.retries.push(b)
   w.machines = w.machines.map(m => m.agentId === b.agentId ? {...m, state:'installing', failed:[], waiting:b.names} : m)
   return {names:b.names}
  },
  '/api/v2/sync/agents/update': req => { w.updates.push(req.postDataJSON()); return sync },
 }
}
async function open(options = {}, w = world()) {
 const f = await openWorkspace(browser, {width:1440, height:900, sync, ...options, routes:routes(w)})
 await f.page.goto(`${base}/sync`)
 await expect(f.page.getByRole('heading', {level:1})).toBeVisible()
 return {...f, w}
}
const row = (page, host) => page.getByRole('listitem').filter({has:page.getByRole('heading', {name:host, exact:true})})
// Visible text under 14 px inside main.
const under14 = page => page.evaluate(() => [...document.querySelectorAll('main *')]
 .filter(el => el.checkVisibility() && [...el.childNodes].some(n => n.nodeType === 3 && n.textContent.trim()) && parseFloat(getComputedStyle(el).fontSize) < 14)
 .map(el => `${el.tagName} "${el.textContent.trim().slice(0, 30)}"`))

try {
 // The heading is one sentence for every machine, as the Skills page's header says it.
 if (run('sentence')) {
  for (const [work, machines, sentence] of [
   [noWork, 1, 'Everything is in sync.'],
   [{...noWork, failed:1}, 1, '1 machine needs a look.'],
   [{...noWork, installing:2, updating:2}, 2, 'Installing on 2 machines…'],
   [{...noWork, removing:1, updating:1}, 1, 'Coming off 1 machine…'],
   [{...noWork, needsUpdate:1, needsUpdateHost:'old-mac'}, 1, 'Update reet on old-mac to keep its skills in sync.'],
   [{...noWork, needsNewerController:1, needsNewerControllerHost:'future'}, 1, 'future runs a newer reet than this controller; update the controller to keep its skills in sync.'],
   [noWork, 0, 'No machines yet.'],
  ]) {
   const w = world()
   w.work = work
   w.machines = w.machines.slice(1, 1 + machines)
   const {context, page, errors} = await open({}, w)
   await expect(page.getByRole('heading', {level:1})).toHaveText(sentence)
   expect(await page.title()).toMatch(/^Sync ·/)
   expect(errors).toEqual([])
   await context.close()
  }
  passed.push('sentence')
 }

 // One row per machine, each with its line; what failed says why; the activity is newest first.
 if (run('rows')) {
  const {context, page, errors, state} = await open()
  await expect(page.getByRole('heading', {level:1})).toHaveText('1 machine needs a look.')
  await expect(page.getByText("Your 118 Active skills go on every machine. Archived ones come off. That's the only rule. Sync is paused on build-box, so reet leaves its skills as they are.")).toBeVisible()
  const machines = page.getByRole('list', {name:'Machines'})
  await expect(machines.locator(':scope > li')).toHaveCount(6)
  await expect(row(page, 'ThinkPad')).toContainText('2 skills failed: seo-cluster and seo-profound')
  await expect(row(page, 'ThinkPad')).toContainText('seo-cluster: No write access to ~/.claude/skills')
  await expect(row(page, 'ThinkPad')).toContainText('seo-profound: Disk full')
  await expect(row(page, 'Elitebook')).toContainText('In sync · 118 skills · checked 2 min ago')
  await expect(row(page, 'studio')).toContainText("Offline · last seen 3 h ago · catches up when it's back")
  await expect(row(page, 'studio')).toContainText('Waiting: seo-bing and seo-profound.')
  await expect(row(page, 'build-box')).toContainText('Paused by you · left alone')
  await expect(row(page, 'old-mac')).toContainText('Update reet on old-mac to keep its skills in sync')
  await expect(row(page, 'old-mac').getByRole('button', {name:'Update reet on old-mac'})).toBeVisible()
  await expect(row(page, 'future')).toContainText('future runs a newer reet than this controller; update the controller to keep its skills in sync')
  // Only a machine with failures has Retry; one newer than this controller has neither Retry nor the switch.
  await expect(page.getByRole('button', {name:/^Retry/})).toHaveCount(1)
  await expect(row(page, 'future').getByRole('switch')).toHaveCount(0)
  await expect(row(page, 'future').getByRole('button')).toHaveCount(0)
  for (const [host, on] of [['ThinkPad', true], ['Elitebook', true], ['studio', true], ['build-box', false], ['old-mac', true]]) {
   const sw = row(page, host).getByRole('switch', {name:`Sync on ${host}`})
   await expect(sw).toHaveAttribute('aria-checked', String(on))
   await expect(sw).toContainText(on ? 'Sync on' : 'Paused')
  }
  const activity = page.getByRole('list', {name:'Recent activity'}).getByRole('listitem')
  await expect(activity).toHaveCount(3)
  await expect(activity.nth(0)).toContainText('2 minRemoved seo-bing from Elitebook and ThinkPad.')
  await expect(activity.nth(0).locator('strong')).toHaveText('seo-bing')
  await expect(activity.nth(1)).toContainText('1 hseo-cluster failed on ThinkPad: No write access to ~/.claude/skills.')
  await expect(activity.nth(2)).toContainText('YesterdayUpdated askme on Elitebook.')
  await expect(page.getByRole('link', {name:'Advanced', exact:true})).toHaveAttribute('href', '/sync/advanced')
  expect(state.unexpected).toEqual([])
  expect(errors).toEqual([])
  await context.close()
  passed.push('rows')
 }

 // The switch pauses and resumes: PATCH /api/v2/sync/agents with pause or resume, by pointer or keyboard.
 if (run('pause')) {
  const {context, page, errors, w} = await open()
  const sw = row(page, 'Elitebook').getByRole('switch', {name:'Sync on Elitebook'})
  await sw.click()
  await expect.poll(() => w.patches).toEqual([{id:'a-elite', action:'pause'}])
  await expect(sw).toHaveAttribute('aria-checked', 'false')
  await expect(row(page, 'Elitebook')).toContainText('Paused by you · left alone')
  await expect(page.getByRole('status').filter({hasText:'Paused sync on Elitebook.'})).toBeVisible()
  const paused = row(page, 'build-box').getByRole('switch', {name:'Sync on build-box'})
  await paused.focus()
  await page.keyboard.press('Space')
  await expect.poll(() => w.patches).toEqual([{id:'a-elite', action:'pause'}, {id:'a-build', action:'resume'}])
  await expect(paused).toHaveAttribute('aria-checked', 'true')
  await expect(paused).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
  passed.push('pause')
 }

 // Retry asks the machine to try every skill that failed there again, and says so.
 if (run('retry')) {
  const {context, page, errors, w} = await open()
  await row(page, 'ThinkPad').getByRole('button', {name:'Retry seo-cluster and seo-profound on ThinkPad'}).click()
  await expect.poll(() => w.retries).toEqual([{agentId:'a-think', names:['seo-cluster', 'seo-profound']}])
  await expect(page.getByRole('status').filter({hasText:'Trying seo-cluster and seo-profound again on ThinkPad.'})).toBeVisible()
  await expect(row(page, 'ThinkPad')).toContainText('Installing 2 skills…')
  await expect(page.getByRole('button', {name:/^Retry/})).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
  passed.push('retry')
 }

 // A machine below the skills protocol is offered the existing update: its review, then the update queued.
 if (run('update')) {
  const {context, page, errors, w} = await open()
  await row(page, 'old-mac').getByRole('button', {name:'Update reet on old-mac'}).click()
  const dialog = page.getByRole('dialog', {name:'Update reet on old-mac'})
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', {name:/^(Update|Start update)/}).click()
  await expect.poll(() => w.updates).toEqual([{id:'a-old', version:'latest'}])
  await expect(page.getByRole('status').filter({hasText:'Update queued for old-mac.'})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
  passed.push('update')
 }

 // A viewer sees where each machine stands, with nothing to press.
 if (run('viewer')) {
  const {context, page, errors} = await open({status:{user:{...owner, role:'viewer'}}})
  await expect(row(page, 'ThinkPad')).toContainText('2 skills failed')
  await expect(page.getByRole('button', {name:/^Retry/})).toHaveCount(0)
  await expect(page.getByRole('switch')).toHaveCount(0)
  await expect(row(page, 'build-box')).toContainText('Paused')
  expect(errors).toEqual([])
  await context.close()
  passed.push('viewer')
 }

 // At 1440, 1024 and 390 px, light and dark: nothing scrolls sideways, every target is 44 px, no text under 14 px.
 if (run('layout')) {
  for (const [width, theme] of [[1440, 'light'], [1440, 'dark'], [1024, 'light'], [1024, 'dark'], [390, 'light'], [390, 'dark']]) {
   const {context, page, errors} = await open({width, height:844, theme, touch:width === 390})
   await expect(row(page, 'ThinkPad')).toBeVisible()
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px ${theme}`).toBe(true)
   for (const el of await page.locator('main [role=switch], main .machines button, main .advanced a').all()) {
    const box = await el.boundingBox()
    expect(box.height, `${await el.textContent()} at ${width}px ${theme}`).toBeGreaterThanOrEqual(44)
    expect(box.x + box.width, `${await el.textContent()} right edge at ${width}px ${theme}`).toBeLessThanOrEqual(width)
   }
   expect(await under14(page), `${width}px ${theme}`).toEqual([])
   // Its colours are the theme's: the row card is not the page's background in either theme.
   const [bg, card] = await page.evaluate(() => [getComputedStyle(document.body).backgroundColor, getComputedStyle(document.querySelector('main .grid section')).backgroundColor])
   expect(card, `${width}px ${theme}`).not.toEqual(bg)
   expect(errors).toEqual([])
   await context.close()
  }
  passed.push('layout')
 }

 console.log(`Sync page passed: ${passed.join(', ')}`)
} finally {
 await browser.close()
}
