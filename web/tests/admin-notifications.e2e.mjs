// Admin → Notifications: the instance defaults in one panel, grouped by area,
// with Save in the panel head and the existing locked security notice.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const adminStatus = {
  version: 'v1.4.2', commit: 'fixture', db: { path: '/data/reet.db', size: 1024 },
  counts: { all: 0, liked: 0, bookmarked: 0, used: 0 }, stages: { use: 0, try: 0, shelf: 0 },
  environments: [], events: [], skillsSh: true, metaLastOK: '', metaLastError: '',
  topics: { lastSynced: '', lastError: '' },
  settings: { refreshEnabled: true, topicsEnabled: true, linkListsEnabled: false, rollupIndividual: true, pruneServerEnvironmentsDays: 0, instanceName: 'Test' },
}
const defaults = [
  {
    key: 'machine.offline', audience: 'user', group: 'Machines', label: 'A machine goes offline',
    description: 'When a connector stops reporting.', default: { enabled: true, mode: 'immediate', owner: true, admin: true },
  },
  {
    key: 'sync.failed', audience: 'user', group: 'Sync', label: 'A sync fails',
    description: 'When a machine cannot apply a selection.', default: { enabled: true, mode: 'immediate', owner: true, admin: true },
  },
  {
    key: 'ticket.reply', audience: 'staff', group: 'Support', label: 'A ticket gets a reply',
    description: 'When somebody answers a support ticket.', default: { enabled: true, mode: 'immediate', owner: true, admin: true },
  },
  {
    key: 'security.signin', audience: 'user', group: 'Security', label: 'A new sign-in',
    description: 'A new browser or device signs in.', locked: 'Security notices are always sent.', default: { enabled: true, mode: 'immediate', owner: true, admin: true },
  },
]
const makeRoutes = () => {
  let current = defaults.map((item) => ({ ...item, default: { ...item.default } }))
  const saved = []
  return {
    saved,
    routes: {
      '/api/admin/status': adminStatus,
      '/api/v2/admin/notifications/defaults': request => {
        if (request.method() === 'PATCH') {
          const body = request.postDataJSON()
          saved.push(body)
          current = current.map((item) => {
            const change = body.defaults.find((entry) => entry.kind === item.key)
            if (!change) return item
            return { ...item, default: { ...item.default, enabled: change.enabled, mode: change.mode, owner: change.owner, admin: change.admin } }
          })
          return { defaults: current, done: 'Defaults saved.' }
        }
        return { defaults: current }
      },
    },
  }
}
const open = options => {
  const state = makeRoutes()
  return openWorkspace(browser, { width: 1440, routes: state.routes, ...options }).then((opened) => ({ ...opened, saved: state.saved }))
}
const panel = page => page.getByRole('region', { name: 'Notification defaults' })
const count = page => panel(page).getByRole('status').first()
const row = (page, name) => page.locator('.row').filter({ hasText: name })

try {
  if (run('panel and groups')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/notifications`)
    await expect(count(page)).toHaveText('4 notification kinds')
    for (const name of ['Machines', 'Sync', 'Support', 'Security']) {
      await expect(page.getByRole('group', { name })).toBeVisible()
    }
    await expect(page.getByText('Security notices are always sent.', { exact: true })).toBeVisible()
    await expect(row(page, 'A new sign-in').getByRole('checkbox')).toHaveCount(0)
    await expect(row(page, 'A ticket gets a reply').getByRole('checkbox')).toHaveCount(3)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('save')) {
    const { context, page, errors, saved } = await open({})
    await page.goto(`${base}/admin/notifications`)
    const offline = row(page, 'A machine goes offline')
    await offline.getByLabel('Delivery').selectOption('none')
    await offline.getByRole('checkbox', { name: 'On' }).uncheck()
    await panel(page).getByRole('button', { name: 'Save defaults' }).click()
    await expect(page.getByText('Defaults saved.', { exact: true })).toBeVisible()
    expect(saved.length).toBe(1)
    expect(saved[0].defaults.map((entry) => entry.kind)).toEqual(['machine.offline', 'sync.failed', 'ticket.reply'])
    expect(saved[0].defaults[0]).toEqual({ kind: 'machine.offline', enabled: false, mode: 'none', owner: true, admin: true })
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('retry')) {
    let attempts = 0
    const { context, page, errors } = await open({
      routes: {
        ...makeRoutes().routes,
        '/api/v2/admin/notifications/defaults': request => {
          attempts++
          return attempts === 1 ? { status: 500, json: { error: 'The defaults are unavailable.' } } : { defaults }
        },
      },
    })
    await page.goto(`${base}/admin/notifications`)
    await expect(page.getByRole('heading', { name: 'The defaults could not be read.' })).toBeVisible()
    await page.getByRole('button', { name: 'Try again' }).click()
    await expect(count(page)).toHaveText('4 notification kinds')
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/admin/notifications`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const select = row(page, 'A machine goes offline').getByLabel('Delivery')
      const box = await select.boundingBox()
      expect(box.height, `delivery height at ${width}px`).toBeGreaterThanOrEqual(44)
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Admin notifications: one grouped defaults panel, the locked security notice and a phone layout.')
