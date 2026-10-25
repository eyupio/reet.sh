// First-run and phone-width contract: the menu stays on screen, setup ends on Control, unknown routes say so,
// auth screens fit a phone, and touch devices are not handed a keyboard on load.
import { chromium, expect } from '@playwright/test'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const owner = {id:1,name:'Sam',email:'sam@example.test',role:'owner'}

async function open({width, height=812, theme='light', status}) {
 const context = await browser.newContext({viewport:{width,height}})
 const page = await context.newPage()
 const state = {status:{mode:'accounts',setupRequired:false,bootstrapRequired:false,signupEnabled:false,user:owner,...status}, theme, unexpected:[]}
 const errors = []
 page.on('pageerror',e=>errors.push(e.message))
 await page.route('**/api/**',route=>{
  const path = new URL(route.request().url()).pathname
  if(path==='/api/v2/auth/status')return route.fulfill({json:state.status})
  if(path==='/api/v2/auth/setup'){state.status={...state.status,setupRequired:false,user:owner};return route.fulfill({json:owner})}
  if(path==='/api/v2/profile')return route.fulfill({json:{profile:{theme:state.theme,shelfView:'list',cardsExpanded:false}}})
  if(path==='/api/version')return route.fulfill({json:{version:'test',commit:'fixture'}})
  if(path==='/api/skills')return route.fulfill({json:{skills:[],counts:{all:0,used:0,liked:0,bookmarked:0}}})
  if(path==='/api/environments')return route.fulfill({json:{environments:[],machines:[]}})
  if(path==='/api/tags')return route.fulfill({json:{tags:[]}})
  if(path==='/api/v2/lists')return route.fulfill({json:{lists:[],linkEnabled:false}})
  if(path==='/api/v2/sync')return route.fulfill({json:{environments:[],machines:[],agents:[]}})
  if(path==='/api/v2/machines/health')return route.fulfill({json:{serverNow:Date.now(),machines:[]}})
  if(path==='/api/v2/terminal/machines')return route.fulfill({json:{machines:[]}})
  if(path==='/api/v2/terminal/sessions')return route.fulfill({json:{sessions:[]}})
  if(path==='/api/v2/notifications/count')return route.fulfill({json:{unread:0}})
  state.unexpected.push(path)
  return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 })
 return {context, page, state, errors}
}

try {
 // The phone menu opens inside the screen at every width where the compact menu is used.
 for (const width of run('menu') ? [320,375,414,768,999] : []) {
  const {context,page,errors} = await open({width})
  await page.goto(base)
  await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
  await page.locator('nav[aria-label="Main navigation"] summary',{hasText:'Menu'}).click()
  const panel = await page.locator('nav[aria-label="Main navigation"] .mobile .links').evaluate(el=>{const r=el.getBoundingClientRect();return{left:r.left,right:r.right,width:innerWidth}})
  expect(panel.left, `menu panel left edge at ${width}px`).toBeGreaterThanOrEqual(0)
  expect(panel.right, `menu panel right edge at ${width}px`).toBeLessThanOrEqual(panel.width)
  await expect(page.locator('nav[aria-label="Main navigation"] .mobile .links').getByRole('link',{name:'Machines'})).toBeInViewport({ratio:1})
  expect(errors).toEqual([])
  await context.close()
 }

 // Creating the owner ends on Control, at /, with nothing stealing focus.
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
  await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
  expect(await page.title()).toContain('Control')
  expect(errors).toEqual([])
  await context.close()
 }

 // A signed-in person who opens an entry address goes to Control, not to a stale form or the shelf.
 for (const path of run('entry') ? ['/signin','/signup','/setup','/welcome'] : []) {
  const {context,page} = await open({width:1440})
  await page.goto(`${base}${path}`)
  await page.waitForURL(`${base}/`)
  await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
  await context.close()
 }

 // Unknown addresses say so. The shelf lives at /skills.
 if (run('notfound')) {
  const {context,page} = await open({width:1440})
  await page.goto(`${base}/this-page-does-not-exist`)
  await expect(page.getByRole('heading',{name:'Page not found'})).toBeVisible()
  expect(await page.title()).toContain('Page not found')
  await expect(page.getByRole('heading',{name:'Your central skill library.'})).toHaveCount(0)
  await page.getByRole('link',{name:'Back to Control'}).click()
  await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('heading',{name:'Your central skill library.'})).toBeVisible()
  await context.close()
 }

 // Phone auth screens: the headline keeps its space, and the form with its button fits the first screen.
 for (const route of run('authfit') ? ['/signin','/setup'] : []) {
  const {context,page} = await open({width:375,height:812,status:{user:null,setupRequired:route==='/setup'}})
  await page.goto(`${base}${route}`)
  const headline = await page.locator('.headline').innerText()
  expect(headline).toContain('Your AI workspace. A reet good place to work.')
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

 // An explicit dark theme keeps a readable warning colour, same as the system dark theme.
 if (run('dark')) {
  const {context,page} = await open({width:1440,theme:'dark'})
  await page.goto(base)
  await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
  expect(await page.evaluate(()=>document.documentElement.dataset.theme)).toBe('dark')
  expect(await page.evaluate(()=>getComputedStyle(document.documentElement).getPropertyValue('--warn').trim())).toBe('#fbbf24')
  await context.close()
 }
 console.log('First run: phone menu on screen, setup ends on Control, unknown routes, phone auth fit, touch focus and dark warning colour passed.')
} finally {await browser.close()}
