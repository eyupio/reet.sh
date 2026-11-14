// Which agents are signed in on a machine, and their plan limits when the account has them on: the machine's settings
// dialog and the Start dialog's hint. Runs against the built app with every API answer mocked; the reader on the machine,
// the protocol and the controller have their own Go tests.
import { chromium, expect } from '@playwright/test'
import { installApi, tmachines } from './helpers/longtext.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const only = process.env.REET_E2E_ONLY ? new RegExp(process.env.REET_E2E_ONLY, 'i') : null
const run = name => !only || only.test(name)

const HOST = 'dev-laptop', A = tmachines[0].agentId, now = Date.now()
const minute = 60_000, hour = 60 * minute
const signIn = {tool:'claude', state:'signed_in', method:'subscription', account:'person@example.com', plan:'max', limitsState:'not_asked'}
const none = {tool:'codex', state:'none', limitsState:'not_asked'}
const figures = [{kind:'session', usedPercent:19, resetsAt:now + 2 * hour + 10 * minute}, {kind:'week', usedPercent:60, resetsAt:now + 72 * hour}, {kind:'week', label:'Opus', usedPercent:92}]
let answer, refreshed, reads, refreshes, capable

function reset() {
  answer = {state:'read', sentence:'', planLimits:false, checkedAt:now - 3 * minute, accounts:[signIn, none]}
  refreshed = null; reads = 0; refreshes = 0; capable = true
}

async function open({viewport = {width:1280, height:900}, touch = false} = {}) {
  const context = await browser.newContext({viewport, hasTouch:touch, isMobile:touch})
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', e => errors.push(e.message))
  const unmocked = await installApi(page, {extra:{
    '/api/v2/terminal/machines': () => ({machines:tmachines.map(m => m.agentId === A ? {...m, capabilities:{...m.capabilities, agentAccounts:capable}} : m)}),
    [`/api/v2/machines/${A}/accounts`]: () => { reads++; return answer },
    [`/api/v2/machines/${A}/accounts/refresh`]: () => { refreshes++; if (refreshed) answer = refreshed; return answer },
  }})
  return {context, page, errors, unmocked}
}

async function settings(page) {
  await page.goto(`${base}/machines`)
  await page.getByRole('article', {name:HOST, exact:true}).locator('summary[aria-label^="More options"]').click()
  await page.getByRole('menuitem', {name:'Settings…', exact:true}).click()
  const dialog = page.getByRole('dialog', {name:HOST, exact:true})
  await expect(dialog.getByRole('heading', {name:'Agents', exact:true})).toBeVisible()
  return dialog
}

try {
  if (run('sign-ins without plan limits')) {
    reset()
    const {context, page, errors} = await open()
    const dialog = await settings(page)
    const claude = dialog.locator('[data-agent=claude]'), codex = dialog.locator('[data-agent=codex]')
    await expect(claude.locator('.sign-in')).toHaveText('Signed in as person@example.com · Max plan')
    await expect(codex.locator('.sign-in')).toHaveText('No sign-in found on this machine')
    const row = dialog.locator('.sign-ins')
    await expect(row).toContainText('Sign-ins read 3 min ago.')
    // With the setting off there are no figures, and the page says where to turn them on.
    await expect(row).toContainText('Plan limits are off. Turn them on in Settings > Agents to see how much of each plan is left.')
    await expect(dialog.locator('.limits')).toHaveCount(0)
    await expect(dialog.locator('.limits-note')).toHaveCount(0)
    expect(reads).toBe(1)
    // A person asking now: the machine is asked again and the answer replaces the one shown.
    refreshed = {...answer, checkedAt:Date.now(), accounts:[{...signIn, account:'other@example.com', plan:'pro'}, {tool:'codex', state:'signed_in', method:'api_key', limitsState:'unsupported'}]}
    await row.getByRole('button', {name:'Check sign-ins', exact:true}).click()
    await expect(claude.locator('.sign-in')).toHaveText('Signed in as other@example.com · Pro plan')
    await expect(codex.locator('.sign-in')).toHaveText('Signed in with an API key')
    await expect(row).toContainText('Sign-ins read just now.')
    expect(refreshes).toBe(1)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('plan limits')) {
    reset()
    answer = {...answer, planLimits:true, accounts:[{...signIn, limitsState:'read', limits:figures, limitsAt:now - 3 * minute}, none]}
    const {context, page, errors} = await open()
    const dialog = await settings(page)
    const claude = dialog.locator('[data-agent=claude]')
    const list = claude.getByRole('list', {name:'Claude Code plan limits', exact:true})
    await expect(list.getByRole('listitem')).toHaveCount(3)
    await expect(list.getByRole('listitem').nth(0)).toContainText('Session 81% left')
    await expect(list.getByRole('listitem').nth(0)).toContainText(/resets in 2 h \d+ min/)
    await expect(list.getByRole('listitem').nth(1)).toContainText('Week 40% left')
    await expect(list.getByRole('listitem').nth(1)).toContainText(/resets in (2 days|3 days)/)
    await expect(list.getByRole('listitem').nth(2)).toContainText('Opus this week 8% left')
    await expect(list.getByRole('listitem').nth(2)).toHaveClass(/low/)
    await expect(list.getByRole('listitem').nth(0)).not.toHaveClass(/low/)
    await expect(list.getByRole('listitem').nth(0).locator('.limit-meter > span')).toHaveAttribute('style', /width:\s*81%/)
    await expect(dialog.locator('.sign-ins')).not.toContainText('Plan limits are off')
    await expect(claude.locator('.limits-note')).toHaveCount(0)
    // A sign-in that needs renewing keeps the last figures and says what to do; nothing claims they are current.
    refreshed = {...answer, checkedAt:Date.now(), accounts:[{...signIn, limitsState:'renew', limits:figures, limitsAt:now - 12 * minute}, none]}
    await dialog.getByRole('button', {name:'Check sign-ins', exact:true}).click()
    await expect(claude.locator('.limits-note')).toHaveText("Claude Code's sign-in needs renewing before its limits can be read. Start Claude Code on this machine once; reet never renews a sign-in itself. Showing the figures read 12 min ago.")
    await expect(list.getByRole('listitem')).toHaveCount(3)
    // No figures at all: only the reason.
    refreshed = {...answer, checkedAt:Date.now(), accounts:[{...signIn, limitsState:'failed'}, none]}
    await dialog.getByRole('button', {name:'Check sign-ins', exact:true}).click()
    await expect(claude.locator('.limits-note')).toHaveText("This machine couldn't read Claude Code's limits just now.")
    await expect(claude.locator('.limits')).toHaveCount(0)
    for (const theme of ['light', 'dark']) {
      await page.evaluate(theme => { document.documentElement.dataset.theme = theme }, theme)
      if (process.env.REET_UI_SCREENSHOTS) await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/agent-accounts-${theme}-1280.png`})
    }
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('a machine that cannot say')) {
    reset()
    answer = {state:'off', sentence:"Terminal access is off on that machine, so reet doesn't read its agents' sign-ins.", planLimits:false, checkedAt:now - 2 * hour, accounts:[signIn, none]}
    const {context, page, errors} = await open()
    const dialog = await settings(page)
    // What was kept is still shown, with why it is not newer and how old it is.
    await expect(dialog.locator('.sign-ins')).toContainText("Terminal access is off on that machine, so reet doesn't read its agents' sign-ins. Showing what was read 2 h ago.")
    await expect(dialog.locator('[data-agent=claude] .sign-in')).toHaveText('Signed in as person@example.com · Max plan')
    await context.close()
    // An older reet on the machine does not report the capability: nothing is asked, and the page says what to do.
    reset(); capable = false
    const older = await open()
    const second = await settings(older.page)
    await expect(second.locator('.sign-ins')).toContainText(`Update reet on ${HOST} to see which agents are signed in.`)
    await expect(second.getByRole('button', {name:'Check sign-ins', exact:true})).toBeDisabled()
    await expect(second.locator('.sign-in')).toHaveCount(0)
    expect(reads).toBe(0)
    expect(refreshes).toBe(0)
    expect(errors).toEqual([]); expect(older.errors).toEqual([])
    await older.context.close()
  }

  if (run('start dialog hint')) {
    reset()
    const {context, page, errors} = await open()
    await page.goto(`${base}/machines`)
    await page.getByRole('article', {name:HOST, exact:true}).getByRole('button', {name:'Start an agent', exact:true}).first().click()
    const dialog = page.getByRole('dialog', {name:'Start an agent', exact:true})
    const agent = dialog.getByRole('combobox', {name:'Agent', exact:true})
    await agent.selectOption('codex')
    const hint = dialog.locator('[data-sign-in-hint]')
    await expect(hint).toHaveText(`No Codex sign-in was found on ${HOST}. Sign in to Codex there first, or its first message may be refused.`)
    // A signed-in agent gets no hint. The hint is advice: it changes nothing about whether the dialog can start.
    const start = dialog.getByRole('button', {name:'Start', exact:true})
    await agent.selectOption('claude')
    await expect(hint).toHaveCount(0)
    const withoutHint = await start.isDisabled()
    await agent.selectOption('codex')
    await expect(hint).toBeVisible()
    expect(await start.isDisabled()).toBe(withoutHint)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      reset()
      answer = {...answer, planLimits:true, accounts:[{...signIn, account:'a.very.long.address.without.breaks@subdomain.example.com', limitsState:'renew', limits:figures, limitsAt:now - 12 * minute}, none]}
      const {context, page, errors} = await open({viewport:{width, height:800}, touch:true})
      const dialog = await settings(page)
      await expect(dialog.locator('[data-agent=claude] .limits li')).toHaveCount(3)
      for (const theme of ['light', 'dark']) {
        await page.evaluate(theme => { document.documentElement.dataset.theme = theme }, theme)
        // Nothing in the dialog is wider than the dialog, and the page does not scroll sideways.
        const wide = await dialog.evaluate(d => [...d.querySelectorAll('.sign-ins, [data-agent] .copy, .limits li, .limits-note')].filter(el => el.getBoundingClientRect().right > d.getBoundingClientRect().right + 0.5 || el.scrollWidth > el.clientWidth + 1).map(el => el.className))
        expect(wide).toEqual([])
        expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
        const button = await dialog.getByRole('button', {name:'Check sign-ins', exact:true}).boundingBox()
        expect(button.height).toBeGreaterThanOrEqual(44)
        if (process.env.REET_UI_SCREENSHOTS) await dialog.locator('[data-agent=claude]').screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/agent-accounts-${theme}-${width}.png`})
      }
      expect(errors).toEqual([])
      await context.close()
    }
  }
  console.log('Agent sign-ins and plan limits passed.')
} finally { await browser.close() }
