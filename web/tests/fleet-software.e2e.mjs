// Fleet software is one sentence and one button: "OpenCode can be updated on 2 machines" and "Update 2 machines". The button
// asks once, naming the machines in order, before anything starts. Choosing machines, ordering them and updating one on its
// own are still there, under Machines, for the few who want them.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const plan = (agentId, host, state, extra = {}) => ({
  agentId, host, state, sessionsRunning: 0, canApply: true, sentence: state === 'none' ? 'OpenCode is current.' : 'A reviewed plan is ready.',
  ...(state === 'update' ? { action: 'update', version: '1.18.0', target: '1.19.5', source: 'verified tarball' } : {}),
  ...(state === 'none' ? { version: '1.19.5' } : {}),
  ...extra,
})
const laptop = () => plan('a-laptop', 'Laptop', 'update')
const build = () => plan('a-build', 'Build server', 'update')
const desktop = () => plan('a-desktop', 'Desktop', 'none')
const busy = () => plan('a-busy', 'Mac mini', 'update', { sessionsRunning: 1 })
const member = (position, agentId, host, state, extra = {}) => ({ position, agentId, host, action: 'update', target: '1.19.5', state, ...extra })
const batch = (state, ...members) => ({ id: 'b1', tool: 'opencode', state, createdAt: 1, updatedAt: 1, members })

// A workspace whose software answers come from `world`: the machines' plans, the batch (if any), and what happens when one starts.
async function open(options, world) {
  const starts = []
  const fixture = await openWorkspace(browser, {
    ...options,
    routes: {
      '/api/v2/settings/software-mirror': { enabled: false, cached: false },
      '/api/v2/software/plans': () => ({ machines: world.rows }),
      '/api/v2/software/batches/active': () => ({ json: world.active ?? null }),
      '/api/v2/software/batches': request => {
        starts.push(request.postDataJSON())
        if (world.refuse) return { status: 409, json: { error: world.refuse } }
        world.active = world.started
        return world.started
      },
      '/api/v2/software/batches/b1': () => world.poll ? world.poll() : world.active,
      '/api/v2/software/batches/b1/members/1/skip': () => { world.active = world.afterSkip; return world.afterSkip },
    },
  })
  return { ...fixture, starts }
}
const openModal = async page => {
  await page.goto(`${base}/machines`)
  await page.getByRole('button', { name: 'Software', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Fleet software' })
  await expect(dialog).toBeVisible()
  return dialog
}
const capture = async (page, name) => {
  if (!process.env.REET_CAPTURE) return
  await mkdir(process.env.REET_CAPTURE, { recursive: true })
  await page.waitForTimeout(400)
  await page.screenshot({ path: `${process.env.REET_CAPTURE}/${name}.png`, fullPage: false })
}
const notOverflowing = page => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)

try {
  // Two machines can update: one sentence naming them and one button, which asks once before it starts anything. Machines that
  // cannot be updated right now are named and left out. The list is there, closed, until someone wants it.
  if (run('one press')) {
    for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
      const world = {
        rows: [laptop(), build(), desktop(), busy()],
        started: batch('running', member(0, 'a-laptop', 'Laptop', 'running'), member(1, 'a-build', 'Build server', 'pending')),
        // Once both are updated their plans say so, so there is nothing left to press for.
        poll: () => {
          world.rows = [plan('a-laptop', 'Laptop', 'none'), plan('a-build', 'Build server', 'none'), desktop(), busy()]
          return batch('done', member(0, 'a-laptop', 'Laptop', 'applied', { version: '1.19.5' }), member(1, 'a-build', 'Build server', 'applied', { version: '1.19.5' }))
        },
      }
      const { context, page, starts, errors } = await open({ width, height: width === 390 ? 812 : 900, touch: width === 390, theme }, world)
      const dialog = await openModal(page)
      const status = dialog.getByRole('status').filter({ hasText: 'OpenCode can be updated on 2 machines.' })
      await expect(status).toContainText('Laptop and Build server, to 1.19.5.')
      await expect(status).toContainText("Mac mini can't be updated right now.")
      await expect(dialog.getByText("Machines that aren't connected are skipped")).toBeVisible()
      await expect(dialog.locator('details.machines')).not.toHaveAttribute('open', '')
      await capture(page, `fleet-software-ready-${width}-${theme}`)
      await dialog.getByRole('button', { name: 'Update 2 machines', exact: true }).click()
      // It asks once, naming the machines in the order they will be updated. Nothing has started.
      const confirm = page.getByRole('dialog', { name: 'Update 2 machines?', exact: true })
      await expect(confirm).toBeVisible()
      await expect(confirm.locator('li').nth(0)).toContainText('Laptop')
      await expect(confirm.locator('li').nth(1)).toContainText('Build server')
      await expect(confirm).toContainText("A machine that isn't connected is skipped.")
      expect(starts).toHaveLength(0)
      await capture(page, `fleet-software-confirm-${width}-${theme}`)
      // Cancelling changes nothing, and the one button is still there.
      await confirm.getByRole('button', { name: 'Cancel', exact: true }).click()
      await expect(confirm).toBeHidden()
      expect(starts).toHaveLength(0)
      await dialog.getByRole('button', { name: 'Update 2 machines', exact: true }).click()
      await confirm.getByRole('button', { name: 'Update 2 machines', exact: true }).click()
      await expect.poll(() => starts.length).toBe(1)
      expect(starts[0]).toMatchObject({ tool: 'opencode', agents: ['a-laptop', 'a-build'] })
      await expect(dialog.getByRole('status').filter({ hasText: 'Updating Laptop…' })).toContainText('0 of 2 done.')
      await capture(page, `fleet-software-working-${width}-${theme}`)
      await expect(dialog.getByRole('status').filter({ hasText: 'Updated 2 machines.' })).toBeVisible({ timeout: 12000 })
      await expect(dialog.getByRole('button', { name: /^Update \d+ machines?$/ })).toHaveCount(0)
      await capture(page, `fleet-software-done-${width}-${theme}`)
      expect(await notOverflowing(page)).toBe(true)
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // A machine that is not connected is skipped, and the sentence says so instead of reporting a failure.
  if (run('skipped')) {
    const world = {
      rows: [laptop(), build()],
      started: batch('running', member(0, 'a-laptop', 'Laptop', 'running'), member(1, 'a-build', 'Build server', 'pending')),
      // By the time the batch ends, the laptop is current and the build server, which was off, is still behind.
      poll: () => {
        world.rows = [plan('a-laptop', 'Laptop', 'none'), build()]
        return batch('done', member(0, 'a-laptop', 'Laptop', 'applied', { version: '1.19.5' }), member(1, 'a-build', 'Build server', 'skipped', { code: 'offline' }))
      },
    }
    const { context, page, starts } = await open({ width: 1280 }, world)
    const dialog = await openModal(page)
    await dialog.getByRole('button', { name: 'Update 2 machines', exact: true }).click()
    await page.getByRole('dialog', { name: 'Update 2 machines?', exact: true }).getByRole('button', { name: 'Update 2 machines', exact: true }).click()
    const status = dialog.getByRole('status').filter({ hasText: 'Updated 1 machine.' })
    await expect(status).toContainText('Build server was skipped.', { timeout: 12000 })
    await expect(dialog.getByText("That machine wasn't connected when its turn came, so nothing was changed on it.")).toBeVisible()
    // The machine that was skipped is still behind, so the same button is there for it, without starting over first.
    await dialog.getByRole('button', { name: 'Update 1 machine', exact: true }).click()
    await page.getByRole('dialog', { name: 'Update 1 machine?', exact: true }).getByRole('button', { name: 'Update 1 machine', exact: true }).click()
    await expect.poll(() => starts.length).toBe(2)
    expect(starts[1]).toMatchObject({ tool: 'opencode', agents: ['a-build'] })
    await context.close()
  }

  // Nothing to press: the sentence says why, with no button, and what explains it is already open.
  if (run('nothing to press')) {
    const current = await open({ width: 1280 }, { rows: [desktop(), plan('a-other', 'Other', 'none')] })
    const dialog = await openModal(current.page)
    await expect(dialog.getByRole('status').filter({ hasText: 'Everything is up to date.' })).toBeVisible()
    await expect(dialog.getByRole('button', { name: /^(Update|Install on) / })).toHaveCount(0)
    await current.context.close()

    const blocked = await open({ width: 1280 }, { rows: [busy(), desktop()] })
    const blockedDialog = await openModal(blocked.page)
    await expect(blockedDialog.getByRole('status').filter({ hasText: "1 machine can't be updated right now." })).toBeVisible()
    await expect(blockedDialog.locator('details.machines')).toHaveAttribute('open', '')
    await expect(blockedDialog.getByText(/OpenCode session is running/).first()).toBeVisible()
    await blocked.context.close()
  }

  // The hand-picked path is still there: choose a machine, review the order, then start. It asks the same question.
  if (run('by hand')) {
    const world = {
      rows: [laptop(), build()],
      started: batch('running', member(0, 'a-build', 'Build server', 'running')),
      poll: () => batch('running', member(0, 'a-build', 'Build server', 'running')),
    }
    const { context, page, starts } = await open({ width: 1280 }, world)
    const dialog = await openModal(page)
    await dialog.getByText('Machines · 2').click()
    await dialog.getByRole('checkbox', { name: 'Include Build server in a batch' }).check()
    await expect(dialog.getByRole('region', { name: 'Batch order' })).toContainText('Build server')
    await dialog.getByRole('button', { name: 'Review batch (1)' }).click()
    const confirm = page.getByRole('dialog', { name: 'Update 1 machine?', exact: true })
    await expect(confirm).toBeVisible()
    await expect(confirm).toContainText('Build server')
    await expect(confirm).not.toContainText('Laptop')
    expect(starts).toHaveLength(0)
    await confirm.getByRole('button', { name: 'Update 1 machine', exact: true }).click()
    await expect(dialog.getByRole('status').filter({ hasText: 'Updating Build server…' })).toBeVisible()
    expect(starts).toHaveLength(1)
    expect(starts[0]).toMatchObject({ tool: 'opencode', agents: ['a-build'] })
    await context.close()
  }

  // A failure stops the rest and says where, with the choices right there.
  if (run('stopped')) {
    const stopped = batch('stopped', member(0, 'a-laptop', 'Laptop', 'applied', { version: '1.19.5' }), member(1, 'a-build', 'Build server', 'failed', { code: 'download_failed' }))
    const world = {
      rows: [laptop(), build()], active: stopped,
      afterSkip: batch('done', member(0, 'a-laptop', 'Laptop', 'applied', { version: '1.19.5' }), member(1, 'a-build', 'Build server', 'skipped')),
    }
    const { context, page } = await open({ width: 1280 }, world)
    const dialog = await openModal(page)
    const status = dialog.getByRole('status').filter({ hasText: 'Stopped on Build server.' })
    await expect(status).toContainText("The machine couldn't download the reviewed artifact. Nothing was changed.")
    await expect(dialog.getByRole('button', { name: 'Retry' })).toBeVisible()
    await dialog.getByRole('button', { name: 'Skip and continue' }).click()
    await expect(dialog.getByRole('status').filter({ hasText: 'Updated 1 machine.' })).toContainText('Build server was skipped.')
    await context.close()
  }

  // A refused start says why in words, in the question that was asked, and leaves it ready to try again.
  if (run('refused')) {
    const world = { rows: [laptop(), build()], refuse: 'That machine is already in another batch.' }
    const { context, page } = await open({ width: 1280 }, world)
    const dialog = await openModal(page)
    await dialog.getByRole('button', { name: 'Update 2 machines', exact: true }).click()
    const confirm = page.getByRole('dialog', { name: 'Update 2 machines?', exact: true })
    await confirm.getByRole('button', { name: 'Update 2 machines', exact: true }).click()
    await expect(confirm.getByRole('alert')).toContainText('That machine is already in another batch.')
    await expect(confirm.getByRole('button', { name: 'Update 2 machines', exact: true })).toBeEnabled()
    // Cancelling clears the message, so it is not still there the next time it is asked.
    await confirm.getByRole('button', { name: 'Cancel', exact: true }).click()
    await dialog.getByRole('button', { name: 'Update 2 machines', exact: true }).click()
    await expect(confirm.getByRole('alert')).toHaveCount(0)
    await context.close()
  }
} finally {
  await browser.close()
}
console.log('Fleet software: one sentence and one button that asks once; skipped, stopped and refused say so; choosing by hand still works.')
