import { isWorking, pendingPermissions, pendingQuestions, toolState, toolTarget, type ChatItem, type ChatState } from './chat'

type Tool = Extract<ChatItem, { kind: 'tool' }>
export type GafferPhase = 'starting' | 'planning' | 'reading' | 'searching' | 'editing' | 'running' | 'fetching' | 'working' | 'responding' | 'approval' | 'question' | 'waiting' | 'connecting' | 'reconnecting' | 'error' | 'cancelled' | 'ended' | 'idle'
export type GafferProgress = {
  phase: GafferPhase; label: string; target: string; visible: boolean; active: boolean
  turnKey: string; completed: number; running: number; failed: number; recent: Tool[]; partial: boolean
}

const phases: Record<string, [GafferPhase, string]> = {
  read: ['reading', '{agent} reading the files'], search: ['searching', '{agent} searching the project'],
  edit: ['editing', '{agent} making changes'], delete: ['editing', '{agent} removing a file'], move: ['editing', '{agent} moving a file'],
  execute: ['running', '{agent} running a command'], fetch: ['fetching', '{agent} fetching a resource'],
}

const named = (text: string, agent: string): string => text.replaceAll('{agent}', agent.trim() || 'Agent')

/** Only report evidence in the current turn. Replayed history, elapsed time and banter never imply new work or success. */
export function gafferProgress(chat: ChatState, dir = '', replayed = true, ended = false, agent = 'Agent'): GafferProgress {
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
  if (chat.connection === 'reconnecting') return result('reconnecting', named('Reconnecting to {agent}', agent), 'Live progress is unavailable until the connection returns.')
  if (chat.connection === 'connecting' || !replayed) return { ...result('connecting', named('Catching up with {agent}', agent)), visible: false }
  if (chat.status.state === 'error') return result('error', named('{agent} needs a hand', agent), 'See the error above before sending another message.')
  if (chat.status.state === 'cancelled') return result('cancelled', named('{agent} stopped', agent), chat.status.detail || 'Send a message when you’re ready to carry on.')
  const approval = pendingPermissions(chat)[0]
  if (approval) return result('approval', 'Waiting for your approval', approval.command || approval.path || approval.title)
  const question = pendingQuestions(chat)[0]
  if (question) return result('question', 'Waiting for your answer', question.question.message)
  if (!isWorking(chat)) return result('idle', start >= 0 ? 'Turn finished' : 'Ready for a job', 'Ready for your next instruction.')
  if (chat.status.state === 'starting') return result('starting', named('Getting {agent} ready', agent), '', true)
  if (chat.status.state === 'waiting') return result('waiting', named('Waiting for {agent}', agent), chat.status.detail || 'No new activity has been reported.')
  const current = activeTools.at(-1)
  if (current) {
    const [phase, label] = phases[current.toolKind] ?? ['working', '{agent} using a tool']
    return result(phase, named(current.status === 'pending' ? '{agent} preparing a tool' : label, agent), toolTarget(current, dir), true)
  }
  if (items.at(-1)?.kind === 'agent') return result('responding', named('{agent} writing a reply', agent), '', true)
  return result('planning', named('{agent} working out the next step', agent), '', true)
}

// Gaffer directs and encourages; the named agent does the work. These are personality, never claims about tools or results.
const quotes: Record<GafferPhase, readonly string[]> = {
  starting: ['Reet, {agent}. Kettle’s on; let’s get this job lined up.', 'Ey up, {agent}. One good job at a time.', 'Right then, {agent}. You bring the graft; I’ll mind the brief.'],
  planning: ['Measure twice, {agent}. We’ve only got the one project.', 'Have a proper think, {agent}. Clever beats hasty.', 'Steady on, {agent}. Even a good plan needs its boots on.', 'Work out the next step, {agent}. I’ll keep the big picture in view.'],
  reading: ['Read the small print, {agent}. That’s where the plot twists hide.', 'Get to know the place, {agent}. We’re guests in this code.', 'Have a proper look, {agent}. No points for guessing.'],
  searching: ['Keep looking, {agent}. It’ll be in the last place we look. Usually is.', 'Follow the clues, {agent}. Leave the wild goose to me.', 'Find the right bit first, {agent}. Saves a fair bit of bother.'],
  editing: ['Nice and steady, {agent}. Good work’s worth the care.', 'Mind the edges, {agent}. That’s where the quality lives.', 'Make it tidy, {agent}. Future us has enough on.', 'Small, thoughtful changes, {agent}. I’m a gaffer, not a gambler.'],
  running: ['Let the command do its bit, {agent}. I’ll supervise the supervising.', 'Watch the output, {agent}. It knows more than my hunch does.', 'Give it room, {agent}. No amount of staring makes a kettle quicker.'],
  fetching: ['Bring back what we need, {agent}. No souvenirs.', 'Check the source, {agent}. Pub talk’s no specification.', 'Fetch carefully, {agent}. Quality in, quality out.'],
  working: ['Keep at it, {agent}. I’ll mind the job sheet.', 'One step at a time, {agent}. We’ll get there together.', 'Crack on, {agent}. Give us a nudge if you need a hand.'],
  responding: ['Put it plainly, {agent}. Good work deserves a clear handover.', 'Tell ’em what you found, {agent}. Leave the trumpet at home.', 'Make it useful, {agent}. Plain words, proper detail.'],
  approval: ['Hold there, {agent}. This one’s the customer’s call.', 'Tools down a moment, {agent}. We’ll wait for the go-ahead.', 'Ask first, {agent}. Good manners cost nowt.'],
  question: ['Ask away, {agent}. Better a question than a right old muddle.', 'Hold that thought, {agent}. Let’s hear what they reckon.', 'Give ’em a moment, {agent}. We don’t rush a good answer.'],
  waiting: ['Stand by, {agent}. We’ll wait for word before calling it progress.', 'Steady, {agent}. A quiet spell isn’t a finished job.', 'Let’s wait for the next update, {agent}. No tall tales on my watch.'],
  connecting: ['Hold on, {agent}. Let’s get the line sorted.'],
  reconnecting: ['Hold tight, {agent}. Let’s get the line back before we guess owt.', 'Lost the line, {agent}. I’ll save the patter till we’re back.'],
  error: ['Easy, {agent}. Let’s understand the snag before another go.', 'No blame, {agent}. Show us the snag and we’ll take it from there.'],
  cancelled: ['Tools down, {agent}. We’ll carry on when they’re ready.', 'Reet, {agent}. Park it there for now.'],
  ended: ['That’s this shift over, {agent}. Cheers for the graft.'],
  idle: ['Cheers for the graft, {agent}. Next move’s theirs.', 'Reet, {agent}. Let’s give them room to review.', 'Your turn to put your feet up, {agent}. Briefly, mind.'],
}

export function gafferQuote(phase: GafferPhase, agent = 'Agent', previous = '', random = Math.random): string {
  const all = quotes[phase].map(quote => named(quote, agent))
  const options = all.filter(quote => quote !== previous)
  const pool = options.length ? options : all
  return pool[Math.min(pool.length - 1, Math.max(0, Math.floor(random() * pool.length)))]
}

/** This is explicitly time observed in this view, since SSE replay has no authoritative turn-start timestamp. */
export function observedTime(seconds: number): string {
  const value = Math.max(0, Math.floor(seconds))
  return value < 60 ? `${value}s` : `${Math.floor(value / 60)}m ${String(value % 60).padStart(2, '0')}s`
}

export function recentToolState(tool: Tool): string { return toolState(tool.status).word }
