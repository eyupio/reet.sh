// Pressing Sync all machines is the whole job. A skill that cannot come across is one calm note, never a wall of
// red errors, and no internal error code reaches the page. This is the account from the bug report: a dozen skills
// left out, a copy that failed twice, copies that failed to save, names shared by two sources, and three machines
// (one offline, one online, one with automatic sync off).
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace, skill } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now()
const full = n => `${n} cannot be synced privately: private skill storage is full of skills still in use (at most 128 skills or 64 MiB per account). Delete a written skill you no longer need, then sync again.`
const written = ['reet-api-design', 'reet-architect', 'reet-babysit', 'reet-cost-optimisation', 'reet-mcp-design', 'reet-observability-and-reliability', 'reet-performance', 'reet-product-design'].map(name => ({ id: `hosted/reet.example/sam/${name}`, name, slug: name, source: '', sourceType: 'local', problem: full(name) }))
const seo = { id: 'local/seo', name: 'seo', slug: 'seo', source: '', sourceType: 'local', problem: 'This skill folder could not be copied from its machine; it may contain unsupported files, or be larger than 256 files or 8 MiB.' }
const ordinary = Array.from({ length: 24 }, (_, i) => skill(`skill-${i}`))
const library = [...ordinary, ...written, seo]
const copyRequest = (name, state, error, createdAt) => ({ id: `${name}-${createdAt}`, requestKey: `k${createdAt}`, agentId: 'a2', environmentId: 'prod', host: 'PROD_Snicket', agent: 'agents', project: '', name, syncSkillId: `local/${name}`, destinationId: 'prod', state, ...(error ? { error } : {}), bundleId: '', expectedBase: '', createdAt, updatedAt: createdAt, expiresAt: createdAt + 1, excluded: [] })
const failedCopy = (name, error, createdAt) => copyRequest(name, 'failed', error, createdAt)
const imports = [failedCopy('seo', 'capture_failed', 1), failedCopy('seo', 'capture_failed', 2), failedCopy('reet-ux-ui', 'transfer_failed', 3), failedCopy('reet-security', 'transfer_failed', 4), failedCopy('reet-product-design', 'transfer_failed', 5)]
const scope = { id: 'prod', host: 'PROD_Snicket', agent: 'agents', name: 'PROD_Snicket', skills: [], skillCount: 0, complete: true, scannedAt: now, lastSeen: now }
const sync = { environments: [scope], machines: [{ host: 'PROD_Snicket', groups: [scope], skillCount: 0 }], agents: [
  { id: 'a1', host: 'Elitebook', version: '1.18.0', lastSeen: now - 15 * 3600_000, paused: false, autoUpdate: true, keepSynced: true },
  { id: 'a2', host: 'PROD_Snicket', version: '1.19.5', lastSeen: now, paused: false, autoUpdate: true, keepSynced: true },
  { id: 'a3', host: 'jnnngs-ThinkPad-X1-Carbon-Gen-11', version: '1.19.5', lastSeen: now - 15 * 3600_000, paused: false, autoUpdate: true, keepSynced: false },
], targets: [{ target: scope, sourceId: 'library', sourceName: 'Shared library', mode: 'additive', desired: ordinary, missing: [], extra: [], revision: 'r', appliedRevision: 'r', followSource: true, updatedAt: now }], jobs: [], runs: [] }
const duplicates = ['improve-animations', 'improve', 'setup-matt-pocock-skills'].map(slug => ({ slug, keep: skill(slug), remove: [skill(slug)] }))
try {
  for (const width of [390, 1280]) for (const theme of ['light', 'dark']) {
    const fixture = await openWorkspace(browser, { width, height: width === 390 ? 812 : 960, touch: width === 390, theme, skills: ordinary, sync, routes: {
      '/api/v2/sync': () => sync,
      '/api/v2/sync/candidates': () => ({ skills: library, duplicates }),
      '/api/v2/skill-imports': () => ({ requests: imports, capabilities: [] }),
    } })
    const { page, errors } = fixture
    await page.goto(`${base}/sync`)
    const section = page.getByRole('region', { name: 'Your skills, everywhere.' })
    await expect(section.getByRole('button', { name: 'Sync all machines', exact: true })).toBeEnabled()

    // One calm region, not a banner per skill.
    const leftOut = section.locator('.left-out')
    await expect(leftOut).toHaveCount(1)
    await expect(leftOut).toContainText('9 skills are left out for now. Everything else is syncing.')
    await expect(section.locator('.message.error')).toHaveCount(0)
    // Eight skills share one reason and one has its own, so it is two lines and starts open. Each is said once.
    await expect(leftOut.locator('details')).toHaveAttribute('open', '')
    await expect(leftOut.locator('li')).toHaveCount(2)
    const storage = leftOut.locator('li', { hasText: 'reet-api-design' })
    await expect(storage).toContainText('reet-architect')
    await expect(storage).toContainText('reet-product-design')
    await expect(storage).toContainText('private skill storage is full of skills still in use')
    expect(await leftOut.innerText()).not.toMatch(/(cannot be synced privately[^\n]*){2}/)
    // The name is not said twice in a row, as in "reet-api-design: reet-api-design cannot be synced".
    expect(await storage.innerText()).not.toMatch(/reet-api-design[:\s]+reet-api-design/)
    const copyFailure = leftOut.locator('li', { hasText: 'seo' })
    await expect(copyFailure).toHaveCount(1)
    await expect(copyFailure).toContainText('could not be copied from its machine')

    // No internal code, and no "View private copy" row per failed copy.
    const text = await section.innerText()
    expect(text).not.toMatch(/capture_failed|transfer_failed/)
    await expect(section.getByRole('button', { name: 'View private copy' })).toHaveCount(0)

    // Machines: each says what is true, without an error.
    const machines = section.locator('.machine-list')
    await expect(machines.locator('article', { hasText: 'Elitebook' })).toContainText('Offline · library sync resumes when this machine reconnects')
    await expect(machines.locator('article', { hasText: 'Elitebook' })).toContainText('Waiting for machine')
    await expect(machines.locator('article', { hasText: 'jnnngs-ThinkPad' })).toContainText('Sync all machines to enable')
    await expect(machines.locator('article', { hasText: 'PROD_Snicket' })).not.toContainText('needs attention')

    // Shared names are information, inside the library list.
    await section.locator('summary').filter({ hasText: 'In your library' }).click()
    await expect(section.getByText('3 names are on your shelf more than once', { exact: false })).toBeVisible()
    await expect(section.getByText('improve-animations, improve, setup-matt-pocock-skills')).toBeVisible()

    // No page-wide horizontal scroll on a phone.
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
    expect(overflow, `horizontal overflow at ${width}px`).toBeLessThanOrEqual(0)
    expect(errors).toEqual([])
    if (process.env.REET_CAPTURE) {
      await mkdir(process.env.REET_CAPTURE, { recursive: true })
      await leftOut.scrollIntoViewIfNeeded()
      await page.screenshot({ path: `${process.env.REET_CAPTURE}/sync-left-out-${width}-${theme}.png`, fullPage: true })
    }
    await fixture.context.close()
  }
  // A copy that fails while the card is open is reported without a reload. The library list is not polled, so the
  // card has to notice that a copy changed state and read the library again. Without that, the failed copy would drop
  // out of the progress line and its reason would not appear until the page was reloaded.
  {
    let phase = 'copying'
    const waiting = { id: 'local/seo', name: 'seo', slug: 'seo', source: '', sourceType: 'local' }
    const fixture = await openWorkspace(browser, { width: 1280, height: 960, skills: ordinary, sync, routes: {
      '/api/v2/sync': () => sync,
      '/api/v2/sync/candidates': () => ({ skills: [...ordinary, phase === 'copying' ? waiting : seo], duplicates: [] }),
      '/api/v2/skill-imports': () => ({ requests: [phase === 'copying' ? copyRequest('seo', 'previewing', '', 1) : copyRequest('seo', 'failed', 'capture_failed', 1)], capabilities: [] }),
    } })
    const { page, errors } = fixture
    await page.goto(`${base}/sync`)
    const section = page.getByRole('region', { name: 'Your skills, everywhere.' })
    await expect(section.getByText('Copying seo privately from your machines.', { exact: false })).toBeVisible()
    await expect(section.locator('.left-out')).toHaveCount(0)
    phase = 'failed'
    // The copies are read every five seconds, and the library is read again when one changes.
    await expect(section.locator('.left-out')).toContainText('seo', { timeout: 15000 })
    await expect(section.locator('.left-out')).toContainText('could not be copied from its machine')
    await expect(section.getByText('Copying seo privately', { exact: false })).toHaveCount(0)
    expect(await section.innerText()).not.toMatch(/capture_failed/)
    expect(errors).toEqual([])
    await fixture.context.close()
  }
  console.log('Sync left out: one calm note for skills that cannot sync, no raw codes, no per-skill red, shared names explained, machines honest.')
} finally { await browser.close() }
