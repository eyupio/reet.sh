import type { FleetSoftwareRow, SoftwareEntry } from './terminal/api'
export type SoftwarePlanAction = 'install' | 'update' | 'rollback' | 'remove'

/** One line of installation evidence, including what is absent and why. */
export function softwareLine(entry: SoftwareEntry): string {
  switch (entry.state) {
    case 'installed': return entry.managed
      ? ['Managed by reet', entry.version ? `verified ${entry.version}` : '', entry.path].filter(Boolean).join(' · ')
      : `Installed outside reet${entry.path ? ` at ${entry.path}` : ''}. reet can add its own verified copy.`
    case 'missing': return `Not installed. The reviewed package is ${entry.package ?? 'unknown'}${entry.source ? ` from ${entry.source}` : ''}.`
    case 'unknown': return 'The installation was not recognised, so reet shows only what it observed.'
    case 'unsupported': return 'reet has no reviewed maintenance plan for this installation.'
  }
}

/** The plans a person can review for this evidence, in a stable order. */
export function softwarePlanActions(entry: SoftwareEntry): SoftwarePlanAction[] {
  const operations = entry.operations ?? []
  if (operations.includes('plan_install') && (entry.state === 'missing' || (entry.state === 'installed' && !entry.managed))) return ['install']
  if (entry.state !== 'installed' || !entry.managed) return []
  const actions: SoftwarePlanAction[] = []
  if (operations.includes('plan_update')) actions.push('update')
  if (entry.previous) actions.push('rollback')
  if (operations.includes('plan_remove')) actions.push('remove')
  return actions
}

/** One line for a machine in the plan-only fleet review. */
export function fleetSoftwareLabel(row: Pick<FleetSoftwareRow, 'state' | 'version' | 'target'>): string {
  switch (row.state) {
    case 'update': return `Update to ${row.target ?? 'the reviewed version'}`
    case 'install': return row.version ? `Install ${row.target ?? 'the reviewed version'} alongside` : `Install ${row.target ?? 'the reviewed version'}`
    case 'none': return 'Up to date'
    default: return 'Needs attention'
  }
}

/** What stops this machine's plan from being applied, or '' when nothing does. */
export function fleetSoftwareWarning(row: Pick<FleetSoftwareRow, 'state' | 'sessionsRunning' | 'canApply'>): string {
  if (row.state === 'none' || row.state === 'unknown') return ''
  if (row.sessionsRunning > 0) {
    return `${row.sessionsRunning} OpenCode ${row.sessionsRunning === 1 ? 'session is' : 'sessions are'} running; reet refuses to apply until ${row.sessionsRunning === 1 ? 'it stops' : 'they stop'}.`
  }
  if (!row.canApply) return 'Update reet on this machine to apply.'
  return ''
}

/** True when the fleet review may apply this row's plan right now. */
export function fleetSoftwareCanApply(row: Pick<FleetSoftwareRow, 'state' | 'sessionsRunning' | 'canApply'>): boolean {
  return (row.state === 'install' || row.state === 'update') && row.canApply && row.sessionsRunning === 0
}
