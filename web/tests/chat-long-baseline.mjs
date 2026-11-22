import { chromium } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

// A measurement, not a test: how a long chat behaves today, so a later change to how chats are sent or drawn has
// figures to be compared with. It opens one chat on the production bundle, replays a long history through the chat's
// event stream, then streams an agent's reply a piece at a time, and prints what it measured as JSON:
//   - replay: the bytes of the history's events and how long the page took to draw them
//   - update: how long one streamed piece takes to reach the screen (median, 95th percentile, worst), and its bytes
//   - scroll: how far the page is from the bottom once the history is drawn, the furthest it falls behind while it
//     follows a streaming reply (it scrolls smoothly, so it trails) and where it ends, and how far the line being read
//     moves while the person has scrolled back to read
// Run it against `vite preview`, as the browser suites are: node web/tests/chat-long-baseline.mjs
// REET_CHAT_EVENTS (history events, 2000) and REET_CHAT_UPDATES (streamed pieces, 200) change its size.
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const EVENTS = Number(process.env.REET_CHAT_EVENTS || 2000), UPDATES = Number(process.env.REET_CHAT_UPDATES || 200)
const A = 'connector-long000000000001', S = 'session-long000000000001'
const now = Date.now()
const session = { id: S, agentId: A, title: 'Long chat', state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'codex', model: 'gpt-5-codex', effort: 'low', yolo: false, projectId: null, dir: '/tmp', structured: true }
const machine = { agentId: A, host: 'long-machine', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, healthEnabled: true, readiness: 'ready', acp: true, tools: ['codex'] } }
const none = { reported: false, inputTokens: 0, outputTokens: 0, cachedTokens: 0, contextUsed: 0, contextSize: 0, updatedAt: 0 }

const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const out = { events: EVENTS, updates: UPDATES, runs: [] }
try {
  for (const [name, width, height] of [['desktop', 1360, 900], ['phone', 390, 844]]) {
    const { page, context, errors } = await openWorkspace(browser, {
      width, height, terminalStatus: active, terminalMachines: [machine],
      routes: { '/api/v2/terminal/sessions': { sessions: [session] }, [`/api/v2/terminal/sessions/${S}/chat/usage`]: none },
    })
    await page.addInitScript(() => {
      class Stream {
        constructor() { this.listeners = {}; window.__chatStream = this; setTimeout(() => this.onopen?.({}), 0) }
        addEventListener(type, fn) { (this.listeners[type] ??= []).push(fn) }
        close() {}
        emit(type, data) { for (const fn of this.listeners[type] ?? []) fn({ data: JSON.stringify(data) }) }
      }
      window.EventSource = Stream
    })
    await page.goto(`${base}/sessions?agent=${A}&session=${S}`)
    await page.waitForFunction(() => window.__chatStream)
    await page.locator('[role=log]').waitFor()
    const run = await page.evaluate(async ([EVENTS, UPDATES]) => {
      const stream = window.__chatStream, log = document.querySelector('[role=log]')
      const frame = () => new Promise(done => requestAnimationFrame(() => done()))
      const shown = async text => { for (let i = 0; i < 600 && !log.textContent.includes(text); i++) await frame(); await frame(); return log.textContent.includes(text) }
      // The log scrolls smoothly, so a position is only what it says once it has stopped moving.
      const settle = async () => { for (let still = 0, last = '', i = 0; still < 10 && i < 900; i++) { await frame(); const here = `${log.scrollTop}/${log.scrollHeight}`; still = here === last ? still + 1 : 0; last = here } }
      const rank = (list, p) => { const s = [...list].sort((a, b) => a - b); return Math.round(s[Math.min(s.length - 1, Math.floor(p * s.length))] * 10) / 10 }
      const bytes = event => new TextEncoder().encode(JSON.stringify(event)).length
      // A history as a working session leaves it: a request, a reply in three pieces with a table and code, a tool's result.
      const history = []
      let seq = 0
      while (history.length < EVENTS) {
        const turn = history.length
        history.push({ seq: ++seq, kind: 'user', text: `Request ${turn}: tidy the export and say what changed.` })
        for (const piece of [`Reply ${turn}: looking at the export. `, 'It reads the ledger in pages and writes one row for each line.\n\n| Change | Where |\n|---|---|\n| Page size | `export.go` |\n| Totals | `ledger.go` |\n\n', '```sh\ngo test ./internal/export/...\n```\n\nBoth pass; nothing else changed.'])
          history.push({ seq: ++seq, kind: 'agent', messageId: `m${turn}`, text: piece })
        history.push({ seq: ++seq, kind: 'tool', toolCallId: `c${turn}`, title: 'export.go', toolKind: 'edit', status: 'completed', text: 'Edit applied successfully.', path: '/tmp/export.go', diff: '@@ -1 +1 @@\n-pageSize = 50\n+pageSize = 200', additions: 1, deletions: 1, durationMs: 120 })
      }
      const replayBytes = history.reduce((sum, event) => sum + bytes(event), 0)
      let started = performance.now()
      stream.emit('reset', { truncated: false })
      for (const event of history) stream.emit('chat', event)
      const drawn = await shown(`Request ${history.at(-5)?.text.match(/\d+/)?.[0] ?? ''}`) && await shown('Both pass; nothing else changed.')
      const replayMs = Math.round(performance.now() - started)
      const fromBottom = () => Math.round(log.scrollHeight - log.scrollTop - log.clientHeight)
      await settle()
      const replayEndGap = fromBottom()
      // Following: the person is at the bottom while a reply streams in.
      const follow = [], gaps = []
      let updateBytes = 0
      stream.emit('chat', { seq: ++seq, kind: 'user', text: 'Now explain the whole export, slowly.' })
      for (let i = 0; i < UPDATES; i++) {
        const event = { seq: ++seq, kind: 'agent', messageId: 'live', text: `piece-${i}-follow and a few more words of the reply. ` }
        updateBytes += bytes(event)
        started = performance.now()
        stream.emit('chat', event)
        if (!await shown(`piece-${i}-follow`)) throw new Error(`piece ${i} never reached the screen`)
        follow.push(performance.now() - started)
        gaps.push(fromBottom())
      }
      await settle()
      const followEndGap = fromBottom()
      // Reading: the person has scrolled back, and what they are reading should stay where it is.
      log.scrollTo({ top: Math.round(log.scrollHeight / 2), behavior: 'instant' })
      await settle()
      const top = log.getBoundingClientRect().top
      const anchor = [...log.querySelectorAll('*')].find(el => el.children.length === 0 && el.textContent.trim() && el.getBoundingClientRect().top >= top + 40)
      const at = anchor.getBoundingClientRect().top, scrolled = log.scrollTop
      const read = []
      let drift = 0
      for (let i = 0; i < UPDATES; i++) {
        started = performance.now()
        stream.emit('chat', { seq: ++seq, kind: 'agent', messageId: 'live', text: `piece-${i}-read and a few more words of the reply. ` })
        await frame(); await frame()
        read.push(performance.now() - started)
        drift = Math.max(drift, Math.abs(anchor.getBoundingClientRect().top - at))
      }
      await settle()
      drift = Math.max(drift, Math.abs(anchor.getBoundingClientRect().top - at))
      return {
        replay: { events: history.length, bytes: replayBytes, ms: replayMs, drawn },
        update: { bytesEach: Math.round(updateBytes / UPDATES), medianMs: rank(follow, 0.5), p95Ms: rank(follow, 0.95), worstMs: rank(follow, 1) },
        scroll: { afterReplayGapPx: replayEndGap, followingWorstGapPx: Math.max(...gaps), followingEndGapPx: followEndGap, readingAnchorDriftPx: Math.round(drift * 10) / 10, readingScrollTopMovedPx: Math.round(Math.abs(log.scrollTop - scrolled)), readingFrameP95Ms: rank(read, 0.95) },
        page: { elements: document.querySelectorAll('*').length, logElements: log.querySelectorAll('*').length, heapMb: performance.memory ? Math.round(performance.memory.usedJSHeapSize / 1048576) : null },
      }
    }, [EVENTS, UPDATES])
    out.runs.push({ name, width, height, ...run, pageErrors: errors.length })
    await context.close()
  }
} finally { await browser.close() }
console.log(JSON.stringify(out, null, 2))
