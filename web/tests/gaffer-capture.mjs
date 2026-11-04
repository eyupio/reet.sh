// Reproducible screenshots of the actual components, using the scripted fixture rather than a live session.
import { chromium, expect } from '@playwright/test'
import { mkdir, mkdtemp, rm } from 'node:fs/promises'
import { execFileSync } from 'node:child_process'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
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
  await page.screenshot({ path: `${directory}gaffer-theme-review.png`, fullPage: true })
  for (const [theme, width] of [['dark', 1280], ['light', 1280], ['dark', 390], ['light', 390]]) {
    await page.setViewportSize({ width, height: width === 390 ? 1000 : 950 })
    await page.goto(`${harness.url}/tests/fixtures/gaffer-progress.html?theme=${theme}&scene=Reading`)
    const panel = page.getByRole('region', { name: 'Gaffer’s work update' })
    await expect(panel).toHaveAttribute('data-phase', 'reading')
    await panel.locator('.head image').evaluate(async el => {
      const image = new Image(); image.src = el.href.baseVal; await image.decode()
    })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await panel.getByRole('button', { name: 'Say ey up to Gaffer' }).click()
    await page.screenshot({ path: `${directory}gaffer-theme-${width === 390 ? 'phone' : 'desktop'}-${theme}.png`, fullPage: true })
  }
  await page.setViewportSize({ width: 1100, height: 1400 })
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.goto(`${harness.url}/tests/fixtures/gaffer-artwork.html`)
  await page.locator('.repertoire image').evaluateAll(images => Promise.all(images.map(async el => {
    const image = new Image(); image.src = el.href.baseVal; await image.decode()
  })))
  await page.evaluate(() => {
    window.__repertoireAnimations = [...document.querySelectorAll('.repertoire .gaffer-avatar')].flatMap(el => el.getAnimations({ subtree: true }))
    for (const animation of window.__repertoireAnimations) { animation.pause(); animation.currentTime = 0 }
  })
  const frames = await mkdtemp(join(tmpdir(), 'gaffer-theme-'))
  try {
    for (let frame = 0; frame < 90; frame++) {
      await page.evaluate(time => { for (const animation of window.__repertoireAnimations) animation.currentTime = time }, frame * 1000 / 15)
      await page.locator('.repertoire').screenshot({ path: join(frames, `${String(frame).padStart(3, '0')}.png`), animations: 'allow' })
    }
    execFileSync('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-y', '-framerate', '15', '-i', join(frames, '%03d.png'), '-filter_complex', '[0:v]fps=12,scale=720:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=64:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle', '-loop', '0', `${directory}gaffer-theme-animation.gif`])
  } finally { await rm(frames, { recursive: true, force: true }) }
  const transitions = await mkdtemp(join(tmpdir(), 'gaffer-transition-'))
  const demos = []
  try {
    for (const theme of ['dark', 'light']) {
      await mkdir(join(transitions, theme))
      const demo = await browser.newPage({ viewport: { width: 1000, height: 1000 } })
      await demo.goto(`${harness.url}/tests/fixtures/gaffer-artwork.html?theme=${theme}`)
      const region = demo.getByRole('region', { name: 'Phosphor bust' })
      await region.locator('image').evaluateAll(images => Promise.all(images.map(async el => {
        const image = new Image(); image.src = el.href.baseVal; await image.decode()
      })))
      await region.evaluate((node, theme) => {
        node.style.cssText = 'position:relative;background:var(--card)'
        const label = document.createElement('span'); label.textContent = theme === 'dark' ? 'Dark' : 'Light'
        label.style.cssText = 'position:absolute;top:4px;left:8px;font:12px var(--mono);color:var(--ink)'
        node.append(label)
      }, theme)
      demos.push({ page: demo, region, theme })
    }
    const states = ['Rest', 'Greeting', 'Reading', 'Tea', 'Biscuit', 'Shrug', 'Steady on', 'Rest']
    for (let frame = 0; frame < 80; frame++) {
      if (frame % 10 === 0) await Promise.all(demos.map(({ page }) => page.evaluate(state => {
        [...document.querySelectorAll('nav button')].find(button => button.textContent === state).click()
      }, states[Math.floor(frame / 10)])))
      await new Promise(resolve => setTimeout(resolve, 50))
      await Promise.all(demos.map(({ region, theme }) => region.screenshot({ path: join(transitions, theme, `${String(frame).padStart(3, '0')}.png`), animations: 'allow' })))
    }
    execFileSync('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-y', '-framerate', '10', '-i', join(transitions, 'dark', '%03d.png'), '-framerate', '10', '-i', join(transitions, 'light', '%03d.png'), '-filter_complex', '[0:v][1:v]hstack,split[a][b];[a]palettegen=max_colors=96:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle', '-loop', '0', `${directory}gaffer-theme-transitions.gif`])
  } finally {
    for (const { page } of demos) await page.close()
    await rm(transitions, { recursive: true, force: true })
  }
  console.log('Saved transparent Gaffer artwork, both themes at desktop/phone sizes, and recordings of the performances and live state transitions.')
} finally {
  await browser.close()
  await harness.close()
}
