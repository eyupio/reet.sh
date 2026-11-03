import { writable } from 'svelte/store'

// Account preference only: pausing the portrait must never pause progress or banter.
export const gafferAnimations = writable(true)

// Shared by CSS playback and the welcome lifecycle so the scene outlives its four frames.
export const GAFFER_GESTURE_MS = 1_800
