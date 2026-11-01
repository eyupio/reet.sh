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
   await expect(page.getByRole('heading',{level:1})).toContainText('Your whole AI setup shows up.')
   await expect(page.getByText('Ey up. Your AI control room.',{exact:true})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Enrol a machine with one command.'})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Agents, skills and setups stay in sync.'})).toBeVisible()
   // The illustration shows the real enrolment: the product's own two commands, a machine reporting in, and the fleet in sync.
   await expect(page.locator('.workspace-preview')).toContainText('reet agent enroll --server')
   await expect(page.locator('.workspace-preview')).toContainText('reet agent run')
   await expect(page.locator('.workspace-preview .preview-machine')).toHaveCount(3)
   await expect(page.getByText('On the workbench',{exact:true})).toBeVisible()
   // Invite-only: members of a private instance come back to sign in, so that leads. Install and invite are real buttons, not quiet text.
   const cta=page.locator('.cta-row')
   await expect(cta.getByRole('link',{name:'Sign in'})).toHaveAttribute('href','/signin')
   await expect(cta.getByRole('link',{name:'Sign in'})).toHaveClass(/primary/)
   await expect(cta.getByRole('link',{name:'Request an invite'})).toHaveAttribute('href',/^mailto:owner@example.test/)
   await expect(cta.getByRole('link',{name:'Request an invite'})).not.toHaveClass(/quiet/)
   await expect(cta.getByRole('link',{name:'Install reet.sh'})).toHaveAttribute('href','#self-host')
   await expect(cta.getByRole('link',{name:'Install reet.sh'})).not.toHaveClass(/quiet/)
   await expect(cta.getByRole('link',{name:'Create an account'})).toHaveCount(0)
   // The trust line says the product is free and open source, at a readable size.
   await expect(page.getByLabel('Why reet.sh')).toContainText('AGPL-3.0')
   expect(await page.locator('.trust span').first().evaluate(el=>parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(12)
   // Nothing in the illustration is below 11px.
   const tiny=await page.locator('.workspace-preview').evaluate(root=>{const found=[];const walk=document.createTreeWalker(root,NodeFilter.SHOW_TEXT);for(let node;node=walk.nextNode();){if(!node.textContent.trim())continue;const size=parseFloat(getComputedStyle(node.parentElement).fontSize);if(size<11)found.push(`${size}px "${node.textContent.trim().slice(0,30)}"`)}return found})
   expect(tiny,'illustration text under 11px').toEqual([])
   await expect(page.getByRole('figure')).toContainText('Illustration.')
   // Public pages carry the help and trust links, and the roadmap link opens a short roadmap, not an internal log.
   const footer=page.getByRole('contentinfo')
   await expect(footer.getByRole('link',{name:'GitHub'})).toHaveAttribute('href','https://github.com/eyupio/reet.sh')
   await expect(footer.getByRole('link',{name:'Docs'})).toHaveAttribute('href','https://github.com/eyupio/reet.sh#quick-start')
   await expect(footer.getByRole('link',{name:'AGPL-3.0'})).toHaveAttribute('href','https://github.com/eyupio/reet.sh/blob/main/LICENSE')
   await expect(page.getByRole('link',{name:/See what.s next/})).toHaveAttribute('href','https://github.com/eyupio/reet.sh/blob/main/docs/roadmap.md')
   await layout(); await noOldPitch()
   if(screenshots)await page.screenshot({path:`${screenshots}/public-${width}-${colorScheme}.png`,fullPage:true})
   await cta.getByRole('link',{name:'Install reet.sh'}).click()
   await expect(page.getByRole('heading',{name:'Give your machines a home.'})).toBeInViewport()
   await page.evaluate(()=>scrollTo(0,0))
   await cta.getByRole('link',{name:'Sign in'}).click()
   await expect(page.getByRole('heading',{name:'Sign in to your workspace'})).toBeVisible()
   // A wide screen with a keyboard starts in the field; a phone is not handed its soft keyboard on load.
   if(width>=700)await expect(page.locator('#signin-email')).toBeFocused()
   else await expect(page.locator('#signin-email')).not.toBeFocused()
   await page.locator('#signin-email').fill('engineer@example.test')
   await page.getByLabel('Password',{exact:true}).fill('incorrect-fixture')
   await page.getByRole('button',{name:'Sign in',exact:true}).click()
   await expect(page.getByRole('alert')).toContainText('Email or password is incorrect.')
   await layout();await noOldPitch()
   if(screenshots)await page.screenshot({path:`${screenshots}/signin-${width}-${colorScheme}.png`,fullPage:true})
   status={...status,signupEnabled:true}
   await page.goto(base)
   // A visitor who can sign up is led to sign-up; returning members still have a Sign in button.
   await expect(page.locator('.cta-row').getByRole('link',{name:'Create an account'})).toHaveClass(/primary/)
   await expect(page.locator('.cta-row').getByRole('link',{name:'Sign in'})).not.toHaveClass(/primary/)
   await page.getByRole('link',{name:'Create an account'}).click()
   await expect(page.getByRole('heading',{name:'Create your account'})).toBeVisible()
   await expect(page.getByLabel('Confirm password',{exact:true})).toBeVisible()
   await layout();await noOldPitch()
   status={...status,setupRequired:true,bootstrapRequired:true,signupEnabled:false}
   await page.goto(base)
   // Before setup no account exists, so the header does not offer to sign in.
   await expect(page.getByRole('navigation',{name:'Public navigation'}).getByRole('link',{name:/Sign in/})).toHaveCount(0)
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
