import { CATALOGUE } from './catalogue'

export type ModelState = 'listed' | 'unsupported' | 'failed' | 'off' | 'offline' | 'update'
export type ModelEntry = { id: string; provider: string }
export type MachineModels = {
  tool: string
  state: ModelState
  models: ModelEntry[]
  more: boolean
  source: string
  version: string
  fetchedAt: number | null
  checkedAt: number | null
  reason: string
  sentence: string
}
export type ModelFavourite = { tool: string; model: string; agentDefault: boolean }
export type ModelOption = {
  value: string
  label: string
  detail?: string
  group: string
  disabled?: boolean
  reason?: string
  starred?: boolean
}
export type ModelOptionsInput = {
  tool: string
  current?: string
  machine?: MachineModels | null
  favourites: ModelFavourite[]
  recent: string[]
  suggestions: string[]
  selectedModel?: string | null
  host?: string
  agentName?: string
}
export type LaunchDefaultSource = {
  defaultTool?: string
  defaultModel?: string
  defaultEffort?: string
}
export type ResolveLaunchDefaultsInput = {
  explicitAgent?: string
  project?: LaunchDefaultSource | null
  account?: LaunchDefaultSource | null
  favouriteDefault?: { tool: string; model: string } | string | null
  availableAgents?: readonly string[]
}
export type ResolvedLaunchDefaults = { agent: string; model: string; effort: string }
export type ModelStatus = { label: string; sentence: string; canRefresh: boolean; age: string }
export type RecentSession = { tool?: string | null; model?: string | null; createdAt?: number | null; updatedAt?: number | null }

const agentNameOf = (tool: string): string => CATALOGUE.find(agent => agent.id === tool)?.name ?? tool

export function providerOf(id: string): string {
  const slash = id.indexOf('/')
  return slash < 0 ? '' : id.slice(0, slash)
}

export function dedupeModels(entries: readonly ModelEntry[]): ModelEntry[] {
  const seen = new Set<string>()
  const out: ModelEntry[] = []
  for (const entry of entries) {
    if (seen.has(entry.id)) continue
    seen.add(entry.id)
    out.push({ id: entry.id, provider: entry.provider ?? providerOf(entry.id) })
  }
  return out
}

export function recentModels(sessions: readonly RecentSession[], tool: string, limit = 3): string[] {
  const count = Number.isFinite(limit) ? Math.max(0, Math.floor(limit)) : 0
  if (count === 0) return []
  const newest = [...sessions].sort((a, b) => ((b.updatedAt ?? b.createdAt) ?? 0) - ((a.updatedAt ?? a.createdAt) ?? 0))
  const seen = new Set<string>()
  const out: string[] = []
  for (const session of newest) {
    if (session.tool !== tool) continue
    const model = String(session.model ?? '').trim()
    if (!model || seen.has(model)) continue
    seen.add(model)
    out.push(model)
    if (out.length === count) break
  }
  return out
}

export function modelOptions({
  tool, current = '', machine = null, favourites, recent, suggestions,
  selectedModel = null, host = '', agentName = '',
}: ModelOptionsInput): ModelOption[] {
  const out: ModelOption[] = []
  const seen = new Set<string>()
  const add = (option: ModelOption) => {
    if (seen.has(option.value)) return
    seen.add(option.value)
    out.push(option)
  }

  const agent = agentName || agentNameOf(tool)
  const machineName = host || 'this machine'
  const missingDetail = `not in ${machineName}’s list`
  add({
    value: '', label: 'Agent default',
    detail: host ? `${host}’s ${agent} decides` : 'the agent decides',
    group: 'Agent default',
  })

  for (const favourite of favourites) {
    if (favourite.tool !== tool || !favourite.model) continue
    const missing = !!machine?.models?.length && !machine.models.some(entry => entry.id === favourite.model)
    add({
      value: favourite.model, label: favourite.model, group: 'Favourites', starred: true,
      ...(missing ? { detail: missingDetail } : {}),
    })
  }

  for (const model of recent) {
    const value = String(model ?? '').trim()
    if (!value) continue
    add({ value, label: value, group: 'Recent' })
  }

  const machineList = machine?.models?.length ? dedupeModels(machine.models) : []
  const providers = [...new Set(machineList.map(entry => entry.provider ?? providerOf(entry.id)))]
  for (const provider of providers) {
    const group = provider ? `On ${machineName} · ${provider}` : `On ${machineName}`
    for (const entry of machineList) {
      if ((entry.provider ?? providerOf(entry.id)) !== provider) continue
      add({ value: entry.id, label: entry.id, group })
    }
  }

  for (let i = 0; i < suggestions.length; i++) {
    const value = String(suggestions[i] ?? '').trim()
    if (!value) continue
    const missing = machineList.length > 0 && !machineList.some(entry => entry.id === value)
    add({
      value, label: value, group: 'reet’s suggestions',
      ...(missing ? { detail: missingDetail } : {}),
    })
  }

  const selected = selectedModel == null ? current : selectedModel
  const value = String(selected ?? '').trim()
  if (value && !seen.has(value)) add({ value, label: value, group: 'Current' })

  return out
}

export function resolveLaunchDefaults({
  explicitAgent = '', project = null, account = null, favouriteDefault = null, availableAgents = [],
}: ResolveLaunchDefaultsInput): ResolvedLaunchDefaults {
  const agent = explicitAgent || project?.defaultTool || account?.defaultTool || availableAgents[0] || ''
  let model = ''
  if (agent && project?.defaultTool === agent && project.defaultModel) model = project.defaultModel
  else if (agent && account?.defaultTool === agent && account.defaultModel) model = account.defaultModel
  else if (typeof favouriteDefault === 'string') model = favouriteDefault
  else if (favouriteDefault && favouriteDefault.tool === agent) model = favouriteDefault.model

  let effort = ''
  if (agent && project?.defaultTool === agent && project.defaultEffort) effort = project.defaultEffort
  else if (agent && account?.defaultTool === agent && account.defaultEffort) effort = account.defaultEffort
  if (!CATALOGUE.find(entry => entry.id === agent)?.effort) effort = ''

  return { agent, model, effort }
}

export function formatAge(now: number, timestamp: number | null | undefined): string {
  if (typeof timestamp !== 'number' || !Number.isFinite(timestamp) || timestamp <= 0 || timestamp > now) return ''
  const seconds = Math.floor((now - timestamp) / 1000)
  if (seconds < 60) return `${seconds} s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} h ago`
  return `${Math.floor(hours / 24)} d ago`
}

const STATUS_LABELS: Record<ModelState, string> = {
  listed: 'Read',
  unsupported: 'Not supported',
  failed: 'Couldn’t read',
  off: 'Off',
  offline: 'Offline',
  update: 'Update reet',
}

function statusSentence(state: ModelState, host: string): string {
  const machine = host || 'that machine'
  switch (state) {
    case 'listed': return `Models read from ${machine}.`
    case 'unsupported': return 'reet can’t read this agent’s models; showing suggestions.'
    case 'failed': return 'That machine couldn’t read its models.'
    case 'off': return 'Models are switched off on that machine.'
    case 'offline': return host ? `${host} is offline.` : 'That machine is offline.'
    case 'update': return 'Update reet on that machine to read its models.'
  }
}

export function modelStatus(machine: MachineModels | null | undefined, host: string): ModelStatus {
  if (!machine) return { label: STATUS_LABELS.offline, sentence: statusSentence('offline', host), canRefresh: false, age: '' }
  const state = machine.state
  const at = state === 'failed' ? machine.checkedAt : machine.fetchedAt
  return {
    label: STATUS_LABELS[state] ?? state,
    sentence: machine.sentence?.trim() ? machine.sentence : statusSentence(state, host),
    canRefresh: state === 'listed' || state === 'failed',
    age: formatAge(Date.now(), at),
  }
}

export function effortChoices(tool: string): string[] {
  return CATALOGUE.find(entry => entry.id === tool)?.effort ? ['', 'low', 'medium', 'high'] : ['']
}

export function moveFavourite<T>(items: readonly T[], index: number, delta: number): T[] {
  const out = [...items]
  const to = index + delta
  if (!Number.isInteger(index) || index < 0 || index >= out.length || !Number.isInteger(to) || to < 0 || to >= out.length) return out
  const [item] = out.splice(index, 1)
  out.splice(to, 0, item)
  return out
}

export function favouriteKey(tool: string, model: string): string {
  return tool + '\u0000' + model
}

export function starLabel(tool: string, model: string, starred: boolean): string {
  return `${starred ? 'Unstar' : 'Star'} ${model} for ${agentNameOf(tool)}`
}
