import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const shots = process.env.REET_UI_SCREENSHOTS || `${process.env.RUNNER_TEMP || '/tmp'}/terminal-browser/workbench`
await mkdir(shots, { recursive: true })
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now()
const provider = { id: 'a'.repeat(32), revision: 1, name: 'Test Gaffer', kind: 'openai_compatible', baseUrl: 'https://example.test/v1', model: 'test', enabled: true, isDefault: true, allowSessions: true, allowCode: true, hasKey: true, createdAt: now, updatedAt: now }
const capabilities = { protocolVersion: 1, os: 'linux', osUser: 'dev', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready', launch: true, agentsV2: true, acp: true, browse: true, tools: ['claude'], acpAgents: [{ tool: 'claude', adapter: 'claude-agent-acp', state: 'available' }] }
const machine = { agentId: 'm1', host: 'Workshop', ready: true, serverEnabled: true, capabilities }
const project = { id: 'p1', host: 'm1', machine: 'Workshop', dir: '/work/reet', name: 'Reet', isRepo: true, baseBranch: 'main', worktreeDefault: 'inherit', defaultTool: 'claude', defaultModel: '', defaultEffort: '', hasStarterPrompt: false, colour: '', position: 0, createdAt: now, updatedAt: now, version: 1 }

async function open({ width = 1440, theme = 'dark', routes = {}, status } = {}) {
  const w = await openWorkspace(browser, { width, height: 1100, theme, touch: width < 1000, status, terminalMachines: [machine], terminalStatus: { configured: true, available: true, reason: '' }, routes: {
    '/api/v2/machines/health': { serverNow: now, machines: [{ ...machine, state: 'connected' }] },
    '/api/v2/folder-projects': { projects: [project], savedWaiting: 0 },
    '/api/v2/gaffer/providers': { providers: [provider], presets: [], available: true, canUseLocal: true, maxProviders: 12 },
    '/api/v2/settings/yolo': { allowed: false },
    ...routes,
  } })
  const writes = []
  w.page.on('request', r => { if (new URL(r.url()).pathname.startsWith('/api/') && r.method() !== 'GET') writes.push({ path: new URL(r.url()).pathname, body: r.postDataJSON() }) })
  return { ...w, writes }
}

const workbench = page => page.getByRole('region', { name: 'How would you like to work?', exact: true })
const noSideways = page => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)

try {
  for (const theme of ['light', 'dark']) for (const width of [320, 390, 960, 1440]) {
    const w = await open({ width, theme })
    try {
      const { page } = w
      await page.goto(base)
      const start = workbench(page)
      await expect(start).toBeVisible()
      await expect(start.getByRole('article').getByRole('heading')).toHaveText(['Start Factory work', 'Start an agent session', 'Open a terminal'])
      const buttons = start.locator('.choice .btn')
      await expect(buttons).toHaveCount(3)
      const styles = await buttons.evaluateAll(elements => elements.map(el => { const css = getComputedStyle(el); return { background: css.backgroundColor, color: css.color, radius: css.borderRadius, padding: css.padding } }))
      expect(styles[1]).toEqual(styles[0]); expect(styles[2]).toEqual(styles[0])
      // A short label keeps its natural width; making every action fill its card is a regression.
      expect(await start.getByRole('button', { name: 'Open terminal', exact: true }).evaluate(el => { const parent = el.parentElement, css = getComputedStyle(parent); return el.getBoundingClientRect().width < parent.clientWidth - parseFloat(css.paddingLeft) - parseFloat(css.paddingRight) - 2 })).toBe(true)
      expect(await start.getByRole('heading', { name: 'How would you like to work?', exact: true }).evaluate(el => getComputedStyle(el).color === getComputedStyle(document.body).color)).toBe(true)
      expect(await noSideways(page)).toBe(true)
      expect(await page.evaluate(() => localStorage.getItem('reet.home.view.1'))).toBeNull()
      expect(w.writes).toEqual([])
      await page.screenshot({ path: `${shots}/home-${width}-${theme}.png`, fullPage: true })

      // Factory starts directly from Home. Merely opening or dismissing the composer makes no model or session write.
      const factory = start.getByRole('button', { name: 'Give Gaffer an outcome', exact: true })
      await factory.click()
      const outcome = page.getByRole('dialog', { name: 'Give Gaffer an outcome', exact: true })
      await expect(outcome.getByRole('textbox', { name: 'Outcome', exact: true })).toBeVisible()
      expect(new URL(page.url()).pathname).toBe('/')
      expect(w.writes).toEqual([])
      await page.keyboard.press('Escape')
      await expect(outcome).toHaveCount(0)
      await expect(factory).toBeFocused()

      await start.getByRole('button', { name: 'Start a session', exact: true }).click()
      await expect(page.getByRole('dialog', { name: 'Start an agent', exact: true })).toBeVisible()
      await page.keyboard.press('Escape')
      await expect(page.getByRole('dialog', { name: 'Start an agent', exact: true })).toHaveCount(0)

      // Planning opens the real composer, and closing it returns focus to the now-visible planning tab.
      await start.getByRole('button', { name: 'Plan a session', exact: true }).click()
      const plan = page.getByRole('dialog', { name: 'A little planning. Proper progress.', exact: true })
      await expect(plan).toBeVisible()
      await plan.getByRole('button', { name: 'Close session composer', exact: true }).click()
      const planning = page.getByRole('button', { name: 'Planning board', exact: true })
      await expect(planning).toHaveAttribute('aria-pressed', 'true')
      await expect(planning).toBeFocused()
      await page.reload()
      await expect(planning).toHaveAttribute('aria-pressed', 'true')
      await expect(workbench(page)).toHaveCount(0)
      expect(w.writes).toEqual([])

      await page.goto(`${base}/factory`)
      const welcome = page.getByRole('region', { name: 'Put Gaffer on the job.', exact: true })
      await expect(welcome).toBeVisible()
      await expect(page.getByText(/waiting for you ·/)).toHaveCount(0)
      await expect(welcome.getByRole('listitem')).toHaveCount(4)
      await expect(page.getByRole('navigation', { name: 'Filter the floor', exact: true })).toHaveCount(0)
      await expect(page.getByRole('heading', { name: 'Put Gaffer on the job.', exact: true })).toHaveCSS('color', await page.locator('body').evaluate(el => getComputedStyle(el).color))
      expect(await noSideways(page)).toBe(true)
      await page.screenshot({ path: `${shots}/factory-${width}-${theme}.png`, fullPage: true })
      await welcome.getByRole('button', { name: 'Fix failing CI', exact: true }).click()
      await expect(welcome.getByRole('textbox', { name: 'What would you like done?', exact: true })).toHaveValue('Fix the failing CI workflow and explain what caused it.')
      expect(w.writes).toEqual([])
      await welcome.getByRole('button', { name: 'Give Gaffer an outcome', exact: true }).click()
      await expect(outcome.getByRole('textbox', { name: 'Outcome', exact: true })).toHaveValue('Fix the failing CI workflow and explain what caused it.')
      await page.keyboard.press('Escape')
      await expect(outcome).toHaveCount(0)
      await expect(welcome.getByRole('button', { name: 'Give Gaffer an outcome', exact: true })).toBeFocused()
      expect(w.writes).toEqual([])
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }

  // Existing backlog work keeps the board as the initial view rather than being hidden by the empty-session launcher.
  {
    const w = await open({ routes: { '/api/v2/session-backlog': { drafts: [{ id: 'd1', title: 'Saved idea', prompt: '', idea: true, agentId: 'm1', projectId: 'p1', dir: '/work/reet', tool: 'claude', model: '', effort: '', imageCount: 0, starting: false, version: 1, createdAt: now, updatedAt: now }] } } })
    try {
      await w.page.goto(base)
      await expect(w.page.getByRole('button', { name: 'Planning board', exact: true })).toHaveAttribute('aria-pressed', 'true')
      await expect(w.page.getByText('Saved idea', { exact: true })).toBeVisible()
      await expect(workbench(w.page)).toHaveCount(0)
      expect(w.writes).toEqual([])
    } finally { await w.context.close() }
  }
  // Failed and loading reads are never advertised as an empty workspace, and a viewer receives no launch controls.
  for (const config of [
    { routes: { '/api/v2/terminal/sessions': { status: 503, json: { error: 'Unavailable' } } } },
    { status: { user: { id: 2, name: 'Reader', role: 'viewer' } } },
  ]) {
    const w = await open(config)
    try {
      await w.page.goto(base)
      if (!config.status) { await w.page.getByRole('button', { name: 'Continue working', exact: true }).click(); await expect(w.page.getByText("Couldn't read your sessions just now.", { exact: true })).toBeVisible() }
      else await expect(w.page.getByRole('heading', { name: 'Home', exact: true })).toBeVisible()
      await expect(workbench(w.page)).toHaveCount(0)
      expect(w.writes).toEqual([])
    } finally { await w.context.close() }
  }
  // The plain-terminal choice still uses the existing terminal create route, not a new agent or Factory request.
  {
    const w = await open({ routes: { '/api/v2/terminal/sessions': request => request.method() === 'POST' ? { id: 'created-shell', agentId: 'm1', title: 'Terminal', state: 'running', createdAt: now, updatedAt: now } : { sessions: [] } } })
    try {
      await w.page.route(url => new URL(url).pathname === '/sessions', route => route.fulfill({ contentType: 'text/html', body: '<title>Terminal opened</title>' }))
      await w.page.goto(base)
      await workbench(w.page).getByRole('button', { name: 'Open terminal', exact: true }).click()
      await expect(w.page).toHaveURL(/session=created-shell/)
      expect(w.writes).toHaveLength(1)
      expect(w.writes[0].path).toBe('/api/v2/terminal/sessions')
      expect(w.writes[0].body.agentId).toBe('m1')
      expect(w.writes[0].body.tool).toBeUndefined()
    } finally { await w.context.close() }
  }
  console.log('Workbench: ordered, equally styled natural-width launch buttons; direct Factory and planning flows; both themes at phone, tablet and desktop widths; read failures and read-only accounts preserved.')
} finally { await browser.close() }
