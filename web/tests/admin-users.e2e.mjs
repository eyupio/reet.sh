// Admin → Users: the people list in the shared collection grammar, with a count,
// search, role chips and the management actions still on each person's row.
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
const users = [
  { id: 1, name: 'Sam Taylor', email: 'sam@example.test', role: 'owner', createdAt: now, tokens: 0, lastActive: now - 60_000 },
  { id: 2, name: 'Alex Morgan', email: 'alex@example.test', role: 'admin', createdAt: now, tokens: 2, lastActive: now - 3_600_000 },
  { id: 3, name: 'Jo Patel', email: 'jo@example.test', role: 'member', createdAt: now, tokens: 0, lastActive: 0 },
  { id: 4, name: 'Riley Chen', email: 'riley@example.test', role: 'viewer', createdAt: now, tokens: 1, disabledAt: now - 86_400_000, lastActive: now - 172_800_000 },
]
const makeRoutes = () => {
  let current = users.map(user => ({ ...user }))
  return {
    '/api/admin/status': adminStatus,
    '/api/v2/admin/users': request => {
      if (request.method() !== 'GET') {
        const body = request.postDataJSON()
        current = current.map(user => user.id === body.id ? {
          ...user,
          ...(body.role ? { role: body.role } : {}),
          ...(body.disabled === undefined ? {} : { disabledAt: body.disabled ? Date.now() : undefined }),
        } : user)
      }
      return { users: current }
    },
    '/api/v2/admin/sso': { enabled: false },
  }
}
const open = options => openWorkspace(browser, { width: 1440, routes: makeRoutes(), ...options })
const panel = page => page.getByRole('region', { name: 'Users' })
const count = page => panel(page).getByRole('status').first()
const scope = (page, name) => page.getByRole('group', { name: 'Show people' }).getByRole('button', { name })

try {
  if (run('counts and search')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/users`)
    await expect(count(page)).toHaveText('4 people')
    await expect(scope(page, /^All/)).toHaveText(/All\s*4/)
    await expect(scope(page, /^Owners/)).toHaveText(/Owners\s*1/)
    await expect(scope(page, /^Admins/)).toHaveText(/Admins\s*1/)
    await expect(scope(page, /^Members/)).toHaveText(/Members\s*1/)
    await expect(scope(page, /^Viewers/)).toHaveText(/Viewers\s*1/)
    await expect(page.getByText('you', { exact: true })).toBeVisible()
    await expect(page.getByText('Disabled', { exact: true })).toBeVisible()

    await page.getByLabel('Search people').fill('alex')
    await expect(count(page)).toHaveText('1 of 4 people')
    await expect(scope(page, /^All/)).toHaveText(/All\s*1/)
    await expect(scope(page, /^Admins/)).toHaveText(/Admins\s*1/)
    await expect(page.getByText('Alex Morgan', { exact: true })).toBeVisible()
    await expect(page.getByText('Jo Patel', { exact: true })).toHaveCount(0)

    await page.getByLabel('Search people').fill('nowt')
    await expect(count(page)).toHaveText('0 of 4 people')
    await expect(page.getByRole('heading', { name: 'No person matches that search or role.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('4 people')

    await scope(page, /^Viewers/).click()
    await expect(count(page)).toHaveText('1 of 4 people')
    await expect(page.getByText('Riley Chen', { exact: true })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('row actions')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/users`)
    await expect(page.getByLabel('Instance role for Alex Morgan')).toHaveValue('admin')
    await page.getByLabel('Instance role for Jo Patel').selectOption('viewer')
    await expect(page.getByLabel('Instance role for Jo Patel')).toHaveValue('viewer')

    await page.getByText('Manage Alex Morgan', { exact: true }).click()
    for (const name of ['Disable', 'Reset password', 'Revoke tokens', 'Make owner', 'Delete']) {
      await expect(page.getByRole('button', { name, exact: true })).toBeVisible()
    }
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/admin/users`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const search = await page.getByLabel('Search people').boundingBox()
      expect(search.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      for (const name of [/^All/, /^Owners/, /^Admins/, /^Members/, /^Viewers/]) {
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
console.log('Admin users: one panel, honest role counts, search and row actions on a phone.')
