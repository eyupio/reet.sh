// Machines can be found by host, saved alias or a skill any scope reported, without leaving the page.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const sk = id => ({ id: `demo/toolkit/${id}`, name: id, slug: id, source: 'demo/toolkit', sourceType: 'github' })
const group = (id, host, skills) => ({ id, host, agent: 'agents', name: host, skills, skillCount: skills.length, complete: true, scannedAt: now, lastSeen: now, os: 'linux', arch: 'amd64', version: 'test' })
const dev = group('dev-agents', 'dev-laptop', [sk('code-review'), sk('testing')])
const build = group('build-agents', 'build-server', [sk('deployment')])
const sync = {
  environments: [dev, build],
  machines: [
    { host: 'dev-laptop', aliases: ['old-laptop'], groups: [dev], skillCount: 2 },
    { host: 'build-server', groups: [build], skillCount: 1 },
  ],
  agents: [
    { id: 'a', host: 'dev-laptop', version: 'test', lastSeen: now, paused: false, autoUpdate: false },
    { id: 'b', host: 'build-server', version: 'test', lastSeen: now, paused: false, autoUpdate: false },
  ],
  targets: [], jobs: [], runs: [], updateJobs: [], releases: { latest: { version: 'v1.4.2' } },
}
const open = options => openWorkspace(browser, { width: 1440, sync, inventory: { environments: sync.environments, machines: sync.machines }, ...options })
const fleet = page => page.getByRole('region', { name: 'Machines' })
const count = page => fleet(page).locator('.fleet-count')

try {
  if (run('by skill, host and alias')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/machines`)
    await expect(count(page)).toHaveText('2 machines')
    const search = page.getByLabel('Search machines')

    await search.fill('deployment')
    await expect(count(page)).toHaveText('1 of 2 machines')
    await expect(page.getByRole('article', { name: 'build-server' })).toBeVisible()
    await expect(page.getByRole('article', { name: 'dev-laptop' })).toHaveCount(0)

    await search.fill('code-review')
    await expect(count(page)).toHaveText('1 of 2 machines')
    await expect(page.getByRole('article', { name: 'dev-laptop' })).toBeVisible()
    await expect(page.getByRole('article', { name: 'build-server' })).toHaveCount(0)

    await search.fill('old-laptop')
    await expect(count(page)).toHaveText('1 of 2 machines')
    await expect(page.getByRole('article', { name: 'dev-laptop' })).toBeVisible()

    await search.fill('nowt')
    await expect(count(page)).toHaveText('0 of 2 machines')
    await expect(page.getByText('No machine here matches “nowt”.')).toBeVisible()
    await page.getByRole('button', { name: 'Clear search' }).click()
    await expect(count(page)).toHaveText('2 machines')
    await expect(page.getByRole('article')).toHaveCount(2)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/machines`)
      const box = await page.getByLabel('Search machines').boundingBox()
      expect(box, `search at ${width}px`).not.toBeNull()
      expect(box.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      await page.getByLabel('Search machines').fill('deployment')
      await expect(count(page)).toHaveText('1 of 2 machines')
      expect(errors).toEqual([])
      await context.close()
    }
  }
  if (run('action feedback')) {
    for (const width of [1440, 390]) {
      let fail = false
      const active = { ...sync, jobs: [{ id: 'scan-b', agentId: 'b', kind: 'scan', status: 'queued', createdAt: now, updatedAt: now }] }
      const { context, page, errors } = await open({ width, routes: {
        '/api/v2/sync/jobs': () => fail ? { status: 409, json: { error: 'Machine is unavailable' } } : active,
        '/api/v2/sync': () => active,
      } })
      await page.goto(`${base}/machines`)
      const machine = page.getByRole('article', { name: 'build-server', exact: true })
      const scan = async () => {
        await machine.getByLabel('More options for build-server').click()
        await machine.getByRole('menuitem', { name: 'Rescan machine' }).click()
      }
      await machine.scrollIntoViewIfNeeded()
      await scan()
      const position = await page.evaluate(() => scrollY)
      await expect(page.locator('.toast.ok')).toContainText('Scan queued')
      const dock = page.locator('.sync-dock')
      const clearOfDock = async () => page.evaluate(() => {
        const toast = document.querySelector('.toast'), dock = document.querySelector('.sync-dock')
        if (!toast || !dock) return false
        const t = toast.getBoundingClientRect(), d = dock.getBoundingClientRect()
        const button = toast.querySelector('button.close'), b = button.getBoundingClientRect()
        return t.top >= 0 && t.bottom <= d.top - 8 && button.contains(document.elementFromPoint(b.x + b.width / 2, b.y + b.height / 2))
      })
      await expect(dock).toBeVisible()
      await expect.poll(clearOfDock).toBe(true)
      await dock.locator('.dock-main').click()
      await expect(dock).toHaveClass(/expanded/)
      await expect.poll(clearOfDock).toBe(true)
      await expect.poll(() => page.evaluate(() => scrollY)).toBe(position)
      await expect(page.locator('.banners .notice')).toHaveCount(0)
      // Replacing a toast during its exit animation must not dismiss its successor.
      await page.getByRole('button', { name: 'Dismiss', exact: true }).click()
      fail = true
      await scan()
      await page.waitForTimeout(250)
      await expect(page.locator('.toast.bad')).toContainText('Machine is unavailable')
      await expect(page.locator('.toast.bad')).toHaveAttribute('role', 'alert')
      await expect.poll(clearOfDock).toBe(true)
      await dock.getByRole('button', { name: 'Minimise sync activity' }).click()
      await expect.poll(clearOfDock).toBe(true)
      await dock.getByRole('button', { name: 'Hide sync activity' }).click()
      await expect.poll(() => page.evaluate(() => document.documentElement.style.getPropertyValue('--sync-dock-clearance'))).toBe('')
      await expect(machine).toBeInViewport()
      expect(errors).toEqual([])
      await context.close()
    }
  }

  if (run('upgrade progress')) {
    for (const width of [1440, 390]) {
      const upgradeSync = { ...sync, agents: sync.agents.map(agent => ({ ...agent, version: 'v1.0.0' })) }
      const queued = { ...upgradeSync, updateJobs: [{ id: 'upgrade-a', agentId: 'a', kind: 'update', status: 'queued', revision: 'latest', createdAt: now, updatedAt: now }] }
      const { context, page, errors } = await open({ width, sync: upgradeSync, routes: { '/api/v2/sync/agents/update': queued } })
      await page.goto(`${base}/machines`)
      const panel = page.locator('[data-agent-update="a"]')
      await panel.getByRole('button', { name: /Review update/ }).click()
      await page.getByRole('dialog').getByRole('button', { name: /Update to/ }).click()
      await expect(page.locator('.toast')).toContainText('queued for dev-laptop')
      await expect(panel).toContainText('Update queued')
      await expect(panel).toBeInViewport()
      await expect(page.getByRole('status').filter({ hasText: 'queued for dev-laptop' }).filter({ hasNot: page.getByRole('button', { name: 'Dismiss' }) })).toHaveCount(0)
      await page.evaluate(() => window.scrollTo(0, 0))
      await page.getByRole('button', { name: 'View progress' }).click()
      await expect(panel).toBeInViewport()
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Machines search: host, alias and skill names narrow the fleet with an honest count and a clear way back.')
