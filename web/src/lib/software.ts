import type { FleetSoftwareRow, SoftwareBatch, SoftwareBatchMember, SoftwareEntry } from './terminal/api'
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

/** The action and target a batch member was frozen with. */
export function batchMemberAction(member: Pick<SoftwareBatchMember, 'action' | 'target'>): string {
  if (member.action === 'install') return `Install ${member.target ?? 'the reviewed version'}`
  if (member.action === 'update') return `Update to ${member.target ?? 'the reviewed version'}`
  return ''
}

/** One line for a machine's place in a batch. */
export function batchMemberLabel(member: Pick<SoftwareBatchMember, 'state' | 'version'>): string {
  switch (member.state) {
    case 'applied': return member.version ? `Applied ${member.version}` : 'Applied'
    case 'running': return 'Applying'
    case 'failed': return 'Failed'
    case 'skipped': return 'Skipped'
    case 'excluded': return 'Not included'
    default: return 'Waiting'
  }
}

/** What a member's stored code or exclusion reason means, or '' when it says nothing. */
export function batchMemberNote(member: Pick<SoftwareBatchMember, 'code' | 'reason'>): string {
  if (member.reason) return member.reason
  switch (member.code) {
    case 'offline': return "That machine wasn't connected when its turn came."
    case 'apply_unsupported': return 'Update reet on that machine to install agents.'
    case 'changed': return "The machine's installation changed since you reviewed the plan."
    case 'sessions_running': return 'An OpenCode session started on that machine.'
    case 'download_failed': return "The machine couldn't download the reviewed artifact. Nothing was changed."
    case 'checksum_failed': return "The downloaded artifact didn't match its recorded checksum. Nothing was installed."
    case 'install_failed': return "The machine couldn't install the reviewed artifact. The earlier version, if any, was kept."
    case 'timed_out': return 'That machine took too long to apply the plan. Nothing was changed.'
    case 'busy': return 'That machine was already installing an agent.'
    case 'not_managed': return "reet didn't install that OpenCode, so it won't change it."
    case 'off': return 'Software changes are switched off on that machine.'
    case 'in_another_batch': return 'That machine is already in another batch.'
    case 'unknown_machine': return "That machine isn't enrolled."
    case 'bad_answer': return 'The machine sent an unexpected answer.'
    case 'apply_failed': return "That machine's apply did not finish."
    default: return ''
  }
}

/** One line for the batch as a whole. */
export function batchStateLabel(state: SoftwareBatch['state']): string {
  switch (state) {
    case 'pending': return 'Waiting to start'
    case 'running': return 'Applying'
    case 'stopped': return 'Stopped on a failure'
    case 'done': return 'Finished'
    case 'abandoned': return 'Abandoned'
  }
}

/** True when the batch can be abandoned now: it has not ended and nothing is applying. */
export function batchCanAbandon(batch: Pick<SoftwareBatch, 'state' | 'members'>): boolean {
  return batch.state !== 'done' && batch.state !== 'abandoned' && !batch.members.some(member => member.state === 'running')
}

/** The failed member a stopped batch is waiting on, if any. */
export function batchFailedMember(batch: Pick<SoftwareBatch, 'members'>): SoftwareBatchMember | undefined {
  return batch.members.find(member => member.state === 'failed')
}
