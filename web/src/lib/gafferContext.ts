import { writable } from 'svelte/store'

export type GafferSessionContext = { id: string; title: string; structured: boolean; model: string; tool: string }
// The active workspace publishes identity only. This is a UI scope selector,
// never permission to read the worker's conversation or send it instructions.
export const gafferSessionContext = writable<GafferSessionContext | null>(null)
