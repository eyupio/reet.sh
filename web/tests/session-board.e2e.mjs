import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
import { mkdir } from 'node:fs/promises'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:5173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now(), minute = 60_000
const caps = { protocolVersion: 1, os: 'linux', osUser: 'sam', terminalEnabled: true, healthEnabled: true, readiness: 'ready', launch: true, tools: ['claude', 'codex', 'cline', 'opencode'], agentsV2: true, uploads: true, browse: true, terminalControl: true }
const machines = [{ agentId: 'm1', host: 'Studio', ready: true, serverEnabled: true, capabilities: caps }, { agentId: 'm2', host: 'MacBook Pro', ready: false, serverEnabled: true, capabilities: caps }]
const project = (id, name, dir, host = 'm1') => ({ id, name, host, machine: host === 'm1' ? 'Studio' : 'MacBook Pro', dir, isRepo: true, baseBranch: 'main', worktreeDefault: 'inherit', defaultTool: 'claude', defaultModel: 'sonnet', defaultEffort: '', hasStarterPrompt: false, colour: 'blue', position: 0, createdAt: now, updatedAt: now, version: 1 })
const projects = [project('p1', 'Reet', '/home/sam/reet'), project('p2', 'Storefront', '/home/sam/storefront'), project('p3', 'Personal site', '/Users/sam/site', 'm2')]
const draft = (id, title, prompt, over = {}) => ({ id, title, prompt, agentId: 'm1', projectId: 'p1', dir: '/home/sam/reet', tool: 'claude', model: 'sonnet', effort: '', imageCount: 0, images: [], starting: false, version: 1, createdAt: now - 40 * minute, updatedAt: now - 20 * minute, ...over })
const session = (id, title, over = {}) => ({ id, title, agentId: 'm1', projectId: 'p1', dir: '/home/sam/reet', state: 'running', tool: 'codex', model: 'gpt-5-codex', attention: 'working', createdAt: now - 80 * minute, updatedAt: now - 5 * minute, exitCode: null, ...over })
async function workspace(width = 1440, theme = 'light', touch = false) {
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
  const app = await openWorkspace(browser, { width, height: 1000, theme, touch, terminalMachines: machines, terminalStatus: { configured: true, available: true, reason: '' }, routes })
  await app.page.goto(base)
  await expect(app.page.getByRole('heading', { name: 'A reet good day’s work.' })).toBeVisible()
  await expect(app.page.locator('.session-card')).toHaveCount(7)
  return { ...app, box }
}
try {
  await mkdir('../work/screenshots', { recursive: true })
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
  await composer.getByRole('textbox', { name: /^Prompt/ }).fill('Make session filtering feel effortless')
  await composer.getByRole('button', { name: 'Machine & folder', exact: true }).click()
  await composer.getByLabel('Machine', { exact: true }).selectOption('m2')
  await composer.getByLabel('Folder', { exact: false }).fill('/Users/sam/notes')
  await expect(composer.getByText('This machine is offline.', { exact: false })).toBeVisible()
  await composer.getByLabel('Agent', { exact: true }).selectOption('cline')
  await composer.getByLabel('Effort', { exact: true }).selectOption('high')
  await composer.locator('input[type=file]').setInputFiles({name:'reference.png',mimeType:'image/png',buffer:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=','base64')})
  await expect(composer.locator('.image')).toHaveCount(1)
  await composer.getByRole('button', { name: 'Add to backlog' }).click()
  await expect(composer).toHaveCount(0); await expect(page.locator('.session-card')).toHaveCount(8)
  expect(box.saves[0].images).toHaveLength(1)
  expect(box.saves[0]).toMatchObject({ agentId: 'm2', dir: '/Users/sam/notes', effort: 'high', projectId: '', title: 'Make session filtering feel effortless' })
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
  console.log('Session board: stage views, combined filters, offline capture, edit, delete, start, light/dark and responsive checks passed.')
} finally { await browser.close() }
