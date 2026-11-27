// Home holds one honest shape until its first read settles: the same skeleton the route chunk showed, then real
// content or a way to retry. It never draws a confident empty Home before knowing.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
try {
  {
    let release
    const dataGate = new Promise(done => (release = done))
    const { context, page, errors } = await openWorkspace(browser, { width: 1440, gates: { '/api/v2/terminal/sessions': dataGate } })
    try {
      await page.goto(base)
      await expect(page.locator('[data-loading-page="home"]')).toBeVisible()
      await expect(page.getByText('Nothing is waiting for you.', { exact: true })).toHaveCount(0)
      await expect(page.getByText(/0 waiting for you/)).toHaveCount(0)
      release()
      await expect(page.locator('[data-loading-page="home"]')).toHaveCount(0)
      await expect(page.getByRole('heading', { name: 'Home', exact: true })).toBeVisible()
      await expect(page.getByText('Nothing is waiting for you.', { exact: true })).toBeVisible()
      expect(errors).toEqual([])
    } finally {
      release?.()
      await context.close()
    }
  }
  {
    const { context, page, errors } = await openWorkspace(browser, {
      width: 1440,
      routes: { '/api/v2/terminal/sessions': { status: 503, json: { error: 'unavailable' } } },
    })
    try {
      await page.goto(base)
      await expect(page.getByRole('alert')).toBeVisible()
      await expect(page.getByRole('button', { name: 'Reload page' })).toBeVisible()
      await expect(page.locator('[data-loading-page="home"]')).toHaveCount(0)
      await expect(page.getByText('Nothing is waiting for you.', { exact: true })).toHaveCount(0)
      await expect(page.getByText(/0 waiting for you/)).toHaveCount(0)
      expect(errors).toEqual([])
    } finally {
      await context.close()
    }
  }
  console.log('Home refresh: the skeleton holds until the first read settles, then real content; a failed first read offers Reload instead of an empty Home.')
} finally { await browser.close() }
