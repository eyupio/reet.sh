// Account → API tokens: the token list in the collection grammar, with a create
// card, count, search, used/never-used chips and the existing revoke action.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const DAY = 86_400_000
const makeState = () => {
  let tokens = [
    { id: 1, name: 'laptop scan', createdAt: now - 10 * DAY, lastUsedAt: now - DAY },
    { id: 2, name: 'ci pipeline', createdAt: now - 5 * DAY },
    { id: 3, name: 'deploy key for the build server and its nightly jobs', createdAt: now - 2 * DAY, lastUsedAt: now - 3_600_000 },
  ]
  return {
    routes: {
      '/api/v2/tokens': request => {
        if (request.method() === 'POST') {
          const body = request.postDataJSON()
          const token = { id: 4, name: body.name, token: 'reet_new_secret_value', createdAt: now }
          tokens = [...tokens, { id: token.id, name: token.name, createdAt: token.createdAt }]
          return token
        }
        if (request.method() === 'DELETE') {
          const id = Number(new URL(request.url()).searchParams.get('id'))
          tokens = tokens.filter((token) => token.id !== id)
          return {}
        }
        return { tokens }
      },
    },
  }
}
const open = options => {
  const state = makeState()
  return openWorkspace(browser, { width: 1440, routes: state.routes, ...options })
}
const panel = page => page.getByRole('region', { name: 'Personal API tokens' })
const count = page => panel(page).getByRole('status').first()
const scope = (page, name) => page.getByRole('group', { name: 'Show tokens' }).getByRole('button', { name })

try {
  if (run('counts and search')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/account/tokens`)
    await expect(count(page)).toHaveText('3 tokens')
    await expect(scope(page, /^All/)).toHaveText(/All\s*3/)
    await expect(scope(page, /^Used/)).toHaveText(/Used\s*2/)
    await expect(scope(page, /^Never used/)).toHaveText(/Never used\s*1/)

    await page.getByLabel('Search tokens').fill('scan')
    await expect(count(page)).toHaveText('1 of 3 tokens')
    await expect(page.getByText('laptop scan', { exact: true })).toBeVisible()
    await expect(page.getByText('ci pipeline', { exact: true })).toHaveCount(0)

    await page.getByLabel('Search tokens').fill('nowt')
    await expect(count(page)).toHaveText('0 of 3 tokens')
    await expect(page.getByRole('heading', { name: 'No token matches that search or filter.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('3 tokens')

    await scope(page, /^Never used/).click()
    await expect(count(page)).toHaveText('1 of 3 tokens')
    await expect(page.getByText('ci pipeline', { exact: true })).toBeVisible()
    await expect(page.getByText('laptop scan', { exact: true })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('create and revoke')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/account/tokens`)
    await page.getByLabel('Token name').fill('new scanner')
    await page.getByRole('button', { name: 'Create token' }).click()
    await expect(page.getByText('reet_new_secret_value', { exact: true })).toBeVisible()
    await expect(count(page)).toHaveText('4 tokens')
    await expect(page.getByText('new scanner', { exact: true })).toBeVisible()

    const row = page.locator('.token-list > li').filter({ hasText: 'ci pipeline' })
    await row.getByRole('button', { name: 'Revoke' }).click()
    await page.getByRole('dialog').getByRole('button', { name: 'Revoke token' }).click()
    await expect(page.getByText('ci pipeline', { exact: true })).toHaveCount(0)
    await expect(count(page)).toHaveText('3 tokens')
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/account/tokens`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const search = await page.getByLabel('Search tokens').boundingBox()
      expect(search.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      for (const name of [/^All/, /^Used/, /^Never used/]) {
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
console.log('Account tokens: one panel, honest used counts, search, create and revoke, and a phone layout.')
