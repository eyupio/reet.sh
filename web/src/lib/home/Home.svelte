<script lang="ts">
  import FleetPlanMeters from '../FleetPlanMeters.svelte'
  import { onMount, tick } from 'svelte'
  import { signalAttentionChanged } from '../attentionSignal'
  import { ApiError, getProfile, getSyncState, listGafferGoals, listGafferOutcomes, loadNewSkills, UnauthorizedError, type NewSkill, type SyncState, type GafferGoalOverviewView } from '../api'
  import { FACTORY_SESSION_END_DEFAULT, factoryMembers, factorySessionEndModeOf, type FactoryMember, type FactorySessionEndMode } from '../gafferOutcome'
  import { gafferChatEntry, GAFFER_GOALS_CHANGED } from '../gafferContext'
  import { createTerminal, folderProjectList, listProjectGroups, machineHealth, markSessionSeen, readiness, terminalMachines, folderProjectForLegacy, sessionHref, terminalSessions, TerminalError,
    type FolderProject, type MachineHealth, type ProjectGroup, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { notStartedText, refusalText, titleFor } from '../agentLaunch'
  import { hasPending } from '../attention'
  import { announceWaiting, fleetCards, handedProjects, reporting, inFlight, projectRunning, projectWorktrees, sessionOwners, type FleetCard, type GroupBy, type HomeSession } from '../home'
  import { clampRailWidth, RAIL_DEFAULT, readWidth } from '../shell/workspaceLayout'
  import type { LaunchDefaults } from '../launchBar'
  import StartAgentDialog from '../StartAgentDialog.svelte'
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'
  import FleetStrip from './FleetStrip.svelte'
  import InFlight from './InFlight.svelte'
  import SessionBoard from './SessionBoard.svelte'
  import SessionWaitingItem from './SessionWaitingItem.svelte'
  import ListSection from '../factory/ListSection.svelte'
  import WaitingItem from '../factory/WaitingItem.svelte'
  import StagePipeline from '../factory/StagePipeline.svelte'
  import ProjectPicker from '../factory/ProjectPicker.svelte'
  import PipelineChoices, { usable } from '../factory/PipelineChoices.svelte'
  import { inSection, stageCaption, type FactoryItem } from '../factory/factoryList'
  import { readSections, writeSections } from '../factory/sectionMemory'
  import { waitingSummary, workingSummary, type WaitingEntry } from './homeWaiting'
  import { whatWaits } from './whatWaits'
  import { deskPaneReader, offDesks, type DeskRef } from './deskSessions'
  import { getDesk, listDesks } from '../desks/api'
  import { keepFocusIn } from '../shell/keepFocus'
  import { short } from '../gafferOutcome'
  import type { GafferOutcome as Outcome, GafferOutcomeSession } from '../api'
  import ProjectRailHost from './ProjectRailHost.svelte'
  import NewSessionChoice, { type NewSessionKind } from '../terminal/NewSessionChoice.svelte'
  import LaunchDialog from './LaunchDialog.svelte'
  import StartWorkbench from './StartWorkbench.svelte'
  import GafferOutcome from '../GafferOutcome.svelte'

  // Writers get the persistent session board and project rail. New agents start from the standard Start an agent modal;
  // the launch bar opens in a modal for Select and an address that hands over projects.
  // SessionBoard owns backlog reads; these loaders supply sessions and the fleet.
  // Both refresh every 15 seconds and retain the last successful read on failure.
  let { userId, writer, canAdd, accounts, moved = false, legacyProject = '', timeZone = '', dateFormat = 'locale', onsignedout, onattention }: {
    userId: number
    /** The person's time zone and date format, for the dates in a pipeline's Goal and limits dialog. */
    timeZone?: string
    dateFormat?: string
    name?: string
    /** A writer in accounts mode: the full Home. Otherwise the read-only Home. */
    writer: boolean
    /** Whether this sign-in can add a machine. */
    canAdd: boolean
    accounts: boolean
    /** The page was opened at a retired /projects address: say once, for this page load, where projects live now. */
    moved?: boolean
    /** The saved Project id an old /projects/<id> address named: its folder project is looked up and chosen. */
    legacyProject?: string
    onsignedout: (e: unknown) => void
    /** What waits for the person (Factory items, sessions and Gaffer's goals), for the bell and the document title. */
    onattention?: (entries: WaitingEntry[], onseen: (sessionId: string) => Promise<void>, onanswered: () => Promise<void>) => void
  } = $props()

  const POLL_MS = 15_000
  // The Machines page opens Add a machine (or the connect steps) when it arrives with this fragment.
  const machinesAt = (fragment: string) => `/machines#${fragment}`

  let health = $state<MachineHealth[] | undefined>()
  let sessions = $state<HomeSession[] | undefined>()
  let sync = $state<SyncState | undefined>()
  let projects = $state<FolderProject[] | undefined>()
  // The rail's groups, in their saved order; a project names its group by id.
  let groups = $state<ProjectGroup[] | undefined>()
  // The project or group the board is narrowed to (null: everything).
  let scope = $state<{ kind: 'project' | 'group'; id: string } | null>(null)
  // The project list starts open; a fold is kept in this browser.
  let railCollapsed = $state((() => {
    try { return localStorage.getItem('reet.rail.collapsed') === '1' } catch { return false }
  })())
  // The rail's width on a wide desktop, also remembered in this browser.
  const savedRailWidth = (() => {
    try { return readWidth(localStorage.getItem('reet.rail.width'), RAIL_DEFAULT, width => clampRailWidth(width, innerWidth)) }
    catch { return RAIL_DEFAULT }
  })()
  let railWidth = $state(savedRailWidth)
  const setRailWidth = (width: number) => { railWidth = clampRailWidth(width, innerWidth) }
  const commitRailWidth = (width: number) => {
    railWidth = clampRailWidth(width, innerWidth)
    try { localStorage.setItem('reet.rail.width', String(railWidth)) } catch { /* The rail keeps its width for this page. */ }
  }
  // The terminal machines (with readiness and launch capabilities), read with the rest for Start an agent and every start.
  let terminals = $state<TerminalMachine[] | undefined>()
  let savedWaiting = $state(0)
  // The project an address chose (/?project=<id>): its card is marked and the board is narrowed to it.
  let chosen = $state(new URLSearchParams(location.search).get('project') ?? '')
  // The account's launch defaults (base branch, worktrees by default), read once; start dialogs wait for the answer or its failure.
  let profile = $state<LaunchDefaults>({}), profileRead = $state(false)
  // The account's choice for a session that ends: remove it from history too. Read with the launch defaults.
  let removeDefault = $state(false)
  // The account's choice for a Factory outcome when the session working in it is stopped.
  let factorySessionEndDefault = $state<FactorySessionEndMode>(FACTORY_SESSION_END_DEFAULT)
  // The launch bar's modal, for Select and an address that hands Home projects.
  let launchOpen = $state(false)
  // The standard Start an agent dialog, opened from the rail's + or a machine's New session.
  let agentStart = $state<{ opener: HTMLElement | null; project?: FolderProject; agentId?: string } | null>(null)
  let factoryStart = $state<{ opener: HTMLElement } | null>(null)
  let board = $state<SessionBoard>(), planningButton = $state<HTMLButtonElement>()
  let backlogCount = $state<number | undefined>()
  let now = $state(Date.now())
  let problem = $state('')
  // The last read of the Factory or of Gaffer's goals failed: the list may lack what waits there, and says so.
  let factoryFailed = $state(false)
  let followups = $state<GafferGoalOverviewView | undefined>()
  // Which sessions are part of a Factory pipeline, from the same read as the pipelines below.
  let factory = $state<Map<string, FactoryMember>>(new Map())
  // The Factory's pipelines that are not archived: the ones waiting for the person join Home's list, the working ones
  // have a section of their own.
  let factoryRecords = $state<Outcome[]>([])
  // The skills New on a machine: one Waiting for you entry opens their review on the Skills page.
  let newSkills = $state<NewSkill[]>([])
  // The desks, read with the rest: a session on a desk leaves Continue working, the board and the head's count of open
  // sessions, and when it needs the person it stays in Waiting for you, marked with its desk. A failed read keeps the desks
  // last read and says nothing (Home works without them); until one has been read, no session counts as a desk session.
  let desks = $state<DeskRef[] | undefined>()
  // The pane each waiting desk session is in, so its link opens the desk there (read only for those desks).
  let deskPanes = $state<Map<string, string>>(new Map())
  const readPanes = deskPaneReader(getDesk)
  let readOnce = false
  const life = new AbortController()
  const bounded = () => AbortSignal.any([life.signal, AbortSignal.timeout(POLL_MS)])

  const emptySync: SyncState = { environments: [], targets: [], agents: [], jobs: [] }
  const cards = $derived(fleetCards(health ?? [], sessions ?? [], sync ?? emptySync, now, terminals ?? []))
  const owners = $derived(sessionOwners(sessions ?? [], projects ?? []))
  const worktrees = $derived(projectWorktrees(sessions ?? []))
  const machineReporting = $derived(new Map(cards.map(c => [c.agentId, reporting(c)])))
  // What waits for the person, by the one rule every page uses (whatWaits): Factory items, sessions and Gaffer's goals
  // together, oldest first. The bell and the title count all of it; the page shows what is in the chosen project or group.
  const waits = $derived(whatWaits(sessions ?? [], now, owners, health ?? [], factoryRecords, followups?.goals, { desks, panes: deskPanes }, newSkills))
  // Home's session lists leave desk sessions out (a Factory pipeline's session is its Factory item, and stays as it is).
  const listed = $derived(sessions && offDesks(sessions, desks, new Set(factory.keys())))
  // The Projects panel counts what Home lists: a session on a desk is left out, as it is from the lists (`listed`).
  const running = $derived(projectRunning(listed ?? []))
  const needs = $derived(waits.cards)
  const items = $derived(waits.items)
  const allWaiting = $derived(waits.entries)
  const byConversation = $derived(new Map((followups?.goals ?? []).map(row => [row.goal.conversationId, row])))
  const pairs = $derived(new Map<string, { record: Outcome; piece: GafferOutcomeSession }>(factoryRecords.flatMap(record => record.sessions.map(piece => [`${record.id}.${piece.key}`, { record, piece }] as const))))
  const sessionProject = $derived(new Map((sessions ?? []).map(s => [s.id, s.projectId ?? ''])))
  const projectOf = (entry: WaitingEntry) => entry.source === 'factory' ? entry.item.projectId : entry.source === 'skills' ? '' : sessionProject.get(entry.source === 'session' ? entry.card.sessionId : entry.goal.sessionId) ?? ''
  const inScope = (projectId: string) => scopedIds === undefined || scopedIds.includes(projectId)
  // New skills belong to no project: their entry shows whatever the page is narrowed to.
  const waiting = $derived(allWaiting.filter(entry => entry.source === 'skills' || inScope(projectOf(entry))))
  const working = $derived(inSection(items, 'working').filter(item => inScope(item.projectId)))
  // The list shows ten at first and ten more each time.
  const WAITING_PAGE = 10
  let waitingLimit = $state(WAITING_PAGE)
  // A new project or group starts the list at its first ten again.
  const scopeKey = $derived(scope ? `${scope.kind}:${scope.id}` : '')
  $effect(() => { void scopeKey; waitingLimit = WAITING_PAGE })
  // What waits and works in each project, whatever the page is narrowed to, for the Projects button of a narrow screen.
  const projectCounts = $derived.by(() => {
    const counts = new Map<string, { waiting: number; working: number }>()
    const count = (id: string) => { const found = counts.get(id) ?? { waiting: 0, working: 0 }; counts.set(id, found); return found }
    for (const entry of allWaiting) if (entry.source !== 'skills') count(projectOf(entry)).waiting++
    for (const item of inSection(items, 'working')) count(item.projectId).working++
    return counts
  })
  const LIVE_STATES = new Set(['creating', 'running', 'end_requested'])
  const openSessions = $derived((listed ?? []).filter(s => LIVE_STATES.has(s.state) && inScope(s.projectId ?? '')).length)
  const countsLine = $derived([
    `${waiting.length} waiting for you`,
    working.length ? `${working.length} ${working.length === 1 ? 'pipeline' : 'pipelines'} working` : '',
    openSessions ? `${openSessions} ${openSessions === 1 ? 'session' : 'sessions'} open` : '',
  ].filter(Boolean).join(' · '))
  const connected = $derived(cards.filter(reporting))

  // Which sections are open, remembered in this browser for this person. The first time, only Waiting for you is.
  const SECTIONS_FIRST = { waiting: true }
  const storage = () => { try { return localStorage } catch { return undefined } }
  const sectionsKey = $derived(`reet.home.sections.${userId}`)
  let opened = $state<Record<string, boolean>>({ ...SECTIONS_FIRST }), openedFor = ''
  $effect.pre(() => { if (openedFor !== sectionsKey) { openedFor = sectionsKey; opened = readSections(storage(), sectionsKey, SECTIONS_FIRST) } })
  function toggleSection(name: string) {
    opened = { ...opened, [name]: !opened[name] }
    writeSections(storage(), sectionsKey, opened)
  }
  // With nothing waiting the section stays open to say so, whatever was remembered. Closing it then holds for this visit.
  let waitingShut = $state(false)
  const waitingOpen = $derived(!!opened.waiting || (!waiting.length && !waitingShut))
  function toggleWaiting() {
    if (waitingOpen) { waitingShut = true; if (opened.waiting) toggleSection('waiting') }
    else { waitingShut = false; toggleSection('waiting') }
  }

  // A pipeline's choices are carried out as on the Factory page, with the same dialogs; Details opens it there.
  let choices = $state<PipelineChoices>(), busy = $state(false)
  const latest = (record: Outcome) => factoryRecords.find(item => item.id === record.id) ?? record
  const pipelineHref = (item: FactoryItem) => `/factory?pipeline=${encodeURIComponent(item.id)}`

  // The entry of Waiting for you that last held the focus. When an answer, Mark seen or a choice takes it away, the focus
  // goes to the entry now in its place, else the one before, else the section's head (keepFocusIn), never to the body.
  let waitingList = $state<HTMLUListElement>(), focusedAt = -1
  // Focus anywhere else (the bell, a dialog) forgets it: what is done there keeps the focus there.
  const noteFocus = (event: FocusEvent) => {
    const item = event.target instanceof Element && waitingList?.contains(event.target) ? event.target.closest('.waiting-list > li') : null
    focusedAt = item?.parentElement ? [...item.parentElement.children].indexOf(item) : -1
  }
  async function keepWaitingFocus() {
    await tick()
    if (focusedAt >= 0) keepFocusIn(waitingList, focusedAt, document.querySelector<HTMLElement>('section[aria-label="Waiting for you"] button[aria-expanded]'))
  }

  async function markSeen(sessionId: string) {
    sessions = (sessions ?? []).map(s => s.id === sessionId ? { ...s, changedSinceSeen: false, attention: s.attention === 'waiting' && !hasPending(s) ? 'working' : s.attention } : s)
    // The entry leaves at once; the focus does not fall to the body meanwhile.
    void keepWaitingFocus()
    try {
      await markSessionSeen(sessionId, bounded())
      signalAttentionChanged()
      await load(true)
      await keepWaitingFocus()
    } catch (e) {
      if (refused(e, true)) { life.abort(); onsignedout(new UnauthorizedError()) }
    }
  }

  const answered = async () => { await load(true); await keepWaitingFocus() }

  $effect(() => { onattention?.(allWaiting, markSeen, answered) })

  // Polite live region for transitions into Waiting for you, throttled to at most once per session per minute.
  let waitingNotice = $state('')
  let previousWaiting: Set<string> | null = null
  const lastWaitingAnnounced = new Map<string, number>()
  $effect(() => {
    if (sessions === undefined) return
    const currentWaiting = new Set(needs.filter(c => c.kind === 'waiting').map(c => c.sessionId))
    if (previousWaiting !== null) {
      const text = announceWaiting(needs, previousWaiting, lastWaitingAnnounced, Date.now())
      if (text) waitingNotice = text
    }
    previousWaiting = currentWaiting
  })

  // Only a refusal of the credential this sign-in uses means it is signed out: the token rides the sync read, the cookie (an
  // accounts-mode sign-in) every read. The health read is cookie-only, so a single-token or open instance never makes it.
  const refused = (e: unknown, cookie: boolean) => e instanceof UnauthorizedError || (cookie && accounts && e instanceof TerminalError && e.status === 401)
  // Machines from the health report when this sign-in can read it; otherwise (a single token, an open instance, or an account
  // the health read refuses with 403) from the sync agents, connected when the agent reported in the last 90 seconds.
  // svelte-ignore state_referenced_locally
  let healthReadable = accounts
  const fromAgents = (s: SyncState, at: number): MachineHealth[] =>
    (s.agents ?? []).map(a => ({ agentId: a.id, host: a.host, state: a.lastSeen > at - 90_000 ? 'connected' : 'never_connected' }))
  let sessionsFailed = $state(false), projectsFailed = $state(false), groupsFailed = $state(false)

  // Each load has a generation. A poll is skipped while another load runs; an explicit refresh (after a start or a save)
  // supersedes it, and an answer from an older load is dropped so it never overwrites a newer list.
  let loading = false, generation = 0
  async function load(explicit = false) {
    if (life.signal.aborted || (loading && !explicit)) return
    const mine = ++generation
    const stale = () => life.signal.aborted || mine !== generation
    loading = true
    const signOut = () => { life.abort(); onsignedout(new UnauthorizedError()) }
    try {
      const [h, y, s, p, g, t, f, o, d, n] = await Promise.allSettled([
        healthReadable ? machineHealth(bounded()) : Promise.resolve(undefined),
        getSyncState(bounded()),
        writer ? terminalSessions(bounded()) : Promise.resolve(undefined),
        writer ? folderProjectList(bounded()) : Promise.resolve(undefined),
        writer ? listProjectGroups(bounded()) : Promise.resolve(undefined),
        // The launch bar's machines: readiness and launch capabilities change while Home stays open, so they are read with the rest.
        writer ? terminalMachines(bounded()) : Promise.resolve(undefined),
        writer && accounts ? listGafferGoals(bounded()) : Promise.resolve(undefined),
        // The Factory's pipelines, read with the sessions so a pipeline's session is never shown twice while one is late.
        // A failed read keeps the last pipelines and says so; 403 and 404 are an account without Gaffer, and stay quiet.
        writer && accounts ? listGafferOutcomes(bounded()) : Promise.resolve(undefined),
        writer ? listDesks(bounded()) : Promise.resolve(undefined),
        // Never a problem line: a failed read keeps the new skills as last read.
        writer ? loadNewSkills(bounded()) : Promise.resolve(undefined),
      ])
      if (stale()) return
      if ([h, y, s, p, g, t, f, o].some(r => r.status === 'rejected' && refused(r.reason, r !== y))) return signOut()
      const at = Date.now()
      let failed = false
      factoryFailed = false
      // An account without Gaffer is refused (403) or has no such read (404): nothing went wrong.
      const missed = (r: PromiseSettledResult<unknown>) => r.status === 'rejected' && !(r.reason instanceof ApiError && (r.reason.status === 403 || r.reason.status === 404))
      if (y.status === 'fulfilled') sync = y.value; else failed = true
      if (h.status === 'rejected' && h.reason instanceof TerminalError && h.reason.status === 403 && health === undefined) healthReadable = false
      if (h.status === 'fulfilled' && h.value) health = h.value.machines ?? []
      else if (!healthReadable) { if (y.status === 'fulfilled') health = fromAgents(y.value, at) }
      // A health read that failed for a passing reason keeps the cards as last read.
      else failed = true
      if (writer) {
        if (accounts) {
          if (f.status === 'fulfilled' && f.value) followups = f.value
          if (o.status === 'fulfilled' && o.value) { const floor = o.value.filter(record => !record.archivedAt); factory = factoryMembers(floor); factoryRecords = floor }
          factoryFailed = missed(f) || missed(o)
        }
        if (s.status === 'fulfilled' && s.value) { sessions = s.value; sessionsFailed = false } else { failed = true; sessionsFailed = sessions === undefined }
        if (p.status === 'fulfilled' && p.value) { projects = p.value.projects; savedWaiting = p.value.savedWaiting ?? 0; projectsFailed = false } else { failed = true; projectsFailed = projects === undefined }
        if (g.status === 'fulfilled' && g.value) { groups = g.value; groupsFailed = false } else { failed = true; groupsFailed = groups === undefined }
        if (t.status === 'fulfilled' && t.value) terminals = t.value; else failed = true
        // Never a problem line nor a sign-out: without the desks Home shows every session, as it did before desks.
        if (d.status === 'fulfilled' && d.value) desks = d.value
        if (n.status === 'fulfilled' && n.value) newSkills = n.value
      }
      now = at
      problem = failed ? (readOnce ? "Couldn't refresh just now. Showing what was last read." : "Couldn't read everything just now. Home tries again every 15 seconds.")
        : factoryFailed ? (readOnce ? 'The Factory could not be read; showing what was last read.' : 'The Factory could not be read. Home tries again every 15 seconds.') : ''
      readOnce = true
      if (writer && desks) {
        // The panes of the desks holding a session that waits (each desk read once per version).
        const wanted = new Set(waits.entries.flatMap(entry => entry.source === 'session' && entry.desk ? [entry.card.sessionId] : entry.source === 'gaffer' && entry.desk ? [entry.goal.sessionId] : []))
        const panes = await readPanes(desks, wanted, bounded())
        if (!stale()) deskPanes = panes
      }
    } finally {
      if (mine === generation) loading = false
    }
  }

  // Tablet and phone widths leave the rail to the Sessions screen (the phone rail, PR H): Home keeps machines, Needs you,
  // the board and the start modal there.
  const NARROW = '(max-width: 1099px)'
  let narrow = $state(typeof matchMedia === 'function' && matchMedia(NARROW).matches)
  // Below 600 px a waiting card's stages are circles with a caption; from 600 px each stage is named.
  const PHONE = '(max-width: 599px)'
  let phone = $state(typeof matchMedia === 'function' && matchMedia(PHONE).matches)

  let homeView = $state<'board' | 'continue'>('board')
  let homeViewChosen = $state(false)
  let continueGroup = $state<GroupBy>('project')
  function chooseHomeView(view: 'board' | 'continue') {
    homeViewChosen = true
    homeView = view
    try { localStorage.setItem(`reet.home.view.${userId}`, view) } catch { /* The view still works for this visit. */ }
  }
  const continueSessions = $derived((listed ?? []).filter(s => scopedIds === undefined || scopedIds.includes(s.projectId ?? '')))
  const continueGroups = $derived(inFlight(continueSessions, owners, health ?? [], continueGroup, now))
  const continueOffline = $derived((health ?? []).filter(m => m.state !== 'connected' && continueSessions.some(s => s.agentId === m.agentId)).map(m => ({agentId: m.agentId, host: m.host})))
  const projectColours = $derived(new Map((projects ?? []).map(p => [p.id, p.colour])))
  const showWorkbench = $derived(sessions !== undefined && !sessionsFailed && !scope && continueGroups.length === 0)
  const workbenchVisible = $derived(homeView === 'continue' && showWorkbench)
  // A genuinely empty account starts with useful choices. Existing work, explicit view preferences and project handoffs
  // keep their view; a failed backlog read is not mistaken for an empty one. This automatic choice is never persisted.
  $effect(() => {
    if (!homeViewChosen && showWorkbench && backlogCount === 0 && !chosen && !handed) homeView = 'continue'
  })
  function startFromWorkbench(kind: NewSessionKind, opener: HTMLElement) {
    notice = null
    if (kind === 'factory') { factoryStart = { opener }; return }
    if (kind === 'agent') { startNewSession(opener); return }
    projectStart = { opener }
    chooseProjectStart('terminal')
  }
  async function planFromWorkbench() {
    chooseHomeView('board')
    await tick()
    // The board tab remains mounted after the launcher goes, so closing the composer has a visible focus destination.
    if (planningButton) board?.planSession(planningButton)
  }
  async function closeFactoryStart() {
    const opener = factoryStart?.opener
    factoryStart = null
    await load(true)
    await tick()
    if (opener?.isConnected) opener.focus()
  }

  let notice = $state<{ text: string; href?: string; bad?: boolean } | null>(null)
  // The board's scope: a project shows its sessions, a group its projects' sessions.
  const scopedIds = $derived.by(() => {
    if (!scope) return undefined
    if (scope.kind === 'project') return [scope.id]
    const group = (groups ?? []).find(g => g.id === scope?.id)
    if (!group) return [] as string[]
    return (projects ?? []).filter(p => (p.groupId ?? '') === group.id).map(p => p.id)
  })
  const scopeName = $derived(scope ? (scope.kind === 'project' ? (projects ?? []).find(p => p.id === scope?.id)?.name : (groups ?? []).find(g => g.id === scope?.id)?.name) ?? 'Selection' : '')
  // A removal or a fresh read that no longer names the chosen project or group ends the scope.
  $effect(() => {
    if (!scope || !projects) return
    if (scope.kind === 'project' && !projects.some(p => p.id === scope?.id)) scope = null
    else if (scope.kind === 'group' && groups && !groups.some(g => g.id === scope?.id)) scope = null
  })
  // An address that names a project (/?project=<id>) opens the board on it, once the list is read.
  let chosenScoped = false
  $effect(() => {
    if (chosenScoped || !chosen || !projects) return
    chosenScoped = true
    if (projects.some(p => p.id === chosen)) scope = { kind: 'project', id: chosen }
  })
  function setRailCollapsed(next: boolean) {
    railCollapsed = next
    try { localStorage.setItem('reet.rail.collapsed', next ? '1' : '0') } catch { /* The rail still folds. */ }
  }

  // The rail and what its cards do (ProjectRailHost); the launch bar's project slot opens its Add project too.
  let host = $state<ProjectRailHost>()
  function openAdd(opener: HTMLElement) { void host?.openAdd(opener) }
  // Choosing a card marks it as the project the board's Start a session dialog starts in too.
  let launchProject = $state<string | null>(null)
  function chooseProject(project: FolderProject) { launchProject = project.id }
  // Select in the rail: the chosen projects, in rail order, are the launch bar's projects (Start in several projects). Only
  // this page holds them; nothing goes to browser storage. The Sessions screen (a tablet or phone, where the rail lives)
  // hands its choice over in the address, as project ids only (`/?projects=<id>,<id>`).
  let selecting = $state(false), selected = $state<string[]>([])
  const handed = new URLSearchParams(location.search).get('projects')
  const inRailOrder = (ids: string[]) => (projects ?? []).filter(p => ids.includes(p.id)).map(p => p.id)
  function setSelecting(on: boolean, first?: FolderProject) {
    selecting = on
    selected = on && first ? [first.id] : []
  }
  function toggleSelected(project: FolderProject) {
    selected = inRailOrder(selected.includes(project.id) ? selected.filter(id => id !== project.id) : [...selected, project.id])
  }
  // A project removed while chosen leaves the selection.
  $effect(() => {
    const list = projects
    if (!list || selected.every(id => list.some(p => p.id === id))) return
    selected = inRailOrder(selected)
  })
  // The projects the Sessions screen chose, once the list that names them has been read (handedProjects: known ids only, once
  // each, at most 12). The address then drops `projects`, keeping the rest of it, so a reload does not choose them again.
  let handedApplied = false
  $effect(() => {
    if (handedApplied || !projects || handed === null) return
    handedApplied = true
    const ids = handedProjects(handed, projects)
    if (ids.length) { selecting = true; selected = ids; launchOpen = true }
    const url = new URL(location.href)
    url.searchParams.delete('projects')
    history.replaceState(history.state, '', `${url.pathname}${url.search}${url.hash}`)
  })
  // A write's answer is the list as it now is (or a project in it): it replaces what was read, superseding a poll in flight.
  function settle(next: FolderProject[]) { generation++; loading = false; projects = next }
  function settleGroups(next: ProjectGroup[]) { generation++; loading = false; groups = next }
  // An old /projects/<id> address: its folder project, if one was made from it, is chosen and named in the address.
  async function findLegacy(id: string) {
    try {
      const found = await folderProjectForLegacy(id, bounded())
      if (!found || life.signal.aborted) return
      // An old /projects/<id> handoff chooses the project; the board is narrowed to it.
      chosen = found
      history.replaceState(history.state, '', `/?project=${encodeURIComponent(found)}`)
    } catch { /* no folder project was made from it: Home stays at / */ }
  }

  // A start goes straight to the new session, at the address the Machines and Terminals pages use. The row is added first, so
  // Home has it if the browser brings this page back from its cache on Back; the bar stays locked until the page goes.
  function started(session: TerminalSession): 'leaving' {
    sessions = [session, ...(sessions ?? []).filter(s => s.id !== session.id)]
    location.assign(sessionHref(session))
    return 'leaving'
  }
  // The next page never arrived (the bar let go after a few seconds): say what started, with the way to it.
  function leaveFailed(session: TerminalSession) {
    const host = health?.find(m => m.agentId === session.agentId)?.host ?? terminals?.find(m => m.agentId === session.agentId)?.host ?? session.agentId
    notice = { text: `Started ${session.title} on ${host}.`, href: sessionHref(session) }
    void load(true)
  }

  // A machine's + offers what to start there: a session (which asks which project on it, or a terminal), a new project on
  // it, or a plain terminal. A project then opens Start an agent, so its defaults, refusals and navigation are the same as
  // the modal's Start everywhere else in the app.
  function newSessionOn(card: FleetCard, opener: HTMLElement) { notice = null; projectStart = { opener, agentId: card.agentId, machine: card.name } }
  // New project opens the rail's Add project dialog with this machine already chosen.
  function newProjectOn(card: FleetCard, opener: HTMLElement) { notice = null; void host?.openAdd(opener, card.agentId) }
  function startProjectNow(project: FolderProject, opener?: HTMLElement | null) {
    const from = opener ?? null
    if (!terminals) { notice = { text: 'Home is still opening. Try again in a moment.', bad: true }; return }
    agentStart = { opener: from, project }
  }
  // A project's + asks what to start in it: an agent session, a terminal in its folder, or an outcome for the Factory.
  // New session and a machine's + ask the same, for the whole account or for that machine.
  let projectStart = $state<{ project?: FolderProject; agentId?: string; machine?: string; opener: HTMLElement | null } | null>(null)
  // A terminal with no machine named, on an account with several: which machine it opens on.
  let terminalPick = $state<{ opener: HTMLElement | null } | null>(null)
  function chooseProjectStart(kind: NewSessionKind) {
    const target = projectStart
    projectStart = null
    if (!target) return
    const { project, agentId, opener } = target
    if (kind === 'factory') { location.assign(`/factory?outcome=1${project ? `&project=${encodeURIComponent(project.id)}` : ''}`); return }
    if (kind === 'agent') {
      if (project) { startProjectNow(project, opener); return }
      if (!terminals) { notice = { text: 'Home is still opening. Try again in a moment.', bad: true }; return }
      agentStart = { opener, agentId }
      return
    }
    if (project) { void startTerminalOn(project.host, opener, project); return }
    if (agentId) { void startTerminalOn(agentId, opener); return }
    if (!terminals) { notice = { text: 'Home is still opening. Try again in a moment.', bad: true }; return }
    if (terminals.length === 1) { void startTerminalOn(terminals[0].agentId, opener); return }
    if (!terminals.length) { notice = { text: 'Connect a machine first: a terminal session runs on one of your machines.', bad: true }; opener?.focus(); return }
    terminalPick = { opener }
  }
  // New session on the board starts now, in the same Start an agent dialog as the rail and the machine cards.
  function startNewSession(opener: HTMLElement) {
    if (!terminals) { notice = { text: 'Home is still opening. Try again in a moment.', bad: true }; return }
    notice = null
    agentStart = { opener }
  }
  // A terminal session is a shell on the machine: no agent, no prompt, no project. It goes straight to the new session.
  // A request whose reply was lost is kept per machine and sent again with the same request id, exactly as the launch
  // bar's Start does, so choosing New terminal twice can never make a second shell.
  type ShellRequest = { requestId: string; title: string }
  let shellRequests = $state<Record<string, ShellRequest>>({})
  let shellStarting = ''
  function forgetShell(key: string) { const next = { ...shellRequests }; delete next[key]; shellRequests = next }
  // With a project the shell opens in its folder and belongs to it; its request is kept apart from the machine's own.
  async function startTerminalOn(agentId: string, opener?: HTMLElement | null, project?: FolderProject) {
    if (shellStarting) return
    const why = readiness(terminals?.find(m => m.agentId === agentId))
    if (why) { notice = { text: why, bad: true }; opener?.focus(); return }
    const key = project ? `${agentId}:${project.id}` : agentId
    const request = shellRequests[key] ?? { requestId: crypto.randomUUID(), title: titleFor('', project?.dir ?? '') }
    if (!shellRequests[key]) shellRequests = { ...shellRequests, [key]: request }
    shellStarting = key
    notice = null
    try {
      const session = await createTerminal(agentId, request.requestId, request.title, 100, 30, bounded(), project ? { dir: project.dir, projectId: project.id } : undefined)
      if (life.signal.aborted) return
      const notStarted = notStartedText(session.state)
      if (notStarted) { forgetShell(key); notice = { text: notStarted, bad: true }; return }
      forgetShell(key)
      sessions = [session, ...(sessions ?? []).filter(s => s.id !== session.id)]
      location.assign(sessionHref(session))
    } catch (e) {
      if (life.signal.aborted) return
      if (e instanceof UnauthorizedError || (e instanceof TerminalError && e.status === 401)) { life.abort(); onsignedout(new UnauthorizedError()); return }
      if (e instanceof TerminalError && [400, 403, 404, 409, 422].includes(e.status)) {
        forgetShell(key)
        notice = { text: refusalText(e.status, e.code, e.message), bad: true }
      } else {
        notice = { text: "We didn't hear back, so the terminal may have started. Choosing New terminal again asks about the same request.", bad: true }
      }
    } finally { shellStarting = '' }
  }

  // The redirect note goes into its live region after the page is drawn, so it is announced.
  let movedShown = $state(false)
  onMount(() => {
    try {
      railCollapsed = localStorage.getItem('reet.rail.collapsed') === '1'
      const savedView = localStorage.getItem(`reet.home.view.${userId}`)
      if (savedView === 'continue' || savedView === 'board') { homeView = savedView; homeViewChosen = true }
    } catch { /* Keep projects compact when storage is unavailable. */ }
    // The Sessions page's New session (and a project card's New session here) arrive at /#launch: the bar opens in its modal.
    const launchAtHash = () => { if (location.hash === '#launch') launchOpen = true }
    launchAtHash()
    window.addEventListener('hashchange', launchAtHash)
    const goalsChanged = () => { void load(true) }
    window.addEventListener(GAFFER_GOALS_CHANGED, goalsChanged)
    if (moved) void tick().then(() => { movedShown = true })
    void load()
    // The profile read gives up after 10 seconds, so the bar never waits on it for good (it then uses each project's own base).
    // Once Home has gone, a late answer changes nothing and the timer is cleared.
    let giveUpTimer: ReturnType<typeof setTimeout> | undefined
    if (writer) Promise.race([getProfile(), new Promise<never>((_, no) => { giveUpTimer = setTimeout(() => no(new Error('timeout')), 10_000) })])
      .then(p => { if (!life.signal.aborted) { profile = { defaultBaseBranch: p.profile.defaultBaseBranch, worktreesByDefault: p.profile.worktreesByDefault, defaultTool: p.profile.defaultTool, defaultModel: p.profile.defaultModel, defaultEffort: p.profile.defaultEffort }; removeDefault = p.profile.terminalRemoveOnClose ?? false; factorySessionEndDefault = factorySessionEndModeOf(p.profile.factorySessionEndMode) } }, () => {})
      .finally(() => { clearTimeout(giveUpTimer); if (!life.signal.aborted) profileRead = true })
    if (writer && legacyProject) void findLegacy(legacyProject)
    const query = matchMedia(NARROW), small = matchMedia(PHONE), resized = () => { narrow = query.matches; phone = small.matches }
    query.addEventListener('change', resized); small.addEventListener('change', resized)
    // The poll holds while a pipeline's goal or merge dialog is open, as the Factory page's does.
    const timer = setInterval(() => { if (document.visibilityState === 'visible' && !choices?.holdsRefresh()) void load() }, POLL_MS)
    const shown = () => { if (document.visibilityState === 'visible') void load() }
    document.addEventListener('visibilitychange', shown)
    // Back from the session the person just started: a page restored from the browser's cache shows lists as they were, so
    // it reads everything again. (The launch bar unlocks itself.)
    const restored = (e: PageTransitionEvent) => { if (e.persisted) void load(true) }
    window.addEventListener('pageshow', restored)
    return () => { clearTimeout(giveUpTimer); query.removeEventListener('change', resized); small.removeEventListener('change', resized); clearInterval(timer); document.removeEventListener('visibilitychange', shown); window.removeEventListener('pageshow', restored); window.removeEventListener('hashchange', launchAtHash); window.removeEventListener(GAFFER_GOALS_CHANGED, goalsChanged); life.abort() }
  })
</script>

<svelte:window onfocusin={noteFocus} />

<!-- One thing waiting: a Factory pipeline with its choices, or a session. A row on a wide screen, a card on a narrow one. -->
{#snippet waitingEntry(entry: WaitingEntry)}
  {#if entry.source === 'factory'}{@const pair = pairs.get(entry.id)}{#if pair}
    <WaitingItem item={entry.item} layout={narrow ? 'card' : 'row'} pipeline={narrow ? (phone ? 'circles' : 'labelled') : 'dots'} {busy}
      permission={entry.approval} sessionId={pair.piece.sessionId ?? ''} usable={choice => usable(choice, pair.piece)}
      onchoose={choice => choices?.choose(choice, pair.record, pair.piece)} detailsHref={pipelineHref(entry.item)} ondetails={() => location.assign(pipelineHref(entry.item))} onanswered={answered}/>
  {/if}
  {:else if entry.source === 'session'}<SessionWaitingItem card={entry.card} desk={entry.desk?.name} layout={narrow ? 'card' : 'row'} onseen={markSeen} onanswered={answered}/>
  {:else if entry.source === 'skills'}
    <a class="new-skills" href={entry.href}>
      <span class="new-skills-kind">Skills</span>
      <span class="new-skills-text">{entry.text}</span>
      <span class="new-skills-ask">Keep or archive them on the Skills page</span>
    </a>
  {:else}<SessionWaitingItem goal={entry.goal} desk={entry.desk?.name} layout={narrow ? 'card' : 'row'}/>{/if}
{/snippet}

<div class="home" class:writer class:rail-collapsed={railCollapsed} style:--rail-width={`${railWidth}px`}>

  <!-- On a desktop the project list is Home's left column. On a tablet or phone it lives on the Sessions screen, and Home
       keeps only what adds a project from the launch sentence. -->
  {#if writer}
    <ProjectRailHost bind:this={host} rail={!narrow} {projects} {groups} failed={projectsFailed} {savedWaiting} {sessions} {sessionsFailed} reporting={machineReporting} {running} known={sessions !== undefined} {worktrees}
      chosen={launchProject ?? chosen} {selecting} {selected} onselecting={setSelecting} ontoggle={toggleSelected} onchoose={(project) => chooseProject(project)}
      {scope} onscope={(next) => (scope = next)} collapsed={railCollapsed} oncollapsed={setRailCollapsed} {userId}
      width={railWidth} onwidth={setRailWidth} onwidthcommit={commitRailWidth} onpick={(project) => { chooseProject(project); scope = { kind: 'project', id: project.id } }}
      onnew={(project, opener) => { notice = null; projectStart = { project, opener } }}
      signal={bounded} onsettle={settle} onsettlegroups={settleGroups} onwaiting={(n) => (savedWaiting = n)} onmachines={(m) => (terminals = m)} onnotice={(n) => (notice = n)} onreload={() => void load(true)}
      onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }}>
      {#snippet selectAction()}
        {#if selected.length >= 2}<button type="button" class="start-chosen" onclick={() => (launchOpen = true)}>Start in these projects</button>{/if}
      {/snippet}
    </ProjectRailHost>
  {/if}
  <div class="column">
    <header class="workspace-heading" class:writer>
      <h1>Home</h1>
      {#if writer}
        <p class="counts">{countsLine}</p>
        <span class="break" aria-hidden="true"></span>
        {#if narrow && (projects ?? []).length}<span class="picker"><ProjectPicker projects={projects ?? []} counts={projectCounts} chosen={scope?.kind === 'project' ? scope.id : ''} group={scope?.kind === 'group' ? scopeName : ''} onchoose={id => (scope = id ? { kind: 'project', id } : null)}/></span>{/if}
        <span class="plan-slot"><FleetPlanMeters machines={terminals} caption="Plans" max={3}/></span>
        {#if !workbenchVisible}<div class="home-controls">
          {#if accounts}<button type="button" class="btn home-outcome" onclick={event => { notice = null; factoryStart = { opener: event.currentTarget } }}>New Factory outcome</button>{/if}
          <button type="button" class="btn primary home-start" onclick={event => { notice = null; projectStart = { opener: event.currentTarget } }}>New session</button>
        </div>{/if}
      {:else}
        <p class="welcome-note">Your sessions and plans.</p>
        <div class="workspace-summary"><span>{connected.length} {connected.length === 1 ? 'machine' : 'machines'} connected</span></div>
      {/if}
    </header>
    <div class="live" role="status" aria-live="polite">{#if movedShown}<p class="notice">{writer ? (narrow ? 'Projects now live on the Sessions screen.' : 'Projects now live in the rail on Home.') : 'Projects are on Home for accounts that can start sessions.'}</p>{/if}{#if problem}<p class="problem">{problem}</p>{/if}</div>
    {#if writer}
      <div class="live" role="status" aria-live="polite">{#if notice}<p class="notice" class:bad={notice.bad}>{notice.text}{#if notice.href}{' '}<a href={notice.href}>Open it</a>{/if}</p>{/if}{#if waitingNotice}<p class="vh">{waitingNotice}</p>{/if}</div>
      <ListSection name="Waiting for you" count={waiting.length} summary={waitingSummary(waiting, now)} open={waitingOpen} ontoggle={toggleWaiting}>
        {#if waiting.length}
          <ul class="waiting-list" class:rows={!narrow} class:cards={narrow} bind:this={waitingList}>
            {#each waiting.slice(0, waitingLimit) as entry (entry.id)}<li>{@render waitingEntry(entry)}</li>{/each}
          </ul>
          {#if waiting.length > WAITING_PAGE}<div class="attention-window"><span>Showing {Math.min(waitingLimit, waiting.length)} of {waiting.length}</span><button type="button" onclick={() => waitingLimit = waitingLimit < waiting.length ? waitingLimit + WAITING_PAGE : WAITING_PAGE}>{waitingLimit < waiting.length ? `Show ${Math.min(WAITING_PAGE, waiting.length - waitingLimit)} more` : 'Show fewer'}</button>{#if waitingLimit < waiting.length}<button type="button" onclick={async event => { const panel = event.currentTarget.parentElement; waitingLimit = waiting.length; await tick(); panel?.querySelector('button')?.focus() }}>View all {waiting.length}</button>{/if}</div>{/if}
        {:else}<p class="none">{factoryFailed ? 'Nothing that could be read is waiting for you.' : 'Nothing is waiting for you.'}</p>{/if}
      </ListSection>
      {#if working.length}
        <ListSection name="Factory, working without you" count={working.length} summary={workingSummary(working)} open={!!opened.working} ontoggle={() => toggleSection('working')}>
          <ul class="working-list" class:rows={!narrow}>
            {#each working as item (item.id)}{@const caption = stageCaption(item.nodes, item.at, 'dots')}
              <li><a class="working-row" href={pipelineHref(item)} aria-label={`${item.title}, ${item.chip.toLowerCase()}${caption ? `: ${caption}` : ''}. For: ${item.objective}. Open it on the Factory page`}>
                <span class="chipcell"><span class="chip" class:good={item.kind === 'running'}>{item.chip}</span></span>
                <span class="text"><strong class="title">{item.title}</strong>{' '}<span class="where" title={item.objective}>· {short(item.objective)}</span></span>
                <span class="stagecell"><StagePipeline nodes={item.nodes} at={item.at} size="dots" label={`Stages of ${item.title}`}/></span>
              </a></li>
            {/each}
          </ul>
          <p class="factory-foot"><a href="/factory">Open the Factory</a></p>
        </ListSection>
      {/if}
      <div class="home-views" role="group" aria-label="Home view">
        <button type="button" aria-pressed={homeView === 'continue'} onclick={() => chooseHomeView('continue')}>Continue working</button>
        <button type="button" bind:this={planningButton} aria-pressed={homeView === 'board'} onclick={() => chooseHomeView('board')}>Planning board</button>
      </div>
      {#if homeView === 'continue'}<div>
        {#if scope}<p class="continue-scope">{scopeName} <button type="button" class="btn" onclick={() => (scope = null)}>Show all projects</button></p>{/if}
        {#if showWorkbench}<StartWorkbench factory={accounts} onchoose={startFromWorkbench} onplan={() => void planFromWorkbench()} />{:else}<InFlight heading="Continue working" {factory} groups={continueGroups} {owners} machines={health ?? []} offline={continueOffline} {now} loaded={sessions !== undefined} failed={sessionsFailed}
          groupBy={continueGroup} onchoose={value => (continueGroup = value)} {projectColours} terminals={terminals ?? []} {removeDefault} {factorySessionEndDefault}
          onchanged={() => load(true)} onnotice={value => (notice = value)} onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }} />{/if}
      </div>
      {/if}
      <div hidden={homeView !== 'board'}>
      <SessionBoard bind:this={board} embedded {factory} {userId} sessions={listed} {sessionsFailed} projects={projects ?? []} machines={terminals ?? []} health={health ?? []} {profile} {now} initialProject={launchProject ?? chosen} {removeDefault} {factorySessionEndDefault} onbacklog={count => (backlogCount = count)}
        scopeIds={scopedIds} scopeKind={scope?.kind} scopeName={scopeName} onclearscope={() => (scope = null)}
        onchanged={() => load(true)} onstart={(opener) => startNewSession(opener)} onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }} />
      </div>
      {#if factoryStart && accounts}<GafferOutcome room={Math.max(0, 10 - (followups?.goals ?? []).filter(row => row.goal.state === 'active').length)} {userId} onstarted={() => load(true)} onclose={() => void closeFactoryStart()} />{/if}
      <ListSection name="Your machines" region={false} count={`${connected.length} of ${cards.length} connected`} summary={connected.map(card => card.name).join(', ')} open={!!opened.machines} ontoggle={() => toggleSection('machines')}>
        <FleetStrip {cards} sessions={sessions !== undefined} addHref={canAdd ? machinesAt(accounts ? 'add-machine' : 'connect-machine') : null} loaded={health !== undefined}
          onnew={newSessionOn} onproject={newProjectOn} onterminal={(card, opener) => void startTerminalOn(card.agentId, opener)} />
      </ListSection>
      <PipelineChoices bind:this={choices} bind:busy {userId} {timeZone} {dateFormat} goals={byConversation} {latest} onchanged={async () => { await load(true); await keepWaitingFocus() }} onnotice={(text, bad = false) => { notice = text ? { text, bad } : null }} onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }}/>
      {#if projectStart}
        <NewSessionChoice project={projectStart.project?.name ?? ''} machine={projectStart.project ? projectStart.project.machine || terminals?.find(m => m.agentId === projectStart?.project?.host)?.host || '' : projectStart.machine ?? ''} factory={accounts}
          onchoose={chooseProjectStart} onclose={() => { const opener = projectStart?.opener; projectStart = null; opener?.focus() }} />
      {/if}
      {#if terminalPick}
        <WorkspaceDialog title="New terminal session" onclose={() => { const opener = terminalPick?.opener; terminalPick = null; opener?.focus() }}>
          <p class="pick-copy">Choose the machine the terminal runs on.</p>
          <div class="pick-machines">{#each terminals ?? [] as item (item.agentId)}{@const why = readiness(item)}<button type="button" class="pick-machine" disabled={!!why} onclick={() => { const opener = terminalPick?.opener; terminalPick = null; void startTerminalOn(item.agentId, opener) }}><strong>{item.host}</strong><span>{why || 'Ready'}</span></button>{/each}</div>
        </WorkspaceDialog>
      {/if}
      {#if agentStart && terminals}
        <StartAgentDialog machines={terminals} snapshot={sync?.machines ?? []} projects={projects ?? []}
          initialAgent={agentStart.project?.host ?? agentStart.agentId ?? ''} projectId={agentStart.project?.id ?? ''}
          opener={agentStart.opener} onstarted={started} onleavefailed={leaveFailed}
          onaddmachine={() => location.assign(machinesAt(accounts ? 'add-machine' : 'connect-machine'))}
          onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }}
          onclose={() => (agentStart = null)} />
      {/if}
      {#if launchOpen}
        <LaunchDialog {projects} {projectsFailed} machines={terminals} {sessions} {profile} initialProject={chosen} selection={selecting ? selected : []}
          ready={projects !== undefined && (sessions !== undefined || sessionsFailed) && profileRead}
          onproject={(id) => (launchProject = id)} onstarted={started} onrun={() => void load(true)} onrundone={() => { if (selecting) setSelecting(false) }} onleavefailed={leaveFailed} onaddproject={openAdd}
          onprojectadded={(p) => { if (projects && !projects.some(x => x.id === p.id)) settle([...projects, p]) }}
          onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }} onclose={() => (launchOpen = false)} />
      {/if}
    {:else}
      <FleetStrip {cards} sessions={false} addHref={canAdd ? machinesAt(accounts ? 'add-machine' : 'connect-machine') : null} loaded={health !== undefined} />
    {/if}
  </div>
</div>

<style>
  /* Skills New on a machine: one row opening their review on the Skills page. */
  .new-skills { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px; min-height: 44px; box-sizing: border-box; padding: 14px 16px; color: var(--ink); text-decoration: none; overflow-wrap: anywhere; }
  .new-skills:hover .new-skills-text { text-decoration: underline; text-underline-offset: 2px; }
  .new-skills:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; border-radius: 10px; }
  .new-skills-kind { flex: none; padding: 2px 10px; border-radius: 999px; border: 1px solid var(--accent); color: var(--accent-text); font-size: 0.875rem; font-weight: 600; white-space: nowrap; }
  .new-skills-text { min-width: 0; font-weight: 700; }
  .new-skills-ask { min-width: 0; color: var(--muted); font-size: 0.875rem; }
  /* The head: the title and one line of counts, then the plans and the two ways to start something. On a narrow screen
     the title and the buttons share the first line and the Projects button, the plans and the counts the second. */
  .workspace-heading { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 6px 16px; padding: 8px 0 12px; }
  .workspace-heading.writer { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; }
  .workspace-heading h1 { font: 500 2rem/1.2 var(--serif); margin: 0; }
  .workspace-heading:not(.writer) h1 { grid-column: 1; }
  .counts { flex: 1 1 240px; min-width: 0; margin: 0; color: var(--muted); font-size: 0.9375rem; font-variant-numeric: tabular-nums; }
  .break { display: none; }
  .picker { display: inline-flex; }
  .welcome-note { grid-column: 1 / -1; color: var(--muted); font-size: 14px; margin: 0; }
  .home-controls { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .home-controls .btn { min-height: 44px; padding: 8px 14px; border-radius: 8px; font: 500 14px var(--sans); white-space: nowrap; }
  @media (max-width: 1099px) {
    .workspace-heading.writer > h1 { order: 0; flex: 1 1 auto; }
    .workspace-heading.writer > .home-controls { order: 1; }
    .workspace-heading.writer > .break { order: 2; display: block; flex: 0 0 100%; height: 0; }
    .workspace-heading.writer > .picker { order: 3; }
    .workspace-heading.writer > .plan-slot { order: 4; }
    /* Beside the Projects button, wrapping within its own room rather than taking a line of its own. */
    .workspace-heading.writer > .counts { order: 5; flex: 1 1 160px; }
  }
  .attention-window { display: flex; flex-wrap: wrap; gap: 4px 16px; align-items: center; font-size: 14px; color: var(--muted); }
  .attention-window button { min-height: 44px; padding: 4px 0; background: transparent; border: 0; color: var(--ink); font: inherit; text-decoration: underline; cursor: pointer; }
  .attention-window button:focus-visible { outline: 2px solid var(--accent); }
  .workspace-summary { grid-column: 1 / -1; display: flex; flex-wrap: wrap; gap: 6px 18px; color: var(--muted); font-size: 14px; font-variant-numeric: tabular-nums; }
  /* What is left of each agent's plan; nothing is drawn, and no gap kept, where there are no figures. */
  .plan-slot { display: inline-flex; align-items: center; }
  .plan-slot:empty, .plan-slot:not(:has(*)) { display: none; }
  /* The header is 61 px on a desktop, 65 px on a tablet and 57 px on a phone (measured at each breakpoint). These only set the
     column's minimum height; if the header changes, the cost is a small gap or a little extra scroll, never an overlap. */
  .home { line-height: 1.3; --top: 61px; display: grid; grid-template-columns: minmax(0, 1fr); }
  .home :global(p) { line-height: 1.55; }
  /* On a wide desktop the project rail is Home's left column: 301 px by default, as wide as the person drags it, or its
     56 px strip while folded. The rail and the board are independent: either folds without narrowing the other. */
  .home.writer { grid-template-columns: var(--rail-width, 301px) minmax(0, 1fr); }
  .home.writer.rail-collapsed { grid-template-columns: 56px minmax(0, 1fr); }
  /* Name the two columns explicitly: the rail and the board are the page's two grid items, and an absolute heading
     must never leave either of them waiting for the next row. */
  .home.writer :global(aside.rail .card:not(.chosen):not(.scoped):not(.picked)) { border-color: transparent; border-radius: 8px; background: transparent; }
  .home.writer :global(aside.rail .card:not(.chosen):not(.scoped):not(.picked):hover) { background: var(--line-soft); }
  .home.writer :global(aside.rail .strip) { height: calc(100dvh - var(--top)); min-height: 0; overflow-y: auto; scrollbar-width: thin; }
  .home.writer :global(aside.rail .strip > button) { flex-shrink: 0; }
  .home.writer > :global(aside.rail) { grid-column: 1; grid-row: 1; min-width: 0; }
  .home.writer > .column { grid-column: 2; grid-row: 1; }
  .home:not(.writer) > .column { grid-column: 1; grid-row: 1; }
  .column { min-width: 0; display: flex; flex-direction: column; gap: 12px; padding: 16px 24px 0; min-height: calc(100dvh - var(--top) - var(--bottom-chrome, 0px)); }
  .home:not(.writer) .column { padding-bottom: 36px; }
  .pick-copy { margin: 0 0 12px; color: var(--muted); }
  .pick-machines { display: grid; gap: 8px; }
  .pick-machine { display: grid; gap: 4px; min-height: 44px; text-align: left; padding: 12px 16px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font: inherit; cursor: pointer; }
  .pick-machine:hover:not(:disabled), .pick-machine:focus-visible { border-color: var(--accent); }
  .pick-machine:disabled { cursor: default; opacity: 0.7; }
  .pick-machine span { font-size: 0.875rem; color: var(--muted); }
  .problem, .notice { margin: 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); }
  .problem { border-color: var(--warn); }
  .notice { border-color: var(--good); }
  .notice.bad { border-color: var(--bad); }
  .home-views { display: flex; flex-wrap: wrap; gap: 4px; border-bottom: 1px solid var(--line); padding-bottom: 8px; margin-bottom: 4px; }
  .home-views button { min-height: 44px; padding: 8px 14px; border: 1px solid transparent; border-radius: 8px; background: none; color: var(--muted); font: 500 14px var(--sans); cursor: pointer; }
  .home-views button[aria-pressed=true] { border-color: var(--line); background: var(--card); color: var(--ink); }
  .home-views button:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
  .continue-scope button { min-height: 44px; }
  .continue-scope { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; color: var(--muted); }
  /* What waits and what works, in one frame each on a wide screen: rows share their columns (chip, text, stages, buttons)
     across Factory and session rows and across both lists, and the frame draws the lines between them. On a narrow
     screen each item is a card of its own. */
  .column { --col-chip: 124px; --col-stages: 180px; --col-actions: 248px; }
  .waiting-list, .working-list { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: minmax(0, 1fr); min-width: 0; }
  .waiting-list.rows, .working-list.rows { border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .waiting-list.rows > li + li, .working-list > li + li { border-top: 1px solid var(--line-soft); }
  .waiting-list.cards { gap: 10px; }
  .none { margin: 0; padding: 4px 4px 8px; color: var(--muted); }
  .working-list:not(.rows) { border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .working-row { display: grid; grid-template-columns: var(--col-chip) minmax(0, 1fr) var(--col-stages) var(--col-actions); column-gap: 16px; align-items: start; padding: 12px 16px; color: var(--ink); text-decoration: none; border-radius: 12px; }
  .working-row:hover .title { text-decoration: underline; }
  .working-row:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  .working-row .chipcell { align-self: start; }
  .working-row .text { min-width: 0; overflow-wrap: anywhere; line-height: 1.4; }
  .working-row .where { color: var(--muted); font-size: 0.875rem; }
  .working-row .stagecell { min-width: 0; }
  .chip { display: inline-block; padding: 2px 10px; border-radius: 999px; border: 1px solid var(--line-strong); font-size: 0.875rem; font-weight: 600; white-space: nowrap; }
  .chip.good { border-color: var(--good); color: var(--good); }
  .working-list:not(.rows) .working-row { grid-template-columns: minmax(0, 1fr); row-gap: 6px; }
  .working-list:not(.rows) .working-row .chipcell { justify-self: start; }
  .factory-foot { margin: 0; padding: 0 4px; font-size: 14px; }
  .factory-foot a { display: inline-flex; align-items: center; min-height: 36px; color: var(--accent-text); font-weight: 600; }
  @media (pointer: coarse) { .factory-foot a { min-height: 44px; } }
  .start-chosen { min-height: 34px; padding: 4px 12px; border: 1px solid var(--accent); border-radius: 8px; background: var(--accent); color: var(--accent-ink); font: 600 14px var(--sans); cursor: pointer; }
  /* The live regions are always present, so a sentence put into one is announced; empty, they take no room. */
  .live:empty { position: absolute; width: 1px; height: 1px; overflow: hidden; }

  @media (max-width: 1239px) {
    .home.writer, .home.writer.rail-collapsed { grid-template-columns: 56px minmax(0, 1fr); }
  }
  @media (max-width: 1099px) {
    .home.writer > .column { grid-column: 1; }
  }
  /* Tablet: machines, Needs you, then In flight, one column; the project rail is on the Sessions screen. */
  @media (max-width: 1099px) {
    .home { font-size: 15px; --top: 65px; }
    .home.writer, .home.writer.rail-collapsed { grid-template-columns: minmax(0, 1fr); }
  }
  /* Phone: one column with 16 px gutters; the launch bar sits above the tab bar. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .home { --top: 57px; }
    .column { padding: 12px 16px 0; gap: 12px; }
  }
</style>
