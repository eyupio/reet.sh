// The Skills page prunes in place: Archive, Delete and Restore never switch the tab, scroll the page or open another one.
// Focus moves to the next row's button, a toast names the skill and the machines with Undo, Delete asks only for a skill
// you wrote or one reet has no copy of, bulk Archive is one request, and every row works on a phone in light and dark.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skillRow, skillsPageOf } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const off = {state:'off',pending:0,total:3,hosts:[],machines:[]}
// Six Active skills (one you wrote, one that failed on ThinkPad) and three Archived, two of them from the upgrade.
function world() {
 return {
  rows: [
   skillRow('reet-babysit', {originKind:'written', origin:'', yours:true, description:'Watch a pull request until it is ready to merge.'}),
   skillRow('seo-bing', {description:'Bing Webmaster Tools checks and URL submissions.', tags:[{id:1,name:'seo'}]}),
   skillRow('seo-cluster', {description:'Group keywords into topic clusters mapped to pages.', health:{state:'failed',pending:1,total:3,hosts:['ThinkPad'],reason:'no write access',machines:[{host:'Elitebook',state:'ok'},{host:'ThinkPad',state:'failed',detail:'no write access'},{host:'studio',state:'ok'}]}}),
   skillRow('svg-animations', {liked:true}),
   skillRow('ui-ux-pro-max'),
   skillRow('writing-plans', {health:{state:'installing',pending:1,total:3,hosts:['studio'],machines:[]}}),
   skillRow('improve-animations', {status:'archived', originKind:'kept', origin:'Elitebook', health:off}),
   skillRow('microsoft-foundry', {status:'archived', upgradedFrom:'removed', health:off}),
   skillRow('seo-sxo', {status:'archived', upgradedFrom:'parked', health:off}),
  ],
  deleted: [], posts: [],
  // Skills reet holds no copy of, though the page last read that it did: the server refuses their Delete first.
  noCopy: new Set(['item-ui-ux-pro-max']),
  notice: {parked:1, removed:1},
 }
}

function routes(w) {
 const page = status => skillsPageOf(w.rows, status, {notice:w.notice})
 return {
  '/api/v2/skills': req => page(new URL(req.url()).searchParams.get('status') || 'active'),
  '/api/v2/skills/status': req => {
   const b = req.postDataJSON(); w.posts.push(['status', b])
   w.rows = w.rows.map(r => b.ids.includes(r.id) ? {...r, status:b.status} : r)
   return page(b.tab || b.status)
  },
  '/api/v2/skills/delete': req => {
   const b = req.postDataJSON(); w.posts.push(['delete', b])
   const chosen = w.rows.filter(r => b.ids.includes(r.id))
   const written = chosen.filter(r => r.yours).map(r => r.name)
   if (written.length && !b.writtenConfirmed) return {status:409, json:{code:'written', names:written, error:`You wrote ${written.join(', ')}.`}}
   const none = chosen.filter(r => !r.hasCopy || w.noCopy.has(r.id)).map(r => r.name)
   if (none.length && !b.noCopyConfirmed) return {status:409, json:{code:'no-copy', names:none, error:`reet has no copy of ${none.join(', ')}.`}}
   w.deleted.push(...chosen); w.rows = w.rows.filter(r => !b.ids.includes(r.id))
   return {deleted:chosen.length}
  },
  '/api/v2/skills/undelete': req => {
   const b = req.postDataJSON(); w.posts.push(['undelete', b])
   const back = w.deleted.filter(r => b.ids.includes(r.id))
   w.deleted = w.deleted.filter(r => !b.ids.includes(r.id)); w.rows = [...w.rows, ...back]
   return {restored:back.length}
  },
  '/api/v2/skills/retry': req => { w.posts.push(['retry', req.postDataJSON()]); return {retried:1} },
  '/api/v2/skills/notice': req => {
   const b = req.postDataJSON(); w.posts.push(['notice', b])
   if (b.action === 'restore-parked') w.rows = w.rows.map(r => r.upgradedFrom === 'parked' && r.status === 'archived' ? {...r, status:'active'} : r)
   w.notice = null
   return {ok:true}
  },
 }
}

async function open(options = {}) {
 const w = world()
 const f = await openWorkspace(browser, {width:1440, height:640, ...options, routes:routes(w)})
 await f.page.goto(`${base}/skills`)
 await expect(f.page.locator('[data-skill]').first()).toBeVisible()
 return {...f, w}
}
const row = (page, name) => page.locator(`[data-skill="item-${name}"]`)
const tab = (page, name) => page.getByRole('tablist', {name:'Skill state'}).getByRole('tab', {name:new RegExp(`^${name}`)})
const toast = page => page.getByRole('status').filter({has:page.getByRole('button', {name:'Undo'})})
const posts = (w, kind) => w.posts.filter(p => p[0] === kind).map(p => p[1])

try {
 // Archive the 2nd of 6: the tab, the scroll position and the place in the list all stay; focus is on the next row.
 if (run('archive')) {
  const {context, page, errors, w} = await open()
  await expect(page.locator('[data-skill]')).toHaveCount(6)
  expect(await page.locator('[data-skill]').evaluateAll(els => els.map(e => e.dataset.skill))).toEqual(['item-reet-babysit','item-seo-bing','item-seo-cluster','item-svg-animations','item-ui-ux-pro-max','item-writing-plans'])
  await page.evaluate(() => window.scrollTo(0, 160))
  const before = await page.evaluate(() => window.scrollY)
  expect(before, 'the page can scroll at this size').toBeGreaterThan(100)
  await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
  await expect(row(page, 'seo-bing')).toHaveCount(0)
  await expect(tab(page, 'Active')).toHaveAttribute('aria-selected', 'true')
  await expect(tab(page, 'Active')).toHaveAccessibleName('Active 5')
  await expect(tab(page, 'Archived')).toHaveAccessibleName('Archived 4')
  // Its text keeps the label apart from the count too, for anything that reads text rather than the accessible name.
  expect(await page.getByRole('tablist', {name:'Skill state'}).getByRole('tab').evaluateAll(els => els.map(el => el.textContent.replace(/\s+/g, ' ').trim()))).toEqual(['Active 5', 'Archived 4'])
  expect(await page.evaluate(() => window.scrollY)).toBe(before)
  const next = row(page, 'seo-cluster').locator('[data-act="archive"]')
  await expect(next).toBeFocused()
  await expect(next).toHaveAccessibleName('Archive seo-cluster')
  await expect(toast(page)).toContainText('Archived seo-bing · coming off 3 machines')
  expect(posts(w, 'status')).toEqual([{ids:['item-seo-bing'], status:'archived', tab:'active'}])
  expect(new URL(page.url()).pathname).toBe('/skills')
  // Undo puts it back in its place, on the same tab.
  await toast(page).getByRole('button', {name:'Undo'}).click()
  await expect(row(page, 'seo-bing')).toBeVisible()
  expect(posts(w, 'status')[1]).toEqual({ids:['item-seo-bing'], status:'active', tab:'active'})
  expect((await page.locator('[data-skill]').evaluateAll(els => els.map(e => e.dataset.skill)))[1]).toBe('item-seo-bing')
  await expect(tab(page, 'Active')).toHaveAttribute('aria-selected', 'true')
  expect(await page.evaluate(() => window.scrollY)).toBe(before)
  // The last row hands focus back to the one before it.
  await row(page, 'writing-plans').getByRole('button', {name:'Archive writing-plans'}).click()
  await expect(row(page, 'ui-ux-pro-max').locator('[data-act="archive"]')).toBeFocused()
  expect(errors).toEqual([])
  await context.close()
 }

 // Delete asks once for a skill you wrote, and not for any other. A refusal for a skill reet has no copy of opens the
 // same dialog, and the delete is sent again with the flag only once the person agrees.
 if (run('delete')) {
  const {context, page, errors, w} = await open()
  await row(page, 'reet-babysit').getByRole('button', {name:'Delete reet-babysit'}).click()
  const dialog = page.getByRole('dialog', {name:'Delete reet-babysit?'})
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText('You wrote this one.')
  expect(posts(w, 'delete')).toEqual([])
  await dialog.getByRole('button', {name:'Delete', exact:true}).click()
  await expect(row(page, 'reet-babysit')).toHaveCount(0)
  expect(posts(w, 'delete')).toEqual([{ids:['item-reet-babysit'], writtenConfirmed:true, noCopyConfirmed:false}])
  await expect(toast(page)).toContainText('Deleted reet-babysit · coming off 3 machines')
  // Focus lands on the next row's Archive, not its Delete, so pressing Enter again does not delete that one too.
  await expect(row(page, 'seo-bing').locator('[data-act="archive"]')).toBeFocused()
  await toast(page).getByRole('button', {name:'Undo'}).click()
  await expect(row(page, 'reet-babysit')).toBeVisible()
  expect(posts(w, 'undelete')).toEqual([{ids:['item-reet-babysit']}])

  // A plain row: no dialog, the delete goes at once.
  await row(page, 'svg-animations').getByRole('button', {name:'Delete svg-animations'}).click()
  await expect(row(page, 'svg-animations')).toHaveCount(0)
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(posts(w, 'delete').at(-1)).toEqual({ids:['item-svg-animations'], writtenConfirmed:false, noCopyConfirmed:false})

  // The server says reet has no copy: the same dialog, then the flag, once.
  await row(page, 'ui-ux-pro-max').getByRole('button', {name:'Delete ui-ux-pro-max'}).click()
  const ask = page.getByRole('dialog', {name:'Delete ui-ux-pro-max?'})
  await expect(ask).toContainText('reet has no copy of this skill.')
  await expect(row(page, 'ui-ux-pro-max')).toBeVisible()
  await ask.getByRole('button', {name:'Delete', exact:true}).click()
  await expect(row(page, 'ui-ux-pro-max')).toHaveCount(0)
  expect(posts(w, 'delete').slice(-2)).toEqual([{ids:['item-ui-ux-pro-max'], writtenConfirmed:false, noCopyConfirmed:false}, {ids:['item-ui-ux-pro-max'], writtenConfirmed:false, noCopyConfirmed:true}])
  await expect(tab(page, 'Active')).toHaveAttribute('aria-selected', 'true')
  expect(errors).toEqual([])
  await context.close()
 }

 // Select 3 and Archive them together: one request with all three, and one toast that counts them.
 if (run('bulk')) {
  const {context, page, errors, w} = await open()
  await page.getByRole('button', {name:'Select', exact:true}).click()
  for (const name of ['svg-animations','ui-ux-pro-max','writing-plans']) await page.getByRole('checkbox', {name:`Select ${name}`}).check()
  const bar = page.getByRole('toolbar', {name:'Selected skills'})
  await expect(bar).toContainText('3 selected')
  // The bar is a toolbar: the arrow keys, Home and End move along its buttons.
  await bar.getByRole('button', {name:'Archive', exact:true}).focus()
  await page.keyboard.press('ArrowRight')
  await expect(bar.getByRole('button', {name:'Delete', exact:true})).toBeFocused()
  await page.keyboard.press('End')
  await expect(bar.getByRole('button', {name:'Cancel', exact:true})).toBeFocused()
  await page.keyboard.press('ArrowRight')
  await expect(bar.getByRole('button', {name:'Archive', exact:true})).toBeFocused()
  await page.keyboard.press('ArrowLeft')
  await expect(bar.getByRole('button', {name:'Cancel', exact:true})).toBeFocused()
  await page.keyboard.press('Home')
  await expect(bar.getByRole('button', {name:'Archive', exact:true})).toBeFocused()
  // One Tab stop: Tab leaves the bar, and Shift+Tab comes back to the button last used in it.
  await page.keyboard.press('ArrowRight')
  await page.keyboard.press('Tab')
  expect(await page.evaluate(() => !document.activeElement?.closest('[role="toolbar"]'))).toBe(true)
  await page.keyboard.press('Shift+Tab')
  await expect(bar.getByRole('button', {name:'Delete', exact:true})).toBeFocused()
  expect(await bar.evaluate(el => [...el.querySelectorAll('button')].filter(b => b.tabIndex === 0).map(b => b.textContent.trim()))).toEqual(['Delete'])
  await page.keyboard.press('Home')
  await bar.getByRole('button', {name:'Archive', exact:true}).click()
  await expect(page.locator('[data-skill]')).toHaveCount(3)
  expect(posts(w, 'status')).toEqual([{ids:['item-svg-animations','item-ui-ux-pro-max','item-writing-plans'], status:'archived', tab:'active'}])
  await expect(toast(page)).toContainText('Archived 3 skills · coming off 3 machines')
  await expect(bar).toHaveCount(0)
  await expect(row(page, 'seo-cluster').locator('[data-act="archive"]')).toBeFocused()
  await expect(tab(page, 'Active')).toHaveAttribute('aria-selected', 'true')
  expect(errors).toEqual([])
  await context.close()
 }

 // The rest of the bulk bar: Delete (asking once when the server says reet has no copy of one), Restore on the Archived
 // tab, and Add tag, each one request for the whole selection.
 if (run('bulk more')) {
  const {context, page, errors, w} = await open()
  const tagged = []
  await page.route('**/api/skills/tags', route => { tagged.push(route.request().postDataJSON()); return route.fulfill({json:{tagged:2}}) })
  const bar = page.getByRole('toolbar', {name:'Selected skills'})
  const select = async (...names) => {
   await page.getByRole('button', {name:'Select', exact:true}).click()
   for (const name of names) await page.getByRole('checkbox', {name:`Select ${name}`}).check()
  }
  // Delete
  await select('svg-animations', 'ui-ux-pro-max')
  await bar.getByRole('button', {name:'Delete', exact:true}).click()
  const ask = page.getByRole('dialog', {name:'Delete 2 skills?'})
  await expect(ask).toContainText('reet has no copy of ui-ux-pro-max.')
  await ask.getByRole('button', {name:'Delete', exact:true}).click()
  await expect(row(page, 'svg-animations')).toHaveCount(0)
  await expect(row(page, 'ui-ux-pro-max')).toHaveCount(0)
  expect(posts(w, 'delete')).toEqual([{ids:['item-svg-animations','item-ui-ux-pro-max'], writtenConfirmed:false, noCopyConfirmed:false}, {ids:['item-svg-animations','item-ui-ux-pro-max'], writtenConfirmed:false, noCopyConfirmed:true}])
  await expect(toast(page)).toContainText('Deleted 2 skills · coming off 3 machines')
  await toast(page).getByRole('button', {name:/^Undo/}).click()
  await expect(row(page, 'svg-animations')).toBeVisible()
  expect(posts(w, 'undelete')).toEqual([{ids:['item-svg-animations','item-ui-ux-pro-max']}])
  // Tag
  await select('reet-babysit', 'seo-bing')
  await bar.getByRole('button', {name:'Add tag', exact:true}).click()
  await bar.getByLabel('Tag the selected skills').fill('workflow')
  await bar.getByRole('button', {name:'Add tag', exact:true}).click()
  await expect.poll(() => tagged).toEqual([{ids:['demo/toolkit/reet-babysit','demo/toolkit/seo-bing'], add:['workflow'], remove:[]}])
  await expect(page.getByRole('status').filter({hasText:'Sorted. Tagged 2 skills with workflow.'})).toBeVisible()
  await expect(bar).toHaveCount(0)
  // Restore
  await tab(page, 'Archived').click()
  await select('improve-animations', 'seo-sxo')
  await bar.getByRole('button', {name:'Restore', exact:true}).click()
  await expect(row(page, 'improve-animations')).toHaveCount(0)
  await expect(row(page, 'seo-sxo')).toHaveCount(0)
  expect(posts(w, 'status')).toEqual([{ids:['item-improve-animations','item-seo-sxo'], status:'active', tab:'archived'}])
  await expect(toast(page)).toContainText('Restored 2 skills · going back on 3 machines')
  await expect(tab(page, 'Archived')).toHaveAttribute('aria-selected', 'true')
  expect(errors).toEqual([])
  await context.close()
 }

 // The bar's Add tag step by keyboard: the field and its buttons are in plain Tab order, an empty field skips the Add tag
 // it cannot use, Escape cancels, and the field opens empty again.
 if (run('tag keys')) {
  const {context, page, errors} = await open()
  await page.getByRole('button', {name:'Select', exact:true}).click()
  await page.getByRole('checkbox', {name:'Select seo-bing'}).check()
  const bar = page.getByRole('toolbar', {name:'Selected skills'})
  const inBar = () => page.evaluate(() => !!document.activeElement?.closest('[role="toolbar"]'))
  await bar.getByRole('button', {name:'Add tag', exact:true}).focus()
  await page.keyboard.press('Enter')
  const field = bar.getByLabel('Tag the selected skills')
  await expect(field).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(bar.getByRole('button', {name:'Cancel', exact:true})).toBeFocused()
  await page.keyboard.press('Shift+Tab')
  await expect(field).toBeFocused()
  await page.keyboard.type('seo')
  await page.keyboard.press('Tab')
  await expect(bar.getByRole('button', {name:'Add tag', exact:true})).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(bar.getByRole('button', {name:'Cancel', exact:true})).toBeFocused()
  await page.keyboard.press('Tab')
  expect(await inBar()).toBe(false)
  await field.focus()
  await page.keyboard.press('Escape')
  await expect(field).toHaveCount(0)
  await expect(bar.getByRole('button', {name:'Add tag', exact:true})).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(field).toBeFocused()
  await expect(field).toHaveValue('')
  expect(errors).toEqual([])
  await context.close()
 }

 // Every row let go of while the Add tag step is open: Escape in the field, or Cancel, puts focus on the bar's first
 // control that still works (its Cancel), never on an Add tag that cannot be used, so focus stays in the bar.
 if (run('tag deselected')) {
  const {context, page, errors} = await open()
  await page.getByRole('button', {name:'Select', exact:true}).click()
  const pick = page.getByRole('checkbox', {name:'Select seo-bing'})
  await pick.check()
  const bar = page.getByRole('toolbar', {name:'Selected skills'})
  for (const close of ['Escape', 'Cancel']) {
   await bar.getByRole('button', {name:'Add tag', exact:true}).click()
   const field = bar.getByLabel('Tag the selected skills')
   await expect(field).toBeFocused()
   await pick.uncheck()
   await expect(bar).toContainText('0 selected')
   if (close === 'Escape') {
    await field.focus()
    await page.keyboard.press('Escape')
   } else {
    await bar.getByRole('button', {name:'Cancel', exact:true}).click()
   }
   await expect(field).toHaveCount(0)
   await expect(bar.getByRole('button', {name:'Add tag', exact:true})).toBeDisabled()
   await expect(bar.getByRole('button', {name:'Cancel', exact:true}), `focus after ${close}`).toBeFocused()
   await pick.check()
  }
  expect(errors).toEqual([])
  await context.close()
 }

 // On the Archived tab: Restore sends its request and stays on Archived; the upgrade notice's Restore the Parked ones
 // sends its own.
 if (run('archived')) {
  const {context, page, errors, w} = await open()
  await tab(page, 'Archived').click()
  await expect(tab(page, 'Archived')).toHaveAttribute('aria-selected', 'true')
  const notice = page.getByRole('region', {name:'Upgrade'})
  await expect(notice).toContainText('1 Parked skill is now Archived and came off your machines.')
  await row(page, 'improve-animations').getByRole('button', {name:'Restore improve-animations'}).click()
  await expect(row(page, 'improve-animations')).toHaveCount(0)
  expect(posts(w, 'status')).toEqual([{ids:['item-improve-animations'], status:'active', tab:'archived'}])
  await expect(toast(page)).toContainText('Restored improve-animations · going back on 3 machines')
  await expect(tab(page, 'Archived')).toHaveAttribute('aria-selected', 'true')
  await notice.getByRole('button', {name:'Restore the Parked ones'}).click()
  await expect(row(page, 'seo-sxo')).toHaveCount(0)
  expect(posts(w, 'notice')).toEqual([{action:'restore-parked'}])
  await expect(notice).toHaveCount(0)
  await expect(tab(page, 'Archived')).toHaveAttribute('aria-selected', 'true')
  expect(errors).toEqual([])
  await context.close()
 }

 // While a change is on its way its button says so, and a second press sends nothing more.
 if (run('busy')) {
  const w = world()
  let release
  const held = new Promise(resolve => { release = resolve })
  const r = routes(w)
  const status = r['/api/v2/skills/status']
  r['/api/v2/skills/status'] = async req => { const answer = status(req); await held; return answer }
  const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
  await page.goto(`${base}/skills`)
  const archive = row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'})
  await archive.click()
  await expect(archive).toHaveAttribute('aria-busy', 'true')
  await expect(archive).toHaveAttribute('aria-disabled', 'true')
  await expect(row(page, 'seo-bing').getByRole('button', {name:'Delete seo-bing'})).toHaveAttribute('aria-disabled', 'true')
  // Playwright will not press a button marked disabled, so the second press is dispatched the way a script would.
  await archive.dispatchEvent('click')
  release()
  await expect(row(page, 'seo-bing')).toHaveCount(0)
  expect(posts(w, 'status')).toHaveLength(1)
  expect(errors).toEqual([])
  await context.close()
 }

 // Reads that fail: the one the person waits for says so, and the header says nothing about machines until a read has
 // worked; a read that then works clears the message; a read in the background fails quietly. A write that fails keeps
 // the row where it is and says why.
 if (run('errors')) {
  const w = world()
  const r = routes(w)
  const answer = r['/api/v2/skills']
  let failing = true
  r['/api/v2/skills'] = req => failing ? {status:500, json:{error:'The library could not be read.'}} : answer(req)
  r['/api/v2/skills/status'] = () => ({status:500, json:{error:'Archive did not go through.'}})
  const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
  const alert = page.getByRole('alert')
  const chip = page.locator('main .sync-chip')
  await page.goto(`${base}/skills`)
  await expect(alert).toContainText('The library could not be read.')
  await expect(chip).toHaveCount(0)
  failing = false
  await tab(page, 'Archived').click()
  await expect(row(page, 'improve-animations')).toBeVisible()
  await expect(alert).toHaveCount(0)
  await expect(chip).toBeVisible()
  await tab(page, 'Active').click()
  await expect(row(page, 'seo-bing')).toBeVisible()
  // In the background: nothing to say.
  failing = true
  const quiet = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v2/skills' && res.status() === 500)
  await page.evaluate(() => window.dispatchEvent(new Event('reet-shelf-changed')))
  await quiet
  await page.waitForTimeout(300)
  await expect(alert).toHaveCount(0)
  failing = false
  // A write the server refuses: the row stays, on the same tab, and the reason shows.
  await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
  await expect(alert).toContainText('Archive did not go through.')
  await expect(row(page, 'seo-bing')).toBeVisible()
  await expect(row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'})).not.toHaveAttribute('aria-busy', 'true')
  await expect(tab(page, 'Active')).toHaveAttribute('aria-selected', 'true')
  await expect(toast(page)).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // Undo from the keyboard: the toast names Ctrl+Z (⌘Z on a Mac), which undoes the last change from anywhere but a text
 // field, and the toast stays while focus is on it.
 if (run('keyboard')) {
  const {context, page, errors, w} = await open()
  const archive = row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'})
  await archive.focus()
  await page.keyboard.press('Enter')
  await expect(row(page, 'seo-bing')).toHaveCount(0)
  await expect(toast(page).getByRole('button', {name:'Undo (Ctrl+Z)'})).toBeVisible()
  await expect(toast(page).getByRole('button', {name:'Undo (Ctrl+Z)'})).toHaveAttribute('aria-keyshortcuts', 'Control+Z')
  // Typing in a field keeps its own undo.
  await page.getByRole('searchbox', {name:'Search skills'}).focus()
  await page.keyboard.press('Control+z')
  await page.waitForTimeout(200)
  expect(posts(w, 'status')).toHaveLength(1)
  await row(page, 'seo-cluster').locator('[data-act="archive"]').focus()
  await page.keyboard.press('Control+z')
  await expect(row(page, 'seo-bing')).toBeVisible()
  expect(posts(w, 'status')[1]).toEqual({ids:['item-seo-bing'], status:'active', tab:'active'})
  await expect(toast(page)).toHaveCount(0)
  // With focus on the toast it stays past its 8 seconds.
  await row(page, 'svg-animations').getByRole('button', {name:'Archive svg-animations'}).click()
  await toast(page).getByRole('button', {name:/^Undo/}).focus()
  await page.waitForTimeout(9000)
  await expect(toast(page)).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
  // On a Mac it is ⌘Z.
  const mac = world()
  const m = await openWorkspace(browser, {width:1440, height:900, routes:routes(mac)})
  await m.page.addInitScript(() => Object.defineProperty(navigator, 'platform', {get:() => 'MacIntel'}))
  await m.page.goto(`${base}/skills`)
  await row(m.page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
  await expect(toast(m.page).getByRole('button', {name:'Undo (⌘Z)'})).toHaveAttribute('aria-keyshortcuts', 'Meta+Z')
  await m.page.keyboard.press('Meta+z')
  await expect(row(m.page, 'seo-bing')).toBeVisible()
  expect(m.errors).toEqual([])
  await m.context.close()
 }

 // Undo after moving to the other tab: the skill goes back to where it was, and focus stays on the page, on the tab.
 if (run('undo elsewhere')) {
  const {context, page, errors, w} = await open()
  await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
  await tab(page, 'Archived').click()
  await expect(row(page, 'seo-bing')).toBeVisible()
  await toast(page).getByRole('button', {name:/^Undo/}).click()
  await expect(row(page, 'seo-bing')).toHaveCount(0)
  expect(posts(w, 'status')[1]).toEqual({ids:['item-seo-bing'], status:'active', tab:'archived'})
  await expect(tab(page, 'Archived')).toBeFocused()
  await tab(page, 'Active').click()
  await expect(row(page, 'seo-bing')).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // Two changes whose answers come back out of order: the older answer never puts back what the newer one took away.
 if (run('order')) {
  const w = world()
  const r = routes(w)
  const status = r['/api/v2/skills/status']
  let slow = true
  r['/api/v2/skills/status'] = async req => {
   const answer = status(req)
   if (slow) { slow = false; await new Promise(resolve => setTimeout(resolve, 1200)) }
   return answer
  }
  const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
  await page.goto(`${base}/skills`)
  const slowAnswer = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v2/skills/status' && res.request().postDataJSON().ids[0] === 'item-seo-bing')
  await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
  await row(page, 'svg-animations').getByRole('button', {name:'Archive svg-animations'}).click()
  await expect(row(page, 'svg-animations')).toHaveCount(0)
  await slowAnswer
  await page.waitForTimeout(300)
  await expect(row(page, 'seo-bing')).toHaveCount(0)
  await expect(row(page, 'svg-animations')).toHaveCount(0)
  await expect(page.locator('[data-skill]')).toHaveCount(4)
  await expect(tab(page, 'Active')).toHaveAccessibleName('Active 4')
  expect(errors).toEqual([])
  await context.close()
 }

 // An Undo whose answer is slow, and a Like pressed meanwhile: the Like's quicker answer does not stop the Undo's from
 // putting the row back at once, and the Like stays.
 if (run('undo race')) {
  const w = world()
  const r = routes(w)
  const status = r['/api/v2/skills/status']
  let slow = false
  r['/api/v2/skills/status'] = async req => {
   const answer = status(req)
   if (slow) { slow = false; await new Promise(resolve => setTimeout(resolve, 1500)) }
   return answer
  }
  r['/api/skill'] = req => {
   const b = req.postDataJSON()
   w.rows = w.rows.map(x => x.shelfId === b.id ? {...x, liked:b.liked} : x)
   return {id:b.id, liked:b.liked}
  }
  const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
  await page.goto(`${base}/skills`)
  await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
  await expect(row(page, 'seo-bing')).toHaveCount(0)
  slow = true
  const undone = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v2/skills/status' && res.request().postDataJSON().status === 'active')
  await toast(page).getByRole('button', {name:/^Undo/}).click()
  await row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'}).click()
  await undone
  // Well inside the page's 15 second read: the row comes back from the Undo itself.
  await expect(row(page, 'seo-bing')).toBeVisible({timeout:3000})
  await expect(row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'})).toHaveAttribute('aria-pressed', 'true')
  expect(errors).toEqual([])
  await context.close()
 }

 // Search finds what Markdown and CSV export find: a word in the name, the shelf id, the description or the notes, and
 // tag:x for a tag; a tag's name alone is not a word it searches.
 if (run('search')) {
  const {context, page, errors} = await open()
  const box = page.getByRole('searchbox', {name:'Search skills'})
  const shown = () => page.locator('[data-skill]').evaluateAll(els => els.map(e => e.dataset.skill))
  await box.fill('webmaster')
  await expect.poll(shown).toEqual(['item-seo-bing'])
  await box.fill('toolkit/svg')
  await expect.poll(shown).toEqual(['item-svg-animations'])
  await box.fill('seo')
  await expect.poll(shown).toEqual(['item-seo-bing', 'item-seo-cluster'])
  await box.fill('tag:seo')
  await expect.poll(shown).toEqual(['item-seo-bing'])
  await box.fill('tag:seo bing')
  await expect.poll(shown).toEqual(['item-seo-bing'])
  await box.fill('pull request')
  await expect.poll(shown).toEqual(['item-reet-babysit'])
  expect(errors).toEqual([])
  await context.close()
 }

 // A Like pressed while an Archive is on its way: the Archive's answer, older than the Like, must not leave the Like
 // undone once both have answered.
 if (run('like in flight')) {
  const w = world()
  const r = routes(w)
  const status = r['/api/v2/skills/status']
  r['/api/v2/skills/status'] = async req => { const answer = status(req); await new Promise(resolve => setTimeout(resolve, 1200)); return answer }
  r['/api/skill'] = async req => {
   const b = req.postDataJSON()
   w.rows = w.rows.map(x => x.shelfId === b.id ? {...x, liked:b.liked} : x)
   await new Promise(resolve => setTimeout(resolve, 2400))
   return {id:b.id, liked:b.liked}
  }
  const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
  await page.goto(`${base}/skills`)
  const liked = page.waitForResponse(res => new URL(res.url()).pathname === '/api/skill')
  await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
  await row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'}).click()
  await liked
  // Well inside the page's 15 second read.
  await expect(row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'})).toHaveAttribute('aria-pressed', 'true', {timeout:3000})
  await expect(row(page, 'seo-bing')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // A change that starts and answers while the page is reading a tab again after other changes: that read is out of
 // date when it lands, so the page reads the tab once more at once, not at the 11 second read after the grace or the
 // 15 second poll. Two ways in: a second Like while the read after an Archive and a Like is on its way, and a Like
 // while the read after a Delete's Undo is on its way.
 if (run('catch-up') || run('catch-up undo')) {
  const wait = ms => new Promise(resolve => setTimeout(resolve, ms))
  const isRead = (u, status) => u.pathname === '/api/v2/skills' && u.searchParams.get('status') === status
  // The next read of the Active tab, once `hold` is set, is slow.
  const holding = (w, r) => {
   const answer = r['/api/v2/skills']
   const state = {hold:false}
   r['/api/v2/skills'] = async req => {
    const out = answer(req)
    if (state.hold && isRead(new URL(req.url()), 'active')) { state.hold = false; await wait(1500) }
    return out
   }
   r['/api/skill'] = async req => {
    const b = req.postDataJSON()
    w.rows = w.rows.map(x => x.shelfId === b.id ? {...x, liked:b.liked} : x)
    if (state.slowLike) { state.slowLike = false; await wait(2400) }
    return {id:b.id, liked:b.liked}
   }
   return state
  }
  const nextRead = page => page.waitForRequest(req => isRead(new URL(req.url()), 'active'))
  if (run('catch-up')) {
   const w = world()
   const r = routes(w)
   const status = r['/api/v2/skills/status']
   r['/api/v2/skills/status'] = async req => { const answer = status(req); await wait(1200); return answer }
   const state = holding(w, r)
   state.slowLike = true
   const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
   await page.goto(`${base}/skills`)
   await expect(row(page, 'seo-bing')).toBeVisible()
   state.hold = true
   const caughtUp = nextRead(page)
   await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
   await row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'}).click()
   await caughtUp
   const second = page.waitForResponse(res => new URL(res.url()).pathname === '/api/skill')
   await row(page, 'reet-babysit').getByRole('button', {name:'Like reet-babysit'}).click()
   await second
   await expect(row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'}), 'the first Like').toHaveAttribute('aria-pressed', 'true', {timeout:4000})
   await expect(row(page, 'reet-babysit').getByRole('button', {name:'Like reet-babysit'})).toHaveAttribute('aria-pressed', 'true')
   await expect(row(page, 'seo-bing')).toHaveCount(0)
   expect(errors).toEqual([])
   await context.close()
  }
  if (run('catch-up undo')) {
   const w = world()
   const r = routes(w)
   const state = holding(w, r)
   const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
   await page.goto(`${base}/skills`)
   await expect(row(page, 'svg-animations')).toBeVisible()
   const reread = page.waitForResponse(res => isRead(new URL(res.url()), 'active'))
   await row(page, 'svg-animations').getByRole('button', {name:'Delete svg-animations'}).click()
   await expect(row(page, 'svg-animations')).toHaveCount(0)
   await reread
   state.hold = true
   const caughtUp = nextRead(page)
   await toast(page).getByRole('button', {name:/^Undo/}).click()
   await caughtUp
   const liked = page.waitForResponse(res => new URL(res.url()).pathname === '/api/skill')
   await row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'}).click()
   await liked
   await expect(row(page, 'svg-animations'), 'the row the Undo brought back').toBeVisible({timeout:4000})
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // A Like stays pressed while an older change's answer lands, whichever of the two answers first: the page never shows
 // it unpressed in between.
 if (run('like flicker')) {
  const wait = ms => new Promise(resolve => setTimeout(resolve, ms))
  for (const likeFirst of [false, true]) {
   const w = world()
   const r = routes(w)
   const status = r['/api/v2/skills/status']
   r['/api/v2/skills/status'] = async req => { const answer = status(req); await wait(likeFirst ? 1500 : 800); return answer }
   r['/api/skill'] = async req => {
    const b = req.postDataJSON()
    w.rows = w.rows.map(x => x.shelfId === b.id ? {...x, liked:b.liked} : x)
    await wait(likeFirst ? 300 : 2000)
    return {id:b.id, liked:b.liked}
   }
   const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
   await page.goto(`${base}/skills`)
   await expect(row(page, 'ui-ux-pro-max')).toBeVisible()
   // Every value the Like button shows from now on.
   await page.locator('ul.list').evaluate(list => {
    window.__pressed = []
    new MutationObserver(records => {
     for (const m of records) if (m.target.getAttribute('aria-label') === 'Like ui-ux-pro-max') window.__pressed.push(m.target.getAttribute('aria-pressed'))
    }).observe(list, {subtree:true, attributes:true, attributeFilter:['aria-pressed']})
   })
   const archived = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v2/skills/status')
   const liked = page.waitForResponse(res => new URL(res.url()).pathname === '/api/skill')
   await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
   await row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'}).click()
   await Promise.all([archived, liked])
   await expect(row(page, 'seo-bing')).toHaveCount(0)
   await page.waitForTimeout(600)
   await expect(row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'})).toHaveAttribute('aria-pressed', 'true')
   const shown = await page.evaluate(() => window.__pressed.filter((v, i, all) => v !== all[i - 1]))
   expect(shown, `the Like's values when ${likeFirst ? 'it answers first' : 'the Archive answers first'}`).toEqual(['true'])
   expect(errors).toEqual([])
   await context.close()
  }
  // A Like that fails goes back, and says why.
  {
   const w = world()
   const r = routes(w)
   r['/api/skill'] = () => ({status:500, json:{error:'The like did not go through.'}})
   const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
   await page.goto(`${base}/skills`)
   const heart = row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'})
   await heart.click()
   await expect(page.getByRole('alert')).toContainText('The like did not go through.')
   await expect(heart).toHaveAttribute('aria-pressed', 'false')
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // A row taken off the tab stays off it when an older change's answer, made before it went, lands afterwards: a Delete
 // while a slow Archive is on its way never shows the deleted row again. Every render of the list is recorded. An
 // Archive's Undo still brings its row back at once while another change is on its way.
 if (run('delete flicker')) {
  const wait = ms => new Promise(resolve => setTimeout(resolve, ms))
  {
   const w = world()
   const r = routes(w)
   const status = r['/api/v2/skills/status']
   r['/api/v2/skills/status'] = async req => { const answer = status(req); await wait(1500); return answer }
   const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
   await page.goto(`${base}/skills`)
   await expect(row(page, 'svg-animations')).toBeVisible()
   // Whether the deleted row is in the list, after every change to it.
   await page.locator('ul.list').evaluate(list => {
    window.__there = [true]
    new MutationObserver(() => window.__there.push(!!list.querySelector('[data-skill="item-svg-animations"]'))).observe(list, {childList:true, subtree:true})
   })
   const archived = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v2/skills/status')
   await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
   await row(page, 'svg-animations').getByRole('button', {name:'Delete svg-animations'}).click()
   await expect(row(page, 'svg-animations')).toHaveCount(0)
   await archived
   await expect(row(page, 'seo-bing')).toHaveCount(0)
   await page.waitForTimeout(800)
   const there = await page.evaluate(() => window.__there.filter((v, i, all) => v !== all[i - 1]))
   expect(there, 'the deleted row in the list, render by render').toEqual([true, false])
   await expect(page.locator('[data-skill]')).toHaveCount(4)
   expect(errors).toEqual([])
   await context.close()
  }
  {
   const w = world()
   const r = routes(w)
   r['/api/skill'] = async req => {
    const b = req.postDataJSON()
    w.rows = w.rows.map(x => x.shelfId === b.id ? {...x, liked:b.liked} : x)
    await wait(3000)
    return {id:b.id, liked:b.liked}
   }
   const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
   await page.goto(`${base}/skills`)
   // The Archive answers while the Like is on its way, so its row is held off the tab when Undo is pressed.
   await row(page, 'ui-ux-pro-max').getByRole('button', {name:'Like ui-ux-pro-max'}).click()
   await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
   await expect(row(page, 'seo-bing')).toHaveCount(0)
   await toast(page).getByRole('button', {name:/^Undo/}).click()
   // Well before the Like answers.
   await expect(row(page, 'seo-bing'), 'the row the Undo brought back').toBeVisible({timeout:1500})
   await page.waitForTimeout(2500)
   await expect(row(page, 'seo-bing')).toBeVisible()
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // The page gone (the sign-in screen took its place) while a change was on its way: when the change answers, the page
 // starts no reads, neither at once nor after the grace period.
 if (run('unmount')) {
  const w = world()
  const r = routes(w)
  const status = r['/api/v2/skills/status']
  let release
  const held = new Promise(resolve => { release = resolve })
  r['/api/v2/skills/status'] = async req => { const answer = status(req); await held; return answer }
  const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
  await page.clock.install()
  await page.goto(`${base}/skills`)
  await expect(row(page, 'seo-bing')).toBeVisible()
  await page.clock.pauseAt(await page.evaluate(() => Date.now() + 200))
  const sent = page.waitForRequest(req => new URL(req.url()).pathname === '/api/v2/skills/status')
  await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
  await sent
  await page.evaluate(() => window.dispatchEvent(new CustomEvent('reet-signed-out')))
  await expect(page.locator('.skills-page')).toHaveCount(0)
  const reads = []
  page.on('request', req => { const u = new URL(req.url()); if (u.pathname === '/api/v2/skills' && req.method() === 'GET') reads.push(u.search) })
  const answered = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v2/skills/status')
  release()
  await answered
  await page.waitForTimeout(300)
  await page.clock.fastForward(12000)
  await page.waitForTimeout(300)
  expect(reads, 'reads the gone page started').toEqual([])
  expect(errors).toEqual([])
  await context.close()
 }

 // A failed row says where, toned, with Retry; Retry posts the skill.
 if (run('retry')) {
  const {context, page, errors, w} = await open()
  const health = row(page, 'seo-cluster').locator('.health')
  await expect(health).toContainText('Failed on ThinkPad')
  await expect(health).toHaveAttribute('data-tone', 'bad')
  await expect(row(page, 'writing-plans').locator('.health')).toContainText('Installing on 1 of 3')
  await expect(row(page, 'seo-bing').locator('.health')).toContainText('On all 3 machines')
  await expect(row(page, 'seo-bing').getByRole('button', {name:/^Retry/})).toHaveCount(0)
  const request = page.waitForRequest(r => new URL(r.url()).pathname === '/api/v2/skills/retry')
  await row(page, 'seo-cluster').getByRole('button', {name:'Retry seo-cluster'}).click()
  await request
  expect(posts(w, 'retry')).toEqual([{items:['item-seo-cluster']}])
  expect(errors).toEqual([])
  await context.close()
 }

 // The header's line says what the machines are doing, as the server counts it over every skill: coming off after an
 // Archive, installing after an Undo, updating when both happen at once.
 if (run('chip')) {
  const on = state => ['Elitebook','ThinkPad','studio'].map(host => ({host, state}))
  const w = {rows:[skillRow('alpha'), skillRow('beta'), skillRow('gamma'), skillRow('delta')], deleted:[], posts:[], noCopy:new Set(), notice:null}
  // The server's count: each of the three machines, once, by what its skills have left to do.
  const workOf = () => {
   const installing = w.rows.some(x => x.health.state === 'installing'), removing = w.rows.some(x => x.health.state === 'removing') || w.deleted.length > 0
   return {failed:0, installing:installing ? 3 : 0, removing:removing ? 3 : 0, updating:installing || removing ? 3 : 0}
  }
  const r = routes(w)
  r['/api/v2/skills'] = req => skillsPageOf(w.rows, new URL(req.url()).searchParams.get('status') || 'active', {work:workOf()})
  r['/api/v2/skills/status'] = req => {
   const b = req.postDataJSON(); w.posts.push(['status', b])
   w.rows = w.rows.map(x => b.ids.includes(x.id) ? {...x, status:b.status, health:{state:b.status === 'archived' ? 'removing' : 'installing', pending:3, total:3, hosts:[], machines:on(b.status === 'archived' ? 'removing' : 'installing')}} : x)
   return skillsPageOf(w.rows, b.tab || b.status, {work:workOf()})
  }
  const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
  await page.goto(`${base}/skills`)
  const chip = page.locator('main .sync-chip')
  await expect(chip).toHaveText('Everything is in sync.')
  await row(page, 'beta').getByRole('button', {name:'Archive beta'}).click()
  await expect(toast(page)).toContainText('Archived beta · coming off 3 machines')
  await expect(chip).toHaveText('Coming off 3 machines…')
  await expect(chip).toHaveAttribute('data-tone', 'work')
  await toast(page).getByRole('button', {name:/^Undo/}).click()
  await expect(chip).toHaveText('Installing on 3 machines…')
  await row(page, 'gamma').getByRole('button', {name:'Archive gamma'}).click()
  await expect(chip).toHaveText('Updating 3 machines…')
  expect(errors).toEqual([])
  await context.close()
 }

 // A machine below the skills protocol gets no skill work: the header says to update reet there, even when no machine
 // follows the library yet.
 if (run('needs update')) {
  const w = world()
  const r = routes(w)
  r['/api/v2/skills'] = req => skillsPageOf(w.rows, new URL(req.url()).searchParams.get('status') || 'active', {machines:0, work:{failed:0, installing:0, removing:0, updating:0, needsUpdate:1, needsUpdateHost:'ThinkPad'}})
  const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
  await page.goto(`${base}/skills`)
  const chip = page.locator('main .sync-chip')
  await expect(chip).toHaveText('Update reet on ThinkPad to keep its skills in sync')
  await expect(chip).toHaveAttribute('data-tone', 'bad')
  expect(errors).toEqual([])
  await context.close()
 }

 // A machine whose reet is newer than this controller gets no skill work either: the header says to update the
 // controller, even when no machine follows the library yet, and never that everything is in sync.
 if (run('newer reet')) {
  const w = world()
  const r = routes(w)
  r['/api/v2/skills'] = req => skillsPageOf(w.rows, new URL(req.url()).searchParams.get('status') || 'active', {machines:0, work:{failed:0, installing:0, removing:0, updating:0, needsUpdate:0, needsNewerController:1, needsNewerControllerHost:'ThinkPad'}})
  // The longer line fits the header at each size, in light and dark.
  for (const [width, theme] of [[1440,'light'],[1024,'dark'],[390,'light'],[390,'dark']]) {
   const {context, page, errors} = await openWorkspace(browser, {width, height:844, theme, touch:width === 390, routes:r})
   await page.goto(`${base}/skills`)
   const chip = page.locator('main .sync-chip')
   await expect(chip).toHaveText('ThinkPad runs a newer reet than this controller; update the controller to keep its skills in sync')
   await expect(chip).toHaveAttribute('data-tone', 'bad')
   const box = await chip.boundingBox()
   expect(box.x + box.width, `chip right edge at ${width}px ${theme}`).toBeLessThanOrEqual(width)
   expect(box.height, `chip height at ${width}px ${theme}`).toBeGreaterThanOrEqual(44)
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px ${theme}`).toBe(true)
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // A change reaches the machines only after the server's 10 second grace (an Undo inside it costs them nothing), so
 // the machines' work counts a deleted skill only then. The page reads once more just after the grace, so the header
 // says it is coming off then, not at the next 15 second read. The page's clock is driven by the test.
 if (run('grace')) {
  const w = {rows:[skillRow('alpha'), skillRow('delta')], deleted:[], posts:[], noCopy:new Set(), notice:null, graceOver:false}
  const r = routes(w)
  const workOf = () => w.graceOver && w.deleted.length ? {failed:0, installing:0, removing:3, updating:3} : {failed:0, installing:0, removing:0, updating:0}
  r['/api/v2/skills'] = req => skillsPageOf(w.rows, new URL(req.url()).searchParams.get('status') || 'active', {work:workOf()})
  const {context, page, errors} = await openWorkspace(browser, {width:1440, height:900, routes:r})
  await page.clock.install()
  await page.goto(`${base}/skills`)
  await expect(row(page, 'delta')).toBeVisible()
  await page.clock.pauseAt(await page.evaluate(() => Date.now() + 200))
  const chip = page.locator('main .sync-chip')
  await expect(chip).toHaveText('Everything is in sync.')
  await row(page, 'delta').getByRole('button', {name:'Delete delta'}).click()
  await expect(row(page, 'delta')).toHaveCount(0)
  await expect(toast(page)).toContainText('Deleted delta · coming off 3 machines')
  // Inside the grace the machines have nothing to do yet.
  await expect(chip).toHaveText('Everything is in sync.')
  w.graceOver = true
  await page.clock.fastForward(11000)
  await expect(chip).toHaveText('Coming off 3 machines…')
  expect(errors).toEqual([])
  await context.close()
 }

 // A Jump result lands on the row and brings it into view.
 if (run('jump')) {
  const w = world()
  const {context, page, errors} = await openWorkspace(browser, {width:1440, height:500, routes:routes(w)})
  await page.goto(`${base}/skills?skill=${encodeURIComponent('item-writing-plans')}`)
  await expect(row(page, 'writing-plans')).toBeInViewport()
  await expect(row(page, 'writing-plans')).toHaveClass(/flash/)
  expect(errors).toEqual([])
  await context.close()
 }

 // At 390 px in light and dark: nothing scrolls sideways, every row button is a 44 px target, and no text is under 14 px.
 // At 1024 and 1440 px nothing scrolls sideways either.
 if (run('phone')) {
  for (const [width, theme] of [[390,'light'],[390,'dark'],[1024,'light'],[1440,'dark']]) {
   const {context, page, errors} = await open({width, height:844, theme, touch:width === 390})
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px ${theme}`).toBe(true)
   const small = await page.locator('.skills-page').evaluate(root => [...root.querySelectorAll('*')].filter(el => el.checkVisibility() && [...el.childNodes].some(n => n.nodeType === 3 && n.textContent.trim()) && parseFloat(getComputedStyle(el).fontSize) < 14).map(el => `${el.tagName} "${el.textContent.trim().slice(0, 30)}"`))
   expect(small, `text under 14 px at ${width}px ${theme}`).toEqual([])
   if (width === 390) {
    for (const button of await page.locator('[data-skill] button').all()) {
     const box = await button.boundingBox()
     const name = await button.evaluate(el => el.getAttribute('aria-label') || el.textContent.trim())
     expect(box.height, `${name} height at ${width}px ${theme}`).toBeGreaterThanOrEqual(44)
     expect(box.width, `${name} width at ${width}px ${theme}`).toBeGreaterThanOrEqual(44)
     expect(box.x + box.width, `${name} right edge at ${width}px ${theme}`).toBeLessThanOrEqual(width)
    }
    // Selecting: each row's checkbox is a 44 px target too (the label around it is what a finger presses).
    await page.getByRole('button', {name:'Select', exact:true}).click()
    const checks = page.locator('[data-skill] input')
    await expect(checks).toHaveCount(6)
    for (const input of await checks.all()) {
     const hit = await input.evaluate(el => { const r = (el.closest('label') ?? el).getBoundingClientRect(); return {w:r.width, h:r.height, right:r.right, name:el.getAttribute('aria-label')} })
     expect(hit.h, `${hit.name} target height at ${width}px ${theme}`).toBeGreaterThanOrEqual(44)
     expect(hit.w, `${hit.name} target width at ${width}px ${theme}`).toBeGreaterThanOrEqual(44)
     expect(hit.right, `${hit.name} right edge at ${width}px ${theme}`).toBeLessThanOrEqual(width)
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width selecting at ${width}px ${theme}`).toBe(true)
    await page.getByRole('button', {name:'Select', exact:true}).click()
    await row(page, 'seo-bing').getByRole('button', {name:'Archive seo-bing'}).click()
    await expect(toast(page)).toBeVisible()
    const t = await toast(page).boundingBox()
    expect(t.x, 'toast left edge').toBeGreaterThanOrEqual(0)
    expect(t.x + t.width, 'toast right edge').toBeLessThanOrEqual(width)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width after Archive at ${width}px ${theme}`).toBe(true)
   }
   expect(errors).toEqual([])
   await context.close()
  }
 }
 console.log('Skills pruning passed: Archive keeps the tab, the scroll and the place with focus on the next row, Undo puts it back (Ctrl+Z too, and after a tab change), Delete asks only for a skill you wrote or one reet has no copy of, bulk Archive, Delete, Restore and Tag are one request each, Restore and Restore the Parked ones send theirs, the header says what the machines are doing, failures say why without stale messages, answers out of order never undo a newer change, busy rows say so, Retry posts, a jump lands on its row, and phones hold 44 px targets in light and dark.')
} finally {await browser.close()}
