// Account → Password: the three fields in one panel, the mismatch check and the
// existing change-password request.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const makeState = () => {
  const changes = []
  return {
    changes,
    routes: {
      '/api/v2/profile/password': request => {
        changes.push(request.postDataJSON())
        return {}
      },
      '/api/v2/profile/sessions': { sessions: [] },
    },
  }
}
const open = options => {
  const state = makeState()
  return openWorkspace(browser, { width: 1440, routes: state.routes, ...options }).then((opened) => ({ ...opened, changes: state.changes }))
}
const panel = page => page.getByRole('region', { name: 'Password' })
const count = page => panel(page).getByRole('status').first()

try {
  if (run('mismatch and change')) {
    const { context, page, errors, changes } = await open({})
    await page.goto(`${base}/account/security`)
    await expect(count(page)).toHaveText('3 fields')
    const form = page.locator('#password-form')
    await form.getByLabel('Current password').fill('oldpass123')
    const newInputs = form.locator('input[autocomplete="new-password"]')
    await newInputs.nth(0).fill('newpass123')
    await newInputs.nth(1).fill('different123')
    await panel(page).getByRole('button', { name: 'Change password' }).click()
    await expect(page.getByText('The new passwords do not match.', { exact: true })).toBeVisible()
    expect(changes).toEqual([])

    await newInputs.nth(1).fill('newpass123')
    await panel(page).getByRole('button', { name: 'Change password' }).click()
    await expect(page.getByText('Password changed. Every other session was signed out.', { exact: true })).toBeVisible()
    expect(changes).toEqual([{ current: 'oldpass123', password: 'newpass123' }])
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/account/security`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      await expect(panel(page).getByRole('button', { name: 'Change password' })).toBeVisible()
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Account password: one panel, the mismatch check, the request body and a phone layout.')
