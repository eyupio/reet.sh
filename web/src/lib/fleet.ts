import type { Machine, SyncState } from './api'

export type FleetOperation = { agentId: string; targetId: string; kind: 'scan' | 'preview'; label: string }
export type FleetPlan = { operations: FleetOperation[]; skipped: string[] }
const active = new Set(['queued', 'running', 'awaiting-approval', 'approved'])

// Build an explicit work list. Preview jobs never approve changes to installed files.
export function fleetPlan(state: SyncState, kind: 'scan' | 'preview', now = Date.now()): FleetPlan {
  const operations: FleetOperation[] = [], skipped: string[] = []
  const agents = state.agents ?? []
  const items = kind === 'scan'
    ? agents.map(agent => ({ agent, targetId: '', label: agent.host }))
    : state.targets.map(target => ({ agent: agents.find(a => a.host.toLowerCase() === target.target.host.toLowerCase()), targetId: target.target.id, label: `${target.target.host} · ${target.target.agent}${target.target.project ? ` · ${target.target.project}` : ''}` }))
  const seen = new Set<string>()
  for (const item of items) {
    const { agent, targetId, label } = item
    const key = `${agent?.id}:${targetId}`
    if (seen.has(key)) continue
    seen.add(key)
    let reason = ''
    if (!agent) reason = 'not enrolled'
    else if (agent.paused) reason = 'paused'
    else if (agent.lastSeen <= now - 90000) reason = 'offline'
    else if ((state.jobs ?? []).some(job => job.agentId === agent.id && active.has(job.status) && (kind === 'scan' || job.kind !== 'preview' || job.targetId === targetId))) reason = 'job already pending'
    if (!reason && kind === 'preview') {
      const env = state.environments.find(env => env.id === targetId)
      if (!env?.complete || env.scannedAt <= now - 14 * 86400000) reason = 'needs a fresh complete scan'
    }
    if (reason) skipped.push(`${label}: ${reason}`)
    else operations.push({ agentId: agent!.id, targetId, kind, label })
  }
  return { operations, skipped }
}

// Keep successful responses even when a later machine fails. Never retry writes automatically.
export async function queueFleet(plan: FleetPlan, queue: (operation: FleetOperation) => Promise<void>) {
  const queued: string[] = [], failed: string[] = []
  for (const operation of plan.operations) {
    try { await queue(operation); queued.push(operation.label) }
    catch (error) { failed.push(`${operation.label}: ${error instanceof Error ? error.message : String(error)}`) }
  }
  return { queued, failed }
}

export function destinationScopes(machines: Machine[], destination: string) {
  return machines.filter(machine => destination === '*' || machine.host === destination)
    .flatMap(machine => machine.groups.filter(group => !group.agent.startsWith('reet')))
}
