// Phone-width and touch contract for controls that were hidden, squashed or mislabelled.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace, skillRow, skillsPageOf, terminalMachine } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // Every Skills control is reachable on a phone: the two tabs, search, Filters (which holds Liked, Tag, List and Sort) and
 // Select, with nothing past the right edge behind a hidden scroll.
 if (run('filters')) {
  const rows = [skillRow('code-review',{tags:[{id:1,name:'review'}],lists:[{id:7,name:'Daily kit'}]}), skillRow('old-notes',{status:'archived'})]
  const {context,page,errors} = await open({width:375,routes:{'/api/v2/skills':req=>skillsPageOf(rows,new URL(req.url()).searchParams.get('status')||'active')}})
  await page.goto(`${base}/skills`)
  const tabs = page.getByRole('tablist',{name:'Skill state'})
  const find = page.getByRole('group',{name:'Find skills'})
  await expect(find).toBeVisible()
  const filters = find.getByRole('button',{name:/^Filters/})
  await expect(filters).toHaveAttribute('aria-expanded','false')
  await filters.click()
  for (const [what,control] of [['Filters',filters],['Active',tabs.getByRole('tab',{name:/^Active/})],['Archived',tabs.getByRole('tab',{name:/^Archived/})],['Search',find.getByRole('searchbox',{name:'Search skills'})],['Liked',find.getByRole('button',{name:'Liked'})],['Tag',find.getByRole('combobox',{name:'Tag'})],['List',find.getByRole('combobox',{name:'List'})],['Sort',find.getByRole('combobox',{name:'Sort by'})],['Select',find.getByRole('button',{name:'Select',exact:true})]]) {
   const box = await control.boundingBox()
   expect(box, `${what} exists`).not.toBeNull()
   expect(box.x + box.width, `${what} right edge`).toBeLessThanOrEqual(375)
   expect(box.x, `${what} left edge`).toBeGreaterThanOrEqual(0)
   expect(box.height, `${what} height`).toBeGreaterThanOrEqual(44)
  }
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
 }

 // The custom hosted-skill prefix field is wide enough to type in.
 if (run('prefix')) {
  const {context,page,errors} = await open({width:375})
  await page.goto(`${base}/account`)
  const input = page.getByLabel('Your skill name prefix',{exact:true})
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

 // A row's buttons say what they do to the machines: Archive and Delete on an Active skill, Restore and Delete on an
 // Archived one, each naming its skill. The shelf's stage labels are gone.
 if (run('cardlabel')) {
  const rows = [skillRow('code-review'), skillRow('old-notes',{status:'archived'})]
  const {context,page} = await open({width:1440,routes:{'/api/v2/skills':req=>skillsPageOf(rows,new URL(req.url()).searchParams.get('status')||'active')}})
  await page.goto(`${base}/skills`)
  const row = page.locator('[data-skill="item-code-review"]')
  await expect(row.getByRole('button',{name:'Archive code-review'})).toHaveText('Archive')
  await expect(row.getByRole('button',{name:'Delete code-review'})).toHaveText('Delete')
  for (const old of ['Save to test','Mark in use','Start using','Stop using']) await expect(page.getByRole('button',{name:old})).toHaveCount(0)
  await page.getByRole('tab',{name:/^Archived/}).click()
  await expect(page.locator('[data-skill="item-old-notes"]').getByRole('button',{name:'Restore old-notes'})).toHaveText('Restore')
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
   await page.getByRole('button',{name:'Jump to a session, project, skill or machine'}).click()
   const jump = page.getByRole('combobox',{name:'Jump to a session, project, skill or machine'})
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
 console.log('Phone layout: Skills controls reachable, prefix field usable, Show button touch-sized, row labels honest, Jump results above the keyboard at 320 and 390 px.')
} finally {await browser.close()}
