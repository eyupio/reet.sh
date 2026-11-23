<script lang="ts" module>
  import type { Event as ActivityEvent, GafferGoalOverview, GafferOutcome as Outcome, GafferOutcomeSession } from '../api'
  import type { NeedChoice } from '../gafferOutcome'
  import type { PlanEntry } from '../agentAccounts'
  import type { NeedCard } from '../home'

  type Pair = { record: Outcome; piece: GafferOutcomeSession }
  /** What the detail asks the page to do. Each keeps the name of the page's own handler, or of the dialog it opens. */
  export type DetailActions = {
    choose: (choice: NeedChoice, record: Outcome, piece: GafferOutcomeSession, goal: GafferGoalOverview | undefined) => void
    setDecisions: (record: Outcome, on: boolean) => void
    setYolo: (record: Outcome, on: boolean) => void
    mergeMode: (record: Outcome, piece: GafferOutcomeSession, on: boolean) => void
    report: (record: Outcome) => void
    /** Reads the outcome's history afresh, a failed read included. Never takes the entry away. */
    loadHistory: (record: Outcome) => void
    stop: (record: Outcome) => void
    putAway: (record: Outcome, mode: 'archive' | 'delete' | 'restore') => void
    retry: (record: Outcome, piece: GafferOutcomeSession) => void
    pick: (record: Outcome, title: string, on: boolean) => void
    releasing: (record: Outcome) => void
    deleting: (record: Outcome) => void
    adding: (record: Outcome) => void
    /** Saves the finished outcome's plan as a private template. */
    savingTemplate: (record: Outcome) => void
    /** Opens a template, from the line that says an outcome was started from it. */
    openTemplate: (id: string) => void
    moving: (pair: Pair) => void
    /** Opens the Tell dialog with no words of Gaffer's in it, as today's Tell the agent… button does. */
    telling: (pair: Pair) => void
    sending: (pair: Pair) => void
    merging: (pair: Pair) => void
    managing: (goal: GafferGoalOverview) => void
    /** A delivery decision, evidence or criteria were recorded: the page reads the outcomes again. */
    changed: () => void | Promise<void>
  }
  /** The page's pure helpers, under their own names. */
  export type DetailHelpers = {
    crew: (piece: GafferOutcomeSession, record: Outcome) => string
    planFor: (record: Outcome, piece: GafferOutcomeSession) => PlanEntry[]
    sessionHref: (record: Outcome, id: string) => string
    when: (at: number) => string
    usable: (choice: NeedChoice, piece: GafferOutcomeSession) => boolean
    mergesByItself: (record: Outcome, piece: GafferOutcomeSession) => boolean
    pipelineOf: (event: ActivityEvent) => string
  }
  /** An outcome's history as the page reads it when asked. */
  export type HistoryEntry = { events: ActivityEvent[]; error: string; loading: boolean }
</script>

<script lang="ts">
  // One pipeline, all of it: where it stands, what waits on the person, the facts of the stage they pick, the rest in
  // rows that open, and one bar of what can be done. Every action is the page's own; this only draws and asks.
  import { changedAfterAccept, DEFAULT_STAGES, MAX_STAGE_DECISIONS, agentActivity, agentTokensText, ago, contextLeft, decisionText, outcomeAgentUsage, outcomeFinished, outcomeUsage, pipelineAgentUsage, pipelineStatus, pipelineUsage, sessionLost, stageChoice, stageLabel, timeLeft, tokensText } from '../gafferOutcome'
  import { chatToolName } from '../terminal/api'
  import { sourceLabel } from '../clineSource'
  import { eventLabel } from '../events'
  import { checkLabel, checkTone, safeInsightURL } from '../sessionInsights'
  import { ensurePullInsight, sessionInsights } from '../sessions/insightStore'
  import GafferProse from '../GafferProse.svelte'
  import SkillBuddy from '../SkillBuddy.svelte'
  import PlanMeters from '../PlanMeters.svelte'
  import Approval from '../shell/Approval.svelte'
  import StagePipeline from './StagePipeline.svelte'
  import DeliveryReview from './DeliveryReview.svelte'
  import TemplateLine from './TemplateLine.svelte'
  import { ciRun, ciWord, gafferFixes } from './githubFacts'
  import type { FactoryItem } from './factoryList'

  let { record, piece, item, goal, goals, busy, reporting, clock, planClock, timeline, ticked, expanded = $bindable(), narrow = false, reviewShown = false, removeMode = 'archive', position, permission, onanswered, actions, helpers, onback, onprev, onnext }: {
    record: Outcome
    piece: GafferOutcomeSession
    item: FactoryItem
    goal: GafferGoalOverview | undefined
    goals: ReadonlyMap<string, GafferGoalOverview>
    busy: boolean
    reporting: string
    clock: number
    planClock: number
    timeline: HistoryEntry | undefined
    ticked: string[]
    expanded: Record<string, boolean>
    narrow?: boolean
    /** The address or a button asked for the delivery review: it is read at once. */
    reviewShown?: boolean
    /** The account's default for putting a finished outcome away: that entry comes first in the menu. */
    removeMode?: 'archive' | 'delete'
    position?: { at: number; of: number }
    /** The tool approval the pipeline's session holds, answered at the head of the bar as on the waiting card. */
    permission?: NonNullable<NeedCard['approval']>
    onanswered?: () => void | Promise<void>
    actions: DetailActions
    helpers: DetailHelpers
    onback?: () => void
    onprev?: () => void
    onnext?: () => void
  } = $props()

  const uid = $props.id()
  // A pipeline that has just had its last criterion removed keeps its card for this visit, so the notice and the focus have somewhere to be.
  let hadCriteria = $state(false)
  $effect(() => { if (piece.criteria?.length) hadCriteria = true })
  const nodes = $derived(item.nodes)
  const status = $derived(pipelineStatus(piece, !!goal?.attention))
  const lost = $derived(sessionLost(piece))
  const alive = $derived(piece.sessionState !== 'ended' && piece.sessionState !== 'gone')
  const finished = $derived(outcomeFinished(record))
  // An archived outcome's pipelines are kept to be read: their only action is Restore, in the menu.
  const archived = $derived(!!record.archivedAt)
  // Allow once and Deny, when the item is an approval and the session holds one on record.
  const asking = $derived(!archived && item.kind === 'approval' && !!permission && !!piece.sessionId)
  const taken = $derived(record.decisions ?? [])
  const inHand = $derived(piece.stages?.[piece.stage ?? 0])
  const last = $derived((piece.stage ?? 0) >= nodes.length - 1)
  const hasMerge = $derived(nodes.some(node => node.kind === 'merge'))
  const auto = $derived(helpers.mergesByItself(record, piece))
  const tone = $derived(item.section === 'waiting' ? 'you' : item.kind === 'running' ? 'good' : 'plain')

  // GitHub is read from the session in hand, whose folder and branch the whole pipeline shares. The store keeps one
  // observation per session, so every surface asking about this pipeline shares it.
  const github = $derived(piece.sessionId ? $sessionInsights[piece.sessionId]?.github : undefined)
  const pull = $derived(github?.pull)
  const pullURL = $derived(safeInsightURL(pull?.url))
  const checks = $derived(pull?.checks ?? [])
  const checksWord = $derived(ciWord(checks))
  const ciClock = $derived(ciRun(checks, clock))
  const fixes = $derived(gafferFixes(record.decisions, piece.key))
  const readAt = $derived(github?.checkedAt ? helpers.when(github.checkedAt) : '')
  $effect(() => {
    const id = piece.sessionId
    if (!id || record.archivedAt) return
    void ensurePullInsight(id, alive)
  })

  // The stage the person picked, for this pipeline only: another pipeline starts on its stage in hand.
  let chosen = $state<{ id: string; index: number } | null>(null)
  const selected = $derived(chosen?.id === item.id ? chosen.index : item.at)
  const node = $derived(selected >= 0 ? nodes[selected] : undefined)
  const here = $derived(!node || (selected === item.at && node.state !== 'done'))
  const agentLine = (n: NonNullable<typeof node>) => `${chatToolName(n.tool)}${n.model ? ` · ${n.model}` : ''}${n.source ? ` · ${sourceLabel(n.source)}` : ''}${n.effort ? ` · ${n.effort} effort` : ''}${n.effortNotOffered && !n.effort ? ` · agent's default effort (${n.effortNotOffered} is not offered)` : ''}`

  // Gaffer's latest review sits with what waits when it is why the pipeline waits, and with the stage otherwise.
  const usageLimited = $derived(!lost && !!goal?.workerUsageLimited)
  const reviewWaits = $derived(item.section === 'waiting' && !usageLimited && !lost && status.label === 'Needs you' && !!goal)
  const used = $derived(inHand?.decided ?? 0)
  // A decision Gaffer could not take, or chose to leave, keeps this stop with the person; saying it is deciding would be
  // a promise nothing is going to keep.
  const leftToYou = $derived(used > 0 && taken.some(d => d.failed && d.key === piece.key && d.stage === inHand?.kind))
  const gafferNote = $derived(record.archivedAt || !record.automation || item.kind === 'merge' || item.kind === 'delivery' || usageLimited ? '' : leftToYou ? 'Gaffer left this one to you. See what it decided, below.' : goal?.goal.waitingOnWorker ? (used >= MAX_STAGE_DECISIONS ? '' : 'If the agent is asking a multiple-choice question and nothing else, Gaffer answers it. You can still answer first.') : used >= MAX_STAGE_DECISIONS ? `Gaffer has taken its ${MAX_STAGE_DECISIONS} decisions on this stage, so this one is yours.` : 'Gaffer is deciding this. You can still choose first.')

  // One bar: the choices of what waits, then the buttons a pipeline has today, each only once. A choice and a button
  // that do the same thing share a key. Opening the session is a link (`href`).
  type Entry = { key: string; label: string; primary?: boolean; recommended?: boolean; href?: string; run: () => void }
  const bar = $derived.by(() => {
    const out: Entry[] = [], does = new Set<string>()
    if (archived) return out
    if (item.section === 'waiting' && item.need) for (const choice of item.need.choices) {
      // Stopping the outcome is in its menu. Opening the session is in the head, and in the bar too only when it is the
      // choice to take (Gaffer advised it, or it is the only one); with an approval on record, Allow and Deny answer instead.
      if (choice.id === 'stop' || (choice.id === 'open' && !choice.primary) || (choice.id === 'answer' && asking)) continue
      if (lost ? choice.id === 'move' && !(nodes.length && piece.conversationId) : !helpers.usable(choice, piece)) continue
      does.add(choice.id)
      const href = (choice.id === 'open' || choice.id === 'answer') && piece.sessionId ? helpers.sessionHref(record, piece.sessionId) : undefined
      out.push({ key: choice.id, label: choice.label, primary: choice.primary, recommended: choice.recommended, href, run: () => actions.choose(choice, record, piece, goal) })
    }
    const add = (entry: Entry) => { if (!does.has(entry.key)) { does.add(entry.key); out.push(entry) } }
    if (piece.state === 'ready_to_merge') add({ key: 'merge', label: 'Approve merge', run: () => actions.merging({ record, piece }) })
    if (!usageLimited && piece.sessionId && piece.conversationId && alive && !goal?.goal.waitingOnWorker && ['following', 'unfollowed', 'ready_to_merge'].includes(piece.state)) add({ key: 'tell', label: 'Tell the agent…', run: () => actions.telling({ record, piece }) })
    if (!usageLimited && nodes.length && !lost && ['following', 'ready_to_merge', 'done', 'unfollowed'].includes(piece.state)) add({ key: 'send', label: 'Send back…', run: () => actions.sending({ record, piece }) })
    if (!usageLimited && piece.state === 'following' && nodes.length && piece.conversationId) add({ key: 'move', label: last ? 'Finish this stage' : 'Move to next stage', run: () => actions.moving({ record, piece }) })
    if (['failed', 'unfollowed', 'cancelled'].includes(piece.state)) add({ key: 'restart', label: 'Try again', primary: item.kind === 'failed', run: () => actions.retry(record, piece) })
    // A finished outcome needs nothing from the line, so the one thing to do next is to put it away. The account's
    // default comes first, and Delete asks first in its own dialog.
    if (finished) {
      add({ key: 'template', label: 'Save as template…', run: () => actions.savingTemplate(record) })
      const archive = { key: 'putaway-archive', label: 'Archive outcome', primary: true, run: () => actions.putAway(record, 'archive') }
      const remove = { key: 'putaway-delete', label: 'Delete outcome…', run: () => actions.deleting(record) }
      if (removeMode === 'delete') { add(remove); add(archive) } else { add(archive); add(remove) }
    }
    return out
  })
  // A goal's limits are opened from Usage and budget, unless the bar already offers a new budget.
  const goalButton = $derived(!!goal && !archived && !bar.some(entry => entry.key === 'budget'))

  // The headlines the closed rows show.
  const proposals = $derived(record.report?.proposals ?? [])
  const chosenCount = $derived(proposals.filter(p => ticked.includes(p.title)).length)
  const reportLine = $derived(proposals.length ? `${proposals.length} ${proposals.length === 1 ? 'pipeline' : 'pipelines'} proposed` : record.report ? `written ${helpers.when(record.report.at)}` : 'Not asked for yet')
  const usageLine = $derived(`Gaffer ${tokensText(pipelineUsage(piece, goal?.goal))} · agents ${agentTokensText(pipelineAgentUsage(piece))}`)
  const decidedLine = $derived(taken.length ? `${taken.length} ${taken.length === 1 ? 'decision' : 'decisions'}` : 'Nothing yet')
  const historyLine = $derived(!timeline ? 'Open to read' : timeline.loading ? 'Reading…' : timeline.error ? 'Could not be read' : !timeline.events.length ? 'Nothing recorded yet' : `${timeline.events.length} ${timeline.events.length === 1 ? 'event' : 'events'} · last ${ago(timeline.events[0].at, clock)} ago`)
  const settingsLine = $derived([record.automation ? 'Gaffer decides' : 'You decide', record.yolo ? 'YOLO' : 'Ask me', ...(hasMerge ? [auto ? 'merges by itself' : 'merge waits for you'] : [])].join(' · '))
  const totals = $derived(outcomeUsage(record.sessions.map(p => pipelineUsage(p, p.conversationId ? goals.get(p.conversationId)?.goal : undefined)), record.report))
  const agents = $derived(outcomeAgentUsage(record.sessions))

  let menu = $state<HTMLDetailsElement>(), menuButton = $state<HTMLElement>()
  const fromMenu = (run: () => void) => { if (menu) menu.open = false; run() }
  // The menu closes on Escape and on a press outside it, and the focus goes back to its button (unless the press was on
  // something that takes the focus itself).
  // After a press the browser moves the focus itself once the press is handled, so the focus is given back after that.
  function closeMenu(refocus: boolean) { if (!menu?.open) return; menu.open = false; if (refocus) setTimeout(() => menuButton?.focus({ preventScroll: true })) }
  $effect(() => {
    const key = (event: KeyboardEvent) => { if (event.key === 'Escape' && menu?.open) { event.preventDefault(); closeMenu(true) } }
    const press = (event: PointerEvent) => {
      const target = event.target as Element | null
      if (!menu?.open || !target || menu.contains(target)) return
      closeMenu(!target.closest('a[href],button,input,select,textarea,summary,label'))
    }
    document.addEventListener('keydown', key); document.addEventListener('pointerdown', press, true)
    return () => { document.removeEventListener('keydown', key); document.removeEventListener('pointerdown', press, true) }
  })
  // The objective is a sentence; before " · pipeline n of m" its closing full stop goes.
  const objective = $derived(record.objective.trim().replace(/\.+$/, ''))
</script>

{#snippet review(text: string, key: string)}{@const whole = expanded[key]}
  <div class="report"><p class="eyebrow">Gaffer’s latest review</p><div class="prose-clamp" class:open={whole}><GafferProse {text}/></div>{#if text.length > 280}<button class="inline" type="button" aria-expanded={!!whole} onclick={() => expanded = { ...expanded, [key]: !whole }}>{whole ? 'Show less' : 'Read all of it'}</button>{/if}</div>
{/snippet}

<section class="detail" aria-labelledby={`${uid}-title`}>
  {#if narrow}<nav class="back-bar" aria-label="Waiting items">
    <button class="btn quiet" type="button" onclick={() => onback?.()}>‹ Factory</button>
    {#if position}<span class="place">{position.at} of {position.of} waiting for you</span>
      <button class="btn step" type="button" aria-label="Previous waiting item" disabled={position.at <= 1} onclick={() => onprev?.()}>‹</button>
      <button class="btn step" type="button" aria-label="Next waiting item" disabled={position.at >= position.of} onclick={() => onnext?.()}>›</button>{/if}
  </nav>{/if}

  <header class="head">
    <div class="title">
      <p class="eyebrow">{objective} · pipeline {item.index} of {item.of}</p>
      <div class="name"><h2 id={`${uid}-title`} tabindex="-1">{piece.title}</h2><span class="chip {tone}">{item.chip}</span></div>
      <p class="meta">{record.projectName || 'Project'} · started {helpers.when(record.createdAt)}{#if record.state === 'starting'} · <span class="live">the controller is starting sessions</span>{:else if record.state === 'stopped'} · stopped{/if}{#if record.archivedAt} · archived {helpers.when(record.archivedAt)}{/if}</p>
      {#if record.templateId && record.templateVersion}{#key record.templateId + record.templateVersion}<TemplateLine id={record.templateId} version={record.templateVersion} name={record.templateName} onopen={actions.openTemplate}/>{/key}{/if}
    </div>
    <div class="head-actions">
      {#if piece.sessionId && piece.sessionState !== 'gone'}<a class="btn quiet" href={helpers.sessionHref(record, piece.sessionId)}>{alive ? 'Open session' : 'Read the ended session'}</a>{/if}
      <details class="menu" bind:this={menu}>
        <!-- Said outright: not every reader of the page, a test runner among them, takes a summary to be a button. -->
        <!-- svelte-ignore a11y_no_redundant_roles -->
        <summary class="btn quiet" role="button" aria-label="More for this outcome" bind:this={menuButton}>⋯</summary>
        <ul>
          {#if record.archivedAt}<li><button type="button" disabled={busy} onclick={() => fromMenu(() => actions.putAway(record, 'restore'))}>Restore</button></li>
          {:else}
            {#if record.state === 'starting'}<li><button type="button" disabled={busy} onclick={() => fromMenu(() => actions.stop(record))}>Stop starting sessions</button></li>{/if}
            {#if !finished}<li><button type="button" disabled={busy} onclick={() => fromMenu(() => actions.releasing(record))}>Stop following</button></li>
            {:else}{#snippet archive()}<li><button type="button" disabled={busy} onclick={() => fromMenu(() => actions.putAway(record, 'archive'))}>Archive</button></li>{/snippet}
              {#snippet remove()}<li><button class="danger" type="button" disabled={busy} onclick={() => fromMenu(() => actions.deleting(record))}>Delete</button></li>{/snippet}
              {#if removeMode === 'delete'}{@render remove()}{@render archive()}{:else}{@render archive()}{@render remove()}{/if}{/if}
          {/if}
        </ul>
      </details>
    </div>
  </header>

  <StagePipeline {nodes} at={item.at} size="labelled" waiting={item.section === 'waiting'} {selected} onselect={index => (chosen = { id: item.id, index })} label={`Choose a stage of ${piece.title}`}/>

  {#if node && selected >= 0}<SkillBuddy context={{outcomeId: record.id, pipeline: piece.key, stage: selected}} disabled={archived} sessionHref={piece.stages?.[selected]?.sessionId ? helpers.sessionHref(record, piece.stages[selected].sessionId!) : ''}/>{/if}

  {#if item.section !== 'waiting' && item.why}<p class="note">{item.why}</p>{/if}

  {#if item.section === 'waiting'}<section class="waits" aria-label="What is waiting">
    <p class="waits-head">{usageLimited ? 'Worker usage limit reached' : 'Waiting for you'}</p>
    <p>{item.need?.why ?? item.why}</p>
    {#if usageLimited}{@const plan = helpers.planFor(record, piece)}{#if plan.length}<p class="hint">{plan.map(entry => `${chatToolName(entry.tool)} plan: ${entry.plan.detail}`).join(' ')}</p>{/if}{/if}
    {#if reviewWaits && goal?.goal.summary}{@render review(goal.goal.summary, piece.conversationId ?? '')}{/if}
    {#if item.need?.recommend?.by === 'gaffer'}<p class="advice"><strong>Gaffer recommends: {item.need.recommend.label.replace(/…$/, '')}.</strong> {item.need.recommend.why}</p>{/if}
    {#if !usageLimited && goal?.goal.advice?.choice === 'tell' && goal.goal.advice.text}<p class="said">It would say: “{goal.goal.advice.text}”</p>{/if}
    {#if gafferNote}<p class="hint">{gafferNote}</p>{/if}
    {#if item.kind === 'delivery'}<p class="hint">{changedAfterAccept(piece.note) ? 'The work changed after you accepted it, so your acceptance and any merge approval were withdrawn. Review what is there now.' : 'The pipeline’s work is ready to be checked against the criteria you agreed.'} The delivery review is below. Accepting the delivery is not approving the merge.</p>{/if}
    {#if lost}<p class="hint">A new session works in the same folder and branch. Moving on starts the next stage in a session of its own.</p>{/if}
  </section>{/if}

  {#if piece.criteria?.length || hadCriteria}<DeliveryReview {record} {piece} {archived} {reviewShown} when={helpers.when} onchanged={actions.changed}/>{/if}

  <section class="stage" aria-label={node ? `${node.label} stage` : 'This pipeline'}>
    <p class="stage-head"><strong>{node ? `${node.label} stage` : 'This pipeline'}</strong>{#if nodes.length > 1}<span class="hint">Pick another stage above to see that one.</span>{/if}</p>
    {#if here}
      <dl class="facts">
        <div><dt>Agent</dt><dd>{helpers.crew(piece, record)}</dd></div>
        {#if piece.state !== 'done' && inHand?.balanced}<div><dt>Balanced</dt><dd>{inHand.balanced}</dd></div>{/if}
        {#if piece.state !== 'done'}{@const plan = helpers.planFor(record, piece)}
          {#if plan.length}<div><dt>Plan left</dt><dd><PlanMeters entries={plan} now={planClock} wide /></dd></div>{/if}{/if}
        {#if piece.state === 'following' || piece.state === 'delivery_review' || piece.state === 'ready_to_merge'}{@const applied = stageChoice(piece.stages?.length ? inHand : piece.applied)}
          {#if applied}<div><dt>Applied</dt><dd class:low={applied.tone === 'bad'}><strong>{applied.label}.</strong> {applied.text}</dd></div>{/if}{/if}
        {#if piece.state === 'following'}{@const activity = agentActivity(inHand, clock)}
          {#if inHand?.startedAt}<div><dt>In this stage</dt><dd>{ago(inHand.startedAt, clock)}</dd></div>{/if}
          {#if activity.text}<div><dt>Doing now</dt><dd class:low={activity.quiet}>{activity.text}</dd></div>{/if}{/if}
        {#if goal}<div><dt>Gaffer reviews</dt><dd>{goal.goal.usedReviews} of {goal.goal.maxReviews}</dd></div>{/if}
      </dl>
      {#if !reviewWaits && goal?.goal.summary}{@render review(goal.goal.summary, piece.conversationId ?? '')}{/if}
    {:else if node && node.state === 'done'}
      <dl class="facts">
        <div><dt>Who did it</dt><dd>{node.tier ? `${node.tier} stage · ` : ''}{agentLine(node)}</dd></div>
        <div><dt>How it ended</dt><dd>{node.moved ? 'Done: you moved it on' : node.auto ? 'Done: Gaffer decided to move on' : 'Done: Gaffer handed it over'}</dd></div>
      </dl>
      {#if node.tierWhy}<p class="handover">Gaffer’s assessment: {node.tierWhy}</p>{/if}
      {#if node.balanced}<p class="handover"><strong>Balanced.</strong> {node.balanced}</p>{/if}
      {#if node.choice}<p class="handover" class:low={node.choice.tone === 'bad'}><strong>{node.choice.label}.</strong> {node.choice.text}</p>{/if}
      {#if node.summary}<div class="report"><p class="eyebrow">Handover</p><p>{node.summary}</p></div>{/if}
    {:else if node}
      <p>{node.tier ? `${node.tier} stage · ` : ''}{agentLine(node)} will do this stage.</p>
      {#if node.tierWhy}<p class="handover">Gaffer’s assessment: {node.tierWhy}</p>{/if}
    {/if}
  </section>

  {#if pull}
    <section class="github" aria-label="GitHub">
      <p class="stage-head"><strong>GitHub</strong>{#if readAt}<span class="hint">Read {readAt}</span>{/if}</p>
      <dl class="facts">
        <div><dt>Pull request</dt><dd>{#if pullURL}<a href={pullURL} target="_blank" rel="noopener noreferrer">#{pull.number}</a>{:else}#{pull.number}{/if} · {pull.state.toLowerCase()}{#if pull.draft} · draft{/if}{#if pull.title} <span class="split">{pull.title}</span>{/if}</dd></div>
        <div><dt>CI</dt><dd>{checksWord}</dd></div>
        <div><dt>CI run time</dt><dd class:low={ciClock.tone === 'warn'}>{ciClock.text}</dd></div>
        <div><dt>Gaffer fixes</dt><dd class:low={fixes.count > 0}>{fixes.count ? `${fixes.count} ${fixes.count === 1 ? 'fix' : 'fixes'}${fixes.stages.length ? ` · ${fixes.stages.map(stageLabel).join(', ')}` : ''}` : 'None'}</dd></div>
      </dl>
      {#if checks.length}
        <details class="ci">
          <summary>{checks.length} CI {checks.length === 1 ? 'stage' : 'stages'}</summary>
          <ul aria-label="CI stages">
            {#each checks as check, index (check.name + (check.url ?? '') + index)}
              {@const link = safeInsightURL(check.url)}
              <li><span class="ci-state" data-tone={checkTone(check.state)}>{checkLabel(check.state)}</span>{#if link}<a href={link} target="_blank" rel="noopener noreferrer">{check.name}</a>{:else}<span>{check.name}</span>{/if}</li>
            {/each}
          </ul>
        </details>
      {:else}<p class="hint">No CI checks were reported for this pull request.</p>{/if}
    </section>
  {/if}

  <div class="rows">
    <details class="row">
      <summary><span class="label">Gaffer’s report</span><span class="headline">{reportLine}</span></summary>
      <div class="body">
        {#if record.report}
          <p class="eyebrow">Gaffer’s report · {helpers.when(record.report.at)}{record.report.auto ? ' · made by itself when a pipeline finished or stopped' : ''}</p>
          <div class="prose-clamp report-clamp" class:open={expanded[`report-${record.id}`]}><GafferProse text={record.report.text}/></div>
          {#if record.report.text.length > 400}<button class="inline" type="button" aria-expanded={!!expanded[`report-${record.id}`]} onclick={() => expanded = { ...expanded, [`report-${record.id}`]: !expanded[`report-${record.id}`] }}>{expanded[`report-${record.id}`] ? 'Show less' : 'Read all of it'}</button>{/if}
          {#if proposals.length && archived}
            <p class="hint">Gaffer suggested adding {proposals.length === 1 ? 'this pipeline' : 'these pipelines'}. Restore the outcome to add any.</p>
            <ul class="proposals" aria-label="Proposed pipelines">
              {#each proposals as proposal (proposal.title)}<li><span class="proposal"><strong>{proposal.title}</strong>{#if proposal.why}<span class="why">{proposal.why}</span>{/if}<span class="why">{(proposal.stages?.length ? proposal.stages : DEFAULT_STAGES).map(stageLabel).join(' → ')}</span></span></li>{/each}
            </ul>
          {:else if proposals.length}
            <p class="hint">Gaffer suggests adding {proposals.length === 1 ? 'this pipeline' : 'these pipelines'}. Nothing is added or started unless you approve it, and you choose each one’s agent, model and effort then.</p>
            <ul class="proposals" aria-label="Proposed pipelines">
              {#each proposals as proposal (proposal.title)}<li><label><input type="checkbox" checked={ticked.includes(proposal.title)} onchange={event => actions.pick(record, proposal.title, event.currentTarget.checked)}/><span><strong>{proposal.title}</strong>{#if proposal.why}<span class="why">{proposal.why}</span>{/if}<span class="why">{(proposal.stages?.length ? proposal.stages : DEFAULT_STAGES).map(stageLabel).join(' → ')}</span></span></label></li>{/each}
            </ul>
            <div class="actions"><button class="btn primary" type="button" disabled={busy || !chosenCount} onclick={() => actions.adding(record)}>{chosenCount === 1 ? 'Add 1 pipeline' : `Add ${chosenCount} pipelines`}</button></div>
          {/if}
        {/if}
        {#if !record.report && archived}<p class="hint">No report was asked for.</p>{/if}
        {#if !archived}<div class="actions"><button class="btn" type="button" disabled={busy || !!reporting} onclick={() => actions.report(record)}>{reporting === record.id ? 'Gaffer is reporting…' : record.report ? 'Ask Gaffer again' : 'Ask Gaffer for a report'}</button></div>{/if}
      </div>
    </details>

    <details class="row">
      <summary><span class="label">Usage and budget</span><span class="headline">{usageLine}</span></summary>
      <div class="body">
        <dl class="facts" aria-label="This pipeline">
          {#if goal && goal.goal.state === 'active'}<div><dt>Reviews left</dt><dd>{Math.max(0, goal.goal.maxReviews - goal.goal.usedReviews)} of {goal.goal.maxReviews}</dd></div>
            <div><dt>Time left</dt><dd>{timeLeft(goal.goal.expiresAt, clock)}</dd></div>{/if}
          {#if goal?.goal.lastReviewedAt}<div><dt>Last review</dt><dd>{helpers.when(goal.goal.lastReviewedAt)}</dd></div>{/if}
          <div><dt>Gaffer tokens</dt><dd>{tokensText(pipelineUsage(piece, goal?.goal))}</dd></div>
          <div><dt>Agent tokens</dt><dd>{agentTokensText(pipelineAgentUsage(piece))}</dd></div>
          {#if piece.state === 'following'}{@const room = contextLeft(inHand)}<div><dt>Context left</dt><dd class:low={room.low}>{room.text || 'Not reported'}</dd></div>{/if}
        </dl>
        <dl class="facts totals" aria-label="This outcome so far">
          <div><dt>Pipelines</dt><dd>{record.sessions.length}</dd></div>
          <div><dt>Gaffer reviews</dt><dd>{totals.reviews}</dd></div>
          <div><dt>Gaffer tokens</dt><dd>{tokensText(totals)}</dd></div>
          <div><dt>Agent tokens</dt><dd>{agentTokensText(agents.total)}{#if agents.byAgent.length > 1}{' '}<span class="split">({agents.byAgent.map(entry => `${chatToolName(entry.tool)} ${agentTokensText({ tokens: entry.tokens, cached: 0, reported: true })}`).join(', ')})</span>{/if}</dd></div>
          <div><dt>Budget per stage</dt><dd>{record.maxReviews} {record.maxReviews === 1 ? 'review' : 'reviews'} · {record.hours} h</dd></div>
        </dl>
        {#if goal && goalButton}{@const g = goal}<div class="actions"><button class="btn" type="button" onclick={() => actions.managing(g)}>{lost || status.label === 'Needs you' ? 'Goal and limits' : 'Review goal'}</button></div>{/if}
      </div>
    </details>

    <details class="row">
      <summary><span class="label">What Gaffer decided</span><span class="headline">{decidedLine}</span></summary>
      <div class="body">
        {#if taken.length}<ol class="taken" aria-label="What Gaffer decided, newest first">{#each [...taken].reverse() as d (`${d.at}-${d.key}`)}{@const words = decisionText(d)}<li class:failed={d.failed}>
          <p class="taken-head"><time datetime={new Date(d.at).toISOString()}>{helpers.when(d.at)}</time><span>{d.title} · {stageLabel(d.stage)}</span></p>
          <p><span class="about">{words.about}.</span> <strong>{words.did}.</strong> {d.why}</p>
          {#if d.text}<p class="said">{d.choice === 'answer' ? d.text : `It told the agent: “${d.text}”`}</p>{/if}
        </li>{/each}</ol>
        {:else}<p class="hint">Gaffer has taken no decision on this outcome.</p>{/if}
      </div>
    </details>

    <!-- Read afresh at every opening, so a move or a send-back since the last one is there. -->
    <details class="row" ontoggle={event => { if (event.currentTarget.open) actions.loadHistory(record) }}>
      <summary><span class="label">History</span><span class="headline">{historyLine}</span></summary>
      <div class="body">
        {#if timeline}
          <p class="eyebrow">Newest first</p>
          {#if timeline.loading}<p role="status">Reading the history…</p>
          {:else if timeline.error}<p role="alert" class="warning">{timeline.error}</p>
          {:else if !timeline.events.length}<p class="hint">Nothing recorded yet. Moves and stops are recorded from the controller version that added this history.</p>
          {:else}<ol class="history">{#each timeline.events as event (event.id)}{@const title = record.sessions.length > 1 ? record.sessions.find(p => p.key === helpers.pipelineOf(event))?.title : ''}<li><time datetime={new Date(event.at).toISOString()}>{helpers.when(event.at)}</time><span>{eventLabel(event)}{#if title}<span class="why">{title}</span>{/if}</span></li>{/each}</ol>{/if}
        {/if}
      </div>
    </details>

    <details class="row">
      <summary><span class="label">Settings for this outcome</span><span class="headline">{settingsLine}</span></summary>
      <div class="body settings">
        <div class="setting"><p class="setting-name">Who decides when a stage stops</p>
          {#if !finished && !archived}<div class="who" role="radiogroup" aria-label="Who decides when a stage stops">
            <label class:on={!record.automation}><input type="radio" name={`who-${record.id}`} checked={!record.automation} disabled={busy} onchange={() => actions.setDecisions(record, false)}/>You decide</label>
            <label class:on={!!record.automation}><input type="radio" name={`who-${record.id}`} checked={!!record.automation} disabled={busy} onchange={() => actions.setDecisions(record, true)}/>Gaffer decides</label>
          </div>{:else}<p>{record.automation ? 'Gaffer decides' : 'You decide'}</p>{/if}
          {#if record.automation}<p class="hint">When a stage stops, Gaffer chooses what happens next and records what it chose and why, at most {MAX_STAGE_DECISIONS} times for a stage. It also answers an agent’s multiple-choice question. Each decision is one request to Gaffer’s model. Merges, approvals and questions that need your own words still come to you.</p>{/if}
        </div>
        <dl class="facts">
          <div><dt>Approvals</dt><dd><span class:yolo={record.yolo}>{record.yolo ? 'YOLO' : 'Ask me'}</span>{#if !record.archivedAt && !finished}{' '}<button class="inline" type="button" disabled={busy} aria-label={record.yolo ? 'Make this outcome’s agents ask me' : 'Put this outcome on YOLO'} onclick={() => actions.setYolo(record, !record.yolo)}>{record.yolo ? 'Make them ask me' : 'Switch to YOLO…'}</button>{/if}</dd></div>
          {#if hasMerge}<div><dt>Merge</dt><dd>{auto ? 'By itself when green' : piece.mergeApproved ? 'You approved it' : record.yolo && piece.autoMerge ? 'Waits for you (YOLO)' : 'Waits for you'}{#if !archived && !['done', 'ready_to_merge', 'unfollowed', 'cancelled', 'failed'].includes(piece.state) && !piece.mergeApproved}{' '}<button class="inline" type="button" disabled={busy} aria-label={`${auto ? 'Make' : 'Let'} ${piece.title} ${auto ? 'wait for you before merging' : 'merge by itself'}`} onclick={() => actions.mergeMode(record, piece, !auto)}>{auto ? 'Make it wait for me' : 'Let it merge by itself'}</button>{/if}</dd></div>{/if}
          <div><dt>Reports</dt><dd>{record.autoReport ? 'By itself and on request' : 'On request'}</dd></div>
        </dl>
      </div>
    </details>
  </div>

  {#if bar.length || asking}<div class="bar" role="group" data-floating-clearance="avoid" aria-labelledby={`${uid}-ask`}>
    <p class="ask" id={`${uid}-ask`}>What do you want to do?</p>
    {#if asking && permission && piece.sessionId}<Approval sessionId={piece.sessionId} approval={permission} {onanswered} />{/if}
    {#if bar.length}<div class="choices">{#each bar as entry (entry.key)}
      {#snippet choice()}{#if entry.href}<a class="btn" class:primary={entry.primary} href={entry.href}>{entry.label}{#if entry.recommended}<span class="sr-only">{' '}Recommended</span>{/if}</a>
      {:else}<button class="btn" class:primary={entry.primary} type="button" disabled={busy} onclick={entry.run}>{entry.label}{#if entry.recommended}<span class="sr-only">{' '}Recommended</span>{/if}</button>{/if}{/snippet}
      <!-- The recommended choice is marked by a caption above it, which widens nothing, so the choices keep one line. -->
      {#if entry.recommended}<span class="lead"><span class="rec" aria-hidden="true">Recommended</span>{@render choice()}</span>{:else}{@render choice()}{/if}{/each}</div>{/if}
  </div>{/if}
</section>

<style>
  /* A column that fills the pane it is put in, so the bar can rest at the foot of it. The page sets --detail-inset (the
     pane's side padding, which the bar runs across) and --detail-bottom (what covers the foot of the screen). */
  .detail{display:flex;flex-direction:column;gap:16px;min-width:0;min-height:100%;box-sizing:border-box;font-size:14px;line-height:1.5}
  p{margin:0;overflow-wrap:anywhere}
  h2{font:500 1.35rem var(--serif);margin:0;overflow-wrap:anywhere;min-width:0}
  .eyebrow,.meta,.hint,.why{font-size:0.875rem;color:var(--muted)}
  .warning{color:var(--bad)}
  .back-bar{display:flex;align-items:center;gap:8px;padding:8px 0;border-bottom:1px solid var(--line)}
  .back-bar .place{margin-left:auto;color:var(--muted);font-size:0.875rem}
  .back-bar .btn{min-height:44px}.back-bar .step{min-width:44px;padding:0 12px}
  .head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;flex-wrap:wrap}
  .title{display:grid;gap:2px;min-width:0;flex:1 1 280px}
  .name{display:flex;align-items:center;flex-wrap:wrap;gap:6px 12px;min-width:0}
  .live{color:var(--accent-text);font-weight:600}
  .chip{padding:2px 10px;border-radius:999px;border:1px solid var(--line-strong);font-size:0.875rem;font-weight:600;white-space:nowrap}
  .chip.you{border-color:var(--accent);color:var(--accent-text)}.chip.good{border-color:var(--good);color:var(--good)}
  .head-actions{display:flex;align-items:center;gap:8px}
  .head-actions .btn{min-height:44px;text-decoration:none}
  .menu{position:relative}
  .menu summary{list-style:none;min-width:44px;font-weight:600}.menu summary::-webkit-details-marker{display:none}
  .menu ul{position:absolute;right:0;top:calc(100% + 4px);z-index:6;list-style:none;margin:0;padding:6px;min-width:220px;display:grid;gap:2px;border:1px solid var(--line);border-radius:10px;background:var(--card);box-shadow:var(--shadow)}
  .menu ul button{width:100%;min-height:40px;padding:0 12px;border:0;border-radius:6px;background:none;color:var(--ink);font:inherit;text-align:left;cursor:pointer}
  .menu ul button:hover:not(:disabled){background:var(--line-soft)}.menu ul button:disabled{opacity:.5;cursor:default}
  .menu ul button.danger{color:var(--bad)}
  .waits{display:grid;gap:6px;padding:12px 16px;border-radius:10px;background:var(--tint);border:1px solid var(--accent)}
  .waits-head{font-weight:600}
  .advice{overflow-wrap:anywhere}.said{color:var(--muted);overflow-wrap:anywhere}
  .note{color:var(--ink)}
  h2:focus{outline:none}h2:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .stage{display:grid;gap:10px;padding:12px 16px;border:1px solid var(--line);border-radius:10px;background:var(--card);min-width:0}
  .github{display:grid;gap:10px;padding:12px 16px;border:1px solid var(--line);border-radius:10px;background:var(--card);min-width:0}
  .github a{color:var(--accent-text);font-weight:600}
  .ci{border-top:1px solid var(--line);padding-top:8px}
  .ci summary{display:flex;align-items:center;gap:8px;min-height:32px;cursor:pointer;font-weight:600}
  .ci summary::-webkit-details-marker{display:none}.ci summary{list-style:none}
  .ci summary::before{content:"›";display:inline-block;width:12px;color:var(--muted);transition:transform .15s}
  .ci[open] summary::before{transform:rotate(90deg)}
  .ci ul{list-style:none;margin:4px 0 0;padding:0;display:grid;gap:4px;max-height:260px;overflow:auto}
  .ci li{display:flex;align-items:baseline;gap:10px;min-width:0;padding:4px 0;border-top:1px solid var(--line-soft)}
  .ci li:first-child{border-top:0}
  .ci li a,.ci li>span:last-child{min-width:0;overflow-wrap:anywhere}
  .ci-state{flex:none;min-width:6.5rem;font-size:0.875rem;font-weight:600;color:var(--muted)}
  .ci-state[data-tone="good"]{color:var(--good)}.ci-state[data-tone="warn"]{color:var(--warn)}.ci-state[data-tone="bad"]{color:var(--bad)}
  .stage-head{display:flex;flex-wrap:wrap;align-items:baseline;gap:4px 10px}
  .handover{color:var(--muted);font-size:0.875rem}.handover.low{color:var(--warn);font-weight:600}
  .report{border-left:2px solid var(--line-strong);padding-left:12px;display:grid;gap:4px;justify-items:start}
  .prose-clamp{max-height:6.2em;overflow:hidden;min-width:0;max-width:100%}.prose-clamp.open{max-height:none}.report-clamp{max-height:9em}
  .facts{display:flex;flex-wrap:wrap;gap:8px 24px;margin:0}.facts div{display:grid;gap:0;min-width:0}
  .facts dt{font-size:0.875rem;color:var(--muted)}.facts dd{margin:0;font-variant-numeric:tabular-nums;overflow-wrap:anywhere}
  .facts dd.low{color:var(--warn);font-weight:600}.facts dd .yolo{color:var(--warn);font-weight:600}.split{color:var(--muted)}
  .totals{padding-top:10px;border-top:1px solid var(--line)}
  .rows{border:1px solid var(--line);border-radius:10px;background:var(--card);min-width:0}
  .row+.row{border-top:1px solid var(--line)}
  .row summary{display:flex;align-items:center;flex-wrap:wrap;gap:4px 12px;min-height:44px;padding:4px 16px;cursor:pointer}
  .row summary::before{content:"›";display:inline-block;width:12px;color:var(--muted);transition:transform .15s}
  .row[open] summary::before{transform:rotate(90deg)}
  .row summary::-webkit-details-marker{display:none}.row summary{list-style:none}
  .row .headline{margin-left:auto;color:var(--muted);font-size:0.875rem;overflow-wrap:anywhere}
  .row summary:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}
  .body{display:grid;gap:10px;padding:4px 16px 14px;min-width:0}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.actions .btn{min-height:44px}
  .proposals{list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:12px}.proposals li+li{border-top:1px solid var(--line)}
  .proposals label{display:flex;align-items:flex-start;gap:12px;padding:10px 14px;min-height:44px;cursor:pointer}
  .proposals input{width:20px;height:20px;margin:2px 0 0;flex:none;accent-color:var(--accent)}
  .proposals label>span,.proposals .proposal{display:grid;gap:2px;min-width:0;overflow-wrap:anywhere}.proposals .proposal{padding:10px 14px}
  .taken{list-style:none;margin:0;padding:0;display:grid;gap:8px}
  .taken li{display:grid;gap:4px;padding:8px 10px;border-radius:8px;background:var(--bg);border:1px solid var(--line);overflow-wrap:anywhere}.taken li.failed{border-style:dashed}
  .taken-head{display:flex;flex-wrap:wrap;gap:4px 10px;color:var(--muted)}.taken .about,.taken .said{color:var(--muted)}
  .history{list-style:none;margin:0;padding:0;display:grid;gap:6px;max-height:320px;overflow:auto}
  .history li{display:grid;grid-template-columns:minmax(0,11rem) minmax(0,1fr);gap:12px;font-size:.9375rem}.history time{color:var(--muted)}
  .history .why{display:block}
  .setting{display:grid;gap:6px}.setting-name{font-size:0.875rem;color:var(--muted)}
  .who{display:inline-flex;justify-self:start;border:1px solid var(--line);border-radius:10px;overflow:hidden;background:var(--card)}
  .who label{display:flex;align-items:center;gap:8px;min-height:44px;padding:0 14px;font-weight:600;cursor:pointer;color:var(--muted)}
  .who label+label{border-left:1px solid var(--line)}.who label.on{color:var(--ink);background:var(--tint)}.who input{accent-color:var(--accent)}
  .inline{border:0;background:none;padding:0 2px;font:inherit;font-weight:600;color:var(--accent-text);cursor:pointer;min-height:24px;text-decoration:underline}
  .bar{position:sticky;bottom:var(--detail-bottom,0px);z-index:4;display:grid;gap:6px;margin:auto calc(-1 * var(--detail-inset,0px)) 0;padding:12px var(--detail-inset,0px) 16px;border-top:1px solid var(--line);background:var(--card)}
  .ask{font-size:0.875rem;color:var(--muted)}
  .choices{display:flex;flex-wrap:wrap;align-items:flex-end;gap:8px 6px}.choices .btn{min-height:44px;padding-inline:12px;text-decoration:none;display:inline-flex;align-items:center;justify-content:center}
  .lead{display:grid;gap:2px;min-width:0}
  .rec{font-size:0.875rem;font-weight:600;color:var(--accent-text);line-height:1.3}
  .sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
  a:focus-visible,button:focus-visible,summary:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:560px){.choices .btn,.choices .lead{flex:1 1 auto}.history li{grid-template-columns:minmax(0,1fr);gap:0}.row .headline{margin-left:24px;flex-basis:100%}}
  @media(pointer:coarse){.inline{min-height:44px}.menu ul button{min-height:44px}}
</style>
