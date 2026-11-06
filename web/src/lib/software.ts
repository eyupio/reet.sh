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
    // A skipped machine: nothing reached it, so nothing was changed and nothing needs reviewing.
    case 'offline': return "That machine wasn't connected when its turn came, so nothing was changed on it. Review the fleet again once it is back."
    // A failed one: the connection dropped after the plan went out, so the outcome is not known.
    case 'connection_lost': return 'The connection to that machine dropped while it was applying the plan, so it may or may not have finished. Retry checks what is installed before doing anything.'
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

/**
 * The whole fleet in one sentence and one button. A person who opens Fleet software wants to know whether their machines
 * are up to date and, if not, to press one thing. Choosing machines, ordering them and applying one by one stay available
 * under "Machines" for the few who want them.
 */
export type FleetSoftwareState = 'empty' | 'ready' | 'blocked' | 'attention' | 'current' | 'working' | 'stopped' | 'finished'
export type FleetSoftwareSummary = {
  state: FleetSoftwareState
  headline: string
  /** What else is worth a short line. Never an instruction to read the rest of the page. */
  detail: string
  /** The label of the one button, or '' when there is nothing to press. */
  button: string
  /** The machines the button updates, in the order they are listed. */
  ready: string[]
}

function machinesLabel(count: number): string {
  return `${count} ${count === 1 ? 'machine' : 'machines'}`
}

/** "a", "a and b", "a, b and c", and a count once the list is too long to read as a sentence. */
function nameList(names: string[]): string {
  if (names.length > 3) return machinesLabel(names.length)
  if (names.length <= 1) return names[0] ?? ''
  return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`
}

function batchSummary(batch: SoftwareBatch): FleetSoftwareSummary {
  const members = batch.members.filter(member => member.state !== 'excluded')
  const nameOf = (member: SoftwareBatchMember) => member.host || member.agentId
  const none = { button: '', ready: [] as string[] }
  switch (batch.state) {
    case 'pending':
    case 'running': {
      const done = members.filter(member => member.state === 'applied' || member.state === 'skipped').length
      const current = members.find(member => member.state === 'running')
      return { ...none, state: 'working', headline: current ? `Updating ${nameOf(current)}…` : 'Updating your machines…', detail: `${done} of ${members.length} done.` }
    }
    case 'stopped': {
      const failed = batchFailedMember(batch)
      return { ...none, state: 'stopped', headline: failed ? `Stopped on ${nameOf(failed)}.` : 'Stopped on a failure.', detail: (failed && batchMemberNote(failed)) || 'Retry it, skip it and carry on, or stop here.' }
    }
    case 'done': {
      const applied = members.filter(member => member.state === 'applied').length
      const skipped = members.filter(member => member.state === 'skipped').map(nameOf)
      return {
        ...none, state: 'finished',
        headline: applied ? `Updated ${machinesLabel(applied)}.` : 'Nothing was updated.',
        detail: skipped.length ? `${nameList(skipped)} ${skipped.length === 1 ? 'was' : 'were'} skipped.` : '',
      }
    }
    default:
      return { ...none, state: 'finished', headline: 'The batch was abandoned.', detail: '' }
  }
}

/** The one button for these machines, and the machines it updates. */
function pressFor(ready: FleetSoftwareRow[]): { button: string; ready: string[] } {
  const installs = ready.filter(row => row.state === 'install').length
  return {
    button: installs === ready.length ? `Install on ${machinesLabel(ready.length)}` : `Update ${machinesLabel(ready.length)}`,
    ready: ready.map(row => row.agentId),
  }
}

/** One sentence for the fleet's software, and the one button that fixes what can be fixed now. */
export function fleetSoftwareSummary(rows: FleetSoftwareRow[], batch: SoftwareBatch | null): FleetSoftwareSummary {
  const ready = rows.filter(fleetSoftwareCanApply)
  if (batch) {
    const summary = batchSummary(batch)
    // Once a batch has ended, whatever can still update is one press away again, without starting over first.
    return summary.state === 'finished' && ready.length ? { ...summary, ...pressFor(ready) } : summary
  }
  if (!rows.length) return { state: 'empty', headline: 'No machines yet.', detail: '', button: '', ready: [] }
  const blocked = rows.filter(row => (row.state === 'update' || row.state === 'install') && !fleetSoftwareCanApply(row))
  const attention = rows.filter(row => row.state === 'unknown')
  const names = (list: FleetSoftwareRow[]) => nameList(list.map(row => row.host))
  const rest = [
    blocked.length ? `${names(blocked)} can't be updated right now.` : '',
    attention.length ? `${names(attention)} ${attention.length === 1 ? 'needs' : 'need'} a look.` : '',
  ].filter(Boolean)

  if (ready.length) {
    const installs = ready.filter(row => row.state === 'install').length
    const verb = installs === 0 ? 'updated' : installs === ready.length ? 'installed' : 'updated or installed'
    const targets = new Set(ready.map(row => row.target ?? ''))
    const target = targets.size === 1 ? [...targets][0] : ''
    return {
      state: 'ready',
      headline: `OpenCode can be ${verb} on ${machinesLabel(ready.length)}.`,
      detail: [`${names(ready)}${target ? `, to ${target}` : ''}.`, ...rest].join(' '),
      ...pressFor(ready),
    }
  }
  if (blocked.length) {
    return {
      state: 'blocked',
      headline: `${machinesLabel(blocked.length)} can't be updated right now.`,
      detail: blocked.length === 1 ? `${blocked[0].host}: ${fleetSoftwareWarning(blocked[0])}` : `${names(blocked)}. The reasons are listed under Machines.`,
      button: '', ready: [],
    }
  }
  if (attention.length) {
    return { state: 'attention', headline: `${machinesLabel(attention.length)} ${attention.length === 1 ? 'needs' : 'need'} a look.`, detail: `Check ${names(attention)} below.`, button: '', ready: [] }
  }
  return { state: 'current', headline: 'Everything is up to date.', detail: '', button: '', ready: [] }
}

/** What a person should do about a machine whose software could not be reviewed. */
export type SoftwareGuide = { title: string; why: string; found?: string; command?: string; then: string }

/** Guidance for an "unknown" fleet row: what to do, what to run on that machine, and why. Null when nothing needs doing. */
export function fleetSoftwareGuide(row: Pick<FleetSoftwareRow, 'state' | 'host' | 'reason' | 'evidence'>): SoftwareGuide | null {
  if (row.state !== 'unknown') return null
  switch (row.reason) {
    case 'agent_too_old': return {
      title: `Update reet on ${row.host}`,
      why: 'That machine runs a reet version that cannot report its installed software yet, so reet has nothing to plan from.',
      command: 'reet upgrade',
      then: 'Run it in a terminal on that machine, then press Refresh.',
    }
    case 'no_opencode_evidence': return {
      title: `Refresh ${row.host}'s software report`,
      why: 'The machine reported its software, but its report has no OpenCode entry.',
      command: 'reet upgrade',
      then: 'Run it on that machine, then press Refresh. If this stays, open that machine\'s Settings to see what it reported.',
    }
    case 'unreadable_install': return {
      title: `Check OpenCode on ${row.host}`,
      why: row.evidence?.reason === 'managed_broken'
        ? 'reet installed OpenCode here itself, but that copy can no longer be trusted, so reet will not use or change it.'
        : 'reet found OpenCode but could not tell how it was installed, so it will not guess at a plan.',
      found: evidenceFound(row.evidence),
      then: evidenceThen(row.evidence),
    }
    case 'no_plan': return {
      title: `No plan for ${row.host}`,
      why: 'reet has no reviewed plan for this machine\'s platform or installation, so it cannot change it safely.',
      found: evidenceFound(row.evidence),
      then: 'Nothing to run. Manage OpenCode on that machine yourself, or check back after a reet release.',
    }
    case 'read_failed': return {
      title: `Couldn't read ${row.host}`,
      why: 'reet failed to load that machine\'s record just now. This is usually temporary.',
      then: 'Press Refresh to try again. If it keeps failing, check that the machine is still connected.',
    }
    default: return {
      title: `Check ${row.host}`,
      why: 'reet could not review this machine\'s software.',
      then: 'Press Refresh. If it stays, open that machine\'s Settings to see what it reported.',
    }
  }
}

type FleetEvidence = NonNullable<FleetSoftwareRow['evidence']>

/** What the machine reported about OpenCode, in words, or undefined when it reported nothing useful. */
function evidenceFound(evidence?: FleetEvidence): string | undefined {
  if (!evidence) return undefined
  const where = evidence.path ? ` at ${evidence.path}` : ''
  switch (evidence.reason) {
    case 'unreadable': return `reet found OpenCode${where} but couldn't read its version.`
    case 'package_unknown': return `OpenCode is installed${where}, but reet doesn't recognise the package.`
    case 'platform_unsupported': return 'reet has no reviewed OpenCode build for that machine\'s platform.'
    case 'managed_broken': return 'Its integrity check failed: the saved receipt or the program file no longer matches what reet installed.'
    default: return evidence.path ? `OpenCode was seen at ${evidence.path}.` : undefined
  }
}

function evidenceThen(evidence?: FleetEvidence): string {
  switch (evidence?.reason) {
    case 'managed_broken': return 'Settings has no reinstall for this yet. On that machine, delete the folder ~/.reet/software/opencode (this removes reet\'s own OpenCode copy and its rollback version), then press Refresh and apply the install plan.'
    case 'platform_unsupported': return 'Nothing to run. Manage OpenCode on that machine yourself, or check back after a reet release.'
    default: return 'Nothing to run here. Fix or reinstall OpenCode on that machine, then press Refresh.'
  }
}
