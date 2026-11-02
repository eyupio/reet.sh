// Setups is a master-detail page: the saved profiles now sit in the same collection panel as the
// other directory pages, with a count, a search and honest recency, while the detail keeps its work.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const names = ['laptop-claude', 'build-server', 'workstation', 'pi-minimal']
const profiles = names.map((name, i) => ({ name, revision: 2, snapshotId: `snap-${name}`, snapshotDigest: 'sha256:' + String(i).repeat(64), updatedAt: now - i * 86_400_000 }))
const record = name => ({
  schema_version: 1,
  name,
  revisions: [{
    revision: 2,
    created_at: new Date(now).toISOString(),
    snapshot_id: `snap-${name}`,
    snapshot_digest: 'sha256:' + 'a'.repeat(64),
    snapshot_taken_at: new Date(now).toISOString(),
    preview: { schema_version: 1, os: 'linux', architecture: 'amd64', agents: [], skills: [], memories: [], classes: [], credentials_included: false },
  }],
})
const routes = () => ({
  '/api/v2/setup/profiles': { profiles },
  ...Object.fromEntries(names.map(name => [`/api/v2/setup/profiles/${name}`, record(name)])),
})
const open = options => openWorkspace(browser, { width: 1440, routes: routes(), ...options })
const panel = page => page.getByRole('region', { name: 'Setup profiles' })
const count = page => panel(page).getByRole('status').first()
const row = (page, name) => panel(page).getByRole('button', { name: new RegExp(`^${name}`) })

try {
  if (run('count and search')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/profiles`)
    await expect(count(page)).toHaveText('4 setups')
    await expect(row(page, 'laptop-claude')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'laptop-claude', exact: true })).toBeVisible()
    await expect(page.getByText('Latest revision 2 · 1 kept', { exact: true })).toBeVisible()

    await page.getByLabel('Search setups').fill('build')
    await expect(count(page)).toHaveText('1 of 4 setups')
    await expect(row(page, 'build-server')).toBeVisible()
    await expect(panel(page).getByRole('button')).toHaveCount(1)

    await page.getByLabel('Search setups').fill('nowt')
    await expect(count(page)).toHaveText('0 of 4 setups')
    await expect(page.getByText('No setup matches “nowt”.')).toBeVisible()
    await page.getByRole('button', { name: 'Clear search' }).click()
    await expect(count(page)).toHaveText('4 setups')
    await expect(panel(page).getByRole('button')).toHaveCount(4)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('choosing a profile')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/profiles`)
    await expect(row(page, 'laptop-claude')).toHaveAttribute('aria-pressed', 'true')
    await row(page, 'build-server').click()
    await expect(row(page, 'build-server')).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByRole('heading', { name: 'build-server', exact: true })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/profiles`)
      const box = await page.getByLabel('Search setups').boundingBox()
      expect(box, `search at ${width}px`).not.toBeNull()
      expect(box.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      await page.getByLabel('Search setups').fill('pi')
      await expect(count(page)).toHaveText('1 of 4 setups')
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Setups: profiles in one panel, a count and search, honest selection and a phone layout.')
