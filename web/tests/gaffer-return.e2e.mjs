import { chromium, expect } from '@playwright/test'
import { startHarness } from './helpers/harness.mjs'

const harness = await startHarness()
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 950 } })
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  await page.clock.install()
  await page.goto(`${harness.url}/tests/fixtures/gaffer-progress.html`)
  const panel = page.getByRole('region', { name: 'Gaffer’s work update' })
  const avatar = panel.locator('.gaffer-avatar')
  await expect(panel).toHaveAttribute('data-phase', 'editing')
  async function returnToWork() {
    await page.getByRole('button', { name: 'Editing', exact: true }).click()
    await page.getByRole('button', { name: 'Leave session', exact: true }).click()
    await page.clock.fastForward(20_000)
    await page.getByRole('button', { name: 'Return to session', exact: true }).click()
    await page.clock.fastForward(120)
    await expect(avatar).toHaveAttribute('data-pose', 'tea')
  }

  await returnToWork()
  const frames = avatar.locator('.frames')
  const duration = await frames.evaluate(el => parseFloat(getComputedStyle(el).animationDuration) * 1000)
  expect(duration).toBeGreaterThan(0)
  // The production welcome timer must not remove the strip halfway through its playback.
  await page.clock.fastForward(duration / 2 + 100)
  await expect(avatar).toHaveAttribute('data-pose', 'tea')
  const continuingSip = await frames.elementHandle()
  await panel.getByRole('button', { name: 'Say ey up to Gaffer' }).click()
  expect(await continuingSip.evaluate(el => el.isConnected)).toBe(true)
  await expect(avatar).toHaveAttribute('data-pose', 'tea')
  // Virtual time controls lifecycle timers; sample CSS playback explicitly as the browser clock is separate.
  expect(await frames.evaluate((el, duration) => {
    const [animation] = el.getAnimations(); animation.pause(); animation.currentTime = duration * .8
    return new DOMMatrix(getComputedStyle(el).transform).m41
  }, duration)).toBe(-1152)
  expect(await frames.evaluate((el, duration) => {
    el.getAnimations()[0].currentTime = duration + 1
    return new DOMMatrix(getComputedStyle(el).transform).m41
  }, duration)).toBe(-1152)
  await expect(avatar).toHaveAttribute('data-pose', 'tea')
  await page.clock.fastForward(duration / 2 + 400)
  await expect(avatar).toHaveAttribute('data-pose', 'duty')
  await expect(avatar).toHaveAttribute('data-reaction', 'startle')

  for (const [state, pose] of [['Error', 'concerned'], ['Approval', 'attention']]) {
    await returnToWork()
    await page.getByRole('button', { name: state, exact: true }).click()
    await expect(avatar).toHaveAttribute('data-pose', pose)
    await expect(avatar.locator('.frames')).toHaveCount(0)
    await page.clock.fastForward(3_000)
    await expect(avatar).toHaveAttribute('data-pose', pose)
    await expect(avatar).not.toHaveAttribute('data-reaction', 'startle')
  }
  await returnToWork()
  await panel.getByRole('button', { name: 'Pause Gaffer animation' }).click()
  await page.clock.fastForward(3_000)
  await expect(avatar).toHaveAttribute('data-pose', 'duty')
  expect(await avatar.evaluate(el => el.getAnimations({ subtree: true }).length)).toBe(0)
  await expect(avatar).not.toHaveAttribute('data-reaction', 'startle')
  expect(errors).toEqual([])
  console.log('Gaffer return: real tea welcome reaches the last frame and rests before startle; errors, approval and pause cancel the pending welcome.')
} finally {
  await browser.close()
  await harness.close()
}
