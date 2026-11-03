// Machines → Sync: a Remove link on an installed badge opens the exact-set review
// with the skill unticked, but no files change until the person saves and approves.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const a = skill('code-review'), b = skill('release-notes')
const scope = { id: 'dev-laptop-agents', host: 'dev-laptop', agent: 'agents', name: 'dev-laptop', skills: [a, b], skillCount: 2, complete: true, scannedAt: now, lastSeen: now, os: 'linux', arch: 'amd64', version: 'test' }
const machine = { host: 'dev-laptop', groups: [scope], skillCount: 2 }
const target = { target: scope, sourceId: 'shelf', sourceName: 'Shelf', desired: [a, b], missing: [], extra: [], revision: 'r1', updatedAt: now, mode: 'additive' }
const sync = { environments: [scope],machines: [machine],agents: [{ id: 'agent-1', host: 'dev-laptop', version: 'test', lastSeen: now, paused: false, autoUpdate: false }], targets: [target], jobs: [], runs: [], updateJobs: [] }
const candidate = item => ({ id: item.id, name: item.name, slug: item.slug, source: item.source, sourceType: item.sourceType })

try {
  if (run('prepared removal')) {
    const writes = []
    const { context, page, errors } = await openWorkspace(browser, {
      width: 1440,
      sync,
      routes: {
        '/api/v2/sync': request => { if (request.method() !== 'GET') writes.push({ path: '/api/v2/sync', body: request.postDataJSON() }); return sync },
        '/api/v2/sync/candidates': { skills: [candidate(a), candidate(b)] },
        '/api/v2/sync/desired': request => { writes.push({ path: '/api/v2/sync/desired', body: request.postDataJSON() }); return sync },
        '/api/v2/sync/run': request => { writes.push({ path: '/api/v2/sync/run', body: request.postDataJSON() }); return sync },
      },
    })
    await page.goto(`${base}/sync?machine=dev-laptop&remove=${encodeURIComponent(a.id)}`)
    await expect(page.getByText('Removal prepared. Review the exact set below, then save; no files change until you preview and approve.', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Choose what to sync' })).toBeVisible()
    await expect(page.getByLabel('Sync mode')).toHaveValue('mirror')
    const row = page.locator('.skill-options .skill').filter({ hasText: 'code-review' })
    await expect(row.getByRole('checkbox')).not.toBeChecked()
    const kept = page.locator('.skill-options .skill').filter({ hasText: 'release-notes' })
    await expect(kept.getByRole('checkbox')).toBeChecked()
    expect(writes).toEqual([])
    expect(errors).toEqual([])
    await context.close()
  }
  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await openWorkspace(browser, {
        width, height: 812, touch: true, sync,
        routes: {
          '/api/v2/sync': sync,
          '/api/v2/sync/candidates': { skills: [candidate(a), candidate(b)] },
        },
      })
      await page.goto(`${base}/sync?machine=dev-laptop&remove=${encodeURIComponent(a.id)}`)
      await expect(page.getByText('Removal prepared. Review the exact set below, then save; no files change until you preview and approve.', { exact: true })).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      await expect(page.getByLabel('Sync mode')).toHaveValue('mirror')
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Sync removal: a Remove link prepares the exact-set review without changing files.')
