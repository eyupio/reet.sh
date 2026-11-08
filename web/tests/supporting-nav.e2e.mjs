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
    // The row is a sibling of the header inside .page-head, never inside the header element.
    await expect(page.locator('.page-heading').getByRole('navigation')).toHaveCount(0)
    await expect(page.locator('.page-head > .row').getByRole('navigation', { name: 'Help & community' })).toHaveCount(1)
    expect(errors).toEqual([])
    await context.close()
  }

  // Signed out, Community still names its group (the page is public); there is no sign-in to give a second page, so no row is needed.
  if (run('community signed out')) {
    const { context, page, errors } = await open({ status: { user: null }, routes: { '/api/community': { lists: [], skills: [] } } })
    await page.goto(`${base}/community`)
    await expect(page.getByRole('heading', { level: 1, name: 'Community' })).toBeVisible()
    await expect(page.locator('.page-heading').getByText('Help & community', { exact: true })).toBeVisible()
    await expect(page.locator('.page-head').getByRole('navigation')).toHaveCount(0)
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
    // The 44px target does not move the title: with the link swapped for plain text the h1 sits at the same height.
    const h1 = page.getByRole('heading', { level: 1, name: 'Launch checklist' })
    const withLink = (await h1.boundingBox()).y
    await back.evaluate(link => link.replaceWith(document.createTextNode(link.textContent)))
    const withoutLink = (await h1.boundingBox()).y
    expect(Math.abs(withLink - withoutLink), `h1 moved ${withLink - withoutLink}px for the label link`).toBeLessThanOrEqual(1)
    expect(errors).toEqual([])
    await context.close()
  }
  // A support ticket: the label links back to Support, the subject is the one h1, the ticket number and status stay on the page, and there is no row.
  if (run('ticket')) {
    const now = Date.now()
    const ticket = { id: 41, opener: 'Sam', subject: 'Sync stalls on build-box', status: 'waiting', messages: 1, createdAt: now - 6e5, updatedAt: now - 6e5, assignee: 'Alex', assigneeId: 2 }
    const { context, page, errors } = await open({ routes: { '/api/v2/tickets/41': { ticket, messages: [{ id: 1, ticketId: 41, author: 'Sam', authorKind: 'user', body: 'It stalls.', internal: false, createdAt: now - 6e5 }] } } })
    await page.goto(`${base}/support/41`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: 'Sync stalls on build-box', exact: true })).toBeVisible()
    const head = page.locator('.page-head')
    await expect(head.getByRole('link', { name: 'Support', exact: true })).toHaveAttribute('href', '/support')
    await expect(head.getByText('Ticket #41', { exact: false })).toBeVisible()
    await expect(head.getByText('Waiting on you', { exact: true })).toBeVisible()
    await expect(head.getByRole('navigation')).toHaveCount(0)
    await expect(page.getByRole('link', { name: '← All tickets' })).toHaveAttribute('href', '/support')
    await expect(page.getByRole('region', { name: 'Reply' })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  // Machines: the title is the menu name and the section row holds Inventory & connection, Sync and the Software action.
  if (run('machines')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/machines`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: 'Machines', exact: true })).toBeVisible()
    await expect(page.locator('.page-heading').getByText('Machines', { exact: true }).first()).toBeVisible()
    const row = page.getByRole('navigation', { name: 'Machines', exact: true })
    await expect(row).toHaveCount(1)
    await expect(row.getByRole('link', { name: 'Inventory & connection' })).toHaveAttribute('aria-current', 'page')
    await expect(row.getByRole('link', { name: 'Sync', exact: true })).toHaveAttribute('href', '/sync')
    await expect(row.getByRole('button', { name: 'Software', exact: true })).toBeVisible()
    await expect(page.locator('.page-head').getByRole('navigation', { name: 'Machines', exact: true })).toHaveCount(1)
    // Software is an action: it opens its dialog and does not navigate.
    await row.getByRole('button', { name: 'Software', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Fleet software' })
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: 'Close fleet software' }).click()
    await expect(dialog).toBeHidden()
    expect(new URL(page.url()).pathname).toBe('/machines')
    expect(errors).toEqual([])
    await context.close()
  }

  // The sync view is titled Sync and marks the Sync link current; the row (with Software) stays.
  if (run('machines sync')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/sync`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: 'Sync', exact: true })).toBeVisible()
    const row = page.getByRole('navigation', { name: 'Machines', exact: true })
    await expect(row.getByRole('link', { name: 'Sync', exact: true })).toHaveAttribute('aria-current', 'page')
    await expect(row.getByRole('link', { name: 'Inventory & connection' })).not.toHaveAttribute('aria-current', 'page')
    await expect(row.getByRole('button', { name: 'Software', exact: true })).toBeVisible()
    // The row is the shared one inside the page header (the old Machines navigation sat outside it).
    await expect(page.locator('.page-head').getByRole('navigation', { name: 'Machines', exact: true })).toHaveCount(1)
    expect(errors).toEqual([])
    await context.close()
  }

  // One machine: the label links back to Machines, the host is the title, and the Sync link and Software action stay reachable.
  if (run('machine focus')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/machines?machine=build-box`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: 'build-box', exact: true })).toBeVisible()
    await expect(page.locator('.page-heading').getByRole('link', { name: 'Machines', exact: true })).toHaveAttribute('href', '/machines')
    const row = page.getByRole('navigation', { name: 'Machines', exact: true })
    await expect(row.getByRole('link', { name: 'Sync', exact: true })).toHaveAttribute('href', '/sync')
    await expect(row.getByRole('button', { name: 'Software', exact: true })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  // Instance administration: the group label, and exactly one row (Admin's own sections), never a second Workspace row beside it.
  if (run('admin')) {
    const now = Date.now()
    const { context, page, errors } = await open({ routes: {
      '/api/admin/status': { version: 'v1.4.2', commit: 'fixture', db: { path: '/data/reet.db', size: 1024 }, counts: { all: 0, liked: 0, bookmarked: 0, used: 0 }, stages: { use: 0, try: 0, shelf: 0 }, environments: [], events: [], skillsSh: true, metaLastOK: '', metaLastError: '', topics: { lastSynced: '', lastError: '' }, settings: { refreshEnabled: true, topicsEnabled: true, linkListsEnabled: false, rollupIndividual: true, pruneServerEnvironmentsDays: 0, instanceName: 'Test' } },
      '/api/v2/admin/users': { users: [{ id: 1, name: 'Sam', email: 'sam@example.test', role: 'owner', createdAt: now, tokens: 0, lastActive: now }] },
      '/api/v2/admin/sso': { enabled: false },
    } })
    await page.goto(`${base}/admin/users`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.locator('.page-heading').getByText('Instance administration', { exact: true })).toBeVisible()
    await expect(page.locator('.page-head').getByRole('navigation')).toHaveCount(1)
    const row = page.getByRole('navigation', { name: 'Admin', exact: true })
    await expect(row.getByRole('link', { name: 'Users' })).toHaveAttribute('aria-current', 'page')
    await expect(page.getByRole('navigation', { name: 'Workspace', exact: true })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  // Account: the label, the person as the one h1, and Account's own section row.
  if (run('account')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/account/profile`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: 'Sam', exact: true })).toBeVisible()
    await expect(page.locator('.page-heading').getByText('Account', { exact: true })).toBeVisible()
    const row = page.getByRole('navigation', { name: 'Account', exact: true })
    await expect(row).toHaveCount(1)
    await expect(row.getByRole('link', { name: 'Profile' })).toHaveAttribute('aria-current', 'page')
    // The Machines and Compare & sync entries still lead to the Machines page and its sync view.
    await expect(row.getByRole('link', { name: 'Machines', exact: true })).toHaveAttribute('href', '/machines')
    await expect(row.getByRole('link', { name: 'Compare & sync' })).toHaveAttribute('href', '/machines?view=compare')
    // The row spans the content column under the avatar: it is not indented beside it, and it stays on one line.
    const column = await page.locator('section.account').boundingBox()
    const nav = await row.boundingBox()
    expect(Math.abs(nav.x - column.x), 'Account row left edge vs the content column').toBeLessThanOrEqual(1)
    expect(Math.abs(nav.x + nav.width - (column.x + column.width)), 'Account row right edge vs the content column').toBeLessThanOrEqual(1)
    const avatar = await page.locator('section.account .avatar').boundingBox()
    expect(avatar.y + avatar.height, 'avatar ends above the row').toBeLessThanOrEqual(nav.y)
    expect(errors).toEqual([])
    await context.close()
  }

  // The other pages: one h1 that is the menu name, the group label, and the row with the current page marked.
  const pages = [
    { name: 'setups', path: '/profiles', title: 'Setups', group: 'Skills & distribution', current: 'Setups', others: ['Lists', 'Write a skill'] },
    { name: 'write a skill', path: '/hosting', title: 'Write a skill', group: 'Skills & distribution', current: 'Write a skill', others: ['Setups', 'Lists'] },
    { name: 'support', path: '/support', title: 'Support', group: 'Help & community', current: 'Support', others: ['Community'], routes: { '/api/v2/tickets': { tickets: [] }, '/api/v2/groups': { groups: [] } } },
    { name: 'teams', path: '/spaces', title: 'Teams & organisations', group: 'Workspace', current: 'Teams & organisations', others: ['Instance administration'], routes: { '/api/v2/groups': { groups: [] } } },
  ]
  for (const item of pages) if (run(item.name)) {
    const { context, page, errors } = await open({ routes: item.routes })
    await page.goto(`${base}${item.path}`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: item.title, exact: true })).toBeVisible()
    await expect(page.locator('.page-heading').getByText(item.group, { exact: true })).toBeVisible()
    const row = page.locator('.page-head').getByRole('navigation', { name: item.group, exact: true })
    await expect(row).toHaveCount(1)
    await expect(row.getByRole('link', { name: item.current, exact: true })).toHaveAttribute('aria-current', 'page')
    await expect(row.locator('.desktop [aria-current="page"]')).toHaveCount(1)
    for (const other of item.others) await expect(row.getByRole('link', { name: other, exact: true })).not.toHaveAttribute('aria-current', 'page')
    await expect(page.locator('.page-heading').getByRole('navigation')).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  // Activity as its own page (an instance without accounts; with accounts it is a section of Account) is not in the More list: its label names the workspace, and it has no row.
  if (run('activity')) {
    const { context, page, errors } = await open({ status: { mode: 'none', user: null } })
    await page.goto(`${base}/activity`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(page.getByRole('heading', { level: 1, name: 'Activity', exact: true })).toBeVisible()
    await expect(page.locator('.page-heading').getByText('This workspace', { exact: true })).toBeVisible()
    await expect(page.locator('.page-head').getByRole('navigation')).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  // Admin's h1 is the open section, not the menu name.
  if (run('admin title')) {
    const now = Date.now()
    const { context, page, errors } = await open({ routes: {
      '/api/admin/status': { version: 'v1.4.2', commit: 'fixture', db: { path: '/data/reet.db', size: 1024 }, counts: { all: 0, liked: 0, bookmarked: 0, used: 0 }, stages: { use: 0, try: 0, shelf: 0 }, environments: [], events: [], skillsSh: true, metaLastOK: '', metaLastError: '', topics: { lastSynced: '', lastError: '' }, settings: { refreshEnabled: true, topicsEnabled: true, linkListsEnabled: false, rollupIndividual: true, pruneServerEnvironmentsDays: 0, instanceName: 'Test' } },
      '/api/v2/admin/users': { users: [{ id: 1, name: 'Sam', email: 'sam@example.test', role: 'owner', createdAt: now, tokens: 0, lastActive: now }] },
      '/api/v2/admin/sso': { enabled: false },
    } })
    await page.goto(`${base}/admin/users`)
    await expect(page.getByRole('heading', { level: 1, name: 'Users', exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    expect(errors).toEqual([])
    await context.close()
  }

  // What a person may see: the row only lists pages they can open, and a lone page has a label and no row.
  if (run('permissions')) {
    const viewer = await open({ status: { user: { id: 2, name: 'Vi', email: 'vi@example.test', role: 'viewer' } } })
    await viewer.page.goto(`${base}/shared`)
    await expect(viewer.page.getByRole('heading', { level: 1, name: 'Lists', exact: true })).toBeVisible()
    const row = viewer.page.getByRole('navigation', { name: 'Skills & distribution' })
    await expect(row.getByRole('link', { name: 'Setups' })).toBeVisible()
    await expect(row.getByRole('link', { name: 'Write a skill' })).toHaveCount(0)
    expect(viewer.errors).toEqual([])
    await viewer.context.close()

    // Write a skill is not in a viewer's More list, but a viewer who deep-links it still sees the group label (and no row of one).
    const deepLink = await open({ status: { user: { id: 2, name: 'Vi', email: 'vi@example.test', role: 'viewer' } } })
    await deepLink.page.goto(`${base}/hosting`)
    await expect(deepLink.page.getByRole('heading', { level: 1 })).toHaveCount(1)
    await expect(deepLink.page.getByRole('heading', { level: 1, name: 'Write a skill', exact: true })).toBeVisible()
    await expect(deepLink.page.locator('.page-heading').getByText('Skills & distribution', { exact: true })).toBeVisible()
    expect(deepLink.errors).toEqual([])
    await deepLink.context.close()

    // With the support desk off Support is gone, so Help & community is Community alone: the label shows and no row does.
    const noDesk = await open({ status: { supportDesk: false }, routes: { '/api/community': { lists: [], skills: [] } } })
    await noDesk.page.goto(`${base}/community`)
    await expect(noDesk.page.getByRole('heading', { level: 1, name: 'Community', exact: true })).toBeVisible()
    await expect(noDesk.page.locator('.page-heading').getByText('Help & community', { exact: true })).toBeVisible()
    await expect(noDesk.page.locator('.page-head').getByRole('navigation')).toHaveCount(0)
    expect(noDesk.errors).toEqual([])
    await noDesk.context.close()

    // A writer who is not an administrator has Teams alone in Workspace: the label, no row.
    const writer = await open({ status: { user: { id: 3, name: 'Wren', email: 'wren@example.test', role: 'writer' } }, routes: { '/api/v2/groups': { groups: [] } } })
    await writer.page.goto(`${base}/spaces`)
    await expect(writer.page.getByRole('heading', { level: 1, name: 'Teams & organisations', exact: true })).toBeVisible()
    await expect(writer.page.locator('.page-heading').getByText('Workspace', { exact: true })).toBeVisible()
    await expect(writer.page.locator('.page-head').getByRole('navigation')).toHaveCount(0)
    expect(writer.errors).toEqual([])
    await writer.context.close()
  }

  // A phone: the row folds into one menu that names the current page, opens and closes, and an action closes it before it runs.
  if (run('phone menu')) {
    const { context, page, errors } = await open({ width: 390 })
    await page.goto(`${base}/shared`)
    const row = page.getByRole('navigation', { name: 'Skills & distribution' })
    const menu = row.locator('details.mobile')
    await expect(menu.locator('summary')).toBeVisible()
    await expect(menu.locator('summary')).toContainText('Lists')
    await expect(row.locator('.desktop')).toBeHidden()
    await expect(menu).not.toHaveAttribute('open', '')
    await menu.locator('summary').click()
    await expect(menu).toHaveAttribute('open', '')
    await expect(row.getByRole('link', { name: 'Setups' })).toBeVisible()
    await expect(row.getByRole('link', { name: 'Lists' })).toHaveAttribute('aria-current', 'page')
    await menu.locator('summary').click()
    await expect(menu).not.toHaveAttribute('open', '')
    await expect(row.getByRole('link', { name: 'Setups' })).toBeHidden()

    // Software is an action: choosing it closes the menu and opens the dialog.
    await page.goto(`${base}/machines`)
    const machines = page.getByRole('navigation', { name: 'Machines', exact: true })
    const fold = machines.locator('details.mobile')
    await fold.locator('summary').click()
    await expect(fold).toHaveAttribute('open', '')
    await machines.getByRole('button', { name: 'Software', exact: true }).click()
    await expect(page.getByRole('dialog', { name: 'Fleet software' })).toBeVisible()
    await expect(fold).not.toHaveAttribute('open', '')
    expect(errors).toEqual([])
    await context.close()
  }

  // The narrowest phone: no horizontal scroll on the pages that carry a row.
  if (run('320px')) {
    for (const path of ['/shared', '/community', '/spaces', '/machines', '/account/profile']) {
      const { context, page, errors } = await open({ width: 320, routes: { '/api/community': { lists: [], skills: [] }, '/api/v2/groups': { groups: [] } } })
      await page.goto(`${base}${path}`)
      await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
      await page.waitForTimeout(300)
      const sizes = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }))
      expect(sizes.scroll, `${path} overflows at 320px`).toBeLessThanOrEqual(sizes.client)
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Supporting navigation: the supporting pages have the group label, one h1 and the section row where there is one.')
