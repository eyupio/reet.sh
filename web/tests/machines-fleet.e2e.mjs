// Machines: a large fleet sorts by attention, and a compact status filter appears
// only when the fleet is big enough to need it.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const scope = (host, over = {}) => ({ id: `${host}-agents`, host, agent: 'agents', name: host, skills: [], skillCount: 0, complete: true, scannedAt: now, lastSeen: now, os: 'linux', arch: 'amd64', version: 'test', ...over })
const hosts = ['work-1', 'att-1', 'att-2', 'setup-1', 'sync-1', 'sync-2', 'sync-3']
const environments = [
  scope('work-1'),
  scope('att-1'),
  scope('att-2', { scannedAt: now - 15 * 86400000 }),
  scope('setup-1'),
  scope('sync-1'), scope('sync-2'), scope('sync-3'),
]
const machines = hosts.map((host) => ({ host, groups: environments.filter((env) => env.host === host), skillCount: 0 }))
const agent = (host, over = {}) => ({ id: `${host}-agent`, host, version: 'test', lastSeen: now, paused: false, autoUpdate: false, ...over })
const target = (env, over = {}) => ({ target: env, sourceId: 'shelf', sourceName: 'Shelf', desired: [], missing: [], extra: [], revision: 'r1', appliedRevision: 'r1', updatedAt: now, mode: 'additive', ...over })
const sync = {
  environments,
  machines,
  agents: [agent('work-1'), agent('att-1', { paused: true }), agent('att-2'), agent('sync-1'), agent('sync-2'), agent('sync-3')],
  targets: [
    target(environments[0]),
    target(environments[2]),
    target(environments[4]), target(environments[5]), target(environments[6]),
  ],
  jobs: [{ id: 'job-1', agentId: 'work-1-agent', targetId: 'work-1-agents', kind: 'apply', status: 'running', revision: 'r1', hash: '', plan: {}, message: '', createdAt: now, updatedAt: now }],
  runs: [],
  updateJobs: [],
}
const open = options => openWorkspace(browser, { width: 1440, sync, ...options })
const order = async page => (await page.locator('article.machine h3').allTextContents()).map((name) => name.trim())
const status = page => page.getByLabel('Status')

try {
  if (run('sort and filter')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/machines`)
    await expect(page.getByText('7 machines', { exact: true })).toBeVisible()
    expect(await order(page)).toEqual(['work-1', 'att-1', 'att-2', 'setup-1', 'sync-1', 'sync-2', 'sync-3'])

    await status(page).selectOption('attention')
    await expect(page.locator('.fleet-count')).toHaveText('3 of 7 machines')
    expect(await order(page)).toEqual(['att-1', 'att-2', 'setup-1'])
    await expect(page.getByRole('article', { name: 'work-1' })).toHaveCount(0)

    await status(page).selectOption('working')
    await expect(page.locator('.fleet-count')).toHaveText('1 of 7 machines')
    expect(await order(page)).toEqual(['work-1'])

    await status(page).selectOption('synced')
    await expect(page.locator('.fleet-count')).toHaveText('3 of 7 machines')
    expect(await order(page)).toEqual(['sync-1', 'sync-2', 'sync-3'])

    await page.getByLabel('Search machines').fill('att')
    await expect(page.getByText('No machine here matches “att”.', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(page.locator('.fleet-count')).toHaveText('7 machines')
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/machines`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const box = await status(page).boundingBox()
      expect(box.height, `status height at ${width}px`).toBeGreaterThanOrEqual(44)
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Machines fleet: attention order, compact status filter, empty recovery and a phone layout.')
