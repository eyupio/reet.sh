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
export function gafferProgress(chat: ChatState, dir = '', replayed = true, ended = false, agent = 'Agent', sessionFailed = false): GafferProgress {
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
  if (sessionFailed || (ended && chat.status.state === 'error')) return result('error', 'This session ended with a problem', 'Review the session error and handover before starting again.')
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
  starting: ['Reet, {agent}. Kettle on. Big ideas. Unreasonably confident eyebrow.', 'Ey up, {agent}. Hard hats on. Metaphorical ones; mind the ceiling.', 'Right, {agent}. You bring the brains. I’ve brought a very official nod.', 'Boots on, {agent}. My motivational biscuit isn’t going to eat itself.', "Reet, {agent}. Fresh brief, clean boots, suspiciously empty biscuit tin.", "Clocking in, {agent}. My cap has passed its annual leadership inspection.", "Ready when you are, {agent}. I’ve sharpened my pencil and my opinions."],
  planning: ['Measure twice, {agent}. Interpretive dance once. Then a plan.', 'Have a think, {agent}. I’ll do my thoughtful eyebrow.', 'Steady, {agent}. A cunning plan beats a panicked spreadsheet.', 'Work out the next step, {agent}. My serious face doesn’t do crystal balls.', "Think it through, {agent}. “We’ll cross that bridge” is how I ended up in a beck.", "A proper plan, {agent}. Back of a biscuit packet is acceptable. Biscuit first, obviously.", "Take your time, {agent}. I once rushed a brew. Still in therapy.", "Before we start, {agent}: do we know where we’re going, or are we following my eyebrow?"],
  reading: ['Read the small print, {agent}. That’s where the plot twists live rent-free.', 'Have a proper look, {agent}. I’ve put my serious face on. Same face, different eyebrow.', 'Mind the footnotes, {agent}. Tiny writing, massive opinions.', 'Get to know the place, {agent}. No rearranging the furniture before we’ve found the door.', "Read on, {agent}. I’ve got my glasses on. They’re mostly for authority.", "Get the context, {agent}. Last time I skipped a page, I assembled a wardrobe with a sunroof.", "Quiet in the reading room, {agent}. Even my biscuit is on silent.", "Have a look, {agent}. The comments might explain it. Or start an argument."],
  searching: ['Keep looking, {agent}. If it’s behind the sofa, I’m retiring.', 'Follow the clues, {agent}. I’ll interrogate this biscuit.', 'Find the right bit, {agent}. My “it’s probably over there” face isn’t evidence.', 'Any sign, {agent}? I’ve narrowed it down to somewhere in the project.', "Hunt it down, {agent}. I found a tenner in this cap once. Best sprint we ever had.", "Try the obvious place, {agent}. Then the place we said it definitely wasn’t.", "Keep looking, {agent}. My filing system is “somewhere safe”, which has never helped.", "Follow the trail, {agent}. If you find my reading glasses, they’re probably on my head."],
  editing: ['Make it tidy, {agent}. Future us is already a bit cross with present us.', 'Careful with the edges, {agent}. This is a codebase, not a pie crust.', 'Small changes, {agent}. Save the big flourishes for my eyebrows.', 'Nice and steady, {agent}. I’m a gaffer, not a keyboard stunt double.', "Neat stitches, {agent}. Nobody wants a patch that looks like my allotment fence.", "Keep it focused, {agent}. This cap’s the only thing here allowed a massive brim.", "Easy does it, {agent}. You’re changing code, not negotiating with a jammed shed door.", "Tidy work, {agent}. Leave the place as you’d like to find it. With more biscuits, ideally."],
  running: ['Let it run, {agent}. I’ll supervise the supervising. It’s a growth industry.', 'Watch the output, {agent}. My lucky eyebrow is not a test strategy.', 'Give it room, {agent}. I tried shouting “faster” at the kettle. Formal complaint pending.', 'Eyes on the command, {agent}. I’ll handle the competitive eyebrow exercises.', "Off it goes, {agent}. I’m watching with the intensity of a man waiting for toast.", "Let the command speak, {agent}. I’ve tried giving the terminal a pep talk. It asked for stdin.", "Watch it carefully, {agent}. I’ve got a stopwatch and absolutely no athletic ambitions.", "Steady on, {agent}. A spinning thing isn’t a result. Learned that from the washing machine."],
  fetching: ['Bring back what we need, {agent}. No souvenir mugs. We’ve discussed this.', 'Check the source, {agent}. “A bloke down the pub” isn’t a dependency.', 'Fetch carefully, {agent}. The internet’s a big shed with some odd shelves.', "Mind the provenance, {agent}. I once ordered a spanner and got a motivational poster.", "Bring back the right thing, {agent}. Last fetch I did was a brew. Personal best.", "Out to the supplies shed, {agent}. Check the label before you carry owt home."],
  working: ['Crack on, {agent}. I’m nodding with managerial intent.', 'One step at a time, {agent}. I run a strict no-chaos policy. In theory.', 'Keep at it, {agent}. I’ve put “supervise” on my list so I can tick something off.', "Crack on, {agent}. I’ve delegated everything except looking busy. Specialist skill, that.", "Keep it steady, {agent}. My management style is firm but crumbly. Like a good shortbread.", "One thing at a time, {agent}. I tried multitasking once. Tea in the pencil pot."],
  responding: ['Put it plainly, {agent}. The executive summary doesn’t need a fog machine.', 'Tell ’em what you found, {agent}. I’ll put the tiny trumpet away.', 'Clear handover, {agent}. Fewer mystery novels, more useful details.', "Plain English, {agent}. If it needs a glossary, I’m charging by the eyebrow.", "Write it up, {agent}. I’ll resist adding “as per my last biscuit”.", "Useful details, {agent}. My executive summary of most meetings is “could’ve been a brew”."],
  approval: ['Hold there, {agent}. This one’s the customer’s call.', 'Tools down a moment, {agent}. We’ll wait for the go-ahead.', 'Ask first, {agent}. Good manners cost nowt.', "Your call, boss. {agent} is waiting. My cap doesn’t come with signing authority.", "Hold fire, {agent}. An enthusiastic nod from me isn’t permission.", "Stand easy, {agent}. I’ve put the big rubber stamp away till they decide."],
  question: ['Ask away, {agent}. Better a question than a right old muddle.', 'Hold that thought, {agent}. Let’s hear what they reckon.', 'Give ’em a moment, {agent}. We don’t rush a good answer.', "Over to you. {agent} needs an answer; my only suggestion was “more tea”.", "Take a moment, {agent}. I’ll put my listening cap on. Same cap, different department.", "No guessing, {agent}. That’s how I once painted the wrong shed."],
  waiting: ['Stand by, {agent}. We’ll wait for word before calling it progress.', 'Steady, {agent}. A quiet spell isn’t a finished job.', 'Let’s wait for the next update, {agent}. No tall tales on my watch.', "Waiting on word, {agent}. My watch says “patience”. Cheap watch, very philosophical.", "Keep the spot, {agent}. I’ll practise looking available without starting a meeting.", "Quiet spell, {agent}. Still on duty. The mug is part of the uniform."],
  connecting: ['Hold on, {agent}. Let’s get the line sorted.', "Finding the line, {agent}. I’m resisting the urge to tap the screen with a spanner.", "One moment, {agent}. Getting connected. The tin cans were a prototype."],
  reconnecting: ['Hold tight, {agent}. Let’s get the line back before we guess owt.', 'Lost the line, {agent}. I’ll save the patter till we’re back.', "Line’s gone quiet, {agent}. I’ve checked the string between the tin cans.", "Stay put, {agent}. No updates coming through yet. My eyebrow isn’t a Wi-Fi antenna.", "Connection first, {agent}. Then we can stop glaring professionally at the router."],
  error: ['Easy, {agent}. Let’s understand the snag before another go.', 'No blame, {agent}. Show us the snag and we’ll take it from there.', "Snag on the job, {agent}. Spanner out. Guesswork away.", "Right, {agent}. Read the error. My “give it a whack” policy is under review.", "Something needs a look, {agent}. I’ve put my serious cap on. Yes, the same one.", "Easy does it, {agent}. No victory biscuits till we understand what happened."],
  cancelled: ['Tools down, {agent}. We’ll carry on when they’re ready.', 'Reet, {agent}. Park it there for now.', "Parked for now, {agent}. Even my eyebrow’s clocked off.", "Tools down, {agent}. Nobody gets a medal for ignoring the stop sign.", "Taking a breather, {agent}. The next move belongs to them."],
  ended: ['That’s the shift, {agent}. Job sheet below. I’ve even used my neat handwriting.', "Shift’s over, {agent}. Handing in my timesheet. “Supervision” appears quite a lot.", "Session closed, {agent}. Job sheet below. I’ll lock up the biscuit tin.", "That’s us parked, {agent}. Read the handover before my cap becomes a pillow.", "Clocked off, {agent}. Any snags are on the job sheet. I’m not sweeping owt under the rug."],
  idle: ['Reet, {agent}. Handover time. I’ll put my executive face on. It’s the same face.', 'Cheers, {agent}. Job sheet below; biscuit negotiations can wait.', 'Over to them, {agent}. A little less eyebrow, a little more handover.', "Over to you, {agent}. I’ll be here, looking like I understand the budget.", "Handover time, {agent}. My contribution was mostly hat, but it was a very good hat.", "Read the job sheet, {agent}. The eyebrow adds atmosphere; the details do the talking.", "Ready for the next brief, {agent}. I’ve cleared my diary. It was just “brew” twelve times."],
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
