import type { Machine, SyncAgent, SyncEnvironment } from './api'
import { sameHost } from './contentReview'
import { agentGroups } from './machines'
import { shellQuote, retryDelay, supportedAgents } from './machineSetup'
import type { ReleaseChannel } from './machineSetup'

// What the "Add a machine" dialog says and where it lands. Pure: no DOM, no fetch, no clock of its own (callers pass
// `now`). Everything here is only what the controller has been told: a connector reported in, a scan arrived. None of
// it says the command worked or that a machine is in good shape.

// The server's limit on a machine name, counted in UTF-8 bytes (Go's len), not characters.
export const MAX_MACHINE_NAME_BYTES = 200
export const JOIN_WATCH_CAP_MS = 15 * 60 * 1000
const SCAN_WINDOW_MS = 14 * 86_400_000

// The Unicode text-direction overrides and isolates, which can make a name read differently from what it is.
const BIDI = (n: number): boolean => (n >= 0x202a && n <= 0x202e) || (n >= 0x2066 && n <= 0x2069)
// C0, DEL, C1, and the two Unicode line separators.
const hasControl = (s: string): boolean => [...s].some(c => { const n = c.codePointAt(0)!; return n <= 0x1f || (n >= 0x7f && n <= 0x9f) || n === 0x2028 || n === 0x2029 })

export type NameCheck = { ok: true; name: string } | { ok: false; reason: string }

export function validateMachineName(name: string): NameCheck {
  const trimmed = String(name ?? '').trim()
  if (trimmed === '') return { ok: false, reason: 'Enter a name for the machine.' }
  if (new TextEncoder().encode(trimmed).length > MAX_MACHINE_NAME_BYTES) return { ok: false, reason: `That name is too long. Use at most ${MAX_MACHINE_NAME_BYTES} bytes; accented letters and emoji take more than one.` }
  if ([...trimmed].some(c => BIDI(c.codePointAt(0)!))) return { ok: false, reason: 'Use a name without text-direction override characters.' }
  if (hasControl(trimmed)) return { ok: false, reason: 'Use a name without line breaks or other control characters.' }
  return { ok: true, name: trimmed }
}

/** Install, save the connection, then enrol and run: one paste. The name is quoted by the builders; no token appears. */
export function joinCommands(site: string, name: string, channel: ReleaseChannel): string {
  const host = name.trim()
  return `curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh -s --${channel === 'dev' ? ' --version dev' : ''} --server ${shellQuote(site)} --name ${shellQuote(host)}`
}

export type JoinStage = 'waiting' | 'already_here' | 'connected' | 'first_scan' | 'syncing' | 'sync_issue' | 'ready' | 'unknown'
export type JoinView = { stage: JoinStage; label: string; detail: string; canUse: boolean; connectorIds?: string[] }
/** What was already reporting under the name: connector ids, and the newest complete supported scan (0 if none). */
export type JoinBaseline = { agentIds: string[]; scannedAt: number } | null

/** A first scan is only the start: new enrolments finish after library sync. */
export function joinSyncState(view: JoinView, skills: import('./api').SyncSkills | null = null): JoinView {
  if (view.stage !== 'ready') return view
  const reports = skills?.machines.filter(item => view.connectorIds?.includes(item.agentId)) ?? []
  const report = reports.find(item => item.state === 'ok') ?? reports[0]
  if (!report) return { stage: 'syncing', label: 'Waiting for skill sync report', detail: 'The connection and inventory arrived. Reet is waiting for this machine to confirm its library state.', canUse: false }
  if (report.state === 'ok') return { stage: 'ready', label: 'Ready · library in sync', detail: 'Active skills follow your library automatically.', canUse: true }
  if (['installing', 'removing', 'updating'].includes(report.state)) return { stage: 'syncing', label: 'Syncing your skill library', detail: 'This machine is applying the library. You can close this dialog; sync continues.', canUse: false }
  return { stage: 'sync_issue', label: 'Sync needs attention', detail: report.failed.map(f => `${f.name}: ${f.detail}`).join('; ') || 'Open Sync to see why this machine is waiting.', canUse: false }
}

const isAgent = (a: unknown): a is SyncAgent => !!a && typeof a === 'object' && typeof (a as SyncAgent).id === 'string' && typeof (a as SyncAgent).host === 'string'
const isMachine = (m: unknown): m is Machine => !!m && typeof m === 'object' && typeof (m as Machine).host === 'string' && Array.isArray((m as Machine).groups) &&
  (m as Machine).groups.every(g => !!g && typeof g === 'object' && typeof g.agent === 'string')

/**
 * The host a typed name is saved under. After a rename or merge the server keeps the old name as an alias (stored lower case)
 * of the machine, and enrolling under it reports as that machine's host. Anything else is the trimmed name itself.
 */
export function canonicalHost(name: string, machines: Machine[]): string {
  const typed = String(name ?? '').trim()
  const key = typed.toLowerCase()
  if (!Array.isArray(machines)) return typed
  for (const m of machines) {
    if (!m || typeof m !== 'object' || typeof m.host !== 'string' || !Array.isArray(m.aliases)) continue
    if (m.aliases.some(a => typeof a === 'string' && a === key)) return m.host
  }
  return typed
}

function scans(host: string, machines: Machine[]): SyncEnvironment[] {
  return machines.filter(m => sameHost(m.host, host)).flatMap(m => supportedAgents(m))
}

/** The connectors reporting under the name now and the newest complete supported scan; null when there is neither. */
export function joinBaseline(name: string, agents: SyncAgent[], machines: Machine[]): JoinBaseline {
  const list = Array.isArray(machines) ? machines : []
  const host = canonicalHost(name, list)
  const ids = (Array.isArray(agents) ? agents : []).filter(a => isAgent(a) && sameHost(a.host, host)).map(a => a.id)
  const scannedAt = Math.max(0, ...scans(host, list.filter(isMachine)).filter(g => g.complete === true && g.scannedAt > 0).map(g => g.scannedAt))
  return ids.length > 0 || scannedAt > 0 ? { agentIds: ids, scannedAt } : null
}

const UNKNOWN: JoinView = { stage: 'unknown', label: 'Something changed. Check again.', detail: 'reet.sh could not tell what state this machine is in.', canUse: false }
const PAUSED = 'Its connector is paused. Resume it on the Machines page.'
const NO_SCAN = "It hasn't reported a complete scan of an agent folder in the last 14 days."

// `after` is the scan time that was already there: a scan counts only if it is strictly newer.
function hasRecentScan(host: string, machines: Machine[], now: number, after: number): boolean {
  // Written so that a missing or non-numeric scan time fails rather than passes.
  return scans(host, machines).some(g => g.complete === true && g.scannedAt > after && g.scannedAt > 0 && now - g.scannedAt <= SCAN_WINDOW_MS)
}

function judge(host: string, connectors: SyncAgent[], machines: Machine[], now: number, after: number): { paused: boolean; scanned: boolean } {
  const running = connectors.some(a => !a.paused)
  return { paused: !running, scanned: running && hasRecentScan(host, machines, now, after) }
}

export function joinState(i: { name: string; machines: Machine[]; agents: SyncAgent[]; baseline: JoinBaseline; now: number }): JoinView {
  const checked = validateMachineName(i.name)
  if (!checked.ok || !Array.isArray(i.machines) || !Array.isArray(i.agents) || !i.machines.every(isMachine) || !i.agents.every(isAgent) || !Number.isFinite(i.now)) return UNKNOWN
  const typed = checked.name
  const host = canonicalHost(typed, i.machines)
  const saved = host !== typed ? ` You typed ${typed}; reet.sh saved it under ${host}.` : ''
  const here = i.agents.filter(a => sameHost(a.host, host))

  if (i.baseline && i.baseline.agentIds.length > 0) {
    const known = new Set(i.baseline.agentIds)
    const fresh = here.filter(a => !known.has(a.id))
    if (fresh.length === 0) {
      // Sticky: a healthy machine's lastSeen moves with every heartbeat, so only a new connector id ends this.
      const j = judge(host, here, i.machines, i.now, 0)
      const label = `A machine called ${host} is already here.`
      if (here.length === 0) return { stage: 'already_here', label, detail: "It isn't reporting at the moment, so it can't be used yet. Choose another name, or check again." + saved, canUse: false }
      if (j.paused) return { stage: 'already_here', label, detail: PAUSED + ' Or choose another name.' + saved, canUse: false }
      if (!j.scanned) return { stage: 'already_here', label, detail: NO_SCAN + ' Or choose another name.' + saved, canUse: false }
      return { stage: 'already_here', label, detail: 'It was already reporting before you started. Use it, or choose another name.' + saved, canUse: true }
    }
    return normal(host, typed, fresh, i.machines, i.now, i.baseline.scannedAt)
  }
  return normal(host, typed, here, i.machines, i.now, i.baseline?.scannedAt ?? 0)
}

function normal(host: string, typed: string, connectors: SyncAgent[], machines: Machine[], now: number, after: number): JoinView {
  if (connectors.length === 0) return { stage: 'waiting', label: 'Waiting for it', detail: 'No connector with this name has reported in yet.' + (host !== typed ? ` The machine will appear as ${host}.` : '') + ' Nothing happens until you run the commands on that machine.', canUse: false }
  const j = judge(host, connectors, machines, now, after)
  if (j.paused) return { stage: 'connected', label: 'Connected', detail: PAUSED, canUse: false }
  if (!j.scanned) return { stage: 'first_scan', label: 'Waiting for its first scan', detail: 'A connector has reported in. ' + NO_SCAN, canUse: false }
  return { stage: 'ready', label: 'Ready', detail: 'A connector has reported in and a complete scan of an agent folder has arrived. That does not show the machine works as you expect.', canUse: true, connectorIds: connectors.filter(agent => !agent.paused).map(agent => agent.id) }
}

export const joinDelay = (failures: number): number => retryDelay(failures, 3000, 20000)
