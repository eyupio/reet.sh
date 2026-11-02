// Admin → Settings: one instance panel, the three immediate settings, and the
// settings blocks only on this page.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const baseStatus = {
  version: 'v1.4.2', commit: 'fixture', db: { path: '/data/reet.db', size: 1024 },
  counts: { all: 0, liked: 0, bookmarked: 0, used: 0 }, stages: { use: 0, try: 0, shelf: 0 },
  environments: [], events: [], skillsSh: true, metaLastOK: '', metaLastError: '',
  topics: { lastSynced: '', lastError: '' },
}
const rollup = { generatedAt: now, individual: true, teamSize: 1, nothingInstalled: 0, members: [], lists: [], skills: [], drift: { unlisted: [], unused: [] } }
const makeState = () => {
  let settings = { refreshEnabled: true, topicsEnabled: true, linkListsEnabled: false, rollupIndividual: false, pruneServerEnvironmentsDays: 30, instanceName: 'Test' }
  let yolo = { allowed: false, changedBy: 'Sam Taylor', changedAt: now }
  let branchPrune = { afterWorktree: false, changedBy: 'Sam Taylor', changedAt: now }
  let mirror = { enabled: false, cached: false }
  const calls = { settings: [], yolo: [], prune: [], mirror: [], cache: 0 }
  const routes = {
    '/api/admin/status': () => ({ ...baseStatus, settings }),
    '/api/admin/settings': request => {
      const body = request.postDataJSON()
      calls.settings.push(body)
      settings = { ...settings, ...body }
      return {}
    },
    '/api/admin/rollup': rollup,
    '/api/v2/settings/yolo': request => {
      if (request.method() === 'PUT') {
        const body = request.postDataJSON()
        calls.yolo.push(body)
        yolo = { ...yolo, allowed: body.allowed }
      }
      return yolo
    },
    '/api/v2/settings/branch-prune': request => {
      if (request.method() === 'PUT') {
        const body = request.postDataJSON()
        calls.prune.push(body)
        branchPrune = { ...branchPrune, afterWorktree: body.afterWorktree }
      }
      return branchPrune
    },
    '/api/v2/settings/software-mirror': request => {
      if (request.method() === 'PUT') {
        const body = request.postDataJSON()
        calls.mirror.push(body)
        mirror = { ...mirror, enabled: body.enabled }
      }
      return mirror
    },
    '/api/v2/settings/software-mirror/cache': () => {
      calls.cache++
      mirror = { ...mirror, cached: true, version: 'v1.4.2', size: 12_345 }
      return mirror
    },
  }
  return { calls, routes }
}
const open = options => {
  const state = makeState()
  return openWorkspace(browser, { width: 1440, routes: state.routes, ...options }).then((opened) => ({ ...opened, calls: state.calls }))
}
const panel = page => page.getByRole('region', { name: 'Instance settings' })
const count = page => panel(page).getByRole('status').first()

try {
  if (run('panel and scope')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/settings`)
    await expect(count(page)).toHaveText('6 settings')
    await expect(page.getByRole('group', { name: 'Background jobs' })).toBeVisible()
    await expect(page.getByRole('group', { name: 'Sharing and identity' })).toBeVisible()
    await expect(panel(page).getByRole('button', { name: 'Save settings' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Approvals' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Repository maintenance' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Software artifact mirror' })).toBeVisible()

    await page.goto(`${base}/admin/activity`)
    await expect(page.getByRole('heading', { name: 'Approvals' })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: 'Repository maintenance' })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: 'Software artifact mirror' })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('save')) {
    const { context, page, errors, calls } = await open({})
    await page.goto(`${base}/admin/settings`)
    await page.getByRole('checkbox', { name: 'Let list owners share read-only links' }).check()
    await page.getByLabel('Instance name').fill('Studio')
    await panel(page).getByRole('button', { name: 'Save settings' }).click()
    await expect(page.getByText('Sorted. Settings saved.', { exact: true })).toBeVisible()
    expect(calls.settings).toHaveLength(1)
    expect(calls.settings[0]).toEqual({
      refreshEnabled: true, topicsEnabled: true, linkListsEnabled: true,
      rollupIndividual: false, pruneServerEnvironmentsDays: 30, instanceName: 'Studio',
    })

    await page.getByRole('checkbox', { name: 'Allow YOLO' }).check()
    await expect(page.getByText('YOLO is allowed for this account.', { exact: true })).toBeVisible()
    expect(calls.yolo).toEqual([{ allowed: true }])

    await page.getByRole('checkbox', { name: 'Prune merged branches after a worktree is removed' }).check()
    await expect(page.getByText('Merged local branches are pruned after a worktree is removed.', { exact: true })).toBeVisible()
    expect(calls.prune).toEqual([{ afterWorktree: true }])

    await page.getByRole('checkbox', { name: 'Serve the reviewed artifact to machines' }).check()
    await expect(page.getByText('Machines may fetch the reviewed artifact from this controller.', { exact: true })).toBeVisible()
    expect(calls.mirror).toEqual([{ enabled: true }])
    await page.getByRole('button', { name: 'Cache the reviewed artifact now' }).click()
    await expect(page.getByText('The reviewed artifact is cached.', { exact: true })).toBeVisible()
    expect(calls.cache).toBe(1)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/admin/settings`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const toggle = page.getByRole('checkbox', { name: 'Six-hourly metadata refresh' }).locator('..')
      const box = await toggle.boundingBox()
      expect(box.height, `toggle height at ${width}px`).toBeGreaterThanOrEqual(44)
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Admin settings: one instance panel, the immediate settings on this page, and a phone layout.')
