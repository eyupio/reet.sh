import { writable } from 'svelte/store'
import type { SessionInsight } from '../sessionInsights'
// Last observations for inspected sessions in this visit; never persist paths or PR content.
export const sessionInsights = writable<Record<string, { status?: SessionInsight; github?: SessionInsight }>>({})
export function rememberInsight(id: string, view: 'status' | 'github', value: SessionInsight) {
  sessionInsights.update(all => Object.fromEntries(Object.entries({ ...all, [id]: { ...all[id], [view]: value } }).slice(-64)))
}
