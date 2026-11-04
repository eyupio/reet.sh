import { chromium, expect } from '@playwright/test'
import { startHarness } from './helpers/harness.mjs'

const harness = await startHarness()
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const errors = []
try {
  const page = await browser.newPage({ viewport: { width: 1100, height: 1000 }, reducedMotion: 'reduce' })
  page.on('pageerror', error => errors.push(error.message))
  await page.goto(`${harness.url}/tests/fixtures/gaffer-artwork.html?theme=light`)
  const avatar = page.getByRole('region', { name: 'Phosphor bust' }).locator('.gaffer-avatar')
  const stableAvatar = await avatar.elementHandle()
  async function inkPixels() {
    return avatar.evaluate(async node => {
      const bounds = node.getBoundingClientRect(), width = Math.round(bounds.width), height = Math.round(bounds.height)
      const clone = node.cloneNode(true)
      const source = [node, ...node.querySelectorAll('*')], target = [clone, ...clone.querySelectorAll('*')]
      // Inline the actual computed SVG presentation and embed its images, keeping the canvas origin-clean.
      const properties = ['color', 'fill', 'fill-opacity', 'stroke', 'stroke-width', 'stroke-linejoin', 'stroke-linecap', 'opacity', 'transform', 'transform-origin', 'transform-box', 'mix-blend-mode', 'isolation', 'overflow', 'flood-color', 'flood-opacity', 'filter']
      for (let i = 0; i < source.length; i++) {
        const style = getComputedStyle(source[i])
        for (const property of properties) target[i].style.setProperty(property, style.getPropertyValue(property))
        if (source[i].tagName === 'image') {
          const blob = await (await fetch(source[i].href.baseVal)).blob()
          const uri = await new Promise(resolve => { const reader = new FileReader(); reader.onload = () => resolve(reader.result); reader.readAsDataURL(blob) })
          target[i].setAttribute('href', uri)
        }
      }
      clone.setAttribute('width', width); clone.setAttribute('height', height)
      const url = URL.createObjectURL(new Blob([new XMLSerializer().serializeToString(clone)], { type: 'image/svg+xml' }))
      try {
        const image = new Image(); image.src = url; await image.decode()
        const canvas = document.createElement('canvas'); canvas.width = width; canvas.height = height
        const context = canvas.getContext('2d'); context.drawImage(image, 0, 0)
        const data = context.getImageData(0, 0, width, height).data
        const ink = getComputedStyle(node).color.match(/[\d.]+/g).slice(0, 3).map(Number)
        let drawn = 0, matching = 0
        for (let i = 0; i < data.length; i += 4) if (data[i + 3] > 200) {
          drawn++
          if (ink.every((channel, c) => Math.abs(channel - data[i + c]) <= 2)) matching++
        }
        const topAlpha = data[(5 * width + Math.floor(width / 2)) * 4 + 3]
        return { topAlpha, drawn, ink, fraction: drawn ? matching / drawn : 0 }
      } finally { URL.revokeObjectURL(url) }
    })
  }
  const palettes = []
  for (const theme of ['light', 'dark']) {
    await page.evaluate(theme => { document.documentElement.dataset.theme = theme }, theme)
    for (const state of ['Rest', 'Greeting', 'Reading', 'Tea', 'Biscuit', 'Shrug', 'Steady on']) {
      await page.getByRole('button', { name: state, exact: true }).click()
      const pixels = await inkPixels()
      expect(pixels.topAlpha, `${theme}/${state}: no screen behind the drawing`).toBe(0)
      expect(pixels.drawn, `${theme}/${state}: visible ink remains`).toBeGreaterThan(500)
      expect(pixels.fraction, `${theme}/${state}: artwork uses the app's ink colour`).toBeGreaterThan(.98)
      palettes.push(pixels.ink)
    }
  }
  expect(palettes[0]).not.toEqual(palettes[7])
  expect(await stableAvatar.evaluate(node => node.isConnected)).toBe(true)
  await page.evaluate(() => delete document.documentElement.dataset.theme)
  await page.emulateMedia({ colorScheme: 'light', reducedMotion: 'reduce' })
  const systemLight = await inkPixels()
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
  const systemDark = await inkPixels()
  expect(systemLight.ink).not.toEqual(systemDark.ink)
  expect(systemLight.topAlpha).toBe(0); expect(systemDark.topAlpha).toBe(0)

  const moving = await browser.newPage({ viewport: { width: 1100, height: 1000 } })
  moving.on('pageerror', error => errors.push(error.message))
  await moving.goto(`${harness.url}/tests/fixtures/gaffer-artwork.html`)
  const movingAvatar = moving.getByRole('region', { name: 'Phosphor bust' }).locator('.gaffer-avatar')
  const wrapper = await movingAvatar.elementHandle()
  const now = new Date()
  await moving.clock.install({ time: now }); await moving.clock.pauseAt(new Date(now.getTime() + 1000))
  await moving.getByRole('button', { name: 'Greeting', exact: true }).click()
  await expect(movingAvatar.locator('.scene.leaving')).toHaveCount(1)
  await expect(movingAvatar.locator('.scene:not(.leaving)')).toHaveCount(1)
  const outgoing = await movingAvatar.locator('.scene.leaving').elementHandle()
  const fading = await outgoing.evaluate(node => {
    const animation = node.getAnimations().find(a => a.animationName?.includes('scene-out'))
    animation.pause(); animation.currentTime = 110
    return Number(getComputedStyle(node).opacity)
  })
  expect(fading).toBeGreaterThan(0); expect(fading).toBeLessThan(1)
  expect(await wrapper.evaluate(node => node.isConnected)).toBe(true)
  const originalFade = await outgoing.evaluate(node => getComputedStyle(node).animationName)
  await moving.getByRole('button', { name: 'Reading', exact: true }).click()
  const resumed = await outgoing.evaluate(node => {
    const animation = node.getAnimations().find(a => a.animationName?.includes('scene-out'))
    const name = animation.animationName
    animation.pause(); animation.currentTime = 0
    return { name, opacity: Number(getComputedStyle(node).opacity) }
  })
  expect(resumed.name).not.toBe(originalFade)
  expect(resumed.opacity).toBeCloseTo(fading, 2)
  for (const state of ['Biscuit', 'Tea', 'Shrug']) {
    await moving.getByRole('button', { name: state, exact: true }).click()
    expect(await movingAvatar.locator('.scene').count()).toBeLessThanOrEqual(4)
  }
  await moving.clock.fastForward(250)
  await expect(movingAvatar.locator('.scene')).toHaveCount(1)
  await moving.getByRole('button', { name: 'Reading', exact: true }).click()
  await moving.getByRole('button', { name: 'Error', exact: true }).click()
  await expect(movingAvatar.locator('.scene')).toHaveCount(1)
  await expect(movingAvatar.locator('.snag')).toHaveCount(1)
  await expect(movingAvatar.locator('.frames')).toHaveCount(0)
  await moving.getByRole('button', { name: 'Greeting', exact: true }).click()
  await moving.getByRole('button', { name: 'Pause', exact: true }).click()
  await expect(movingAvatar.locator('.scene')).toHaveCount(1)
  expect(await movingAvatar.evaluate(node => node.getAnimations({ subtree: true }).length)).toBe(0)
  await moving.getByRole('button', { name: 'Resume', exact: true }).click()
  await moving.emulateMedia({ reducedMotion: 'reduce' })
  await moving.getByRole('button', { name: 'Reading', exact: true }).click()
  await expect(movingAvatar.locator('.scene')).toHaveCount(1)
  expect(await movingAvatar.evaluate(node => node.getAnimations({ subtree: true }).length)).toBe(0)
  await moving.emulateMedia({ reducedMotion: 'no-preference' })
  await moving.getByRole('button', { name: 'Greeting', exact: true }).click()
  await moving.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, value: true })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await expect(movingAvatar.locator('.scene')).toHaveCount(1)
  expect(await movingAvatar.evaluate(node => node.getAnimations({ subtree: true }).length)).toBe(0)

  // Work-card completion must not resize the button or swap to a differently framed portrait.
  const cardPage = await browser.newPage({ viewport: { width: 1280, height: 1000 }, reducedMotion: 'reduce' })
  cardPage.on('pageerror', error => errors.push(error.message))
  await cardPage.goto(`${harness.url}/tests/fixtures/gaffer-progress.html`)
  const card = cardPage.getByRole('region', { name: 'Gaffer’s work update' })
  await expect(card).toHaveAttribute('data-phase', 'editing')
  let sequence = 100
  for (const width of [1280, 390]) {
    await cardPage.setViewportSize({ width, height: 1000 })
    // Actual stream updates, rather than the fixture buttons which deliberately reset/replay the whole chat.
    await cardPage.evaluate(({ seq, width }) => {
      window.__gaffer.emit({ seq, kind: 'user', text: 'Keep the avatar aligned while this job finishes.' })
      window.__gaffer.emit({ seq: seq + 1, kind: 'status', state: 'working' })
      window.__gaffer.emit({ seq: seq + 2, kind: 'tool', toolCallId: `alignment-${width}`, toolKind: 'edit', path: '/project/page.ts', status: 'in_progress' })
    }, { seq: sequence, width })
    sequence += 3
    await expect(card).toHaveAttribute('data-phase', 'editing')
    const button = card.getByRole('button', { name: 'Say ey up to Gaffer' })
    const before = await button.boundingBox(), cardAvatar = await card.locator('.gaffer-avatar').elementHandle()
    await cardPage.evaluate(({ seq, width }) => {
      window.__gaffer.emit({ seq, kind: 'tool', toolCallId: `alignment-${width}`, toolKind: 'edit', path: '/project/page.ts', status: 'completed' })
      window.__gaffer.emit({ seq: seq + 1, kind: 'status', state: 'idle' })
    }, { seq: sequence, width })
    sequence += 2
    await expect(card).toHaveAttribute('data-phase', 'idle')
    const after = await button.boundingBox()
    expect(after.width).toBe(before.width); expect(after.height).toBe(before.height)
    expect(await cardAvatar.evaluate(node => node.isConnected)).toBe(true)
    await expect(card.locator('.gaffer-avatar')).toHaveAttribute('data-artwork', 'bust')
    expect(await cardPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
  expect(errors).toEqual([])
  console.log('Gaffer: transparent pixels and matching app ink across all artwork in both themes, live/system theme switching, stable wrapper, actual blends, bounded layers, cleanup, immediate errors and motion-off passed.')
} finally {
  await browser.close()
  await harness.close()
}
