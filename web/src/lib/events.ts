import type { Event } from './api'
import { launchActivity, toolName } from './agentLaunch'

type Detail = {
  host?: string
  name?: string
  username?: string
  published?: boolean
  from?: string
  to?: string
  imported?: number
  created?: number
  ignored?: number
  pruned?: number
  restored?: number
  already?: boolean
  count?: number
  files?: number
  liked?: boolean
  role?: string
  email?: string
  disabled?: boolean
  enabled?: boolean
  connection?: string
  ok?: boolean
  template?: string
  deleted?: boolean
  removed?: boolean
  list?: string
  ticket?: number
  kind?: string
  message?: number
  status?: string
  assignee?: number
  by?: string
  subject?: string
  group?: string
  tool?: string
  model?: string
  dir?: string
  members?: number
  previousName?: string
  position?: number
  attempt?: number
  branch?: string
  project?: string
  folder?: string
  folderProjects?: unknown[]
  path?: string
  pages?: number
  machine?: string
  state?: string
  defaultTool?: string
  defaultModel?: string
  defaultEffort?: string
}

function detail(e: Event): Detail {
  if (!e.detail) return {}
  try {
    return JSON.parse(e.detail) as Detail
  } catch {
    return {}
  }
}

const repositories = (n: number): string => `${n} ${n === 1 ? 'repository' : 'repositories'}`

function subject(e: Event, d: Detail): string {
  return d.name || e.skillName || e.skillId || 'a skill'
}

export function eventLabel(e: Event): string {
  const d = detail(e)
  const name = subject(e, d)
  switch (e.action) {
    case 'merge-machines': return `Consolidated machine ${d.from || ''} as ${d.to || ''}`
    case 'forget-machine': return `Forgot machine ${d.host || ''}`
    case 'terminal-launch': return launchActivity(d)
    case 'yolo-launch': return `Started ${toolName(d.tool || '')} on YOLO${d.host || d.machine ? ` on ${d.host || d.machine}` : ''}`
    case 'yolo-allow': return `YOLO allowed a request in a ${toolName(d.tool || '')} session`
    case 'yolo-allowed': return 'Allowed YOLO sessions for the account'
    case 'yolo-disallowed': return 'Turned off YOLO sessions for the account'
    case 'project-create': return d.name ? `Created project ${d.name}${typeof d.members === 'number' ? ` with ${repositories(d.members)}` : ''}` : 'Created a project'
    case 'project-update': return d.previousName && d.name ? `Renamed project ${d.previousName} to ${d.name}` : d.name ? `Updated project ${d.name}` : 'Updated a project'
    case 'project-delete': return d.name ? `Deleted project ${d.name}` : 'Deleted a project'
    case 'project-run': return typeof d.members === 'number' ? `Started a Project run on ${repositories(d.members)}` : 'Started a Project run'
    case 'project-run-retry': return typeof d.position === 'number' ? `Tried repository ${d.position + 1} of a Project run again${typeof d.attempt === 'number' ? ` (attempt ${d.attempt})` : ''}` : 'Tried a repository of a Project run again'
    case 'projects-migrated': {
      if (!d.project) return 'Moved a saved Project into the rail'
      const n = Array.isArray(d.folderProjects) ? d.folderProjects.length : 0
      return n > 0 ? `Moved saved Project ${d.project} into the rail as ${n} ${n === 1 ? 'project' : 'projects'}` : `Looked at saved Project ${d.project}: none of its folders could become a project`
    }
    case 'project-add': return d.name ? `Added project ${d.name}${d.folder ? ` (${d.folder} on ${d.host || 'a machine'})` : ''}` : 'Added a project'
    case 'project-remove': return d.name ? `Removed project ${d.name}; its folder on ${d.host || 'its machine'} is untouched` : 'Removed a project'
    case 'folder-browse': return d.path ? `Browsed folders from ${d.path}${typeof d.pages === 'number' ? ` (${d.pages} ${d.pages === 1 ? 'page' : 'pages'})` : ''}` : 'Browsed folders on a machine'
    case 'worktree-remove': return `Removed a worktree on ${d.host || 'a machine'}${d.branch ? `; the branch ${d.branch} is kept` : ''}`
    case 'sync-agent-update': return `Queued a reet update for ${d.name || 'a machine'}`
    case 'sync-agent-auto-update': return `Queued a reet update for ${d.name || 'a machine'} automatically`
    case 'add':
      return `Added ${name}`
    case 'like':
      return `Liked ${name}`
    case 'unlike':
      return `Unliked ${name}`
    case 'stage':
      return `Moved ${name} to ${d.to || 'another stage'}`
    case 'notes':
      return `Updated notes on ${name}`
    case 'tag':
      return `Changed tags on ${name}`
    case 'remove':
      return `Removed ${name}`
    case 'ignore':
      return `Ignored ${name} for future scans`
    case 'unignore':
      return `Allowed ${name} in future scans`
    case 'link':
      return `Linked ${name} to skills.sh`
    case 'refresh':
      return `Refreshed ${name}`
    case 'install-copy':
      return `Copied an install command for ${name}`
    case 'scan': {
      const found = d.imported ?? 0
      const gone = d.pruned ?? 0
      return gone > 0 ? `Scanned ${found} skills, removed ${gone}` : `Scanned ${found} skills`
    }
    case 'import':
      return `Imported ${d.imported ?? 0} skills`
    case 'restore':
      return `Restored ${d.restored ?? 0} skills`
    case 'export':
      return 'Exported the shelf'
    case 'tag-rename':
      return 'Renamed a tag'
    case 'tag-merge':
      return 'Merged two tags'
    case 'patch':
      return `Updated ${name}`
    case 'bulk-stage':
      return `Moved ${d.count ?? 0} skills to ${d.to || 'another stage'}`
    case 'bulk-like':
      return `${d.liked ? 'Liked' : 'Unliked'} ${d.count ?? 0} skills`
    case 'bulk-tag':
      return `Tagged ${d.count ?? 0} skills`
    case 'bulk-remove':
      return `Removed ${d.count ?? 0} skills`
    case 'bulk-ignore':
      return `Ignored ${d.count ?? 0} skills for future scans`
    case 'bulk-patch':
      return `Updated ${d.count ?? 0} skills`
    case 'settings':
      return 'Changed instance settings'
    case 'notification-prefs':
      return 'Changed their notification choices'
    case 'notification-defaults':
      return 'Changed the notification defaults'
    case 'ticket-open':
      return `Opened support ticket #${d.ticket ?? ''}${d.subject ? `: ${d.subject}` : ''}`.trim()
    case 'ticket-reply':
      return `Replied to support ticket #${d.ticket ?? ''}`
    case 'ticket-note':
      return `Added an internal note to support ticket #${d.ticket ?? ''}`
    case 'ticket-close':
      return `Closed support ticket #${d.ticket ?? ''}`
    case 'ticket-status':
      return `Changed support ticket #${d.ticket ?? ''} to ${d.status || 'another status'}`
    case 'ticket-assign':
      return `Reassigned support ticket #${d.ticket ?? ''}`
    case 'mail-settings':
      return `Changed the mail server${d.host ? ` (${d.host})` : ''}`
    case 'mail-check':
      return `${d.ok ? 'Checked' : 'Failed to check'} the mail server${d.host ? ` (${d.host})` : ''}`
    case 'mail-test':
      return `Sent a test email to ${d.to || 'an administrator'}${d.ok === false ? ' (failed)' : ''}`
    case 'mail-resend':
      return `Queued ${d.template || 'an email'} to ${d.to || 'a recipient'} again`
    case 'password-reset-request':
      return `Password reset requested for ${d.email || 'an account'}`
    case 'password-reset':
      return `${d.name || 'Someone'} reset their password`
    case 'group-notify':
      return `Changed team notifications${d.enabled === false ? ' (paused)' : ''}`
    case 'group-create':
      return 'Created a team or organisation'
    case 'group-update':
      return d.deleted ? 'Deleted a team or organisation' : 'Renamed a team or organisation'
    case 'group-membership':
      return d.removed ? 'Removed a group member' : 'Changed group membership'
    case 'list-create':
      return `Created the list ${d.list || ''}`.trim()
    case 'list-update':
      return `Updated the list ${d.list || ''}`.trim()
    case 'token-create':
      return 'Created an API token'
    case 'token-revoke':
      return 'Revoked an API token'
    case 'mcp-access':
      return typeof d.enabled === 'boolean' ? (d.enabled ? 'Enabled MCP access' : 'Paused MCP access') : 'Updated browser WebMCP permissions'
    case 'mcp-create':
      return `Created the MCP connection ${d.name || ''}`.trim()
    case 'mcp-authorize':
      return `Authorized ${d.name || 'an MCP client'}`
    case 'mcp-update':
      return `Updated the MCP connection ${d.name || ''}`.trim()
    case 'mcp-rotate':
      return `Replaced the bearer token for ${d.name || 'an MCP connection'}`
    case 'mcp-revoke':
      return `Revoked ${d.name || 'an MCP connection'}`
    case 'mcp-revoke-all':
      return 'Revoked all MCP connections'
    case 'mcp-call':
      return `Made a change through ${d.connection || 'an MCP client'}`
    case 'user-role':
      return `Changed ${d.name || 'a user'}'s role to ${d.role}`
    case 'user-disable':
      return `${d.name || 'A user'} ${d.disabled ? 'disabled' : 'enabled'}`
    case 'user-reset':
      return `Reset the password for ${d.name || 'a user'}`
    case 'user-revoke':
      return `Revoked ${d.name || 'a user'}'s API and MCP tokens`
    case 'user-transfer':
      return `${d.name || 'A user'} is the new owner`
    case 'user-delete':
      return `Deleted ${d.name || 'a user'}`
    case 'invite':
      return `Invited ${d.email || 'someone'}`
    case 'join':
      return `${d.name || 'Someone'} joined`
    case 'setup':
      return 'Created the owner account'
    case 'sign-in':
      return `${d.name || 'Someone'} signed in`
    case 'sign-out':
      return `${d.name || 'Someone'} signed out`
    case 'publishing-username':
      return `Claimed publishing username ${d.username || ''}`.trim()
    case 'hosted-save':
      return `${d.published ? 'Published' : 'Saved a draft of'} ${name}`
    case 'hosted-delete':
      return `Deleted hosted skill ${name}`
    case 'profile': {
      const changed: string[] = [], cleared: string[] = []
      const word = (xs: string[]): string => xs.length > 1 ? `${xs.slice(0, -1).join(', ')} and ${xs[xs.length - 1]}` : xs[0] ?? ''
      if (Object.hasOwn(d, 'defaultTool')) (d.defaultTool ? changed : cleared).push(d.defaultTool ? `agent to ${toolName(d.defaultTool)}` : 'the default agent')
      if (Object.hasOwn(d, 'defaultModel')) (d.defaultModel ? changed : cleared).push(d.defaultModel ? `model to ${d.defaultModel}` : 'the default model')
      if (Object.hasOwn(d, 'defaultEffort')) (d.defaultEffort ? changed : cleared).push(d.defaultEffort ? `thinking to ${d.defaultEffort}` : 'the default thinking')
      if (changed.length && cleared.length) return `Changed the default ${word(changed)}, and cleared ${word(cleared)}`
      if (changed.length) return `Changed the default ${word(changed)}`
      if (cleared.length) return `Cleared ${word(cleared)}`
      return 'Updated your profile'
    }
    case 'password-change':
      return 'Changed your password'
    case 'session-end':
      return 'Signed out a session'
    case 'session-end-others':
      return `Signed out ${d.count ?? 0} other sessions`
    case 'backup-settings':
      return 'Changed backup settings'
    case 'backup-run':
      return 'Took a backup'
    case 'backup-delete':
      return 'Deleted a backup from this machine'
    case 'backup-restore':
      return `Restored ${d.restored ?? 0} skills from a backup`
    case 'backup-remote-add':
      return `Added backup destination ${d.name || ''}`.trim()
    case 'backup-remote-change':
      return `Changed backup destination ${d.name || ''}`.trim()
    case 'backup-remote-remove':
      return `Removed backup destination ${d.name || ''}`.trim()
    case 'skill-content-adopt':
      return `Recorded ownership of ${d.files === 1 ? '1 file' : `${d.files ?? 0} files`} of ${name}`
    case 'skill-content-release':
      return `Released ownership of ${name}`
    case 'machine-models-read': {
      const tool = toolName(d.tool)
      switch (d.state) {
        case 'listed': return `Read ${tool}'s models${typeof d.count === 'number' ? ` (${d.count})` : ''}`
        case 'unsupported': return `${tool} can't list its models on that machine`
        case 'failed': return `Couldn't read ${tool}'s models on that machine`
        case 'off': return `Model reads are off for ${tool} on that machine`
        case 'update': return `Update reet to read ${tool}'s models on that machine`
        case 'offline': return `${tool}'s models were not read; the machine was offline`
      }
      return `Asked ${tool} for its models`
    }
    case 'model-favourite-added':
      return d.model ? `Starred ${d.model} for ${toolName(d.tool)}` : 'Starred a model'
    case 'model-favourite-removed':
      return d.model ? `Unstarred ${d.model} for ${toolName(d.tool)}` : 'Unstarred a model'
    case 'model-favourite-default':
      return d.model ? `Made ${d.model} the default for ${toolName(d.tool)}` : `Cleared the default model for ${toolName(d.tool)}`
  }
  return `${e.action.replace(/-/g, ' ')} ${name}`.trim()
}

export function eventTitle(e: Event): string {
  return `${new Date(e.at).toLocaleString()} · ${e.source}`
}

/** The areas activity is grouped into, in the order the filters are offered. */
export type EventCategory = 'agents' | 'projects' | 'skills' | 'lists' | 'account' | 'admin'

export const EVENT_CATEGORIES: { id: EventCategory; label: string }[] = [
  { id: 'agents', label: 'Agents' },
  { id: 'projects', label: 'Projects' },
  { id: 'skills', label: 'Skills' },
  { id: 'lists', label: 'Lists' },
  { id: 'account', label: 'Account' },
  { id: 'admin', label: 'Admin' },
]

const CATEGORY_ACTIONS: Record<string, EventCategory> = {
  'terminal-launch': 'agents',
  'yolo-launch': 'agents',
  'yolo-allow': 'agents',
  'yolo-allowed': 'admin',
  'yolo-disallowed': 'admin',
  'machine-models-read': 'agents',
  'model-favourite-added': 'agents',
  'model-favourite-removed': 'agents',
  'model-favourite-default': 'agents',
  'worktree-remove': 'agents',
  'merge-machines': 'agents',
  'forget-machine': 'agents',
  'projects-migrated': 'projects',
  'folder-browse': 'projects',
  add: 'skills',
  like: 'skills',
  unlike: 'skills',
  stage: 'skills',
  notes: 'skills',
  tag: 'skills',
  remove: 'skills',
  ignore: 'skills',
  unignore: 'skills',
  link: 'skills',
  refresh: 'skills',
  'install-copy': 'skills',
  scan: 'skills',
  import: 'skills',
  restore: 'skills',
  export: 'skills',
  patch: 'skills',
  'tag-rename': 'skills',
  'tag-merge': 'skills',
  'sign-in': 'account',
  'sign-out': 'account',
  profile: 'account',
  'password-change': 'account',
  'password-reset-request': 'account',
  'password-reset': 'account',
  'session-end': 'account',
  'session-end-others': 'account',
  'token-create': 'account',
  'token-revoke': 'account',
  setup: 'account',
  join: 'account',
  invite: 'account',
  'publishing-username': 'account',
  settings: 'admin',
  'notification-prefs': 'admin',
  'notification-defaults': 'admin',
}

/** Which area one audit event belongs to; an unknown action is treated as account activity. */
export function eventCategory(e: Event): EventCategory {
  const known = CATEGORY_ACTIONS[e.action]
  if (known) return known
  if (e.action.startsWith('project-')) return 'projects'
  if (e.action.startsWith('sync-agent-')) return 'agents'
  if (e.action.startsWith('bulk-') || e.action.startsWith('hosted-') || e.action.startsWith('skill-content-')) return 'skills'
  if (e.action.startsWith('list-')) return 'lists'
  if (e.action.startsWith('backup-') || e.action.startsWith('mail-') || e.action.startsWith('group-') || e.action.startsWith('ticket-')) return 'admin'
  if (e.action.startsWith('user-') || e.action.startsWith('mcp-')) return 'account'
  return 'account'
}

/**
 * A failure that the Activity list can point somewhere: a mail check or test that failed is fixed in Admin → Email.
 * Returns null for everything else, so only events with a real remedy grow a link.
 */
export function eventRemedy(e: Event): { label: string; href: string } | null {
  if (e.action !== 'mail-check' && e.action !== 'mail-test') return null
  const d = detail(e)
  if (d.ok !== false) return null
  return { label: 'Check the mail server settings', href: '/admin/email' }
}
