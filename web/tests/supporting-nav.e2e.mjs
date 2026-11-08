// The supporting pages share one quiet header: a group label, the menu name as the one h1, and a row of the
// other pages in the same group with the current one marked.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const open = options => openWorkspace(browser, { width: 1440, routes: { '/api/v2/lists': { lists: [], userId: 1, linkEnabled: false } }, ...options })

try {
  // Lists sits in Skills & distribution: the label names the group, the title is the menu name, and the row holds its siblings.
  if (run('lists')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/shared`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: 'Lists' })).toBeVisible()
    await expect(page.locator('.page-heading').getByText('Skills & distribution', { exact: true })).toBeVisible()
    const row = page.getByRole('navigation', { name: 'Skills & distribution' })
    await expect(row.getByRole('link', { name: 'Setups' })).toHaveAttribute('href', '/profiles')
    await expect(row.getByRole('link', { name: 'Lists' })).toHaveAttribute('aria-current', 'page')
    await expect(row.getByRole('link', { name: 'Write a skill' })).toHaveAttribute('href', '/hosting')
    expect(errors).toEqual([])
    await context.close()
  }
} finally {
  await browser.close()
}
console.log('Supporting navigation: the Lists page has the group label, one h1 and the section row.')
