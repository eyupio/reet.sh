// Account → Sessions: the browser list in the collection grammar, with a count,
// search, this/other chips and the existing sign-out actions.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const DAY = 86_400_000
const currentUA = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36'
const safariUA = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15'
const firefoxUA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:131.0) Gecko/20100101 Firefox/131.0'
const makeState = () => {
  const all = [
    { id: 's1', device: currentUA, createdAt: now - 30 * DAY, lastSeen: now, expiresAt: now + 30 * DAY, current: true },
    { id: 's2', device: safariUA, createdAt: now - 5 * DAY, lastSeen: now - DAY, expiresAt: now + 25 * DAY, current: false },
    { id: 's3', device: firefoxUA, createdAt: now - 10 * DAY, lastSeen: now - 2 * DAY, expiresAt: now + 20 * DAY, current: false },
  ]
  let sessions = all
  return {
    routes: {
      '/api/v2/profile/sessions': () => ({ sessions }),
      '/api/v2/profile/sessions/end-others': () => {
        sessions = sessions.filter((session) => session.current)
        return { ended: 2 }
      },
      '/api/v2/profile/sessions/s2': () => {
        sessions = sessions.filter((session) => session.id !== 's2')
        return {}
      },
    },
  }
}
const open = options => {
  const state = makeState()
  return openWorkspace(browser, { width: 1440, routes: state.routes, ...options })
}
const panel = page => page.getByRole('region', { name: 'Your sign-ins' })
const count = page => panel(page).getByRole('status').first()
const scope = (page, name) => page.getByRole('group', { name: 'Show sign-ins' }).getByRole('button', { name })

try {
  if (run('counts and search')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/account/sessions`)
    await expect(count(page)).toHaveText('3 sign-ins')
    await expect(scope(page, /^All/)).toHaveText(/All\s*3/)
    await expect(scope(page, /^This browser/)).toHaveText(/This browser\s*1/)
    await expect(scope(page, /^Other browsers/)).toHaveText(/Other browsers\s*2/)
    await expect(page.getByText('this browser', { exact: true })).toBeVisible()

    await page.getByLabel('Search sign-ins').fill('safari')
    await expect(count(page)).toHaveText('1 of 3 sign-ins')
    await expect(scope(page, /^This browser/)).toHaveText(/This browser\s*0/)
    await expect(page.getByText('Safari on Macintosh', { exact: true })).toBeVisible()
    await expect(page.getByText('Chrome on Linux', { exact: true })).toHaveCount(0)

    await page.getByLabel('Search sign-ins').fill('nowt')
    await expect(count(page)).toHaveText('0 of 3 sign-ins')
    await expect(page.getByRole('heading', { name: 'No sign-in matches that search or filter.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(page)).toHaveText('3 sign-ins')

    await scope(page, /^Other browsers/).click()
    await expect(count(page)).toHaveText('2 of 3 sign-ins')
    await expect(page.getByText('Safari on Macintosh', { exact: true })).toBeVisible()
    await expect(page.getByText('Firefox on Windows', { exact: true })).toBeVisible()
    await expect(page.getByText('Chrome on Linux', { exact: true })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('sign out')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/account/sessions`)
    await page.locator('.session-list > li').filter({ hasText: 'Safari on Macintosh' }).getByRole('button', { name: 'Sign out' }).click()
    await expect(page.getByText('That session is signed out.', { exact: true })).toBeVisible()
    await expect(count(page)).toHaveText('2 sign-ins')
    await expect(panel(page).getByRole('button', { name: 'Sign out the others' })).toBeEnabled()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('sign out others')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/account/sessions`)
    await panel(page).getByRole('button', { name: 'Sign out the others' }).click()
    await expect(page.getByText('2 other sessions are signed out.', { exact: true })).toBeVisible()
    await expect(count(page)).toHaveText('1 sign-in')
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/account/sessions`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const search = await page.getByLabel('Search sign-ins').boundingBox()
      expect(search.height, `search height at ${width}px`).toBeGreaterThanOrEqual(44)
      for (const name of [/^All/, /^This browser/, /^Other browsers/]) {
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
console.log('Account sessions: one panel, honest browser counts, search, sign-out actions and a phone layout.')
