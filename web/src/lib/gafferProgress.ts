import { isWorking, pendingPermissions, pendingQuestions, toolState, toolTarget, type ChatItem, type ChatState } from './chat'

type Tool = Extract<ChatItem, { kind: 'tool' }>
export type GafferPhase = 'starting' | 'planning' | 'reading' | 'searching' | 'editing' | 'running' | 'fetching' | 'working' | 'responding' | 'approval' | 'question' | 'waiting' | 'connecting' | 'reconnecting' | 'error' | 'cancelled' | 'ended' | 'idle'
export type GafferProgress = {
  phase: GafferPhase; label: string; target: string; visible: boolean; active: boolean
  turnKey: string; completed: number; running: number; failed: number; recent: Tool[]; partial: boolean
}

const phases: Record<string, [GafferPhase, string]> = {
  read: ['reading', 'Worker reading the files'], search: ['searching', 'Worker searching the project'],
  edit: ['editing', 'Worker making changes'], delete: ['editing', 'Worker removing a file'], move: ['editing', 'Worker moving a file'],
  execute: ['running', 'Worker running a command'], fetch: ['fetching', 'Worker fetching a resource'],
}

/** Only report evidence in the current turn. Replayed history, elapsed time and banter never imply new work or success. */
export function gafferProgress(chat: ChatState, dir = '', replayed = true, ended = false): GafferProgress {
  let start = -1
  for (let i = chat.items.length - 1; i >= 0; i--) { if (chat.items[i].kind === 'user') { start = i; break } }
  const items = chat.items.slice(start + 1)
  const tools = items.filter((item): item is Tool => item.kind === 'tool' && item.toolKind !== 'think')
  const activeTools = tools.filter(item => item.status === 'in_progress' || item.status === 'pending')
  const base = {
    turnKey: start >= 0 ? chat.items[start].key : 'unavailable',
    completed: tools.filter(item => item.status === 'completed').length,
    running: activeTools.length, failed: tools.filter(item => item.status === 'failed').length,
    recent: tools.slice(-5).reverse(), partial: chat.truncated,
  }
  const result = (phase: GafferPhase, label: string, target = '', active = false): GafferProgress => ({
    ...base, phase, label, target, active,
    visible: active || ['approval', 'question', 'waiting', 'reconnecting', 'error', 'cancelled'].includes(phase) || start >= 0,
  })
  if (ended || chat.status.state === 'ended') return result('ended', 'This session has ended')
  if (chat.connection === 'reconnecting') return result('reconnecting', 'Reconnecting to the worker', 'Live progress is unavailable until the connection returns.')
  if (chat.connection === 'connecting' || !replayed) return { ...result('connecting', 'Catching up with the worker'), visible: false }
  if (chat.status.state === 'error') return result('error', 'Worker needs a hand', 'See the error above before sending another message.')
  if (chat.status.state === 'cancelled') return result('cancelled', 'Worker stopped', chat.status.detail || 'Send a message when you’re ready to carry on.')
  const approval = pendingPermissions(chat)[0]
  if (approval) return result('approval', 'Waiting for your approval', approval.command || approval.path || approval.title)
  const question = pendingQuestions(chat)[0]
  if (question) return result('question', 'Waiting for your answer', question.question.message)
  if (!isWorking(chat)) return result('idle', start >= 0 ? 'Turn finished' : 'Ready for a job', 'Ready for your next instruction.')
  if (chat.status.state === 'starting') return result('starting', 'Getting the worker ready', '', true)
  if (chat.status.state === 'waiting') return result('waiting', 'Waiting for the worker', chat.status.detail || 'No new activity has been reported.')
  const current = activeTools.at(-1)
  if (current) {
    const [phase, label] = phases[current.toolKind] ?? ['working', 'Worker using a tool']
    return result(phase, current.status === 'pending' ? 'Worker preparing a tool' : label, toolTarget(current, dir), true)
  }
  if (items.at(-1)?.kind === 'agent') return result('responding', 'Worker writing a reply', '', true)
  return result('planning', 'Worker working out the next step', '', true)
}

// Gaffer directs and encourages; the worker does the work. These are personality, never claims about tools or results.
const quotes: Record<GafferPhase, readonly string[]> = {
  starting: ['Reet, worker. Kettle’s on; let’s get this job lined up.', 'Ey up, worker. One good job at a time.', 'Right then, worker. You bring the graft; I’ll mind the brief.'],
  planning: ['Measure twice, worker. We’ve only got the one project.', 'Have a proper think, worker. Clever beats hasty.', 'Steady on, worker. Even a good plan needs its boots on.', 'Work out the next step, worker. I’ll keep the big picture in view.'],
  reading: ['Read the small print, worker. That’s where the plot twists hide.', 'Get to know the place, worker. We’re guests in this code.', 'Have a proper look, worker. No points for guessing.'],
  searching: ['Keep looking, worker. It’ll be in the last place we look. Usually is.', 'Follow the clues, worker. Leave the wild goose to me.', 'Find the right bit first, worker. Saves a fair bit of bother.'],
  editing: ['Nice and steady, worker. Good work’s worth the care.', 'Mind the edges, worker. That’s where the quality lives.', 'Make it tidy, worker. Future us has enough on.', 'Small, thoughtful changes, worker. I’m a gaffer, not a gambler.'],
  running: ['Let the command do its bit, worker. I’ll supervise the supervising.', 'Watch the output, worker. It knows more than my hunch does.', 'Give it room, worker. No amount of staring makes a kettle quicker.'],
  fetching: ['Bring back what we need, worker. No souvenirs.', 'Check the source, worker. Pub talk’s no specification.', 'Fetch carefully, worker. Quality in, quality out.'],
  working: ['Keep at it, worker. I’ll mind the job sheet.', 'One step at a time, worker. We’ll get there together.', 'Crack on, worker. Give us a nudge if you need a hand.'],
  responding: ['Put it plainly, worker. Good work deserves a clear handover.', 'Tell ’em what you found, worker. Leave the trumpet at home.', 'Make it useful, worker. Plain words, proper detail.'],
  approval: ['Hold there, worker. This one’s the customer’s call.', 'Tools down a moment, worker. We’ll wait for the go-ahead.', 'Ask first, worker. Good manners cost nowt.'],
  question: ['Ask away, worker. Better a question than a right old muddle.', 'Hold that thought, worker. Let’s hear what they reckon.', 'Give ’em a moment, worker. We don’t rush a good answer.'],
  waiting: ['Stand by, worker. We’ll wait for word before calling it progress.', 'Steady, worker. A quiet spell isn’t a finished job.', 'Let’s wait for the next update, worker. No tall tales on my watch.'],
  connecting: ['Hold on, worker. Let’s get the line sorted.'],
  reconnecting: ['Hold tight, worker. Let’s get the line back before we guess owt.', 'Lost the line, worker. I’ll save the patter till we’re back.'],
  error: ['Easy, worker. Let’s understand the snag before another go.', 'No blame, worker. Show us the snag and we’ll take it from there.'],
  cancelled: ['Tools down, worker. We’ll carry on when they’re ready.', 'Reet, worker. Park it there for now.'],
  ended: ['That’s this shift over, worker. Cheers for the graft.'],
  idle: ['Cheers for the graft, worker. Next move’s theirs.', 'Reet, worker. Let’s give them room to review.', 'Your turn to put your feet up, worker. Briefly, mind.'],
}

export function gafferQuote(phase: GafferPhase, previous = '', random = Math.random): string {
  const options = quotes[phase].filter(quote => quote !== previous)
  const pool = options.length ? options : quotes[phase]
  return pool[Math.min(pool.length - 1, Math.max(0, Math.floor(random() * pool.length)))]
}

/** This is explicitly time observed in this view, since SSE replay has no authoritative turn-start timestamp. */
export function observedTime(seconds: number): string {
  const value = Math.max(0, Math.floor(seconds))
  return value < 60 ? `${value}s` : `${Math.floor(value / 60)}m ${String(value % 60).padStart(2, '0')}s`
}

export function recentToolState(tool: Tool): string { return toolState(tool.status).word }
