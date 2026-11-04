import { chromium, expect } from '@playwright/test'
import { startHarness } from './helpers/harness.mjs'

const shots = process.env.GAFFER_SHOTS || ''
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const harness = await startHarness()
const errors = []
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
  page.on('pageerror', error => errors.push(error.message))
  const open = (scene, compact) => page.goto(`${harness.url}/tests/fixtures/gaffer-progress.html?scene=${scene}${compact ? '&compact=1' : ''}`)
  const card = page.getByRole('region', { name: 'Gaffer’s work update' })
  const shot = async name => { if (shots) await card.screenshot({ path: `${shots}/${name}.png` }) }

  // Starts as a full card; the card's own button folds it to one line that still speaks in Gaffer's voice.
  await open('Finished')
  await expect(card).not.toHaveAttribute('data-mode', 'minimised')
  await expect(card.getByRole('button', { name: 'Job sheet' })).toBeVisible()
  await shot('full')
  await card.getByRole('button', { name: 'Minimise Gaffer’s card to one line' }).click()
  await expect(card).toHaveAttribute('data-mode', 'minimised')
  await expect(card.getByRole('button', { name: 'Job sheet' })).toHaveCount(0)
  await expect(card.locator('.mini-status')).toContainText('Turn finished')
  await expect(card.locator('.mini-quote')).toContainText('“')
  await expect(card.locator('.mini-count')).toContainText('done')
  const height = (await card.boundingBox()).height
  expect(height).toBeLessThan(60)
  // One line: the quote is ellipsised, never wrapped, and its full text is on hover.
  expect(await card.locator('.mini-quote').evaluate(el => getComputedStyle(el).whiteSpace)).toBe('nowrap')
  expect(await card.locator('.mini-quote').getAttribute('title')).toBeTruthy()
  await shot('minimised')
  // Exactly one live status while minimised, and the portrait still answers a poke.
  await expect(card.locator('[role=status]')).toHaveCount(1)
  await card.getByRole('button', { name: 'Say ey up to Gaffer' }).click()
  await expect(card.locator('.mini-quote')).toHaveAttribute('aria-live', 'polite')
  await card.getByRole('button', { name: 'Expand Gaffer’s card' }).click()
  await expect(card).not.toHaveAttribute('data-mode', 'minimised')
  await expect(card.getByRole('button', { name: 'Job sheet' })).toBeVisible()

  // The account default opens a card minimised, and a card can still be expanded for itself.
  await open('Finished', true)
  await expect(card).toHaveAttribute('data-mode', 'minimised')
  await card.getByRole('button', { name: 'Expand Gaffer’s card' }).click()
  await expect(card.getByRole('button', { name: 'Job sheet' })).toBeVisible()

  // A snag and a decision keep the one line, with their tone.
  await open('Snag', true)
  await expect(card.locator('.mini-count')).toContainText('failed')
  await shot('snag')
  await open('Approval', true)
  await expect(card).toHaveClass(/needs-you/)
  await expect(card.locator('.mini-status')).not.toBeEmpty()
  await shot('needs-you')

  // Phone width keeps one line.
  const phone = await browser.newPage({ viewport: { width: 390, height: 800 } })
  await phone.goto(`${harness.url}/tests/fixtures/gaffer-progress.html?scene=Finished&compact=1`)
  const small = phone.getByRole('region', { name: 'Gaffer’s work update' })
  await expect(small).toHaveAttribute('data-mode', 'minimised')
  expect((await small.boundingBox()).height).toBeLessThan(64)
  expect(await phone.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  if (shots) await small.screenshot({ path: `${shots}/phone.png` })

  // A decided approval is a pill on its own tool row, not a second card.
  await open('Finished')
  await expect(page.locator('.tool').filter({ hasText: 'npm run check' })).toBeVisible() // the fixture's own scene has settled
  await page.evaluate(() => {
    const s = window.__gaffer
    s.emit({ seq: 1001, kind: 'tool', toolCallId: 'pill1', toolKind: 'execute', title: 'sleep 180', command: 'sleep 180; gh pr view 486', status: 'in_progress' })
    s.emit({ seq: 1002, kind: 'permission', permissionId: 'pill-p', toolCallId: 'pill1', toolKind: 'execute', title: 'sleep 180', command: 'sleep 180; gh pr view 486', text: 'Poll the PR checks.' })
  })
  await expect(page.locator('.approval').filter({ hasText: 'sleep 180' })).toBeVisible()
  await page.evaluate(() => window.__gaffer.emit({ seq: 1003, kind: 'permission_resolved', permissionId: 'pill-p', allow: true }))
  const row = page.locator('.tool').filter({ hasText: 'sleep 180' })
  await expect(page.locator('.approval')).toHaveCount(0)
  await expect(row.locator('.pill')).toHaveText('Allowed')
  await row.locator('.tool-head').click()
  await expect(row.locator('.approval-note')).toContainText('Allowed once.')
  await expect(row.locator('.approval-note')).toContainText('Poll the PR checks.')
  if (shots) await row.screenshot({ path: `${shots}/pill.png` })
  await page.evaluate(() => {
    const s = window.__gaffer
    s.emit({ seq: 1004, kind: 'tool', toolCallId: 'pill2', toolKind: 'execute', title: 'rm -rf build', command: 'rm -rf build', status: 'cancelled' })
    s.emit({ seq: 1005, kind: 'permission', permissionId: 'pill-q', toolCallId: 'pill2', toolKind: 'execute', title: 'rm -rf build', command: 'rm -rf build' })
    s.emit({ seq: 1006, kind: 'permission_resolved', permissionId: 'pill-q', allow: false })
  })
  await expect(page.locator('.tool').filter({ hasText: 'rm -rf build' }).locator('.pill')).toHaveText('Denied')

  expect(errors).toEqual([])
  console.log('Gaffer minimise: one-line row, per-card toggle, account default, tone, phone fit, and approval pills passed.')
} finally {
  await browser.close()
  await harness.close?.()
}
