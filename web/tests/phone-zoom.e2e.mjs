// Phone page-zoom contract: on a real phone one unbreakable string (a long host, path or skill name) or a flex item that
// will not shrink makes the browser lay the page out wider than the screen and zoom the whole page out. The document then
// reports scrollWidth == innerWidth, so a plain "no horizontal overflow" check passes while everything is unreadably small.
// Only a mobile context (isMobile: the viewport meta tag is honoured) reproduces it, so every check here runs in one.
//
// For every screen, at 320, 360, 390 and 412 wide, with every <details> closed and then open:
//  - every text field is at least 16 px, so focusing it never zooms the page,
//  - the layout viewport is the device width (innerWidth == width, visualViewport.scale == 1), and
//  - every control is inside the viewport, or inside a horizontal scroller that shows there is more (a peeking item or
//    a fade mask). A control cut off by an overflow:hidden/clip box cannot be reached at all.
//  - no control runs out of the bordered box around it, and no button label is wider than its button.
// Plus: the long skill name fits its heading once the card is expanded. The fixture's names contain no hyphens on purpose;
// a browser may break a line after a hyphen, which would hide the problem.
// REET_E2E_ONLY=<regexp> limits the screens (matched against the name), REET_E2E_WIDTHS=320,390 the widths.
import { chromium, expect } from '@playwright/test'
import { homeExtra, HOST80, installApi, LONGHOST, LONGSKILL, PROJECT80, SESSION80 } from './helpers/longtext.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const only = process.env.REET_E2E_ONLY ? new RegExp(process.env.REET_E2E_ONLY, 'i') : null
const widths = (process.env.REET_E2E_WIDTHS || '320,360,390,412').split(',').map(Number)
const UA = 'Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36'

// A machine whose home holds one 120-character folder path with no break opportunities, and folders with long names in it.
const PATH120 = ('/home/dev/' + 'an_unbroken_folder_name_for_the_add_project_sheet'.repeat(3)).slice(0, 120)
const addProjectExtra = {
 '/api/v2/terminal/machines': {machines:[{agentId:'connector-long120', host:HOST80, ready:true, serverEnabled:true, capabilities:{protocolVersion:1, os:'linux', osUser:'dev', terminalEnabled:true, terminalControl:true, healthEnabled:true, readiness:'ready', launch:true, browse:true, tools:['claude']}}]},
 '/api/v2/machines/connector-long120/browse': () => ({path:PATH120, home:'/home/dev', parent:'/home/dev', repo:'root', repoTop:'', more:true, partial:false, scanned:3, omitted:0,
  entries:[{name:LONGSKILL, dir:true, repoRoot:true, hidden:false}, {name:PROJECT80, dir:true, repoRoot:false, hidden:false}]}),
}

// Two long-named projects on the long-named machine, and a start in both: one started, one refused with its sentence.
const PROJECT80B = ('Docs_site_rebuild_with_another_long_unbroken_project_name' + '_abcdefghij'.repeat(8)).slice(0, 80)
const launchRunMember = (position, project, name, state, over = {}) => ({position, host:project.host, dir:project.dir, tool:'claude', model:'', state, code:'', sentence:'', sessionId:'',
 worktreePath:'', worktreeRepo:'', worktreeBranch:'', worktreeRun:'', worktreeProject:'', sharesCheckout:false, attempt:1, projectId:project.id, name, base:'', ...over})
const launchRunExtra = (() => {
 const [first] = homeExtra['/api/v2/folder-projects'].projects
 const second = {...first, id:'fp81', name:PROJECT80B, dir:`${first.dir}_docs`, baseBranch:'', position:1}
 const run = {id:'run-long', origin:'selection', projectId:'', projectName:`${PROJECT80}, ${PROJECT80B}`, state:'partial', startedAt:Date.now(), useWorktrees:true, worktreeProject:'', members:[
  launchRunMember(0, first, PROJECT80, 'started', {sessionId:'session-long80'}),
  launchRunMember(1, second, PROJECT80B, 'failed', {code:'session_limit', sentence:'The terminal session limit has been reached. End a session and try again.'})]}
 // A machine that takes each project's own base (agentsV2), so Start is offered.
 const machines = homeExtra['/api/v2/terminal/machines'].machines.map(m => ({...m, capabilities:{...m.capabilities, agentsV2:true}}))
 return {...homeExtra, '/api/v2/terminal/machines':{machines}, '/api/v2/folder-projects':{projects:[first, second], savedWaiting:0}, '/api/v2/folder-projects/runs':run, '/api/v2/folder-projects/runs/run-long':run}
})()

// Dialogs open from the long-host machine's card, the one whose name is hardest to fit.
const click = name => async page => { const b = page.getByRole('article', {name:LONGHOST}).getByRole('button', {name, exact:true}).first(); await b.scrollIntoViewIfNeeded(); await b.click(); await page.locator('dialog[open]').first().waitFor({timeout:3000}) }
// Home with 80-character machine, session and Project names, the More list opened over it, Jump's results and the launch
// bar's Change sheet opened from Home.
const home = {path:'/', extra:homeExtra, ready:async page => { for (const name of [HOST80, SESSION80, PROJECT80]) await page.getByText(name).first().waitFor({state:'attached', timeout:5000}) }}
// The Sessions screen with the same names: on a phone it holds the project rail (PR H).
const sessionsRail = {path:'/sessions', extra:homeExtra, ready:async page => { for (const name of [SESSION80, PROJECT80]) await page.getByText(name).first().waitFor({state:'attached', timeout:5000}) }}
const SCREENS = [
 {name:'home', ...home},
 {name:'home: more list', ...home, action:async page => { await page.getByRole('navigation', {name:'Main navigation'}).getByRole('button', {name:'More'}).click(); await page.locator('dialog[open]').first().waitFor({timeout:3000}) }},
 {name:'home: jump results', ...home, action:async page => { await page.getByRole('button', {name:'Jump to a session, project or machine'}).click(); await page.getByRole('combobox', {name:'Jump to a session, project or machine'}).fill('abcdefghij'); await page.getByRole('listbox', {name:'Jump results'}).getByRole('option').first().waitFor({timeout:3000}) }},
 {name:'home: launch Change sheet', ...home, action:async page => { await page.getByRole('form', {name:'Start a session'}).getByRole('button', {name:'Change', exact:true}).click(); await page.locator('dialog[open]').first().waitFor({timeout:3000}) }},
 // The launch sheets: a slot's list opened from the Change sheet (the project list, with its Find field), and the status sheet
 // of a start in several projects with two long-named projects, one started and one refused with its sentence.
 {name:'home: launch project list', ...home, action:async page => {
  await page.getByRole('form', {name:'Start a session'}).getByRole('button', {name:'Change', exact:true}).click()
  const sheet = page.getByRole('dialog', {name:'Change the launch'})
  await sheet.getByRole('button', {name:/^Project: /}).click()
  await page.getByRole('listbox', {name:'Project'}).waitFor({timeout:3000}) }},
 {name:'home: launch run status sheet', ...home, path:'/?projects=fp80,fp81', extra:launchRunExtra, action:async page => {
  // The two projects chosen on the Sessions screen's rail arrive in the address (ids only).
  await page.getByRole('form', {name:'Start a session'}).locator('.summary-text').filter({hasText:'2 projects'}).waitFor({timeout:5000})
  await page.getByRole('form', {name:'Start a session'}).getByRole('button', {name:'Start the session'}).click()
  await page.getByRole('dialog', {name:'Start in several projects'}).getByRole('button', {name:/^Retry /}).waitFor({timeout:3000}) }},
 {name:'sessions', path:'/sessions'},
 {name:'machines', path:'/machines'},
 {name:'machines: start agent dialog', path:'/machines', action:click('Start an agent')},
 {name:'machines: settings dialog', path:'/machines', action:async page => {
  await page.getByRole('article',{name:LONGHOST}).locator('summary[aria-label^="More options"]').click()
  await page.getByRole('menuitem',{name:'Settings…',exact:true}).click()
  const d=page.locator('dialog[open]').last()
  await expect(d.getByRole('heading',{name:'Agents',exact:true})).toBeVisible()
  const claude=d.locator('[data-agent="claude"]')
  await expect(claude.locator('.model-state')).toHaveText(/^2 read .+ ago$/)
  await expect(claude.getByRole('button',{name:'Refresh Claude Code models'})).toBeEnabled()
 }},
 {name:'machines: commands dialog', path:'/machines', action:async page => { await page.getByRole('article',{name:LONGHOST}).locator('summary[aria-label^="More options"]').click(); await page.getByRole('menuitem',{name:'Commands…',exact:true}).click() }},
 {name:'machine focus page', path:`/machines?machine=${LONGHOST}`},
 {name:'sync', path:'/sync'},
 {name:'skills', path:'/skills'},
 {name:'setups', path:'/profiles'},
 {name:'lists (shared)', path:'/shared'},
 {name:'list page', path:'/lists/7'},
 // The project rail is on the Sessions screen on a phone: a card's menu, and the Edit sheet over the long-named project.
 {name:'sessions: rail select', ...sessionsRail, action:async page => {
  const rail = page.locator('aside.rail')
  const menu = rail.getByRole('button', {name:'Projects menu'}); await menu.scrollIntoViewIfNeeded(); await menu.click()
  await page.getByRole('menuitem', {name:'Select projects'}).click()
  for (const name of [PROJECT80, PROJECT80B]) await rail.getByRole('checkbox', {name:new RegExp(`^${name}`)}).click()
  await rail.getByRole('link', {name:'Start in these on Home'}).waitFor({timeout:3000}) }, extra:launchRunExtra},
 {name:'sessions: project menu', ...sessionsRail, action:async page => { const b = page.getByRole('button', {name:`More for ${PROJECT80}`}); await b.scrollIntoViewIfNeeded(); await b.click(); await page.getByRole('menu').waitFor({timeout:3000}) }},
 {name:'sessions: edit project sheet', ...sessionsRail, extra:{...homeExtra, '/api/v2/folder-projects/fp80':homeExtra['/api/v2/folder-projects'].projects[0]}, action:async page => {
  const b = page.getByRole('button', {name:`More for ${PROJECT80}`}); await b.scrollIntoViewIfNeeded(); await b.click()
  await page.getByRole('menuitem', {name:'Edit'}).click(); await page.locator('dialog[open]').first().waitFor({timeout:3000}) }},
 // The old Project runs, kept to read in Activity, with one opened.
 {name:'activity: runs from projects', path:'/activity', action:async page => { const b = page.getByRole('region', {name:'Runs from Projects'}).getByRole('button', {name:/^Site/}); await b.scrollIntoViewIfNeeded(); await b.click(); await page.locator('.run-body').waitFor({timeout:3000}) }},
 {name:'hosting', path:'/hosting'},
 {name:'spaces', path:'/spaces'},
 {name:'support', path:'/support'},
 {name:'support ticket', path:'/support/41'},
 {name:'notifications', path:'/notifications'},
 ...['profile','agents','security','sso','sessions','mcp','tokens','backups'].map(p => ({name:`account ${p}`, path:`/account/${p}`})),
 {name:'activity', path:'/activity'}, // /activity is rewritten to /account/activity in place (no reload) and opens the account's Activity section
 // The Add project sheet behind the rail's Add (on the Sessions screen on a phone), with a 120-character folder chosen and the
 // browser open on it: the folder field, the breadcrumb, the folder names and the pinned chosen folder.
 {name:'add project sheet', ...sessionsRail, extra:{...homeExtra, ...addProjectExtra}, action:async page => {
  const add = page.getByRole('button', {name:'Add a project', exact:true}).first()
  await add.scrollIntoViewIfNeeded()
  await add.click()
  const d = page.locator('dialog[open]')
  await d.getByRole('button', {name:'Use this folder'}).click()
  await d.getByRole('button', {name:'Browse'}).click()
  await d.getByRole('list', {name:/^Folders in /}).getByRole('button').first().waitFor({timeout:3000})
 }},
 ...['overview','environments','users','team','support','email','notifications','backups','settings','activity'].map(p => ({name:`admin ${p}`, path:`/admin/${p}`})),
]

// Runs in the page. Returns the problems found, as readable strings.
function probe(width) {
 const problems = []
 const vw = innerWidth
 const sel = el => el.tagName.toLowerCase() + (el.id ? '#' + el.id : '') + [...el.classList].filter(c => !c.startsWith('svelte-')).slice(0, 2).map(c => '.' + c).join('')
 // The deepest elements that end past the screen's edge: where the widening comes from.
 const widest = () => [...document.querySelectorAll('body *')].filter(e => e.getBoundingClientRect().right > width + 1 && ![...e.children].some(c => c.getBoundingClientRect().right > width + 1)).slice(0, 3).map(e => `${sel(e)} ${Math.round(e.getBoundingClientRect().right)}px`).join(', ')
 if (vw !== width) problems.push(`page laid out ${vw}px wide on a ${width}px screen (zoomed to ${(width / vw).toFixed(2)}); widest: ${widest()}`)
 if (visualViewport && Math.abs(visualViewport.scale - 1) > 0.01) problems.push(`visual viewport scale ${visualViewport.scale.toFixed(2)}`)
 const label = el => (el.getAttribute('aria-label') || el.innerText || el.value || el.title || '').trim().replace(/\s+/g, ' ').slice(0, 40)
 // Inside a closed <details>, anywhere but its own summary: not shown, whatever its box says.
 const folded = el => { for (let n = el.parentElement; n; n = n.parentElement) if (n.tagName === 'DETAILS' && !n.open && !(el.closest('summary')?.parentElement === n)) return true; return false }
 const visible = el => { const s = getComputedStyle(el); if (s.display === 'none' || s.visibility === 'hidden') return false; const r = el.getBoundingClientRect(); return r.width > 2 && r.height > 2 }
 const dialog = document.querySelector('dialog[open]')
 const root = dialog || document.body
 // A scroller shows there is more when an item straddles its edge (peeks) or the edge fades out under a mask.
 const cued = sc => { const s = getComputedStyle(sc); if ((s.maskImage || s.webkitMaskImage || 'none') !== 'none') return true; const r = sc.getBoundingClientRect(); return [...sc.children].some(c => { const q = c.getBoundingClientRect(); return (q.left < r.right - 16 && q.right > r.right + 1) || (q.right > r.left + 16 && q.left < r.left - 1) }) }
 // A field under 16 px makes a phone zoom in when it is focused.
 for (const field of root.querySelectorAll('input:not([type=checkbox]):not([type=radio]):not([type=hidden]):not([type=range]):not([type=color]):not([type=file]),select,textarea'))
  if (!field.closest('.vh,.xterm') && !folded(field) && visible(field) && parseFloat(getComputedStyle(field).fontSize) < 16) problems.push(`"${label(field) || field.name || field.type}" (${sel(field)}) is a ${getComputedStyle(field).fontSize} field: focusing it zooms the page`)
 const controls = [...root.querySelectorAll('a[href],button,input:not([type=hidden]),select,textarea,summary,[role=button],[role=tab],[tabindex="0"]')]
  .filter(el => !el.closest('.vh,.xterm') && !folded(el) && visible(el))
 for (const el of controls) {
  const r = el.getBoundingClientRect()
  let handled = false
  for (let n = el.parentElement; n && n !== document.documentElement && n !== document.body; n = n.parentElement) {
   const ox = getComputedStyle(n).overflowX
   if (!/(hidden|clip|auto|scroll)/.test(ox)) continue
   const nr = n.getBoundingClientRect()
   if (r.right <= nr.right + 2 && r.left >= nr.left - 2) continue
   if (/(auto|scroll)/.test(ox) && n.scrollWidth > n.clientWidth + 1) { if (!cued(n)) problems.push(`"${label(el)}" (${sel(el)}) is hidden in ${sel(n)}, which scrolls sideways with no sign there is more`); handled = true; break }
   problems.push(`"${label(el)}" (${sel(el)}) is cut off by ${sel(n)} (overflow-x: ${ox}) at ${Math.round(r.left)}..${Math.round(r.right)} of ${Math.round(nr.left)}..${Math.round(nr.right)}`); handled = true; break
  }
  // A control that runs out of the box drawn around it (a card, a panel, a command box) looks broken even when nothing clips it.
  if (!handled) for (let n = el.parentElement; n && n !== document.body; n = n.parentElement) {
   const cs = getComputedStyle(n)
   const boxed = parseFloat(cs.borderLeftWidth) > 0 && parseFloat(cs.borderRightWidth) > 0 && cs.borderLeftStyle !== 'none'
   if (!boxed) continue
   const nr = n.getBoundingClientRect()
   if (r.right > nr.right + 2 || r.left < nr.left - 2) { problems.push(`"${label(el)}" (${sel(el)}) runs out of ${sel(n)} at ${Math.round(r.left)}..${Math.round(r.right)} of ${Math.round(nr.left)}..${Math.round(nr.right)}`); handled = true }
   break
  }
  // A label wider than its own button runs out of it (and over whatever is next to it). 8 px of slack allows for a turned chevron.
  if (!handled && el.matches('button,a.btn,summary,[role=button]') && getComputedStyle(el).overflowX === 'visible' && el.scrollWidth > el.clientWidth + 8) problems.push(`"${label(el)}" (${sel(el)}) has a label ${el.scrollWidth - el.clientWidth}px wider than the control`)
  if (!handled && (r.right > vw + 1 || r.left < -1)) problems.push(`"${label(el)}" (${sel(el)}) sits off the page at ${Math.round(r.left)}..${Math.round(r.right)}`)
 }
 return problems
}

async function phone(width) {
 // This screen's own API answers, ahead of the shared ones; each width runs in parallel with its own.
 const extra = {}
 const context = await browser.newContext({viewport:{width,height:800}, deviceScaleFactor:2, isMobile:true, hasTouch:true, userAgent:UA})
 const page = await context.newPage()
 const errors = []
 page.on('pageerror', e => errors.push(e.message))
 const unmocked = await installApi(page, {extra})
 // Settled: no API answer outstanding for 120 ms; much quicker than the 500 ms of 'networkidle' on every screen. Every answer is
 // mocked and instant, so a request open for over a second was cancelled by the page (some never report that) and is ignored.
 const open = new Map()
 let last = Date.now()
 const api = r => r.url().includes('/api/')
 page.on('request', r => { if (api(r)) { open.set(r, Date.now()); last = Date.now() } })
 for (const ev of ['requestfinished', 'requestfailed']) page.on(ev, r => { if (open.delete(r)) last = Date.now() })
 const busy = () => [...open.values()].some(t => Date.now() - t < 1000)
 const settled = async () => { const until = Date.now() + 5000; while (Date.now() < until && (busy() || Date.now() - last < 120)) await new Promise(r => setTimeout(r, 20)) }
 const visit = async path => { open.clear(); await page.goto(/^https?:/.test(path) ? path : base + path); await page.locator('main').first().waitFor(); await settled() }
 return {context, page, errors, unmocked, settled, visit, extra}
}

async function screen(p, width, s) {
 const found = []
 for (const k of Object.keys(p.extra)) delete p.extra[k]
 Object.assign(p.extra, s.extra ?? {})
 await p.visit(s.path)
 if (s.ready) await s.ready(p.page)
 if (s.action) { await s.action(p.page); await p.settled() }
 for (const state of ['closed', 'open']) {
  if (state === 'open') {
   const opened = await p.page.evaluate(() => { let n = 0; for (const d of document.querySelectorAll('main details:not([open]), dialog[open] details:not([open])')) { d.open = true; n++ } return n })
   if (!opened) continue
   await p.page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))))
   await p.settled()
  }
  for (const problem of await p.page.evaluate(probe, width)) found.push(`${width}px ${s.name} (details ${state}): ${problem}`)
 }
 return found
}

// A long skill name is clamped to two lines while its card is collapsed; expanded, all of it reads on the screen.
async function expandedName(p, width) {
 await p.visit('/skills')
 await p.page.getByRole('button', {name:`Expand ${LONGSKILL}`}).click()
 const h = p.page.locator('h3', {hasText:LONGSKILL}).first()
 await h.scrollIntoViewIfNeeded()
 const fit = await h.evaluate(el => { const r = el.getBoundingClientRect(); return {w:el.scrollWidth - el.clientWidth, h:el.scrollHeight - el.clientHeight, left:r.left, right:r.right, vw:innerWidth} })
 return fit.w > 1 || fit.h > 1 || fit.left < 0 || fit.right > fit.vw ? [`${width}px skills: the expanded long skill name does not fit its heading (${JSON.stringify(fit)})`] : []
}

async function check(width) {
 const p = await phone(width)
 const failures = []
 try {
  for (const s of SCREENS) if (!only || only.test(s.name)) failures.push(...await screen(p, width, s))
  if (!only || only.test('skills')) failures.push(...await expandedName(p, width))
 } finally { await p.context.close() }
 if (p.errors.length) failures.push(`${width}px page errors: ${p.errors.join(' | ')}`)
 if (p.unmocked.length) failures.push(`${width}px unmocked API calls: ${[...new Set(p.unmocked)].join(', ')}`)
 return failures
}

try {
 const started = Date.now()
 const failures = [...new Set((await Promise.all(widths.map(check))).flat())]
 if (failures.length) throw new Error(`${failures.length} phone zoom problems:\n${failures.join('\n')}`)
 console.log(`Phone zoom: ${only ? 'selected' : SCREENS.length} screens at ${widths.join(', ')}px keep the device width with every control reachable (${((Date.now() - started) / 1000).toFixed(1)}s).`)
} finally { await browser.close() }
