// Connecting the first machine: one entry point on Machines opens the "Add a machine" dialog, the step-by-step panel
// waits behind it, and the panel's main button says what it needs instead of sitting disabled.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, terminalMachine, active } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // Machines has one connect action; the other pages still deep-link into it.
 if (run('entries')) {
  let {context,page} = await open({width:1440})
  await page.goto(`${base}/machines`)
  await expect(page.getByRole('button',{name:'Add a machine',exact:true})).toBeVisible()
  await expect(page.getByRole('link',{name:'Connect your first machine'})).toHaveCount(0)
  await expect(page.getByRole('button',{name:'Connect a machine',exact:true})).toHaveCount(0)
  await context.close()

  ;({context,page} = await open({width:1440}))
  await page.goto(`${base}/sessions`)
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
 // The key step is an enrolment code, made on a button, and never a personal token. From the first machine's own entry.
 if (run('code')) {
  const CODE = 'reet_enrol_FIXTURE_CODE_9f3a'
  const calls = [], tokenCalls = []
  let life = 900, refuse
  const routes = {
   '/api/v2/sync/enrol-codes': request => { calls.push(request.method()); return refuse ? { status: refuse.status, json: { error: refuse.error } } : { status: 201, json: { code: CODE, expiresAt: Math.floor(Date.now() / 1000) + life } } },
   '/api/v2/tokens': request => { tokenCalls.push(request.method()); return { status: 500, json: { error: 'Unexpected token call.' } } },
  }
  const {context,page,errors} = await open({width:390,theme:'dark',routes,scale:2})
  await page.goto(`${base}/machines#add-machine`)
  const dialog = page.getByRole('dialog',{name:'Add a machine'})
  await expect(dialog).toBeVisible()
  const key = dialog.locator('section[aria-labelledby$="-key"]')
  await expect(key.getByRole('heading',{name:'Give it an enrolment code'})).toBeVisible()
  await expect(dialog).not.toContainText(/machine token/i)
  const shot = async name => { if (process.env.REET_E2E_SHOTS) await key.screenshot({path:`${process.env.REET_E2E_SHOTS}/first-${name}-390-dark.png`}) }
  await shot('before')
  expect(calls).toEqual([])
  // A refusal is the server's sentence by the button.
  refuse = {status:429,error:'you have 20 unused enrolment codes; use one or wait for them to expire'}
  await key.getByRole('button',{name:'Make an enrolment code'}).click()
  await expect(key.getByRole('alert')).toHaveText(refuse.error)
  await shot('error')
  refuse = undefined
  await key.getByRole('button',{name:'Make an enrolment code'}).click()
  await expect(key.locator('.fresh code')).toHaveText(CODE)
  await expect(key.locator('.fresh')).toContainText('Paste this when the machine asks for an enrolment code.')
  await expect(key.locator('.fresh')).toContainText(/Expires in 1[45]:\d\d/)
  await expect(key.getByRole('button',{name:'Copy enrolment code'})).toBeVisible()
  await shot('code')
  expect(calls).toEqual(['POST','POST'])
  expect(tokenCalls).toEqual([])
  expect(await page.evaluate(()=>location.href+JSON.stringify(localStorage)+JSON.stringify(sessionStorage))).not.toContain(CODE)
  // Closing and reopening does not bring it back.
  await dialog.getByRole('button',{name:/Close/}).click()
  await expect(dialog).toHaveCount(0)
  await page.getByRole('button',{name:'Add a machine'}).first().click()
  await expect(dialog).toBeVisible()
  await expect(dialog.getByText(CODE)).toHaveCount(0)
  // A short-lived code runs out in front of the person, who is offered another.
  life = 2
  await key.getByRole('button',{name:'Make an enrolment code'}).click()
  await expect(key.locator('.fresh code')).toHaveText(CODE)
  await expect(key.getByText('This code has run out.')).toBeVisible({timeout:8000})
  await expect(dialog.getByText(CODE)).toHaveCount(0)
  await shot('expired')
  await expect(key.getByRole('button',{name:'Make another code'})).toBeVisible()
  expect(tokenCalls).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }
 // The step-by-step panel's key step is the same enrolment code, and makes no personal token.
 if (run('panel')) {
  const CODE = 'reet_enrol_FIXTURE_CODE_9f3a'
  for (const [width, theme] of [[1280, 'light'], [390, 'dark']]) {
   const calls = [], tokenCalls = []
   const routes = {
    '/api/v2/sync/enrol-codes': request => { calls.push(request.method()); return { status: 201, json: { code: CODE, expiresAt: Math.floor(Date.now() / 1000) + 900 } } },
    '/api/v2/tokens': request => { tokenCalls.push(request.method()); return { status: 500, json: { error: 'Unexpected token call.' } } },
   }
   const {context,page,errors} = await open({width,theme,routes,scale:2})
   await page.goto(`${base}/machines#connect-machine`)
   await page.getByLabel(/Machine name/).first().fill('studio-laptop')
   await page.getByRole('button',{name:/Continue to authentication/}).click()
   const panel = page.locator('details#connect-machine .step-panel')
   await expect(panel.getByRole('heading',{name:'Give this machine an enrolment code.'})).toBeVisible()
   await expect(panel).not.toContainText(/machine token|personal API token|Create machine token/i)
   const shot = async name => { if (process.env.REET_E2E_SHOTS) await panel.screenshot({path:`${process.env.REET_E2E_SHOTS}/panel-${name}-${width}-${theme}.png`}) }
   await shot('before')
   await panel.getByRole('button',{name:'Make an enrolment code'}).click()
   await expect(panel.locator('.fresh code')).toHaveText(CODE)
   await expect(panel.locator('.fresh')).toContainText(/Expires in 1[45]:\d\d/)
   await expect(panel.getByRole('button',{name:'Copy enrolment code'})).toBeVisible()
   await shot('code')
   expect(calls).toEqual(['POST'])
   expect(tokenCalls).toEqual([])
   expect(await page.evaluate(()=>location.href+JSON.stringify(localStorage)+JSON.stringify(sessionStorage))).not.toContain(CODE)
   // The last step tells the person to paste the code, not a token.
   await panel.getByRole('button',{name:'Continue to terminal connection'}).click()
   await expect(panel).toContainText('When prompted, paste the enrolment code.')
   await expect(panel).not.toContainText(/personal API token/i)
   expect(tokenCalls).toEqual([])
   expect(errors).toEqual([])
   await context.close()
  }
 }
 console.log('First machine: one entry dialog from every button, the panel waits behind it, Continue says what it needs, the key step is an enrolment code.')
} finally {await browser.close()}
