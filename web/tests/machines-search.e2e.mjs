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
} finally {
  await browser.close()
}
console.log('Machines search: host, alias and skill names narrow the fleet with an honest count and a clear way back.')
