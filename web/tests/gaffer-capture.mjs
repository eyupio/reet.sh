// Reproducible screenshots of the actual components, using the scripted fixture rather than a live session.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { startHarness } from './helpers/harness.mjs'

const directory = fileURLToPath(new URL('../../docs/images/ui/', import.meta.url))
await mkdir(directory, { recursive: true })
const harness = await startHarness()
const browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 1000 }, reducedMotion: 'reduce' })
  await page.goto(`${harness.url}/tests/fixtures/gaffer-review.html`)
  await page.locator('.intro-art image').evaluateAll(images => Promise.all(images.map(async el => {
    const image = new Image(); image.src = el.href.baseVal; await image.decode()
  })))
  await page.screenshot({ path: `${directory}gaffer-phosphor-review.png`, fullPage: true })
  for (const [theme, width] of [['dark', 1280], ['light', 1280], ['dark', 390], ['light', 390]]) {
    await page.setViewportSize({ width, height: width === 390 ? 1000 : 950 })
    await page.goto(`${harness.url}/tests/fixtures/gaffer-progress.html?theme=${theme}&scene=Reading`)
    const panel = page.getByRole('region', { name: 'Gaffer’s work update' })
    await expect(panel).toHaveAttribute('data-phase', 'reading')
    await panel.locator('.head image').evaluate(async el => {
      const image = new Image(); image.src = el.href.baseVal; await image.decode()
    })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: `${directory}gaffer-phosphor-${width === 390 ? 'phone' : 'desktop'}-${theme}.png`, fullPage: true })
  }
  console.log('Saved selected-art review and actual Gaffer work cards in both themes at desktop and phone widths.')
} finally {
  await browser.close()
  await harness.close()
}
