// Pruning merged local branches from the Machines page: the project's menu opens a plan, the warning says what will go and
// what is kept and why, and the prune deletes exactly the branches shown. Synthetic API answers; nothing runs on a machine.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const OID = 'a'.repeat(40)
const caps = { protocolVersion: 1, os: 'linux', osUser: 'dev', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready', launch: true, worktree: true, branches: true, branchPrune: true, tools: ['claude'] }
const project = { id: 'fp1', host: 'a1', machine: 'studio', dir: '/home/dev/site', name: 'Site', isRepo: true, baseBranch: '', worktreeDefault: 'inherit', defaultTool: '', defaultModel: '', hasStarterPrompt: false, note: '', position: 0, createdAt: 1, updatedAt: 1, version: 1 }
const plan = { current: 'main', default: 'main', base: 'main', more: false, branches: [{ name: 'old', oid: OID, skip: '' }, { name: 'wip', oid: OID, skip: 'not_merged' }] }

const box = { prune: null }
const routes = {
  '/api/v2/terminal/machines': { machines: [{ agentId: 'a1', host: 'studio', ready: true, serverEnabled: true, capabilities: caps }] },
  '/api/v2/terminal/sessions': { sessions: [] },
  '/api/v2/folder-projects': { projects: [project], savedWaiting: 0 },
  '/api/v2/machines/a1/branches/prune': request => {
    if (request.method() === 'GET') return plan
    box.prune = request.postDataJSON()
    return { deleted: ['old'], skipped: [] }
  },
}
try {
  const app = await openWorkspace(browser, { width: 1360, height: 900, routes })
  await app.page.goto(`${base}/machines`)
  await app.page.locator('details.projects-drawer > summary').click()
  const bar = app.page.locator('section.bar')
  await expect(bar).toBeVisible()
  await bar.getByRole('button', { name: 'Actions for Site' }).click()
  await app.page.getByRole('menuitem', { name: 'Prune merged branches…' }).click()

  const dialog = app.page.getByRole('dialog', { name: 'Prune merged branches in Site' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByText('This deletes 1 local branch.', { exact: false })).toBeVisible()
  await expect(dialog.getByText("reet never deletes unmerged work")).toBeVisible()
  const willGo = dialog.getByRole('list', { name: 'Branches to delete' })
  await expect(willGo.getByText('old')).toBeVisible()
  const kept = dialog.getByRole('list', { name: 'Branches kept' })
  await expect(kept.getByText('wip')).toBeVisible()
  await expect(kept.getByText("its work isn't in main")).toBeVisible()

  await dialog.getByRole('button', { name: 'Delete 1 branch' }).click()
  await expect(dialog.getByText('Deleted 1 branch.', { exact: false })).toBeVisible()
  expect(box.prune).toEqual({ dir: '/home/dev/site', base: 'main', branches: [{ name: 'old', oid: OID }] })
  expect(app.errors).toEqual([])
  await app.context.close()
  console.log('Branch prune: the project menu plans, the warning and kept list say what goes and why, and the prune deletes exactly the branch shown.')
} finally {
  await browser.close()
}
