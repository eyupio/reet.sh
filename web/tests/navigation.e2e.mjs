// Navigation contract: the areas the landing page sells are one click away, notifications live in the account menu,
// and each area keeps one name wherever it appears.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill, terminalMachine, active } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)
const main = page => page.locator('nav[aria-label="Main navigation"] .desktop')

try {
 // Skills and Setups are top level; the rest sit under More.
 if (run('structure')) {
  const {context,page,errors} = await open({width:1440})
  await page.goto(base)
  await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
  const top = await main(page).locator(':scope > a, :scope > details > summary').allInnerTexts()
  expect(top.map(t=>t.trim())).toEqual(['Control','Machines','Terminals','Skills','Setups','More'])
  await main(page).locator('summary',{hasText:'More'}).click()
  const more = await main(page).locator('.group-links a').allInnerTexts()
  expect(more.map(t=>t.trim())).toEqual(['Sync','Lists','Community','Support','Teams & organisations','Host skills'])
  await expect(page.locator('.notif-link')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // The current area is marked, including inside More.
 if (run('current')) {
  for (const [path,link] of [['/skills','Skills'],['/profiles','Setups'],['/machines','Machines'],['/terminals','Terminals']]) {
   const {context,page} = await open({width:1440})
   await page.goto(`${base}${path}`)
   await expect(main(page).getByRole('link',{name:link,exact:true})).toHaveAttribute('aria-current','page')
   await context.close()
  }
  const {context,page} = await open({width:1440})
  await page.goto(`${base}/sync`)
  await main(page).locator('summary',{hasText:'More'}).click()
  await expect(main(page).locator('summary',{hasText:'More'})).toHaveClass(/current/)
  await expect(main(page).locator('.group-links').getByRole('link',{name:'Sync',exact:true})).toHaveAttribute('aria-current','page')
  await context.close()
 }

 // Notifications are in the account menu, with the unread count on the menu button and beside the link.
 if (run('notifications')) {
  const {context,page} = await open({width:1440,unread:3})
  await page.goto(base)
  const menu = page.getByRole('group').locator('summary[aria-label^="Account menu"]')
  await expect(menu).toHaveAttribute('aria-label',/3 unread notifications/)
  await menu.click()
  const link = page.locator('.panel').getByRole('link',{name:/^Notifications/})
  await expect(link).toHaveAttribute('href','/notifications')
  await expect(link).toContainText('3')
  await context.close()
  const quiet = await open({width:1440,unread:0})
  await quiet.page.goto(base)
  await expect(quiet.page.locator('summary[aria-label^="Account menu"]')).not.toHaveAttribute('aria-label',/unread/)
  await quiet.page.goto(`${base}/notifications`)
  await expect(quiet.page.locator('summary[aria-label^="Account menu"]')).toHaveClass(/active/)
  await quiet.context.close()
 }

 // One name per area: Control, Skills, Setups, Sync.
 if (run('names')) {
  // A machine is connected, so Control shows its shortcut cards (a first-run Control shows only the setup steps).
  const {context,page} = await open({width:1440,skills:[skill('code-review')],terminalStatus:active,terminalMachines:[terminalMachine('a','dev-laptop',true)]})
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('heading',{name:'Skills.',exact:true})).toBeVisible()
  expect(await page.title()).toMatch(/^Skills ·/)
  await expect(page.getByRole('link',{name:'Sync skills',exact:true})).toHaveAttribute('href','/sync')
  await expect(page.getByRole('link',{name:'Distribute skills'})).toHaveCount(0)
  await page.goto(base)
  const shortcuts = page.locator('section[aria-label="Workspace shortcuts"]')
  await expect(shortcuts.getByRole('link',{name:/^Setups/})).toHaveAttribute('href','/profiles')
  await expect(shortcuts.getByRole('link',{name:/^Sync/})).toHaveAttribute('href','/sync')
  await expect(page.getByText('Reusable AI setups')).toHaveCount(0)
  await expect(page.getByText('Skills & fleet sync')).toHaveCount(0)
  await page.goto(`${base}/machines`)
  await expect(page.locator('main').getByText('Control',{exact:true})).toBeVisible()
  await expect(page.getByText('Your control centre')).toHaveCount(0)
  const tabs = await page.locator('nav[aria-label="Machines"] a').allInnerTexts()
  expect(tabs.map(t=>t.trim())).toEqual(['Inventory & connection','Sync','Terminals','Setups →'])
  await context.close()
 }
 // Light, dark or system can be chosen from the account menu, takes effect at once, and is saved to the profile.
 if (run('theme')) {
  const {context,page,state} = await open({width:1440})
  await page.goto(base)
  await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
  await page.locator('summary[aria-label^="Account menu"]').click()
  const group = page.locator('.panel').getByRole('group',{name:'Theme'})
  await expect(group.getByRole('button',{name:'Light'})).toHaveAttribute('aria-pressed','true')
  await group.getByRole('button',{name:'Dark'}).click()
  await expect(group.getByRole('button',{name:'Dark'})).toHaveAttribute('aria-pressed','true')
  expect(await page.evaluate(()=>document.documentElement.dataset.theme)).toBe('dark')
  await expect.poll(()=>state.patches).toContainEqual({theme:'dark'})
  await group.getByRole('button',{name:'Match this device'}).click()
  expect(await page.evaluate(()=>document.documentElement.dataset.theme)).toBeUndefined()
  await expect.poll(()=>state.patches).toContainEqual({theme:'system'})
  await context.close()
 }

 // The signed-in header is one row on a phone and does not overflow at the widths either side of the menu breakpoint.
 if (run('header')) {
  for (const [width,maxHeight] of [[375,100],[1001,110],[1024,110]]) {
   const {context,page} = await open({width,unread:12})
   await page.goto(base)
   await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
   const m = await page.evaluate(()=>({overflow:document.documentElement.scrollWidth-innerWidth,height:document.querySelector('.site-header').getBoundingClientRect().height}))
   expect(m.overflow, `header overflow at ${width}px`).toBeLessThanOrEqual(0)
   expect(m.height, `header height at ${width}px`).toBeLessThanOrEqual(maxHeight)
   await context.close()
  }
 }
 console.log('Navigation: Skills and Setups top level, More contents, current marking, notifications in the account menu, one name per area.')
} finally {await browser.close()}
