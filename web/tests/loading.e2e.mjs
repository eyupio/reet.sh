// Slow route chunks must leave a usable shell, a fitting skeleton and a way to
// recover if the download fails. Exercise production chunks, not dev imports.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const capture = process.env.LOADING_SCREENSHOTS
try {
  for (const width of [375, 1440]) for (const theme of ['light', 'dark']) {
    for (const [path, chunk, shape, signedIn, ready] of [
      ['/machines', 'Machines', 'rows', true, /^Machines$/],
      ['/signin', 'AccountSignIn', 'auth', false, /Sign in/],
      ['/welcome', 'AccountSignIn', 'auth', false, /Sign in/],
      ['/account', 'Account', 'form', true, /Sam/],
    ]) {
      const { context, page, errors } = await openWorkspace(browser, { width, theme, ...(!signedIn ? { status: { user: null } } : {}) })
      await context.addInitScript(theme => localStorage.setItem('reet-theme', theme), theme)
      let release
      const gate = new Promise(resolve => (release = resolve))
      const requested = []
      page.on('request', r => requested.push(r.url()))
      await page.route(`**/assets/${chunk}-*.js`, async route => { await gate; await route.continue() })
      await page.goto(base + path, { waitUntil: 'domcontentloaded' })
      const placeholder = page.locator(`[data-loading-page="${shape}"]`)
      await expect(placeholder).toBeVisible()
      await expect(page.locator('#boot')).toHaveCount(0)
      const before = await page.locator('.site-header').boundingBox()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      if (capture && width === 1440 && (chunk === 'Machines' || chunk === 'AccountSignIn')) await page.screenshot({ path: `${capture}/reet-${chunk.toLowerCase()}-${theme}.png` })
      if (theme === 'dark') {
        await page.emulateMedia({ reducedMotion: 'reduce' })
        const animations = await placeholder.locator('i').evaluateAll(nodes => nodes.map(n => getComputedStyle(n).animationName))
        expect(animations.every(name => name === 'none')).toBe(true)
      }
      release()
      await expect(placeholder).toHaveCount(0)
      await expect(page.locator('h1').first()).toContainText(ready)
      const after = await page.locator('.site-header').boundingBox()
      expect(after.y).toBe(before.y)
      expect(after.height).toBe(before.height)
      expect(requested.some(url => /\/assets\/Admin-.*\.js/.test(url))).toBe(false)
      expect(errors).toEqual([])
      await context.close()
    }
  }
  const { context, page } = await openWorkspace(browser, { width: 1440 })
  await page.route('**/assets/Spaces-*.js', route => route.abort('failed'))
  await page.goto(base + '/spaces')
  await expect(page.getByRole('alert')).toContainText('This page could not load')
  await expect(page.getByRole('button', { name: 'Reload page' })).toBeVisible()
  await context.close()
  console.log('Loading: public and signed-in lazy chunks, stable header, phone widths, both themes, reduced motion and failed download recovery.')
} finally { await browser.close() }
