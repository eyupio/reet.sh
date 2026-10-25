// First run on Control: say what stands between a new owner and a working terminal, with one clear next step,
// and show nothing that cannot work yet.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, terminalMachine, active } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)
const step = (page, name) => page.locator(`[data-step="${name}"]`)
const heading = page => page.getByRole('heading',{name:'Get your first terminal running'})

try {
 // Nothing connected and terminals stopped: one filled button, the command to start terminals, nothing that cannot work.
 if (run('first')) {
  for (const width of [1440,375]) {
   const {context,page,errors} = await open({width})
   await page.goto(base)
   await expect(heading(page)).toBeVisible()
   await expect(step(page,'controller')).toHaveAttribute('data-state','todo')
   await expect(step(page,'controller')).toContainText('Remote access is stopped')
   await expect(step(page,'controller')).toContainText('reet terminal start')
   await expect(step(page,'machine')).toHaveAttribute('data-state','todo')
   await expect(page.locator('main .btn.primary'), `filled buttons at ${width}px`).toHaveCount(1)
   await expect(page.locator('main .btn.primary')).toHaveText('Connect your first machine')
   await expect(page.getByRole('link',{name:/Open terminals/})).toHaveCount(0)
   await expect(page.getByRole('link',{name:/Open workspace/})).toHaveCount(0)
   await expect(page.getByText('0 active sessions')).toHaveCount(0)
   await expect(page.getByRole('heading',{name:/Your machines/})).toHaveCount(0)
   await expect(page.locator('section[aria-label="Workspace shortcuts"]')).toHaveCount(0)
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // The reason terminals are blocked is shown in words.
 if (run('reasons')) {
  for (const [reason,text] of [['https_required','no usable HTTPS public URL'],['accounts_required','not using accounts authentication']]) {
   const {context,page} = await open({width:1440,terminalStatus:{configured:true,available:false,reason}})
   await page.goto(base)
   await expect(step(page,'controller')).toContainText(text)
   await expect(step(page,'controller')).toHaveAttribute('data-state','todo')
   await context.close()
  }
 }

 // Steps complete as things happen: server on, machine connected but not yet allowed, then ready.
 if (run('progress')) {
  let {context,page} = await open({width:1440,terminalStatus:active})
  await page.goto(base)
  await expect(heading(page)).toBeVisible()
  await expect(step(page,'controller')).toHaveAttribute('data-state','done')
  await expect(step(page,'machine')).toHaveAttribute('data-state','todo')
  await context.close()

  ;({context,page} = await open({width:1440,terminalStatus:active,terminalMachines:[terminalMachine('a','dev-laptop',false)]}))
  await page.goto(base)
  await expect(heading(page)).toBeVisible()
  await expect(step(page,'machine')).toHaveAttribute('data-state','done')
  await expect(step(page,'consent')).toHaveAttribute('data-state','todo')
  await expect(step(page,'consent')).toContainText('reet agent terminal setup')
  await expect(page.getByRole('article',{name:'Machine dev-laptop'})).toBeVisible()
  await context.close()
 }

 // A ready machine on a running server: no checklist, the working view.
 if (run('ready')) {
  const {context,page} = await open({width:1440,terminalStatus:active,terminalMachines:[terminalMachine('a','dev-laptop',true)]})
  await page.goto(base)
  await expect(page.getByRole('article',{name:'Machine dev-laptop'})).toBeVisible()
  await expect(heading(page)).toHaveCount(0)
  await expect(page.getByRole('link',{name:/Open terminals/})).toBeVisible()
  await context.close()
 }

 // A viewer cannot run terminals, so is not walked through setting them up.
 if (run('viewer')) {
  const {context,page} = await open({width:1440,status:{user:{id:2,name:'Pat',email:'pat@example.test',role:'viewer'}}})
  await page.goto(base)
  await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
  await expect(heading(page)).toHaveCount(0)
  await context.close()
 }
 console.log('Readiness: checklist on first run, reasons in words, steps complete in order, hidden once ready and for viewers.')
} finally {await browser.close()}
