// The public Community page: one collection surface, honest tab counts, a recoverable
// empty search and a phone layout that keeps every control reachable.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const list = (id, name, description, slug, skillCount, updatedAt) => ({ id, ownerId: 1, ownerName: 'Sam Taylor', name, description, visibility: 'public', slug, skillCount, memberCount: 2, createdAt: now - 8.64e7, updatedAt })
const community = {
  lists: [
    list(1, 'Launch checklist', 'Everything for a release week.', 'launch-checklist', 9, now - 3.6e6),
    list(2, 'Data wrangling', 'CSV, SQL and charts without the fuss.', 'data-wrangling', 14, now - 8.64e7),
    list(3, 'Docs that ship', 'Release notes, READMEs and changelogs.', 'docs-that-ship', 8, now - 1.728e8),
  ],
  skills: [
    { ...skill('pdf'), useCount: 7, memberCount: 4, note: '' },
    { ...skill('frontend-design'), useCount: 3, memberCount: 2, note: '' },
    { ...skill('code-review'), useCount: 9, memberCount: 5, note: '' },
  ],
}
const open = options => openWorkspace(browser, { width: 1440, routes: { '/api/community': community }, ...options })
const count = page => page.getByRole('region', { name: 'Community' }).getByRole('status').first()
const tab = (page, name) => page.getByRole('group', { name: 'Show community' }).getByRole('button', { name })

try {
  // The panel says how many records the active tab holds; each tab carries its own honest figure.
  if (run('counts and tabs')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/community`)
    await expect(count(page)).toHaveText('3 lists')
    await expect(tab(page, /^Lists/)).toHaveText(/Lists\s*3/)
    await expect(tab(page, /^Skills/)).toHaveText(/Skills\s*3/)
    await expect(page.getByRole('link', { name: /Launch checklist/ })).toHaveAttribute('href', '/l/launch-checklist')
    await expect(page.getByRole('link', { name: /Data wrangling/ })).toHaveAttribute('href', '/l/data-wrangling')

    await tab(page, /^Skills/).click()
    await expect(count(page)).toHaveText('3 skills')
    await expect(page.getByRole('button', { name: 'Save to your library' })).toHaveCount(3)
    await expect(page.getByRole('button', { name: 'Copy install command' })).toHaveCount(3)
    await page.getByRole('button', { name: 'Save to your library' }).first().click()
    await expect(page.getByRole('button', { name: 'Saved to your library' })).toBeVisible()
    await expect(page.getByRole('status').filter({ hasText: 'Saved pdf to your library.' })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  // A search narrows the active count, the tab counts and the empty state, and clearing it brings them back.
  if (run('search and honest counts')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/community`)
    await page.getByLabel('Search community').fill('data')
    await expect(count(page)).toHaveText('1 of 3 lists')
    await expect(tab(page, /^Lists/)).toHaveText(/Lists\s*1/)
    await expect(tab(page, /^Skills/)).toHaveText(/Skills\s*0/)

    await tab(page, /^Skills/).click()
    await expect(count(page)).toHaveText('0 of 3 skills')
    await expect(page.getByRole('heading', { name: 'Nowt matches just yet.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear search' }).click()
    await expect(count(page)).toHaveText('3 skills')

    await tab(page, /^Lists/).click()
    await page.getByLabel('Search community').fill('pdf')
    await expect(count(page)).toHaveText('0 of 3 lists')
    await tab(page, /^Skills/).click()
    await expect(count(page)).toHaveText('1 of 3 skills')
    await expect(page.getByRole('button', { name: 'Save to your library' })).toHaveCount(1)
    expect(errors).toEqual([])
    await context.close()
  }

  // A phone keeps the search, both tabs and every row reachable without sideways scroll.
  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/community`)
      await expect(count(page)).toHaveText('3 lists')
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      for (const name of [/^Lists/, /^Skills/]) {
        const box = await tab(page, name).boundingBox()
        expect(box.height, `tab ${name} at ${width}px`).toBeGreaterThanOrEqual(44)
        expect(box.x + box.width, `tab right edge at ${width}px`).toBeLessThanOrEqual(width)
      }
      const row = await page.getByRole('link', { name: /Launch checklist/ }).boundingBox()
      expect(row.x + row.width, `row right edge at ${width}px`).toBeLessThanOrEqual(width)
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // The reading state lives in the panel, not in a bare sentence above the page.
  if (run('loading')) {
    let release
    const gate = new Promise(resolve => { release = resolve })
    const { context, page, errors } = await openWorkspace(browser, { width: 1440, routes: { '/api/community': community }, gates: { '/api/community': gate } })
    await page.goto(`${base}/community`)
    await expect(page.getByRole('region', { name: 'Community' }).getByRole('status', { name: /Reading the community/ })).toBeVisible()
    await expect(count(page)).toHaveText('Reading the community…')
    release()
    await expect(count(page)).toHaveText('3 lists')
    expect(errors).toEqual([])
    await context.close()
  }
} finally {
  await browser.close()
}
console.log('Community: one surface, honest tab counts, a recoverable empty search and a phone layout.')
