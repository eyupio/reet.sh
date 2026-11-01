import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill } from './helpers/workspace.mjs'
import { mkdir } from 'node:fs/promises'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:5173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now(), minute = 60_000
const caps = { protocolVersion: 1, os: 'linux', osUser: 'sam', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, tools: ['claude', 'codex', 'cline', 'opencode'], agentsV2: true, yolo: true, uploads: true, browse: true, terminalControl: true }
const machines = [{ agentId: 'm1', host: 'Studio', ready: true, serverEnabled: true, capabilities: caps }, { agentId: 'm2', host: 'MacBook Pro', ready: false, serverEnabled: true, capabilities: caps }]
const project = (id, name, dir, host = 'm1') => ({ id, name, host, machine: host === 'm1' ? 'Studio' : 'MacBook Pro', dir, isRepo: true, baseBranch: 'main', worktreeDefault: 'inherit', defaultTool: 'claude', defaultModel: 'sonnet', defaultEffort: '', hasStarterPrompt: false, colour: 'blue', position: 0, createdAt: now, updatedAt: now, version: 1 })
const projects = [project('p1', 'Reet', '/home/sam/reet'), project('p2', 'Storefront', '/home/sam/storefront'), project('p3', 'Personal site', '/Users/sam/site', 'm2')]
const draft = (id, title, prompt, over = {}) => ({ id, title, prompt, agentId: 'm1', projectId: 'p1', dir: '/home/sam/reet', tool: 'claude', model: 'sonnet', effort: '', imageCount: 0, images: [], starting: false, version: 1, createdAt: now - 40 * minute, updatedAt: now - 20 * minute, ...over })
const session = (id, title, over = {}) => ({ id, title, agentId: 'm1', projectId: 'p1', dir: '/home/sam/reet', state: 'running', tool: 'codex', model: 'gpt-5-codex', attention: 'working', createdAt: now - 80 * minute, updatedAt: now - 5 * minute, exitCode: null, ...over })
async function workspace(width = 1440, theme = 'light', touch = false, entry = '/') {
  const box = {
    drafts: [draft('draft-design', 'Give the settings page a proper polish', 'Refine the account settings layout. Focus on spacing, clear labels and a calmer visual hierarchy.', { imageCount: 2 }), draft('draft-tests', 'Cover the session recovery edge cases', 'Add focused coverage for a reconnect after the browser goes offline.', { tool: 'cline', model: 'anthropic/claude-sonnet', effort: 'high', projectId: 'p2', dir: '/home/sam/storefront' }), draft('draft-site', 'Bring the case studies up to date', 'Refresh the project stories and add the new screenshots.', { agentId: 'm2', projectId: 'p3', dir: '/Users/sam/site', tool: 'opencode', model: 'anthropic/claude-sonnet' })],
    sessions: [session('s1', 'Build a faster project picker'), session('s2', 'Review the checkout changes', { attention: 'waiting', tool: 'claude', model: 'opus', projectId: 'p2', dir: '/home/sam/storefront' }), session('s3', 'Tidy up the keyboard shortcuts', { state: 'exited', attention: 'finished', exitCode: 0, updatedAt: now - 35 * minute }), session('s4', 'Document the new launch options', { state: 'ended', attention: 'finished', tool: 'claude', model: 'sonnet', updatedAt: now - 90 * minute })],
    starts: [], saves: [], deletes: [], failList: false,
  }
  const routes = {
    '/api/v2/machines/health': { serverNow: now, machines: machines.map(m => ({ agentId: m.agentId, host: m.host, state: m.ready ? 'connected' : 'unreachable', capabilities: caps })) },
    '/api/v2/terminal/machines': { machines },
    '/api/v2/terminal/sessions': () => ({ sessions: box.sessions }),
    '/api/v2/folder-projects': { projects, savedWaiting: 0 },
    '/api/v2/machines/m1/models': { models: [{ id: 'sonnet', provider: '' }, { id: 'opus', provider: '' }], state: 'listed' },
    '/api/v2/model-favourites': { favourites: [] },
    '/api/v2/terminal/yolo': { allowed: false },
    '/api/v2/settings/yolo': { allowed: true },
    '/api/v2/session-backlog': request => {
      if (request.method() === 'POST') {
        const input = request.postDataJSON(); box.saves.push(input)
        const made = { ...input, id: `draft-new-${box.saves.length}`, imageCount: input.images.length, starting: false, version: 1, createdAt: now, updatedAt: now }
        box.drafts.push(made); register(made.id); return made
      }
      return box.failList ? { status: 500, json: { error: 'unavailable' } } : { drafts: box.drafts.map(d => ({ ...d, images: undefined })) }
    },
  }
  let register = id => {
    routes[`/api/v2/session-backlog/${id}`] = request => {
      const found = box.drafts.find(d => d.id === id)
      if (request.method() === 'DELETE') { box.deletes.push(id); box.drafts = box.drafts.filter(d => d.id !== id); return { ok: true } }
      if (request.method() === 'PUT') { const input = request.postDataJSON(); Object.assign(found, input, { version: found.version + 1, imageCount: input.images.length }); return found }
      return found
    }
    routes[`/api/v2/session-backlog/${id}/start`] = request => {
      box.starts.push({ id, ...request.postDataJSON() }); const found = box.drafts.find(d => d.id === id)
      const started = session(`started-${id}`, found.title, { tool: found.tool, model: found.model, effort: found.effort })
      found.sessionId = started.id; box.sessions.unshift(started); return started
    }
  }
  box.drafts.forEach(d => register(d.id))
  const app = await openWorkspace(browser, { width, height: 1000, theme, touch, terminalMachines: machines, terminalStatus: { configured: true, available: true, reason: '' }, routes,
    skills: [skill('brainstorm'), skill('polish')] })
  await app.page.goto(new URL(entry, base).href)
  await expect(app.page.getByRole('heading', { name: 'A reet good day’s work.' })).toBeVisible()
  await expect(app.page.locator('.session-card')).toHaveCount(7)
  return { ...app, box }
}
try {
  await mkdir('../work/screenshots', { recursive: true })
  // Existing Sessions links and project handoffs must reveal their launch form,
  // while a normal Home visit keeps the board as the primary view.
  for (const [entry, projectLabel, focused, width] of [
    ['/', '', false, 1440],
    ['/#launch', '', true, 1440],
    ['/?project=p2', 'Storefront', false, 1440],
    ['/?projects=p1,p2', '2 projects', false, 1440],
    ['/?projects=p1,p2#launch', '2 projects', true, 1440],
    ['/#launch', '', true, 390],
  ]) {
    const app = await workspace(width, 'light', width === 390, entry)
    const section = app.page.locator('details.quick-launch')
    await expect(section).toHaveJSProperty('open', entry !== '/')
    if (entry !== '/') {
      const launch = app.page.getByRole('form', { name: 'Start a session', exact: true })
      await expect(launch).toBeVisible()
      await expect(launch.getByRole('textbox', { name: 'Describe the job', exact: true })).toBeVisible()
      // The board keeps the second column beside the rail, and an open Quick launch stays in the viewport under a long board.
      const column = await app.page.locator('.column').boundingBox()
      const rail = await app.page.locator('aside.rail').boundingBox()
      if (rail) expect(column.x).toBeGreaterThanOrEqual(rail.x + rail.width - 1)
      const launchBox = await launch.boundingBox()
      const view = app.page.viewportSize()
      expect(launchBox.y).toBeGreaterThanOrEqual(0)
      expect(launchBox.y + launchBox.height).toBeLessThanOrEqual(view.height + 1)
      if (focused) {
        await expect(launch.getByRole('textbox', { name: 'Describe the job', exact: true })).toBeFocused()
        await expect.poll(() => new URL(app.page.url()).hash).toBe('')
      }
      if (projectLabel === '2 projects') await expect(launch.locator('.sentence').getByText('2 projects', { exact: true })).toBeVisible()
      else if (projectLabel) await expect(launch.getByRole('button', { name: `Project: ${projectLabel}`, exact: true })).toBeVisible()
    }
    expect(app.errors).toEqual([])
    await app.context.close()
  }
  const app = await workspace(), { page, box } = app
  page.setDefaultTimeout(7000)
  await page.screenshot({ path: '../work/screenshots/session-board-desktop.png', fullPage: true })
  await page.getByRole('button', { name: /^Backlog/ }).click()
  await expect(page.locator('.stage')).toHaveCount(1); await expect(page.locator('.session-card')).toHaveCount(3)
  await page.screenshot({path:'../work/screenshots/session-backlog-focus.png',fullPage:true})
  await page.getByRole('button', { name: /^Active/ }).click(); await expect(page.locator('.session-card')).toHaveCount(2)
  await page.screenshot({path:'../work/screenshots/session-active-focus.png',fullPage:true})
  await page.reload(); await expect(page.getByRole('button',{name:/^Active/})).toHaveAttribute('aria-pressed','true'); await expect(page.locator('.session-card')).toHaveCount(2)
  await page.getByRole('button', { name: /^Finished/ }).click(); await expect(page.locator('.session-card')).toHaveCount(2)
  await page.screenshot({path:'../work/screenshots/session-finished-focus.png',fullPage:true})
  await page.getByRole('button', { name: 'Board', exact: true }).click()
  await page.getByRole('searchbox', { name: 'Search sessions' }).fill('keyboard')
  await expect(page.locator('.session-card')).toHaveCount(1)
  await page.getByRole('button', { name: 'Clear search' }).click()
  await page.getByRole('button', { name: 'Filters', exact: true }).click()
  await page.locator('.filter-panel').getByLabel('Studio', { exact: true }).check()
  await page.locator('.filter-panel').getByLabel('Claude Code', { exact: true }).check()
  await expect(page.locator('.session-card')).toHaveCount(3)
  await page.screenshot({path:'../work/screenshots/session-board-filters.png',fullPage:true})
  await page.keyboard.press('Escape'); await expect(page.locator('.filter-panel')).toHaveCount(0)
  await page.getByRole('button', { name: 'Clear all', exact: true }).click()
  await page.getByRole('button', { name: 'New session', exact: true }).click()
  const composer = page.getByRole('dialog')
  await expect(composer).toBeVisible()
  await page.screenshot({path:'../work/screenshots/session-composer-desktop.png',fullPage:false})
  const promptBox = composer.getByRole('textbox', { name: /^Prompt/ })
  await promptBox.fill('Make session filtering feel effortless')
  await composer.getByLabel('Agent', { exact: true }).selectOption('cline')
  await composer.getByRole('button', { name: 'Model: Agent default', exact: true }).click()
  const modelList = page.getByRole('listbox', { name: 'Model' })
  await expect(modelList.getByRole('option', { name: 'opus', exact: true })).toBeVisible()
  await modelList.getByRole('option', { name: 'opus', exact: true }).click()
  await expect(composer.getByRole('button', { name: 'Model: opus', exact: true })).toBeVisible()
  await composer.getByRole('button', { name: 'Effort: Agent default', exact: true }).click()
  const effortList = page.getByRole('listbox', { name: 'Effort' })
  await effortList.getByRole('option', { name: 'high', exact: true }).click()
  await expect(composer.getByRole('button', { name: 'Effort: high', exact: true })).toBeVisible()
  await composer.getByRole('button', { name: 'Machine & folder', exact: true }).click()
  await composer.getByLabel('Machine', { exact: true }).selectOption('m2')
  await composer.getByLabel('Folder', { exact: false }).fill('/Users/sam/notes')
  await expect(composer.getByText('This machine is offline.', { exact: false })).toBeVisible()
  await composer.getByRole('button', { name: /Use a skill/ }).click()
  const skillSearch = page.getByRole('searchbox', { name: 'Search your skills' })
  await expect(skillSearch).toBeVisible()
  await skillSearch.fill('brain')
  const skillList = page.getByRole('listbox', { name: 'Skill' })
  await expect(skillList.getByRole('option', { name: /brainstorm/ })).toBeVisible()
  await skillList.getByRole('option', { name: /brainstorm/ }).click()
  const withSkill = await promptBox.inputValue()
  expect(withSkill).toContain('npx skills use')
  await promptBox.fill(`${withSkill}\nKeep the tone calm.`)
  await composer.locator('input[type=file]').setInputFiles({name:'reference.png',mimeType:'image/png',buffer:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=','base64')})
  await expect(composer.locator('.image')).toHaveCount(1)
  await composer.getByRole('button', { name: 'Add to backlog' }).click()
  await expect(composer).toHaveCount(0); await expect(page.locator('.session-card')).toHaveCount(8)
  expect(box.saves[0].images).toHaveLength(1)
  expect(box.saves[0]).toMatchObject({ agentId: 'm2', dir: '/Users/sam/notes', effort: 'high', model: 'opus', projectId: '', title: 'Make session filtering feel effortless' })
  expect(box.saves[0].prompt).toContain('npx skills use')
  expect(box.saves[0].prompt).toContain('Keep the tone calm.')
  await page.getByRole('button', { name: 'Edit Make session filtering feel effortless', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.getByRole('dialog').getByLabel('Session title', { exact: false }).fill('Refine the session filters')
  await page.getByRole('button', { name: 'Save changes', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Refine the session filters', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Delete Refine the session filters', exact: true }).click()
  await page.getByRole('button', { name: 'Delete item', exact: true }).click()
  await expect(page.locator('.session-card')).toHaveCount(7)
  const startCard = page.locator('.session-card').filter({ hasText: 'Cover the session recovery edge cases' })
  await startCard.getByRole('button', { name: 'Start', exact: true }).click()
  await expect(page.locator('.active .session-card')).toHaveCount(3)
  await expect(page.locator('.backlog .session-card')).toHaveCount(2)
  expect(box.starts).toHaveLength(1)
  // Launch options travel with a backlog item; YOLO is saved as intent and confirmed only at Start.
  await page.getByRole('button', { name: 'New session', exact: true }).click()
  const options = page.getByRole('dialog')
  await options.getByRole('textbox', { name: /^Prompt/ }).fill('Tighten the checkout copy')
  await options.getByLabel('Agent', { exact: true }).selectOption('codex')
  await expect(options.getByRole('group', { name: 'Approvals' })).toBeVisible()
  await expect(options.getByLabel('Start in a fresh worktree')).toBeVisible()
  await options.getByLabel('Start in a fresh worktree').check()
  await options.getByLabel('Base branch').fill('develop')
  await options.getByRole('button', { name: 'YOLO', exact: true }).click()
  await expect(options.getByRole('note')).toContainText('confirm')
  await options.getByRole('button', { name: 'Add to backlog' }).click()
  await expect(options).toHaveCount(0)
  expect(box.saves.at(-1)).toMatchObject({ tool: 'codex', yolo: true, worktree: true, base: 'develop', chat: false })
  const startsBefore = box.starts.length
  const yoloCard = page.locator('.session-card').filter({ hasText: 'Tighten the checkout copy' })
  await yoloCard.getByRole('button', { name: 'Start', exact: true }).click()
  const confirm = page.getByRole('alertdialog')
  await expect(confirm).toContainText('Start this session on YOLO?')
  expect(box.starts).toHaveLength(startsBefore)
  await confirm.getByRole('button', { name: 'Keep asking me' }).click()
  expect(box.starts).toHaveLength(startsBefore)
  await yoloCard.getByRole('button', { name: 'Start', exact: true }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Start with YOLO' }).click()
  await expect.poll(() => box.starts.length).toBe(startsBefore + 1)
  // An idea is a placeholder: saved without a prompt, never startable until promoted.
  await page.getByRole('button', { name: 'New session', exact: true }).click()
  const ideaForm = page.getByRole('dialog')
  await ideaForm.getByLabel('Just capture an idea').check()
  await ideaForm.getByRole('button', { name: 'Save idea' }).click()
  await expect(ideaForm.getByRole('alert')).toContainText('title or a note')
  await ideaForm.getByLabel('Session title', { exact: false }).fill('Rethink onboarding')
  await ideaForm.getByRole('button', { name: 'Save idea' }).click()
  await expect(ideaForm).toHaveCount(0)
  expect(box.saves.at(-1)).toMatchObject({ idea: true, prompt: '', title: 'Rethink onboarding' })
  const ideaCard = page.locator('.session-card').filter({ hasText: 'Rethink onboarding' })
  await expect(ideaCard.locator('.idea-badge')).toHaveText('Idea')
  await expect(ideaCard.getByRole('button', { name: 'Start', exact: true })).toBeDisabled()
  await expect(ideaCard.locator('.start-note')).toContainText('switch off Idea')
  const startsAfterIdea = box.starts.length
  await page.getByRole('button', { name: 'Edit Rethink onboarding', exact: true }).click()
  const promote = page.getByRole('dialog')
  await promote.getByLabel('Just capture an idea').uncheck()
  await promote.getByRole('textbox', { name: /^Prompt/ }).fill('Sketch the onboarding flow')
  await promote.getByRole('button', { name: 'Save changes', exact: true }).click()
  await expect(promote).toHaveCount(0)
  expect(box.saves.length).toBeGreaterThan(0)
  await expect(ideaCard.locator('.idea-badge')).toHaveCount(0)
  await expect(ideaCard.getByRole('button', { name: 'Start', exact: true })).toBeEnabled()
  expect(box.starts).toHaveLength(startsAfterIdea)
  expect(app.errors).toEqual([])
  await app.context.close()
  for (const [width, theme, touch] of [[1440, 'dark', false], [390, 'light', true], [320, 'light', true], [768, 'light', true]]) {
    const app = await workspace(width, theme, touch)
    await app.page.screenshot({ path: `../work/screenshots/session-board-${width}-${theme}.png`, fullPage: true })
    expect(await app.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    if(touch) expect(await app.page.evaluate(()=>innerWidth)).toBe(width)
    if(width===390) { expect(await app.page.getByRole('searchbox',{name:'Search sessions'}).evaluate(el=>parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(16); await app.page.getByRole('button',{name:/^Active/}).click(); await app.page.screenshot({path:'../work/screenshots/session-mobile-focus.png',fullPage:false}) }
    await app.page.getByRole('button', { name: 'New session', exact: true }).click()
    await expect(app.page.getByRole('dialog')).toBeVisible()
    expect(await app.page.getByRole('dialog').evaluate(el => el.getBoundingClientRect().right <= innerWidth)).toBe(true)
    if(width===390) expect(await app.page.getByRole('dialog').getByRole('textbox',{name:'Prompt',exact:true}).evaluate(el=>parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(16)
    await app.page.screenshot({ path: `../work/screenshots/session-composer-${width}-${theme}.png`, fullPage: false })
    expect(app.errors).toEqual([])
    await app.context.close()
  }
  console.log('Session board: launch links and project handoffs, stage views, combined filters, offline capture, edit, delete, start, light/dark and responsive checks passed.')
} finally { await browser.close() }
