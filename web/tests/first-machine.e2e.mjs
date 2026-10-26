// Connecting the first machine: every entry point opens the same "Add a machine" dialog, the step-by-step panel waits
// behind it, and the panel's main button says what it needs instead of sitting disabled.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, terminalMachine, active } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // Every "connect a machine" button leads to the dialog, not to a different flow.
 if (run('entries')) {
  let {context,page} = await open({width:1440})
  await page.goto(`${base}/control`)
  await expect(page.getByRole('link',{name:'Connect your first machine'})).toHaveAttribute('href','/machines#add-machine')
  await context.close()

  ;({context,page} = await open({width:1440,terminalStatus:active,terminalMachines:[terminalMachine('a','dev-laptop',true)]}))
  await page.goto(`${base}/control`)
  await expect(page.getByRole('link',{name:'Connect a machine'})).toHaveAttribute('href','/machines#add-machine')
  await context.close()

  ;({context,page} = await open({width:1440}))
  await page.goto(`${base}/terminals`)
  await expect(page.getByRole('link',{name:/Connect a machine/})).toHaveAttribute('href','/machines#add-machine')
  await context.close()
 }

 // Arriving from one of them: the dialog is open with the name field ready, and the second flow is not opened behind it.
 if (run('arrival')) {
  const {context,page,errors} = await open({width:1440})
  const panelOpen = () => page.locator('details#connect-machine').evaluate(el=>el.open)
  await page.goto(`${base}/machines#add-machine`)
  const dialog = page.getByRole('dialog',{name:'Add a machine'})
  await expect(dialog).toBeVisible()
  await expect(dialog.getByLabel('Machine name')).toBeFocused()
  // Let the page finish loading its inventory: that is when the panel used to open itself.
  await expect(page.getByRole('heading',{name:'Inventory'})).toBeVisible()
  await page.waitForLoadState('networkidle')
  expect(await panelOpen(), 'step-by-step panel while the dialog is open').toBe(false)
  await dialog.getByRole('button',{name:/Close/}).click()
  await expect(dialog).toHaveCount(0)
  expect(await panelOpen(), 'step-by-step panel after closing the dialog').toBe(false)
  // The panel is still there when asked for: "Prefer the step-by-step setup?" carries the typed name into it.
  await page.getByRole('button',{name:'Add a machine'}).first().click()
  await expect(dialog).toBeVisible()
  await dialog.getByLabel('Machine name').fill('studio-laptop')
  await dialog.getByRole('button',{name:/step-by-step/}).click()
  await expect.poll(panelOpen).toBe(true)
  await expect(page.locator('details#connect-machine').getByLabel('Machine name',{exact:true})).toHaveValue('studio-laptop')
  expect(errors).toEqual([])
  await context.close()
 }

 // The step-by-step panel's main button is enabled, and says what is missing when the name is empty.
 if (run('continue')) {
  const {context,page} = await open({width:1440})
  await page.goto(`${base}/machines#connect-machine`)
  const next = page.getByRole('button',{name:/Continue to authentication/})
  await expect(next).toBeVisible()
  await expect(next).toBeEnabled()
  await next.click()
  await expect(page.getByRole('alert').filter({hasText:'Name this machine to continue.'})).toBeVisible()
  const name = page.getByLabel(/Machine name/).first()
  await expect(name).toBeFocused()
  await expect(name).toHaveAttribute('aria-invalid','true')
  await name.fill('studio-laptop')
  await expect(page.getByText('Name this machine to continue.')).toHaveCount(0)
  await next.click()
  await expect(page.getByText('Step 2 · Authenticate')).toBeVisible()
  await context.close()
 }
 console.log('First machine: one entry dialog from every button, the panel waits behind it, Continue says what it needs.')
} finally {await browser.close()}
