import { writable } from 'svelte/store'
import type { GafferReaction } from './gafferPersonality'

// Account preference only: pausing the portrait must never pause progress or banter.
export const gafferAnimations = writable(true)

// Account preference: how each Gaffer card starts. A card's own toggle overrides it for that card only.
export const gafferCompact = writable(true)
/** The person's standing permission for Gaffer to look after a session they point it at, without a review each time. */
export const gafferStanding = writable(true)
/** Whether a new pipeline is allowed to merge by itself unless the person says otherwise for it. */
export const gafferAutoMerge = writable(false)

// Shared by CSS playback and the welcome lifecycle so the scene outlives its four frames.
export const GAFFER_GESTURE_MS = 2_600
export const GAFFER_SETTLE_MS = 300
export const GAFFER_TRANSITION_MS = 220

/** A little anticipation, the gesture, then a pause to settle. Also controls the production lifecycle. */
export function gafferGestureMs(reaction: GafferReaction): number {
  switch (reaction) {
    case 'tip-cap': return 2_400
    case 'scan': return 3_000
    case 'sip': return 3_400
    case 'biscuit': return 3_600
    case 'huff': return 2_800
    case 'scratch-head': case 'scratch-ear': return 3_200
    case 'scratch-chin': return 3_600
    case 'look-around': return 3_800
    case 'nose-twitch': return 2_600
    case 'rest-head': return 5_200
    case 'nod': case 'shake': return 1_500
    case 'listen': case 'scribble': case 'watch': return 1_800
    case 'startle': return 1_300
    case 'stretch': return 2_000
    default: return GAFFER_GESTURE_MS
  }
}
