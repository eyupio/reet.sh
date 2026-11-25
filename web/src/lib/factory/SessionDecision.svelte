<script lang="ts">
  // The decision a Factory pipeline is waiting on the person for, shown in the session that is doing the pipeline's
  // stage in hand. The choices are the ones on the Factory card, recommended first; the ones that need the Factory's own
  // dialogs (a merge, a new budget, stopping the outcome) go to the pipeline's card there.
  import { onMount } from 'svelte'
  import { listGafferGoals, listGafferOutcomes, type GafferGoalOverview, type GafferOutcome } from '../api'
  import { MAX_STAGE_DECISIONS, deliveryHref, waitingDecisions, type FactoryMember, type NeedChoice } from '../gafferOutcome'
  import Decision from './Decision.svelte'
  import PipelineChoices from './PipelineChoices.svelte'

  let { member, sessionId }: { member: FactoryMember; sessionId: string } = $props()
  let record = $state<GafferOutcome | null>(null), goals = $state(new Map<string, GafferGoalOverview>())
  let busy = $state(false), notice = $state(''), bad = $state(false)
  // Carries out the choices exactly as the Factory card does, so the bell and the title follow at once.
  let pipeline = $state<PipelineChoices>()
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
    const timer = setInterval(() => { if (!document.hidden && !busy) void load() }, 8000)
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
  // A merge, a new budget and stopping the outcome have their reviews on the Factory page, as does the delivery review.
  const own = ['retry', 'again', 'replace-worker', 'restart', 'backup', 'move', 'tell']
  function choose(choice: NeedChoice) {
    const piece = waiting?.piece
    if (!record || !piece) return
    if (own.includes(choice.id)) pipeline?.choose(choice, record, piece)
    else location.href = choice.id === 'review' ? deliveryHref(member.outcomeId, member.key) : card
  }
</script>

{#if waiting && (choices.length || waiting.kind === 'usage_limit')}<div class="session-decision">
  <Decision title={waiting.piece.title} stage={waiting.stage} href={card} need={waiting.need} {choices} {busy} onchoose={choose} {note}
    review={waiting.kind === 'usage_limit' ? '' : goal?.goal.summary ?? ''}/>
</div>{/if}
{#if notice}<p class="session-decision-note" class:bad role={bad ? 'alert' : 'status'}>{notice}</p>{/if}
<PipelineChoices bind:this={pipeline} bind:busy {goals} latest={item => record?.id === item.id ? record : item} onchanged={load} onnotice={(text, failed = false) => { notice = text; bad = failed }}/>

<style>
  .session-decision{padding:8px 12px 0;min-width:0}
  .session-decision-note{margin:0;padding:6px 12px 0;color:var(--muted)}.session-decision-note.bad{color:var(--bad)}
</style>
