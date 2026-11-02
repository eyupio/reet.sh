// The notification centre and Support: one panel per collection, honest counts, a recoverable
// empty search and a phone layout. Activity already uses the same category-chip feed.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const note = (id, title, body, read, tone = '') => ({ id, kind: 'notice', title, body, link: '', tone, readAt: read ? now - 60_000 : undefined, createdAt: now - id * 60_000, updatedAt: now - id * 60_000 })
let notifications = [
  note(1, 'Support replied to “Sync stalls on build-box”', 'The connector log points at a stale token.', false),
  note(2, 'New sign-in to your account', 'Firefox on Linux · 192.0.2.40', false, 'warning'),
  note(3, 'Jo Patel joined Platform engineering', '', true),
  note(4, 'Instance backup taken', 'Encrypted, verified and copied to the off-site destination.', false),
]
const prefs = [
  { key: 'ticket_reply', audience: 'user', group: 'Support', label: 'Ticket replies', description: 'Someone answers a ticket you opened.', locked: '', enabled: true, mode: 'immediate', source: 'you' },
  { key: 'security', audience: 'user', group: 'Account', label: 'Sign-in and password notices', description: 'Sign-ins, password changes and token activity.', security: true, locked: 'always', enabled: true, mode: 'immediate', source: 'locked' },
  { key: 'backup', audience: 'staff', group: 'Operations', label: 'Backup results', description: 'A backup is taken, verified or fails.', locked: '', enabled: true, mode: 'immediate', source: 'platform' },
]
const tickets = [
  { id: 1, opener: 'Sam Taylor', subject: 'Sync stalls on build-box', status: 'open', messages: 3, updatedAt: now - 6e5 },
  { id: 2, opener: 'Jo Patel', groupName: 'Platform engineering', subject: 'Invite link expired too early', status: 'waiting', messages: 2, updatedAt: now - 9e6 },
  { id: 3, opener: 'Sam Taylor', subject: 'Terminal paste strips indentation', status: 'closed', messages: 5, updatedAt: now - 1.7e8 },
]
const notificationRoutes = () => ({
  '/api/v2/notifications': request => {
    const only = new URL(request.url()).searchParams.get('unread') === '1'
    return {
      notifications: only ? notifications.filter((n) => !n.readAt) : notifications,
      unread: notifications.filter((n) => !n.readAt).length,
    }
  },
  '/api/v2/notifications/preferences': { preferences: prefs },
  '/api/v2/notifications/read': request => {
    const body = request.postDataJSON()
    const ids = body?.ids ?? []
    notifications = notifications.map((n) => ids.length === 0 || ids.includes(n.id) ? { ...n, readAt: n.readAt ?? Date.now() } : n)
    return { unread: notifications.filter((n) => !n.readAt).length }
  },
})
const supportRoutes = () => ({
  '/api/v2/tickets': { tickets },
  '/api/v2/groups': { groups: [] },
})
const open = (path, options = {}) => openWorkspace(browser, { width: 1440, routes: path === 'notifications' ? notificationRoutes() : supportRoutes(), ...options })
const count = (page, label) => page.getByRole('region', { name: label }).getByRole('status').first()
const scope = (page, group, name) => page.getByRole('group', { name: group }).getByRole('button', { name })

try {
  if (run('notifications')) {
    const { context, page, errors } = await open('notifications', { unread: 3 })
    await page.goto(`${base}/notifications`)
    await expect(count(page, 'Your notifications')).toHaveText('4 notifications · 3 unread')
    await expect(scope(page, 'Show notifications', /^All/)).toHaveText(/All\s*4/)
    await expect(scope(page, 'Show notifications', /^Unread/)).toHaveText(/Unread\s*3/)

    // A row with no link marks itself read in place.
    await page.getByRole('button', { name: /Support replied to/ }).click()
    await expect(count(page, 'Your notifications')).toHaveText('4 notifications · 2 unread')
    await expect(page.getByRole('button', { name: /Support replied to/ })).not.toContainText('unread')

    await scope(page, 'Show notifications', /^Unread/).click()
    await expect(count(page, 'Your notifications')).toHaveText('2 unread')
    await expect(page.getByRole('button', { name: /New sign-in/ })).toBeVisible()
    await expect(page.getByRole('button', { name: /Jo Patel joined/ })).toHaveCount(0)

    await scope(page, 'Show notifications', /^All/).click()
    await page.getByRole('button', { name: 'Mark all read' }).click()
    await expect(count(page, 'Your notifications')).toHaveText('4 notifications')
    await expect(page.getByRole('button', { name: 'Mark all read' })).toBeDisabled()

    // A locked security notice is a fact, not a disabled checkbox.
    await expect(page.getByText('Always on', { exact: true })).toBeVisible()
    await expect(page.getByRole('checkbox', { name: /Sign-in and password notices/ })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  // The server-side unread query still reaches an old unread notification when the newest page is read.
  if (run('unread beyond the first page')) {
    const old = { ...note(51, 'An older unread notice', 'This one sits below the newest page.', false), createdAt: now - 90 * 86_400_000, updatedAt: now - 90 * 86_400_000 }
    const pageRows = [
      ...notifications.map((n, i) => ({ ...n, id: 100 + i, readAt: now - 1000, title: `Read notice ${i + 1}`, body: '' })),
      ...Array.from({ length: 46 }, (_, i) => ({ ...note(200 + i, `Read notice ${i + 5}`, '', true), readAt: now - 1000 })),
    ].slice(0, 50)
    const { context, page, errors } = await openWorkspace(browser, {
      width: 1440,
      routes: {
        '/api/v2/notifications': request => {
          const only = new URL(request.url()).searchParams.get('unread') === '1'
          return { notifications: only ? [old] : pageRows, unread: 1 }
        },
        '/api/v2/notifications/preferences': { preferences: prefs },
      },
    })
    await page.goto(`${base}/notifications`)
    await expect(count(page, 'Your notifications')).toHaveText('50 notifications · 1 unread')
    await expect(scope(page, 'Show notifications', /^Unread/)).toHaveText(/Unread\s*1/)
    await scope(page, 'Show notifications', /^Unread/).click()
    await expect(count(page, 'Your notifications')).toHaveText('1 unread')
    await expect(page.getByRole('button', { name: /An older unread notice/ })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('support')) {
    const { context, page, errors } = await open('support')
    await page.goto(`${base}/support`)
    await expect(count(page, 'Tickets you can see')).toHaveText('3 tickets')
    await expect(scope(page, 'Show tickets', /^All/)).toHaveText(/All\s*3/)
    await expect(scope(page, 'Show tickets', /^With support/)).toHaveText(/With support\s*1/)
    await expect(scope(page, 'Show tickets', /^Waiting on you/)).toHaveText(/Waiting on you\s*1/)
    await expect(scope(page, 'Show tickets', /^Closed/)).toHaveText(/Closed\s*1/)

    await page.getByLabel('Search tickets').fill('build')
    await expect(count(page, 'Tickets you can see')).toHaveText('1 of 3 tickets')
    await expect(scope(page, 'Show tickets', /^All/)).toHaveText(/All\s*1/)
    await expect(scope(page, 'Show tickets', /^Waiting on you/)).toHaveText(/Waiting on you\s*0/)
    await expect(page.getByRole('link', { name: /Sync stalls on build-box/ })).toBeVisible()

    await scope(page, 'Show tickets', /^Waiting on you/).click()
    await expect(count(page, 'Tickets you can see')).toHaveText('0 of 3 tickets')
    await expect(page.getByRole('heading', { name: 'No ticket matches that search or status.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page, 'Tickets you can see')).toHaveText('3 tickets')
    await expect(page.getByRole('link', { name: /Sync stalls on build-box/ })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const notes = await open('notifications', { width, height: 812, touch: true, unread: 3 })
      await notes.page.goto(`${base}/notifications`)
      expect(await notes.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `notifications width at ${width}px`).toBe(true)
      for (const name of [/^All/, /^Unread/]) {
        const box = await scope(notes.page, 'Show notifications', name).boundingBox()
        expect(box.height, `notification scope at ${width}px`).toBeGreaterThanOrEqual(44)
      }
      expect(notes.errors).toEqual([])
      await notes.context.close()

      const help = await open('support', { width, height: 812, touch: true })
      await help.page.goto(`${base}/support`)
      expect(await help.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `support width at ${width}px`).toBe(true)
      for (const name of [/^All/, /^With support/, /^Waiting on you/, /^Closed/]) {
        const box = await scope(help.page, 'Show tickets', name).boundingBox()
        expect(box.height, `ticket scope at ${width}px`).toBeGreaterThanOrEqual(44)
      }
      expect(help.errors).toEqual([])
      await help.context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Inbox: notifications and support use one panel grammar, honest counts, a recoverable empty state and a phone layout.')
