import type { FleetSetupBatch, FleetSetupFile, FleetSetupItem, FleetSetupMachine, FleetSetupMember } from './terminal/api'

const AGENTS: Record<string, string> = { claude: 'Claude Code', codex: 'Codex', opencode: 'OpenCode', pi: 'Pi' }

/** The agent's name as a person says it. */
export function agentName(agent: string): string {
  return AGENTS[agent] ?? agent
}

function machinesLabel(count: number): string {
  return `${count} ${count === 1 ? 'machine' : 'machines'}`
}

/** "a", "a and b", "a, b and c". */
function list(names: string[]): string {
  if (names.length <= 1) return names[0] ?? ''
  return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`
}

/** "Claude Code instructions (CLAUDE.md)": what a copied item is, in plain words. */
export function setupItemName(item: Pick<FleetSetupItem, 'agent' | 'kind' | 'name'>): string {
  return `${agentName(item.agent)} ${item.kind === 'instructions' ? 'instructions' : 'settings'} (${item.name})`
}

export function sizeLabel(bytes: number): string {
  if (bytes < 1024) return `${bytes} ${bytes === 1 ? 'byte' : 'bytes'}`
  return `${(bytes / 1024).toFixed(bytes < 10 * 1024 ? 1 : 0)} KB`
}

/** Why a file is left alone on a machine. */
export function blockedReason(reason?: string): string {
  switch (reason) {
    case 'no_location': return 'reet has no place for it on that machine'
    case 'unreadable': return "reet couldn't read the file there"
    case 'destination_symlink': return 'the file there is a symbolic link'
    case 'destination_error': return "reet couldn't check the file there"
    case 'destination_unsupported': return "the file there isn't a regular file"
    default: return 'reet will not touch it'
  }
}

/** Said on its own line in the review and again in the question, so it is never missed. */
export const NEVER_COPIED = 'Credentials, MCP servers, model defaults and project files are never copied.'
export const KEPT_ONLY_WHILE_COPYING = 'The controller holds the copied text only until the copy finishes, and for 24 hours at most.'

/** The word on a file's chip: an instruction file's text is replaced, a settings file is merged field by field. */
export function fileChip(file: Pick<FleetSetupFile, 'kind' | 'state'>): string {
  switch (file.state) {
    case 'create': return 'Create'
    case 'merge': return file.kind === 'instructions' ? 'Replace' : 'Merge'
    case 'unchanged': return 'Unchanged'
    default: return 'Blocked'
  }
}

/** One line for what a machine's plan does with one file. */
export function fleetFileLine(file: Pick<FleetSetupFile, 'agent' | 'kind' | 'name' | 'state' | 'fields' | 'reason'>): string {
  const what = `${agentName(file.agent)} ${file.name}`
  const fields = file.fields?.length ? ` ${list(file.fields)}` : ''
  switch (file.state) {
    case 'create': return `Creates ${what}${file.kind === 'settings' && fields ? ` with${fields}` : ''}.`
    case 'merge': return file.kind === 'settings'
      ? `Changes${fields || ' some fields'} in ${what}.`
      : `Replaces the text of ${what}.`
    case 'unchanged': return `${what} is already the same.`
    default: return `${what} is left alone: ${blockedReason(file.reason)}.`
  }
}

/** The permission fields that would let agents do more without asking, named with their agent: "Claude Code permissions". */
export function widenedFields(files: Pick<FleetSetupFile, 'agent' | 'widens'>[]): string[] {
  return [...new Set(files.flatMap(file => (file.widens ?? []).map(field => `${agentName(file.agent)} ${field}`)))]
}

/** What a widening machine would be given: each file that widens, the fields that widen and any added lines the plan lists. Only what the plan says. */
export function widenEntries(files: Pick<FleetSetupFile, 'agent' | 'name' | 'widens' | 'added'>[]): { file: string; fields: string[]; added: string[] }[] {
  return files.filter(file => file.widens?.length).map(file => ({ file: `${agentName(file.agent)} ${file.name}`, fields: file.widens ?? [], added: file.added ?? [] }))
}

/** The typed word's label, naming the machines whose permissions widen. */
export function confirmLabel(machines: Pick<FleetSetupMachine, 'host' | 'widens'>[]): string {
  const hosts = machines.filter(machine => machine.widens).map(machine => machine.host)
  return `Type ${CONFIRM_WORD} to allow wider permissions${hosts.length ? ` on ${list(hosts)}` : ''}`
}

/** The warning for a machine whose copy widens what agents may do, naming what widens. */
export function widenWarning(host: string, files: Pick<FleetSetupFile, 'agent' | 'widens'>[]): string {
  const fields = widenedFields(files)
  return `This copy lets agents do more without asking on ${host}${fields.length ? `: ${list(fields)}` : ''}.`
}

/** The files a plan would write. */
export function changingFiles<T extends Pick<FleetSetupFile, 'state'>>(files: T[]): T[] {
  return files.filter(file => file.state === 'create' || file.state === 'merge')
}

/** What a changed line count reads like in a summary: "2 added, 1 removed". */
export function diffCounts(file: Pick<FleetSetupFile, 'added' | 'removed'>): string {
  const added = file.added?.length ?? 0, removed = file.removed?.length ?? 0
  return [added ? `${added} added` : '', removed ? `${removed} removed` : ''].filter(Boolean).join(', ')
}

/** Why a file's lines are not shown: the machine's own file looks like it holds a credential, so the plan carries none of its text. */
export function credentialNote(file: Pick<FleetSetupFile, 'holdsCredential'>): string {
  return file.holdsCredential ? 'This file on that machine looks like it holds a credential, so its lines are not shown.' : ''
}

/** Why a machine is left out of the review, from its closed code; the controller's own sentence when the code is new. */
export function machineExcluded(machine: Pick<FleetSetupMachine, 'code' | 'reason'>): string {
  switch (machine.code) {
    case 'same_machine': return 'This is the machine the setup was read from.'
    case 'not_enrolled': return "That machine isn't enrolled."
    case 'unsupported': return 'Update reet on that machine to copy a setup to it.'
    case 'terminal_off': return 'Terminal access is off on that machine.'
    case 'locked': return 'That machine is locked.'
    case 'not_allowed': return "That machine doesn't allow setup changes. Run reet agent allow-apply on there to allow them."
    case 'offline': return "That machine isn't connected."
    case 'busy': return 'That machine is already reading or writing its setup. Read again in a moment.'
    case 'plan_failed': return "reet couldn't get a plan from that machine."
    case 'unchanged': return 'Nothing to change: it already has this setup.'
    default: return machine.reason || ''
  }
}

export function machineStateLabel(machine: Pick<FleetSetupMachine, 'state'>): string {
  return machine.state === 'ready' ? 'Ready' : machine.state === 'unchanged' ? 'Already the same' : 'Left out'
}

/** The machines a person can choose: those with something to copy. */
export function readyMachines<T extends Pick<FleetSetupMachine, 'state'>>(machines: T[]): T[] {
  return machines.filter(machine => machine.state === 'ready')
}

/** True when any chosen machine would let agents do more without asking. */
export function anyWidens(machines: Pick<FleetSetupMachine, 'widens'>[]): boolean {
  return machines.some(machine => machine.widens)
}

export const CONFIRM_WORD = 'copy'

/** True when what was typed answers the question. Nothing needs typing unless a machine widens. */
export function confirmed(typed: string, needsWord: boolean): boolean {
  return !needsWord || typed.trim().toLowerCase() === CONFIRM_WORD
}

/** The `confirm` the request carries: the word, only when one was needed. */
export function confirmToSend(typed: string, needsWord: boolean): string | undefined {
  return needsWord && confirmed(typed, true) ? CONFIRM_WORD : undefined
}

export function copyButton(count: number): string {
  return `Copy to ${machinesLabel(count)}`
}

/** One line for a machine's place in the batch. */
export function setupMemberLabel(member: Pick<FleetSetupMember, 'state' | 'undoneAt'>): string {
  switch (member.state) {
    case 'applied': return member.undoneAt ? 'Undone' : 'Copied'
    case 'running': return 'Copying'
    case 'failed': return 'Failed'
    case 'skipped': return 'Skipped'
    case 'excluded': return 'Left out'
    default: return 'Waiting'
  }
}

/** What a member's stored code or reason means, or '' when it says nothing. */
export function setupMemberNote(member: Pick<FleetSetupMember, 'state' | 'code' | 'reason'>): string {
  if (member.state === 'excluded') return machineExcluded(member) || member.reason || ''
  switch (member.code) {
    // Skipped: nothing reached the machine.
    case 'offline': return "That machine wasn't connected when its turn came, so nothing was changed on it."
    case 'abandoned': return 'The copy was abandoned before this machine had its turn. Nothing was changed.'
    case 'expired': return 'The copy waited more than 24 hours, so reet dropped it. Nothing was changed.'
    case 'paused_by_restore': return 'This controller was restored from a copy, so this machine was held back. Retry it to continue.'
    case 'not_reviewed': return "That machine wasn't in the review. Read the source machine again to include it."
    // Failed.
    case 'connection_lost': return 'The connection dropped while that machine was applying, so it may or may not have finished. Retry checks before it writes anything.'
    case 'changed': return "That machine's files changed since you reviewed the copy, so nothing was written. Skip it, then read the source again to review it afresh."
    case 'not_allowed': return "That machine doesn't allow setup changes. Run reet agent allow-apply on there, then retry."
    case 'locked': return 'That machine is locked. Unlock it, then retry.'
    case 'off': return 'Terminal access is off on that machine. Turn it on, then retry.'
    case 'busy': return 'That machine was already reading or writing its setup. Retry in a moment.'
    case 'nothing': return 'There was nothing left to change on that machine.'
    case 'invalid': return 'That machine refused the copy as malformed. Nothing was changed.'
    case 'write_failed': return "That machine couldn't write a file. It put back what it had already changed."
    case 'timed_out': return 'That machine took too long to answer, so it may or may not have finished. Retry checks before it writes anything.'
    case 'apply_unsupported': return 'Update reet on that machine to copy a setup to it.'
    case 'bad_answer': return 'That machine sent an answer reet could not read.'
    case 'apply_failed': return "That machine's copy did not finish."
    case 'content_expired': return 'The controller no longer holds the copied text. Abandon this copy and read the source again.'
  }
  return member.reason || ''
}

export function setupBatchStateLabel(state: FleetSetupBatch['state']): string {
  switch (state) {
    case 'pending': return 'Waiting to start'
    case 'running': return 'Copying'
    case 'stopped': return 'Stopped on a failure'
    case 'done': return 'Finished'
    case 'abandoned': return 'Abandoned'
  }
}

/** True when the batch can be abandoned now: it has not ended and nothing is applying. */
export function setupBatchCanAbandon(batch: Pick<FleetSetupBatch, 'state' | 'members'>): boolean {
  return batch.state !== 'done' && batch.state !== 'abandoned' && !batch.members.some(member => member.state === 'running')
}

export function setupFailedMember<T extends Pick<FleetSetupMember, 'state'>>(batch: { members: T[] }): T | undefined {
  return batch.members.find(member => member.state === 'failed')
}

/** An applied machine can be undone once, while the controller still has its journal. */
export function setupMemberCanUndo(member: Pick<FleetSetupMember, 'state' | 'journal' | 'undoneAt'>): boolean {
  return member.state === 'applied' && !!member.journal && !member.undoneAt
}

/** The batch in one sentence, and one more line when something is worth saying. */
export function setupBatchSummary(batch: Pick<FleetSetupBatch, 'state' | 'members'>): { state: 'working' | 'stopped' | 'finished' | 'undone'; headline: string; detail: string } {
  const members = batch.members.filter(member => member.state !== 'excluded')
  const name = (member: FleetSetupMember) => member.host || member.agentId
  switch (batch.state) {
    case 'pending':
    case 'running': {
      const current = members.find(member => member.state === 'running')
      const done = members.filter(member => member.state === 'applied' || member.state === 'skipped').length
      return { state: 'working', headline: `Copying to ${current ? name(current) : 'your machines'}…`, detail: `${done} of ${members.length} done.` }
    }
    case 'stopped': {
      const failed = setupFailedMember(batch)
      return { state: 'stopped', headline: failed ? `Stopped on ${name(failed)}.` : 'Stopped on a failure.', detail: failed ? `Retry ${name(failed)}, skip it and carry on, or abandon the copy.` : 'Retry it, skip it and carry on, or abandon the copy.' }
    }
    case 'done': {
      const applied = members.filter(member => member.state === 'applied')
      const undone = applied.filter(member => member.undoneAt).length
      const skipped = members.filter(member => member.state === 'skipped').map(name)
      const kept = applied.length - undone
      return {
        // Once every copied machine is put back, the copy is not a success to show as one.
        state: !kept && applied.length ? 'undone' : 'finished',
        headline: kept ? `Copied to ${machinesLabel(kept)}.` : applied.length ? `Copy undone on ${machinesLabel(applied.length)}.` : 'Nothing was copied.',
        detail: [undone && kept ? `${undone} undone.` : '', skipped.length ? `${list(skipped)} ${skipped.length === 1 ? 'was' : 'were'} skipped.` : ''].filter(Boolean).join(' '),
      }
    }
    default:
      return { state: 'finished', headline: 'The copy was abandoned.', detail: '' }
  }
}

/** A plain sentence for a refusal, by its code. The controller's own sentence is used where it names a file. */
export function setupErrorMessage(error: unknown, fallback: string): string {
  const { code, message } = (error ?? {}) as { code?: string; message?: string }
  switch (code) {
    case 'machine_not_found': return "That machine isn't enrolled."
    case 'setup_unsupported': return 'That machine needs a newer reet, with terminal access on, for this. Update reet there and try again.'
    case 'setup_offline': return "That machine isn't connected. Try again when it reconnects."
    case 'setup_terminal_off': return 'Terminal access is off on that machine.'
    case 'setup_locked': return 'That machine is locked.'
    case 'setup_busy': return 'That machine is already reading or writing its setup. Try again in a moment.'
    case 'setup_nothing': return 'That machine has no global instruction files or settings reet can copy.'
    case 'setup_too_large': return "That machine's setup is too large for reet to send in one message (about 60 KB). Shorten its instruction files, then read it again."
    case 'setup_credential': return message && /^reet won't copy /.test(message) ? message : "A file on that machine looks like it holds a credential, so reet won't copy it. Remove the credential there, then read it again."
    case 'setup_read_invalid': return "That machine sent a setup reet can't copy."
    case 'setup_failed': return "That machine couldn't do it. Nothing was changed."
    case 'setup_timed_out': return 'That machine took too long to answer.'
    case 'setup_not_allowed': return "That machine doesn't allow setup changes. Run reet agent allow-apply on there to allow them."
    case 'setup_request_invalid': return 'Choose the machine to copy from, or the machines to copy to.'
    case 'setup_batch_empty': return 'Choose between one and 100 machines that have something to copy.'
    case 'setup_batch_duplicate': return 'A machine appears more than once in that copy.'
    case 'setup_batch_source_only': return 'Choose a machine other than the one the setup was read from.'
    case 'setup_batch_in_progress': return 'Another setup copy is still waiting or running. Finish or abandon it first.'
    case 'setup_read_expired': return 'That review has expired. Read the source machine again.'
    case 'request_id_required': return "That copy couldn't be started. Try again."
    case 'setup_confirm_required': return 'Type copy to confirm. This copy lets agents do more without asking on at least one machine.'
    case 'setup_batch_not_found': return "That setup copy doesn't exist any more."
    case 'setup_batch_member_not_failed': return "That machine isn't waiting on a failure."
    case 'setup_batch_finished': return 'That setup copy has already finished.'
    case 'setup_batch_running': return 'A machine in that copy is still applying. Try again when it finishes.'
    case 'software_batch_position_invalid': return "That machine isn't in this copy."
    case 'setup_undo_done': return 'That copy was already undone.'
    case 'setup_undo_unavailable': return 'That machine has no applied copy to undo.'
    case 'setup_changed': return 'A file on that machine changed after the copy, so reet left it alone. Nothing was changed.'
    case 'setup_no_journal': return "That machine no longer has the copy's journal, so it can't be undone from here."
  }
  return message || fallback
}
