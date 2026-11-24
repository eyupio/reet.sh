// Machines: a large fleet sorts by attention, and a compact status filter appears
// only when the fleet is big enough to need it.
import { chromium, expect } from '@playwright/test'
import { active, openWorkspace, terminalMachine } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const scope = (host, over = {}) => ({ id: `${host}-agents`, host, agent: 'agents', name: host, skills: [], skillCount: 0, complete: true, scannedAt: now, lastSeen: now, os: 'linux', arch: 'amd64', version: 'test', ...over })
const hosts = ['work-1', 'att-1', 'att-2', 'setup-1', 'sync-1', 'sync-2', 'sync-3']
const environments = [
  scope('work-1'),
  scope('att-1'),
  scope('att-2', { scannedAt: now - 15 * 86400000 }),
  scope('setup-1'),
  scope('sync-1'), scope('sync-2'), scope('sync-3'),
]
const machines = hosts.map((host) => ({ host, groups: environments.filter((env) => env.host === host), skillCount: 0 }))
const agent = (host, over = {}) => ({ id: `${host}-agent`, host, version: 'test', lastSeen: now, paused: false, autoUpdate: false, ...over })
const target = (env, over = {}) => ({ target: env, sourceId: 'shelf', sourceName: 'Shelf', desired: [], missing: [], extra: [], revision: 'r1', appliedRevision: 'r1', updatedAt: now, mode: 'additive', ...over })
const sync = {
  environments,
  machines,
  agents: [agent('work-1'), agent('att-1', { paused: true }), agent('att-2'), agent('sync-1'), agent('sync-2'), agent('sync-3')],
  targets: [
    target(environments[0]),
    target(environments[2]),
    target(environments[4]), target(environments[5]), target(environments[6]),
  ],
  jobs: [{ id: 'job-1', agentId: 'work-1-agent', targetId: 'work-1-agents', kind: 'apply', status: 'running', revision: 'r1', hash: '', plan: {}, message: '', createdAt: now, updatedAt: now }],
  runs: [],
  updateJobs: [],
}
const skillHealth = {active:2,work:{},activity:[],machines:hosts.filter(h=>h!=='setup-1').map(host=>({agentId:`${host}-agent`,host,state:host==='work-1'?'installing':host.startsWith('att')?'paused':'ok',waiting:[],failed:[]}))}
const open = options => openWorkspace(browser, { width: 1440, sync, routes:{'/api/v2/sync/skills':skillHealth}, ...options })
const order = async page => (await page.locator('article.machine h3').allTextContents()).map((name) => name.trim())
const status = page => page.getByLabel('Status')

try {
  if (run('sort and filter')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/machines`)
    await expect(page.getByText('7 machines', { exact: true })).toBeVisible()
    expect(await order(page)).toEqual(['work-1', 'att-1', 'att-2', 'setup-1', 'sync-1', 'sync-2', 'sync-3'])
    // Open terminal asks the workspace for a new shell on this machine, never an existing agent session.
    await expect(page.getByRole('article', { name: 'work-1' }).getByRole('link', { name: 'Open terminal' }))
      .toHaveAttribute('href', `/machines?view=terminal&agent=${encodeURIComponent('work-1-agent')}&new=shell`)

    await status(page).selectOption('attention')
    await expect(page.locator('.fleet-count')).toHaveText('3 of 7 machines')
    expect(await order(page)).toEqual(['att-1', 'att-2', 'setup-1'])
    await expect(page.getByRole('article', { name: 'work-1' })).toHaveCount(0)

    await status(page).selectOption('working')
    await expect(page.locator('.fleet-count')).toHaveText('1 of 7 machines')
    expect(await order(page)).toEqual(['work-1'])

    await status(page).selectOption('synced')
    await expect(page.locator('.fleet-count')).toHaveText('3 of 7 machines')
    expect(await order(page)).toEqual(['sync-1', 'sync-2', 'sync-3'])

    await page.getByLabel('Search machines').fill('att')
    await expect(page.getByText('No machine here matches “att”.', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(page.locator('.fleet-count')).toHaveText('7 machines')
    expect(errors).toEqual([])
    await context.close()
  }

  // A machine enrolled with a personal API token carries a quiet warning, and a how-to on demand. No other machine does.
  // One such machine says it in full on its row; several say it once above the list and each row only marks itself.
  if (run('legacy')) {
    const WHY = 'Enrolled with a personal API token, which can do most of what your account can and never expires.'
    const HOW = 'On that machine run this with a new enrolment code, then revoke the token that machine used, under Account.'
    const CMD = 'reet agent enroll --again'
    const KEEPS = "It keeps the machine's settings and asks for the code."
    const withLegacy = hosts => ({ ...sync, agents: sync.agents.map(a => ({ ...a, legacyToken: hosts.includes(a.host) })) })
    for (const [width, theme, touch] of [[1280, 'light', false], [390, 'dark', true]]) {
      const card = (page, host) => page.locator('article.machine').filter({ has: page.getByRole('heading', { name: host, exact: true }) })
      {
        const { context, page, errors } = await open({ width, height: 900, theme, touch, scale: 2, sync: withLegacy(['work-1']) })
        await page.goto(`${base}/machines`)
        await expect(card(page, 'work-1').getByText(WHY)).toBeVisible()
        await expect(page.getByText(/Enrolled with a personal API token/)).toHaveCount(1)
        await expect(page.locator('[data-legacy-note]')).toHaveCount(0)
        await expect(card(page, 'att-1').locator('[data-legacy-token]')).toHaveCount(0)
        const how = card(page, 'work-1').locator('.legacy-how')
        await expect(how.getByText(HOW)).toBeHidden()
        await how.locator('summary').click()
        await expect(how.getByText(HOW)).toBeVisible()
        await expect(how.locator('pre')).toHaveText(CMD)
        await expect(how.getByText(KEEPS)).toBeVisible()
        await expect(how).not.toContainText(/agent\.json|allow-apply/)
        const again = card(page, 'work-1').getByRole('button', { name: 'Enrol again: work-1' })
        if (touch) {
          expect((await how.locator('summary').boundingBox()).height, 'how-to summary height').toBeGreaterThanOrEqual(44)
          expect((await again.boundingBox()).height, 'Enrol again height').toBeGreaterThanOrEqual(44)
        }
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
        if (process.env.REET_E2E_SHOTS) await card(page, 'work-1').screenshot({ path: `${process.env.REET_E2E_SHOTS}/legacy-one-${width}-${theme}.png` })
        // Its button opens the dialog to enrol that machine again: the name is shown, not asked for.
        await again.click()
        const dialog = page.getByRole('dialog', { name: 'Enrol work-1 again' })
        await expect(dialog).toBeVisible()
        await expect(dialog.getByLabel('Machine name')).toHaveCount(0)
        await expect(dialog.locator('pre.commands')).toHaveText(CMD)
        expect(errors).toEqual([])
        await context.close()
      }
      {
        const five = ['work-1', 'att-1', 'att-2', 'sync-1', 'sync-2']
        const { context, page, errors } = await open({ width, height: 900, theme, touch, scale: 2, sync: withLegacy(five) })
        await page.goto(`${base}/machines`)
        const note = page.locator('[data-legacy-note]')
        await expect(note).toContainText('5 machines were enrolled with a personal API token.')
        await expect(note).toContainText('A personal API token can do most of what your account can and never expires.')
        await expect(page.getByText(/Enrolled with a personal API token/)).toHaveCount(0)
        await expect(page.locator('[data-legacy-token]')).toHaveCount(5)
        await expect(page.getByText('Personal token', { exact: true })).toHaveCount(5)
        await expect(page.getByRole('button', { name: /^Enrol again: / })).toHaveCount(5)
        await expect(card(page, 'sync-3').locator('[data-legacy-token]')).toHaveCount(0)
        await note.locator('summary').click()
        await expect(note.getByText('On each machine run this with a new enrolment code, then revoke the token that machine used, under Account.')).toBeVisible()
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
        if (process.env.REET_E2E_SHOTS) {
          await note.screenshot({ path: `${process.env.REET_E2E_SHOTS}/legacy-five-note-${width}-${theme}.png` })
          await card(page, 'att-2').screenshot({ path: `${process.env.REET_E2E_SHOTS}/legacy-five-row-${width}-${theme}.png` })
        }
        expect(errors).toEqual([])
        await context.close()
      }
    }
  }
  // A machine its owner locked does not look healthy: Locked is its header status, since when, in place of the green dot and the
  // inventory badge. The lock is said once as a line, the rest on demand, nothing offers to unlock it, and what the lock would
  // refuse (Start an agent, Open terminal, updates) is disabled with that line as its reason. Sessions still running are counted.
  if (run('locked')) {
    const SENTENCE = 'Locked on the machine. Only someone there can unlock it, with reet agent unlock.'
    const MEANS = ['Locked on the machine: reet cannot start, attach to or change anything there. It still reports its health and inventory.', 'To unlock, run reet agent unlock on it. Copy unlock command']
    const lockedAt = Math.floor(Date.now() / 1000) - 2 * 3600
    const terminals = [terminalMachine('work-1-agent', 'work-1', false), terminalMachine('att-2-agent', 'att-2', true)]
    terminals[0].locked = true; terminals[0].lockedAt = lockedAt; terminals[0].capabilities.locked = true
    const lockedSync = { ...sync, agents: sync.agents.map(a => ({ ...a, locked: a.host === 'work-1', ...(a.host === 'work-1' ? { lockedAt } : {}) })) }
    const live = (n) => ({ sessions: Array.from({ length: n }, (_, i) => ({ id: `s${i}`, agentId: 'work-1-agent', title: `Job ${i}`, state: 'running', createdAt: now, updatedAt: now, exitCode: null })).concat([{ id: 'old', agentId: 'work-1-agent', title: 'Old', state: 'ended', createdAt: now, updatedAt: now, exitCode: 0 }]) })
    const lockedHealth = { ...skillHealth, machines: skillHealth.machines.map(m => m.host === 'work-1' ? { ...m, state: 'installing' } : m) }
    for (const [sessions, shot] of [[0, 'machines-locked'], [2, 'machines-locked-running'], [1, null]]) {
      for (const [width, theme, touch] of [[1280, 'light', false], [390, 'dark', true]]) {
        const card = (page, host) => page.locator('article.machine').filter({ has: page.getByRole('heading', { name: host, exact: true }) })
        const { context, page, errors } = await open({ width, height: 1000, theme, touch, scale: 2, sync: lockedSync, terminalMachines: terminals, terminalStatus: active, routes: { '/api/v2/sync/skills': lockedHealth, '/api/v2/terminal/sessions': live(sessions) } })
        await page.goto(`${base}/machines`)
        const locked = card(page, 'work-1'), other = card(page, 'att-2')
        // The header status: Locked, since when, with the full date on hover; no green dot, no inventory badge, no "online".
        const badge = locked.locator('.machine-head .badge')
        await expect(badge).toHaveText('Locked since 2 hr ago')
        await expect(badge).toHaveAttribute('title', /\d{4}|\d{1,2}[:.]\d{2}/)
        await expect(locked.locator('.machine-head .dot')).toHaveCount(0)
        await expect(locked).not.toContainText(/Inventory current|Sync agent online|Installing skills/)
        await expect(locked.locator('.sync-copy strong')).toHaveText('Skills wait until it is unlocked there.')
        await expect(other.locator('[data-locked]')).toHaveCount(0)
        await expect(other.locator('.machine-head .badge')).not.toContainText('Locked')
        // The lock is said once as a line, in code, and it is what the disabled actions point at.
        const line = locked.locator('[data-locked] .legacy-line')
        await expect(line).toHaveText('!' + SENTENCE)
        await expect(line.locator('code')).toHaveText('reet agent unlock')
        await expect(locked.getByText(/Only someone there can unlock it/)).toHaveCount(1)
        for (const name of ['Start an agent', 'Open terminal']) {
          const b = locked.getByRole('button', { name })
          await expect(b).toBeDisabled()
          await expect(b).toHaveAccessibleDescription(SENTENCE)
        }
        await expect(locked.locator('.start-reason')).toHaveCount(0)
        await expect(other.getByRole('button', { name: 'Start an agent' })).toBeEnabled()
        await expect(other.getByRole('link', { name: 'Open terminal' })).toBeVisible()
        await expect(locked.locator('[data-agent-update]').getByRole('button', { name: /^Update reet/ })).toBeDisabled()
        // The sessions the lock leaves running are counted, in the singular for one.
        const running = locked.locator('[data-locked] .running')
        if (sessions === 0) await expect(running).toHaveCount(0)
        else {
          const want = sessions === 1
            ? '1 session is still running on this machine and can run commands there, including reet agent unlock. End it from reet, or with reet agent lock --end-sessions on the machine.'
            : '2 sessions are still running on this machine and can run commands there, including reet agent unlock. End them from reet, or with reet agent lock --end-sessions on the machine.'
          await expect(running).toHaveText(want)
        }
        // What it means is on demand, in two lines, with the product's copy button beside the command.
        const how = locked.locator('[data-locked] details')
        await expect(how.getByText(MEANS[0])).toBeHidden()
        await how.locator('summary').click()
        await expect(how.locator('p').nth(0)).toHaveText(MEANS[0])
        await expect(how.locator('p').nth(1)).toHaveText(MEANS[1])
        await expect(how.getByRole('button', { name: 'Copy unlock command' })).toBeVisible()
        // Nothing in reet unlocks it, and no literal backtick shows for it anywhere on the page.
        await expect(page.getByRole('button', { name: /^unlock/i })).toHaveCount(0)
        await expect(page.getByRole('link', { name: /unlock/i })).toHaveCount(0)
        await expect(page.getByRole('menuitem', { name: /unlock/i })).toHaveCount(0)
        expect(await page.evaluate(() => document.body.innerText.includes('`')), 'a literal backtick').toBe(false)
        // Quiet: the mark and the status are the muted ink, not amber or red.
        const colour = (loc) => loc.evaluate((el) => getComputedStyle(el).color)
        const muted = await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--muted').trim())
        expect(muted).not.toBe('')
        expect(await colour(locked.locator('.head-mark')), 'mark colour').toBe(await colour(badge))
        if (touch) expect((await how.locator('summary').boundingBox()).height, 'what this means height').toBeGreaterThanOrEqual(44)
        if (touch) expect((await locked.getByRole('button', { name: 'Open terminal' }).boundingBox()).height, 'Open terminal height').toBeGreaterThanOrEqual(44)
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
        if (shot && process.env.REET_E2E_SHOTS) await locked.screenshot({ path: `${process.env.REET_E2E_SHOTS}/${shot}-${width}-${theme}.png` })
        expect(errors).toEqual([])
        await context.close()
      }
    }
  }
  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/machines`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      const box = await status(page).boundingBox()
      expect(box.height, `status height at ${width}px`).toBeGreaterThanOrEqual(44)
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Machines fleet: attention order, compact status filter, empty recovery and a phone layout.')
