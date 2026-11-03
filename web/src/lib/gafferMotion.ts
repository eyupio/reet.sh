import { writable } from 'svelte/store'

// Account preference only: pausing the portrait must never pause progress or banter.
export const gafferAnimations = writable(true)
