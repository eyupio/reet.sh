// Admin → Team usage: the adoption report in the collection grammar, with a count,
// search, list/tag filters and one row per skill.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const adminStatus = {
  version: 'v1.4.2', commit: 'fixture', db: { path: '/data/reet.db', size: 1024 },
  counts: { all: 0, liked: 0, bookmarked: 0, used: 0 }, stages: { use: 0, try: 0, shelf: 0 },
  environments: [], events: [], skillsSh: true, metaLastOK: '', metaLastError: '',
  topics: { lastSynced: '', lastError: '' },
  settings: { refreshEnabled: true, topicsEnabled: true, linkListsEnabled: false, rollupIndividual: true, pruneServerEnvironmentsDays: 0, instanceName: 'Test' },
}
const members = [
  { id: 1, name: 'Sam Taylor', email: 'sam@example.test', role: 'owner', used: 3, installed: 2 },
  { id: 2, name: 'Alex Morgan', email: 'alex@example.test', role: 'admin', used: 2, installed: 1 },
  { id: 3, name: 'Jo Patel', email: 'jo@example.test', role: 'member', used: 1, installed: 0 },
  { id: 4, name: 'Riley Chen', email: 'riley@example.test', role: 'viewer', used: 0, installed: 0 },
]
const skill = (over = {}) => ({ id: 'a', name: 'code-review', source: 'demo/toolkit', tags: ['review'], lists: [{ id: 1, name: 'Team essentials' }], usedBy: 3, onShelf: 4, installedBy: 3, environmentLinks: 2, members: [{ id: 1, name: 'Sam Taylor', used: true, environments: ['dev-laptop'] }, { id: 2, name: 'Alex Morgan', used: true, environments: ['build-server'] }], ...over })
const rollup = {
  generatedAt: now, individual: true, teamSize: 4, nothingInstalled: 1, members,
  lists: [{ id: 1, name: 'Team essentials' }, { id: 2, name: 'Design reviews' }],
  skills: [
    skill(),
    skill({ id: 'b', name: 'design-review', source: 'demo/design', tags: ['design'], lists: [{ id: 2, name: 'Design reviews' }], usedBy: 1, onShelf: 2, installedBy: 1, environmentLinks: 1, members: [{ id: 1, name: 'Sam Taylor', used: true, environments: ['dev-laptop'] }] }),
    skill({ id: 'c', name: 'pdf', source: 'demo/toolkit', tags: [], lists: [], usedBy: 2, onShelf: 3, installedBy: 2, environmentLinks: 2 }),
    skill({ id: 'd', name: 'unused-skill', source: 'demo/old', tags: ['old'], lists: [{ id: 2, name: 'Design reviews' }], usedBy: 0, onShelf: 1, installedBy: 0, environmentLinks: 0, members: [] }),
  ],
  drift: { unlisted: ['c'], unused: [{ listId: 2, listName: 'Design reviews', skillId: 'd', name: 'unused-skill', source: 'demo/old' }] },
}
const routes = () => ({ '/api/admin/status': adminStatus, '/api/admin/rollup': rollup })
const open = options => openWorkspace(browser, { width: 1440, routes: routes(), ...options })
const panel = page => page.getByRole('region', { name: 'Team usage' })
const count = page => panel(page).getByRole('status').first()

try {
  if (run('counts and filters')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/team`)
    await expect(count(page)).toHaveText('4 skills')
    await expect(page.getByText('4 people', { exact: true })).toBeVisible()
    await expect(page.getByText('1 used by more than half', { exact: true })).toBeVisible()
    await expect(page.getByText('1 in use, on no list', { exact: true })).toBeVisible()
    await expect(page.getByText('1 list skills unused', { exact: true })).toBeVisible()

    await page.getByLabel('Search skills').fill('code')
    await expect(count(page)).toHaveText('1 of 4 skills')
    await expect(page.getByText('code-review', { exact: true })).toBeVisible()
    await expect(page.getByText('design-review', { exact: true })).toHaveCount(0)

    await page.getByLabel('Search skills').fill('nowt')
    await expect(count(page)).toHaveText('0 of 4 skills')
    await expect(page.getByRole('heading', { name: 'No skill matches those filters.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('4 skills')

    await page.getByLabel('List', { exact: true }).selectOption('1')
    await expect(count(page)).toHaveText('1 of 4 skills')
    await page.getByLabel('List', { exact: true }).selectOption('0')
    await page.getByLabel('Tag', { exact: true }).selectOption('design')
    await expect(count(page)).toHaveText('1 of 4 skills')
    await expect(page.getByText('design-review', { exact: true })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('rows and details')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/team`)
    const row = page.locator('.usage-row').filter({ hasText: 'code-review' })
    const metrics = row.locator('.usage-metrics')
    await expect(metrics).toContainText('In use')
    await expect(metrics).toContainText('3 / 4')
    await expect(metrics).toContainText('Saved by')
    await expect(metrics).toContainText('4')
    await expect(metrics).toContainText('Agent reports')
    await expect(metrics).toContainText('2')
    await row.getByText('2 people · 2 agent reports', { exact: true }).click()
    await expect(row.locator('.memberchips').getByText('Sam Taylor', { exact: true })).toBeVisible()
    await expect(row.locator('.memberchips').getByText('Alex Morgan', { exact: true })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/admin/team`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const search = await page.getByLabel('Search skills').boundingBox()
      expect(search.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      await page.getByLabel('Search skills').fill('code')
      await expect(count(page)).toHaveText('1 of 4 skills')
      await expect(page.locator('.usage-row')).toHaveCount(1)
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Admin team usage: one panel, honest count and filters, row details and a phone layout.')
