import { chromium, expect } from '@playwright/test'
import { startHarness } from './helpers/harness.mjs'

const harness = await startHarness()
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const errors = []
try {
  const page = await browser.newPage()
  page.on('pageerror', error => errors.push(error.message))
  await page.goto(`${harness.url}/tests/fixtures/gaffer-artwork.html`)
  const portrait = page.getByRole('region', { name: 'Grid portrait' }).locator('.gaffer-avatar')
  const bust = page.getByRole('region', { name: 'Phosphor bust' }).locator('.gaffer-avatar')
  await expect(portrait).toHaveAttribute('data-artwork', 'portrait')
  await expect(bust).toHaveAttribute('data-artwork', 'bust')
  for (const avatar of [portrait, bust]) {
    expect(await avatar.locator('.head image').evaluate(async el => {
      const image = new Image(); image.src = el.href.baseVal; await image.decode()
      return image.naturalWidth
    })).toBeGreaterThan(400)
  }
  for (const [button, scene, row] of [['Greeting', 'greeting', 0], ['Reading', 'reading', 1], ['Tea', 'tea', 2]]) {
    await page.getByRole('button', { name: button, exact: true }).click()
    await expect(bust).toHaveAttribute('data-artwork', scene)
    expect(Number(await bust.locator('.frames').getAttribute('y'))).toBeCloseTo(-row * 1024 / 3)
    // Sample every source frame, plus the resting frame after the one-shot gesture ends.
    for (const [time, x] of [[0, 0], [500, -384], [1000, -768], [1500, -1152], [1801, 0]]) {
      expect(await bust.locator('.frames').evaluate((el, time) => {
        const [animation] = el.getAnimations(); animation.pause(); animation.currentTime = time
        return new DOMMatrix(getComputedStyle(el).transform).m41
      }, time)).toBe(x)
    }
  }
  await page.getByRole('button', { name: 'Reading', exact: true }).click()
  await page.getByRole('button', { name: 'Pause', exact: true }).click()
  expect(await bust.evaluate(el => el.getAnimations({ subtree: true }).length)).toBe(0)
  expect(await bust.locator('.frames').evaluate(el => getComputedStyle(el).transform)).toBe('none')
  await page.getByRole('button', { name: 'Resume', exact: true }).click()
  await page.getByRole('button', { name: 'Disable animation', exact: true }).click()
  expect(await bust.evaluate(el => el.getAnimations({ subtree: true }).length)).toBe(0)
  await page.getByRole('button', { name: 'Enable animation', exact: true }).click()
  for (const [button, badge] of [['Error', '.snag'], ['Approval', '.question']]) {
    await page.getByRole('button', { name: button, exact: true }).click()
    await expect(bust).toHaveAttribute('data-artwork', 'bust')
    await expect(bust.locator(badge)).toHaveCount(1)
    await expect(bust.locator('.frames')).toHaveCount(0)
  }
  const reduced = await browser.newPage({ reducedMotion: 'reduce' })
  await reduced.goto(`${harness.url}/tests/fixtures/gaffer-artwork.html`)
  for (const scene of ['Greeting', 'Reading', 'Tea']) {
    await reduced.getByRole('button', { name: scene, exact: true }).click()
    expect(await reduced.locator('.gaffer-avatar').evaluateAll(avatars => avatars.every(el => el.getAnimations({ subtree: true }).length === 0))).toBe(true)
  }
  expect(errors).toEqual([])
  console.log('Gaffer artwork: both approved portraits load; all three strips, frame bounds, resting frames, pause, preference, reduced motion and status priority passed.')
} finally {
  await browser.close()
  await harness.close()
}
