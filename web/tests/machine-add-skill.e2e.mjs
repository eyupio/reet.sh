// Machines → Add a skill: the machine card menu opens a picker of shelf skills,
// reuses the existing install intent, and keeps installed skills out of the form.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const installed = skill('release-notes')
const candidate = skill('code-review')
const scope = { id: 'dev-laptop-agents', host: 'dev-laptop', agent: 'agents', name: 'dev-laptop', skills: [installed], skillCount: 1, complete: true, scannedAt: now, lastSeen: now, os: 'linux', arch: 'amd64', version: 'test' }
const machine = { host: 'dev-laptop', groups: [scope], skillCount: 1 }
const state = () => ({ environments: [scope], machines: [machine], agents: [{ id: 'agent-1', host: 'dev-laptop', version: 'test', lastSeen: now, paused: false, autoUpdate: false }], targets: [], jobs: [], runs: [], updateJobs: [] })

const open = options => {
  let desired = null
  let syncRun = null
  const routes = {
    '/api/v2/sync': state(),
    '/api/v2/sync/candidates': { skills: [candidate, installed].map((item) => ({ id: item.id, name: item.name, slug: item.slug, source: item.source, sourceType: item.sourceType })) },
    '/api/v2/sync/desired': request => { desired = request.postDataJSON(); return state() },
    '/api/v2/sync/run': request => { syncRun = request.postDataJSON(); return state() },
  }
  return openWorkspace(browser, { width: 1440, sync: state(), routes, ...options }).then((opened) => ({ ...opened, desired: () => desired, syncRun: () => syncRun }))
}
const laptop = page => page.getByRole('article', { name: 'dev-laptop' })

try {
  if (run('picker and add')) {
    const { context, page, errors, desired, syncRun } = await open({})
    await page.goto(`${base}/machines`)
    await laptop(page).locator('summary[aria-label^="More options"]').click()
    await page.getByRole('menuitem', { name: /^Add a skill/ }).click()
    const dialog = page.getByRole('dialog', { name: 'Add a skill to dev-laptop' })
    await expect(dialog).toBeVisible()
    await expect(dialog.getByText('code-review', { exact: true })).toBeVisible()
    await expect(dialog.getByText('release-notes', { exact: true })).toBeVisible()
    await expect(dialog.getByText('Installed', { exact: true })).toBeVisible()
    await expect(dialog.getByRole('button', { name: 'Add release-notes to dev-laptop' })).toHaveCount(0)

    await dialog.getByRole('button', { name: 'Add code-review to dev-laptop' }).click()
    await expect(dialog).toHaveCount(0)
    await expect(page.getByText('Installing code-review on dev-laptop. Follow it on the card.', { exact: true })).toBeVisible()
    expect(desired()).toEqual({ targetId: 'dev-laptop-agents', sourceId: 'shelf', listId: 0, skillIds: [candidate.id], mode: 'additive', revision: '' })
    expect(syncRun()).toEqual({ agentId: 'agent-1', targetIds: ['dev-laptop-agents'] })
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/machines`)
      await laptop(page).locator('summary[aria-label^="More options"]').click()
      await page.getByRole('menuitem', { name: /^Add a skill/ }).click()
      const dialog = page.getByRole('dialog', { name: 'Add a skill to dev-laptop' })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const search = await dialog.getByLabel('Search skills').boundingBox()
      expect(search.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      const add = await dialog.getByRole('button', { name: 'Add code-review to dev-laptop' }).boundingBox()
      expect(add.height, `add height at ${width}px`).toBeGreaterThanOrEqual(44)
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Machine add a skill: the picker lists shelf skills, marks installed ones, queues the existing install intent and fits a phone.')
