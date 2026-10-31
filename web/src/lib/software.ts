import type { SoftwareEntry } from './terminal/api'
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
