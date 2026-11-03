import type { GafferPhase } from './gafferProgress'

export type GafferPose = 'duty' | 'tea' | 'asleep' | 'attention' | 'concerned'
export type GafferReaction = '' | 'ponder' | 'scan' | 'scribble' | 'watch' | 'nod' | 'tip-cap' | 'wink' | 'sip' | 'startle' | 'stretch' | 'shake' | 'listen'
export const GAFFER_AWAY_AFTER = 15_000

/** Decoration follows confirmed status. A snag or a request for input always outranks a tea break. */
export function gafferPose(phase: GafferPhase, away = false): GafferPose {
  if (['error', 'reconnecting', 'connecting', 'cancelled'].includes(phase)) return 'concerned'
  if (phase === 'approval' || phase === 'question') return 'attention'
  if (away) return phase === 'ended' || phase === 'idle' ? 'asleep' : 'tea'
  return 'duty'
}

export function gafferGesture(phase: GafferPhase): GafferReaction {
  switch (phase) {
    case 'planning': return 'ponder'
    case 'reading': case 'searching': case 'fetching': return 'scan'
    case 'editing': return 'scribble'
    case 'running': case 'connecting': case 'reconnecting': return 'watch'
    case 'approval': case 'question': return 'listen'
    case 'error': case 'cancelled': return 'shake'
    case 'ended': case 'idle': return 'tip-cap'
    default: return 'nod'
  }
}

const returnQuotes: Record<GafferPose, readonly string[]> = {
  tea: ['Oh! You’re back. This is a strategic brew. {agent} is still on the job.', 'Me? Tea break? I’m testing mug ergonomics. {agent}, look busy.', 'Ey up! Kettle inspection complete. {agent} has the actual work update.'],
  asleep: ['Wasn’t asleep. Just reading the inside of my cap. {agent}’s job sheet is below.', 'Ey up! Power nap. Very senior management. {agent}’s handover is below.', 'Who, me? Snoring? That was the executive summary warming up. {agent}, back me up.'],
  attention: ['You’re back. {agent} needs your say-so. I’ve kept my helpful eyebrow ready.', 'Ey up. {agent} has a question for you. I tried answering with a biscuit. No joy.'],
  concerned: ['Ey up. There’s a snag with {agent}. Details first; kettle second.', 'You’re back. {agent} needs a look. I’ve put the emergency biscuit on standby.'],
  duty: ['Ey up. {agent} has the work update. I have the hat. Clear division of labour.'],
}
const clickQuotes: Record<GafferPose, readonly string[]> = {
  duty: ['That’s a flat cap, not a doorbell. {agent}, make a note.', 'You rang? {agent} does the clever bit. I provide hat-based leadership.', 'One poke: supervision. Two pokes: consultancy rates. {agent}, fetch the invoice.', 'Careful. This eyebrow is a precision instrument. {agent} gets the keyboard.'],
  tea: ['Brew’s mine. {agent} has the update. Form an orderly queue.', 'No dunking your cursor in my tea. {agent}, tell ’em.'],
  asleep: ['I was checking for bugs on the inside of my eyelids. {agent} has the handover.', 'Five more min— ah. Yes. Executive presence. {agent}, carry on.'],
  attention: ['I’d tip my cap, but {agent} still needs your answer.', 'Your call, boss. Poking me won’t answer {agent}’s question.'],
  concerned: ['I’m on snag duty. {agent}’s details are above. My spanner is mostly moral support.', 'Easy with the poking. {agent} needs a look, and I’m holding the emergency biscuit.'],
}

export function gafferAside(kind: 'return' | 'click', pose: GafferPose, agent: string, previous = '', random = Math.random): string {
  const all = (kind === 'return' ? returnQuotes : clickQuotes)[pose].map(text => text.replaceAll('{agent}', agent.trim() || 'Agent'))
  const options = all.filter(text => text !== previous)
  const pool = options.length ? options : all
  return pool[Math.min(pool.length - 1, Math.max(0, Math.floor(random() * pool.length)))]
}
