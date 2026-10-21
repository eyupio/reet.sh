import type { Event } from './api'

type Detail = {
  name?: string
  from?: string
  to?: string
  imported?: number
  created?: number
  ignored?: number
  pruned?: number
  restored?: number
  already?: boolean
  count?: number
  liked?: boolean
  role?: string
  email?: string
  disabled?: boolean
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
    case 'token-create':
      return 'Created an API token'
    case 'token-revoke':
      return 'Revoked an API token'
    case 'user-role':
      return `Changed ${d.name || 'a user'}'s role to ${d.role}`
    case 'user-disable':
      return `${d.name || 'A user'} ${d.disabled ? 'disabled' : 'enabled'}`
    case 'user-reset':
      return `Reset the password for ${d.name || 'a user'}`
    case 'user-revoke':
      return `Revoked ${d.name || 'a user'}'s API tokens`
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
  }
  return `${e.action.replace(/-/g, ' ')} ${name}`.trim()
}

export function eventTitle(e: Event): string {
  return `${new Date(e.at).toLocaleString()} · ${e.source}`
}
