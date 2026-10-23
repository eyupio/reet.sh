import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'

const base = process.env.REET_BROWSER_URL
if (!base || !process.env.REET_BROWSER_COOKIE) throw new Error('Run through the real Go terminal integration fixture')
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || undefined, args: ['--no-sandbox'] })
const context = await browser.newContext({ viewport: { width: 1360, height: 1000 } })
await context.addCookies([{ name: 'reet_session', value: process.env.REET_BROWSER_COOKIE, url: base, httpOnly: true, sameSite: 'Lax' }])
const page = await context.newPage()
const errors = []
let sockets = 0
page.on('websocket', () => sockets++)
page.on('pageerror', error => errors.push(error.message))
const pane = page.getByRole('region', { name: 'Terminal on laptop' })
const screen = page.locator('.xterm-accessibility-tree')
async function live() { await expect(pane).toHaveAttribute('data-connection-state', 'live', { timeout: 20000 }) }
async function command(text) { await page.getByRole('button', { name: 'Focus terminal' }).click(); await page.keyboard.type(text); await page.keyboard.press('Enter') }
async function state() { return page.evaluate(async () => (await fetch('/_test/state')).json()) }

try {
  await page.goto(`${base}/machines?view=terminal&agent=${encodeURIComponent(process.env.REET_BROWSER_AGENT)}`)
  await expect(page.getByRole('heading', { name: 'Your machine. Your shell.' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'New terminal' })).toBeEnabled()
  await page.getByLabel('New session name').fill('Browser durability')
  await page.getByRole('button', { name: 'New terminal', exact: true }).click()
  await live()
  await expect(screen).toContainText('stage6-ready>')
  await expect(page.getByLabel('Machine health')).toContainText('42.0')
  await command("sleep 300 & LONG_PID=$!; printf '__browser_%s__ %s\\n' 'started' \"$LONG_PID\"")
  await expect(screen).toContainText('__browser_started__')
  const original = await state()
  expect(original.sessions).toBe(1)
  expect(Number(original.pid)).toBeGreaterThan(0)
  await page.reload()
  await live()
  await command("kill -0 \"$LONG_PID\" && printf '__browser_%s__\\n' 'resumed'")
  await expect(screen).toContainText('__browser_resumed__')
  expect(await state()).toEqual(original)
  const beforeRestart = sockets
  await page.evaluate(async () => { const response = await fetch('/_test/restart-agent', { method: 'POST' }); if (!response.ok) throw new Error('test agent restart failed') })
  await expect.poll(() => sockets, { timeout: 20000 }).toBeGreaterThan(beforeRestart)
  await live()
  await command("kill -0 \"$LONG_PID\" && printf '__browser_%s__\\n' 'agent-survived'")
  await expect(screen).toContainText('__browser_agent-survived__')
  expect(await state()).toEqual(original)
  await command("printf '\\360\\237\\222\\245 __split_%s__\\n' 'utf8'")
  await expect(screen).toContainText('💥 __split_utf8__')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  expect(errors).toEqual([])
  if (process.env.REET_BROWSER_ARTIFACT_DIR) { await mkdir(process.env.REET_BROWSER_ARTIFACT_DIR, { recursive: true }); await page.screenshot({ path: `${process.env.REET_BROWSER_ARTIFACT_DIR}/terminal-desktop.png`, fullPage: true }) }
  console.log(`Browser terminal passed: original shell PID ${original.pid}, one logical session, long job preserved through browser refresh and agent reconnect.`)
} finally { await browser.close() }
