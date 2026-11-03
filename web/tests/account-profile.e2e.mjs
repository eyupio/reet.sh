// Account → Profile: the preferences in one panel, grouped by identity,
// appearance, shelf/terminal and publishing.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const makeState = () => {
  let profile = {
    theme: 'system', timezone: 'Europe/London', dateFormat: 'locale', shelfView: 'list', cardsExpanded: false,
    hostedPrefix: 'reet-', terminalRemoveOnClose: false, updatedAt: now,
  }
  const patches = []
  return {
    patches,
    routes: {
      '/api/v2/profile': request => {
        if (request.method() === 'PATCH') {
          const body = request.postDataJSON()
          patches.push(body)
          profile = { ...profile, ...body, updatedAt: Date.now() }
        }
        return { user: { id: 1, name: 'Sam Taylor', email: 'sam@example.test', role: 'owner' }, profile, sessionCount: 1, accounts: true }
      },
    },
  }
}
const open = options => {
  const state = makeState()
  return openWorkspace(browser, { width: 1440, routes: state.routes, ...options }).then((opened) => ({ ...opened, patches: state.patches }))
}
const panel = page => page.getByRole('region', { name: 'Your preferences' })
const count = page => panel(page).getByRole('status').first()

try {
  if (run('panel and save')) {
    const { context, page, errors, patches } = await open({})
    await page.goto(`${base}/account/profile`)
    await expect(count(page)).toHaveText('8 preferences')
    for (const name of ['Identity', 'Appearance', 'Shelf and terminal', 'Publishing']) {
      await expect(page.getByRole('group', { name })).toBeVisible()
    }

    await page.locator('#profile-form input[autocomplete="name"]').fill('Sam Taylor')
    const form = page.locator('#profile-form')
    await expect(form.getByLabel('Dates')).toHaveValue('locale')
    await form.getByLabel('Theme').selectOption('dark')
    await form.getByLabel('Time zone').fill('Europe/London')
    await form.getByLabel('Dates').selectOption('dmy')
    await form.getByLabel('Terminal history default').selectOption({ label: 'Remove from history after it ends' })
    await form.getByLabel('Default shelf layout').selectOption('board')
    await form.getByLabel('Default skill cards').selectOption({ label: 'Expanded — all details' })
    await form.getByLabel('Hosted skill name prefix').fill('acme-')
    await panel(page).getByRole('button', { name: 'Save profile' }).click()
    await expect(page.getByText('Reet, defaults saved. Your preferences follow you to every browser you sign in from.', { exact: true })).toBeVisible()
    expect(patches).toHaveLength(1)
    expect(patches[0]).toMatchObject({
      name: 'Sam Taylor', theme: 'dark', timezone: 'Europe/London', dateFormat: 'dmy',
      shelfView: 'board', cardsExpanded: true, terminalRemoveOnClose: true, hostedPrefix: 'acme-',
    })
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/account/profile`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      await expect(panel(page).getByRole('button', { name: 'Save profile' })).toBeVisible()
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Account profile: one grouped preferences panel, the save payload and a phone layout.')
