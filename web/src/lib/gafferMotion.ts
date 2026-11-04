import { writable } from 'svelte/store'
import type { GafferReaction } from './gafferPersonality'

// Account preference only: pausing the portrait must never pause progress or banter.
export const gafferAnimations = writable(true)

// Account preference: how each Gaffer card starts. A card's own toggle overrides it for that card only.
export const gafferCompact = writable(false)

// Shared by CSS playback and the welcome lifecycle so the scene outlives its four frames.
export const GAFFER_GESTURE_MS = 4_200
export const GAFFER_SETTLE_MS = 300
export const GAFFER_TRANSITION_MS = 220

/** A little anticipation, the gesture, then a pause to settle. Also controls the production lifecycle. */
export function gafferGestureMs(reaction: GafferReaction): number {
  switch (reaction) {
    case 'tip-cap': return 3_800
    case 'scan': return 4_600
    case 'sip': return 5_200
    case 'biscuit': return 5_600
    case 'huff': return 4_400
    case 'nod': case 'shake': return 2_600
    case 'listen': case 'scribble': case 'watch': return 2_800
    case 'startle': return 2_200
    case 'stretch': return 3_200
    default: return GAFFER_GESTURE_MS
  }
}
