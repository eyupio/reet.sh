// Teams & organisations: one scannable surface, honest scope counts, management folded
// per group, and a create form that announces itself and returns focus.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const groups = [
  {
    id: 9, name: 'Eyup Ltd', kind: 'organisation', ownerId: 1, role: 'owner',
    members: [
      { id: 1, name: 'Sam Taylor', email: 'sam@example.test', role: 'owner' },
      { id: 2, name: 'Alex Morgan', email: 'alex@example.test', role: 'manager' },
    ],
  },
  {
    id: 1, name: 'Platform engineering', kind: 'team', parentId: 9, ownerId: 1, role: 'owner',
    members: [
      { id: 1, name: 'Sam Taylor', email: 'sam@example.test', role: 'owner' },
      { id: 3, name: 'Jo Patel', email: 'jo@example.test', role: 'member' },
      { id: 4, name: 'Riley Chen', email: 'riley@example.test', role: 'member' },
    ],
  },
  // The API sends members only for a group the person owns or manages.
  { id: 2, name: 'Design', kind: 'team', ownerId: 2, role: 'member' },
]
const events = [
  { key: 'list-shared', label: 'A list is shared', description: 'Someone shares a list with this group.' },
  { key: 'member-added', label: 'A person joins', description: 'Someone is added to this group.' },
]
const notify = id => ({
  notify: { groupId: id, enabled: true, events: ['list-shared'], recipients: 'managers', extraEmails: [] },
  events, mailReady: true, mailMode: 'smtp', groupName: groups.find(group => group.id === id)?.name ?? '',
})
const routes = () => ({
  '/api/v2/groups': { groups },
  '/api/v2/groups/9/notifications': notify(9),
  '/api/v2/groups/1/notifications': notify(1),
})
const open = options => openWorkspace(browser, { width: 1440, routes: routes(), ...options })
const count = page => page.getByRole('region', { name: 'Your teams and organisations' }).getByRole('status').first()
const chip = (page, name) => page.getByRole('group', { name: 'Show groups' }).getByRole('button', { name })
const group = (page, name) => page.locator('details.group').filter({ hasText: name })

try {
  // The panel holds the figure and each scope chip says how many groups pressing it shows.
  if (run('counts and scopes')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/spaces`)
    await expect(count(page)).toHaveText('3 groups')
    await expect(chip(page, /^All/)).toHaveText(/All\s*3/)
    await expect(chip(page, /^Organisations/)).toHaveText(/Organisations\s*1/)
    await expect(chip(page, /^Teams/)).toHaveText(/Teams\s*2/)
    await expect(chip(page, /^Managed by you/)).toHaveText(/Managed by you\s*2/)

    await chip(page, /^Organisations/).click()
    await expect(count(page)).toHaveText('1 of 3 groups')
    await expect(page.getByText('Eyup Ltd', { exact: true })).toBeVisible()
    await expect(page.getByText('Platform engineering', { exact: true })).toHaveCount(0)

    await chip(page, /^Teams/).click()
    await expect(count(page)).toHaveText('2 of 3 groups')
    await expect(page.getByText('Platform engineering', { exact: true })).toBeVisible()
    await expect(page.getByText('Design', { exact: true })).toBeVisible()

    await chip(page, /^Managed by you/).click()
    await expect(count(page)).toHaveText('2 of 3 groups')
    await expect(page.getByText('Design', { exact: true })).toHaveCount(0)

    await chip(page, /^All/).click()
    await expect(count(page)).toHaveText('3 groups')
    expect(errors).toEqual([])
    await context.close()
  }

  // Search reads group names and the members the API sent; clearing it brings every group back.
  if (run('search and honest counts')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/spaces`)
    await page.getByLabel('Search groups').fill('alex')
    await expect(count(page)).toHaveText('1 of 3 groups')
    await expect(chip(page, /^All/)).toHaveText(/All\s*1/)
    await expect(chip(page, /^Organisations/)).toHaveText(/Organisations\s*1/)
    await expect(chip(page, /^Teams/)).toHaveText(/Teams\s*0/)
    await expect(page.getByText('Eyup Ltd', { exact: true })).toBeVisible()
    await expect(page.getByText('Platform engineering', { exact: true })).toHaveCount(0)

    await page.getByLabel('Search groups').fill('nowt')
    await expect(count(page)).toHaveText('0 of 3 groups')
    await expect(page.getByRole('heading', { name: 'Nowt matches just yet.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('3 groups')
    expect(errors).toEqual([])
    await context.close()
  }

  // A group opens where it sits; its members and tools stay out of the way until asked for.
  if (run('folded management')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/spaces`)
    await group(page, 'Platform engineering').locator('summary').first().click()
    await expect(group(page, 'Platform engineering').getByText('Jo Patel', { exact: true })).toBeVisible()
    await expect(group(page, 'Platform engineering').getByRole('button', { name: 'Add person' })).toBeVisible()
    await expect(group(page, 'Platform engineering').getByText('Group settings', { exact: true })).toBeVisible()
    await expect(group(page, 'Platform engineering').getByText('Notifications', { exact: true })).toBeVisible()

    // A group the person only belongs to says who can change it and never claims a member figure.
    await expect(group(page, 'Design').locator('.members')).toHaveCount(0)
    await group(page, 'Design').locator('summary').first().click()
    await expect(group(page, 'Design').getByText('Ask this group’s manager to update its membership.', { exact: false })).toBeVisible()
    await expect(group(page, 'Design').getByRole('button', { name: 'Add person' })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  // The create form is a page control: it opens, takes focus, closes on Cancel and gives focus back.
  if (run('create form')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/spaces`)
    const opener = page.getByRole('button', { name: 'Create a group' })
    await opener.click()
    await expect(page.getByRole('heading', { name: 'Create a team or organisation' })).toBeVisible()
    await expect(page.locator('#group-name')).toBeFocused()
    await page.getByRole('button', { name: 'Cancel' }).click()
    await expect(page.getByRole('heading', { name: 'Create a team or organisation' })).toHaveCount(0)
    await expect(opener).toBeFocused()
    expect(errors).toEqual([])
    await context.close()
  }

  // A phone keeps every scope, group and summary reachable, with no sideways scroll.
  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/spaces`)
      await expect(count(page)).toHaveText('3 groups')
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      for (const name of [/^All/, /^Organisations/, /^Teams/, /^Managed by you/]) {
        const box = await chip(page, name).boundingBox()
        expect(box.height, `chip ${name} at ${width}px`).toBeGreaterThanOrEqual(44)
        expect(box.x + box.width, `chip right edge at ${width}px`).toBeLessThanOrEqual(width)
      }
      const summary = await group(page, 'Platform engineering').locator('summary').first().boundingBox()
      expect(summary.height, `group summary at ${width}px`).toBeGreaterThanOrEqual(44)
      expect(summary.x + summary.width, `group right edge at ${width}px`).toBeLessThanOrEqual(width)
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Teams: one scannable surface, honest scope counts, folded management and a focused create form.')
