// Navigation contract: the areas the landing page sells are one click away, notifications live in the account menu,
// and each area keeps one name wherever it appears.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, owner, skill, terminalMachine, active } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)
const tabs = page => page.getByRole('navigation',{name:'Main navigation'})

try {
 // The desktop tabs are Home, Sessions, Machines and Skills for a writer, with Home current at /, the Jump search and
 // the bell beside them; the rest of the old navigation sits under More in the account menu, and Activity is in Account.
 if (run('writer desktop shows five tabs with Home current')) {
  const {context,page,errors} = await open({width:1440})
  await page.goto(base)
  await expect(page.getByRole('region',{name:'Your machines',exact:true})).toBeVisible()
  expect(await page.title()).toBe('Home · reet.sh')
  const top = await tabs(page).getByRole('link').allInnerTexts()
  expect(top.map(t=>t.trim())).toEqual(['Home','Sessions','Machines','Skills'])
  await expect(tabs(page).getByRole('link',{name:'Home',exact:true})).toHaveAttribute('aria-current','page')
  for (const [name,href] of [['Home','/'],['Sessions','/terminals'],['Machines','/machines'],['Skills','/skills']])
   await expect(tabs(page).getByRole('link',{name,exact:true})).toHaveAttribute('href',href)
  // Activity is no longer a top-level tab: it lives inside the account.
  await expect(tabs(page).getByRole('link',{name:'Activity',exact:true})).toHaveCount(0)
  // The bell is a button named for the Needs you count; with nothing waiting it carries no badge.
  const bell = page.getByRole('button',{name:'Needs you: 0',exact:true})
  await expect(bell).toBeVisible()
  await expect(bell.locator('.badge')).toHaveCount(0)
  // With nothing waiting the bell is quiet: not in the warning colour.
  const colours = await page.evaluate(()=>({bell:getComputedStyle(document.querySelector('.bell-button')).color,warn:getComputedStyle(document.documentElement).getPropertyValue('--warn').trim()}))
  const hex = c=>'#'+c.match(/\d+/g).slice(0,3).map(n=>Number(n).toString(16).padStart(2,'0')).join('')
  expect(hex(colours.bell)).not.toBe(colours.warn.toLowerCase())
  await bell.click()
  await expect(page.getByRole('banner').getByText('Nothing needs you.',{exact:true})).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('banner').getByText('Nothing needs you.',{exact:true})).toBeHidden()
  await expect(bell).toBeFocused()
  // The search box is there, and / focuses it from anywhere but a field.
  const jump = page.getByRole('combobox',{name:'Jump to a session, project or machine'})
  await expect(jump).toBeVisible()
  await page.locator('main').click({position:{x:5,y:5}})
  await page.keyboard.press('/')
  await expect(jump).toBeFocused()
  await expect(jump).toHaveValue('')
  // One row: the header stays the mock-up's slim bar.
  const h = await page.evaluate(()=>document.querySelector('.site-header').getBoundingClientRect().height)
  expect(h).toBeLessThanOrEqual(72)
  expect(errors).toEqual([])
  await context.close()
 }

 // Jump finds sessions, folder projects and machines the workspace already has, and opens the matching page (a project on
 // Home, chosen).
 if (run('jump')) {
  const session = {id:'s1',agentId:'a',title:'Refactor the store layer',state:'running',createdAt:Date.now(),updatedAt:Date.now(),exitCode:null}
  const project = {id:'fp1',host:'a',machine:'studio',dir:'/home/dev/store',name:'Store rewrite',isRepo:true,baseBranch:'',worktreeDefault:'inherit',defaultTool:'',defaultModel:'',hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1}
  const {context,page,errors} = await open({width:1440,terminalStatus:active,terminalMachines:[terminalMachine('a','studio',true)],
   routes:{'/api/v2/terminal/sessions':{sessions:[session]},'/api/v2/folder-projects':{projects:[project],savedWaiting:0}}})
  await page.goto(`${base}/skills`)
  const jump = page.getByRole('combobox',{name:'Jump to a session, project or machine'})
  await jump.fill('store')
  const results = page.getByRole('listbox',{name:'Jump results'})
  await expect(results.getByRole('option')).toHaveCount(2)
  await expect(results.getByRole('option',{name:/Refactor the store layer/})).toHaveAttribute('href','/terminals?agent=a&session=s1')
  await expect(results.getByRole('option',{name:/Store rewrite/})).toHaveAttribute('href','/?project=fp1')
  await jump.fill('stud')
  await expect(results.getByRole('option',{name:/studio/})).toHaveAttribute('href','/machines?machine=studio')
  await jump.press('ArrowDown')
  await expect(results.getByRole('option',{name:/studio/})).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(results).toBeHidden()
  await expect(jump).toBeFocused()
  await jump.fill('nothing like it')
  await expect(page.getByText('Nothing matches.',{exact:true})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // A phone has four tabs along the bottom: Home, Sessions, Machines and More, 64 px tall with 44 px targets; More lists the rest.
 if (run('phone shows four bottom tabs and More lists the rest')) {
  for (const width of [320,390]) {
   const {context,page,errors} = await open({width,touch:true})
   await page.goto(base)
   await expect(page.getByRole('region',{name:'Your machines',exact:true})).toBeVisible()
   const bar = tabs(page)
   const items = await bar.locator(':scope a, :scope button').allInnerTexts()
   expect(items.map(t=>t.trim())).toEqual(['Home','Sessions','Machines','More'])
   const m = await bar.evaluate(el=>{const r=el.getBoundingClientRect();return{bottom:r.bottom,height:r.height,targets:[...el.querySelectorAll('a,button')].map(a=>{const b=a.getBoundingClientRect();return[b.width,b.height]})}})
   expect(m.height).toBeGreaterThanOrEqual(64)
   expect(Math.round(m.bottom)).toBe(812)
   for (const [w,h] of m.targets) { expect(w).toBeGreaterThanOrEqual(44); expect(h).toBeGreaterThanOrEqual(44) }
   await expect(bar.getByRole('link',{name:'Home'})).toHaveAttribute('aria-current','page')
   // The top bar holds the logo, Jump, the bell and the account; no desktop tabs.
   const bell = page.getByRole('button',{name:'Needs you: 0',exact:true})
   await expect(bell).toBeVisible()
   // Its panel opens inside the screen, even at 320 px.
   await bell.click()
   const panel = await page.getByRole('banner').getByText('Nothing needs you.',{exact:true}).locator('..').evaluate(el=>{const r=el.getBoundingClientRect();return{left:r.left,right:r.right}})
   expect(panel.left, `bell panel left edge at ${width}px`).toBeGreaterThanOrEqual(0)
   expect(panel.right, `bell panel right edge at ${width}px`).toBeLessThanOrEqual(width)
   await page.keyboard.press('Escape')
   // More's buttons say they open a dialog and whether it is open.
   await expect(bar.getByRole('button',{name:'More'})).toHaveAttribute('aria-haspopup','dialog')
   await expect(bar.getByRole('button',{name:'More'})).toHaveAttribute('aria-expanded','false')
   await bar.getByRole('button',{name:'More'}).click()
   const more = page.getByRole('dialog',{name:'More'})
   await expect(more).toBeVisible()
   const links = await more.getByRole('link').allInnerTexts()
   expect(links.map(t=>t.trim())).toEqual(['Skills','Control','Setups','Sync','Lists','Community','Support','Teams & organisations','Host skills'])
   await expect(more.getByRole('link',{name:'Control'})).toHaveAttribute('href','/control')
   await page.keyboard.press('Escape')
   await expect(more).toBeHidden()
   await expect(bar.getByRole('button',{name:'More'})).toBeFocused()
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
   // The last thing on the page (the footer, or the main area on Home, which has none) can be scrolled clear of the bar.
   // Home's lists arrive after first paint and grow <main>, so scroll and measure together until it settles.
   await expect.poll(async()=>{
    await page.evaluate(()=>scrollTo(0,document.documentElement.scrollHeight))
    return page.locator('footer, main').last().evaluate(el=>el.getBoundingClientRect().bottom)
   }).toBeLessThanOrEqual(812-64+1)
   expect(errors).toEqual([])
   await context.close()
  }
  // A tablet keeps the four tabs at the top, with More there too.
  const {context,page} = await open({width:834,touch:true})
  await page.goto(base)
  const items = await tabs(page).locator(':scope a, :scope button').evaluateAll(els=>els.filter(e=>e.getClientRects().length).map(e=>e.textContent.trim()))
  expect(items).toEqual(['Home','Sessions','Machines','More'])
  const top = await tabs(page).evaluate(el=>el.getBoundingClientRect().top)
  expect(top).toBeLessThan(80)
  await tabs(page).getByRole('button',{name:'More'}).click()
  await expect(page.getByRole('dialog',{name:'More'}).getByRole('link',{name:'Skills',exact:true})).toBeVisible()
  await page.keyboard.press('Escape')
  // Every link in the account menu, More included, is a 44 px target on a touch tablet.
  await page.locator('summary[aria-label^="Account menu"]').click()
  const heights = await page.locator('.panel a, .panel button').evaluateAll(els=>els.map(e=>[e.textContent.trim(),e.getBoundingClientRect().height]))
  for (const [name,h] of heights) expect(h, `${name} target height at 834px touch`).toBeGreaterThanOrEqual(44)
  await context.close()
  // A touch screen 1100 px or wider keeps Jump in the bar, 44 px tall.
  const wide = await open({width:1180,touch:true})
  await wide.page.goto(base)
  const field = await wide.page.locator('.jump').boundingBox()
  expect(field.height).toBeGreaterThanOrEqual(44)
  await wide.context.close()
 }

 // Nothing the old navigation reached is lost: every former destination is a tab or sits under More in the account menu.
 // Projects left More: folder projects live in Home's rail, and /projects goes there.
 if (run('every former nav destination reachable, Control under More')) {
  const {context,page,errors} = await open({width:1440})
  await page.goto(base)
  await page.locator('summary[aria-label^="Account menu"]').click()
  const more = page.locator('.panel').getByRole('group',{name:'More'})
  const names = await more.getByRole('link').allInnerTexts()
  expect(names.map(t=>t.trim())).toEqual(['Control','Setups','Sync','Lists','Community','Support','Teams & organisations','Host skills'])
  const hrefs = [...await tabs(page).getByRole('link').evaluateAll(els=>els.map(e=>e.getAttribute('href'))), ...await more.getByRole('link').evaluateAll(els=>els.map(e=>e.getAttribute('href')))]
  for (const former of ['/control','/machines','/terminals','/skills','/profiles','/sync','/shared','/community','/support','/spaces','/hosting'])
   expect(hrefs, `${former} is reachable`).toContain(former)
  await expect(more.getByRole('link',{name:'Control'})).toHaveAttribute('href','/control')
  // Notifications, the account and the theme are still in the same menu.
  await expect(page.locator('.panel').getByRole('link',{name:/^Notifications/})).toHaveAttribute('href','/notifications')
  await expect(page.locator('.panel').getByRole('group',{name:'Theme'})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // Projects is no longer under More, in the account menu or the phone's More sheet: the rail on Home has them.
 if (run('Projects no longer under More')) {
  for (const [width,touch] of [[1440,false],[390,true]]) {
   const {context,page,errors} = await open({width,touch})
   await page.goto(base)
   if (touch) {
    await page.getByRole('navigation',{name:'Main navigation'}).getByRole('button',{name:'More'}).click()
    await expect(page.getByRole('dialog',{name:'More'}).getByRole('link',{name:'Control'})).toBeVisible()
    await expect(page.getByRole('dialog',{name:'More'}).getByRole('link',{name:'Projects'})).toHaveCount(0)
   } else {
    await page.locator('summary[aria-label^="Account menu"]').click()
    await expect(page.locator('.panel').getByRole('group',{name:'More'}).getByRole('link',{name:'Control'})).toBeVisible()
    await expect(page.locator('.panel').getByRole('link',{name:'Projects'})).toHaveCount(0)
   }
   await expect(page.locator('a[href="/projects"], a[href^="/projects/"]')).toHaveCount(0)
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // The existing pages keep their addresses: /terminals is the Sessions tab and /control opens as before; /projects goes to
 // Home, where folder projects live.
 if (run('/terminals and /control still open their pages')) {
  const {context,page,errors} = await open({width:1440})
  await page.goto(`${base}/control`)
  await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
  expect(await page.title()).toBe('Control · reet.sh')
  await expect(tabs(page).locator('[aria-current="page"]')).toHaveCount(0)
  await page.goto(`${base}/terminals`)
  expect(await page.title()).toBe('Sessions · reet.sh')
  await expect(tabs(page).getByRole('link',{name:'Sessions',exact:true})).toHaveAttribute('aria-current','page')
  await page.goto(`${base}/projects`)
  await expect(page).toHaveURL(`${base}/`)
  expect(await page.title()).toBe('Home · reet.sh')
  expect(errors).toEqual([])
  await context.close()
 }

 // Activity is the account's list, newest first, grouped by day and filterable; the old /activity address goes to the
 // account section without a reload, and an instance without accounts keeps its own Activity page.
 if (run('/activity lists the account activity')) {
  const events = [{id:3,at:Date.now()-30000,source:'ui',action:'project-delete',detail:JSON.stringify({name:'Site'})},{id:2,at:Date.now()-120000,source:'ui',action:'project-create',detail:JSON.stringify({name:'Site',members:2})},{id:1,at:Date.now()-180000,source:'ui',action:'add',detail:JSON.stringify({name:'Toolkit'})}]
  const {context,page,errors} = await open({width:1440,routes:{'/api/events':{events}}})
  await page.goto(`${base}/account/profile`)
  await expect(page.getByRole('navigation',{name:'Account'}).getByRole('link',{name:'Activity'}).first()).toHaveAttribute('href','/account/activity')
  // The old top-level address opens the account's Activity section and is rewritten to the account address.
  await page.goto(`${base}/activity`)
  await expect(page).toHaveURL(`${base}/account/activity`)
  expect(await page.title()).toBe('Activity · Account · reet.sh')
  await expect(page.getByRole('heading',{name:'Activity',level:2})).toBeVisible()
  const lines = await page.locator('main ul.events .label').allInnerTexts()
  expect(lines).toEqual(['Deleted project Site','Created project Site with 2 repositories','Added Toolkit'])
  await expect(page.getByRole('navigation',{name:'Account'}).getByRole('link',{name:'Activity'}).first()).toHaveAttribute('aria-current','page')
  // Grouped by day, with a filter for each area present and counts that add up.
  await expect(page.getByRole('heading',{name:'Today',level:3})).toBeVisible()
  const filters = page.getByRole('group',{name:'Filter activity'}).getByRole('button')
  await expect(filters.getByText('All')).toBeVisible()
  await expect(page.getByRole('group',{name:'Filter activity'}).getByRole('button',{name:/^Projects/})).toHaveAttribute('aria-pressed','false')
  await page.getByRole('group',{name:'Filter activity'}).getByRole('button',{name:/^Skills/}).click()
  await expect(page.getByRole('group',{name:'Filter activity'}).getByRole('button',{name:/^Skills/})).toHaveAttribute('aria-pressed','true')
  await expect(page.locator('main ul.events .label')).toHaveText(['Added Toolkit'])
  await page.goto(`${base}/account#activity`)
  await expect(page).toHaveURL(`${base}/account/activity`)
  await expect(page.locator('main ul.events li')).toHaveCount(3)
  await page.goto(`${base}/account/activity`)
  await expect(page).toHaveURL(`${base}/account/activity`)
  expect(errors).toEqual([])
  await context.close()
  // An open instance has no accounts: Activity keeps its own page and does not call it "Your account".
  const openMode = await open({width:1440,status:{mode:'none',user:null},routes:{'/api/events':{events}}})
  await openMode.page.goto(`${base}/activity`)
  await expect(openMode.page.locator('main ul.events li')).toHaveCount(3)
  await expect(openMode.page.getByText('Your account',{exact:true})).toHaveCount(0)
  await expect(openMode.page.getByRole('heading',{name:'Activity',level:1})).toBeVisible()
  await openMode.context.close()
 }

 // Only a writer in accounts mode can open sessions: a viewer, a single-token sign-in and an open instance get no Sessions tab
 // and no bell, and More still reaches every page they could reach before.
 if (run('viewer sees no Sessions tab and no bell')) {
  for (const [what,options] of [['viewer',{status:{user:{...owner,role:'viewer'}}}],['token',{status:{mode:'token',user:null},token:true}],['open',{status:{mode:'none',user:null}}]]) {
   const {context,page,errors} = await open({width:1440,...options})
   if (options.token) await page.addInitScript(()=>localStorage.setItem('reet-token','fixture-token'))
   await page.goto(base)
   await expect(page.getByRole('region',{name:'Your machines',exact:true})).toBeVisible()
   const top = await tabs(page).getByRole('link').allInnerTexts()
   // An account viewer has no Activity tab (it lives in their account); a token or open sign-in has nowhere else to put it.
   expect(top.map(t=>t.trim()), what).toEqual(options.status?.mode === 'none' || options.status?.mode === 'token' ? ['Home','Machines','Skills','Activity'] : ['Home','Machines','Skills'])
   await expect(page.getByRole('button',{name:/^Needs you/})).toHaveCount(0)
   await expect(page.getByRole('link',{name:'Sessions'})).toHaveCount(0)
   expect(errors, what).toEqual([])
   await context.close()
   const phone = await open({width:390,touch:true,...options})
   if (options.token) await phone.page.addInitScript(()=>localStorage.setItem('reet-token','fixture-token'))
   await phone.page.goto(base)
   await expect(phone.page.getByRole('region',{name:'Your machines',exact:true})).toBeVisible()
   const bar = await tabs(phone.page).locator(':scope a, :scope button').allInnerTexts()
   expect(bar.map(t=>t.trim()), `${what} phone`).toEqual(['Home','Machines','More'])
   await expect(phone.page.getByRole('button',{name:/^Needs you/})).toHaveCount(0)
   await phone.context.close()
  }
 }

 // The tab for the page in view, and only that tab, carries aria-current="page"; a page under More marks More.
 if (run('tab links use aria-current=page')) {
  for (const [path,link] of [['/','Home'],['/terminals','Sessions'],['/machines','Machines'],['/skills','Skills']]) {
   const {context,page} = await open({width:1440})
   await page.goto(`${base}${path}`)
   await expect(tabs(page).getByRole('link',{name:link,exact:true})).toHaveAttribute('aria-current','page')
   await expect(tabs(page).locator('[aria-current]')).toHaveCount(1)
   await context.close()
  }
  for (const [path,link] of [['/sync','Sync'],['/profiles','Setups'],['/control','Control']]) {
   const {context,page} = await open({width:1440})
   await page.goto(`${base}${path}`)
   await expect(tabs(page).locator('[aria-current]')).toHaveCount(0)
   const menu = page.locator('summary[aria-label^="Account menu"]')
   await expect(menu).toHaveClass(/active/)
   await menu.click()
   await expect(page.locator('.panel').getByRole('group',{name:'More'}).getByRole('link',{name:link,exact:true})).toHaveAttribute('aria-current','page')
   await context.close()
  }
  const {context,page} = await open({width:390,touch:true})
  await page.goto(`${base}/sync`)
  await expect(tabs(page).getByRole('button',{name:'More'})).toHaveClass(/current/)
  // Not by colour alone: the current More carries the same underline as a current tab, and says so to assistive tech.
  await expect(tabs(page).getByRole('button',{name:/^More/})).toHaveAccessibleName('More, current page inside')
  const mark = await tabs(page).getByRole('button',{name:/^More/}).evaluate(el=>getComputedStyle(el,'::after').content)
  expect(mark).not.toBe('none')
  await tabs(page).getByRole('button',{name:'More'}).click()
  await expect(page.getByRole('dialog',{name:'More'}).getByRole('link',{name:'Sync'})).toHaveAttribute('aria-current','page')
  await context.close()
 }

 // On a phone, Jump opens a search row; with the keyboard up while typing in it, the tab bar steps aside and the field stays above the keyboard.
 if (run('phone search with the keyboard open hides the tab bar')) {
  for (const width of [320,390]) {
   const {context,page,errors} = await open({width,touch:true})
   await page.addInitScript(()=>{
    const events=new EventTarget(),real=window.visualViewport
    let height=null
    const visual={get width(){return innerWidth},get height(){return height??innerHeight},offsetTop:0,offsetLeft:0,pageTop:0,pageLeft:0,scale:1,
     addEventListener:(...args)=>{events.addEventListener(...args);real?.addEventListener(...args)},removeEventListener:(...args)=>{events.removeEventListener(...args);real?.removeEventListener(...args)}}
    Object.defineProperty(window,'visualViewport',{configurable:true,get:()=>visual})
    window.__keyboard=value=>{height=value;events.dispatchEvent(new Event('resize'))}
   })
   await page.goto(base)
   const bar = tabs(page)
   await expect(bar).toBeVisible()
   // The keyboard over the page without the search focused (another field) leaves the bar alone.
   await page.evaluate(()=>window.__keyboard(420))
   await page.waitForTimeout(100)
   await expect(bar).toBeVisible()
   await page.evaluate(()=>window.__keyboard(null))
   await page.getByRole('button',{name:'Jump to a session, project or machine'}).click()
   const jump = page.getByRole('combobox',{name:'Jump to a session, project or machine'})
   await expect(jump).toBeFocused()
   const size = await jump.evaluate(el=>parseFloat(getComputedStyle(el).fontSize))
   expect(size, 'search input 16px so focusing never zooms').toBeGreaterThanOrEqual(16)
   await page.evaluate(()=>window.__keyboard(420))
   await expect(bar).toBeHidden()
   const box = await jump.boundingBox()
   expect(box.y + box.height).toBeLessThanOrEqual(420)
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
   await page.evaluate(()=>window.__keyboard(null))
   await expect(bar).toBeVisible()
   await page.evaluate(()=>window.__keyboard(420))
   await expect(bar).toBeHidden()
   await jump.blur()
   await expect(bar).toBeVisible()
   expect(errors).toEqual([])
   await context.close()
  }
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
  await page.goto(`${base}/control`)
  const shortcuts = page.locator('section[aria-label="Workspace shortcuts"]')
  await expect(shortcuts.getByRole('link',{name:/^Setups/})).toHaveAttribute('href','/profiles')
  await expect(shortcuts.getByRole('link',{name:/^Sync/})).toHaveAttribute('href','/sync')
  await expect(page.getByText('Reusable AI setups')).toHaveCount(0)
  await expect(page.getByText('Skills & fleet sync')).toHaveCount(0)
  await page.goto(`${base}/machines`)
  await expect(page.locator('main').getByText('Control',{exact:true})).toBeVisible()
  await expect(page.getByText('Your control centre')).toHaveCount(0)
  const tabs = await page.locator('nav[aria-label="Machines"] a').allInnerTexts()
  expect(tabs.map(t=>t.trim())).toEqual(['Inventory & connection','Sync','Sessions','Setups →'])
  await context.close()
 }
 // Light, dark or system can be chosen from the account menu, takes effect at once, and is saved to the profile.
 if (run('theme')) {
  const {context,page,state} = await open({width:1440})
  await page.goto(base)
  await expect(page.getByRole('region',{name:'Your machines',exact:true})).toBeVisible()
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

 // A theme chosen in the menu is what the open Profile form saves next, so changing another preference does not put the old theme back.
 if (run('theme-form')) {
  const {context,page,state} = await open({width:1440})
  await page.goto(`${base}/account/profile`)
  const select = page.locator('label:has(> span:text-is("Theme")) select')
  await expect(select).toHaveValue('light')
  await page.locator('summary[aria-label^="Account menu"]').click()
  await page.locator('.panel').getByRole('group',{name:'Theme'}).getByRole('button',{name:'Dark'}).click()
  await expect(select).toHaveValue('dark')
  await page.getByPlaceholder('Europe/London').fill('Europe/Paris')
  await page.getByRole('button',{name:'Save profile'}).click()
  await expect.poll(()=>state.patches.some(p=>p.timezone==='Europe/Paris')).toBe(true)
  expect(state.patches.find(p=>p.timezone==='Europe/Paris').theme).toBe('dark')
  expect(await page.evaluate(()=>document.documentElement.dataset.theme)).toBe('dark')
  await context.close()
 }

 // The signed-in header is one row on a phone and does not overflow at the widths either side of the menu breakpoint.
 if (run('header')) {
  for (const [width,maxHeight] of [[375,100],[1001,110],[1024,110]]) {
   const {context,page} = await open({width,unread:12})
   await page.goto(base)
   await expect(page.getByRole('region',{name:'Your machines',exact:true})).toBeVisible()
   const m = await page.evaluate(()=>({overflow:document.documentElement.scrollWidth-innerWidth,height:document.querySelector('.site-header').getBoundingClientRect().height}))
   expect(m.overflow, `header overflow at ${width}px`).toBeLessThanOrEqual(0)
   expect(m.height, `header height at ${width}px`).toBeLessThanOrEqual(maxHeight)
   await context.close()
  }
 }
 // Fonts are reet's own: every font request goes to this origin, the headings, body and code faces all load, and none fails.
 if (run('fonts')) {
  const {context,page,errors} = await open({width:1440})
  const fonts = [], failed = []
  page.on('request',r=>{if(r.resourceType()==='font')fonts.push(r.url())})
  page.on('requestfailed',r=>{if(r.resourceType()==='font')failed.push(r.url())})
  await page.goto(base)
  await expect(page.getByRole('region',{name:'Your machines',exact:true})).toBeVisible()
  await page.evaluate(()=>document.fonts.ready)
  const loaded = await page.evaluate(()=>[...document.fonts].filter(f=>f.status==='loaded').map(f=>`${f.family.replace(/"/g,'')} ${f.weight}`))
  for (const face of ['Fraunces 600','IBM Plex Sans 400']) expect(loaded, `${face} loaded`).toContain(face)
  expect(fonts.length, 'fonts were requested').toBeGreaterThan(0)
  for (const url of fonts) expect(new URL(url).origin, url).toBe(new URL(base).origin)
  expect(failed).toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }
 console.log('Navigation: five desktop tabs with Home current, the bell and Jump, four phone tabs with More, every former page reachable, /terminals and /control kept, /projects to Home and Projects no longer under More, /activity, no Sessions or bell for viewers, token and open sign-ins, aria-current, phone search with the keyboard up, notifications in the account menu, one name per area, same-origin fonts.')
} finally {await browser.close()}
