// Admin → Email: the outbox in the collection grammar, with a count, search,
// state chips whose figures follow the search, and one row per message.
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
const settings = {
  source: 'database', mode: 'smtp', summary: 'Sending through smtp.example.test:587 with STARTTLS.',
  host: 'smtp.example.test', port: 587, tls: 'starttls', user: 'reet@example.test', passwordSet: true,
  from: 'reet@example.test', fromName: 'reet.sh', replyTo: '', baseUrl: 'https://reet.example.test',
  baseUrlInUse: 'https://reet.example.test', dkimSelector: 'reet', updatedAt: now - DAY, updatedBy: 'Sam Example',
}
const messages = [
  { id: 'm1', template: 'invite', to: 'ali.really.long.email.address@example-company.test', subject: 'You have been invited to reet.sh', state: 'failed', attempts: 3, lastError: '550 5.7.1 relaying denied for this very long reason that goes on', via: 'smtp', createdAt: now - DAY, secret: true, resendable: true },
  { id: 'm2', template: 'reset', to: 'jo@example.test', subject: 'Reset your password', state: 'queued', attempts: 0, createdAt: now - 2 * DAY, nextAttemptAt: now + 300_000, secret: false, resendable: false },
  { id: 'm3', template: 'welcome', to: 'riley@example.test', subject: 'Welcome to reet.sh', state: 'held', attempts: 0, createdAt: now - 3 * DAY, secret: false, resendable: false },
  { id: 'm4', template: 'signin', to: 'sam@example.test', subject: 'Your sign-in link', state: 'delivered', attempts: 1, createdAt: now - 4 * DAY, secret: true, resendable: true },
  { id: 'm5', template: 'ticket-reply', to: 'team@example.test', subject: 'A ticket has a reply', state: 'bounced', attempts: 1, lastError: '550 mailbox unavailable', createdAt: now - 5 * DAY, secret: false, resendable: true },
]
const outbox = { mode: 'smtp', summary: 'Mail is sent through your SMTP server.', messages, counts: { failed: 1, queued: 1, held: 1, delivered: 1, bounced: 1 } }
const routes = () => ({
  '/api/admin/status': adminStatus,
  '/api/v2/admin/mail': { settings },
  '/api/v2/admin/mail/outbox': outbox,
})
const open = options => openWorkspace(browser, { width: 1440, routes: routes(), ...options })
const panel = page => page.getByRole('region', { name: 'Outbox' })
const count = page => panel(page).getByRole('status').first()
const scope = (page, name) => page.getByRole('group', { name: 'Show messages' }).getByRole('button', { name })

try {
  if (run('counts and search')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/email`)
    await expect(count(page)).toHaveText('5 messages')
    for (const [name, figure] of [['All', 5], ['Queued', 1], ['Sending', 0], ['Held', 1], ['Delivered', 1], ['Failed', 1], ['Bounced', 1]]) {
      await expect(scope(page, new RegExp(`^${name}`))).toHaveText(new RegExp(`${name}\\s*${figure}`))
    }

    await page.getByLabel('Search outbox').fill('invite')
    await expect(count(page)).toHaveText('1 of 5 messages')
    await expect(scope(page, /^All/)).toHaveText(/All\s*1/)
    await expect(scope(page, /^Failed/)).toHaveText(/Failed\s*1/)
    await expect(scope(page, /^Queued/)).toHaveText(/Queued\s*0/)
    await expect(page.getByText('You have been invited to reet.sh', { exact: true })).toBeVisible()
    await expect(page.getByText('ali.really.long.email.address@example-company.test')).toBeVisible()
    await expect(page.getByText('550 5.7.1 relaying denied for this very long reason that goes on')).toBeVisible()
    await expect(page.getByText('Reset your password', { exact: true })).toHaveCount(0)

    await page.getByLabel('Search outbox').fill('relaying')
    await expect(count(page)).toHaveText('1 of 5 messages')

    await page.getByLabel('Search outbox').fill('nowt')
    await expect(count(page)).toHaveText('0 of 5 messages')
    await expect(page.getByRole('heading', { name: 'No message matches that search or state.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('5 messages')

    await scope(page, /^Failed/).click()
    await expect(count(page)).toHaveText('1 of 5 messages')
    await page.getByLabel('Search outbox').fill('reset')
    await expect(count(page)).toHaveText('0 of 5 messages')
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('5 messages')
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('rows')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/email`)
    await page.getByLabel('Search outbox').fill('invite')
    const row = panel(page).getByRole('listitem').filter({ hasText: 'You have been invited to reet.sh' })
    await expect(row).toContainText('invite · sign-in link')
    await expect(row).toContainText('3 tries')
    await expect(row.getByRole('button', { name: 'Resend' })).toBeVisible()

    await page.getByLabel('Search outbox').fill('welcome')
    const held = panel(page).getByRole('listitem').filter({ hasText: 'Welcome to reet.sh' })
    await expect(held).toContainText('Held')
    await expect(held.getByRole('button', { name: 'Resend' })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('outbox retry')) {
    let attempts = 0
    const { context, page, errors } = await open({
      routes: {
        ...routes(),
        '/api/v2/admin/mail/outbox': () => {
          attempts++
          return attempts === 1 ? { status: 500, json: { error: 'The outbox is unavailable.' } } : outbox
        },
      },
    })
    await page.goto(`${base}/admin/email`)
    await expect(page.getByRole('heading', { name: 'The outbox could not be read.' })).toBeVisible()
    await page.getByRole('button', { name: 'Try again' }).click()
    await expect(count(page)).toHaveText('5 messages')
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/admin/email`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const search = await page.getByLabel('Search outbox').boundingBox()
      expect(search.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      for (const name of [/^All/, /^Queued/, /^Sending/, /^Held/, /^Delivered/, /^Failed/, /^Bounced/]) {
        const box = await scope(page, name).boundingBox()
        expect(box.height, `scope height at ${width}px`).toBeGreaterThanOrEqual(44)
      }
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Admin email: one outbox panel, honest state counts, search, row anatomy and a phone layout.')
