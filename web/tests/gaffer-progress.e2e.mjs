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
  expect(await panel.locator('.head image').evaluate(async el => {
    const portrait = new Image(); portrait.src = el.href.baseVal; await portrait.decode()
    return portrait.naturalWidth
  })).toBeGreaterThan(0)
  expect(await panel.locator('.gaffer-avatar').evaluate(el => el.getAnimations({ subtree: true }).length)).toBeGreaterThanOrEqual(1)
  await expect(panel.getByRole('region', { name: 'Gaffer’s job sheet' })).toHaveCount(0)
  await expect(panel.locator('.handover')).toHaveCount(0)
  await panel.getByRole('button', { name: /Tool activity/ }).click()
  await expect(panel.getByRole('listitem')).toHaveCount(3)
  await expect(panel).toContainText('Latest tool calls this turn')

  // Sparse banter continues when motion is paused. Facts, timers and keyboard interactions stay live.
  const first = await panel.locator('.quote').textContent()
  await page.clock.fastForward(10_000)
  await expect(panel.locator('.quote')).toHaveText(first)
  // Playwright's virtual clock does not advance CSS animation time. Let the one-shot gesture settle.
  await page.waitForTimeout(2_100)
  expect(await panel.locator('.gaffer-avatar').evaluate(el => el.getAnimations({ subtree: true }).filter(animation => !animation.effect.target.closest('.eyes')).length)).toBe(0)
  await page.clock.fastForward(22_000)
  await expect(panel.locator('.quote')).not.toHaveText(first)
  await panel.getByRole('button', { name: 'Pause Gaffer animation' }).click()
  const pausedQuote = await panel.locator('.quote').textContent()
  await page.clock.fastForward(32_000)
  await expect(panel.locator('.quote')).not.toHaveText(pausedQuote)
  await expect(panel.locator('.elapsed')).not.toHaveText('0s in view')
  expect(await panel.locator('.gaffer-avatar').evaluate(el => el.getAnimations({ subtree: true }).length === 0)).toBe(true)
  await panel.getByRole('button', { name: 'Resume Gaffer animation' }).click()

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
      await expect(panel.locator('.gaffer-avatar')).toHaveAttribute('data-pose', ['approval', 'question'].includes(phase) ? 'attention' : ['error', 'reconnecting'].includes(phase) ? 'concerned' : 'duty')
    }
    if (phase === 'reconnecting') {
      await expect(panel).toContainText('Live progress is unavailable')
      await expect(panel).toContainText('Last seen:')
    }
  }

  // Counts and observed time start again for a genuinely new prompt; idle does not claim tests passed.
  await expect(panel).toContainText('Turn finished')
  // The summary is Gaffer speaking in his own card: no separate executive-summary section exists.
  await expect(panel.getByRole('region', { name: 'Gaffer’s executive summary' })).toHaveCount(0)
  const summary = panel.getByRole('region', { name: 'Gaffer’s job sheet' })
  await expect(summary).toHaveCount(0)
  await expect(panel.locator('.handover')).toContainText('front-facing tweed flat cap, with calmer gestures')
  await expect(panel.locator('.chips')).toContainText('4 done')
  const sheetButton = panel.getByRole('button', { name: 'Job sheet' })
  await expect(sheetButton).toHaveAttribute('aria-expanded', 'false')
  await sheetButton.click()
  await expect(summary).toContainText('This turn')
  await expect(summary).toContainText('Files touched')
  await expect(summary).toContainText('Commands & results (1)')
  await expect(summary).toContainText('npm run check')
  await expect(summary).not.toContainText('All tests passed')
  await sheetButton.click()
  await expect(summary).toHaveCount(0)
  await page.getByRole('button', { name: 'Snag', exact: true }).click()
  await expect(panel.locator('.chips')).toContainText('1 failed')
  await expect(panel.locator('.supervising')).toContainText('Needs a look')
  await sheetButton.click()
  await expect(summary).toContainText('Needs a look')
  await expect(summary).toContainText('1 tool call reported failure')
  await expect(panel.locator('.handover')).toContainText('One fixture check still needs attention')
  await page.getByRole('button', { name: 'Ended', exact: true }).click()
  await expect(panel).toHaveAttribute('data-phase', 'ended')
  await expect(sheetButton).toHaveAttribute('aria-expanded', 'false')
  await sheetButton.click()
  await expect(summary).toContainText('Whole session')
  await expect(summary).toContainText('Start a new session')
  await expect(panel.getByRole('button', { name: 'Pause Gaffer animation' })).toBeVisible()
  await page.getByRole('button', { name: 'Finished', exact: true }).click()
  await page.evaluate(() => {
    window.__gaffer.emit({ seq: 100, kind: 'user', text: 'A different job' })
    window.__gaffer.emit({ seq: 101, kind: 'status', state: 'working' })
  })
  await expect(panel).toHaveAttribute('data-phase', 'planning')
  await expect(summary).toHaveCount(0)
  await expect(panel.getByRole('button', { name: /Tool activity/ })).toHaveCount(0)
  await expect(panel.locator('.elapsed')).toHaveText('0s in view')

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
    const pause = card.getByRole('button', { name: 'Pause Gaffer animation' })
    expect((await pause.boundingBox()).height).toBeGreaterThanOrEqual(40)
    await card.getByRole('button', { name: /Tool activity/ }).focus()
    await phone.keyboard.press('Enter')
    await expect(card.getByRole('listitem')).toHaveCount(3)
    await phone.getByRole('button', { name: 'Approval', exact: true }).click()
    await expect(card).toHaveAttribute('data-phase', 'approval')
    await expect(phone.getByRole('button', { name: 'Allow once', exact: true })).toBeVisible()
    expect(await phone.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await phone.getByRole('button', { name: 'Finished', exact: true }).click()
    await expect(card.locator('.chips')).toContainText('4 done')
    const jobSheet = card.getByRole('button', { name: 'Job sheet' })
    expect((await jobSheet.boundingBox()).height).toBeGreaterThanOrEqual(width < 541 ? 40 : 28)
    await jobSheet.focus()
    await phone.keyboard.press('Enter')
    const recap = card.getByRole('region', { name: 'Gaffer’s job sheet' })
    await expect(recap).toBeVisible()
    expect(await recap.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
    await expect(recap.getByText('npm run check', { exact: true })).toBeVisible()
    expect(await card.locator('.gaffer-avatar').evaluate(el => el.getAnimations({ subtree: true }).length)).toBe(0)
    await expect(phone.getByRole('textbox', { name: /Message/ })).toBeVisible()
    await context.close()
  }
  // Clicks are accessible, contextual and rate limited; they never issue a request to the agent.
  const portrait = panel.getByRole('button', { name: 'Say ey up to Gaffer' })
  await page.getByRole('button', { name: 'Editing', exact: true }).click()
  await portrait.focus(); await page.keyboard.press('Enter')
  await expect(panel.locator('.quote')).toContainText('OpenCode')
  const clicked = await panel.locator('.quote').textContent()
  await portrait.click()
  await expect(panel.locator('.quote')).toHaveText(clicked)
  await page.clock.fastForward(2_100)
  await portrait.click()
  await expect(panel.locator('.quote')).not.toHaveText(clicked)

  // Returning after leaving a session works across component unmount, not just a hidden browser tab.
  for (const [scene, pose, reaction] of [['Editing', 'tea', 'startle'], ['Ended', 'asleep', 'stretch'], ['Error', 'concerned', 'shake'], ['Approval', 'attention', 'listen']]) {
    await page.getByRole('button', { name: scene, exact: true }).click()
    await page.getByRole('button', { name: 'Leave session', exact: true }).click()
    await page.clock.fastForward(20_000)
    await page.getByRole('button', { name: 'Return to session', exact: true }).click()
    await page.clock.fastForward(120)
    await expect(panel.locator('.gaffer-avatar')).toHaveAttribute('data-pose', pose)
    if (pose === 'tea' || pose === 'asleep') {
      const duration = pose === 'tea' ? await panel.locator('.frames').evaluate(el => parseFloat(getComputedStyle(el).animationDuration) * 1000) : 600
      await page.clock.fastForward(duration + 400)
      await expect(panel.locator('.gaffer-avatar')).toHaveAttribute('data-pose', 'duty')
    }
    await expect(panel.locator('.gaffer-avatar')).toHaveAttribute('data-reaction', reaction)
    if (scene === 'Ended') await expect(panel.locator('.handover')).toBeVisible()
  }

  await page.getByRole('button', { name: 'Editing', exact: true }).click()
  await page.getByRole('button', { name: 'Disable animation', exact: true }).click()
  expect(await page.locator('.gaffer-avatar').evaluateAll(avatars => avatars.every(el => el.getAnimations({ subtree: true }).length === 0))).toBe(true)
  const stillQuote = await panel.locator('.quote').textContent()
  await page.clock.fastForward(32_000)
  await expect(panel.locator('.quote')).not.toHaveText(stillQuote)
  await page.getByRole('button', { name: 'Finished', exact: true }).click()
  await expect(panel.locator('.handover')).toBeVisible()
  await page.getByRole('button', { name: 'Enable animation', exact: true }).click()
  await page.close()
  expect(errors).toEqual([])
  console.log('Gaffer: animation, pause, replay, handover-in-card summaries, failures, session end, keyboard and six responsive/theme checks passed.')
} finally {
  await browser.close()
  await harness.close()
}
