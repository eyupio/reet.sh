import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const catalogue = asURL(compile('./catalogue.ts'))
const modelSource = compile('./models.ts').replace("from './catalogue'", `from '${catalogue}'`)
const {
  providerOf, dedupeModels, recentModels, modelOptions, resolveLaunchDefaults,
  modelStatus, formatAge, effortChoices, moveFavourite, favouriteKey, starLabel,
} = await import(asURL(modelSource))
const terminal = await import(asURL(compile('./terminal/api.ts').replace("from '../projects'", `from '${asURL('export {}')}'`)))
const api = await import(asURL(compile('./api.ts')))

test('providers, deduplication and recent models', () => {
  assert.equal(providerOf('anthropic/claude-sonnet-4-5'), 'anthropic')
  assert.equal(providerOf('local-model'), '')
  assert.equal(providerOf('/leading'), '')
  assert.equal(providerOf(''), '')

  const entries = [
    { id: 'anthropic/one', provider: 'anthropic' },
    { id: 'openai/two', provider: 'openai' },
    { id: 'anthropic/one', provider: 'changed' },
    { id: 'three', provider: '' },
  ]
  assert.deepEqual(dedupeModels(entries), [
    { id: 'anthropic/one', provider: 'anthropic' },
    { id: 'openai/two', provider: 'openai' },
    { id: 'three', provider: '' },
  ])
  assert.deepEqual(entries.map(e => e.id), ['anthropic/one', 'openai/two', 'anthropic/one', 'three'])

  const sessions = [
    { tool: 'opencode', model: 'old', createdAt: 1 },
    { tool: 'opencode', model: 'newest', createdAt: 30 },
    { tool: '', model: 'shell', createdAt: 40 },
    { tool: 'opencode', model: '', createdAt: 50 },
    { tool: 'claude', model: 'claude-model', createdAt: 60 },
    { tool: 'opencode', model: 'newest', createdAt: 20 },
    { tool: 'opencode', model: ' middle ', createdAt: 10 },
  ]
  assert.deepEqual(recentModels(sessions, 'opencode'), ['newest', 'middle', 'old'])
  assert.deepEqual(recentModels(sessions, 'opencode', 2), ['newest', 'middle'])
  assert.deepEqual(recentModels(sessions, 'opencode', 0), [])
  assert.deepEqual(recentModels([], 'opencode'), [])
})

const machine = {
  tool: 'opencode', state: 'listed',
  models: [
    { id: 'anthropic/claude-sonnet-4-5', provider: 'anthropic' },
    { id: 'openai/gpt-5', provider: 'openai' },
    { id: 'anthropic/claude-opus-4', provider: 'anthropic' },
    { id: 'local-model', provider: '' },
    { id: 'openai/gpt-5', provider: 'openai' },
  ],
  more: false, source: 'opencode models', version: '1.2.3',
  fetchedAt: 1_000, checkedAt: 1_000, reason: '', sentence: 'Models read from studio.',
}
const favourites = [
  { tool: 'claude', model: 'favourite-other', agentDefault: true },
  { tool: 'opencode', model: 'anthropic/claude-opus-4', agentDefault: false },
  { tool: 'opencode', model: 'anthropic/not-listed', agentDefault: true },
  { tool: 'opencode', model: 'anthropic/claude-opus-4', agentDefault: false },
]

test('model options keep the documented group order and deduplicate across groups', () => {
  const options = modelOptions({
    tool: 'opencode', current: 'current-model', machine, favourites,
    recent: ['openai/gpt-5', 'anthropic/claude-opus-4', 'recent-a', 'recent-a', ''],
    suggestions: ['suggest-a', 'anthropic/claude-sonnet-4-5', 'suggest-a', ''],
    host: 'studio', agentName: 'OpenCode',
  })
  assert.deepEqual(options[0], { value: '', label: 'Agent default', detail: 'studio’s OpenCode decides', group: 'Agent default' })
  assert.deepEqual([...new Set(options.map(o => o.group))], [
    'Agent default', 'Favourites', 'Recent', 'On studio · anthropic', 'On studio', 'reet’s suggestions', 'Current',
  ])
  const values = options.map(o => o.value)
  assert.equal(new Set(values).size, values.length)
  assert.ok(!options.some(o => o.value === 'favourite-other'))

  const favs = options.filter(o => o.group === 'Favourites')
  assert.deepEqual(favs.map(o => o.value), ['anthropic/claude-opus-4', 'anthropic/not-listed'])
  assert.deepEqual(favs.map(o => o.starred), [true, true])
  assert.equal(favs[0].detail, undefined)
  assert.equal(favs[1].detail, 'not in studio’s list')

  assert.deepEqual(options.filter(o => o.group === 'Recent').map(o => o.value), ['openai/gpt-5', 'recent-a'])
  assert.deepEqual(options.filter(o => o.group.startsWith('On ')).map(o => ({ value: o.value, group: o.group })), [
    { value: 'anthropic/claude-sonnet-4-5', group: 'On studio · anthropic' },
    { value: 'local-model', group: 'On studio' },
  ])
  assert.deepEqual(options.filter(o => o.group === 'reet’s suggestions').map(o => ({ value: o.value, detail: o.detail })), [
    { value: 'suggest-a', detail: 'not in studio’s list' },
  ])
  assert.deepEqual(options.filter(o => o.group === 'Current').map(o => o.value), ['current-model'])
})

test('model options without a machine list stay honest and selectedModel wins over current', () => {
  const options = modelOptions({
    tool: 'claude', current: 'old', selectedModel: 'new',
    favourites: [{ tool: 'claude', model: 'fav', agentDefault: false }],
    recent: [], suggestions: ['hint'], host: '', agentName: '',
  })
  assert.equal(options[0].detail, 'the agent decides')
  assert.ok(!options.some(o => o.group.startsWith('On ')))
  assert.equal(options.find(o => o.value === 'fav').detail, undefined)
  assert.equal(options.find(o => o.value === 'hint').detail, undefined)
  assert.deepEqual(options.filter(o => o.group === 'Current').map(o => o.value), ['new'])
})

test('launch defaults follow the precedence chain and never take a mismatched default', () => {
  assert.deepEqual(resolveLaunchDefaults({ availableAgents: [] }), { agent: '', model: '', effort: '' })
  assert.deepEqual(resolveLaunchDefaults({ availableAgents: ['gemini', 'claude'] }), { agent: 'gemini', model: '', effort: '' })

  assert.deepEqual(resolveLaunchDefaults({
    explicitAgent: 'cline',
    project: { defaultTool: 'cline', defaultModel: 'project-model', defaultEffort: 'high' },
    account: { defaultTool: 'cline', defaultModel: 'account-model', defaultEffort: 'low' },
    availableAgents: ['claude', 'cline'],
  }), { agent: 'cline', model: 'project-model', effort: 'high' })

  assert.deepEqual(resolveLaunchDefaults({
    explicitAgent: 'cline',
    project: { defaultTool: 'claude', defaultModel: 'project-model', defaultEffort: 'high' },
    account: { defaultTool: 'cline', defaultModel: 'account-model', defaultEffort: 'low' },
    availableAgents: ['claude', 'cline'],
  }), { agent: 'cline', model: 'account-model', effort: 'low' })

  assert.deepEqual(resolveLaunchDefaults({
    explicitAgent: 'gemini',
    project: { defaultTool: 'claude', defaultModel: 'project-model', defaultEffort: 'high' },
    account: { defaultTool: 'claude', defaultModel: 'account-model', defaultEffort: 'high' },
    favouriteDefault: { tool: 'claude', model: 'favourite-model' },
    availableAgents: ['claude', 'gemini'],
  }), { agent: 'gemini', model: '', effort: '' })

  assert.deepEqual(resolveLaunchDefaults({
    project: { defaultTool: 'claude', defaultModel: 'project-model' },
    account: { defaultTool: 'claude', defaultModel: 'account-model', defaultEffort: 'high' },
    availableAgents: ['claude'],
  }), { agent: 'claude', model: 'project-model', effort: '' })

  assert.deepEqual(resolveLaunchDefaults({
    project: { defaultTool: 'gemini', defaultModel: '' },
    account: { defaultTool: 'gemini', defaultModel: 'account-model' },
    availableAgents: ['gemini'],
  }), { agent: 'gemini', model: 'account-model', effort: '' })

  assert.deepEqual(resolveLaunchDefaults({
    account: { defaultTool: 'cline', defaultModel: 'account-model', defaultEffort: 'low' },
    availableAgents: ['cline'],
  }), { agent: 'cline', model: 'account-model', effort: 'low' })

  assert.deepEqual(resolveLaunchDefaults({
    account: { defaultTool: 'claude', defaultModel: 'account-model' },
    favouriteDefault: { tool: 'claude', model: 'favourite-model' },
    availableAgents: ['claude'],
  }), { agent: 'claude', model: 'account-model', effort: '' })

  assert.deepEqual(resolveLaunchDefaults({
    favouriteDefault: { tool: 'claude', model: 'favourite-model' },
    availableAgents: ['claude'],
  }), { agent: 'claude', model: 'favourite-model', effort: '' })

  assert.deepEqual(resolveLaunchDefaults({
    favouriteDefault: 'string-model',
    availableAgents: ['gemini'],
  }), { agent: 'gemini', model: 'string-model', effort: '' })

  assert.deepEqual(resolveLaunchDefaults({
    project: { defaultTool: 'claude', defaultModel: 'project-model', defaultEffort: 'high' },
    availableAgents: ['claude'],
  }), { agent: 'claude', model: 'project-model', effort: '' })
})

const now = Date.now()
const statusMachine = (state, over = {}) => ({
  tool: 'opencode', state, models: [], more: false, source: '', version: '',
  fetchedAt: null, checkedAt: null, reason: '', sentence: '', ...over,
})

test('model status says what happened, how old it is and whether Refresh is honest', () => {
  assert.deepEqual(modelStatus(statusMachine('listed', {
    fetchedAt: now - 3 * 3_600_000, checkedAt: now, sentence: 'Models read from studio.',
  }), 'studio'), { label: 'Read', sentence: 'Models read from studio.', canRefresh: true, age: '3 h ago' })

  assert.deepEqual(modelStatus(statusMachine('failed', {
    fetchedAt: now - 2 * 86_400_000, checkedAt: now - 2 * 60_000, sentence: 'Reading again failed (timed out).',
  }), 'studio'), { label: 'Couldn’t read', sentence: 'Reading again failed (timed out).', canRefresh: true, age: '2 min ago' })

  assert.deepEqual(modelStatus(statusMachine('failed', {
    fetchedAt: now - 2 * 86_400_000, checkedAt: now - 2 * 60_000,
  }), 'studio'), { label: 'Couldn’t read', sentence: 'That machine couldn’t read its models.', canRefresh: true, age: '2 min ago' })

  for (const [state, label, sentence] of [
    ['unsupported', 'Not supported', 'reet can’t read this agent’s models; showing suggestions.'],
    ['off', 'Off', 'Models are switched off on that machine.'],
    ['update', 'Update reet', 'Update reet on that machine to read its models.'],
    ['offline', 'Offline', 'studio is offline.'],
  ]) {
    assert.deepEqual(modelStatus(statusMachine(state, { fetchedAt: now - 5 * 60_000 }), 'studio'), {
      label, sentence, canRefresh: false, age: '5 min ago',
    })
  }

  assert.deepEqual(modelStatus(null, 'studio'), {
    label: 'Offline', sentence: 'studio is offline.', canRefresh: false, age: '',
  })
  assert.deepEqual(modelStatus(undefined, ''), {
    label: 'Offline', sentence: 'That machine is offline.', canRefresh: false, age: '',
  })
})

test('formatAge uses seconds, minutes, hours and days and leaves gaps blank', () => {
  assert.equal(formatAge(now, null), '')
  assert.equal(formatAge(now, 0), '')
  assert.equal(formatAge(now, now + 1), '')
  assert.equal(formatAge(now, now), '0 s ago')
  assert.equal(formatAge(now, now - 59_000), '59 s ago')
  assert.equal(formatAge(now, now - 60_000), '1 min ago')
  assert.equal(formatAge(now, now - 3_599_000), '59 min ago')
  assert.equal(formatAge(now, now - 3_600_000), '1 h ago')
  assert.equal(formatAge(now, now - 23 * 3_600_000), '23 h ago')
  assert.equal(formatAge(now, now - 86_400_000), '1 d ago')
  assert.equal(formatAge(now, now - 2 * 86_400_000), '2 d ago')
})

test('effort choices, favourite moves, keys and star labels', () => {
  assert.deepEqual(effortChoices('cline'), ['', 'low', 'medium', 'high'])
  assert.deepEqual(effortChoices('claude'), [''])
  assert.deepEqual(effortChoices('unknown'), [''])

  const items = ['a', 'b', 'c']
  assert.deepEqual(moveFavourite(items, 0, 1), ['b', 'a', 'c'])
  assert.deepEqual(moveFavourite(items, 2, -1), ['a', 'c', 'b'])
  assert.deepEqual(moveFavourite(items, 0, -1), ['a', 'b', 'c'])
  assert.deepEqual(moveFavourite(items, 2, 1), ['a', 'b', 'c'])
  assert.deepEqual(moveFavourite(items, 1, 5), ['a', 'b', 'c'])
  assert.deepEqual(moveFavourite(items, -1, 1), ['a', 'b', 'c'])
  assert.notEqual(moveFavourite(items, 1, 0), items)
  assert.deepEqual(items, ['a', 'b', 'c'])

  assert.equal(favouriteKey('claude', 'anthropic/sonnet'), 'claude\u0000anthropic/sonnet')
  assert.equal(starLabel('opencode', 'anthropic/sonnet', false), 'Star anthropic/sonnet for OpenCode')
  assert.equal(starLabel('claude', 'sonnet', true), 'Unstar sonnet for Claude Code')
  assert.equal(starLabel('custom-tool', 'model', false), 'Star model for custom-tool')
})

const calls = []
let responder = () => new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
const mockFetch = async (url, init = {}) => {
  calls.push({
    url: String(url), method: init.method ?? 'GET', body: init.body,
    contentType: new Headers(init.headers).get('Content-Type') ?? undefined,
    credentials: init.credentials, cache: init.cache, signal: init.signal,
  })
  return responder(url, init)
}
const reset = () => {
  calls.length = 0
  responder = () => new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
const last = () => calls[calls.length - 1]

test('machine model clients use the documented routes, methods and bodies', async () => {
  const originalFetch = globalThis.fetch
  globalThis.fetch = mockFetch
  try {
    const signal = new AbortController().signal
    reset()
    responder = () => new Response(JSON.stringify({ tool: 'opencode', state: 'listed' }), { status: 200 })
    assert.deepEqual(await terminal.machineModels('a/b', 'open/code', signal), { tool: 'opencode', state: 'listed' })
    assert.deepEqual(last(), {
      url: '/api/v2/machines/a%2Fb/models?tool=open%2Fcode', method: 'GET', body: undefined,
      contentType: undefined, credentials: 'same-origin', cache: 'no-store', signal,
    })

    reset()
    responder = () => new Response(JSON.stringify({ tool: 'opencode', state: 'listed' }), { status: 200 })
    assert.deepEqual(await terminal.refreshMachineModels('a/b', 'open/code'), { tool: 'opencode', state: 'listed' })
    assert.equal(last().url, '/api/v2/machines/a%2Fb/models/refresh')
    assert.equal(last().method, 'POST')
    assert.equal(last().body, '{"tool":"open/code"}')
    assert.equal(last().contentType, 'application/json')
    assert.equal(last().credentials, 'same-origin')
    assert.equal(last().cache, 'no-store')

    reset()
    responder = () => new Response(JSON.stringify({
      machines: [{ agentId: 'a', host: 'studio', tool: 'opencode', state: 'listed' }],
    }), { status: 200 })
    assert.deepEqual(await terminal.allModels('open/code'), {
      machines: [{ agentId: 'a', host: 'studio', tool: 'opencode', state: 'listed' }],
    })
    assert.equal(last().url, '/api/v2/models?tool=open%2Fcode')
    assert.equal(last().method, 'GET')
    assert.equal(last().credentials, 'same-origin')
    assert.equal(last().cache, 'no-store')

    reset()
    responder = () => new Response(JSON.stringify({
      error: 'Too many model reads. Try again in a minute.',
      code: 'models_rate_limited',
      detail: 'Wait a moment.',
      field: 'tool',
    }), { status: 429 })
    const error = await terminal.refreshMachineModels('a', 'opencode').then(() => null, e => e)
    assert.ok(error instanceof terminal.TerminalError)
    assert.equal(error.status, 429)
    assert.equal(error.code, 'models_rate_limited')
    assert.equal(error.message, 'Too many model reads. Try again in a minute.')
    assert.equal(error.detail, 'Wait a moment.')
    assert.equal(error.field, 'tool')
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('model favourite clients use the documented routes, methods and bodies', async () => {
  const originalFetch = globalThis.fetch
  globalThis.fetch = mockFetch
  try {
    reset()
    responder = () => new Response(JSON.stringify({ version: 2, favourites: [] }), { status: 200 })
    assert.deepEqual(await api.listModelFavourites(), { version: 2, favourites: [] })
    assert.equal(last().url, '/api/v2/profile/model-favourites')
    assert.equal(last().method, 'GET')
    assert.equal(last().body, undefined)

    reset()
    responder = () => new Response(JSON.stringify({ version: 3, favourites: [] }), { status: 201 })
    assert.deepEqual(await api.addModelFavourite('claude', 'anthropic/sonnet'), { version: 3, favourites: [] })
    assert.equal(last().url, '/api/v2/profile/model-favourites')
    assert.equal(last().method, 'POST')
    assert.equal(last().body, '{"tool":"claude","model":"anthropic/sonnet"}')
    assert.equal(last().contentType, 'application/json')

    reset()
    responder = () => new Response(null, { status: 204 })
    assert.equal(await api.removeModelFavourite('claude', 'anthropic/sonnet'), undefined)
    assert.equal(last().url, '/api/v2/profile/model-favourites?tool=claude&model=anthropic%2Fsonnet')
    assert.equal(last().method, 'DELETE')
    assert.equal(last().body, undefined)

    reset()
    responder = () => new Response(JSON.stringify({ version: 4, favourites: [] }), { status: 200 })
    const items = [{ tool: 'claude', model: 'sonnet' }, { tool: 'opencode', model: 'anthropic/opus' }]
    assert.deepEqual(await api.reorderModelFavourites(3, items), { version: 4, favourites: [] })
    assert.equal(last().url, '/api/v2/profile/model-favourites/order')
    assert.equal(last().method, 'PUT')
    assert.equal(last().body, JSON.stringify({ version: 3, items }))
    assert.equal(last().contentType, 'application/json')

    reset()
    responder = () => new Response(JSON.stringify({ version: 5, favourites: [] }), { status: 200 })
    assert.deepEqual(await api.setModelFavouriteDefault('claude', 'sonnet'), { version: 5, favourites: [] })
    assert.equal(last().url, '/api/v2/profile/model-favourites/default')
    assert.equal(last().method, 'PUT')
    assert.equal(last().body, '{"tool":"claude","model":"sonnet"}')

    reset()
    responder = () => new Response(JSON.stringify({ error: 'That model is not a favourite.' }), { status: 422 })
    const error = await api.addModelFavourite('claude', 'anthropic/sonnet').then(() => null, e => e)
    assert.ok(error instanceof api.ApiError)
    assert.equal(error.status, 422)
    assert.equal(error.message, 'That model is not a favourite.')
  } finally {
    globalThis.fetch = originalFetch
  }
})
