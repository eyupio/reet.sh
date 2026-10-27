import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const api = await import(asURL(compile('./api.ts').replace("from '../projects'", `from '${asURL('export {}')}'`)))

// fetchReplying stubs fetch with one answer and records every request.
function fetchReplying(status, body, headers = {}) {
  const calls = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init, headers: new Headers(init.headers) })
    return new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })
  }
  return calls
}
const project = (over = {}) => ({ id: 'p1', host: 'a1b2c3', machine: 'studio', dir: '/w/reet', name: 'reet', isRepo: true, baseBranch: '', worktreeDefault: 'inherit', defaultTool: '', defaultModel: '', hasStarterPrompt: false, position: 0, createdAt: 1, updatedAt: 1, version: 1, ...over })

function cookieOnly(call) {
  assert.equal(call.init.credentials, 'same-origin')
  assert.equal(call.init.cache, 'no-store')
  assert.equal(call.headers.get('Authorization'), null)
  assert.equal(call.headers.get('X-Reet-Token'), null)
  assert.ok(!String(call.url).includes('t='), 'no token in the URL')
}

test('folder project routes and methods', async () => {
  let calls = fetchReplying(200, { projects: [project()] })
  assert.deepEqual(await api.listFolderProjects(), [project()])
  assert.equal(calls[0].url, '/api/v2/folder-projects')
  assert.equal(calls[0].init.method, undefined)
  cookieOnly(calls[0])

  calls = fetchReplying(201, project(), { ETag: '"1"' })
  const input = { host: 'a1b2c3', dir: '/w/reet', starterPrompt: 'two words' }
  await api.createFolderProject(input)
  assert.equal(calls[0].url, '/api/v2/folder-projects')
  assert.equal(calls[0].init.method, 'POST')
  assert.deepEqual(JSON.parse(calls[0].init.body), input)
  assert.equal(calls[0].headers.get('Content-Type'), 'application/json')
  assert.equal(calls[0].headers.get('If-Match'), null)
  cookieOnly(calls[0])

  calls = fetchReplying(200, project({ version: 4 }), { ETag: '"4"' })
  await api.updateFolderProject('p1', { name: 'Reet' }, 3)
  assert.equal(calls[0].url, '/api/v2/folder-projects/p1')
  assert.equal(calls[0].init.method, 'PATCH')
  assert.equal(calls[0].headers.get('If-Match'), '"3"')
  assert.deepEqual(JSON.parse(calls[0].init.body), { name: 'Reet' })
  cookieOnly(calls[0])

  calls = fetchReplying(200, { ok: true, message: 'Project removed. Its sessions and any worktrees stay on studio.' })
  assert.equal((await api.deleteFolderProject('p1', 4)).message, 'Project removed. Its sessions and any worktrees stay on studio.')
  assert.equal(calls[0].init.method, 'DELETE')
  assert.equal(calls[0].headers.get('If-Match'), '"4"')
  assert.equal(calls[0].init.body, undefined)
  cookieOnly(calls[0])

  calls = fetchReplying(200, { projects: [project({ id: 'b' }), project({ id: 'a' })] })
  const order = await api.reorderFolderProjects(['b', 'a'], [2, 1])
  assert.deepEqual(order.map(p => p.id), ['b', 'a'])
  assert.equal(calls[0].url, '/api/v2/folder-projects/reorder')
  assert.equal(calls[0].init.method, 'POST')
  assert.deepEqual(JSON.parse(calls[0].init.body), { ids: ['b', 'a'], versions: [2, 1] })
})

test('ids are encoded into one path segment', async () => {
  const calls = fetchReplying(200, project())
  await api.getFolderProject('a/../b?c#d e')
  assert.equal(calls[0].url, '/api/v2/folder-projects/a%2F..%2Fb%3Fc%23d%20e')
  assert.equal(api.folderProjectPath('../reorder'), '/api/v2/folder-projects/..%2Freorder')
})

test('the version comes from the ETag', async () => {
  fetchReplying(200, project({ version: 2, starterPrompt: 'two words' }), { ETag: '"7"' })
  const read = await api.getFolderProject('p1')
  assert.equal(read.version, 7)
  assert.equal(read.starterPrompt, 'two words')
  fetchReplying(201, project({ version: 1 }))
  assert.equal((await api.createFolderProject({ host: 'studio', dir: '/w' })).version, 1, 'body version without an ETag')
  fetchReplying(200, project({ version: 5 }), { ETag: 'W/"9"' })
  assert.equal((await api.updateFolderProject('p1', {}, 4)).version, 5, 'a weak or odd ETag is ignored')
})

test('a change without a real version is not sent', async () => {
  const calls = fetchReplying(200, project())
  for (const bad of [0, -1, 1.5, Number.NaN, undefined]) {
    await assert.rejects(api.updateFolderProject('p1', { name: 'x' }, bad))
    await assert.rejects(api.deleteFolderProject('p1', bad))
  }
  await assert.rejects(api.reorderFolderProjects(['a', 'b'], [1]))
  assert.equal(calls.length, 0)
})

test('browse URL: every value encoded, only set parameters, hidden only when true', async () => {
  assert.equal(api.browseURL('m1', { session: 's1' }), '/api/v2/machines/m1/browse?session=s1')
  const url = api.browseURL('m/1', { path: '/w/a b&c=d#e+f%', after: 'x?y', filter: 'ré', hidden: true, session: 's 1' })
  assert.ok(url.startsWith('/api/v2/machines/m%2F1/browse?'))
  const q = new URL(url, 'https://reet.example').searchParams
  assert.deepEqual([...q.keys()], ['path', 'after', 'filter', 'hidden', 'session'])
  assert.equal(q.get('path'), '/w/a b&c=d#e+f%')
  assert.equal(q.get('after'), 'x?y')
  assert.equal(q.get('filter'), 'ré')
  assert.equal(q.get('hidden'), 'true')
  assert.equal(q.get('session'), 's 1')
  assert.ok(!url.includes('#') && !url.includes(' ') && !url.includes('&c='), url)
  assert.ok(!api.browseURL('m1', { hidden: false, path: '', session: 's' }).includes('hidden'))
  assert.ok(!api.browseURL('m1', { hidden: false, path: '', session: 's' }).includes('path'))

  const calls = fetchReplying(200, { path: '/w', home: '/w', parent: '', repo: 'none', repoTop: '', entries: [], more: false, partial: false, scanned: 0, omitted: 0 })
  const result = await api.browseFolder('m1', { path: '/w', session: 's1' })
  assert.equal(result.repo, 'none')
  assert.equal(calls[0].url, '/api/v2/machines/m1/browse?path=%2Fw&session=s1')
  assert.equal(calls[0].init.method, undefined)
  cookieOnly(calls[0])
})

test('branches URL', async () => {
  assert.equal(api.branchesURL('m1', '/w/reet'), '/api/v2/machines/m1/branches?dir=%2Fw%2Freet')
  const q = new URL(api.branchesURL('m 1', '/w/a&b', 'feat/#1'), 'https://reet.example')
  assert.equal(q.pathname, '/api/v2/machines/m%201/branches')
  assert.equal(q.searchParams.get('dir'), '/w/a&b')
  assert.equal(q.searchParams.get('filter'), 'feat/#1')
  const calls = fetchReplying(200, { current: 'main', default: 'main', branches: ['main'], more: false })
  assert.deepEqual(await api.listBranches('m1', '/w', 'ma'), { current: 'main', default: 'main', branches: ['main'], more: false })
  assert.equal(calls[0].url, '/api/v2/machines/m1/branches?dir=%2Fw&filter=ma')
  cookieOnly(calls[0])
})

test('refusals keep the sentence, code and field', async () => {
  const cases = [
    [409, { code: 'project_exists', field: 'dir', error: 'studio already has a project for that folder.' }, () => api.createFolderProject({ host: 'studio', dir: '/w' }), api.FOLDER_PROJECT_ERROR_CODES],
    [409, { code: 'project_limit', error: 'You have 100 projects, the most reet keeps. Remove one first.' }, () => api.createFolderProject({ host: 'studio', dir: '/w' }), api.FOLDER_PROJECT_ERROR_CODES],
    [409, { code: 'project_changed', error: 'Someone changed this project. Reload it, then try again.' }, () => api.updateFolderProject('p1', {}, 1), api.FOLDER_PROJECT_ERROR_CODES],
    [429, { code: 'project_rate_limited', error: 'Slow down a moment, then try again.' }, () => api.deleteFolderProject('p1', 1), api.FOLDER_PROJECT_ERROR_CODES],
    [409, { code: 'browse_off', error: 'That machine has folder browsing switched off.' }, () => api.browseFolder('m1', { session: 's' }), api.BROWSE_ERROR_CODES],
    [409, { code: 'browse_unsupported', error: 'Update reet on that machine to browse its folders.' }, () => api.browseFolder('m1', { session: 's' }), api.BROWSE_ERROR_CODES],
    [409, { code: 'browse_busy', error: 'That machine is already listing a folder. Try again in a moment.' }, () => api.browseFolder('m1', { session: 's' }), api.BROWSE_ERROR_CODES],
    [429, { code: 'browse_rate_limited', error: 'Slow down a moment, then try again.' }, () => api.browseFolder('m1', { session: 's' }), api.BROWSE_ERROR_CODES],
    [504, { code: 'machine_timeout', error: "That machine didn't answer in time. Try again." }, () => api.browseFolder('m1', { session: 's' }), api.BROWSE_ERROR_CODES],
    [502, { code: 'machine_error', error: "That machine sent an answer reet didn't expect. Try again." }, () => api.listBranches('m1', '/w'), api.BRANCHES_ERROR_CODES],
    [409, { code: 'branches_unsupported', error: 'Update reet on that machine to list its branches.' }, () => api.listBranches('m1', '/w'), api.BRANCHES_ERROR_CODES],
    [409, { code: 'branches_denied', error: "That machine isn't offering its branches right now. Check that terminal access is on there and git is installed." }, () => api.listBranches('m1', '/w'), api.BRANCHES_ERROR_CODES],
    [409, { code: 'branches_busy', error: 'That machine is already listing branches. Try again in a moment.' }, () => api.listBranches('m1', '/w'), api.BRANCHES_ERROR_CODES],
    [422, { code: 'unknown_machine', field: 'host', error: "That machine isn't enrolled on this account." }, () => api.createFolderProject({ host: 'x', dir: '/w' }), api.FOLDER_PROJECT_ERROR_CODES],
    [404, { code: 'project_not_found', error: "That project isn't on this account." }, () => api.getFolderProject('gone'), api.FOLDER_PROJECT_ERROR_CODES],
  ]
  for (const [status, body, call, codes] of cases) {
    fetchReplying(status, body)
    const error = await call().then(() => assert.fail(body.code), e => e)
    assert.ok(error instanceof api.TerminalError, body.code)
    assert.equal(error.status, status)
    assert.equal(error.message, body.error)
    assert.equal(error.code, body.code)
    assert.equal(error.field, body.field)
    assert.equal(api.refusalCode(error, codes), body.code)
  }
  // A code the caller does not handle, a body that is not JSON, and a plain error map to no code.
  fetchReplying(400, { code: 'invalid_request', error: "That request isn't a project reet can read." })
  const unhandled = await api.createFolderProject({}).catch(e => e)
  assert.equal(api.refusalCode(unhandled, api.FOLDER_PROJECT_ERROR_CODES), undefined)
  assert.equal(unhandled.message, "That request isn't a project reet can read.")
  globalThis.fetch = async () => new Response('<html>', { status: 502 })
  const html = await api.browseFolder('m1', { session: 's' }).catch(e => e)
  assert.equal(html.status, 502)
  assert.equal(html.code, undefined)
  assert.equal(api.refusalCode(new Error('x'), api.BROWSE_ERROR_CODES), undefined)
  for (const code of ['off', 'not_dir']) assert.equal(api.refusalCode(new api.TerminalError(409, 'x', code), api.FOLDER_PROJECT_ERROR_CODES), undefined)
})

test('a project names its machine by enrollment id, which browse and branches take as it is', async () => {
  // The same 404 code as a terminal create naming a removed project.
  assert.ok(api.FOLDER_PROJECT_ERROR_CODES.includes('project_not_found'))
  assert.ok(!api.FOLDER_PROJECT_ERROR_CODES.includes('not_found'))
  const p = project({ host: 'a1b2c3', machine: 'studio' })
  const calls = fetchReplying(200, { current: 'main', default: 'main', branches: [], more: false })
  await api.listBranches(p.host, p.dir)
  assert.equal(calls[0].url, '/api/v2/machines/a1b2c3/branches?dir=%2Fw%2Freet')
  assert.ok(!calls[0].url.includes('studio'))
})

test('the list says how many saved repositories wait, and importing them is one POST', async () => {
  let calls = fetchReplying(200, { projects: [project()], savedWaiting: 3 })
  assert.deepEqual(await api.folderProjectList(), { projects: [project()], savedWaiting: 3 })
  assert.equal(calls[0].url, '/api/v2/folder-projects')
  cookieOnly(calls[0])

  const answer = { projects: [project()], savedWaiting: 0, created: 3, skipped: 0, message: 'Added 3 saved repositories as projects.' }
  calls = fetchReplying(200, answer)
  assert.deepEqual(await api.importSavedProjects(), answer)
  assert.equal(calls[0].url, '/api/v2/folder-projects/import-saved')
  assert.equal(calls[0].init.method, 'POST')
  assert.deepEqual(JSON.parse(calls[0].init.body), {})
  cookieOnly(calls[0])
})

test('one sentence for a refusal or a request that never arrived', () => {
  assert.equal(api.refusalSentence(new api.TerminalError(422, "reet can't read that folder.", 'denied')), "reet can't read that folder.")
  assert.equal(api.refusalSentence(new TypeError('Failed to fetch')), "That didn't reach reet. Check your connection, then try again.")
  assert.equal(api.refusalSentence(undefined), "That didn't reach reet. Check your connection, then try again.")
})
