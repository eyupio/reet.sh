// The Agents home across screens and themes (PR H, task H4): Home, the Add project dialog, the launch bar's slot list and
// Change sheet, the Sessions page and the session screen, at every width the spec names, light and dark, with and without
// reduced motion, and with the phone keyboard up. A local one-off, not run in CI (the CI jobs keep a smaller subset; see
// docs/ui-verification.md): about 200 states take several minutes, more than the plan's CI budget allows.
//
// Run against a preview of the built app (as the e2e fixtures are):
//   npm --prefix web run preview -- --host 127.0.0.1 --port 4173 &
//   node web/tests/matrix/agents-home.matrix.mjs
// REET_AXE=<path to axe.min.js> also runs axe-core (WCAG 2.0/2.1 A and AA, 2.2 AA) at 390, 834 and 1440 px in both themes
// and prints violations and the `incomplete` count; axe-core is not a dependency of the repository.
// REET_MATRIX_ONLY=<screen> runs one screen. Synthetic API answers; nothing is started on a machine.
//
// Each state asserts: no sideways scroll; on a touch screen every control at least 44 px (48 px tall for a phone's primary
// action), with the exceptions named in `exempt`; the focus ring visible on every Tab stop; a state dot always beside its word.
import { chromium } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { active, openWorkspace } from '../helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const only = process.env.REET_MATRIX_ONLY
const axeSource = process.env.REET_AXE ? readFileSync(process.env.REET_AXE, 'utf8') : ''

// The mock-up's fleet: studio and server report, desk does not; reet, site and notes.
const MIN = 60_000, HOUR = 60 * MIN
const caps = os => ({protocolVersion:1,os,osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,tools:['claude','codex','opencode'],worktree:true,browse:true,branches:true,agentsV2:true})
const tmachines = [['a1','studio'],['a2','server'],['a3','desk']].map(([agentId,host]) => ({agentId,host,ready:agentId !== 'a3',serverEnabled:true,capabilities:caps(agentId === 'a3' ? 'darwin' : 'linux')}))
const report = cpu => ({cpuPercent:{value:cpu},memory:{totalBytes:{value:4},availableBytes:{value:2}},disk:{totalBytes:{value:4},availableBytes:{value:2}},hostUptimeSeconds:{value:1},scope:'host'})
const health = () => ({serverNow:Date.now(),machines:tmachines.map((m, i) => ({agentId:m.agentId,host:m.host,state:m.agentId === 'a3' ? 'unreachable' : 'connected',capabilities:m.capabilities,connection:{connected:m.agentId !== 'a3',reportFresh:m.agentId !== 'a3',missedProbes:0,reconnects:0,path:'direct',...(m.agentId !== 'a3' ? {report:report([38,12][i])} : {})}}))})
const project = (id, name, host, machine, colour, position) => ({id,host,machine,dir:`/home/dev/${name}`,name,isRepo:true,baseBranch:'main',worktreeDefault:'inherit',colour,defaultTool:'',defaultModel:'',hasStarterPrompt:false,note:'',position,createdAt:1,updatedAt:1,version:1})
const projects = () => [project('p1','reet','a1','studio','green',0), project('p2','site','a2','server','blue',1), project('p3','notes','a3','desk','',2)]
const session = (id, agentId, title, ago, over={}) => ({id,agentId,title,state:'running',createdAt:Date.now()-2*HOUR,updatedAt:Date.now()-ago,exitCode:null,...over})
const sessions = () => [
 session('s1','a1','Refactor the store layer',12*MIN,{tool:'claude',model:'sonnet',projectId:'p1',worktreePath:'/home/dev/.reet/worktrees/reet/reet/a1b2c3d4e5f60718',worktreeRepo:'reet',worktreeBranch:'reet/reet-a1b2c3d4e5f60718'}),
 session('s2','a1','Add browse-v1 protocol',2*MIN,{tool:'claude',model:'sonnet',projectId:'p1'}),
 session('s3','a1','Docs pass',HOUR,{tool:'codex',projectId:'p1',state:'exited',exitCode:0}),
 session('s4','a2','Speed up the runner pool',34*MIN,{tool:'claude',projectId:'p2'}),
 session('s5','a3','Review code',9*MIN,{tool:'codex',projectId:'p3'}),
]
const sync = () => ({environments:[],machines:[],agents:[{id:'a1',host:'studio',version:'test',lastSeen:Date.now(),paused:false,autoUpdate:false}],
 targets:[{target:{id:'t1',host:'studio'},revision:'r1',sourceId:'',sourceName:'',desired:[],missing:[],extra:[],updatedAt:1}],
 jobs:[{id:'j1',agentId:'a1',targetId:'t1',kind:'apply',status:'succeeded',revision:'r1',hash:'',plan:{actions:[]},message:'',createdAt:1,updatedAt:Date.now()-3*MIN}],runs:[],updateJobs:[]})
const changed = ['internal/terminalproto/browse.go','internal/terminalhost/browse.go','docs/terminal-launch.md']
const routes = {
 '/api/v2/machines/health': () => health(),
 '/api/v2/terminal/sessions': () => ({sessions:sessions()}),
 '/api/v2/folder-projects': () => ({projects:projects(),savedWaiting:0}),
 '/api/v2/folder-projects/p1': () => ({...projects()[0], starterPrompt:''}),
 '/api/v2/terminal/commands': {commands:[]},
 '/api/v2/terminal/worktrees/status': {path:'',branch:'reet/reet-a1b2c3d4e5f60718',changed:3,unpushed:0,paths:changed,unreferenced:0,hidden:0,nested:false,incomplete:false},
 '/api/v2/machines/a1/branches': {current:'main',default:'main',branches:['main','develop'],more:false},
 '/api/v2/machines/a1/browse': request => {
  const path = new URL(request.url()).searchParams.get('path') || '/home/dev'
  return {path,home:'/home/dev',parent:'',repo:'none',repoTop:'',entries:path === '/home/dev' ? [{name:'reet',dir:true,repoRoot:true,hidden:false},{name:'site',dir:true,repoRoot:true,hidden:false},{name:'notes',dir:true,repoRoot:false,hidden:false}] : [],more:false,partial:false,scanned:3,omitted:0}
 },
}
// A visual viewport whose height a test can shrink, as an on-screen keyboard does (as sessions-phone.e2e.mjs).
const keyboardHook = () => {
 const events = new EventTarget(), real = window.visualViewport
 let height = null
 const visual = {get width(){return innerWidth},get height(){return height ?? innerHeight},offsetTop:0,offsetLeft:0,pageTop:0,pageLeft:0,scale:1,
  addEventListener:(...a) => { events.addEventListener(...a); real?.addEventListener(...a) },removeEventListener:(...a) => { events.removeEventListener(...a); real?.removeEventListener(...a) }}
 Object.defineProperty(window, 'visualViewport', {configurable:true, get:() => visual})
 window.__keyboard = value => { height = value; events.dispatchEvent(new Event('resize')) }
}

async function open({width, height, touch, theme, motion}) {
 const opened = await openWorkspace(browser, {width, height, touch, theme, routes, sync:sync(), terminalStatus:active, terminalMachines:tmachines})
 await opened.page.emulateMedia({reducedMotion:motion, colorScheme:theme})
 await opened.page.addInitScript(keyboardHook)
 await opened.page.route('**/api/v2/terminal/sessions/*/attachments', route => route.fulfill({json:{ticket:'matrix-fixture-ticket001'}}))
 await opened.context.routeWebSocket('**/api/v2/terminal/socket', socket => socket.onMessage(m => { if (typeof m === 'string' && JSON.parse(m).ticket) { socket.send('{"type":"ready"}'); socket.send(Buffer.from('ready> ')) } }))
 return opened
}
const visible = locator => locator.waitFor({state:'visible', timeout:10_000})
// Each screen: where it opens and what makes it ready; `field` is what has the keyboard on a phone.
const screens = {
 home: {go: async page => { await page.goto(base); await visible(page.getByRole('region',{name:'In flight'}).locator('a.row').first()) },
  field: page => page.getByRole('form',{name:'Start a session'}).locator('textarea')},
 'add-project': {go: async (page, wide) => {
  await page.goto(wide ? base : `${base}/sessions`)
  const add = page.getByRole('button',{name:'Add a project',exact:true}).first()
  await visible(add); await add.click()
  await visible(page.getByRole('dialog',{name:'Add a project'}).getByRole('list',{name:/^Folders in /}))
 }},
 launch: {go: async (page, wide) => {
  await page.goto(base)
  const bar = page.getByRole('form',{name:'Start a session'})
  await visible(bar.locator('textarea'))
  const change = bar.getByRole('button',{name:'Change',exact:true})
  if (await change.isVisible()) { await change.click(); await visible(page.getByRole('dialog',{name:'Change the launch'})) }
  else { await bar.getByRole('button',{name:/^Agent: /}).click(); await visible(page.getByRole('listbox',{name:'Agent'})) }
 }},
 sessions: {go: async page => { await page.goto(`${base}/sessions`); await visible(page.locator('main .session-row').first()) },
  field: page => page.getByRole('searchbox').first()},
 session: {go: async page => {
  await page.goto(`${base}/sessions?agent=a1&session=s1`)
  await page.locator('.pane[data-connection-state=live]').waitFor({timeout:10_000})
 }, field: page => page.locator('.xterm-helper-textarea'), focus: page => page.locator('.xterm-helper-textarea').focus()},
}
const widths = [
 {name:'1440', width:1440, height:900}, {name:'1360', width:1360, height:900}, {name:'1024', width:1024, height:768},
 {name:'834', width:834, height:1112}, {name:'834 touch', width:834, height:1112, touch:true},
 {name:'412 phone', width:412, height:915, touch:true, phone:true}, {name:'390 phone', width:390, height:844, touch:true, phone:true},
 {name:'360 phone', width:360, height:800, touch:true, phone:true}, {name:'320 phone', width:320, height:700, touch:true, phone:true},
 {name:'844 × 390 sideways', width:844, height:390, touch:true, phone:true, sideways:true},
]

// What one state finds: sideways scroll, small targets, Tab stops without a visible ring, dots without words, motion.
const inspect = (page, {touch, phone, motion}) => page.evaluate(({touch, phone, motion}) => {
 // checkVisibility leaves out what a closed <details> holds (laid out, but not drawn).
 const shown = el => { const r = el.getBoundingClientRect(), s = getComputedStyle(el); return r.width > 0 && r.height > 0 && r.bottom > 0 && r.right > 0 && el.checkVisibility({visibilityProperty:true}) && !el.closest('[inert],[aria-hidden=true]') && !(s.clip === 'rect(0px, 0px, 0px, 0px)' || s.clipPath === 'inset(50%)') }
 const name = el => (el.getAttribute('aria-label') || el.textContent || el.getAttribute('name') || el.tagName).replace(/\s+/g, ' ').trim().slice(0, 40)
 const scope = document.querySelector('dialog[open]') || document
 // Exempt, each for a stated reason: the terminal's own text area and its keys (the spec allows 40 px for them), links inside
 // a sentence (WCAG 2.5.8 inline exception), the radio inputs behind segmented controls (their label is the target).
 // The site footer (account, Admin and EyUp.io links) predates the agents home and is outside it; it is listed in
 // docs/ui-verification.md instead.
 const exempt = el => el.closest('.xterm') || (el.closest('footer') && !el.closest('main, dialog, aside, .pane')) || (el.tagName === 'A' && el.closest('p') && !el.matches('.btn, .button')) || (el.matches('input[type=radio], input[type=checkbox]') && el.closest('label'))
 const small = []
 if (touch) for (const el of scope.querySelectorAll('a[href], button, input:not([type=hidden]), select, textarea, summary, [role=button], [role=option], [role=tab], [role=menuitem]')) {
  if (!shown(el) || exempt(el)) continue
  // A field inside a label (the search boxes) is tapped through its label, which is the target.
  const r = (el.matches('input, select, textarea') && el.closest('label')) ? el.closest('label').getBoundingClientRect() : el.getBoundingClientRect()
  // The terminal's own keys (the pane's toolbar and extra keys) are 40 × 40 px on every touch screen; on a phone the rest of
  // the terminal's chrome (the workspace toolbar, the pane's header and footer) is too, as docs/terminal-workspace.md
  // (Phones) says, so the terminal keeps its rows.
  const terminalChrome = el.closest('.terminal-toolbar, .mobile-keys') || (phone && el.closest('.workspace-toolbar, .pane'))
  const need = terminalChrome ? 40 : 44
  if (Math.round(r.height) < need || (Math.round(r.width) < need && !el.matches('input, textarea, select'))) small.push(`${name(el)} ${Math.round(r.width)}×${Math.round(r.height)}`)
  else if (phone && !terminalChrome && el.matches('.btn.primary, button.primary') && Math.round(r.height) < 48) small.push(`${name(el)} primary ${Math.round(r.height)} tall`)
 }
 const words = /running|exited|ended|ending|starting|stopped|connection lost|online|offline|reconnecting|paused|not reporting|no report|last known|waiting|finished|failed|started|in sync|out of date|not checked|live/i
 const mute = []
 for (const dot of document.querySelectorAll('[class~=dot], [class*="-dot"]')) {
  if (!shown(dot)) continue
  const holder = dot.closest('a, li, button, [role=option], .row, .session-row, .tab, .pill, .card, article, header, p') || dot.parentElement
  if (!words.test(holder.textContent + ' ' + (holder.getAttribute('aria-label') || ''))) mute.push(name(holder))
 }
 const moving = motion === 'reduce' ? document.getAnimations().filter(a => a.playState === 'running' && (a.effect?.getComputedTiming().duration ?? 0) > 20).map(a => a.animationName || a.transitionProperty || 'animation') : []
 return {sideways:document.documentElement.scrollWidth > innerWidth ? document.documentElement.scrollWidth : 0, small, mute, moving}
}, {touch, phone, motion})

// Tab through the first stops of the page (or the open dialog) and check each has a visible focus indicator.
async function tabStops(page, count = 14) {
 const missing = []
 for (let i = 0; i < count; i++) {
  await page.keyboard.press('Tab')
  const seen = await page.evaluate(() => {
   const el = document.activeElement
   if (!el || el === document.body || el.closest('.xterm')) return null
   // A ring is an outline or a shadow on the element or on the label around it (a visually hidden radio, a search box whose
   // frame takes the ring), or a frame whose colour changes with focus.
   const nodes = [el, el.closest('label'), el.parentElement].filter(Boolean)
   const look = () => nodes.map(node => { const s = getComputedStyle(node); return {ring:(s.outlineStyle !== 'none' && parseFloat(s.outlineWidth) >= 1) || (s.boxShadow && s.boxShadow !== 'none'), border:s.borderColor + s.backgroundColor} })
   const focused = look()
   if (focused.some(n => n.ring)) return ''
   el.blur(); const blurred = look(); el.focus()
   return focused.some((n, i) => n.border !== blurred[i].border) ? '' : (el.getAttribute('aria-label') || el.textContent || el.tagName).replace(/\s+/g, ' ').trim().slice(0, 40)
  })
  if (seen) missing.push(seen)
 }
 return [...new Set(missing)]
}

const failures = [], axeRows = []
let states = 0
for (const [screen, def] of Object.entries(screens)) {
 if (only && only !== screen) continue
 for (const w of widths) for (const theme of ['light','dark']) for (const motion of ['no-preference','reduce']) {
  const label = `${screen} @ ${w.name} ${theme}${motion === 'reduce' ? ' reduced-motion' : ''}`
  const {context, page, errors} = await open({...w, theme, motion})
  try {
   await def.go(page, w.width >= 1100)
   await page.waitForTimeout(motion === 'reduce' ? 50 : 400)
   states++
   const found = await inspect(page, {...w, motion})
   const problems = []
   if (found.sideways) problems.push(`sideways scroll to ${found.sideways}px`)
   if (found.small.length) problems.push(`small targets: ${found.small.join('; ')}`)
   if (found.mute.length) problems.push(`dot without a word: ${found.mute.join('; ')}`)
   if (found.moving.length) problems.push(`moving with reduced motion: ${found.moving.join(', ')}`)
   if (theme === 'light' && motion === 'no-preference') {
    const rings = await tabStops(page)
    if (rings.length) problems.push(`no visible focus: ${rings.join('; ')}`)
   }
   // Home on a phone puts Needs you first, and has no project rail (it is on Sessions).
   if (screen === 'home' && w.phone && !w.sideways) {
    const order = await page.evaluate(() => { const n = document.querySelector('[aria-label="Needs you"], section[aria-labelledby] h2'); const need = [...document.querySelectorAll('section')].find(s => /Needs you/.test(s.querySelector('h2')?.textContent || '')); const fl = [...document.querySelectorAll('section')].find(s => /In flight/.test(s.querySelector('h2')?.textContent || '')); return need && fl ? need.getBoundingClientRect().top < fl.getBoundingClientRect().top : null })
    if (order !== true) problems.push(`Needs you is not above In flight (${order})`)
    if (await page.getByRole('complementary',{name:'Projects'}).count()) problems.push('project rail on phone Home')
   }
   // The keyboard up on a phone: the field stays above it and nothing scrolls sideways.
   if (w.phone && def.field && motion === 'no-preference') {
    if (def.focus) await def.focus(page); else await def.field(page).focus()
    const keyboard = Math.round(w.height * (w.sideways ? .45 : .55))
    await page.evaluate(k => window.__keyboard(k), keyboard)
    await page.waitForTimeout(450)
    const box = await def.field(page).boundingBox()
    const sideways = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)
    if (!box || box.y + Math.min(box.height, 24) > keyboard + 1) problems.push(`keyboard open: field ${box ? `at ${Math.round(box.y)}–${Math.round(box.y + box.height)}` : 'gone'} below the keyboard at ${keyboard}`)
    if (sideways) problems.push('keyboard open: sideways scroll')
    await page.evaluate(() => window.__keyboard(null))
   }
   if (axeSource && motion === 'no-preference' && ['1440','834 touch','390 phone'].includes(w.name)) {
    await page.addScriptTag({content:axeSource})
    const r = await page.evaluate(() => window.axe.run(document, {runOnly:{type:'tag', values:['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa']}}))
    axeRows.push(`${screen} ${w.name.split(' ')[0]} ${theme}: ${r.violations.length} violations${r.violations.map(v => ` [${v.id}: ${v.nodes.map(n => n.target.join(' ')).slice(0,3).join(' | ')}]`).join('')}; incomplete ${r.incomplete.length}${r.incomplete.map(v => ` [${v.id}: ${v.nodes.length}]`).join('')}`)
   }
   if (errors.length) problems.push(`page errors: ${errors.join('; ')}`)
   if (problems.length) failures.push(`${label}\n   - ${problems.join('\n   - ')}`)
  } catch (error) {
   failures.push(`${label}\n   - did not open: ${error.message.split('\n')[0]}`)
  } finally { await context.close() }
 }
}
await browser.close()
console.log(`${states} states checked.`)
if (axeRows.length) console.log(`axe:\n ${axeRows.join('\n ')}`)
if (failures.length) { console.log(`${failures.length} with findings:\n${failures.join('\n')}`); process.exitCode = 1 }
else console.log('No findings.')
