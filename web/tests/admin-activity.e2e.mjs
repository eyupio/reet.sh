// Admin → Activity: the audit list in the collection grammar, with a count,
// search, area chips and one row per event.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const events = [
  { id: 1, at: now - 60_000, actor: 'Sam Taylor', source: 'ui', action: 'ticket-open', detail: '{"ticket":41,"subject":"Sync failed"}' },
  { id: 2, at: now - 3_600_000, actor: 'Sam Taylor', source: 'ui', action: 'add', skillName: 'code-review' },
  { id: 3, at: now - 7_200_000, actor: 'Sam Taylor', source: 'ui', action: 'profile', detail: '{"defaultTool":"claude"}' },
  { id: 4, at: now - 10_800_000, actor: 'Alex Morgan', source: 'ui', action: 'project-create', detail: '{"name":"Site","members":2}' },
  { id: 5, at: now - 14_400_000, actor: 'Sam Taylor', source: 'ui', action: 'mail-check', detail: '{"host":"smtp.example.test","ok":false}' },
  { id: 6, at: now - 18_000_000, actor: 'Sam Taylor', source: 'ui', action: 'yolo-allowed' },
]
const adminStatus = {
  version: 'v1.4.2', commit: 'fixture', db: { path: '/data/reet.db', size: 1024 },
  counts: { all: 0, liked: 0, bookmarked: 0, used: 0 }, stages: { use: 0, try: 0, shelf: 0 },
  environments: [], events, skillsSh: true, metaLastOK: '', metaLastError: '',
  topics: { lastSynced: '', lastError: '' },
  settings: { refreshEnabled: true, topicsEnabled: true, linkListsEnabled: false, rollupIndividual: true, pruneServerEnvironmentsDays: 0, instanceName: 'Test' },
}
const open = options => openWorkspace(browser, { width: 1440, routes: { '/api/admin/status': adminStatus }, ...options })
const panel = page => page.getByRole('region', { name: 'Recent activity' })
const count = page => panel(page).getByRole('status').first()
const scope = (page, name) => page.getByRole('group', { name: 'Show activity' }).getByRole('button', { name })

try {
  if (run('counts and search')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/activity`)
    await expect(count(page)).toHaveText('6 events')
    for (const [name, figure] of [['All', 6], ['Agents', 0], ['Projects', 1], ['Skills', 1], ['Lists', 0], ['Account', 1], ['Admin', 3]]) {
      await expect(scope(page, new RegExp(`^${name}`))).toHaveText(new RegExp(`${name}\\s*${figure}`))
    }

    await page.getByLabel('Search activity').fill('ticket')
    await expect(count(page)).toHaveText('1 of 6 events')
    await expect(scope(page, /^Admin/)).toHaveText(/Admin\s*1/)
    await expect(scope(page, /^Skills/)).toHaveText(/Skills\s*0/)
    await expect(page.getByText('Opened support ticket #41: Sync failed', { exact: true })).toBeVisible()
    await expect(page.getByText('Added code-review', { exact: true })).toHaveCount(0)

    await page.getByLabel('Search activity').fill('claude')
    await expect(count(page)).toHaveText('1 of 6 events')
    await expect(page.getByText('Changed the default agent to Claude Code', { exact: true })).toBeVisible()

    await page.getByLabel('Search activity').fill('nowt')
    await expect(count(page)).toHaveText('0 of 6 events')
    await expect(page.getByRole('heading', { name: 'No event matches that search or area.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('6 events')

    await scope(page, /^Admin/).click()
    await expect(count(page)).toHaveText('3 of 6 events')
    await expect(page.getByText('Opened support ticket #41: Sync failed', { exact: true })).toBeVisible()
    await expect(page.getByText('Failed to check the mail server (smtp.example.test)', { exact: true })).toBeVisible()
    await expect(page.getByText('Allowed YOLO sessions for the account', { exact: true })).toBeVisible()
    await expect(page.getByText('Added code-review', { exact: true })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/admin/activity`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const search = await page.getByLabel('Search activity').boundingBox()
      expect(search.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      for (const name of [/^All/, /^Agents/, /^Projects/, /^Skills/, /^Lists/, /^Account/, /^Admin/]) {
        const box = await scope(page, name).boundingBox()
        expect(box.height, `scope height at ${width}px`).toBeGreaterThanOrEqual(44)
      }
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Admin activity: one audit panel, honest area counts, search and a phone layout.')
