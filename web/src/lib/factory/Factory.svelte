<script lang="ts">
  // The Factory: every outcome Gaffer owns, drawn as production lines. Each pipeline shows the stages it runs through,
  // which agent and model does the stage in hand, and the numbers that say how it is going. Reading the page starts
  // nothing; the only things that act are the buttons, and each says what it does.
  import { onMount, tick } from 'svelte'
  import { addGafferPipelines, approveGafferMerge, controlGafferGoal, getGafferGoal, listGafferOutcomeHistory, type Event as ActivityEvent, moveGafferPipelineOn, sendGafferPipelineBack, setGafferPipelineAutoMerge, tellGafferPipelineAgent, getGafferSettings, getProfile, archiveGafferOutcome, restoreGafferOutcome, deleteGafferOutcome, listGafferGoals, listGafferOutcomes, retryGafferPipeline, reviewGafferOutcome, stopGafferOutcome, stopFollowingGafferOutcome, type GafferGoalOverview, type GafferOutcome as Outcome, type GafferOutcomeSession } from '../api'
  import { chatToolName, listFolderProjects, terminalMachines } from '../terminal/api'
  import { GAFFER_GOALS_CHANGED } from '../gafferContext'
  import { DEFAULT_STAGES, agentActivity, ago, needsYou, pipelineAnchor, type NeedChoice, agentTokensText, contextLeft, factoryTally, outcomeFinished, outcomeAgentUsage, outcomeUsage, pipelineAgentUsage, pipelineStatus, pipelineUsage, stageLabel, timeLeft, tokensText, track } from '../gafferOutcome'
  import { eventLabel } from '../events'
  import { sourceLabel } from '../clineSource'
  import GafferProse from '../GafferProse.svelte'
  import { formatWhen } from '../time'
  import GafferCompanionAvatar from '../GafferCompanionAvatar.svelte'
  import GafferOutcome from '../GafferOutcome.svelte'
  import GafferFollowedSessions from '../GafferFollowedSessions.svelte'
  import GafferGoal from '../GafferGoal.svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import AddPipelines, { type AddedPipeline } from './AddPipelines.svelte'
  import PipelineNote from './PipelineNote.svelte'
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'

  let { userId = 0, timeZone = '', dateFormat = 'locale' }: { userId?: number; timeZone?: string; dateFormat?: string } = $props()
  let records = $state<Outcome[]>([]), goals = $state<GafferGoalOverview[]>([]), loading = $state(true), error = $state(''), readAt = $state(0)
  // What a new installation still needs before a pipeline can run. Each is read once and again after an outcome dialog closes.
  let setup = $state<{ provider: boolean; machine: boolean; project: boolean } | null>(null)
  let outcomeOpen = $state(false), outcomeButton = $state<HTMLButtonElement>()
  let merging = $state<{ record: Outcome; piece: GafferOutcomeSession } | null>(null), managing = $state<GafferGoalOverview | null>(null)
  let busy = $state(false), notice = $state(''), floorKey = $state(0)
  // Asking Gaffer for a report on an outcome, and approving pipelines it proposed. `picked` holds the ticked proposals per outcome.
  let reporting = $state(''), picked = $state<Record<string, string[]>>({}), adding = $state<Outcome | null>(null), clock = $state(Date.now())
  let controller: AbortController | undefined, epoch = 0

  const byConversation = $derived(new Map(goals.map(row => [row.goal.conversationId, row])))
  const attention = $derived(new Set(goals.filter(row => row.attention).map(row => row.goal.conversationId)))
  // Outcomes put away leave the floor and the tallies; they stay in the record and can be shown and restored.
  const floor = $derived(records.filter(record => !record.archivedAt))
  const shelved = $derived(records.filter(record => record.archivedAt))
  let showArchived = $state(false), removeMode = $state<'archive' | 'delete'>('archive'), deleting = $state<Outcome | null>(null), releasing = $state<Outcome | null>(null)
  const listed = $derived(showArchived ? records : floor)
  const tally = $derived(factoryTally(floor, attention))
  // Goals the page draws inside a pipeline are left out of the list of other sessions below.
  const drawn = $derived(records.flatMap(record => record.sessions.flatMap(piece => [piece.conversationId, ...(piece.stages ?? []).map(stage => stage.conversationId)])).filter((id): id is string => !!id))
  // A pipeline's own sessions are not offered again below, except one that was started and is not followed.
  const drawnSessions = $derived(records.flatMap(record => record.sessions.filter(piece => piece.state !== 'unfollowed').flatMap(piece => [piece.sessionId, ...(piece.stages ?? []).map(stage => stage.sessionId)])).filter((id): id is string => !!id))
  const ready = $derived(!!setup && setup.provider && setup.machine && setup.project)
  const live = $derived(records.some(record => record.state === 'starting'))

  async function load() {
    controller?.abort(); controller = new AbortController()
    const current = controller, mine = ++epoch
    try {
      const [owned, view] = await Promise.all([listGafferOutcomes(current.signal), listGafferGoals(current.signal)])
      if (mine !== epoch) return
      records = owned; goals = view.goals; readAt = view.readAt; error = ''
    } catch (e) { if (mine === epoch && !current.signal.aborted) error = e instanceof Error ? e.message : 'The factory could not be read.' }
    finally { if (mine === epoch) loading = false }
  }
  async function loadSetup() {
    try {
      const [settings, machines, projects] = await Promise.all([getGafferSettings(), terminalMachines().catch(() => []), listFolderProjects().catch(() => [])])
      setup = { provider: settings.available && settings.providers.some(p => p.isDefault && p.enabled), machine: machines.some(m => m.ready), project: projects.length > 0 }
    } catch { setup = null }
  }
  onMount(() => {
    void load(); void loadSetup()
    void getProfile().then(p => { removeMode = p.profile.factoryRemoveMode === 'delete' ? 'delete' : 'archive' }).catch(() => { /* archiving is the safe default */ })
    // A page opened to give Gaffer an outcome (from Home or the chat) starts with that dialog up.
    if (new URLSearchParams(location.search).get('outcome') === '1') outcomeOpen = true
    const timer = setInterval(() => { clock = Date.now(); if (!document.hidden && !outcomeOpen && !managing && !merging && !adding && !reporting) void load() }, 5000)
    const changed = () => void load()
    window.addEventListener(GAFFER_GOALS_CHANGED, changed)
    return () => { epoch++; controller?.abort(); clearInterval(timer); window.removeEventListener(GAFFER_GOALS_CHANGED, changed) }
  })
  async function closeOutcome() {
    outcomeOpen = false; floorKey++
    if (location.search) history.replaceState(null, '', location.pathname)
    await Promise.all([load(), loadSetup()]); await tick(); outcomeButton?.focus({ preventScroll: true })
  }
  async function approve() {
    if (!merging || busy) return
    const { record, piece } = merging
    busy = true; notice = ''
    try {
      const fresh = records.find(item => item.id === record.id) ?? record
      await approveGafferMerge(fresh, piece.key)
      notice = `Merge approved for “${piece.title}”. Its agent merges once every check passes, no review thread is open and there are no conflicts.`
    } catch (e) { notice = e instanceof Error ? e.message : 'The merge was not approved.' }
    busy = false; merging = null
    await load()
  }
  async function stop(record: Outcome) {
    if (busy) return
    busy = true; notice = ''
    try { await stopGafferOutcome(record); notice = 'Stopped. Sessions not yet started stay unstarted; the ones already started keep running.' }
    catch (e) { notice = e instanceof Error ? e.message : 'The outcome was not stopped.' }
    busy = false
    await load()
  }
  // Ends an outcome that is not finished (a session was lost or removed, or the work is no longer wanted) so it can be archived or deleted.
  async function stopFollowing(record: Outcome) {
    if (busy) return
    busy = true; notice = ''; releasing = null
    try { await stopFollowingGafferOutcome(record); notice = `Stopped following “${record.objective}”. Sessions that still exist keep running. You can now archive or delete it.` }
    catch (e) { notice = e instanceof Error ? e.message : 'The outcome was not stopped.' }
    busy = false
    await load()
  }
  // A finished outcome is one line until it is opened, so a long list of old work does not fill the screen. The person's own
  // choice for an outcome, or for all of them, wins; one that is still moving, or has a pipeline to try again, starts open.
  let outcomeFolds = $state<Record<string, boolean>>({})
  const outcomeShown = (record: Outcome) => outcomeFolds[record.id] ?? allOpen ?? (!outcomeFinished(record) || record.sessions.some(piece => ['failed', 'unfollowed'].includes(piece.state)))
  async function putAway(record: Outcome, mode: 'archive' | 'delete' | 'restore') {
    if (busy) return
    busy = true; notice = ''; deleting = null
    try {
      const current = latest(record)
      if (mode === 'delete') { await deleteGafferOutcome(current); notice = `Deleted “${record.objective}”. Its sessions and pull requests were not touched.` }
      else if (mode === 'archive') { await archiveGafferOutcome(current); notice = `Archived “${record.objective}”. Show archived outcomes to bring it back.` }
      else { await restoreGafferOutcome(current); notice = `Restored “${record.objective}”.` }
    } catch (e) { notice = e instanceof Error ? e.message : 'That outcome was not changed.' }
    busy = false
    await load()
  }
  const latest = (record: Outcome) => records.find(item => item.id === record.id) ?? record
  // A review of Gaffer's own that did not complete is tried again by resuming the goal under its current permissions.
  async function reviewAgain(piece: GafferOutcomeSession) {
    if (busy || !piece.conversationId) return
    busy = true; notice = ''
    try {
      const view = await getGafferGoal(piece.conversationId)
      if (!view.goal) throw new Error('That goal is no longer there.')
      if (!view.allowed) throw new Error(view.reason || 'Review this goal’s permissions before trying again.')
      await controlGafferGoal(piece.conversationId, view.goal.revision, 'resume', view.pins, true)
      notice = `Gaffer is reviewing “${piece.title}” again.`
    } catch (e) { notice = e instanceof Error ? e.message : 'The review was not tried again.' }
    busy = false
    await load()
  }
  // What happened to an outcome's pipelines, read from the activity log when asked for.
  let timeline = $state<Record<string, { events: ActivityEvent[]; error: string; loading: boolean }>>({})
  async function toggleHistory(record: Outcome) {
    if (timeline[record.id]) { const { [record.id]: _, ...rest } = timeline; timeline = rest; return }
    timeline = { ...timeline, [record.id]: { events: [], error: '', loading: true } }
    try { timeline = { ...timeline, [record.id]: { events: await listGafferOutcomeHistory(record.id), error: '', loading: false } } }
    catch (e) { timeline = { ...timeline, [record.id]: { events: [], error: e instanceof Error ? e.message : 'The history could not be read.', loading: false } } }
  }
  // A pipeline is one line until it is opened. The ones that want the person, or have stopped, start open; the person's
  // own choice for a pipeline, or for all of them, wins after that.
  let folds = $state<Record<string, boolean>>({}), allOpen = $state<boolean | null>(null)
  const shown = (id: string, tone: string) => folds[id] ?? allOpen ?? (tone === 'you' || tone === 'bad' || arrived === id)
  // One choice from a card that needs the person. Each does what its button says and nothing else.
  let expanded = $state<Record<string, boolean>>({})
  function choose(choice: NeedChoice, record: Outcome, piece: GafferOutcomeSession, goal: GafferGoalOverview) {
    if (choice.id === 'answer' || choice.id === 'open') { if (piece.sessionId) location.href = sessionHref(record, piece.sessionId) }
    else if (choice.id === 'retry' || choice.id === 'again') void reviewAgain(piece)
    else if (choice.id === 'tell') telling = { record, piece }
    else if (choice.id === 'move') moving = { record, piece }
    else if (choice.id === 'budget') managing = goal
  }
  // A link from Home or elsewhere names one pipeline: bring it into view and mark it once the page has it.
  let arrived = $state(''), sought = false
  $effect(() => {
    if (sought || loading || !records.length) return
    sought = true
    const id = decodeURIComponent(location.hash.slice(1))
    if (!id.startsWith('pipeline-')) return
    const record = records.find(item => id.startsWith(`pipeline-${item.id}-`))
    if (record) reveal(record, id)
  })
  // The headline numbers that want the person are buttons: they go to the first card of that kind.
  function jump(kind: 'you' | 'ready') {
    for (const record of records) for (const piece of record.sessions) {
      const goal = piece.conversationId ? byConversation.get(piece.conversationId) : undefined
      const label = pipelineStatus(piece, !!goal?.attention).label
      if ((kind === 'you' && label === 'Needs you') || (kind === 'ready' && label === 'Ready to merge')) {
        reveal(record, pipelineAnchor(record.id, piece.key))
        return
      }
    }
  }
  // A link or a number names one pipeline: open its outcome if it is folded, then bring it into view and mark it.
  function reveal(record: Outcome, id: string) {
    outcomeFolds = { ...outcomeFolds, [record.id]: true }
    void tick().then(() => { const card = document.getElementById(id); if (card) { arrived = id; card.scrollIntoView({ block: 'center' }); card.focus({ preventScroll: true }) } })
  }
  const pipelineOf = (event: ActivityEvent) => { try { return String(JSON.parse(event.detail || '{}').pipeline ?? '') } catch { return '' } }
  // The person's own controls over a pipeline: move it on from the stage in hand, and how its merge is decided.
  let moving = $state<{ record: Outcome; piece: GafferOutcomeSession } | null>(null)
  async function moveOn() {
    if (!moving || busy) return
    const { record, piece } = moving
    busy = true; notice = ''; moving = null
    try { await moveGafferPipelineOn(record, piece.key); notice = `Moved “${piece.title}” on. The controller is starting its next stage.` }
    catch (e) { notice = e instanceof Error ? e.message : 'That pipeline was not moved on.' }
    busy = false
    await load()
  }
  // Telling the working agent what to do next, and sending a pipeline back to a stage to be done again.
  let telling = $state<{ record: Outcome; piece: GafferOutcomeSession } | null>(null), sending = $state<{ record: Outcome; piece: GafferOutcomeSession } | null>(null)
  const requestId = () => Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('')
  async function tell(text: string) {
    if (!telling || busy) return
    const { record, piece } = telling
    busy = true; notice = ''
    try {
      const sent = await tellGafferPipelineAgent(record, piece.key, requestId(), text)
      notice = sent.message.state === 'sent' || sent.message.state === 'queued' || sent.message.state === 'sending' ? `Your instruction is with the agent of “${piece.title}” for its next turn.` : `The instruction for “${piece.title}” was not delivered: ${sent.message.note || sent.message.state}.`
      telling = null
    } catch (e) { notice = e instanceof Error ? e.message : 'The instruction was not sent.' }
    busy = false
    await load()
  }
  async function sendBack(reason: string, stage: string) {
    if (!sending || busy) return
    const { record, piece } = sending
    busy = true; notice = ''
    try { await sendGafferPipelineBack(record, piece.key, stage, reason); notice = `Sent “${piece.title}” back to ${stageLabel(stage).toLowerCase()}. The controller is starting that stage again.`; sending = null }
    catch (e) { notice = e instanceof Error ? e.message : 'That pipeline was not sent back.' }
    busy = false
    await load()
  }
  // A YOLO pipeline acts without approvals, so letting it merge by itself as well is asked for separately.
  let yoloMerge = $state<{ record: Outcome; piece: GafferOutcomeSession } | null>(null)
  const mergesByItself = (record: Outcome, piece: GafferOutcomeSession) => !!piece.autoMerge && (!record.yolo || !!piece.yoloMergeOk)
  async function mergeMode(record: Outcome, piece: GafferOutcomeSession, on: boolean, yoloConfirmed = false) {
    if (busy) return
    if (on && record.yolo && !yoloConfirmed) { yoloMerge = { record, piece }; return }
    yoloMerge = null
    busy = true; notice = ''
    try { await setGafferPipelineAutoMerge(record, piece.key, on, yoloConfirmed); notice = on ? `“${piece.title}” will merge by itself once every check passes.` : `“${piece.title}” will stop at Ready to merge and wait for you.` }
    catch (e) { notice = e instanceof Error ? e.message : 'The merge setting was not changed.' }
    busy = false
    await load()
  }
  async function retry(record: Outcome, piece: GafferOutcomeSession) {
    if (busy) return
    busy = true; notice = ''
    try { await retryGafferPipeline(latest(record), piece.key); notice = `Trying “${piece.title}” again.` }
    catch (e) { notice = e instanceof Error ? e.message : 'That pipeline was not tried again.' }
    busy = false
    await load()
  }
  async function report(record: Outcome) {
    if (busy || reporting) return
    reporting = record.id; notice = ''
    try {
      const next = await reviewGafferOutcome(latest(record))
      records = records.map(item => item.id === next.id ? next : item)
      // Every proposal starts ticked; nothing is added until the person says so.
      picked = { ...picked, [next.id]: (next.report?.proposals ?? []).map(item => item.title) }
    } catch (e) { notice = e instanceof Error ? e.message : 'Gaffer could not report on this outcome.' }
    reporting = ''
  }
  const chosenProposals = (record: Outcome) => (record.report?.proposals ?? []).filter(item => (picked[record.id] ?? []).includes(item.title))
  function pick(record: Outcome, title: string, on: boolean) {
    const now = picked[record.id] ?? []
    picked = { ...picked, [record.id]: on ? [...now, title] : now.filter(item => item !== title) }
  }
  async function add(chosen: AddedPipeline[]) {
    if (!adding || busy) return
    const record = adding
    busy = true; notice = ''; adding = null
    try {
      await addGafferPipelines(latest(record), chosen)
      notice = chosen.length === 1 ? `Added “${chosen[0].title}”. The controller is starting it.` : `Added ${chosen.length} pipelines. The controller is starting them.`
    } catch (e) { notice = e instanceof Error ? e.message : 'The pipelines were not added. Nothing was started.' }
    busy = false
    await load()
  }
  const sessionHref = (record: Outcome, id: string) => `/sessions?agent=${encodeURIComponent(record.agentId)}&session=${encodeURIComponent(id)}`
  const when = (at: number) => formatWhen(at, timeZone, dateFormat)
  const crew = (piece: GafferOutcomeSession, record: Outcome) => {
    const stage = piece.stages?.[piece.stage ?? 0]
    const tool = stage?.tool ?? record.tool, model = stage?.model ?? record.model ?? ''
    // A stage's own empty source is Cline's own choice; the outcome's source is only the fallback when a piece has no stage.
    const source = stage ? stage.provider ?? '' : record.provider ?? ''
    return `${chatToolName(tool)}${model ? ` · ${model}` : ''}${source ? ` · ${sourceLabel(source)}` : ''}${stage?.effort ? ` · ${stage.effort} effort` : ''}`
  }
</script>

<div class="factory">
  <header class="top">
    <div class="identity"><h1>Factory</h1><p class="sub">Hand Gaffer the outcome. It plans the pipelines, you approve, and it runs each one to the finish.</p></div>
    <div class="top-actions"><a class="btn quiet" href="/account/gaffer">Gaffer settings</a><button class="btn primary" type="button" bind:this={outcomeButton} onclick={() => outcomeOpen = true}>Give Gaffer an outcome</button></div>
  </header>

  <ul class="tiles" aria-label="Factory at a glance">
    <li class:you={tally.needYou > 0}>{#if tally.needYou}<button type="button" class="tile-jump" aria-label={`${tally.needYou} need you. Go to the first`} onclick={() => jump('you')}><strong>{tally.needYou}</strong><span>Need you · show me</span></button>{:else}<strong>0</strong><span>Need you</span>{/if}</li>
    <li class:you={tally.ready > 0}>{#if tally.ready}<button type="button" class="tile-jump" aria-label={`${tally.ready} ready to merge. Go to the first`} onclick={() => jump('ready')}><strong>{tally.ready}</strong><span>Ready to merge · show me</span></button>{:else}<strong>0</strong><span>Ready to merge</span>{/if}</li>
    <li><strong>{tally.running}</strong><span>Running</span></li>
    <li><strong>{tally.done}</strong><span>Done</span></li>
  </ul>

  {#if records.length}<div class="fold-all">{#if shelved.length}<button class="btn quiet" type="button" aria-pressed={showArchived} onclick={() => showArchived = !showArchived}>{showArchived ? 'Hide archived' : `Show archived (${shelved.length})`}</button>{/if}<button class="btn quiet" type="button" aria-pressed={allOpen === true} onclick={() => { allOpen = allOpen === true ? false : true; folds = {}; outcomeFolds = {} }}>{allOpen === true ? 'Collapse all' : 'Expand all'}</button></div>{/if}
  {#if notice}<p role="status" class="notice">{notice}</p>{/if}
  {#if error}<p role="alert" class="warning">{error}{#if records.length}{' '}Showing the last successful read.{/if}</p>{/if}

  {#if setup && !ready}<section class="setup" aria-label="Set up your factory">
    <GafferCompanionAvatar size={48}/>
    <div class="setup-body">
      <h2>Three things and you’re in business.</h2>
      <ol>
        <li class:done={setup.machine}><span class="tick" aria-hidden="true">{setup.machine ? '✓' : '1'}</span><div><strong>Connect a machine</strong><p>Agents run on your own machines. {#if !setup.machine}<a href="/machines">Add a machine</a>{:else}Connected.{/if}</p></div></li>
        <li class:done={setup.project}><span class="tick" aria-hidden="true">{setup.project ? '✓' : '2'}</span><div><strong>Add a project</strong><p>A folder on that machine for the work, ideally a git repository so each pipeline gets its own worktree. {#if !setup.project}<a href="/">Add a project on Home</a>{:else}Added.{/if}</p></div></li>
        <li class:done={setup.provider}><span class="tick" aria-hidden="true">{setup.provider ? '✓' : '3'}</span><div><strong>Choose Gaffer’s brains</strong><p>Gaffer needs its own model to plan and to follow the work. Pick one that supports tools and make it the default. {#if !setup.provider}<a href="/account/gaffer">Open Gaffer settings</a>{:else}Chosen.{/if}</p></div></li>
      </ol>
      <p class="hint">Looking after sessions is switched on by default once Gaffer has a provider, and pipelines stop at <strong>Ready to merge</strong> unless you allow auto-merge. Both are in Gaffer settings.</p>
    </div>
  </section>{/if}

  <section class="lines" aria-label="Pipelines">
    {#if loading}<p role="status">Reading the factory…</p>
    {:else if !listed.length && shelved.length}<div class="empty"><h2>Nothing on the floor.</h2><p>Every outcome is archived. Show archived to look at them or bring one back.</p></div>
    {:else if !records.length}<div class="empty"><h2>No pipelines yet.</h2><p>Give Gaffer an outcome and its pipelines appear here, each moving through plan, design, code, review, test, pull request, babysitting and merge.</p></div>
    {:else}
      {#each listed as record (record.id)}{@const open_ = outcomeShown(record)}{@const finished = outcomeFinished(record)}{@const usage = outcomeUsage(record.sessions.map(piece => pipelineUsage(piece, piece.conversationId ? byConversation.get(piece.conversationId)?.goal : undefined)), record.report)}<article class="outcome" class:shelved={!!record.archivedAt} aria-label={`Outcome: ${record.objective}`}>
        <header class="outcome-head">
          <div class="outcome-title"><p class="eyebrow">Outcome</p><h2>{record.objective}</h2><p class="meta">{record.projectName || 'Project'} · started {when(record.createdAt)}{#if record.state === 'starting'} · <span class="live">the controller is starting sessions</span>{:else if record.state === 'stopped'} · stopped{/if}{#if record.archivedAt} · archived {when(record.archivedAt)}{/if}{#if !open_} · {record.sessions.length} {record.sessions.length === 1 ? 'pipeline' : 'pipelines'}, {record.sessions.every(piece => piece.state === 'done') ? 'all done' : finished ? 'finished' : 'in progress'}{/if}</p></div>
          <div class="outcome-actions">
            <button class="btn quiet" type="button" aria-expanded={open_} onclick={() => outcomeFolds = { ...outcomeFolds, [record.id]: !open_ }}>{open_ ? 'Hide details' : 'Show details'}</button>
            {#if open_}
              <button class="btn quiet" type="button" disabled={busy || !!reporting} onclick={() => void report(record)}>{reporting === record.id ? 'Gaffer is reporting…' : record.report ? 'Ask Gaffer again' : 'Ask Gaffer for a report'}</button>
              <button class="btn quiet" type="button" aria-expanded={!!timeline[record.id]} onclick={() => void toggleHistory(record)}>{timeline[record.id] ? 'Hide history' : 'History'}</button>
            {/if}
            {#if record.state === 'starting'}<button class="btn quiet" type="button" disabled={busy} onclick={() => void stop(record)}>Stop starting sessions</button>{/if}
            {#if !finished}<button class="btn quiet" type="button" disabled={busy} onclick={() => releasing = record}>Stop following</button>{/if}
            {#if finished}
              {#if record.archivedAt}<button class="btn quiet" type="button" disabled={busy} onclick={() => void putAway(record, 'restore')}>Restore</button>{:else}<button class={removeMode === 'archive' ? 'btn' : 'btn quiet'} type="button" disabled={busy} onclick={() => void putAway(record, 'archive')}>Archive</button>{/if}
              <button class={removeMode === 'delete' && !record.archivedAt ? 'btn danger' : 'btn quiet'} type="button" disabled={busy} onclick={() => deleting = record}>Delete</button>
            {/if}
          </div>
        </header>
        {#if open_}
        <dl class="facts totals" aria-label="This outcome so far">
          <div><dt>Pipelines</dt><dd>{record.sessions.length}</dd></div>
          <div><dt>Gaffer reviews</dt><dd>{usage.reviews}</dd></div>
          <div><dt>Gaffer tokens</dt><dd>{tokensText(usage)}</dd></div>
          {#if true}{@const agents = outcomeAgentUsage(record.sessions)}<div><dt>Agent tokens</dt><dd>{agentTokensText(agents.total)}{#if agents.byAgent.length > 1}{' '}<span class="split">({agents.byAgent.map(item => `${chatToolName(item.tool)} ${agentTokensText({ tokens: item.tokens, cached: 0, reported: true })}`).join(', ')})</span>{/if}</dd></div>{/if}
          <div><dt>Budget per stage</dt><dd>{record.maxReviews} reviews · {record.hours} h</dd></div>
          <div><dt>Approvals</dt><dd class:yolo={record.yolo}>{record.yolo ? 'YOLO' : 'Ask me'}</dd></div>
          <div><dt>Reports</dt><dd>{record.autoReport ? 'By itself and on request' : 'On request'}</dd></div>
        </dl>
        {#if timeline[record.id]}{@const log = timeline[record.id]}<section class="outcome-history" aria-label="History of this outcome">
          <p class="eyebrow">History · newest first</p>
          {#if log.loading}<p role="status">Reading the history…</p>
          {:else if log.error}<p role="alert" class="warning">{log.error}</p>
          {:else if !log.events.length}<p class="hint">Nothing recorded yet. Moves and stops are recorded from the controller version that added this history.</p>
          {:else}<ol class="history">{#each log.events as event (event.id)}{@const title = record.sessions.length > 1 ? record.sessions.find(piece => piece.key === pipelineOf(event))?.title : ''}<li><time datetime={new Date(event.at).toISOString()}>{when(event.at)}</time><span>{eventLabel(event)}{#if title}<span class="why">{title}</span>{/if}</span></li>{/each}</ol>{/if}
        </section>{/if}
        {#if record.report}<section class="outcome-report" aria-label="Gaffer’s report on this outcome">
          <p class="eyebrow">Gaffer’s report · {when(record.report.at)}{record.report.auto ? ' · made by itself when a pipeline finished or stopped' : ''}</p>
          <div class="prose-clamp report-clamp" class:open={expanded[`report-${record.id}`]}><GafferProse text={record.report.text}/></div>
          {#if record.report.text.length > 400}<button class="inline" type="button" aria-expanded={!!expanded[`report-${record.id}`]} onclick={() => expanded = { ...expanded, [`report-${record.id}`]: !expanded[`report-${record.id}`] }}>{expanded[`report-${record.id}`] ? 'Show less' : 'Read all of it'}</button>{/if}
          {#if record.report.proposals?.length}
            <p class="hint">Gaffer suggests adding {record.report.proposals.length === 1 ? 'this pipeline' : 'these pipelines'}. Nothing is added or started unless you approve it, and you choose each one’s agent, model and effort then.</p>
            <ul class="proposals" aria-label="Proposed pipelines">
              {#each record.report.proposals as item (item.title)}<li><label><input type="checkbox" checked={(picked[record.id] ?? []).includes(item.title)} onchange={event => pick(record, item.title, event.currentTarget.checked)}/><span><strong>{item.title}</strong>{#if item.why}<span class="why">{item.why}</span>{/if}<span class="why">{(item.stages?.length ? item.stages : DEFAULT_STAGES).map(stageLabel).join(' → ')}</span></span></label></li>{/each}
            </ul>
            <div class="actions"><button class="btn primary" type="button" disabled={busy || !chosenProposals(record).length} onclick={() => adding = record}>{chosenProposals(record).length === 1 ? 'Add 1 pipeline' : `Add ${chosenProposals(record).length} pipelines`}</button></div>
          {/if}
        </section>{/if}
        <ul class="pipes" aria-label="Pipelines of this outcome">
          {#each record.sessions as piece (piece.key)}{@const goal = piece.conversationId ? byConversation.get(piece.conversationId) : undefined}{@const status = pipelineStatus(piece, !!goal?.attention)}{@const nodes = track(piece)}{@const id = pipelineAnchor(record.id, piece.key)}{@const open = shown(id, status.tone)}<li class="pipe" id={pipelineAnchor(record.id, piece.key)} tabindex="-1" class:arrived={arrived === pipelineAnchor(record.id, piece.key)} class:you={status.tone === 'you'} class:bad={status.tone === 'bad'}>
            <div class="pipe-head"><h3>{piece.title}</h3>{#if nodes.length}<span class="where">{stageLabel(nodes[Math.min(piece.stage ?? 0, nodes.length - 1)].kind)} · {Math.min((piece.stage ?? 0) + 1, nodes.length)} of {nodes.length}</span>{/if}<span class="badge {status.tone}">{status.label}</span>
              <button class="fold" type="button" aria-expanded={open} aria-label={`${open ? 'Hide' : 'Show'} details of ${piece.title}`} onclick={() => folds = { ...folds, [id]: !open }}>{open ? 'Hide details' : 'Show details'}</button></div>
            {#if !open}<p class="now brief">{status.label === 'Needs you' && goal ? (goal.reason || goal.goal.note) : piece.note}</p>{:else}
            {#if nodes.length}<ol class="track" aria-label={`Stages of ${piece.title}`}>
              {#each nodes as node, index (node.kind)}{@const look = node.state === 'running' && status.label === 'Needs you' ? 'waiting' : node.state}<li class={look} aria-current={look === 'running' || look === 'waiting' ? 'step' : undefined} title={`${node.label}: ${chatToolName(node.tool)}${node.model ? ` · ${node.model}` : ''}${node.source ? ` · ${sourceLabel(node.source)}` : ''}${node.moved ? ' · moved on by you' : ''}${node.summary ? ` — ${node.summary}` : ''}`}>
                <span class="dot" class:moved={node.moved} aria-hidden="true">{node.moved ? '»' : look === 'done' ? '✓' : look === 'waiting' ? '!' : look === 'failed' ? '×' : index + 1}</span><span class="name">{node.label}</span><span class="sr-only">{node.moved ? 'moved on by you, not handed over by Gaffer' : look === 'done' ? 'done' : look === 'running' ? 'running now' : look === 'waiting' ? 'waiting for you' : look === 'failed' ? 'stopped' : 'to come'}</span>
              </li>{/each}
            </ol>{/if}
            {#if status.label === 'Needs you' && goal}{@const need = needsYou(goal.goal, goal.reason, (piece.stage ?? 0) >= nodes.length - 1)}<section class="decide" aria-label={`What ${piece.title} needs from you`}>
              <p class="decide-why">{need.why}</p>
              {#if goal.goal.summary}{@const whole = expanded[piece.conversationId ?? '']}<div class="report"><p class="eyebrow">Gaffer’s latest review</p><div class="prose-clamp" class:open={whole}><GafferProse text={goal.goal.summary}/></div>{#if goal.goal.summary.length > 280}<button class="inline" type="button" aria-expanded={!!whole} onclick={() => expanded = { ...expanded, [piece.conversationId ?? '']: !whole }}>{whole ? 'Show less' : 'Read all of it'}</button>{/if}</div>{/if}
              <p class="decide-ask">What do you want to do?</p>
              <div class="decide-choices">{#each need.choices.filter(choice => (choice.id !== 'move' || (piece.state === 'following' && nodes.length && piece.conversationId)) && (choice.id !== 'tell' || (piece.sessionId && piece.conversationId)) && ((choice.id !== 'open' && choice.id !== 'answer') || piece.sessionId)) as choice (choice.id)}<button class="btn" class:primary={choice.primary} type="button" disabled={busy} onclick={() => choose(choice, record, piece, goal)}>{choice.label}</button>{/each}</div>
            </section>
            {:else}<p class="now">{piece.note}</p>{/if}
            {#if goal?.goal.summary && status.label !== 'Needs you'}{@const whole = expanded[piece.conversationId ?? '']}<div class="report"><p class="eyebrow">Gaffer’s latest review</p><div class="prose-clamp" class:open={whole}><GafferProse text={goal.goal.summary}/></div>{#if goal.goal.summary.length > 280}<button class="inline" type="button" aria-expanded={!!whole} onclick={() => expanded = { ...expanded, [piece.conversationId ?? '']: !whole }}>{whole ? 'Show less' : 'Read all of it'}</button>{/if}</div>{/if}
            <dl class="facts">
              <div><dt>Doing this stage</dt><dd>{crew(piece, record)}</dd></div>
              {#if piece.state === 'following'}{@const stage = piece.stages?.[piece.stage ?? 0]}{@const activity = agentActivity(stage, clock)}
                {#if stage?.startedAt}<div><dt>In this stage</dt><dd>{ago(stage.startedAt, clock)}</dd></div>{/if}
                {#if activity.text}<div><dt>Agent</dt><dd class:low={activity.quiet}>{activity.text}</dd></div>{/if}{/if}
              {#if nodes.some(node => node.kind === 'merge')}{@const auto = mergesByItself(record, piece)}<div><dt>Merge</dt><dd>{auto ? 'By itself when green' : piece.mergeApproved ? 'You approved it' : record.yolo && piece.autoMerge ? 'Waits for you (YOLO)' : 'Waits for you'}{#if piece.state !== 'done' && !piece.mergeApproved}{' '}<button class="inline" type="button" disabled={busy} aria-label={`${auto ? 'Make' : 'Let'} ${piece.title} ${auto ? 'wait for you before merging' : 'merge by itself'}`} onclick={() => void mergeMode(record, piece, !auto)}>{auto ? 'Make it wait for me' : 'Let it merge by itself'}</button>{/if}</dd></div>{/if}
            </dl>
            <details class="more-facts"><summary>Usage and budget</summary><dl class="facts">
              {#if goal && goal.goal.state === 'active'}<div><dt>Reviews left</dt><dd>{Math.max(0, goal.goal.maxReviews - goal.goal.usedReviews)} of {goal.goal.maxReviews}</dd></div>
                <div><dt>Time left</dt><dd>{timeLeft(goal.goal.expiresAt, clock)}</dd></div>{/if}
              {#if goal?.goal.lastReviewedAt}<div><dt>Last review</dt><dd>{when(goal.goal.lastReviewedAt)}</dd></div>{/if}
              <div><dt>Gaffer tokens</dt><dd>{tokensText(pipelineUsage(piece, goal?.goal))}</dd></div>
              <div><dt>Agent tokens</dt><dd>{agentTokensText(pipelineAgentUsage(piece))}</dd></div>
              {#if piece.state === 'following'}{@const room = contextLeft(piece.stages?.[piece.stage ?? 0])}<div><dt>Context left</dt><dd class:low={room.low}>{room.text || 'Not reported'}</dd></div>{/if}
            </dl></details>
            <div class="actions">
              {#if piece.state === 'ready_to_merge'}<button class="btn primary" type="button" disabled={busy} onclick={() => merging = { record, piece }}>Approve merge</button>{/if}
              {#if status.label === 'Needs you' && goal}
                <!-- The choices above cover opening the session, telling the agent and moving on; what is left is the rest. -->
                <button class="btn quiet" type="button" onclick={() => managing = goal}>Goal and limits</button>
              {:else}
                {#if piece.sessionId}<a class="btn" href={sessionHref(record, piece.sessionId)}>Open session</a>{/if}
                {#if goal}<button class="btn quiet" type="button" onclick={() => managing = goal}>Review goal</button>{/if}
                {#if piece.state === 'following' && nodes.length && piece.conversationId}{@const last = (piece.stage ?? 0) >= nodes.length - 1}<button class="btn" type="button" disabled={busy} onclick={() => moving = { record, piece }}>{last ? 'Finish this stage' : 'Move to next stage'}</button>{/if}
                {#if piece.sessionId && piece.conversationId && ['following', 'unfollowed', 'ready_to_merge'].includes(piece.state)}<button class="btn quiet" type="button" disabled={busy} onclick={() => telling = { record, piece }}>Tell the agent…</button>{/if}
              {/if}
              {#if nodes.length && ['following', 'ready_to_merge', 'done', 'unfollowed'].includes(piece.state)}<button class="btn quiet" type="button" disabled={busy} onclick={() => sending = { record, piece }}>Send back…</button>{/if}
              {#if ['failed', 'unfollowed', 'cancelled'].includes(piece.state)}<button class="btn" type="button" disabled={busy} onclick={() => void retry(record, piece)}>Try again</button>{/if}
            </div>
            {/if}
          </li>{/each}
        </ul>
        {/if}
      </article>{/each}
    {/if}
  </section>

  <section class="floor" aria-label="Other sessions">
    <h2>Other sessions</h2>
    <p class="hint">Sessions you started yourself: the ones Gaffer looks after, and the running chats it does not yet.</p>
    {#key floorKey}<GafferFollowedSessions inline hidden={drawn} hiddenSessions={drawnSessions} {userId} {timeZone} {dateFormat} onerror={e => e instanceof Error ? e.message : 'The sessions could not be read.'} onchanged={load} onselect={() => {}} onhistory={() => {}} onclose={() => {}}/>{/key}
  </section>
  {#if readAt}<p class="foot">Read {when(readAt)}{live ? ', refreshing while sessions start' : ''}. Reading the factory makes no model request and starts nothing.</p>{/if}
</div>

{#if outcomeOpen}<GafferOutcome room={Math.max(0, 10 - goals.filter(row => row.goal.state === 'active').length)} {userId} onstarted={load} onclose={() => void closeOutcome()}/>{/if}
{#if releasing}<ConfirmDialog title={`Stop following “${releasing.objective}”?`} body="Gaffer and the controller stop looking after every unfinished pipeline in this outcome and start nothing more for it. Sessions that still exist keep running, and nothing is merged or closed. Use this when a session was lost or removed, or the work is no longer wanted; you can then archive or delete the outcome." confirmLabel="Stop following" oncancel={() => releasing = null} onconfirm={() => releasing && void stopFollowing(releasing)}/>{/if}
{#if deleting}<ConfirmDialog title={`Delete “${deleting.objective}”?`} body="This erases the outcome, its report and its totals from reet for good. The sessions and pull requests it started are not touched. To keep the record out of sight instead, archive it." confirmLabel="Delete outcome" danger oncancel={() => deleting = null} onconfirm={() => deleting && void putAway(deleting, 'delete')}/>{/if}
{#if adding}<AddPipelines record={adding} proposals={chosenProposals(adding)} {busy} oncancel={() => adding = null} onconfirm={pipelines => void add(pipelines)}/>{/if}
{#if moving}{@const at = moving.piece.stages?.[moving.piece.stage ?? 0]?.kind ?? ''}{@const next = moving.piece.stages?.[(moving.piece.stage ?? 0) + 1]?.kind ?? ''}<ConfirmDialog title={next ? `Move “${moving.piece.title}” on to ${stageLabel(next).toLowerCase()}?` : `Finish the ${stageLabel(at).toLowerCase()} stage of “${moving.piece.title}”?`} body={`This is your call: the ${stageLabel(at).toLowerCase()} stage is closed as it stands, without waiting for Gaffer to find it done${next ? `, and the controller starts the ${stageLabel(next).toLowerCase()} stage with Gaffer’s last review as its handover` : ''}. ${next === 'merge' && !mergesByItself(moving.record, moving.piece) ? 'It then stops at Ready to merge and waits for you.' : next === 'merge' ? 'This pipeline merges by itself once every check passes.' : 'Nothing is merged by this.'}`} confirmLabel={next ? 'Move on' : 'Finish stage'} oncancel={() => moving = null} onconfirm={() => void moveOn()}/>{/if}
{#if telling}<PipelineNote title={`Tell the agent of “${telling.piece.title}”`} intro="Your words go to this pipeline’s working agent for its next turn, exactly as you write them. It cannot interrupt the agent or answer an approval." label="Instruction" placeholder="Write the plan as text now: files, approach, risks. Change no code." confirmLabel="Send to the agent" {busy} oncancel={() => telling = null} onconfirm={text => void tell(text)}/>{/if}
{#if sending}{@const upTo = Math.min(sending.piece.stage ?? 0, (sending.piece.stages?.length ?? 1) - 1)}<PipelineNote title={`Send “${sending.piece.title}” back`} intro="The stage you choose, and every stage after it, is done again in a fresh agent session in the same folder and branch. The agent is told your reason. Nothing already committed is undone." label="Why it is going back" placeholder="The review found the export leaks other tenants’ rows." confirmLabel="Send back" stages={(sending.piece.stages ?? []).slice(0, upTo + 1).map(item => ({ kind: item.kind, label: stageLabel(item.kind) }))} {busy} oncancel={() => sending = null} onconfirm={(text, stage) => void sendBack(text, stage)}/>{/if}
{#if yoloMerge}<ConfirmDialog title={`Let “${yoloMerge.piece.title}” merge by itself on YOLO?`} body="This pipeline’s agent runs on YOLO: it acts without asking you. If it may also merge by itself, nothing stops for you between the code and the merge. It still merges only once every check passes, no review thread is open and there are no conflicts. A merge cannot be undone from here." confirmLabel="Let it merge by itself" oncancel={() => yoloMerge = null} onconfirm={() => { const { record, piece } = yoloMerge!; void mergeMode(record, piece, true, true) }}/>{/if}
{#if merging}<ConfirmDialog title={`Merge “${merging.piece.title}”?`} body="This tells the pipeline’s agent to merge its pull request once every check passes, no review thread is open and there are no conflicts. If any of those is not true it fixes that first or brings it back to you. A merge cannot be undone from here." confirmLabel="Approve merge" oncancel={() => merging = null} onconfirm={() => void approve()}/>{/if}
{#if managing}<WorkspaceDialog title="Look after this session" onclose={() => { managing = null; void load() }}><GafferGoal id={managing.goal.conversationId} sessionId={managing.goal.sessionId} {userId} {timeZone} {dateFormat} onchanged={load}/></WorkspaceDialog>{/if}

<style>
  .factory{display:grid;gap:20px;min-width:0;max-width:1100px;margin:0 auto;padding:4px 0 40px;font-size:14px;line-height:1.5}
  p{margin:0;overflow-wrap:anywhere}h1{font:500 2rem var(--serif);margin:0}h2{font:500 1.35rem var(--serif);margin:0;overflow-wrap:anywhere}h3{font:600 1.05rem var(--sans);margin:0;overflow-wrap:anywhere;min-width:0}
  .sub,.hint,.meta,.foot{color:var(--muted)}.warning{color:var(--bad)}.eyebrow{font-size:0.875rem;color:var(--muted);letter-spacing:.02em}
  .top{display:flex;align-items:flex-end;justify-content:space-between;gap:12px 20px;flex-wrap:wrap}.identity{display:grid;gap:4px;min-width:0;flex:1 1 320px}.top-actions{display:flex;gap:8px;flex-wrap:wrap}.top-actions .btn{min-height:44px;text-decoration:none;display:inline-flex;align-items:center}
  .tiles{list-style:none;margin:0;padding:0;display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px}.tiles li{display:grid;gap:2px;padding:12px 14px;border:1px solid var(--line);border-radius:12px;background:var(--card);min-width:0}.tiles strong{font:500 1.6rem var(--serif);font-variant-numeric:tabular-nums;line-height:1.1}.tiles span{font-size:0.875rem;color:var(--muted)}.tiles li.you{border-color:var(--accent);background:var(--tint)}
  .notice{padding:10px 14px;border:1px solid var(--line);border-radius:10px;background:var(--card)}
  .setup{display:grid;grid-template-columns:auto 1fr;gap:16px;padding:16px;border:1px solid var(--line);border-radius:14px;background:var(--tint)}.setup-body{display:grid;gap:12px;min-width:0}.setup ol{list-style:none;margin:0;padding:0;display:grid;gap:10px}.setup li{display:grid;grid-template-columns:auto 1fr;gap:10px;align-items:start}.setup li p{color:var(--muted)}.tick{display:inline-grid;place-items:center;width:24px;height:24px;border-radius:50%;border:1px solid var(--line-strong);font-size:0.875rem;font-variant-numeric:tabular-nums}.setup li.done .tick{background:var(--accent);border-color:var(--accent);color:var(--accent-ink)}.setup a{color:var(--accent-text);font-weight:600}
  .lines{display:grid;gap:16px;min-width:0}.empty{padding:28px 20px;border:1px dashed var(--line-strong);border-radius:14px;display:grid;gap:8px;text-align:center;justify-items:center}.empty p{color:var(--muted);max-width:56ch}
  .outcome{border:1px solid var(--line);border-radius:14px;background:var(--card);min-width:0;overflow:hidden}.outcome-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;flex-wrap:wrap;padding:16px 18px;border-bottom:1px solid var(--line)}.outcome-title{display:grid;gap:2px;min-width:0;flex:1 1 280px}.live{color:var(--accent-text);font-weight:600}
  .outcome-actions{display:flex;gap:8px;flex-wrap:wrap}.outcome-actions .btn{min-height:44px}.totals{padding:12px 18px;border-bottom:1px solid var(--line);background:var(--bg)}
  .outcome-report{display:grid;gap:10px;padding:14px 18px;border-bottom:1px solid var(--line)}.proposals{list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:12px}.proposals li+li{border-top:1px solid var(--line)}.proposals label{display:flex;align-items:flex-start;gap:12px;padding:10px 14px;min-height:44px;cursor:pointer}.proposals input{width:20px;height:20px;margin:2px 0 0;flex:none;accent-color:var(--accent)}.proposals label>span{display:grid;gap:2px;min-width:0;overflow-wrap:anywhere}.why{font-size:0.875rem;color:var(--muted)}
  .pipes{list-style:none;margin:0;padding:0}.pipe{display:grid;gap:10px;padding:16px 18px;min-width:0}.pipe+.pipe{border-top:1px solid var(--line)}.pipe.you{background:var(--tint)}
  .pipe-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}
  .badge{font-size:0.875rem;font-weight:600;border-radius:999px;padding:2px 10px;border:1px solid var(--line);white-space:nowrap}.badge.you{border-color:var(--accent);color:var(--accent-text);background:var(--card)}.badge.busy{color:var(--ink)}.badge.good{border-color:var(--good,var(--line-strong));color:var(--good,var(--ink))}.badge.bad{border-color:var(--bad);color:var(--bad)}.badge.idle{color:var(--muted)}
  /* The production line: stages joined by a rail. Done stages are filled, the one in hand is ringed, the rest are to come. */
  .track{list-style:none;margin:0;padding:2px 0 0;display:flex;gap:0;min-width:0;overflow-x:auto;scrollbar-width:thin}
  .track li{position:relative;flex:1 1 0;min-width:64px;display:grid;justify-items:center;gap:4px;padding:0 4px 2px;color:var(--muted);font-size:0.875rem;text-align:center}
  .track li+li::before{content:"";position:absolute;top:13px;right:50%;width:100%;height:2px;background:var(--line);z-index:0}
  .track li.done+li::before{background:var(--good)}
  .dot{position:relative;z-index:1;display:inline-grid;place-items:center;width:28px;height:28px;border-radius:50%;border:2px solid var(--line-strong);background:var(--card);font-size:0.875rem;font-weight:600;font-variant-numeric:tabular-nums}
  .track li.done{color:var(--ink)}.track li.done .dot{background:var(--good);border-color:var(--good);color:var(--bg)}.track li.done .dot.moved{background:var(--card);color:var(--good)}
  .outcome-history{padding:12px 16px;border-top:1px solid var(--line)}.history{list-style:none;margin:8px 0 0;padding:0;display:grid;gap:6px;max-height:320px;overflow:auto}.history li{display:grid;grid-template-columns:minmax(0,11rem) minmax(0,1fr);gap:12px;font-size:.9375rem}.history time{color:var(--muted)}@media(max-width:560px){.history li{grid-template-columns:minmax(0,1fr);gap:0}}
  /* Status reads by colour and shape together: a green tick is done, a pulsing green ring is running now, amber with "!" waits for the person, red has stopped. */
  .track li.running{color:var(--ink);font-weight:600}.track li.running .dot{border-color:var(--good);color:var(--good)}
  .track li.running .dot::after{content:"";position:absolute;inset:-2px;border-radius:50%;border:2px solid var(--good);animation:factory-pulse 1.8s ease-out infinite;pointer-events:none}
  .track li.waiting{color:var(--warn);font-weight:600}.track li.waiting .dot{border-color:var(--warn);background:var(--warn);color:var(--bg)}
  @keyframes factory-pulse{0%{transform:scale(1);opacity:.75}70%,100%{transform:scale(1.9);opacity:0}}
  .badge.busy{border-color:var(--good);color:var(--good)}.badge.busy::before{content:"";display:inline-block;width:8px;height:8px;margin-right:6px;border-radius:50%;background:var(--good);animation:factory-blink 1.8s ease-in-out infinite}
  .badge.good{border-color:var(--good);color:var(--good)}.badge.good::before{content:"✓";margin-right:5px}
  @keyframes factory-blink{0%,100%{opacity:1}50%{opacity:.35}}
  @media(prefers-reduced-motion:reduce){.track li.running .dot::after{animation:none;transform:scale(1.35);opacity:.45}.badge.busy::before{animation:none}}
  .track li.failed{color:var(--bad)}.track li.failed .dot{border-color:var(--bad);color:var(--bad)}
  @media(prefers-reduced-motion:no-preference){.track li.running .dot{animation:pulse 2s ease-in-out infinite}}@keyframes pulse{50%{box-shadow:0 0 0 7px transparent}}
  .now{color:var(--ink)}.pipe.bad .now{color:var(--bad)}.callout{padding:8px 12px;border-radius:8px;background:var(--card);border:1px solid var(--accent)}.report{border-left:2px solid var(--line-strong);padding-left:12px;display:grid;gap:4px;justify-items:start}.prose-clamp{max-height:6.2em;overflow:hidden;min-width:0;max-width:100%}.prose-clamp.open{max-height:none}.report-clamp{max-height:9em}.outcome.shelved{opacity:0.8}.btn.danger{border-color:var(--bad);color:var(--bad)}
  .tile-jump{display:grid;gap:2px;width:100%;min-height:44px;padding:0;border:0;background:none;color:inherit;font:inherit;text-align:left;cursor:pointer}.tile-jump:focus-visible{outline:2px solid var(--accent);outline-offset:4px}.tile-jump span{text-decoration:underline}
  .more-facts summary{cursor:pointer;min-height:32px;color:var(--muted);font-size:.875rem}.more-facts[open] summary{margin-bottom:8px}.more-facts summary:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .pipe-head .where{color:var(--muted);font-size:.875rem;white-space:nowrap}.fold{margin-left:auto;min-height:44px;padding:0 10px;border:0;background:none;color:var(--accent-text);font:inherit;font-size:.875rem;text-decoration:underline;cursor:pointer}.fold:focus-visible{outline:2px solid var(--accent);outline-offset:2px}.pipe-head .badge{margin-left:0}.now.brief{margin:0;display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2;line-clamp:2;overflow:hidden}.fold-all{display:flex;justify-content:flex-end}
  .decide{display:grid;gap:8px;padding:12px 14px;border-radius:10px;background:var(--card);border:1px solid var(--accent)}.decide-why{margin:0;color:var(--ink)}.decide-ask{margin:0;font-weight:600}.decide-choices{display:flex;flex-wrap:wrap;gap:8px}.decide-choices .btn{min-height:44px}@media(max-width:560px){.decide-choices .btn{flex:1 1 100%}}
  .pipe.arrived{outline:2px solid var(--accent);outline-offset:3px}.pipe:focus{outline:2px solid var(--accent);outline-offset:3px}.clamp{display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:3;line-clamp:3;overflow:hidden}
  .inline{border:0;background:none;padding:0 2px;font:inherit;font-weight:600;color:var(--accent-text);cursor:pointer;min-height:24px;text-decoration:underline}.inline:focus-visible{outline:2px solid var(--accent);outline-offset:2px}@media(pointer:coarse){.inline{min-height:44px}}
  .facts dd.low{color:var(--warn);font-weight:600}.split{color:var(--muted)}
  .facts dd.yolo{color:var(--warn);font-weight:600}
  .facts{display:flex;flex-wrap:wrap;gap:8px 24px;margin:0}.facts div{display:grid;gap:0;min-width:0}.facts dt{font-size:0.875rem;color:var(--muted)}.facts dd{margin:0;font-variant-numeric:tabular-nums;overflow-wrap:anywhere}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.actions .btn{min-height:44px;text-decoration:none;display:inline-flex;align-items:center;box-sizing:border-box}
  .floor{display:grid;gap:10px;min-width:0;border-top:1px solid var(--line);padding-top:20px}.foot{font-size:0.875rem}
  .sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap}
  a:focus-visible,button:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:640px){.track{overflow:visible}.track li{min-width:0;padding:0 1px 2px;font-size:0.875rem}.dot{width:24px;height:24px;font-size:0.875rem}.track li+li::before{top:11px}.pipe,.outcome-head{padding:14px 12px}.tiles{grid-template-columns:repeat(2,minmax(0,1fr))}.setup{grid-template-columns:1fr}.top-actions{width:100%}.top-actions .btn{flex:1 1 auto;justify-content:center}}
</style>
