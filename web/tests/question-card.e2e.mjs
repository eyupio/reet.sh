// The typed-question card on its own fixture page (tests/fixtures/question-card.html): the real chat pane with a scripted stream and answers that
// are only recorded. It checks the card as a person with a phone or a screen reader meets it: the accessibility tree, keyboard order, and on an
// emulated touch phone that every control fits the screen and is large enough to press. Chromium emulation, not a device: the same fixture is
// served on the network for a real phone (`npm run fixture:questions`), and docs/device-checks.md lists what to try there.
import { chromium, expect } from '@playwright/test'
import { startHarness } from './helpers/harness.mjs'

const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const harness = await startHarness()
const URL = `${harness.url}/tests/fixtures/question-card.html`
const errors = []
async function open(options = {}) {
  const context = await browser.newContext(options)
  const page = await context.newPage()
  page.on('pageerror', e => errors.push(e.message))
  await page.goto(URL)
  await page.getByRole('button', { name: 'Start a turn', exact: true }).click()
  return { context, page }
}
try {
  // Desktop: raise a question, find it by role, answer it with the keyboard.
  {
    const { context, page } = await open({ viewport: { width: 1100, height: 900 } })
    await page.getByRole('button', { name: 'Ask: choose one', exact: true }).click()
    const card = page.getByRole('group', { name: 'Which approach should I take?' })
    await expect(card).toBeVisible()
    // What a screen reader is given: one group named by the question, labelled controls, and the actions as buttons.
    await expect(card.getByRole('radio', { name: /^Approach A/ })).toBeVisible()
    await expect(card.getByRole('radio', { name: /^Approach B/ })).toBeVisible()
    await expect(card.getByRole('textbox', { name: /Anything else/ })).toBeVisible()
    for (const name of ['Cancel question', 'Skip', 'Send answer']) await expect(card.getByRole('button', { name, exact: true })).toBeVisible()
    // Keyboard: choose with the arrow keys, press Send answer from the keyboard.
    await card.getByRole('radio', { name: /^Approach A/ }).focus()
    await page.keyboard.press('Space')
    await expect(card.getByRole('radio', { name: /^Approach A/ })).toBeChecked()
    await card.getByRole('button', { name: 'Send answer', exact: true }).focus()
    await page.keyboard.press('Enter')
    await expect(page.getByTestId('answers')).toContainText('"action":"accept"')
    await expect(page.getByTestId('answers')).toContainText('"approach":"a"')
    // Once the agent has the answer the card is decided and keeps no controls; the next question then takes its place.
    await page.getByRole('button', { name: 'Agent saw the answer', exact: true }).click()
    await expect(card.getByRole('button', { name: 'Send answer', exact: true })).toHaveCount(0)
    // Not choosing is explained, not silent.
    await page.getByRole('button', { name: 'Ask: choose one', exact: true }).click()
    const second = page.getByRole('group', { name: 'Which approach should I take?' }).last()
    await second.getByRole('button', { name: 'Send answer', exact: true }).click()
    await expect(second.getByText('Choose one.')).toBeVisible()
    await context.close()
  }
  // Phone: touch, 390 wide. Nothing scrolls sideways, every control is inside the screen, and every control is at least 40 px tall.
  {
    const { context, page } = await open({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true, deviceScaleFactor: 3 })
    await page.getByRole('button', { name: 'Ask: choose several', exact: true }).tap()
    const card = page.getByRole('group', { name: 'Which checks should I run?' })
    await expect(card).toBeVisible()
    await page.getByRole('button', { name: 'Ask: long text', exact: true }).tap()
    const long = page.getByRole('group', { name: /^This is a deliberately long question/ })
    await expect(long).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const small = []
    for (const group of [card, long]) {
      const controls = group.locator('input:not([type=hidden]), textarea, button, select, [role=radio], [role=checkbox]')
      for (let i = 0; i < await controls.count(); i++) {
        const control = controls.nth(i)
        // A checkbox or radio is pressed through its label; measure what is pressed.
        const box = await control.evaluate(el => {
          const target = el.closest('label') ?? el
          const r = target.getBoundingClientRect()
          return { x: r.x, w: r.width, h: r.height, name: (el.getAttribute('aria-label') || el.textContent || el.getAttribute('type') || el.tagName).trim().slice(0, 30) }
        })
        expect(box.x, `${box.name} starts inside the screen`).toBeGreaterThanOrEqual(0)
        expect(box.x + box.w, `${box.name} ends inside the screen`).toBeLessThanOrEqual(391)
        if (box.h < 40) small.push(`${box.name} (${Math.round(box.h)}px)`)
      }
    }
    // Reported, not hidden: the controls pressed by touch that are under 40 px tall.
    console.log('touch targets under 40px tall:', small.length ? small.join(', ') : 'none')
    expect(small, 'controls too small to press on a phone').toEqual([])
    // Answer the multi-select by touch.
    await card.getByRole('checkbox', { name: 'Lint' }).tap()
    await card.getByRole('checkbox', { name: 'Build' }).tap()
    await card.getByRole('button', { name: 'Send answer', exact: true }).tap()
    await expect(page.getByTestId('answers')).toContainText('"checks":["x","z"]')
    await context.close()
  }
  expect(errors).toEqual([])
  console.log('Question card fixture passed: roles and names for a screen reader, keyboard answer, the unanswered explanation, and a 390 px touch phone with no sideways scroll, every control on screen and at least 40 px tall.')
} finally {
  await browser.close()
  await harness.close()
}
