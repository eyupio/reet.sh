// Account → MCP access: the connected clients in one panel, with a count,
// search, kind chips and the existing connection actions.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const DAY = 86_400_000
const makeState = () => {
  let connections = [
    { id: 1, name: 'Claude Desktop', scope: 'write', enabled: true, kind: 'oauth', createdAt: now - 5 * DAY, expiresAt: now + 25 * DAY, lastUsedAt: now - 3_600_000 },
    { id: 2, name: 'Laptop scanner', scope: 'read', enabled: true, kind: 'bearer', createdAt: now - 2 * DAY, expiresAt: now + 28 * DAY, lastUsedAt: now - DAY },
    { id: 3, name: 'Old phone', scope: 'read', enabled: false, kind: 'bearer', createdAt: now - 10 * DAY, expiresAt: now + 20 * DAY, lastUsedAt: now - 5 * DAY },
  ]
  const access = () => ({ enabled: true, browserEnabled: false, browserScope: 'read', canWrite: true, endpoint: 'https://reet.example.test/mcp', connections })
  return {
    routes: {
      '/api/v2/mcp': request => {
        if (request.method() === 'PATCH') return access()
        return access()
      },
      '/api/v2/mcp/connections/1': request => {
        const body = request.postDataJSON()
        connections = connections.map((connection) => connection.id === 1 ? { ...connection, ...(body.enabled === undefined ? {} : { enabled: body.enabled }) } : connection)
        return access()
      },
    },
  }
}
const open = options => {
  const state = makeState()
  return openWorkspace(browser, { width: 1440, routes: state.routes, ...options })
}
const panel = page => page.getByRole('region', { name: 'Connected clients' })
const count = page => panel(page).getByRole('status').first()
const scope = (page, name) => page.getByRole('group', { name: 'Show connections' }).getByRole('button', { name })

try {
  if (run('counts and search')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/account/mcp`)
    await expect(count(page)).toHaveText('3 connections')
    await expect(scope(page, /^All/)).toHaveText(/All\s*3/)
    await expect(scope(page, /^OAuth/)).toHaveText(/OAuth\s*1/)
    await expect(scope(page, /^Bearer token/)).toHaveText(/Bearer token\s*2/)

    await page.getByLabel('Search connections').fill('laptop')
    await expect(count(page)).toHaveText('1 of 3 connections')
    await expect(scope(page, /^OAuth/)).toHaveText(/OAuth\s*0/)
    await expect(scope(page, /^Bearer token/)).toHaveText(/Bearer token\s*1/)
    await expect(page.getByText('Laptop scanner', { exact: true })).toBeVisible()
    await expect(page.getByText('Claude Desktop', { exact: true })).toHaveCount(0)

    await page.getByLabel('Search connections').fill('nowt')
    await expect(count(page)).toHaveText('0 of 3 connections')
    await expect(page.getByRole('heading', { name: 'No connection matches that search or kind.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('3 connections')

    await scope(page, /^OAuth/).click()
    await expect(count(page)).toHaveText('1 of 3 connections')
    await expect(page.getByText('Claude Desktop', { exact: true })).toBeVisible()
    await expect(page.getByText('Laptop scanner', { exact: true })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('pause')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/account/mcp`)
    const row = panel(page).getByRole('listitem').filter({ hasText: 'Claude Desktop' })
    await row.getByRole('button', { name: 'Pause' }).click()
    await expect(page.getByText('Connection paused.', { exact: true })).toBeVisible()
    await expect(row.getByRole('button', { name: 'Resume' })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/account/mcp`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const search = await page.getByLabel('Search connections').boundingBox()
      expect(search.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      for (const name of [/^All/, /^OAuth/, /^Bearer token/]) {
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
console.log('Account MCP access: one client panel, honest kind counts, search, pause and a phone layout.')
