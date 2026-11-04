// A standalone installation opens its own account entry, without reading private inventories.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const screenshots = process.env.REET_UI_SCREENSHOTS
if (screenshots) await mkdir(screenshots,{recursive:true})
try {
 for (const colorScheme of ['light','dark']) for (const width of [1440,390,320]) {
  const context=await browser.newContext({viewport:{width,height:960},colorScheme})
  const page=await context.newPage()
  const errors=[],unexpected=[]
  let status={mode:'accounts',user:null,setupRequired:false,signupEnabled:false,contactEmail:'owner@example.test',passwordReset:true}
  page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname
   if(path==='/api/v2/auth/status')return route.fulfill({json:status})
   if(path==='/api/version')return route.fulfill({json:{version:'test',commit:'fixture'}})
   if(path==='/api/v2/auth/sign-in')return route.fulfill({status:401,json:{error:'Email or password is incorrect.'}})
   if(path==='/api/v2/auth/invite')return route.fulfill({json:{email:'new@example.test',role:'member',expiresAt:Date.now()+86400000}})
   unexpected.push(path)
   return route.fulfill({status:403,json:{error:'Private API must not be called'}})
  })
  const layout=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  const noPitch=async()=>{
   await expect(page.locator('.welcome,.workspace-preview,.self-host,.next-up,.guides')).toHaveCount(0)
   await expect(page.locator('body')).not.toContainText('One command on any machine')
  }
  await page.goto(base)
  await expect(page.getByRole('heading',{name:'Sign in to your workspace'})).toBeVisible()
  await expect(page).toHaveTitle('Sign in · reet.sh')
  await expect(page.locator('.form-panel .instance')).toContainText(new URL(base).host)
  await expect(page.getByRole('link',{name:'Request an invite'})).toHaveAttribute('href',/^mailto:owner@example.test/)
  await expect(page.locator('header').getByRole('link',{name:/Documentation/})).toHaveAttribute('href','https://reet.sh/')
  await layout();await noPitch()
  if(width>=700)await expect(page.locator('#signin-email')).toBeFocused()
  else await expect(page.locator('#signin-email')).not.toBeFocused()
  const picker=page.locator('header .theme-picker')
  await expect(picker.getByRole('button')).toHaveCount(3)
  await picker.getByRole('button',{name:'Dark'}).click()
  expect(await page.evaluate(()=>document.documentElement.dataset.theme)).toBe('dark')
  await page.reload()
  expect(await page.evaluate(()=>document.documentElement.dataset.theme)).toBe('dark')
  await page.locator('header .theme-picker').getByRole('button',{name:'Match this device'}).click()
  await page.locator('#signin-email').fill('engineer@example.test')
  await page.getByLabel('Password',{exact:true}).fill('incorrect-fixture')
  await page.getByRole('button',{name:'Sign in',exact:true}).click()
  await expect(page.getByRole('alert')).toContainText('Email or password is incorrect.')
  await layout();await noPitch()
  if(screenshots)await page.screenshot({path:screenshots+'/signin-'+width+'-'+colorScheme+'.png',fullPage:true})
  status={...status,signupEnabled:true}
  await page.goto(base)
  await page.getByRole('link',{name:'Create an account'}).click()
  await expect(page.getByRole('heading',{name:'Create your account'})).toBeVisible()
  await expect(page.getByLabel('Confirm password',{exact:true})).toBeVisible()
  await layout();await noPitch()
  status={...status,setupRequired:true,bootstrapRequired:true,signupEnabled:false}
  await page.goto(base)
  await expect(page.getByRole('heading',{name:'Create your account'})).toBeVisible()
  await expect(page.getByText('Enrol your first machine, enable terminal access',{exact:false})).toBeVisible()
  await expect(page.getByLabel('Access token',{exact:true})).toBeVisible()
  await layout();await noPitch()
  await page.goto(base+'/join?token=t')
  await expect(page.getByRole('heading',{name:'Join reet.sh'})).toBeVisible()
  await expect(page.getByText("Your team's agents, skills and setups are one sign-in away.")).toBeVisible()
  await layout()
  status={mode:'token'}
  await page.goto(base)
  await expect(page.getByRole('heading',{name:'Open your workspace'})).toBeVisible()
  await layout();await noPitch()
  expect(unexpected).toEqual([]);expect(errors).toEqual([])
  await context.close()
 }
 console.log('Standalone entry: account, invite, sign-up, owner setup and token modes; both themes; desktop/phone; no private inventory reads.')
} finally {await browser.close()}
