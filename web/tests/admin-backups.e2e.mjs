// Admin → Backups: the copies and recent runs in the collection grammar, with
// counts, search, honest chips and the existing per-copy actions; and moving a
// controller from the page: fence, final copy, lift, and the import's address
// and live-source tick.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name

const now = Date.now()
const DAY = 86_400_000
const adminStatus = {
  version: 'v1.4.2', commit: 'fixture', db: { path: '/data/reet.db', size: 1024 },
  counts: { all: 0, liked: 0, bookmarked: 0, used: 0 }, stages: { use: 0, try: 0, shelf: 0 },
  environments: [], events: [], skillsSh: true, metaLastOK: '', metaLastError: '',
  topics: { lastSynced: '', lastError: '' },
  settings: { refreshEnabled: true, topicsEnabled: true, linkListsEnabled: false, rollupIndividual: true, pruneServerEnvironmentsDays: 0, instanceName: 'Test' },
}
const backup = {
  scope: 'instance', enabled: true, dir: '/var/lib/reet/backups',
  key: { fingerprint: 'age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqs', source: 'file', path: '/var/lib/reet/backup.key' },
  settings: { everySeconds: 86_400, keep: 7, destination: 'both', compression: 'gzip', hasPassword: true, nextAt: now + DAY, lastAt: now - DAY, lastState: 'succeeded', lastError: '', updatedAt: now },
  defaults: { everySeconds: 86_400, keep: 7, userEverySeconds: 86_400, userKeep: 7, destination: 'local', compression: 'gzip' },
  remotes: [{
    name: 's3-eu', source: 'database', endpoint: 'https://s3.eu-west-2.amazonaws.com', region: 'eu-west-2',
    bucket: 'reet-backups-production', prefix: 'instance/', accessKey: 'AKIAEXAMPLE', pathStyle: null,
    keep: 14, disabled: false, hasSecret: true, where: 's3://reet-backups-production/instance/', uploading: false, lastUploadAt: now - DAY,
  }],
  runs: [
    { id: 1, scope: 'instance', trigger: 'schedule', state: 'succeeded', startedAt: now - DAY, finishedAt: now - DAY + 5000, bytes: 1_234_567, object: 'reet-b1.tar.age', offsite: 's3-eu', error: '' },
    { id: 2, scope: 'instance', trigger: 'manual', state: 'failed', startedAt: now - 2 * DAY, finishedAt: now - 2 * DAY + 5000, bytes: 0, object: '', offsite: '', error: 'The destination refused the copy.' },
    { id: 3, scope: 'instance', trigger: 'schedule', state: 'running', startedAt: now - 60_000, finishedAt: 0, bytes: 0, object: '', offsite: 's3-eu', error: '' },
  ],
  items: [
    { id: 'b1', kind: 'instance', target: 'reet-b1.tar.age', takenAt: now - DAY, bytes: 1_234_567, version: 'v1.4.2', local: true, remotes: [] },
    { id: 'b2', kind: 'instance', target: 'reet-b2.tar.age', takenAt: now - 2 * DAY, bytes: 2_345_678, version: 'v1.4.2', local: false, remotes: ['s3-eu'] },
    { id: 'b3', kind: 'instance', target: 'reet-b3.tar.age', takenAt: now - 3 * DAY, bytes: 3_456_789, version: 'v1.4.1', local: true, remotes: ['s3-eu', 'r2'] },
  ],
  findings: [{ severity: 'warning', code: 'key_local', message: 'The backup key is stored on the same disk as the backups.' }],
}
const routes = () => ({
  '/api/admin/status': adminStatus,
  '/api/v2/admin/backups': backup,
  '/api/v2/admin/backups/import': { pending: null },
  '/api/v2/admin/backups/items/b1/verify': { ok: false, problems: ['The checksum does not match.'] },
  // What an older controller answered when nothing was removed: null lists.
  '/api/v2/admin/backups/prune': { removed: null, remotes: [{ name: 's3-eu', removed: null }] },
})
const open = options => openWorkspace(browser, { width: 1440, routes: routes(), ...options })

// A controller that holds every part of a move, with a fence the page can raise and lift and a run that seals the
// copy with the password it was sent. `seen` records what the page asked for, which is half of what is checked.
const ALL = ['copy-origin', 'fence', 'restore-public-url', 'copy-password-request', 'fence-address', 'fence-machines']
// What the fenced controller has heard from its machines: one gone, one being turned away now, one switched off since
// before the fence, and one with following turned off.
const MACHINES = [
  { id: 'm-studio', host: 'studio', owner: 'sam@example.test', lastSeen: now - 30_000, toldAt: now - 40_000, leftAt: now - 30_000, leftTo: 'https://reet.new.example', held: '' },
  { id: 'm-desk', host: 'desk', owner: 'sam@example.test', lastSeen: now - 5_000, toldAt: now - 5_000, leftAt: 0, leftTo: '', held: '' },
  { id: 'm-laptop', host: 'laptop', owner: 'ada@example.test', lastSeen: now - 3 * DAY, toldAt: 0, leftAt: 0, leftTo: '', held: '' },
  { id: 'm-lab', host: 'lab-builder-with-a-long-hostname-01', owner: 'ada@example.test', lastSeen: now - 5_000, toldAt: now - 5_000, leftAt: 0, leftTo: '', held: 'follow-off' },
]
const NOT_IN_COPY = [
  { names: ['REET_DATA_DIR', 'REET_ADDR', 'REET_PUBLIC_URL'], say: 'Where it keeps data, what it listens on, and its public address.' },
  { names: ['REET_BACKUP_KEY'], say: 'The backup key.' },
]
function moving({ features = ALL, fenced = false, movedTo = '', final = null, stage, pending = null } = {}) {
  const seen = { raised: [], readdressed: [], lifted: 0, runs: [], fenceReads: 0, staged: [], machineReads: 0 }
  let fence = fenced ? { fenced: true, since: now - 60_000, movedTo } : { fenced: false, since: 0, movedTo: '' }
  const items = [...backup.items.map(item => ({ ...item, fenced: false })), ...(final ? [final] : [])]
  return {
    seen,
    routes: {
      ...routes(),
      '/api/version': features ? { version: 'test', commit: 'fixture', features } : { version: 'test', commit: 'fixture' },
      '/api/v2/admin/backups': () => ({ ...backup, items }),
      '/api/v2/admin/fence': request => {
        if (request.method() === 'POST') {
          const body = request.postDataJSON()
          seen.raised.push(body)
          fence = { fenced: true, since: now, movedTo: body.movedTo ?? '' }
        } else if (request.method() === 'PATCH') {
          const body = request.postDataJSON()
          seen.readdressed.push(body)
          fence = { ...fence, movedTo: body.movedTo }
        } else if (request.method() === 'DELETE') {
          seen.lifted++
          fence = { fenced: false, since: 0, movedTo: '' }
        } else seen.fenceReads++
        return fence
      },
      '/api/v2/admin/fence/machines': () => {
        seen.machineReads++
        return { fenced: fence.fenced, since: fence.since, now, machines: MACHINES }
      },
      '/api/v2/admin/backups/run': request => {
        const body = request.postDataJSON()
        seen.runs.push(body)
        const item = { id: 'instance-final', kind: 'instance', target: 'instance', takenAt: now + 1000, bytes: 4096, version: 'test', local: true, remotes: [], protection: body.password ? 'password' : 'key', fenced: fence.fenced }
        items.unshift(item)
        return item
      },
      '/api/v2/admin/backups/import': request => {
        if (request.method() !== 'POST') return { pending, notInCopy: NOT_IN_COPY }
        const body = request.postDataBuffer()?.toString('latin1') ?? ''
        seen.staged.push(body)
        return stage(body)
      },
    },
  }
}
const field = (body, name) => body.match(new RegExp(`name="${name}"\\r\\n\\r\\n([^\\r]*)`))?.[1]
const moveAway = page => page.getByRole('region', { name: 'Move this controller away' })
const moveIn = page => page.getByRole('group', { name: 'Move a controller in' })
const report = {
  id: 'instance-final', takenAt: now, schemaVersion: 1, users: 2, usersByRole: { owner: 1, member: 1 }, shelves: 2, skills: 5, lists: 1, tags: 3,
  environments: 1, machines: 2, destinations: 0, sessionsKept: 1, sessionsEnded: 0, tokensRevoked: 0, mcpConnectionsRevoked: 0, tokensKept: true,
  identityDropped: true, unreadable: [], publicUrl: base, mailBaseRewritten: true, syncOriginsRewritten: 2, mcpGrantsStale: 1,
  paused: ['Gaffer goal "Ship the release" is paused.', '3 queued message(s) are held.'],
}
const copies = page => page.getByRole('region', { name: 'Backup copies' })
const runs = page => page.getByRole('region', { name: 'Recent runs' })
const count = panel => panel.getByRole('status').first()
const tab = (page, name) => page.getByRole('tablist', { name: 'Backup sections' }).getByRole('tab', { name })
const openTab = async (page, name) => { await tab(page, name).click(); await expect(tab(page, name)).toHaveAttribute('aria-selected', 'true') }
const scope = (page, group, name) => page.getByRole('group', { name: group }).getByRole('button', { name })

try {
  if (run('copies')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/backups`)
    await openTab(page, /^Copies/)
    await expect(count(copies(page))).toHaveText('3 copies')
    for (const [name, figure] of [['All', 3], ['This machine', 2], ['Off-site', 2], ['Damaged', 0]]) {
      await expect(scope(page, 'Show copies', new RegExp(`^${name}`))).toHaveText(new RegExp(`${name}\\s*${figure}`))
    }

    await page.getByLabel('Search copies').fill('b2')
    await expect(count(copies(page))).toHaveText('1 of 3 copies')
    await expect(scope(page, 'Show copies', /^Off-site/)).toHaveText(/Off-site\s*1/)
    await expect(scope(page, 'Show copies', /^This machine/)).toHaveText(/This machine\s*0/)
    await expect(page.getByText('b2', { exact: true })).toBeVisible()
    await expect(page.getByText('b1', { exact: true })).toHaveCount(0)

    await page.getByLabel('Search copies').fill('s3-eu')
    await expect(count(copies(page))).toHaveText('2 of 3 copies')
    await page.getByLabel('Search copies').fill('nowt')
    await expect(count(copies(page))).toHaveText('0 of 3 copies')
    await expect(page.getByRole('heading', { name: 'No copy matches that search or filter.' })).toBeVisible()
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(copies(page))).toHaveText('3 copies')

    await scope(page, 'Show copies', /^This machine/).click()
    await expect(count(copies(page))).toHaveText('2 of 3 copies')
    await expect(page.getByText('b1', { exact: true })).toBeVisible()
    await expect(page.getByText('b2', { exact: true })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('verify')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/backups`)
    await openTab(page, /^Copies/)
    const row = copies(page).getByRole('listitem').filter({ hasText: 'b1' })
    await row.getByRole('button', { name: 'Verify' }).click()
    await expect(row.getByText('The checksum does not match.', { exact: true })).toBeVisible()
    await expect(scope(page, 'Show copies', /^Damaged/)).toHaveText(/Damaged\s*1/)
    await scope(page, 'Show copies', /^Damaged/).click()
    await expect(count(copies(page))).toHaveText('1 of 3 copies')
    await expect(page.getByText('b1', { exact: true })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('runs')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/backups`)
    await openTab(page, /^Runs/)
    await expect(count(runs(page))).toHaveText('3 runs')
    for (const [name, figure] of [['All', 3], ['Succeeded', 1], ['Failed', 1], ['Running', 1]]) {
      await expect(scope(page, 'Show runs', new RegExp(`^${name}`))).toHaveText(new RegExp(`${name}\\s*${figure}`))
    }
    await page.getByLabel('Search runs').fill('manual')
    await expect(count(runs(page))).toHaveText('1 of 3 runs')
    await expect(page.getByText('The destination refused the copy.', { exact: true })).toBeVisible()
    await page.getByLabel('Search runs').fill('nowt')
    await expect(count(runs(page))).toHaveText('0 of 3 runs')
    await page.getByRole('button', { name: 'Clear filters' }).click()
    await expect(count(runs(page))).toHaveText('3 runs')
    await scope(page, 'Show runs', /^Failed/).click()
    await expect(count(runs(page))).toHaveText('1 of 3 runs')
    expect(errors).toEqual([])
    await context.close()
  }

  // The order is the safety of a move: no final copy before the fence, a copy that says it was taken fenced, and an
  // undo that is always there.
  if (run('move-away')) {
    const { seen, routes: moved } = moving()
    const { context, page, errors } = await open({ routes: moved })
    await page.goto(`${base}/admin/backups`)
    await openTab(page, /^Move/)
    const card = moveAway(page)
    await expect(card.getByRole('heading', { name: 'Fence this controller' })).toBeVisible()
    await expect(card.getByText('Available once the controller is fenced', { exact: false })).toBeVisible()
    await expect(card.getByRole('button', { name: 'Take the final copy' })).toHaveCount(0)
    await expect(page.getByText('This controller is fenced.')).toHaveCount(0)
    // Nothing is left to run on a machine: there is no command to copy.
    await expect(card.getByText('Machines follow on their own')).toBeVisible()
    await expect(card.getByText('reet agent set-server', { exact: false })).toHaveCount(0)
    expect(seen.machineReads).toBe(0)

    // A path is refused beside the field, before the request, and nothing is sent.
    await card.getByLabel('New address (optional)').fill('https://reet.new.example/app')
    await expect(card.getByText('Use the address alone, with nothing after the host name.')).toBeVisible()
    await expect(card.getByRole('button', { name: 'Fence this controller' })).toBeDisabled()
    // A plain http address elsewhere would be fenced but never followed, and the page says so first.
    await card.getByLabel('New address (optional)').fill('http://reet.new.example')
    await expect(card.getByText('Machines only follow an https:// address on their own', { exact: false })).toBeVisible()
    await card.getByLabel('New address (optional)').fill('https://reet.new.example/')

    // The dialog says what stops and that it can be undone; cancelling changes nothing.
    await card.getByRole('button', { name: 'Fence this controller' }).click()
    const dialog = page.getByRole('dialog', { name: 'Fence this controller?' })
    await expect(dialog).toContainText('Every change is refused')
    await expect(dialog).toContainText('Nothing is deleted')
    await expect(dialog).toContainText('lift the fence')
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    expect(seen.raised).toEqual([])
    await card.getByRole('button', { name: 'Fence this controller' }).click()
    await page.getByRole('dialog', { name: 'Fence this controller?' }).getByRole('button', { name: 'Fence this controller' }).click()
    await expect(page.getByText('This controller is fenced.')).toBeVisible()
    expect(seen.raised).toEqual([{ movedTo: 'https://reet.new.example' }])
    await expect(card.getByText('Fenced since')).toBeVisible()
    await expect(card.getByRole('link', { name: 'https://reet.new.example' })).toBeVisible()

    // Each machine as the fence has heard from it, and the only one needing a hand says why.
    await expect(card.getByText('1 of 4', { exact: false })).toBeVisible()
    const row = host => card.getByRole('list', { name: 'Machines' }).getByRole('listitem').filter({ hasText: host })
    await expect(row('studio')).toContainText('Moved to https://reet.new.example')
    await expect(row('desk')).toContainText('waiting for the new controller')
    await expect(row('laptop')).toContainText('Not heard from since the fence')
    await expect(row('lab-builder')).toContainText('Following is turned off on this machine')

    // The copy password is this copy's own: twelve characters, sent with the run and with nothing else.
    const take = card.getByRole('button', { name: 'Take the final copy' })
    await expect(take).toBeDisabled()
    await card.getByLabel('Copy password').fill('too short')
    await expect(card.getByText('A copy password needs at least 12 characters.')).toBeVisible()
    await expect(take).toBeDisabled()
    await card.getByLabel('Copy password').fill('a password for the move')
    await take.click()
    await expect(card.getByText('taken fenced')).toBeVisible()
    expect(seen.runs).toEqual([{ password: 'a password for the move' }])
    await expect(card.getByText('It opens with its password alone', { exact: false })).toBeVisible()
    const download = card.getByRole('link', { name: 'Download the final copy' })
    await expect(download).toHaveAttribute('href', '/api/v2/admin/backups/items/instance-final/download?format=zip')
    await expect(download).toHaveAttribute('download', '')
    // The password is not left in the page once the copy exists.
    await card.getByRole('button', { name: 'Take another' }).click()
    await expect(card.getByLabel('Copy password')).toHaveValue('')
    await openTab(page, /^Copies/)
    await expect(copies(page).getByRole('listitem').filter({ hasText: 'instance-final' }).getByText('taken fenced')).toBeVisible()
    await openTab(page, /^Move/)

    // The fence is in place after a reload, read from the controller, and the final copy is found again from the list.
    await page.reload()
    await openTab(page, /^Move/)
    await expect(page.getByText('This controller is fenced.')).toBeVisible()
    await expect(moveAway(page).getByRole('link', { name: 'Download the final copy' })).toBeVisible()

    await moveAway(page).getByRole('button', { name: 'Lift the fence' }).click()
    const lift = page.getByRole('dialog', { name: 'Lift the fence?' })
    await expect(lift).toContainText('two controllers would work from one set of data')
    await lift.getByRole('button', { name: 'Lift the fence' }).click()
    await expect(page.getByText('This controller is fenced.')).toHaveCount(0)
    expect(seen.lifted).toBe(1)
    await expect(moveAway(page).getByRole('link', { name: 'Download the final copy' })).toHaveCount(0)
    await expect(moveAway(page).getByText('Available once the controller is fenced', { exact: false })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  // A fence raised before the new address was known is given it later, without lifting the fence (which would set the
  // controller's work going again), and changed the same way.
  if (run('move-readdress')) {
    const { seen, routes: moved } = moving({ fenced: true })
    const { context, page, errors } = await open({ routes: moved })
    await page.goto(`${base}/admin/backups`)
    await openTab(page, /^Move/)
    const card = moveAway(page)
    await expect(card.getByText('so machines do not know where to go', { exact: false })).toBeVisible()
    await expect(card.getByText('Give the new address in step 1', { exact: false })).toBeVisible()
    const save = card.getByRole('button', { name: 'Save the address' })
    await expect(save).toBeDisabled()
    await card.getByLabel('New address', { exact: true }).fill('https://reet.new.example/app')
    await expect(save).toBeDisabled()
    await card.getByLabel('New address', { exact: true }).fill('https://reet.new.example/')
    await save.click()
    await expect(card.getByRole('link', { name: 'https://reet.new.example' })).toBeVisible()
    expect(seen.readdressed).toEqual([{ movedTo: 'https://reet.new.example' }])
    expect(seen.lifted).toBe(0)
    await expect(card.getByText('Give the new address in step 1', { exact: false })).toHaveCount(0)

    await card.getByRole('button', { name: 'Change the address' }).click()
    await card.getByLabel('Change the new address').fill('https://reet.other.example')
    await card.getByRole('button', { name: 'Save the address' }).click()
    await expect(card.getByRole('link', { name: 'https://reet.other.example' })).toBeVisible()
    expect(seen.readdressed).toEqual([{ movedTo: 'https://reet.new.example' }, { movedTo: 'https://reet.other.example' }])
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('tabs')) {
    const { context, page, errors } = await open({})
    await page.goto(`${base}/admin/backups`)
    await expect(tab(page, /^Overview/)).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('heading', { name: 'Needs attention' })).toBeVisible()
    await expect(copies(page)).toHaveCount(0)
    await expect(runs(page)).toHaveCount(0)
    await expect(page.getByRole('heading', { name: 'Schedule and retention' })).toHaveCount(0)
    expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight * 2), 'overview fits in two screens').toBe(true)
    await tab(page, /^Overview/).focus()
    await page.keyboard.press('ArrowRight')
    await expect(tab(page, /^Copies/)).toHaveAttribute('aria-selected', 'true')
    await expect(copies(page)).toBeVisible()
    await page.keyboard.press('End')
    await expect(tab(page, /^Move/)).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByText('Move a controller in')).toBeVisible()
    await openTab(page, /^Schedule/)
    await expect(page.getByRole('heading', { name: 'Schedule and retention' })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  // A copy the fence did not hold for is said to be so, not handed over as the final one.
  if (run('move-away-unfenced-copy')) {
    const { routes: moved } = moving({ fenced: true })
    moved['/api/v2/admin/backups/run'] = () => ({ id: 'instance-raced', kind: 'instance', target: 'instance', takenAt: now + 1000, bytes: 4096, local: true, remotes: [], protection: 'password', fenced: false })
    const { context, page, errors } = await open({ routes: moved })
    await page.goto(`${base}/admin/backups`)
    await openTab(page, /^Move/)
    const card = moveAway(page)
    await card.getByLabel('Copy password').fill('a password for the move')
    await card.getByRole('button', { name: 'Take the final copy' }).click()
    await expect(card.getByText('not marked fenced')).toBeVisible()
    await expect(card.getByText('Take it again.', { exact: false })).toBeVisible()
    await expect(card.getByRole('link', { name: 'Download the final copy' })).toHaveCount(0)
    await expect(card.getByRole('button', { name: 'Take the final copy' })).toBeVisible()
    expect(errors).toEqual([])
    await context.close()
  }

  if (run('prune')) {
    const { context, page, errors } = await open({})
    page.on('dialog', dialog => dialog.accept())
    await page.goto(`${base}/admin/backups`)
    await openTab(page, /^Destinations/)
    await page.getByRole('button', { name: 'Apply retention now' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Removed 0 copies.' })).toBeVisible()
    await expect(page.getByRole('alert').filter({ hasText: 'Cannot read properties' })).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  // The page offers only what the controller says it holds, and only the owner may fence.
  if (run('move-honest')) {
    {
      const { seen, routes: older } = moving({ features: null })
      const { context, page, errors } = await open({ routes: older })
      await page.goto(`${base}/admin/backups`)
      await openTab(page, /^Move/)
      await expect(moveIn(page)).toBeVisible()
      await expect(moveAway(page)).toHaveCount(0)
      await expect(moveIn(page).getByLabel("This controller's address (optional)")).toHaveCount(0)
      expect(seen.fenceReads).toBe(0)
      expect(errors).toEqual([])
      await context.close()
    }
    {
      const { seen, routes: partial } = moving({ features: ['fence'], fenced: true })
      const { context, page, errors } = await open({ routes: partial })
      await page.goto(`${base}/admin/backups`)
      await openTab(page, /^Move/)
      const card = moveAway(page)
      // No password of its own for this copy, so none is asked for and none is sent.
      await expect(card.getByLabel('Copy password')).toHaveCount(0)
      await card.getByRole('button', { name: 'Take the final copy' }).click()
      await expect(card.getByRole('link', { name: 'Download the final copy' })).toBeVisible()
      expect(seen.runs).toEqual([{}])
      await expect(moveIn(page).getByLabel("This controller's address (optional)")).toHaveCount(0)
      expect(errors).toEqual([])
      await context.close()
    }
    {
      const { routes: fencedRoutes } = moving({ fenced: true, movedTo: 'https://reet.new.example' })
      const { context, page, errors } = await open({ routes: fencedRoutes, status: { user: { id: 2, name: 'Ada', email: 'ada@example.test', role: 'admin' } } })
      await page.goto(`${base}/admin/backups`)
      // An admin has no Move section at all; the banner still tells them.
      await expect(tab(page, /^Move/)).toHaveCount(0)
      await expect(page.getByText('This controller is fenced.')).toBeVisible()
      await expect(page.getByText('Only the owner can lift the fence.', { exact: false })).toBeVisible()
      await expect(moveAway(page)).toHaveCount(0)
      await expect(moveIn(page)).toHaveCount(0)
      expect(errors).toEqual([])
      await context.close()
    }
  }

  // The tick that accepts a copy of a running controller is asked for by the controller's refusal and never before,
  // and a refusal costs the person nothing they typed.
  if (run('move-in')) {
    let attempt = 0
    const { seen, routes: incoming } = moving({
      stage: () => ++attempt === 1
        ? { status: 409, json: { error: 'this copy was taken from a controller that was not stopped: its source was still running. Fence the old controller and take a final copy.', code: 'copy_source_live' } }
        : { staged: true, restartRequired: true, report, notInCopy: NOT_IN_COPY },
    })
    const { context, page, errors } = await open({ routes: incoming })
    await page.goto(`${base}/admin/backups`)
    await openTab(page, /^Move/)
    const card = moveIn(page)
    const address = card.getByLabel("This controller's address (optional)")
    await expect(address).toHaveValue(base)
    await expect(card.getByText('The old controller is stopped or gone')).toHaveCount(0)
    const stageButton = card.getByRole('button', { name: 'Check and stage' })
    await expect(stageButton).toBeDisabled()

    await card.locator('input[type=file]').setInputFiles({ name: 'instance-final.zip', mimeType: 'application/zip', buffer: Buffer.from('sealed') })
    await card.getByLabel('Copy password (if it has one)').fill('a password for the move')
    await address.fill('https://reet.new.example/app')
    await expect(card.getByText('Use the address alone, with nothing after the host name.')).toBeVisible()
    await expect(stageButton).toBeDisabled()
    await address.fill('https://reet.new.example')
    await stageButton.click()

    // The refusal is the controller's own text, beside the form, and everything typed is still there.
    await expect(card.getByRole('alert')).toContainText('was not stopped')
    expect(field(seen.staged[0], 'publicUrl')).toBe('https://reet.new.example')
    expect(field(seen.staged[0], 'allowLiveSource')).toBeUndefined()
    await expect(address).toHaveValue('https://reet.new.example')
    await expect(card.getByLabel('Copy password (if it has one)')).toHaveValue('a password for the move')
    const tick = card.getByRole('checkbox', { name: /The old controller is stopped or gone/ })
    await expect(tick).toBeVisible()
    await expect(tick).not.toBeChecked()
    await expect(stageButton).toBeDisabled()
    await tick.check()
    await stageButton.click()

    await expect(card.getByText('is staged. Restart reet to put it in place.', { exact: false })).toBeVisible()
    expect(field(seen.staged[1], 'allowLiveSource')).toBe('1')
    expect(field(seen.staged[1], 'password')).toBe('a password for the move')
    await expect(card.getByText(`Saved addresses now point to ${base}.`)).toBeVisible()
    await expect(card.getByText('1 MCP grant(s) issued for the old address were removed', { exact: false })).toBeVisible()
    await expect(card.getByText('Gaffer goal "Ship the release" is paused.')).toBeVisible()
    await card.getByText('Not in the copy: set these by hand on this machine').click()
    await expect(card.getByText('REET_DATA_DIR, REET_ADDR, REET_PUBLIC_URL')).toBeVisible()
    await expect(card.getByText('machines move to it on their own', { exact: false })).toBeVisible()
    await expect(card.getByText('reet agent set-server', { exact: false })).toHaveCount(0)
    await expect(card.getByRole('alert')).toHaveCount(0)
    expect(errors).toEqual([])
    await context.close()
  }

  // A fence stops every write, so the shell says so on any page an admin opens. Someone who is not an admin is not
  // asked about it, and learns it from the write the controller refuses.
  if (run('shell')) {
    {
      const { routes: fencedRoutes } = moving({ fenced: true, movedTo: 'https://reet.new.example' })
      const { context, page } = await open({ routes: fencedRoutes })
      await page.goto(`${base}/admin/overview`)
      const strip = page.locator('.fence-strip')
      await expect(strip).toContainText('This controller is fenced.')
      await expect(strip).toContainText('It has moved to https://reet.new.example.')
      await expect(strip.getByRole('link', { name: 'Open Backups' })).toHaveAttribute('href', '/admin/backups')
      // Backups says it in full, so the line is not repeated above it.
      await page.goto(`${base}/admin/backups`)
      await expect(page.getByText('This controller is fenced.')).toHaveCount(1)
      await expect(page.locator('.fence-strip')).toHaveCount(0)
      await context.close()
    }
    {
      const { seen, routes: fencedRoutes } = moving({ fenced: true, movedTo: 'https://reet.new.example' })
      fencedRoutes['/api/skills'] = request => request.method() === 'GET' ? { skills: [], counts: { all: 0, used: 0, liked: 0, bookmarked: 0 } }
        : { status: 423, json: { error: 'This controller has moved, so it no longer accepts changes.', code: 'controller_moved', fenced: true, since: now, movedTo: 'https://reet.new.example' } }
      const { context, page } = await open({ routes: fencedRoutes, status: { user: { id: 3, name: 'Mo', email: 'mo@example.test', role: 'member' } } })
      await page.goto(`${base}/skills`)
      await page.getByLabel('Skill install command, URL or slug').fill('demo/toolkit/pdf')
      await expect(page.locator('.fence-strip')).toHaveCount(0)
      expect(seen.fenceReads).toBe(0)
      await page.getByRole('button', { name: 'Add to library' }).click()
      await expect(page.locator('.fence-strip')).toContainText('It has moved to https://reet.new.example.')
      await expect(page.locator('.fence-strip').getByRole('link')).toHaveCount(0)
      expect(seen.fenceReads).toBe(0)
      await context.close()
    }
  }

  if (run('move-phone')) {
    for (const width of [390, 320]) {
      for (const theme of ['light', 'dark']) {
        const final = { id: 'instance-20261113T101500Z-9f2c4a7be1d0', kind: 'instance', target: 'instance', takenAt: now, bytes: 4096, local: true, remotes: [], protection: 'password', fenced: true }
        const { routes: fencedRoutes } = moving({ fenced: true, movedTo: 'https://reet-controller.new-machine.example.com', final, pending: { ...report, publicUrl: 'https://reet-controller.new-machine.example.com' } })
        const { context, page, errors } = await open({ width, height: 812, touch: true, theme, routes: fencedRoutes })
        await page.goto(`${base}/admin/backups`)
      await openTab(page, /^Move/)
        await expect(moveAway(page).getByRole('link', { name: 'Download the final copy' })).toBeVisible()
        await moveIn(page).getByText('Not in the copy: set these by hand on this machine').click()
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px, ${theme}`).toBe(true)
        await expect(moveAway(page).getByText('1 of 4', { exact: false })).toBeVisible()
        for (const name of ['Change the address', 'Lift the fence', 'Take another']) {
          const box = await moveAway(page).getByRole('button', { name }).boundingBox()
          expect(box.height, `${name} height at ${width}px`).toBeGreaterThanOrEqual(44)
        }
        if (process.env.REET_UI_SCREENSHOTS) {
          await moveAway(page).screenshot({ path: `${process.env.REET_UI_SCREENSHOTS}/move-away-fixture-${width}-${theme}.png` })
          await moveIn(page).screenshot({ path: `${process.env.REET_UI_SCREENSHOTS}/move-in-fixture-${width}-${theme}.png` })
        }
        expect(errors).toEqual([])
        await context.close()
      }
    }
  }

  if (run('phone')) {
    for (const width of [390, 320]) {
      const { context, page, errors } = await open({ width, height: 812, touch: true })
      await page.goto(`${base}/admin/backups`)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `document width at ${width}px`).toBe(true)
      for (const [name, label] of [[/^Copies/, 'Search copies'], [/^Runs/, 'Search runs']]) {
        await openTab(page, name)
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${name} width at ${width}px`).toBe(true)
        const box = await page.getByLabel(label).boundingBox()
        expect(box.height, `${label} height at ${width}px`).toBeGreaterThanOrEqual(44)
      }
      await openTab(page, /^Copies/)
      for (const name of [/^All/, /^This machine/, /^Off-site/, /^Damaged/]) {
        const box = await scope(page, 'Show copies', name).boundingBox()
        expect(box.height, `copy chip height at ${width}px`).toBeGreaterThanOrEqual(44)
      }
      expect(errors).toEqual([])
      await context.close()
    }
  }
} finally {
  await browser.close()
}
console.log('Admin backups: sectioned tabs, copies and runs in one grammar, honest chips, search, verify, a null-safe prune and a phone layout; a move away (fence, final copy, lift), a move in (address, live-source tick), honest offers and the shell line.')
