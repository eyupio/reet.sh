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
const { runSummary, worktreeRefusalText, memberLine, branchKept, STARTED_MEANS } = p
const run = (states, state, over = {}) => ({ id: 'r1', projectId: 'p1', projectName: 'Site', state, startedAt: 1, useWorktrees: true, worktreeProject: 'site', members: states.map((s, i) => ({ position: i, host: 'a1', dir: '/r/' + i, tool: 'claude', model: '', state: s, code: s === 'failed' ? 'folder_unavailable' : '', sentence: s === 'failed' ? "That folder isn't available on that machine." : '', sessionId: s === 'started' ? 'sess' + i : '', worktreePath: '', worktreeRepo: '', worktreeBranch: '', worktreeRun: '', worktreeProject: 'site', sharesCheckout: false, attempt: 0 })), ...over })
for (const gone of ['validateProjectName', 'validateMember', 'validateMembers', 'planRun', 'resolvedModel', 'confirmSentence', 'runStateLabel', 'projectInput', 'MAX_PROJECTS', 'MAX_MEMBERS']) if (p[gone] !== undefined) throw new Error(`${gone} is still exported`)

const caps = (over = {}) => ({ protocolVersion: 1, os: 'linux', osUser: 'me', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready', launch: true, worktree: true, tools: ['claude', 'codex'], ...over })
const machine = (agentId, over = {}, capOver = {}) => ({ agentId, host: agentId + '-host', ready: true, serverEnabled: true, capabilities: caps(capOver), ...over })
const member = (host, dir, over = {}) => ({ host, dir, tool: '', model: '', position: 0, ...over })
const project = (members, over = {}) => ({ id: 'p1', name: 'Site', defaultTool: 'claude', defaultModel: '', useWorktrees: true, hasSharedPrompt: false, members: members.map((m, i) => ({ ...m, position: i })), version: 1, createdAt: 1, updatedAt: 1, ...over })

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

test('saved Projects and their runs are read for Activity, and nothing can change them from here', async () => {
  await withFetch(() => json(200, { projects: [{ id: 'p1' }] }), async requests => {
    assert.deepEqual(await client.listProjects(), [{ id: 'p1' }])
    assert.equal(requests[0].path, '/api/v2/projects')
  })
  await withFetch(() => json(200, { runs: [{ id: 'r1' }] }), async requests => {
    assert.deepEqual(await client.projectRuns('p1'), [{ id: 'r1' }])
    assert.equal(requests[0].path, '/api/v2/projects/p1/runs')
  })
  await withFetch(() => json(200, { id: 'r1' }), async requests => {
    await client.getRun('r1')
    assert.equal(requests[0].path, '/api/v2/project-runs/r1')
  })
  for (const gone of ['saveProject', 'getProject', 'deleteProject', 'startRun', 'retryMember']) assert.equal(client[gone], undefined, gone)
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
  await withFetch(() => json(404, { error: "That run isn't on this account.", code: 'run_not_found' }), async () => {
    await assert.rejects(() => client.getRun('r9'), e => e.status === 404 && e.code === 'run_not_found' && e.field === undefined)
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

test('Remove worktree is never offered while the sessions are unknown: not read yet, or the read failed', () => {
  const { removeOffer } = p
  const m = { position: 0, host: 'a1', dir: '/r', tool: 'claude', model: '', state: 'started', code: '', sentence: '', sessionId: 's1', worktreePath: '/w', worktreeRepo: 'r', worktreeBranch: 'reet/p-t', worktreeRun: 't', worktreeProject: 'p', sharesCheckout: false, attempt: 1 }
  assert.deepEqual(removeOffer(m, undefined), { offer: false, reason: 'Checking whether its session has ended…' })
  assert.deepEqual(removeOffer(m, undefined, true), { offer: false, reason: "Couldn't check whether its session has ended, so its worktree can't be removed from here just now. Reload the page to try again." })
  // A failed read wins over an old list.
  assert.equal(removeOffer(m, [{ id: 's1', agentId: 'a1', state: 'ended' }], true).offer, false)
  assert.equal(removeOffer({ ...m, state: 'failed' }, undefined).reason, '')
})

test('a machine is named by its host once the machines are read, by its id until then, and plainly when it has left', () => {
  const { hostLabel } = p
  const machines = [machine('a1')]
  assert.equal(hostLabel('a1', machines), 'a1-host')
  assert.equal(hostLabel('a1', undefined), 'a1')
  assert.equal(hostLabel('gone', machines), 'A machine no longer on this account')
})

test("a saved folder is cleaned as the server cleans it, so a run's folder matches its project", () => {
  const { cleanPath } = p
  for (const [dir, clean] of [['/a//b', '/a/b'], ['/a/./b/', '/a/b'], ['/a/c/../b', '/a/b'], ['/../a', '/a'], ['/', '/'], ['/home/dev//./store/', '/home/dev/store']]) assert.equal(cleanPath(dir), clean, dir)
})
