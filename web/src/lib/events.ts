import type { Event } from './api'
import { launchActivity } from './agentLaunch'

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
}

function detail(e: Event): Detail {
  if (!e.detail) return {}
  try {
    return JSON.parse(e.detail) as Detail
  } catch {
    return {}
  }
}

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
    case 'profile':
      return 'Updated your profile'
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
  }
  return `${e.action.replace(/-/g, ' ')} ${name}`.trim()
}

export function eventTitle(e: Event): string {
  return `${new Date(e.at).toLocaleString()} · ${e.source}`
}
