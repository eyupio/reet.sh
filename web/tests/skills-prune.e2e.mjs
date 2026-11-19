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
  await expect(tab(page, 'Active')).toContainText('5')
  await expect(tab(page, 'Archived')).toContainText('4')
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
  await expect(row(page, 'seo-bing').locator('[data-act="delete"]')).toBeFocused()
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
 console.log('Skills pruning passed: Archive keeps the tab, the scroll and the place with focus on the next row, Undo puts it back, Delete asks only for a skill you wrote or one reet has no copy of, bulk Archive is one request, Restore and Restore the Parked ones send theirs, Retry posts, a jump lands on its row, and phones hold 44 px targets in light and dark.')
} finally {await browser.close()}
