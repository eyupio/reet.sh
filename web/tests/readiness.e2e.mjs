// Terminal setup guidance lives inside the one machine-setup flow: the server switch, then the machine's own consent.
// There is no separate readiness checklist competing with it on Machines.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, terminalMachine, active } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)
const panel = page => page.locator('details#connect-machine')
const stepButton = (page,name) => panel(page).locator('nav.steps').getByRole('button',{name:new RegExp(name)})
const serverBlock = page => panel(page).locator('.server-terminal')

try {
 // Nothing connected and terminals stopped: the flow still says how to start them, and nothing claims it is ready.
 if (run('first')) {
  for (const width of [1440,375]) {
   const {context,page,errors} = await open({width})
   await page.goto(`${base}/machines#connect-machine`)
   await expect(panel(page)).toHaveJSProperty('open',true)
   await expect(page.locator('section.readiness')).toHaveCount(0)
   await stepButton(page,'Connect terminal').click()
   await expect(serverBlock(page)).toContainText('Remote access is stopped')
   await expect(serverBlock(page)).toContainText('reet terminal start')
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // The server's own reason is shown in words, not a bare "not ready".
 if (run('reasons')) {
  for (const [reason,text] of [['https_required','no usable HTTPS public URL'],['accounts_required','not using accounts authentication']]) {
   const {context,page} = await open({width:1440,terminalStatus:{configured:true,available:false,reason}})
   await page.goto(`${base}/machines#connect-machine`)
   await stepButton(page,'Connect terminal').click()
   await expect(serverBlock(page)).toContainText(text)
   await context.close()
  }
 }

 // A running server plus a ready machine: the step says so and offers the way in.
 if (run('ready')) {
  const {context,page} = await open({width:1440,terminalStatus:active,terminalMachines:[terminalMachine('a','dev-laptop',true)]})
  await page.goto(`${base}/machines#connect-machine`)
  await panel(page).getByLabel('Machine name',{exact:true}).fill('dev-laptop')
  await stepButton(page,'Connect terminal').click()
  await expect(serverBlock(page)).toContainText('Controller access is enabled')
  await expect(panel(page)).toContainText('dev-laptop is ready for terminals.')
  await expect(panel(page).getByRole('link',{name:/Open terminals for dev-laptop/})).toBeVisible()
  await context.close()
 }

 // A viewer cannot run terminals, so gets no terminal setup block at all.
 if (run('viewer')) {
  const {context,page} = await open({width:1440,status:{user:{id:2,name:'Pat',email:'pat@example.test',role:'viewer'}}})
  await page.goto(`${base}/machines#connect-machine`)
  await stepButton(page,'Connect terminal').click()
  await expect(serverBlock(page)).toHaveCount(0)
  await context.close()
 }
 console.log('Readiness: terminal guidance inside the setup flow, server reasons in words, ready state and viewer.')
} finally {await browser.close()}
