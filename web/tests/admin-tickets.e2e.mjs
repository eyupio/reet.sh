// Admin → Support: the ticket queue in the collection grammar, with a count,
// search, status chips and the existing conversation detail.
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
const tickets = [
  { id: 1, opener: 'Sam Taylor', subject: 'Sync stalls on build-box', status: 'open', messages: 3, updatedAt: now - 6e5, createdAt: now - 8e5, assignee: 'Alex Morgan', assigneeId: 2 },
  { id: 2, opener: 'Jo Patel', groupName: 'Platform engineering', subject: 'Invite link expired', status: 'waiting', messages: 2, updatedAt: now - 9e6, createdAt: now - 1e7, assignee: '', assigneeId: 0 },
  { id: 3, opener: 'Riley Chen', subject: 'Paste strips indentation', status: 'closed', messages: 5, updatedAt: now - 1.7e8, createdAt: now - 2e8, assignee: '', assigneeId: 0 },
]
const messages = [{ id: 1, ticketId: 1, author: 'Sam Taylor', authorKind: 'user', body: 'It stalls on the build machine.', internal: false, createdAt: now - 7e5 }]
const users = [{ id: 1, name: 'Sam Taylor', email: 'sam@example.test', role: 'owner', createdAt: now, tokens: 0 }, { id: 2, name: 'Alex Morgan', email: 'alex@example.test', role: 'admin', createdAt: now, tokens: 0 }]
const routes = () => ({
  '/api/admin/status': adminStatus,
  '/api/v2/admin/users': { users },
  '/api/v2/admin/tickets': { tickets, counts: { open: 1, waiting: 1, closed: 1 } },
  '/api/v2/admin/tickets/1': { ticket: tickets[0], messages },
})
const open = options => openWorkspace(browser, { width: 1440, routes: routes(), ...options })
const panel = page => page.getByRole('region', { name: 'Ticket queue' })
const count = page => panel(page).getByRole('status').first()
const scope = (page, name) => page.getByRole('group', { name: 'Ticket status' }).getByRole('button', { name })

try {
  if (run('counts and filters')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/support`)
    await expect(count(page)).toHaveText('1 of 3 tickets')
    await expect(scope(page, /^Open/)).toHaveText(/Open\s*1/)
    await expect(scope(page, /^Waiting/)).toHaveText(/Waiting\s*1/)
    await expect(scope(page, /^Closed/)).toHaveText(/Closed\s*1/)
    await expect(scope(page, /^All/)).toHaveText(/All\s*3/)

    await scope(page, /^All/).click()
    await expect(count(page)).toHaveText('3 tickets')
    await expect(page.getByText('Sync stalls on build-box', { exact: true })).toBeVisible()
    await expect(page.getByText('Paste strips indentation', { exact: true })).toBeVisible()

    await page.getByLabel('Search tickets').fill('invite')
    await expect(count(page)).toHaveText('1 of 3 tickets')
    await expect(scope(page, /^All/)).toHaveText(/All\s*1/)
    await expect(scope(page, /^Waiting/)).toHaveText(/Waiting\s*1/)
    await expect(page.getByText('Invite link expired', { exact: true })).toBeVisible()
    await expect(page.getByText('Sync stalls on build-box', { exact: true })).toHaveCount(0)

    await scope(page, /^Closed/).click()
    await expect(count(page)).toHaveText('0 of 3 tickets')
    await expect(page.getByRole('heading', { name: 'No ticket matches that search or status.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('3 tickets')
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('detail')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/support`)
    await scope(page, /^All/).click()
    await page.getByText('Sync stalls on build-box', { exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Sync stalls on build-box', exact: true })).toBeVisible()
    await expect(page.getByRole('list', { name: 'Conversation' })).toContainText('It stalls on the build machine.')
    await expect(page.locator('.controls').getByLabel('Status')).toHaveValue('open')
    await expect(page.locator('.controls').getByLabel('Assign to')).toHaveValue('2')
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/admin/support`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const search = await page.getByLabel('Search tickets').boundingBox()
      expect(search.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      for (const name of [/^Open/, /^Waiting/, /^Closed/, /^All/]) {
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
console.log('Admin support: one ticket panel, honest status counts, search, detail and a phone layout.')
