import type { NotificationDefault } from './api'

export type NotificationDefaultGroup = { name: string; items: NotificationDefault[] }

export function notificationDefaultGroups(defaults: NotificationDefault[]): NotificationDefaultGroup[] {
  const groups: NotificationDefaultGroup[] = []
  for (const item of defaults) {
    let group = groups.find((candidate) => candidate.name === item.group)
    if (!group) {
      group = { name: item.group, items: [] }
      groups.push(group)
    }
    group.items.push(item)
  }
  return groups
}

export function notificationDefaultPayload(defaults: NotificationDefault[]) {
  return defaults.filter((item) => !item.locked).map((item) => ({
    kind: item.key,
    enabled: item.default.enabled,
    mode: item.default.mode,
    owner: item.default.owner,
    admin: item.default.admin,
  }))
}

export function notificationDefaultCountLabel(count: number, loading = false): string {
  if (loading) return 'Reading the defaults…'
  if (count === 0) return 'No notification kinds'
  return `${count} notification ${count === 1 ? 'kind' : 'kinds'}`
}
