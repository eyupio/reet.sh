import type { GafferPhase } from './gafferProgress'

export type GafferPose = 'duty' | 'tea' | 'asleep' | 'attention' | 'concerned'
export type GafferReaction = '' | 'ponder' | 'scan' | 'scribble' | 'watch' | 'nod' | 'tip-cap' | 'sip' | 'startle' | 'stretch' | 'shake' | 'listen' | 'biscuit' | 'shrug' | 'huff'
export const GAFFER_AWAY_AFTER = 15_000
export const GAFFER_CLICK_GAP = 650
export const GAFFER_CLICK_WINDOW = 20_000

export type GafferClicks = { count: number; at: number }
/** A deliberate second nudge counts; pointer double-clicks and held keys do not flood the portrait. */
export function gafferClickRun(previous: GafferClicks, now: number): GafferClicks | null {
  const gap = now - previous.at
  if (gap >= 0 && gap < GAFFER_CLICK_GAP) return null
  return { count: gap >= 0 && gap <= GAFFER_CLICK_WINDOW ? Math.min(previous.count + 1, 999) : 1, at: now }
}

/** A nudge is answered with a small, physical acknowledgement of whatever he is already doing: he flinches, gives a double nod,
 *  or glances aside. It never swaps the scene, so a half-drunk cup of tea stays a cup of tea. Later nudges earn a different one. */
export type GafferPoke = 'flinch' | 'bob' | 'sway'
const POKES: readonly GafferPoke[] = ['flinch', 'bob', 'sway']
export function gafferPoke(count: number): GafferPoke {
  return POKES[(Math.max(1, Math.floor(count)) - 1) % POKES.length]
}

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
  duty: ['That’s a flat cap, not a doorbell. {agent}, make a note.', 'You rang? {agent} does the clever bit. I provide hat-based leadership.', 'Careful. This eyebrow is a precision instrument. {agent} gets the keyboard.', 'Ey up, boss. {agent} has the keyboard. I’ve got a biscuit with structural concerns.', 'Now then. {agent} minds the code. I mind whether this counts as a meeting.', 'Aye? {agent} is on the job. I’m the decorative management layer.', 'Reet. {agent} has the brief. I’ve brought a face that suggests I’ve read it.', 'Ey up. {agent} gets the clever questions. I get “is the kettle on?”'],
  tea: ['Brew’s mine. {agent} has the update. Form an orderly queue.', 'No dunking your cursor in my tea. {agent}, tell ’em.', 'Steady. {agent} has the job sheet. This mug contains the management strategy.', 'Aye, it’s a working brew. {agent} works. I brew. Teamwork.', 'That biscuit’s load-bearing. {agent}, nobody touch the biscuit.', 'I’m checking the dunking depth. {agent} gets the important measurements.'],
  asleep: ['I was checking for bugs on the inside of my eyelids. {agent} has the handover.', 'Five more min— ah. Yes. Executive presence. {agent}, carry on.', 'Wasn’t snoring. That was a progress indicator. {agent} has the actual handover.', 'Ey up! {agent} has the job sheet. I was merely giving my eyebrows a lie-down.', 'A strategic pause, boss. {agent} wrote the handover; I warmed the chair.', 'Dreamt I’d approved a meeting. Woke up sharpish. {agent} has the details.'],
  attention: ['I’d tip my cap, but {agent} still needs your answer.', 'Your call, boss. Poking me won’t answer {agent}’s question.', 'Aye, I’ve an opinion. {agent} needs your decision. Different departments.', 'My signing authority covers the biscuit tin. {agent} needs you for this one.', 'Have a look at {agent}’s question. My helpful eyebrow has reached its limit.'],
  concerned: ['I’m on snag duty. {agent}’s details are above. My spanner is mostly moral support.', 'Easy with the poking. {agent} needs a look, and I’m holding the emergency biscuit.', 'Reet. {agent} has a snag. We’ll read it before deploying the stern tut.', 'I’ve put the kettle on standby. {agent}’s error gets the attention first.', 'No blame, boss. {agent} has the details. We’ll untangle it, one bit at a time.'],
}

const cheekyQuotes = [
  'Twice? Reet, that’s a meeting. {agent}, take minutes. Very small ones.',
  'Again? {agent}, add “professional poking” to the timesheet.',
  'You can nudge the foreman, but you can’t hurry a custard cream. {agent} has the work update.',
  'That’s enough supervision to qualify as a committee. {agent}, hide the flip chart.',
  'Aye, I’m still here. {agent} does the coding; I maintain the eyebrow service.',
  'Steady on. This cap’s held on by experience and a rather firm opinion. {agent}, carry on.',
]
const persistentQuotes = [
  'Keep that up and I’m invoicing in custard creams. {agent}, itemise the crumbs.',
  'I’ve had less supervision from a pigeon with a chip. {agent}, don’t encourage ’em.',
  'Reet. You’re deputy assistant eyebrow inspector. {agent} still gets the keyboard.',
  'Tha’s making a career of this. {agent}, put the kettle on the organisation chart.',
  'We’re one poke away from a subcommittee. {agent}, bolt the meeting-room door.',
  'I came to supervise {agent}, and somehow I’m the interactive exhibit. Typical.',
  'My union says every fifth poke requires a biscuit. {agent}, check the small print.',
  'The cap is not a refresh button. {agent} has the update; I have crumbs in my pocket.',
]

export function gafferClickAside(pose: GafferPose, agent: string, count: number, previous = '', random = Math.random): string {
  if (pose !== 'duty' || count < 2) return gafferAside('click', pose, agent, previous, random)
  return chooseAside(count < 4 ? cheekyQuotes : persistentQuotes, agent, previous, random)
}

function chooseAside(quotes: readonly string[], agent: string, previous: string, random: () => number): string {
  const all = quotes.map(text => text.replaceAll('{agent}', agent.trim() || 'Agent'))
  const options = all.filter(text => text !== previous)
  const pool = options.length ? options : all
  return pool[Math.min(pool.length - 1, Math.max(0, Math.floor(random() * pool.length)))]
}

export function gafferAside(kind: 'return' | 'click', pose: GafferPose, agent: string, previous = '', random = Math.random): string {
  return chooseAside((kind === 'return' ? returnQuotes : clickQuotes)[pose], agent, previous, random)
}
