<script lang="ts">
  // The decision a Factory pipeline is waiting on the person for, shown in the session that is doing the pipeline's
  // stage in hand. The choices are the ones on the Factory card, recommended first; the ones that need the Factory's own
  // dialogs (a merge, a new budget, stopping the outcome) go to the pipeline's card there.
  import { onMount } from 'svelte'
  import { controlGafferGoal, getGafferGoal, listGafferGoals, listGafferOutcomes, moveGafferPipelineOn, retryGafferPipeline, tellGafferPipelineAgent, type GafferGoalOverview, type GafferOutcome, type GafferOutcomeSession } from '../api'
  import { MAX_STAGE_DECISIONS, waitingDecisions, type FactoryMember, type NeedChoice } from '../gafferOutcome'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import Decision from './Decision.svelte'
  import PipelineNote from './PipelineNote.svelte'

  let { member, sessionId }: { member: FactoryMember; sessionId: string } = $props()
  let record = $state<GafferOutcome | null>(null), goals = $state(new Map<string, GafferGoalOverview>())
  let busy = $state(false), notice = $state(''), bad = $state(false), dialogError = $state('')
  let moving = $state(false), telling = $state(false), tellStart = $state(''), tellRequest = ''
  let disposed = false
  async function load() {
    try {
      const [found, view] = await Promise.all([listGafferOutcomes(), listGafferGoals()])
      if (disposed) return
      record = found.find(item => item.id === member.outcomeId) ?? null
      goals = new Map(view.goals.map(item => [item.goal.conversationId, item]))
    } catch { /* The Factory page says when it cannot be read; the session simply shows no decision. */ }
  }
  onMount(() => {
    void load()
    const timer = setInterval(() => { if (!document.hidden && !busy && !moving && !telling) void load() }, 8000)
    return () => { disposed = true; clearInterval(timer) }
  })
  // Only the session doing the stage in hand carries the pipeline's decision.
  const waiting = $derived(record && !record.archivedAt ? waitingDecisions(record, goals).find(item => item.piece.key === member.key && item.piece.sessionId === sessionId) : undefined)
  const card = $derived(`/factory#pipeline-${member.outcomeId}-${member.key}`)
  // In the session itself there is nothing to open, and an agent's question is answered right here.
  const choices = $derived((waiting?.need.choices ?? []).filter(choice => choice.id !== 'open' && choice.id !== 'answer' && (choice.id !== 'move' || waiting?.piece.state === 'following') && (choice.id !== 'tell' || waiting?.kind === 'goal')))
  const goal = $derived(waiting?.piece.conversationId ? goals.get(waiting.piece.conversationId) : undefined)
  const used = $derived(waiting?.piece.stages?.[waiting.piece.stage ?? 0]?.decided ?? 0)
  const taken = $derived(record?.decisions ?? [])
  const kind = $derived(waiting?.piece.stages?.[waiting.piece.stage ?? 0]?.kind ?? '')
  // Why Gaffer is leaving this to the person, so the card answers "why is it not sending this itself?" and says what to
  // do about it. A lost session or a merge is the person's whatever the outcome's decisions setting is.
  const note = $derived(waiting?.kind !== 'goal' || !record ? '' : !record.automation
    ? 'You decide this outcome’s decisions. Pick one below, or turn on Gaffer decides on the Factory card to have Gaffer take them.'
    : used >= MAX_STAGE_DECISIONS ? `Gaffer has taken its ${MAX_STAGE_DECISIONS} decisions on this stage, so this one is yours. Pick one below.`
    : used > 0 && taken.some(item => item.failed && item.key === waiting.piece.key && item.stage === kind) ? 'Gaffer left this one to you. Pick one below; its reason is on the Factory card.'
    : 'Gaffer is deciding this now. You can still choose first.')
  const requestId = () => Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('')
  const fail = (e: unknown, fallback: string) => { bad = true; notice = e instanceof Error ? e.message : fallback }
  async function act(run: () => Promise<string>, fallback: string) {
    if (busy) return
    busy = true; notice = ''; bad = false
    try { notice = await run() } catch (e) { fail(e, fallback) }
    busy = false
    await load()
  }
  function choose(choice: NeedChoice) {
    const piece = waiting?.piece as GafferOutcomeSession | undefined
    if (!record || !piece) return
    const current = record
    if (choice.id === 'retry' || choice.id === 'again') void act(async () => {
      const view = await getGafferGoal(piece.conversationId ?? '')
      if (!view.goal) throw new Error('That goal is no longer there.')
      if (!view.allowed) throw new Error(view.reason || 'Review this goal’s permissions before trying again.')
      await controlGafferGoal(piece.conversationId ?? '', view.goal.revision, 'resume', view.pins, true)
      return 'Gaffer is reviewing this stage again.'
    }, 'The review was not tried again.')
    else if (choice.id === 'restart') void act(async () => { await retryGafferPipeline(current, piece.key); return 'Running this stage again in a new session.' }, 'That pipeline was not tried again.')
    else if (choice.id === 'move') moving = true
    else if (choice.id === 'tell') { tellStart = choice.recommended && goal?.goal.advice?.choice === 'tell' ? goal.goal.advice.text ?? '' : ''; tellRequest = requestId(); dialogError = ''; telling = true }
    // A merge, a new budget and stopping the outcome each have their own review on the Factory.
    else location.href = card
  }
  async function moveOn() {
    const piece = waiting?.piece, current = record
    moving = false
    if (!piece || !current) return
    await act(async () => { await moveGafferPipelineOn(current, piece.key, piece.stages?.[piece.stage ?? 0]?.kind ?? ''); return 'Moved on. The controller is starting the next stage in a session of its own.' }, 'That pipeline was not moved on.')
  }
  async function tell(text: string) {
    const piece = waiting?.piece, current = record
    if (!piece || !current || busy) return
    busy = true; dialogError = ''
    try {
      const sent = await tellGafferPipelineAgent(current, piece.key, tellRequest, text)
      telling = false; bad = false
      notice = ['sent', 'queued', 'sending'].includes(sent.message.state) ? 'Your instruction is with the agent for its next turn.' : `The instruction was not delivered: ${sent.message.note || sent.message.state}.`
    } catch (e) { dialogError = e instanceof Error ? e.message : 'The instruction was not sent.' }
    busy = false
    await load()
  }
</script>

{#if waiting && choices.length}<div class="session-decision">
  <Decision title={waiting.piece.title} stage={waiting.stage} href={card} need={waiting.need} {choices} {busy} onchoose={choose} {note}
    review={goal?.goal.summary ?? ''}/>
</div>{/if}
{#if notice}<p class="session-decision-note" class:bad role={bad ? 'alert' : 'status'}>{notice}</p>{/if}
{#if moving && waiting}<ConfirmDialog title={`Move “${waiting.piece.title}” on?`} body="This closes the stage in hand as it stands and starts the next stage in a session of its own. Gaffer’s review of this stage is not waited for." confirmLabel="Move on" onconfirm={() => void moveOn()} oncancel={() => moving = false}/>{/if}
{#if telling && waiting}<PipelineNote title={`Tell the agent of “${waiting.piece.title}”`} intro="Your words go to this pipeline’s working agent for its next turn, exactly as you write them. It cannot interrupt the agent or answer an approval." label="Instruction" initial={tellStart} confirmLabel="Send to the agent" {busy} error={dialogError} oncancel={() => telling = false} onconfirm={text => void tell(text)}/>{/if}

<style>
  .session-decision{padding:8px 12px 0;min-width:0}
  .session-decision-note{margin:0;padding:6px 12px 0;color:var(--muted)}.session-decision-note.bad{color:var(--bad)}
</style>
