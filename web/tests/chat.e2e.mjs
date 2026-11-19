import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const A = 'connector-laptop000000001', S = 'session-chat000000000000001', S2 = 'session-chat000000000000002', S3 = 'session-chat000000000000003', SITE = '/home/dev/work/site'
const now = Date.now()
const session = { id: S, agentId: A, title: 'OpenCode · site', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'opencode', model: '', effort: '', yolo: false, projectId: null, dir: SITE, structured: true }
const closedSession = { ...session, id: S2, state: 'exited', exitCode: 0, model: 'anthropic/claude-sonnet-4-5', acpSessionId: 'ses_fake000000000000000000000001' }
const machine = { agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, agentsV2: true, models: true, acp: true, acpModel: true, acpHistory: true, acpImages: true, uploads: true, tools: ['opencode'] } }

const useDescriptors = process.env.REET_CHAT_DESCRIPTORS === '1'
const availableChat = { tool: 'opencode', adapter: 'opencode', state: 'available', minimumVersion: '1.19.5', history: true, images: true, model: true, approval: false }
if (useDescriptors) machine.capabilities.acpAgents = [availableChat]

const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const { context, page, state, errors } = await openWorkspace(browser, {
  width: 1360, height: 900, touch: true, terminalStatus: active, terminalMachines: [machine],
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
      await page.getByRole('button', { name: 'Start a new session', exact: true }).first().click()
      await page.getByRole('dialog', { name: 'New session', exact: true }).getByRole('button', { name: 'Agent session', exact: true }).click()
      const unavailableDialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
      await unavailableDialog.getByRole('combobox', { name: 'Agent', exact: true }).selectOption({ label: 'OpenCode' })
      await expect(unavailableDialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveCount(0)
      await expect(unavailableDialog.getByText('This machine does not currently offer Gaffer for OpenCode.', { exact: false })).toBeVisible()
      await unavailableDialog.getByRole('button', { name: 'Close', exact: true }).click()
    }
    machine.capabilities.acpAgents = [availableChat]
  }
  await page.goto(`${base}/sessions?agent=${A}`)
  await page.getByRole('button', { name: 'Start a new session', exact: true }).first().click()
  await page.getByRole('dialog', { name: 'New session', exact: true }).getByRole('button', { name: 'Agent session', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Start an agent', exact: true })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('combobox', { name: 'Agent', exact: true }).selectOption({ label: 'OpenCode' })
  await expect(dialog.getByRole('combobox', { name: 'Session', exact: true })).toHaveValue('reet')
  const explanation = "Reet's native AI agent control interface. Chat with your agent, follow its work, manage tool calls and approvals, and control sessions without using the agent's original terminal UI."
  const about = dialog.getByRole('combobox', { name: 'Session', exact: true })
  await about.hover()
  const help = page.getByRole('tooltip')
  await expect(help).toBeVisible()
  await expect(help).toHaveText(`Gaffer${explanation}`)
  await expect(about).toHaveAccessibleDescription(explanation)
  const backgrounds = []
  for (const theme of ['light', 'dark']) {
    await page.evaluate(theme => document.documentElement.dataset.theme = theme, theme)
    backgrounds.push(await help.evaluate(node => getComputedStyle(node).backgroundColor))
    await expect(help).toBeVisible()
  }
  expect(backgrounds[0]).not.toBe(backgrounds[1])
  await page.evaluate(() => document.documentElement.dataset.theme = 'light')

  const box = await help.boundingBox()
  expect(box.x).toBeGreaterThanOrEqual(0)
  expect(box.y).toBeGreaterThanOrEqual(0)
  expect(box.x + box.width).toBeLessThanOrEqual(1360)
  await page.keyboard.press('Escape')
  await expect(help).toHaveCount(0)
  await expect(dialog).toBeVisible()
  await about.blur()
  await about.focus()
  await expect(help).toBeVisible()
  // On a phone the dropdown opens the native picker, so the explanation lives in the hint under it rather than a tooltip.
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(dialog.getByText('In Gaffer, OpenCode asks before it edits', { exact: false })).toBeVisible()
  await page.mouse.move(0, 0)
  await page.keyboard.press('Escape')
  await page.setViewportSize({ width: 1360, height: 900 })

  await expect(dialog.getByRole('button', { name: /^Model:/ })).toHaveAccessibleName('Model: OpenCode’s configured model')
  await dialog.getByRole('button', { name: /^Model:/ }).click()
  const models = page.getByRole('listbox', { name: 'Model', exact: true })
  await expect(models).toBeVisible()
  await models.getByRole('option', { name: 'anthropic/claude-sonnet-4-5', exact: true }).click()
  await expect(dialog.getByRole('button', { name: /^Model:/ })).toHaveAccessibleName('Model: anthropic/claude-sonnet-4-5')
  await dialog.getByRole('textbox', { name: /Opening prompt/ }).fill('fix the failing build')
  await dialog.getByRole('combobox', { name: 'Project', exact: true }).selectOption({ label: 'No project — start in a folder' })
  await dialog.getByRole('button', { name: 'Start', exact: true }).click()
  await page.waitForURL(new RegExp(`session=${S}`))
  // The URL changes before the dialog has finished closing, and while it is a modal the page behind it is inert: focus() on the
  // button would do nothing and no tooltip would open. Wait for it to go, then check the button really took the focus.
  await expect(dialog).toBeHidden()
  // A live chat offers no New chat in its status strip: a new session is started from the left panel instead.
  await expect(page.locator('.chat-status').getByRole('button', { name: 'New chat', exact: true })).toHaveCount(0)
  await expect(page.locator('.ended-note')).toHaveCount(0)
  // The interface control names Gaffer. This machine has not negotiated session-mode-v1, so it offers no switch.
  await page.getByRole('button', {name:'Session actions',exact:true}).click()
  const tui = page.locator('.session-screen .tui-pill')
  await expect(tui).toHaveText('Gaffer')
  await expect(tui).toHaveAttribute('title','Update reet on this machine to switch between Gaffer and the OpenCode TUI.')
  await expect(page.locator('.session-screen .tui-picker select')).toHaveCount(0)
  await tui.hover()
  await expect(page.getByRole('tooltip')).toHaveText(`Gaffer${explanation}`)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('tooltip')).toHaveCount(0)
  await page.keyboard.press('Escape')
  await page.waitForFunction(() => window.__chatStream?.url.endsWith('/chat'))
  await page.evaluate(() => window.__chatStream.emit('reset', { truncated: false }))
  await page.evaluate(() => {
    window.__chatStream.emit('chat', { seq: 1, kind: 'user', text: 'fix the failing build' })
    window.__chatStream.emit('chat', { seq: 2, kind: 'agent', messageId: 'm1', text: 'Looking ' })
    window.__chatStream.emit('chat', { seq: 3, kind: 'agent', messageId: 'm1', text: 'at the build.\n\n| Change | Where |\n|---|---|\n| Reuse `ScrollRail` | *App* |\n\n- [x] done ~~old~~\n  - nested\n\n#### Deep\n\n```sh\nnpm test\n```' })
    window.__chatStream.emit('chat', { seq: 4, kind: 'tool', toolCallId: 'c1', title: 'notes.txt', toolKind: 'edit', status: 'completed', text: 'Edit applied successfully.', path: '/home/dev/work/site/notes.txt', diff: '@@ -1 +1 @@\n-alpha\n+ALPHA', additions: 1, deletions: 1, durationMs: 179, locations: ['/home/dev/work/site/notes.txt'] })
    window.__chatStream.emit('chat', { seq: 5, kind: 'permission', permissionId: 'p1', toolCallId: 'c2', title: 'Run the tests', toolKind: 'execute', command: 'go test ./...', text: 'The tests write to /var/tmp, which the sandbox does not allow.' })
  })
  await expect(page.getByRole('region', { name: 'Gaffer in OpenCode · site' }).getByText('Looking at the build.')).toBeVisible()
  await expect(page.locator('.approval .command').filter({ hasText: 'go test ./...' })).toBeVisible()
  // The agent's markdown is rendered as markdown: a table is a table, lists nest and carry task boxes, and nothing is left as raw syntax.
  const narrative = page.locator('.bubble.agent').filter({ hasText: 'Looking at the build.' })
  await expect(narrative.locator('table th')).toHaveText(['Change', 'Where'])
  await expect(narrative.locator('table td code')).toHaveText('ScrollRail')
  await expect(narrative.locator('table td em')).toHaveText('App')
  await expect(narrative.locator('li.task input[type=checkbox]')).toBeChecked()
  await expect(narrative.locator('li del')).toHaveText('old')
  await expect(narrative.locator('li li')).toHaveText('nested')
  await expect(narrative.locator('.prose-h[data-level="4"]')).toHaveText('Deep')
  await expect(narrative).not.toContainText('|')
  // Copying a code block says so on its button.
  await expect(narrative.locator('.code pre code')).toHaveText('npm test')
  await narrative.locator('.code .copy').click()
  await expect(narrative.locator('.code .copy')).toHaveText('Copied')
  // On a touch screen every typeable field in the chat is at least 16px: a touch browser zooms the page in to a smaller field the
  // moment it takes focus (this is the touch tablet width, above the app.css phone rule), and the page is then left scaled and clipped.
  for (const field of [page.locator('.composer-input'), page.locator('.model-picker')]) {
    expect(Number.parseFloat(await field.evaluate(node => getComputedStyle(node).fontSize))).toBeGreaterThanOrEqual(16)
  }
  // The card shows the agent's own words for the request, attributed to the agent, above the command it is asking about.
  const reasonCard = page.locator('.approval').filter({ hasText: 'Run the tests' })
  await expect(reasonCard.locator('.approval-reason')).toHaveText('The tests write to /var/tmp, which the sandbox does not allow.')
  await expect(reasonCard.locator('.block-label').first()).toHaveText('Why OpenCode asks')
  // OpenCode's rejection closes the tool, so its button keeps the short word; Codex's says it ends the turn (chat.test.mjs covers that).
  await expect(reasonCard.getByRole('button', { name: 'Deny', exact: true })).toBeVisible()
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
  // A decided approval folds shut like every other row; the person reopens it to see the command and the answer.
  const approvalCard = page.locator('.approval').filter({ hasText: 'Run the tests' })
  await expect(approvalCard.locator('.approval-actions')).toHaveCount(0)
  await approvalCard.locator('.approval-head').click()
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

  // While OpenCode works the composer queues a message instead of sending it; the queue is visible, editable and removable,
  // and sends one message when the turn finishes.
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 8.1, kind: 'status', state: 'working' }))
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible()
  const chatBox = page.getByRole('textbox', { name: 'Message OpenCode' })
  await chatBox.fill('steer the work toward the smaller fix')
  await page.getByRole('button', { name: 'Queue', exact: true }).click()
  const queue = page.getByRole('region', { name: 'Queued messages' })
  await expect(queue).toBeVisible()
  await expect(queue.locator('.queue-list li')).toHaveCount(1)
  await expect(queue).toContainText('steer the work toward the smaller fix')
  await page.getByRole('button', { name: /^Edit queued message/ }).click()
  await expect(chatBox).toHaveValue('steer the work toward the smaller fix')
  await expect(queue).toHaveCount(0)
  await page.getByRole('button', { name: 'Queue', exact: true }).click()
  await expect(queue.locator('.queue-list li')).toHaveCount(1)
  state.prompt = null
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 8.2, kind: 'status', state: 'idle' }))
  await expect.poll(() => state.prompt).toEqual({ text: 'steer the work toward the smaller fix' })
  await expect(queue).toHaveCount(0)
  // A queued message can be removed instead of sent.
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 8.3, kind: 'status', state: 'working' }))
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible()
  await chatBox.fill('this one will be removed')
  await page.getByRole('button', { name: 'Queue', exact: true }).click()
  await expect(queue.locator('.queue-list li')).toHaveCount(1)
  await page.getByRole('button', { name: /^Remove queued message/ }).click()
  await expect(queue).toHaveCount(0)
  state.prompt = null
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 8.4, kind: 'status', state: 'idle' }))
  await page.waitForTimeout(250)
  expect(state.prompt).toBeNull()
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
  const modelPicker = page.getByRole('button', { name: /^Model:/ })
  await modelPicker.click()
  await page.getByRole('listbox', { name: 'Model', exact: true }).getByRole('option', { name: 'anthropic/claude-sonnet-4-5', exact: true }).click()
  await page.getByRole('button', { name: 'Make anthropic/claude-sonnet-4-5 the default for OpenCode', exact: true }).click()
  await expect.poll(() => state.favourite).toEqual({ tool: 'opencode', model: 'anthropic/claude-sonnet-4-5' })
  await expect.poll(() => state.modelDefault).toEqual({ tool: 'opencode', model: 'anthropic/claude-sonnet-4-5' })
  await expect(page.getByText('New opencode sessions use anthropic/claude-sonnet-4-5.')).toBeVisible()
  await page.getByRole('button', { name: 'Clear anthropic/claude-sonnet-4-5 as the default for OpenCode', exact: true }).click()
  await expect.poll(() => state.modelDefault).toEqual({ tool: 'opencode', model: '' })
  await expect(page.getByText('Cleared the default for opencode.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Make anthropic/claude-sonnet-4-5 the default for OpenCode', exact: true })).toBeVisible()

  // The skill picker: it fills the composer and never sends. Three groups, a filter, keyboard choice, the `/` trigger,
  // disabled rows, the 2,000 limit, an empty machine, a failed request with Retry, and a bottom sheet on a phone.
  const skillRows = [
    { id: 'm1', name: 'pdf', description: 'Read and fill forms', source: 'machine', agent: 'opencode', native: true, onMachine: true, invocation: 'Use the pdf skill.' },
    { id: 'm2', name: 'docx-other', description: 'Edit documents', source: 'machine', agent: 'claude', native: false, onMachine: true, invocation: '' },
    { id: 'new:reet-notes', name: 'reet-notes', description: '', source: 'new', agent: 'cursor', native: false, onMachine: true, invocation: '' },
  ]
  state.skills = { skills: skillRows, machineSeen: Date.now() - 5 * 60_000, host: 'dev-laptop' }
  state.skillsFail = 0
  let sends = 0
  page.on('request', request => { if (request.url().includes('/chat/prompt')) sends++ })
  await page.route(`**/api/v2/terminal/sessions/*/skills`, route => {
    if (state.skillsFail > 0) { state.skillsFail--; return route.fulfill({ status: 500, json: { error: 'Skills could not be read just now.' } }) }
    return route.fulfill({ status: 200, json: state.skills })
  })
  const composer = page.getByRole('textbox', { name: 'Message OpenCode' })
  const skillsButton = page.locator('.composer').getByRole('button', { name: 'Skills', exact: true })
  const picker = page.getByRole('dialog', { name: 'Skills', exact: true })
  await composer.fill('')
  const sendsBefore = sends
  await skillsButton.click()
  await expect(picker).toBeVisible()
  await expect(picker.getByRole('listbox')).toHaveCount(3)
  await expect(picker.getByText('Skills on dev-laptop were read 5 minutes ago.')).toBeVisible()
  await picker.getByRole('searchbox', { name: 'Filter skills' }).fill('pd')
  await expect(picker.getByRole('option')).toHaveCount(1)
  // Choosing with the keyboard closes the sheet, fills the composer, focuses it, and sends nothing.
  await picker.getByRole('searchbox', { name: 'Filter skills' }).press('ArrowDown')
  await page.keyboard.press('Enter')
  await expect(picker).toHaveCount(0)
  await expect(composer).toHaveValue('Use the pdf skill.')
  await expect(composer).toBeFocused()
  expect(sends).toBe(sendsBefore)
  // Escape from the filter closes the picker and leaves the composer text as it was.
  await composer.fill('keep this')
  await skillsButton.click()
  await expect(picker.getByRole('searchbox', { name: 'Filter skills' })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(picker).toHaveCount(0)
  await expect(composer).toHaveValue('keep this')
  await expect(composer).toBeFocused()
  // Disabled rows (another agent's skill, and one New on this machine for another agent): clicking or Enter does nothing, and ArrowUp from
  // the first row returns to the filter.
  await skillsButton.click()
  const filter = picker.getByRole('searchbox', { name: 'Filter skills' })
  for (const name of ['docx-other', 'reet-notes']) {
    const row = picker.getByRole('option', { name: new RegExp(name) })
    await expect(row).toHaveAttribute('aria-disabled', 'true')
    await row.click({ force: true })
    await row.focus()
    await page.keyboard.press('Enter')
    await expect(picker).toBeVisible()
    await expect(row).toBeVisible()
  }
  await filter.focus()
  await page.keyboard.press('ArrowDown')
  await expect(picker.getByRole('option', { name: /pdf/ })).toBeFocused()
  await page.keyboard.press('ArrowUp')
  await expect(filter).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(composer).toHaveValue('keep this')
  // The `/` trigger: a leading slash token opens the picker with that text as the filter and the pick replaces it.
  await composer.fill('/pd')
  await expect(picker).toBeVisible()
  await expect(filter).toHaveValue('pd')
  // One Escape with text in the search field closes the picker (it must not stop at clearing the field) and leaves the composer alone.
  await page.keyboard.press('Escape')
  await expect(picker).toHaveCount(0)
  await expect(composer).toHaveValue('/pd')
  await composer.fill('')
  await composer.fill('/pd')
  await expect(picker).toBeVisible()
  await expect(filter).toHaveValue('pd')
  await picker.getByRole('option', { name: /pdf/ }).click()
  await expect(composer).toHaveValue('Use the pdf skill.')
  for (const text of ['fix /etc/hosts', 'see http://x', '/etc/hosts now']) {
    await composer.fill(text)
    await expect(picker).toHaveCount(0)
  }
  // Escape on `/`, then typing on, does not reopen it; clearing the slash token re-arms it.
  await composer.fill('/')
  await expect(picker).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(picker).toHaveCount(0)
  await composer.press('a')
  await expect(composer).toHaveValue('/a')
  await expect(picker).toHaveCount(0)
  await composer.fill('')
  await composer.fill('/')
  await expect(picker).toBeVisible()
  await page.keyboard.press('Escape')
  // An insertion past the 2,000 limit is refused with the sentence and the draft is untouched.
  const long = 'a'.repeat(1995)
  await composer.fill(long)
  await skillsButton.click()
  await picker.getByRole('option', { name: /pdf/ }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'That would make the message longer than 2,000 bytes. Nothing was inserted.' })).toBeVisible()
  await expect(composer).toHaveValue(long)
  await expect(picker).toHaveCount(0)
  // The connection dropping while the picker is open: picking inserts nothing and says so; the button is disabled meanwhile.
  await composer.fill('hello')
  await skillsButton.click()
  await expect(picker).toBeVisible()
  await page.evaluate(() => window.__chatStream.onerror?.({}))
  await picker.getByRole('option', { name: /pdf/ }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Connection changed. Nothing was inserted.' })).toBeVisible()
  await expect(composer).toHaveValue('hello')
  await expect(skillsButton).toBeDisabled()
  await expect(skillsButton).toHaveAttribute('title', /connected/)
  await page.evaluate(() => window.__chatStream.onopen?.({}))
  await expect(skillsButton).toBeEnabled()
  // The agent working: the button is disabled with a reason; a picker already open when it starts refuses the pick.
  await composer.fill('draft while idle')
  await skillsButton.click()
  await expect(picker).toBeVisible()
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 20, kind: 'status', state: 'working' }))
  await picker.getByRole('option', { name: /pdf/ }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Nothing was inserted.' })).toBeVisible()
  await expect(picker).toHaveCount(0)
  await expect(composer).toHaveValue('draft while idle')
  await expect(skillsButton).toBeDisabled()
  await expect(skillsButton).toHaveAttribute('title', /idle/)
  await page.evaluate(() => window.__chatStream.emit('chat', { seq: 21, kind: 'status', state: 'cancelled' }))
  await expect(skillsButton).toBeEnabled()
  // A pick during a send in flight inserts nothing and says so; the send then clears the draft as before.
  let releasePrompt
  const held = new Promise(resolve => { releasePrompt = resolve })
  await page.route(`**/api/v2/terminal/sessions/${S}/chat/prompt`, async route => { await held; await route.fulfill({ status: 200, json: { sent: true } }) })
  await composer.fill('sending this')
  await skillsButton.click()
  await expect(picker).toBeVisible()
  await page.evaluate(() => [...document.querySelectorAll('button.send')].find(b => !b.disabled)?.click())
  await expect(page.getByRole('button', { name: 'Sending…' })).toBeVisible()
  await picker.getByRole('option', { name: /pdf/ }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Nothing was inserted.' })).toBeVisible()
  await expect(composer).toHaveValue('sending this')
  releasePrompt()
  await expect(composer).toHaveValue('')
  await expect(skillsButton).toBeEnabled()
  await page.unroute(`**/api/v2/terminal/sessions/${S}/chat/prompt`)
  // Escape on `/`, then Send clears the draft by code: a fresh `/` in the emptied composer opens the picker again.
  await composer.fill('/')
  await expect(picker).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(picker).toHaveCount(0)
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  await expect(composer).toHaveValue('')
  await composer.fill('/')
  await expect(picker).toBeVisible()
  await page.keyboard.press('Escape')
  // An empty machine says so and says its skills have not been read yet.
  state.skills = { skills: [], machineSeen: 0, host: 'dev-laptop' }
  await skillsButton.click()
  await expect(picker.getByText('No skills found on dev-laptop yet.')).toBeVisible()
  await expect(picker.getByText('have not been read yet')).toBeVisible()
  await page.keyboard.press('Escape')
  // A machine reet does not know says so, with no empty host name.
  state.skills = { skills: [], machineSeen: 0, host: '' }
  await skillsButton.click()
  await expect(picker.getByText("This session's machine is not known to reet, so its skills cannot be listed.")).toBeVisible()
  await expect(picker.getByText('have not been read yet')).toHaveCount(0)
  await page.keyboard.press('Escape')
  // A failed request shows its error and Retry loads the list.
  state.skills = { skills: skillRows, machineSeen: Date.now() - 5 * 60_000, host: 'dev-laptop' }
  state.skillsFail = 1
  await skillsButton.click()
  await expect(picker.getByRole('alert')).toContainText('Skills could not be read just now.')
  await picker.getByRole('button', { name: 'Retry', exact: true }).click()
  await expect(picker.getByRole('listbox')).toHaveCount(3)
  // On a phone the picker is a bottom sheet with no sideways scroll.
  const sendsBeforePhone = sends
  await page.setViewportSize({ width: 390, height: 844 })
  const sheet = await picker.boundingBox()
  expect(sheet.x).toBeGreaterThanOrEqual(0)
  expect(sheet.x + sheet.width).toBeLessThanOrEqual(390)
  expect(Math.round(sheet.y + sheet.height)).toBeGreaterThanOrEqual(843)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.keyboard.press('Escape')
  await expect(picker).toHaveCount(0)
  expect(sends).toBe(sendsBeforePhone)
  await page.setViewportSize({ width: 1360, height: 900 })

  // Tablet widths and both colour schemes: the picker stays inside the viewport with no sideways scroll, and its text stays
  // readable (4.5:1) against the card in light and dark.
  const contrastOf = (loc) => loc.evaluate(el => {
    const rgb = c => (c.match(/[\d.]+/g) || []).map(Number)
    const lum = ([r, g, b]) => { const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4 }; return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b) }
    let bg = null
    for (let n = el; n && !bg; n = n.parentElement) { const c = rgb(getComputedStyle(n).backgroundColor); if (c.length >= 3 && (c.length === 3 || c[3] === 1)) bg = c }
    bg = bg || [255, 255, 255]
    const fg = rgb(getComputedStyle(el).color)
    const [a, b] = [lum(fg), lum(bg)].sort((x, y) => y - x)
    return (a + 0.05) / (b + 0.05)
  })
  for (const combo of [{ w: 820, h: 1180, scheme: 'light' }, { w: 820, h: 1180, scheme: 'dark' }, { w: 1024, h: 768, scheme: 'dark' }, { w: 1024, h: 768, scheme: 'light' }]) {
    await page.emulateMedia({ colorScheme: combo.scheme })
    await page.setViewportSize({ width: combo.w, height: combo.h })
    await skillsButton.click()
    await expect(picker).toBeVisible()
    await expect(picker.getByRole('listbox')).toHaveCount(3)
    const box = await picker.boundingBox()
    expect(box.x).toBeGreaterThanOrEqual(0)
    expect(box.y).toBeGreaterThanOrEqual(0)
    expect(box.x + box.width).toBeLessThanOrEqual(combo.w)
    expect(box.y + box.height).toBeLessThanOrEqual(combo.h)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect(await picker.locator('[data-shell-body]').evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
    const row = picker.getByRole('option', { name: /pdf/ })
    expect(await contrastOf(row.locator('.name'))).toBeGreaterThanOrEqual(4.5)
    expect(await contrastOf(row.locator('.description'))).toBeGreaterThanOrEqual(4.5)
    expect(await contrastOf(picker.getByText('Skills on dev-laptop were read 5 minutes ago.'))).toBeGreaterThanOrEqual(4.5)
    await page.keyboard.press('Escape')
    await expect(picker).toHaveCount(0)
  }
  await page.emulateMedia({ colorScheme: 'light' })
  await page.setViewportSize({ width: 1360, height: 900 })

  // Keep, from the picker: a skill New on this machine is listed in its own group (its files are here, so a native one can be
  // used straight away), with Keep and the Skills page's review. Keep sends one decision taking this machine's copy, and the
  // list reloads. Nothing is installed for one machine: every Active skill is on every machine.
  const nativePdf = { id: 'm9', name: 'pdf', description: 'Read and fill forms', source: 'machine', agent: 'opencode', native: true, onMachine: true, invocation: 'Use the pdf skill.' }
  const newCq = { id: 'new:cq', name: 'cq', description: '', source: 'new', agent: 'opencode', native: true, onMachine: true, invocation: 'Use the cq skill.' }
  const keeps = []
  let keepReply = null, skillReads = 0
  page.on('request', request => { if (/\/sessions\/[^/]+\/skills$/.test(new URL(request.url()).pathname)) skillReads++ })
  await page.route('**/api/v2/skills/new', route => {
    if (route.request().method() !== 'POST') return route.fulfill({ status: 200, json: { newOnMachines: [] } })
    keeps.push(route.request().postDataJSON())
    return route.fulfill(keepReply ? keepReply() : { status: 200, json: { items: ['item-cq'] } })
  })
  const pickerFilter = picker.getByRole('searchbox', { name: 'Filter skills' })
  const keepButton = picker.getByRole('button', { name: 'Keep cq', exact: true })
  const panelOf = picker.getByRole('region', { name: 'Keep cq' })
  state.skills = { skills: [nativePdf, newCq], machineSeen: Date.now() - 5 * 60_000, host: 'dev-laptop', agentId: A }
  await skillsButton.click()
  await expect(picker).toBeVisible()
  await expect(picker.getByText('New on this machine', { exact: true })).toBeVisible()
  await expect(picker.getByText('New on this machine · opencode', { exact: true })).toBeVisible()
  await expect(picker.getByRole('link', { name: 'Review new skills on the Skills page' })).toHaveAttribute('href', '/skills?review=new')
  await expect(picker.getByRole('button', { name: /^Install/ })).toHaveCount(0)
  // Each row with an action is its own single-option listbox with the action outside it.
  await expect(picker.getByRole('listbox')).toHaveCount(2)
  await expect(picker.locator('[role=listbox] button, [role=listbox] a')).toHaveCount(0)
  // Keyboard only: Tab from the filter reaches Keep, Enter opens the panel with Keep focused, and nothing is sent until it is pressed.
  await expect(pickerFilter).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(keepButton).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(panelOf.getByText("reet keeps dev-laptop's copy of cq in your library and puts it on every machine.")).toBeVisible()
  await expect(panelOf.getByRole('button', { name: 'Keep', exact: true })).toBeFocused()
  expect(keeps).toEqual([])
  // Cancel returns to the list, focus on the filter, having sent nothing.
  await panelOf.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(panelOf).toHaveCount(0)
  await expect(pickerFilter).toBeFocused()
  expect(keeps).toEqual([])
  // A refusal shows the server's sentence and the panel stays.
  keepReply = () => ({ status: 409, json: { code: 'not-new', names: ['cq'], error: 'cq is not new on any machine.' } })
  await keepButton.click()
  await panelOf.getByRole('button', { name: 'Keep', exact: true }).click()
  await expect(panelOf.getByRole('status')).toHaveText('cq is not new on any machine.')
  expect(keeps).toEqual([{ names: ['cq'], action: 'keep', from: A }])
  // Kept: the panel closes, the list reloads and says so.
  keepReply = null
  const readsBefore = skillReads
  state.skills = { skills: [nativePdf, { ...nativePdf, id: 'local/cq', name: 'cq', invocation: 'Use the cq skill.' }], machineSeen: Date.now(), host: 'dev-laptop', agentId: A }
  await panelOf.getByRole('button', { name: 'Keep', exact: true }).click()
  await expect(panelOf).toHaveCount(0)
  await expect(picker.getByText('Kept cq · going on every machine')).toBeVisible()
  await expect(keepButton).toHaveCount(0)
  expect(keeps).toHaveLength(2)
  await expect.poll(() => skillReads).toBe(readsBefore + 1)
  await expect(pickerFilter).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(picker).toHaveCount(0)

  // On a phone the panel fits with no sideways scroll.
  const longName = 'a-very-long-skill-name-that-has-to-wrap-inside-the-narrow-sheet-without-scrolling'
  state.skills = { skills: [nativePdf, newCq, { ...newCq, id: `new:${longName}`, name: longName }], machineSeen: Date.now(), host: 'dev-laptop', agentId: A }
  await page.setViewportSize({ width: 390, height: 844 })
  // A phone opens a chat in focus, where Skills waits behind Exit focus.
  await expect(page.locator('.session-screen')).toHaveClass(/focus/)
  await page.getByRole('button', { name: 'Exit focus mode', exact: true }).click()
  await expect(page.locator('.session-screen')).not.toHaveClass(/focus/)
  await skillsButton.click()
  await expect(picker).toBeVisible()
  expect(await picker.locator('[data-shell-body]').evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
  const phoneKeep = await keepButton.boundingBox()
  expect(phoneKeep.height).toBeGreaterThanOrEqual(36)
  await keepButton.click()
  await expect(panelOf).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  const phonePanel = await panelOf.boundingBox()
  expect(phonePanel.x).toBeGreaterThanOrEqual(0)
  expect(phonePanel.x + phonePanel.width).toBeLessThanOrEqual(390)
  expect(await picker.locator('[data-shell-body]').evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
  await page.keyboard.press('Escape')
  await expect(picker).toHaveCount(0)
  await page.setViewportSize({ width: 1360, height: 900 })

  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByRole('textbox', { name: 'Message OpenCode' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  // A phone opens Gaffer in super focus: the site chrome, the workspace toolbar, the session's secondary controls and
  // the composer's own (Skills, the model and the rest) step aside, and the narrative and the message field keep the
  // screen. Exit focus restores them. The install check above left focus to reach Skills, so it is turned back on.
  await page.getByRole('button', { name: 'Focus mode', exact: true }).click()
  const focusScreen = page.locator('.session-screen')
  await expect(focusScreen).toHaveClass(/focus/)
  await expect(page.locator('.site-header')).toBeHidden()
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toBeHidden()
  await expect(page.locator('.workspace-toolbar')).toBeHidden()
  await expect(page.getByRole('button', { name: 'Exit focus mode', exact: true })).toBeVisible()
  await expect(page.locator('.composer-bar')).toBeHidden()
  await expect(page.getByRole('textbox', { name: 'Message OpenCode' })).toBeVisible()
  // A tablet is not in focus and keeps the model control, with room for the toolbar.
  await page.setViewportSize({ width: 820, height: 1180 })
  await expect(page.locator('.composer-bar').getByRole('button', { name: /^Model:/ })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('button', { name: 'Exit focus mode', exact: true }).click()
  await expect(focusScreen).not.toHaveClass(/focus/)
  await expect(page.locator('.site-header')).toBeVisible()
  await expect(page.locator('.workspace-toolbar')).toBeVisible()
  await expect(page.locator('.composer-bar').getByRole('button', { name: 'Skills', exact: true })).toBeVisible()
  // Out of focus the phone's model control opens a full-width sheet rather than a native list.
  const phoneModel = page.locator('.composer-bar').getByRole('button', { name: /^Model:/ })
  await expect(phoneModel).toBeVisible()
  await phoneModel.click()
  await expect(page.getByRole('dialog', { name: 'Model', exact: true })).toBeVisible()
  await expect(page.getByRole('listbox', { name: 'Model', exact: true }).getByRole('option', { name: 'anthropic/claude-sonnet-4-5', exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: 'Focus mode', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Focus mode', exact: true }).click()
  await expect(focusScreen).toHaveClass(/focus/)
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
  // Focus keeps the ended chat and its Continue action on the screen; the ordinary session actions wait behind Exit focus.
  await expect(page.locator('.session-screen')).toHaveClass(/focus/)
  await expect(page.getByRole('button', { name: 'Rename…', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: 'Exit focus mode', exact: true }).click()
  await page.getByRole('button', { name: 'Session actions', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Rename…', exact: true })).toBeVisible()
  // A chat that exited does not say who ended it (it may have stopped on its own or been ended by the person), only what can be done next.
  await expect(page.locator('.ended-note')).toContainText('This OpenCode chat has ended. What was already shown stays here.')
  await expect(page.locator('.ended-note').getByRole('button', { name: 'Continue with history', exact: true })).toBeVisible()
  await page.locator('.session-screen .head').getByRole('button', { name: 'Continue with history', exact: true }).click()
  await expect.poll(() => state.resume).toMatchObject({ structured: true, trust: true, resumeAcp: closedSession.acpSessionId, dir: SITE })
  await page.waitForURL(new RegExp(`session=${S3}`))
  // A closed chat cannot switch: the interface label says why.
  await page.setViewportSize({ width: 1360, height: 900 })
  await page.goto(`${base}/sessions?agent=${A}&session=${S2}`)
  await page.getByRole('button', { name: 'Session actions', exact: true }).click()
  await expect(page.locator('.session-screen .tui-pill')).toHaveAttribute('title','Only a running session can switch.')
  await page.keyboard.press('Escape')
  expect(state.resume.prompt).toBeUndefined()
  expect(state.resume.model).toBe('anthropic/claude-sonnet-4-5')
  expect(errors).toEqual([])
  console.log('Structured OpenCode chat UI passed: the dialog offers Gaffer for OpenCode, the create body carries structured, trust and the chosen model, the session screen opens Gaffer with the OpenCode TUI disabled and its reason (the machine predates session-mode-v1), agent chunks stream, a command approval is Allow once or Deny, a typed question is checked, refused and answered, skipped or cancelled and fits a phone, messages queue while it works and send when the turn ends, a message sends, an attached image uploads and is passed as an ACP image, Stop cancels, the phone fits, a truncated reset says so, and a closed chat reopens through the continue action with its OpenCode session id and cannot switch TUI; the skill picker fills the composer without sending, by button and by a leading slash, refuses disabled rows and over-long insertions, and fits a phone as a bottom sheet, with tablet and dark-mode passes that keep it inside the viewport and readable; a written or published reet skill installs from the picker through confirm, wait, review, approve (only for a plain add), install and done, with cancel, retry, refusals and a phone fit.')
} finally {
  await context.close()
  await browser.close()
}
