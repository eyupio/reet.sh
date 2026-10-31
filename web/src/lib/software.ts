import type { SoftwareEntry } from './terminal/api'

/** One line of installation evidence, including what is absent and why. */
export function softwareLine(entry: SoftwareEntry): string {
  switch (entry.state) {
    case 'installed': return [entry.manager ?? 'installation', entry.source ?? 'unrecognised source', entry.path].filter(Boolean).join(' · ')
    case 'missing': return `Not installed. The reviewed package is ${entry.package ?? 'unknown'}${entry.source ? ` from ${entry.source}` : ''}.`
    case 'unknown': return 'The installation was not recognised, so reet shows only what it observed.'
    case 'unsupported': return 'reet has no reviewed maintenance plan for this installation.'
  }
}

/** The plan a person can review for this evidence, or none. */
export function softwarePlanAction(entry: SoftwareEntry): 'install' | 'update' | '' {
  const operations = entry.operations ?? []
  if (entry.state === 'missing' && operations.includes('plan_install')) return 'install'
  if (entry.state === 'installed' && operations.includes('plan_update')) return 'update'
  return ''
}
