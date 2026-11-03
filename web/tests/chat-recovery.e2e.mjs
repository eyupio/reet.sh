import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { join } from 'node:path'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-recovery000000001', S = 'session-recovery000000000001', NEXT = 'session-recovery000000000002'
const original = { id: S, agentId: A, title: 'OpenCode · recovery', state: 'lost', createdAt: Date.now(), updatedAt: Date.now(), exitCode: null, tool: 'opencode', model: 'provider/model', dir: '/home/dev/site', structured: true, acpSessionId: 'ses_original000000000000000001' }
const descriptor = { tool: 'opencode', adapter: 'opencode', state: 'available', history: true, model: true }
const machine = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, readiness: 'ready', launch: true, agentsV2: true, tools: ['opencode'], acp: true, acpAgents: [descriptor] } }
let row = { ...original }, next = null, failure = null
const requests = []
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { context, page, errors } = await openWorkspace(browser, {
  width: 1360, height: 900, terminalStatus: active, terminalMachines: [machine],
  routes: {
    '/api/v2/terminal/sessions': req => {
      if (req.method() === 'POST') {
        requests.push(req.postDataJSON())
        if (failure) return failure
        next = { ...row, id: NEXT, state: 'running' }
        return next
      }
      return { sessions: next ? [row, next] : [row] }
    },
  },
})
await page.addInitScript(() => {
  window.EventSource = class {
    constructor() { setTimeout(() => this.onopen?.({}), 0) }
    addEventListener() {}
    close() {}
  }
})
const banner = page.locator('.ended-note')
const recover = banner.getByRole('button', { name: 'Continue with history', exact: true })
async function open() { await page.goto(`${base}/sessions?agent=${A}&session=${S}`); await expect(banner).toBeVisible() }
try {
  for (const [state, width, theme] of [['lost', 1360, 'light'], ['lost', 390, 'dark'], ['exited', 1360, 'dark'], ['ended', 320, 'light']]) {
    row = { ...original, state }; next = null
    const before = requests.length
    await page.setViewportSize({ width, height: 900 })
    await open()
    await page.evaluate(theme => document.documentElement.dataset.theme = theme, theme)
    await expect(recover).toBeEnabled()
    await expect(banner).toContainText('Any interrupted reply needs a new prompt.')
    await expect(banner).not.toContainText('could not be restored')
    const buttonBox = await recover.boundingBox()
    expect(buttonBox.x).toBeGreaterThanOrEqual(0)
    expect(buttonBox.x + buttonBox.width).toBeLessThanOrEqual(width)
    if (process.env.REET_UI_SCREENSHOTS) {
      await mkdir(process.env.REET_UI_SCREENSHOTS, { recursive: true })
      await page.screenshot({ path: join(process.env.REET_UI_SCREENSHOTS, `chat-recovery-${state}-${width}-${theme}.png`), fullPage: true })
    }
    expect(requests).toHaveLength(before)
    await recover.click()
    await page.waitForURL(new RegExp(`session=${NEXT}`))
    expect(requests).toHaveLength(before + 1)
    expect(requests.at(-1)).toMatchObject({ tool: 'opencode', structured: true, trust: true, resumeAcp: original.acpSessionId, dir: original.dir, model: original.model })
    expect(requests.at(-1).prompt).toBeUndefined()
    expect(requests.at(-1).resume).toBeUndefined()
  }

  // An uncertain reply keeps the same operation ID; retrying must not create another session.
  row = { ...original }; next = null
  failure = { status: 504, json: { error: 'The machine has not answered yet.' } }
  await open()
  await recover.click()
  await expect(page.getByRole('alert')).toContainText('The machine has not answered yet.')
  await expect(banner).toBeVisible()
  await expect(recover).toBeEnabled()
  const uncertain = requests.at(-1)
  failure = null
  await recover.click()
  await page.waitForURL(new RegExp(`session=${NEXT}`))
  expect(requests.at(-1)).toEqual(uncertain)

  // A definite reload refusal leaves the old transcript and both choices visible.
  row = { ...original }; next = null
  failure = { status: 422, json: { error: 'Saved history is unavailable.' } }
  await open()
  await recover.click()
  await expect(page.getByRole('alert')).toContainText('Saved history is unavailable.')
  await expect(recover).toBeEnabled()
  await expect(banner.getByRole('button', { name: 'Start a new chat' })).toBeEnabled()
  expect(new URL(page.url()).searchParams.get('session')).toBe(S)
  failure = null

  for (const unsupported of [
    { ...original, acpSessionId: '' },
    { ...original, tool: 'claude' },
  ]) {
    row = unsupported
    await open()
    await expect(recover).toHaveCount(0)
    await expect(banner).toContainText('Start a new chat to carry on.')
  }
  row = { ...original }
  machine.capabilities.acpAgents = [{ ...descriptor, history: false }]
  await open()
  await expect(recover).toHaveCount(0)
  machine.capabilities.acpAgents = [descriptor]
  machine.ready = false
  await open()
  await expect(recover).toBeDisabled()
  expect(errors).toEqual([])
  console.log('Chat recovery passed: exact history ID, lost/ended/exited states, desktop and phone fit, uncertain-reply retry, reload refusal, unsupported history and offline machine.')
} finally {
  await context.close()
  await browser.close()
}
