import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const launchURL = asURL(compile('./agentLaunch.ts'))
const apiURL = asURL(compile('./terminal/api.ts').replace("from '../projects'", `from '${asURL('export {}')}'`))
const p = await import(asURL(compile('./projects.ts').replace("from './agentLaunch'", `from '${launchURL}'`).replace("from './terminal/api'", `from '${apiURL}'`)))
const launch = await import(launchURL)
const { validateProjectName, validateMember, validateMembers, planRun, runSummary, worktreeRefusalText, memberLine, confirmSentence, branchKept, STARTED_MEANS } = p

const caps = (over = {}) => ({ protocolVersion: 1, os: 'linux', osUser: 'me', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready', launch: true, worktree: true, tools: ['claude', 'codex'], ...over })
const machine = (agentId, over = {}, capOver = {}) => ({ agentId, host: agentId + '-host', ready: true, serverEnabled: true, capabilities: caps(capOver), ...over })
const member = (host, dir, over = {}) => ({ host, dir, tool: '', model: '', position: 0, ...over })
const project = (members, over = {}) => ({ id: 'p1', name: 'Site', defaultTool: 'claude', defaultModel: '', useWorktrees: true, hasSharedPrompt: false, members: members.map((m, i) => ({ ...m, position: i })), version: 1, createdAt: 1, updatedAt: 1, ...over })

test('project names are 1 to 80 characters of plain text, unique ignoring case', () => {
  assert.equal(validateProjectName('Site'), null)
  assert.equal(validateProjectName('a'.repeat(80)), null)
  assert.equal(validateProjectName('é'.repeat(80)), null)
  assert.equal(validateProjectName(''), 'Give the Project a name of 1 to 80 characters, without line breaks or control characters.')
  assert.equal(validateProjectName('   '), 'Give the Project a name of 1 to 80 characters, without line breaks or control characters.')
  assert.notEqual(validateProjectName('a'.repeat(81)), null)
  assert.notEqual(validateProjectName('two\nlines'), null)
  assert.notEqual(validateProjectName('bell\u0007'), null)
  assert.notEqual(validateProjectName('zero​width'), null)
  assert.equal(validateProjectName('Site', ['Other']), null)
  assert.equal(validateProjectName('  site ', ['SITE']), 'Another Project on this account already has that name.')
})

test('a member needs an enrolled machine, an absolute folder, a known tool and a usable model', () => {
  const machines = [machine('a1')]
  assert.equal(validateMember(member('a1', '/home/me/site'), machines, []), null)
  assert.equal(validateMember(member('a1', '/home/me/site', { tool: 'codex', model: 'gpt-5-codex' }), machines, []), null)
  assert.equal(validateMember(member('zz', '/home/me/site'), machines, []), "That machine isn't enrolled on this account.")
  assert.equal(validateMember(member('', '/home/me/site'), machines, []), 'Choose a machine.')
  assert.equal(validateMember(member('a1', ''), machines, []), 'Choose a project folder.')
  assert.equal(validateMember(member('a1', 'site'), machines, []), 'Use a full path that starts with /.')
  assert.equal(validateMember(member('a1', '/x\u0000y'), machines, []), 'Use a folder path without control or invisible characters.')
  assert.equal(validateMember(member('a1', '/a', { tool: 'vim' }), machines, []), 'Choose one of the agents reet can start.')
  assert.equal(validateMember(member('a1', '/a', { model: '-bad' }), machines, []), launch.validateModel('-bad'))
})

test('a Project holds at most 12 members and each machine and folder once', () => {
  const ok = Array.from({ length: 12 }, (_, i) => member('a1', `/r/${i}`))
  assert.equal(validateMembers(ok), null)
  assert.equal(validateMembers([...ok, member('a1', '/r/x')]), 'A Project can have at most 12 repositories.')
  assert.equal(validateMembers([member('a1', '/r/x'), member('a1', '/r/x/')]), 'Each machine and folder can be in a Project only once.')
  assert.equal(validateMembers([member('a1', '/r/x'), member('a2', '/r/x')]), null)
  // The same folder written another way is the same folder, as the server's path.Clean sees it.
  for (const alias of ['/r//x', '/r/./x', '/r/y/../x', '/r/x/.', '//r/x', '/../r/x']) assert.equal(validateMembers([member('a1', '/r/x'), member('a1', alias)]), 'Each machine and folder can be in a Project only once.', alias)
  assert.equal(validateMembers([member('a1', '/'), member('a1', '/r/..')]), 'Each machine and folder can be in a Project only once.')
  assert.equal(validateMembers([member('a1', '/r/x'), member('a1', '/r/xx')]), null)
  assert.equal(validateMembers([member('a1', '/r/x'), member(' A1 ', '/r/x')]), 'Each machine and folder can be in a Project only once.')
})

test('planRun starts a ready, launch-capable member with the tool installed, and resolves defaults', () => {
  const proj = project([member('a1', '/r/one'), member('a1', '/r/two', { tool: 'codex', model: 'gpt-5-codex' })], { defaultModel: 'sonnet' })
  const plan = p.planRun(proj, [machine('a1')], undefined, [])
  assert.deepEqual(plan.skipped, [])
  assert.equal(plan.start.length, 2)
  assert.deepEqual(plan.start.map(s => [s.position, s.tool, s.model, s.worktree, s.dir, s.host, s.machine]), [[0, 'claude', 'sonnet', true, '/r/one', 'a1', 'a1-host'], [1, 'codex', 'gpt-5-codex', true, '/r/two', 'a1', 'a1-host']])
  assert.equal(plan.start[0].toolName, 'Claude Code')
})

test('planRun skips with each reason, in order, and names it in plain words', () => {
  const skip = (proj, machines, sessions = []) => planRun(proj, machines, undefined, sessions).skipped.map(s => [s.code, s.reason])
  assert.deepEqual(skip(project([member('nope', '/r')]), [machine('a1')]), [['not_enrolled', "That machine isn't enrolled on this account."]])
  assert.deepEqual(skip(project([member('a1', '/r')]), [machine('a1', { ready: false })]), [['not_ready', 'The machine is reconnecting. Existing shells will be kept.']])
  assert.deepEqual(skip(project([member('a1', '/r')]), [machine('a1', { serverEnabled: false })])[0][0], 'not_ready')
  assert.deepEqual(skip(project([member('a1', '/r')]), [machine('a1', { capabilities: undefined, ready: false })])[0], ['not_ready', 'Start the background agent on this machine to connect.'])
  assert.deepEqual(skip(project([member('a1', '/r')]), [machine('a1', {}, { launch: false })]), [['no_launch', 'Update reet on that machine to start agents.']])
  assert.deepEqual(skip(project([member('a1', '/r', { tool: 'gemini' })]), [machine('a1')]), [['tool_not_installed', "That agent isn't installed on that machine."]])
  assert.deepEqual(skip(project([member('a1', '/r')], { defaultTool: 'opencode' }), [machine('a1')]), [['tool_not_installed', "That agent isn't installed on that machine."]])
  assert.deepEqual(skip(project([member('a1', '/r')]), [machine('a1', {}, { worktree: false })]), [['no_worktree', 'Update reet on that machine to start agents with worktrees.']])
  // The same machine is fine when the Project makes no worktrees.
  assert.deepEqual(planRun(project([member('a1', '/r')], { useWorktrees: false }), [machine('a1', {}, { worktree: false })], undefined, []).skipped, [])
  // Per-machine limit is 8 open sessions; closed sessions do not count.
  const open = (agentId, n, state = 'running') => Array.from({ length: n }, (_, i) => ({ id: `${agentId}-${state}-${i}`, agentId, state }))
  assert.deepEqual(skip(project([member('a1', '/r')]), [machine('a1')], open('a1', 8)), [['machine_session_limit', 'That machine already has 8 sessions, the most it can have. End one, then start again.']])
  assert.deepEqual(skip(project([member('a1', '/r')]), [machine('a1')], [...open('a1', 7), ...open('a1', 5, 'ended'), ...open('a1', 2, 'lost'), ...open('a1', 1, 'exited')]), [])
  assert.deepEqual(skip(project([member('a1', '/r')]), [machine('a1')], [...open('a1', 7, 'creating'), ...open('a1', 1, 'end_requested')])[0][0], 'machine_session_limit')
  // Per-account limit is 32 open sessions.
  const others = [...open('b1', 8), ...open('b2', 8), ...open('b3', 8), ...open('b4', 8)]
  assert.deepEqual(skip(project([member('a1', '/r')]), [machine('a1')], others), [['account_session_limit', 'This account already has 32 sessions, the most it can have. End one, then start again.']])
})

test('planRun counts the sessions this run would add, so later members are skipped at the limits', () => {
  const members = Array.from({ length: 4 }, (_, i) => member('a1', `/r/${i}`))
  const sessions = Array.from({ length: 6 }, (_, i) => ({ id: 's' + i, agentId: 'a1', state: 'running' }))
  const plan = planRun(project(members), [machine('a1')], undefined, sessions)
  assert.deepEqual(plan.start.map(s => s.position), [0, 1])
  assert.deepEqual(plan.skipped.map(s => [s.member.dir, s.code]), [['/r/2', 'machine_session_limit'], ['/r/3', 'machine_session_limit']])
  const machines = ['a1', 'a2', 'a3', 'a4', 'a5'].map(a => machine(a))
  const busy = [...['a1', 'a2', 'a3'].flatMap(a => Array.from({ length: 8 }, (_, i) => ({ id: a + i, agentId: a, state: 'running' }))), ...Array.from({ length: 7 }, (_, i) => ({ id: 'd' + i, agentId: 'a4', state: 'running' }))]
  const plan2 = planRun(project([member('a5', '/r/0'), member('a5', '/r/1'), member('a5', '/r/2')]), machines, undefined, busy)
  assert.deepEqual(plan2.start.map(s => s.position), [0])
  assert.deepEqual(plan2.skipped.map(s => s.code), ['account_session_limit', 'account_session_limit'])
})

test('planRun keeps each skipped member whole, and an empty Project plans nothing', () => {
  const plan = planRun(project([member('a1', '/r/one'), member('zz', '/r/two')]), [machine('a1')], undefined, [])
  assert.equal(plan.start.length, 1)
  assert.equal(plan.skipped[0].member.dir, '/r/two')
  assert.equal(plan.skipped[0].position, 1)
  assert.deepEqual(planRun(project([]), [], undefined, []), { start: [], skipped: [] })
  // The session limits can only be applied from the account's sessions, so a call without them is refused.
  assert.throws(() => planRun(project([member('a1', '/r')]), [machine('a1')], undefined))
})

test('the confirmation sentence says what starts, that nothing runs until Start, and where the prompt shows', () => {
  assert.equal(confirmSentence(2, false, true), 'Starts 2 agents, each in its own git worktree and branch under ~/.reet/worktrees on its machine. reet never removes a worktree by itself. Nothing runs until you press Start.')
  assert.equal(confirmSentence(1, true, false), 'Starts 1 agent in the saved folder itself, so any other session there shares that checkout. Nothing runs until you press Start. The prompt is kept with the session record and is visible to other users of that machine in its process list.')
  assert.equal(confirmSentence(0, false, true), 'No repository can start. Nothing runs until you press Start.')
})

const run = (states, state, over = {}) => ({ id: 'r1', projectId: 'p1', projectName: 'Site', state, startedAt: 1, useWorktrees: true, worktreeProject: 'site', members: states.map((s, i) => ({ position: i, host: 'a1', dir: '/r/' + i, tool: 'claude', model: '', state: s, code: s === 'failed' ? 'folder_unavailable' : '', sentence: s === 'failed' ? "That folder isn't available on that machine." : '', sessionId: s === 'started' ? 'sess' + i : '', worktreePath: '', worktreeRepo: '', worktreeBranch: '', worktreeRun: '', worktreeProject: 'site', sharesCheckout: false, attempt: 0 })), ...over })

test('run summaries say exactly what happened and never claim more than started', () => {
  assert.equal(runSummary(run(['started', 'started', 'started'], 'complete')), 'All 3 started.')
  assert.equal(runSummary(run(['started'], 'complete')), 'Started.')
  assert.equal(runSummary(run(['started', 'failed', 'started'], 'partial')), '2 of 3 started, 1 failed.')
  assert.equal(runSummary(run(['failed', 'failed'], 'failed')), 'None of the 2 started.')
  assert.equal(runSummary(run(['failed'], 'failed')), "That one didn't start.")
  assert.equal(runSummary(run(['started', 'pending', 'pending'], 'running')), 'Starting: 1 of 3 started so far.')
  assert.equal(runSummary(run(['started', 'failed', 'pending'], 'running')), 'Starting: 1 of 3 started so far, 1 failed.')
  assert.equal(runSummary(run(['started', 'removed'], 'complete')), 'All 2 started.')
  assert.equal(STARTED_MEANS, "Started means the machine started the program in tmux in that folder. It doesn't mean the agent is signed in or making progress.")
  for (const s of [runSummary(run(['started'], 'complete')), STARTED_MEANS]) assert.doesNotMatch(s, /verified|healthy/i)
})

test('a member line gives its state in words', () => {
  const r = run(['pending', 'started', 'failed', 'removed'], 'running')
  assert.deepEqual(r.members.map(memberLine), ['Waiting to start.', 'Started.', "That folder isn't available on that machine.", 'Started. Its worktree has been removed.'])
  const odd = run(['failed'], 'failed'); odd.members[0].sentence = ''
  assert.equal(memberLine(odd.members[0]), 'Something changed. Check again.')
})

test('worktree refusals use the spec sentences', () => {
  const t = [
    [422, 'not_repo_root', "That folder isn't the top of a git repository."],
    [422, 'repo_busy', 'That repository is in the middle of a merge, rebase or similar. Finish or abort it first.'],
    [422, 'worktree_exists', "A worktree or branch with this run's name is already on that machine, so nothing was started."],
    [422, 'outside_home', 'The folder ~/.reet/worktrees on that machine is a link, belongs to another user or can be changed by others. Fix it, then try again.'],
    [422, 'not_registered', "Git doesn't list that folder as this run's worktree, so reet won't touch it."],
    [422, 'has_changes', "This worktree has changes you haven't saved. Commit, stash or discard them in the worktree, then check again."],
    [422, 'unreferenced_commits', 'Some commits in this worktree are not on any branch. Put them on a branch first, then check again.'],
    [422, 'nested_repository', 'This worktree contains another git repository. Move or remove it first.'],
    [422, 'hidden_changes', 'Git is not tracking some edits in this worktree. Review them before removing.'],
    [422, 'check_incomplete', "reet couldn't check all of this worktree. Remove it yourself."],
    [409, 'worktree_changed', 'That worktree changed since you looked. Check again.'],
    [409, 'worktree_unsupported', 'Update reet on that machine to start agents with worktrees.'],
    [404, 'not_found', "That worktree isn't on that machine. It may have been removed already."],
    [409, 'worktree_busy', 'That machine is already working on a worktree. Try again in a minute.'],
    [503, 'worktree_unavailable', "That machine couldn't do that just now. Try again in a moment."],
    [503, 'worktree_mismatch', "That machine's answer did not match the request. Check again."],
  ]
  for (const [status, code, text] of t) assert.equal(worktreeRefusalText(status, code), text, code)
  assert.equal(worktreeRefusalText(422, 'git_failed', "Git couldn't check that worktree."), "Git couldn't check that worktree.")
  assert.equal(worktreeRefusalText(422, 'git_failed', "Git couldn't remove that worktree."), "Git couldn't remove that worktree.")
  assert.equal(worktreeRefusalText(422, 'git_failed'), "Git couldn't make a worktree there.")
  assert.equal(worktreeRefusalText(422, 'worktree_refused', 'That machine refused to remove the worktree.'), 'That machine refused to remove the worktree.')
  assert.equal(worktreeRefusalText(401), "You've been signed out. Sign in again to continue.")
  assert.equal(worktreeRefusalText(500, 'mystery'), 'Something changed. Check again.')
  assert.equal(branchKept('reet/site-abc'), 'The branch reet/site-abc is kept.')
})

test('refusalText has a sentence for every code Projects and runs can answer with', () => {
  const { refusalText } = launch
  const t = [
    [422, 'unknown_position', 'Choose repositories that are in this Project, each once.'],
    [409, 'not_failed', 'Only a repository that failed can be tried again.'],
    [403, 'terminals_disabled', "Remote terminals are switched off on this server, so agents can't be started."],
    [409, 'unknown_outcome', "We didn't hear back, so the agent may have started. Check your sessions before retrying."],
    [409, 'session_lost', 'That machine lost the session before it started. Start it again.'],
    [409, 'machine_refused', 'That machine refused to start the agent. Check the machine, then try again.'],
    [409, 'invalid_launch', "reet couldn't send that start request. Check the Project's settings and try again."],
    [409, 'request_changed', 'That request changed. Start it again.'],
    [422, 'request_id_required', 'Starting a run needs a request id: 16 to 128 letters, digits, hyphens or underscores.'],
    [409, 'run_in_progress', 'This Project is still starting. Wait for it to finish, then start again.'],
    [500, 'internal_error', 'Something went wrong on our side. Check your sessions, then retry.'],
    [409, 'worktree_busy', 'That machine is already working on a worktree. Try again in a minute.'],
    [503, 'worktree_unavailable', "That machine couldn't do that just now. Try again in a moment."],
    [503, 'worktree_mismatch', "That machine's answer did not match the request. Check again."],
    [409, 'project_changed', 'Someone changed this Project. Reload it and try again.'],
    [422, 'not_repo_root', "That folder isn't the top of a git repository."],
    [422, 'has_changes', "This worktree has changes you haven't saved. Commit, stash or discard them in the worktree, then check again."],
    [409, 'name_taken', 'Another Project on this account already has that name.'],
    [409, 'project_limit', 'This account already has 50 Projects, the most it can have. Delete one first.'],
    [404, 'run_not_found', "That run isn't on this account."],
    [400, 'invalid_version', "reet couldn't tell which version of this Project you're changing. Reload it and try again."],
    [400, 'invalid_request', "reet couldn't read that request. Reload the page and try again."],
    [413, 'too_large', 'That request is too large for a Project. Shorten the shared prompt or the folder paths.'],
    [422, 'invalid_project', "Something in that Project can't be saved. Check each field and try again."],
    [409, 'member_state', "That repository can't do that in the state it is in now. Reload the run."],
    [409, 'member_stale', 'Something changed. Check again.'],
    [500, 'server_error', "reet couldn't do that just now. Try again."],
    [403, 'accounts_required', 'Projects need an account signed in through the browser.'],
    [403, 'read_only', "This account is read-only, so it can't use Projects."],
    [403, 'origin_refused', "That request didn't come from this reet's own address. Open reet from its usual address and try again."],
  ]
  for (const [status, code, text] of t) assert.equal(refusalText(status, code), text, code)
  // Existing behaviour is untouched.
  assert.equal(refusalText(409, 'launch_unsupported'), 'Update reet on that machine to start agents.')
  assert.equal(refusalText(403, undefined, 'Terminals are disabled by the server'), 'Browser terminals are switched off on this server.')
  assert.equal(refusalText(503), 'That machine is reconnecting. Try again in a moment.')
})

// The client: every update and delete carries If-Match, every start carries a request id, and 200 and 201 read the same.
const client = await import(apiURL)
async function withFetch(reply, body) {
  const original = globalThis.fetch, requests = []
  globalThis.fetch = async (path, init) => { requests.push({ path, init }); return reply(path, init) }
  try { await body(requests) } finally { globalThis.fetch = original }
}
const json = (status, value) => new Response(JSON.stringify(value), { status })
const headerOf = (r, name) => new Headers(r.init.headers).get(name)

test('listing, reading and saving a Project use the right routes, and an update always sends If-Match', async () => {
  await withFetch(() => json(200, { projects: [{ id: 'p1' }] }), async requests => {
    assert.deepEqual(await client.listProjects(), [{ id: 'p1' }])
    assert.equal(requests[0].path, '/api/v2/projects')
  })
  await withFetch(() => json(201, { id: 'p2', version: 1 }), async requests => {
    assert.deepEqual(await client.saveProject({ name: 'Site' }), { id: 'p2', version: 1 })
    assert.equal(requests[0].init.method, 'POST')
    assert.equal(requests[0].path, '/api/v2/projects')
    assert.equal(headerOf(requests[0], 'If-Match'), null)
    assert.deepEqual(JSON.parse(requests[0].init.body), { name: 'Site' })
  })
  await withFetch(() => json(200, { id: 'p1', version: 8 }), async requests => {
    await client.saveProject({ name: 'Renamed' }, { id: 'p/1', version: 7 })
    assert.equal(requests[0].init.method, 'PATCH')
    assert.equal(requests[0].path, '/api/v2/projects/p%2F1')
    assert.equal(headerOf(requests[0], 'If-Match'), '"7"')
    assert.equal(requests[0].init.signal instanceof AbortSignal, true)
  })
  await withFetch(() => json(200, { id: 'p1', version: 3 }), async requests => {
    assert.equal((await client.getProject('p1')).version, 3)
    assert.equal(requests[0].path, '/api/v2/projects/p1')
  })
})

test('deleting a Project sends If-Match with the version that was read', async () => {
  await withFetch(() => json(200, { ok: true, mayHaveWorktrees: true, message: 'Project deleted. Its runs are kept.' }), async requests => {
    const out = await client.deleteProject({ id: 'p1', version: 4 })
    assert.equal(out.mayHaveWorktrees, true)
    assert.equal(requests[0].init.method, 'DELETE')
    assert.equal(headerOf(requests[0], 'If-Match'), '"4"')
  })
})

test('starting a run always sends a request id and positions, and 200 and 201 give the same run', async () => {
  for (const status of [200, 201]) {
    await withFetch(() => json(status, { id: 'r1', state: 'running', members: [] }), async requests => {
      const out = await client.startRun('p1', [0, 2], 'a'.repeat(32))
      assert.equal(out.id, 'r1')
      assert.equal(requests[0].path, '/api/v2/projects/p1/runs')
      assert.equal(requests[0].init.method, 'POST')
      assert.deepEqual(JSON.parse(requests[0].init.body), { requestId: 'a'.repeat(32), positions: [0, 2] })
    })
  }
  await assert.rejects(() => client.startRun('p1', [0], ''), /request id/i)
  await assert.rejects(() => client.startRun('p1', [], 'a'.repeat(32)), /at least one/i)
})

test('runs are read and a member retried by position', async () => {
  await withFetch(() => json(200, { runs: [{ id: 'r1' }] }), async requests => {
    assert.deepEqual(await client.projectRuns('p1'), [{ id: 'r1' }])
    assert.equal(requests[0].path, '/api/v2/projects/p1/runs')
  })
  await withFetch(() => json(200, { id: 'r1' }), async requests => {
    await client.getRun('r1')
    assert.equal(requests[0].path, '/api/v2/project-runs/r1')
    await client.retryMember('r1', 3)
    assert.equal(requests[1].path, '/api/v2/project-runs/r1/members/3/retry')
    assert.equal(requests[1].init.method, 'POST')
    assert.equal(requests[1].init.body, '{}')
  })
})

test('worktree status and remove name the run, and remove states what was shown', async () => {
  const target = { agentId: 'a1', project: 'site', repo: 'app', run: 'abc123abc123abcd' }
  await withFetch(() => json(200, { path: '/p', branch: 'reet/site-abc', changed: 0, unpushed: 1, paths: [], unreferenced: 0, hidden: 0, nested: false, incomplete: false }), async requests => {
    assert.equal((await client.worktreeStatus(target)).unpushed, 1)
    assert.equal(requests[0].path, '/api/v2/terminal/worktrees/status')
    assert.deepEqual(JSON.parse(requests[0].init.body), target)
    await client.removeWorktree(target, { changed: 0, unpushed: 1 })
    assert.equal(requests[1].path, '/api/v2/terminal/worktrees/remove')
    assert.deepEqual(JSON.parse(requests[1].init.body), { ...target, changed: 0, unpushed: 1 })
  })
})

test('a refusal carries its status, code and git detail', async () => {
  await withFetch(() => json(422, { error: "Git couldn't remove that worktree.", code: 'git_failed', message: "Git couldn't remove that worktree.", detail: 'fatal: nope' }), async () => {
    await assert.rejects(() => client.removeWorktree({ agentId: 'a', project: 's', repo: 'r', run: 'abc123abc123abcd' }, { changed: 0, unpushed: 0 }), e => e.status === 422 && e.code === 'git_failed' && e.detail === 'fatal: nope')
  })
  await withFetch(() => json(409, { error: 'x', code: 'project_changed' }), async () => {
    await assert.rejects(() => client.saveProject({ name: 'n' }, { id: 'p1', version: 1 }), e => e.status === 409 && e.code === 'project_changed' && e.field === undefined)
  })
  await withFetch(() => json(422, { error: 'Repository 2: …', code: 'invalid_dir', field: 'members.1.dir' }), async () => {
    await assert.rejects(() => client.saveProject({ name: 'n' }), e => e.status === 422 && e.code === 'invalid_dir' && e.field === 'members.1.dir')
  })
})

test('a field refusal that names a repository keeps the server sentence; anything else uses refusalText', () => {
  const { projectRefusalText } = p
  assert.equal(projectRefusalText(422, 'invalid_dir', 'Repository 2: the folder must be an absolute path starting with /, at most 4096 bytes, without control characters or line separators.'), 'Repository 2: the folder must be an absolute path starting with /, at most 4096 bytes, without control characters or line separators.')
  assert.equal(projectRefusalText(422, 'unknown_machine', "Repository 1: that machine isn't enrolled on this account."), "Repository 1: that machine isn't enrolled on this account.")
  assert.equal(projectRefusalText(422, 'invalid_tool', 'Repository 3: choose one of the agents reet can start.'), 'Repository 3: choose one of the agents reet can start.')
  assert.equal(projectRefusalText(422, 'invalid_model', 'Repository 3: that model name…'), 'Repository 3: that model name…')
  assert.equal(projectRefusalText(422, 'invalid_prompt', 'The shared prompt must be valid text.'), 'The shared prompt must be valid text.')
  // Without a message, or for another code, the fixed sentences.
  assert.equal(projectRefusalText(422, 'invalid_tool', ''), 'Something changed. Check again.')
  assert.equal(projectRefusalText(409, 'name_taken', 'anything'), 'Another Project on this account already has that name.')
  assert.equal(projectRefusalText(409, 'project_changed', 'whatever'), 'Someone changed this Project. Reload it and try again.')
  // A server message with control characters is not shown.
  assert.equal(projectRefusalText(422, 'invalid_dir', 'bad\u0007'), 'Something changed. Check again.')
})

const state = (over = {}) => ({ path: '/home/me/.reet/worktrees/site/app/abc', branch: 'reet/site-abc', changed: 0, unpushed: 0, paths: [], unreferenced: 0, hidden: 0, nested: false, incomplete: false, ...over })

test('a worktree can be removed only when nothing blocks it, and changes always block', () => {
  const { worktreeBlockers } = p
  assert.deepEqual(worktreeBlockers(state()), [])
  assert.deepEqual(worktreeBlockers(state({ unpushed: 3 })), [])
  assert.deepEqual(worktreeBlockers(state({ changed: 1, paths: ['a.txt'] })), ["This worktree has changes you haven't saved. Commit, stash or discard them in the worktree, then check again."])
  assert.deepEqual(worktreeBlockers(state({ unreferenced: 2 })), ['Some commits in this worktree are not on any branch. Put them on a branch first, then check again.'])
  assert.deepEqual(worktreeBlockers(state({ hidden: 1 })), ['Git is not tracking some edits in this worktree. Review them before removing.'])
  assert.deepEqual(worktreeBlockers(state({ nested: true })), ['This worktree contains another git repository. Move or remove it first.'])
  assert.deepEqual(worktreeBlockers(state({ incomplete: true })), ["reet couldn't check all of this worktree. Remove it yourself."])
  assert.equal(worktreeBlockers(state({ changed: 4, unreferenced: 1, hidden: 1, nested: true, incomplete: true })).length, 5)
})

test('the remove confirmation states the counts that were shown and what is deleted and kept', () => {
  const { removeConfirmText } = p
  assert.equal(removeConfirmText(state()), 'Remove the worktree at /home/me/.reet/worktrees/site/app/abc? It has no uncommitted changes and no unpushed commits. Ignored files in it, such as .env or node_modules, are deleted with the folder. The branch reet/site-abc is kept.')
  assert.equal(removeConfirmText(state({ unpushed: 1 })), 'Remove the worktree at /home/me/.reet/worktrees/site/app/abc? It has no uncommitted changes and 1 unpushed commit, which stays on the branch. Ignored files in it, such as .env or node_modules, are deleted with the folder. The branch reet/site-abc is kept.')
  assert.equal(removeConfirmText(state({ unpushed: 4 })).includes('4 unpushed commits, which stay on the branch'), true)
})

test('Remove worktree is offered only for a started member with a worktree whose session has ended', () => {
  const { removeOffer } = p
  const m = (over = {}) => ({ position: 0, host: 'a1', dir: '/r', tool: 'claude', model: '', state: 'started', code: '', sentence: '', sessionId: 's1', worktreePath: '/w', worktreeRepo: 'r', worktreeBranch: 'reet/p-t', worktreeRun: 't', worktreeProject: 'p', sharesCheckout: false, attempt: 1, ...over })
  assert.deepEqual(removeOffer(m(), [{ id: 's1', agentId: 'a1', state: 'ended' }]), { offer: true, reason: '' })
  // A session the list no longer has has ended too.
  assert.deepEqual(removeOffer(m(), []), { offer: true, reason: '' })
  for (const st of ['creating', 'running', 'end_requested']) assert.deepEqual(removeOffer(m(), [{ id: 's1', agentId: 'a1', state: st }]), { offer: false, reason: 'End its session before removing its worktree.' }, st)
  assert.equal(removeOffer(m({ state: 'pending' }), []).offer, false)
  assert.equal(removeOffer(m({ state: 'failed' }), []).offer, false)
  assert.equal(removeOffer(m({ state: 'removed' }), []).offer, false)
  assert.equal(removeOffer(m({ worktreeRun: '', worktreeRepo: '', sharesCheckout: true }), []).offer, false)
  assert.equal(removeOffer(m({ worktreeRepo: '' }), []).offer, false)
})

test('a run is waited on while any member is pending', () => {
  const { runPending } = p
  assert.equal(runPending({ members: [{ state: 'started' }, { state: 'pending' }] }), true)
  assert.equal(runPending({ members: [{ state: 'started' }, { state: 'failed' }, { state: 'removed' }] }), false)
})

test('a run state is named in plain words', () => {
  const { runStateLabel } = p
  assert.equal(runStateLabel('running'), 'Starting')
  assert.equal(runStateLabel('complete'), 'All started')
  assert.equal(runStateLabel('partial'), 'Some failed')
  assert.equal(runStateLabel('failed'), 'None started')
  assert.equal(runStateLabel('odd'), 'odd')
})

test("a member that switches agent without naming a model gets that agent's own default, as the server resolves it", () => {
  const proj = project([member('a1', '/r/0'), member('a1', '/r/1', { tool: 'codex' }), member('a1', '/r/2', { tool: 'codex', model: 'gpt-5-codex' }), member('a1', '/r/3', { tool: 'claude' }), member('a1', '/r/4', { model: 'haiku' })], { defaultModel: 'opus' })
  assert.deepEqual(planRun(proj, [machine('a1')], undefined, []).start.map(s => s.model), ['opus', '', 'gpt-5-codex', 'opus', 'haiku'])
  assert.equal(p.resolvedModel({ defaultTool: 'claude', defaultModel: '' }, { tool: '', model: '' }), '')
})
