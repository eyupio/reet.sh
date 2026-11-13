// Open terminal: the machine card's one action lands in a usable session, or starts one plain shell.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })

const now = Date.now()
const agentId = 'a1'
const caps = { protocolVersion: 1, os: 'linux', osUser: 'dev', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready' }
const machine = { agentId, host: 'studio', ready: true, serverEnabled: true, capabilities: caps }
const scope = { id: 'studio-agents', host: 'studio', agent: 'agents', name: 'studio', skills: [], skillCount: 0, complete: true, scannedAt: now, lastSeen: now, os: 'linux', arch: 'amd64', version: 'test' }
const sync = {
  environments: [scope],
  machines: [{ host: 'studio', groups: [scope], skillCount: 0 }],
  agents: [{ id: agentId, host: 'studio', version: 'test', lastSeen: now, paused: false, autoUpdate: false }],
  targets: [], jobs: [], runs: [], updateJobs: [],
}
const session = (id, state) => ({ id, agentId, title: id, state, createdAt: now, updatedAt: now, exitCode: null })

try {
  const creates = []
  let sessions = [session('session-running-0001', 'running')]
  const sessionsRoute = async request => {
    if (request.method() === 'GET') return { sessions }
    const body = request.postDataJSON()
    creates.push(body)
    const created = { ...session('session-new-0000001', 'creating'), title: body.title || 'Terminal' }
    sessions = [created, ...sessions]
    return created
  }
  const opened = await openWorkspace(browser, {
    width: 1440,
    sync,
    terminalStatus: { configured: true, available: true, reason: '' },
    terminalMachines: [machine],
    routes: {
      '/api/v2/terminal/sessions': sessionsRoute,
      '/api/v2/terminal/sessions/session-running-0001/attachments': { ticket: 'fixture-ticket00000000000' },
      '/api/v2/terminal/sessions/session-creating-0001/attachments': { ticket: 'fixture-ticket00000000000' },
      '/api/v2/terminal/sessions/session-new-0000001/attachments': { ticket: 'fixture-ticket00000000000' },
    },
  })
  const { page, errors } = opened
  await page.routeWebSocket('**/api/v2/terminal/socket', socket => {
    socket.onMessage(message => {
      if (typeof message !== 'string') return
      const data = JSON.parse(message)
      if (data.ticket) {
        socket.send('{"type":"ready"}')
        socket.send(Buffer.from('\x1b[?2026h\x1b[2J\x1b[HFixture shell\r\nready> \x1b[?2026l'))
      }
    })
  })
  const sessionParam = () => new URL(page.url()).searchParams.get('session')

  // The machine card's action points at the Sessions workspace with the one-action intent.
  await page.goto(`${base}/machines`)
  await expect(page.getByRole('link', { name: 'Open terminal', exact: true }).first()).toHaveAttribute('href', '/sessions?agent=a1&open=1')

  // A running session is reused: no creation, and the address names that session.
  await page.goto(`${base}/sessions?agent=a1&open=1`)
  await expect.poll(sessionParam).toBe('session-running-0001')
  expect(creates).toHaveLength(0)

  // A creation still in flight is reused too.
  sessions = [session('session-creating-0001', 'creating')]
  await page.goto(`${base}/sessions?agent=a1&open=1`)
  await expect.poll(sessionParam).toBe('session-creating-0001')
  expect(creates).toHaveLength(0)

  // Ended and exited sessions are never reused: one plain shell is started, with no agent launch.
  sessions = [session('session-ended-000001', 'ended'), session('session-exited-00001', 'exited')]
  await page.goto(`${base}/sessions?agent=a1&open=1`)
  await expect.poll(sessionParam).toBe('session-new-0000001')
  expect(creates).toHaveLength(1)
  expect(creates[0]).toMatchObject({ agentId, title: 'Terminal', cols: 100, rows: 30 })
  expect(creates[0]).not.toHaveProperty('tool')
  expect(creates[0]).not.toHaveProperty('structured')
  expect(creates[0]).not.toHaveProperty('launch')

  // A second click while the new session is still starting joins it instead of creating another.
  await page.goto(`${base}/sessions?agent=a1&open=1`)
  await expect.poll(sessionParam).toBe('session-new-0000001')
  expect(creates).toHaveLength(1)

  // An offline machine is explained instead of showing a blank terminal, and nothing is created.
  machine.ready = false
  sessions = []
  await page.goto(`${base}/sessions?agent=a1&open=1`)
  await expect(page.getByRole('alert')).toContainText('Open terminal needs studio to be online')
  expect(creates).toHaveLength(1)

  expect(errors).toEqual([])
  console.log('Open terminal passed: card link, running and creating reuse, ended ignored, one plain shell, repeat click and offline message.')
} finally { await browser.close() }
