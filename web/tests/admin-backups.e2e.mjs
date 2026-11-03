// Admin → Backups: the copies and recent runs in the collection grammar, with
// counts, search, honest chips and the existing per-copy actions.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const DAY = 86_400_000
const adminStatus = {
  version: 'v1.4.2', commit: 'fixture', db: { path: '/data/reet.db', size: 1024 },
  counts: { all: 0, liked: 0, bookmarked: 0, used: 0 }, stages: { use: 0, try: 0, shelf: 0 },
  environments: [], events: [], skillsSh: true, metaLastOK: '', metaLastError: '',
  topics: { lastSynced: '', lastError: '' },
  settings: { refreshEnabled: true, topicsEnabled: true, linkListsEnabled: false, rollupIndividual: true, pruneServerEnvironmentsDays: 0, instanceName: 'Test' },
}
const backup = {
  scope: 'instance', enabled: true, dir: '/var/lib/reet/backups',
  key: { fingerprint: 'age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqs', source: 'file', path: '/var/lib/reet/backup.key' },
  settings: { everySeconds: 86_400, keep: 7, destination: 'both', compression: 'gzip', hasPassword: true, nextAt: now + DAY, lastAt: now - DAY, lastState: 'succeeded', lastError: '', updatedAt: now },
  defaults: { everySeconds: 86_400, keep: 7, userEverySeconds: 86_400, userKeep: 7, destination: 'local', compression: 'gzip' },
  remotes: [{
    name: 's3-eu', source: 'database', endpoint: 'https://s3.eu-west-2.amazonaws.com', region: 'eu-west-2',
    bucket: 'reet-backups-production', prefix: 'instance/', accessKey: 'AKIAEXAMPLE', pathStyle: null,
    keep: 14, disabled: false, hasSecret: true, where: 's3://reet-backups-production/instance/', uploading: false, lastUploadAt: now - DAY,
  }],
  runs: [
    { id: 1, scope: 'instance', trigger: 'schedule', state: 'succeeded', startedAt: now - DAY, finishedAt: now - DAY + 5000, bytes: 1_234_567, object: 'reet-b1.tar.age', offsite: 's3-eu', error: '' },
    { id: 2, scope: 'instance', trigger: 'manual', state: 'failed', startedAt: now - 2 * DAY, finishedAt: now - 2 * DAY + 5000, bytes: 0, object: '', offsite: '', error: 'The destination refused the copy.' },
    { id: 3, scope: 'instance', trigger: 'schedule', state: 'running', startedAt: now - 60_000, finishedAt: 0, bytes: 0, object: '', offsite: 's3-eu', error: '' },
  ],
  items: [
    { id: 'b1', kind: 'instance', target: 'reet-b1.tar.age', takenAt: now - DAY, bytes: 1_234_567, version: 'v1.4.2', local: true, remotes: [] },
    { id: 'b2', kind: 'instance', target: 'reet-b2.tar.age', takenAt: now - 2 * DAY, bytes: 2_345_678, version: 'v1.4.2', local: false, remotes: ['s3-eu'] },
    { id: 'b3', kind: 'instance', target: 'reet-b3.tar.age', takenAt: now - 3 * DAY, bytes: 3_456_789, version: 'v1.4.1', local: true, remotes: ['s3-eu', 'r2'] },
  ],
  findings: [{ severity: 'warning', code: 'key_local', message: 'The backup key is stored on the same disk as the backups.' }],
}
const routes = () => ({
  '/api/admin/status': adminStatus,
  '/api/v2/admin/backups': backup,
  '/api/v2/admin/backups/import': { pending: null },
  '/api/v2/admin/backups/items/b1/verify': { ok: false, problems: ['The checksum does not match.'] },
})
const open = options => openWorkspace(browser, { width: 1440, routes: routes(), ...options })
const copies = page => page.getByRole('region', { name: 'Backup copies' })
const runs = page => page.getByRole('region', { name: 'Recent runs' })
const count = panel => panel.getByRole('status').first()
const scope = (page, group, name) => page.getByRole('group', { name: group }).getByRole('button', { name })

try {
  if (run('copies')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/backups`)
    await expect(count(copies(page))).toHaveText('3 copies')
    for (const [name, figure] of [['All', 3], ['This machine', 2], ['Off-site', 2], ['Damaged', 0]]) {
      await expect(scope(page, 'Show copies', new RegExp(`^${name}`))).toHaveText(new RegExp(`${name}\\s*${figure}`))
    }

    await page.getByLabel('Search copies').fill('b2')
    await expect(count(copies(page))).toHaveText('1 of 3 copies')
    await expect(scope(page, 'Show copies', /^Off-site/)).toHaveText(/Off-site\s*1/)
    await expect(scope(page, 'Show copies', /^This machine/)).toHaveText(/This machine\s*0/)
    await expect(page.getByText('b2', { exact: true })).toBeVisible()
    await expect(page.getByText('b1', { exact: true })).toHaveCount(0)

    await page.getByLabel('Search copies').fill('s3-eu')
    await expect(count(copies(page))).toHaveText('2 of 3 copies')
    await page.getByLabel('Search copies').fill('nowt')
    await expect(count(copies(page))).toHaveText('0 of 3 copies')
    await expect(page.getByRole('heading', { name: 'No copy matches that search or filter.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(copies(page))).toHaveText('3 copies')

    await scope(page, 'Show copies', /^This machine/).click()
    await expect(count(copies(page))).toHaveText('2 of 3 copies')
    await expect(page.getByText('b1', { exact: true })).toBeVisible()
    await expect(page.getByText('b2', { exact: true })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('verify')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/backups`)
    const row = copies(page).getByRole('listitem').filter({ hasText: 'b1' })
    await row.getByRole('button', { name: 'Verify' }).click()
    await expect(row.getByText('The checksum does not match.', { exact: true })).toBeVisible()
    await expect(scope(page, 'Show copies', /^Damaged/)).toHaveText(/Damaged\s*1/)
    await scope(page, 'Show copies', /^Damaged/).click()
    await expect(count(copies(page))).toHaveText('1 of 3 copies')
    await expect(page.getByText('b1', { exact: true })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('runs')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/backups`)
    await expect(count(runs(page))).toHaveText('3 runs')
    for (const [name, figure] of [['All', 3], ['Succeeded', 1], ['Failed', 1], ['Running', 1]]) {
      await expect(scope(page, 'Show runs', new RegExp(`^${name}`))).toHaveText(new RegExp(`${name}\\s*${figure}`))
    }
    await page.getByLabel('Search runs').fill('manual')
    await expect(count(runs(page))).toHaveText('1 of 3 runs')
    await expect(page.getByText('The destination refused the copy.', { exact: true })).toBeVisible()
    await page.getByLabel('Search runs').fill('nowt')
    await expect(count(runs(page))).toHaveText('0 of 3 runs')
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(runs(page))).toHaveText('3 runs')
    await scope(page, 'Show runs', /^Failed/).click()
    await expect(count(runs(page))).toHaveText('1 of 3 runs')
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/admin/backups`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      for (const label of ['Search copies', 'Search runs']) {
        const box = await page.getByLabel(label).boundingBox()
        expect(box.height, `${label} height at ${width}px`).toBeGreaterThanOrEqual(44)
      }
      for (const name of [/^All/, /^This machine/, /^Off-site/, /^Damaged/]) {
        const box = await scope(page, 'Show copies', name).boundingBox()
        expect(box.height, `copy chip height at ${width}px`).toBeGreaterThanOrEqual(44)
      }
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Admin backups: copies and runs in one grammar, honest chips, search, verify and a phone layout.')
