import { writable } from 'svelte/store'

export type GafferSessionContext = { id: string; title: string; structured: boolean; model: string; tool: string }
// The active workspace publishes identity only. This is a UI scope selector,
// never permission to read the worker's conversation or send it instructions.
export const gafferSessionContext = writable<GafferSessionContext | null>(null)

// The companion owns its conversation; the worker exposes only a nearby entry point.
export const gafferChatEntry = writable<((opener: HTMLElement) => void) | null>(null)
export const gafferChatDock = writable<HTMLElement | null>(null)
export function dockGafferChat(node: HTMLElement) {
  gafferChatDock.set(node)
  return { destroy() { gafferChatDock.update(current => current === node ? null : current) } }
}
