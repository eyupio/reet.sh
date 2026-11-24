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
const skillHealth = {active:2,work:{},activity:[],machines:hosts.filter(h=>h!=='setup-1').map(host=>({agentId:`${host}-agent`,host,state:host==='work-1'?'installing':host.startsWith('att')?'paused':'ok',waiting:[],failed:[]}))}
const open = options => openWorkspace(browser, { width: 1440, sync, routes:{'/api/v2/sync/skills':skillHealth}, ...options })
const order = async page => (await page.locator('article.machine h3').allTextContents()).map((name) => name.trim())
const status = page => page.getByLabel('Status')

try {
  if (run('sort and filter')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/machines`)
    await expect(page.getByText('7 machines', { exact: true })).toBeVisible()
    expect(await order(page)).toEqual(['work-1', 'att-1', 'att-2', 'setup-1', 'sync-1', 'sync-2', 'sync-3'])
    // Open terminal asks the workspace for a new shell on this machine, never an existing agent session.
    await expect(page.getByRole('article', { name: 'work-1' }).getByRole('link', { name: 'Open terminal' }))
      .toHaveAttribute('href', `/machines?view=terminal&agent=${encodeURIComponent('work-1-agent')}&new=shell`)

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

  // A machine that last connected with a personal API token carries a quiet warning, and a how-to on demand. No other machine does.
  if (run('legacy')) {
    const legacySync = { ...sync, agents: sync.agents.map(a => a.host === 'work-1' ? { ...a, legacyToken: true } : { ...a, legacyToken: false }) }
    for (const [width, theme, touch] of [[1280, 'light', false], [390, 'dark', true]]) {
      const { context, page, errors } = await open({ width, height: 900, theme, touch, scale: 2, sync: legacySync })
      await page.goto(`${base}/machines`)
      const card = host => page.locator('article.machine').filter({ has: page.getByRole('heading', { name: host, exact: true }) })
      await expect(card('work-1').getByText('Connected with a personal API token.')).toBeVisible()
      await expect(page.getByText('Connected with a personal API token.')).toHaveCount(1)
      await expect(card('att-1').locator('[data-legacy-token]')).toHaveCount(0)
      const how = card('work-1').locator('.legacy-how')
      await expect(how.getByText('Run reet agent enroll on it with a new enrolment code, then revoke the old token under Account.')).toBeHidden()
      await how.locator('summary').click()
      await expect(how.getByText('Run reet agent enroll on it with a new enrolment code, then revoke the old token under Account.')).toBeVisible()
      if (touch) expect((await how.locator('summary').boundingBox()).height, 'how-to summary height').toBeGreaterThanOrEqual(44)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      if (process.env.REET_E2E_SHOTS) await card('work-1').screenshot({ path: `${process.env.REET_E2E_SHOTS}/legacy-${width}-${theme}.png` })
      // Its button opens Add a machine, where the code is made.
      await how.getByRole('button', { name: 'Make an enrolment code' }).click()
      const dialog = page.getByRole('dialog', { name: 'Add a machine' })
      await expect(dialog.getByRole('button', { name: 'Make an enrolment code' })).toBeVisible()
      await expect(dialog.getByLabel('Machine name', { exact: true })).toHaveValue('work-1')
      expect(errors).toEqual([])
      await context.close()
    }
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
