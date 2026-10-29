// Sync activity on a phone: /sync owns the detail, so no dock covers it; elsewhere the dock is a compact bar.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const open = options => openWorkspace(browser, options)
const now = Date.now()
const skills = [
  { id: 'demo/repo/review', name: 'review', slug: 'review', source: 'demo/repo', sourceType: 'github' },
  { id: 'demo/repo/test', name: 'test', slug: 'test', source: 'demo/repo', sourceType: 'github' },
]
const scope = (id, agent) => ({ id, host: 'Elitebook', agent, name: 'Elitebook', skills, skillCount: 2, complete: true, scannedAt: now, lastSeen: now, os: 'linux', arch: 'amd64', version: 'test' })
const groups = [scope('opencode', 'opencode'), scope('shared', 'agents')]
const target = (id, agent) => ({ target: { ...scope(id, agent) }, sourceId: 'shelf', sourceName: 'Shelf', desired: skills, missing: skills, extra: [], revision: 'r1', appliedRevision: '', updatedAt: now, mode: 'mirror' })
const failed = (id, kind, targetId, message, updatedAt) => ({ id, agentId: 'a1', targetId, kind, status: 'failed', revision: 'r1', hash: '', plan: {}, message, stage: '', stages: [], createdAt: updatedAt - 1000, updatedAt })
const sync = {
  environments: groups,
  machines: [{ host: 'Elitebook', groups, skillCount: 2 }],
  agents: [{ id: 'a1', host: 'Elitebook', version: '1.2.3', lastSeen: now, paused: false, autoUpdate: false }],
  targets: [target('opencode', 'opencode'), target('shared', 'agents')],
  runs: [],
  updateJobs: [],
  jobs: [
    failed('p1', 'preview', 'opencode', 'cannot read target skills directory: open /home/dev/.config/opencode/skills: no such file or directory', now - 9 * 3600000),
    failed('p2', 'preview', 'shared', 'cannot read target skills directory: open /home/dev/.config/opencode/skills: no such file or directory', now - 9 * 3600000),
    failed('a1', 'apply', 'opencode', 'remote apply disabled locally; run reet agent allow-apply on in a terminal on this machine to opt in', now - 6 * 3600000),
  ],
}
const queued = {
  ...sync,
  jobs: [{ id: 'q1', agentId: 'a1', targetId: 'opencode', kind: 'preview', status: 'queued', revision: 'r1', hash: '', plan: {}, message: '', stage: '', stages: [], createdAt: now, updatedAt: now }],
}

try {
  const detail = await open({ width: 390, height: 844, touch: true, sync })
  await detail.page.goto(`${base}/sync#sync-jobs`)
  await expect(detail.page.locator('.sync-dock')).toHaveCount(0)
  await expect(detail.page.getByRole('heading', { name: 'Sync needs attention on Elitebook', exact: true }).first()).toBeVisible()
  await expect(detail.page.locator('#sync-jobs-title')).toHaveText('Sync needs attention on Elitebook')
  await expect(detail.page.locator('#sync-jobs-title')).toBeInViewport()
  await expect(detail.page.getByText('2 jobs failed on Elitebook. Open one to see the exact fix.')).toBeVisible()
  const failedPreview = detail.page.locator('.activity-hosts .activity-job').filter({ hasText: 'Preview failed' })
  await expect(failedPreview).toHaveCount(1)
  await failedPreview.locator('summary').click()
  // The later apply on OpenCode supersedes its earlier preview failure.
  await expect(detail.page.getByText('Earlier activity (1)')).toBeVisible()
  await expect(detail.page.getByText('Skills folder missing')).toBeVisible()
  expect(await detail.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(detail.errors).toEqual([])
  await detail.context.close()

  const elsewhere = await open({ width: 390, height: 844, touch: true, sync: queued })
  await elsewhere.page.goto(`${base}/machines`)
  const dock = elsewhere.page.locator('.sync-dock')
  await expect(dock).toBeVisible()
  await expect(dock.getByText('Queued on Elitebook')).toBeVisible()
  const box = await dock.boundingBox()
  expect(box.height, 'collapsed dock height').toBeLessThanOrEqual(96)
  await dock.locator('.dock-main').click()
  await expect(dock).toHaveClass(/expanded/)
  await expect(dock.getByRole('link', { name: 'View all activity' })).toBeVisible()
  await dock.getByRole('button', { name: 'Hide sync activity' }).click()
  await expect(dock).toHaveCount(0)
  expect(await elsewhere.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(elsewhere.errors).toEqual([])
  await elsewhere.context.close()
  console.log('Sync activity: duplicate failures grouped on /sync, compact dock elsewhere, both phone-clean.')
} finally {
  await browser.close()
}
