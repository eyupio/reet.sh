// One fake API for the whole signed-in app with long, unbreakable names everywhere (hosts, paths, skills, sessions, tags),
// assembled from the other web/tests fixtures plus shapes from web/src/lib/api.ts. Used by phone-zoom.e2e.mjs.
const now = Date.now(), DAY = 86400000
// A 57-character host and a 70-character path with no break opportunities (a browser may break after a hyphen, but not at a
// dot, a slash or an underscore): the strings that make a phone zoom the page out.
export const LONGHOST = 'build.server.eu.west.2a.very.long.hostname.without.breaks'
export const LONGPATH = '/srv/projects/a/really/deep/folder/path/that/goes/on/and/on/for_ever_1'
export const LONGSKILL = 'a_very_long_skill_name_that_has_no_spaces_at_all_and_must_wrap'
const skill = (id, extra = {}) => ({ id: `demo/toolkit/${id}`, name: id, slug: id, source: 'demo/toolkit', sourceType: 'github', url: `https://www.skills.sh/demo/toolkit/${id}`, description: 'Reviews pull requests for correctness, style and missing tests before you merge them into main.', notes: '', installs: 1234, installsWeek: 56, trend: [1, 3, 2, 5, 8, 6, 9], liked: false, bookmarked: false, used: true, createdAt: now - 3 * DAY, updatedAt: now, tags: [{ id: 1, name: 'review' }, { id: 2, name: 'testing' }], ...extra })
export const skills = [skill('code-review'), skill('testing', { used: false, bookmarked: true }), skill('deployment', { liked: true }), skill('pdf', { used: false }), skill(LONGSKILL, { used: false })]
const scope = (id, host, agent, items, project = '') => ({ id, host, agent, name: host, kind: 'agent', project, path: project || `/home/dev/.${agent}/skills`, skills: items, skillCount: items.length, complete: true, scannedAt: now, lastSeen: now, os: 'linux', arch: 'amd64', version: 'test' })
const environments = [
  scope('laptop-shared', 'dev-laptop', 'agents', skills.slice(0, 2)),
  scope('laptop-claude', 'dev-laptop', 'claude', skills.slice(0, 1), '/home/dev/work/site'),
  scope('build-shared', LONGHOST, 'agents', skills),
  scope('pi-shared', 'pi', 'codex', skills.slice(2), LONGPATH),
]
environments[0].content = { state: 'observed', schemaVersion: 1, receivedAt: now, skills: [{ name: 'code-review', complete: true, digest: 'a'.repeat(64), files: 2, bytes: 123 }] }
environments[1].content = { state: 'stale', schemaVersion: 1, receivedAt: now - 600000, skills: [{ name: 'code-review', complete: false, files: 0, bytes: 0, issues: ['symlink'] }] }
const hosts = ['dev-laptop', LONGHOST, 'pi']
const machines = hosts.map(host => ({ host, groups: environments.filter(e => e.host === host), skillCount: environments.filter(e => e.host === host).reduce((a, e) => a + e.skillCount, 0) }))
const A = 'connector-laptop000000001', B = 'connector-build000000001', C = 'connector-pi00000000000001'
const agents = [{ id: A, host: 'dev-laptop', version: 'test', lastSeen: now, paused: false, autoUpdate: true }, { id: B, host: LONGHOST, version: 'v1.3.0', lastSeen: now, paused: false, autoUpdate: false }, { id: C, host: 'pi', version: 'test', lastSeen: now - 2 * DAY, paused: true, autoUpdate: false }]
const updateJobs = [{ id: 'u2', agentId: B, targetId: '', kind: 'update', status: 'failed', revision: 'latest', hash: '', plan: {}, message: 'The checksum did not match.', createdAt: now - 120000, updatedAt: now - 60000, stages: [{ key: 'queued', label: 'Queued for the machine', status: 'done' }, { key: 'verify', label: 'Verifying the checksum and build', status: 'failed', detail: 'Checksum mismatch' }] }]
const targets = environments.map(target => ({ target, sourceId: 'shelf', sourceName: 'Central library', mode: 'additive', desired: skills.slice(0, 3), missing: skills.slice(0, 3).filter(s => !target.skills.some(a => a.id === s.id)), extra: [], revision: `rev-${target.id}`, updatedAt: now }))
export const sync = { environments, machines, agents, releases: { latest: { version: 'v1.4.2' } }, updateJobs, targets, jobs: [], runs: [] }
const caps = over => ({ protocolVersion: 1, os: 'linux', osUser: 'dev', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready', launch: true, worktree: true, models: true, tools: ['claude', 'codex', 'gemini'], ...over })
export const tmachines = [{ agentId: A, host: 'dev-laptop', ready: true, serverEnabled: true, capabilities: caps() }, { agentId: B, host: LONGHOST, ready: true, serverEnabled: true, capabilities: caps({ osUser: 'root' }) }, { agentId: C, host: 'pi', ready: false, serverEnabled: true, capabilities: caps({ terminalEnabled: false, readiness: 'terminal_disabled' }) }]
export const sessions = [{ id: 'session-build00000000001', agentId: A, title: 'OpenCode', state: 'running', createdAt: now, updatedAt: now, exitCode: null }, { id: 'session-build00000000002', agentId: B, title: 'Claude Code in a_really_long_session_title_without_any_spaces', state: 'running', createdAt: now, updatedAt: now, exitCode: null }]
const member = (host, dir, position, over = {}) => ({ host, dir, tool: '', model: '', position, ...over })
export const projects = [
  { id: 'proj1', name: 'Site', defaultTool: 'claude', defaultModel: 'opus', sharedPrompt: 'Read the README and tidy the tests', hasSharedPrompt: true, useWorktrees: true, members: [member('dev-laptop', '/home/dev/work/site', 0), member(LONGHOST, LONGPATH, 1, { tool: 'codex' })], createdAt: now - DAY, updatedAt: now, version: 3 },
  { id: 'proj2', name: 'An extremely long project name that goes on for a while', defaultTool: 'codex', defaultModel: '', sharedPrompt: '', hasSharedPrompt: false, useWorktrees: false, members: [], createdAt: now - DAY, updatedAt: now, version: 1 },
]
const runs = [{ id: 'run1', requestId: 'r1', projectId: 'proj1', projectName: 'Site', state: 'running', startedAt: now - 3600000, useWorktrees: true, worktreeProject: 'site', members: [
  { position: 0, host: 'dev-laptop', dir: '/home/dev/work/site', tool: 'claude', model: 'opus', state: 'started', code: '', sentence: '', sessionId: sessions[0].id, worktreePath: '/home/dev/.reet/worktrees/site/site/tok0', worktreeRepo: '/home/dev/work/site', worktreeBranch: 'reet/site-tok0', worktreeRun: 'wr1', worktreeProject: 'site', sharesCheckout: false, attempt: 1 },
  { position: 1, host: LONGHOST, dir: LONGPATH, tool: 'codex', model: '', state: 'failed', code: 'machine_offline', sentence: 'The machine was offline when the run started.', sessionId: '', worktreePath: '', worktreeRepo: '', worktreeBranch: '', worktreeRun: '', worktreeProject: '', sharesCheckout: false, attempt: 1 }] }]
// Folder projects (Home's rail): one with a long name on the long-named machine, one plain.
const fp = (id, name, host, machine, dir, over = {}) => ({ id, host, machine, dir, name, isRepo: true, baseBranch: '', worktreeDefault: 'inherit', defaultTool: '', defaultModel: '', hasStarterPrompt: false, note: '', position: 0, createdAt: now - DAY, updatedAt: now, version: 2, ...over })
export const folderProjects = [
  fp('fp1', 'site', A, 'dev-laptop', '/home/dev/work/site', { baseBranch: 'main', hasStarterPrompt: true }),
  fp('fp2', 'An extremely long project name that goes on for a while', B, LONGHOST, LONGPATH, { isRepo: false, position: 1, note: 'Site, An extremely long saved Project name' }),
]
const events = Array.from({ length: 12 }, (_, i) => ({ id: i + 1, at: now - i * 3600000, actor: 'Sam Example', source: i % 2 ? 'ui' : 'cli', action: ['skill-add', 'terminal-launch', 'project-run', 'sync-apply'][i % 4], skillId: 'demo/toolkit/code-review', skillName: 'code-review', detail: JSON.stringify({ machine: A, host: 'dev-laptop', tool: 'claude', model: 'opus', dir: '/home/dev/work/site', project: 'proj1', name: 'Site', run: 'run1', members: 2 }) }))
const owner = { id: 1, name: 'Sam Example', email: 'sam@example.test', role: 'owner' }
const lists = [{ id: 7, ownerId: 1, ownerName: 'Sam Example', name: 'Team favourites', description: 'The skills the platform team actually uses every day.', visibility: 'team', slug: 'team-favourites', role: 'owner', skillCount: 3, memberCount: 4, createdAt: now - DAY, updatedAt: now }, { id: 8, ownerId: 2, ownerName: 'Ali', sharedWithMe: true, name: 'Public picks', description: 'Shared with everyone.', visibility: 'public', slug: 'public-picks', role: 'viewer', skillCount: 2, memberCount: 1, createdAt: now - DAY, updatedAt: now }]
const listDetail = { ...lists[0], skills: skills.slice(0, 3).map(s => ({ ...s, note: 'Use before every PR.', addedBy: 1, addedByName: 'Sam Example', useCount: 3, memberCount: 4 })), members: [owner, { id: 2, name: 'Ali Long-Surname-With-Hyphens', email: 'ali.really.long.email.address@example-company.test', role: 'editor' }], linkEnabled: true, canEdit: true, canManage: true, canTransfer: true, audience: { people: [], excluded: [] } }
const tickets = [{ id: 41, userId: 1, opener: 'Sam Example', subject: 'Terminal shows a blank screen after switching apps on my phone', status: 'open', assignee: 'Admin', messages: 3, createdAt: now - DAY, updatedAt: now }, { id: 42, userId: 1, opener: 'Sam Example', subject: 'Sync failed', status: 'closed', messages: 1, createdAt: now - 2 * DAY, updatedAt: now - DAY }]
const ticketDetail = { ticket: tickets[0], messages: [{ id: 1, ticketId: 41, authorKind: 'user', author: 'Sam Example', body: 'When I come back to the terminal on my phone it is blank.\n\n```\n$ reet doctor --verbose --machine build.server.eu.west.2a.very.long.hostname.without.breaks\n```', createdAt: now - DAY }, { id: 2, ticketId: 41, authorKind: 'admin', author: 'Admin', body: 'Thanks, we are looking into it.', createdAt: now - 3600000 }, { id: 3, ticketId: 41, authorKind: 'admin', author: 'Admin', body: 'Internal: likely tmux resize.', internal: true, createdAt: now }] }
const notifications = Array.from({ length: 6 }, (_, i) => ({ id: i + 1, kind: 'machine', title: ['Machine dev-laptop went offline', 'Sync finished on pi', 'Your backup failed', 'New sign-in from a phone', 'Ticket #41 has a reply', 'Update available: v1.4.2'][i], body: 'More detail about what happened, and what you can do about it next.', link: '/machines', tone: i === 2 ? 'danger' : i === 0 ? 'warning' : '', readAt: i > 2 ? now : undefined, createdAt: now - i * 3600000, updatedAt: now }))
const pref = (key, audience, group, label) => ({ key, audience, group, label, description: 'Tell me when this happens, by email and in the app.', enabled: true, mode: 'immediate', source: 'default' })
const prefs = [pref('machine.offline', 'user', 'Machines', 'A machine goes offline'), pref('sync.failed', 'user', 'Sync', 'A sync fails'), pref('security.signin', 'user', 'Security', 'A new sign-in'), pref('ticket.reply', 'staff', 'Support', 'A ticket gets a reply')]
const defaults = prefs.map(p => ({ ...p, default: { enabled: true, mode: 'immediate', owner: true, admin: true } }))
const backupStatus = admin => ({ scope: admin ? 'instance' : 'user', enabled: true, dir: '/var/lib/reet/backups', key: { fingerprint: 'age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqs', source: 'file', path: '/var/lib/reet/backup.key' }, settings: { everySeconds: 86400, keep: 7, destination: 'both', compression: 'gzip', hasPassword: true, nextAt: now + DAY, lastAt: now - DAY, lastState: 'succeeded', lastError: '', updatedAt: now }, defaults: { everySeconds: 86400, keep: 7, userEverySeconds: 86400, userKeep: 7, destination: 'local', compression: 'gzip' }, remotes: [{ name: 's3-eu', source: 'database', endpoint: 'https://s3.eu-west-2.amazonaws.com', region: 'eu-west-2', bucket: 'reet-backups-production', prefix: 'instance/', accessKey: 'AKIAEXAMPLE', pathStyle: null, keep: 14, disabled: false, hasSecret: true, where: 's3://reet-backups-production/instance/', uploading: false, lastUploadAt: now - DAY }], runs: [{ id: 1, scope: 'instance', trigger: 'schedule', state: 'succeeded', startedAt: now - DAY, finishedAt: now - DAY + 5000, bytes: 1234567, object: 'reet-2026-10-25T00-00-00Z.tar.age', offsite: 's3-eu', error: '' }], items: [{ id: 'b1', kind: admin ? 'instance' : 'user', target: 'reet-2026-10-25T00-00-00Z.tar.age', takenAt: now - DAY, bytes: 1234567, version: 'v1.4.2', local: true, remotes: ['s3-eu'] }], findings: [{ severity: 'warning', code: 'key_local', message: 'The backup key is stored on the same disk as the backups.' }] })
const adminStatus = { version: 'v1.4.2', commit: 'abc1234', db: { path: '/var/lib/reet/reet.db', size: 12345678 }, counts: { all: 5, liked: 1, bookmarked: 1, used: 3 }, stages: { use: 3, try: 1, shelf: 1 }, environments, events, skillsSh: true, metaLastOK: new Date(now).toISOString(), metaLastError: '', topics: { lastSynced: new Date(now).toISOString(), lastError: '' }, settings: { refreshEnabled: true, topicsEnabled: true, linkListsEnabled: true, rollupIndividual: false, pruneServerEnvironmentsDays: 30, instanceName: 'reet' } }
const users = [{ ...owner, createdAt: now - 30 * DAY, lastActive: now, tokens: 2 }, { id: 2, name: 'Ali Long-Surname-With-Hyphens', email: 'ali.really.long.email.address@example-company.test', role: 'editor', createdAt: now - 3 * DAY, lastActive: now - DAY, tokens: 0 }, { id: 3, name: 'Viewer', email: 'v@example.test', role: 'viewer', createdAt: now - DAY, disabledAt: now, tokens: 0 }]
const rollup = { generatedAt: now, individual: true, teamSize: 3, nothingInstalled: 1, members: users.map(u => ({ id: u.id, name: u.name, email: u.email, role: u.role, used: 2, installed: 3 })), lists: [{ id: 7, name: 'Team favourites' }], skills: skills.map(s => ({ id: s.id, name: s.name, source: s.source, tags: ['review'], lists: [{ id: 7, name: 'Team favourites' }], usedBy: 2, onShelf: 3, installedBy: 2, environmentLinks: 4, members: users.map(u => ({ id: u.id, name: u.name, used: true, environments: ['dev-laptop'] })) })), drift: { unlisted: ['pdf'], unused: [{ listId: 7, listName: 'Team favourites', skillId: 'x', name: 'deployment', source: 'demo/toolkit' }] } }
const mail = { source: 'database', mode: 'smtp', summary: 'Sending through smtp.example.test:587 with STARTTLS.', host: 'smtp.example.test', port: 587, tls: 'starttls', user: 'reet@example.test', passwordSet: true, from: 'reet@example.test', fromName: 'reet.sh', replyTo: '', baseUrl: 'https://reet.example.test', baseUrlInUse: 'https://reet.example.test', dkimSelector: 'reet', updatedAt: now, updatedBy: 'Sam Example' }
const outbox = { mode: 'smtp', summary: 'Mail is sent through your SMTP server.', messages: [{ id: 'm1', template: 'invite', to: 'ali.really.long.email.address@example-company.test', subject: 'You have been invited to reet.sh', state: 'failed', attempts: 3, lastError: '550 5.7.1 relaying denied for this very long reason that goes on', via: 'smtp', createdAt: now - DAY, secret: true, resendable: true }], counts: { failed: 1, delivered: 12 } }
const hosting = { username: 'sam', prefix: 'reet-', skills: [{ slug: 'reet-review', description: 'Our review checklist', content: '---\nname: reet-review\n---\n# Review\nCheck everything twice. A very long line of markdown content that will not wrap nicely in a phone-sized editor or preview block ever.', files: { 'scripts/check.sh': '#!/bin/sh\necho ok' }, published: true, createdAt: now - DAY, updatedAt: now }] }
const profileRecord = { schema_version: 1, name: 'laptop-claude', revisions: [{ revision: 2, created_at: new Date(now).toISOString(), snapshot_id: 'snap-' + 'a'.repeat(24), snapshot_digest: 'sha256:' + 'b'.repeat(64), snapshot_taken_at: new Date(now).toISOString(), preview: { os: 'linux', architecture: 'amd64', schema_version: 1, agents: [{ id: 'claude', name: 'Claude Code', state: 'installed', components: [{ kind: 'settings', source: '~/.claude/settings.json', scope: 'user', state: 'captured', included_fields: ['model', 'permissions'], excluded_fields: ['apiKey'] }, { kind: 'mcp', source: '~/.claude.json', scope: 'user', declarations: 3, selected_declarations: 2, declaration_names: ['github', 'linear', 'a_really_long_mcp_server_name_with_no_spaces'] }] }], skills: [{ name: 'code-review', agent_id: 'claude', scope: 'user', source: '~/.claude/skills/code-review', files: 3, bytes: 4096, digest: 'c'.repeat(64) }], classes: [{ kind: 'credentials', selection: 'excluded', handling: 'never stored', reason: 'Secrets stay on the machine.' }], credentials_included: false, limitations: ['Project-scoped settings are not captured.'] }, software: [{ agent_id: 'claude', name: 'Claude Code', application_version: '2.0.1', installation_package: 'npm:@anthropic-ai/claude-code' }] }] }

// Home with 80-character unbroken machine, session and project names (phone-zoom.e2e.mjs): a health report for that machine,
// a session on it started in a folder project with a long base branch, so the names reach the strip, In flight and the rail.
const pad = (head, n = 80) => (head + '_abcdefghij'.repeat(8)).slice(0, n)
export const HOST80 = (LONGHOST + '.and.then.some.more.of.it.for.the.home.page.strip').slice(0, 80)
export const SESSION80 = pad('Refactor_the_store_layer_and_split_the_session_cache')
export const PROJECT80 = pad('Store_rewrite_with_a_long_unbroken_project_name')
const L = 'connector-long80000000001'
export const homeExtra = {
  '/api/v2/machines/health': () => ({ serverNow: Date.now(), machines: [{ agentId: L, host: HOST80, state: 'connected', capabilities: caps(), connection: { connected: true, reportFresh: true, missedProbes: 0, reconnects: 0, path: 'direct', report: { cpuPercent: { value: 42 }, scope: 'host' } } }, { agentId: C, host: 'pi', state: 'unreachable', capabilities: caps(), connection: { connected: false, reportFresh: false, missedProbes: 3, reconnects: 1, path: 'direct' } }] }),
  '/api/v2/terminal/sessions': { sessions: [{ id: 'session-long80', agentId: L, title: SESSION80, state: 'running', createdAt: now - 3600000, updatedAt: now - 600000, exitCode: null, projectId: 'fp80', worktreePath: `/home/dev/.reet/worktrees/store/${pad('tree', 40)}`, worktreeBranch: `reet/${pad('branch', 60)}` }, { id: 'session-pi', agentId: C, title: SESSION80.replace('Refactor', 'Offline'), state: 'running', createdAt: now - 3600000, updatedAt: now - 600000, exitCode: null }] },
  '/api/v2/terminal/machines': { machines: [{ agentId: L, host: HOST80, ready: true, serverEnabled: true, capabilities: caps() }] },
  '/api/v2/folder-projects': { projects: [fp('fp80', PROJECT80, L, HOST80, LONGPATH, { baseBranch: pad('feature/branch', 70) })], savedWaiting: 0 },
}

export async function installApi(page, { status = null, extra = {} } = {}) {
  const unmocked = []
  page.__unmocked = unmocked
  await page.route('**/api/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname, method = req.method()
    const json = (data, s = 200) => route.fulfill({ status: s, json: data })
    if (extra[path] !== undefined) { const a = typeof extra[path] === 'function' ? extra[path](req) : extra[path]; return json(a) }
    if (path === '/api/v2/auth/status') return json(status || { mode: 'accounts', setupRequired: false, bootstrapRequired: false, signupEnabled: true, user: owner, sso: { enabled: true, label: 'Company SSO' }, passwordReset: true })
    if (path === '/api/v2/profile') return json({ user: owner, sessionCount: 3, accounts: true, profile: { theme: 'system', timezone: 'Europe/London', dateFormat: '', shelfView: 'list', cardsExpanded: false, hostedPrefix: 'reet-', updatedAt: now } })
    if (path === '/api/v2/profile/model-favourites') return json({ version: 0, favourites: [] })
    if (path === '/api/v2/settings/yolo') return json({ allowed: true, changedBy: 'Owner', changedAt: now })
    if (path === '/api/v2/settings/software-mirror' && method === 'GET') return json({ enabled: false, cached: false })
    if (path === '/api/v2/software/batches/active') return json(null)
    if (/^\/api\/v2\/machines\/[^/]+\/models$/.test(path)) return json({ tool: url.searchParams.get('tool') || 'opencode', state: 'listed', models: [{ id: 'anthropic/claude-sonnet-4-5', provider: 'anthropic' }, { id: 'openai/gpt-5', provider: 'openai' }], more: false, source: 'opencode models', version: '', fetchedAt: now, checkedAt: now, reason: '', sentence: '' })
    if (/^\/api\/v2\/machines\/[^/]+\/models\/refresh$/.test(path)) return json({ tool: 'opencode', state: 'listed', models: [{ id: 'anthropic/claude-sonnet-4-5', provider: 'anthropic' }, { id: 'openai/gpt-5', provider: 'openai' }], more: false, source: 'opencode models', version: '', fetchedAt: now, checkedAt: now, reason: '', sentence: '' })
    if (path === '/api/version') return json({ version: 'v1.4.2', commit: 'fixture' })
    if (path === '/api/skills') return json({ skills, counts: { all: skills.length, used: 3, liked: 1, bookmarked: 1 } })
    if (path === '/api/skill') return json(skills[0])
    if (path === '/api/environments') return json({ environments, machines })
    if (path === '/api/tags') return json({ tags: [{ id: 1, name: 'review', source: 'user', count: 3 }, { id: 2, name: 'testing', source: 'topic', count: 2 }, { id: 3, name: 'a_really_long_tag_name_without_spaces', source: 'user', count: 1 }] })
    if (path === '/api/v2/lists' && method === 'GET') return json({ lists, linkEnabled: true, userId: 1 })
    if (/^\/api\/v2\/lists\/\d+$/.test(path)) return json(listDetail)
    if (/^\/api\/v2\/public\/lists\//.test(path)) return json({ ...listDetail, visibility: 'public', canEdit: false, canManage: false, canTransfer: false, members: undefined })
    if (path === '/api/community') return json({ lists, skills: listDetail.skills })
    if (path === '/api/v2/sync') return json(sync)
    if (path === '/api/v2/sync/candidates') return json({ skills })
    if (path === '/api/v2/setup/profiles') return json({ profiles: [{ name: 'laptop-claude', revision: 2, snapshotId: 'snap-' + 'a'.repeat(24), snapshotDigest: 'sha256:' + 'b'.repeat(64), updatedAt: now }, { name: 'a_very_long_setup_profile_name_for_the_build_server', revision: 1, snapshotId: 'snap-x', snapshotDigest: 'sha256:' + 'd'.repeat(64), updatedAt: now - DAY }] })
    if (path.startsWith('/api/v2/setup/profiles/')) return json(profileRecord)
    if (path === '/api/v2/hosting') return json(hosting)
    if (path === '/api/v2/machines/health') return json({ serverNow: Date.now(), machines: [] })
    if (path === '/api/v2/terminal/status') return json({ configured: true, available: true, reason: '' })
    if (path === '/api/v2/terminal/machines') return json({ machines: tmachines })
    if (path === '/api/v2/terminal/sessions' && method === 'GET') return json({ sessions })
    if (path === '/api/v2/terminal/commands') return json({ commands: [{ id: 'c1', name: 'Tail the app log', command: 'tail -f /var/log/app/production-application-server.log | grep --line-buffered -i error', createdAt: now }] })
    if (path.endsWith('/attachments')) return json({ ticket: 'fixture-ticket00000000000' })
    if (path.endsWith('/history')) return route.fulfill({ body: '' })
    if (path.endsWith('/command-history')) return json({ entries: [], sources: [], truncated: false, skipped: 0 })
    if (path === '/api/v2/notifications/count') return json({ unread: 3 })
    if (path === '/api/v2/notifications') return json({ notifications, unread: 3 })
    if (path === '/api/v2/notifications/preferences') return json({ preferences: prefs })
    if (path === '/api/v2/admin/notifications/defaults') return json({ defaults })
    if (path === '/api/events') return json({ events })
    if (path === '/api/ignored') return json({ ignored: [{ id: 'x/y/z', name: 'old-skill', reason: 'Not useful', at: now }] })
    if (path === '/api/v2/session-backlog' && method === 'GET') return json({ drafts: [] })
    if (path === '/api/v2/folder-projects' && method === 'GET') return json({ projects: folderProjects, savedWaiting: 0 })
    if (path === '/api/v2/project-groups' && method === 'GET') return json({ groups: [] })
    let f = path.match(/^\/api\/v2\/folder-projects\/([^/]+)$/); if (f) return json({ ...(folderProjects.find(p => p.id === f[1]) || folderProjects[0]), starterPrompt: 'Read the README and tidy the tests' })
    if (/^\/api\/v2\/machines\/[^/]+\/browse$/.test(path)) return json({ path: '/home/dev', home: '/home/dev', parent: '', repo: 'none', repoTop: '', entries: [{ name: 'work', dir: true, repoRoot: false, hidden: false }, { name: 'a_really_long_folder_name_without_any_spaces_at_all_for_the_list', dir: true, repoRoot: true, hidden: false }], more: false, partial: false, scanned: 2, omitted: 0 })
    if (path === '/api/v2/projects' && method === 'GET') return json({ projects })
    let m = path.match(/^\/api\/v2\/projects\/([^/]+)$/); if (m) return json(projects.find(p => p.id === m[1]) || projects[0])
    m = path.match(/^\/api\/v2\/projects\/([^/]+)\/runs$/); if (m) return json({ runs: runs.filter(r => r.projectId === m[1]) })
    m = path.match(/^\/api\/v2\/project-runs\/([^/]+)$/); if (m) return json(runs[0])
    if (path === '/api/v2/tickets' && method === 'GET') return json({ tickets })
    if (/^\/api\/v2\/tickets\/\d+$/.test(path)) return json(ticketDetail)
    if (path === '/api/v2/admin/tickets') return json({ tickets, counts: { open: 1, waiting: 0, closed: 1 } })
    if (/^\/api\/v2\/admin\/tickets\/\d+$/.test(path)) return json(ticketDetail)
    if (path === '/api/v2/tokens' && method === 'GET') return json({ tokens: [{ id: 1, name: 'laptop scanner', createdAt: now - DAY, lastUsedAt: now }, { id: 2, name: 'ci_pipeline_token_for_the_build_server', createdAt: now - DAY }] })
    if (path === '/api/v2/mcp') return json({ enabled: true, browserEnabled: true, browserScope: 'read', canWrite: true, endpoint: 'https://reet.example.test/api/v2/mcp', connections: [{ id: 1, name: 'Claude Desktop', scope: 'write', enabled: true, kind: 'oauth', createdAt: now - DAY, expiresAt: now + 30 * DAY, lastUsedAt: now }] })
    if (path === '/api/v2/mcp/connections') return json({ connections: [] })
    if (path === '/api/v2/mcp/browser/tools') return json({ tools: [] })
    if (path === '/api/v2/mcp/authorization') return json({ clientName: 'Claude Desktop', callbackHost: 'localhost:33418', scope: 'write', enabled: true, canWrite: true, account: 'sam@example.test' })
    if (path === '/api/v2/profile/sessions') return json({ sessions: [{ id: 's1', device: 'Chrome on Android (Pixel 7)', createdAt: now - DAY, lastSeen: now, expiresAt: now + 30 * DAY, current: true }, { id: 's2', device: 'Safari on macOS', createdAt: now - 5 * DAY, lastSeen: now - DAY, expiresAt: now + 25 * DAY, current: false }] })
    if (path === '/api/v2/auth/oidc/identity') return json({ enabled: true, label: 'Company SSO', linked: true, hasPassword: true, linkedAt: now - DAY, lastLoginAt: now, canUnlink: true })
    if (path === '/api/v2/admin/sso') return json({ enabled: true, issuer: 'https://login.example-company.test/realms/engineering', redirectUrl: 'https://reet.example.test/api/v2/auth/oidc/callback', label: 'Company SSO', passwordLogin: true, autoCreate: false, linkByEmail: true, defaultRole: 'viewer' })
    if (path === '/api/v2/settings/branch-prune') return json({ afterWorktree: true, changedBy: 'Owner', changedAt: now - DAY })
    if (path === '/api/v2/backups') return json(backupStatus(false))
    if (path === '/api/v2/admin/backups') return json(backupStatus(true))
    if (path === '/api/admin/status') return json(adminStatus)
    if (path === '/api/admin/rollup') return json(rollup)
    if (path === '/api/v2/admin/users') return json({ users })
    if (path === '/api/v2/admin/invites') return json({ invites: [{ email: 'new.person@example.test', role: 'editor', expiresAt: now + DAY }] })
    if (path === '/api/v2/admin/mail') return json({ settings: mail })
    if (path === '/api/v2/admin/mail/outbox') return json(outbox)
    if (path === '/api/v2/groups') return json({ groups: [{ id: 1, name: 'Platform', kind: 'organisation', ownerId: 1, role: 'owner', members: users.slice(0, 2) }, { id: 2, name: 'Mobile', kind: 'team', parentId: 1, ownerId: 1, role: 'owner', members: users }] })
    if (/^\/api\/v2\/groups\/\d+\/notifications$/.test(path)) return json({ notify: { groupId: 1, enabled: true, events: ['machine.offline'], recipients: 'managers', extraEmails: [] }, events: [{ key: 'machine.offline', label: 'A machine goes offline', description: 'When a connector stops reporting.' }], mailReady: true, mailMode: 'smtp', groupName: 'Platform' })
    if (path === '/api/v2/auth/invite') return json({ email: 'new.person@example.test', role: 'editor', expiresAt: now + DAY })
    if (path === '/api/v2/skill-imports') return json({ requests: [], capabilities: [{ agentId: A, ready: true, observedAt: now }] })
    if (path === '/api/v2/skill-bundles') return json({ bundles: [] })
    if (path === '/api/v2/skill-content-targets') return json({ targets: [] })
    if (path === '/api/v2/skill-content-previews') return json({ previews: [], capabilities: [{ agentId: A, ready: true, observedAt: now }] })
    if (path === '/api/v2/skill-content-receipts') return json({ receipts: [] })
    if (path === '/api/v2/terminal/worktrees/status') return json({ state: 'clean', path: '/home/dev/.reet/worktrees/site/site/tok0', branch: 'reet/site-tok0', changes: 0 })
    if (method !== 'GET') return json({ ok: true })
    unmocked.push(`${method} ${path}`)
    return json({ error: `Unmocked ${path}` }, 404)
  })
  // Terminal socket: ready, then a full-screen redraw sized to the ticket (as web/tests/terminal-workspace.e2e.mjs).
  await page.routeWebSocket('**/api/v2/terminal/socket', ws => {
    ws.onMessage(m => {
      if (typeof m !== 'string') return
      let msg; try { msg = JSON.parse(m) } catch { return }
      if (msg.ticket) { ws.send('{"type":"ready"}'); const cols = msg.cols || 80, rows = msg.rows || 24; let s = '\x1b[2J\x1b[H'; for (let r = 1; r <= rows; r++) s += `\x1b[${r};1H` + (`row ${r} opencode build/default `.repeat(4)).slice(0, cols); ws.send(Buffer.from(s)) }
    })
  })
  return unmocked
}
