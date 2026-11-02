import { chromium, expect } from '@playwright/test'
import { startHarness } from './helpers/harness.mjs'

const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const harness = await startHarness()
const errors = []
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 950 } })
  page.on('pageerror', error => errors.push(error.message))
  await page.clock.install()
  await page.goto(`${harness.url}/tests/fixtures/gaffer-progress.html`)
  const panel = page.getByRole('region', { name: 'Gaffer’s work update' })
  await expect(panel).toHaveAttribute('data-phase', 'editing')
  await expect(panel).toContainText('OpenCode making changes')
  await expect(panel.locator('.byline')).toContainText('OpenCode')
  await expect(panel.locator('.byline')).not.toContainText('worker')
  await expect(panel).toContainText('2 done')
  await expect(panel).toContainText('web/src/lib/terminal/GafferProgress.svelte')
  await panel.getByRole('button', { name: /Tool activity/ }).click()
  await expect(panel.getByRole('listitem')).toHaveCount(3)
  await expect(panel).toContainText('Latest tool calls this turn')

  // Banter rotates without an immediate repeat. Pause freezes decoration while facts and time continue updating.
  const first = await panel.locator('.quote').textContent()
  await page.clock.fastForward(13_000)
  await expect(panel.locator('.quote')).not.toHaveText(first)
  await panel.getByRole('button', { name: 'Pause Gaffer animation and quotes' }).click()
  const pausedQuote = await panel.locator('.quote').textContent()
  await page.clock.fastForward(25_000)
  await expect(panel.locator('.quote')).toHaveText(pausedQuote)
  await expect(panel.locator('.elapsed')).not.toHaveText('0s in view')
  const pausedAnimation = await panel.locator('.head').evaluate(el => getComputedStyle(el).animationPlayState)
  expect(pausedAnimation).toBe('paused')
  await panel.getByRole('button', { name: 'Resume Gaffer animation and quotes' }).click()

  // Reconnection replays the same prompt after a transient empty reset; that is not a new turn or a new timer.
  await page.evaluate(() => window.__gaffer.disconnect())
  await expect(panel).toHaveAttribute('data-phase', 'reconnecting')
  await page.clock.fastForward(5_000)
  const elapsedSeconds = text => { const minutes = /^(\d+)m/.exec(text); const seconds = /(\d+)s/.exec(text); return Number(minutes?.[1] ?? 0) * 60 + Number(seconds?.[1] ?? 0) }
  const beforeReplay = elapsedSeconds(await panel.locator('.elapsed').textContent())
  await page.evaluate(() => {
    window.__gaffer.reconnect()
    window.__gaffer.reset()
    window.__gaffer.emit({ seq: 1, kind: 'user', text: 'The same prompt replayed' })
    window.__gaffer.emit({ seq: 2, kind: 'status', state: 'working' })
    window.__gaffer.emit({ seq: 3, kind: 'tool', toolCallId: 'replayed', toolKind: 'edit', status: 'in_progress', path: '/home/dev/reet.sh/web/src/app.css' })
  })
  await expect(panel).toHaveAttribute('data-phase', 'editing')
  expect(elapsedSeconds(await panel.locator('.elapsed').textContent())).toBeGreaterThanOrEqual(beforeReplay)

  for (const [name, phase] of [['Thinking', 'planning'], ['Reading', 'reading'], ['Searching', 'searching'], ['Running', 'running'], ['Approval', 'approval'], ['Question', 'question'], ['Reconnecting', 'reconnecting'], ['Error', 'error'], ['Finished', 'idle']]) {
    await page.getByRole('button', { name, exact: true }).click()
    await expect(panel).toHaveAttribute('data-phase', phase)
    if (phase === 'approval' || phase === 'question' || phase === 'reconnecting' || phase === 'error' || phase === 'idle') {
      await expect(panel.locator('.activity-line')).toHaveCount(0)
      expect(await panel.locator('.head').evaluate(el => getComputedStyle(el).animationName)).toBe('none')
    }
    if (phase === 'reconnecting') {
      await expect(panel).toContainText('Live progress is unavailable')
      await expect(panel).toContainText('Last seen:')
    }
  }

  // Counts and observed time start again for a genuinely new prompt; idle does not claim tests passed.
  await expect(panel).toContainText('Turn finished')
  await page.evaluate(() => {
    window.__gaffer.emit({ seq: 100, kind: 'user', text: 'A different job' })
    window.__gaffer.emit({ seq: 101, kind: 'status', state: 'working' })
  })
  await expect(panel).toHaveAttribute('data-phase', 'planning')
  await expect(panel.getByRole('button', { name: /Tool activity/ })).toHaveCount(0)
  await expect(panel.locator('.elapsed')).toHaveText('0s in view')
  await page.close()

  // Narrow panes in both themes: long targets stay inside the card, controls remain usable, reduced motion is honoured.
  for (const theme of ['light', 'dark']) for (const width of [320, 390, 768]) {
    const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce' })
    const phone = await context.newPage()
    phone.on('pageerror', error => errors.push(error.message))
    await phone.goto(`${harness.url}/tests/fixtures/gaffer-progress.html?theme=${theme}`)
    const card = phone.getByRole('region', { name: 'Gaffer’s work update' })
    await expect(card).toHaveAttribute('data-phase', 'editing')
    expect(await phone.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(await card.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
    expect(await card.locator('.head').evaluate(el => getComputedStyle(el).animationName)).toBe('none')
    const pause = card.getByRole('button', { name: 'Pause Gaffer animation and quotes' })
    expect((await pause.boundingBox()).height).toBeGreaterThanOrEqual(40)
    await card.getByRole('button', { name: /Tool activity/ }).focus()
    await phone.keyboard.press('Enter')
    await expect(card.getByRole('listitem')).toHaveCount(3)
    await phone.getByRole('button', { name: 'Approval', exact: true }).click()
    await expect(card).toHaveAttribute('data-phase', 'approval')
    await expect(phone.getByRole('button', { name: 'Allow once', exact: true })).toBeVisible()
    expect(await phone.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await context.close()
  }
  expect(errors).toEqual([])
  console.log('Gaffer: state, rotation, pause, timer, replay, keyboard and six responsive/theme checks passed.')
} finally {
  await browser.close()
  await harness.close()
}
