// The Lists page: one collection surface, honest chip counts, the search and the saved
// layout, on a desktop and on a phone.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const list = (id, name, description, visibility, skillCount, ownerName, role, updatedAt, ownerId = 1, over = {}) => ({
  id, ownerId, ownerName, name, description, visibility, skillCount, memberCount: 3, role,
  sharedWithMe: ownerId !== 1, createdAt: now - 8.64e7, updatedAt, ...over,
})
const lists = [
  list(1, 'Team essentials', 'The baseline set every engineer gets on day one.', 'team', 12, 'Sam Taylor', 'owner', now - 3.6e6),
  list(2, 'Writing and research', 'Drafting, editing and source-checking helpers.', 'personal', 6, 'Sam Taylor', 'owner', now - 8.64e7),
  list(3, 'Launch checklist', 'Everything for a release week.', 'public', 9, 'Alex Morgan', 'viewer', now - 1.728e8, 2),
  list(4, 'Design reviews', 'Screenshots, critique and accessibility passes.', 'members', 4, 'Jo Patel', 'editor', now - 2.592e8, 3),
  list(5, 'Unlisted: client kit', 'A link-only collection for the Northwind team.', 'link', 7, 'Sam Taylor', 'owner', now - 4.32e8),
]
const routes = over => ({ '/api/v2/lists': { lists, userId: 1, linkEnabled: true, ...over } })
const open = options => openWorkspace(browser, { routes: routes(), ...options })

const count = page => page.getByRole('region', { name: 'Your lists' }).getByRole('status').first()
const chip = (page, name) => page.getByRole('group', { name: 'Show lists' }).getByRole('button', { name })
const rows = page => page.locator('a.list')

try {
  // The panel holds one count from the API, and each scope chip says how many lists pressing it shows.
  if (run('counts and scopes')) {
    const { context, page, errors } = await open({ width: 1440 })
    await page.goto(`${base}/shared`)
    await expect(count(page)).toHaveText('5 lists')
    await expect(chip(page, /^All/)).toHaveText(/All\s*5/)
    await expect(chip(page, /^Mine/)).toHaveText(/Mine\s*3/)
    await expect(chip(page, /^Shared with me/)).toHaveText(/Shared with me\s*2/)
    await expect(chip(page, /^Teams/)).toHaveText(/Teams\s*1/)
    await expect(chip(page, /^Legacy/)).toHaveText(/Legacy\s*1/)

    await chip(page, /^Mine/).click()
    await expect(count(page)).toHaveText('3 of 5 lists')
    await expect(rows(page)).toHaveCount(3)
    await expect(page.getByRole('link', { name: /Team essentials/ })).toBeVisible()
    await expect(page.getByRole('link', { name: /Writing and research/ })).toBeVisible()
    await expect(page.getByRole('link', { name: /Unlisted: client kit/ })).toBeVisible()

    await chip(page, /^Shared with me/).click()
    await expect(count(page)).toHaveText('2 of 5 lists')
    await expect(page.getByRole('link', { name: /Launch checklist/ })).toBeVisible()
    await expect(page.getByRole('link', { name: /Design reviews/ })).toBeVisible()

    await chip(page, /^Teams/).click()
    await expect(count(page)).toHaveText('1 of 5 lists')
    await expect(page.getByRole('link', { name: /Team essentials/ })).toBeVisible()

    await chip(page, /^Legacy/).click()
    await expect(count(page)).toHaveText('1 of 5 lists')
    await expect(page.getByRole('link', { name: /Design reviews/ })).toBeVisible()

    await chip(page, /^All/).click()
    await expect(count(page)).toHaveText('5 lists')
    await expect(rows(page)).toHaveCount(5)
    expect(errors).toEqual([])
    await context.close()
  }

  // A search narrows both the rows and what each chip promises, and the empty result can be recovered from.
  if (run('search and honest counts')) {
    const { context, page, errors } = await open({ width: 1440 })
    await page.goto(`${base}/shared`)
    await page.getByLabel('Search lists').fill('alex')
    await expect(count(page)).toHaveText('1 of 5 lists')
    await expect(chip(page, /^All/)).toHaveText(/All\s*1/)
    await expect(chip(page, /^Mine/)).toHaveText(/Mine\s*0/)
    await expect(chip(page, /^Shared with me/)).toHaveText(/Shared with me\s*1/)
    await expect(chip(page, /^Teams/)).toHaveText(/Teams\s*0/)
    await expect(chip(page, /^Legacy/)).toHaveCount(0)
    await expect(rows(page)).toHaveCount(1)

    await page.getByLabel('Search lists').fill('zzz')
    await expect(count(page)).toHaveText('0 of 5 lists')
    await expect(page.getByRole('heading', { name: 'Nowt matches just yet.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('5 lists')
    await expect(rows(page)).toHaveCount(5)
    expect(errors).toEqual([])
    await context.close()
  }

  // The layout choice rides the existing key, survives a reload, and never hides a record.
  if (run('saved layout')) {
    const { context, page, errors } = await open({ width: 1440 })
    await page.goto(`${base}/shared`)
    await page.getByRole('button', { name: 'Cards' }).click()
    await expect(page.getByRole('button', { name: 'Cards' })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByRole('region', { name: 'Your lists' }).locator('.cards')).toBeVisible()
    expect(await page.evaluate(() => localStorage.getItem('reet-lists-layout'))).toBe('cards')
    await page.reload()
    await expect(count(page)).toHaveText('5 lists')
    await expect(page.getByRole('button', { name: 'Cards' })).toHaveAttribute('aria-pressed', 'true')
    await page.getByRole('button', { name: 'Rows' }).click()
    expect(await page.evaluate(() => localStorage.getItem('reet-lists-layout'))).toBe('rows')
    expect(errors).toEqual([])
    await context.close()
  }

  // A phone keeps every control reachable, every control a finger target, and no sideways scroll.
  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/shared`)
      await expect(count(page)).toHaveText('5 lists')
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      for (const name of [/^All/, /^Mine/, /^Shared with me/, /^Teams/, /^Legacy/]) {
        const box = await chip(page, name).boundingBox()
        expect(box, `chip ${name} at ${width}px`).not.toBeNull()
        expect(box.height, `chip height at ${width}px`).toBeGreaterThanOrEqual(44)
        expect(box.x + box.width, `chip right edge at ${width}px`).toBeLessThanOrEqual(width)
      }
      for (const name of ['Rows', 'Cards']) {
        const box = await page.getByRole('button', { name }).boundingBox()
        expect(box.height, `${name} height at ${width}px`).toBeGreaterThanOrEqual(44)
      }
      const row = await rows(page).last().boundingBox()
      expect(row.height, `last row height at ${width}px`).toBeGreaterThanOrEqual(44)
      expect(row.x + row.width, `last row right edge at ${width}px`).toBeLessThanOrEqual(width)
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // The reading state lives in the panel, and the count says it is still reading.
  if (run('loading')) {
    let release
    const gate = new Promise(resolve => { release = resolve })
    const { context, page, errors } = await openWorkspace(browser, { width: 1440, routes: routes(), gates: { '/api/v2/lists': gate } })
    await page.goto(`${base}/shared`)
    await expect(page.getByRole('region', { name: 'Your lists' }).getByRole('status', { name: /Reading your lists/ })).toBeVisible()
    await expect(count(page)).toHaveText('Reading your lists…')
    release()
    await expect(count(page)).toHaveText('5 lists')
    expect(errors).toEqual([])
    await context.close()
  }

  // A scope that is not in the data is not offered.
  if (run('no empty legacy chip')) {
    const two = lists.filter(list => list.visibility !== 'members')
    const { context, page } = await openWorkspace(browser, { width: 1440, routes: { '/api/v2/lists': { lists: two, userId: 1, linkEnabled: true } } })
    await page.goto(`${base}/shared`)
    await expect(count(page)).toHaveText('4 lists')
    await expect(chip(page, /^Legacy/)).toHaveCount(0)
    await context.close()
  }
} finally {
  await browser.close()
}
console.log('Lists: one surface, honest chip counts, a recoverable empty search, a saved layout and a phone toolbar.')
