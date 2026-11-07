// The terminal workspace holds one skeleton from the boot shell to the live screen. Its card, toolbar, sessions panel,
// session area and details column must keep the widths the real workspace takes, on a phone, a tablet and wide desktops,
// for a named session and for a bare workspace.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, terminalMachine, active } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const now = Date.now(), agent = 'a1', sessionId = 's1'
const machine = terminalMachine(agent, 'studio', true)
const session = { id: sessionId, agentId: agent, title: 'Build workspace', state: 'running', createdAt: now - 3600000, updatedAt: now, exitCode: null, structured: false, dir: '/home/dev/reet', projectId: 'p1' }
const routes = {
  '/api/v2/terminal/machines': { machines: [machine] },
  '/api/v2/terminal/sessions': { sessions: [session] },
  '/api/v2/terminal/status': active,
  '/api/v2/folder-projects': { projects: [], savedWaiting: 0 },
  '/api/v2/projects': { projects: [] },
  '/api/v2/yolo': { allowed: false },
  [`/api/v2/terminal/sessions/${sessionId}/seen`]: {},
  [`/api/v2/terminal/sessions/${sessionId}/skills`]: { skills: [], machineSeen: now, host: 'studio' },
  [`/api/v2/terminal/sessions/${sessionId}/inspect`]: { repo: true, branch: 'main', changed: 0, conflicts: 0, ahead: 0, behind: 0, checkedAt: now, files: [] },
}
const boxOf = async locator => (await locator.isVisible().catch(() => false) ? locator.boundingBox() : null)
const sameBox = (actual, expected, label) => {
  expect(actual, `${label} missing`).not.toBeNull()
  expect(Math.abs(actual.x - expected.x), `${label} moved horizontally`).toBeLessThanOrEqual(0.6)
  expect(Math.abs(actual.width - expected.width), `${label} changed width`).toBeLessThanOrEqual(0.6)
}
try {
  for (const width of [390, 834, 1200, 1440, 1600]) for (const bare of [false, true]) {
    const path = bare ? '/sessions' : `/sessions?agent=${agent}&session=${sessionId}`
    let release
    const dataGate = new Promise(resolve => (release = resolve))
    const { context, page, errors } = await openWorkspace(browser, {
      width, height: 900, touch: width < 800, routes,
      gates: { '/api/v2/terminal/machines': dataGate, '/api/v2/terminal/sessions': dataGate },
    })
    try {
      await page.goto(base + path)
      const skeleton = page.locator('.workspace-skeleton')
      await expect(skeleton).toBeVisible()
      await page.waitForTimeout(300)
      const sk = {
        card: await boxOf(skeleton),
        toolbar: await boxOf(skeleton.locator('.toolbar')),
        panel: await boxOf(skeleton.locator('.panel')),
        stage: await boxOf(skeleton.locator('.stage')),
        details: await boxOf(skeleton.locator('.details')),
      }
      release()
      if (bare) await expect(page.locator('.workspace .empty').first()).toBeVisible()
      else await expect(page.locator('.session-screen')).toBeVisible()
      const card = await page.locator('.workspace').boundingBox()
      const toolbar = await boxOf(page.locator('.workspace-toolbar'))
      const panel = await boxOf(page.locator('#terminal-sessions'))
      const stage = bare ? await boxOf(page.locator('.terminal-stage')) : await boxOf(page.locator('.session-screen .stage'))
      const details = await boxOf(page.locator('[aria-label="Session context"]'))
      sameBox(card, sk.card, `${path} ${width}: workspace card`)
      sameBox(toolbar, sk.toolbar, `${path} ${width}: toolbar`)
      if (sk.panel) sameBox(panel, sk.panel, `${path} ${width}: sessions panel`)
      else expect(panel, `${path} ${width}: a skeleton without a panel grew one`).toBeNull()
      sameBox(stage, sk.stage, `${path} ${width}: session area`)
      if (sk.details) sameBox(details, sk.details, `${path} ${width}: details panel`)
      else expect(details, `${path} ${width}: a skeleton without details grew one`).toBeNull()
      expect(errors).toEqual([])
    } finally {
      release()
      await context.close()
    }
  }
  console.log('Loading workspace: boot, route, chunk and live screen keep the card, toolbar, sessions panel, session area and details widths across phone, tablet and desktop, named and bare.')
} finally { await browser.close() }
