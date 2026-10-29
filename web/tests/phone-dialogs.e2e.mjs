// Phone dialog contract: every modal dialog in the app, on phones held upright and sideways, with and without the
// on-screen keyboard. The dialogs share one shell (web/src/lib/dialogShell.ts and web/src/dialog-shell.css); this checks
// what the shell promises, through each dialog's own way in.
//
// Each dialog is opened at 320 x 568, 360 x 640, 390 x 844 and 412 x 915 upright and 844 x 390 and 915 x 412 sideways,
// in a mobile context (isMobile, hasTouch), with reduced motion. Then:
//  - at rest: the page behind does not move while the dialog is scrolled by wheel and by touch, over its scrolling part
//    and over its header; the root has overflow hidden; every scrolling part has overscroll-behavior contain (on it or
//    on a scrolling part around it inside the dialog); the dialog fits the screen, and on an
//    upright phone (under 560 px wide) it is a sheet: the full screen for a form, the bottom edge for a confirmation;
//    a Close control is at least 44 x 44 px, in view and not covered; the main action can be scrolled to, is in view and
//    is not covered; sideways, the part that scrolls is at least 140 px tall and at least 40% of the screen, and the
//    footer's buttons share one row when their own widths fit in it.
//  - with the keyboard open, for a dialog with a text field: the field is focused, and the keyboard is simulated twice,
//    by shrinking the page to 55% of its height (a browser that resizes the page, as the terminal asks for) and by
//    shrinking only the visual viewport (Safari, and Chrome on Android by default). The dialog stays inside the part
//    above the keyboard; the field is wholly in view, inside every box that clips it and not under a pinned footer or
//    header; the main action is in view and not covered; the footer's buttons share one row when they fit. Nothing
//    scrolls the field into view but the dialog itself.
// REET_E2E_ONLY=<regexp> limits the dialogs, REET_E2E_VIEWPORTS=320x568,844x390 the screens. REET_DIALOG_SHOTS=<dir> saves a
// screenshot of each state there, and REET_DIALOG_LOG=1 prints what was measured.
import { chromium, expect } from '@playwright/test'
import { installApi, LONGHOST, sessions } from './helpers/longtext.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const only = process.env.REET_E2E_ONLY ? new RegExp(process.env.REET_E2E_ONLY, 'i') : null
const VIEWPORTS = (process.env.REET_E2E_VIEWPORTS || '320x568,360x640,390x844,412x915,844x390,915x412').split(',').map(v => v.split('x').map(Number))
const shots = process.env.REET_DIALOG_SHOTS, log = !!process.env.REET_DIALOG_LOG
const UA = 'Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36'

const card = (page, name) => page.getByRole('article', {name:LONGHOST}).getByRole('button', {name, exact:true}).first()
const press = (name, options = {}) => async page => { const b = page.getByRole('button', {name, ...options}).first(); await b.scrollIntoViewIfNeeded(); await b.click() }
// A terminal page with a live pane, and a button in its toolbar (which may scroll sideways on a phone).
const live = `/sessions?agent=${sessions[0].agentId}&session=${sessions[0].id}`
const tool = name => async page => { await page.locator('.pane[data-connection-state=live]').first().waitFor({timeout:5000}); const b = page.locator('.pane').first().getByRole('button', {name, exact:true}).first(); await b.scrollIntoViewIfNeeded(); await b.click() }
// A choice in a rail card's menu.
const menu = (project, item) => async page => { await press(`More for ${project}`, {exact:true})(page); await page.getByRole('menuitem', {name:item, exact:true}).click() }
const cardMenu = item => async page => { await page.getByRole('article',{name:LONGHOST}).locator('summary[aria-label^="More options"]').click(); await page.getByRole('menuitem',{name:item,exact:true}).click() }
const TEXT = 'input:not([type=checkbox]):not([type=radio]):not([type=hidden])'
const ended = {'/api/v2/terminal/sessions': {sessions:sessions.map(s => ({...s, state:'ended'}))}}
// field: the text field typed into with the keyboard open. primary: the main action, when it is not the dialog's
// button.primary or submit button. sheet: how it sits on an upright phone.
const DIALOGS = [
 {name:'start agent', path:'/machines', open:p => card(p, 'Start an agent').click(), field:'textarea', sheet:'full'},
 {name:'add machine', path:'/machines#add-machine', field:TEXT, sheet:'full'},
 {name:'private skills', path:'/machines', open:p => p.locator('[data-private-skills]:not([disabled])').first().evaluate(el => el.click()), sheet:'full'},
 {name:'skill imports', path:'/machines', open:press('Import a machine skill…'), sheet:'full'},
 {name:'machine commands', path:'/machines', open:cardMenu('Commands…'), primary:'Done', sheet:'full'},
 {name:'machine settings', path:'/machines', open:cardMenu('Settings…'), primary:'Forget machine…', sheet:'full', check:async page => {
  const d=page.locator('dialog[open]').last()
  await expect(d.getByRole('heading',{name:'Agents',exact:true})).toBeVisible()
  const claude=d.locator('[data-agent="claude"]')
  await expect(claude.locator('.model-state')).toHaveText(/^2 read .+ ago$/)
  await expect(claude.getByRole('button',{name:'Refresh Claude Code models'})).toBeEnabled()
  await expect(d).not.toContainText(/not connected for terminals|is offline/)
 }},
 {name:'confirmation', path:'/machines', open:async p => { await cardMenu('Settings…')(p); await p.getByRole('button', {name:'Forget machine…'}).click() }, sheet:'bottom'},
 // Folder projects in the project rail, on the Sessions screen on a phone (PR H): Add, a card's Edit, Worktrees and Remove from reet.
 {name:'add project', path:'/sessions', open:press('Add a project', {exact:true}), field:TEXT, sheet:'full'},
 {name:'edit project', path:'/sessions', open:menu('site', 'Edit'), field:TEXT, sheet:'full'},
 {name:'project worktrees', path:'/sessions', open:menu('site', 'Worktrees'), sheet:'full'},
 {name:'remove project', path:'/sessions', open:menu('site', 'Remove from reet'), sheet:'bottom'},
 // An old Project run's worktree, from Runs from Projects in Activity.
 {name:'remove worktree', path:'/activity', extra:ended, open:async p => { await press(/^Site/)(p); await press(/^Remove worktree/)(p) }, sheet:'full'},
 {name:'new list', path:'/shared', open:press('Create a list'), field:TEXT, sheet:'full'},
 // A field low in the form (Selected people), so the browser's own scroll leaves it on the bottom edge, under the pinned
 // (sticky) footer.
 {name:'new list (people)', path:'/shared', open:async p => { await press('Create a list')(p); await p.locator('dialog[open] input[name=audience][value=private]').check() }, field:'textarea', sheet:'full'},
 {name:'manage tags', path:'/skills', open:async p => { await p.locator('button[aria-controls="tools-panel"]:visible').first().click(); await press('Manage tags')(p); await p.locator('dialog[open]').getByRole('button', {name:'Rename'}).last().click() }, field:TEXT, sheet:'full'},
 {name:'terminal workspace', path:'/sessions?view=terminal', open:press('New session', {exact:true}), field:'input[type=search]', sheet:'full'},
 // The terminal's drawers (placement="drawer"), opened from a live pane's toolbar. last: the dialog's last control must be
 // reachable, so nothing below the screen's edge is lost.
 {name:'appearance drawer', path:live, open:tool('Appearance'), last:true, sheet:'full'},
 {name:'history drawer', path:live, open:tool('Recent history'), last:true, sheet:'full'},
 {name:'commands drawer', path:live, open:tool('Saved commands'), field:'input[type=search]', last:true, sheet:'full'},
 {name:'command editor drawer', path:live, open:async p => { await tool('Saved commands')(p); await p.locator('dialog[open]').getByRole('button', {name:/New command/}).click() }, field:'textarea', last:true, sheet:'full'},
 {name:'select text', path:live, open:tool('Select text'), last:true, sheet:'bottom'},
]

// Runs in the page: the top dialog's geometry against the part of the screen that can be seen.
function measure({field, primary, last}) {
 const vv = visualViewport, vw = innerWidth
 const band = {top:vv.offsetTop, bottom:vv.offsetTop + vv.height}
 const dialogs = [...document.querySelectorAll('dialog[open]')], d = dialogs[dialogs.length - 1]
 if (!d) return {missing:true}
 const box = el => { const r = el.getBoundingClientRect(); return {top:r.top, bottom:r.bottom, left:r.left, right:r.right, width:r.width, height:r.height} }
 const round = r => r && Object.fromEntries(Object.entries(r).map(([k, v]) => [k, typeof v === 'number' ? Math.round(v) : v]))
 // The part of el that can be seen: inside the visual viewport and every box that clips it.
 const seen = el => {
  const r = box(el)
  let top = Math.max(r.top, band.top), bottom = Math.min(r.bottom, band.bottom), left = Math.max(r.left, 0), right = Math.min(r.right, vw)
  for (let n = el.parentElement; n && n !== document.documentElement; n = n.parentElement) {
   const s = getComputedStyle(n), modal = n.matches('dialog:modal')
   if (s.overflowX === 'visible' && s.overflowY === 'visible') { if (modal) break; continue }
   const q = n.getBoundingClientRect()
   top = Math.max(top, q.top + n.clientTop); bottom = Math.min(bottom, q.top + n.clientTop + n.clientHeight)
   left = Math.max(left, q.left + n.clientLeft); right = Math.min(right, q.left + n.clientLeft + n.clientWidth)
   // A modal dialog sits in the top layer: nothing around it in the document clips it.
   if (modal) break
  }
  return {top, bottom, left, right, whole:top <= r.top + 1 && bottom >= r.bottom - 1 && left <= r.left + 1 && right >= r.right - 1}
 }
 const hits = (el, points) => points.every(([x, y]) => { const t = document.elementFromPoint(x, y); return !!t && (t === el || el.contains(t)) })
 const covered = (el, points) => points.map(([x, y]) => document.elementFromPoint(x, y)).filter(t => t && t !== el && !el.contains(t)).map(t => t.tagName.toLowerCase() + [...t.classList].filter(c => !c.startsWith('svelte-')).map(c => '.' + c).join(''))[0] || ''
 const shown = el => { const s = getComputedStyle(el), r = el.getBoundingClientRect(); return s.display !== 'none' && s.visibility !== 'hidden' && r.width > 0 && r.height > 0 }
 // The page itself is held: in Chromium the dialog's overscroll-behavior already stops a scroll reaching it, so the lock
 // (for Safari, and for anything outside a scrolling part) is read from its style.
 const out = {vw, vh:innerHeight, band:round(band), dialog:round(box(d)), sheet:d.dataset.shell || '', pageOverflow:getComputedStyle(document.documentElement).overflowY}
 // Close: the control named Close (Close ×, Close settings, and so on).
 const close = [...d.querySelectorAll('button')].find(b => shown(b) && /^close\b/i.test((b.getAttribute('aria-label') || b.textContent || '').trim()))
 if (close) { const r = box(close), s = seen(close); out.close = {...round(r), whole:s.whole, hit:hits(close, [[r.left + r.width / 2, r.top + r.height / 2]])} }
 // Scrolling parts: overscroll-behavior, and the room left for the content in the main one.
 const scrollers = [d, ...d.querySelectorAll('*')].filter(n => /(auto|scroll)/.test(getComputedStyle(n).overflowY) && n.scrollHeight > n.clientHeight + 1)
 // A scroll that reaches the end of a scrolling part is handed on to the next scroll container out, up to the page, unless
 // one on the way (the part itself, or one around it inside the dialog) has overscroll-behavior contain or none.
 const held = n => { for (let a = n; a; a = a.parentElement) { const s = getComputedStyle(a); if (/(auto|scroll|hidden)/.test(s.overflowY) && /(contain|none)/.test(s.overscrollBehaviorY)) return true; if (a === d) break } return false }
 out.chaining = scrollers.filter(n => !held(n)).map(n => n.tagName.toLowerCase() + [...n.classList].filter(c => !c.startsWith('svelte-')).map(c => '.' + c).join(''))
 const main = scrollers.sort((a, b) => b.clientHeight - a.clientHeight)[0]
 if (main) {
  const s = seen(main), r = main.getBoundingClientRect()
  let room = Math.max(0, Math.min(s.bottom, r.top + main.clientTop + main.clientHeight) - Math.max(s.top, r.top))
  for (const n of main.querySelectorAll('*')) if (getComputedStyle(n).position === 'sticky' && n.offsetParent) room -= n.getBoundingClientRect().height
  out.content = Math.round(room)
 }
 // The footer with the main action. On a short screen its buttons share one row whenever their own widths fit in it.
 const foot = [...d.querySelectorAll('[data-shell-foot]')].find(shown)
 if (foot) {
  out.foot = Math.round(foot.getBoundingClientRect().height)
  const bs = [...foot.querySelectorAll('button, a.btn')].filter(shown).map(b => ({b, r:b.getBoundingClientRect()})).sort((x, y) => x.r.top - y.r.top)
  let rows = 0, bottom = -Infinity
  for (const {r} of bs) if (r.top >= bottom - 1) { rows++; bottom = r.bottom } else bottom = Math.max(bottom, r.bottom)
  // A button's own width: its label, padding and border, whatever width a rule stretches it to.
  const own = b => { const range = document.createRange(); range.selectNodeContents(b); const s = getComputedStyle(b); return range.getBoundingClientRect().width + parseFloat(s.paddingLeft) + parseFloat(s.paddingRight) + parseFloat(s.borderLeftWidth) + parseFloat(s.borderRightWidth) }
  const fs = getComputedStyle(foot), room = foot.clientWidth - parseFloat(fs.paddingLeft) - parseFloat(fs.paddingRight)
  out.footRows = rows
  out.footFits = bs.reduce((sum, {b}) => sum + own(b), 0) + 12 * Math.max(0, bs.length - 1) <= room
 }
 if (field) {
  const f = document.activeElement
  if (!f || !d.contains(f) || !f.matches(field)) out.field = {lost:true, active:f?.tagName}
  else {
   const r = box(f), s = seen(f), pts = [[r.left + 6, r.top + 4], [r.left + r.width / 2, r.top + r.height / 2], [r.right - 6, r.bottom - 4]]
   out.field = {...round(r), whole:s.whole, hit:hits(f, pts), coveredBy:covered(f, pts)}
  }
 }
 // Reachable: scrolled into view inside the dialog's own scrolling parts, as a person would, then in view and on top.
 const reach = el => {
  for (let n = el.parentElement; n; n = n.parentElement) {
   if (/(auto|scroll)/.test(getComputedStyle(n).overflowY) && n.scrollHeight > n.clientHeight + 1) {
    const q = n.getBoundingClientRect(), r = el.getBoundingClientRect()
    const top = Math.max(q.top + n.clientTop, band.top), bottom = Math.min(q.top + n.clientTop + n.clientHeight, band.bottom)
    if (r.bottom > bottom) n.scrollTop += r.bottom - bottom + 4; else if (r.top < top) n.scrollTop -= top - r.top + 4
   }
   if (n === d) break
  }
  const r = box(el), s = seen(el), pts = [[r.left + r.width / 2, r.top + r.height / 2]]
  return {text:(el.getAttribute('aria-label') || el.textContent || '').trim().slice(0, 30), ...round(r), whole:s.whole, hit:hits(el, pts), coveredBy:covered(el, pts)}
 }
 const buttons = [...d.querySelectorAll('button, a.btn')].filter(shown)
 const main2 = primary ? buttons.find(b => (b.getAttribute('aria-label') || b.textContent || '').trim() === primary) : buttons.filter(b => b.matches('.primary, [type=submit]')).pop()
 if (main2) out.primary = reach(main2)
 // The last control in the dialog: everything above it can be scrolled to as well.
 if (last) {
  const controls = [...d.querySelectorAll('button, input:not([type=hidden]), select, textarea, a[href], summary')].filter(el => shown(el) && !el.closest('[data-shell-head]'))
  const end = controls[controls.length - 1]
  if (end) out.last = reach(end)
 }
 return out
}

const problems = (m, {width, height, state, spec}) => {
 const out = []
 const say = text => out.push(`${width}x${height} ${spec.name} (${state}): ${text}`)
 if (m.missing) { say('no dialog is open'); return out }
 const {dialog:d, band} = m
 if (d.left < -1 || d.right > m.vw + 1 || d.top < band.top - 1 || d.bottom > band.bottom + 1) say(`the dialog (${d.top}..${d.bottom}, ${d.left}..${d.right}) does not fit the visible part of the screen (${band.top}..${band.bottom}, 0..${m.vw})`)
 if (width < 560) {
  if (d.left > 1 || d.right < m.vw - 1) say(`not a sheet: the dialog is ${d.width}px wide on a ${m.vw}px screen`)
  if (spec.sheet === 'full' && (d.top > band.top + 1 || d.bottom < band.bottom - 1)) say(`not a full-screen sheet: ${d.top}..${d.bottom} of ${band.top}..${band.bottom}`)
  if (spec.sheet === 'bottom' && Math.abs(d.bottom - band.bottom) > 1) say(`not on the bottom edge: it ends at ${d.bottom} of ${band.bottom}`)
 }
 if (m.pageOverflow !== 'hidden') say(`the page behind can still scroll (overflow ${m.pageOverflow} on the root)`)
 if (m.chaining.length) say(`scrolling hands on to the page from ${m.chaining.join(', ')} (overscroll-behavior)`)
 if (m.close) {
  if (m.close.width < 44 - 0.5 || m.close.height < 44 - 0.5) say(`the Close control is ${m.close.width} x ${m.close.height}px, under 44 x 44`)
  if (state === 'rest' && (!m.close.whole || !m.close.hit)) say(`the Close control is out of view or covered (${JSON.stringify(m.close)})`)
 }
 if (m.primary && (!m.primary.whole || !m.primary.hit)) say(`the main action "${m.primary.text}" is out of reach (${JSON.stringify(m.primary)})`)
 if (spec.primary && !m.primary) say(`the main action "${spec.primary}" is missing`)
 if (spec.last && !m.last) say('it has no controls')
 if (m.last && (!m.last.whole || !m.last.hit)) say(`its last control "${m.last.text}" is out of reach (${JSON.stringify(m.last)})`)
 if (state === 'rest' && height <= 500 && m.content !== undefined) {
  const need = Math.max(140, Math.round(0.4 * (band.bottom - band.top)))
  if (m.content < need) say(`the header and footer leave ${m.content}px for the content, under ${need}px`)
 }
 if ((state !== 'rest' || height <= 500) && m.footFits && m.footRows > 1) say(`the footer's buttons take ${m.footRows} rows on a short screen, though they fit in one`)
 if (state !== 'rest') {
  if (!m.field || m.field.lost) say(`the field lost focus (${JSON.stringify(m.field)})`)
  else if (!m.field.whole || !m.field.hit) say(`the focused field is hidden (${JSON.stringify({...m.field, band})})`)
 }
 return out
}

const frames = page => page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(() => setTimeout(r, 60)))))

async function phone([width, height]) {
 const context = await browser.newContext({viewport:{width, height}, deviceScaleFactor:2, isMobile:true, hasTouch:true, userAgent:UA, reducedMotion:'reduce'})
 const page = await context.newPage()
 const errors = []
 page.on('pageerror', e => errors.push(e.message))
 const extra = {}
 const unmocked = await installApi(page, {extra})
 const cdp = await context.newCDPSession(page)
 const open = new Map()
 let last = Date.now()
 const api = r => r.url().includes('/api/')
 page.on('request', r => { if (api(r)) { open.set(r, Date.now()); last = Date.now() } })
 for (const ev of ['requestfinished', 'requestfailed']) page.on(ev, r => { if (open.delete(r)) last = Date.now() })
 const busy = () => [...open.values()].some(t => Date.now() - t < 1000)
 const settled = async () => { const until = Date.now() + 5000; while (Date.now() < until && (busy() || Date.now() - last < 120)) await new Promise(r => setTimeout(r, 20)) }
 return {width, height, context, page, errors, unmocked, extra, cdp, settled}
}

// The page behind must not move while the dialog is scrolled: by wheel and by a finger, over its scrolling part and
// over its header. The page is made long first, so there is always something that could scroll.
async function scrollLock(p, spec) {
 const {page, cdp} = p
 const points = await page.evaluate(() => {
  const ds = document.querySelectorAll('dialog[open]'), d = ds[ds.length - 1], r = d.getBoundingClientRect()
  const scrollers = [...d.querySelectorAll('*')].filter(n => /(auto|scroll)/.test(getComputedStyle(n).overflowY))
  const body = scrollers.sort((a, b) => b.clientHeight - a.clientHeight)[0]
  const b = (body || d).getBoundingClientRect()
  const clamp = (x, y) => [Math.round(Math.min(Math.max(x, 2), innerWidth - 2)), Math.round(Math.min(Math.max(y, 2), visualViewport.height - 2))]
  return [clamp(b.left + b.width / 2, b.top + Math.min(b.height / 2, 120)), clamp(r.left + 12, r.top + 10)]
 })
 const before = await page.evaluate(() => scrollY)
 for (const [x, y] of points) {
  await page.mouse.move(x, y)
  for (let i = 0; i < 4; i++) await page.mouse.wheel(0, 1200)
  await cdp.send('Input.synthesizeScrollGesture', {x, y, yDistance:-1200, gestureSourceType:'touch', speed:6000})
 }
 await frames(page)
 const after = await page.evaluate(() => scrollY)
 return after === before ? [] : [`${p.width}x${p.height} ${spec.name} (rest): the page behind moved from ${before} to ${after} while the dialog was scrolled`]
}

async function record(p, spec, state, m) {
 if (log) console.log(`${p.width}x${p.height} ${spec.name} (${state}): dialog ${m.dialog?.top}..${m.dialog?.bottom} of ${m.band?.top}..${m.band?.bottom}, content ${m.content ?? '-'}px, close ${m.close ? `${m.close.width}x${m.close.height}` : '-'}, foot ${m.foot ?? '-'}px, field ${m.field ? `${m.field.top}..${m.field.bottom}` : '-'}, main ${m.primary ? `${m.primary.top}..${m.primary.bottom}` : '-'}`)
 if (shots) await p.page.screenshot({path:`${shots}/${spec.name.replace(/\W+/g, '-')}-${p.width}x${p.height}-${state}.png`})
}

async function keyboard(p, spec, mode) {
 const {page, width, height} = p
 const field = page.locator('dialog[open]').last().locator(`${spec.field}:visible`).first()
 try { await field.focus({timeout:3000}) } catch { return [`${width}x${height} ${spec.name} (keyboard, ${mode}): no ${spec.field} to type into`] }
 await frames(page)
 if (mode === 'layout') await page.setViewportSize({width, height:Math.round(height * 0.55)})
 else await page.evaluate(() => {
  const vv = visualViewport, h = Math.round(innerHeight * 0.55)
  Object.defineProperty(vv, 'height', {configurable:true, get:() => h})
  vv.dispatchEvent(new Event('resize'))
 })
 await frames(page)
 const m = await page.evaluate(measure, {field:spec.field, primary:spec.primary, last:spec.last})
 await record(p, spec, `keyboard-${mode}`, m)
 const found = problems(m, {width, height, state:`keyboard, ${mode}`, spec})
 if (mode === 'layout') await page.setViewportSize({width, height})
 else await page.evaluate(() => { delete visualViewport.height; visualViewport.dispatchEvent(new Event('resize')) })
 await frames(page)
 return found
}

async function check(viewport) {
 const p = await phone(viewport)
 const failures = []
 try {
  for (const spec of DIALOGS) {
   if (only && !only.test(spec.name)) continue
   for (const k of Object.keys(p.extra)) delete p.extra[k]
   Object.assign(p.extra, spec.extra || {})
   // A fresh document each time: a path that differs only in its #hash would otherwise keep the last dialog open.
   await p.page.goto('about:blank')
   await p.page.goto(base + spec.path)
   await p.page.locator('main').first().waitFor()
   await p.settled()
   await p.page.evaluate(() => { document.body.style.paddingBottom = '3000px' })
   try {
    if (spec.open) await spec.open(p.page)
    await p.page.locator('dialog[open]').last().waitFor({timeout:3000})
   } catch (e) { failures.push(`${p.width}x${p.height} ${spec.name}: could not open it (${e.message.split('\n')[0]})`); continue }
   await p.settled()
   await frames(p.page)
   if (spec.check) { try { await spec.check(p.page) } catch (e) { failures.push(`${p.width}x${p.height} ${spec.name}: ${e.message.split('\n')[0]}`) } }
   const rest = await p.page.evaluate(measure, {primary:spec.primary, last:spec.last})
   await record(p, spec, 'rest', rest)
   failures.push(...problems(rest, {width:p.width, height:p.height, state:'rest', spec}))
   failures.push(...await scrollLock(p, spec))
   if (spec.field) for (const mode of ['layout', 'visual']) failures.push(...await keyboard(p, spec, mode))
  }
 } finally { await p.context.close() }
 if (p.errors.length) failures.push(`${p.width}x${p.height} page errors: ${p.errors.join(' | ')}`)
 if (p.unmocked.length) failures.push(`${p.width}x${p.height} unmocked API calls: ${[...new Set(p.unmocked)].join(', ')}`)
 return failures
}

try {
 const started = Date.now()
 const failures = [...new Set((await Promise.all(VIEWPORTS.map(check))).flat())]
 if (failures.length) throw new Error(`${failures.length} phone dialog problems:\n${failures.join('\n')}`)
 console.log(`Phone dialogs: ${only ? 'selected' : DIALOGS.length} dialogs at ${VIEWPORTS.map(v => v.join('x')).join(', ')} hold the page still, fit the screen and keep the focused field and main action in view (${((Date.now() - started) / 1000).toFixed(1)}s).`)
} finally { await browser.close() }
