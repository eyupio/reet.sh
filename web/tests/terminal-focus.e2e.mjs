// The session screen on phones and tablets: focus mode for a terminal (on by default on a phone), the room it gives
// back, Ask Gaffer in the header on touch screens, a tablet with its keyboard up, the sessions drawer and docked
// panel, the Sessions tab's running list, the composer while typing, and a choice not yet made in Start an agent.
// Synthetic API answers and a fixture terminal socket that repaints for the size it is given.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY, run = name => !only || only === name
const now = Date.now(), minute = 60_000
const caps = { protocolVersion: 1, os: 'linux', arch: 'amd64', osUser: 'sam', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready', launch: true, browse: true, branches: true, worktree: true, agentsV2: true, acp: true, acpModel: true, tools: ['claude', 'codex', 'opencode'] }
const machines = [{ agentId: 'a1', host: 'studio', ready: true, serverEnabled: true, capabilities: caps }, { agentId: 'a2', host: 'laptop', ready: true, serverEnabled: true, capabilities: caps }]
const project = (id, name, agent, host, colour, position) => ({ id, name, host: agent, machine: host, dir: `/home/sam/${id}`, isRepo: true, baseBranch: 'main', worktreeDefault: 'inherit', defaultTool: 'claude', defaultModel: '', defaultEffort: '', hasStarterPrompt: false, colour, note: '', position, createdAt: now, updatedAt: now, version: 1 })
const projects = [project('p1', 'Reet', 'a1', 'studio', 'green', 0), project('p2', 'Storefront and checkout', 'a1', 'studio', 'blue', 1)]
const session = (id, title, over = {}) => ({ id, title, agentId: 'a1', projectId: 'p1', dir: '/home/sam/p1', state: 'running', tool: 'claude', model: 'sonnet', attention: 'working', attentionSource: 'terminal', createdAt: now - 80 * minute, updatedAt: now - 5 * minute, exitCode: null, ...over })
const sessions = [
  session('t1', 'Refactor the store layer so reads and writes share one interface'),
  session('c1', 'Fix the failing checkout build', { tool: 'opencode', model: 'anthropic/claude-sonnet-4-5', structured: true, projectId: 'p2', dir: '/home/sam/p2', attentionSource: 'chat' }),
  session('t2', 'Speed up the runner pool', { agentId: 'a2', projectId: null, attention: 'waiting', changedSinceSeen: true, updatedAt: now - 40 * minute }),
  session('t3', 'Tidy up the keyboard shortcuts', { state: 'exited', attention: 'finished', exitCode: 0 }),
]
// A full-screen app that repaints every row for the size it was given, with its input on the last row.
const app = (cols, rows) => { let out = '\x1b[?1049h\x1b[2J'; for (let row = 1; row <= rows; row++) out += `\x1b[${row};1H` + (row === 1 ? `agent ${cols}x${rows}` : row === rows ? '> ask anything' : `row ${row}`).slice(0, cols); return Buffer.from(out) }
// A visual viewport whose height a test can shrink, as an on-screen keyboard does, and a chat stream that says nothing.
const hooks = () => {
  const events = new EventTarget(), real = window.visualViewport
  let height = null
  const visual = { get width() { return innerWidth }, get height() { return height ?? innerHeight }, offsetTop: 0, offsetLeft: 0, pageTop: 0, pageLeft: 0, scale: 1, addEventListener: (...args) => { events.addEventListener(...args); real?.addEventListener(...args) }, removeEventListener: (...args) => { events.removeEventListener(...args); real?.removeEventListener(...args) } }
  Object.defineProperty(window, 'visualViewport', { configurable: true, get: () => visual })
  window.__keyboard = value => { height = value; events.dispatchEvent(new Event('resize')) }
  // A chat stream that says what a test tells it to.
  window.EventSource = class { constructor() { this.listeners = {}; window.__chatStream = this; setTimeout(() => this.onopen?.({}), 0) } addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn) } close() {} emit(type, data) { for (const fn of this.listeners[type] ?? []) fn({ data: JSON.stringify(data) }) } }
}
// A speech recognition that hears what the test tells it to.
const speechStandIn = () => {
  window.SpeechRecognition = class {
    constructor() { window.__speech = this }
    start() { window.__listening = true }
    stop() { window.__listening = false; this.onend?.() }
    abort() { window.__listening = false; this.onend?.() }
    hear(text, final = true) { this.onresult?.({ resultIndex: 0, results: [Object.assign([{ transcript: text }], { isFinal: final })] }) }
    fail(error) { this.onerror?.({ error }); this.stop() }
  }
}
async function open({ width, height, touch = true, path = '/sessions?agent=a1&session=t1', sessionFocus, speech } = {}) {
  const sizes = []
  const opened = await openWorkspace(browser, { width, height, touch, theme: 'dark', sessionFocus, terminalStatus: active, terminalMachines: machines, routes: {
    '/api/v2/machines/health': () => ({ serverNow: Date.now(), machines: machines.map(m => ({ ...m, state: 'connected', connection: { connected: true, reportFresh: true, missedProbes: 0, reconnects: 0, path: 'direct' } })) }),
    '/api/v2/terminal/sessions': { sessions }, '/api/v2/folder-projects': { projects, savedWaiting: 0 }, '/api/v2/terminal/commands': { commands: [] },
    '/api/v2/terminal/worktrees/status': { path: '', branch: '', changed: 0, unpushed: 0, paths: [], unreferenced: 0, hidden: 0, nested: false, incomplete: false },
    '/api/v2/settings/yolo': { allowed: false },
    '/api/v2/terminal/sessions/c1/chat/prompt': (request, state) => { state.prompt = request.postDataJSON(); return { sent: true } },
  } })
  const { page } = opened
  await page.addInitScript(hooks)
  if (speech === true) await page.addInitScript(speechStandIn)
  if (speech === false) await page.addInitScript(() => { delete window.SpeechRecognition; delete window.webkitSpeechRecognition })
  await page.route('**/attachments', async route => { const body = route.request().postDataJSON(); sizes.push({ cols: body.cols, rows: body.rows }); await route.fulfill({ json: { ticket: 'fixture-ticket00000000000' } }) })
  await page.route('**/history', route => route.fulfill({ body: 'saved output\n' }))
  await page.routeWebSocket('**/api/v2/terminal/socket', socket => socket.onMessage(message => {
    if (typeof message !== 'string') return
    const data = JSON.parse(message)
    if (data.ticket) { socket.send('{"type":"ready"}'); const size = sizes.at(-1); socket.send(app(size.cols, size.rows)) }
    else if (data.type === 'resize') { sizes.push({ cols: data.cols, rows: data.rows }); socket.send(app(data.cols, data.rows)) }
  }))
  await page.goto(base + path)
  return { ...opened, sizes }
}
const live = page => expect(page.locator('.pane')).toHaveAttribute('data-connection-state', 'live')
// The rows the host was last told, once no new size has been sent for half a second.
async function rows(page, sizes) {
  for (let last = -1; last !== sizes.length;) { last = sizes.length; await page.waitForTimeout(500) }
  return sizes.at(-1).rows
}
const shown = (page, selector) => page.evaluate(selector => { const e = document.querySelector(selector); return !!e && e.getClientRects().length > 0 }, selector)
const screen = page => page.locator('.session-screen')
const noSidewaysScroll = page => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)

try {
  if (run('phone focus')) {
    // A first visit: the terminal is in focus. The site header, workspace toolbar, pane header and status footer step aside.
    const { page, sizes, errors, context } = await open({ width: 390, height: 844 })
    await live(page)
    await expect(screen(page)).toHaveClass(/focus/)
    for (const selector of ['.site-header', '.workspace-toolbar', '.pane > header', '.pane > footer']) expect(await shown(page, selector), `${selector} in focus`).toBe(false)
    for (const selector of ['.terminal-toolbar', '.mobile-keys']) expect(await shown(page, selector), `${selector} in focus`).toBe(true)
    const focused = await rows(page, sizes)
    expect(focused).toBeGreaterThanOrEqual(44)
    // The terminal reaches the foot of the screen: no strip is held back under the keys.
    const keys = await page.locator('.mobile-keys').boundingBox()
    expect(844 - (keys.y + keys.height)).toBeLessThanOrEqual(16)
    // Ask Gaffer is in the header at 44 px, and no circle floats over the agent's prompt.
    const entry = screen(page).locator('.head .gaffer-entry')
    await expect(entry).toBeVisible()
    const entryBox = await entry.boundingBox()
    expect(Math.round(entryBox.width)).toBe(44); expect(Math.round(entryBox.height)).toBe(44)
    await expect(page.locator('.gaffer-launcher')).toBeHidden()
    // Session actions stay: the pane header, where End and Rename also live, has stepped aside.
    await expect(screen(page).getByRole('button', { name: 'Session actions', exact: true })).toBeVisible()
    // The session's name gets the first row to itself.
    await expect(screen(page).locator('.title')).toHaveText(sessions[0].title)
    expect(await noSidewaysScroll(page)).toBe(true)

    // Exit focus brings the full screen back, and the terminal is told its smaller size.
    const exit = screen(page).getByRole('button', { name: 'Exit focus mode', exact: true })
    await expect(exit).toHaveAttribute('aria-pressed', 'true')
    await exit.click()
    await expect(screen(page)).not.toHaveClass(/focus/)
    for (const selector of ['.site-header', '.workspace-toolbar', '.pane > header', '.pane > footer']) expect(await shown(page, selector), `${selector} after Exit focus`).toBe(true)
    const full = await rows(page, sizes)
    expect(focused - full).toBeGreaterThanOrEqual(12)
    expect(await page.evaluate(() => localStorage.getItem('reet-terminal-layout:1:context:focus'))).toBe('0')

    // Focus again: the button does what it says for a terminal, and does not raise the keyboard by focusing it.
    const focus = screen(page).getByRole('button', { name: 'Focus mode', exact: true })
    await expect(focus).toHaveAttribute('title', 'Give the screen to the terminal')
    await focus.click()
    await expect(screen(page)).toHaveClass(/focus/)
    expect(await rows(page, sizes)).toBe(focused)
    expect(await page.evaluate(() => document.activeElement?.classList.contains('xterm-helper-textarea'))).toBe(false)

    // Typing with the keyboard up: one header row, and the rest above the keyboard is the terminal.
    await page.locator('.xterm-helper-textarea').focus()
    await page.evaluate(() => window.__keyboard(490))
    await expect(screen(page)).toHaveClass(/keyboard/)
    await expect(screen(page).locator('.header-actions')).toBeHidden()
    await expect(screen(page).locator('.title')).toBeVisible()
    expect(await rows(page, sizes)).toBeGreaterThanOrEqual(24)
    await page.evaluate(() => { window.__keyboard(null); document.activeElement?.blur() })
    await expect(screen(page).locator('.header-actions')).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('sideways')) {
    const { page, sizes, context } = await open({ width: 844, height: 390 })
    await live(page)
    await expect(screen(page)).toHaveClass(/focus/)
    expect(await rows(page, sizes)).toBeGreaterThanOrEqual(18)
    await expect(page.locator('.gaffer-launcher')).toBeHidden()
    await expect(screen(page).locator('.head .gaffer-entry')).toBeVisible()
    await context.close()
  }

  if (run('tablet')) {
    // A touch tablet keeps the full screen (focus mode is a phone's), with the sessions panel docked.
    const { page, sizes, errors, context } = await open({ width: 820, height: 1180 })
    await live(page)
    await expect(screen(page)).not.toHaveClass(/focus/)
    await expect(screen(page).getByRole('button', { name: 'Focus mode', exact: true })).toBeHidden()
    await expect(page.locator('.gaffer-launcher')).toBeHidden()
    await expect(screen(page).locator('.head .gaffer-entry')).toBeVisible()
    // Beside a docked panel the name has its own row: nothing is cut while there is room on the line.
    const title = screen(page).locator('.title')
    const titleBox = await title.boundingBox(), headBox = await screen(page).locator('.head').boundingBox()
    expect(titleBox.width).toBeGreaterThan(headBox.width * 0.6)
    // Each project's header in the panel is one line, its name shortened, its count and buttons level with it.
    const panel = page.locator('#terminal-sessions')
    await expect(panel).toBeVisible()
    for (const header of await panel.locator('.session-group.tree h2').all()) {
      const box = await header.boundingBox()
      expect(box.height, await header.innerText()).toBeLessThanOrEqual(48)
      const marks = await header.locator(':scope > small, :scope > button').evaluateAll(nodes => nodes.map(n => { const r = n.getBoundingClientRect(); return Math.round(r.top + r.height / 2) }))
      expect(Math.max(...marks) - Math.min(...marks)).toBeLessThanOrEqual(2)
    }
    expect(await panel.locator('.fold-title', { hasText: 'Storefront and checkout' }).evaluate(e => getComputedStyle(e).whiteSpace)).toBe('nowrap')
    await expect(panel.getByRole('link', { name: 'Manage projects' })).toHaveCSS('white-space', 'nowrap')

    // The keyboard up: the top bar, workspace toolbar, pane header and footer give their rows to the terminal.
    const before = await rows(page, sizes)
    await page.locator('.xterm-helper-textarea').focus()
    await page.evaluate(() => window.__keyboard(680))
    await expect(screen(page)).toHaveClass(/keyboard/)
    for (const selector of ['.site-header', '.workspace-toolbar', '.pane > header', '.pane > footer']) expect(await shown(page, selector), `${selector} with the tablet keyboard up`).toBe(false)
    const typing = await rows(page, sizes)
    expect(typing).toBeGreaterThanOrEqual(26)
    await page.evaluate(() => { window.__keyboard(null); document.activeElement?.blur() })
    await expect(screen(page)).not.toHaveClass(/keyboard/)
    for (const selector of ['.site-header', '.workspace-toolbar', '.pane > header', '.pane > footer']) expect(await shown(page, selector), `${selector} after the keyboard goes`).toBe(true)
    expect(await rows(page, sizes)).toBe(before)
    expect(await noSidewaysScroll(page)).toBe(true)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('sessions tab')) {
    // The Sessions tab with nothing chosen: every machine's running sessions, what needs the person first.
    const { page, context, errors } = await open({ width: 390, height: 844, path: '/sessions' })
    await expect(page.getByRole('heading', { name: 'Pick up where you left off' })).toBeVisible()
    await expect(page.getByText('3 sessions are running. Jump back into one, or start something new.')).toBeVisible()
    const list = page.getByLabel('Running sessions')
    await expect(list.getByRole('button')).toHaveText([/Speed up the runner pool\s*laptop · Needs you/, /Refactor the store layer.*\s*studio · Working/, /Fix the failing checkout build\s*studio · Working/])
    for (const button of await list.getByRole('button').all()) expect((await button.boundingBox()).height).toBeGreaterThanOrEqual(48)
    // The list comes before the start buttons.
    expect((await list.boundingBox()).y).toBeLessThan((await page.getByRole('button', { name: '✦ Start a new agent session' }).boundingBox()).y)

    // The drawer is wide enough to read a name, over a scrim; a tap on the scrim closes it.
    await page.getByRole('button', { name: 'Sessions panel' }).click()
    const drawer = page.locator('#terminal-sessions.drawer')
    await expect(drawer).toBeVisible()
    expect((await drawer.boundingBox()).width).toBeGreaterThanOrEqual(320)
    const scrim = page.locator('.drawer-scrim')
    await expect(scrim).toBeVisible()
    const name = drawer.locator('.session-row strong', { hasText: 'Fix the failing checkout build' })
    expect(await name.evaluate(e => e.scrollWidth <= e.clientWidth)).toBe(true)
    await scrim.click({ position: { x: 370, y: 300 } })
    await expect(drawer).toHaveCount(0)
    await expect(scrim).toHaveCount(0)

    // A row opens that session.
    await list.getByRole('button', { name: /Refactor the store layer/ }).click()
    await live(page)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('chat typing')) {
    // A chat in focus on a phone gives the conversation the screen: one header row (Back, the name, the state, Ask
    // Gaffer, Exit focus), the agent's state said once in the strip above the composer, and a message field with its
    // send button. The view switch, Details, the session's actions and the composer's Prompts, Skills, image,
    // dictation, effort and model wait behind Exit focus. Typing changes none of that; the header's buttons step aside.
    const { page, context } = await open({ width: 390, height: 844, path: '/sessions?agent=a1&session=c1' })
    const input = page.locator('textarea.composer-input')
    await expect(input).toBeVisible()
    await expect(page.locator('.gaffer-launcher')).toBeHidden()
    const quiet = async () => {
      // The line under the header is kept for a screen reader and takes no room while the chat is simply idle or working.
      const box = await page.locator('.chat-status').boundingBox()
      expect(box === null || box.height <= 1).toBe(true)
      await expect(page.getByRole('button', { name: 'Prompts', exact: true })).toBeHidden()
      await expect(page.getByRole('button', { name: 'Skills', exact: true })).toBeHidden()
      await expect(page.locator('.model-controls')).toBeHidden()
      await expect(page.getByRole('button', { name: /^Send/ })).toBeVisible()
    }
    await quiet()
    await expect(screen(page).getByRole('button', { name: 'Session actions', exact: true })).toBeHidden()
    await expect(screen(page).getByRole('group', { name: 'Session view', exact: true })).toBeHidden()
    await expect(screen(page).getByRole('button', { name: 'Session details panel', exact: true })).toBeHidden()
    await expect(screen(page).locator('.header-actions').getByRole('button', { name: 'Ask Gaffer', exact: true })).toBeVisible()
    await expect(screen(page).getByRole('button', { name: 'Exit focus mode', exact: true })).toBeVisible()
    // One row: the header is no taller than its 44 px controls and the safe area.
    expect((await screen(page).locator('.head').boundingBox()).height).toBeLessThanOrEqual(52)
    await input.focus()
    await page.evaluate(() => window.__keyboard(490))
    await expect(screen(page)).toHaveClass(/keyboard/)
    await expect(screen(page).locator('.header-actions')).toBeHidden()
    await quiet()
    await page.evaluate(() => { window.__keyboard(null); document.activeElement?.blur() })
    await quiet()
    // Exit focus brings all of it back.
    await screen(page).getByRole('button', { name: 'Exit focus mode', exact: true }).click()
    await expect(page.locator('.chat-status')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Prompts', exact: true })).toBeVisible()
    await expect(screen(page).getByRole('group', { name: 'Session view', exact: true })).toBeVisible()
    await context.close()
  }

  if (run('chat queue')) {
    // Queued messages in focus are one line that opens: how many, what goes next, and how many were not sent. The
    // list, with each message's own actions, is one tap away and nothing in it is lost.
    const { page, context } = await open({ width: 390, height: 844, path: '/sessions?agent=a1&session=c1' })
    const queue = [
      { id: 'q1', text: 'This one was refused.', images: [], state: 'failed', intent: 'wait', note: 'The agent refused this message.', revision: 2, origin: '' },
      { id: 'q2', text: 'Add the missing error case, then run the tests again.', images: [], state: 'queued', intent: 'wait', note: '', revision: 1, origin: 'companion' },
      { id: 'q3', text: 'Also check the header order.', images: [], state: 'queued', intent: 'wait', note: '', revision: 1, origin: '' },
    ]
    await page.route('**/api/v2/terminal/sessions/c1/chat/queue', route => route.fulfill({ json: { queue } }))
    // The queue belongs to a machine with live chat controls.
    await page.route('**/api/v2/terminal/machines', route => route.fulfill({ json: { machines: machines.map(m => ({ ...m, capabilities: { ...caps, acpLiveControls: true } })) } }))
    await page.route('**/api/v2/terminal/sessions/c1/chat/controls', route => route.fulfill({ json: { controls: { working: true, waiting: false, steering: true, model: 'm', options: [] } } }))
    await page.reload()
    const region = page.getByRole('region', { name: 'Queued messages', exact: true })
    const summary = region.getByRole('button', { name: /^Queued 3/ })
    await expect(summary).toBeVisible()
    await expect(summary).toHaveAttribute('aria-expanded', 'false')
    await expect(summary).toContainText('1 not sent')
    // What goes next is the first message that can still go, and Gaffer's is named as Gaffer's.
    await expect(summary).toContainText('Gaffer: Add the missing error case')
    await expect(region.getByRole('listitem')).toHaveCount(0)
    expect((await region.boundingBox()).height).toBeLessThanOrEqual(56)
    await summary.click()
    await expect(summary).toHaveAttribute('aria-expanded', 'true')
    await expect(region.getByRole('listitem')).toHaveCount(3)
    await expect(region.getByRole('button', { name: 'Retry', exact: true })).toBeVisible()
    await expect(region.getByRole('button', { name: 'Stop and send', exact: true })).toBeVisible()
    await summary.click()
    await expect(region.getByRole('listitem')).toHaveCount(0)
    // Out of focus the list is simply shown, as before, with no summary line.
    await screen(page).getByRole('button', { name: 'Exit focus mode', exact: true }).click()
    await expect(region.getByRole('listitem')).toHaveCount(3)
    await expect(summary).toBeHidden()
    await context.close()
  }

  if (run('start an agent')) {
    // A choice not made yet reads as guidance, not as a mistake.
    const { page, context } = await open({ width: 390, height: 844, path: '/sessions' })
    await page.getByRole('button', { name: '✦ Start a new agent session' }).click()
    const dialog = page.getByRole('dialog', { name: 'Start an agent' })
    const hint = dialog.getByText('Choose a project, or choose No project to start in a folder.')
    await expect(hint).toBeVisible()
    const colours = await hint.evaluate(e => { const probe = document.createElement('span'); probe.style.color = 'var(--bad)'; document.body.append(probe); const bad = getComputedStyle(probe).color; probe.style.color = 'var(--muted)'; const muted = getComputedStyle(probe).color; probe.remove(); return { is: getComputedStyle(e).color, bad, muted } })
    expect(colours.is).toBe(colours.muted)
    expect(colours.is).not.toBe(colours.bad)
    await context.close()
  }
  if (run('approval said once fewer')) {
    // An approval on a phone is said by the pane's status line and by the card that asks; the strip above the composer
    // does not say it a third time. A tablet has the room, and keeps the strip.
    const approval = page => page.evaluate(() => { const s = window.__chatStream; s.emit('reset', { truncated: false }); s.emit('chat', { seq: 1, kind: 'user', text: 'fix the build' }); s.emit('chat', { seq: 2, kind: 'permission', permissionId: 'p1', toolCallId: 'c1', title: 'Run the tests', toolKind: 'execute', command: 'npm test', text: 'The tests write outside the sandbox.' }) })
    for (const [width, height, strip] of [[390, 844, false], [820, 1180, true]]) {
      const { page, context } = await open({ width, height, path: '/sessions?agent=a1&session=c1' })
      await expect(page.locator('textarea.composer-input')).toBeVisible()
      await expect(page.locator('.conversation-status')).toBeVisible()
      await approval(page)
      await expect(page.getByRole('button', { name: 'Allow once' })).toBeVisible()
      await expect(page.locator('.chat-status')).toContainText('Waiting for your approval.')
      if (strip) await expect(page.locator('.conversation-status')).toBeVisible()
      else await expect(page.locator('.conversation-status')).toBeHidden()
      // Answered, the strip is back with the turn's progress.
      await page.evaluate(() => { const s = window.__chatStream; s.emit('chat', { seq: 3, kind: 'permission_resolved', permissionId: 'p1', allow: true }); s.emit('chat', { seq: 4, kind: 'status', state: 'working' }) })
      await expect(page.locator('.conversation-status')).toBeVisible()
      await context.close()
    }
  }

  if (run('notch')) {
    // The page runs edge to edge. With a home indicator below (portrait) or a notch at either side (sideways), nothing
    // a person reads or presses sits under them.
    for (const [width, height, insets] of [[390, 844, { top: 0, left: 0, bottom: 34, right: 0 }], [844, 390, { top: 0, left: 47, bottom: 21, right: 47 }]]) {
      const { page, sizes, context } = await open({ width, height })
      const cdp = await context.newCDPSession(page)
      await cdp.send('Emulation.setSafeAreaInsetsOverride', { insets })
      expect(await page.locator('meta[name="viewport"]').getAttribute('content')).toContain('viewport-fit=cover')
      await page.reload()
      await live(page)
      const held = await rows(page, sizes)
      const edges = await page.evaluate(() => { const box = selector => { const r = document.querySelector(selector).getBoundingClientRect(); return { left: r.left, right: r.right, bottom: r.bottom } }; return { keys: box('.mobile-keys'), fixed: box('.key-fixed'), head: box('.session-screen .head'), screen: box('.xterm-screen') } })
      for (const [name, box] of Object.entries(edges)) {
        expect(box.left, `${width}x${height} ${name} left`).toBeGreaterThanOrEqual(insets.left)
        expect(box.right, `${width}x${height} ${name} right`).toBeLessThanOrEqual(width - insets.right)
        expect(box.bottom, `${width}x${height} ${name} bottom`).toBeLessThanOrEqual(height - insets.bottom)
      }
      expect(await noSidewaysScroll(page)).toBe(true)
      // The room given up is the inset and no more: a row or two of the terminal.
      await cdp.send('Emulation.setSafeAreaInsetsOverride', { insets: { top: 0, left: 0, bottom: 0, right: 0 } })
      await page.reload(); await live(page)
      const full = await rows(page, sizes)
      expect(full - held).toBeGreaterThanOrEqual(1)
      expect(full - held).toBeLessThanOrEqual(4)
      // Home, sideways: the tab bar and the floating Ask Gaffer keep clear of the notch too.
      if (width > height) {
        await cdp.send('Emulation.setSafeAreaInsetsOverride', { insets })
        await page.goto(base + '/')
        const launcher = page.locator('.gaffer-launcher')
        await expect(launcher).toBeVisible()
        const box = await launcher.boundingBox()
        expect(box.x + box.width).toBeLessThanOrEqual(width - insets.right)
        const heading = await page.getByRole('heading', { name: 'Home', exact: true }).boundingBox()
        expect(heading.x).toBeGreaterThanOrEqual(insets.left)
        expect(await noSidewaysScroll(page)).toBe(true)
      }
      await context.close()
    }
  }

  if (run('attention grouping')) {
    // Home's Continue working and the Sessions list can be grouped by attention: what needs you, then problems, then
    // what is done, then what is still going. The four choices fit a 360 px phone.
    const { page, context, errors } = await open({ width: 360, height: 640, path: '/' })
    await page.getByRole('button', { name: 'Continue working' }).first().click()
    const choice = page.getByRole('radiogroup', { name: 'Group by' })
    await expect(choice.getByRole('radio')).toHaveCount(4)
    await choice.getByText('Priority', { exact: true }).click()
    await expect(choice.getByRole('radio', { name: 'Priority' })).toBeChecked()
    const groups = page.getByRole('region', { name: 'Continue working' }).locator('.group h3, .group-head h3, h3')
    await expect(groups.first()).toContainText('Needs you')
    const labels = (await groups.allInnerTexts()).map(text => text.split('\n')[0].trim())
    expect(labels).toEqual(['Needs you', 'Finished', 'Working'])
    expect(await noSidewaysScroll(page)).toBe(true)
    const box = await choice.boundingBox()
    expect(box.x + box.width).toBeLessThanOrEqual(360)
    expect(errors).toEqual([])
    await context.close()
  }
  if (run('overlays and insets')) {
    // Every overlay a session screen opens keeps what it shows clear of a notch and the home indicator, upright and
    // sideways: its inner edge (inside any border that takes the inset) is within the safe area.
    for (const [width, height, insets] of [[390, 844, { top: 47, left: 0, bottom: 34, right: 0 }], [844, 390, { top: 0, left: 47, bottom: 21, right: 47 }]]) {
      const { page, context } = await open({ width, height, path: '/sessions' })
      const cdp = await context.newCDPSession(page)
      await cdp.send('Emulation.setSafeAreaInsetsOverride', { insets })
      const inside = async (selector, name) => {
        const edge = await page.evaluate(selector => {
          const element = [...document.querySelectorAll(selector)].filter(e => e.getClientRects().length).at(-1)
          if (!element) return null
          const r = element.getBoundingClientRect(), c = getComputedStyle(element)
          return { left: r.left + parseFloat(c.borderLeftWidth), right: r.right - parseFloat(c.borderRightWidth), top: r.top + parseFloat(c.borderTopWidth), bottom: r.bottom - parseFloat(c.borderBottomWidth) }
        }, selector)
        const label = `${width}x${height} ${name}`
        expect(edge, `${label} is open`).not.toBeNull()
        expect(edge.left, `${label} left`).toBeGreaterThanOrEqual(insets.left - 0.5)
        expect(edge.right, `${label} right`).toBeLessThanOrEqual(width - insets.right + 0.5)
        expect(edge.top, `${label} top`).toBeGreaterThanOrEqual(insets.top - 0.5)
        expect(edge.bottom, `${label} bottom`).toBeLessThanOrEqual(height - insets.bottom + 0.5)
      }
      const session = async () => { await page.goto(base + '/sessions?agent=a1&session=t1'); await live(page) }
      await page.goto(base + '/sessions')
      await page.getByRole('button', { name: '✦ Start a new agent session' }).click()
      await expect(page.getByRole('dialog', { name: 'Start an agent' })).toBeVisible()
      await inside('dialog[open]', 'Start an agent')
      await page.goto(base + '/sessions')
      await page.getByRole('button', { name: 'Sessions panel' }).click()
      await expect(page.locator('#terminal-sessions')).toBeVisible()
      // The drawer slides in over 160 ms: measure it once it has stopped.
      await expect.poll(() => page.locator('#terminal-sessions').evaluate(e => getComputedStyle(e).transform === 'none' || getComputedStyle(e).transform === 'matrix(1, 0, 0, 1, 0, 0)')).toBe(true)
      await inside('#terminal-sessions', 'the sessions drawer')
      await session()
      await page.locator('.terminal-toolbar').getByRole('button', { name: 'Compose', exact: true }).click()
      await expect(page.getByRole('dialog', { name: 'Compose' })).toBeVisible()
      await inside('dialog[open]', 'Compose')
      await session()
      await page.locator('.terminal-toolbar').getByRole('button', { name: 'Appearance' }).evaluate(button => button.click())
      await expect(page.locator('dialog[open]')).toBeVisible()
      await inside('dialog[open]', 'the Appearance drawer')
      await session()
      await screen(page).getByRole('button', { name: 'Session actions', exact: true }).click()
      await expect(page.getByRole('dialog', { name: 'Session actions' })).toBeVisible()
      await inside('dialog[open]', 'Session actions')
      await session()
      await screen(page).locator('.head .gaffer-entry').click()
      await expect(page.locator('.gaffer-chat')).toBeVisible()
      await inside('.gaffer-chat', 'the Gaffer panel')
      await context.close()
    }
  }

  if (run('chat dictation')) {
    // The chat composer has Dictate where the browser has speech recognition: a finished phrase joins the draft, the
    // line under it says where the audio goes, and nothing is sent by speaking.
    // In focus the composer is the message field alone, so Dictate is used with the full session screen.
    const { page, context, state } = await open({ width: 390, height: 844, path: '/sessions?agent=a1&session=c1', speech: true, sessionFocus: false })
    const input = page.locator('textarea.composer-input')
    await expect(input).toBeVisible()
    const dictate = page.locator('.composer').getByRole('button', { name: 'Dictate', exact: true })
    await expect(dictate).toBeVisible()
    const size = await dictate.boundingBox()
    expect(Math.round(size.width)).toBeGreaterThanOrEqual(44); expect(Math.round(size.height)).toBeGreaterThanOrEqual(44)
    await input.fill('Fix the build')
    await dictate.click()
    await expect(page.locator('.composer').getByRole('button', { name: 'Stop dictating' })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.locator('.composer').getByText('Listening. Your browser sends the audio to its own speech service, not to reet.')).toBeVisible()
    await page.evaluate(() => window.__speech.hear('and then', false))
    await expect(page.locator('.composer').getByText('and then… Your browser sends the audio')).toBeVisible()
    await page.evaluate(() => window.__speech.hear(' and then run the checkout tests.'))
    await expect(input).toHaveValue('Fix the build and then run the checkout tests.')
    expect(state.prompt).toBeUndefined()
    // Sending stops the listening.
    await page.getByRole('button', { name: /^Send/ }).click()
    await expect.poll(() => state.prompt?.text).toBe('Fix the build and then run the checkout tests.')
    await expect(dictate).toHaveAttribute('aria-pressed', 'false')
    expect(await page.evaluate(() => window.__listening)).toBe(false)
    // A refusal says what to do.
    await dictate.click()
    await page.evaluate(() => window.__speech.fail('not-allowed'))
    await expect(page.locator('.composer').getByText('The browser did not allow the microphone. Allow it for this site, then try again.')).toBeVisible()
    await context.close()
    // No speech recognition, no button.
    const plain = await open({ width: 390, height: 844, path: '/sessions?agent=a1&session=c1', speech: false })
    await expect(plain.page.locator('textarea.composer-input')).toBeVisible()
    await expect(plain.page.locator('.composer').getByRole('button', { name: 'Dictate' })).toHaveCount(0)
    await plain.context.close()
  }
  console.log('Session screen on phones and tablets: terminal focus mode and the rows it gives back, Ask Gaffer in the header on touch screens, the tablet keyboard layout, the docked panel and the drawer, the Sessions tab, the composer while typing, Start an agent guidance, an approval said twice and not three times, safe areas beside a notch and above a home indicator, grouping by attention, every overlay inside the safe area, and dictation in the chat composer.')
} finally {
  await browser.close()
}
