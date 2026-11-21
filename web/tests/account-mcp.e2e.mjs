// Account → MCP access: the connected clients in one panel, with a count,
// search, kind chips, the existing connection actions and the switch that
// lets a management connection approve Factory merges.
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
    { id: 1, name: 'Claude Desktop', scope: 'write', enabled: true, kind: 'oauth', createdAt: now - 5 * DAY, expiresAt: now + 25 * DAY, lastUsedAt: now - 3_600_000, approveMerges: false },
    { id: 2, name: 'Laptop scanner', scope: 'read', enabled: true, kind: 'bearer', createdAt: now - 2 * DAY, expiresAt: now + 28 * DAY, lastUsedAt: now - DAY, approveMerges: false },
    { id: 3, name: 'Old phone', scope: 'read', enabled: false, kind: 'bearer', createdAt: now - 10 * DAY, expiresAt: now + 20 * DAY, lastUsedAt: now - 5 * DAY, approveMerges: false },
  ]
  const state = { failing: false, sent: [] }
  const access = () => ({ enabled: true, browserEnabled: false, browserScope: 'read', canWrite: true, endpoint: 'https://reet.example.test/mcp', connections })
  return {
    routes: {
      '/api/v2/mcp': request => {
        if (request.method() === 'PATCH') return access()
        return access()
      },
      '/api/v2/mcp/connections/1': request => {
        const body = request.postDataJSON()
        state.sent.push(body)
        if (state.failing) return { status: 500, json: { error: 'could not save the connection' } }
        connections = connections.map((connection) => connection.id === 1 ? { ...connection, ...(body.enabled === undefined ? {} : { enabled: body.enabled }), ...(body.approveMerges === undefined ? {} : { approveMerges: body.approveMerges }) } : connection)
        return access()
      },
    },
    state,
  }
}
const open = async options => {
  const made = makeState()
  return { ...await openWorkspace(browser, { width: 1440, routes: made.routes, ...options }), api: made.state }
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

  if (run('merge approval')) {
    const { context, page, errors, api } = await open({})
    await page.goto(`${base}/account/mcp`)
    const row = panel(page).getByRole('listitem').filter({ hasText: 'Claude Desktop' })
    const box = row.getByRole('checkbox', { name: 'May approve Factory merges' })
    // Off until the person allows it, and only a management connection has the switch.
    await expect(box).not.toBeChecked()
    await expect(box).toHaveAccessibleDescription('Turn this on only for a client you trust: an agent using this connection could then approve its own work.')
    await expect(panel(page).getByRole('checkbox', { name: 'May approve Factory merges' })).toHaveCount(1)
    // The keyboard reaches it and switches it.
    await box.focus()
    await page.keyboard.press('Space')
    await expect(page.getByText('This connection may now approve Factory merges.', { exact: true })).toBeVisible()
    await expect(box).toBeChecked()
    expect(api.sent).toEqual([{ approveMerges: true }])
    // A change that was not saved is not left looking made.
    api.failing = true
    await box.click()
    await expect(page.getByRole('alert')).toContainText('could not save the connection')
    await expect(box).toBeChecked()
    api.failing = false
    await box.click()
    await expect(page.getByText('This connection may no longer approve Factory merges.', { exact: true })).toBeVisible()
    await expect(box).not.toBeChecked()
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
      const merges = await page.locator('label', { hasText: 'May approve Factory merges' }).boundingBox()
      expect(merges.height, `merge approval height at ${width}px`).toBeGreaterThanOrEqual(44)
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Account MCP access: one client panel, honest kind counts, search, pause, merge approval and a phone layout.')
