import { enrollCommands, scanCommand, shellQuote } from './machineSetup'

export type SyncProblemKind =
  | 'skills-folder'
  | 'skills-folder-unreadable'
  | 'remote-apply'
  | 'background-agent'
  | 'enrollment'
  | 'project'
  | 'scan'
  | 'stale-plan'
  | 'busy'
  | 'source'
  | 'paused'
  | 'unconnected'
  | 'review'
  | 'other'

export type SyncProblem = {
  kind: SyncProblemKind
  title: string
  detail: string
  /** Exact commands, already shell-quoted, to run on the machine. */
  commands: string[]
  hint?: string
  /** A fresh scan or preview can clear this without changing anything on the machine. */
  retry: boolean
  /** The person still has to run something on the machine before a retry can work. */
  local: boolean
  /** A page that finishes the job when there is no command to run. */
  href?: string
}

export type SyncProblemContext = {
  host?: string
  site?: string
  project?: string
}

const missingFolder = /cannot read target skills directory:\s*open\s+(.+?):\s*(?:no such file or directory|the system cannot find the path specified)/i
const targetFolder = /cannot read target skills directory/i
const remoteApply = /remote apply disabled locally/i
const backgroundAgent = /requires the long-running background or service agent/i
const enrollmentOwner = /enrollment belongs to another local machine or user/i
const alreadyEnrolled = /configuration already exists/i
const projectScope = /project is not enrolled/i
const scanBlocked = /scan incomplete|no changes allowed|cannot read target skills directory[\s\S]*permission denied/i
const stalePlan = /approved plan no longer matches|target identity or revision changed|controller target changed|local skills changed|desired revision changed|preview again/i
const busy = /another sync may be running|cannot lock target/i
const sharedSource = /is shared with|source conflict|local-only skill|no supported install source|has no supported install source|duplicate skill names/i
const paused = /sync is paused/i
const unconnected = /has no reet agent yet|no machine named|is connected\. refresh machines/i
const needsReview = /local changes need review|reconcile/i

function machine(context: SyncProblemContext): string {
  return context.host?.trim() || 'the machine'
}

function site(context: SyncProblemContext): string {
  return context.site?.trim() || 'https://reet.sh'
}

function agentCommand(context: SyncProblemContext): string {
  return `reet agent enroll --server ${shellQuote(site(context))} --env ${shellQuote(machine(context))}`
}

/**
 * Turns an agent failure into the same three things every time: what went
 * wrong, the exact command that fixes it, and whether trying again from the
 * web is safe. Keep the raw message out of sight unless it is all we have.
 */
export function explainSyncFailure(message: string, context: SyncProblemContext = {}): SyncProblem {
  const text = (message ?? '').trim()
  const host = machine(context)

  const folder = missingFolder.exec(text)
  if (folder) {
    const directory = folder[1].trim()
    return {
      kind: 'skills-folder',
      title: 'Skills folder missing',
      detail: `${host} has no skills folder at ${directory}. Create it there, then retry. A current agent also creates it on its next sync.`,
      commands: [`mkdir -p ${shellQuote(directory)}`],
      hint: 'If this machine uses a custom --config, add it to the command.',
      retry: true,
      local: true,
    }
  }
  if (remoteApply.test(text)) {
    return {
      kind: 'remote-apply',
      title: 'Applying changes is off on this machine',
      detail: `Turn it on at ${host}, then retry. Reet cannot flip it from the web — that is what stops an approved plan writing on its own.`,
      commands: ['reet agent allow-apply on'],
      hint: 'Scans and previews work without it. If the command is unknown, update the CLI with reet upgrade first.',
      retry: true,
      local: true,
    }
  }
  if (backgroundAgent.test(text)) {
    return {
      kind: 'background-agent',
      title: 'The background agent is not running',
      detail: `Start it on ${host}, then retry. The update needs the long-running agent so it can restart into the new build.`,
      commands: ['reet agent start', 'reet agent status'],
      retry: true,
      local: true,
    }
  }
  if (enrollmentOwner.test(text)) {
    return {
      kind: 'enrollment',
      title: 'This enrollment belongs to another machine or user',
      detail: `The private configuration on ${host} was made elsewhere. Enroll this installation separately. If it is the same machine, revoke the old enrollment in Machines first.`,
      commands: [agentCommand(context), 'reet agent run'],
      retry: true,
      local: true,
    }
  }
  if (alreadyEnrolled.test(text)) {
    return {
      kind: 'enrollment',
      title: 'This machine is already enrolled',
      detail: 'A private configuration is already here, so re-enrolling is refused. Check the existing connection instead.',
      commands: ['reet agent status'],
      retry: true,
      local: true,
    }
  }
  if (projectScope.test(text)) {
    const project = context.project?.trim()
    return {
      kind: 'project',
      title: 'That project is not part of this enrollment',
      detail: `The agent only works in the folders it was enrolled with. Add ${project || 'the project'} to this installation's scopes on the machine, then retry.`,
      commands: [`${agentCommand(context)}${project ? ` --project ${shellQuote(project)}` : ' --project /absolute/path'}`],
      retry: true,
      local: true,
    }
  }
  if (scanBlocked.test(text)) {
    return {
      kind: 'scan',
      title: 'The scan could not read every folder',
      detail: 'Reet keeps the last good inventory when part of a scan cannot be read, so a permission problem never looks like a removal. Fix the folder, then ask for a fresh scan.',
      commands: [scanCommand(site(context), host)],
      retry: true,
      local: true,
    }
  }
  if (targetFolder.test(text)) {
    return {
      kind: 'skills-folder-unreadable',
      title: 'Reet could not read the skills folder',
      detail: `The folder exists on ${host} but could not be read. Check its permissions and owner, then retry. Reet will not treat an unreadable folder as empty.`,
      commands: [],
      retry: true,
      local: true,
    }
  }
  if (paused.test(text)) {
    return {
      kind: 'paused',
      title: 'Sync is paused for this machine',
      detail: 'Resume sync in the machine settings, then retry. Pausing stops new jobs and periodic scans; a running operation may finish.',
      commands: [],
      retry: false,
      local: false,
      href: context.host ? `/machines?machine=${encodeURIComponent(host)}` : '/machines',
    }
  }
  if (unconnected.test(text)) {
    return {
      kind: 'unconnected',
      title: 'That machine is not connected yet',
      detail: text || `Connect ${host} once, then the install works from here in one click.`,
      commands: [enrollCommands(site(context), host)],
      retry: true,
      local: true,
      href: '/machines#add-machine',
    }
  }
  if (needsReview.test(text)) {
    return {
      kind: 'review',
      title: 'Local changes need a review first',
      detail: 'The machine has skills that changed since the saved set. Review both sides, save the selection, then preview again. Nothing was changed.',
      commands: [],
      retry: false,
      local: false,
      href: '/sync#sync-selection',
    }
  }
  if (stalePlan.test(text)) {
    return {
      kind: 'stale-plan',
      title: 'The plan changed before it ran',
      detail: 'Reet refuses to apply a plan that no longer matches the files or revision it checked. Nothing changed. A fresh preview is safe.',
      commands: [],
      retry: true,
      local: false,
    }
  }
  if (busy.test(text)) {
    return {
      kind: 'busy',
      title: 'Another sync is using this machine',
      detail: 'Wait for the other run to finish, then retry. A stale lock is safe to remove only after checking that no sync is still running.',
      commands: [],
      retry: true,
      local: false,
    }
  }
  if (sharedSource.test(text)) {
    return {
      kind: 'source',
      title: 'This skill needs a source decision',
      detail: `${text} Link the skill to a source, or move the other copy aside, then retry.`,
      commands: [],
      retry: false,
      local: false,
      href: '/sync#sync-selection',
    }
  }
  return {
    kind: 'other',
    title: 'That did not finish',
    detail: text || 'The machine stopped before saying why. Try again, or open Sync activity for the full message.',
    commands: [],
    retry: true,
    local: false,
  }
}
