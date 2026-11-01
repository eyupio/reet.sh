import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-laptop000000001', S = 'session-chat000000000000001', S2 = 'session-chat000000000000002', S3 = 'session-chat000000000000003', SITE = '/home/dev/work/site'
const now = Date.now()
const session = { id: S, agentId: A, title: 'OpenCode · site', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'opencode', model: '', effort: '', yolo: false, projectId: null, dir: SITE, structured: true }
const closedSession = { ...session, id: S2, state: 'exited', exitCode: 0, model: 'anthropic/claude-sonnet-4-5', acpSessionId: 'ses_fake000000000000000000000001' }
const machine = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, agentsV2: true, models: true, acp: true, acpHistory: true, acpImages: true, uploads: true, tools: ['opencode'] } }

const useDescriptors = process.env.REET_CHAT_DESCRIPTORS === '1'
const availableChat = { tool: 'opencode', adapter: 'opencode', state: 'available', minimumVersion: '1.19.5', history: true, images: true, model: true, approval: false }
if (useDescriptors) machine.capabilities.acpAgents = [availableChat]

const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { context, page, state, errors } = await openWorkspace(browser, {
  width: 1360, height: 900, touch: false, terminalStatus: active, terminalMachines: [machine],
  routes: {
    '/api/v2/terminal/sessions': req => {
      if (req.method() === 'POST') {
        const body = req.postDataJSON()
        if (body.resumeAcp) {
          state.resume = body
          state.newSession = { ...closedSession, id: S3, title: body.title, state: 'running' }
          state.resumed = true
          return state.newSession
        }
        state.create = body
        return { ...session, title: body.title, state: 'running' }
      }
      if (state.closed) return { sessions: state.resumed ? [closedSession, state.newSession] : [closedSession] }
      return { sessions: [session] }
    },
    '/api/v2/folder-projects': { projects: [], savedWaiting: 0 },
    [`/api/v2/machines/${A}/models`]: { tool: 'opencode', state: 'listed', models: [{ id: 'anthropic/claude-sonnet-4-5', provider: 'anthropic' }], more: false, source: 'opencode models', version: '', fetchedAt: now, checkedAt: now, reason: '', sentence: '' },
    '/api/v2/profile/model-favourites': req => {
      if (req.method() === 'POST') {
        state.favourite = req.postDataJSON()
        state.favourites = [{ tool: state.favourite.tool, model: state.favourite.model, agentDefault: false }]
        return { version: 2, favourites: state.favourites }
      }
      return { version: 1, favourites: state.favourites ?? [] }
    },
    '/api/v2/profile/model-favourites/default': req => {
      state.modelDefault = req.postDataJSON()
      state.favourites = (state.favourites ?? []).map(f => ({ ...f, agentDefault: f.tool === state.modelDefault.tool && f.model === state.modelDefault.model && !!state.modelDefault.model }))
      return { version: 3, favourites: state.favourites }
    },
    [`/api/v2/terminal/sessions/${S}/chat/prompt`]: req => { state.prompt = req.postDataJSON(); return { sent: true } },
    [`/api/v2/terminal/sessions/${S}/chat/cancel`]: () => { state.cancelled = true; return { cancelled: true } },
    [`/api/v2/terminal/sessions/${S}/chat/permissions/p1`]: req => { state.answer = req.postDataJSON(); return { answered: true } },
  },
})
await page.route(`**/api/v2/machines/${A}/uploads`, async route => {
  state.upload = route.request().headers()['content-type']
  await route.fulfill({ status: 200, headers: { 'X-Reet-Upload-Path': encodeURIComponent('/home/dev/launch-uploads/shot.png'), 'X-Reet-Upload-Reference': encodeURIComponent('/home/dev/launch-uploads/shot.png') }, json: { uploaded: 'true' } })
})
await page.addInitScript(() => {
  class FakeEventSource {
    constructor(url) { this.url = url; this.listeners = {}; this.closed = false; window.__chatStream = this; setTimeout(() => this.onopen?.({}), 0) }
    addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn) }
    close() { this.closed = true }
    emit(type, data) { for (const fn of this.listeners[type] ?? []) fn({ data: JSON.stringify(data) }) }
  }
  window.EventSource = FakeEventSource
})
try {
  if (useDescriptors) {
    for (const unavailable of [[], [{ ...availableChat, state: 'missing' }], [{ ...availableChat, state: 'unverified' }], [{ ...availableChat, state: 'incompatible' }]]) {
      machine.capabilities.acpAgents = unavailable
      await page.goto(`${base}/sessions?agent=${A}`)
      await page.getByRole('button', { name: 'New agent session', exact: true }).click()
      const unavailableDialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
      await unavailableDialog.getByRole('radio', { name: 'OpenCode', exact: true }).check()
      await expect(unavailableDialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveCount(0)
      await expect(unavailableDialog.getByText('This machine does not currently offer the reet TUI for OpenCode.', { exact: false })).toBeVisible()
      await unavailableDialog.getByRole('button', { name: 'Close', exact: true }).click()
    }
    machine.capabilities.acpAgents = [availableChat]
  }
  await page.goto(`${base}/sessions?agent=${A}`)
  await page.getByRole('button', { name: 'New agent session', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('radio', { name: 'OpenCode', exact: true }).check()
  await expect(dialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveValue('reet')
  await expect(dialog.getByRole('button', { name: /^Model:/ })).toHaveAccessibleName('Model: OpenCode’s configured model')
  await dialog.getByRole('button', { name: /^Model:/ }).click()
  const models = page.getByRole('listbox', { name: 'Model', exact: true })
  await expect(models).toBeVisible()
  await models.getByRole('option', { name: 'anthropic/claude-sonnet-4-5', exact: true }).click()
  await expect(dialog.getByRole('button', { name: /^Model:/ })).toHaveAccessibleName('Model: anthropic/claude-sonnet-4-5')
  await dialog.getByRole('textbox', { name: /Opening prompt/ }).fill('fix the failing build')
  await dialog.getByRole('button', { name: 'Start', exact: true }).click()
  await page.waitForURL(new RegExp(`session=${S}`))
  await expect(page.getByRole('button', { name: 'reet TUI', exact: true })).toHaveAttribute('aria-pressed', 'true')
  // This machine has not negotiated session-mode-v1, so the OpenCode TUI is offered with the reason it cannot switch.
  await expect(page.getByRole('button', { name: 'OpenCode TUI', exact: true })).toHaveAttribute('aria-pressed', 'false')
  await expect(page.getByRole('button', { name: 'OpenCode TUI', exact: true })).toHaveAttribute('aria-disabled', 'true')
  await expect(page.getByRole('button', { name: 'OpenCode TUI', exact: true })).toHaveAccessibleDescription('Update reet on this machine to switch between the reet TUI and the OpenCode TUI.')
  await page.waitForFunction(() => window.__chatStream?.url.endsWith('/chat'))
  await page.evaluate(() => window.__chatStream.emit('reset', { truncated: false }))
  await page.evaluate(() => {
    window.__chatStream.emit('chat', { seq: 1, kind: 'user', text: 'fix the failing build' })
    window.__chatStream.emit('chat', { seq: 2, kind: 'agent', messageId: 'm1', text: 'Looking ' })
    window.__chatStream.emit('chat', { seq: 3, kind: 'agent', messageId: 'm1', text: 'at the build.' })
    window.__chatStream.emit('chat', { seq: 4, kind: 'tool', toolCallId: 'c1', title: 'notes.txt', toolKind: 'edit', status: 'completed', text: 'Edit applied successfully.', path: '/home/dev/work/site/notes.txt', diff: '@@ -1 +1 @@\n-alpha\n+ALPHA', additions: 1, deletions: 1, durationMs: 179, locations: ['/home/dev/work/site/notes.txt'] })
    window.__chatStream.emit('chat', { seq: 5, kind: 'permission', permissionId: 'p1', toolCallId: 'c2', title: 'Run the tests', toolKind: 'execute', command: 'go test ./...' })
  })
  await expect(page.getByText('Looking at the build.')).toBeVisible()
  await expect(page.getByText('go test ./...')).toBeVisible()
  // A completed tool is one scannable row: kind, relative target, diff counts, duration and state, with its diff folded away.
  const editRow = page.locator('.tool').filter({ hasText: 'notes.txt' })
  await expect(editRow).toContainText('Edit')
  await expect(editRow).toContainText('notes.txt')
  await expect(editRow).toContainText('+1')
  await expect(editRow).toContainText('−1')
  await expect(editRow).toContainText('179 ms')
  await expect(editRow).toContainText('Done')
  await expect(editRow.locator('.tool-body')).toHaveCount(0)
  await editRow.locator('.tool-head').click()
  await expect(editRow.locator('.tool-body')).toBeVisible()
  await expect(editRow).toContainText('1 added, 1 removed')
  await expect(editRow).toContainText('+ALPHA')
  await expect(editRow).toContainText('-alpha')
  await page.getByRole('button', { name: 'Allow once', exact: true }).click()
  await expect.poll(() => state.answer).toEqual({ allow: true })
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 6, kind: 'permission_resolved', permissionId: 'p1', allow: true }))
  await expect(page.getByText('Allowed once.')).toBeVisible()
  await page.getByRole('textbox', { name: 'Message OpenCode' }).fill('now run the linter')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  await expect.poll(() => state.prompt).toEqual({ text: 'now run the linter' })
  const [chooser] = await Promise.all([
   page.waitForEvent('filechooser'),
   page.locator('.composer').getByRole('button', { name: 'Attach image' }).click(),
  ])
  await chooser.setFiles({ name: 'shot.png', mimeType: 'image/png', buffer: Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]) })
  await expect(page.locator('.composer .attachments')).toContainText('shot.png')
  await page.getByRole('textbox', { name: 'Message OpenCode' }).fill('look at this shot')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  await expect.poll(() => state.upload).toBe('image/png')
  await expect.poll(() => state.prompt).toEqual({ text: 'look at this shot', images: ['/home/dev/launch-uploads/shot.png'] })
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 7, kind: 'status', state: 'working' }))
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Stop', exact: true }).click()
  await expect.poll(() => state.cancelled).toBe(true)
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 8, kind: 'status', state: 'cancelled' }))
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toHaveCount(0)
  expect(state.create).toMatchObject({ structured: true, trust: true, tool: 'opencode', model: 'anthropic/claude-sonnet-4-5', prompt: 'fix the failing build' })
  expect(state.create.effort).toBeUndefined()

  // A typed agent question: labelled controls, feedback before sending, the machine's refusal kept on the card, and
  // Skip and Cancel as different answers from Send.
  state.questionCalls = []
  let questionRefusals = 1
  await page.route(`**/api/v2/terminal/sessions/${S}/chat/questions/*`, async route => {
    const request = route.request()
    state.questionCalls.push({ id: new URL(request.url()).pathname.split('/').pop(), body: request.postDataJSON() })
    if (state.questionCalls.at(-1).body.action === 'accept' && questionRefusals-- > 0) {
      await route.fulfill({ status: 422, json: { error: 'That answer was not accepted: answer rejected for approach: that choice was not offered', code: 'question_invalid' } })
      return
    }
    await route.fulfill({ status: 200, json: { answered: true } })
  })
  const form = {
    message: 'Which approach should I take?',
    fields: [
      { key: 'approach', title: 'Approach', kind: 'single', required: true, options: [{ value: 'a', label: 'Approach A', description: 'The quick one' }, { value: 'b', label: 'Approach B' }] },
      { key: 'checks', title: 'Checks to run', kind: 'multi', maxItems: 2, options: [{ value: 'x', label: 'Lint' }, { value: 'y', label: 'Unit tests' }, { value: 'z', label: 'Build' }] },
      { key: 'note', title: 'Anything else', kind: 'text', maxLength: 200 },
    ],
  }
  await page.evaluate(form => window.__chatStream.emit('chat', { seq: 9, kind: 'question', questionId: 'q1', toolCallId: 'c3', question: form }), form)
  const question = page.getByRole('group', { name: 'Which approach should I take?' })
  await expect(question).toBeVisible()
  await expect(page.getByText('Waiting for your answer.')).toBeVisible()
  await expect(question.getByRole('radio', { name: /^Approach A/ })).toBeVisible()
  await expect(question.getByRole('textbox', { name: /Anything else/ })).toBeVisible()
  // An empty required choice is caught before anything is sent, and focus moves to it.
  await question.getByRole('button', { name: 'Send answer', exact: true }).click()
  await expect(question.getByText('Choose one.')).toBeVisible()
  expect(await page.evaluate(() => document.activeElement?.getAttribute('data-invalid'))).toBe('true')
  expect(state.questionCalls.length).toBe(0)
  await question.getByRole('radio', { name: /^Approach A/ }).check()
  await expect(question.getByText('Choose one.')).toHaveCount(0)
  // The limit on a multi select is checked too.
  for (const name of ['Lint', 'Unit tests', 'Build']) await question.getByRole('checkbox', { name }).check()
  await question.getByRole('button', { name: 'Send answer', exact: true }).click()
  await expect(question.getByText('Choose at most 2.')).toBeVisible()
  expect(state.questionCalls.length).toBe(0)
  await question.getByRole('checkbox', { name: 'Build' }).uncheck()
  // The machine refuses once: its reason shows on the card, the question stays open and the choices are kept.
  await question.getByRole('button', { name: 'Send answer', exact: true }).click()
  await expect(question.getByRole('alert')).toContainText('That answer was not accepted')
  await expect(question.getByRole('radio', { name: /^Approach A/ })).toBeChecked()
  await expect(question.getByRole('checkbox', { name: 'Unit tests' })).toBeChecked()
  await expect(question.getByRole('button', { name: 'Send answer', exact: true })).toBeEnabled()
  await question.getByRole('button', { name: 'Send answer', exact: true }).click()
  await expect.poll(() => state.questionCalls.length).toBe(2)
  // An empty optional text answer is left out rather than sent blank.
  expect(state.questionCalls[1]).toEqual({ id: 'q1', body: { action: 'accept', answers: { approach: 'a', checks: ['x', 'y'] } } })
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 10, kind: 'question_resolved', questionId: 'q1', reason: 'accept' }))
  await expect(question.getByText('Answered.')).toBeVisible()
  await expect(question.getByText('Approach A', { exact: true })).toBeVisible()
  await expect(question.getByText('Lint, Unit tests')).toBeVisible()
  await expect(question.getByRole('button', { name: 'Send answer', exact: true })).toHaveCount(0)
  await expect(page.getByText('Waiting for your answer.')).toHaveCount(0)

  // Skip lets the agent carry on; Cancel stops its tool call; a timeout closes the card with its reason.
  await page.evaluate(form => window.__chatStream.emit('chat', { seq: 11, kind: 'question', questionId: 'q2', question: { ...form, message: 'Second question?' } }), form)
  const second = page.getByRole('group', { name: 'Second question?' })
  await second.getByRole('button', { name: 'Skip', exact: true }).click()
  await expect.poll(() => state.questionCalls.at(-1)).toEqual({ id: 'q2', body: { action: 'decline' } })
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 12, kind: 'question_resolved', questionId: 'q2', reason: 'decline' }))
  await expect(second.getByText('Skipped.')).toBeVisible()
  await page.evaluate(form => window.__chatStream.emit('chat', { seq: 13, kind: 'question', questionId: 'q3', question: { ...form, message: 'Third question?' } }), form)
  const third = page.getByRole('group', { name: 'Third question?' })
  await third.getByRole('button', { name: 'Cancel question', exact: true }).click()
  await expect.poll(() => state.questionCalls.at(-1)).toEqual({ id: 'q3', body: { action: 'cancel' } })
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 14, kind: 'question_cancelled', questionId: 'q3', reason: 'timeout' }))
  await expect(third.getByText('Closed: the question timed out.')).toBeVisible()
  // Another tab answers first: this tab's late answer is refused as gone, the card says so, and the resolution that
  // arrives on the stream closes it. The same card replayed from history (a reset then the stored events) comes back
  // decided, never as a live form.
  await page.unroute(`**/api/v2/terminal/sessions/${S}/chat/questions/*`)
  await page.route(`**/api/v2/terminal/sessions/${S}/chat/questions/*`, route => route.fulfill({ status: 409, json: { error: 'That question is no longer waiting.' } }))
  await page.evaluate(form => window.__chatStream.emit('chat', { seq: 16, kind: 'question', questionId: 'q5', question: { ...form, message: 'Raced question?' } }), form)
  const raced = page.getByRole('group', { name: 'Raced question?' })
  await raced.getByRole('button', { name: 'Skip', exact: true }).click()
  await expect(raced.getByRole('alert')).toContainText('no longer waiting')
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 17, kind: 'question_resolved', questionId: 'q5', reason: 'accept' }))
  await expect(raced.getByText('Answered.')).toBeVisible()
  await expect(raced.getByRole('button', { name: 'Skip', exact: true })).toHaveCount(0)
  await page.evaluate(form => {
    window.__chatStream.emit('reset', { truncated: false })
    window.__chatStream.emit('chat', { seq: 1, kind: 'question', questionId: 'q6', question: { ...form, message: 'Replayed question?' } })
    window.__chatStream.emit('chat', { seq: 2, kind: 'question_resolved', questionId: 'q6', reason: 'decline' })
  }, form)
  const replayed = page.getByRole('group', { name: 'Replayed question?' })
  await expect(replayed.getByText('Skipped.')).toBeVisible()
  await expect(replayed.getByRole('button', { name: 'Send answer', exact: true })).toHaveCount(0)
  await page.evaluate(() => window.__chatStream.emit('reset', { truncated: false }))
  // A malformed question is ignored; it never leaves a card the person cannot answer.
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 15, kind: 'question', questionId: 'q4', question: { message: 'Broken', fields: [{ key: 'k', kind: 'number' }] } }))
  await expect(page.getByRole('group', { name: 'Broken' })).toHaveCount(0)

  // The live chat's model switch offers Make default, stars a chosen model that is not yet a favourite, and Clear default removes it.
  const modelInput = page.locator('.model-controls input')
  await modelInput.fill('anthropic/claude-sonnet-4-5')
  await page.getByRole('button', { name: 'Make anthropic/claude-sonnet-4-5 the default for OpenCode', exact: true }).click()
  await expect.poll(() => state.favourite).toEqual({ tool: 'opencode', model: 'anthropic/claude-sonnet-4-5' })
  await expect.poll(() => state.modelDefault).toEqual({ tool: 'opencode', model: 'anthropic/claude-sonnet-4-5' })
  await expect(page.getByText('New opencode sessions use anthropic/claude-sonnet-4-5.')).toBeVisible()
  await page.getByRole('button', { name: 'Clear anthropic/claude-sonnet-4-5 as the default for OpenCode', exact: true }).click()
  await expect.poll(() => state.modelDefault).toEqual({ tool: 'opencode', model: '' })
  await expect(page.getByText('Cleared the default for opencode.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Make anthropic/claude-sonnet-4-5 the default for OpenCode', exact: true })).toBeVisible()

  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('textbox', { name: 'Message OpenCode' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  // The question card fits a phone: no sideways scroll and every control stays inside the viewport. (Its larger touch
  // targets come from the same coarse-pointer rule as the approval buttons; this mouse-driven run does not exercise it.)
  await page.evaluate(form => window.__chatStream.emit('chat', { seq: 30, kind: 'question', questionId: 'q9', question: { ...form, message: 'Phone question with a fairly long line of text that has to wrap inside the card?' } }), form)
  const phoneQuestion = page.getByRole('group', { name: /^Phone question/ })
  await expect(phoneQuestion).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  for (const name of ['Cancel question', 'Skip', 'Send answer']) {
    const box = await phoneQuestion.getByRole('button', { name, exact: true }).boundingBox()
    expect(box.x).toBeGreaterThanOrEqual(0)
    expect(box.x + box.width).toBeLessThanOrEqual(390)
  }
  const textbox = await phoneQuestion.getByRole('textbox', { name: /Anything else/ }).boundingBox()
  expect(textbox.x + textbox.width).toBeLessThanOrEqual(390)
  await page.evaluate(() => window.__chatStream.emit('reset', { truncated: true }))
  await expect(page.getByText('Earlier messages in this reet session are no longer available.')).toBeVisible()

  // A fast stream, or a continued chat replaying its history, is bounded in the browser: the newest items are kept, the
  // log says the earlier ones are gone, and the page stays responsive rather than stalling on a growing list.
  await page.evaluate(() => window.__chatStream.emit('reset', { truncated: false }))
  await page.evaluate(() => {
    for (let i = 1; i <= 1200; i++) window.__chatStream.emit('chat', { seq: i, kind: 'tool', toolCallId: `c${i}`, title: 'Run', toolKind: 'execute', status: 'completed', text: 'out' })
  })
  await expect(page.locator('.tool')).toHaveCount(400)
  await expect(page.getByText('Earlier messages in this reet session are no longer available.')).toBeVisible()
  const settled = await page.evaluate(() => new Promise(resolve => { const at = performance.now(); setTimeout(() => resolve(performance.now() - at), 0) }))
  expect(settled).toBeLessThan(1000)

  // A closed chat keeps OpenCode's session id and reopens as a new row that loads its history.
  state.closed = true
  await page.goto(`${base}/sessions?agent=${A}&session=${S2}`)
  await expect(page.getByRole('button', { name: 'Session actions' })).toBeVisible()
  await page.getByRole('button', { name: 'Session actions' }).click()
  await page.getByRole('menuitem', { name: /Continue in reet TUI with history/ }).click()
  await expect.poll(() => state.resume).toMatchObject({ structured: true, trust: true, resumeAcp: closedSession.acpSessionId, dir: SITE })
  await page.waitForURL(new RegExp(`session=${S3}`))
  // A closed chat cannot switch: the menu offers no switch and the OpenCode TUI says why (on a wider screen, where the
  // header shows the switch).
  await page.setViewportSize({ width: 1360, height: 900 })
  await page.goto(`${base}/sessions?agent=${A}&session=${S2}`)
  await expect(page.getByRole('button', { name: 'OpenCode TUI', exact: true })).toHaveAccessibleDescription('Only a running session can switch.')
  await page.getByRole('button', { name: 'Session actions' }).click()
  await expect(page.getByRole('menuitem', { name: /Switch to the/ })).toHaveCount(0)
  await page.keyboard.press('Escape')
  expect(state.resume.prompt).toBeUndefined()
  expect(state.resume.model).toBe('anthropic/claude-sonnet-4-5')
  expect(errors).toEqual([])
  console.log('Structured OpenCode chat UI passed: the dialog offers the reet TUI for OpenCode, the create body carries structured, trust and the chosen model, the session screen opens reet TUI with the OpenCode TUI disabled and its reason (the machine predates session-mode-v1), agent chunks stream, a command approval is Allow once or Deny, a typed question is checked, refused and answered, skipped or cancelled and fits a phone, a message sends, an attached image uploads and is passed as an ACP image, Stop cancels, the phone fits, a truncated reset says so, and a closed chat reopens through the continue action with its OpenCode session id and cannot switch TUI.')
} finally {
  await context.close()
  await browser.close()
}
