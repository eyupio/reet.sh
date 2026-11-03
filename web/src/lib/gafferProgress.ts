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
    visible: active || ['approval', 'question', 'waiting', 'reconnecting', 'error', 'cancelled'].includes(phase) || start >= 0
      || (chat.items.length > 0 && (phase === 'ended' || (phase === 'idle' && chat.truncated))),
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
  starting: ['Reet, {agent}. Kettle on. Big ideas. Unreasonably confident eyebrow.', 'Ey up, {agent}. Hard hats on. Metaphorical ones; mind the ceiling.', 'Right, {agent}. You bring the brains. I’ve brought a very official nod.', 'Boots on, {agent}. My motivational biscuit isn’t going to eat itself.'],
  planning: ['Measure twice, {agent}. Interpretive dance once. Then a plan.', 'Have a think, {agent}. I’ll do my thoughtful eyebrow.', 'Steady, {agent}. A cunning plan beats a panicked spreadsheet.', 'Work out the next step, {agent}. My serious face doesn’t do crystal balls.'],
  reading: ['Read the small print, {agent}. That’s where the plot twists live rent-free.', 'Have a proper look, {agent}. I’ve put my serious face on. Same face, different eyebrow.', 'Mind the footnotes, {agent}. Tiny writing, massive opinions.', 'Get to know the place, {agent}. No rearranging the furniture before we’ve found the door.'],
  searching: ['Keep looking, {agent}. If it’s behind the sofa, I’m retiring.', 'Follow the clues, {agent}. I’ll interrogate this biscuit.', 'Find the right bit, {agent}. My “it’s probably over there” face isn’t evidence.', 'Any sign, {agent}? I’ve narrowed it down to somewhere in the project.'],
  editing: ['Make it tidy, {agent}. Future us is already a bit cross with present us.', 'Careful with the edges, {agent}. This is a codebase, not a pie crust.', 'Small changes, {agent}. Save the big flourishes for my eyebrows.', 'Nice and steady, {agent}. I’m a gaffer, not a keyboard stunt double.'],
  running: ['Let it run, {agent}. I’ll supervise the supervising. It’s a growth industry.', 'Watch the output, {agent}. My lucky eyebrow is not a test strategy.', 'Give it room, {agent}. I tried shouting “faster” at the kettle. Formal complaint pending.', 'Eyes on the command, {agent}. I’ll handle the competitive eyebrow exercises.'],
  fetching: ['Bring back what we need, {agent}. No souvenir mugs. We’ve discussed this.', 'Check the source, {agent}. “A bloke down the pub” isn’t a dependency.', 'Fetch carefully, {agent}. The internet’s a big shed with some odd shelves.'],
  working: ['Crack on, {agent}. I’m nodding with managerial intent.', 'One step at a time, {agent}. I run a strict no-chaos policy. In theory.', 'Keep at it, {agent}. I’ve put “supervise” on my list so I can tick something off.'],
  responding: ['Put it plainly, {agent}. The executive summary doesn’t need a fog machine.', 'Tell ’em what you found, {agent}. I’ll put the tiny trumpet away.', 'Clear handover, {agent}. Fewer mystery novels, more useful details.'],
  approval: ['Hold there, {agent}. This one’s the customer’s call.', 'Tools down a moment, {agent}. We’ll wait for the go-ahead.', 'Ask first, {agent}. Good manners cost nowt.'],
  question: ['Ask away, {agent}. Better a question than a right old muddle.', 'Hold that thought, {agent}. Let’s hear what they reckon.', 'Give ’em a moment, {agent}. We don’t rush a good answer.'],
  waiting: ['Stand by, {agent}. We’ll wait for word before calling it progress.', 'Steady, {agent}. A quiet spell isn’t a finished job.', 'Let’s wait for the next update, {agent}. No tall tales on my watch.'],
  connecting: ['Hold on, {agent}. Let’s get the line sorted.'],
  reconnecting: ['Hold tight, {agent}. Let’s get the line back before we guess owt.', 'Lost the line, {agent}. I’ll save the patter till we’re back.'],
  error: ['Easy, {agent}. Let’s understand the snag before another go.', 'No blame, {agent}. Show us the snag and we’ll take it from there.'],
  cancelled: ['Tools down, {agent}. We’ll carry on when they’re ready.', 'Reet, {agent}. Park it there for now.'],
  ended: ['That’s the shift, {agent}. Job sheet below. I’ve even used my neat handwriting.'],
  idle: ['Reet, {agent}. Handover time. I’ll put my executive face on. It’s the same face.', 'Cheers, {agent}. Job sheet below; biscuit negotiations can wait.', 'Over to them, {agent}. A little less eyebrow, a little more handover.'],
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
