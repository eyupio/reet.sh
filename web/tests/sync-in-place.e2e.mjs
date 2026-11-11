// Sync all machines is one press wherever it is offered: the Skills header, the toast after adding a skill and the notice
// after saving a skill you wrote. Each does the whole job where it is, and none opens the Sync page for a second click.
// With no machine to sync to, or on a read-only account, it still opens Sync, which says what is missing.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace, skill } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const now = Date.now()
const one = skill('review')
const scope = { id: 'laptop-agents', host: 'Laptop', agent: 'agents', name: 'Laptop', skills: [], skillCount: 0, complete: true, scannedAt: now, lastSeen: now }
const laptop = { id: 'a1', host: 'Laptop', version: 'test', lastSeen: now, paused: false, autoUpdate: false, keepSynced: true }
const desktop = { id: 'a2', host: 'Desktop', lastSeen: now - 300000, paused: false, keepSynced: true }
const target = { target: scope, sourceId: 'library', sourceName: 'Shared library', mode: 'additive', desired: [one], missing: [one], extra: [], revision: 'r1', appliedRevision: '', updatedAt: now - 600000, followSource: true }
// Laptop is missing a skill and Desktop is switched off.
const behind = () => ({ environments: [scope], machines: [{ host: 'Laptop', groups: [scope], skillCount: 0 }], agents: [laptop, desktop], targets: [target], jobs: [], runs: [], updateJobs: [] })
// What the server answers once the button is pressed: Laptop is working through its install.
const working = () => ({
  ...behind(),
  runs: [{ id: 'run-1', agentId: 'a1', targets: ['laptop-agents'], cursor: 0, status: 'running', createdAt: now, updatedAt: now }],
  jobs: [{ id: 'job-1', agentId: 'a1', targetId: 'laptop-agents', kind: 'auto', status: 'running', revision: 'r1', hash: '', plan: {}, message: '', stage: 'apply', stages: [{ key: 'read', label: 'Read machine', status: 'done' }, { key: 'plan', label: 'Plan changes', status: 'done' }, { key: 'apply', label: 'Apply changes', status: 'running', detail: 'Installing review · 1 of 1' }], createdAt: now, updatedAt: now }],
})
// Everything is already where it should be.
const current = () => ({ ...behind(), targets: [{ ...target, missing: [], appliedRevision: 'r1', updatedAt: now - 600000 }] })

// A workspace whose Sync all machines answers from `answer()`, recording every press.
async function open(options, answer) {
  let sync = options.sync ?? behind()
  const presses = []
  const fixture = await openWorkspace(browser, {
    skills: [one], ...options, sync,
    routes: {
      '/api/v2/sync': () => sync,
      '/api/v2/sync/library': request => {
        presses.push(request.postDataJSON())
        const result = answer ? answer() : working()
        if (result.status) return result
        sync = result
        return sync
      },
      '/api/ignored': { ignored: [] },
      '/api/skills': request => request.method() === 'POST'
        ? { ...skill('fresh'), created: true }
        : { skills: [one], counts: { all: 1, used: 0, liked: 0, bookmarked: 0 } },
      ...options.routes,
    },
  })
  return { ...fixture, presses }
}
const capture = async (page, name) => {
  if (!process.env.REET_CAPTURE) return
  await mkdir(process.env.REET_CAPTURE, { recursive: true })
  // Toasts and the dock slide in; look at them once they have settled.
  await page.waitForTimeout(600)
  await page.screenshot({ path: `${process.env.REET_CAPTURE}/${name}.png`, fullPage: false })
}
const dock = page => page.getByRole('complementary', { name: 'Sync activity' })
const syncButton = page => page.getByRole('button', { name: 'Sync all machines', exact: true })

try {
  // The Skills header: one press, and the activity dock follows it live. The page stays where it is.
  if (run('header')) {
    for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
      const { context, page, presses, errors } = await open({ width, height: width === 390 ? 812 : 900, touch: width === 390, theme })
      await page.goto(`${base}/skills`)
      await expect(page.getByRole('link', { name: 'Sync all machines' }), 'a button, not a link to another page').toHaveCount(0)
      await syncButton(page).click()
      await expect(dock(page)).toContainText('Syncing your machines')
      await expect(dock(page)).toContainText('All connected machines')
      expect(presses).toEqual([{}])
      expect(new URL(page.url()).pathname).toBe('/skills')
      await expect(syncButton(page)).toBeEnabled()
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      await capture(page, `in-place-header-${width}-${theme}`)
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // Nothing to do: one line says where things stand, and nothing pretends to be working.
  if (run('already in sync')) {
    const { context, page, presses } = await open({ width: 1280 }, current)
    await page.goto(`${base}/skills`)
    await syncButton(page).click()
    const toast = page.getByRole('status').filter({ hasText: 'Everything is in sync.' })
    await expect(toast).toContainText("Desktop will catch up when it's back online.")
    await expect(dock(page)).toHaveCount(0)
    expect(presses).toEqual([{}])
    await capture(page, 'in-place-current-1280-light')
    await context.close()
  }

  // A refusal says why, in words, on whichever page the button is on, and the button is ready to try again.
  if (run('refused')) {
    const { context, page } = await open({ width: 1280 }, () => ({ status: 409, json: { error: 'The library needs a source conflict resolved.' } }))
    await page.goto(`${base}/skills`)
    await syncButton(page).click()
    await expect(page.getByRole('alert').filter({ hasText: 'The library needs a source conflict resolved.' })).toBeVisible()
    await expect(syncButton(page)).toBeEnabled()
    await expect(dock(page)).toHaveCount(0)
    await context.close()
  }

  // After adding a skill the toast offers the same press. It syncs where it is.
  if (run('toast')) {
    for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
      const { context, page, presses, errors } = await open({ width, height: width === 390 ? 812 : 900, touch: width === 390, theme })
      await page.goto(`${base}/skills`)
      await page.getByLabel('Skill install command, URL or slug').fill('skills.sh/demo/toolkit/fresh')
      await page.getByRole('button', { name: 'Add to library' }).click()
      const toast = page.getByRole('status').filter({ hasText: 'Ready for your machines.' })
      await expect(toast).toContainText('Added fresh to your library.')
      await capture(page, `in-place-toast-${width}-${theme}`)
      await toast.getByRole('button', { name: 'Sync all machines' }).click()
      await expect(dock(page)).toContainText('Syncing your machines')
      expect(presses).toEqual([{}])
      expect(new URL(page.url()).pathname).toBe('/skills')
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // After saving a skill you wrote, the notice offers the same press on the Write a skill page.
  if (run('written skill')) {
    for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
      const hosted = { slug: 'reet-review', description: 'Reviews code', content: '---\nname: reet-review\ndescription: Reviews code\n---\nReview carefully.', files: {}, published: false, createdAt: now, updatedAt: now }
      const { context, page, presses, errors } = await open({
        width, height: width === 390 ? 812 : 900, touch: width === 390, theme,
        hosting: { username: 'sam', prefix: 'reet-', skills: [hosted] },
        routes: { '/api/v2/hosting/skills/reet-review': request => request.postDataJSON() },
      })
      await page.goto(`${base}/hosting`)
      await page.locator('.skill-choice').filter({ hasText: 'reet-review' }).click()
      await page.locator('.save-dock').getByRole('button', { name: 'Save draft', exact: true }).click()
      const notice = page.getByRole('status').filter({ hasText: 'Ready for your machines.' })
      await expect(notice).toBeVisible()
      await expect(page.getByRole('link', { name: 'Sync all machines' })).toHaveCount(0)
      await capture(page, `in-place-written-${width}-${theme}`)
      await notice.getByRole('button', { name: 'Sync all machines' }).click()
      await expect(dock(page)).toContainText('Syncing your machines')
      // The activity dock keeps clear of the save bar, so Save draft stays reachable while it shows.
      await expect.poll(() => page.evaluate(() => {
        const a = document.querySelector('.sync-dock')?.getBoundingClientRect(), b = document.querySelector('.save-dock')?.getBoundingClientRect()
        return !!a && !!b && a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top
      }), { message: 'the activity dock covers the save bar' }).toBe(false)
      await capture(page, `in-place-written-dock-${width}-${theme}`)
      expect(presses).toEqual([{}])
      expect(new URL(page.url()).pathname).toBe('/hosting')
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // No machine yet: there is nothing to sync to, so the header and the toast still open Sync, which leads to adding one.
  if (run('no machine')) {
    const empty = { environments: [], machines: [], agents: [], targets: [], jobs: [], runs: [], updateJobs: [] }
    const { context, page, presses } = await open({ width: 1280, sync: empty })
    await page.goto(`${base}/skills`)
    await expect(page.getByRole('link', { name: 'Sync all machines' })).toHaveAttribute('href', '/sync')
    await page.getByLabel('Skill install command, URL or slug').fill('skills.sh/demo/toolkit/fresh')
    await page.getByRole('button', { name: 'Add to library' }).click()
    await page.getByRole('status').filter({ hasText: 'Ready for your machines.' }).getByRole('button', { name: 'Sync all machines' }).click()
    await page.waitForURL(/\/sync$/)
    expect(presses).toEqual([])
    await context.close()
  }

  // A sync that stops because the machine already has the skill from somewhere else says so in plain words in the dock,
  // and the fix it names is a button there: first it reads what would change, then changes it once agreed.
  if (run('source conflict')) {
    const failed = () => ({
      ...behind(),
      jobs: [{ id: 'job-9', agentId: 'a1', targetId: 'laptop-agents', kind: 'auto', status: 'failed', revision: 'r1', hash: '', plan: {}, message: 'review is shared with agents from a different source; resolve it before syncing', createdAt: now, updatedAt: now }],
    })
    const calls = []
    // The press starts a run; the next read of the sync state is the failed attempt.
    let pressed = false
    const { context, page } = await open({
      width: 390,
      routes: {
        '/api/v2/sync': () => pressed ? failed() : behind(),
        '/api/v2/sync/source-conflicts/resolve': request => {
          const body = request.postDataJSON()
          calls.push(body.confirm ? 'apply' : 'preview')
          return { changes: [{ targetId: 'laptop-agents', id: 'demo/toolkit/review', name: 'review', slug: 'review', from: 'demo/toolkit', to: 'other/source', agent: 'agents' }] }
        },
        '/api/v2/sync/run': () => { calls.push('run'); return working() },
      },
    }, () => { pressed = true; return working() })
    await page.goto(`${base}/skills`)
    await syncButton(page).click()
    const panel = dock(page)
    await panel.getByRole('button', { name: /Sync failed on Laptop.*Open sync activity/ }).click()
    await expect(panel).toContainText('Laptop already has a copy of review that came from somewhere else, in the shared skills folder, so reet did not overwrite it.')
    await expect(panel).not.toContainText('resolve it before syncing')
    await panel.getByRole('button', { name: 'Auto fix', exact: true }).click()
    const confirm = panel.getByRole('group', { name: 'Confirm auto fix' })
    await expect(confirm).toContainText('Keep the copy of review already on Laptop and sync again? Nothing on the machine is changed.')
    expect(calls).toEqual(['preview'])
    await capture(page, 'sync-in-place-source-conflict-390')
    await confirm.getByRole('button', { name: 'Auto fix & sync' }).click()
    await expect(confirm).toHaveCount(0)
    expect(calls).toEqual(['preview', 'apply', 'run'])
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await context.close()
  }

  // A read-only account cannot sync, so it is not handed a button that would be refused. It still opens Sync to read where things stand.
  if (run('read-only')) {
    const { context, page, presses } = await open({ width: 1280, status: { user: { id: 2, name: 'Reader', role: 'viewer' } } })
    await page.goto(`${base}/skills`)
    await expect(page.getByRole('link', { name: 'Sync all machines' })).toHaveAttribute('href', '/sync')
    await expect(syncButton(page)).toHaveCount(0)
    expect(presses).toEqual([])
    await context.close()
  }
} finally {
  await browser.close()
}
console.log('Sync in place: the Skills header, the add-skill toast and the written-skill notice each sync every machine without leaving the page; no machine or a read-only account still opens Sync.')
