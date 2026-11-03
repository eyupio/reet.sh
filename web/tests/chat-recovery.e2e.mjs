import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { join } from 'node:path'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-recovery000000001', S = 'session-recovery000000000001', NEXT = 'session-recovery000000000002'
const original = { id: S, agentId: A, title: 'OpenCode · recovery', state: 'lost', createdAt: Date.now(), updatedAt: Date.now(), exitCode: null, tool: 'opencode', model: 'provider/model', dir: '/home/dev/site', structured: true, acpSessionId: 'ses_original000000000000000001' }
const descriptor = { tool: 'opencode', adapter: 'opencode', state: 'available', history: true, model: true }
const machineLegacy = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, readiness: 'ready', launch: true, agentsV2: true, tools: ['opencode'], acp: true, acpAgents: [descriptor] } }
const machineNew = { ...machineLegacy, capabilities: { ...machineLegacy.capabilities, chatReattach: true } }
const machine = machineLegacy
const machines = [machineLegacy]
let row = { ...original }, next = null, failure = null, holdCreate = null
let reattachMode = 'ok', reattachHold = null
const requests = []
const reattachCalls = []
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { context, page, errors } = await openWorkspace(browser, {
  width: 1360, height: 900, terminalStatus: active, terminalMachines: machines,
  routes: {
    [`/api/v2/terminal/sessions/${S}/reattach`]: async req => {
      reattachCalls.push(req.method())
      if (reattachHold) await reattachHold
      if (reattachMode === 'fail-once') { reattachMode = 'ok'; return { status: 502, json: { error: 'The machine could not reattach this conversation.', code: 'chat_reattach_failed' } } }
      // A refusal object is answered on every call.
      if (typeof reattachMode === 'object') return reattachMode
      row = { ...row, state: 'running' }
      return { status: 204, json: '' }
    },
    '/api/v2/terminal/sessions': async req => {
      if (req.method() === 'POST') {
        requests.push(req.postDataJSON())
        if (holdCreate) await holdCreate
        if (failure) return failure
        next = { ...row, id: NEXT, state: 'running' }
        return next
      }
      return { sessions: next ? [row, next] : [row] }
    },
  },
})
await page.addInitScript(() => {
  // A stub stream: the page can push named events (as the server's `chat` stream sends them) after load.
  window.__sources = []
  window.EventSource = class {
    constructor() { this.listeners = {}; this.closed = false; window.__sources.push(this); setTimeout(() => this.onopen?.({}), 0) }
    addEventListener(name, fn) { (this.listeners[name] ||= []).push(fn) }
    close() { this.closed = true }
  }
  window.__push = events => { for (const source of window.__sources) if (!source.closed) for (const event of events) for (const fn of source.listeners.chat || []) fn({ data: JSON.stringify(event) }) }
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

  // --- In-place reattach (machine reports chatReattach) ---
  machines[0] = machineNew
  const prompt = page.locator(`[id="${S}-chat-prompt"]`)
  const note = page.locator('.ended-note')
  const reconnecting = page.getByText('Reconnecting to your conversation…')
  const rowLabel = page.locator('.session-row', { hasText: original.title }).locator('small')
  const gate = () => { let release; const held = new Promise(resolve => { release = resolve }); return { held, release } }
  const reset = (state = 'lost') => { row = { ...original, state }; next = null; failure = null; holdCreate = null; reattachMode = 'ok'; reattachHold = null; reattachCalls.length = 0; requests.length = 0 }
  // The header menu offers Continue in Gaffer with history only where the chat cannot reconnect in place.
  const continueItem = page.getByRole('menuitem', { name: /Continue in Gaffer with history/ })
  const headerMenu = async present => {
    await page.locator('.session-screen').getByRole('button', { name: 'Session actions' }).click()
    await expect(page.getByRole('menuitem').first()).toBeVisible()
    await expect(continueItem).toHaveCount(present ? 1 : 0)
    await page.keyboard.press('Escape')
    await expect(page.getByRole('menuitem')).toHaveCount(0)
  }
  const overflow = () => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)

  // 1. Auto-reattach in place, and the same stream carries on afterwards.
  reset()
  await page.setViewportSize({ width: 1360, height: 900 })
  await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
  await expect.poll(() => reattachCalls.length).toBe(1)
  await expect(prompt).toBeEnabled()
  await expect(note).toHaveCount(0)
  expect(new URL(page.url()).searchParams.get('session')).toBe(S)
  await expect(page.locator('.log[role="log"]')).toHaveCount(1)
  // The 204 can beat the status event: a real event on the stream that was open all along must still render here.
  await page.evaluate(() => window.__push([
    { seq: 1, kind: 'agent', messageId: 'm1', text: 'Back with you after reconnecting.' },
    { seq: 2, kind: 'status', state: 'idle' },
  ]))
  await expect(page.locator('.log')).toContainText('Back with you after reconnecting.')
  await expect(prompt).toBeEnabled()
  await expect(note).toHaveCount(0)
  await expect(page.getByText('This reet session has ended.')).toHaveCount(0)
  await expect(page.locator('.ended-banner')).toHaveCount(0)
  expect(new URL(page.url()).searchParams.get('session')).toBe(S)
  expect(reattachCalls).toEqual(['POST'])
  expect(requests).toHaveLength(0)
  expect((await page.evaluate(async () => (await (await fetch('/api/v2/terminal/sessions')).json()).sessions.length))).toBe(1)
  await page.waitForTimeout(300)
  expect(reattachCalls).toHaveLength(1)

  // 2. Pending: held call, status text, no buttons, composer off, sidebar label, no overflow.
  for (const [width, theme] of [[320, 'light'], [320, 'dark'], [390, 'light'], [390, 'dark']]) {
    reset()
    const held = gate(); reattachHold = held.held
    await page.setViewportSize({ width, height: 900 })
    await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
    await page.evaluate(theme => document.documentElement.dataset.theme = theme, theme)
    await expect(reconnecting).toBeVisible()
    await expect(note.getByRole('button')).toHaveCount(0)
    await expect(prompt).toBeDisabled()
    expect(await overflow()).toBeLessThanOrEqual(0)
    await expect.poll(() => reattachCalls.length).toBe(1)
    if (process.env.REET_UI_SCREENSHOTS) await page.screenshot({ path: join(process.env.REET_UI_SCREENSHOTS, `chat-reattach-pending-${width}-${theme}.png`), fullPage: true })
    held.release()
    await expect(prompt).toBeEnabled()
  }
  await page.setViewportSize({ width: 1360, height: 900 })
  reset()
  // The sidebar is a closed drawer on a phone, so its row label is checked at desktop width.
  { const held = gate(); reattachHold = held.held
    await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
    await expect(reconnecting).toBeVisible()
    await expect(rowLabel).toContainText('Reconnecting')
    await headerMenu(false)
    held.release(); await expect(prompt).toBeEnabled() }

  // 3. Failure then retry: no automatic second call; Try again sends exactly one more.
  reset(); reattachMode = 'fail-once'
  await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
  await expect(note).toContainText('The machine could not reattach this conversation.')
  const tryAgain = note.getByRole('button', { name: 'Try again', exact: true })
  await expect(tryAgain).toBeVisible()
  await expect(note.getByRole('button', { name: 'Start a new chat', exact: true })).toBeVisible()
  await page.waitForTimeout(500)
  expect(reattachCalls).toHaveLength(1)
  expect(requests).toHaveLength(0)
  await headerMenu(false)
  await tryAgain.click()
  await expect(prompt).toBeEnabled()
  await expect(note).toHaveCount(0)
  expect(reattachCalls).toHaveLength(2)
  expect(new URL(page.url()).searchParams.get('session')).toBe(S)

  // 4. Double click on Try again sends one call.
  reset(); reattachMode = 'fail-once'
  await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
  await expect(tryAgain).toBeVisible()
  const held4 = gate(); reattachHold = held4.held
  await tryAgain.evaluate(el => { el.click(); el.click() })
  await expect(reconnecting).toBeVisible()
  await page.waitForTimeout(300)
  expect(reattachCalls).toHaveLength(2)
  held4.release()
  await expect(prompt).toBeEnabled()
  expect(reattachCalls).toHaveLength(2)

  // 5. An ended chat keeps the manual path and never reconnects by itself.
  reset('ended')
  await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
  await expect(note.getByRole('button', { name: 'Continue with history', exact: true })).toBeVisible()
  await expect(note.getByRole('button', { name: 'Reconnect', exact: true })).toHaveCount(0)
  await page.waitForTimeout(500)
  expect(reattachCalls).toHaveLength(0)

  // 5b. An exited chat the person did not end (a crash) still reconnects by itself, and its sidebar row says Reconnecting.
  reset('exited')
  { const held = gate(); reattachHold = held.held
    await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
    await expect(reconnecting).toBeVisible()
    await expect(rowLabel).toContainText('Reconnecting')
    // While pending only the status line is a live region; the note itself does not announce again.
    await expect(page.locator('.ended-note.reattaching')).not.toHaveAttribute('role', /.+/)
    held.release() }
  await expect(prompt).toBeEnabled()
  expect(reattachCalls).toEqual(['POST'])
  expect(requests).toHaveLength(0)

  // 5c. An exited chat the controller knows the person ended (rows stored before ended chats were recorded as ended) is
  // refused; the automatic attempt falls back quietly to the ended note with Continue with history, no failure card.
  reset('exited'); reattachMode = { status: 409, json: { error: 'You ended this chat. Continue it with history as a new session.', code: 'chat_reattach_ended' } }
  await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
  await expect.poll(() => reattachCalls.length).toBe(1)
  await expect(note.getByRole('button', { name: 'Continue with history', exact: true })).toBeVisible()
  await expect(note.getByRole('button', { name: 'Try again', exact: true })).toHaveCount(0)
  await expect(note.getByRole('button', { name: 'Reconnect', exact: true })).toHaveCount(0)
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(note).not.toContainText('Could not reconnect')
  await headerMenu(true)
  await page.waitForTimeout(300)
  expect(reattachCalls).toHaveLength(1)
  expect(requests).toHaveLength(0)

  // 5d. Refusals a retry cannot fix say why and offer Start a new chat only.
  for (const [code, error] of [['chat_reattach_yolo_refused', 'YOLO is turned off on this machine, so this YOLO chat cannot be reconnected.'], ['chat_reattach_capacity', 'This machine has reached its session limit.'], ['chat_reattach_yolo_not_allowed', 'An owner or admin can allow YOLO for this account in Settings.']]) {
    reset(); reattachMode = { status: 409, json: { error, code } }
    await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
    await expect(page.getByRole('alert')).toContainText(error)
    await expect(note.getByRole('button', { name: 'Try again', exact: true })).toHaveCount(0)
    await expect(note.getByRole('button', { name: 'Start a new chat', exact: true })).toBeVisible()
    await expect(note.getByRole('button', { name: 'Continue with history', exact: true })).toHaveCount(0)
    // The heading is said once, not again in the reason.
    await expect(note).toContainText('Could not reconnect.')
    await headerMenu(false)
    expect(reattachCalls).toHaveLength(1)
  }

  // 5e. A chat still starting on the machine is retryable.
  reset(); reattachMode = { status: 409, json: { error: 'Still reconnecting. Try again in a moment.', code: 'chat_reattach_starting' } }
  await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
  await expect(page.getByRole('alert')).toContainText('Still reconnecting. Try again in a moment.')
  await expect(note.getByRole('button', { name: 'Try again', exact: true })).toBeVisible()

  // 5f. A folder with plugins (ruling R10): the files are shown, Continue with history (trust: true, a new row) is
  // offered again in the pane and the header menu, and there is no Try again.
  reset(); reattachMode = { status: 409, json: { error: 'This folder has OpenCode plugins, so reet did not reconnect the chat in place. Continue it with history only if you trust this folder.', code: 'chat_reattach_trust', detail: 'This folder has agent extensions: .opencode/plugins/x.js. Start a chat only in a folder you trust.' } }
  await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
  await expect(page.getByRole('alert')).toContainText('This folder has OpenCode plugins')
  await expect(note).toContainText('.opencode/plugins/x.js')
  await expect(note.getByRole('button', { name: 'Try again', exact: true })).toHaveCount(0)
  await expect(note.getByRole('button', { name: 'Start a new chat', exact: true })).toBeVisible()
  const trusted = note.getByRole('button', { name: 'Continue with history', exact: true })
  await expect(trusted).toBeEnabled()
  await headerMenu(true)
  expect(reattachCalls).toHaveLength(1)
  { const created = gate(); holdCreate = created.held
    await trusted.click()
    await expect(note).toContainText('Starting a new session…')
    await expect(trusted).toBeDisabled()
    created.release() }
  await page.waitForURL(new RegExp(`session=${NEXT}`))
  expect(requests).toHaveLength(1)
  expect(requests[0]).toMatchObject({ tool: 'opencode', structured: true, trust: true, resumeAcp: original.acpSessionId })
  expect(reattachCalls).toHaveLength(1)

  // 6. A machine without chatReattach keeps the old flow; while the create is held the pane says so and the button is off.
  machineLegacy.ready = true
  machines[0] = machineLegacy
  reset()
  await open()
  await expect(note.getByRole('button', { name: 'Reconnect', exact: true })).toHaveCount(0)
  await headerMenu(true)
  const created = gate(); holdCreate = created.held
  await recover.click()
  await expect(banner).toContainText('Starting a new session…')
  await expect(recover).toBeDisabled()
  expect(reattachCalls).toHaveLength(0)
  created.release()
  await page.waitForURL(new RegExp(`session=${NEXT}`))
  expect(reattachCalls).toHaveLength(0)
  holdCreate = null
  machines[0] = machineLegacy
  reset()
  expect(errors).toEqual([])
  console.log('Chat recovery passed: exact history ID, lost/ended/exited states, desktop and phone fit, uncertain-reply retry, reload refusal, unsupported history, offline machine, in-place reattach (auto, pending, failure, retry, double click, stream continuity), ended chat, crashed exited chat, quiet ended refusal, final refusals, still starting, plugin folder and legacy machine.')
} finally {
  await context.close()
  await browser.close()
}
