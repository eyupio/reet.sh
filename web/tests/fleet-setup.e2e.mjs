// Copy a setup to other machines, from the Setups page: read one machine, review what each other machine would change,
// choose and order them, confirm once (with the typed word `copy` only when a machine would let agents do more without
// asking), follow the batch, and undo a machine afterwards. The API is answered from fixtures.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace, terminalMachine } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const machines = [terminalMachine('a-studio', 'Studio', true), terminalMachine('a-laptop', 'Laptop', true), terminalMachine('a-build', 'Build server', true), terminalMachine('a-desk', 'Desktop', true), terminalMachine('a-mini', 'Mac mini', true), terminalMachine('a-pi', 'Pi box', true)]
const file = (agent, kind, name, state, extra = {}) => ({ agent, kind, name, state, ...extra })
const machine = (agentId, host, state, extra = {}) => ({ agentId, host, state, widens: false, fingerprint: 'f'.repeat(64), files: [], ...extra })
const settingsMerge = file('claude', 'settings', 'settings.json', 'merge', { fields: ['permissions', 'outputStyle'], widens: ['permissions'] })
const instructionsMerge = file('claude', 'instructions', 'CLAUDE.md', 'merge', { added: ['Always run the tests before you finish.', 'Keep commits small.'], removed: ['Never run the tests.'] })
const laptop = (widens = true) => machine('a-laptop', 'Laptop', 'ready', { widens, files: [widens ? settingsMerge : file('claude', 'settings', 'settings.json', 'merge', { fields: ['outputStyle'] }), instructionsMerge] })
const build = () => machine('a-build', 'Build server', 'ready', { files: [file('claude', 'instructions', 'CLAUDE.md', 'create'), file('codex', 'settings', 'config.toml', 'unchanged'), file('opencode', 'settings', 'opencode.json', 'blocked', { reason: 'destination_symlink' })] })
const desktop = () => machine('a-desk', 'Desktop', 'unchanged', { code: 'unchanged', reason: 'Nothing to change on that machine.', files: [file('claude', 'instructions', 'CLAUDE.md', 'unchanged')] })
const mini = () => machine('a-mini', 'Mac mini', 'excluded', { code: 'not_allowed', reason: 'Setup changes are not allowed.' })
const pi = () => machine('a-pi', 'Pi box', 'excluded', { code: 'offline', reason: "That machine isn't connected." })
const review = (over = {}) => ({
  readId: 'read-1', expiresAt: Date.now() + 15 * 60_000, source: { agentId: 'a-studio', host: 'Studio', readAt: Date.now() },
  items: [
    { agent: 'claude', kind: 'instructions', name: 'CLAUDE.md', format: 'text', label: '/home/me/.claude/CLAUDE.md', bytes: 812 },
    { agent: 'claude', kind: 'settings', name: 'settings.json', format: 'json', label: '/home/me/.claude/settings.json', bytes: 1536 },
    { agent: 'codex', kind: 'settings', name: 'config.toml', format: 'toml', label: '/home/me/.codex/config.toml', bytes: 240 },
  ],
  machines: [laptop(), build(), desktop(), mini(), pi()], ...over,
})
const member = (position, agentId, host, state, extra = {}) => ({ position, agentId, host, fingerprint: 'f'.repeat(64), widens: false, files: [file('claude', 'instructions', 'CLAUDE.md', 'merge')], state, code: '', reason: '', journal: '', undoneAt: 0, ...extra })
const batch = (state, ...members) => ({ id: 'b1', sourceAgentId: 'a-studio', sourceHost: 'Studio', readAt: Date.now() - 60_000, state, widens: false, contentHeld: state !== 'done', createdAt: 1, updatedAt: 1, members })
const applied = (position, agentId, host, extra = {}) => member(position, agentId, host, 'applied', { journal: `j-${agentId}`, ...extra })

async function open(options, world) {
  const requests = { source: [], start: [], steps: [] }
  const fixture = await openWorkspace(browser, {
    ...options,
    terminalMachines: machines,
    routes: {
      '/api/v2/setup/fleet/source': request => {
        requests.source.push(request.postDataJSON())
        return world.sourceRefused ?? world.review ?? review()
      },
      '/api/v2/setup/fleet/batches': request => {
        requests.start.push(request.postDataJSON())
        if (world.startRefused) return world.startRefused
        world.active = world.started
        return { status: 201, json: world.started }
      },
      '/api/v2/setup/fleet/batches/active': () => ({ json: world.active ?? null }),
      '/api/v2/setup/fleet/batches/b1': () => world.poll ? world.poll() : world.active,
      '/api/v2/setup/fleet/batches/b1/abandon': () => { requests.steps.push('abandon'); world.active = world.afterAbandon; return world.afterAbandon },
      '/api/v2/setup/fleet/batches/b1/members/1/retry': () => { requests.steps.push('retry'); return world.afterRetry },
      '/api/v2/setup/fleet/batches/b1/members/1/skip': () => { requests.steps.push('skip'); world.active = world.afterSkip; return world.afterSkip },
      '/api/v2/setup/fleet/batches/b1/members/0/undo': () => { requests.steps.push('undo'); return world.undo() },
    },
  })
  return { ...fixture, ...requests, requests }
}
const openDialog = async (page, shot) => {
  await page.goto(`${base}/profiles`)
  await page.getByRole('button', { name: 'Copy to other machines', exact: true }).waitFor()
  if (shot) await capture(page, shot)
  await page.getByRole('button', { name: 'Copy to other machines', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Copy a setup to other machines' })
  await expect(dialog).toBeVisible()
  return dialog
}
const readSource = async dialog => {
  await dialog.getByRole('combobox', { name: 'Copy from' }).selectOption('a-studio')
  await dialog.getByRole('button', { name: 'Read setup', exact: true }).click()
  await expect(dialog.getByRole('region', { name: 'What would be copied' })).toBeVisible()
}
const capture = async (page, name) => {
  if (!process.env.REET_CAPTURE) return
  await mkdir(process.env.REET_CAPTURE, { recursive: true })
  await page.waitForTimeout(400)
  await page.screenshot({ path: `${process.env.REET_CAPTURE}/${name}.png`, fullPage: false })
}
const notOverflowing = page => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
const shape = (width, theme) => ({ width, height: width === 390 ? 844 : 900, touch: width === 390, theme, scale: 1 })

try {
  // Read the source, then see what would be copied and what each other machine would do. Excluded and unchanged machines say
  // why, a blocked file says why, and the changed instruction lines are shown. Without a widening machine, the question has
  // no typed word and the request carries none.
  if (run('review')) {
    for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
      const world = {
        review: review({ machines: [laptop(false), build(), desktop(), mini(), pi()] }),
        started: batch('running', member(0, 'a-laptop', 'Laptop', 'running'), member(1, 'a-build', 'Build server', 'pending')),
        poll: () => batch('done', applied(0, 'a-laptop', 'Laptop'), applied(1, 'a-build', 'Build server')),
      }
      const { context, page, errors, requests } = await open(shape(width, theme), world)
      const dialog = await openDialog(page, `fleet-setup-page-${width}-${theme}`)
      // It opens on the first thing to do, and says plainly what is never copied and how long the text is held.
      await expect(dialog.getByRole('combobox', { name: 'Copy from' })).toBeFocused()
      await expect(dialog).toContainText('Credentials, MCP servers, model defaults and project files are never copied.')
      await expect(dialog).toContainText('The controller holds the copied text only until the copy finishes, and for 24 hours at most.')
      await readSource(dialog)
      expect(requests.source).toEqual([{ agentId: 'a-studio' }])
      const copied = dialog.getByRole('region', { name: 'What would be copied' })
      await expect(copied).toContainText('Studio')
      await expect(copied.getByText('Claude Code instructions (CLAUDE.md)')).toBeVisible()
      await expect(copied.getByText('812 bytes')).toBeVisible()
      await expect(copied.getByText('1.5 KB')).toBeVisible()
      await expect(copied.getByText('Codex settings (config.toml)')).toBeVisible()
      // What is never copied is a line of its own, not part of a muted paragraph.
      await expect(copied.locator('p.never')).toHaveText('Credentials, MCP servers, model defaults and project files are never copied.')
      // Every other machine has a row with its plan.
      const rows = dialog.getByRole('region', { name: 'Other machines' })
      await expect(rows.locator('li[data-machine="Laptop"]')).toContainText('Ready')
      // Settings are merged field by field; an instruction file's text is replaced.
      await expect(rows.locator('li[data-machine="Laptop"] .chip')).toHaveText(['Merge', 'Replace'])
      await expect(rows.locator('li[data-machine="Build server"] .chip')).toHaveText(['Create', 'Unchanged', 'Blocked'])
      await expect(rows.locator('li[data-machine="Laptop"]')).toContainText('Changes outputStyle in Claude Code settings.json')
      await expect(rows.locator('li[data-machine="Laptop"]')).toContainText('Replaces the text of Claude Code CLAUDE.md.')
      await expect(rows.locator('li[data-machine="Laptop"]')).toContainText('Always run the tests before you finish.')
      await expect(rows.locator('li[data-machine="Laptop"]')).toContainText('Never run the tests.')
      await expect(rows.locator('li[data-machine="Laptop"]')).not.toContainText('do more without asking')
      await expect(rows.locator('li[data-machine="Build server"]')).toContainText('Creates Claude Code CLAUDE.md.')
      await expect(rows.locator('li[data-machine="Build server"]')).toContainText('Codex config.toml is already the same.')
      await expect(rows.locator('li[data-machine="Build server"]')).toContainText('OpenCode opencode.json is left alone: the file there is a symbolic link.')
      await expect(rows.locator('li[data-machine="Desktop"]')).toContainText('Already the same')
      await expect(rows.locator('li[data-machine="Desktop"]')).toContainText('Nothing to change: it already has this setup.')
      await expect(rows.locator('li[data-machine="Mac mini"]')).toContainText('Left out')
      await expect(rows.locator('li[data-machine="Mac mini"]')).toContainText('reet agent allow-apply')
      await expect(rows.locator('li[data-machine="Pi box"]')).toContainText("That machine isn't connected.")
      // Only machines with something to copy can be chosen.
      await expect(rows.getByRole('checkbox')).toHaveCount(2)
      await expect(rows.getByRole('checkbox', { name: 'Laptop' })).toBeChecked()
      // On a phone every control is a 44 px target (a checkbox is reached through its label).
      if (width === 390) {
        const small = await dialog.locator('button, select, input:not([type=checkbox]), summary, .row-head label').evaluateAll(nodes => nodes.filter(node => node.getClientRects().length).map(node => ({ name: (node.textContent || node.getAttribute('aria-label') || node.tagName).trim().slice(0, 30), height: Math.round(node.getBoundingClientRect().height), width: Math.round(node.getBoundingClientRect().width) })).filter(box => box.height < 44 || box.width < 44))
        expect(small).toEqual([])
      }
      // The action bar is pinned: the button is on screen before anything is scrolled, and the checkbox says its state.
      const copyTo = dialog.getByRole('button', { name: 'Copy to 2 machines', exact: true })
      await expect(copyTo).toBeInViewport({ ratio: 1 })
      await expect(rows.getByRole('checkbox', { name: 'Laptop' })).toHaveAccessibleDescription('Ready')
      await capture(page, `fleet-setup-review-${width}-${theme}`)
      await rows.evaluate(element => element.scrollIntoView({ block: 'start' }))
      await capture(page, `fleet-setup-machines-${width}-${theme}`)

      // The order can be changed and a machine left out; the one button names the count.
      // The order list is there only while more than one machine is ticked.
      const order = dialog.getByRole('region', { name: 'Copy order' })
      await expect(order.locator('li').nth(0)).toContainText('Laptop')
      await order.getByRole('button', { name: 'Move Laptop later' }).click()
      await expect(order.locator('li').nth(0)).toContainText('Build server')
      await order.getByRole('button', { name: 'Leave out Build server' }).click()
      await expect(order).toHaveCount(0)
      await expect(dialog.getByRole('button', { name: 'Copy to 1 machine', exact: true })).toBeVisible()
      await rows.getByRole('checkbox', { name: 'Build server' }).check()
      await expect(order.locator('li').nth(1)).toContainText('Build server')
      await order.getByRole('button', { name: 'Move Build server earlier' }).click()
      await expect(order.locator('li').nth(0)).toContainText('Build server')

      await dialog.getByRole('button', { name: 'Copy to 2 machines', exact: true }).click()
      const confirm = page.getByRole('dialog', { name: 'Copy to 2 machines?', exact: true })
      await expect(confirm).toBeVisible()
      await expect(confirm).toContainText('read from Studio')
      await expect(confirm.locator('li').nth(0)).toContainText('Build server')
      await expect(confirm.locator('li').nth(0)).toContainText('Creates Claude Code CLAUDE.md.')
      await expect(confirm.locator('li').nth(1)).toContainText('Laptop')
      await expect(confirm).toContainText('A machine that isn\'t connected is skipped.')
      await expect(confirm.locator('p.never')).toHaveText('Credentials, MCP servers, model defaults and project files are never copied.')
      // Nothing needs typing without a widening machine.
      await expect(confirm.getByRole('textbox')).toHaveCount(0)
      expect(requests.start).toHaveLength(0)
      await capture(page, `fleet-setup-confirm-plain-${width}-${theme}`)
      await confirm.getByRole('button', { name: 'Copy to 2 machines', exact: true }).click()
      await expect.poll(() => requests.start.length).toBe(1)
      expect(requests.start[0]).toMatchObject({ readId: 'read-1', agents: ['a-build', 'a-laptop'] })
      expect(Object.keys(requests.start[0]).sort()).toEqual(['agents', 'readId', 'requestId'])
      expect(requests.start[0].requestId).toMatch(/^[A-Za-z0-9_-]{16,128}$/)
      await expect(dialog.getByRole('status').filter({ hasText: 'Copying to Laptop…' })).toBeVisible()
      await expect(dialog.getByRole('status').filter({ hasText: 'Copied to 2 machines.' })).toBeVisible({ timeout: 12000 })
      expect(await notOverflowing(page)).toBe(true)
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // Ten machines: the one button stays on screen however far the list is scrolled, with its order list above it.
  if (run('ten machines')) {
    for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
      const many = Array.from({ length: 10 }, (_, index) => machine(`a-m${index}`, `Machine ${index + 1}`, 'ready', { widens: index === 3, files: [index === 3 ? settingsMerge : instructionsMerge] }))
      const world = { review: review({ machines: many }) }
      const { context, page, errors } = await open(shape(width, theme), world)
      const dialog = await openDialog(page)
      await readSource(dialog)
      const copyTo = dialog.getByRole('button', { name: 'Copy to 10 machines', exact: true })
      await expect(copyTo).toBeInViewport({ ratio: 1 })
      await dialog.locator('li[data-machine="Machine 10"]').scrollIntoViewIfNeeded()
      await expect(copyTo).toBeInViewport({ ratio: 1 })
      await expect(dialog.getByRole('region', { name: 'Copy order' }).locator('li')).toHaveCount(10)
      if (width === 390) expect((await copyTo.boundingBox()).height).toBeGreaterThanOrEqual(44)
      await dialog.getByRole('region', { name: 'Copy order' }).scrollIntoViewIfNeeded()
      await capture(page, `fleet-setup-review-10-${width}-${theme}`)
      expect(await notOverflowing(page)).toBe(true)
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // A machine that would let agents do more without asking is flagged where it is reviewed, and the copy needs the word typed.
  // The request carries exactly `copy`. Leaving that machine out takes the word away.
  if (run('widening')) {
    for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
      const world = {
        started: batch('running', member(0, 'a-laptop', 'Laptop', 'running', { widens: true }), member(1, 'a-build', 'Build server', 'pending')),
        poll: () => batch('running', member(0, 'a-laptop', 'Laptop', 'running', { widens: true }), member(1, 'a-build', 'Build server', 'pending')),
      }
      const { context, page, errors, requests } = await open(shape(width, theme), world)
      const dialog = await openDialog(page)
      await readSource(dialog)
      const row = dialog.locator('li[data-machine="Laptop"]')
      await expect(row.getByRole('note')).toContainText('This copy lets agents do more without asking on Laptop: Claude Code permissions.')
      // The checkbox says both its state and the warning.
      await expect(row.getByRole('checkbox')).toHaveAccessibleDescription(/Ready\s+This copy lets agents do more without asking on Laptop/)
      await expect(dialog.locator('li[data-machine="Build server"]').getByRole('note')).toHaveCount(0)

      await dialog.getByRole('button', { name: 'Copy to 2 machines', exact: true }).click()
      const confirm = page.getByRole('dialog', { name: 'Copy to 2 machines?', exact: true })
      // The warning is its own bordered block above the machines, naming what the plan says would widen.
      const warning = confirm.locator('.widen.block')
      await expect(warning).toContainText('This copy lets agents do more without asking.')
      await expect(warning).toContainText('Laptop')
      await expect(warning).toContainText('Claude Code settings.json: permissions')
      expect(await warning.evaluate(node => getComputedStyle(node).borderLeftWidth === getComputedStyle(node).borderTopWidth)).toBe(true)
      expect(await warning.evaluate(node => node.compareDocumentPosition(node.parentElement.querySelector('ol')) & Node.DOCUMENT_POSITION_FOLLOWING)).toBeTruthy()
      await expect(confirm.locator('p.never')).toHaveText('Credentials, MCP servers, model defaults and project files are never copied.')
      const go = confirm.getByRole('button', { name: 'Copy to 2 machines', exact: true })
      // The word is still exactly `copy`; its label says whose permissions widen and it points at the warning.
      const word = confirm.getByRole('textbox', { name: 'Type copy to allow wider permissions on Laptop' })
      expect(await word.getAttribute('aria-describedby')).toBe(await warning.getAttribute('id'))
      // The typed word is where focus starts. It is not given away as a placeholder, and its border can be seen.
      await expect(word).toBeFocused()
      expect(await word.getAttribute('placeholder')).toBeNull()
      expect(await word.evaluate(input => {
        const probe = document.createElement('div')
        probe.style.cssText = 'border:1px solid var(--line-deep)'
        document.body.append(probe)
        const expected = getComputedStyle(probe).borderTopColor
        probe.remove()
        // At rest: a focused field turns its border to the accent.
        input.blur()
        const rest = getComputedStyle(input).borderTopColor === expected
        input.focus()
        return rest
      })).toBe(true)
      await expect(go).toBeDisabled()
      await word.fill('cop')
      await word.press('Enter')
      await expect(go).toBeDisabled()
      expect(requests.start).toHaveLength(0)
      await capture(page, `fleet-setup-confirm-word-${width}-${theme}`)
      await word.fill('  Copy ')
      await expect(go).toBeEnabled()
      await word.press('Enter')
      await expect.poll(() => requests.start.length).toBe(1)
      expect(requests.start[0]).toMatchObject({ readId: 'read-1', agents: ['a-laptop', 'a-build'], confirm: 'copy' })
      // Focus goes to the sentence that says what happened, not to a button that is gone.
      await expect(dialog.getByRole('status').filter({ hasText: 'Copying to Laptop…' })).toBeFocused()
      expect(await notOverflowing(page)).toBe(true)
      expect(errors).toEqual([])
      await context.close()
    }

    const left = await open({ width: 1280 }, { started: batch('running', member(0, 'a-build', 'Build server', 'running')) })
    const dialog = await openDialog(left.page)
    await readSource(dialog)
    await dialog.getByRole('region', { name: 'Copy order' }).getByRole('button', { name: 'Leave out Laptop' }).click()
    await expect(dialog.getByRole('region', { name: 'Copy order' })).toHaveCount(0)
    await dialog.getByRole('button', { name: 'Copy to 1 machine', exact: true }).click()
    const confirm = left.page.getByRole('dialog', { name: 'Copy to 1 machine?', exact: true })
    await expect(confirm.getByRole('textbox')).toHaveCount(0)
    await confirm.getByRole('button', { name: 'Copy to 1 machine', exact: true }).click()
    await expect.poll(() => left.requests.start.length).toBe(1)
    expect(left.requests.start[0]).not.toHaveProperty('confirm')
    await left.context.close()
  }

  // Refusals say what happened in plain words. A credential names the file and agent and never shows text; an expired review
  // says to read again and leaves nothing to confirm.
  if (run('refusals')) {
    const refusals = [
      [{ status: 422, json: { error: "reet won't copy CLAUDE.md (claude): it looks like it holds a credential. Remove it on that machine and read it again.", code: 'setup_credential' } }, "reet won't copy CLAUDE.md (claude): it looks like it holds a credential."],
      [{ status: 413, json: { error: 'raw', code: 'setup_too_large' } }, 'about 60 KB'],
      [{ status: 409, json: { error: 'raw', code: 'setup_unsupported' } }, 'needs a newer reet'],
      [{ status: 409, json: { error: 'raw', code: 'setup_nothing' } }, 'no global instruction files or settings'],
      [{ status: 504, json: { error: 'raw', code: 'setup_timed_out' } }, 'took too long'],
    ]
    for (const [sourceRefused, sentence] of refusals) {
      const { context, page, errors } = await open({ width: 1280 }, { sourceRefused })
      const dialog = await openDialog(page)
      await dialog.getByRole('button', { name: 'Read setup', exact: true }).click()
      await expect(dialog.getByRole('alert')).toContainText(sentence)
      await expect(dialog.getByRole('region', { name: 'What would be copied' })).toHaveCount(0)
      await expect(dialog.getByRole('button', { name: 'Read setup', exact: true })).toBeEnabled()
      expect(errors).toEqual([])
      await context.close()
    }

    const world = { startRefused: { status: 410, json: { error: 'raw', code: 'setup_read_expired', field: 'readId' } } }
    const { context, page } = await open({ width: 1280 }, world)
    const dialog = await openDialog(page)
    await readSource(dialog)
    await dialog.getByRole('button', { name: 'Copy to 2 machines', exact: true }).click()
    await page.getByRole('dialog', { name: 'Copy to 2 machines?', exact: true }).getByRole('textbox').fill('copy')
    await page.keyboard.press('Enter')
    await expect(dialog.getByRole('alert')).toContainText('That review has expired. Read the source machine again.')
    await expect(page.getByRole('dialog', { name: 'Copy to 2 machines?', exact: true })).toBeHidden()
    await expect(dialog.getByRole('region', { name: 'What would be copied' })).toHaveCount(0)
    await context.close()
  }

  // A failed machine stops the rest, with Retry, Skip and continue and Abandon right there, in words about what happened.
  if (run('stopped')) {
    for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
      const stopped = batch('stopped', applied(0, 'a-laptop', 'Laptop'), member(1, 'a-build', 'Build server', 'failed', { code: 'write_failed' }), member(2, 'a-mini', 'Mac mini', 'pending'))
      const world = {
        active: stopped,
        afterRetry: batch('running', applied(0, 'a-laptop', 'Laptop'), member(1, 'a-build', 'Build server', 'running'), member(2, 'a-mini', 'Mac mini', 'pending')),
        afterSkip: batch('done', applied(0, 'a-laptop', 'Laptop'), member(1, 'a-build', 'Build server', 'skipped', { code: 'write_failed' }), applied(2, 'a-mini', 'Mac mini')),
        afterAbandon: batch('abandoned', applied(0, 'a-laptop', 'Laptop'), member(1, 'a-build', 'Build server', 'failed', { code: 'write_failed' }), member(2, 'a-mini', 'Mac mini', 'skipped', { code: 'abandoned' })),
      }
      const { context, page, errors, requests } = await open(shape(width, theme), world)
      // An active batch is offered on the page itself, and opening it restores the progress.
      await page.goto(`${base}/profiles`)
      await expect(page.getByRole('status').filter({ hasText: 'A setup copy is in progress.' })).toBeVisible()
      await page.getByRole('button', { name: 'Open it', exact: true }).click()
      const dialog = page.getByRole('dialog', { name: 'Copy a setup to other machines' })
      const status = dialog.getByRole('status').filter({ hasText: 'Stopped on Build server.' })
      // The status is the headline alone; the rest of the block sits beside it.
      await expect(status).toHaveText('Stopped on Build server.')
      await expect(dialog.locator('.overview')).toContainText('Retry Build server, skip it and carry on, or abandon the copy.')
      await expect(dialog.locator('li[data-machine="Build server"]')).toContainText("That machine couldn't write a file. It put back what it had already changed.")
      await expect(dialog.locator('.overview')).toContainText('From Studio, read')
      // Nothing to read while a copy is waiting on a decision.
      await expect(dialog.getByRole('button', { name: 'Read setup' })).toHaveCount(0)
      await expect(dialog.getByRole('button', { name: 'Retry', exact: true })).toBeVisible()
      await expect(dialog.getByRole('button', { name: 'Abandon copy', exact: true })).toBeEnabled()
      await expect(dialog.locator('li[data-machine="Laptop"]')).toContainText('Copied')
      await capture(page, `fleet-setup-stopped-${width}-${theme}`)
      expect(await notOverflowing(page)).toBe(true)
      if (width === 1280 && theme === 'light') {
        await dialog.getByRole('button', { name: 'Retry', exact: true }).click()
        await expect(dialog.getByRole('status').filter({ hasText: 'Copying to Build server…' })).toBeFocused()
        expect(requests.steps).toEqual(['retry'])
      } else if (theme === 'light') {
        await dialog.getByRole('button', { name: 'Skip and continue', exact: true }).click()
        await expect(dialog.getByRole('status').filter({ hasText: 'Copied to 2 machines.' })).toBeFocused()
        await expect(dialog.locator('.overview')).toContainText('Build server was skipped.')
        expect(requests.steps).toEqual(['skip'])
      } else {
        await dialog.getByRole('button', { name: 'Abandon copy', exact: true }).click()
        await expect(dialog.getByRole('status').filter({ hasText: 'The copy was abandoned.' })).toBeFocused()
        await expect(dialog.getByRole('button', { name: 'Start another copy' })).toBeVisible()
        // The button is not part of the live region.
        await expect(dialog.getByRole('status').filter({ hasText: 'Start another copy' })).toHaveCount(0)
        expect(requests.steps).toEqual(['abandon'])
      }
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // Each finished machine can be undone, after the question is answered plainly. A file that changed since, or a copy already
  // undone, is said in words and changes nothing. A finished copy is still there after a reload.
  if (run('undo')) {
    for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
      const done = batch('done', applied(0, 'a-laptop', 'Laptop'), applied(1, 'a-build', 'Build server'))
      const undone = batch('done', applied(0, 'a-laptop', 'Laptop', { undoneAt: Date.now() }), applied(1, 'a-build', 'Build server'))
      const world = { review: review(), started: batch('running', member(0, 'a-laptop', 'Laptop', 'running'), member(1, 'a-build', 'Build server', 'pending')), poll: () => done }
      let attempt = 0
      world.undo = () => {
        attempt += 1
        if (attempt === 1) return { status: 409, json: { error: 'raw', code: 'setup_changed' } }
        return undone
      }
      const { context, page, errors, requests } = await open(shape(width, theme), world)
      const dialog = await openDialog(page)
      await readSource(dialog)
      await dialog.getByRole('button', { name: 'Copy to 2 machines', exact: true }).click()
      const confirm = page.getByRole('dialog', { name: 'Copy to 2 machines?', exact: true })
      await confirm.getByRole('textbox', { name: 'Type copy to allow wider permissions on Laptop' }).fill('copy')
      await confirm.getByRole('button', { name: 'Copy to 2 machines', exact: true }).click()
      await expect(dialog.getByRole('status').filter({ hasText: 'Copied to 2 machines.' })).toBeVisible({ timeout: 12000 })
      await expect(dialog.getByRole('button', { name: 'Undo copy on Laptop' })).toBeVisible()
      await expect(dialog.getByRole('button', { name: 'Undo copy on Build server' })).toBeVisible()

      await dialog.getByRole('button', { name: 'Undo copy on Laptop' }).click()
      const ask = page.getByRole('dialog', { name: 'Undo the copy on Laptop?', exact: true })
      await expect(ask).toContainText('reet puts back the files it changed on Laptop')
      expect(requests.steps).toEqual([])
      // A file changed since the copy: the sentence says nothing was changed, and the question stays to try again or cancel.
      await ask.getByRole('button', { name: 'Undo copy', exact: true }).click()
      await expect(ask.getByRole('alert')).toContainText('A file on that machine changed after the copy, so reet left it alone. Nothing was changed.')
      await ask.getByRole('button', { name: 'Undo copy', exact: true }).click()
      await expect(ask).toBeHidden()
      expect(requests.steps).toEqual(['undo', 'undo'])
      await expect(dialog.locator('li[data-machine="Laptop"]')).toContainText('Undone')
      await expect(dialog.getByRole('button', { name: 'Undo copy on Laptop' })).toHaveCount(0)
      await expect(dialog.getByRole('button', { name: 'Undo copy on Build server' })).toBeVisible()
      await expect(dialog.getByRole('status').filter({ hasText: 'Copied to 1 machine.' })).toBeFocused()
      await expect(dialog.locator('.overview')).toContainText('1 undone.')
      // An undone machine says so and no longer says what the copy changed.
      await expect(dialog.locator('li[data-machine="Laptop"]')).toContainText('Put back as it was.')
      await expect(dialog.locator('li[data-machine="Laptop"]')).not.toContainText('Replaces the text')
      await expect(dialog.locator('li[data-machine="Build server"]')).toContainText('Replaces the text')
      await capture(page, `fleet-setup-undone-${width}-${theme}`)
      expect(await notOverflowing(page)).toBe(true)

      expect(errors).toEqual([])
      await context.close()
    }
  }

  // Once every copied machine is put back, the headline stops reading as a success.
  if (run('all undone')) {
    for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
      const undone = batch('done', applied(0, 'a-laptop', 'Laptop', { undoneAt: 5 }), applied(1, 'a-build', 'Build server', { undoneAt: 6 }))
      const { context, page, errors } = await open(shape(width, theme), { active: null, poll: () => undone })
      await page.addInitScript(() => localStorage.setItem('reet-fleet-setup-last', 'b1'))
      const dialog = await openDialog(page)
      const status = dialog.getByRole('status').filter({ hasText: 'Copy undone on 2 machines.' })
      await expect(status).toBeVisible()
      await expect(dialog.locator('.overview')).toHaveAttribute('data-state', 'undone')
      await expect(dialog.getByText('Put back as it was.')).toHaveCount(2)
      await expect(dialog.locator('li[data-machine="Laptop"]')).not.toContainText('Replaces the text')
      await expect(dialog.getByRole('button', { name: /^Undo copy/ })).toHaveCount(0)
      expect(await status.evaluate(node => getComputedStyle(node).color)).toBe(await page.evaluate(() => { const probe = document.createElement('div'); probe.style.color = 'var(--muted)'; document.body.append(probe); const color = getComputedStyle(probe).color; probe.remove(); return color }))
      await capture(page, `fleet-setup-undone-all-${width}-${theme}`)
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // A finished copy is remembered by this browser, so its machines can still be undone after the page is reloaded.
  if (run('remembered')) {
    const done = batch('done', applied(0, 'a-laptop', 'Laptop'), applied(1, 'a-build', 'Build server'))
    const world = {
      review: review(), started: batch('running', member(0, 'a-laptop', 'Laptop', 'running'), member(1, 'a-build', 'Build server', 'pending')),
      // Once it has finished, the controller no longer holds it as the active copy.
      poll: () => { world.active = null; return done },
      undo: () => ({ status: 409, json: { error: 'raw', code: 'setup_undo_done' } }),
    }
    const { context, page, errors } = await open({ width: 1280 }, world)
    let dialog = await openDialog(page)
    await readSource(dialog)
    await dialog.getByRole('button', { name: 'Copy to 2 machines', exact: true }).click()
    await page.getByRole('dialog', { name: 'Copy to 2 machines?', exact: true }).getByRole('textbox').fill('copy')
    await page.getByRole('dialog', { name: 'Copy to 2 machines?', exact: true }).getByRole('button', { name: 'Copy to 2 machines', exact: true }).click()
    await expect(dialog.getByRole('status').filter({ hasText: 'Copied to 2 machines.' })).toBeVisible({ timeout: 12000 })
    await page.reload()
    dialog = await openDialog(page)
    await expect(dialog.getByRole('status').filter({ hasText: 'Copied to 2 machines.' })).toBeVisible()
    await dialog.getByRole('button', { name: 'Undo copy on Laptop' }).click()
    const ask = page.getByRole('dialog', { name: 'Undo the copy on Laptop?', exact: true })
    await ask.getByRole('button', { name: 'Undo copy', exact: true }).click()
    await expect(ask.getByRole('alert')).toContainText('That copy was already undone.')
    await ask.getByRole('button', { name: 'Cancel', exact: true }).click()
    await dialog.getByRole('button', { name: 'Start another copy' }).click()
    await expect(dialog.getByRole('button', { name: 'Read setup', exact: true })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }
} finally {
  await browser.close()
}
console.log('Fleet setup copy: read and review each machine (create, merge, unchanged, blocked, excluded); widening needs the typed word and plain copies none; refusals, stop with Retry, Skip and Abandon, and Undo say what happened.')
