// One glossary for skills and sync, and one place for Sync in the navigation.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const now = Date.now()
const one = skill('review')
const scope = { id: 'dev-laptop-agents', host: 'dev-laptop', agent: 'agents', name: 'dev-laptop', skills: [], skillCount: 0, complete: true, scannedAt: now, lastSeen: now, os: 'linux', arch: 'amd64', version: 'test' }
const target = { target: scope, sourceId: 'shelf', sourceName: 'Shelf', mode: 'additive', desired: [one], missing: [one], extra: [], revision: 'rev-1', appliedRevision: 'rev-1', updatedAt: now }
const sync = {
  environments: [scope],
  machines: [{ host: 'dev-laptop', groups: [scope], skillCount: 0 }],
  agents: [{ id: 'agent-1', host: 'dev-laptop', version: 'test', lastSeen: now, paused: false, autoUpdate: false }],
  targets: [target], jobs: [], runs: [], updateJobs: [],
}
let hosted = { slug: 'reet-review', description: 'Reviews code', content: '---\nname: reet-review\ndescription: Reviews code\n---\nReview carefully.', files: {}, published: false, createdAt: now, updatedAt: now }

try {
  if (run('one vocabulary and one Sync place')) {
    const { context, page, errors } = await openWorkspace(browser, {
      width: 1280, skills: [one], sync, hosting: { username: 'sam', prefix: 'reet-', skills: [hosted] },
      routes: {
        '/api/v2/sync/candidates': { skills: [one] },
        '/api/v2/hosting/skills/reet-review': request => { hosted = request.postDataJSON(); return hosted },
      },
    })

    await page.goto(`${base}/sync`)
    await page.locator('details.advanced-selection > summary').click()
    await expect(page.getByLabel('Machine', { exact: true })).toHaveValue('dev-laptop')
    await expect(page.getByText('Install locations · dev-laptop', { exact: true })).toBeVisible()
    await expect(page.getByText('Differences by machine', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Sync plans', exact: true })).toBeVisible()

    const advanced = await page.locator('details.advanced-selection').innerText()
    expect(advanced).not.toMatch(/\b(desired|saved set|scopes?|environments?|targets?|destinations?)\b/i)
    expect(advanced).not.toMatch(/agent groups?/i)

    await page.locator('section[aria-labelledby="saved-sets"] .more-actions > summary').filter({ hasText: 'More actions' }).click()
    await page.getByRole('button', { name: 'Edit sync plan', exact: true }).click()
    await expect(page.getByText('Editing sync plan · revision rev-1', { exact: true })).toBeVisible()

    // Sync belongs to Machines, not More.
    await page.getByRole('button', { name: /^More/ }).first().click()
    const more = page.getByRole('dialog', { name: 'More' })
    await expect(more.getByRole('link', { name: 'Sync', exact: true })).toHaveCount(0)
    await expect(more.getByRole('link', { name: 'Write a skill', exact: true })).toBeVisible()
    await page.keyboard.press('Escape')

    // Writing is writing, and its save notice says what to do next.
    await page.goto(`${base}/hosting`)
    await expect(page.getByRole('heading', { name: 'Write a skill.', exact: true })).toBeVisible()
    await expect(page).toHaveTitle(/Write a skill · reet\.sh/)
    await page.locator('.skill-choice').filter({ hasText: 'reet-review' }).click()
    await expect.poll(() => page.evaluate(() => {
      const launcher = document.querySelector('.gaffer-launcher')?.getBoundingClientRect()
      const dock = document.querySelector('.save-dock')?.getBoundingClientRect()
      return !!launcher && !!dock && dock.bottom <= launcher.top - 10 && launcher.bottom <= innerHeight
    })).toBe(true)
    await page.locator('.save-dock').getByRole('button', { name: 'Save draft', exact: true }).click()
    await expect(page.getByText(/Ready for your machines/)).toBeVisible()
    // With a machine to sync to, the next step is the press itself, not a link to another page (sync-in-place.e2e.mjs follows it).
    await expect(page.getByRole('button', { name: 'Sync all machines', exact: true })).toBeVisible()

    await page.goto(`${base}/skills`)
    await expect(page.getByRole('heading', { name: /Your drafts/ })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }
} finally {
  await browser.close()
}
console.log('Skills and sync vocabulary: one glossary, Sync only under Machines, and a writing flow that names its next step.')
