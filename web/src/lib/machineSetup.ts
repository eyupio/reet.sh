import type { Machine, SyncEnvironment } from './api'
import { agentGroups } from './machines'

export type ReleaseChannel = 'stable' | 'dev'

export function shellQuote(value: string): string {
  return "'" + value.replaceAll("'", "'\\''") + "'"
}

export function installCommands(channel: ReleaseChannel): string {
  return `curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh${channel === 'dev' ? ' -s -- --version dev' : ''}\nexport PATH="$HOME/.local/bin:$PATH"\nreet --version`
}

export function configureCommand(site: string): string {
  return `reet configure --url ${shellQuote(site)} --version latest`
}

export function scanCommand(site: string, host: string): string {
  return `reet scan --server ${shellQuote(site)} --env ${shellQuote(host)} --prune --reconcile`
}

export function enrollCommands(site: string, host: string, allowApply = false): string {
  return `reet agent enroll --server ${shellQuote(site)} --env ${shellQuote(host)}${allowApply ? ' --allow-apply' : ''}\nreet agent run`
}

export function profilePullCommand(site: string, profile: string): string {
  return `reet setup profile pull ${shellQuote(profile)} --server ${shellQuote(site)}`
}

export function profileFetchCommand(profile: string, revision: number): string {
  return `reet setup fetch --s3 --profile ${shellQuote(profile)} --revision ${revision}`
}

export function profileRestoreCommand(profile: string, revision: number): string {
  return `reet setup restore --profile ${shellQuote(profile)} --revision ${revision}`
}

export function profileVerifyCommand(profile: string, revision: number): string {
  return `reet setup verify --profile ${shellQuote(profile)} --revision ${revision}`
}

export function upgradeCommands(channel: ReleaseChannel): string {
  return `reet upgrade --check\nreet upgrade${channel === 'dev' ? ' --version dev' : ''}\nreet doctor`
}

export const CONNECTION_POLL_MS = 3000
export const CONNECTION_POLL_MAX_MS = 20000

// A blinking controller should not turn into a request storm. Wait the normal
// interval while things are healthy, then back off while checks keep failing.
export function retryDelay(failures: number, base = CONNECTION_POLL_MS, maximum = CONNECTION_POLL_MAX_MS): number {
  if (failures <= 0) return base
  return Math.min(maximum, base * 2 ** Math.min(failures, 4))
}

export type ConnectionStage = 'blocked' | 'waiting' | 'needs-setup' | 'reconnecting' | 'ready'
type ReadinessMachine = {
  host: string
  ready: boolean
  serverEnabled?: boolean
  capabilities?: { terminalControl?: boolean; terminalEnabled?: boolean; readiness?: string }
}

// One place that decides what the connection watch is actually saying, so the
// words, the colour and the next action cannot disagree with each other.
export function connectionStage(machine: ReadinessMachine | undefined, terminalAccess: boolean): ConnectionStage {
  if (!terminalAccess) return 'blocked'
  if (!machine) return 'waiting'
  if (machine.serverEnabled === false) return 'blocked'
  const capabilities = machine.capabilities
  if (!capabilities || !capabilities.terminalControl || !capabilities.terminalEnabled) return 'needs-setup'
  if (capabilities.readiness && capabilities.readiness !== 'ready') return 'needs-setup'
  if (!machine.ready) return 'reconnecting'
  return 'ready'
}

export function syncLink(host: string): string {
  return `/machines?view=compare&machine=${encodeURIComponent(host)}`
}

export function supportedAgents(machine?: Machine): SyncEnvironment[] {
  return agentGroups(machine).filter((group) => ['agents', 'claude', 'cursor', 'opencode'].includes(group.agent))
}

export function scanOnlyAgents(machine?: Machine): SyncEnvironment[] {
  return agentGroups(machine).filter((group) => !['agents', 'claude', 'cursor', 'opencode'].includes(group.agent))
}

export function machineStatus(machine: Machine, now = Date.now()): 'current' | 'stale' | 'partial' | 'empty' {
  const groups = agentGroups(machine)
  if (groups.length === 0) return 'empty'
  if (groups.some((group) => !group.complete)) return 'partial'
  if (groups.some((group) => group.scannedAt <= now - 14 * 86400000)) return 'stale'
  return 'current'
}
