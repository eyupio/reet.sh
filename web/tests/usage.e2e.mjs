import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { active, openWorkspace } from './helpers/workspace.mjs'

// The Usage page: every agent's plan left and the tokens its sessions reported in a period, Gaffer's own requests and the
// sessions behind the figures. Every agent always has a block that says what is known, or why nothing is. Runs against
// the built app with every API answer mocked; the controller's figures have their own Go tests.
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173', shots = process.env.REET_UI_SCREENSHOTS || '/tmp/reet-usage'
await mkdir(shots, { recursive: true })
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY ? new RegExp(process.env.REET_E2E_ONLY, 'i') : null
const run = name => !only || only.test(name)

const now = Date.now(), minute = 60_000, hour = 60 * minute, day = 24 * hour
const caps = { protocolVersion: 1, os: 'linux', osUser: 'sam', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready', launch: true, acp: true, tools: ['claude', 'codex', 'opencode'] }
const machine = (agentId, host, reads = true, over = {}) => ({ agentId, host, ready: true, serverEnabled: true, capabilities: { ...caps, agentAccounts: reads, ...over } })
const accounts = list => ({ state: 'read', sentence: '', planLimits: true, checkedAt: now, accounts: list })
const claudePlan = { tool: 'claude', state: 'signed_in', method: 'subscription', account: 'sam@example.test', plan: 'max', limitsState: 'read', limitsAt: now - 3 * minute, limits: [{ kind: 'session', usedPercent: 19, resetsAt: now + 2 * hour + 10 * minute }, { kind: 'week', usedPercent: 90, resetsAt: now + 3 * day }] }
const codexKey = { tool: 'codex', state: 'signed_in', method: 'api_key', limitsState: 'unsupported' }
// Any agent a machine reads can carry windows: the blocks draw whatever arrives, scoped and monthly windows included.
const opencodePlan = { tool: 'opencode', state: 'signed_in', method: 'subscription', account: 'sam-go', plan: 'go', limitsState: 'read', limitsAt: now - minute, limits: [{ kind: 'session', usedPercent: 10, resetsAt: now + 3 * hour }, { kind: 'week', usedPercent: 50 }, { kind: 'month', usedPercent: 25, resetsAt: now + 12 * day }] }
const copilotPlan = { tool: 'copilot', state: 'signed_in', method: 'subscription', account: 'sam-gh', plan: 'pro', limitsState: 'read', limitsAt: now - minute, limits: [{ kind: 'month', label: 'Premium', usedPercent: 30 }, { kind: 'month', label: 'Chat', usedPercent: 0 }, { kind: 'month', label: 'Completions', usedPercent: 95 }] }
const agyPlan = { tool: 'antigravity', state: 'signed_in', method: 'subscription', account: 'sam@example.test', plan: 'pro', limitsState: 'rate_limited', limitsAt: now - 12 * minute, limits: [{ kind: 'week', label: 'Other models', usedPercent: 40 }] }
const AGENTS = [['claude', 'Claude Code', 'chat'], ['codex', 'Codex', 'chat'], ['opencode', 'OpenCode', 'chat'], ['cline', 'Cline', 'chat'], ['antigravity', 'Google Antigravity (AGY)', 'chat'], ['copilot', 'GitHub Copilot', 'terminal_only']]
const zero = () => AGENTS.map(([tool, name, reports]) => ({ tool, name, reports, input: 0, output: 0, cached: 0, sessions: 0, factoryStages: 0 }))
const session = (id, over = {}) => ({ id, title: `Session ${id}`, tool: 'claude', model: 'made-up-large', agentId: 'a1', host: 'studio', projectId: null, structured: true, state: 'ended', archived: false, factory: false, input: 1000, output: 200, cached: 0, lastAt: now - 10 * minute, contextUsed: 0, contextSize: 0, ...over })

// The figures for a range: today is smaller than seven days, so a period change shows in the totals.
function figures(from, to, { since = now - 20 * day, more = false } = {}) {
  const long = to - from > 2 * day
  const agents = zero()
  agents[0] = { ...agents[0], input: long ? 1_100_000 : 90_000, output: long ? 100_000 : 10_000, cached: long ? 900_000 : 0, sessions: long ? 14 : 2, factoryStages: long ? 3 : 1 }
  agents[2] = { ...agents[2], input: long ? 300_000 : 0, output: long ? 100_000 : 0, sessions: long ? 7 : 0 }
  return {
    from, to, since, more,
    agents,
    gaffer: [{ provider: 'Made-up provider', model: 'made-up-chat', input: 30_000, output: 4_000, replies: 12 }],
    sessions: [
      session('s1', { title: 'Export the billing API', factory: true, lastAt: now - 5 * minute, input: 80_000, output: 9_000, contextUsed: 90_000, contextSize: 200_000 }),
      session('s2', { title: 'Tidy the old importer', tool: 'opencode', model: 'made-up-small', host: 'laptop', agentId: 'a2', archived: true, state: '', lastAt: now - hour }),
      session('s3', { title: 'Read the logs', lastAt: now - 2 * hour }),
    ],
  }
}

async function open({ width = 1440, height = 900, touch = false, theme = 'light', machines = [machine('a1', 'studio'), machine('a2', 'laptop')], plans = { a1: accounts([claudePlan, codexKey, opencodePlan, copilotPlan, agyPlan]), a2: accounts([]) }, usage = (from, to) => figures(from, to), scale } = {}) {
  const asked = [], reads = []
  const opened = await openWorkspace(browser, { width, height, touch, theme, scale, terminalStatus: active, terminalMachines: machines, sessionFocus: false, routes: {
    '/api/v2/usage': request => { const q = new URL(request.url()).searchParams, from = Number(q.get('from')), to = Number(q.get('to')); asked.push({ from, to }); return usage(from, to, asked.length) },
    ...Object.fromEntries(Object.entries(plans).map(([id, answer]) => [`/api/v2/machines/${id}/accounts`, () => { reads.push(id); return answer }])),
  } })
  await opened.page.addInitScript(() => { window.EventSource = class { addEventListener() {} close() {} } })
  return { ...opened, asked, reads }
}
const block = (page, tool) => page.locator(`[data-usage-agent=${tool}]`)
const noSideways = page => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)

try {
  if (run('more item')) {
    for (const [width, touch] of [[1440, false], [390, true]]) {
      const { page, context, errors } = await open({ width, height: 860, touch })
      await page.goto(`${base}/`)
      await page.getByRole('button', { name: /^More/ }).first().click()
      const more = page.getByRole('navigation', { name: 'More pages' })
      const item = more.getByRole('link', { name: 'Usage', exact: true })
      await expect(item).toHaveAttribute('href', '/usage')
      await item.click()
      await expect(page).toHaveURL(`${base}/usage`)
      await expect(page.getByRole('heading', { level: 1, name: 'Usage' })).toBeVisible()
      await expect(page).toHaveTitle('Usage · reet.sh')
      expect(await page.getByRole('heading', { level: 1 }).count()).toBe(1)
      expect(errors).toEqual([])
      await context.close()
    }
  }

  if (run('every agent')) {
    const { page, context, errors, state, reads } = await open()
    await page.goto(`${base}/usage`)
    // Blocks in the catalogue's order, each always present.
    await expect(page.locator('[data-usage-agent]')).toHaveCount(AGENTS.length)
    expect(await page.locator('[data-usage-agent] h3 span:not(.agent-mark)').allInnerTexts()).toEqual(AGENTS.map(([, name]) => name))
    // A plan: the account, the plan and each window, with when it was read and where.
    const claude = block(page, 'claude')
    await expect(claude.locator('.account')).toHaveText('sam@example.test')
    await expect(claude.locator('.pill')).toHaveText('Max plan')
    await expect(claude.locator('.windows li .words').nth(0)).toHaveText(/^Session 81% left · resets in 2 h \d+ min$/)
    await expect(claude.locator('.windows li .words').nth(1)).toHaveText(/^Week 10% left · resets in (2|3) days$/)
    await expect(claude.locator('.windows li').nth(1)).toHaveClass(/low/)
    await expect(claude.getByRole('meter', { name: 'Session left' })).toHaveAttribute('aria-valuenow', '81')
    await expect(claude.locator('.source').first()).toHaveText('Read on studio 3 min ago.')
    // An API key, an agent whose plan reet cannot read, and one that runs only in a terminal.
    await expect(block(page, 'codex').locator('[data-plan-why]')).toHaveText('Signed in with an API key on studio: there is no plan allowance to read.')
    // OpenCode, Copilot and Antigravity plans are drawn like any other: session, week and month; scoped monthly windows;
    // a scoped window kept from a read the provider slowed down, dimmed, with that line.
    await expect(block(page, 'opencode').locator('.windows .words')).toHaveText([/^Session 90% left · resets in 3 h$/, 'Week 50% left', /^Month 75% left · resets in 1[12] days$/])
    await expect(block(page, 'copilot').locator('.windows .words')).toHaveText(['Premium this month 70% left', 'Chat this month 100% left', 'Completions this month 5% left'])
    await expect(block(page, 'copilot').locator('.windows li').nth(2)).toHaveClass(/low/)
    await expect(block(page, 'antigravity').locator('.windows .words')).toHaveText(['Other models this week 60% left'])
    await expect(block(page, 'antigravity').locator('.plan')).toHaveClass(/stale/)
    await expect(block(page, 'antigravity').locator('[data-plan-note]')).toHaveText("Google Antigravity (AGY)'s provider asked reet to slow down. Showing the figures read 12 min ago.")
    // Cline: no machine answers for it, so reet cannot read its plan.
    await expect(block(page, 'cline').locator('[data-plan-why]')).toHaveText("reet can't read this agent's plan.")
    await expect(block(page, 'copilot').locator('[data-tokens]')).toHaveText('GitHub Copilot runs only in a terminal here, which reports no token counts.')
    // Tokens for the period, and an agent that reported nothing in it.
    await expect(claude.locator('[data-tokens] [data-figure]')).toHaveText('100k tokens')
    await expect(claude.locator('[data-tokens] .line')).toHaveText('2 sessions · 1 Factory stage')
    await expect(claude.locator('[data-tokens] .detail')).toHaveText('Input 90k · Output 10k')
    await expect(block(page, 'cline').locator('[data-tokens]')).toHaveText('No tokens reported in this period.')
    // Gaffer, per provider and model.
    await expect(page.locator('[data-usage-gaffer]')).toContainText('Made-up provider · made-up-chat')
    await expect(page.locator('[data-usage-gaffer]')).toContainText('34k tokens')
    await expect(page.locator('[data-usage-gaffer]')).toContainText('12 replies · Input 30k · Output 4,000')
    expect(reads.sort()).toEqual(['a1', 'a2'])
    expect(state.unexpected).toEqual([])
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('sentences')) {
    // No sign-in, and a read that failed: each said in one sentence, from what the machines answered.
    const { page, context } = await open({ machines: [machine('a1', 'studio')], plans: { a1: accounts([{ ...claudePlan, limitsState: 'failed', limits: undefined, limitsAt: undefined }, { tool: 'codex', state: 'none', limitsState: 'not_asked' }, { ...opencodePlan, limitsState: 'renew', limits: undefined, limitsAt: undefined }]) } })
    await page.goto(`${base}/usage`)
    await expect(block(page, 'claude').locator('[data-plan-why]')).toHaveText("Claude Code's provider could not be reached from studio just now.")
    await expect(block(page, 'codex').locator('[data-plan-why]')).toHaveText('No sign-in found on studio.')
    // A sign-in that has run out, with no figures kept: say that the agent renews it when it next runs.
    await expect(block(page, 'opencode').locator('[data-plan-why]')).toHaveText("OpenCode's sign-in on studio has run out; it renews itself the next time OpenCode runs there.")
    await context.close()
    // A machine with terminal access off, and an older reet.
    const off = await open({ machines: [machine('a1', 'studio', false, { terminalEnabled: false })], plans: {} })
    await off.page.goto(`${base}/usage`)
    await expect(block(off.page, 'claude').locator('[data-plan-why]')).toHaveText("Terminal access is off on studio, so reet doesn't read its agents' sign-ins.")
    expect(off.reads).toEqual([])
    await off.context.close()
    const none = await open({ machines: [], plans: {} })
    await none.page.goto(`${base}/usage`)
    await expect(block(none.page, 'codex').locator('[data-plan-why]')).toHaveText('No connected machine reports sign-ins.')
    await none.context.close()
  }

  if (run('period')) {
    const { page, context, asked } = await open()
    await page.goto(`${base}/usage`)
    const totals = page.locator('[data-usage-totals]')
    await expect(totals).toHaveText('100k tokens across 2 sessions')
    await expect(totals).toHaveAttribute('aria-live', 'polite')
    const group = page.getByRole('group', { name: 'Period' })
    await expect(group.getByRole('button', { name: 'Today' })).toHaveAttribute('aria-pressed', 'true')
    expect(asked).toHaveLength(1)
    const today = asked[0]
    // Today starts at local midnight in the account's zone (the fixture's Europe/London) and is a day long or so.
    expect(today.to - today.from).toBeGreaterThanOrEqual(23 * hour); expect(today.to - today.from).toBeLessThanOrEqual(25 * hour)
    expect(today.from).toBeLessThanOrEqual(now); expect(today.to).toBeGreaterThan(now)
    await group.getByRole('button', { name: '7 days' }).click()
    await expect(group.getByRole('button', { name: '7 days' })).toHaveAttribute('aria-pressed', 'true')
    await expect(group.getByRole('button', { name: 'Today' })).toHaveAttribute('aria-pressed', 'false')
    await expect(totals).toHaveText('1.6M tokens across 21 sessions')
    expect(asked).toHaveLength(2)
    expect(asked[1].to).toBe(today.to)
    expect(asked[1].to - asked[1].from).toBeGreaterThanOrEqual(7 * day - hour); expect(asked[1].to - asked[1].from).toBeLessThanOrEqual(7 * day + hour)
    await expect(block(page, 'claude').locator('[data-tokens] [data-figure]')).toHaveText('1.2M tokens')
    await expect(block(page, 'claude').locator('[data-tokens] .line')).toHaveText('14 sessions · 3 Factory stages')
    // OpenCode, with no plan on this account's other fixture, still has its own figure for the period.
    await expect(block(page, 'opencode').locator('[data-tokens] [data-figure]')).toHaveText('400k tokens')
    await expect(block(page, 'claude').locator('[data-tokens] .detail')).toHaveText('Input 1.1M · Output 100k · Cached 900k')
    // Remembered across a reload, per account.
    await page.reload()
    await expect(page.getByRole('group', { name: 'Period' }).getByRole('button', { name: '7 days' })).toHaveAttribute('aria-pressed', 'true')
    await expect(totals).toHaveText('1.6M tokens across 21 sessions')
    expect(await page.evaluate(() => localStorage.getItem('reet.usage.period.1'))).toBe('week')
    // A range that starts before the first count says since when.
    await group.getByRole('button', { name: '30 days' }).click()
    await expect(totals).toHaveText(/^1\.6M tokens across 21 sessions · counted since \d{1,2} [A-Z][a-z]{2}$/)
    await context.close()
  }

  if (run('sessions')) {
    const { page, context } = await open({ usage: (from, to) => figures(from, to, { more: true }) })
    await page.goto(`${base}/usage`)
    const section = page.locator('[data-usage-sessions]')
    await expect(section.getByRole('heading', { level: 2, name: 'Sessions in this period' })).toBeVisible()
    const table = section.getByRole('table')
    await expect(table.getByRole('columnheader')).toHaveText(['Session', 'Agent', 'Model', 'Machine', 'Tokens', 'Context left', 'Last activity'])
    const first = table.locator('tr[data-session=s1]')
    await expect(first.getByRole('link', { name: 'Export the billing API' })).toHaveAttribute('href', '/sessions?session=s1')
    await expect(first).toContainText('Factory')
    await expect(first.locator('td').nth(3)).toHaveText('89k')
    await expect(first.locator('td').nth(4)).toHaveText('55% left')
    // A session removed from the history has no link, and says so.
    const gone = table.locator('tr[data-session=s2]')
    await expect(gone.getByRole('link')).toHaveCount(0)
    await expect(gone).toContainText('Tidy the old importer')
    await expect(gone).toContainText('removed from history')
    // Numbers are right-aligned in tabular figures.
    expect(await first.locator('td.num').first().evaluate(el => [getComputedStyle(el).textAlign, getComputedStyle(el).fontVariantNumeric])).toEqual(['right', 'tabular-nums'])
    await expect(section.locator('.more')).toHaveText('Showing the latest 3 sessions; more reported tokens in this period.')
    await context.close()
    // On a phone, the same sessions are cards.
    const phone = await open({ width: 390, height: 844, touch: true })
    await phone.page.goto(`${base}/usage`)
    const cards = phone.page.locator('[data-usage-sessions] li[data-session]')
    await expect(cards).toHaveCount(3)
    await expect(phone.page.locator('[data-usage-sessions] table')).toHaveCount(0)
    await expect(cards.nth(0).getByRole('link', { name: 'Export the billing API' })).toBeVisible()
    await expect(cards.nth(0)).toContainText('Factory')
    await expect(cards.nth(1).getByRole('link')).toHaveCount(0)
    await expect(cards.nth(1)).toContainText('Removed from history')
    const link = await cards.nth(0).getByRole('link').boundingBox()
    expect(link.height).toBeGreaterThanOrEqual(44)
    expect(await noSideways(phone.page)).toBe(true)
    await phone.context.close()
  }

  if (run('empty account')) {
    const { page, context } = await open({ machines: [], plans: {}, usage: (from, to) => ({ from, to, since: 0, agents: zero(), gaffer: [], sessions: [], more: false }) })
    await page.goto(`${base}/usage`)
    await expect(page.locator('[data-usage-totals]')).toHaveText('Nothing has been counted yet. reet counts what your agents report from now on.')
    await expect(page.locator('[data-usage-agent]')).toHaveCount(AGENTS.length)
    for (const [tool] of AGENTS.filter(([, , reports]) => reports === 'chat')) await expect(block(page, tool).locator('[data-tokens]')).toHaveText('No tokens reported in this period.')
    await expect(page.locator('[data-usage-gaffer]')).toContainText('Gaffer made no model requests in this period.')
    await expect(page.locator('[data-usage-sessions]')).toContainText('No session reported tokens in this period.')
    await expect(page.locator('.closing')).toContainText('reet attaches no price')
    expect(await page.locator('.closing').evaluate(el => parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(14)
    await context.close()
  }

  if (run('failed read')) {
    let down = false
    const { page, context, asked } = await open({ usage: (from, to) => down ? { status: 503, json: { error: 'down' } } : figures(from, to) })
    await page.clock.install({ time: now })
    await page.goto(`${base}/usage`)
    const totals = page.locator('[data-usage-totals]')
    await expect(totals).toHaveText('100k tokens across 2 sessions')
    // The minute's quiet refresh fails: the figures stay and the page says so, with a way to try again.
    down = true
    await page.clock.runFor(61_000)
    await expect(page.getByRole('alert')).toContainText('Usage could not be read.')
    expect(asked.length).toBe(2)
    await expect(totals).toHaveText('100k tokens across 2 sessions')
    await expect(block(page, 'claude').locator('[data-tokens] .line')).toHaveText('2 sessions · 1 Factory stage')
    down = false
    await page.getByRole('button', { name: 'Try again' }).click()
    await expect(page.getByRole('alert')).toHaveCount(0)
    expect(asked.length).toBe(3)
    await context.close()
    // A first read that fails has nothing to keep: the sentence and the button.
    const first = await open({ usage: () => ({ status: 500, json: { error: 'down' } }) })
    await first.page.goto(`${base}/usage`)
    await expect(first.page.getByRole('alert')).toContainText('Usage could not be read.')
    await expect(first.page.getByRole('button', { name: 'Try again' })).toBeVisible()
    await first.context.close()
    // A refused session signs out.
    const signedOut = await open({ usage: () => ({ status: 401, json: { error: 'sign in' } }) })
    await signedOut.page.goto(`${base}/usage`)
    await expect(signedOut.page.getByRole('heading', { name: /sign in/i }).first()).toBeVisible()
    await signedOut.context.close()
  }

  if (run('chips link')) {
    const { page, context } = await open()
    await page.goto(`${base}/`)
    const strip = page.locator('[data-plan-left] button.strip').first()
    await expect(strip).toBeVisible()
    await strip.click()
    const panel = page.getByRole('dialog', { name: 'Plan limits', exact: true })
    const link = panel.getByRole('link', { name: 'See all usage', exact: true })
    await expect(link).toHaveAttribute('href', '/usage')
    await link.click()
    await expect(page).toHaveURL(`${base}/usage`)
    await expect(page.getByRole('heading', { level: 1, name: 'Usage' })).toBeVisible()
    await context.close()
  }

  if (run('settings link')) {
    const { page, context } = await open()
    await page.goto(`${base}/account/profile#agents`)
    await expect(page.getByRole('heading', { name: 'Usage', exact: true })).toBeVisible()
    await expect(page.getByRole('checkbox', { name: "Show how much of each agent's plan is left" })).toHaveCount(0)
    await page.locator('[data-usage-note]').getByRole('link', { name: 'See all usage' }).click()
    await expect(page).toHaveURL(`${base}/usage`)
    await context.close()
  }

  if (run('sizes')) {
    for (const [width, touch] of [[320, true], [390, true], [834, true], [1440, false]]) {
      for (const theme of ['light', 'dark']) {
        const { page, context, errors } = await open({ width, height: 900, touch, theme, scale: 2 })
        await page.goto(`${base}/usage`)
        await expect(page.locator('[data-usage-agent]')).toHaveCount(AGENTS.length)
        await expect(page.locator('[data-usage-sessions] [data-session]').first()).toBeVisible()
        expect(await noSideways(page), `${width} ${theme}`).toBe(true)
        // Nothing reads smaller than 14 px (an agent's mark is a decorative tile, hidden from assistive technology).
        const small = await page.locator('main').evaluate(main => [...main.querySelectorAll('*')].filter(el => el.childNodes.length && [...el.childNodes].some(n => n.nodeType === 3 && n.textContent.trim()) && el.getClientRects().length && !el.closest('[aria-hidden=true]') && parseFloat(getComputedStyle(el).fontSize) < 14).map(el => `${el.tagName}.${el.className}:${el.textContent.trim().slice(0, 20)}`))
        expect(small, `${width} ${theme}`).toEqual([])
        if (touch) {
          for (const button of await page.getByRole('group', { name: 'Period' }).getByRole('button').all()) expect((await button.boundingBox()).height).toBeGreaterThanOrEqual(44)
        }
        // A focused period button shows its ring.
        await page.getByRole('group', { name: 'Period' }).getByRole('button', { name: '7 days' }).focus()
        await page.keyboard.press('Shift+Tab'); await page.keyboard.press('Tab')
        expect(await page.evaluate(() => getComputedStyle(document.activeElement).outlineStyle)).not.toBe('none')
        await page.screenshot({ path: `${shots}/usage-${width}-${theme}.png`, fullPage: true })
        expect(errors).toEqual([])
        await context.close()
      }
    }
  }
  console.log('Usage page passed.')
} finally { await browser.close() }
