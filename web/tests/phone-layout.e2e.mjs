// Phone-width and touch contract for controls that were hidden, squashed or mislabelled.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace, skill, terminalMachine } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

// The agents fixture shaped like the mock-up: reet and site report, desk does not; a live terminal answers its socket.
const caps = os => ({protocolVersion:1,os,osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,tools:['claude','codex'],worktree:true,browse:true,branches:true,agentsV2:true})
const tmachines = [['a1','studio'],['a2','server'],['a3','desk']].map(([agentId,host]) => ({agentId,host,ready:agentId !== 'a3',serverEnabled:true,capabilities:caps('linux')}))
const health = () => ({serverNow:Date.now(),machines:tmachines.map(m => ({agentId:m.agentId,host:m.host,state:m.agentId === 'a3' ? 'unreachable' : 'connected',capabilities:m.capabilities,connection:{connected:m.agentId !== 'a3',reportFresh:m.agentId !== 'a3',missedProbes:0,reconnects:0,path:'direct'}}))})
const fproject = (id, name, host, machine, colour, position) => ({id,host,machine,dir:`/home/dev/${name}`,name,isRepo:true,baseBranch:'main',worktreeDefault:'inherit',colour,defaultTool:'',defaultModel:'',hasStarterPrompt:false,note:'',position,createdAt:1,updatedAt:1,version:1})
const fprojects = [fproject('p1','reet','a1','studio','green',0), fproject('p2','site','a2','server','blue',1), fproject('p3','notes','a3','desk','',2)]
const fsessions = [
 {id:'s1',agentId:'a1',title:'Refactor the store layer',state:'running',createdAt:Date.now()-7_200_000,updatedAt:Date.now()-720_000,exitCode:null,tool:'claude',model:'sonnet',projectId:'p1',worktreePath:'/home/dev/.reet/worktrees/reet/reet/a1b2c3d4e5f60718',worktreeRepo:'reet',worktreeBranch:'reet/reet-a1b2c3d4e5f60718'},
 {id:'s2',agentId:'a2',title:'Speed up the runner pool',state:'running',createdAt:Date.now()-7_200_000,updatedAt:Date.now()-2_040_000,exitCode:null,tool:'claude',projectId:'p2',yolo:true},
 {id:'s3',agentId:'a3',title:'Review code',state:'running',createdAt:Date.now()-7_200_000,updatedAt:Date.now()-540_000,exitCode:null,tool:'codex',projectId:'p3'},
]
// A visual viewport whose height a test can shrink, as an on-screen keyboard does.
const keyboardHook = () => {
 const events=new EventTarget(),real=window.visualViewport
 let height=null
 const visual={get width(){return innerWidth},get height(){return height??innerHeight},offsetTop:0,offsetLeft:0,pageTop:0,pageLeft:0,scale:1,
  addEventListener:(...args)=>{events.addEventListener(...args);real?.addEventListener(...args)},removeEventListener:(...args)=>{events.removeEventListener(...args);real?.removeEventListener(...args)}}
 Object.defineProperty(window,'visualViewport',{configurable:true,get:()=>visual})
 window.__keyboard=value=>{height=value;events.dispatchEvent(new Event('resize'))}
}
// A phone (touch) on a session's screen, or on `path`.
async function openSession({width, height, path='/sessions?agent=a1&session=s1'}) {
 const opened = await open({width, height, touch:true, terminalStatus:active, terminalMachines:tmachines, routes:{
  '/api/v2/machines/health': () => health(),
  '/api/v2/terminal/sessions': {sessions:fsessions},
  '/api/v2/folder-projects': {projects:fprojects,savedWaiting:0},
  '/api/v2/terminal/commands': {commands:[]},
  '/api/v2/terminal/worktrees/status': {path:'',branch:'',changed:0,unpushed:0,paths:[],unreferenced:0,hidden:0,nested:false,incomplete:false},
 }})
 await opened.page.addInitScript(keyboardHook)
 await opened.page.route('**/api/v2/terminal/sessions/*/attachments', route => route.fulfill({json:{ticket:'phone-layout-ticket0001'}}))
 await opened.context.routeWebSocket('**/api/v2/terminal/socket', socket => socket.onMessage(message => { if (typeof message === 'string' && JSON.parse(message).ticket) { socket.send('{"type":"ready"}'); socket.send(Buffer.from('ready> ')) } }))
 await opened.page.goto(`${base}${path}`)
 if (path.includes('session=')) await expect(opened.page.locator('.pane')).toHaveAttribute('data-connection-state','live')
 return opened
}
// Whole rows the terminal's screen shows: its height over the rendered row height.
const visibleRows = page => page.evaluate(() => { const screen = document.querySelector('.screen'), rows = document.querySelectorAll('.xterm-rows>div').length, grid = document.querySelector('.xterm-screen').getBoundingClientRect().height; return Math.floor(screen.clientHeight / (grid / rows) + 0.01) })

try {
 // Every shelf filter is reachable on a phone: nothing sits past the right edge behind a hidden scroll.
 if (run('filters')) {
  const {context,page,errors} = await open({width:375,skills:[skill('code-review')]})
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('group',{name:'Filter and sort'})).toBeVisible()
  for (const name of ['All','In use','To test','On the shelf','Liked','Select']) {
   const box = await page.locator('.filters button',{hasText:name}).first().boundingBox()
   expect(box, `${name} filter exists`).not.toBeNull()
   expect(box.x + box.width, `${name} filter right edge`).toBeLessThanOrEqual(375)
   expect(box.x, `${name} filter left edge`).toBeGreaterThanOrEqual(0)
  }
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
 }

 // The custom hosted-skill prefix field is wide enough to type in.
 if (run('prefix')) {
  const {context,page,errors} = await open({width:375})
  await page.goto(`${base}/account`)
  const input = page.getByLabel('Hosted skill name prefix',{exact:true})
  await expect(input).toBeVisible()
  const box = await input.boundingBox()
  expect(box.width, 'prefix field width at 375px').toBeGreaterThanOrEqual(140)
  expect(box.x + box.width, 'prefix field right edge').toBeLessThanOrEqual(375)
  expect(errors).toEqual([])
  await context.close()
 }

 // The password Show button is a touch-sized target.
 if (run('showbutton')) {
  const {context,page} = await open({width:375,status:{user:null,setupRequired:true}})
  await page.goto(`${base}/setup`)
  const box = await page.getByRole('button',{name:'Show password'}).first().boundingBox()
  expect(box.height, 'Show button height').toBeGreaterThanOrEqual(44)
  expect(box.width, 'Show button width').toBeGreaterThanOrEqual(44)
  await context.close()
 }

 // "Start using" only marks a skill as in use; the label says so, since installing is a separate action.
 if (run('cardlabel')) {
  const {context,page} = await open({width:1440,skills:[skill('code-review')]})
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('button',{name:'Save to test'})).toBeVisible()
  await expect(page.getByRole('button',{name:'Mark in use'})).toBeVisible()
  await expect(page.getByRole('button',{name:'Start using'})).toHaveCount(0)
  await context.close()
 }
 // Jump search on a phone with the keyboard up, at 320 and 390 px: the field is 16 px (focusing never zooms), the bar steps
 // aside, and the results for 80-character unbroken session, Project and machine names sit between the field and the
 // keyboard, every one reachable by scrolling the list, each at least 44 px tall, with nothing wider than the screen.
 if (run('Jump search with the keyboard open')) {
  const long = what => `${what}_${'abcdefghij'.repeat(8)}`.slice(0, 80)
  const host = long('host.eu.west.build.server'), title = long('Refactor_the_store_layer_session'), project = long('Store_rewrite_project')
  const sessions = Array.from({length:6}, (_, i) => ({id:`s${i}`,agentId:'a1',title:`${i}${title}`.slice(0, 80),state:'running',createdAt:1,updatedAt:1,exitCode:null}))
  for (const width of [320,390]) {
   const {context,page,errors} = await open({width,touch:true,terminalStatus:active,terminalMachines:[terminalMachine('a1',host,true)],
    routes:{'/api/v2/terminal/sessions':{sessions},'/api/v2/folder-projects':{projects:[{id:'fp1',host:'a1',machine:host,dir:'/home/dev/store',name:project,isRepo:true,baseBranch:'',worktreeDefault:'inherit',defaultTool:'',defaultModel:'',hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1}],savedWaiting:0}}})
   await page.addInitScript(()=>{
    const events=new EventTarget(),real=window.visualViewport
    let height=null
    const visual={get width(){return innerWidth},get height(){return height??innerHeight},offsetTop:0,offsetLeft:0,pageTop:0,pageLeft:0,scale:1,
     addEventListener:(...args)=>{events.addEventListener(...args);real?.addEventListener(...args)},removeEventListener:(...args)=>{events.removeEventListener(...args);real?.removeEventListener(...args)}}
    Object.defineProperty(window,'visualViewport',{configurable:true,get:()=>visual})
    window.__keyboard=value=>{height=value;events.dispatchEvent(new Event('resize'))}
   })
   await page.goto(`${base}/skills`)
   await page.getByRole('button',{name:'Jump to a session, project or machine'}).click()
   const jump = page.getByRole('combobox',{name:'Jump to a session, project or machine'})
   await expect(jump).toBeFocused()
   expect(await jump.evaluate(el=>parseFloat(getComputedStyle(el).fontSize)), `${width}px: 16 px field`).toBeGreaterThanOrEqual(16)
   const keyboard = 400
   await page.evaluate(k=>window.__keyboard(k), keyboard)
   await expect(page.getByRole('navigation',{name:'Main navigation'})).toBeHidden()
   await jump.fill('abcdefghij')
   const list = page.getByRole('listbox',{name:'Jump results'})
   await expect(list.getByRole('option')).toHaveCount(8)
   for (const name of [project, host]) await expect(list.locator('.label').getByText(name,{exact:true})).toHaveCount(1)
   const results = page.locator('.results')
   const box = await results.boundingBox()
   expect(box.y + box.height, `${width}px: results end above the keyboard`).toBeLessThanOrEqual(keyboard + 1)
   expect(box.x, `${width}px`).toBeGreaterThanOrEqual(0)
   expect(box.x + box.width, `${width}px`).toBeLessThanOrEqual(width)
   // Each result, scrolled into the list, sits inside it, above the keyboard and within the screen, at least 44 px tall.
   for (const option of await list.getByRole('option').all()) {
    await option.evaluate(el => el.scrollIntoView({block:'nearest'}))
    const r = await option.boundingBox()
    expect(r.height, `${width}px option height`).toBeGreaterThanOrEqual(44)
    expect(r.y + r.height, `${width}px option above the keyboard`).toBeLessThanOrEqual(keyboard + 1)
    expect(r.x + r.width, `${width}px option within the screen`).toBeLessThanOrEqual(width)
   }
   expect(await page.evaluate(()=>[document.documentElement.scrollWidth<=innerWidth, innerWidth]), `${width}px: no sideways scroll or zoom`).toEqual([true, width])
   expect(errors).toEqual([])
   await context.close()
  }
 }
 // The session screen on a phone (PR H): the mock-up's slim header (back, title, project · machine, the state) and, with the
 // keyboard up for the terminal, the site chrome (top bar and tab bar) steps aside by the same usable-height rule
 // (keyboardChrome with the 'session' focus); the terminal's field stays inside the visual viewport, above the keyboard.
 if (run('session with keyboard open')) {
  for (const width of [320,390]) {
   const height = 844, keyboard = Math.round(height * .55)
   const {context,page,errors} = await openSession({width,height})
   const header = page.locator('.session-screen .head')
   await expect(header).toBeVisible()
   await expect(header.getByRole('link',{name:'Back to sessions'})).toHaveAttribute('href','/sessions')
   // One way back: the toolbar's arrow gives way to the header's.
   await expect(page.getByRole('link',{name:'Back to sessions'})).toHaveCount(1)
   await expect(header.locator('.where-phone')).toHaveText('reet · studio')
   await expect(header.locator('.pill')).toContainText('Running')
   for (const target of await header.locator('a, button').all()) if (await target.isVisible()) {
    const b = await target.boundingBox()
    expect(Math.min(b.width, b.height), `${width}px header target`).toBeGreaterThanOrEqual(44)
   }
   await expect(page.locator('.site-header')).toBeVisible()
   await page.getByRole('button',{name:'Focus terminal',exact:true}).click()
   const field = page.locator('.xterm-helper-textarea')
   await expect(field).toBeFocused()
   await page.evaluate(k => window.__keyboard(k), keyboard)
   // The top bar and the tab bar step aside; the slim header stays, as drawn.
   await expect(page.locator('.site-header')).toBeHidden()
   await expect(page.getByRole('navigation',{name:'Main navigation'})).toBeHidden()
   await expect(page.locator('html')).toHaveAttribute('data-chrome-aside','')
   await expect(header).toBeVisible()
   await page.waitForTimeout(450)
   const pane = await page.locator('.pane').boundingBox(), caret = await field.boundingBox(), head = await header.boundingBox()
   expect(head.y, `${width}px: the slim header starts at the top`).toBeLessThanOrEqual(12)
   expect(pane.y + pane.height, `${width}px: the pane ends above the keyboard`).toBeLessThanOrEqual(keyboard + 1)
   expect(pane.y + pane.height, `${width}px: and uses the room down to it`).toBeGreaterThanOrEqual(keyboard - 10)
   // The slim header staying costs no rows: the pane's own header steps aside instead (24 rows before and after, PR H).
   expect(await visibleRows(page), `${width}px: keyboard-open rows`).toBeGreaterThanOrEqual(24)
   expect(caret.y, `${width}px: the field is inside the visual viewport`).toBeGreaterThanOrEqual(0)
   expect(caret.y + caret.height, `${width}px: the field is above the keyboard`).toBeLessThanOrEqual(keyboard)
   expect(await page.evaluate(() => [document.documentElement.scrollWidth <= innerWidth, innerWidth]), `${width}px: no sideways scroll or zoom`).toEqual([true, width])
   if (process.env.REET_UI_SCREENSHOTS) await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/session-keyboard-${width}.png`})
   // REET_AXE=<axe.min.js> (outside the repository) checks the keyboard-up layout too.
   if (process.env.REET_AXE) {
    await page.addScriptTag({path:process.env.REET_AXE})
    const r = await page.evaluate(() => window.axe.run(document, {runOnly:{type:'tag',values:['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa']}}))
    console.log(`axe ${width} keyboard up: ${r.violations.length} violations ${JSON.stringify(r.violations.map(v => [v.id, v.nodes.map(n => n.target.join(' '))]))}, ${r.incomplete.length} incomplete ${JSON.stringify(r.incomplete.map(v => [v.id, v.nodes.length]))}`)
   }
   // The keyboard goes: the chrome comes back.
   await page.evaluate(() => window.__keyboard(null))
   await expect(page.locator('.site-header')).toBeVisible()
   await expect(page.locator('html')).not.toHaveAttribute('data-chrome-aside','')
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // A phone held sideways (844 × 390): the Sessions screen and the session screen fit without sideways scroll; the session
 // screen gives its header's rows to the terminal (the pane names the session) and keeps at least 12 rows, and with the
 // keyboard up the top bar steps aside too.
 if (run('sideways 844 × 390')) {
  const width = 844, height = 390
  const {context,page,errors} = await openSession({width,height,path:'/sessions'})
  await expect(page.locator('main .session-row')).toHaveCount(3)
  await expect(page.getByRole('navigation',{name:'Main navigation'})).toBeVisible()
  await expect(page.locator('main aside.rail')).toBeVisible()
  expect(await page.evaluate(() => [document.documentElement.scrollWidth <= innerWidth, innerWidth]), 'Sessions sideways').toEqual([true, width])
  await page.goto(`${base}/sessions?agent=a1&session=s1`)
  await expect(page.locator('.pane')).toHaveAttribute('data-connection-state','live')
  await expect(page.locator('.session-screen .head')).toBeHidden()
  await expect(page.getByRole('navigation',{name:'Main navigation'})).toHaveCount(0)
  await page.waitForTimeout(450)
  expect(await visibleRows(page), 'sideways rows with the keyboard closed').toBeGreaterThanOrEqual(12)
  expect(await page.evaluate(() => [document.documentElement.scrollWidth <= innerWidth, innerWidth]), 'session sideways').toEqual([true, width])
  const top = Math.round(height * .44)
  await page.locator('.xterm-helper-textarea').focus()
  await page.evaluate(k => window.__keyboard(k), top)
  await expect(page.locator('.site-header')).toBeHidden()
  await expect(page.locator('html')).toHaveAttribute('data-chrome-aside','')
  await page.waitForTimeout(450)
  const pane = await page.locator('.pane').boundingBox()
  expect(pane.y + pane.height).toBeLessThanOrEqual(top + 1)
  if (process.env.REET_UI_SCREENSHOTS) await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/session-sideways-keyboard.png`})
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
 }

 // The phone rail on Sessions (moved from under In flight on Home): the full rail with its menu, Select and state words, under
 // the session groups as the mock-up's "Projects · N" with Add a project; phone Home has no rail any more.
 if (run('phone rail on Sessions')) {
  const {context,page,errors} = await openSession({width:390,height:844,path:'/sessions'})
  const rail = page.locator('main aside.rail')
  await expect(rail).toBeVisible()
  await expect(rail.getByRole('heading',{name:'Projects · 3'})).toBeVisible()
  const add = rail.getByRole('button',{name:'Add a project'})
  await expect(add).toHaveText('Add a project')
  expect((await add.boundingBox()).height).toBeGreaterThanOrEqual(44)
  // Below the sessions, as drawn.
  expect((await rail.boundingBox()).y).toBeGreaterThan((await page.locator('main .session-row').last().boundingBox()).y)
  // Each card says its state in words.
  await expect(rail.getByRole('button',{name:'reet, Green, studio, main, 1 running, 1 worktree in use',exact:true})).toBeVisible()
  await expect(rail.getByRole('button',{name:'site, Blue, server, main, 1 running',exact:true})).toBeVisible()
  await expect(rail.locator('.card .state')).toHaveText(['1 running','1 running','1 running'])
  for (const target of await rail.locator('button').all()) if (await target.isVisible()) expect((await target.boundingBox()).height).toBeGreaterThanOrEqual(44)
  // The card's menu.
  await rail.getByRole('button',{name:'More for reet',exact:true}).click()
  const menu = page.getByRole('menu',{name:'reet'})
  for (const item of ['New session here','Edit','Worktrees','Move up','Move down','Remove from reet']) await expect(menu.getByRole('menuitem',{name:item})).toBeVisible()
  await page.keyboard.press('Escape')
  // Select (the heading's menu, or a long press on touch), then the chosen projects go to Home's launch bar in the address (ids only).
  await rail.getByRole('button',{name:'Projects menu'}).click()
  await page.getByRole('menuitem',{name:'Select projects'}).click()
  await rail.getByRole('checkbox',{name:/^reet, /}).click()
  await expect(rail.getByRole('link',{name:'Start in these on Home'})).toHaveCount(0)
  await rail.getByRole('checkbox',{name:/^site, /}).click()
  await expect(rail.getByRole('link',{name:'Start in these on Home'})).toHaveAttribute('href','/?projects=p1,p2')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  // A card opens Home's launch bar on that project; Home on a phone shows no rail.
  await rail.getByRole('button',{name:'Done selecting'}).click()
  await rail.getByRole('button',{name:/^site, /}).click()
  await expect(page).toHaveURL(`${base}/?project=p2`)
  await expect(page.locator('.summary-text b')).toHaveText('site')
  await expect(page.locator('main aside.rail')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }
 console.log('Phone layout: shelf filters reachable, prefix field usable, Show button touch-sized, card label honest, Jump results above the keyboard at 320 and 390 px, the session screen with the keyboard open at 320 and 390 px, sideways 844 × 390, and the phone rail on Sessions.')
} finally {await browser.close()}
