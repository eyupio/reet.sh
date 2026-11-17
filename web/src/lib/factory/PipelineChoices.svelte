<script lang="ts" module>
  import type { GafferOutcome as Outcome, GafferOutcomeSession } from '../api'
  import type { NeedChoice } from '../gafferOutcome'

  /** Where a pipeline's session is opened. */
  export const sessionHref = (record: Outcome, id: string) => `/sessions?agent=${encodeURIComponent(record.agentId)}&session=${encodeURIComponent(id)}`
  /** Whether a pipeline merges by itself: a YOLO pipeline only once that was confirmed for YOLO. */
  export const mergesByItself = (record: Outcome, piece: GafferOutcomeSession) => !!piece.autoMerge && (!record.yolo || !!piece.yoloMergeOk)
  /** An objective can run to paragraphs; a title or a notice names it in a line. */
  export const short = (text: string) => { const line = text.replace(/\s+/g, ' ').trim(); return line.length > 80 ? `${line.slice(0, 79)}…` : line }
  /** The choices a pipeline can actually take right now: no moving on without a stage in hand, no telling an agent that
   *  is not there, no opening a session that was removed. */
  export const usable = (choice: NeedChoice, piece: GafferOutcomeSession) => {
    const alive = piece.sessionState !== 'ended' && piece.sessionState !== 'gone'
    if (choice.id === 'move') return piece.state === 'following' && !!piece.stages?.length && !!piece.conversationId
    if (choice.id === 'tell') return !!piece.sessionId && !!piece.conversationId && alive
    if (choice.id === 'open' || choice.id === 'answer') return !!piece.sessionId && piece.sessionState !== 'gone'
    return true
  }
</script>

<script lang="ts">
  // Carries out a pipeline's choice, and holds the dialogs those choices open: move on, tell the agent, approve merge,
  // stop following, and goal and limits. The Factory page mounts it once, and so does Home. It reads nothing by itself:
  // what changed is said through `onchanged`, and what happened through `onnotice`.
  import { approveGafferMerge, controlGafferGoal, getGafferGoal, moveGafferPipelineOn, retryGafferPipeline, stopFollowingGafferOutcome, tellGafferPipelineAgent, type GafferGoalOverview } from '../api'
  import { stageLabel } from '../gafferOutcome'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'
  import GafferGoal from '../GafferGoal.svelte'
  import PipelineNote from './PipelineNote.svelte'

  let { userId = 0, timeZone = '', dateFormat = 'locale', goals, busy = $bindable(false), latest = record => record, onchanged, onnotice }: {
    userId?: number
    timeZone?: string
    dateFormat?: string
    /** Gaffer's goals by conversation id: a pipeline's goal is found here. */
    goals: ReadonlyMap<string, GafferGoalOverview>
    /** True while an action is under way. Shared with the page, so one action waits for another as before. */
    busy?: boolean
    /** The newest read of an outcome, so an action is sent against its current revision. */
    latest?: (record: Outcome) => Outcome
    /** Something was done or a dialog closed: the page reads again. */
    onchanged: () => void | Promise<void>
    /** What an action did, or with `bad`, why it did not. An empty text clears the last one. */
    onnotice: (text: string, bad?: boolean) => void
    /** Accepted for the pages that mount this; nothing here signs out today, a refused action is said in a notice. */
    onsignedout?: (e: unknown) => void
  } = $props()

  type Pair = { record: Outcome; piece: GafferOutcomeSession }
  let moving = $state<Pair | null>(null), telling = $state<Pair | null>(null), merging = $state<Pair | null>(null)
  let managing = $state<GafferGoalOverview | null>(null), releasing = $state<Outcome | null>(null)
  // What Gaffer would say to the agent, as the words the Tell dialog starts from.
  let tellStart = $state('')
  // Why the Tell dialog's last attempt was refused, shown inside it rather than behind it.
  let dialogError = $state('')
  const fail = (e: unknown, fallback: string) => onnotice(e instanceof Error ? e.message : fallback, true)
  const requestId = () => Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('')
  // One request ID for each opening of the dialog: a second press after a lost answer reads the first's receipt instead of telling the agent twice.
  let tellRequest = ''
  $effect(() => { if (telling) tellRequest = requestId(); dialogError = '' })

  // One choice from an item or the detail. Each does what its button says and nothing else; "restart" is also a failed
  // pipeline's Try again, which has no goal.
  export function choose(choice: NeedChoice, record: Outcome, piece: GafferOutcomeSession) {
    const goal = piece.conversationId ? goals.get(piece.conversationId) : undefined
    if (choice.id === 'answer' || choice.id === 'open') { if (piece.sessionId) location.href = sessionHref(record, piece.sessionId) }
    else if (choice.id === 'retry' || choice.id === 'again') void reviewAgain(piece)
    else if (choice.id === 'tell') { tellStart = choice.recommended && goal?.goal.advice?.choice === 'tell' ? goal.goal.advice.text ?? '' : ''; telling = { record, piece } }
    else if (choice.id === 'merge') merging = { record, piece }
    else if (choice.id === 'move') moving = { record, piece }
    else if (choice.id === 'budget') { if (goal) managing = goal }
    else if (choice.id === 'restart') void retry(record, piece)
    else if (choice.id === 'stop') releasing = record
  }
  /** Asks to stop following an outcome, as its menu's Stop following does. */
  export function release(record: Outcome) { releasing = record }
  /** Opens a goal and its limits, as the detail's Goal and limits button does. */
  export function manage(goal: GafferGoalOverview) { managing = goal }
  /** While the goal or the merge dialog is open, the page holds its refresh. */
  export function holdsRefresh() { return !!managing || !!merging }

  // A review of Gaffer's own that did not complete is tried again by resuming the goal under its current permissions.
  async function reviewAgain(piece: GafferOutcomeSession) {
    if (busy || !piece.conversationId) return
    busy = true; onnotice('')
    try {
      const view = await getGafferGoal(piece.conversationId)
      if (!view.goal) throw new Error('That goal is no longer there.')
      if (!view.allowed) throw new Error(view.reason || 'Review this goal’s permissions before trying again.')
      await controlGafferGoal(piece.conversationId, view.goal.revision, 'resume', view.pins, true)
      onnotice(`Gaffer is reviewing “${piece.title}” again.`)
    } catch (e) { fail(e, 'The review was not tried again.') }
    busy = false
    await onchanged()
  }
  async function retry(record: Outcome, piece: GafferOutcomeSession) {
    if (busy) return
    busy = true; onnotice('')
    try { await retryGafferPipeline(latest(record), piece.key); onnotice(`Trying “${piece.title}” again.`) }
    catch (e) { fail(e, 'That pipeline was not tried again.') }
    busy = false
    await onchanged()
  }
  async function moveOn() {
    if (!moving || busy) return
    const { record, piece } = moving
    busy = true; onnotice(''); moving = null
    try { await moveGafferPipelineOn(record, piece.key, piece.stages?.[piece.stage ?? 0]?.kind ?? ''); onnotice(`Moved “${piece.title}” on. The controller is starting its next stage.`) }
    catch (e) { fail(e, 'That pipeline was not moved on.') }
    busy = false
    await onchanged()
  }
  async function tell(text: string) {
    if (!telling || busy) return
    const { record, piece } = telling
    busy = true; onnotice(''); dialogError = ''
    try {
      const sent = await tellGafferPipelineAgent(record, piece.key, tellRequest, text)
      onnotice(sent.message.state === 'sent' || sent.message.state === 'queued' || sent.message.state === 'sending' ? `Your instruction is with the agent of “${piece.title}” for its next turn.` : `The instruction for “${piece.title}” was not delivered: ${sent.message.note || sent.message.state}.`)
      telling = null
    } catch (e) { dialogError = e instanceof Error ? e.message : 'The instruction was not sent.' }
    busy = false
    await onchanged()
  }
  async function approve() {
    if (!merging || busy) return
    const { record, piece } = merging
    busy = true; onnotice('')
    try {
      await approveGafferMerge(latest(record), piece.key)
      onnotice(`Merge approved for “${piece.title}”. Its agent merges once every check passes, no review thread is open and there are no conflicts.`)
    } catch (e) { fail(e, 'The merge was not approved.') }
    busy = false; merging = null
    await onchanged()
  }
  // Ends an outcome that is not finished (a session was lost or removed, or the work is no longer wanted) so it can be archived or deleted.
  async function stopFollowing(record: Outcome) {
    if (busy) return
    busy = true; onnotice(''); releasing = null
    try { await stopFollowingGafferOutcome(latest(record)); onnotice(`Stopped following “${short(record.objective)}”. Sessions that still exist keep running. You can now archive or delete it.`) }
    catch (e) { fail(e, 'The outcome was not stopped.') }
    busy = false
    await onchanged()
  }
</script>

{#if releasing}<ConfirmDialog title={`Stop following “${short(releasing.objective)}”?`} body="Gaffer and the controller stop looking after every unfinished pipeline in this outcome and start nothing more for it. Sessions that still exist keep running, and nothing is merged or closed. Use this when a session was lost or removed, or the work is no longer wanted; you can then archive or delete the outcome." confirmLabel="Stop following" oncancel={() => releasing = null} onconfirm={() => releasing && void stopFollowing(releasing)}/>{/if}
{#if moving}{@const at = moving.piece.stages?.[moving.piece.stage ?? 0]?.kind ?? ''}{@const next = moving.piece.stages?.[(moving.piece.stage ?? 0) + 1]?.kind ?? ''}<ConfirmDialog title={next ? `Move “${moving.piece.title}” on to ${stageLabel(next).toLowerCase()}?` : `Finish the ${stageLabel(at).toLowerCase()} stage of “${moving.piece.title}”?`} body={`This is your call: the ${stageLabel(at).toLowerCase()} stage is closed as it stands, without waiting for Gaffer to find it done${next ? `, and the controller starts the ${stageLabel(next).toLowerCase()} stage with Gaffer’s last review as its handover` : ''}. ${next === 'merge' && !mergesByItself(moving.record, moving.piece) ? 'It then stops at Ready to merge and waits for you.' : next === 'merge' ? 'This pipeline merges by itself once every check passes.' : 'Nothing is merged by this.'}`} confirmLabel={next ? 'Move on' : 'Finish stage'} oncancel={() => moving = null} onconfirm={() => void moveOn()}/>{/if}
{#if telling}<PipelineNote title={`Tell the agent of “${telling.piece.title}”`} intro="Your words go to this pipeline’s working agent for its next turn, exactly as you write them. It cannot interrupt the agent or answer an approval." label="Instruction" placeholder="Write the plan as text now: files, approach, risks. Change no code." initial={tellStart} confirmLabel="Send to the agent" {busy} error={dialogError} oncancel={() => telling = null} onconfirm={text => void tell(text)}/>{/if}
{#if merging}<ConfirmDialog title={`Merge “${merging.piece.title}”?`} body="This tells the pipeline’s agent to merge its pull request once every check passes, no review thread is open and there are no conflicts. If any of those is not true it fixes that first or brings it back to you. A merge cannot be undone from here." confirmLabel="Approve merge" oncancel={() => merging = null} onconfirm={() => void approve()}/>{/if}
{#if managing}<WorkspaceDialog title="Look after this session" onclose={() => { managing = null; void onchanged() }}><GafferGoal id={managing.goal.conversationId} sessionId={managing.goal.sessionId} {userId} {timeZone} {dateFormat} {onchanged}/></WorkspaceDialog>{/if}
