import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { active, openWorkspace } from './helpers/workspace.mjs'

// What is left of each agent's plan, on Home and on the Sessions toolbar: a chip per account with the agent's mark and
// the share left of its tightest window, a summary under a mouse, and every limit with its reset on a click. The same
// account on two machines is one chip; a machine that does not read sign-ins is never asked.
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173', shots = process.env.REET_UI_SCREENSHOTS || '/tmp/reet-plan-meters'
await mkdir(shots, { recursive: true })
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now(), minute = 60_000, day = 86_400_000
const caps = { protocolVersion: 1, os: 'linux', osUser: 'sam', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready', launch: true, acp: true, acpLiveControls: true, tools: ['claude', 'codex'] }
const machine = (agentId, host, reads) => ({ agentId, host, ready: true, serverEnabled: true, capabilities: { ...caps, agentAccounts: reads } })
const plan = (tool, account, name, limits, limitsState = 'read', limitsAt = now) => ({ tool, state: 'signed_in', method: 'subscription', account, plan: name, limitsState, limitsAt, limits })
const claude = plan('claude', 'sam@example.test', 'max', [{ kind: 'session', usedPercent: 19, resetsAt: now + 130 * minute }, { kind: 'week', usedPercent: 88, resetsAt: now + 3 * day + 60 * minute }])
const answer = accounts => ({ state: 'read', sentence: '', planLimits: true, checkedAt: now, accounts })
const session = { id: 's1', agentId: 'a1', title: 'Export API', tool: 'claude', model: 'opus', structured: true, state: 'running', createdAt: now, updatedAt: now, exitCode: null, dir: '/work/billing' }

async function open({ width, height, touch = false, theme = 'light', machines, reads }) {
  const opened = await openWorkspace(browser, { width, height, touch, theme, terminalStatus: active, terminalMachines: machines, sessionFocus: false, routes: {
    '/api/v2/terminal/sessions': { sessions: [session] },
    // studio and laptop are signed in to the same Claude account; laptop read it an hour earlier. Codex is on laptop
    // only, and its latest read failed, so its figures are three hours old.
    '/api/v2/machines/a1/accounts': () => { reads.push('a1'); return answer([claude]) },
    '/api/v2/machines/a2/accounts': () => { reads.push('a2'); return answer([{ ...claude, limitsAt: now - 60 * minute, limits: [{ kind: 'session', usedPercent: 5 }] }, plan('codex', 'sam@example.test', 'plus', [{ kind: 'session', usedPercent: 42, resetsAt: now + 95 * minute }, { kind: 'week', usedPercent: 31, resetsAt: now + 5 * day + 60 * minute }], 'failed', now - 180 * minute)]) },
    '/api/v2/machines/a3/accounts': () => { reads.push('a3'); return answer([]) },
  } })
  await opened.page.addInitScript(() => { window.EventSource = class { addEventListener() {} close() {} } })
  return opened
}
const fleet = [machine('a1', 'studio', true), machine('a2', 'laptop', true), machine('a3', 'old-box', false)]
const inside = node => { const box = node.getBoundingClientRect(); return box.left >= 0 && box.top >= 0 && box.right <= innerWidth + 0.5 && box.bottom <= innerHeight + 0.5 }

try {
  for (const [path, place] of [['/', 'Home'], ['/sessions', 'Sessions']]) {
    const reads = [], { page, context, errors } = await open({ width: 1440, height: 900, machines: fleet, reads })
    await page.goto(base + path)
    const meters = page.locator('[data-plan-left]'), strip = meters.locator('button.strip')
    await expect(strip).toBeVisible()
    // One chip per account, not per machine, each with its agent's mark; the freshest read of a shared account wins.
    const chips = strip.locator('.chip')
    await expect(chips).toHaveCount(2)
    await expect(chips.nth(0).locator('.value')).toHaveText('12%')
    await expect(chips.nth(0)).toHaveClass(/low/)
    await expect(chips.nth(1).locator('.value')).toHaveText('58%')
    // Figures kept from a failed read dim and are no alarm.
    await expect(chips.nth(1)).toHaveClass(/stale/)
    await expect(strip).toHaveAccessibleName(/^Plan limits\. Claude Code: Session 81% left · Week 12% left\. Codex: Session 58% left · Week 69% left \(read 3 h ago\)\. Open for every limit/)
    // A machine that does not read sign-ins is never asked.
    expect(reads).not.toContain('a3'); expect(reads).toContain('a1'); expect(reads).toContain('a2')

    // Under a mouse: both accounts in a tip that takes no clicks, gone when the mouse leaves.
    const tip = meters.getByRole('tooltip')
    await expect(tip).toBeHidden()
    await strip.hover()
    await expect(tip).toBeVisible()
    await expect(tip).toContainText('Claude Code'); await expect(tip).toContainText('Session 81% left · Week 12% left'); await expect(tip).toContainText('Codex')
    expect(await tip.evaluate(inside)).toBe(true)
    await page.screenshot({ path: `${shots}/plan-meters-${place.toLowerCase()}-1440-hover.png` })
    await page.mouse.move(700, 600)
    await expect(tip).toBeHidden()
    // Keyboard focus shows the same tip; Enter opens the panel and closes the tip.
    await strip.focus()
    await page.keyboard.press('Tab'); await page.keyboard.press('Shift+Tab')
    await expect(tip).toBeVisible()
    await page.keyboard.press('Enter')
    const panel = page.getByRole('dialog', { name: 'Plan limits', exact: true })
    await expect(panel).toBeVisible(); await expect(tip).toBeHidden()
    await expect(strip).toHaveAttribute('aria-expanded', 'true')
    await expect(panel.getByRole('button', { name: 'Close plan limits', exact: true })).toBeFocused()
    const own = panel.getByRole('region', { name: 'Claude Code, sam@example.test', exact: true }), kept = panel.getByRole('region', { name: 'Codex, sam@example.test', exact: true })
    await expect(own).toContainText('Max'); await expect(own).toContainText('Read on studio just now.')
    await expect(own.getByRole('meter', { name: 'Session left', exact: true })).toHaveAttribute('aria-valuenow', '81')
    await expect(own.getByRole('meter', { name: 'Week left', exact: true })).toHaveAttribute('aria-valuenow', '12')
    await expect(own).toContainText(/Resets in 2 h \d+ min/); await expect(own).toContainText('Resets in 3 days')
    await expect(kept).toContainText('Plus'); await expect(kept).toContainText('Read on laptop 3 h ago.')
    await expect(kept).toContainText("This machine couldn't read Codex's limits just now. Showing the figures read 3 h ago.")
    await expect(panel.getByRole('link', { name: 'Machines', exact: true })).toHaveAttribute('href', '/machines')
    expect(await panel.evaluate(inside)).toBe(true)
    await page.screenshot({ path: `${shots}/plan-meters-${place.toLowerCase()}-1440-open.png` })
    // Escape closes it and gives focus back; so does a click anywhere else.
    await page.keyboard.press('Escape')
    await expect(panel).toBeHidden(); await expect(strip).toBeFocused(); await expect(strip).toHaveAttribute('aria-expanded', 'false')
    await strip.click(); await expect(panel).toBeVisible()
    await page.mouse.click(700, 700)
    await expect(panel).toBeHidden()
    expect(errors).toEqual([])
    await context.close()
  }

  // A phone has no hover: a tap opens the panel, which fits the screen in both themes, with a 44 px close button.
  for (const theme of ['light', 'dark']) {
    const reads = [], { page, context, errors } = await open({ width: 390, height: 844, touch: true, theme, machines: fleet, reads })
    await page.goto(base + '/')
    const meters = page.locator('[data-plan-left]'), strip = meters.locator('button.strip')
    await expect(strip).toBeVisible()
    expect((await strip.boundingBox()).height).toBeGreaterThanOrEqual(36)
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
    await strip.tap()
    const panel = page.getByRole('dialog', { name: 'Plan limits', exact: true })
    await expect(panel).toBeVisible()
    await expect(meters.getByRole('tooltip')).toBeHidden()
    expect(await panel.evaluate(inside)).toBe(true)
    const close = await panel.getByRole('button', { name: 'Close plan limits', exact: true }).boundingBox()
    expect(Math.round(close.width)).toBeGreaterThanOrEqual(44); expect(Math.round(close.height)).toBeGreaterThanOrEqual(44)
    await page.screenshot({ path: `${shots}/plan-meters-home-390-${theme}-open.png` })
    await panel.getByRole('button', { name: 'Close plan limits', exact: true }).tap()
    await expect(panel).toBeHidden()
    expect(errors).toEqual([])
    await context.close()
  }

  // No machine reads sign-ins: nothing is asked and nothing is drawn, on either page.
  {
    const reads = [], { page, context } = await open({ width: 1440, height: 900, machines: [machine('a3', 'old-box', false)], reads })
    for (const path of ['/', '/sessions']) {
      await page.goto(base + path)
      await expect(page.getByRole('heading', { level: 1 }).first()).toBeVisible()
      await expect(page.locator('[data-plan-left]')).toHaveCount(0)
    }
    expect(reads).toEqual([])
    await context.close()
  }
  console.log('Plan meters: one chip per account on Home and Sessions, a tip under a mouse and on keyboard focus, every limit on a click, a phone panel that fits, and nothing asked of a machine that does not read sign-ins.')
} finally { await browser.close() }
