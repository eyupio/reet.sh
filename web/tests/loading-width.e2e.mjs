// Compare the HTML shell, auth wait, lazy route and loaded page widths.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const release = []
try {
  for (const width of [375, 1100, 1200, 1440, 1920]) for (const path of ['/', '/machines', '/sessions', '/sessions/', '/terminals/', '/sessions?view=list', '/machines?view=terminal']) for (const collapsed of path === '/' ? [null, false, true] : [false]) {
    let bundleReady, authReady, pageReady
    const bundleGate = new Promise(resolve => (bundleReady = resolve))
    const authGate = new Promise(resolve => (authReady = resolve))
    const pageGate = new Promise(resolve => (pageReady = resolve))
    release.push(bundleReady, authReady, pageReady)
    const { context, page, errors } = await openWorkspace(browser, { width, gates: { '/api/v2/auth/status': authGate } })
    await context.addInitScript(collapsed => {
      localStorage.setItem('reet-token', 'boot-fixture')
      if(collapsed === null)localStorage.removeItem('reet.rail.collapsed')
      else localStorage.setItem('reet.rail.collapsed', collapsed ? '1' : '0')
    }, collapsed)
    await page.route('**/assets/index-*.js', async route => { await bundleGate; await route.continue() })
    const chunk = path === '/' ? 'Home' : path === '/sessions?view=list' ? 'SessionsPage' : 'Machines'
    await page.route(`**/assets/${chunk}-*.js`, async route => { await pageGate; await route.continue() })
    await page.goto(base + path, { waitUntil: 'commit' })
    const shell = page.locator('#boot .boot-main[data-boot="app"]')
    await expect(shell).toBeVisible()
    const staticBox = await shell.boundingBox()
    const staticContent = await shell.locator('.boot-page-head').boundingBox()
    const header = await page.locator('#boot .boot-bar-inner').boundingBox()
    const bootLogo = await page.locator('#boot .boot-brand').boundingBox()
    expect(header.x).toBe(0)
    expect(header.width).toBe(width)
    // On Home the project list is inside the workspace panel, so it is 300 px wide with no second rail column.
    const workspaceRail = width >= 1100 && !path.includes('view=terminal') && (path === '/' || path === '/machines' || path === '/sessions?view=list') ? (path === '/' ? 300 : 228) : 0
    const rail = 0
    if (path === '/') {
      expect(staticBox.x).toBe(workspaceRail + rail)
      expect(staticBox.width).toBe(width - workspaceRail - rail)
    }
    bundleReady()
    await expect(page.locator('#main-content')).toBeAttached()
    const authBox = await page.locator('#main-content').boundingBox()
    const authContent = path === '/machines' ? await page.locator('.boot-page > .head').boundingBox() : null
    const homeAuthContent = path === '/' ? await page.locator('.workspace-content > .head').boundingBox() : null
    authReady()
    await expect(page.locator('#boot')).toHaveCount(0)
    await expect(page.locator('[data-loading-page]')).toBeVisible()
    const chunkBox = await page.locator('#main-content').boundingBox()
    const homeChunkContent = path === '/' ? await page.locator('.workspace-content > .head').boundingBox() : null
    pageReady()
    await expect(page.locator('[data-loading-page]')).toHaveCount(0)
    // The boot shell's logo must land exactly where the app draws it, or a refresh shows the logo resize as the shell hands over.
    const finalLogo = await page.locator('.logo').first().boundingBox()
    for (const edge of ['x', 'y', 'width', 'height']) {
      expect(Math.abs(finalLogo[edge] - bootLogo[edge]), `${path} ${width}: logo ${edge} moved`).toBeLessThanOrEqual(0.6)
    }
    // Every desktop page draws the logo at the sidebar's size, including the session route's slim bar.
    if (width >= 1100) expect(finalLogo.width, `${path} ${width}: desktop logo width`).toBeCloseTo(134, 0)
    if (path === '/') await expect(page.locator('.home.writer > .column')).toBeVisible()
    if (path === '/machines') await expect(page.locator('h1').first()).toHaveText('Machines')
    const loadedBox = await page.locator('#main-content').boundingBox()
    for (const box of [authBox, chunkBox]) {
      expect(box.x, `${path} ${width}: main left changed`).toBe(loadedBox.x)
      expect(box.width, `${path} ${width}: main width changed`).toBe(loadedBox.width)
    }
    if (path === '/') {
      const column = await page.locator('.home.writer > .column').boundingBox()
      expect(column.x).toBe(staticBox.x)
      expect(column.width).toBe(staticBox.width)
      const padding = await page.locator('.home.writer > .column').evaluate(el => parseFloat(getComputedStyle(el).paddingLeft))
      for (const box of [staticContent, homeAuthContent, homeChunkContent]) {
        expect(box.x, `Home ${width}: content gutter changed`).toBe(column.x + padding)
        expect(box.width, `Home ${width}: content width changed`).toBe(column.width - padding * 2)
      }
    } else if (path === '/machines') {
      const heading = await page.locator('.page-heading').first().boundingBox()
      expect(authContent.x).toBe(heading.x)
      expect(authContent.width).toBe(heading.width)
      expect(staticBox.width).toBe(loadedBox.width)
      expect(staticBox.x).toBe(loadedBox.x)
    }
    expect(errors).toEqual([])
    await context.close()
  }
  console.log('Loading widths: HTML boot, auth wait, lazy route and loaded content agree across phone, tablet and wide desktops.')
} catch (error) {
  console.error(error)
  throw error
} finally {
  for (const ready of release) ready()
  await browser.close()
}
