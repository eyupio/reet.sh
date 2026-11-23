import { chromium } from '@playwright/test'
import { active, openWorkspace } from './helpers/workspace.mjs'

// A measurement, not a test: what the Factory page's refresh costs today, so a later change to how it reads can be
// compared with figures. It opens the Factory on the production bundle with the API answered from a synthetic Factory
// sized like stage 0's (6 outcomes × 5 pipelines × 8 stages, three handed over in each), and prints JSON:
//   - idle: the API requests and response bytes per minute, by path, while the page stays open and visible for a minute
//   - decision: over several trials, one working pipeline turns to ready_to_merge in the fixture, and the time until
//     its "Approve the merge" button is visible (median, 95th percentile, worst); the poll interval bounds it
//   - draw: the time from the first outcome read that carries the change being answered to the change on the screen
// Run it against `vite preview`, as the browser suites are: node web/tests/factory-refresh-baseline.mjs
// REET_FACTORY_IDLE_MS (60000) and REET_FACTORY_TRIALS (10) change its length.
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const IDLE = Number(process.env.REET_FACTORY_IDLE_MS || 60000), TRIALS = Number(process.env.REET_FACTORY_TRIALS || 10)
const A = 'connector-baseline00000001', now = Date.now()
const KINDS = ['plan', 'design', 'code', 'review', 'test', 'pr', 'babysit', 'merge']
// The same lengths as internal/api/gaffer_factory_baseline_test.go, in bytes of text.
const LEN = { objective: 600, title: 40, prompt: 1500, note: 100, tierWhy: 100, summary: 2000, goalSummary: 400 }

// Plain prose of n bytes from a small vocabulary, seeded, so it compresses about as real notes do.
let seed = 12345
const rand = () => { seed = (seed + 0x6d2b79f5) | 0; let t = Math.imul(seed ^ (seed >>> 15), 1 | seed); t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t; return ((t ^ (t >>> 14)) >>> 0) / 4294967296 }
const vocab = 'the export reads ledger page each line row totals billing csv endpoint test tests handler store migration review branch pull request commit check passes fails error case empty account owner date range header column format writes returns handles invalid limit retry timeout queue worker stage plan design code merge approve before after with without only every one two three it is was are and or but not so then also file files function change changes keeps adds removes says shows from into for of on in at by to a an'.split(' ')
const text = n => { let s = ''; while (s.length < n) s += (s ? ' ' : '') + vocab[Math.floor(rand() * vocab.length)]; return s.slice(0, n) }
const hex = n => Array.from({ length: n }, () => Math.floor(rand() * 16).toString(16)).join('')

function factory() {
  const outcomes = [], goals = [], sessions = []
  for (let o = 0; o < 6; o++) {
    const record = { id: `${'0'.repeat(26)}fac70${o}`, revision: 2, objective: text(LEN.objective), state: 'settled', note: text(LEN.note), agentId: A, projectId: 'project-billing', projectName: 'billing', dir: '/home/dev/work/billing', tool: 'opencode', model: 'anthropic/claude-sonnet-4-5', providerId: 'b'.repeat(32), maxReviews: 12, hours: 24, createdAt: now - 7200000 + o, updatedAt: now, sessions: [] }
    for (let p = 0; p < 5; p++) {
      const key = String.fromCharCode(97 + p), sessionId = hex(48), conversationId = hex(32), title = text(LEN.title)
      const piece = { key, title, prompt: text(LEN.prompt), requestId: hex(32), state: 'following', note: text(LEN.note), sessionId, conversationId, updatedAt: now, stage: 3, workDir: `/home/dev/work/billing-${key}`, sessionState: 'running', stages: [] }
      KINDS.forEach((kind, s) => {
        const stage = { kind, tool: 'opencode', model: 'anthropic/claude-sonnet-4-5', effort: 'high', tier: 'standard', tierWhy: text(LEN.tierWhy), state: 'pending', requestId: hex(32) }
        if (s < 3) Object.assign(stage, { state: 'done', summary: text(LEN.summary), closedBy: 'gaffer', sessionId: hex(48), conversationId: hex(32), startedAt: now - 3600000, endedAt: now - 1800000, reviews: 3, inputTokens: 41000, outputTokens: 2300, usageReported: true, agentInputTokens: 1200000, agentOutputTokens: 48000, agentCachedTokens: 900000, agentUsageReported: true })
        if (s === 3) Object.assign(stage, { state: 'running', sessionId, conversationId, startedAt: now - 600000, contextUsed: 84000, contextSize: 200000, agentState: 'working', agentActiveAt: now })
        piece.stages.push(stage)
      })
      record.sessions.push(piece)
      // Each working stage has its goal, which Gaffer is following.
      const goal = { id: hex(32), conversationId, sessionId, revision: 3, text: `Outcome: ${title}`, state: 'active', note: 'Gaffer is following this stage.', providerId: 'b'.repeat(32), providerRevision: 1, model: 'gaffer-model', contextRevision: 1, workerRevision: 1, maxReviews: 12, usedReviews: 2, expiresAt: now + 20 * 3600000, createdAt: now - 600000, updatedAt: now, nextAt: now + 60000, lastSeq: 42, lastObservedAt: now, lastReviewedAt: now, summary: text(LEN.goalSummary), inputTokens: 12000, outputTokens: 800, usageReported: true }
      goals.push({ goal, session: { id: sessionId, agentId: A, title: `${title} · code`, state: 'running' }, available: true, attention: false })
      sessions.push({ id: sessionId, agentId: A, title: `${title} · code`, state: 'running', createdAt: now, updatedAt: now, exitCode: null, tool: 'opencode', model: 'anthropic/claude-sonnet-4-5', structured: true, dir: piece.workDir })
    }
    outcomes.push(record)
  }
  return { outcomes, goals, sessions }
}

const bytes = value => Buffer.byteLength(JSON.stringify(value))
const rank = (list, p) => { const s = [...list].sort((a, b) => a - b); return s.length ? s[Math.min(s.length - 1, Math.floor(p * s.length))] : null }
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const sample = factory()
const out = { idleMs: IDLE, trials: TRIALS, fixture: { outcomesBytes: bytes({ outcomes: sample.outcomes }), goalsBytes: bytes({ goals: sample.goals, readAt: now }) }, runs: [] }
try {
  for (const [name, width, height] of [['desktop', 1360, 900], ['phone', 390, 844]]) {
    const f = factory()
    // `served` is when the first outcome read carrying the change was answered, for the time to draw.
    let changed = false, served = 0
    const { page, context, errors } = await openWorkspace(browser, {
      width, height, touch: width < 500, terminalStatus: active,
      terminalMachines: [{ agentId: A, host: 'Workshop', ready: true, serverEnabled: true, capabilities: { protocolVersion: 1, terminalControl: true, os: 'linux', osUser: 'dev', terminalEnabled: true, readiness: 'ready', acp: true, tools: ['opencode'] } }],
      routes: {
        '/api/v2/gaffer/outcomes': () => { if (changed && !served) served = Date.now(); return { outcomes: f.outcomes } },
        '/api/v2/gaffer/goals': () => ({ goals: f.goals, readAt: Date.now() }),
        '/api/v2/terminal/sessions': () => ({ sessions: f.sessions }),
        '/api/v2/gaffer/providers': { providers: [{ id: 'b'.repeat(32), revision: 1, name: 'Workshop Gaffer', kind: 'openai_compatible', baseUrl: 'https://example.test/v1', model: 'gaffer-model', enabled: true, isDefault: true, localOnly: false, allowStatus: false, allowSessions: true, allowCode: true, hasKey: true, createdAt: now, updatedAt: now }], presets: [], available: true, canUseLocal: true, maxProviders: 12 },
        '/api/v2/folder-projects': { projects: [{ id: 'project-billing', name: 'billing', agentId: A, dir: '/home/dev/work/billing', version: 1, createdAt: now, updatedAt: now }], savedWaiting: 0 },
        '/api/v2/settings/yolo': { allowed: false },
      },
    })
    await page.addInitScript(() => { window.EventSource = class { addEventListener() {} close() {} } })
    // Every API answer the page asked for, by path.
    let log = []
    page.on('response', async response => {
      const url = new URL(response.url())
      if (!url.pathname.startsWith('/api/')) return
      const size = (await response.body().catch(() => Buffer.alloc(0))).length
      log.push({ path: url.pathname, size, at: Date.now() })
    })
    await page.goto(`${base}/factory`)
    const first = f.outcomes[0].sessions[0]
    await page.getByText('0 waiting for you · 30 working', { exact: false }).first().waitFor({ timeout: 60000 })
    const visibility = await page.evaluate(() => document.visibilityState)
    // A minute with the page open and nothing changing.
    log = []
    const started = Date.now()
    await page.waitForTimeout(IDLE)
    const minutes = (Date.now() - started) / 60000, byPath = {}
    for (const entry of log) { const row = byPath[entry.path] ??= { requests: 0, bytes: 0 }; row.requests++; row.bytes += entry.size }
    for (const row of Object.values(byPath)) { row.requestsPerMinute = Math.round(row.requests / minutes * 10) / 10; row.bytesPerMinute = Math.round(row.bytes / minutes) }
    const idle = { minutes: Math.round(minutes * 100) / 100, visibility, totalRequestsPerMinute: Math.round(log.length / minutes * 10) / 10, totalBytesPerMinute: Math.round(log.reduce((sum, e) => sum + e.size, 0) / minutes), byPath }
    // Trials: the pipeline waits for the person; how soon does its decision show?
    const decision = page.getByRole('button', { name: /^Approve the merge/ })
    const toVisible = [], toDraw = [], missed = []
    for (let trial = 0; trial < TRIALS; trial++) {
      Object.assign(first, { state: 'following' }); changed = false; served = 0
      await decision.first().waitFor({ state: 'hidden', timeout: 30000 }).catch(() => {})
      // A random phase against the poll, so the trials do not all land at the same point in its cycle.
      await page.waitForTimeout(500 + Math.floor(rand() * 5000))
      Object.assign(first, { state: 'ready_to_merge' }); f.outcomes[0].revision++
      const at = Date.now(); changed = true
      try {
        const seen = await page.waitForFunction(() => [...document.querySelectorAll('button')].some(b => /^Approve the merge/.test(b.textContent.trim()) && b.checkVisibility?.() !== false && b.getClientRects().length > 0) && Date.now(), null, { polling: 'raf', timeout: 30000 })
        const shown = await seen.jsonValue()
        toVisible.push(shown - at)
        if (served) toDraw.push(shown - served)
      } catch { missed.push(trial) }
    }
    out.runs.push({
      name, width, height, idle,
      decision: { trials: toVisible.length, missed, medianMs: rank(toVisible, 0.5), p95Ms: rank(toVisible, 0.95), worstMs: rank(toVisible, 1), allMs: toVisible },
      draw: { medianMs: rank(toDraw, 0.5), worstMs: rank(toDraw, 1) },
      pageErrors: errors.length,
    })
    await context.close()
  }
} finally { await browser.close() }
console.log(JSON.stringify(out, null, 2))
