// The public Community page: one collection surface, honest tab counts, a recoverable
// empty search and a phone layout that keeps every control reachable.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill, skillsPageOf } from './helpers/workspace.mjs'

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
    expect(errors).toEqual([])
    await context.close()
  }

  // Save asks Active or Archived, with Active preselected, and sends the one chosen.
  if (run('save as active or archived')) {
    const posts = []
    const library = (request) => {
      if (request.method() === 'POST') { const body = request.postDataJSON(); posts.push(body); return { ...skill(body.input.split('/').pop()), id: body.input, created: true } }
      return { skills: [], counts: { all: 0, used: 0, liked: 0, bookmarked: 0 } }
    }
    for (const [choice, notice] of [['Active', 'Saved pdf to your library as Active · going on every machine'], ['Archived', 'Saved pdf to your library as Archived · kept off your machines']]) {
      posts.length = 0
      const { context, page, errors } = await open({ routes: { '/api/community': community, '/api/skills': library } })
      await page.goto(`${base}/community`)
      await tab(page, /^Skills/).click()
      await page.getByRole('button', { name: 'Save to your library' }).first().click()
      const dialog = page.getByRole('dialog', { name: 'Save pdf to your library' })
      await expect(dialog).toBeVisible()
      await expect(dialog.getByRole('radio', { name: /^Active/ })).toBeChecked()
      await expect(dialog.getByRole('radio', { name: /^Archived/ })).not.toBeChecked()
      expect(posts).toEqual([])
      await dialog.getByRole('radio', { name: new RegExp(`^${choice}`) }).check()
      await dialog.getByRole('button', { name: 'Save', exact: true }).click()
      await expect(page.getByRole('status').filter({ hasText: notice })).toBeVisible()
      expect(posts).toEqual([{ input: 'demo/toolkit/pdf', status: choice.toLowerCase() }])
      await expect(page.getByRole('button', { name: 'Saved to your library' })).toBeVisible()
      // reet's copy is the one to use now: Copy prompt, not the original's install command.
      await expect(page.getByRole('button', { name: 'Copy prompt for pdf' })).toBeVisible()
      await expect(page.getByRole('button', { name: 'Copy install command' })).toHaveCount(2)
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // A list's Add all says how many skills go on how many machines before it sends anything.
  if (run('add all asks first')) {
    const names = ['pdf', 'docx', 'xlsx', 'pptx', 'csv', 'sql', 'charts', 'notes', 'review', 'release', 'changelog', 'readme']
    const detail = { id: 7, ownerId: 1, ownerName: 'Sam', name: 'Launch checklist', description: 'Everything for a release week.', visibility: 'public', slug: 'launch-checklist', role: 'reader', skillCount: 12, memberCount: 2, createdAt: now, updatedAt: now, skills: names.map(name => ({ ...skill(name), useCount: 0, memberCount: 2, note: '' })), members: [], linkEnabled: true, canEdit: false, canManage: false, canTransfer: false }
    const posts = []
    const { context, page, errors } = await open({ routes: {
      '/api/v2/lists/7': detail,
      '/api/v2/skills': skillsPageOf([], 'active', { machines: 3 }),
      '/api/v2/lists/7/add-to-shelf': request => { posts.push(request.postDataJSON()); return { added: 12 } },
    } })
    await page.goto(`${base}/lists/7`)
    await page.getByRole('button', { name: 'Add all to my library' }).click()
    const dialog = page.getByRole('dialog', { name: 'Put 12 skills on all 3 machines?' })
    await expect(dialog).toBeVisible()
    await expect(dialog.getByRole('radio', { name: /^Active/ })).toBeChecked()
    expect(posts).toEqual([])
    await dialog.getByRole('radio', { name: /^Archived/ }).check()
    await expect(page.getByRole('dialog', { name: 'Add 12 skills to your library as Archived?' })).toBeVisible()
    await page.getByRole('dialog').getByRole('radio', { name: /^Active/ }).check()
    await page.getByRole('dialog', { name: 'Put 12 skills on all 3 machines?' }).getByRole('button', { name: 'Add all', exact: true }).click()
    await expect(page.getByText('Added 12 skills to your library as Active · going on every machine')).toBeVisible()
    expect(posts).toEqual([{ status: 'active' }])
    // Cancel sends nothing.
    await page.getByRole('button', { name: 'Add all to my library' }).click()
    await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    expect(posts).toHaveLength(1)
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
