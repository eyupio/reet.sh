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

  // Community: the label names its group, the title is the menu name, and the row holds Support beside it when the support desk is on.
  if (run('community')) {
    const { context, page, errors } = await open({ routes: { '/api/community': { lists: [], skills: [] } } })
    await page.goto(`${base}/community`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: 'Community' })).toBeVisible()
    await expect(page.locator('.page-heading').getByText('Help & community', { exact: true })).toBeVisible()
    const row = page.getByRole('navigation', { name: 'Help & community' })
    await expect(row.getByRole('link', { name: 'Community' })).toHaveAttribute('aria-current', 'page')
    await expect(row.getByRole('link', { name: 'Support' })).toHaveAttribute('href', '/support')
    expect(errors).toEqual([])
    await context.close()
  }

  // Signed out, Community still names its group (the page is public); there is no sign-in to give a second page, so no row is needed.
  if (run('community signed out')) {
    const { context, page, errors } = await open({ status: { user: null }, routes: { '/api/community': { lists: [], skills: [] } } })
    await page.goto(`${base}/community`)
    await expect(page.getByRole('heading', { level: 1, name: 'Community' })).toBeVisible()
    await expect(page.locator('.page-heading').getByText('Help & community', { exact: true })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  // Notifications is not in the More list; it names the Account section itself.
  if (run('notifications')) {
    const { context, page, errors } = await open({ routes: { '/api/v2/notifications': { notifications: [], unread: 0 }, '/api/v2/notifications/preferences': { preferences: [] } } })
    await page.goto(`${base}/notifications`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: 'Notifications' })).toBeVisible()
    await expect(page.locator('.page-heading').getByText('Account', { exact: true })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  // A list's own page: the label links back to Lists (a 44px target with a visible focus ring), the visibility stays on the page, and there is no row.
  if (run('list detail')) {
    const detail = { id: 7, ownerId: 1, ownerName: 'Sam', name: 'Launch checklist', description: 'Everything for a release week.', visibility: 'public', slug: 'launch-checklist', role: 'owner', skillCount: 0, memberCount: 1, createdAt: Date.now(), updatedAt: Date.now(), skills: [], members: [], linkEnabled: false, canEdit: true, canManage: true, canTransfer: true }
    const { context, page, errors } = await open({ routes: { '/api/v2/lists/7': detail } })
    await page.goto(`${base}/lists/7`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: 'Launch checklist' })).toBeVisible()
    const head = page.locator('.page-head')
    const back = head.getByRole('link', { name: 'Lists', exact: true })
    await expect(back).toHaveAttribute('href', '/shared')
    await expect(head.getByText('Public community', { exact: true })).toBeVisible()
    await expect(head.getByRole('navigation')).toHaveCount(0)
    const box = await back.boundingBox()
    expect(box.height, 'label link height').toBeGreaterThanOrEqual(44)
    expect(box.width, 'label link width').toBeGreaterThanOrEqual(44)
    await expect(back).toHaveCSS('text-decoration-line', 'underline')
    await back.focus()
    await expect(back).toBeFocused()
    await expect(back).toHaveCSS('outline-style', 'solid')
    await expect(back).toHaveCSS('outline-width', '2px')
    expect(errors).toEqual([])
    await context.close()
  }
} finally {
  await browser.close()
}
console.log('Supporting navigation: the supporting pages have the group label, one h1 and the section row where there is one.')
