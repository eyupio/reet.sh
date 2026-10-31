<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { getProfile, getSyncState, UnauthorizedError, type SyncState } from '../api'
  import { createTerminal, folderProjectList, machineHealth, markSessionSeen, readiness, terminalMachines, folderProjectForLegacy, sessionHref, terminalSessions, TerminalError,
    type FolderProject, type MachineHealth, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { notStartedText, refusalText } from '../agentLaunch'
  import { announceWaiting, attentionCount, fleetCards, handedProjects, reporting, needsYou, projectRunning, projectWorktrees, sessionOwners, type FleetCard, type HomeSession, type NeedCard } from '../home'
  import type { LaunchDefaults } from '../launchBar'
  import LaunchBar from '../launch/LaunchBar.svelte'
  import FleetStrip from './FleetStrip.svelte'
  import SessionBoard from './SessionBoard.svelte'
  import NeedsYou from './NeedsYou.svelte'
  import ProjectRailHost from './ProjectRailHost.svelte'
  import NewSessionDialog from './NewSessionDialog.svelte'

  // Writers get the persistent session board, project rail and quick launch bar.
  // SessionBoard owns backlog reads; these loaders supply sessions and the fleet.
  // Both refresh every 15 seconds and retain the last successful read on failure.
  let { userId, writer, canAdd, accounts, moved = false, legacyProject = '', onsignedout, onattention }: {
    userId: number
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
    /** The Needs you count and cards, for the bell and document title. */
    onattention?: (count: number, cards: NeedCard[], onseen: (sessionId: string) => Promise<void>, onanswered: () => Promise<void>) => void
  } = $props()

  const POLL_MS = 15_000
  // The Machines page opens Add a machine (or the connect steps) when it arrives with this fragment.
  const machinesAt = (fragment: string) => `/machines#${fragment}`

  let health = $state<MachineHealth[] | undefined>()
  let sessions = $state<HomeSession[] | undefined>()
  let sync = $state<SyncState | undefined>()
  let projects = $state<FolderProject[] | undefined>()
  // The terminal machines (with readiness and launch capabilities), read with the rest for the launch bar and every start.
  let terminals = $state<TerminalMachine[] | undefined>()
  let savedWaiting = $state(0)
  // The project an address chose (/?project=<id>): its card is marked and the launch bar opens on it.
  let chosen = $state(new URLSearchParams(location.search).get('project') ?? '')
  // The account's launch defaults (base branch, worktrees by default), read once; the bar waits for the answer or its failure.
  let profile = $state<LaunchDefaults>({}), profileRead = $state(false)
  // The account's choice for a session that ends: remove it from history too. Read with the launch defaults.
  let removeDefault = $state(false)
  let bar = $state<LaunchBar>()
  let quickLaunch = $state(!!new URLSearchParams(location.search).get('project'))
  let now = $state(Date.now())
  let problem = $state('')
  let readOnce = false
  const life = new AbortController()
  const bounded = () => AbortSignal.any([life.signal, AbortSignal.timeout(POLL_MS)])

  const emptySync: SyncState = { environments: [], targets: [], agents: [], jobs: [] }
  const cards = $derived(fleetCards(health ?? [], sessions ?? [], sync ?? emptySync, now, terminals ?? []))
  const owners = $derived(sessionOwners(sessions ?? [], projects ?? []))
  const running = $derived(projectRunning(sessions ?? []))
  const worktrees = $derived(projectWorktrees(sessions ?? []))
  const machineReporting = $derived(new Map(cards.map(c => [c.agentId, reporting(c)])))
  const needs = $derived(needsYou(sessions ?? [], now, owners, health ?? []))

  async function markSeen(sessionId: string) {
    sessions = (sessions ?? []).map(s => s.id === sessionId ? { ...s, changedSinceSeen: false, attention: s.attention === 'waiting' ? 'working' : s.attention } : s)
    try {
      await markSessionSeen(sessionId, bounded())
      void load(true)
    } catch (e) {
      if (refused(e, true)) { life.abort(); onsignedout(new UnauthorizedError()) }
    }
  }

  const answered = async () => { await load(true) }

  $effect(() => { onattention?.(attentionCount(needs), needs, markSeen, answered) })

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
  let sessionsFailed = $state(false), projectsFailed = $state(false)

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
      const [h, y, s, p, t] = await Promise.allSettled([
        healthReadable ? machineHealth(bounded()) : Promise.resolve(undefined),
        getSyncState(bounded()),
        writer ? terminalSessions(bounded()) : Promise.resolve(undefined),
        writer ? folderProjectList(bounded()) : Promise.resolve(undefined),
        // The launch bar's machines: readiness and launch capabilities change while Home stays open, so they are read with the rest.
        writer ? terminalMachines(bounded()) : Promise.resolve(undefined),
      ])
      if (stale()) return
      if ([h, y, s, p, t].some(r => r.status === 'rejected' && refused(r.reason, r !== y))) return signOut()
      const at = Date.now()
      let failed = false
      if (y.status === 'fulfilled') sync = y.value; else failed = true
      if (h.status === 'rejected' && h.reason instanceof TerminalError && h.reason.status === 403 && health === undefined) healthReadable = false
      if (h.status === 'fulfilled' && h.value) health = h.value.machines ?? []
      else if (!healthReadable) { if (y.status === 'fulfilled') health = fromAgents(y.value, at) }
      // A health read that failed for a passing reason keeps the cards as last read.
      else failed = true
      if (writer) {
        if (s.status === 'fulfilled' && s.value) { sessions = s.value; sessionsFailed = false } else { failed = true; sessionsFailed = sessions === undefined }
        if (p.status === 'fulfilled' && p.value) { projects = p.value.projects; savedWaiting = p.value.savedWaiting ?? 0; projectsFailed = false } else { failed = true; projectsFailed = projects === undefined }
        if (t.status === 'fulfilled' && t.value) terminals = t.value; else failed = true
      }
      now = at
      problem = !failed ? '' : readOnce ? "Couldn't refresh just now. Showing what was last read." : "Couldn't read everything just now. Home tries again every 15 seconds."
      readOnce = true
    } finally {
      if (mine === generation) loading = false
    }
  }

  // Tablet and phone widths leave the rail to the Sessions screen (the phone rail, PR H): Home keeps machines, Needs you,
  // In flight and the launch bar there, and the launch sentence's project slot still chooses a project.
  const NARROW = '(max-width: 1099px)'
  let narrow = $state(typeof matchMedia === 'function' && matchMedia(NARROW).matches)

  let notice = $state<{ text: string; href?: string; bad?: boolean } | null>(null)
  // The rail and what its cards do (ProjectRailHost); the launch bar's project slot opens its Add project too.
  let host = $state<ProjectRailHost>()
  function openAdd(opener: HTMLElement) { void host?.openAdd(opener) }
  // Choosing a card (or New session here) puts that project in the launch bar's sentence and moves to the prompt; the rail
  // marks the project the sentence names.
  // The project the launch bar names ('' for none: the home folder or several projects); null until the bar has a choice.
  let launchProject = $state<string | null>(null)
  function chooseProject(project: FolderProject) { quickLaunch = true; void tick().then(() => bar?.choose(project)) }
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
    if (ids.length) { selecting = true; selected = ids }
    const url = new URL(location.href)
    url.searchParams.delete('projects')
    history.replaceState(history.state, '', `${url.pathname}${url.search}${url.hash}`)
  })
  // A write's answer is the list as it now is (or a project in it): it replaces what was read, superseding a poll in flight.
  function settle(next: FolderProject[]) { generation++; loading = false; projects = next }
  // An old /projects/<id> address: its folder project, if one was made from it, is chosen and named in the address.
  async function findLegacy(id: string) {
    try {
      const found = await folderProjectForLegacy(id, bounded())
      if (!found || life.signal.aborted) return
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
  // it, or a plain terminal. A project then starts through the launch bar, so its defaults, refusals and navigation are the
  // same as the bar's Start.
  let chooser = $state<{ card: FleetCard; opener: HTMLElement } | null>(null)
  function newSessionOn(card: FleetCard, opener: HTMLElement) { notice = null; chooser = { card, opener } }
  // New project opens the rail's Add project dialog with this machine already chosen.
  function newProjectOn(card: FleetCard, opener: HTMLElement) { notice = null; void host?.openAdd(opener, card.agentId) }
  function startProjectNow(project: FolderProject) {
    chooser = null
    quickLaunch = true
    if (!bar) { notice = { text: 'Home is still opening. Try again in a moment.', bad: true }; return }
    void bar.startProject(project)
  }
  // A terminal session is a shell on the machine: no agent, no prompt, no project. It goes straight to the new session.
  // A request whose reply was lost is kept per machine and sent again with the same request id, exactly as the launch
  // bar's Start does, so choosing New terminal twice can never make a second shell.
  type ShellRequest = { requestId: string; title: string }
  let shellRequests = $state<Record<string, ShellRequest>>({})
  let shellStarting = ''
  function forgetShell(agentId: string) { const next = { ...shellRequests }; delete next[agentId]; shellRequests = next }
  async function startTerminalOn(card: FleetCard, opener?: HTMLElement | null) {
    chooser = null
    if (shellStarting) return
    const why = readiness(terminals?.find(m => m.agentId === card.agentId))
    if (why) { notice = { text: why, bad: true }; opener?.focus(); return }
    const request = shellRequests[card.agentId] ?? { requestId: crypto.randomUUID(), title: 'Shell' }
    if (!shellRequests[card.agentId]) shellRequests = { ...shellRequests, [card.agentId]: request }
    shellStarting = card.agentId
    notice = null
    try {
      const session = await createTerminal(card.agentId, request.requestId, request.title, 100, 30, bounded())
      if (life.signal.aborted) return
      const notStarted = notStartedText(session.state)
      if (notStarted) { forgetShell(card.agentId); notice = { text: notStarted, bad: true }; return }
      forgetShell(card.agentId)
      sessions = [session, ...(sessions ?? []).filter(s => s.id !== session.id)]
      location.assign(sessionHref(session))
    } catch (e) {
      if (life.signal.aborted) return
      if (e instanceof UnauthorizedError || (e instanceof TerminalError && e.status === 401)) { life.abort(); onsignedout(new UnauthorizedError()); return }
      if (e instanceof TerminalError && [400, 403, 404, 409, 422].includes(e.status)) {
        forgetShell(card.agentId)
        notice = { text: refusalText(e.status, e.code, e.message), bad: true }
      } else {
        notice = { text: "We didn't hear back, so the terminal may have started. Choosing New terminal again asks about the same request.", bad: true }
      }
    } finally { shellStarting = '' }
  }

  // The redirect note goes into its live region after the page is drawn, so it is announced.
  let movedShown = $state(false)
  onMount(() => {
    if (moved) void tick().then(() => { movedShown = true })
    void load()
    // The profile read gives up after 10 seconds, so the bar never waits on it for good (it then uses each project's own base).
    // Once Home has gone, a late answer changes nothing and the timer is cleared.
    let giveUpTimer: ReturnType<typeof setTimeout> | undefined
    if (writer) Promise.race([getProfile(), new Promise<never>((_, no) => { giveUpTimer = setTimeout(() => no(new Error('timeout')), 10_000) })])
      .then(p => { if (!life.signal.aborted) { profile = { defaultBaseBranch: p.profile.defaultBaseBranch, worktreesByDefault: p.profile.worktreesByDefault, defaultTool: p.profile.defaultTool, defaultModel: p.profile.defaultModel, defaultEffort: p.profile.defaultEffort }; removeDefault = p.profile.terminalRemoveOnClose ?? false } }, () => {})
      .finally(() => { clearTimeout(giveUpTimer); if (!life.signal.aborted) profileRead = true })
    if (writer && legacyProject) void findLegacy(legacyProject)
    const query = matchMedia(NARROW), resized = () => { narrow = query.matches }
    query.addEventListener('change', resized)
    const timer = setInterval(() => { if (document.visibilityState === 'visible') void load() }, POLL_MS)
    const shown = () => { if (document.visibilityState === 'visible') void load() }
    document.addEventListener('visibilitychange', shown)
    // Back from the session the person just started: a page restored from the browser's cache shows lists as they were, so
    // it reads everything again. (The launch bar unlocks itself.)
    const restored = (e: PageTransitionEvent) => { if (e.persisted) void load(true) }
    window.addEventListener('pageshow', restored)
    return () => { clearTimeout(giveUpTimer); query.removeEventListener('change', resized); clearInterval(timer); document.removeEventListener('visibilitychange', shown); window.removeEventListener('pageshow', restored); life.abort() }
  })
</script>

<div class="home" class:writer>
  <h1 class="vh">Home</h1>
  <!-- On a desktop the rail is the left column; on a tablet or phone it lives on the Sessions screen, and Home keeps only what
       adds a project from the launch sentence. -->
  {#if writer}
    <ProjectRailHost bind:this={host} rail={!narrow} {projects} failed={projectsFailed} {savedWaiting} {sessions} {sessionsFailed} reporting={machineReporting} {running} known={sessions !== undefined} {worktrees}
      chosen={launchProject ?? chosen} {selecting} {selected} onselecting={setSelecting} ontoggle={toggleSelected} onchoose={(project) => chooseProject(project)}
      onnew={startProjectNow}
      signal={bounded} onsettle={settle} onwaiting={(n) => (savedWaiting = n)} onmachines={(m) => (terminals = m)} onnotice={(n) => (notice = n)} onreload={() => void load(true)}
      onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }} />
  {/if}
  <div class="column">
    <div class="live" role="status" aria-live="polite">{#if movedShown}<p class="notice">{writer ? (narrow ? 'Projects now live on the Sessions screen.' : 'Projects now live in the rail on Home.') : 'Projects are on Home for accounts that can start sessions.'}</p>{/if}{#if problem}<p class="problem">{problem}</p>{/if}</div>
    {#if writer}
      <div class="live" role="status" aria-live="polite">{#if notice}<p class="notice" class:bad={notice.bad}>{notice.text}{#if notice.href}{' '}<a href={notice.href}>Open it</a>{/if}</p>{/if}{#if waitingNotice}<p class="vh">{waitingNotice}</p>{/if}</div>
      <SessionBoard {userId} {sessions} {sessionsFailed} projects={projects ?? []} machines={terminals ?? []} health={health ?? []} {profile} {now} initialProject={launchProject ?? chosen} {removeDefault}
        onchanged={() => load(true)} onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }} />
      {#if needs.length}
        <details class="home-detail attention-detail"><summary>Needs you <span>{needs.length}</span></summary><NeedsYou cards={needs} onseen={markSeen} onanswered={answered} /></details>
      {/if}
      <details class="home-detail"><summary>Your machines <span>{cards.filter(reporting).length} of {cards.length} connected</span></summary>
        <FleetStrip {cards} sessions={sessions !== undefined} addHref={canAdd ? machinesAt(accounts ? 'add-machine' : 'connect-machine') : null} loaded={health !== undefined}
          onnew={newSessionOn} onproject={newProjectOn} onterminal={startTerminalOn} />
      </details>
      {#if chooser && writer}
        <NewSessionDialog host={chooser.card.name} agentId={chooser.card.agentId} machine={terminals?.find(m => m.agentId === chooser?.card.agentId)} machinesLoaded={terminals !== undefined} projects={projects ?? []}
          loaded={projects !== undefined} onproject={startProjectNow}
          onterminal={() => { const target = chooser; if (target) void startTerminalOn(target.card, target.opener) }}
          onclose={() => { const opener = chooser?.opener; chooser = null; opener?.focus() }} />
      {/if}
      <details class="home-detail quick-launch" bind:open={quickLaunch}><summary>Quick launch <span>Start an agent now</span></summary>
      <LaunchBar bind:this={bar} {projects} {projectsFailed} machines={terminals} sessions={sessions} {profile} initialProject={chosen} selection={selecting ? selected : []}
        ready={projects !== undefined && (sessions !== undefined || sessionsFailed) && profileRead}
        onproject={(id) => (launchProject = id)} onstarted={started} onrun={() => void load(true)} onrundone={() => { if (selecting) setSelecting(false) }} onleavefailed={leaveFailed} onaddproject={openAdd} onsignedout={() => { life.abort(); onsignedout(new UnauthorizedError()) }} />
      </details>
    {:else}
      <FleetStrip {cards} sessions={false} addHref={canAdd ? machinesAt(accounts ? 'add-machine' : 'connect-machine') : null} loaded={health !== undefined} />
    {/if}
  </div>
</div>

<style>
  /* The header is 61 px on a desktop, 65 px on a tablet and 57 px on a phone (measured at each breakpoint). These only set the
     column's minimum height, so the docked launch bar starts at the foot of the first screen; if the header changes,
     the cost is a small gap or a little extra scroll, never an overlap. */
  .home { line-height: 1.3; --top: 61px; display: grid; grid-template-columns: minmax(0, 1fr); }
  .home :global(p) { line-height: 1.55; }
  /* The rail is the mock-up's 272 px of cards inside 14 px padding and a 1 px rule: 301 px. */
  .home.writer { grid-template-columns: 301px minmax(0, 1fr); }
  .column { min-width: 0; display: flex; flex-direction: column; gap: 18px; padding: 20px 24px 0; min-height: calc(100dvh - var(--top) - var(--bottom-chrome, 0px)); }
  .home:not(.writer) .column { padding-bottom: 36px; }
  .problem, .notice { margin: 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .problem { border-color: var(--warn); }
  .notice { border-color: var(--good); }
  .notice.bad { border-color: var(--bad); }
  .home-detail { border-top: 1px solid var(--line); padding: 14px 0; }
  .home-detail > summary { cursor: pointer; font-size: 12px; font-weight: 600; color: var(--ink-soft); }
  .home-detail > summary span { font-weight: 400; color: var(--muted); margin-left: 8px; font-size: 11px; }
  .home-detail[open] > summary { margin-bottom: 14px; }
  .attention-detail > summary { color: var(--warn); }
  .quick-launch :global(.launch) { position: relative; bottom: auto; margin: 0; }
  /* The live regions are always present, so a sentence put into one is announced; empty, they take no room. */
  .live:empty { position: absolute; width: 1px; height: 1px; overflow: hidden; }

  @media (max-width: 1239px) { .home.writer { grid-template-columns: 56px minmax(0, 1fr); } }
  /* Tablet: machines, Needs you, then In flight, one column; the project rail is on the Sessions screen. */
  @media (max-width: 1099px) {
    .home { font-size: 15px; --top: 65px; }
    .home.writer { grid-template-columns: minmax(0, 1fr); }
  }
  /* Phone: one column with 16 px gutters; the launch bar sits above the tab bar. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .home { --top: 57px; }
    .column { padding: 14px 16px 0; gap: 14px; }
  }
</style>
