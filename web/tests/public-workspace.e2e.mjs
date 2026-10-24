// Public entry points must explain the workspace without loading private inventories.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const screenshots = process.env.REET_UI_SCREENSHOTS
if (screenshots) await mkdir(screenshots,{recursive:true})
try {
 for (const colorScheme of ['light','dark']) {
  for (const width of [1440,390,320]) {
   const context = await browser.newContext({viewport:{width,height:960},colorScheme})
   const page = await context.newPage()
   const errors=[], unexpected=[]
   let status={mode:'accounts',user:null,setupRequired:false,signupEnabled:false,contactEmail:'owner@example.test',passwordReset:true}
   page.on('pageerror',e=>errors.push(e.message))
   await page.route('**/api/**',route=>{
    const path=new URL(route.request().url()).pathname
    if(path==='/api/v2/auth/status')return route.fulfill({json:status})
    if(path==='/api/version')return route.fulfill({json:{version:'test',commit:'fixture'}})
    if(path==='/api/v2/auth/sign-in')return route.fulfill({status:401,json:{error:'Email or password is incorrect.'}})
    unexpected.push(path)
    return route.fulfill({status:403,json:{error:'Private API must not be called'}})
   })
   const layout=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
   const noOldPitch=async()=>expect(await page.locator('body').innerText()).not.toMatch(/skills\.sh|your shelf|skills reported/i)
   await page.goto(base)
   await expect(page.getByRole('heading',{level:1})).toContainText('For your AI fleet.')
   await expect(page.getByText('Ey up. Your AI control room.',{exact:true})).toBeVisible()
   await expect(page.getByRole('heading',{name:'A reet good agent toolkit.'})).toBeVisible()
   await expect(page.getByText('On the workbench',{exact:true})).toBeVisible()
   await expect(page.getByRole('link',{name:'Open your workspace'})).toHaveAttribute('href','/signin')
   await expect(page.getByRole('link',{name:'Request an invite'})).toHaveAttribute('href',/^mailto:owner@example.test/)
   await expect(page.getByRole('link',{name:'Create an account'})).toHaveCount(0)
   await expect(page.getByRole('figure')).toContainText('Illustration.')
   await layout(); await noOldPitch()
   if(screenshots)await page.screenshot({path:`${screenshots}/public-${width}-${colorScheme}.png`,fullPage:true})
   await page.getByRole('link',{name:'Host it yourself',exact:true}).click()
   await expect(page.getByRole('heading',{name:'Give your machines a home.'})).toBeInViewport()
   await page.getByRole('link',{name:'Open your workspace'}).click()
   await expect(page.getByRole('heading',{name:'Sign in to your workspace'})).toBeVisible()
   await expect(page.locator('#signin-email')).toBeFocused()
   await page.locator('#signin-email').fill('engineer@example.test')
   await page.getByLabel('Password',{exact:true}).fill('incorrect-fixture')
   await page.getByRole('button',{name:'Sign in',exact:true}).click()
   await expect(page.getByRole('alert')).toContainText('Email or password is incorrect.')
   await layout();await noOldPitch()
   if(screenshots)await page.screenshot({path:`${screenshots}/signin-${width}-${colorScheme}.png`,fullPage:true})
   status={...status,signupEnabled:true}
   await page.goto(base)
   await page.getByRole('link',{name:'Create an account'}).click()
   await expect(page.getByRole('heading',{name:'Create your account'})).toBeVisible()
   await expect(page.getByLabel('Confirm password',{exact:true})).toBeVisible()
   await layout();await noOldPitch()
   status={...status,setupRequired:true,bootstrapRequired:true,signupEnabled:false}
   await page.goto(base)
   await page.getByRole('link',{name:'Set up this instance'}).click()
   await expect(page.getByRole('heading',{name:'Create your account'})).toBeVisible()
   await expect(page.getByLabel('Access token',{exact:true})).toBeVisible()
   await layout();await noOldPitch()
   status={mode:'token'}
   await page.goto(base)
   await expect(page.getByRole('heading',{name:'Open your workspace'})).toBeVisible()
   await layout();await noOldPitch()
   expect(unexpected).toEqual([])
   expect(errors).toEqual([])
   await context.close()
  }
 }
 console.log('Public workspace: light/dark, desktop/phone, invite/signup/setup/token entry points passed; no private API requests.')
} finally {await browser.close()}
