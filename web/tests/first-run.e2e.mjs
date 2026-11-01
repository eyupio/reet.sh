// First-run and phone-width contract: the menu stays on screen, setup ends on Home, unknown routes say so,
// auth screens fit a phone, and touch devices are not handed a keyboard on load.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // More opens inside the screen at every width where the tabs fold into Home, Sessions, Machines and More.
 for (const width of run('menu') ? [320,375,414,768,999] : []) {
  const {context,page,errors} = await open({width})
  await page.goto(`${base}/machines`)
  await page.getByRole('navigation',{name:'Main navigation'}).getByRole('button',{name:'More'}).click()
  const sheet = page.getByRole('dialog',{name:'More'})
  const panel = await sheet.evaluate(el=>{const r=el.getBoundingClientRect();return{left:r.left,right:r.right,width:innerWidth}})
  expect(panel.left, `More sheet left edge at ${width}px`).toBeGreaterThanOrEqual(0)
  expect(panel.right, `More sheet right edge at ${width}px`).toBeLessThanOrEqual(panel.width)
  await expect(sheet.getByRole('link',{name:'Skills',exact:true})).toBeInViewport({ratio:1})
  expect(errors).toEqual([])
  await context.close()
 }

 // Creating the owner ends on Home, at /, with nothing stealing focus.
 if (run('setup')) {
  const {context,page,errors} = await open({width:1440,status:{setupRequired:true,user:null}})
  await page.goto(base)
  await page.getByRole('link',{name:'Set up this instance'}).click()
  await page.getByLabel('Your name').fill('Sam')
  await page.getByLabel('Email').fill('sam@example.test')
  await page.getByLabel('Password',{exact:true}).fill('correct-horse-battery')
  await page.getByLabel('Confirm password',{exact:true}).fill('correct-horse-battery')
  await page.getByRole('button',{name:'Create account'}).click()
  await page.waitForURL(`${base}/`)
  await expect(page.getByRole('heading',{name:'A reet good day’s work.',exact:true})).toBeVisible()
  expect(await page.title()).toBe('Home · reet.sh')
  expect(errors).toEqual([])
  await context.close()
 }

 // A signed-in person who opens an entry address goes to Home, not to a stale form or the shelf.
 for (const path of run('entry') ? ['/signin','/signup','/setup','/welcome'] : []) {
  const {context,page} = await open({width:1440})
  await page.goto(`${base}${path}`)
  await page.waitForURL(`${base}/`)
  await expect(page.getByRole('heading',{name:'A reet good day’s work.',exact:true})).toBeVisible()
  await context.close()
 }

 // Unknown addresses say so. The shelf lives at /skills.
 if (run('notfound')) {
  const {context,page} = await open({width:1440})
  await page.goto(`${base}/this-page-does-not-exist`)
  await expect(page.getByRole('heading',{name:'Page not found'})).toBeVisible()
  expect(await page.title()).toContain('Page not found')
  await expect(page.getByRole('heading',{name:'Skills.',exact:true})).toHaveCount(0)
  await page.getByRole('link',{name:'Back to Home'}).click()
  await expect(page.getByRole('heading',{name:'A reet good day’s work.',exact:true})).toBeVisible()
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('heading',{name:'Skills.',exact:true})).toBeVisible()
  await context.close()
 }

 // Phone auth screens: the headline keeps its space, and the form with its button fits the first screen.
 for (const route of run('authfit') ? ['/signin','/setup'] : []) {
  const {context,page} = await open({width:375,height:812,status:{user:null,setupRequired:route==='/setup'}})
  await page.goto(`${base}${route}`)
  const headline = await page.locator('.headline').innerText()
  expect(headline).toContain('Your whole AI setup. On every machine.')
  const submit = await page.locator('form button[type="submit"]').evaluate(el=>el.getBoundingClientRect().bottom)
  expect(submit, `submit button bottom on ${route} at 375x812`).toBeLessThanOrEqual(812)
  await context.close()
 }

 // Touch-sized screens are not given a keyboard on load; desktop keeps the focused field.
 for (const [width,focused] of run('focus') ? [[1440,true],[390,false],[320,false]] : []) {
  for (const [route,id,status] of [['/signin','#signin-email',{user:null}],['/setup','#setup-name',{user:null,setupRequired:true}]]) {
   const {context,page} = await open({width,status})
   await page.goto(`${base}${route}`)
   await expect(page.locator(id)).toBeVisible()
   expect(await page.evaluate(sel=>document.activeElement===document.querySelector(sel),id), `${id} focused at ${width}px`).toBe(focused)
   await context.close()
  }
 }

 // A focused field shows one ring: its outline touches its border instead of floating beside it as a second ring.
 // Checkboxes and radios have no border to join, so they keep their offset.
 if (run('ring')) {
  const {context,page} = await open({width:1440,status:{user:null}})
  await page.goto(`${base}/signin`)
  await expect(page.locator('#signin-email')).toBeVisible()
  const rings = await page.evaluate(()=>{
   const out = {}
   for (const [name,html] of [['input','<input type="text">'],['select','<select><option>a</option></select>'],['textarea','<textarea></textarea>'],['checkbox','<input type="checkbox">']]) {
    const wrap = document.createElement('div')
    wrap.innerHTML = html
    document.body.append(wrap)
    const el = wrap.firstElementChild
    el.focus()
    const s = getComputedStyle(el)
    out[name] = {visible:el.matches(':focus-visible'),style:s.outlineStyle,offset:parseFloat(s.outlineOffset)}
    wrap.remove()
   }
   return out
  })
  for (const name of ['input','select','textarea']) {
   expect(rings[name].visible, `${name} matches :focus-visible`).toBe(true)
   expect(rings[name].style, `${name} outline style`).not.toBe('none')
   expect(rings[name].offset, `${name} outline offset`).toBeLessThanOrEqual(0)
  }
  expect(rings.checkbox.offset, 'checkbox outline offset').toBe(2)
  await context.close()
 }

 // An explicit dark theme keeps a readable warning colour, same as the system dark theme.
 if (run('dark')) {
  const {context,page} = await open({width:1440,theme:'dark'})
  await page.goto(`${base}/machines`)
  await expect(page.getByRole('heading',{name:'Your machines, connected.'})).toBeVisible()
  expect(await page.evaluate(()=>document.documentElement.dataset.theme)).toBe('dark')
  expect(await page.evaluate(()=>getComputedStyle(document.documentElement).getPropertyValue('--warn').trim())).toBe('#e9b44c')
  await context.close()
 }
 console.log('First run: More on screen, setup ends on Home, unknown routes, phone auth fit, touch focus, one focus ring and dark warning colour passed.')
} finally {await browser.close()}
