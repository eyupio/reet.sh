<script lang="ts">
  // The Factory: every pipeline of every outcome Gaffer owns, in a list of what waits for the person, what is working
  // and what is finished, beside the detail of the one chosen (on a narrow screen, the detail is a screen of its own).
  // Reading the page starts nothing; the only things that act are the buttons, and each says what it does.
  import { onMount, tick } from 'svelte'
  import { addGafferPipelines, approveGafferMerge, controlGafferGoal, getGafferGoal, listGafferOutcomeHistory, type Event as ActivityEvent, moveGafferPipelineOn, sendGafferPipelineBack, setGafferPipelineAutoMerge, tellGafferPipelineAgent, getGafferSettings, getProfile, archiveGafferOutcome, restoreGafferOutcome, deleteGafferOutcome, listGafferGoals, listGafferOutcomes, retryGafferPipeline, reviewGafferOutcome, setGafferOutcomeAutomation, setGafferOutcomeYolo, stopGafferOutcome, stopFollowingGafferOutcome, type GafferGoalOverview, type GafferOutcome as Outcome, type GafferOutcomeSession } from '../api'
  import { chatToolName, folderProjectList, listFolderProjects, listProjectGroups, machineHealth, terminalMachines, terminalSessions, TerminalError, type FolderProject, type ProjectGroup, type TerminalSession } from '../terminal/api'
  import { UnauthorizedError } from '../api'
  import { projectRunning, projectWorktrees } from '../home'
  import { clampRailWidth, RAIL_DEFAULT, readWidth } from '../shell/workspaceLayout'
  import ProjectRailHost from '../home/ProjectRailHost.svelte'
  import { GAFFER_GOALS_CHANGED } from '../gafferContext'
  import { pipelineAnchor, stageLabel, type NeedChoice } from '../gafferOutcome'
  import { sourceLabel } from '../clineSource'
  import { planEntries, type MachineAccounts } from '../agentAccounts'
  import { machinePlans, PLAN_REFRESH } from '../planLimits'
  import { floorCandidates } from '../gafferFloor'
  import { formatWhen } from '../time'
  import GafferCompanionAvatar from '../GafferCompanionAvatar.svelte'
  import GafferOutcome from '../GafferOutcome.svelte'
  import GafferFollowedSessions from '../GafferFollowedSessions.svelte'
  import GafferGoal from '../GafferGoal.svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import AddPipelines, { type AddedPipeline } from './AddPipelines.svelte'
  import PipelineNote from './PipelineNote.svelte'
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'
  import ProjectPicker from './ProjectPicker.svelte'
  import FactoryWelcome from './FactoryWelcome.svelte'
  import ListSection from './ListSection.svelte'
  import WaitingItem from './WaitingItem.svelte'
  import PipelineDetail, { type DetailActions, type DetailHelpers } from './PipelineDetail.svelte'
  import { factoryItems, inSection, sectionSummary, type FactoryItem, type FactorySection } from './factoryList'
  import { readSections, writeSections } from './sectionMemory'

  let { userId = 0, timeZone = '', dateFormat = 'locale', onsignedout }: { userId?: number; timeZone?: string; dateFormat?: string; onsignedout?: (e: unknown) => void } = $props()
  let records = $state<Outcome[]>([]), goals = $state<GafferGoalOverview[]>([]), loading = $state(true), error = $state(''), readAt = $state(0)
  // What a new installation still needs before a pipeline can run. Each is read once and again after an outcome dialog closes.
  let setup = $state<{ provider: boolean; machine: boolean; project: boolean } | null>(null)
  let outcomeOpen = $state(false), outcomeButton = $state<HTMLButtonElement>(), outcomeObjective = $state('')
  let welcome = $state<FactoryWelcome>()
  let merging = $state<{ record: Outcome; piece: GafferOutcomeSession } | null>(null), managing = $state<GafferGoalOverview | null>(null)
  let busy = $state(false), notice = $state(''), bad = $state(false), floorKey = $state(0)
  // Why a dialog's last attempt was refused, shown inside that dialog rather than behind it.
  let dialogError = $state('')
  // What went wrong is said where the person is, in the colour of a problem, and announced.
  const fail = (e: unknown, fallback: string) => { notice = e instanceof Error ? e.message : fallback; bad = true }
  // An objective can run to paragraphs; a title or a notice names it in a line.
  const short = (text: string) => { const line = text.replace(/\s+/g, ' ').trim(); return line.length > 80 ? `${line.slice(0, 79)}…` : line }
  // Asking Gaffer for a report on an outcome, and approving pipelines it proposed. `picked` holds the ticked proposals per outcome.
  let reporting = $state(''), picked = $state<Record<string, string[]>>({}), adding = $state<Outcome | null>(null), clock = $state(Date.now())
  let controller: AbortController | undefined, epoch = 0

  const byConversation = $derived(new Map(goals.map(row => [row.goal.conversationId, row])))
  // Outcomes put away leave the floor and the tallies; they stay in the record and can be shown and restored.
  const floor = $derived(records.filter(record => !record.archivedAt))
  const shelved = $derived(records.filter(record => record.archivedAt))
  let showArchived = $state(false), removeMode = $state<'archive' | 'delete'>('archive'), deleting = $state<Outcome | null>(null), releasing = $state<Outcome | null>(null)
  // The project rail, as on Home: the same projects and groups, the same fold and width (kept in this browser). Choosing a
  // project or group narrows the page to its outcomes, and a project is also where the next outcome starts.
  let projects = $state<FolderProject[] | undefined>(), groups = $state<ProjectGroup[] | undefined>(), sessions = $state<TerminalSession[] | undefined>()
  let projectsFailed = $state(false), sessionsFailed = $state(false), savedWaiting = $state(0), machineReporting = $state(new Map<string, boolean>())
  let scope = $state<{ kind: 'project' | 'group'; id: string } | null>(null), outcomeProject = $state('')
  const stored = (key: string) => { try { return localStorage.getItem(key) } catch { return null } }
  let railCollapsed = $state(stored('reet.rail.collapsed') === '1')
  let railWidth = $state(readWidth(stored('reet.rail.width'), RAIL_DEFAULT, width => clampRailWidth(width, innerWidth)))
  const NARROW = '(max-width: 1099px)'
  let narrow = $state(typeof matchMedia === 'function' && matchMedia(NARROW).matches)
  // Below 600 px an item's stages are circles with a caption; from 600 px each stage is named.
  const PHONE = '(max-width: 599px)'
  let phone = $state(typeof matchMedia === 'function' && matchMedia(PHONE).matches)
  const life = new AbortController()
  const bounded = () => AbortSignal.any([life.signal, AbortSignal.timeout(15_000)])
  const running = $derived(projectRunning(sessions ?? [])), worktrees = $derived(projectWorktrees(sessions ?? []))
  const scopedIds = $derived.by(() => {
    if (!scope) return undefined
    if (scope.kind === 'project') return [scope.id]
    const id = scope.id
    return (projects ?? []).filter(p => (p.groupId ?? '') === id).map(p => p.id)
  })
  const scopeName = $derived(scope ? (scope.kind === 'project' ? (projects ?? []).find(p => p.id === scope?.id)?.name : (groups ?? []).find(g => g.id === scope?.id)?.name) ?? 'Selection' : '')
  const inScope = (record: Outcome) => scopedIds === undefined || scopedIds.includes(record.projectId)
  // A removal or a fresh read that no longer names the chosen project or group ends the scope.
  $effect(() => {
    if (!scope || !projects) return
    if (scope.kind === 'project' && !projects.some(p => p.id === scope?.id)) scope = null
    else if (scope.kind === 'group' && groups && !groups.some(g => g.id === scope?.id)) scope = null
  })
  // An address that names a project (/factory?project=<id>) opens the page on it, once the list is read.
  let wanted = new URLSearchParams(location.search).get('project') ?? ''
  $effect(() => {
    if (!wanted || !projects) return
    if (projects.some(p => p.id === wanted)) scope = { kind: 'project', id: wanted }
    wanted = ''
  })
  const signedOut = (e: unknown) => e instanceof UnauthorizedError || (e instanceof TerminalError && e.status === 401)
  // The rail's reads have a generation, so a write's answer (`settle`) is never overwritten by a read already on its way.
  let railGeneration = 0, railLoading = false
  async function loadRail(explicit = false) {
    if (life.signal.aborted || (railLoading && !explicit)) return
    const mine = ++railGeneration
    railLoading = true
    try {
      const [p, g, s, h, m] = await Promise.allSettled([folderProjectList(bounded()), listProjectGroups(bounded()), terminalSessions(bounded()), machineHealth(bounded()), terminalMachines(bounded())])
      if (life.signal.aborted || mine !== railGeneration) return
      if ([p, g, s, h, m].some(r => r.status === 'rejected' && signedOut(r.reason))) { life.abort(); onsignedout?.(new UnauthorizedError()); return }
      if (p.status === 'fulfilled') { projects = p.value.projects; savedWaiting = p.value.savedWaiting ?? 0; projectsFailed = false } else projectsFailed = projects === undefined
      if (g.status === 'fulfilled') groups = g.value
      if (s.status === 'fulfilled') { sessions = s.value; sessionsFailed = false } else sessionsFailed = sessions === undefined
      // Connected machines come from the health report; where this sign-in cannot read it, from the machines that are ready.
      if (h.status === 'fulfilled') machineReporting = new Map((h.value.machines ?? []).map(item => [item.agentId, item.state === 'connected']))
      else if (m.status === 'fulfilled') machineReporting = new Map(m.value.map(item => [item.agentId, item.ready]))
      if (m.status === 'fulfilled') planMachines = new Map(m.value.filter(item => item.capabilities?.agentAccounts === true).map(item => [item.agentId, item.host]))
    } finally { if (mine === railGeneration) railLoading = false }
  }
  function settle(next: FolderProject[]) { railGeneration++; railLoading = false; projects = next; void loadSetup() }
  function settleGroups(next: ProjectGroup[]) { railGeneration++; railLoading = false; groups = next }
  function setRailCollapsed(next: boolean) {
    railCollapsed = next
    try { localStorage.setItem('reet.rail.collapsed', next ? '1' : '0') } catch { /* The rail still folds. */ }
  }
  const setRailWidth = (width: number) => { railWidth = clampRailWidth(width, innerWidth) }
  function commitRailWidth(width: number) {
    setRailWidth(width)
    try { localStorage.setItem('reet.rail.width', String(railWidth)) } catch { /* The rail keeps its width for this page. */ }
  }
  // Give Gaffer an outcome: in the project the page is narrowed to, or the one whose + was pressed.
  function openOutcome(project = scope?.kind === 'project' ? scope.id : '', objective = '') { outcomeProject = project; outcomeObjective = objective; outcomeOpen = true }
  const listed = $derived((showArchived ? records : floor).filter(inScope))
  // Goals the page draws inside a pipeline are left out of the list of other sessions below.
  const drawn = $derived(records.flatMap(record => record.sessions.flatMap(piece => [piece.conversationId, ...(piece.stages ?? []).map(stage => stage.conversationId), ...(piece.retired ?? [])])).filter((id): id is string => !!id))
  // A pipeline's own sessions are not offered again below, except one that was started and is not followed.
  const drawnSessions = $derived(records.flatMap(record => record.sessions.filter(piece => piece.state !== 'unfollowed').flatMap(piece => [piece.sessionId, ...(piece.stages ?? []).map(stage => stage.sessionId)])).filter((id): id is string => !!id))
  const ready = $derived(!!setup && setup.provider && setup.machine && setup.project)
  const live = $derived(records.some(record => record.state === 'starting'))
  // The list and the detail are drawn once there is a pipeline to show.
  const board = $derived(!loading && listed.length > 0)

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
    void load(); void loadSetup(); void loadRail()
    const query = matchMedia(NARROW), small = matchMedia(PHONE), resized = () => { narrow = query.matches; phone = small.matches }
    query.addEventListener('change', resized); small.addEventListener('change', resized)
    const railTimer = setInterval(() => { if (!document.hidden) void loadRail() }, 15_000)
    void getProfile().then(p => { removeMode = p.profile.factoryRemoveMode === 'delete' ? 'delete' : 'archive' }).catch(() => { /* archiving is the safe default */ })
    // A page opened to give Gaffer an outcome (from Home or the chat) starts with that dialog up.
    if (new URLSearchParams(location.search).get('outcome') === '1') openOutcome(wanted)
    const timer = setInterval(() => { clock = Date.now(); if (!document.hidden && !outcomeOpen && !managing && !merging && !adding && !reporting) void load() }, 5000)
    const changed = () => void load()
    window.addEventListener(GAFFER_GOALS_CHANGED, changed)
    return () => { epoch++; controller?.abort(); life.abort(); clearInterval(timer); clearInterval(railTimer); query.removeEventListener('change', resized); small.removeEventListener('change', resized); window.removeEventListener(GAFFER_GOALS_CHANGED, changed) }
  })
  async function closeOutcome() {
    outcomeOpen = false; floorKey++
    address({ outcome: null, project: null })
    await Promise.all([load(), loadSetup(), loadRail(true)]); await tick()
    if (outcomeButton?.isConnected) outcomeButton.focus({ preventScroll: true }); else welcome?.focus()
  }
  async function approve() {
    if (!merging || busy) return
    const { record, piece } = merging
    busy = true; notice = ''; bad = false
    try {
      const fresh = records.find(item => item.id === record.id) ?? record
      await approveGafferMerge(fresh, piece.key)
      notice = `Merge approved for “${piece.title}”. Its agent merges once every check passes, no review thread is open and there are no conflicts.`
    } catch (e) { fail(e, 'The merge was not approved.') }
    busy = false; merging = null
    await load()
  }
  async function stop(record: Outcome) {
    if (busy) return
    busy = true; notice = ''; bad = false
    try { await stopGafferOutcome(latest(record)); notice = 'Stopped. Sessions not yet started stay unstarted; the ones already started keep running.' }
    catch (e) { fail(e, 'The outcome was not stopped.') }
    busy = false
    await load()
  }
  // Ends an outcome that is not finished (a session was lost or removed, or the work is no longer wanted) so it can be archived or deleted.
  async function stopFollowing(record: Outcome) {
    if (busy) return
    busy = true; notice = ''; bad = false; releasing = null
    try { await stopFollowingGafferOutcome(latest(record)); notice = `Stopped following “${short(record.objective)}”. Sessions that still exist keep running. You can now archive or delete it.` }
    catch (e) { fail(e, 'The outcome was not stopped.') }
    busy = false
    await load()
  }
  async function putAway(record: Outcome, mode: 'archive' | 'delete' | 'restore') {
    if (busy) return
    busy = true; notice = ''; bad = false; deleting = null
    try {
      const current = latest(record)
      if (mode === 'delete') { await deleteGafferOutcome(current); notice = `Deleted “${short(record.objective)}”. Its sessions and pull requests were not touched.` }
      else if (mode === 'archive') { await archiveGafferOutcome(current); notice = `Archived “${short(record.objective)}”. Show archived outcomes to bring it back.` }
      else { await restoreGafferOutcome(current); notice = `Restored “${short(record.objective)}”.` }
    } catch (e) { fail(e, 'That outcome was not changed.') }
    busy = false
    await load()
  }
  const latest = (record: Outcome) => records.find(item => item.id === record.id) ?? record
  // A review of Gaffer's own that did not complete is tried again by resuming the goal under its current permissions.
  async function reviewAgain(piece: GafferOutcomeSession) {
    if (busy || !piece.conversationId) return
    busy = true; notice = ''; bad = false
    try {
      const view = await getGafferGoal(piece.conversationId)
      if (!view.goal) throw new Error('That goal is no longer there.')
      if (!view.allowed) throw new Error(view.reason || 'Review this goal’s permissions before trying again.')
      await controlGafferGoal(piece.conversationId, view.goal.revision, 'resume', view.pins, true)
      notice = `Gaffer is reviewing “${piece.title}” again.`
    } catch (e) { fail(e, 'The review was not tried again.') }
    busy = false
    await load()
  }
  // What happened to an outcome's pipelines, read from the activity log when asked for. Each ask reads it again, so a
  // read that failed can be tried again by opening the row once more.
  let timeline = $state<Record<string, { events: ActivityEvent[]; error: string; loading: boolean }>>({})
  async function loadHistory(record: Outcome) {
    timeline = { ...timeline, [record.id]: { events: timeline[record.id]?.events ?? [], error: '', loading: true } }
    try { timeline = { ...timeline, [record.id]: { events: await listGafferOutcomeHistory(record.id), error: '', loading: false } } }
    catch (e) { timeline = { ...timeline, [record.id]: { events: [], error: e instanceof Error ? e.message : 'The history could not be read.', loading: false } } }
  }
  // One choice from an item or the detail. Each does what its button says and nothing else; "restart" is also a failed
  // pipeline's Try again, which has no goal.
  let expanded = $state<Record<string, boolean>>({})
  function choose(choice: NeedChoice, record: Outcome, piece: GafferOutcomeSession, goal: GafferGoalOverview | undefined) {
    if (choice.id === 'answer' || choice.id === 'open') { if (piece.sessionId) location.href = sessionHref(record, piece.sessionId) }
    else if (choice.id === 'retry' || choice.id === 'again') void reviewAgain(piece)
    else if (choice.id === 'tell') { tellStart = choice.recommended && goal?.goal.advice?.choice === 'tell' ? goal.goal.advice.text ?? '' : ''; telling = { record, piece } }
    else if (choice.id === 'merge') merging = { record, piece }
    else if (choice.id === 'move') moving = { record, piece }
    else if (choice.id === 'budget') { if (goal) managing = goal }
    else if (choice.id === 'restart') void retry(record, piece)
    else if (choice.id === 'stop') releasing = record
  }
  // The choices a pipeline can actually take right now: no moving on without a stage in hand, no telling an agent that
  // is not there, no opening a session that was removed.
  const usable = (choice: NeedChoice, piece: GafferOutcomeSession) => {
    const alive = piece.sessionState !== 'ended' && piece.sessionState !== 'gone'
    if (choice.id === 'move') return piece.state === 'following' && !!piece.stages?.length && !!piece.conversationId
    if (choice.id === 'tell') return !!piece.sessionId && !!piece.conversationId && alive
    if (choice.id === 'open' || choice.id === 'answer') return !!piece.sessionId && piece.sessionState !== 'gone'
    return true
  }
  // Whether a started outcome's agents ask the person or act by themselves. Turning YOLO on is confirmed first.
  let yoloAsk = $state<Outcome | null>(null)
  async function setYolo(record: Outcome, on: boolean, confirmed = false) {
    if (busy || !!record.yolo === on) return
    if (on && !confirmed) { yoloAsk = record; return }
    yoloAsk = null
    busy = true; notice = ''; bad = false
    try {
      const { notSwitched } = await setGafferOutcomeYolo(latest(record), on)
      const still = notSwitched.length ? ` The session in hand for ${notSwitched.map(title => `“${title}”`).join(', ')} could not be switched and keeps its own setting; change it in that session, or it changes with the next stage.` : ''
      notice = (on ? 'This outcome is on YOLO: its agents act without asking you, in the sessions in hand and in every stage still to start. A pipeline set to merge by itself waits for you again until you confirm that for YOLO.' : 'This outcome’s agents ask you again before they act.') + still
    }
    catch (e) { fail(e, 'That was not changed.') }
    busy = false
    await load()
  }
  // What Gaffer would say to the agent, as the words the Tell dialog starts from.
  let tellStart = $state('')
  // Who takes an outcome's decisions when a stage stops: the person, or Gaffer for them.
  async function setDecisions(record: Outcome, on: boolean) {
    if (busy || !!record.automation === on) return
    busy = true; notice = ''; bad = false
    try { await setGafferOutcomeAutomation(latest(record), on); notice = on ? 'Gaffer takes this outcome’s decisions from now on, and records each one here. Merges, approvals and questions that need your own words still come to you.' : 'You take this outcome’s decisions again.' }
    catch (e) { fail(e, 'That was not changed.') }
    busy = false
    await load()
  }
  // A link from Home or elsewhere names one pipeline by its anchor (/factory#pipeline-…): it is chosen once the page has it.
  let sought = false
  $effect(() => {
    if (sought || loading || !records.length) return
    sought = true
    const id = decodeURIComponent(location.hash.slice(1))
    if (!id.startsWith('pipeline-')) return
    const record = records.find(item => id.startsWith(`pipeline-${item.id}-`))
    const piece = record?.sessions.find(item => pipelineAnchor(record.id, item.key) === id)
    if (!record || !piece) return
    if (record.archivedAt) showArchived = true
    select(`${record.id}.${piece.key}`)
    // Its section opens so the item can be seen in the list, and the detail's title takes the focus.
    void tick().then(() => {
      const item = byId.get(`${record.id}.${piece.key}`)
      if (item && !opened[item.section]) opened = { ...opened, [item.section]: true }
      void tick().then(() => { if (!narrow) document.getElementById(id)?.scrollIntoView({ block: 'nearest' }) })
    })
    focusTitle()
  })
  const pipelineOf = (event: ActivityEvent) => { try { return String(JSON.parse(event.detail || '{}').pipeline ?? '') } catch { return '' } }
  // The person's own controls over a pipeline: move it on from the stage in hand, and how its merge is decided.
  let moving = $state<{ record: Outcome; piece: GafferOutcomeSession } | null>(null)
  async function moveOn() {
    if (!moving || busy) return
    const { record, piece } = moving
    busy = true; notice = ''; bad = false; moving = null
    try { await moveGafferPipelineOn(record, piece.key, piece.stages?.[piece.stage ?? 0]?.kind ?? ''); notice = `Moved “${piece.title}” on. The controller is starting its next stage.` }
    catch (e) { fail(e, 'That pipeline was not moved on.') }
    busy = false
    await load()
  }
  // Telling the working agent what to do next, and sending a pipeline back to a stage to be done again.
  let telling = $state<{ record: Outcome; piece: GafferOutcomeSession } | null>(null), sending = $state<{ record: Outcome; piece: GafferOutcomeSession } | null>(null)
  const requestId = () => Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('')
  // One request ID for each opening of the dialog: a second press after a lost answer reads the first's receipt instead of telling the agent twice.
  let tellRequest = ''
  $effect(() => { if (telling) tellRequest = requestId(); dialogError = '' })
  $effect(() => { if (sending || adding) dialogError = '' })
  async function tell(text: string) {
    if (!telling || busy) return
    const { record, piece } = telling
    busy = true; notice = ''; bad = false; dialogError = ''
    try {
      const sent = await tellGafferPipelineAgent(record, piece.key, tellRequest, text)
      notice = sent.message.state === 'sent' || sent.message.state === 'queued' || sent.message.state === 'sending' ? `Your instruction is with the agent of “${piece.title}” for its next turn.` : `The instruction for “${piece.title}” was not delivered: ${sent.message.note || sent.message.state}.`
      telling = null
    } catch (e) { dialogError = e instanceof Error ? e.message : 'The instruction was not sent.' }
    busy = false
    await load()
  }
  async function sendBack(reason: string, stage: string) {
    if (!sending || busy) return
    const { record, piece } = sending
    busy = true; notice = ''; bad = false; dialogError = ''
    try { await sendGafferPipelineBack(record, piece.key, stage, reason); notice = `Sent “${piece.title}” back to ${stageLabel(stage).toLowerCase()}. The controller is starting that stage again.`; sending = null }
    catch (e) { dialogError = e instanceof Error ? e.message : 'That pipeline was not sent back.' }
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
    busy = true; notice = ''; bad = false
    try { await setGafferPipelineAutoMerge(record, piece.key, on, yoloConfirmed); notice = on ? `“${piece.title}” will merge by itself once every check passes.` : `“${piece.title}” will stop at Ready to merge and wait for you.` }
    catch (e) { fail(e, 'The merge setting was not changed.') }
    busy = false
    await load()
  }
  async function retry(record: Outcome, piece: GafferOutcomeSession) {
    if (busy) return
    busy = true; notice = ''; bad = false
    try { await retryGafferPipeline(latest(record), piece.key); notice = `Trying “${piece.title}” again.` }
    catch (e) { fail(e, 'That pipeline was not tried again.') }
    busy = false
    await load()
  }
  async function report(record: Outcome) {
    if (busy || reporting) return
    reporting = record.id; notice = ''; bad = false
    try {
      const next = await reviewGafferOutcome(latest(record))
      records = records.map(item => item.id === next.id ? next : item)
      // A new report's proposals start ticked again.
      const { [next.id]: _, ...rest } = picked; picked = rest
    } catch (e) { fail(e, 'Gaffer could not report on this outcome.') }
    reporting = ''
  }
  // Every proposal starts ticked, whoever asked for the report and whenever the page was opened; nothing is added until the person says so.
  const ticked = (record: Outcome) => picked[record.id] ?? (record.report?.proposals ?? []).map(item => item.title)
  const chosenProposals = (record: Outcome) => (record.report?.proposals ?? []).filter(item => ticked(record).includes(item.title))
  function pick(record: Outcome, title: string, on: boolean) {
    const now = ticked(record)
    picked = { ...picked, [record.id]: on ? [...now, title] : now.filter(item => item !== title) }
  }
  async function add(chosen: AddedPipeline[]) {
    if (!adding || busy) return
    const record = adding
    busy = true; notice = ''; bad = false; dialogError = ''
    try {
      await addGafferPipelines(latest(record), chosen)
      adding = null
      notice = chosen.length === 1 ? `Added “${chosen[0].title}”. The controller is starting it.` : `Added ${chosen.length} pipelines. The controller is starting them.`
    } catch (e) { dialogError = e instanceof Error ? e.message : 'The pipelines were not added. Nothing was started.' }
    busy = false
    await load()
  }
  const sessionHref = (record: Outcome, id: string) => `/sessions?agent=${encodeURIComponent(record.agentId)}&session=${encodeURIComponent(id)}`
  const when = (at: number) => formatWhen(at, timeZone, dateFormat)
  // What is left of the plan of the agent doing a pipeline's stage in hand, where the outcome's machine reads sign-ins
  // and the account has plan limits on. The same figures as the machine's settings; nothing is asked of the machine.
  let plans = $state<Record<string, MachineAccounts>>({}), planClock = $state(Date.now())
  let planMachines = $state<Map<string, string>>(new Map())
  const planAgents = $derived([...new Set(records.map(o => o.agentId).filter(id => id && planMachines.has(id)))].sort().join(','))
  $effect(() => {
    const ids = planAgents ? planAgents.split(',') : []
    if (!ids.length) return
    let live = true
    const read = () => { for (const id of ids) void machinePlans(id).then(answer => { if (live && answer) { plans = { ...plans, [id]: answer }; planClock = Date.now() } }) }
    read()
    const timer = setInterval(read, PLAN_REFRESH)
    return () => { live = false; clearInterval(timer) }
  })
  const planFor = (record: Outcome, piece: GafferOutcomeSession) => { const tool = piece.stages?.[piece.stage ?? 0]?.tool ?? record.tool; return planEntries([{ host: planMachines.get(record.agentId) ?? '', answer: plans[record.agentId] }], planClock).filter(entry => entry.tool === tool) }
  const crew = (piece: GafferOutcomeSession, record: Outcome) => {
    const stage = piece.stages?.[piece.stage ?? 0]
    const tool = stage?.tool ?? record.tool, model = stage?.model ?? record.model ?? ''
    // A stage's own empty source is Cline's own choice; the outcome's source is only the fallback when a piece has no stage.
    const source = stage ? stage.provider ?? '' : record.provider ?? ''
    return `${chatToolName(tool)}${model ? ` · ${model}` : ''}${source ? ` · ${sourceLabel(source)}` : ''}${stage?.effort ? ` · ${stage.effort} effort` : ''}`
  }

  // The list: one item per pipeline. A session holding a tool approval makes its pipeline an Approval item, answered
  // from the item itself. An archived outcome's pipelines need nobody, so they sit with the finished.
  const held = $derived(new Map((sessions ?? []).flatMap(session => session.pendingPermission?.permissionId ? [[session.id, session.pendingPermission] as const] : [])))
  const approvalFor = (sessionId: string | undefined) => {
    const p = sessionId ? held.get(sessionId) : undefined
    return p ? { permissionId: p.permissionId, title: p.title || 'A tool call', command: p.command ?? '', path: p.path ?? '', truncated: !!p.truncated, queued: p.queued ?? 0 } : undefined
  }
  // What waits and works in each project, whatever the page is narrowed to: the picker and the wide rail both say it.
  const projectCounts = $derived.by(() => {
    const counts = new Map<string, { waiting: number; working: number }>()
    for (const item of factoryItems(floor, byConversation, new Set(held.keys()))) {
      const count = counts.get(item.projectId) ?? { waiting: 0, working: 0 }
      if (item.section === 'waiting') count.waiting++; else if (item.section === 'working') count.working++
      counts.set(item.projectId, count)
    }
    return counts
  })
  const railDetail = $derived(new Map((projects ?? []).map(p => [p.id, `${projectCounts.get(p.id)?.waiting ?? 0} waiting · ${projectCounts.get(p.id)?.working ?? 0} working`] as const)))
  const items = $derived(factoryItems(listed, byConversation, new Set(held.keys())).map((item): FactoryItem => item.archived ? { ...item, section: 'finished' } : item))
  const pairs = $derived(new Map<string, { record: Outcome; piece: GafferOutcomeSession }>(listed.flatMap(record => record.sessions.map(piece => [`${record.id}.${piece.key}`, { record, piece }] as const))))
  const byId = $derived(new Map(items.map(item => [item.id, item])))
  const waitingItems = $derived(inSection(items, 'waiting'))
  const counted = $derived(items.filter(item => !item.archived))
  const count = (section: FactorySection) => counted.filter(item => item.section === section).length

  // Which sections are open, remembered in this browser for this person. The first time, only Waiting for you is.
  const SECTIONS_FIRST = { waiting: true }
  const storage = () => { try { return localStorage } catch { return undefined } }
  const sectionsKey = $derived(`reet.factory.sections.${userId}`)
  let opened = $state<Record<string, boolean>>({ ...SECTIONS_FIRST }), openedFor = ''
  $effect.pre(() => { if (openedFor !== sectionsKey) { openedFor = sectionsKey; opened = readSections(storage(), sectionsKey, SECTIONS_FIRST) } })
  function toggleSection(name: string) {
    opened = { ...opened, [name]: !opened[name] }
    writeSections(storage(), sectionsKey, opened)
  }

  // The chosen pipeline is named in the address (?pipeline=<outcome>.<key>), so a reload or a shared link reopens it.
  // On a wide screen one is always shown: the chosen one, else the first waiting, else the first working. Once shown it
  // stays, even when it moves to another section. On a narrow screen the detail is open only while one is chosen.
  // On a narrow screen, opening a pipeline from the list is a step the browser's Back undoes: it is pushed onto the
  // history, and `pushed` says so. Previous and next, a wide screen, and a page entered on ?pipeline= replace instead.
  const addressed = (changes: Record<string, string | null>, hash = location.hash) => {
    const url = new URL(location.href)
    for (const [name, value] of Object.entries(changes)) { if (value) url.searchParams.set(name, value); else url.searchParams.delete(name) }
    url.hash = hash
    return url
  }
  const address = (changes: Record<string, string | null>, hash = location.hash) => history.replaceState(history.state, '', addressed(changes, hash))
  let pushed = false
  let chosenId = $state(new URLSearchParams(location.search).get('pipeline') ?? ''), shownId = $state('')
  let pane = $state<HTMLElement>()
  const firstId = $derived((waitingItems[0] ?? inSection(items, 'working')[0])?.id ?? '')
  $effect(() => { if (!narrow && !byId.has(chosenId) && !byId.has(shownId) && shownId !== firstId) shownId = firstId })
  const selectedId = $derived(byId.has(chosenId) ? chosenId : narrow ? '' : byId.has(shownId) ? shownId : firstId)
  const selected = $derived(selectedId ? byId.get(selectedId) : undefined)
  const selectedPair = $derived(selectedId ? pairs.get(selectedId) : undefined)
  const detailOpen = $derived(narrow && !!selected)
  const place = $derived(selected ? waitingItems.findIndex(item => item.id === selected.id) : -1)
  // The detail's title takes the focus when the detail opens as a screen of its own or from a link.
  const focusTitle = () => void tick().then(() => pane?.querySelector<HTMLElement>('h2')?.focus({ preventScroll: true }))
  function select(id: string) {
    chosenId = id
    if (id) shownId = id
    address({ pipeline: id || null }, '')
    if (narrow) void tick().then(() => scrollTo({ top: 0 })); else if (pane) pane.scrollTop = 0
  }
  // Details on an item: on a narrow screen a new history entry, with the focus on the detail's title.
  function open(id: string) {
    if (!narrow || byId.has(chosenId)) { select(id); return }
    history.pushState(history.state, '', addressed({ pipeline: id }, ''))
    pushed = true; chosenId = id; shownId = id
    void tick().then(() => scrollTo({ top: 0 }))
    focusTitle()
  }
  // Back to the list, at the item that was open, with the focus on its Details button.
  const returnTo = (item: FactoryItem | undefined) => {
    if (item) void tick().then(() => {
      const card = document.getElementById(pipelineAnchor(item.outcomeId, item.key))
      card?.scrollIntoView({ block: 'center' })
      card?.querySelector<HTMLElement>('button[aria-label^="Details"]')?.focus({ preventScroll: true })
    })
  }
  function back() {
    if (pushed) { history.back(); return }
    const item = selected
    select('')
    returnTo(item)
  }
  // The browser's Back and Forward move between the list and the detail opened from it.
  onMount(() => {
    const moved = () => {
      const item = selected
      chosenId = new URLSearchParams(location.search).get('pipeline') ?? ''
      pushed = !!chosenId
      if (chosenId) { shownId = chosenId; focusTitle() } else returnTo(item)
    }
    addEventListener('popstate', moved)
    return () => removeEventListener('popstate', moved)
  })
  const step = (by: number) => { const next = waitingItems[place + by]; if (next) select(next.id) }

  // Other sessions: Gaffer's goals that are not a pipeline's, and running chats it does not look after yet, in the scope.
  const inScopeSession = (id: string) => scopedIds === undefined || scopedIds.includes((sessions ?? []).find(session => session.id === id)?.projectId ?? '')
  const others = $derived.by(() => {
    const rows = goals.filter(row => !drawn.includes(row.goal.conversationId) && inScopeSession(row.goal.sessionId))
    const chats = floorCandidates(sessions ?? [], goals.map(row => row.goal)).filter(session => !drawnSessions.includes(session.id) && inScopeSession(session.id))
    const needs = rows.filter(row => row.attention).length
    return { count: rows.length + chats.length, summary: [needs ? `${needs} need${needs === 1 ? 's' : ''} you` : '', rows.length ? `${rows.length} followed` : '', chats.length ? `${chats.length} not followed` : ''].filter(Boolean).join(' · ') }
  })

  const actions: DetailActions = {
    choose, setDecisions, setYolo, mergeMode, report, loadHistory, stop, putAway, retry, pick,
    releasing: record => { releasing = record }, deleting: record => { deleting = record }, adding: record => { adding = record },
    moving: pair => { moving = pair }, telling: pair => { tellStart = ''; telling = pair }, sending: pair => { sending = pair },
    merging: pair => { merging = pair }, managing: goal => { managing = goal },
  }
  const helpers: DetailHelpers = { crew, planFor, sessionHref, when, usable, mergesByItself, pipelineOf }
</script>

{#snippet entry(item: FactoryItem)}{@const pair = pairs.get(item.id)}{#if pair}{@const record = pair.record}{@const piece = pair.piece}{@const goal = piece.conversationId ? byConversation.get(piece.conversationId) : undefined}
  <WaitingItem {item} layout={narrow ? 'card' : 'compact'} pipeline={narrow ? (phone ? 'circles' : 'labelled') : 'dots'} selected={!narrow && item.id === selectedId} {busy}
    permission={approvalFor(piece.sessionId)} sessionId={piece.sessionId ?? ''} usable={choice => usable(choice, piece)} onchoose={choice => choose(choice, record, piece, goal)}
    ondetails={() => open(item.id)} onanswered={() => loadRail(true)} id={pipelineAnchor(record.id, piece.key)}/>
{/if}{/snippet}

{#snippet section(key: FactorySection, name: string, none: string)}{@const own = inSection(items, key)}
  <ListSection {name} count={own.length} summary={sectionSummary(items, key, clock)} open={!!opened[key]} ontoggle={() => toggleSection(key)}>
    {#each own as item (item.id)}{@render entry(item)}{:else}<p class="none">{none}</p>{/each}
  </ListSection>
{/snippet}

<!-- Sessions the person started themselves, which Gaffer looks after or could: a section of their own, closed at first. -->
{#snippet floorSection()}
  <ListSection name="Other sessions" count={others.count} summary={others.summary} open={!!opened.others} ontoggle={() => toggleSection('others')}>
    <div class="floor">
      <p class="hint">Sessions you started yourself: the ones Gaffer looks after, and the running chats it does not yet.{#if scope}{' '}Only those in {scopeName} are listed.{/if}</p>
      {#key floorKey}<GafferFollowedSessions inline hidden={drawn} hiddenSessions={drawnSessions} projectIds={scopedIds} {userId} {timeZone} {dateFormat} onerror={e => e instanceof Error ? e.message : 'The sessions could not be read.'} onchanged={load} onselect={() => {}} onhistory={() => {}} onclose={() => {}}/>{/key}
    </div>
  </ListSection>
{/snippet}

{#snippet foot()}{#if readAt}<p class="foot">Read {when(readAt)}{live ? ', while the controller starts sessions' : ''}. The page refreshes every few seconds. Reading the factory makes no model request and starts nothing.</p>{/if}{/snippet}

<div class="factory-page" class:rail-collapsed={railCollapsed} style:--rail-width={`${railWidth}px`}>
<!-- On a desktop the project list is the page's left column, as on Home. A tablet or phone has no rail here: the outcome
     dialog asks for the project instead. -->
<ProjectRailHost rail={!narrow} {projects} {groups} failed={projectsFailed} {savedWaiting} {sessions} {sessionsFailed} reporting={machineReporting} {running} detail={railDetail} known={sessions !== undefined} {worktrees}
  chosen={scope?.kind === 'project' ? scope.id : ''} {scope} onscope={next => (scope = next)} onchoose={() => {}} onnew={project => openOutcome(project.id)} startWord="New outcome"
  collapsed={railCollapsed} oncollapsed={setRailCollapsed} {userId} width={railWidth} onwidth={setRailWidth} onwidthcommit={commitRailWidth}
  signal={bounded} onsettle={settle} onsettlegroups={settleGroups} onwaiting={n => (savedWaiting = n)} onnotice={n => (notice = n?.text ?? '')} onreload={() => void loadRail(true)}
  onsignedout={() => { life.abort(); onsignedout?.(new UnauthorizedError()) }}/>
<div class="factory" class:split={board && !narrow} class:reading={detailOpen}>
  {#if !detailOpen}
    <header class="top">
      <div class="identity"><h1>Factory</h1>{#if records.length}<p class="counts">{count('waiting')} waiting for you · {count('working')} working · {count('finished')} finished</p>{/if}</div>
      <div class="top-actions">
        {#if shelved.length}<button class="btn quiet" type="button" aria-pressed={showArchived} onclick={() => showArchived = !showArchived}>{showArchived ? 'Hide archived' : `Show archived (${shelved.length})`}</button>{/if}
        <a class="btn quiet" href="/account/gaffer">Gaffer settings</a>
        {#if loading || error || records.length || scope}<button class="btn primary" type="button" bind:this={outcomeButton} onclick={() => openOutcome()}>Give Gaffer an outcome</button>{/if}
      </div>
    </header>
    {#if narrow && (projects ?? []).length}<div class="projects"><ProjectPicker projects={projects ?? []} counts={projectCounts} chosen={scope?.kind === 'project' ? scope.id : ''} onchoose={id => (scope = id ? { kind: 'project', id } : null)}/></div>{/if}
    {#if scope}<p class="scope"><span>Showing <strong>{scopeName}</strong>{scope.kind === 'project' ? '. A new outcome starts in this project.' : '.'}</span> <button type="button" class="btn quiet" onclick={() => (scope = null)}>Show all projects</button></p>{/if}
  {/if}
  {#if notice}<div class="notice" class:bad><p role={bad ? 'alert' : 'status'}>{notice}</p><button class="inline" type="button" aria-label="Dismiss this message" onclick={() => notice = ''}>Dismiss</button></div>{/if}
  {#if error && records.length}<p role="alert" class="warning">{error} Showing the last successful read.</p>{/if}

  {#if setup && !ready && !detailOpen}<section class="setup" aria-label="Set up your factory">
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

  {#if loading}<p role="status">Reading the factory…</p>
  {:else if error && !records.length}<div class="empty"><h2>The factory could not be read.</h2><p>Nothing is shown because nothing was read, not because there is nothing. The page tries again by itself every few seconds.</p><button class="btn" type="button" onclick={() => { loading = true; void load() }}>Try again now</button></div>
  {:else if scope && !listed.length}<div class="empty"><h2>Nothing in {scopeName} yet.</h2><p>{scope.kind === 'project' ? 'Give Gaffer an outcome and its pipelines for this project appear here.' : 'No outcome has been started in this group’s projects.'}</p>{#if scope.kind === 'project'}<button class="btn primary" type="button" onclick={() => openOutcome()}>Give Gaffer an outcome in {scopeName}</button>{/if}</div>
  {:else if !listed.length && shelved.length}<div class="empty"><h2>Nothing on the floor.</h2><p>Every outcome is archived. Show archived to look at them or bring one back.</p></div>
  {:else if !records.length}<FactoryWelcome bind:this={welcome} onstart={objective => openOutcome(undefined, objective)} />
  {:else}
    <div class="board">
      {#if !detailOpen}<div class="list">
        {@render section('waiting', 'Waiting for you', 'Nothing is waiting for you.')}
        {@render section('working', 'Working', 'Nothing is working by itself right now.')}
        {@render section('finished', 'Finished or stopped', 'Nothing has finished or stopped yet.')}
        {@render floorSection()}
        {#if !narrow}{@render foot()}{/if}
      </div>{/if}
      {#if selected && selectedPair}{@const record = selectedPair.record}{@const piece = selectedPair.piece}<div class="pane" bind:this={pane}>
        {#key selected.id}<PipelineDetail {record} {piece} item={selected} goal={piece.conversationId ? byConversation.get(piece.conversationId) : undefined} goals={byConversation} {busy} {reporting} {clock} {planClock}
          timeline={timeline[record.id]} ticked={ticked(record)} bind:expanded {narrow} {removeMode} position={place >= 0 ? { at: place + 1, of: waitingItems.length } : undefined} {actions} {helpers}
          onback={back} onprev={() => step(-1)} onnext={() => step(1)}/>{/key}
      </div>
      {:else if !narrow}<div class="pane resting"><p>Nothing is waiting for you or working. Choose a pipeline in the list to see it here.</p></div>{/if}
    </div>
  {/if}
  {#if !board && !loading}{@render floorSection()}{/if}
  {#if !(board && !narrow) && !detailOpen}{@render foot()}{/if}
</div>

</div>

{#if outcomeOpen}<GafferOutcome projectId={outcomeProject} initialObjective={outcomeObjective} room={Math.max(0, 10 - goals.filter(row => row.goal.state === 'active').length)} {userId} onstarted={load} onclose={() => void closeOutcome()}/>{/if}
{#if releasing}<ConfirmDialog title={`Stop following “${short(releasing.objective)}”?`} body="Gaffer and the controller stop looking after every unfinished pipeline in this outcome and start nothing more for it. Sessions that still exist keep running, and nothing is merged or closed. Use this when a session was lost or removed, or the work is no longer wanted; you can then archive or delete the outcome." confirmLabel="Stop following" oncancel={() => releasing = null} onconfirm={() => releasing && void stopFollowing(releasing)}/>{/if}
{#if deleting}<ConfirmDialog title={`Delete “${short(deleting.objective)}”?`} body="This erases the outcome, its report and its totals from reet for good. The sessions and pull requests it started are not touched. To keep the record out of sight instead, archive it." confirmLabel="Delete outcome" danger oncancel={() => deleting = null} onconfirm={() => deleting && void putAway(deleting, 'delete')}/>{/if}
{#if adding}<AddPipelines record={adding} proposals={chosenProposals(adding)} {busy} error={dialogError} oncancel={() => adding = null} onconfirm={pipelines => void add(pipelines)}/>{/if}
{#if moving}{@const at = moving.piece.stages?.[moving.piece.stage ?? 0]?.kind ?? ''}{@const next = moving.piece.stages?.[(moving.piece.stage ?? 0) + 1]?.kind ?? ''}<ConfirmDialog title={next ? `Move “${moving.piece.title}” on to ${stageLabel(next).toLowerCase()}?` : `Finish the ${stageLabel(at).toLowerCase()} stage of “${moving.piece.title}”?`} body={`This is your call: the ${stageLabel(at).toLowerCase()} stage is closed as it stands, without waiting for Gaffer to find it done${next ? `, and the controller starts the ${stageLabel(next).toLowerCase()} stage with Gaffer’s last review as its handover` : ''}. ${next === 'merge' && !mergesByItself(moving.record, moving.piece) ? 'It then stops at Ready to merge and waits for you.' : next === 'merge' ? 'This pipeline merges by itself once every check passes.' : 'Nothing is merged by this.'}`} confirmLabel={next ? 'Move on' : 'Finish stage'} oncancel={() => moving = null} onconfirm={() => void moveOn()}/>{/if}
{#if telling}<PipelineNote title={`Tell the agent of “${telling.piece.title}”`} intro="Your words go to this pipeline’s working agent for its next turn, exactly as you write them. It cannot interrupt the agent or answer an approval." label="Instruction" placeholder="Write the plan as text now: files, approach, risks. Change no code." initial={tellStart} confirmLabel="Send to the agent" {busy} error={dialogError} oncancel={() => telling = null} onconfirm={text => void tell(text)}/>{/if}
{#if sending}{@const upTo = Math.min(sending.piece.stage ?? 0, (sending.piece.stages?.length ?? 1) - 1)}<PipelineNote title={`Send “${sending.piece.title}” back`} intro="The stage you choose, and every stage after it, is done again in a fresh agent session in the same folder and branch. The agent is told your reason. Nothing already committed is undone." label="Why it is going back" placeholder="The review found the export leaks other tenants’ rows." confirmLabel="Send back" stages={[...new Set((sending.piece.stages ?? []).slice(0, upTo + 1).map(item => item.kind))].map(kind => ({ kind, label: stageLabel(kind) }))} {busy} error={dialogError} oncancel={() => sending = null} onconfirm={(text, stage) => void sendBack(text, stage)}/>{/if}
{#if yoloAsk}<ConfirmDialog title="Put this outcome on YOLO?" body="Its agents will act without asking you: in the session each pipeline has in hand now, and in every stage still to start. Commands run and files change with no approval from you. Each pipeline still stops at Ready to merge unless you have separately let it merge by itself on YOLO. You can switch back at any time." confirmLabel="Switch to YOLO" oncancel={() => yoloAsk = null} onconfirm={() => { const record = yoloAsk!; void setYolo(record, true, true) }}/>{/if}
{#if yoloMerge}<ConfirmDialog title={`Let “${yoloMerge.piece.title}” merge by itself on YOLO?`} body="This pipeline’s agent runs on YOLO: it acts without asking you. If it may also merge by itself, nothing stops for you between the code and the merge. It still merges only once every check passes, no review thread is open and there are no conflicts. A merge cannot be undone from here." confirmLabel="Let it merge by itself" oncancel={() => yoloMerge = null} onconfirm={() => { const { record, piece } = yoloMerge!; void mergeMode(record, piece, true, true) }}/>{/if}
{#if merging}<ConfirmDialog title={`Merge “${merging.piece.title}”?`} body="This tells the pipeline’s agent to merge its pull request once every check passes, no review thread is open and there are no conflicts. If any of those is not true it fixes that first or brings it back to you. A merge cannot be undone from here." confirmLabel="Approve merge" oncancel={() => merging = null} onconfirm={() => void approve()}/>{/if}
{#if managing}<WorkspaceDialog title="Look after this session" onclose={() => { managing = null; void load() }}><GafferGoal id={managing.goal.conversationId} sessionId={managing.goal.sessionId} {userId} {timeZone} {dateFormat} onchanged={load}/></WorkspaceDialog>{/if}

<style>
  /* The rail is the left column on a wide desktop: its saved width, or the 56 px strip while folded. Below that there is no rail. */
  .factory-page{--top:61px;display:grid;grid-template-columns:var(--rail-width,301px) minmax(0,1fr)}.factory-page.rail-collapsed{grid-template-columns:56px minmax(0,1fr)}
  .factory-page>:global(aside.rail){grid-column:1;grid-row:1;min-width:0}.factory-page>.factory{grid-column:2;grid-row:1}
  .factory-page :global(aside.rail .card:not(.chosen):not(.scoped):not(.picked)){border-color:transparent;border-radius:8px;background:transparent}.factory-page :global(aside.rail .card:not(.chosen):not(.scoped):not(.picked):hover){background:var(--line-soft)}
  .factory-page :global(aside.rail .strip){height:calc(100dvh - var(--top));min-height:0;overflow-y:auto;scrollbar-width:thin}.factory-page :global(aside.rail .strip>button){flex-shrink:0}
  @media(max-width:1099px){.factory-page{grid-template-columns:minmax(0,1fr)}.factory-page>.factory{grid-column:1}}
  .factory{--gutter:24px;display:grid;gap:20px;align-content:start;min-width:0;width:100%;max-width:1148px;box-sizing:border-box;margin:0 auto;padding:20px var(--gutter) calc(56px + var(--bottom-chrome,0px));font-size:14px;line-height:1.5}
  .projects{display:flex;align-items:center;flex-wrap:wrap;gap:8px 12px}
  .scope{display:flex;align-items:center;flex-wrap:wrap;gap:10px;color:var(--muted)}.scope strong{color:var(--ink)}.scope .btn{min-height:44px}
  p{margin:0;overflow-wrap:anywhere}h1{font:500 2rem var(--serif);margin:0}h2{font:500 1.35rem var(--serif);margin:0;overflow-wrap:anywhere}
  .hint,.foot,.counts{color:var(--muted)}.warning{color:var(--bad)}
  .top{display:flex;align-items:center;justify-content:space-between;gap:12px 20px;flex-wrap:wrap}.identity{display:flex;align-items:baseline;flex-wrap:wrap;gap:4px 18px;min-width:0;flex:1 1 320px}.counts{font-size:0.9375rem}
  .top-actions{display:flex;gap:8px;flex-wrap:wrap}.top-actions .btn{min-height:44px;text-decoration:none;display:inline-flex;align-items:center}
  /* What an action did, in the flow under the head: it never covers what it is about. */
  .notice{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;padding:10px 14px;border:1px solid var(--line);border-radius:10px;background:var(--card);box-shadow:var(--shadow)}.notice.bad{border-color:var(--bad);color:var(--bad)}.notice p{min-width:0}.notice .inline{flex:none}
  .setup{display:grid;grid-template-columns:auto 1fr;gap:16px;padding:16px;border:1px solid var(--line);border-radius:14px;background:var(--tint)}.setup-body{display:grid;gap:12px;min-width:0}.setup ol{list-style:none;margin:0;padding:0;display:grid;gap:10px}.setup li{display:grid;grid-template-columns:auto 1fr;gap:10px;align-items:start}.setup li p{color:var(--muted)}.tick{display:inline-grid;place-items:center;width:24px;height:24px;border-radius:50%;border:1px solid var(--line-strong);font-size:0.875rem;font-variant-numeric:tabular-nums}.setup li.done .tick{background:var(--accent);border-color:var(--accent);color:var(--accent-ink)}.setup a{color:var(--accent-text);font-weight:600}
  .empty{padding:28px 20px;border:1px dashed var(--line-strong);border-radius:14px;display:grid;gap:8px;text-align:center;justify-items:center}.empty p{color:var(--muted);max-width:56ch}
  /* The list, and beside it on a wide screen the chosen pipeline. Each scrolls by itself within the window, so the
     detail and its bar of actions stay in view while the list is read. */
  .board{display:grid;gap:20px;min-width:0}
  .list{display:grid;grid-template-columns:minmax(0,1fr);gap:20px;align-content:start;min-width:0}
  .none{padding:4px 4px 8px;color:var(--muted)}
  .floor{display:grid;gap:10px;min-width:0}
  /* In the list beside the detail the floor's filters wrap rather than scroll out of sight. */
  .split .floor :global(.filters){flex-wrap:wrap;overflow-x:visible}.foot{font-size:0.875rem}
  .factory.split{max-width:1440px;height:calc(100dvh - var(--top));display:flex;flex-direction:column;gap:16px;padding-bottom:20px}
  .split .board{flex:1 1 auto;min-height:420px;grid-template-columns:clamp(290px,34%,380px) minmax(0,1fr)}
  .split .list{min-height:0;overflow-y:auto;scrollbar-width:thin;padding:2px 10px 24px 2px;margin-left:-2px}
  .split .pane{min-height:0;overflow-y:auto;scrollbar-width:thin;box-sizing:border-box;padding:20px 24px 0;border:1px solid var(--line);border-radius:14px;background:var(--card);--detail-inset:24px}
  .pane.resting{display:grid;place-items:center;padding:24px;color:var(--muted);text-align:center}
  /* On a narrow screen the detail is the page, and its bar rests on whatever covers the foot of the screen. */
  .factory.reading{padding-bottom:var(--bottom-chrome,0px)}
  .reading .pane{--detail-inset:var(--gutter);--detail-bottom:var(--bottom-chrome,0px)}
  .inline{border:0;background:none;padding:0 2px;font:inherit;font-weight:600;color:var(--accent-text);cursor:pointer;min-height:24px;text-decoration:underline}.inline:focus-visible{outline:2px solid var(--accent);outline-offset:2px}@media(pointer:coarse){.inline{min-height:44px}}
  a:focus-visible,button:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:640px){.factory{--gutter:16px}.setup{grid-template-columns:1fr}.top-actions{width:100%}.top-actions .btn{flex:1 1 auto;justify-content:center}}
</style>
