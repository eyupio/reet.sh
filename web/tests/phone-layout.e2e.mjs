// Phone-width and touch contract for controls that were hidden, squashed or mislabelled.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace, skill, terminalMachine } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // Every shelf filter is reachable on a phone: nothing sits past the right edge behind a hidden scroll.
 if (run('filters')) {
  const {context,page,errors} = await open({width:375,skills:[skill('code-review')]})
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('group',{name:'Filter and sort'})).toBeVisible()
  for (const name of ['All','In use','To test','On the shelf','Liked','Select']) {
   const box = await page.locator('.filters button',{hasText:name}).first().boundingBox()
   expect(box, `${name} filter exists`).not.toBeNull()
   expect(box.x + box.width, `${name} filter right edge`).toBeLessThanOrEqual(375)
   expect(box.x, `${name} filter left edge`).toBeGreaterThanOrEqual(0)
  }
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
 }

 // The custom hosted-skill prefix field is wide enough to type in.
 if (run('prefix')) {
  const {context,page,errors} = await open({width:375})
  await page.goto(`${base}/account`)
  const input = page.getByLabel('Hosted skill name prefix',{exact:true})
  await expect(input).toBeVisible()
  const box = await input.boundingBox()
  expect(box.width, 'prefix field width at 375px').toBeGreaterThanOrEqual(140)
  expect(box.x + box.width, 'prefix field right edge').toBeLessThanOrEqual(375)
  expect(errors).toEqual([])
  await context.close()
 }

 // The password Show button is a touch-sized target.
 if (run('showbutton')) {
  const {context,page} = await open({width:375,status:{user:null,setupRequired:true}})
  await page.goto(`${base}/setup`)
  const box = await page.getByRole('button',{name:'Show password'}).first().boundingBox()
  expect(box.height, 'Show button height').toBeGreaterThanOrEqual(44)
  expect(box.width, 'Show button width').toBeGreaterThanOrEqual(44)
  await context.close()
 }

 // "Start using" only marks a skill as in use; the label says so, since installing is a separate action.
 if (run('cardlabel')) {
  const {context,page} = await open({width:1440,skills:[skill('code-review')]})
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('button',{name:'Save to test'})).toBeVisible()
  await expect(page.getByRole('button',{name:'Mark in use'})).toBeVisible()
  await expect(page.getByRole('button',{name:'Start using'})).toHaveCount(0)
  await context.close()
 }
 // The buttons on a machine card are finger-sized on a touch screen, and stay compact with a mouse.
 if (run('cardtouch')) {
  for (const [touch,check] of [[true,h=>expect(h,'card button height on touch').toBeGreaterThanOrEqual(44)],[false,h=>expect(h,'card button height with a mouse').toBeLessThanOrEqual(40)]]) {
   const {context,page,errors} = await open({width:touch?390:1440,touch,terminalStatus:active,terminalMachines:[terminalMachine('a1','dev-box',true)]})
   await page.goto(base)
   const card = page.getByRole('article',{name:'Machine dev-box'})
   await expect(card).toBeVisible()
   const heights = await card.locator('a.btn, button').evaluateAll(els=>els.map(el=>el.getBoundingClientRect().height))
   expect(heights.length, 'the card has buttons to measure').toBeGreaterThan(0)
   for (const h of heights) check(h)
   expect(errors).toEqual([])
   await context.close()
  }
 }
 console.log('Phone layout: shelf filters reachable, prefix field usable, Show button touch-sized, card label honest, machine card buttons finger-sized on touch.')
} finally {await browser.close()}
